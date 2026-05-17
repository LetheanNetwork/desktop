// SPDX-Licence-Identifier: EUPL-1.2

// wails_test.go — *vi.WailsService Shape (a.i) IPC-entry wrapper
// behavioural tests (RFC.tier-auth-substrate.md v3.1 §4.4 /
// Cerberus #72 F-3 / Mantis #1750).
//
// Companion to wrapper_drift_test.go which catches shape drift
// statically via reflection; this file pins the runtime semantics:
//
//   - the wrapper stamps TierRenderer onto an unstamped *core.Core
//     so the substrate's Require gate passes through cleanly;
//   - the stamp clears on return so the next intra-Go call resolves
//     to the TierInternal floor and is denied as expected;
//   - the TierUnknown deny path returns a tier_not_permitted Fail
//     rather than panicking — the wails3 binding sees a structured
//     reject the frontend can render.

package vi_test

import (
	core "dappco.re/go"
	"dappco.re/go/config"
	"dappco.re/go/orm"

	"dappco.re/lthn/desktop/pkg/auth"
	"dappco.re/lthn/desktop/pkg/vi"
)

// newUnstampedWailsService wires a *WailsService over an unstamped
// *core.Core (no auth.SetCaller). The IPC-entry stamp inside the
// wrapper is the only source of caller identity — lets the tests
// observe the wrapper's stamp without confusing it with ambient
// sidecar state.
//
// Returns the wrapper + the underlying *core.Core so tests can probe
// auth.Caller(c) post-call.
func newUnstampedWailsService(t *core.T) (*vi.WailsService, *core.Core) {
	t.Helper()
	c := core.New(
		core.WithName("config", config.NewConfigServiceWith(config.ServiceOptions{
			Path: t.TempDir() + "/lthn.json",
		})),
	)
	core.RequireTrue(t, orm.Register(c).OK)
	mem := orm.NewMemium()
	core.RequireTrue(t, orm.Mount(c, "default", mem).OK)
	for _, schema := range vi.Schemas() {
		core.RequireTrue(t, orm.RegisterSchema(c, schema).OK)
		mem.RegisterTable(schema.Name, schema)
	}
	core.RequireTrue(t, c.ServiceStartup(core.Background(), nil).OK)
	t.Cleanup(func() {
		auth.ClearCaller(c)
		_ = c.ServiceShutdown(core.Background())
	})
	return vi.NewWailsService(vi.NewService(c)), c
}

// TestVi_WailsService_Activity_TierLocalAccepted_Good — the wrapper's
// IPC-entry stamp lifts an unstamped *core.Core to TierRenderer for
// the duration of the call, so the inner *Service.Activity's Require
// gate passes (renderer is in the allow-list). Without the wrapper
// stamp the gate would deny — proves the wrapper is the source of
// the renderer-tier authority on Wails IPC entry. The output is the
// empty-activity envelope (no rows seeded), which IS the success
// shape — the gate fired Ok, not Fail.
func TestVi_WailsService_Activity_TierLocalAccepted_Good(t *core.T) {
	w, _ := newUnstampedWailsService(t)
	r := w.Activity()
	core.RequireTrue(t, r.OK)
	out, ok := r.Value.(vi.ActivityOutput)
	core.RequireTrue(t, ok)
	core.AssertLen(t, out.Items, 0)
}

// TestVi_WailsService_Activity_TierUnknownRejected_Bad — bypassing
// the wrapper (calling *Service.Activity directly) on an unstamped
// *core.Core resolves to the TierInternal floor, which is NOT in
// the allow-list — the gate returns a Fail with a tier_not_permitted
// error. Pins the substrate-side rejection so the wrapper's stamp is
// proven load-bearing: without it the renderer cannot reach the data.
//
// Also exercises the "auth deny is structured Fail not panic" shape
// the wails3 binding can ferry back to JS without crashing the
// renderer.
func TestVi_WailsService_Activity_TierUnknownRejected_Bad(t *core.T) {
	w, c := newUnstampedWailsService(t)
	// First call via wrapper succeeds (stamp on entry).
	core.RequireTrue(t, w.Activity().OK)
	// Defer-clear fired on return → sidecar is empty → next direct
	// call to the substrate resolves to TierInternal floor → denied.
	id := auth.Caller(c)
	core.AssertEqual(t, auth.TierInternal, id.Tier)
	bare := w.Substrate().Activity()
	core.AssertFalse(t, bare.OK)
	core.AssertContains(t, bare.Error(), "tier_not_permitted")
}

// TestVi_WailsService_AllReadMethods_StampsRenderer_Ugly — sequential
// calls across all four wrapper read methods each get their own stamp
// at entry and clear at return; the sidecar is empty at the end. Pins
// the stamp-and-clear cycle. Also exercises the full read surface
// (Catalogue / Sites / Repos / Activity) end-to-end so a future
// contributor who forgets stampRenderer on a new wrapper method
// fails red here when the substrate gate denies.
func TestVi_WailsService_AllReadMethods_StampsRenderer_Ugly(t *core.T) {
	w, c := newUnstampedWailsService(t)
	core.RequireTrue(t, w.Catalogue().OK)
	core.RequireTrue(t, w.Sites().OK)
	core.RequireTrue(t, w.Repos().OK)
	core.RequireTrue(t, w.Activity().OK)
	// Sidecar empty post-cycle — defer ClearCaller fires every method.
	id := auth.Caller(c)
	core.AssertEqual(t, auth.TierInternal, id.Tier)
}

// --- NewWailsService / Substrate accessor ------------------------------

// TestVi_WailsService_NewWailsService_Good — construction returns a
// non-nil wrapper holding the supplied substrate. Mirrors the tasks
// adopter's NewWailsService test for consistency across adopters.
func TestVi_WailsService_NewWailsService_Good(t *core.T) {
	c := core.New()
	svc := vi.NewService(c)
	w := vi.NewWailsService(svc)
	core.AssertNotNil(t, w)
}

// TestVi_WailsService_NewWailsService_Bad — nil substrate is permitted
// at construction time (matches the *Service contract). The failure
// surface is at-call: the wrapper's stampRenderer is nil-safe on c,
// so the first method call panics on the substrate's nil-deref rather
// than the wrapper's. Documenting that here keeps us honest:
// NewWailsService(nil) returns a non-nil pointer.
func TestVi_WailsService_NewWailsService_Bad(t *core.T) {
	w := vi.NewWailsService(nil)
	core.AssertNotNil(t, w)
}

// TestVi_WailsService_Substrate_Good — Substrate() returns the
// wrapped *Service pointer (NOT a fresh copy) so in-Go consumers
// reach the substrate directly without re-constructing. Pins the
// accessor for the wrapper-drift exempt-list contract AND for the
// composition-root pattern (cmd/lthn/app.go drives OnStart via
// Substrate()).
func TestVi_WailsService_Substrate_Good(t *core.T) {
	c := core.New()
	svc := vi.NewService(c)
	w := vi.NewWailsService(svc)
	core.AssertEqual(t, core.Sprintf("%p", svc), core.Sprintf("%p", w.Substrate()))
}

// TestVi_WailsService_ServiceName_Good — the wrapper's ServiceName
// mirrors the substrate so the wails3 binding namespace stays "Vi"
// (frontend imports keep resolving). If a future contributor lets
// wails3 default the namespace to "WailsService" by removing this
// method, every existing Vi.* TS import breaks silently.
func TestVi_WailsService_ServiceName_Good(t *core.T) {
	w := vi.NewWailsService(vi.NewService(core.New()))
	core.AssertEqual(t, "Vi", w.ServiceName())
}
