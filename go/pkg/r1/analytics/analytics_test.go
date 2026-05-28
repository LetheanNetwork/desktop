// SPDX-Licence-Identifier: EUPL-1.2

package analytics

import (
	"os"
	"testing"

	core "dappco.re/go"

	"dappco.re/lthn/desktop/pkg/paths"
	"dappco.re/lthn/desktop/pkg/r1"
)

// tempCorpus repoints $HOME to a fresh temp dir so paths.R1Dir()
// resolves under a per-test root. Mirrors the pattern in
// pkg/r1/store_test.go.
func tempCorpus(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
}

// writeRec is the canonical fixture-writer — exercises the real
// pkg/r1.Write code path so analytics_test covers the full storage
// + analytics roundtrip on every assertion.
func writeRec(t *testing.T, rec r1.R1) {
	t.Helper()
	if rr := r1.Write(rec); !rr.OK {
		t.Fatalf("r1.Write failed: %v", rr.Value)
	}
}

// --- OpenView ---

func TestOpenView_Good(t *testing.T) {
	tempCorpus(t)
	writeRec(t, r1.R1{
		Model: "gemma4-e2b-it-q4", Subject: "english", ProbeID: "P01",
		Substrate: "CONT", Tier: 0, Timestamp: 1000,
		Prompt: "p", Response: "r1",
	})
	writeRec(t, r1.R1{
		Model: "gemma4-e2b-it-q4", Subject: "english", ProbeID: "P02",
		Substrate: "CONT", Tier: 0, Timestamp: 1001,
		Prompt: "p", Response: "r2",
	})

	r := OpenView(Options{})
	if !r.OK {
		t.Fatalf("OpenView failed: %v", r.Value)
	}
	view := r.Value.(*View)
	defer func() { _ = view.Close() }()

	var n int
	if err := view.DB().QueryRow("SELECT count(*) FROM r1").Scan(&n); err != nil {
		t.Fatalf("count(*) FROM r1: %v", err)
	}
	if n != 2 {
		t.Errorf("count(*) = %d, want 2", n)
	}
}

func TestOpenView_UglyEmptyCorpus(t *testing.T) {
	tempCorpus(t)
	// No writes — corpus root may not even exist as a directory yet,
	// but paths.R1Dir() will create the root on first call inside
	// OpenView via the resolveRoot path. Either way the glob matches
	// nothing.

	r := OpenView(Options{})
	if !r.OK {
		t.Fatalf("OpenView on empty corpus should succeed, got: %v", r.Value)
	}
	view := r.Value.(*View)
	defer func() { _ = view.Close() }()

	var n int
	if err := view.DB().QueryRow("SELECT count(*) FROM r1").Scan(&n); err != nil {
		t.Fatalf("count(*) on empty view: %v", err)
	}
	if n != 0 {
		t.Errorf("empty-corpus count(*) = %d, want 0", n)
	}
}

func TestOpenView_UglyMalformedRecord(t *testing.T) {
	tempCorpus(t)
	writeRec(t, r1.R1{
		Model: "gemma4-e2b-it-q4", Subject: "english", ProbeID: "P01",
		Substrate: "CONT", Tier: 0, Timestamp: 1000,
		Prompt: "p", Response: "r1",
	})

	// Append a torn line directly to the JSONL file — simulates a
	// crashed writer mid-record. pkg/r1.decodeLines tolerates this;
	// DuckDB's read_json_auto with ignore_errors=true must match.
	rootR := paths.R1Dir()
	if !rootR.OK {
		t.Fatalf("R1Dir failed: %v", rootR.Value)
	}
	path := core.PathJoin(rootR.Value.(string), "gemma4-e2b-it-q4", "english.jsonl")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("open jsonl for append: %v", err)
	}
	if _, err := f.WriteString("this is not json at all\n"); err != nil {
		_ = f.Close()
		t.Fatalf("append torn line: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close jsonl: %v", err)
	}

	r := OpenView(Options{})
	if !r.OK {
		t.Fatalf("OpenView with torn record should not panic; got: %v", r.Value)
	}
	view := r.Value.(*View)
	defer func() { _ = view.Close() }()

	// DuckDB read_json_auto + ignore_errors=true tolerates malformed
	// lines. Some DuckDB versions surface them as all-NULL rows
	// rather than skipping outright. The contract we want is:
	//  (a) no panic / no query error,
	//  (b) the valid record's fields are intact and queryable.
	// Asserting count where model IS NOT NULL captures both — the
	// torn line either disappears entirely OR appears as a NULL row
	// that gets filtered out.
	var n int
	if err := view.DB().QueryRow("SELECT count(*) FROM r1 WHERE model IS NOT NULL").Scan(&n); err != nil {
		t.Fatalf("count(*) with torn record: %v", err)
	}
	if n != 1 {
		t.Errorf("count(*) WHERE model IS NOT NULL = %d, want 1", n)
	}
}

// --- RecordsPerSubject ---

func TestRecordsPerSubject_Good(t *testing.T) {
	tempCorpus(t)
	model := "gemma4-e2b-it-q4"
	writeRec(t, r1.R1{Model: model, Subject: "english", ProbeID: "P01", Substrate: "CONT", Tier: 0, Timestamp: 1, Prompt: "p", Response: "a"})
	writeRec(t, r1.R1{Model: model, Subject: "english", ProbeID: "P02", Substrate: "CONT", Tier: 0, Timestamp: 2, Prompt: "p", Response: "b"})
	writeRec(t, r1.R1{Model: model, Subject: "chinese", ProbeID: "P03", Substrate: "CONT", Tier: 0, Timestamp: 3, Prompt: "p", Response: "c"})

	r := RecordsPerSubject(Options{}, model)
	if !r.OK {
		t.Fatalf("RecordsPerSubject failed: %v", r.Value)
	}
	rows := r.Value.([]SubjectCount)
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2: %+v", len(rows), rows)
	}
	if rows[0].Subject != "english" || rows[0].Count != 2 {
		t.Errorf("rows[0] = %+v, want {english 2}", rows[0])
	}
	if rows[1].Subject != "chinese" || rows[1].Count != 1 {
		t.Errorf("rows[1] = %+v, want {chinese 1}", rows[1])
	}
}

func TestRecordsPerSubject_GoodAggregateAcrossModels(t *testing.T) {
	tempCorpus(t)
	writeRec(t, r1.R1{Model: "model-a", Subject: "english", ProbeID: "P01", Substrate: "CONT", Tier: 0, Timestamp: 1, Prompt: "p", Response: "a"})
	writeRec(t, r1.R1{Model: "model-b", Subject: "english", ProbeID: "P02", Substrate: "CONT", Tier: 1, Timestamp: 2, Prompt: "p", Response: "b"})
	writeRec(t, r1.R1{Model: "model-b", Subject: "chinese", ProbeID: "P03", Substrate: "CONT", Tier: 1, Timestamp: 3, Prompt: "p", Response: "c"})

	r := RecordsPerSubject(Options{}, "")
	if !r.OK {
		t.Fatalf("RecordsPerSubject aggregate failed: %v", r.Value)
	}
	rows := r.Value.([]SubjectCount)
	if len(rows) != 3 {
		t.Fatalf("got %d rows, want 3: %+v", len(rows), rows)
	}
	// Verify both models appear.
	seen := map[string]bool{}
	for _, row := range rows {
		seen[row.Model] = true
	}
	if !seen["model-a"] || !seen["model-b"] {
		t.Errorf("aggregate missing a model: %+v", rows)
	}
}

// --- CrossTierCounts ---

func TestCrossTierCounts_Good(t *testing.T) {
	tempCorpus(t)
	model := "gemma4-e2b-it-q4"
	writeRec(t, r1.R1{Model: model, Subject: "english", ProbeID: "P01", Substrate: "CONT", Tier: 0, Timestamp: 1, Prompt: "p", Response: "a"})
	writeRec(t, r1.R1{Model: model, Subject: "english", ProbeID: "P02", Substrate: "CONT", Tier: 1, Timestamp: 2, Prompt: "p", Response: "b"})
	writeRec(t, r1.R1{Model: model, Subject: "chinese", ProbeID: "P03", Substrate: "CONT", Tier: 2, Timestamp: 3, Prompt: "p", Response: "c"})
	writeRec(t, r1.R1{Model: model, Subject: "chinese", ProbeID: "P04", Substrate: "CONT", Tier: 2, Timestamp: 4, Prompt: "p", Response: "d"})

	r := CrossTierCounts(Options{}, model)
	if !r.OK {
		t.Fatalf("CrossTierCounts failed: %v", r.Value)
	}
	rows := r.Value.([]TierSubjectCount)
	if len(rows) != 3 {
		t.Fatalf("got %d rows, want 3: %+v", len(rows), rows)
	}

	// Order is tier ASC then count DESC then subject ASC.
	want := []TierSubjectCount{
		{Tier: 0, Subject: "english", Count: 1},
		{Tier: 1, Subject: "english", Count: 1},
		{Tier: 2, Subject: "chinese", Count: 2},
	}
	for i, w := range want {
		if rows[i].Tier != w.Tier || rows[i].Subject != w.Subject || rows[i].Count != w.Count {
			t.Errorf("rows[%d] = %+v, want %+v", i, rows[i], w)
		}
	}
}

// --- LatestPerProbe ---

func TestLatestPerProbe_Good(t *testing.T) {
	tempCorpus(t)
	model := "gemma4-e2b-it-q4"
	subject := "english"

	// Two revisions of P01 — the later ts must win.
	writeRec(t, r1.R1{Model: model, Subject: subject, ProbeID: "P01", Substrate: "CONT", Tier: 0, Timestamp: 100, Prompt: "p", Response: "old"})
	writeRec(t, r1.R1{Model: model, Subject: subject, ProbeID: "P01", Substrate: "CONT", Tier: 0, Timestamp: 200, Prompt: "p", Response: "new"})
	// A separate probe.
	writeRec(t, r1.R1{Model: model, Subject: subject, ProbeID: "P02", Substrate: "CONT", Tier: 0, Timestamp: 150, Prompt: "p", Response: "p2"})

	r := LatestPerProbe(Options{}, model, subject)
	if !r.OK {
		t.Fatalf("LatestPerProbe failed: %v", r.Value)
	}
	recs := r.Value.([]r1.R1)
	if len(recs) != 2 {
		t.Fatalf("got %d recs, want 2: %+v", len(recs), recs)
	}
	// Order is ts DESC, probe_id ASC.
	if recs[0].ProbeID != "P01" || recs[0].Response != "new" || recs[0].Timestamp != 200 {
		t.Errorf("recs[0] = %+v, want {P01 ts=200 new}", recs[0])
	}
	if recs[1].ProbeID != "P02" || recs[1].Response != "p2" {
		t.Errorf("recs[1] = %+v, want {P02 p2}", recs[1])
	}
}

// --- FingerprintTrajectory ---

func TestFingerprintTrajectory_Good(t *testing.T) {
	tempCorpus(t)
	model := "gemma4-e2b-it-q4"
	subject := "english"
	writeRec(t, r1.R1{
		Model: model, Subject: subject, ProbeID: "P01",
		Substrate: "CONT", Tier: 0, Timestamp: 100,
		Prompt: "p", Response: "a",
		Fingerprint: map[string]float64{"vocab_richness": 0.10, "tense_entropy": 1.0},
	})
	writeRec(t, r1.R1{
		Model: model, Subject: subject, ProbeID: "P02",
		Substrate: "CONT", Tier: 0, Timestamp: 200,
		Prompt: "p", Response: "b",
		Fingerprint: map[string]float64{"vocab_richness": 0.25, "tense_entropy": 1.1},
	})
	writeRec(t, r1.R1{
		Model: model, Subject: subject, ProbeID: "P03",
		Substrate: "CONT", Tier: 0, Timestamp: 300,
		Prompt: "p", Response: "c",
		Fingerprint: map[string]float64{"vocab_richness": 0.50, "tense_entropy": 1.2},
	})

	r := FingerprintTrajectory(Options{}, model, subject, "vocab_richness")
	if !r.OK {
		t.Fatalf("FingerprintTrajectory failed: %v", r.Value)
	}
	points := r.Value.([]FingerprintPoint)
	if len(points) != 3 {
		t.Fatalf("got %d points, want 3: %+v", len(points), points)
	}

	wantTs := []int64{100, 200, 300}
	wantVal := []float64{0.10, 0.25, 0.50}
	wantProbe := []string{"P01", "P02", "P03"}
	for i := range points {
		if points[i].Timestamp != wantTs[i] {
			t.Errorf("points[%d].Timestamp = %d, want %d", i, points[i].Timestamp, wantTs[i])
		}
		if points[i].ProbeID != wantProbe[i] {
			t.Errorf("points[%d].ProbeID = %s, want %s", i, points[i].ProbeID, wantProbe[i])
		}
		if points[i].Value != wantVal[i] {
			t.Errorf("points[%d].Value = %v, want %v", i, points[i].Value, wantVal[i])
		}
	}
}

func TestFingerprintTrajectory_UglyMissingDim(t *testing.T) {
	tempCorpus(t)
	model := "gemma4-e2b-it-q4"
	subject := "english"
	writeRec(t, r1.R1{
		Model: model, Subject: subject, ProbeID: "P01",
		Substrate: "CONT", Tier: 0, Timestamp: 100,
		Prompt: "p", Response: "a",
		Fingerprint: map[string]float64{"vocab_richness": 0.1},
	})

	r := FingerprintTrajectory(Options{}, model, subject, "nonexistent_dim")
	if !r.OK {
		t.Fatalf("FingerprintTrajectory on missing dim should return Ok with empty slice; got Fail: %v", r.Value)
	}
	points := r.Value.([]FingerprintPoint)
	if len(points) != 0 {
		t.Errorf("missing-dim trajectory returned %d points, want 0: %+v", len(points), points)
	}
}
