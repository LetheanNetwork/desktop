// SPDX-Licence-Identifier: EUPL-1.2

// plugin_view_capability.go — backend half of Mantis #1523 + #1576. The
// frontend broker at
// frontend-ng/src/app/desktop/surfaces/extensions/plugin-auth-broker.ts
// (grantTokenToFrame) brokers a session-token capability into a third-party
// iframe via
// the §5.1 postMessage handshake (RFC.plugin-views.md). Before the
// broker delivers the token bytes it POSTs to /v1/plugin-view/capability-grant
// so the audit row commits FIRST; on failure the broker aborts the
// postMessage path. This file owns the receiver.
//
// RFC.plugin-view-audit-atomicity.md v2 (Mantis #1576, target_version
// v1.0.0-beta.1) closed the row-vs-grant-occurred semantic gap by
// adopting Option A: ONE audit row per broker call carrying the
// `capabilities: [...]` array + `outcome: "granted"` literal +
// handler-generated `correlation_id`. Per-scope rows are no longer
// emitted. The legacy `{capability: <scalar>}` shape is REJECTED
// with 400 per §5(a) hard-cutover discipline.
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
//   3. Validates capabilities array (non-empty, every literal in the
//      handler allowlist), outcome literal (== "granted"), origin.
//   4. Generates a UUIDv4 correlation_id (handler authority per
//      RFC v2 §3.1 — request body MUST NOT carry one).
//   5. Emits ONE audit.EventPluginViewCapabilityGranted with
//      {plugin_id, capabilities: [...], origin, outcome, correlation_id}
//      in Meta. NEVER the token bytes (Cerberus #1465 + #1468).
//   6. Returns 200 with {correlation_id} body on success, 400 on
//      shape failure, 404 on unknown plugin_id, 500 on audit emit
//      failure (the broker treats anything but 200 as "do not
//      postMessage").
//
// Tier: TierLocal — Stage E.B's middleware gates the route on a
// valid LocalKey OR session token. The capability-grant flow runs
// before the iframe holds a session-token of its own; the broker's
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
// ~300 bytes — plugin_id + capabilities + origin + outcome) but matches
// the /internal/console + /internal/error cap (pkg/bridge) for consistency.
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

// MaxCapabilityBytes caps each capability literal in the array. Today
// the allowlist is one entry ("session-token", 13 bytes); 64 bytes leaves
// headroom for future capability names without admitting abuse.
const MaxCapabilityBytes = 64

// MaxCapabilitiesLen caps the length of the `capabilities` array per
// request. Today the broker only ever sends ["session-token"] (N=1);
// 16 leaves headroom for future bundle-grants without admitting abuse
// (a 64 KiB body packed with 1024 four-byte capabilities would otherwise
// reach the audit substrate before per-entry size validation could fire).
const MaxCapabilitiesLen = 16

// MaxOriginBytes caps the origin URL field. RFC 3986 has no formal URL
// length cap; 2 KiB matches the browser-pragmatic Internet Explorer
// historical limit and exceeds every legitimate localhost loopback
// origin lthn ships (`http://127.0.0.1:9876` is 22 bytes).
const MaxOriginBytes = 2 << 10 // 2 KiB

// allowedCapabilities is the authoritative gate per RFC.plugin-view-
// audit-atomicity.md v2 §10. Adding a new capability literal anywhere
// in the codebase REQUIRES adding it here in the same commit per
// [[feedback_scope_allowlist_lockstep]]. Failure = Cerberus-blocking
// ship-stop.
var allowedCapabilities = map[string]struct{}{
	"session-token": {},
}

// PluginInstalledChecker reports whether a plugin code corresponds
// to a currently-installed plugin. cmd/lthn/main.go wires this via
// (*plugin.Service).ProxyGroup().Has. Nil leaves the check off —
// the audit event still fires but plugin_id isn't constrained.
type PluginInstalledChecker func(code string) bool

// pluginViewCapabilityGrantRequest is the JSON body the broker POSTs
// before its postMessage call. Field names mirror the audit Meta
// keys verbatim so on-the-wire shape == on-disk shape.
//
// RFC.plugin-view-audit-atomicity.md v2 §3.1 + §5(a) — `capabilities`
// is ALWAYS an array (never a scalar); `outcome` literal MUST be the
// string `"granted"`; `correlation_id` is HANDLER-generated and MUST
// NOT appear in the request body. Legacy `capability` scalar shape
// rejected with 400 + codePluginViewGrantInvalid per §3.1.
//
// Usage example (from the Angular plugin-auth-broker grantTokenToFrame):
//
//	await apiFetch("/v1/plugin-view/capability-grant", {
//	    method: "POST",
//	    headers: { "Content-Type": "application/json" },
//	    body: JSON.stringify({
//	        plugin_id:    target.pluginCode,
//	        capabilities: ["session-token"],
//	        origin:       target.targetOrigin,
//	        outcome:      "granted",
//	    }),
//	});
//	// response.json() carries {success:true, data:{correlation_id:"<uuid>"}}
type pluginViewCapabilityGrantRequest struct {
	PluginID     string   `json:"plugin_id"`
	Capabilities []string `json:"capabilities"`
	Origin       string   `json:"origin"`
	Outcome      string   `json:"outcome"`
	// LegacyCapability captures any caller still posting the pre-v2
	// scalar shape so the handler can reject explicitly (400) instead
	// of silently dropping the field. Per RFC v2 §5(a) hard-cutover —
	// no transitional dual-acceptance window.
	LegacyCapability string `json:"capability,omitempty"`
	// LegacyCorrelationID captures any caller attempting to assert a
	// correlation_id from the request side. Handler is the sole
	// authority per §3.1; presence rejected with 400 so a malicious
	// caller cannot fabricate audit-JOIN keys.
	LegacyCorrelationID string `json:"correlation_id,omitempty"`
}

// pluginViewCapabilityGrantResponse is the success-body shape returned
// to the broker. Carries the handler-generated correlation_id so the
// broker can log it locally for forensic reconstruction without ever
// asserting it on the wire.
type pluginViewCapabilityGrantResponse struct {
	CorrelationID string `json:"correlation_id"`
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
//     capability literal / outcome != "granted" / legacy scalar
//     shape / caller-asserted correlation_id
//   - 404 — plugin_id not installed (PluginInstalledChecker returned false)
//   - 500 — audit.Default().Record returned !OK (broker must NOT
//     proceed with postMessage)
//   - 200 — grant recorded; response body carries correlation_id;
//     broker is clear to postMessage the token
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
			"invalid request body — expect {plugin_id, capabilities, origin, outcome}"))
		return
	}
	// RFC v2 §5(a) hard cutover — reject the legacy {capability:<scalar>}
	// shape explicitly with 400 instead of silently dropping it. Caller
	// must migrate to {capabilities:[...], outcome:"granted"} from the
	// same commit; broker (sole producer) ships the new shape in lockstep.
	if req.LegacyCapability != "" {
		c.JSON(http.StatusBadRequest, coreapi.Fail(codePluginViewGrantInvalid,
			"`capability` field deprecated; use `capabilities` array per RFC.plugin-view-audit-atomicity.md v2"))
		return
	}
	// RFC v2 §3.1 — correlation_id is handler-generated. A caller-asserted
	// value lets a compromised broker fabricate audit-JOIN keys, defeating
	// the disambiguation property the field exists to provide. Reject.
	if req.LegacyCorrelationID != "" {
		c.JSON(http.StatusBadRequest, coreapi.Fail(codePluginViewGrantInvalid,
			"`correlation_id` is handler-generated; request body must not carry it"))
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
	if len(req.Origin) > MaxOriginBytes {
		c.JSON(http.StatusBadRequest, coreapi.Fail(codePluginViewGrantInvalid,
			"origin exceeds MaxOriginBytes"))
		return
	}
	if req.PluginID == "" || req.Origin == "" {
		c.JSON(http.StatusBadRequest, coreapi.Fail(codePluginViewGrantInvalid,
			"plugin_id and origin are required"))
		return
	}
	// RFC v2 §3.1 + §4.3 — capabilities MUST be a non-empty array within
	// MaxCapabilitiesLen, every literal within MaxCapabilityBytes, every
	// literal in the allowlist. Empty array, oversize entries, and
	// unlisted literals all reject with 400 + ZERO audit rows emitted
	// per the row-IS-grant discipline-floor.
	if len(req.Capabilities) == 0 {
		c.JSON(http.StatusBadRequest, coreapi.Fail(codePluginViewGrantInvalid,
			"capabilities array required (at least one capability)"))
		return
	}
	if len(req.Capabilities) > MaxCapabilitiesLen {
		c.JSON(http.StatusBadRequest, coreapi.Fail(codePluginViewGrantInvalid,
			"capabilities exceeds MaxCapabilitiesLen"))
		return
	}
	for _, cap := range req.Capabilities {
		if cap == "" {
			c.JSON(http.StatusBadRequest, coreapi.Fail(codePluginViewGrantInvalid,
				"capabilities entries must be non-empty"))
			return
		}
		if len(cap) > MaxCapabilityBytes {
			c.JSON(http.StatusBadRequest, coreapi.Fail(codePluginViewGrantInvalid,
				"capability literal exceeds MaxCapabilityBytes"))
			return
		}
		// Allowlist gate per RFC v2 §10 — load-bearing for
		// [[feedback_scope_allowlist_lockstep]].
		if _, ok := allowedCapabilities[cap]; !ok {
			c.JSON(http.StatusBadRequest, coreapi.Fail(codePluginViewGrantInvalid,
				"unknown capability literal — not in handler allowlist"))
			return
		}
	}
	// RFC v2 §3.1 — outcome literal hard-locked to "granted" in v1. Any
	// other value (including "denied", "missing", or empty) rejects with
	// 400. Future broker outcomes require a new RFC supplanting this one
	// + an explicit handler allowlist extension.
	if req.Outcome != "granted" {
		c.JSON(http.StatusBadRequest, coreapi.Fail(codePluginViewGrantInvalid,
			"outcome must be 'granted' (v1 broker emits no other value)"))
		return
	}
	// Origin scheme + grammar validation — refuse javascript:/data:/file:
	// (XSS vectors if the audit row ever surfaces in an Angular or Lit view
	// that renders meta.origin as a hyperlink) AND require an absolute URL
	// (refuse "../../etc" or bare hostnames). Per RFC.body-cap-middleware.md
	// §3.1 / Amendment A1.
	if !isValidPostMessageOrigin(req.Origin) {
		c.JSON(http.StatusBadRequest, coreapi.Fail(codePluginViewGrantInvalid,
			"origin must be http(s)://host[:port] with no path"))
		return
	}
	if s.opts.PluginInstalledChecker != nil && !s.opts.PluginInstalledChecker(req.PluginID) {
		c.JSON(http.StatusNotFound, coreapi.Fail(codePluginViewGrantUnknownPlugin,
			"plugin not installed: "+req.PluginID))
		return
	}
	// Cerberus #21 (Mantis #1595) — cross-check the request's `origin`
	// against the marketplace ViewRegistry's authoritative
	// LoopbackOriginFor record for the plugin code. The handler already
	// validates plugin_id (PluginInstalledChecker) and origin grammar
	// (isValidPostMessageOrigin) INDEPENDENTLY; without this gate a
	// caller can post a falsified (pluginCode, origin) tuple and the
	// audit row commits even though the registry knows the truth. The
	// browser's postMessage targetOrigin enforcement is the safety floor
	// against token leakage; this gate closes the audit-log-integrity
	// gap.
	//
	// (ok == false) means "registry doesn't know the expected origin
	// for this code" — lit-kind plugins (no loopback), manifest-only
	// entries, non-iframe views. Fall through to the existing flow so
	// non-iframe plugin grants stay accepted.
	if s.opts.PluginOriginChecker != nil {
		expected, ok := s.opts.PluginOriginChecker(req.PluginID)
		if ok && req.Origin != expected {
			c.JSON(http.StatusBadRequest, coreapi.Fail(codePluginViewGrantInvalid,
				"origin does not match registered loopback origin for plugin"))
			return
		}
	}
	// Handler-generated correlation_id (RFC v2 §3.1). Disambiguates
	// near-simultaneous grants for the same (plugin_id, origin, capability)
	// tuple within one NDJSON day file. Empty on RandomBytes failure —
	// downstream readers tolerate the missing field; NOT a panic-worthy
	// condition. CoreGO gap: core.UUIDv4 doesn't exist yet (logged at
	// project_corego_export_gaps); mirrors pkg/account/routes.go
	// serverRequestID() until the export lands.
	correlationID := newCorrelationID()
	// Cerberus #1511 — server generates the audit RequestID. The caller's
	// X-Request-Id header is dropped to prevent forensic deniability (an
	// attacker forging arbitrary values to mimic a legitimate caller's
	// audit-JOIN key, defeating the disambiguation property the field
	// exists to provide). The server's UUID v4 is echoed in the response
	// X-Request-Id header so the legitimate caller can still correlate
	// their request to the audit log. RequestID and correlation_id are
	// SEPARATE values by design — RequestID is the cross-cutting audit
	// substrate key, correlation_id disambiguates within a single
	// (plugin_id, origin, capability) tuple — generated independently so
	// neither can leak the other.
	srvReqID := newCorrelationID()
	c.Header("X-Request-Id", srvReqID)
	// Defensive copy of the capabilities slice so the audit substrate
	// never shares mutable backing with the request body. The forensic
	// record is immutable by construction.
	caps := make([]string, len(req.Capabilities))
	copy(caps, req.Capabilities)
	ev := audit.Event{
		Event:     audit.EventPluginViewCapabilityGranted,
		TS:        core.UnixNow(),
		Scope:     "plugin.view",
		Outcome:   audit.OutcomeOK,
		RequestID: srvReqID,
		Meta: map[string]any{
			"plugin_id":      req.PluginID,
			"capabilities":   caps,
			"origin":         req.Origin,
			"outcome":        req.Outcome,
			"correlation_id": correlationID,
		},
	}
	r := audit.Default().Record(ev)
	if !r.OK {
		c.JSON(http.StatusInternalServerError, coreapi.Fail(codePluginViewGrantAuditFailed,
			"audit emission failed — capability NOT delivered to iframe: "+r.Error()))
		return
	}
	c.JSON(http.StatusOK, coreapi.OK(pluginViewCapabilityGrantResponse{
		CorrelationID: correlationID,
	}))
}

// newCorrelationID generates a UUIDv4 used as the handler-authoritative
// correlation key for plugin-view capability-grant audit rows. Mirrors
// pkg/account/routes.go serverRequestID() — RFC 4122 §4.4 random UUID
// with version + variant bits set. Returns the empty string on
// core.RandomBytes failure; the audit row tolerates the missing field.
//
// CoreGO gap: core.UUIDv4 doesn't exist yet (logged at
// project_corego_export_gaps). Local stand-in until the export lands.
//
// Usage example:
//
//	correlationID := newCorrelationID()
//	ev.Meta["correlation_id"] = correlationID
//	c.JSON(http.StatusOK, coreapi.OK(pluginViewCapabilityGrantResponse{
//	    CorrelationID: correlationID,
//	}))
func newCorrelationID() string {
	r := core.RandomBytes(16)
	if !r.OK {
		return ""
	}
	b, ok := r.Value.([]byte)
	if !ok || len(b) != 16 {
		return ""
	}
	// RFC 4122 §4.4 — version 4 (random) UUID. Top nibble of
	// time_hi_and_version (byte 6) = 0100 = 4. Top two bits of
	// clock_seq_hi_and_reserved (byte 8) = 10 (variant 1).
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return core.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
