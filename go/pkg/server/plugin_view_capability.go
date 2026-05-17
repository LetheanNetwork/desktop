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
	"net/http"

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
const (
	codePluginViewGrantInvalid       = "plugin_view.grant.invalid"
	codePluginViewGrantUnknownPlugin = "plugin_view.grant.unknown_plugin"
	codePluginViewGrantAuditFailed   = "plugin_view.grant.audit_failed"
)

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
	var req pluginViewCapabilityGrantRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, coreapi.Fail(codePluginViewGrantInvalid,
			"invalid request body — expect {plugin_id, capability, origin}"))
		return
	}
	if req.PluginID == "" || req.Capability == "" || req.Origin == "" {
		c.JSON(http.StatusBadRequest, coreapi.Fail(codePluginViewGrantInvalid,
			"plugin_id, capability and origin are all required"))
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
