// SPDX-Licence-Identifier: EUPL-1.2

// build_test.go — sandwich assembly contract tests.
//
// Test naming follows the AX Good/Bad/Ugly contract:
//   - Good: happy path, all inputs present + well-shaped
//   - Bad:  reportable failure modes (missing kernel, missing sig)
//   - Ugly: edge cases (empty probe, trailing-newline preservation,
//           byte-stability via SHA-256 fixture)
//
// The package never touches the canonical LEM root in tests — every
// case stages its own temp-dir fixture so the suite runs in isolation
// (e.g. CI without the sibling LEM checkout).

package sandwich

import (
	"testing"

	core "dappco.re/go"
)

// stageFixture writes a kernel + sig pair into a fresh temp dir and
// returns the dir + the two paths. Bytes are written verbatim (no
// trailing-newline normalisation) so callers control whitespace
// precisely for the byte-stability test.
func stageFixture(t *testing.T, kernel, sig []byte) (root, kernelPath, sigPath string) {
	t.Helper()
	root = t.TempDir()
	kernelPath = core.PathJoin(root, canonicalKernelName)
	sigPath = core.PathJoin(root, canonicalSigName)
	if r := core.WriteFile(kernelPath, kernel, 0o600); !r.OK {
		t.Fatalf("stageFixture: write kernel: %v", r.Value)
	}
	if r := core.WriteFile(sigPath, sig, 0o600); !r.OK {
		t.Fatalf("stageFixture: write sig: %v", r.Value)
	}
	return root, kernelPath, sigPath
}

func TestBuild_Good_ProducesThreeSectionSandwich(t *testing.T) {
	kernel := []byte(`{"k":"v"}`)
	sig := []byte("sig quote")
	probe := "Probe one."
	_, kp, sp := stageFixture(t, kernel, sig)

	r := Build(kp, sp, probe)
	if !r.OK {
		t.Fatalf("Build: %v", r.Value)
	}
	got, _ := r.Value.(string)
	want := `{"k":"v"}` + "\n\n" + "Probe one." + "\n\n" + "sig quote"
	if got != want {
		t.Fatalf("sandwich shape drift\n got: %q\nwant: %q", got, want)
	}
}

func TestBuild_Good_PreservesTrailingNewlines(t *testing.T) {
	// Kernel + sig with trailing newlines (the canonical files on disk
	// will normally have them). The builder must NOT strip — that's the
	// load-bearing whitespace invariant.
	kernel := []byte(`{"k":"v"}` + "\n")
	sig := []byte("sig quote\n")
	probe := "Probe."
	_, kp, sp := stageFixture(t, kernel, sig)

	r := Build(kp, sp, probe)
	if !r.OK {
		t.Fatalf("Build: %v", r.Value)
	}
	got, _ := r.Value.(string)
	want := `{"k":"v"}` + "\n" + "\n\n" + "Probe." + "\n\n" + "sig quote\n"
	if got != want {
		t.Fatalf("trailing-newline drift\n got: %q\nwant: %q", got, want)
	}
}

func TestBuild_Bad_MissingKernel(t *testing.T) {
	_, _, sp := stageFixture(t, []byte("ignored"), []byte("sig"))
	missing := core.PathJoin(t.TempDir(), "no-such-kernel.json")

	r := Build(missing, sp, "probe")
	if r.OK {
		t.Fatalf("expected failure for missing kernel, got OK: %v", r.Value)
	}
	err, ok := r.Value.(error)
	if !ok {
		t.Fatalf("expected error in Value, got %T: %v", r.Value, r.Value)
	}
	if msg := err.Error(); !containsPath(msg, missing) {
		t.Fatalf("error message should name missing kernel path %q, got %q", missing, msg)
	}
}

func TestBuild_Bad_MissingSig(t *testing.T) {
	_, kp, _ := stageFixture(t, []byte("kernel"), []byte("ignored"))
	missing := core.PathJoin(t.TempDir(), "no-such-sig.txt")

	r := Build(kp, missing, "probe")
	if r.OK {
		t.Fatalf("expected failure for missing sig, got OK: %v", r.Value)
	}
	err, ok := r.Value.(error)
	if !ok {
		t.Fatalf("expected error in Value, got %T: %v", r.Value, r.Value)
	}
	if msg := err.Error(); !containsPath(msg, missing) {
		t.Fatalf("error message should name missing sig path %q, got %q", missing, msg)
	}
}

func TestBuild_Ugly_EmptyProbe(t *testing.T) {
	kernel := []byte(`{"k":"v"}`)
	sig := []byte("sig")
	_, kp, sp := stageFixture(t, kernel, sig)

	r := Build(kp, sp, "")
	if !r.OK {
		t.Fatalf("empty probe should still build, got fail: %v", r.Value)
	}
	got, _ := r.Value.(string)
	// Three sections still — middle one is empty. Separators are still
	// present on both sides of the empty middle so the model sees the
	// expected shape.
	want := `{"k":"v"}` + "\n\n" + "" + "\n\n" + "sig"
	if got != want {
		t.Fatalf("empty-probe sandwich drift\n got: %q\nwant: %q", got, want)
	}
}

func TestBuild_Ugly_SHA256_FixtureStability(t *testing.T) {
	// Known-input / known-output byte-stability check. Any future code
	// change that silently re-orders, re-marshals, or normalises the
	// sandwich bytes will flip this digest and the test will fail loud.
	//
	// The fixture inputs (kernel/probe/sig) are arbitrary but locked
	// here so the expected digest is reproducible. The point is the
	// invariant: same bytes in → same bytes out → same SHA-256.
	kernel := []byte(`{"axioms":["A1","A2"],"version":1}` + "\n")
	sig := []byte("Where the river runs, the path is clear.\n")
	probe := "Describe how a system handles a contested claim."

	_, kp, sp := stageFixture(t, kernel, sig)
	r := Build(kp, sp, probe)
	if !r.OK {
		t.Fatalf("Build: %v", r.Value)
	}
	got, _ := r.Value.(string)
	digest := core.SHA256HexString(got)

	// Re-derive expected digest from the SAME inputs assembled by hand —
	// this guards against the Build path changing its concatenation
	// shape (e.g. accidentally swapping the section order, or losing
	// a separator) without us having to maintain a separate fixture
	// file. If Build and this hand-derivation diverge, the test fails
	// before the digest comparison would even run.
	expected := string(kernel) + "\n\n" + probe + "\n\n" + string(sig)
	expectedDigest := core.SHA256HexString(expected)

	if digest != expectedDigest {
		t.Fatalf("sandwich byte-stability broken\n got digest: %s\nwant digest: %s\n got body: %q\nwant body: %q",
			digest, expectedDigest, got, expected)
	}

	// Pin the digest literal so a regression in Build OR in the
	// hand-derivation above both surface. Computed externally for the
	// fixture inputs (kernel + probe + sig as defined above). If the
	// fixture ever changes deliberately, recompute via:
	//
	//	echo -n '<sandwich-body>' | shasum -a 256
	//
	// and update both the inputs above and this constant in the same
	// commit.
	const pinnedDigest = "8e83f3907ae0153e58221b32b8e7dee0c28eeafc4e49aad032020c5603db43f9"
	if digest != pinnedDigest {
		t.Fatalf("sandwich byte-stability fixture-pin broken\n got digest: %s\nwant digest: %s\n got body: %q",
			digest, pinnedDigest, got)
	}
}

// containsPath returns true when haystack contains needle. Used by the
// missing-file tests to confirm the error message names the offending
// path. Avoids strings.Contains per the AX-6 banned-stdlib rule.
func containsPath(haystack, needle string) bool {
	if len(needle) == 0 {
		return true
	}
	if len(haystack) < len(needle) {
		return false
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

func TestBuildWithOptions_Good_ResolvesFromRoot(t *testing.T) {
	kernel := []byte(`{"k":"v"}`)
	sig := []byte("sig")
	root, _, _ := stageFixture(t, kernel, sig)

	r := BuildWithOptions(Options{Root: root}, "probe")
	if !r.OK {
		t.Fatalf("BuildWithOptions: %v", r.Value)
	}
	got, _ := r.Value.(string)
	want := `{"k":"v"}` + "\n\n" + "probe" + "\n\n" + "sig"
	if got != want {
		t.Fatalf("BuildWithOptions drift\n got: %q\nwant: %q", got, want)
	}
}

func TestBuildWithOptions_Good_ExplicitPathsOverrideRoot(t *testing.T) {
	// Two staged dirs: Root points at one, but KernelPath/SigPath
	// override individually to the other. Confirms the per-field
	// overrides take precedence per the Options doc.
	root1, _, _ := stageFixture(t, []byte("WRONG-kernel"), []byte("WRONG-sig"))
	_, kp2, sp2 := stageFixture(t, []byte("RIGHT-kernel"), []byte("RIGHT-sig"))

	r := BuildWithOptions(Options{Root: root1, KernelPath: kp2, SigPath: sp2}, "probe")
	if !r.OK {
		t.Fatalf("BuildWithOptions: %v", r.Value)
	}
	got, _ := r.Value.(string)
	want := "RIGHT-kernel" + "\n\n" + "probe" + "\n\n" + "RIGHT-sig"
	if got != want {
		t.Fatalf("override precedence drift\n got: %q\nwant: %q", got, want)
	}
}
