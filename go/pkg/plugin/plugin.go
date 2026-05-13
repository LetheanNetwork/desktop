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
	"context"
	"sync"
	"time"

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
	mu    sync.RWMutex
	state map[string]*pluginState // keyed by code
	proxy *ProxyGroup
}

// NewService returns the canonical Core service factory.
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
		}
		return core.Ok(svc)
	}
}

// OnStartup is a no-op today — the plugin host registers no
// Core actions; everything goes through the Wails surface. Kept
// for future expansion (e.g. exposing `plugin.list` as a Core
// action so other services can enumerate installed plugins).
func (s *Service) OnStartup(context.Context) core.Result {
	return core.Ok(nil)
}

// OnShutdown stops every running plugin. Plugins ride out the
// host's process group death on SIGKILL anyway, but a clean
// SIGTERM here lets them flush state before going.
func (s *Service) OnShutdown(context.Context) core.Result {
	s.mu.Lock()
	codes := make([]string, 0, len(s.state))
	for code := range s.state {
		codes = append(codes, code)
	}
	s.mu.Unlock()
	for _, code := range codes {
		_ = s.Stop(code)
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
	Code       string    `json:"code"`
	Name       string    `json:"name"`
	Version    string    `json:"version"`
	Namespace  string    `json:"namespace"`
	State      string    `json:"state"` // "stopped" | "starting" | "running" | "dead"
	Port       int       `json:"port,omitempty"`
	PID        int       `json:"pid,omitempty"`
	StartedAt  time.Time `json:"started_at,omitempty"`
	StoppedAt  time.Time `json:"stopped_at,omitempty"`
	LastError  string    `json:"last_error,omitempty"`
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
	manifest  Manifest
	proc      *processHandle // nil when stopped; see runtime.go
	state     string
	port      int
	pid       int
	startedAt time.Time
	stoppedAt time.Time
	lastError string
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
func installRoot() (string, core.Result) {
	home := core.UserHomeDir()
	if !home.OK {
		return "", core.Fail(core.E("plugin.installRoot", "home dir unavailable", nil))
	}
	homeDir, _ := home.Value.(string)
	if homeDir == "" {
		return "", core.Fail(core.E("plugin.installRoot", "home dir empty", nil))
	}
	return core.PathJoin(homeDir, "Lethean", "conf", "plugins"), core.Ok(nil)
}

// pluginDir returns the install directory for one plugin.
func pluginDir(code string) (string, core.Result) {
	root, res := installRoot()
	if !res.OK {
		return "", res
	}
	return core.PathJoin(root, code), core.Ok(nil)
}
