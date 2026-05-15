// SPDX-Licence-Identifier: EUPL-1.2

package opencode

import (
	"bytes"
	goio "io"
	"net"
	"net/http"
	"sync"

	core "dappco.re/go"
	"dappco.re/go/orm"
	"dappco.re/go/process"
)

const (
	// defaultImage is the canonical OCI tag opencode runs inside.
	// lthn/dev:latest bakes opencode-ai in via npm install -g
	// (see core/images/developer/Dockerfile). Override per-host
	// by passing Options{Image: ...}.
	defaultImage = "lthn/dev:latest"

	// containerPort is opencode serve's bind port inside the
	// container. The host-side port is dynamic.
	containerPort = 4096

	startOp   = "opencode.Start"
	stopOp    = "opencode.Stop"
	inspectOp = "opencode.Inspect"
	statusOp  = "opencode.Status"
)

// Options configures the opencode host.
type Options struct {
	// Image overrides the default lthn/dev:latest OCI tag.
	Image string

	// Runtime overrides docker auto-detection ("docker", "podman").
	// Empty = "docker" (the v1 default; borg-run integration is a
	// future iteration that adds "lthn-vm" as the canonical option).
	Runtime string

}

// Service is the opencode host. Embeds *core.ServiceRuntime[Options]
// so process.Service can be resolved at call time + Options are
// typed.
type Service struct {
	*core.ServiceRuntime[Options]
	proxy *SandboxProxyGroup

	// onSandboxChange fires after every Start success + every Stop
	// success. Set via SetOnSandboxChange after the runner exists
	// — the wire-up happens in cmd/lthn after newAppCore returns.
	// Held outside Options because Options is read-only at runtime.
	mu              sync.RWMutex
	onSandboxChange func()

	// eventEmitter forwards opencode-serve's SSE /global/event
	// stream to the host application's event bus. Installed by
	// SetEventEmitter; nil = no consumer (CLI/serve modes), in
	// which case Subscribe is a no-op.
	eventEmitter EventEmitter

	// subscriptions maps sandbox-id → SSE-goroutine cancel func.
	// Created lazily by Subscribe so the zero-value Service is
	// still safe to use without subscription support.
	subscriptions map[string]func()
}

// NewService returns the canonical Core service factory.
//
// Usage example:
//
//	core.WithName("opencode", opencode.NewService(opencode.Options{}))
func NewService(opts Options) func(*core.Core) core.Result {
	return func(c *core.Core) core.Result {
		svc := &Service{
			ServiceRuntime: core.NewServiceRuntime(c, opts),
			proxy:          NewSandboxProxyGroup(),
		}
		// Seed the baseline profile so spawn always has a default
		// to apply via PATCH /config. Idempotent — skips when the
		// profile already exists in the duckdb store.
		if r := svc.SeedDefaultProfile(); !r.OK {
			return r
		}
		return core.Ok(svc)
	}
}

// Register constructs the opencode service for Core registration.
//
// Usage example:
//
//	core.New(core.WithService(opencode.Register))
func Register(c *core.Core) core.Result {
	return NewService(Options{})(c)
}

// ServiceName labels the binding namespace exposed to JS.
func (s *Service) ServiceName() string { return "OpenCode" }

// SetOnSandboxChange swaps the post-Start / post-Stop callback at
// runtime. cmd/lthn wires this from cmdServe after the runner
// exists — at construction time (inside newAppCore) the runner
// hasn't been built yet, so the callback can't be passed via
// Options.OnSandboxChange directly.
//
// Usage example:
//
//	opencodeSvc.SetOnSandboxChange(func() {
//	    runnerSvc.SetDynamicRoutes(opencodeSvc.Routes())
//	})
func (s *Service) SetOnSandboxChange(cb func()) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.onSandboxChange = cb
	s.mu.Unlock()
}

// fireSandboxChange runs the registered callback (if any) under
// the read lock so SetOnSandboxChange callers don't race with
// Start / Stop notifications.
func (s *Service) fireSandboxChange() {
	if s == nil {
		return
	}
	s.mu.RLock()
	cb := s.onSandboxChange
	s.mu.RUnlock()
	if cb != nil {
		cb()
	}
}

// ProxyGroup exposes the reverse-proxy route group so pkg/desktop
// can hand it to the coreapi.Engine at boot — mirrors the
// pkg/plugin.ProxyGroup() shape.
//
// Usage example:
//
//	engine.Register(opencodeSvc.ProxyGroup())
func (s *Service) ProxyGroup() *SandboxProxyGroup { return s.proxy }

// proc resolves the process service at call time. Returns nil when
// the service isn't registered (defensive — process is registered
// before opencode in cmd/lthn/app.go).
func (s *Service) proc() *process.Service {
	if s == nil || s.ServiceRuntime == nil {
		return nil
	}
	c := s.Core()
	if c == nil {
		return nil
	}
	ps, _ := core.ServiceFor[*process.Service](c, "process")
	return ps
}

// runtime returns the configured runtime name ("docker" default).
func (s *Service) runtime() string {
	rt := core.Trim(s.Options().Runtime)
	if rt == "" {
		return "docker"
	}
	return rt
}

// image returns the configured image (defaultImage when unset).
func (s *Service) image() string {
	img := core.Trim(s.Options().Image)
	if img == "" {
		return defaultImage
	}
	return img
}

// Start spawns a new opencode-serve container, persists the
// Sandbox record, registers the reverse-proxy target, waits for
// opencode-serve to be healthy, and applies the named profile via
// PATCH /config. Returns the sandbox ID once everything is ready.
//
// Synchronous — caller knows the sandbox is fully configured when
// Start returns. Total time is ~5-15s (image cached) for container
// boot + opencode-serve binding + config patch.
//
// profileName is the lthn-side opencode.Profile name to apply. Empty
// string falls back to DefaultProfile ("default"). The named profile
// must already exist in the store — SeedDefaultProfile is called at
// service startup so DefaultProfile is always available.
//
// Usage example:
//
//	r := svc.Start("code-review")
//	if r.OK { id := r.Value.(string); _ = id }
func (s *Service) Start(profileName string) core.Result {
	ps := s.proc()
	if ps == nil {
		return core.Fail(core.E(startOp, "process service unavailable", nil))
	}

	profileName = core.Trim(profileName)
	if profileName == "" {
		profileName = DefaultProfile
	}
	profileR := s.GetProfile(profileName)
	if !profileR.OK {
		return profileR
	}
	profile := profileR.Value.(Profile)

	id := core.Sprintf("oc-%d", core.Now().UnixNano())
	portR := allocatePort()
	if !portR.OK {
		return portR
	}
	hostPort := portR.Value.(int)

	// Resolve (or generate-on-first-use) the per-install
	// OPENCODE_SERVER_PASSWORD. Passed to the container via -e so
	// opencode-serve enforces auth; lthn's reverse-proxy + outbound
	// calls inject the matching Authorization header.
	pwR := s.ServerPassword()
	if !pwR.OK {
		return pwR
	}
	password, _ := pwR.Value.(string)

	// Inline-config via OPENCODE_CONFIG_CONTENT — opencode reads this
	// at startup before any provider initialisation, so the narrowed
	// profile (provider.lthn, tool/skill allow-lists, etc.) is the
	// effective config from the first request. PATCH /config does
	// NOT persist provider blocks at runtime; env-var inline is the
	// canonical mechanism.
	args := []string{
		"run", "-d",
		"-p", core.Sprintf("127.0.0.1:%d:%d", hostPort, containerPort),
		"-e", "OPENCODE_CONFIG_CONTENT=" + profile.ToOpenCodeWire(),
		"-e", "OPENCODE_SERVER_PASSWORD=" + password,
		"--name", ContainerName(id),
		s.image(),
		// `opencode web` serves the same /global/*, /provider, /session
		// API as `opencode serve` PLUS the browser-facing web UI at /.
		// We swap to `web` so the user gets both surfaces from one
		// container; the auto-open-browser behaviour silently no-ops
		// inside the container (nothing to open).
		"opencode", "web",
		"--hostname", "0.0.0.0",
		"--port", core.Sprintf("%d", containerPort),
	}

	ctx, cancel := core.WithTimeout(core.Background(), 30*core.Second)
	defer cancel()
	runR := ps.Run(ctx, s.runtime(), args...)
	if !runR.OK {
		return runR
	}

	sb := Sandbox{
		ID:        id,
		Image:     s.image(),
		HostPort:  hostPort,
		Status:    StatusRunning,
		CreatedAt: core.Now(),
	}
	saveR := orm.Of[Sandbox](s.Core()).Save(&sb)
	if !saveR.OK {
		// Best-effort cleanup — try to remove the container we
		// just created so we don't leak. Ignore the cleanup result.
		_ = ps.Run(ctx, s.runtime(), "rm", "-f", ContainerName(id))
		return saveR
	}

	target := core.Sprintf("http://127.0.0.1:%d", hostPort)
	authHeader := s.authHeader()
	s.proxy.Set(id, target, authHeader)

	// Wait for opencode-serve to bind + respond healthy, then apply
	// the profile via PATCH /config. Failures to apply the profile
	// don't fail Start — the sandbox is still usable with opencode's
	// own default config; the patch is a narrowing optimisation.
	if r := waitHealthy(target, authHeader, 30*core.Second); !r.OK {
		_ = ps.Run(core.Background(), s.runtime(), "rm", "-f", ContainerName(id))
		s.proxy.Delete(id)
		return r
	}
	if r := applyProfile(target, authHeader, profile); !r.OK {
		// Log the warning shape but don't propagate — sandbox is
		// up, just running with un-narrowed config.
		_ = r
	}

	// Auto-subscribe — opens an SSE stream if an event emitter is
	// installed (GUI mode). No-op in CLI/serve modes.
	_, _ = s.Subscribe(id)

	// Notify subscribers (runner) that the sandbox set changed.
	s.fireSandboxChange()

	return core.Ok(id)
}

// waitHealthy polls opencode-serve's /global/health until it
// returns 200 OK or the timeout fires. authHeader is the Basic
// Auth credential lthn injects on outbound calls — opencode-serve
// will 401 otherwise when OPENCODE_SERVER_PASSWORD is set.
func waitHealthy(target, authHeader string, timeout core.Duration) core.Result {
	deadline := core.Now().Add(timeout)
	client := &http.Client{Timeout: 2 * core.Second}
	for core.Now().Before(deadline) {
		req, err := http.NewRequest(http.MethodGet, target+"/global/health", nil)
		if err == nil {
			if authHeader != "" {
				req.Header.Set("Authorization", authHeader)
			}
			resp, derr := client.Do(req)
			if derr == nil {
				_ = resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					return core.Ok(nil)
				}
			}
		}
		core.Sleep(500 * core.Millisecond)
	}
	return core.Fail(core.E("opencode.waitHealthy", "opencode-serve did not become healthy within "+timeout.String(), nil))
}

// applyProfile PATCHes opencode-serve's /global/config with the
// profile JSON. The /global/config scope is where opencode reads
// provider definitions + enabled_providers — distinct from the
// project-scoped /config endpoint which writes to <cwd>/config.json
// and is consulted later in the resolution chain. Server-side this
// goes through opencode's Config.update Effect, mergeDeep into
// the existing config file, fs.writeFileString-persisted.
//
// authHeader is the Basic Auth credential lthn injects when
// OPENCODE_SERVER_PASSWORD is set (always set by Start).
func applyProfile(target, authHeader string, p Profile) core.Result {
	body := bytes.NewBufferString(p.ToOpenCodeWire())
	req, err := http.NewRequest(http.MethodPatch, target+"/global/config", body)
	if err != nil {
		return core.Fail(core.E("opencode.applyProfile", "request build failed", err))
	}
	req.Header.Set("Content-Type", "application/json")
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	client := &http.Client{Timeout: 5 * core.Second}
	resp, err := client.Do(req)
	if err != nil {
		return core.Fail(core.E("opencode.applyProfile", "patch failed", err))
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		respBody, _ := goio.ReadAll(resp.Body)
		return core.Fail(core.E("opencode.applyProfile",
			core.Sprintf("patch returned %d: %s", resp.StatusCode, string(respBody)), nil))
	}
	return core.Ok(nil)
}

// Stop kills the sandbox container, marks the record Stopped, and
// drops the reverse-proxy target.
//
// Usage example:
//
//	r := svc.Stop("oc-1735843891234")
//	if r.OK { _ = r }
func (s *Service) Stop(id string) core.Result {
	if core.Trim(id) == "" {
		return core.Fail(core.E(stopOp, "id is required", nil))
	}
	ps := s.proc()
	if ps == nil {
		return core.Fail(core.E(stopOp, "process service unavailable", nil))
	}

	// Cancel the SSE subscription FIRST — the goroutine is reading
	// from the soon-to-die container's /global/event; tearing it
	// down here means no flap of reconnect-retry-fail noise.
	s.Unsubscribe(id)

	ctx, cancel := core.WithTimeout(core.Background(), 15*core.Second)
	defer cancel()
	// docker rm -f stops + removes in one shot. Ignore failure —
	// the container may already be gone; we still want to clean
	// up the orm record + proxy entry.
	_ = ps.Run(ctx, s.runtime(), "rm", "-f", ContainerName(id))

	s.proxy.Delete(id)

	// Mark the record Stopped. Find first to confirm it exists.
	findR := orm.Of[Sandbox](s.Core()).Find(id)
	if findR.OK {
		sb := findR.Value.(Sandbox)
		sb.Status = StatusStopped
		_ = orm.Of[Sandbox](s.Core()).Save(&sb)
	}

	// Notify subscribers (runner) that the sandbox set changed.
	s.fireSandboxChange()

	return core.Ok(nil)
}

// Inspect returns the Sandbox record for a given id. Used by the
// CLI subcommand + future Wails bindings. Returns Fail when the
// record doesn't exist.
//
// Usage example:
//
//	r := svc.Inspect("oc-1735843891234")
//	if r.OK { sb := r.Value.(Sandbox); _ = sb.HostPort }
func (s *Service) Inspect(id string) core.Result {
	if core.Trim(id) == "" {
		return core.Fail(core.E(inspectOp, "id is required", nil))
	}
	return orm.Of[Sandbox](s.Core()).Find(id)
}

// Status returns the list of sandboxes with Status == Running.
// Useful for `lthn opencode status` + the GUI's overview surface.
//
// Usage example:
//
//	r := svc.Status()
//	if r.OK { running := r.Value.([]Sandbox); _ = running }
func (s *Service) Status() core.Result {
	return orm.Of[Sandbox](s.Core()).
		Where("status", "=", StatusRunning).
		Order("created_at", "desc").
		Get()
}

// allocatePort grabs a free host port by listening on 127.0.0.1:0
// and closing immediately — kernel returns a free port the docker
// daemon can then bind. Race window between Close and docker bind
// is negligible on a single-user dev machine.
func allocatePort() core.Result {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return core.Fail(core.E("opencode.allocatePort", "listen failed", err))
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	return core.Ok(port)
}
