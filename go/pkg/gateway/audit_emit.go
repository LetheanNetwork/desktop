// SPDX-Licence-Identifier: EUPL-1.2

// audit_emit.go — typed pkg/audit emit helpers for the gateway
// dispatch boundary. Cerberus #57 F-2 (Mantis #1700) — Repudiation
// gap close. The plugin → host data dispatch path on Handle previously
// emitted ZERO audit events; every /v1/api/gateway/<scope>/<mode>
// call was forensically silent even though gateway is the LAST mute
// boundary in the audit cluster (sandbox / runner / process /
// marketplace already adopt — H#181 / H#188 / H#193 / H#208).
//
// Helper functions wrap audit.Default().Record so the Handle call-site
// stays readable — one emit per gate / dispatch / completion point
// rather than inline-constructed audit.Event literals at four sites.
// The helpers enforce the Cerberus #1465 closure-only-scope discipline
// at the boundary: only the safe Meta keys (bundle_id / route_pattern /
// method / reason / error_code) are accepted; request bodies, headers,
// query params, and X-Bundle-Token bytes never reach the substrate.
//
// Usage example (internal):
//
//	emitDispatchRequested(bundleID, routePattern, ctx.Request.Method)
//	if !r.OK {
//	    emitDispatchFailed(bundleID, routePattern, ctx.Request.Method, r)
//	    return
//	}
//	emitDispatchSucceeded(bundleID, routePattern, ctx.Request.Method)

package gateway

import (
	core "dappco.re/go"

	"dappco.re/lthn/desktop/pkg/audit"
)

// gatewayScope is the Event.Scope literal stamped on every typed audit
// row emitted from this package. Closed-set per RFC.stage-f.md §2;
// the Operations panel's facet chrome learns this literal in the same
// commit a new scope ships.
const gatewayScope = "gateway"

// Closed-set Meta["reason"] literals stamped on EventGatewayDispatchRejected
// rows. Mirrors marketplace's reasonHostNotAllowed discipline — reserved
// literals so a downstream chip-filter UI / forensic walker can
// categorise without re-running the gate logic.
const (
	reasonInvalidBundleToken = "invalid_bundle_token"
	reasonMissingBundleID    = "missing_bundle_id"
	reasonScopeUnavailable   = "scope_unavailable"
	reasonPermissionDenied   = "permission_denied"
)

// routePattern joins the scope + mode params into the public URL path
// shape stamped by pkg/server's POST route. Kept as a helper so the
// pkg/audit Meta key matches the gin route shape one-to-one.
//
// Usage example (internal):
//
//	pattern := routePattern("project.metadata", "read")
//	// pattern == "project.metadata:read"
func routePattern(scope, mode string) string { return scope + ":" + mode }

// emitDispatchRequested fires the Requested row AFTER the auth /
// identity / scope / permission gates pass and BEFORE the registered
// scope Handler runs — so a crash inside the Handler still leaves the
// dispatch decision in the audit substrate. Cerberus #57 F-2 (Mantis
// #1700) — pairs with Succeeded / Failed below.
//
// Usage example (internal):
//
//	emitDispatchRequested(bundleID, routePattern(scope, mode), ctx.Request.Method)
func emitDispatchRequested(bundleID, pattern, method string) {
	_ = audit.Default().Record(audit.Event{
		Event:   audit.EventGatewayDispatchRequested,
		TS:      core.Now().UTC().Unix(),
		Scope:   gatewayScope,
		Outcome: audit.OutcomeOK,
		Meta: map[string]any{
			"bundle_id":     bundleID,
			"route_pattern": pattern,
			"method":        method,
		},
	})
}

// emitDispatchSucceeded fires the Succeeded row when the registered
// scope Handler returns OK. Sibling of Requested above.
//
// Usage example (internal):
//
//	if r.OK { emitDispatchSucceeded(bundleID, pattern, method) }
func emitDispatchSucceeded(bundleID, pattern, method string) {
	_ = audit.Default().Record(audit.Event{
		Event:   audit.EventGatewayDispatchSucceeded,
		TS:      core.Now().UTC().Unix(),
		Scope:   gatewayScope,
		Outcome: audit.OutcomeOK,
		Meta: map[string]any{
			"bundle_id":     bundleID,
			"route_pattern": pattern,
			"method":        method,
		},
	})
}

// emitDispatchFailed fires the Failed row when the registered scope
// Handler returns a non-OK Result. Sibling of Requested above.
//
// The Result is passed in (not the raw error prose string) so the
// audit substrate stamps the BOUNDED audit.ErrorCode(r) literal in
// the error_code Meta key instead of the raw handler prose.
// RFC.error-code-cascade.md §3 (W6) / Mantis #1717 / Cerberus #61
// C-5 — pre-W6 shape leaked plugin / handler-scope
// `*core.Err.Operation: <message>` strings that routinely echo
// caller-controlled input from upstream dependencies (HTTP error
// bodies, file paths, etc.) into the audit substrate's STRIDE-I
// surface. The type-system signature change makes the regression
// unreachable.
//
// Usage example (internal):
//
//	if !r.OK {
//	    emitDispatchFailed(bundleID, pattern, method, r)
//	    return
//	}
func emitDispatchFailed(bundleID, pattern, method string, r core.Result) {
	_ = audit.Default().Record(audit.Event{
		Event:   audit.EventGatewayDispatchFailed,
		TS:      core.Now().UTC().Unix(),
		Scope:   gatewayScope,
		Outcome: audit.OutcomeFailed,
		Meta: map[string]any{
			"bundle_id":     bundleID,
			"route_pattern": pattern,
			"method":        method,
			"error_code":    audit.ErrorCode(r),
		},
	})
}

// emitDispatchRejected fires when Handle refuses to dispatch at one
// of the auth / identity / scope / permission gates BEFORE the scope
// Handler runs. Sibling of EventMarketplaceFetchManifestRejected and
// EventSandboxSpawnRejected — Rejected distinguishes policy-block from
// runtime failure across the audit cluster.
//
// bundleID may be empty when the rejection happened before identity
// resolution (reasonInvalidBundleToken / reasonMissingBundleID); the
// emit tolerates the empty value (a downstream forensic walker
// distinguishes empty-bundle_id as "rejection before identity
// resolved" vs the populated case).
//
// Usage example (internal):
//
//	emitDispatchRejected("", pattern, method, reasonMissingBundleID)
//	emitDispatchRejected(bundleID, pattern, method, reasonPermissionDenied)
func emitDispatchRejected(bundleID, pattern, method, reason string) {
	_ = audit.Default().Record(audit.Event{
		Event:   audit.EventGatewayDispatchRejected,
		TS:      core.Now().UTC().Unix(),
		Scope:   gatewayScope,
		Outcome: audit.OutcomeDenied,
		Meta: map[string]any{
			"bundle_id":     bundleID,
			"route_pattern": pattern,
			"method":        method,
			"reason":        reason,
		},
	})
}
