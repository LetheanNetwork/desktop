// SPDX-Licence-Identifier: EUPL-1.2

// Handler registration — wires per-kind job handlers into Core's
// named-action registry under the "queue.kind.<name>" namespace.
// The worker dispatches by composing the action name from a job's
// Kind field.
//
// Why c.Action and not a parallel registry: the Core action system
// already gives us name-based dispatch, panic recovery, entitlement
// gating, and Exists() introspection. Parallel registry would be
// reinventing the wheel (per reference_corego_primitive_surface).

package queue

import (
	"sync"

	core "dappco.re/go"

	"dappco.re/lthn/desktop/pkg/auth"
)

// HandlerOptions configures one registered job handler.
type HandlerOptions struct {
	// Kind is the Job.Kind value this handler processes. The worker
	// dispatches via c.Action("queue.kind." + Kind).
	Kind string

	// Description is a short human-readable label. Used by the
	// queue/list CLI surface + future telemetry.
	Description string

	// Handler is the actual work function. Receives the Job's
	// Payload decoded into core.Options + a context. Returns
	// core.Ok / core.Fail per Core convention.
	Handler core.ActionHandler

	// PermittedTiers names the auth.CallerTier values that may
	// Enqueue against this kind. RFC v3.1 §5.1 mandatory — empty
	// is rejected at RegisterKind time with
	// ErrPermittedTiersRequired so renderer-tier elevation via
	// kind-string forge (Cerberus #73 F-3 / Mantis #1757) cannot
	// land via an under-specified RegisterKind site. Composes with
	// the outer auth.Require ALLOW gate in Enqueue (BOTH-not-
	// either per §5.0): outer Require gates the writer-surface
	// broadly, inner PermittedTiers narrows per-kind.
	//
	// Today's default-pose for migrated handlers is
	// {TierOperator, TierCascade} — matches Phase 1.5 ALLOW + audit-
	// emit posture. Phase 2 ratchet narrows kind-by-kind (e.g.
	// "backup.nightly" → {TierCron} only).
	PermittedTiers []auth.CallerTier
}

// kindRegistry is the per-Core map from kind → PermittedTiers.
// Mirrors the sync.Map keyed by *core.Core pattern used by
// pkg/auth/coreSidecar for CallerIdentity stamping — keeps per-Core
// state without polluting *core.Core's public surface or requiring a
// core/go primitive extension. Empty entry = kind not registered (or
// registered without PermittedTiers, which RegisterKind rejects).
var kindRegistry sync.Map // map[*core.Core]*core.Registry[[]auth.CallerTier]

// permittedTiersFor returns the PermittedTiers allow-list registered
// for the given kind on the given Core. Returns (nil, false) when the
// kind has no entry; the caller (Enqueue) decides whether that means
// "no handler yet — let worker.dispatch surface the missing-handler
// failure" or "reject up-front". Today's policy: Enqueue accepts an
// unregistered kind (matches pre-Phase-1.5 shape — the worker emits
// the typed "no handler" failure later); per-kind gating only fires
// when an entry exists. Phase 2 may flip to "deny-by-default at
// Enqueue if no kind registered".
func permittedTiersFor(c *core.Core, kind string) ([]auth.CallerTier, bool) {
	if c == nil || kind == "" {
		return nil, false
	}
	v, ok := kindRegistry.Load(c)
	if !ok {
		return nil, false
	}
	reg, ok := v.(*core.Registry[[]auth.CallerTier])
	if !ok {
		return nil, false
	}
	r := reg.Get(kind)
	if !r.OK {
		return nil, false
	}
	tiers, ok := r.Value.([]auth.CallerTier)
	if !ok {
		return nil, false
	}
	return tiers, true
}

// rememberPermittedTiers persists the per-kind PermittedTiers
// allow-list into the per-Core registry. RegisterKind calls this
// after validating opts.PermittedTiers is non-empty.
func rememberPermittedTiers(c *core.Core, kind string, tiers []auth.CallerTier) {
	if c == nil || kind == "" {
		return
	}
	v, _ := kindRegistry.LoadOrStore(c, core.NewRegistry[[]auth.CallerTier]())
	reg, ok := v.(*core.Registry[[]auth.CallerTier])
	if !ok {
		return
	}
	// Copy the slice so a caller mutating opts.PermittedTiers after
	// RegisterKind returns can't retroactively widen the gate.
	cp := make([]auth.CallerTier, len(tiers))
	copy(cp, tiers)
	reg.Set(kind, cp)
}

// actionNamePrefix is the namespace under which all kind handlers
// register. Kept private so consumers compose via Register, not
// raw c.Action calls — keeps the namespace consistent.
const actionNamePrefix = "queue.kind."

// ActionName returns the Core action name for a given job kind.
// Useful for callers that want to inspect handler presence via
// c.Action(queue.ActionName(kind)).Exists().
//
// Usage example:
//
//	if !c.Action(queue.ActionName("lint")).Exists() {
//	    return core.Fail(core.E("queue", "no handler for kind: lint", nil))
//	}
func ActionName(kind string) string {
	return actionNamePrefix + kind
}

// Register installs a handler for a job kind. Overwrites any
// existing handler under the same name (last-write-wins, matches
// Core's c.Action semantics). PermittedTiers is mandatory per RFC
// v3.1 §5.1 (Cerberus #73 F-3 / Mantis #1757) — empty rejects with
// ErrPermittedTiersRequired to close the renderer-tier elevation via
// kind-string forge.
//
// Usage example:
//
//	queue.RegisterKind(c, queue.HandlerOptions{
//	    Kind: "lint",
//	    Description: "Run go vet on a path",
//	    PermittedTiers: []auth.CallerTier{auth.TierOperator, auth.TierCascade},
//	    Handler: func(ctx core.Context, opts core.Options) core.Result {
//	        path := opts.String("path")
//	        return c.Process().Run(ctx, "go", "vet", path)
//	    },
//	})
func RegisterKind(c *core.Core, opts HandlerOptions) core.Result {
	if c == nil {
		return core.Fail(core.E("queue.Register", "core is nil", nil))
	}
	if opts.Kind == "" {
		return core.Fail(core.E("queue.Register", "kind is required", nil))
	}
	if opts.Handler == nil {
		return core.Fail(core.E("queue.Register", "handler is required", nil))
	}
	if len(opts.PermittedTiers) == 0 {
		// Deny-by-construction: an unspecified PermittedTiers is a
		// MISUSE, not "allow all". Per RFC v3.1 §5.1 / Cerberus #73
		// F-3: every kind MUST declare which tiers may Enqueue
		// against it. NewCode populates Err.Code so callers can
		// branch on r.Code() == ErrPermittedTiersRequired.
		return core.Fail(core.NewCode(ErrPermittedTiersRequired,
			"queue.Register: PermittedTiers is required for kind: "+opts.Kind))
	}
	rememberPermittedTiers(c, opts.Kind, opts.PermittedTiers)
	c.Action(ActionName(opts.Kind), opts.Handler)
	return core.Ok(opts.Kind)
}

// AsCron wraps a queue handler so dispatch stamps TierCron on the
// Context before invoking the wrapped fn. Use at RegisterKind time
// for handlers that are by-construction cron / scheduler-originated
// (background backups, periodic reconcile, scheduled cleanups). The
// wrapper preserves the original handler's signature — only the
// pre-call side-effect changes the stamped tier.
//
// The lthn/desktop equivalent of RFC §4.5.2's auth.AsCron(handler)
// QueueHandler wrapper. Lives in pkg/queue (not pkg/auth) because the
// wrapper consumes a queue-handler type (core.ActionHandler, but the
// queue is the only place that semantically registers
// "handler-becomes-cron-via-registration"). Naming preserved between
// substrate (auth.AsCron Context-stamper) and consumer (queue.AsCron
// handler-wrapper) so cron-aware audits grep both surfaces with the
// same literal.
//
// Per RFC §4.5.2: cron-tasks are by-construction trusted (no IPC
// origin); the registration site is the trust root. The wrapper makes
// the trust explicit at the place where humans audit cron
// registrations. Cascade rules that allow-list TierCron now mean
// "callers that came through the cron-scheduler entry point" rather
// than "callers that happen to be intra-Go".
//
// Usage example:
//
//	queue.RegisterKind(c, queue.HandlerOptions{
//	    Kind:    "backup.nightly",
//	    Handler: queue.AsCron(backupHandler),
//	})
func AsCron(fn core.ActionHandler) core.ActionHandler {
	if fn == nil {
		return nil
	}
	return func(ctx core.Context, opts core.Options) core.Result {
		cronCtx := auth.AsCron(ctx)
		return fn(cronCtx, opts)
	}
}

// Kinds returns every registered job kind (the Core actions
// matching the queue.kind.* prefix). Useful for telemetry + the
// queue/list CLI's "available handlers" hint.
//
// Usage example:
//
//	for _, kind := range queue.Kinds(c) {
//	    core.Println("registered kind:", kind)
//	}
func Kinds(c *core.Core) []string {
	if c == nil {
		return nil
	}
	var out []string
	for _, name := range c.Actions() {
		if len(name) > len(actionNamePrefix) && name[:len(actionNamePrefix)] == actionNamePrefix {
			out = append(out, name[len(actionNamePrefix):])
		}
	}
	return out
}
