// SPDX-Licence-Identifier: EUPL-1.2

package opencode

import (
	"context"
	"net"
	"time"

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
// Sandbox record, and registers the reverse-proxy target.
// Returns the sandbox ID — the value the caller hands to
// /v1/api/sandbox/<id>/* URLs.
//
// Usage example:
//
//	r := svc.Start()
//	if r.OK { id := r.Value.(string); _ = id }
func (s *Service) Start() core.Result {
	ps := s.proc()
	if ps == nil {
		return core.Fail(core.E(startOp, "process service unavailable", nil))
	}

	id := core.Sprintf("oc-%d", time.Now().UnixNano())
	portR := allocatePort()
	if !portR.OK {
		return portR
	}
	hostPort := portR.Value.(int)

	args := []string{
		"run", "-d",
		"-p", core.Sprintf("127.0.0.1:%d:%d", hostPort, containerPort),
		"--name", ContainerName(id),
		s.image(),
		"opencode", "serve",
		"--hostname", "0.0.0.0",
		"--port", core.Sprintf("%d", containerPort),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
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
		CreatedAt: time.Now(),
	}
	saveR := orm.Of[Sandbox](s.Core()).Save(&sb)
	if !saveR.OK {
		// Best-effort cleanup — try to remove the container we
		// just created so we don't leak. Ignore the cleanup result.
		_ = ps.Run(ctx, s.runtime(), "rm", "-f", ContainerName(id))
		return saveR
	}

	target := core.Sprintf("http://127.0.0.1:%d", hostPort)
	s.proxy.Set(id, target)

	return core.Ok(id)
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

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
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
