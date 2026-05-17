// SPDX-Licence-Identifier: EUPL-1.2

package opencode

import (
	"strings"
	"testing"
)

// TestUpgrade_RequiresConfirmation_Bad — UpgradeWithConsent MUST
// refuse with "upgrade.requires_confirmation" when the caller has
// not set ConfirmedByUser=true. The refusal happens BEFORE any
// process service lookup or docker pull side effect — proven here
// by driving against a zero Service{} whose proc() returns nil
// (any path that reached `ps == nil` would surface a different
// error message; reaching the docker pull at all would panic on
// the missing core runtime).
//
// Pins the Cerberus #22 MED-2 / Mantis #1619 supply-chain hardening
// gate: a compromised registry, drive-by HTTP request, or cron-loop
// caller MUST NOT be able to mutate the running image without an
// explicit human approval.
func TestUpgrade_RequiresConfirmation_Bad(t *testing.T) {
	svc := &Service{}

	r := svc.UpgradeWithConsent(UpgradeInput{ConfirmedByUser: false})
	if r.OK {
		t.Fatalf("UpgradeWithConsent succeeded without confirmation; want Fail")
	}
	if got := r.Error(); !strings.Contains(got, "upgrade.requires_confirmation") {
		t.Errorf("UpgradeWithConsent error = %q; want substring %q",
			got, "upgrade.requires_confirmation")
	}
}

// TestUpgrade_NoAutoRestartByDefault_Good — UpgradeInput with
// ConfirmedByUser=true but RestartSandboxes=false (the default)
// MUST NOT in-place restart running sandboxes even when the pull
// produces a new digest. The Cerberus #22 MED-2 / Mantis #1619
// gate cannot be relied on alone — confirmation is the consent
// surface, no-auto-restart is the blast-radius surface.
//
// This test asserts the policy at the type level: the
// RestartSandboxes field defaults to false in a zero
// UpgradeInput{}, and the documented contract is that without
// it the Restarted slice in the result stays empty. The full
// integration shape (mocked docker pull producing "Downloaded
// newer image" + asserting Stop was not called) lives at the
// service-tier integration test pass that follows this lane —
// here the unit-level invariant is the zero-value default of
// the gating field.
func TestUpgrade_NoAutoRestartByDefault_Good(t *testing.T) {
	var in UpgradeInput
	if in.RestartSandboxes {
		t.Errorf("UpgradeInput{}.RestartSandboxes = true; want false (no in-place restart unless caller opts in — Cerberus #22 MED-2)")
	}
	if in.ConfirmedByUser {
		t.Errorf("UpgradeInput{}.ConfirmedByUser = true; want false (gate is opt-in — Cerberus #22 MED-2)")
	}

	// And the consent-gated path with the default RestartSandboxes
	// still respects the type-level invariant: even a confirmed
	// caller does not get auto-restart unless they ask for it.
	gated := UpgradeInput{ConfirmedByUser: true}
	if gated.RestartSandboxes {
		t.Errorf("UpgradeInput{ConfirmedByUser: true}.RestartSandboxes = true; want false (consent is necessary but not sufficient for in-place restart)")
	}
}

// TestUpgrade_ConsentGate_PreEmpts_ProcLookup_Ugly — the gate MUST
// fire before any service-resolution side effect. Drives the path
// where confirmation is missing AND the underlying process service
// would also be unavailable: the caller MUST see the
// requires_confirmation error, NOT the process-unavailable error.
// Surface ordering matters for audit + UX — operator's "I forgot
// to tick the box" recovery is different from "the host's process
// runtime is broken".
func TestUpgrade_ConsentGate_PreEmpts_ProcLookup_Ugly(t *testing.T) {
	svc := &Service{}

	// Confirmation absent + proc() will return nil. Gate must win.
	r := svc.UpgradeWithConsent(UpgradeInput{})
	if r.OK {
		t.Fatalf("UpgradeWithConsent returned OK on zero-input; want Fail")
	}
	got := r.Error()
	if !strings.Contains(got, "upgrade.requires_confirmation") {
		t.Errorf("error = %q; want consent-gate to win, not the proc lookup",
			got)
	}
	if strings.Contains(got, "process service unavailable") {
		t.Errorf("error = %q; gate must short-circuit BEFORE proc() — leaking process-state to a non-confirming caller is a different surface",
			got)
	}
}

// validDigestForTests is a real, well-formed sha256 digest used as
// the "happy" input across the digest-gate tests. The byte sequence
// is documentation-only — nothing in the test layer pulls against it.
const validDigestForTests = "sha256:ca59eb28d5ea6a1f50c45a1f1df5c1a9286343e41b389fe89fb4ffac96dbeb84"

// TestUpgrade_DigestEmpty_RefusesByDefault_Bad — UpgradeWithConsent
// MUST refuse a confirmed pull when no ImageDigest is supplied. The
// Mantis #1621 gate fail-closes by default: callers MUST commit to a
// specific manifest digest. The refusal happens BEFORE proc() lookup
// or any docker side effect (proven by driving against zero Service{}
// — any path that reached `ps == nil` would surface a different
// error message).
//
// Pins the Cerberus #22 MED-2 / Mantis #1621 supply-chain hardening
// gate: a compromised registry can substitute ANY image under a
// :latest tag, so the upgrade path cannot silently accept whatever
// the registry serves. Empty digest = "operator hasn't decided what
// to pull" = fail.
func TestUpgrade_DigestEmpty_RefusesByDefault_Bad(t *testing.T) {
	svc := &Service{}

	r := svc.UpgradeWithConsent(UpgradeInput{ConfirmedByUser: true})
	if r.OK {
		t.Fatalf("UpgradeWithConsent succeeded without ImageDigest; want Fail")
	}
	got := r.Error()
	if !strings.Contains(got, "upgrade.digest_required") {
		t.Errorf("UpgradeWithConsent error = %q; want substring %q",
			got, "upgrade.digest_required")
	}
	if strings.Contains(got, "process service unavailable") {
		t.Errorf("error = %q; digest gate must short-circuit BEFORE proc() — "+
			"a caller without a pinned digest does not get to learn the proc state",
			got)
	}
	if strings.Contains(got, "upgrade.requires_confirmation") {
		t.Errorf("error = %q; consent was supplied — gate must report digest_required, "+
			"not requires_confirmation", got)
	}
}

// TestUpgrade_DigestInvalid_Refuses_Bad — UpgradeWithConsent MUST
// refuse a confirmed pull when ImageDigest is malformed (wrong
// algorithm prefix, wrong length, non-hex characters, uppercase
// hex). The refusal surface is "upgrade.digest_invalid" — distinct
// from "upgrade.digest_required" — so the frontend can render
// "that's not a valid sha256:<64hex>" rather than "please supply a
// digest" (the latter is the absent case).
//
// Drives several malformed shapes through the gate to pin the
// validator behaviour against accidental loosening.
func TestUpgrade_DigestInvalid_Refuses_Bad(t *testing.T) {
	svc := &Service{}
	badShapes := []struct {
		name   string
		digest string
	}{
		{"missing algorithm prefix", "ca59eb28d5ea6a1f50c45a1f1df5c1a9286343e41b389fe89fb4ffac96dbeb84"},
		{"wrong algorithm", "md5:ca59eb28d5ea6a1f50c45a1f1df5c1a9286343e41b389fe89fb4ffac96dbeb84"},
		{"short hex", "sha256:ca59eb28"},
		{"long hex", "sha256:ca59eb28d5ea6a1f50c45a1f1df5c1a9286343e41b389fe89fb4ffac96dbeb8400"},
		{"uppercase hex", "sha256:CA59EB28D5EA6A1F50C45A1F1DF5C1A9286343E41B389FE89FB4FFAC96DBEB84"},
		{"non-hex character", "sha256:ca59eb28d5ea6a1f50c45a1f1df5c1a9286343e41b389fe89fb4ffac96dbeb8z"},
		{"prefix only", "sha256:"},
	}
	for _, tc := range badShapes {
		t.Run(tc.name, func(t *testing.T) {
			r := svc.UpgradeWithConsent(UpgradeInput{
				ConfirmedByUser: true,
				ImageDigest:     tc.digest,
			})
			if r.OK {
				t.Fatalf("UpgradeWithConsent(%s=%q) succeeded; want Fail",
					tc.name, tc.digest)
			}
			got := r.Error()
			if !strings.Contains(got, "upgrade.digest_invalid") {
				t.Errorf("UpgradeWithConsent(%s=%q) error = %q; want substring %q",
					tc.name, tc.digest, got, "upgrade.digest_invalid")
			}
			if strings.Contains(got, "process service unavailable") {
				t.Errorf("digest validator must short-circuit BEFORE proc(); error = %q",
					got)
			}
		})
	}
}

// TestUpgrade_DigestPinned_PassesGate_Good — UpgradeWithConsent with
// ConfirmedByUser=true AND a well-formed ImageDigest MUST pass both
// gates and proceed to the substrate. Proof-of-wiring against a zero
// Service{}: the failure surface MUST be "process service
// unavailable" (i.e. both gates were passed and the call reached
// the proc lookup) rather than "upgrade.digest_required" /
// "upgrade.digest_invalid" / "upgrade.requires_confirmation".
//
// The full digest-match pull integration (mocked docker pull
// producing the expected Digest line + asserting equalDigest agrees)
// lives at the service-tier integration test — here we only pin
// the unit-level gate-PASS at the function boundary.
func TestUpgrade_DigestPinned_PassesGate_Good(t *testing.T) {
	svc := &Service{}

	r := svc.UpgradeWithConsent(UpgradeInput{
		ConfirmedByUser: true,
		ImageDigest:     validDigestForTests,
	})
	if r.OK {
		t.Fatalf("UpgradeWithConsent against zero Service{} returned OK; want substrate Fail")
	}
	got := r.Error()
	for _, gateString := range []string{
		"upgrade.requires_confirmation",
		"upgrade.digest_required",
		"upgrade.digest_invalid",
	} {
		if strings.Contains(got, gateString) {
			t.Fatalf("error = %q contains gate-refusal %q; want both gates passed — "+
				"a well-formed confirmed input must reach the substrate", got, gateString)
		}
	}
	// Sanity: substrate path is the "process service unavailable" surface.
	if !strings.Contains(got, "process service unavailable") {
		t.Errorf("error = %q; want substrate failure 'process service unavailable' "+
			"(the only path past both gates on a zero Service{})", got)
	}
}

// TestUpgrade_ValidSHA256Digest_Good — validSHA256Digest MUST accept
// the canonical OCI manifest digest shape (sha256: prefix + exactly
// 64 lowercase hex chars) and reject every reasonable malformation.
//
// Pins the load-bearing primitive that gates whether ImageDigest is
// considered structurally well-formed before it ever reaches the
// docker CLI. Sibling table TestUpgrade_DigestInvalid_Refuses_Bad
// pins the end-to-end refusal at the Service boundary; this test
// pins the primitive in isolation so a future refactor that moves
// the validator (e.g. into a shared helper) trips a smaller, more
// localised assertion.
func TestUpgrade_ValidSHA256Digest_Good(t *testing.T) {
	cases := []struct {
		digest string
		want   bool
	}{
		// Good shapes
		{"sha256:ca59eb28d5ea6a1f50c45a1f1df5c1a9286343e41b389fe89fb4ffac96dbeb84", true},
		{"sha256:0000000000000000000000000000000000000000000000000000000000000000", true},
		{"sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", true},

		// Bad shapes
		{"", false},
		{"sha256:", false},
		{"sha256:short", false},
		{"sha256:CA59EB28D5EA6A1F50C45A1F1DF5C1A9286343E41B389FE89FB4FFAC96DBEB84", false},
		{"ca59eb28d5ea6a1f50c45a1f1df5c1a9286343e41b389fe89fb4ffac96dbeb84", false},
		{"md5:abcd", false},
		{"sha256:ca59eb28d5ea6a1f50c45a1f1df5c1a9286343e41b389fe89fb4ffac96dbeb8g", false},
	}
	for _, tc := range cases {
		if got := validSHA256Digest(tc.digest); got != tc.want {
			t.Errorf("validSHA256Digest(%q) = %v; want %v", tc.digest, got, tc.want)
		}
	}
}

// TestUpgrade_PinnedPullRef_Good — pinnedPullRef MUST produce the
// canonical "<repo>@<digest>" form across the image-string shapes
// the desktop actually configures: bare repo, repo:tag, registry
// host with port + repo:tag. The registry-port-colon must NOT be
// mistaken for a tag colon (covered by the "registry with port"
// case).
//
// Pins the load-bearing primitive that builds the pull argument
// the runtime sees. A regression that left the ":latest" tag on
// the ref would surface a docker syntax error at pull time (you
// can't supply both :tag and @digest); a regression that stripped
// the registry hostname would silently pull from a different
// registry. Both shapes need to round-trip cleanly.
func TestUpgrade_PinnedPullRef_Good(t *testing.T) {
	const d = "sha256:ca59eb28d5ea6a1f50c45a1f1df5c1a9286343e41b389fe89fb4ffac96dbeb84"
	cases := []struct {
		image string
		want  string
	}{
		{"lthn/dev:latest", "lthn/dev@" + d},
		{"lthn/dev", "lthn/dev@" + d},
		{"lthn/dev:v1.2.3", "lthn/dev@" + d},
		{"registry.example.com:5000/lthn/dev:latest", "registry.example.com:5000/lthn/dev@" + d},
		{"registry.example.com:5000/lthn/dev", "registry.example.com:5000/lthn/dev@" + d},
		{"alpine", "alpine@" + d},
		{"alpine:3.19", "alpine@" + d},
	}
	for _, tc := range cases {
		if got := pinnedPullRef(tc.image, d); got != tc.want {
			t.Errorf("pinnedPullRef(%q, …) = %q; want %q", tc.image, got, tc.want)
		}
	}
}

// TestUpgrade_EqualDigest_Good — equalDigest MUST compare digests
// case-insensitively on the hex portion and reject algorithm
// mismatches even when the hex bytes coincide.
//
// Pins the load-bearing primitive that decides whether a pulled
// digest matches the requested digest. A regression that became
// case-sensitive would falsely-mismatch a runtime that reported
// uppercase; a regression that ignored the algorithm prefix would
// accept a sha512 digest as matching a sha256 request — both are
// silent supply-chain hazards.
func TestUpgrade_EqualDigest_Good(t *testing.T) {
	const lower = "sha256:ca59eb28d5ea6a1f50c45a1f1df5c1a9286343e41b389fe89fb4ffac96dbeb84"
	const upper = "sha256:CA59EB28D5EA6A1F50C45A1F1DF5C1A9286343E41B389FE89FB4FFAC96DBEB84"
	const other = "sha256:0000000000000000000000000000000000000000000000000000000000000000"
	cases := []struct {
		a, b string
		want bool
	}{
		{lower, lower, true},
		{lower, upper, true},  // case-insensitive on hex
		{upper, lower, true},  // symmetric
		{lower, other, false}, // genuinely different
		{"sha256:abc", "sha512:abc", false}, // algorithm mismatch
		{"", "", true},
		{lower, "", false},
	}
	for _, tc := range cases {
		if got := equalDigest(tc.a, tc.b); got != tc.want {
			t.Errorf("equalDigest(%q, %q) = %v; want %v", tc.a, tc.b, got, tc.want)
		}
	}
}
