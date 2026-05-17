// SPDX-Licence-Identifier: EUPL-1.2

// plugin_view_capability.go — backend half of Mantis #1523. The
// frontend shim at frontend/src/lit/plugin-view-opencode-shim.ts
// brokers a session-token capability into a third-party iframe via
// the §5.1 postMessage handshake (RFC.plugin-views.md). Before the
// shim delivers the token bytes it POSTs to /v1/plugin-view/capability-grant
// so the audit row commits FIRST; on failure the shim aborts the
// postMessage path. This file owns the receiver.
//
// The endpoint:
//
//   1. Validates the request (bearer-auth applied upstream by
//      coreapi.WithBearerAuth — by the time this handler runs the
//      caller holds a valid session OR localkey token).
//   2. Validates plugin_id matches an installed plugin (via the
//      optional Options.PluginInstalledChecker hook — when nil the
//      check is skipped so non-desktop builds and tests don't have
//      to wire pkg/plugin).
//   3. Validates capability + origin are well-formed.
//   4. Emits audit.EventPluginViewCapabilityGranted with
//      {plugin_id, capability, origin} in Meta. NEVER the token bytes
//      (Cerberus #1465 + #1468).
//   5. Returns 200 on success, 400 on shape failure, 404 on
//      unknown plugin_id, 500 on audit emit failure (the shim treats
//      anything but 200 as "do not postMessage").
//
// Tier: TierLocal — Stage E.B's middleware gates the route on a
// valid LocalKey OR session token. The capability-grant flow runs
// before the iframe holds a session-token of its own; the shim's
// caller (WebView app-shell) already holds the host bearer.

package server

import (
	"errors"
	"net/http"
	"net/url"

	core "dappco.re/go"
	coreapi "dappco.re/go/api"
	"dappco.re/lthn/desktop/pkg/audit"
	"github.com/gin-gonic/gin"
)

// PluginViewCapabilityGrantPath is the canonical mount for the
// capability-grant receiver. Outside the /v1/api/plugin/:code/*
// proxy wildcard namespace on purpose — gin can't mix exact-match
// and wildcard at the same depth without the wildcard swallowing
// the literal.
const PluginViewCapabilityGrantPath = "/v1/plugin-view/capability-grant"

// MaxPluginViewGrantBodyBytes caps the request body on the
// capability-grant receiver. 64 KiB is generous (the real payload is
// ~300 bytes — plugin_id + capability + origin) but matches the
// /internal/console + /internal/error cap (pkg/bridge) for consistency.
// Cerberus #1568 F1: an unbounded body lets a bearer-authorised caller
// POST megabytes that gin streams into the JSON decoder and the
// audit Meta map writes verbatim to disk. The wrap fires BEFORE
// ShouldBindJSON so the decoder errors at the cap, not after the audit
// row commits.
const MaxPluginViewGrantBodyBytes = 64 << 10 // 64 KiB

// MaxPluginIDBytes caps the plugin_id string field. Plugin codes are
// FQDN-shaped (e.g. "code.opencode.com") — 256 bytes is 4x the longest
// realistic value and matches DNS label-sum bounds.
const MaxPluginIDBytes = 256

// MaxCapabilityBytes caps the capability literal field. Today the
// allowlist is one entry ("session-token", 13 bytes); 64 bytes leaves
// headroom for future capability names without admitting abuse.
const MaxCapabilityBytes = 64

// MaxOriginBytes caps the origin URL field. RFC 3986 has no formal URL
// length cap; 2 KiB matches the browser-pragmatic Internet Explorer
// historical limit and exceeds every legitimate localhost loopback
// origin lthn ships (`http://127.0.0.1:9876` is 22 bytes).
const MaxOriginBytes = 2 << 10 // 2 KiB

// PluginInstalledChecker reports whether a plugin code corresponds
// to a currently-installed plugin. cmd/lthn/main.go wires this via
// (*plugin.Service).ProxyGroup().Has. Nil leaves the check off —
// the audit event still fires but plugin_id isn't constrained.
type PluginInstalledChecker func(code string) bool

// pluginViewCapabilityGrantRequest is the JSON body the shim POSTs
// before its postMessage call. Field names mirror the audit Meta
// keys verbatim so on-the-wire shape == on-disk shape.
//
// Usage example (from frontend/src/lit/plugin-view-opencode-shim.ts):
//
//	await apiFetch("/v1/plugin-view/capability-grant", {
//	    method: "POST",
//	    headers: { "Content-Type": "application/json" },
//	    body: JSON.stringify({
//	        plugin_id:  desc.pluginCode,
//	        capability: "session-token",
//	        origin:     desc.loopbackOrigin,
//	    }),
//	});
type pluginViewCapabilityGrantRequest struct {
	PluginID   string `json:"plugin_id"`
	Capability string `json:"capability"`
	Origin     string `json:"origin"`
}

// Error codes the endpoint emits. Mirrors the pkg/audit + pkg/account
// "<namespace>.<verb>.<reason>" discipline so log-tailers + the
// gateway-status surface can pattern-match.
//
// codePluginViewGrantBodyTooLarge is the canonical 413 envelope code
// per RFC.body-cap-middleware.md Amendment A1 — emitted when
// http.MaxBytesReader fires before ShouldBindJSON sees the body.
// Distinct from codePluginViewGrantInvalid so log-tailers can
// distinguish "abuse-shaped" rejection from "shape-broken request".
const (
	codePluginViewGrantInvalid       = "plugin_view.grant.invalid"
	codePluginViewGrantUnknownPlugin = "plugin_view.grant.unknown_plugin"
	codePluginViewGrantAuditFailed   = "plugin_view.grant.audit_failed"
	codePluginViewGrantBodyTooLarge  = "body.too_large"
)

// isValidPostMessageOrigin accepts only http:// or https:// schemes
// with a non-empty host + optional port + NO path/query/fragment.
// Mirrors RFC 6454 origin grammar; rejects javascript:, data:, file:,
// blob: and any bare-hostname / relative-path value. Does NOT constrain
// the host to loopback — the manifest source can legitimately be
// non-loopback (off-box plugin server, future remote-manifest install).
// What this validator enforces is well-formed postMessage origin shape;
// host allow-listing is a separate concern at install-time.
//
// Usage example:
//
//	if !isValidPostMessageOrigin(req.Origin) {
//	    c.JSON(http.StatusBadRequest, coreapi.Fail(codePluginViewGrantInvalid,
//	        "origin must be http(s)://host[:port] with no path"))
//	    return
//	}
//
// Renamed from isValidLoopbackOrigin per RFC.body-cap-middleware.md
// Amendment A1 / Cerberus #16 C-1.
func isValidPostMessageOrigin(s string) bool {
	u, err := url.Parse(s)
	if err != nil {
		return false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	if u.Host == "" {
		return false
	}
	if u.Path != "" && u.Path != "/" {
		return false
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	return true
}

// handlePluginViewCapabilityGrant is the gin handler. Held as a
// method on *Service so it can read opts.PluginInstalledChecker
// without an additional dependency-injection layer.
//
// HTTP status mapping:
//
//   - 400 — body parse failed / required field missing / unknown
//           capability literal
//   - 404 — plugin_id not installed (PluginInstalledChecker returned false)
//   - 500 — audit.Default().Record returned !OK (shim must NOT
//           proceed with postMessage)
//   - 200 — grant recorded; shim is clear to postMessage the token
func (s *Service) handlePluginViewCapabilityGrant(c *gin.Context) {
	// Layer 1 body cap (Cerberus #1568 F1) — wraps the body reader
	// BEFORE ShouldBindJSON so the JSON decoder errors at the cap, not
	// after the audit row commits. Per RFC.body-cap-middleware.md §3.1.
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, MaxPluginViewGrantBodyBytes)

	var req pluginViewCapabilityGrantRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// Amendment A1 — distinguish "body exceeds cap" (413, canonical
		// envelope) from "shape-invalid JSON" (400, existing envelope).
		// errors.As walks the wrap-chain so this fires whether
		// MaxBytesReader is the only wrap (Layer 1 alone) or stacked
		// under the future Layer 2 middleware (Unit C).
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			c.JSON(http.StatusRequestEntityTooLarge, coreapi.Fail(codePluginViewGrantBodyTooLarge,
				"request body exceeds MaxPluginViewGrantBodyBytes"))
			return
		}
		c.JSON(http.StatusBadRequest, coreapi.Fail(codePluginViewGrantInvalid,
			"invalid request body — expect {plugin_id, capability, origin}"))
		return
	}
	// Per-field caps — belt-and-braces against a 64 KiB body packing
	// one 60 KiB field. Fires BEFORE audit emission so abusive payloads
	// never reach the audit substrate.
	if len(req.PluginID) > MaxPluginIDBytes {
		c.JSON(http.StatusBadRequest, coreapi.Fail(codePluginViewGrantInvalid,
			"plugin_id exceeds MaxPluginIDBytes"))
		return
	}
	if len(req.Capability) > MaxCapabilityBytes {
		c.JSON(http.StatusBadRequest, coreapi.Fail(codePluginViewGrantInvalid,
			"capability exceeds MaxCapabilityBytes"))
		return
	}
	if len(req.Origin) > MaxOriginBytes {
		c.JSON(http.StatusBadRequest, coreapi.Fail(codePluginViewGrantInvalid,
			"origin exceeds MaxOriginBytes"))
		return
	}
	if req.PluginID == "" || req.Capability == "" || req.Origin == "" {
		c.JSON(http.StatusBadRequest, coreapi.Fail(codePluginViewGrantInvalid,
			"plugin_id, capability and origin are all required"))
		return
	}
	// Origin scheme + grammar validation — refuse javascript:/data:/file:
	// (XSS vectors if the audit row ever surfaces in a Lit view that
	// renders meta.origin as a hyperlink) AND require an absolute URL
	// (refuse "../../etc" or bare hostnames). Per RFC.body-cap-middleware.md
	// §3.1 / Amendment A1.
	if !isValidPostMessageOrigin(req.Origin) {
		c.JSON(http.StatusBadRequest, coreapi.Fail(codePluginViewGrantInvalid,
			"origin must be http(s)://host[:port] with no path"))
		return
	}
	// Capability allowlist mirrors the frontend shim's contract
	// (CAPABILITY_SESSION_TOKEN). New capabilities REQUIRE a new
	// Mantis ticket — the shim, this allowlist + the audit consumer
	// learn the literal in the same commit.
	if req.Capability != "session-token" {
		c.JSON(http.StatusBadRequest, coreapi.Fail(codePluginViewGrantInvalid,
			"unknown capability literal — only session-token is brokered today"))
		return
	}
	if s.opts.PluginInstalledChecker != nil && !s.opts.PluginInstalledChecker(req.PluginID) {
		c.JSON(http.StatusNotFound, coreapi.Fail(codePluginViewGrantUnknownPlugin,
			"plugin not installed: "+req.PluginID))
		return
	}
	ev := audit.Event{
		Event:     audit.EventPluginViewCapabilityGranted,
		TS:        core.UnixNow(),
		Scope:     "plugin.view",
		Outcome:   audit.OutcomeOK,
		RequestID: c.GetHeader("X-Request-Id"),
		Meta: map[string]any{
			"plugin_id":  req.PluginID,
			"capability": req.Capability,
			"origin":     req.Origin,
		},
	}
	r := audit.Default().Record(ev)
	if !r.OK {
		c.JSON(http.StatusInternalServerError, coreapi.Fail(codePluginViewGrantAuditFailed,
			"audit emission failed — capability NOT delivered to iframe: "+r.Error()))
		return
	}
	c.JSON(http.StatusOK, coreapi.OK(gin.H{
		"recorded": true,
	}))
}
