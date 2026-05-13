// SPDX-Licence-Identifier: EUPL-1.2

// Mounts the dappco.re/go/api Gin engine and dappco.re/go/mcp Service
// HTTP handler onto our pkg/server gin engine so the same surface that
// runs standalone via `lthn serve` also lights up inside the Wails
// WebView's same-origin context — no CORS, no port hunting.
//
// Pattern per docs/src/content/docs/guides/gin-routing.mdx — single
// gin engine handles everything, /wails/* still falls through to
// Wails' own runtime via ginMiddleware (see desktop.go).
//
// The handler chain at runtime:
//
//	/wails/*  ginMiddleware → Wails runtime (assets + bindings + JS bridge)
//	/v1/*     pkg/server          (OpenAI-compatible: chat / completions / models)
//	/health   pkg/server
//	/api/*    dappco.re/go/api    (Gin gateway; route groups via api.Engine.Register)
//	/mcp/*    dappco.re/go/mcp    (MCP transport; Service.ServeHTTP)
//	/*        SPA fallback to embedded frontend dist
//
// Called once from Service.Run() after the SPA is attached and BEFORE
// the Wails app constructs.

package desktop

import (
	"net/http"

	core "dappco.re/go"
	coreapi "dappco.re/go/api"
	"github.com/gin-gonic/gin"

	lthnapi "dappco.re/lthn/desktop/pkg/api"
	"dappco.re/lthn/desktop/pkg/runner"
)

// mountSubsystems attaches the dappco.re/go/api + dappco.re/go/mcp HTTP
// surfaces to the coreapi-spined pkg/server. The spine engine is itself
// a *coreapi.Engine, so subsystem routes mount as RouteGroups that
// inherit the canonical middleware chain (auth, SSRF, sunset, cache,
// tracing).
//
// Returns the same Result shape as everything else on the desktop
// service — Ok if all mounts wired, Fail with the offending subsystem
// name otherwise. A missing service is treated as Ok (the subsystem
// just isn't enabled in this build) rather than Fail.
//
// Usage example:
//
//	if r := mountSubsystems(s.opts.Core, s.opts.Server.Engine(), s.opts.Runner); !r.OK {
//	    return r
//	}
func mountSubsystems(c *core.Core, engine *coreapi.Engine, r *runner.Service) core.Result {
	if c == nil {
		return core.Fail(core.E("desktop.mountSubsystems", "core is nil", nil))
	}
	if engine == nil {
		return core.Fail(core.E("desktop.mountSubsystems", "engine is nil", nil))
	}

	// Register lthn-owned RouteGroups on the api.Service Engine BEFORE
	// wrapping its handler — group registration mutates the underlying
	// gin tree, and Handler() snapshots that tree on first call.
	if r != nil {
		if rr := lthnapi.Register(c, r); !rr.OK {
			return rr
		}
	}

	// api (sub-engine sub-mount) — the standalone *coreapi.Service
	// registered on Core has its own Engine with separate routes;
	// expose those at /api/* so consumers can reach them without
	// colliding with our root surface. Wrapped as a RouteGroup so the
	// outer engine's middleware chain still applies. Stripping /api
	// peels the prefix before the inner engine sees the request.
	if apiSvc, ok := core.ServiceFor[*coreapi.Service](c, "api"); ok && apiSvc != nil && apiSvc.Engine != nil {
		engine.Register(&subEngineGroup{
			name:     "subapi",
			basePath: "/api",
			handler:  http.StripPrefix("/api", apiSvc.Engine.Handler()),
		})
	}

	// mcp — Model Context Protocol. The current dappco.re/go/mcp
	// Service exposes ServeHTTP(ctx, addr) as an entry point (starts
	// a standalone HTTP server) rather than an http.Handler accessor.
	// Mounting under our coreapi engine needs an upstream `Handler()
	// http.Handler` accessor on *mcp.Service — pending a small PR.
	// Until then, an MCP client can still connect via the stdio
	// transport or by running `lthn mcp serve` standalone, just not
	// inside the same-origin Wails WebView. Tracked as a follow-on.
	//
	// (intentionally no /mcp/* route registered here)

	return core.Ok(nil)
}

// subEngineGroup is a coreapi.RouteGroup that proxies every request
// under basePath to an inner http.Handler — used to mount the
// standalone dappco.re/go/api Service's Engine at /api/* on our
// outer engine.
type subEngineGroup struct {
	name     string
	basePath string
	handler  http.Handler
}

func (g *subEngineGroup) Name() string     { return g.name }
func (g *subEngineGroup) BasePath() string { return g.basePath }
func (g *subEngineGroup) RegisterRoutes(rg *gin.RouterGroup) {
	rg.Any("/*proxyPath", gin.WrapH(g.handler))
}
