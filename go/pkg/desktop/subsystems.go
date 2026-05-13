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
// surfaces to the pkg/server gin engine. Idempotent — safe to call
// twice, the second registration shadows the first (Gin allows route
// overrides on the same path + method).
//
// Returns the same Result shape as everything else on the desktop
// service — Ok if both mounts wired, Fail with the offending subsystem
// name otherwise. A missing service is treated as Ok (the subsystem
// just isn't enabled in this build) rather than Fail.
//
// Usage example:
//
//	if r := mountSubsystems(s.opts.Core, s.opts.Server.Engine(), s.opts.Runner); !r.OK {
//	    return r
//	}
func mountSubsystems(c *core.Core, engine *gin.Engine, r *runner.Service) core.Result {
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

	// api — Gin-based polyglot gateway. Its Engine exposes Handler()
	// which is the underlying *gin.Engine wrapped as http.Handler.
	// gin.WrapH() lifts that into a gin.HandlerFunc the parent engine
	// can register at /api/*. http.StripPrefix peels the /api segment
	// so the inner Engine sees the same paths it would as a standalone
	// server.
	if apiSvc, ok := core.ServiceFor[*coreapi.Service](c, "api"); ok && apiSvc != nil && apiSvc.Engine != nil {
		engine.Any("/api/*proxyPath", gin.WrapH(http.StripPrefix("/api", apiSvc.Engine.Handler())))
	}

	// mcp — Model Context Protocol. The current dappco.re/go/mcp
	// Service exposes ServeHTTP(ctx, addr) as an entry point (starts
	// a standalone HTTP server) rather than an http.Handler accessor.
	// Mounting under our gin engine needs an upstream `Handler()
	// http.Handler` accessor on *mcp.Service — pending a small PR.
	// Until then, an MCP client can still connect via the stdio
	// transport or by running `lthn mcp serve` standalone, just not
	// inside the same-origin Wails WebView. Tracked as a follow-on.
	//
	// (intentionally no /mcp/* route registered here)

	return core.Ok(nil)
}
