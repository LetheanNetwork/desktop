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
//	/health   coreapi.Engine built-in
//	/api/*    dappco.re/go/api    (sub-engine mount via subEngineGroup)
//	/mcp/*    coreapi.ToolBridge  (REST façade over registered MCP tools)
//	/*        SPA fallback to embedded frontend dist
//
// Called once from Service.Run() after the SPA is attached and BEFORE
// the Wails app constructs.

package desktop

import (
	"io"
	"net/http"

	core "dappco.re/go"
	coreapi "dappco.re/go/api"
	mcpsvc "dappco.re/go/mcp/pkg/mcp"
	"github.com/gin-gonic/gin"

	lthnapi "dappco.re/lthn/desktop/pkg/api"
	"dappco.re/lthn/desktop/pkg/opencode"
	"dappco.re/lthn/desktop/pkg/plugin"
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

	// mcp — Model Context Protocol. The dappco.re/go/mcp.Service
	// registers tools as ToolRecord entries, each carrying a
	// RESTHandler closure already shaped for HTTP delivery. The
	// coreapi.ToolBridge from v0.8 converts those records into
	// `/mcp/<tool-name>` POST endpoints + OpenAPI paths + JSON-
	// Schema validation in one move.
	//
	// Result: SDK clients can call MCP tools over plain HTTP
	// (with the local bearer token) without speaking the MCP
	// protocol; native MCP clients (Claude Desktop stdio) keep
	// working as before — both routes share the same handler.
	if mcpSvc, ok := core.ServiceFor[*mcpsvc.Service](c, "mcp"); ok && mcpSvc != nil {
		bridge := coreapi.NewToolBridge("/mcp")
		for _, t := range mcpSvc.Tools() {
			bridge.Add(coreapi.ToolDescriptor{
				Name:         t.Name,
				Description:  t.Description,
				Group:        t.Group,
				InputSchema:  t.InputSchema,
				OutputSchema: t.OutputSchema,
			}, adaptMCPRest(t.RESTHandler))
		}
		engine.Register(bridge)
	}

	// plugin — reverse-proxy mount under /v1/api/plugin/<code>/*.
	// The ProxyGroup is registered exactly once; the plugin host
	// service mutates the internal targets map on Start/Stop so
	// route registration doesn't need to be dynamic. See
	// docs/plugin-host-scope.md for the contract.
	if pluginSvc, ok := core.ServiceFor[*plugin.Service](c, "plugin"); ok && pluginSvc != nil {
		engine.Register(pluginSvc.ProxyGroup())
	}

	// opencode — reverse-proxy mount under /v1/api/sandbox/<id>/*.
	// Same shape as the plugin proxy: one RouteGroup registered
	// once; the opencode service mutates the targets map on
	// Start/Stop. The container sees clean paths (/global/health,
	// /session, /provider) because the proxy strips the sandbox-id
	// prefix entirely. See plans/project/lthn/desktop/RFC.opencode.md.
	if opencodeSvc, ok := core.ServiceFor[*opencode.Service](c, "opencode"); ok && opencodeSvc != nil {
		engine.Register(opencodeSvc.ProxyGroup())
	}

	return core.Ok(nil)
}

// adaptMCPRest lifts an mcpsvc.RESTHandler (ctx + body bytes →
// any + error) into a gin.HandlerFunc the ToolBridge can mount.
// The HTTP boundary becomes: read body → call handler → wrap the
// result in coreapi's OK / Fail envelopes so every MCP tool
// responds in the same shape as the rest of the API.
func adaptMCPRest(h mcpsvc.RESTHandler) gin.HandlerFunc {
	if h == nil {
		return func(c *gin.Context) {
			c.AbortWithStatusJSON(http.StatusInternalServerError, coreapi.Fail(
				"tool_unbound",
				"mcp tool registered without a REST handler",
			))
		}
	}
	return func(c *gin.Context) {
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, coreapi.Fail(
				"invalid_body",
				err.Error(),
			))
			return
		}
		ctx := c.Request.Context()
		if ctx == nil {
			ctx = core.Background()
		}
		result, err := h(ctx, body)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, coreapi.Fail(
				"tool_error",
				err.Error(),
			))
			return
		}
		c.JSON(http.StatusOK, coreapi.OK(result))
	}
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
