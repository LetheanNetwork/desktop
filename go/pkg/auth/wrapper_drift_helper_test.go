// SPDX-Licence-Identifier: EUPL-1.2

// wrapper_drift_helper_test.go — pkg-internal coverage for the
// WrapperDriftScan pure function behind AssertWrapperDriftClean.
//
// AssertWrapperDriftClean drives t.Fatalf paths that are hard to
// exercise directly without a synthetic TB (testing.TB has an
// unexported method that locks external mocks out). Splitting the
// scan logic into a pure function means the test matrix below can
// exercise every code path without driving the assert into Fatalf.
//
// Adopter packages still call AssertWrapperDriftClean — these tests
// pin the underlying scan so the assert wrapper stays trivial and
// audit-clean.

package auth_test

import (
	"sort"

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/auth"
)

// scanFixture is a stand-in *Service-shaped value used by the scan
// tests. It carries three exported methods (Alpha, Beta, Gamma) and
// one unexported (delta) so the test can verify the helper ignores
// unexported names and enumerates the exported set faithfully.
//
// The methods themselves are no-ops — the helper only inspects the
// method set, never calls into the method bodies. Returning an int
// keeps each method signature trivially distinct from any other
// reflective consumer that might inspect signatures.
type scanFixture struct{}

func (*scanFixture) Alpha() int { return 1 }
func (*scanFixture) Beta() int  { return 2 }
func (*scanFixture) Gamma() int { return 3 }
func (*scanFixture) delta() int { return 4 } //nolint:unused — used via reflection negative-case

// --- WrapperDriftScan ----------------------------------------------------

// TestWrapperDriftScan_Good — all three exported methods are in the
// gated set and the gated set names nothing extra. Returns empty
// missing + empty stale + empty err. Mirrors the green path adopter
// packages live in once their gated map is in sync.
func TestWrapperDriftScan_Good(t *core.T) {
	miss, stale, err := auth.WrapperDriftScan(&scanFixture{},
		map[string]bool{"Alpha": true, "Beta": true, "Gamma": true},
		nil,
	)
	core.AssertEqual(t, "", err)
	core.AssertLen(t, miss, 0)
	core.AssertLen(t, stale, 0)
}

// TestWrapperDriftScan_Bad_MissingGate — Gamma exists on the
// fixture but is absent from the gated set, simulating the exact
// drift class ADD-4 catches: contributor adds a new method to a
// *Service and forgets the wrapper. Helper reports Gamma in missing.
func TestWrapperDriftScan_Bad_MissingGate(t *core.T) {
	miss, stale, err := auth.WrapperDriftScan(&scanFixture{},
		map[string]bool{"Alpha": true, "Beta": true},
		nil,
	)
	core.AssertEqual(t, "", err)
	core.AssertLen(t, miss, 1)
	core.AssertEqual(t, "Gamma", miss[0])
	core.AssertLen(t, stale, 0)
}

// TestWrapperDriftScan_Bad_StaleGate — gated names a Delta method
// that does not exist on the fixture, simulating a rename / removal
// the test map didn't follow. Helper reports Delta in stale.
func TestWrapperDriftScan_Bad_StaleGate(t *core.T) {
	miss, stale, err := auth.WrapperDriftScan(&scanFixture{},
		map[string]bool{
			"Alpha": true, "Beta": true, "Gamma": true,
			"Delta": true, // <- no such method
		},
		nil,
	)
	core.AssertEqual(t, "", err)
	core.AssertLen(t, miss, 0)
	core.AssertLen(t, stale, 1)
	core.AssertEqual(t, "Delta", stale[0])
}

// TestWrapperDriftScan_Ugly_NilSvc — degenerate input: passing nil
// (no concrete value) returns a clear error string. The assert
// wrapper would Fatalf on this; the pure scan leaves the decision
// to the caller.
func TestWrapperDriftScan_Ugly_NilSvc(t *core.T) {
	miss, stale, err := auth.WrapperDriftScan(nil,
		map[string]bool{"Alpha": true}, nil)
	core.AssertNotEqual(t, "", err)
	core.AssertLen(t, miss, 0)
	core.AssertLen(t, stale, 0)
}

// TestWrapperDriftScan_Ugly_ExemptCoversMissing — Alpha is not in
// the gated set but IS in the exempt set, simulating an inherited
// lifecycle method (e.g. ServiceName from a *core.ServiceRuntime
// embed). Helper treats exempt as legitimate-ungated and does NOT
// flag the method. Pins the carve-out path adopters with embedded
// lifecycle types depend on.
func TestWrapperDriftScan_Ugly_ExemptCoversMissing(t *core.T) {
	miss, stale, err := auth.WrapperDriftScan(&scanFixture{},
		map[string]bool{"Beta": true, "Gamma": true},
		map[string]bool{"Alpha": true},
	)
	core.AssertEqual(t, "", err)
	core.AssertLen(t, miss, 0)
	core.AssertLen(t, stale, 0)
}

// TestWrapperDriftScan_Ugly_BothDirectionsAtOnce — both missing AND
// stale fire on the same scan (one new method ungated + one removed
// method still in the map). Verifies the helper surfaces both rather
// than short-circuiting on the first failure direction.
func TestWrapperDriftScan_Ugly_BothDirectionsAtOnce(t *core.T) {
	miss, stale, err := auth.WrapperDriftScan(&scanFixture{},
		map[string]bool{
			"Alpha": true, "Beta": true,
			"Phantom": true, // stale
			// Gamma intentionally absent (missing)
		},
		nil,
	)
	core.AssertEqual(t, "", err)
	core.AssertLen(t, miss, 1)
	core.AssertEqual(t, "Gamma", miss[0])
	core.AssertLen(t, stale, 1)
	core.AssertEqual(t, "Phantom", stale[0])

	// Sort the slices so the deterministic assertion above doesn't
	// depend on map-iteration ordering for single-element slices —
	// belt-and-braces. (Not load-bearing for single elements but
	// documents the convention if the test grows.)
	sort.Strings(miss)
	sort.Strings(stale)
}

// --- AssertWrapperDriftClean (Good path only — Fatalf paths are    -
// covered indirectly via the scan tests above).                     -

// TestAssertWrapperDriftClean_Good — adopters cargo-cult this shape
// into their own wrapper_drift_test.go files. Verifies the assert
// wrapper is callable from a real test without Fatalf-ing on the
// happy path.
func TestAssertWrapperDriftClean_Good(t *core.T) {
	auth.AssertWrapperDriftClean(t, &scanFixture{},
		map[string]bool{"Alpha": true, "Beta": true, "Gamma": true},
		nil,
	)
}
