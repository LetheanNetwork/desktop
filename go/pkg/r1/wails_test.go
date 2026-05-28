// SPDX-Licence-Identifier: EUPL-1.2

package r1

import "testing"

// --- Lifecycle ---

func TestWailsService_ServiceName_Good(t *testing.T) {
	s := NewWailsService()
	if s == nil {
		t.Fatal("NewWailsService returned nil")
	}
	if got := s.ServiceName(); got != "R1Corpus" {
		t.Errorf("ServiceName = %q, want %q", got, "R1Corpus")
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

// --- Write wire ---

func TestWailsService_Write_Good(t *testing.T) {
	tempCorpus(t)
	s := NewWailsService()
	rec := newRec("gemma4-e2b-it-q4", "english", "P01_TEST")
	r := s.Write(rec)
	if !r.OK {
		t.Fatalf("Write not OK: %v", r.Value)
	}
}

func TestWailsService_Write_UglyMissingModel(t *testing.T) {
	tempCorpus(t)
	s := NewWailsService()
	rec := newRec("", "english", "P01_TEST")
	if r := s.Write(rec); r.OK {
		t.Error("Write with empty Model returned OK, want failure")
	}
}

func TestWailsService_Write_UglyMissingSubject(t *testing.T) {
	tempCorpus(t)
	s := NewWailsService()
	rec := newRec("gemma4-e2b-it-q4", "", "P01_TEST")
	if r := s.Write(rec); r.OK {
		t.Error("Write with empty Subject returned OK, want failure")
	}
}

func TestWailsService_Write_UglyMissingProbe(t *testing.T) {
	tempCorpus(t)
	s := NewWailsService()
	rec := newRec("gemma4-e2b-it-q4", "english", "")
	if r := s.Write(rec); r.OK {
		t.Error("Write with empty ProbeID returned OK, want failure")
	}
}

// --- Read wire ---

func TestWailsService_Read_GoodRoundtrip(t *testing.T) {
	tempCorpus(t)
	s := NewWailsService()
	in := newRec("gemma4-e2b-it-q4", "english", "P01")
	in.Response = "specific wails-side R₁ text"
	in.Voice = "founder"
	if r := s.Write(in); !r.OK {
		t.Fatalf("Write failed: %v", r.Value)
	}

	r := s.Read(Filter{Model: "gemma4-e2b-it-q4"})
	if !r.OK {
		t.Fatalf("Read not OK: %v", r.Value)
	}
	recs, ok := r.Value.([]R1)
	if !ok {
		t.Fatalf("Read returned %T, want []R1", r.Value)
	}
	if len(recs) != 1 {
		t.Fatalf("Read returned %d records, want 1", len(recs))
	}
	if recs[0].Response != in.Response {
		t.Errorf("Response mismatch: %q vs %q", recs[0].Response, in.Response)
	}
	if recs[0].Voice != "founder" {
		t.Errorf("Voice mismatch: %q vs founder", recs[0].Voice)
	}
}

func TestWailsService_Read_GoodEmptyFilterReturnsAll(t *testing.T) {
	tempCorpus(t)
	s := NewWailsService()
	writes := []R1{
		newRec("gemma4-e2b-it-q4", "english", "P01"),
		newRec("gemma4-e2b-it-q4", "chinese", "P02"),
		newRec("gemma4-e4b-it-q4", "english", "P03"),
	}
	for _, w := range writes {
		if r := s.Write(w); !r.OK {
			t.Fatalf("setup Write failed: %v", r.Value)
		}
	}
	r := s.Read(Filter{})
	if !r.OK {
		t.Fatalf("Read{} not OK: %v", r.Value)
	}
	recs := r.Value.([]R1)
	if len(recs) != 3 {
		t.Errorf("Read{} returned %d records, want 3", len(recs))
	}
}

func TestWailsService_Read_GoodModelFilterDiscriminates(t *testing.T) {
	tempCorpus(t)
	s := NewWailsService()
	writes := []R1{
		newRec("gemma4-e2b-it-q4", "english", "P01"),
		newRec("gemma4-e2b-it-q4", "english", "P02"),
		newRec("gemma4-e4b-it-q4", "english", "P01"),
	}
	for _, w := range writes {
		if r := s.Write(w); !r.OK {
			t.Fatalf("setup Write failed: %v", r.Value)
		}
	}
	r := s.Read(Filter{Model: "gemma4-e2b-it-q4"})
	if !r.OK {
		t.Fatalf("Read not OK: %v", r.Value)
	}
	recs := r.Value.([]R1)
	if len(recs) != 2 {
		t.Errorf("Filter{Model:e2b} returned %d records, want 2", len(recs))
	}
	for _, rec := range recs {
		if rec.Model != "gemma4-e2b-it-q4" {
			t.Errorf("filter leaked model %q", rec.Model)
		}
	}
}

func TestWailsService_Read_UglyEmptyCorpus(t *testing.T) {
	tempCorpus(t)
	s := NewWailsService()
	r := s.Read(Filter{})
	if !r.OK {
		t.Fatalf("Read on empty corpus not OK: %v", r.Value)
	}
	recs, ok := r.Value.([]R1)
	if !ok {
		t.Fatalf("Read returned %T, want []R1", r.Value)
	}
	if len(recs) != 0 {
		t.Errorf("empty corpus returned %d records, want 0", len(recs))
	}
}

// --- ListModels / ListSubjects / ListProbes wire ---

func TestWailsService_ListModels_Good(t *testing.T) {
	tempCorpus(t)
	s := NewWailsService()
	for _, m := range []string{"gemma4-e2b-it-q4", "gemma4-e4b-it-q4"} {
		if r := s.Write(newRec(m, "english", "P01")); !r.OK {
			t.Fatalf("setup Write failed: %v", r.Value)
		}
	}
	r := s.ListModels()
	if !r.OK {
		t.Fatalf("ListModels not OK: %v", r.Value)
	}
	models := r.Value.([]string)
	if len(models) != 2 {
		t.Errorf("ListModels returned %d, want 2: %v", len(models), models)
	}
}

func TestWailsService_ListModels_UglyEmptyCorpus(t *testing.T) {
	tempCorpus(t)
	s := NewWailsService()
	r := s.ListModels()
	if !r.OK {
		t.Fatalf("ListModels on empty corpus not OK: %v", r.Value)
	}
	models := r.Value.([]string)
	if len(models) != 0 {
		t.Errorf("empty corpus returned %d models, want 0", len(models))
	}
}

func TestWailsService_ListSubjects_Good(t *testing.T) {
	tempCorpus(t)
	s := NewWailsService()
	for _, subject := range []string{"english", "chinese", "european"} {
		if r := s.Write(newRec("gemma4-e2b-it-q4", subject, "P01")); !r.OK {
			t.Fatalf("setup Write failed: %v", r.Value)
		}
	}
	r := s.ListSubjects("gemma4-e2b-it-q4")
	if !r.OK {
		t.Fatalf("ListSubjects not OK: %v", r.Value)
	}
	subjects := r.Value.([]string)
	if len(subjects) != 3 {
		t.Errorf("ListSubjects returned %d, want 3: %v", len(subjects), subjects)
	}
}

func TestWailsService_ListSubjects_BadMissingModel(t *testing.T) {
	tempCorpus(t)
	s := NewWailsService()
	if r := s.ListSubjects(""); r.OK {
		t.Error("ListSubjects(\"\") returned OK, want failure")
	}
}

func TestWailsService_ListProbes_Good(t *testing.T) {
	tempCorpus(t)
	s := NewWailsService()
	for _, p := range []string{"P01", "P02", "P03"} {
		if r := s.Write(newRec("gemma4-e2b-it-q4", "english", p)); !r.OK {
			t.Fatalf("setup Write failed: %v", r.Value)
		}
	}
	r := s.ListProbes("gemma4-e2b-it-q4", "english")
	if !r.OK {
		t.Fatalf("ListProbes not OK: %v", r.Value)
	}
	probes := r.Value.([]string)
	if len(probes) != 3 {
		t.Errorf("ListProbes returned %d, want 3: %v", len(probes), probes)
	}
}

func TestWailsService_ListProbes_BadMissingSubject(t *testing.T) {
	tempCorpus(t)
	s := NewWailsService()
	if r := s.ListProbes("gemma4-e2b-it-q4", ""); r.OK {
		t.Error("ListProbes with empty subject returned OK, want failure")
	}
}

func TestWailsService_ListProbes_UglyEmptyCorpus(t *testing.T) {
	tempCorpus(t)
	s := NewWailsService()
	r := s.ListProbes("nonexistent", "subject")
	if !r.OK {
		t.Fatalf("ListProbes on missing file not OK: %v", r.Value)
	}
	probes := r.Value.([]string)
	if len(probes) != 0 {
		t.Errorf("ListProbes on missing returned %d, want 0", len(probes))
	}
}
