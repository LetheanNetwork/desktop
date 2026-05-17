// SPDX-Licence-Identifier: EUPL-1.2

// middleware_body_cap.go — Layer 2 of RFC.body-cap-middleware.md (Mantis
// #1568 Unit C, Cerberus #16). Global gin middleware that wraps every
// inbound request body in http.MaxBytesReader using a per-prefix override
// table (longest-prefix wins) with a configurable default for everything
// else. Safe-by-default — a NEW endpoint mounted on coreapi.Engine
// inherits MaxBodyBytesDefault automatically; no per-endpoint opt-in
// required.
//
// Two-layer model:
//
//   - Layer 1 (inner) — per-endpoint wrap at the handler. The canonical
//     example is plugin_view_capability.go's handlePluginViewCapabilityGrant
//     which wraps with MaxPluginViewGrantBodyBytes (64 KiB) AND adds
//     per-field caps + origin validation. Inner-wins by construction —
//     two stacked MaxBytesReader wraps mean the tighter cap fires first.
//   - Layer 2 (this file, outer) — engine-wide default + per-prefix
//     overrides. Catches anything that didn't get a Layer 1 wrap.
//
// Per RFC §3.2.1 — per-endpoint inner-wrap caps stay deliberately OUT of
// DefaultBodyCapOverrides. The registry is for "this prefix needs a
// different default", not "this single endpoint has belt-and-braces".
//
// 413 envelope discipline (Amendment A1/A2): wherever http.MaxBytesReader
// fires, downstream handlers using errors.As(&mbe *http.MaxBytesError)
// render coreapi.Fail("body.too_large", ...) at HTTP 413 instead of the
// gin-default 400 with the leaked stdlib message. capInterceptingReader
// surfaces *http.MaxBytesError verbatim so the canonical pattern fires
// regardless of which layer triggered.

package server

import (
	"io"
	"net/http"

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/gateway"
	"github.com/gin-gonic/gin"
)

// MaxBodyBytesDefault is the body cap applied to every route on the
// pkg/server engine that doesn't carry an explicit override in
// DefaultBodyCapOverrides. 64 KiB matches the per-endpoint defaults the
// hardened endpoints already use (capability-grant, /internal/console,
// /internal/error) and is generous for every JSON-shaped admin route.
const MaxBodyBytesDefault = 64 << 10 // 64 KiB

// MaxBodyBytesHealth caps /health POSTs (if any future variant exists).
// /health carries no payload today; 4 KiB is the floor for any future
// liveness-probe extension.
const MaxBodyBytesHealth = 4 << 10 // 4 KiB

// MaxBodyBytesMCP caps the MCP transport surfaces (/mcp/* + /v1/mcp/*).
// MCP tool calls carry larger argument payloads (file contents,
// transcripts, model outputs). 256 KiB is the starting point per RFC §3.2;
// verify against external/mcp transport_http.go before bumping.
const MaxBodyBytesMCP = 256 << 10 // 256 KiB

// Gateway cap is sourced from pkg/gateway.MaxBodyBytes — single source
// of truth per Amendment A5 (Cerberus #16 C-6). Bumping the gateway cap
// is a one-line change in pkg/gateway and DefaultBodyCapOverrides picks
// it up automatically.

// BodyCapOverride pairs a route-path prefix with the cap that should
// apply when the request path matches the prefix. Longest-prefix wins;
// the middleware pre-sorts the slice longest-prefix-first so the
// linear scan in the hot path picks the most-specific match without
// re-sorting per-request. Static — set at NewService time, not mutable
// per-request.
//
// Sentinel: Bytes <= 0 means "no global cap; the endpoint enforces its
// own streaming budget". Middleware short-circuits without wrapping
// http.MaxBytesReader (which would otherwise reject every body byte for
// cap == 0). Use only on genuinely-streaming endpoints. See Amendment
// A4 / Cerberus #16 F-NEW-1.
//
// Usage example:
//
//	overrides := []server.BodyCapOverride{
//	    {Prefix: "/v1/api/upload/", Bytes: 0}, // streaming opt-out
//	    {Prefix: "/v1/api/gateway/", Bytes: gateway.MaxBodyBytes},
//	}
type BodyCapOverride struct {
	// Prefix is the exact path prefix to match. No glob expansion;
	// "/v1/api/gateway/" matches every URL whose path starts with that
	// literal string.
	Prefix string

	// Bytes is the cap in bytes, passed to http.MaxBytesReader
	// verbatim. <= 0 is the sentinel meaning "no global cap" (per
	// Amendment A4) — the middleware skips the wrap entirely so the
	// endpoint can stream without rejection.
	Bytes int64
}

// DefaultBodyCapOverrides is the starting-point override table for
// pkg/server. Order in this declaration is non-load-bearing — the
// middleware pre-sorts longest-prefix-first inside BodyCapMiddleware so
// /v1/api/gateway/health (if it existed) would inherit the gateway cap,
// not the /health cap.
//
// Per RFC §3.2.1 — /v1/plugin-view/capability-grant deliberately is NOT
// listed here. Its inner-wrap (MaxPluginViewGrantBodyBytes, 64 KiB) lives
// colocated with the per-field caps + origin validation in
// plugin_view_capability.go; moving the body cap to the registry would
// separate it from its semantic siblings without buying any new defence
// (inner-wins by construction already covers the two-wrap stacking case).
//
// Gateway cap sourced from pkg/gateway.MaxBodyBytes directly — single
// source per Amendment A5. Tests pin this table
// (TestBodyCap_DefaultOverrides_Good) so const drift surfaces loudly.
var DefaultBodyCapOverrides = []BodyCapOverride{
	{Prefix: "/v1/api/gateway/", Bytes: gateway.MaxBodyBytes},
	{Prefix: "/mcp/", Bytes: MaxBodyBytesMCP},
	{Prefix: "/v1/mcp/", Bytes: MaxBodyBytesMCP},
	{Prefix: "/health", Bytes: MaxBodyBytesHealth},
}

// BodyCapMiddleware returns a gin.HandlerFunc that wraps every request
// body in http.MaxBytesReader. The cap chosen per-request is the
// longest matching prefix in overrides; falls back to defaultCap when
// no override matches. Apply via coreapi.WithMiddleware at engine
// construction.
//
// Bodies that exceed the cap surface as *http.MaxBytesError via the
// capInterceptingReader. Handlers using ShouldBindJSON / ReadAll see
// the error in the wrap-chain; the canonical pattern (per Amendment A1)
// is errors.As(err, &mbe) → 413 coreapi.Fail("body.too_large", ...).
//
// Sentinel: an override with Bytes <= 0 short-circuits without wrapping
// (per Amendment A4). Use only on streaming endpoints; the endpoint
// then owns its own streaming budget.
//
// Usage example (in pkg/server.NewService):
//
//	apiOpts = append(apiOpts,
//	    coreapi.WithMiddleware(server.BodyCapMiddleware(
//	        server.MaxBodyBytesDefault, server.DefaultBodyCapOverrides)),
//	)
func BodyCapMiddleware(defaultCap int64, overrides []BodyCapOverride) gin.HandlerFunc {
	// Pre-sort longest-prefix-first so the linear scan in the hot path
	// picks the most-specific match without re-sorting per-request.
	// Stable sort is mandatory: equal-length prefixes (e.g. two ten-
	// character prefixes that don't overlap) preserve declaration
	// order so behaviour is deterministic regardless of map iteration.
	sorted := make([]BodyCapOverride, len(overrides))
	copy(sorted, overrides)
	core.SliceSortFunc(sorted, func(a, b BodyCapOverride) bool {
		return len(a.Prefix) > len(b.Prefix)
	})
	return func(c *gin.Context) {
		capBytes := defaultCap
		path := c.Request.URL.Path
		for _, o := range sorted {
			if core.HasPrefix(path, o.Prefix) {
				capBytes = o.Bytes
				break
			}
		}
		if capBytes <= 0 {
			// Amendment A4 / Cerberus #16 F-NEW-1: sentinel — streaming
			// endpoint opts out of global cap. Skip the wrap; cap == 0
			// passed to MaxBytesReader would reject every byte.
			c.Next()
			return
		}
		c.Request.Body = &capInterceptingReader{
			ReadCloser: http.MaxBytesReader(c.Writer, c.Request.Body, capBytes),
			ctx:        c,
			cap:        capBytes,
		}
		c.Next()
	}
}

// capInterceptingReader wraps http.MaxBytesReader and surfaces the
// underlying *http.MaxBytesError to downstream handlers verbatim, so
// the canonical Amendment A1 pattern (errors.As → 413 coreapi.Fail
// envelope) fires regardless of whether the body was read via
// ShouldBindJSON, ReadAll, or a streaming decoder. Without this wrapper
// gin's default error-translation kicks in and the caller sees the
// stdlib "http: request body too large" string leak.
//
// The wrap is a pass-through: Read just delegates to the wrapped
// MaxBytesReader. The point of the named type is the explicit contract
// — future maintainers see the type and the doc-comment instead of
// guessing why the body was wrapped twice.
type capInterceptingReader struct {
	io.ReadCloser
	ctx *gin.Context
	cap int64
}

// Read delegates to the wrapped MaxBytesReader. The error returned
// (notably *http.MaxBytesError) reaches downstream handlers verbatim.
//
// Usage example (inside a handler):
//
//	if err := c.ShouldBindJSON(&req); err != nil {
//	    var mbe *http.MaxBytesError
//	    if errors.As(err, &mbe) {
//	        c.JSON(http.StatusRequestEntityTooLarge,
//	            coreapi.Fail("body.too_large", "request body exceeds cap"))
//	        return
//	    }
//	    // ... handle other parse errors ...
//	}
func (r *capInterceptingReader) Read(p []byte) (int, error) {
	return r.ReadCloser.Read(p)
}
