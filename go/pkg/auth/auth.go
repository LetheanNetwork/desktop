// SPDX-Licence-Identifier: EUPL-1.2

// auth.go — substrate API for the caller-tier package (Unit B1 ship).
//
// Two integration shapes are provided in lockstep, because the
// substrate must serve both Context-shaped callers (gin handler chain,
// queue worker dispatch) and Core-shaped callers (the bulk of the
// desktop's Wails-bound services that hold *core.Core, not Context):
//
//   - Context-shaped (canonical): WithCaller / Caller / Require accept
//     and return core.Context (alias of stdlib context.Context). The
//     stamp lives on a derived context via core.WithValue under an
//     unexported key — opaque to callers, indelible to forge attempts
//     from outside the package. This is the shape the gin → core
//     bridge (#1735 / §4.6.1) uses.
//
//   - Core-shaped (consumer convenience): CallerFromCore / SetCaller
//     accept *core.Core. Read path resolves via c.Context().Value().
//     Write path uses a package-level sync.Map keyed by the *core.Core
//     pointer because core.Core does NOT today expose a context
//     setter (c.context field is unexported and only initialised in
//     core's runtime.go:54 / contract.go:141 — see CoreGO-surface-gap
//     note at the bottom of this file). Read order is sync.Map FIRST
//     then context.Value — so a Core-shaped SetCaller is observable
//     from a Context-shaped Caller(c.Context()) call, and vice versa.
//     The sidecar map's keys are pointer-identity, so a SetCaller is
//     scoped to the specific *core.Core value that received it
//     (test setups using fresh New() per case stay isolated).
//
// Design per RFC.tier-auth-substrate.md v2 §4.3, §4.5.2, §4.6.1.
// Cerberus #66 DREAD folds (ADD-1/3/5) all land here at substrate
// shape; the cascade mechanism (Job.EnqueuerCaller persistence) and
// the AsCron QueueHandler wrapper variant land in B3 pkg/queue
// retrofit. This file ships the AsCascade and AsCron Context-stampers
// so the B3 retrofit has something to call.
//
// Audit-emit on Require deny is INTENTIONALLY OUT OF SCOPE for B1 —
// the audit.EventTierReject event type ships under Unit B2/B3 to
// avoid a cyclic substrate ↔ audit dependency at the bottom of the
// import graph. Until then, Require deny is a quiet (caller-checks-ok)
// signal; the gate behaviour is correct, only the forensic trail is
// deferred. See B2 done-criteria for the audit-emit retrofit.

package auth

import (
	"sync"

	core "dappco.re/go"
	"github.com/gin-gonic/gin"

	"dappco.re/lthn/desktop/pkg/audit"
)

// callerKey is the unexported context-value key used to attach
// CallerIdentity onto a core.Context. Unexported by design: external
// packages cannot stamp identity by passing a colliding key; only
// this package's setters (WithCaller / AsRenderer / etc) can do so.
type callerKey struct{}

// coreSidecar holds Core-shaped CallerIdentity stamps keyed by the
// *core.Core pointer. The map is the workaround for core.Core lacking
// a public context-replacement API today (see CoreGO surface gap
// noted in the package doc). Pointer-identity scoping keeps stamps
// per-Core; test setups that build a fresh core.New() per case do
// not leak identity across cases. Cleared via ClearSidecar (test
// utility) to make leak detection mechanical.
//
// Read+write paths use sync.Map's Load / Store / Delete primitives —
// no external lock needed; the map is correct under concurrent
// readers (e.g. multiple Wails IPC handlers operating on the same
// *core.Core in parallel).
var coreSidecar sync.Map // map[*core.Core]CallerIdentity

// WithCaller returns a derived Context carrying the supplied
// CallerIdentity. The original Context is NOT mutated; the returned
// Context is what downstream callers must observe to see the stamp.
//
//	ctx := auth.WithCaller(c.Context(), auth.CallerIdentity{
//	    Tier: auth.TierOperator, Subject: "snider", Source: "cli",
//	})
//	r := svc.DoThing(ctx, opts)
func WithCaller(ctx core.Context, ident CallerIdentity) core.Context {
	return core.WithValue(ctx, callerKey{}, ident)
}

// AsOperator returns a Context stamped TierOperator with the supplied
// subject (account_id; may be empty for bootstrap-window callers) and
// source breadcrumb. Convenience wrapper over WithCaller for the
// most common stamping call site.
//
//	ctx := auth.AsOperator(c.Context(), "account-snider", "http-session:LTHN-SESS-1")
func AsOperator(ctx core.Context, subject, source string) core.Context {
	return WithCaller(ctx, CallerIdentity{Tier: TierOperator, Subject: subject, Source: source})
}

// AsRenderer returns a Context stamped TierRenderer. Used by the
// Wails-binding interceptor (Shape (a.i) / §4.4 — lands in B5 desktop
// wrapper) to stamp tier on every method entry from the WebView.
//
//	ctx := auth.AsRenderer(c.Context(), "account-snider", "wails")
func AsRenderer(ctx core.Context, subject, source string) core.Context {
	return WithCaller(ctx, CallerIdentity{Tier: TierRenderer, Subject: subject, Source: source})
}

// AsPlugin returns a Context stamped TierPlugin. Today TierPlugin
// behaves identically to TierRenderer at the gate (Q-5 substrate-
// today ruling); the helper exists so plugin-aware call sites can
// already stamp the more-specific tier — when @dappcore/ui injects
// plugin-identifier into the IPC envelope, only the call-site changes,
// not the substrate.
//
//	ctx := auth.AsPlugin(c.Context(), "plugin-marketplace-v1", "wails")
func AsPlugin(ctx core.Context, subject, source string) core.Context {
	return WithCaller(ctx, CallerIdentity{Tier: TierPlugin, Subject: subject, Source: source})
}

// AsCascade returns a Context stamped TierCascade with the TRIGGER's
// subject + source preserved (inherit-not-promote per §4.5). Called
// by the queue worker dispatch path (B3 retrofit) after reconstructing
// CallerIdentity from the persisted Job.EnqueuerCaller columns. The
// preserved subject/source lets downstream gates and audit consumers
// see WHO triggered the cascade, while Tier=TierCascade ensures the
// handler cannot silently inherit the trigger's authority.
//
//	// In B3 queue worker dispatch:
//	ctx := auth.AsCascade(c.Context(), job.EnqueuerSubject, job.EnqueuerSource)
//	return handler(ctx, job)
func AsCascade(ctx core.Context, subject, source string) core.Context {
	return WithCaller(ctx, CallerIdentity{Tier: TierCascade, Subject: subject, Source: source})
}

// AsCron returns a Context stamped TierCron. Called by the cron
// scheduler at dispatch time (B3 retrofit lands the QueueHandler
// wrapper variant per §4.5.2 — `AsCron(QueueHandler) QueueHandler`).
// The Subject is empty by construction (cron jobs are not human-
// attributed); Source is fixed "cron-worker" so audit-trail grep can
// identify cron-originated calls without ambiguity.
//
//	ctx := auth.AsCron(c.Context())
//	return backupHandler(ctx, job)
func AsCron(ctx core.Context) core.Context {
	return WithCaller(ctx, CallerIdentity{Tier: TierCron, Subject: "", Source: "cron-worker"})
}

// AsCLI returns a Context stamped TierCLI. The CLI is collapsed to
// TierOperator semantically (Q-6 ruling) so allow-listing TierCLI
// alone is equivalent to allow-listing TierOperator; the enum value
// is retained for forensic-trail clarity. Source is "cli".
//
//	ctx := auth.AsCLI(c.Context(), "snider")
func AsCLI(ctx core.Context, subject string) core.Context {
	return WithCaller(ctx, CallerIdentity{Tier: TierCLI, Subject: subject, Source: "cli"})
}

// AsInternal returns a Context stamped TierInternal. Used to make
// the intra-Go floor EXPLICIT at the rare call site where it matters
// (e.g. resetting a context that was previously stamped to a more-
// privileged tier to ensure a downstream call sees only the floor).
// In the common case a Context with no stamp already resolves to
// TierInternal via Caller; explicit AsInternal is the audit-clarity
// shape.
//
//	ctx := auth.AsInternal(c.Context())
func AsInternal(ctx core.Context) core.Context {
	return WithCaller(ctx, CallerIdentity{Tier: TierInternal, Subject: "", Source: "internal"})
}

// Caller extracts the CallerIdentity stamped on the Core context.
// Resolution order:
//
//  1. Read CallerIdentity from c.Context().Value(callerKey{}).
//  2. If absent, read from the Core-shaped sidecar map (set via
//     SetCaller).
//  3. If both absent, return zero-value CallerIdentity which resolves
//     to {Tier: TierInternal, Subject: "", Source: ""} — the
//     intra-Go floor per RFC §4.1.
//
// The zero-value resolution is the floor: callers that need the
// renderer/operator gate to FIRE on missing tier should call
// Require, which deny-by-defaults TierInternal against any explicit
// allow-list that doesn't name it.
//
//	id := auth.Caller(c)
//	switch id.Tier {
//	case auth.TierOperator: // ...
//	case auth.TierRenderer: // ...
//	default: // floor / unstamped
//	}
func Caller(c *core.Core) CallerIdentity {
	if c == nil {
		return CallerIdentity{Tier: TierInternal}
	}
	if ctx := c.Context(); ctx != nil {
		if v := ctx.Value(callerKey{}); v != nil {
			if id, ok := v.(CallerIdentity); ok {
				return ensureTier(id)
			}
		}
	}
	if v, ok := coreSidecar.Load(c); ok {
		if id, ok := v.(CallerIdentity); ok {
			return ensureTier(id)
		}
	}
	return CallerIdentity{Tier: TierInternal}
}

// CallerFromContext is the Context-shaped read companion to Caller.
// Used by call sites that hold a core.Context directly (gin handlers
// post-bridge, queue worker dispatch) without a *core.Core in scope.
//
//	id := auth.CallerFromContext(ctx)
//	if id.Tier != auth.TierOperator { return errForbidden }
func CallerFromContext(ctx core.Context) CallerIdentity {
	if ctx == nil {
		return CallerIdentity{Tier: TierInternal}
	}
	if v := ctx.Value(callerKey{}); v != nil {
		if id, ok := v.(CallerIdentity); ok {
			return ensureTier(id)
		}
	}
	return CallerIdentity{Tier: TierInternal}
}

// ensureTier normalises a stored CallerIdentity so that an empty Tier
// (someone called WithCaller(ctx, CallerIdentity{Subject: "x"}) without
// naming a tier) still resolves to the TierInternal floor rather than
// the empty-string tier (which would never match any allow-list and
// would silently break gates).
func ensureTier(id CallerIdentity) CallerIdentity {
	if id.Tier == "" {
		id.Tier = TierInternal
	}
	return id
}

// Deprecated: prefer auth.WithCaller(ctx, ident) + thread the derived
// Context forward (Mantis #1756 partial / RFC.tier-auth-substrate v3.1
// §4.6.2 Option 3 soft-deprecation). The sidecar map this writes to is
// the workaround documented at the bottom of this file for the absence
// of a CoreGO context-replacement API; once the Service-method
// migration ships (Phase 1.5.1 follow-on ticket — closes #1756 F-2
// cascade fully) the sidecar will be deleted and SetCaller will be
// removed. Call sites that already hold a core.Context (gin handler
// chain post-bridge, queue worker dispatch, tasks Service methods) MUST
// migrate to WithCaller; SetCaller stays functional until the cascade
// closes so the H#255 wrapper-port lane and existing pkg/server bridge
// keep working without coordinated cutover.
//
// SetCaller stamps the supplied CallerIdentity onto the Core-shaped
// sidecar map keyed by c. The stamp is observable from subsequent
// Caller(c) reads in the SAME *core.Core instance. The Context-
// shaped Caller path (c.Context()) does NOT see the sidecar stamp
// directly — call sites that need both shapes synchronised should
// prefer WithCaller + thread the returned Context forward.
//
// Used by the HTTP middleware (§4.6.1 Option A / #1735 bridge): the
// middleware receives a *core.Core at construction, sets the
// CallerIdentity per request, and downstream service-method
// dispatches read via Caller(c). This is the ONLY supported entry
// point for the Core-shaped stamp; do NOT use SetCaller from
// service-internal code paths (use WithCaller and thread the Context
// through, so the stamp clears naturally with the call chain).
//
//	// In HTTP middleware:
//	ident := auth.CallerIdentity{
//	    Tier: auth.TierOperator, Subject: claims.AccountID,
//	    Source: "http-session:LTHN-SESS-1",
//	}
//	auth.SetCaller(c, ident)
//	gctx.Next()
//
// Lifetime: a SetCaller stamp persists for the lifetime of the
// *core.Core pointer; HTTP middleware MUST call ClearCaller (or use
// the WithCaller-and-thread shape) to avoid stamps leaking across
// requests when the same *core.Core is reused. Per-request *core.Core
// derivation is the cleaner pattern; ClearCaller is the fallback.
//
// Race-safety note: WithCaller is race-safe by construction because
// core.WithValue returns an immutable derived ctx. SetCaller is also
// race-safe (sync.Map under the hood) but pointer-identity scoping
// across concurrent requests sharing one *core.Core is the source of
// the H#256 surfacing — Context-derived ctx-per-request eliminates
// the shared-pointer dimension entirely.
func SetCaller(c *core.Core, ident CallerIdentity) {
	if c == nil {
		return
	}
	coreSidecar.Store(c, ensureTier(ident))
}

// Deprecated: companion to SetCaller — removed in lockstep when the
// Phase 1.5.1 cascade closes. Prefer the WithCaller-and-thread shape
// so the stamp clears naturally with the call chain (no explicit
// cleanup required).
//
// ClearCaller removes any sidecar stamp for c. Idempotent — safe to
// call when no stamp exists. HTTP middleware should call ClearCaller
// in a deferred block at end of request when reusing *core.Core
// across requests (the per-request-derived-Core pattern is preferred
// but ClearCaller is the fallback when that's not yet wired).
//
//	auth.SetCaller(c, ident)
//	defer auth.ClearCaller(c)
//	gctx.Next()
func ClearCaller(c *core.Core) {
	if c == nil {
		return
	}
	coreSidecar.Delete(c)
}

// FromGin extracts CallerIdentity from a gin.Context (where the
// bootstrap_auth / session middleware stashed it under predictable
// keys) and stamps it onto the supplied Core context. Returns the
// derived Context; callers MUST observe the returned value.
//
// Bridge keys consulted on the gin context (set by B4 retrofit of
// pkg/server middleware):
//
//   - "lthn.auth.tier"     — string CallerTier
//   - "lthn.auth.subject"  — string account_id / "" for bootstrap
//   - "lthn.auth.source"   — string source breadcrumb
//
// Missing keys resolve to TierInternal (deny-by-default at gate).
// This is the fallback shape used when a handler does NOT receive a
// *core.Core from the chain — for the common case the middleware
// uses SetCaller directly on its bound *core.Core per §4.6.1.
//
//	// In a gin handler that has *core.Core in scope:
//	ctx := auth.FromGin(gctx, c.Context())
//	r := svc.DoThing(ctx, opts)
func FromGin(gctx *gin.Context, ctx core.Context) core.Context {
	if gctx == nil {
		return ctx
	}
	tier, _ := gctx.Get(GinKeyTier)
	subject, _ := gctx.Get(GinKeySubject)
	source, _ := gctx.Get(GinKeySource)
	tierStr, _ := tier.(string)
	if tierStr == "" {
		return ctx
	}
	subjectStr, _ := subject.(string)
	sourceStr, _ := source.(string)
	return WithCaller(ctx, CallerIdentity{
		Tier:    CallerTier(tierStr),
		Subject: subjectStr,
		Source:  sourceStr,
	})
}

// Gin context key constants for the FromGin bridge. Exported so the
// B4 pkg/server middleware (the producer side) and any handler-level
// FromGin caller (the consumer side) reference the same string
// literal — typo-proof per design_strings_as_exported_constants.
const (
	GinKeyTier    = "lthn.auth.tier"
	GinKeySubject = "lthn.auth.subject"
	GinKeySource  = "lthn.auth.source"
)

// Require gates a method-entry against an allow-list of acceptable
// tiers. Returns the resolved CallerIdentity AND an ok bool. On !ok,
// the caller MUST return core.Fail with a tier-not-permitted error
// for the supplied op — Require does NOT panic and does NOT short-
// circuit; the explicit caller-return shape keeps the gate visible
// at every call site.
//
//	id, ok := auth.Require(c, "tasks.Service.Create",
//	    auth.TierOperator, auth.TierRenderer)
//	if !ok {
//	    return core.Fail(core.E("tasks.Service.Create",
//	        "tier not permitted: " + auth.Caller(c).Tier.String(),
//	        nil))
//	}
//	issue.Reporter = id.Subject // ENFORCE-not-claim
//
// Deny-by-default: an empty allowed list rejects everything (even
// TierInternal) — Require with no tiers is the explicit "this method
// is forever locked" shape. Pass auth.TierInternal explicitly to
// allow intra-Go callers.
//
// Op-name convention: "<package>.<Type>.<Method>" — drives audit
// queries (event="auth.tier.reject" AND op=<method>) and log
// breadcrumbs. The Op-name string is INTENTIONALLY unstructured here;
// the audit-emit retrofit in B2/B3 promotes it to a typed field on
// EventTierReject.
//
// AUDIT-EMIT (Cerberus #73 F-4 / Mantis #1758 / RFC §4.7): on deny,
// records audit.EventTierReject via audit.Default().Record. The cycle
// risk noted in earlier comments was resolved by verification —
// pkg/audit does NOT import pkg/auth (the substrate-tier audit-cluster
// adopter pattern); the dependency is one-way. Meta-key shape mirrors
// the EventTierReject doc-block at pkg/audit/types.go.
//
//	id, ok := auth.Require(c, "tasks.Service.Create",
//	    auth.TierOperator, auth.TierRenderer)
//	if !ok {
//	    return core.Fail(core.E("tasks.Service.Create",
//	        "tier not permitted: " + auth.Caller(c).Tier.String(),
//	        nil))
//	}
//	issue.Reporter = id.Subject // ENFORCE-not-claim
func Require(c *core.Core, op string, allowed ...CallerTier) (CallerIdentity, bool) {
	id := Caller(c)
	for _, t := range allowed {
		if id.Tier == t {
			return id, true
		}
	}
	emitTierReject(op, id, allowed)
	return id, false
}

// RequireFromContext is the Context-shaped Require companion. Same
// semantics as Require; used by call sites that hold core.Context
// directly (queue worker dispatch, gin handler post-bridge).
//
//	id, ok := auth.RequireFromContext(ctx, "queue.Worker.Dispatch",
//	    auth.TierCascade, auth.TierCron, auth.TierOperator)
//	if !ok { return core.Fail(...) }
func RequireFromContext(ctx core.Context, op string, allowed ...CallerTier) (CallerIdentity, bool) {
	id := CallerFromContext(ctx)
	for _, t := range allowed {
		if id.Tier == t {
			return id, true
		}
	}
	emitTierReject(op, id, allowed)
	return id, false
}

// emitTierReject records the substrate-tier audit.EventTierReject row
// for a deny in Require / RequireFromContext. Bounded-length truncation
// on Subject + Source defends against an upstream caller stamping a
// longer-than-bounded breadcrumb (per Cerberus #1465 closure-only-scope
// discipline). allowed_tiers is joined as a comma-separated literal so
// the forensic walker can distinguish a renderer-against-operator-only
// deny from a renderer-against-internal-only deny without re-running
// the call-site.
//
//	emitTierReject("tasks.Service.Create", id, []CallerTier{TierOperator})
func emitTierReject(op string, id CallerIdentity, allowed []CallerTier) {
	_ = audit.Default().Record(audit.Event{
		Event:   EventTierRejectEventName,
		TS:      core.Now().UTC().Unix(),
		Scope:   "auth",
		Outcome: audit.OutcomeDenied,
		Meta: map[string]any{
			"op":             truncateMeta(op),
			"caller_tier":    string(id.Tier),
			"caller_subject": truncateMeta(id.Subject),
			"caller_source":  truncateMeta(id.Source),
			"allowed_tiers":  joinTiers(allowed),
		},
	})
}

// EventTierRejectEventName mirrors audit.EventTierReject through the
// auth package so call-site greps locate the literal at the deny site
// even if the auth package later vendors a private alias. Sourced from
// the audit package's reserved-schema const to keep the parity-grep
// contract intact.
const EventTierRejectEventName = audit.EventTierReject

// metaMaxLen bounds Subject / Source / op Meta values per Cerberus
// #1465. Sized for typical session-breadcrumb lengths (account IDs,
// session-token UUIDs, dotted op names) with headroom; anything longer
// is an upstream caller bug and the truncated form still discriminates
// for forensic queries.
const metaMaxLen = 256

// truncateMeta enforces metaMaxLen on a Meta string value. Empty
// strings pass through unchanged so the forensic walker can grep
// `caller_subject=""` literally to identify unstamped denies.
//
//	truncateMeta("account-snider") // "account-snider"
//	truncateMeta(strings.Repeat("x", 1024)) // 256-char prefix
func truncateMeta(s string) string {
	if len(s) <= metaMaxLen {
		return s
	}
	return s[:metaMaxLen]
}

// joinTiers stringifies a CallerTier slice as a comma-joined literal
// without importing strings (banned per AX-6). Empty slice returns
// "" so the forensic walker can distinguish "no tiers allowed"
// (forever-locked shape) from a populated allow-list cleanly.
//
//	joinTiers([]CallerTier{TierOperator, TierRenderer}) // "operator,renderer"
//	joinTiers(nil) // ""
func joinTiers(tiers []CallerTier) string {
	if len(tiers) == 0 {
		return ""
	}
	out := ""
	for i, t := range tiers {
		if i > 0 {
			out += ","
		}
		out += string(t)
	}
	return out
}

// Elevate is the explicit privilege-promotion shape. Returns a
// derived Context stamped with the target tier IFF the current
// caller is permitted to elevate to it. Returns (ctx, false) on
// deny — caller MUST NOT use the returned context (it equals the
// input).
//
// Elevation rules (RFC §4.5 inherit-not-promote + §4.5.2 AsCron):
//
//   - TierInternal can elevate to any tier (intra-Go is the trust
//     root for explicit elevation; e.g. test seeds, service-internal
//     transitions like cron-scheduler stamping AsCron).
//   - TierOperator can elevate to TierCascade or TierCron (operator-
//     triggered cron-style work runs at the target tier).
//   - Any other tier MAY NOT elevate (TierRenderer cannot become
//     TierOperator — the F-pattern D failure mode the substrate
//     refuses).
//
// The from→to matrix is intentionally TIGHT; broaden via explicit
// per-method gate rather than relaxing the elevation policy.
//
//	ctx, ok := auth.Elevate(c.Context(), auth.TierCron, "", "cron-worker")
//	if !ok { return core.Fail(core.E("...", "elevation denied", nil)) }
//	return handler(ctx, job)
func Elevate(ctx core.Context, to CallerTier, subject, source string) (core.Context, bool) {
	cur := CallerFromContext(ctx)
	if !canElevate(cur.Tier, to) {
		return ctx, false
	}
	return WithCaller(ctx, CallerIdentity{Tier: to, Subject: subject, Source: source}), true
}

// canElevate encodes the from→to matrix documented on Elevate. Kept
// in a separate helper so the policy is one grep away ("canElevate")
// for auditors and so future relaxations land as a single edit.
func canElevate(from, to CallerTier) bool {
	if from == TierInternal {
		return true
	}
	if from == TierOperator {
		return to == TierCascade || to == TierCron
	}
	return false
}

// ----------------------------------------------------------------------
// CoreGO surface-gap note (Mantis #1738+ candidate, file under
// project_corego_export_gaps)
//
// core.Core today does not expose a context-replacement API — the
// `c.context` field is unexported and only initialised inside the core
// package's runtime.go:54 / contract.go:141. As a result, the
// Core-shaped WithCaller(c, ident) *core.Core signature requested by
// the original B1 brief is NOT IMPLEMENTABLE without a CoreGO surface
// change (e.g. `func (c *Core) WithContext(ctx Context) *Core` that
// returns a shallow-clone with the supplied context). The sidecar
// sync.Map pattern shipped here is the pragmatic workaround that
// keeps the Core-shaped read path (Caller(c)) functional while we
// surface the gap upstream.
//
// Preferred long-term shape: a CoreGO export of either
//   - `func (c *Core) WithContext(ctx Context) *Core` (shallow clone)
//   - or a public `core.WithCoreContext(c *Core, ctx Context) *Core`
// either of which lets WithCaller stamp identity onto the Core
// directly without a sidecar. When that lands, SetCaller and
// ClearCaller migrate to the Core-context shape and the sidecar
// disappears.
// ----------------------------------------------------------------------
