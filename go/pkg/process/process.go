// SPDX-Licence-Identifier: EUPL-1.2

// Package process is lthn/desktop's consumer-side wrapper around the
// dappco.re/go/process Service. The underlying Service is registered
// at app boot via core.WithName("process", ...); this package gives
// CLI / Wails / REST consumers a small, named-method surface so
// cmd/lthn stays thin and the REST mount has a single owner.
//
// Usage example:
//
//	svc := process.NewService(c)
//	r := svc.List(false)
//	if r.OK {
//	    snaps := r.Value
//	    _ = snaps
//	}
package process

import (
	core "dappco.re/go"
	coreapi "dappco.re/go/api"
	coreprocess "dappco.re/go/process"
	processapi "dappco.re/go/process/pkg/api"
	"github.com/gin-gonic/gin"
)

// APIBasePath is the canonical lthn mount for the upstream go-process
// REST surface. Lives under /v1/api/... matching the lthn convention
// (plugin proxies under /v1/api/plugin/, sandbox under /v1/api/sandbox/)
// — the bare /api prefix is reserved for the standalone coreapi.Service
// sub-engine mount in pkg/desktop/subsystems.go.
const APIBasePath = "/v1/api/process"

// rebasedProvider wraps processapi.ProcessProvider so its BasePath
// reads as APIBasePath rather than upstream's "/api/process". Lets the
// outer gin engine host process REST under a non-conflicting prefix.
type rebasedProvider struct {
	inner *processapi.ProcessProvider
}

func (p *rebasedProvider) Name() string                          { return p.inner.Name() }
func (p *rebasedProvider) BasePath() string                      { return APIBasePath }
func (p *rebasedProvider) RegisterRoutes(rg *gin.RouterGroup)    { p.inner.RegisterRoutes(rg) }

// Service wraps the dappco.re/go/process action surface for consumers
// inside this binary. All calls round-trip through c.Action so they
// share the same registry the bridge mount + Wails clients see.
type Service struct {
	core *core.Core
}

// NewService returns a Service bound to the supplied Core. The
// underlying dappco.re/go/process Service must already be registered
// on c (lthn does this at app boot).
//
// Usage example:
//
//	svc := process.NewService(c)
func NewService(c *core.Core) *Service { return &Service{core: c} }

// Register is the core.WithName-compatible factory. Wired in
// cmd/lthn/app.go so the service is reachable from any consumer via
// core.ServiceFor[*Service](c, "lthn-process") — and the server picks
// up its API routes via RoutesProvider discovery.
//
// Usage example:
//
//	core.New(core.WithName("lthn-process", lthnprocess.Register))
func Register(c *core.Core) core.Result {
	return core.Ok(NewService(c))
}

// RouteGroups satisfies pkg/server.RoutesProvider. Returns the
// dappco.re/go/process/pkg/api ProcessProvider re-based to lthn's
// /v1/api/process prefix when the upstream Core process Service is
// available. When upstream isn't registered (e.g. minimal CLI
// invocations), returns nil so the server skips the mount.
//
// Usage example:
//
//	// pkg/server iterates Core services + calls this automatically.
func (s *Service) RouteGroups() []coreapi.RouteGroup {
	upstream, _ := core.ServiceFor[*coreprocess.Service](s.core, "process")
	if upstream == nil {
		return nil
	}
	return []coreapi.RouteGroup{&rebasedProvider{inner: processapi.NewProvider(nil, upstream, nil)}}
}

// Run dispatches `process.run` — synchronous; returns combined output.
//
// Usage example:
//
//	r := svc.Run("echo", []string{"hello"})
func (s *Service) Run(cmd string, args []string) core.Result {
	return s.core.Action("process.run").Run(core.Background(), core.NewOptions(
		core.Option{Key: "command", Value: cmd},
		core.Option{Key: "args", Value: args},
	))
}

// Start dispatches `process.start` — background; returns the registry
// snapshot the caller uses to follow up with Get / Kill / etc.
//
// Usage example:
//
//	r := svc.Start("sleep", []string{"60"})
func (s *Service) Start(cmd string, args []string) core.Result {
	return s.core.Action("process.start").Run(core.Background(), core.NewOptions(
		core.Option{Key: "command", Value: cmd},
		core.Option{Key: "args", Value: args},
	))
}

// Kill dispatches `process.kill` for the given registry id.
//
// Usage example:
//
//	r := svc.Kill("id-1-3cfc5d")
func (s *Service) Kill(id string) core.Result {
	return s.core.Action("process.kill").Run(core.Background(), core.NewOptions(
		core.Option{Key: "id", Value: id},
	))
}

// List dispatches `process.list`. runningOnly filters to processes that
// are still running at snapshot time.
//
// Usage example:
//
//	r := svc.List(true) // running only
func (s *Service) List(runningOnly bool) core.Result {
	return s.core.Action("process.list").Run(core.Background(), core.NewOptions(
		core.Option{Key: "runningOnly", Value: runningOnly},
	))
}

// Get dispatches `process.get` for the given registry id.
//
// Usage example:
//
//	r := svc.Get("id-1-3cfc5d")
func (s *Service) Get(id string) core.Result {
	return s.core.Action("process.get").Run(core.Background(), core.NewOptions(
		core.Option{Key: "id", Value: id},
	))
}

// NewAPIProvider returns the dappco.re/go/process/pkg/api provider that
// mounts /api/process/* REST routes on the lthn server's gin engine.
// Wired by cmd/lthn/main.go into server.Options.ExtraGroups so the
// same action surface used by Wails + CLI is reachable over HTTP from
// the frontend lit components.
//
// Usage example:
//
//	if procSvc, _ := core.ServiceFor[*coreprocess.Service](c, "process"); procSvc != nil {
//	    extras = append(extras, process.NewAPIProvider(procSvc))
//	}
func NewAPIProvider(coreProc *coreprocess.Service) *processapi.ProcessProvider {
	// nil registry → DefaultRegistry; nil hub → REST only (no WS push for now)
	return processapi.NewProvider(nil, coreProc, nil)
}
