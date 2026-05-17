// SPDX-Licence-Identifier: EUPL-1.2

// wrapper_drift_test.go — Phase 1 Unit B.5 wrapper-drift gate for
// pkg/vi (RFC.tier-auth-substrate.md v3.1 §4.4 / §10 B.5,
// Mantis #1721 umbrella + #1750 MED).
//
// Threat-class (Cerberus #72 F-3): shape (a.i) hand-written Require
// gates at the registration site mean a future contributor adding a
// method to *vi.Service can silently bind an ungated method to Wails.
// Additionally, OnStart / OnStop on *vi.Service were directly
// renderer-reachable in the pre-wrapper shape — the wrapper-shape
// split solves the lifecycle leak.
//
// Mitigation: gatedWailsMethods declares the methods on *Service
// known to carry an auth.Require call inside their body, AND
// exemptWailsMethods names the lifecycle methods (ServiceName /
// OnStart / OnStop) that are deliberately ungated because they live
// outside the renderer-reachable surface (the wrapper does NOT
// mirror them).
//
// When adding a read method to *Service: (1) add auth.Require with
// the correct TierX allow-list at the top of the new method body,
// (2) add the method name to gatedWailsMethods below, AND (3) add a
// matching wrapper method on *WailsService that calls
// stampRenderer + delegates. Forgetting step (3) means the new method
// silently never reaches the frontend once registration is on the
// wrapper.
//
// When adding a lifecycle / Go-side-only method to *Service: add the
// method name to exemptWailsMethods and DO NOT mirror it on
// *WailsService — that keeps it Go-callable but never renderer-
// reachable. This is the closure for Cerberus #72 F-3 OnStart /
// OnStop leak.

package vi_test

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/auth"
	"dappco.re/lthn/desktop/pkg/vi"
)

// gatedWailsMethods is the canonical declaration of the methods on
// *vi.Service that carry an auth.Require gate at the registration
// site (service.go). Maps method-name → true so add/remove is a
// one-line diff. Source of truth: grep auth.Require pkg/vi/service.go.
//
// Phase 1 (this ship) covers the 4 renderer-reachable read methods:
// Catalogue, Sites, Repos, Activity. Each permits TierOperator +
// TierRenderer + TierCascade — read-only surface, no secrets in
// output. Any future addition MUST land alongside its gate AND its
// entry here in the same commit.
var gatedWailsMethods = map[string]bool{
	"Catalogue": true, // permits TierOperator + TierRenderer + TierCascade
	"Sites":     true, // permits TierOperator + TierRenderer + TierCascade
	"Repos":     true, // permits TierOperator + TierRenderer + TierCascade
	"Activity":  true, // permits TierOperator + TierRenderer + TierCascade
}

// exemptWailsMethods names methods on *vi.Service that are
// LEGITIMATELY ungated because they are NOT renderer-reachable. The
// *WailsService wrapper deliberately does NOT mirror them, so the
// wails3 binding generator never emits TS bindings for them — the
// "wrapper-shape split" pattern from Cerberus #72 F-3 closure.
//
// ServiceName — labels the wails3 binding namespace ("Vi"); called
// by the binding generator at registration, not from JS.
// OnStart / OnStop — container-lifecycle hooks driven by the Core
// composition root (cmd/lthn/app.go). The wrapper's Substrate()
// accessor exposes them to in-Go callers (composition root + tests)
// without surfacing them to wails3.
var exemptWailsMethods = map[string]bool{
	"ServiceName": true, // wails3 binding-namespace label
	"OnStart":     true, // Go-side-only — lifecycle, NOT in wrapper (Cerberus #72 F-3)
	"OnStop":      true, // Go-side-only — lifecycle, NOT in wrapper (Cerberus #72 F-3)
}

// gatedWailsServiceMethods is the canonical declaration of the methods
// on *vi.WailsService — the Shape (a.i) IPC-entry wrapper (RFC v3.1
// §4.4 / Cerberus #72 F-3 / Mantis #1750). Every method here MUST
// (1) defer stampRenderer(w.svc.core)() at its top, and (2) delegate
// to the matching *Service method. The drift guard below catches a
// new *Service read method added without a matching *WailsService
// entry.
//
// Keep this set in lockstep with gatedWailsMethods above EXCEPT for
// the Cerberus #72 F-3 carve-out: OnStart / OnStop live on *Service
// only and MUST NOT appear here — the omission IS the renderer-deny.
var gatedWailsServiceMethods = map[string]bool{
	"Catalogue": true, // delegates to *Service.Catalogue
	"Sites":     true, // delegates to *Service.Sites
	"Repos":     true, // delegates to *Service.Repos
	"Activity":  true, // delegates to *Service.Activity
}

// exemptWailsServiceMethods carves out non-Wails-binding helpers on
// *WailsService. Substrate() returns the wrapped *Service and is for
// in-Go consumers (composition root, tests); wails3's binding
// generator silently skips methods whose arg/return shape isn't
// primitive/struct/Result, but the reflection-walk in the drift
// helper sees it. Listed here so the helper doesn't flag it as
// missing-gate. ServiceName mirrors *Service.ServiceName so the
// wails3 binding namespace stays "Vi" (drift-helper sees it on both
// types; exempt on both).
var exemptWailsServiceMethods = map[string]bool{
	"Substrate":   true, // returns *Service — accessor for in-Go consumers
	"ServiceName": true, // wails3 binding-namespace label
}

// TestVi_WailsService_AllMethodsWrapped_Good — RFC §10 B.5 canonical
// test name. Asserts the exported method set of *vi.Service matches
// gatedWailsMethods exactly (modulo exemptWailsMethods).
//
// Fails red on either drift direction. See the package doc-comment
// above for what to do when red.
func TestVi_WailsService_AllMethodsWrapped_Good(t *core.T) {
	svc := vi.NewService(core.New())
	auth.AssertWrapperDriftClean(t, svc, gatedWailsMethods, exemptWailsMethods)
}

// TestVi_WailsService_AllMethodsWrapped_Bad — meta-test: simulates the
// missing-gate drift class by dropping one method (Sites) from the
// gated map and asserts WrapperDriftScan reports it. Pins the guard's
// sensitivity so a regression in the helper itself surfaces here
// rather than silently making the Good test trivially pass for the
// wrong reason.
func TestVi_WailsService_AllMethodsWrapped_Bad(t *core.T) {
	svc := vi.NewService(core.New())
	dropped := map[string]bool{}
	for k, v := range gatedWailsMethods {
		if k == "Sites" {
			continue // simulate the contributor forgetting the gate
		}
		dropped[k] = v
	}
	miss, stale, err := auth.WrapperDriftScan(svc, dropped, exemptWailsMethods)
	core.AssertEqual(t, "", err)
	core.AssertLen(t, stale, 0)
	core.AssertLen(t, miss, 1)
	core.AssertEqual(t, "Sites", miss[0])
}

// TestVi_WailsService_AllMethodsWrapped_Ugly — meta-test: simulates
// the stale-gate drift class by adding a non-existent method
// ("Search") to the gated map. WrapperDriftScan must report it. Pins
// the guard's other direction — the brief's "every gate points to a
// real method" criterion (B5.4 a).
func TestVi_WailsService_AllMethodsWrapped_Ugly(t *core.T) {
	svc := vi.NewService(core.New())
	withGhost := map[string]bool{}
	for k, v := range gatedWailsMethods {
		withGhost[k] = v
	}
	withGhost["Search"] = true // not yet implemented
	miss, stale, err := auth.WrapperDriftScan(svc, withGhost, exemptWailsMethods)
	core.AssertEqual(t, "", err)
	core.AssertLen(t, miss, 0)
	core.AssertLen(t, stale, 1)
	core.AssertEqual(t, "Search", stale[0])
}

// --- *WailsService Shape (a.i) wrapper drift ---------------------------

// TestVi_WailsWrapper_AllMethodsWrapped_Good — RFC §10 B.5 canonical
// test name applied to the Shape (a.i) IPC-entry wrapper
// *vi.WailsService. Asserts the exported method set matches
// gatedWailsServiceMethods exactly (modulo exemptWailsServiceMethods).
//
// Catches the new-method drift in BOTH layers: a contributor who adds
// a method to *Service WITHOUT adding the wrapper entry on
// *WailsService fails this test red. The wails3 binding generator
// only walks *WailsService once registration switches over — so an
// un-wrapped *Service read method silently never reaches the frontend.
func TestVi_WailsWrapper_AllMethodsWrapped_Good(t *core.T) {
	w := vi.NewWailsService(vi.NewService(core.New()))
	auth.AssertWrapperDriftClean(t, w, gatedWailsServiceMethods, exemptWailsServiceMethods)
}

// TestVi_WailsWrapper_AllMethodsWrapped_Bad — meta-test: simulates
// the missing-wrapper drift class by dropping one method (Catalogue)
// from the gated map and asserts WrapperDriftScan reports it. Pins
// the guard's sensitivity on *WailsService just as the *Service Bad
// case pins it for the substrate.
func TestVi_WailsWrapper_AllMethodsWrapped_Bad(t *core.T) {
	w := vi.NewWailsService(vi.NewService(core.New()))
	dropped := map[string]bool{}
	for k, v := range gatedWailsServiceMethods {
		if k == "Catalogue" {
			continue // simulate the contributor forgetting the wrapper
		}
		dropped[k] = v
	}
	miss, stale, err := auth.WrapperDriftScan(w, dropped, exemptWailsServiceMethods)
	core.AssertEqual(t, "", err)
	core.AssertLen(t, stale, 0)
	core.AssertLen(t, miss, 1)
	core.AssertEqual(t, "Catalogue", miss[0])
}

// TestVi_WailsWrapper_AllMethodsWrapped_Ugly — meta-test: simulates
// the stale-gate drift class on *WailsService by adding a non-
// existent method ("Briefs") to the gated map. WrapperDriftScan must
// report it. "Briefs" is a plausible future addition (the Vi data
// contract has a Briefs slot deferred per RFC.vi.md) that a
// contributor might pre-emptively register before implementing.
func TestVi_WailsWrapper_AllMethodsWrapped_Ugly(t *core.T) {
	w := vi.NewWailsService(vi.NewService(core.New()))
	withGhost := map[string]bool{}
	for k, v := range gatedWailsServiceMethods {
		withGhost[k] = v
	}
	withGhost["Briefs"] = true // deferred per RFC.vi.md
	miss, stale, err := auth.WrapperDriftScan(w, withGhost, exemptWailsServiceMethods)
	core.AssertEqual(t, "", err)
	core.AssertLen(t, miss, 0)
	core.AssertLen(t, stale, 1)
	core.AssertEqual(t, "Briefs", stale[0])
}

// TestVi_WailsService_OnStart_NotInWrapper_Good — explicit Cerberus
// #72 F-3 closure: OnStart and OnStop MUST NOT exist on *WailsService
// (the wrapper-shape split keeps lifecycle Go-side-only). If a future
// contributor adds OnStart to the wrapper "for symmetry", this test
// fails red — the renderer would regain the path that closed #1750.
//
// The mirror Bad case lives on *Service: those methods MUST still
// exist on the substrate so the composition root can drive them.
func TestVi_WailsService_OnStart_NotInWrapper_Good(t *core.T) {
	w := vi.NewWailsService(vi.NewService(core.New()))
	typ := core.TypeOf(w)
	core.RequireTrue(t, typ != nil)
	for i := 0; i < typ.NumMethod(); i++ {
		name := typ.Method(i).Name
		if name == "OnStart" || name == "OnStop" {
			t.Fatalf("Cerberus #72 F-3 regression: *vi.WailsService.%s exists — "+
				"OnStart/OnStop MUST stay on *Service only so wails3 cannot bind "+
				"them to the renderer. See wails.go doc-comment.", name)
			return
		}
	}
}

// TestVi_WailsService_OnStart_StillOnSubstrate_Bad — paired with the
// _Good above: OnStart and OnStop MUST still exist on *vi.Service so
// the Core composition root (cmd/lthn/app.go) can drive them via
// c.ServiceStartup / ServiceShutdown. If a future contributor removes
// them from *Service to "clean up", the composition root breaks
// silently — this test fails red first.
func TestVi_WailsService_OnStart_StillOnSubstrate_Bad(t *core.T) {
	svc := vi.NewService(core.New())
	typ := core.TypeOf(svc)
	core.RequireTrue(t, typ != nil)
	seen := map[string]bool{}
	for i := 0; i < typ.NumMethod(); i++ {
		seen[typ.Method(i).Name] = true
	}
	core.RequireTrue(t, seen["OnStart"])
	core.RequireTrue(t, seen["OnStop"])
}
