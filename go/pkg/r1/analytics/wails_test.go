// SPDX-Licence-Identifier: EUPL-1.2

package analytics

import (
	"testing"

	"dappco.re/lthn/desktop/pkg/r1"
)

// --- Lifecycle ---

func TestWailsService_ServiceName_Good(t *testing.T) {
	s := NewWailsService()
	if s == nil {
		t.Fatal("NewWailsService returned nil")
	}
	if got := s.ServiceName(); got != "R1Analytics" {
		t.Errorf("ServiceName = %q, want %q", got, "R1Analytics")
	}
}

func TestWailsService_ServiceStartup_Good(t *testing.T) {
	s := NewWailsService()
	r := s.ServiceStartup(nil, nil)
	if !r.OK {
		t.Errorf("ServiceStartup not OK: %v", r.Value)
	}
}

func TestWailsService_ServiceShutdown_Good(t *testing.T) {
	s := NewWailsService()
	r := s.ServiceShutdown()
	if !r.OK {
		t.Errorf("ServiceShutdown not OK: %v", r.Value)
	}
}

// --- RecordsPerSubject wire ---

func TestWailsService_RecordsPerSubject_Good(t *testing.T) {
	tempCorpus(t)
	writeRec(t, r1.R1{
		Model: "gemma4-e2b-it-q4", Subject: "english", ProbeID: "P01",
		Substrate: "CONT", Tier: 0, Timestamp: 1000,
		Prompt: "p", Response: "r",
	})
	writeRec(t, r1.R1{
		Model: "gemma4-e2b-it-q4", Subject: "english", ProbeID: "P02",
		Substrate: "CONT", Tier: 0, Timestamp: 1001,
		Prompt: "p", Response: "r",
	})

	s := NewWailsService()
	r := s.RecordsPerSubject("gemma4-e2b-it-q4")
	if !r.OK {
		t.Fatalf("RecordsPerSubject not OK: %v", r.Value)
	}
	rows := r.Value.([]SubjectCount)
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	if rows[0].Subject != "english" || rows[0].Count != 2 {
		t.Errorf("rows[0] = %+v, want {english, 2}", rows[0])
	}
}

func TestWailsService_RecordsPerSubject_EmptyModelAggregates(t *testing.T) {
	tempCorpus(t)
	writeRec(t, r1.R1{
		Model: "gemma4-e2b-it-q4", Subject: "english", ProbeID: "P01",
		Substrate: "CONT", Tier: 0, Timestamp: 1000,
		Prompt: "p", Response: "r",
	})
	writeRec(t, r1.R1{
		Model: "gemma4-e4b-it-q4", Subject: "russian", ProbeID: "P03",
		Substrate: "CONT", Tier: 1, Timestamp: 1100,
		Prompt: "p", Response: "r",
	})

	s := NewWailsService()
	r := s.RecordsPerSubject("")
	if !r.OK {
		t.Fatalf("RecordsPerSubject not OK: %v", r.Value)
	}
	rows := r.Value.([]SubjectCount)
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2 (one per model)", len(rows))
	}
}

// --- CrossTierCounts wire ---

func TestWailsService_CrossTierCounts_Good(t *testing.T) {
	tempCorpus(t)
	// Tier 0 — base tier captures english + russian.
	writeRec(t, r1.R1{
		Model: "gemma4-e2b-it-q4", Subject: "english", ProbeID: "P01",
		Substrate: "CONT", Tier: 0, Timestamp: 1000, Prompt: "p", Response: "r",
	})
	writeRec(t, r1.R1{
		Model: "gemma4-e2b-it-q4", Subject: "russian", ProbeID: "P01",
		Substrate: "CONT", Tier: 0, Timestamp: 1001, Prompt: "p", Response: "r",
	})
	// Tier 1 — next-up tier captures english only.
	writeRec(t, r1.R1{
		Model: "gemma4-e4b-it-q4", Subject: "english", ProbeID: "P01",
		Substrate: "CONT", Tier: 1, Timestamp: 1100, Prompt: "p", Response: "r",
	})

	s := NewWailsService()
	r := s.CrossTierCounts("")
	if !r.OK {
		t.Fatalf("CrossTierCounts not OK: %v", r.Value)
	}
	rows := r.Value.([]TierSubjectCount)
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want 3 (tier0/english + tier0/russian + tier1/english)", len(rows))
	}
}

func TestWailsService_CrossTierCounts_EmptyCorpus(t *testing.T) {
	tempCorpus(t)
	s := NewWailsService()
	r := s.CrossTierCounts("")
	if !r.OK {
		t.Fatalf("CrossTierCounts on empty corpus should succeed, got: %v", r.Value)
	}
	rows := r.Value.([]TierSubjectCount)
	if len(rows) != 0 {
		t.Errorf("empty-corpus rows = %d, want 0", len(rows))
	}
}

// --- LatestPerProbe wire ---

func TestWailsService_LatestPerProbe_Good(t *testing.T) {
	tempCorpus(t)
	// Older revision of P01.
	writeRec(t, r1.R1{
		Model: "gemma4-e2b-it-q4", Subject: "english", ProbeID: "P01",
		Substrate: "CONT", Tier: 0, Timestamp: 1000,
		Prompt: "p", Response: "older",
	})
	// Newer revision of P01 — should win.
	writeRec(t, r1.R1{
		Model: "gemma4-e2b-it-q4", Subject: "english", ProbeID: "P01",
		Substrate: "CONT", Tier: 0, Timestamp: 1500,
		Prompt: "p", Response: "newer",
	})
	// Single P02.
	writeRec(t, r1.R1{
		Model: "gemma4-e2b-it-q4", Subject: "english", ProbeID: "P02",
		Substrate: "CONT", Tier: 0, Timestamp: 1200,
		Prompt: "p", Response: "p02",
	})

	s := NewWailsService()
	r := s.LatestPerProbe("gemma4-e2b-it-q4", "english")
	if !r.OK {
		t.Fatalf("LatestPerProbe not OK: %v", r.Value)
	}
	recs := r.Value.([]r1.R1)
	if len(recs) != 2 {
		t.Fatalf("recs = %d, want 2 (P01 latest + P02)", len(recs))
	}
	for _, rec := range recs {
		if rec.ProbeID == "P01" && rec.Response != "newer" {
			t.Errorf("P01 latest = %q, want %q", rec.Response, "newer")
		}
	}
}

func TestWailsService_LatestPerProbe_UglyMissingArgs(t *testing.T) {
	tempCorpus(t)
	s := NewWailsService()
	if r := s.LatestPerProbe("", "english"); r.OK {
		t.Error("LatestPerProbe with empty model returned OK, want failure")
	}
	if r := s.LatestPerProbe("gemma4-e2b-it-q4", ""); r.OK {
		t.Error("LatestPerProbe with empty subject returned OK, want failure")
	}
}

// --- FingerprintTrajectory wire ---

func TestWailsService_FingerprintTrajectory_Good(t *testing.T) {
	tempCorpus(t)
	writeRec(t, r1.R1{
		Model: "gemma4-e2b-it-q4", Subject: "english", ProbeID: "P01",
		Substrate: "CONT", Tier: 0, Timestamp: 1000,
		Prompt: "p", Response: "r",
		Fingerprint: map[string]float64{"vocab_richness": 0.42},
	})
	writeRec(t, r1.R1{
		Model: "gemma4-e2b-it-q4", Subject: "english", ProbeID: "P02",
		Substrate: "CONT", Tier: 0, Timestamp: 1100,
		Prompt: "p", Response: "r",
		Fingerprint: map[string]float64{"vocab_richness": 0.51},
	})

	s := NewWailsService()
	r := s.FingerprintTrajectory("gemma4-e2b-it-q4", "english", "vocab_richness")
	if !r.OK {
		t.Fatalf("FingerprintTrajectory not OK: %v", r.Value)
	}
	pts := r.Value.([]FingerprintPoint)
	if len(pts) != 2 {
		t.Fatalf("pts = %d, want 2", len(pts))
	}
	if pts[0].Value != 0.42 || pts[1].Value != 0.51 {
		t.Errorf("values = %v / %v, want 0.42 / 0.51", pts[0].Value, pts[1].Value)
	}
}

func TestWailsService_FingerprintTrajectory_UglyMissingDim(t *testing.T) {
	tempCorpus(t)
	s := NewWailsService()
	if r := s.FingerprintTrajectory("m", "s", ""); r.OK {
		t.Error("FingerprintTrajectory with empty dim returned OK, want failure")
	}
}
