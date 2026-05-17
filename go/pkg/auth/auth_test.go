// SPDX-Licence-Identifier: EUPL-1.2

// auth_test.go — Unit B1 substrate done-criteria per RFC §10.
//
// Test matrix shape (Good / Bad / Ugly):
//
//   - Good: permitted tier, method succeeds, identity round-trips.
//   - Bad:  unpermitted tier, gate rejects, identity preserved.
//   - Ugly: empty/malformed context, gate rejects safely.
//
// Consumer migration (pkg/tasks B2, pkg/queue B3, pkg/server bridge
// B4, pkg/desktop wrapper-drift guard B5) is NOT covered here — those
// land per-unit. This file pins the substrate primitives only.

package auth_test

import (
	"testing"

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/auth"
	"github.com/gin-gonic/gin"
)

// TestAuth_Caller_Good — stamp via WithCaller, read via Caller; the
// round-trip preserves Tier/Subject/Source. Covers the Context-shaped
// happy path (the canonical setter).
func TestAuth_Caller_Good(t *testing.T) {
	c := core.New()
	ctx := auth.WithCaller(c.Context(), auth.CallerIdentity{
		Tier: auth.TierOperator, Subject: "account-snider", Source: "cli",
	})
	id := auth.CallerFromContext(ctx)
	if id.Tier != auth.TierOperator {
		t.Fatalf("tier round-trip: want %q got %q", auth.TierOperator, id.Tier)
	}
	if id.Subject != "account-snider" {
		t.Fatalf("subject round-trip: want %q got %q", "account-snider", id.Subject)
	}
	if id.Source != "cli" {
		t.Fatalf("source round-trip: want %q got %q", "cli", id.Source)
	}
}

// TestAuth_Caller_Good_DefaultInternal — unstamped Caller returns
// TierInternal floor per RFC §4.1 audit row last entry. The floor is
// deliberately the most-restrictive tier; Require gates with explicit
// allow-lists still reject it (see TestAuth_Require_Ugly).
func TestAuth_Caller_Good_DefaultInternal(t *testing.T) {
	c := core.New()
	id := auth.Caller(c)
	if id.Tier != auth.TierInternal {
		t.Fatalf("default tier: want %q got %q", auth.TierInternal, id.Tier)
	}
	if id.Subject != "" || id.Source != "" {
		t.Fatalf("default subject/source: want empty got %q/%q", id.Subject, id.Source)
	}
}

// TestAuth_Caller_Good_NilCoreFloor — Caller(nil) returns the floor
// without panic. Defends test-seed / partial-construct call sites
// from a hidden nil-deref.
func TestAuth_Caller_Good_NilCoreFloor(t *testing.T) {
	id := auth.Caller(nil)
	if id.Tier != auth.TierInternal {
		t.Fatalf("nil-core tier: want %q got %q", auth.TierInternal, id.Tier)
	}
	idCtx := auth.CallerFromContext(nil)
	if idCtx.Tier != auth.TierInternal {
		t.Fatalf("nil-ctx tier: want %q got %q", auth.TierInternal, idCtx.Tier)
	}
}

// TestAuth_Require_Good — caller in the allow-list passes; the
// returned identity matches the stamped value.
func TestAuth_Require_Good(t *testing.T) {
	c := core.New()
	auth.SetCaller(c, auth.CallerIdentity{
		Tier: auth.TierOperator, Subject: "account-snider", Source: "http-session",
	})
	t.Cleanup(func() { auth.ClearCaller(c) })

	id, ok := auth.Require(c, "tasks.Service.Create", auth.TierOperator, auth.TierRenderer)
	if !ok {
		t.Fatalf("Require permitted: want ok got !ok (id=%s)", id)
	}
	if id.Subject != "account-snider" {
		t.Fatalf("Require subject: want %q got %q", "account-snider", id.Subject)
	}
}

// TestAuth_Require_Bad — TierRenderer caller against TierOperator-
// only allow-list is rejected. The identity is still returned (for
// audit-emit at the call site) but ok=false. The forge-rejection
// shape: renderer-tier caller cannot pretend to be operator.
func TestAuth_Require_Bad(t *testing.T) {
	c := core.New()
	auth.SetCaller(c, auth.CallerIdentity{
		Tier: auth.TierRenderer, Subject: "account-renderer", Source: "wails",
	})
	t.Cleanup(func() { auth.ClearCaller(c) })

	id, ok := auth.Require(c, "tasks.Service.Delete", auth.TierOperator)
	if ok {
		t.Fatalf("Require unpermitted: want !ok got ok (id=%s)", id)
	}
	if id.Tier != auth.TierRenderer {
		t.Fatalf("rejected-id tier preserved: want %q got %q", auth.TierRenderer, id.Tier)
	}
	if id.Subject != "account-renderer" {
		t.Fatalf("rejected-id subject preserved: want %q got %q", "account-renderer", id.Subject)
	}
}

// TestAuth_Require_Ugly — unstamped Core (zero CallerIdentity → floor
// TierInternal) against a typical Wails allow-list (Operator/Renderer)
// is rejected. The deny-by-default contract: TierInternal is the floor
// but does NOT satisfy an allow-list that doesn't name it.
func TestAuth_Require_Ugly(t *testing.T) {
	c := core.New()
	// No SetCaller / WithCaller. Caller(c).Tier == TierInternal.

	id, ok := auth.Require(c, "tasks.Service.Create",
		auth.TierOperator, auth.TierRenderer)
	if ok {
		t.Fatalf("Require empty-ctx: want !ok (deny-by-default) got ok (id=%s)", id)
	}
	if id.Tier != auth.TierInternal {
		t.Fatalf("empty-ctx tier: want %q got %q", auth.TierInternal, id.Tier)
	}
}

// TestAuth_Require_Ugly_EmptyAllowList — Require with zero allowed
// tiers is the "forever locked" shape; rejects everything including
// TierInternal.
func TestAuth_Require_Ugly_EmptyAllowList(t *testing.T) {
	c := core.New()
	auth.SetCaller(c, auth.CallerIdentity{Tier: auth.TierOperator})
	t.Cleanup(func() { auth.ClearCaller(c) })

	if _, ok := auth.Require(c, "locked.method"); ok {
		t.Fatalf("Require empty-allow-list: want !ok (lock-shape) got ok")
	}
}

// TestAuth_Require_Good_TierInternalExplicit — allow-listing
// TierInternal explicitly accepts the floor; defends intra-Go test
// seed call sites that need to pass a gate without stamping a
// privileged tier.
func TestAuth_Require_Good_TierInternalExplicit(t *testing.T) {
	c := core.New()
	id, ok := auth.Require(c, "internal.method", auth.TierInternal)
	if !ok {
		t.Fatalf("Require TierInternal: want ok got !ok (id=%s)", id)
	}
}

// TestAuth_SetCaller_Good_Roundtrip — Core-shaped SetCaller is
// observable via Caller(c). Pointer-identity scoping isolates per-
// Core stamps.
func TestAuth_SetCaller_Good_Roundtrip(t *testing.T) {
	c1 := core.New()
	c2 := core.New()
	t.Cleanup(func() {
		auth.ClearCaller(c1)
		auth.ClearCaller(c2)
	})

	auth.SetCaller(c1, auth.CallerIdentity{Tier: auth.TierOperator, Subject: "a"})
	auth.SetCaller(c2, auth.CallerIdentity{Tier: auth.TierRenderer, Subject: "b"})

	if id := auth.Caller(c1); id.Tier != auth.TierOperator || id.Subject != "a" {
		t.Fatalf("c1 stamp: want operator/a got %s", id)
	}
	if id := auth.Caller(c2); id.Tier != auth.TierRenderer || id.Subject != "b" {
		t.Fatalf("c2 stamp: want renderer/b got %s", id)
	}
}

// TestAuth_SetCaller_Good_ContextStampShadows — when both a Context
// stamp AND a sidecar stamp exist on the same *core.Core, the
// Context-stamp wins (it's the more-specific scope; the sidecar is
// the fallback). NOT tested here because Caller(c) reads c.Context()
// FIRST per the documented order, and core.Core's context is fixed
// at New() — so a Context stamp via WithCaller on c.Context() does
// NOT mutate the Core's bound context. The defensible invariant in
// the current shape: sidecar-set + Context-unset reads the sidecar
// (covered by TestAuth_SetCaller_Good_Roundtrip).

// TestAuth_ClearCaller_Good_RemovesStamp — ClearCaller un-stamps;
// subsequent Caller returns the floor. Idempotent.
func TestAuth_ClearCaller_Good_RemovesStamp(t *testing.T) {
	c := core.New()
	auth.SetCaller(c, auth.CallerIdentity{Tier: auth.TierOperator, Subject: "x"})
	auth.ClearCaller(c)
	if id := auth.Caller(c); id.Tier != auth.TierInternal {
		t.Fatalf("post-clear tier: want %q got %q", auth.TierInternal, id.Tier)
	}
	// Idempotent second clear.
	auth.ClearCaller(c)
	auth.ClearCaller(nil) // nil-safe per contract
}

// TestAuth_AsRenderer_Good — convenience stamper produces the same
// shape as direct WithCaller; smoke-test for every AsX helper.
func TestAuth_AsRenderer_Good(t *testing.T) {
	c := core.New()
	cases := []struct {
		name    string
		ctx     core.Context
		wantTier auth.CallerTier
	}{
		{"AsOperator", auth.AsOperator(c.Context(), "s", "src"), auth.TierOperator},
		{"AsRenderer", auth.AsRenderer(c.Context(), "s", "src"), auth.TierRenderer},
		{"AsPlugin", auth.AsPlugin(c.Context(), "s", "src"), auth.TierPlugin},
		{"AsCascade", auth.AsCascade(c.Context(), "s", "src"), auth.TierCascade},
		{"AsCron", auth.AsCron(c.Context()), auth.TierCron},
		{"AsCLI", auth.AsCLI(c.Context(), "s"), auth.TierCLI},
		{"AsInternal", auth.AsInternal(c.Context()), auth.TierInternal},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := auth.CallerFromContext(tc.ctx).Tier; got != tc.wantTier {
				t.Fatalf("%s tier: want %q got %q", tc.name, tc.wantTier, got)
			}
		})
	}
}

// TestAuth_AsCron_Good — TierCron stamp lands with empty Subject and
// fixed "cron-worker" Source per the §4.5.2 contract. The QueueHandler
// wrapper variant (`AsCron(QueueHandler) QueueHandler`) lands in B3
// when the queue handler type stabilises.
func TestAuth_AsCron_Good(t *testing.T) {
	c := core.New()
	ctx := auth.AsCron(c.Context())
	id := auth.CallerFromContext(ctx)
	if id.Tier != auth.TierCron {
		t.Fatalf("AsCron tier: want %q got %q", auth.TierCron, id.Tier)
	}
	if id.Subject != "" {
		t.Fatalf("AsCron subject: want empty got %q", id.Subject)
	}
	if id.Source != "cron-worker" {
		t.Fatalf("AsCron source: want %q got %q", "cron-worker", id.Source)
	}
}

// TestAuth_AsCron_Bad_DirectCallerNotCron — invoking a handler
// directly (without the AsCron stamper) results in NON-cron identity.
// Defends against silent inheritance: a service that calls a cron-
// handler-function from inside a non-cron path does NOT silently
// elevate.
func TestAuth_AsCron_Bad_DirectCallerNotCron(t *testing.T) {
	c := core.New()
	// Operator-stamped context; handler invoked directly (no AsCron
	// wrapper). Resulting Caller is TierOperator, not TierCron.
	ctx := auth.AsOperator(c.Context(), "snider", "cli")
	id := auth.CallerFromContext(ctx)
	if id.Tier == auth.TierCron {
		t.Fatalf("direct-call tier leaked cron: want non-cron got %q", id.Tier)
	}
}

// TestAuth_Elevate_Good — TierInternal floor can elevate to TierCron
// (intra-Go is the elevation trust root). Verifies the canElevate
// matrix.
func TestAuth_Elevate_Good(t *testing.T) {
	c := core.New()
	ctx := c.Context() // unstamped → TierInternal floor

	elevated, ok := auth.Elevate(ctx, auth.TierCron, "", "cron-scheduler")
	if !ok {
		t.Fatalf("Elevate internal→cron: want ok got !ok")
	}
	if id := auth.CallerFromContext(elevated); id.Tier != auth.TierCron {
		t.Fatalf("elevated tier: want %q got %q", auth.TierCron, id.Tier)
	}
}

// TestAuth_Elevate_Good_OperatorToCascade — operator can elevate to
// cascade (cron-style operator-triggered fanout).
func TestAuth_Elevate_Good_OperatorToCascade(t *testing.T) {
	c := core.New()
	ctx := auth.AsOperator(c.Context(), "snider", "cli")
	elevated, ok := auth.Elevate(ctx, auth.TierCascade, "snider", "cascade")
	if !ok {
		t.Fatalf("Elevate operator→cascade: want ok got !ok")
	}
	if id := auth.CallerFromContext(elevated); id.Tier != auth.TierCascade {
		t.Fatalf("elevated tier: want %q got %q", auth.TierCascade, id.Tier)
	}
}

// TestAuth_Elevate_Bad_RendererCannotElevate — TierRenderer cannot
// elevate to ANYTHING (the F-pattern D failure mode the substrate
// refuses). Defensive: even renderer→cron is rejected because the
// elevation policy is tight by design.
func TestAuth_Elevate_Bad_RendererCannotElevate(t *testing.T) {
	c := core.New()
	ctx := auth.AsRenderer(c.Context(), "renderer-acct", "wails")

	targets := []auth.CallerTier{
		auth.TierOperator, auth.TierCron, auth.TierCascade, auth.TierInternal,
	}
	for _, to := range targets {
		t.Run(string(to), func(t *testing.T) {
			elevated, ok := auth.Elevate(ctx, to, "renderer-acct", "renderer-elevate-attempt")
			if ok {
				t.Fatalf("Elevate renderer→%s: want !ok got ok", to)
			}
			// On deny, returned context equals input (caller MUST NOT
			// observe a privileged ctx).
			if id := auth.CallerFromContext(elevated); id.Tier != auth.TierRenderer {
				t.Fatalf("denied-elevate tier: want %q got %q", auth.TierRenderer, id.Tier)
			}
		})
	}
}

// TestAuth_Elevate_Bad_OperatorCannotElevateToOperator — operator
// cannot self-elevate (no-op elevation is rejected by canElevate
// because TierOperator→TierOperator is not in the matrix).
func TestAuth_Elevate_Bad_OperatorCannotElevateToOperator(t *testing.T) {
	c := core.New()
	ctx := auth.AsOperator(c.Context(), "snider", "cli")
	_, ok := auth.Elevate(ctx, auth.TierOperator, "snider", "self-elevate")
	if ok {
		t.Fatalf("Elevate operator→operator: want !ok (tight matrix) got ok")
	}
}

// TestAuth_FromGin_Good_BridgesIdentity — gin context with bridge
// keys produces the expected CallerIdentity on the derived Core
// context. Pins the §4.6.1 Option A bridge contract.
func TestAuth_FromGin_Good_BridgesIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gctx, _ := gin.CreateTestContext(nil)
	gctx.Set(auth.GinKeyTier, string(auth.TierOperator))
	gctx.Set(auth.GinKeySubject, "account-snider")
	gctx.Set(auth.GinKeySource, "http-session:LTHN-SESS-1")

	c := core.New()
	ctx := auth.FromGin(gctx, c.Context())
	id := auth.CallerFromContext(ctx)
	if id.Tier != auth.TierOperator {
		t.Fatalf("FromGin tier: want %q got %q", auth.TierOperator, id.Tier)
	}
	if id.Subject != "account-snider" {
		t.Fatalf("FromGin subject: want %q got %q", "account-snider", id.Subject)
	}
	if id.Source != "http-session:LTHN-SESS-1" {
		t.Fatalf("FromGin source: want %q got %q", "http-session:LTHN-SESS-1", id.Source)
	}
}

// TestAuth_FromGin_Ugly_MissingKeysReturnsFloor — gin context with
// NO bridge keys does NOT stamp; downstream Caller sees the floor.
// The deny-by-default shape for an unauthenticated path that still
// reaches a substrate-gated handler.
func TestAuth_FromGin_Ugly_MissingKeysReturnsFloor(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gctx, _ := gin.CreateTestContext(nil)
	// No Set calls — bridge keys absent.

	c := core.New()
	ctx := auth.FromGin(gctx, c.Context())
	if id := auth.CallerFromContext(ctx); id.Tier != auth.TierInternal {
		t.Fatalf("FromGin missing-keys: want floor %q got %q", auth.TierInternal, id.Tier)
	}
}

// TestAuth_FromGin_Ugly_NilGinReturnsInput — defensive nil-gin path
// returns the input ctx unchanged.
func TestAuth_FromGin_Ugly_NilGinReturnsInput(t *testing.T) {
	c := core.New()
	in := auth.AsOperator(c.Context(), "snider", "test")
	out := auth.FromGin(nil, in)
	if auth.CallerFromContext(out).Subject != "snider" {
		t.Fatalf("FromGin(nil) mutated input ctx")
	}
}

// TestAuth_CallerIdentity_String — formatter shape pin (audit-emit
// consumers grep this format).
func TestAuth_CallerIdentity_String(t *testing.T) {
	cases := []struct {
		id   auth.CallerIdentity
		want string
	}{
		{auth.CallerIdentity{Tier: auth.TierOperator, Subject: "snider", Source: "cli"},
			"tier=operator subject=snider source=cli"},
		{auth.CallerIdentity{Tier: auth.TierCron},
			"tier=cron subject=<empty> source=<empty>"},
		{auth.CallerIdentity{Tier: auth.TierRenderer, Subject: "acct-1"},
			"tier=renderer subject=acct-1 source=<empty>"},
	}
	for _, tc := range cases {
		if got := tc.id.String(); got != tc.want {
			t.Errorf("String(%+v): want %q got %q", tc.id, tc.want, got)
		}
	}
}

// TestAuth_CallerTier_String — enum String round-trip.
func TestAuth_CallerTier_String(t *testing.T) {
	want := []string{"operator", "renderer", "plugin", "cascade", "cron", "cli", "internal"}
	got := []string{
		auth.TierOperator.String(), auth.TierRenderer.String(),
		auth.TierPlugin.String(), auth.TierCascade.String(),
		auth.TierCron.String(), auth.TierCLI.String(),
		auth.TierInternal.String(),
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("tier[%d]: want %q got %q", i, w, got[i])
		}
	}
}

// TestAuth_RequireFromContext_Good — Context-shaped Require companion
// resolves identical to *core.Core-shaped Require given equivalent
// stamps. Pins the dual-API parity.
func TestAuth_RequireFromContext_Good(t *testing.T) {
	c := core.New()
	ctx := auth.AsRenderer(c.Context(), "acct-r", "wails")
	id, ok := auth.RequireFromContext(ctx, "queue.Worker.Dispatch",
		auth.TierRenderer, auth.TierOperator)
	if !ok {
		t.Fatalf("RequireFromContext renderer permitted: want ok got !ok (id=%s)", id)
	}
	if id.Subject != "acct-r" {
		t.Fatalf("RequireFromContext subject: want %q got %q", "acct-r", id.Subject)
	}
}

// TestAuth_RequireFromContext_Bad — Context-shaped reject path.
func TestAuth_RequireFromContext_Bad(t *testing.T) {
	c := core.New()
	ctx := auth.AsRenderer(c.Context(), "acct-r", "wails")
	if _, ok := auth.RequireFromContext(ctx, "op", auth.TierOperator); ok {
		t.Fatalf("RequireFromContext renderer→operator-only: want !ok got ok")
	}
}
