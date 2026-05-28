// SPDX-Licence-Identifier: EUPL-1.2

package calibrate

import "testing"

// autotuneStdout mirrors a real `lthn-mlx auto-tune` run's stdout (the
// CLI has no --json; we parse these lines).
const autotuneStdout = `auto-tune: model=/Users/me/models/lemer-lite workload=chat machine=sha256:abc123
  planning candidates…
  measuring 4 candidates (this can take several minutes)…
    candidate c1: score=0.512
    candidate c2: score=0.842
    candidate c3: score=0.701

selected: c2 (score=0.842)
saved:    /Users/me/Lethean/data/profiles/lemer-lite-chat.json

run ` + "`lthn-mlx serve /Users/me/models/lemer-lite`" + ` — the profile auto-applies.
`

// TestParseCalibrateResult_Good extracts the winner, score, and saved
// profile path from a complete sweep.
func TestParseCalibrateResult_Good(t *testing.T) {
	res, found := parseCalibrateResult(autotuneStdout)
	if !found {
		t.Fatalf("parseCalibrateResult: found=false, want true")
	}
	if res.SelectedID != "c2" {
		t.Errorf("SelectedID = %q, want c2", res.SelectedID)
	}
	if res.Score != "0.842" {
		t.Errorf("Score = %q, want 0.842", res.Score)
	}
	if res.ProfilePath != "/Users/me/Lethean/data/profiles/lemer-lite-chat.json" {
		t.Errorf("ProfilePath = %q, want the saved profile path", res.ProfilePath)
	}
}

// TestParseCalibrateResult_Bad returns found=false when the sweep
// produced no selection (failed before "selected:").
func TestParseCalibrateResult_Bad(t *testing.T) {
	partial := "auto-tune: model=x workload=chat machine=y\n  planning candidates…\n  no tuning candidates\n"
	if _, found := parseCalibrateResult(partial); found {
		t.Errorf("parseCalibrateResult(no-selection): found=true, want false")
	}
	if _, found := parseCalibrateResult(""); found {
		t.Errorf("parseCalibrateResult(empty): found=true, want false")
	}
}

// TestParseCalibrateResult_Ugly survives a malformed selected line (no
// score / no saved line) without panicking — found is still true, the
// missing fields stay empty.
func TestParseCalibrateResult_Ugly(t *testing.T) {
	res, found := parseCalibrateResult("selected: loneCandidate\n")
	if !found {
		t.Fatalf("parseCalibrateResult(no-score): found=false, want true")
	}
	if res.SelectedID != "loneCandidate" {
		t.Errorf("SelectedID = %q, want loneCandidate", res.SelectedID)
	}
	if res.Score != "" || res.ProfilePath != "" {
		t.Errorf("Score/ProfilePath should be empty for a bare selected line, got %q / %q", res.Score, res.ProfilePath)
	}
}

// TestNormaliseWorkload_Good passes valid workloads through unchanged.
func TestNormaliseWorkload_Good(t *testing.T) {
	for _, w := range []string{"chat", "coding", "long_context", "agent_state", "throughput", "low_latency"} {
		if got := normaliseWorkload(w); got != w {
			t.Errorf("normaliseWorkload(%q) = %q, want unchanged", w, got)
		}
	}
}

// TestNormaliseWorkload_Bad falls back to chat for empty / unknown.
func TestNormaliseWorkload_Bad(t *testing.T) {
	for _, w := range []string{"", "  ", "bogus", "CHAT"} {
		if got := normaliseWorkload(w); got != "chat" {
			t.Errorf("normaliseWorkload(%q) = %q, want chat", w, got)
		}
	}
}
