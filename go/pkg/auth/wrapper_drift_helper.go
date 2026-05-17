// SPDX-Licence-Identifier: EUPL-1.2

// wrapper_drift_helper.go — shared test-helper for the Phase-1
// wrapper-drift guard (RFC.tier-auth-substrate.md v2 §4.4 / §10 B.5,
// Mantis #1721 + #1734 MED).
//
// Threat-class: shape (a.i) Wails-binding uses hand-written per-method
// Require gates at the registration site. A future contributor adding a
// new exported method to a registered *Service without a Require call
// silently binds an ungated method to the frontend — and the audit
// trail never notices.
//
// Mitigation: each adopter package owns a wrapper_drift_test.go that
// declares the *Service's currently-gated method set as a map, then
// calls AssertWrapperDriftClean. Reflect over the service value
// enumerates the actual exported method set; the helper compares both
// directions:
//
//   - "missing-gate" → a method exists on *Service but is not in the
//     declared gated set. Fails the test loudly with the method name
//     so the contributor knows exactly which method needs a wrapper
//     entry (and an auth.Require call inside it).
//   - "stale-gate"   → the declared gated set names a method that no
//     longer exists on *Service. Fails the test loudly so the map
//     doesn't drift away from the surface.
//
// The helper deliberately lives in pkg/auth (not _test.go) so adopter
// packages in OTHER directories can import it — Go's test-package
// visibility rules forbid cross-package use of *_test.go symbols, and
// the alternative (per-package copy-paste) is exactly the drift
// surface we are guarding against.
//
// Per RFC §10 B.5 the canonical test name across adopters is
// TestWailsService_AllMethodsWrapped_Good. Helper accepts an exempt
// set so adopters whose *Service embeds lifecycle types (e.g.
// *core.ServiceRuntime) can carve out inherited methods cleanly
// without weakening the gate.
//
// Usage example (adopter in pkg/tasks/wrapper_drift_test.go):
//
//	func TestWailsService_AllMethodsWrapped_Good(t *core.T) {
//	    auth.AssertWrapperDriftClean(t, tasks.NewService(core.New()),
//	        map[string]bool{
//	            "List":    true,
//	            "Get":     true,
//	            "Create":  true,
//	            "Update":  true,
//	            "AddNote": true,
//	        },
//	        nil, // no embedded lifecycle exemptions
//	    )
//	}

package auth

import (
	core "dappco.re/go"
)

// AssertWrapperDriftClean asserts the exported method set of svc
// matches the declared gated set, modulo the exempt set. Used by
// per-package wrapper_drift_test.go files to close the shape (a.i)
// silent-drift surface from RFC.tier-auth-substrate.md §4.4 / ADD-4.
//
// Parameters:
//   - t       — the test handle (any core.TB-compatible value).
//   - svc     — the *Service value to enumerate (must be a non-nil
//     pointer; the helper reflects on its method set).
//   - gated   — the declared set of method names known to carry a
//     Require gate at the registration site. Empty map = "this service
//     should expose zero Wails methods" (legitimate for service-bus
//     consumers that aren't Wails-bound at all).
//   - exempt  — optional set of method names that are LEGITIMATELY
//     ungated (e.g. ServiceName/OnStart/OnStop inherited from a
//     lifecycle embed). Pass nil if the *Service has no embedded
//     lifecycle surface.
//
// Fails the test loudly with the offending method name on either
// drift direction:
//
//   - missing-gate: method exists on *Service but is not in gated
//     (and not in exempt). A contributor added a method without a
//     Require call. The contributor must add an auth.Require gate
//     INSIDE the method, then add the method name to the test's
//     gated map.
//   - stale-gate:   method named in gated does not exist on *Service.
//     A method was renamed or removed; the test's gated map must
//     follow. Otherwise the map drifts away from the surface and the
//     gate becomes decorative.
//
// Usage example (adopter):
//
//	auth.AssertWrapperDriftClean(t, tasks.NewService(core.New()),
//	    map[string]bool{"List": true, "Get": true},
//	    nil)
func AssertWrapperDriftClean(t core.TB, svc any, gated map[string]bool, exempt map[string]bool) {
	t.Helper()
	missing, stale, err := WrapperDriftScan(svc, gated, exempt)
	if err != "" {
		t.Fatalf("AssertWrapperDriftClean: %s", err)
		return
	}
	if len(missing) > 0 {
		t.Fatalf("wrapper-drift (missing-gate): %d method(s) have no Require gate and no exempt entry — %v. Add auth.Require inside the method, then add the name to the test's gated map. RFC §4.4 / ADD-4.",
			len(missing), missing)
		return
	}
	if len(stale) > 0 {
		t.Fatalf("wrapper-drift (stale-gate): %d gated entry/entries name method(s) that no longer exist on the service — %v. Remove the stale entry/entries from the test's gated map. RFC §4.4 / ADD-4.",
			len(stale), stale)
		return
	}
}

// WrapperDriftScan is the pure scanning function behind
// AssertWrapperDriftClean. It returns (missing, stale, err) without
// touching a TB so the scan logic is directly unit-testable from
// pkg/auth tests (the public Assert wrapper calls this then fans out
// to t.Fatalf). Adopter packages should prefer AssertWrapperDriftClean.
//
// Returns:
//   - missing []string — exported method names on svc that are NEITHER
//     in the gated set NOR in the exempt set. Each name is a
//     wrapper-drift bug at the registration site.
//   - stale   []string — gated entries that name methods which do not
//     exist on svc. Each name is a stale map entry — the underlying
//     method was renamed or removed without the test catching up.
//   - err     string   — non-empty when the input is degenerate (nil
//     svc, untyped-nil interface). Callers should fail loudly on err
//     before inspecting missing/stale.
//
// Method names are returned in arbitrary order (map-iteration order).
// Callers that need deterministic ordering for golden-file comparison
// should sort before asserting.
//
// Usage example (helper self-test):
//
//	miss, stale, err := auth.WrapperDriftScan(svc,
//	    map[string]bool{"List": true}, nil)
//	if err != "" || len(miss) != 0 || len(stale) != 0 { /* fail */ }
func WrapperDriftScan(svc any, gated map[string]bool, exempt map[string]bool) (missing []string, stale []string, err string) {
	if svc == nil {
		return nil, nil, "svc is nil — pass a non-nil *Service value"
	}
	typ := core.TypeOf(svc)
	if typ == nil {
		return nil, nil, "TypeOf(svc) is nil — pass a concrete *Service, not an untyped nil"
	}

	// Walk every exported method on the service value. reflect's
	// NumMethod on a pointer type includes both pointer-receiver and
	// value-receiver methods, which matches what Wails binds.
	actual := make(map[string]bool, typ.NumMethod())
	for i := 0; i < typ.NumMethod(); i++ {
		m := typ.Method(i)
		// IsExported filters out unexported methods automatically,
		// but the explicit check makes the intent grep-able.
		if !m.IsExported() {
			continue
		}
		actual[m.Name] = true
	}

	// (a) Missing-gate sweep — every actual method must be in gated
	// OR exempt. A method found in neither is a wrapper-drift bug.
	for name := range actual {
		if gated[name] {
			continue
		}
		if exempt[name] {
			continue
		}
		missing = append(missing, name)
	}

	// (b) Stale-gate sweep — every gated entry must correspond to a
	// real method. Drift in the OTHER direction (rename/remove
	// without map update) is equally dangerous because future
	// auditors trust the gated set as the source of truth.
	for name := range gated {
		if !actual[name] {
			stale = append(stale, name)
		}
	}
	return missing, stale, ""
}
