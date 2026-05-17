// SPDX-Licence-Identifier: EUPL-1.2

// Package auth is the caller-tier substrate for the Lethean Desktop:
// one named primitive (CallerTier + CallerIdentity + Caller extractor +
// Require gate) that Wails-bound, action-bus, and HTTP consumers share
// for method-entry tier discrimination.
//
// Substrate (B1) ship only — this file ships the enum + value types +
// String() formatting. Setters (AsRenderer / AsOperator / AsCron etc),
// extractor (Caller), gate (Require), and HTTP bridge (FromGin) live
// in auth.go. Consumer migration (pkg/tasks, pkg/queue, pkg/server
// middleware, pkg/desktop wrapper-drift guard) is sequenced as units
// B2 / B3 / B4 / B5 per RFC §6 + §10. NO consumer migrates at B1.
//
// Designed per code/lthn/desktop/auth/RFC.tier-auth-substrate.md v2
// (CLEAN-FOR-IMPL after Cerberus #66 DREAD closure-pass #68 / Mantis
// #1721). v2 folded:
//
//   - ADD-1 / #1731 — Job-carries-EnqueuerCaller cascade mechanism
//     (consumed by B3 pkg/queue retrofit; substrate ships AsCascade
//     here so the worker dispatch path has the stamp to call).
//   - ADD-3 / #1733 — AsCron(handler) wrapper that stamps TierCron on
//     dispatch (the QueueHandler-wrapper variant ships when B3 lands
//     the Queue handler type; this file ships the context-level
//     stamper AsCron so the wrapper can compose around it).
//   - ADD-5 / #1735 — gin → core CallerIdentity bridge (FromGin
//     primitive ships here; B4 pkg/server middleware calls it from
//     WithBootstrapAndSessionAuth).
//
// Per RFC §4.1 audit-table: every CallerTier enum value below traces
// SETTER + PERSISTENCE + CONSUMER. This file is the PERSISTENCE leg
// (the enum value defining what can be carried on a Core context);
// auth.go is the SETTER leg (the AsX / WithCaller / SetCaller helpers
// that stamp identity onto the context); the CONSUMER leg is Require
// here (auth.go) plus per-package method-entry gates landing in B2+.

package auth

// CallerTier is the substrate-level identity tier carried on a Core
// context for the duration of a single call chain. Tiers are a SET,
// not a lattice — privilege relations are explicit per-method (the
// Require allow-list at each gate), NOT "higher tier always wins".
// Implicit ordering is precisely how F-pattern D (cascade-inheritance
// leak) happens; the substrate refuses it.
//
//	// Setter usage (B1 substrate ship — see auth.go for the helpers):
//	ctx := auth.AsRenderer(c.Context(), "account-snider", "wails")
//
//	// Gate usage (B2+ consumer adoption shape):
//	id, ok := auth.Require(c, "tasks.Service.Create",
//	    auth.TierOperator, auth.TierRenderer)
//	if !ok {
//	    return core.Fail(core.E("tasks.Service.Create",
//	        "tier not permitted", nil))
//	}
//	issue.Reporter = id.Subject // ENFORCE-not-claim
type CallerTier string

const (
	// TierOperator is a local human past the auth-gate (LTHN-SESS-1
	// session token, LocalKey static bearer, or bootstrap-token in
	// the chicken-egg root window). Subject is account_id when a
	// session is present; empty during bootstrap (pre-identity).
	// CLI entrypoints stamp TierOperator per Q-6 ruling (TierCLI
	// collapsed; CLI is operator-by-floor).
	TierOperator CallerTier = "operator"

	// TierRenderer is the embedded WebView IPC originator. User-driven
	// but the renderer surface is hostile-shaped (XSS / 3rd-party Lit
	// view / hostile plugin loaded under @dappcore/ui). Subject is the
	// renderer-session account_id when a session is live.
	TierRenderer CallerTier = "renderer"

	// TierPlugin is reserved for renderer-context plugin iframes loaded
	// via @dappcore/ui. At substrate today TierPlugin == TierRenderer
	// (Q-5 ruling); per-plugin identity refinement is DEFERRED until
	// @dappcore/ui injects a plugin-identifier into the IPC envelope.
	// Enum value retained for forward-compat (so allow-lists can
	// already name it) and forensic clarity (audit can distinguish
	// when the upstream surface eventually does).
	TierPlugin CallerTier = "plugin"

	// TierCascade is a c.Action handler firing in response to another
	// tier's call. RFC §4.5 policy: tier INHERITS the trigger; the
	// cascade-tier handler sees TierCascade with the TRIGGER's Subject
	// + Source preserved (no silent promotion). Mechanism per §4.5.1
	// (Job-carries-EnqueuerCaller) lands in B3 — substrate ships the
	// AsCascade setter so the worker dispatch path has it to call.
	TierCascade CallerTier = "cascade"

	// TierCron is a scheduled-job dispatch path stamped via the
	// auth.AsCron wrapper at cron-task registration time (§4.5.2).
	// Cron jobs are by-construction enqueued by the scheduler, not by
	// IPC — the wrapper makes the trust explicit at the registration
	// site. Allow-listing TierCron means "callers that came through
	// the cron-scheduler entry point", NOT "callers that happen to be
	// intra-Go" (that's TierInternal).
	TierCron CallerTier = "cron"

	// TierCLI is COLLAPSED to TierOperator at substrate (Q-6 ruling).
	// Retained as an enum value for forensic clarity in audit events
	// (operator answering "was this dispatched from interactive CLI
	// or from the desktop UI?"); no distinct gate semantics — any
	// Require allow-listing TierCLI behaves identically to allow-
	// listing TierOperator.
	TierCLI CallerTier = "cli"

	// TierInternal is the floor for intra-Go callers (one Service
	// calling another via core.ServiceFor, top-of-main startup paths,
	// test seeds). Zero-CallerIdentity on a Core context resolves
	// here per Caller's deny-by-default contract — Require with an
	// empty allow-list intersecting TierInternal still rejects, so
	// "default tier exists" does NOT relax the gate.
	TierInternal CallerTier = "internal"
)

// String returns the wire-format identifier of the tier. The CallerTier
// type is already a string alias, but the method satisfies the
// fmt.Stringer interface and gives consumers a stable formatter for
// audit-event / log emission.
//
//	t := auth.TierRenderer
//	_ = t.String() // "renderer"
func (t CallerTier) String() string { return string(t) }

// CallerIdentity is the value persisted onto a Core context to answer
// two questions at every gated method entry:
//
//   - Tier: which substrate-level tier originated this call? (drives
//     Require allow-listing).
//   - Subject: which user/account/plugin is this call attributable to?
//     (drives ENFORCE-not-claim — see RFC §5.3).
//   - Source: freeform breadcrumb of WHERE the tier was stamped (for
//     audit-trail forensics; "wails", "http-bearer:bootstrap",
//     "http-session:LTHN-SESS-1", "queue-worker", "action-bus",
//     "cron-worker", "cli").
//
// A zero-value CallerIdentity is meaningful: it resolves to
// {Tier: TierInternal, Subject: "", Source: ""}. Consumers that
// receive a zero identity from Caller and rely on Subject for record
// attribution must explicitly elevate via AsOperator / AsRenderer
// before persisting (zero Subject would otherwise blank the field on
// disk).
//
//	id := auth.Caller(c)
//	if id.Subject == "" {
//	    return core.Fail(core.E("tasks.Service.Create",
//	        "caller subject empty; refusing to persist", nil))
//	}
//	issue.Reporter = id.Subject
type CallerIdentity struct {
	// Tier is the substrate tier the caller was stamped with.
	Tier CallerTier

	// Subject is the per-tier identity:
	//   - TierOperator / TierRenderer with session: account_id
	//   - TierPlugin (today): account_id of the hosting renderer
	//     (per-plugin identity DEFERRED — Q-5)
	//   - TierCascade: the TRIGGER's Subject (preserved on inherit)
	//   - TierCron / TierInternal: empty (no human attribution)
	Subject string

	// Source is the freeform breadcrumb (see CallerIdentity doc above).
	// Audit consumers query by Source to reconstruct call origin
	// independent of Tier; e.g. distinguishing "http-bearer:bootstrap"
	// (TierOperator stamped during the chicken-egg root window) from
	// "http-session:LTHN-SESS-1" (TierOperator stamped from a
	// post-bootstrap session) when both report Tier=TierOperator.
	Source string
}

// String returns a stable formatter for audit-event / log emission.
// Shape: "tier=<tier> subject=<subject> source=<source>". Empty
// Subject / Source render as "<empty>" to keep the field separator
// unambiguous when grepping audit emit.
//
//	id := auth.CallerIdentity{Tier: auth.TierRenderer, Subject: "acct-1", Source: "wails"}
//	_ = id.String() // "tier=renderer subject=acct-1 source=wails"
func (id CallerIdentity) String() string {
	subject := id.Subject
	if subject == "" {
		subject = "<empty>"
	}
	source := id.Source
	if source == "" {
		source = "<empty>"
	}
	return "tier=" + id.Tier.String() + " subject=" + subject + " source=" + source
}
