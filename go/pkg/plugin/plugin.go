// SPDX-Licence-Identifier: EUPL-1.2

// Package plugin is the lthn-side plugin runtime. Owns the
// install dir under ~/Lethean/conf/plugins/<code>/, fetches and
// verifies plugin binaries from github.com/dappcore releases,
// supervises each plugin as a managed process via
// dappco.re/go/process, and mounts a reverse-proxy on the
// coreapi.Engine at /v1/api/plugin/<code>/* so plugins serve
// under lthn's same-origin HTTP surface.
//
// Design doc: docs/plugin-host-scope.md.
//
// Phase 1 (this commit): static lifecycle — Install / Start /
// Stop / Remove + List / Status + the proxy mount. Fetch is
// included (Phase 2 was folded in because a host without fetch
// can't be validated end-to-end). Supervision (crash-loop
// detection, restart backoff) and menu/UI integration are still
// follow-on phases.

package plugin

import (

	core "dappco.re/go"
)

// Options configures the plugin host. Empty today — kept for
// shape symmetry with other Core services + future expansion
// (install-root override, allowlist override, etc.).
type Options struct{}

// Service is the plugin host. Embeds *core.ServiceRuntime so it
// can resolve other Core services (process, apikey) at action
// time via s.Core(); the coreapi.Engine gets the proxy route
// group registered at boot in pkg/desktop.
type Service struct {
	*core.ServiceRuntime[Options]
	mu     core.RWMutex
	state  map[string]*pluginState // keyed by plugin code
	proxy  *ProxyGroup
	tokens map[string]string // per-bundle secret → plugin code (Cerberus #1443)
}

// NewService returns the canonical Core service factory.
//
// Usage example:
//
//	core.WithName("plugin", plugin.NewService(plugin.Options{}))
//
// The factory shape is what core.New expects; the underlying
// *Service is reachable later via
// core.ServiceFor[*plugin.Service](c, "plugin").
func NewService(opts Options) func(*core.Core) core.Result {
	return func(c *core.Core) core.Result {
		svc := &Service{
			ServiceRuntime: core.NewServiceRuntime(c, opts),
			state:          map[string]*pluginState{},
			proxy:          NewProxyGroup(),
			tokens:         map[string]string{},
		}
		return core.Ok(svc)
	}
}

// Register constructs the plugin service for Core registration.
//
// Usage example:
//
//	core.New(core.WithService(plugin.Register))
func Register(c *core.Core) core.Result {
	return NewService(Options{})(c)
}

// OnStartup is a no-op today — the plugin host registers no
// Core actions; everything goes through the Wails surface. Kept
// for future expansion (e.g. exposing `plugin.list` as a Core
// action so other services can enumerate installed plugins).
func (s *Service) OnStartup(core.Context) core.Result {
	return core.Ok(nil)
}

// OnShutdown stops every running plugin. Plugins ride out the
// host's process group death on SIGKILL anyway, but a clean
// SIGTERM here lets them flush state before going.
func (s *Service) OnShutdown(core.Context) core.Result {
	s.mu.Lock()
	codes := make([]string, 0, len(s.state))
	for code := range s.state {
		codes = append(codes, code)
	}
	s.mu.Unlock()
	for _, code := range codes {
		if r := s.Stop(code); !r.OK {
			return core.Fail(core.E("plugin.OnShutdown", "stop plugin", r.Value.(error)))
		}
	}
	return core.Ok(nil)
}

// coreApp returns the attached Core, or nil if the service
// hasn't been started yet. Mirrors process.Service's helper of
// the same name.
func (s *Service) coreApp() *core.Core {
	if s == nil || s.ServiceRuntime == nil {
		return nil
	}
	return s.Core()
}

// ProxyGroup exposes the (permanently-registered) reverse-proxy
// route group so pkg/desktop can hand it to the coreapi.Engine.
//
// Example:
//
//	engine.Register(pluginSvc.ProxyGroup())
func (s *Service) ProxyGroup() *ProxyGroup { return s.proxy }

// Status is the runtime state of one plugin. Returned by
// Status / List so the UI can show running/stopped indicators.
type Status struct {
	Code      string    `json:"code"`
	Name      string    `json:"name"`
	Version   string    `json:"version"`
	Namespace string    `json:"namespace"`
	State     string    `json:"state"` // "stopped" | "starting" | "running" | "dead"
	Port      int       `json:"port,omitempty"`
	PID       int       `json:"pid,omitempty"`
	StartedAt core.Time `json:"started_at,omitempty"`
	StoppedAt core.Time `json:"stopped_at,omitempty"`
	LastError string    `json:"last_error,omitempty"`
}

// InstalledPlugin is the manifest-derived summary returned by
// List — what's on disk, regardless of running state.
type InstalledPlugin struct {
	Code      string `json:"code"`
	Name      string `json:"name"`
	Version   string `json:"version"`
	Namespace string `json:"namespace"`
	Dir       string `json:"dir"`
	Status    Status `json:"status"`
}

// pluginState is the host's in-memory record for one running
// plugin. The exported Status struct is computed from this on
// demand so the UI sees a consistent shape.
type pluginState struct {
	manifest     Manifest
	proc         *processHandle // nil when stopped; see runtime.go
	state        string
	port         int
	pid          int
	startedAt    core.Time
	stoppedAt    core.Time
	lastError    string
	// bundleSecret is the per-launch gateway credential (Cerberus #1443).
	// Generated in startPlugin, passed to the binary via --bundle-token,
	// revoked in stopPlugin. Empty for plugins started before the fix.
	bundleSecret string
	// crashAt records timestamps of recent unexpected exits.
	// The supervisor (supervisor.go) trims this to a rolling
	// window and uses len() to decide restart-vs-give-up. v1
	// of the data structure is intentionally a flat slice —
	// at the 3-crash threshold the slice never grows past 4.
	crashAt []core.Time
}

// statusFor projects the current pluginState into the wire-shape
// Status. Caller holds the lock.
func (s *Service) statusFor(code string) Status {
	ps, ok := s.state[code]
	if !ok {
		return Status{Code: code, State: "stopped"}
	}
	return Status{
		Code:      code,
		Name:      ps.manifest.Name,
		Version:   ps.manifest.Version,
		Namespace: ps.manifest.Namespace,
		State:     ps.state,
		Port:      ps.port,
		PID:       ps.pid,
		StartedAt: ps.startedAt,
		StoppedAt: ps.stoppedAt,
		LastError: ps.lastError,
	}
}

// installDir returns the canonical install root —
// $HOME/Lethean/conf/plugins. Per the no-hidden-bloat principle
// (memory: design_no_hidden_user_bloat.md), plugins live in the
// visible Lethean tree, not under ~/.config or similar.
func installRoot() core.Result {
	home := core.UserHomeDir()
	if !home.OK {
		return core.Fail(core.E("plugin.installRoot", "home dir unavailable", nil))
	}
	homeDir, _ := home.Value.(string)
	if homeDir == "" {
		return core.Fail(core.E("plugin.installRoot", "home dir empty", nil))
	}
	return core.Ok(core.PathJoin(homeDir, "Lethean", "conf", "plugins"))
}

// pluginDir returns the install directory for one plugin.
func pluginDir(code string) core.Result {
	rootR := installRoot()
	if !rootR.OK {
		return rootR
	}
	root, _ := rootR.Value.(string)
	return core.Ok(core.PathJoin(root, code))
}

// ─── Per-bundle token registry (Cerberus Mantis #1443) ─────────────────
//
// Each running plugin is issued a unique secret at Start time and
// passes it as the X-Bundle-Token header on outgoing gateway requests.
// pkg/gateway resolves the secret here to derive the authoritative
// bundle code — ignoring the caller-asserted Bundle-ID header — so a
// plugin cannot claim another bundle's identity just by forging a header.

// registerBundleToken stores secret → code. Caller holds s.mu.
func (s *Service) registerBundleToken(code, secret string) {
	s.tokens[secret] = code
}

// revokeBundleToken removes a secret from the registry. Caller holds
// s.mu. Idempotent — missing entries are silently skipped.
func (s *Service) revokeBundleToken(secret string) {
	delete(s.tokens, secret)
}

// LookupBundleCode resolves a per-bundle secret to the plugin code
// that was issued that secret at Start time. Returns ("", false) when
// the secret is unknown or empty.
//
// The gateway calls this via the BundleCodeResolver interface in
// pkg/gateway — no direct import of pkg/plugin from pkg/gateway.
//
// Usage example:
//
//	code, ok := pluginSvc.LookupBundleCode(req.Header.Get("X-Bundle-Token"))
//	if ok { /* code is the authoritative bundle identity */ }
func (s *Service) LookupBundleCode(secret string) (string, bool) {
	if secret == "" {
		return "", false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	code, ok := s.tokens[secret]
	return code, ok
}
