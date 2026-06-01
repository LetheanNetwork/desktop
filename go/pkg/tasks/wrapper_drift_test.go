// SPDX-Licence-Identifier: EUPL-1.2

// wrapper_drift_test.go — Phase 1 Unit B.5 wrapper-drift gate for
// pkg/tasks (RFC.tier-auth-substrate.md v2 §4.4 / §10 B.5,
// Mantis #1721 umbrella + #1734 MED).
//
// Threat-class (Cerberus #66 ADD-4): shape (a.i) hand-written
// Require gates at the registration site mean a future contributor
// adding a method to *tasks.Service can silently bind an ungated
// method to Wails. The frontend gets the new method via wails3
// generate bindings, the auth.Require call is absent, and the audit
// trail never notices the regression.
//
// Mitigation: gatedWailsMethods declares the methods on *Service
// known to carry an auth.Require call inside their body. The test
// AssertWrapperDriftClean reflects on *Service's exported method
// set and asserts the gated map matches the surface in BOTH
// directions:
//
//   - new method appears, gated entry missing → test fails red with
//     "wrapper-drift (missing-gate)" naming the un-gated method.
//   - gated entry remains after method is removed/renamed → test
//     fails red with "wrapper-drift (stale-gate)" naming the dead
//     entry.
//
// When adding a method to *Service: (1) add auth.Require with the
// correct TierX allow-list at the top of the new method body,
// (2) add the method name to gatedWailsMethods below.
//
// When removing/renaming a method on *Service: remove/rename the
// gatedWailsMethods entry in the same commit.

package tasks_test

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/auth"
	"dappco.re/lthn/desktop/pkg/tasks"
)

// gatedWailsMethods is the canonical declaration of the methods on
// *tasks.Service that carry an auth.Require gate at the registration
// site (wails.go). Maps method-name → true so add/remove is a
// one-line diff. Source of truth: grep auth.Require pkg/tasks/wails.go.
//
// Phase 1 (this ship) covers the 5 Wails-bound methods landed by
// H#240 (Mantis #1722 Unit B.2): List, Get, Create, Update, AddNote.
// Any future addition MUST land alongside its gate AND its entry
// here in the same commit.
var gatedWailsMethods = map[string]bool{
	"List":    true, // permits TierOperator + TierRenderer + TierCascade
	"Get":     true, // permits TierOperator + TierRenderer + TierCascade
	"Create":  true, // permits TierOperator + TierRenderer (ENFORCE-not-claim Reporter)
	"Update":  true, // permits TierOperator + TierRenderer
	"AddNote": true, // permits TierOperator + TierRenderer (ENFORCE-not-claim Author)
	"Detect":  true, // permits TierOperator + TierRenderer (files core-lint findings as tasks)
	"DetectUpdates": true, // permits TierOperator + TierRenderer (files outdated-dependency tasks)
	"ClearDetected": true, // permits TierOperator + TierRenderer (removes detector-filed tasks)
}

// exemptWailsMethods is intentionally empty for pkg/tasks: the
// *Service struct does NOT embed a lifecycle type (*core.ServiceRuntime
// etc.), so reflect sees only the methods defined directly in
// wails.go. If a future ServiceRuntime embed lands (e.g. for
// background indexing), populate this map with the inherited
// lifecycle method names (ServiceName / OnStart / OnStop / ...)
// so they're not flagged as missing gates.
var exemptWailsMethods = map[string]bool{}

// gatedWailsServiceMethods is the canonical declaration of the methods
// on *tasks.WailsService — the Shape (a.i) IPC-entry wrapper (RFC v3.1
// §4.4 / Cerberus #73 F-1 / Mantis #1755). Every method here MUST
// (1) defer stampRenderer(w.svc.core)() at its top, and (2) delegate
// to the matching *Service method. The drift guard below catches a
// new *Service method added without a matching *WailsService entry.
//
// Keep this set in lockstep with gatedWailsMethods above: a method
// added to *Service that is NOT wrapped here means a Wails IPC call
// never reaches the substrate (the wails3 generator only walks
// *WailsService once registration switches over).
var gatedWailsServiceMethods = map[string]bool{
	"List":    true, // delegates to *Service.List
	"Get":     true, // delegates to *Service.Get
	"Create":  true, // delegates to *Service.Create (ENFORCE-not-claim Reporter via inner)
	"Update":  true, // delegates to *Service.Update
	"AddNote": true, // delegates to *Service.AddNote (ENFORCE-not-claim Author via inner)
	"Detect":  true, // delegates to *Service.Detect (files core-lint findings as tasks)
	"DetectUpdates": true, // delegates to *Service.DetectUpdates (outdated-dependency tasks)
	"ClearDetected": true, // delegates to *Service.ClearDetected (removes detector-filed tasks)
}

// exemptWailsServiceMethods carves out non-Wails-binding helpers on
// *WailsService. Substrate() returns the wrapped *Service and is for
// in-Go consumers (tests, future HTTP/CLI surfaces); wails3's binding
// generator silently skips methods whose arg/return shape isn't
// primitive/struct/Result, but the reflection-walk in the drift helper
// sees it. Listed here so the helper doesn't flag it as missing-gate.
var exemptWailsServiceMethods = map[string]bool{
	"Substrate": true, // returns *Service — accessor for in-Go consumers
}

// TestWailsService_AllMethodsWrapped_Good — RFC §10 B.5 canonical
// test name. Asserts the exported method set of *tasks.Service
// matches gatedWailsMethods exactly (modulo exemptWailsMethods).
//
// Fails red on either drift direction. See the package doc-comment
// above for what to do when red.
func TestWailsService_AllMethodsWrapped_Good(t *core.T) {
	svc := tasks.NewService(core.New())
	auth.AssertWrapperDriftClean(t, svc, gatedWailsMethods, exemptWailsMethods)
}

// TestWailsService_AllMethodsWrapped_Bad — meta-test: simulates the
// missing-gate drift class by dropping one method (Create) from
// the gated map and asserts WrapperDriftScan reports it. Pins the
// guard's sensitivity so a regression in the helper itself surfaces
// here (rather than silently making the Good test trivially pass
// for the wrong reason).
func TestWailsService_AllMethodsWrapped_Bad(t *core.T) {
	svc := tasks.NewService(core.New())
	dropped := map[string]bool{}
	for k, v := range gatedWailsMethods {
		if k == "Create" {
			continue // simulate the contributor forgetting the wrapper
		}
		dropped[k] = v
	}
	miss, stale, err := auth.WrapperDriftScan(svc, dropped, exemptWailsMethods)
	core.AssertEqual(t, "", err)
	core.AssertLen(t, stale, 0)
	core.AssertLen(t, miss, 1)
	core.AssertEqual(t, "Create", miss[0])
}

// TestWailsService_AllMethodsWrapped_Ugly — meta-test: simulates
// the stale-gate drift class by adding a non-existent method
// ("Search") to the gated map. WrapperDriftScan must report it.
// Pins the guard's other direction — the brief's "every wrapper
// points to a real method" criterion (B5.4 a).
//
// "Search" is a deliberate choice: the wails.go doc-comment notes
// Search is DEFERRED to a future pass, so a contributor pre-emptively
// adding it to the gated map (before implementing the method) is a
// plausible drift this case guards against.
func TestWailsService_AllMethodsWrapped_Ugly(t *core.T) {
	svc := tasks.NewService(core.New())
	withGhost := map[string]bool{}
	for k, v := range gatedWailsMethods {
		withGhost[k] = v
	}
	withGhost["Search"] = true // deferred per wails.go doc-comment
	miss, stale, err := auth.WrapperDriftScan(svc, withGhost, exemptWailsMethods)
	core.AssertEqual(t, "", err)
	core.AssertLen(t, miss, 0)
	core.AssertLen(t, stale, 1)
	core.AssertEqual(t, "Search", stale[0])
}

// --- *WailsService Shape (a.i) wrapper drift ---------------------------

// TestWailsWrapper_AllMethodsWrapped_Good — RFC §10 B.5 canonical test
// name applied to the Shape (a.i) IPC-entry wrapper *WailsService.
// Asserts the exported method set of *tasks.WailsService matches
// gatedWailsServiceMethods exactly (modulo exemptWailsServiceMethods).
//
// Catches the new-method drift in BOTH layers: a contributor who adds
// a method to *Service WITHOUT adding the wrapper entry on *WailsService
// fails this test red. The wails3 binding generator only walks
// *WailsService once registration switches over — so an un-wrapped
// *Service method silently never reaches the frontend.
func TestWailsWrapper_AllMethodsWrapped_Good(t *core.T) {
	w := tasks.NewWailsService(tasks.NewService(core.New()))
	auth.AssertWrapperDriftClean(t, w, gatedWailsServiceMethods, exemptWailsServiceMethods)
}

// TestWailsWrapper_AllMethodsWrapped_Bad — meta-test: simulates the
// missing-wrapper drift class by dropping one method (Create) from the
// gated map and asserts WrapperDriftScan reports it. Pins the guard's
// sensitivity on *WailsService just as the *Service Bad case pins it
// for the substrate.
func TestWailsWrapper_AllMethodsWrapped_Bad(t *core.T) {
	w := tasks.NewWailsService(tasks.NewService(core.New()))
	dropped := map[string]bool{}
	for k, v := range gatedWailsServiceMethods {
		if k == "Create" {
			continue // simulate the contributor forgetting the wrapper
		}
		dropped[k] = v
	}
	miss, stale, err := auth.WrapperDriftScan(w, dropped, exemptWailsServiceMethods)
	core.AssertEqual(t, "", err)
	core.AssertLen(t, stale, 0)
	core.AssertLen(t, miss, 1)
	core.AssertEqual(t, "Create", miss[0])
}

// TestWailsWrapper_AllMethodsWrapped_Ugly — meta-test: simulates the
// stale-gate drift class on *WailsService by adding a non-existent
// method ("Delete") to the gated map. WrapperDriftScan must report it.
// "Delete" is a plausible future addition (issue tombstoning) that a
// contributor might pre-emptively register in the gated map before
// implementing.
func TestWailsWrapper_AllMethodsWrapped_Ugly(t *core.T) {
	w := tasks.NewWailsService(tasks.NewService(core.New()))
	withGhost := map[string]bool{}
	for k, v := range gatedWailsServiceMethods {
		withGhost[k] = v
	}
	withGhost["Delete"] = true // not yet implemented
	miss, stale, err := auth.WrapperDriftScan(w, withGhost, exemptWailsServiceMethods)
	core.AssertEqual(t, "", err)
	core.AssertLen(t, miss, 0)
	core.AssertLen(t, stale, 1)
	core.AssertEqual(t, "Delete", stale[0])
}
