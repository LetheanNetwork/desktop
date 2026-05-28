// SPDX-Licence-Identifier: EUPL-1.2

package seeds

import (
	"testing"

	core "dappco.re/go"
)

// newWailsAt constructs a WailsService rooted at dir so the test
// does not have to depend on /Volumes/Data/ being mounted. Production
// wiring goes through NewWailsService() (Root empty → default root)
// instead — the lifecycle test below covers that shape.
func newWailsAt(dir string) *WailsService { return &WailsService{Root: dir} }

// --- Lifecycle ---

func TestWailsService_ServiceName_Good(t *testing.T) {
	s := NewWailsService()
	if s == nil {
		t.Fatal("NewWailsService returned nil")
	}
	if got := s.ServiceName(); got != "Seeds" {
		t.Errorf("ServiceName = %q, want %q", got, "Seeds")
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

// --- ListSubjects ---

func TestWailsService_ListSubjects_Good(t *testing.T) {
	root := buildFixture(t, "russian")
	s := newWailsAt(root)
	r := s.ListSubjects()
	if !r.OK {
		t.Fatalf("ListSubjects not OK: %v", r.Value)
	}
	subs, ok := r.Value.([]string)
	if !ok {
		t.Fatalf("ListSubjects returned %T, want []string", r.Value)
	}
	// buildFixture lays out english + european + russian +
	// middle-east + chinese — five canonical subjects.
	if len(subs) < 2 {
		t.Fatalf("ListSubjects returned %d subjects, want >=2: %v", len(subs), subs)
	}
	found := map[string]bool{}
	for _, sub := range subs {
		found[sub] = true
	}
	for _, want := range []string{"english", "chinese"} {
		if !found[want] {
			t.Errorf("missing canonical subject %q in %v", want, subs)
		}
	}
}

func TestWailsService_ListSubjects_UglyEmptyCorpus(t *testing.T) {
	s := newWailsAt(t.TempDir())
	r := s.ListSubjects()
	if !r.OK {
		t.Fatalf("ListSubjects on empty corpus not OK: %v", r.Value)
	}
	subs, ok := r.Value.([]string)
	if !ok {
		t.Fatalf("ListSubjects returned %T, want []string", r.Value)
	}
	if len(subs) != 0 {
		t.Errorf("empty corpus returned %d subjects, want 0: %v", len(subs), subs)
	}
}

// --- ListProbes ---

func TestWailsService_ListProbes_Good(t *testing.T) {
	// Build a fixture with three probes in one subject so we can
	// assert first-seen order is preserved end-to-end through the
	// Wails wrapper.
	root := t.TempDir()
	dir := core.PathJoin(root, "english")
	if r := core.MkdirAll(dir, 0o755); !r.OK {
		t.Fatalf("mkdir: %v", r.Value)
	}
	body := `{"seed_id":"P01","prompt":"first"}
{"seed_id":"P02","prompt":"second"}
{"seed_id":"P03","prompt":"third"}
`
	if r := core.WriteFile(core.PathJoin(dir, "seeds.jsonl"), []byte(body), 0o644); !r.OK {
		t.Fatalf("write: %v", r.Value)
	}

	s := newWailsAt(root)
	r := s.ListProbes("english")
	if !r.OK {
		t.Fatalf("ListProbes not OK: %v", r.Value)
	}
	probes, ok := r.Value.([]string)
	if !ok {
		t.Fatalf("ListProbes returned %T, want []string", r.Value)
	}
	want := []string{"P01", "P02", "P03"}
	if len(probes) != len(want) {
		t.Fatalf("got %d probes %v, want %d %v", len(probes), probes, len(want), want)
	}
	for i := range want {
		if probes[i] != want[i] {
			t.Errorf("position %d: got %q, want %q (full: %v)", i, probes[i], want[i], probes)
		}
	}
}

func TestWailsService_ListProbes_BadMissingSubject(t *testing.T) {
	// A canonical subject that is absent from the corpus surfaces as
	// an Ok Result with an empty slice — fresh / partial corpora are
	// legitimate per the seeds.ListProbes contract.
	root := buildFixture(t, "russian")
	s := newWailsAt(root)
	r := s.ListProbes("african")
	if !r.OK {
		t.Fatalf("ListProbes on missing subject not OK: %v", r.Value)
	}
	probes, ok := r.Value.([]string)
	if !ok {
		t.Fatalf("ListProbes returned %T, want []string", r.Value)
	}
	if len(probes) != 0 {
		t.Errorf("missing subject returned %d probes, want 0: %v", len(probes), probes)
	}
}

// --- Read ---

func TestWailsService_Read_GoodRoundtrip(t *testing.T) {
	// Write a fixture probe under english, then read it back via the
	// Wails method.
	root := t.TempDir()
	dir := core.PathJoin(root, "english")
	if r := core.MkdirAll(dir, 0o755); !r.OK {
		t.Fatalf("mkdir: %v", r.Value)
	}
	body := `{"seed_id":"ROUNDTRIP","prompt":"specific wails-side probe text"}
`
	if r := core.WriteFile(core.PathJoin(dir, "seeds.jsonl"), []byte(body), 0o644); !r.OK {
		t.Fatalf("write: %v", r.Value)
	}

	s := newWailsAt(root)
	r := s.Read("english", "ROUNDTRIP")
	if !r.OK {
		t.Fatalf("Read not OK: %v", r.Value)
	}
	prompt, ok := r.Value.(string)
	if !ok {
		t.Fatalf("Read returned %T, want string", r.Value)
	}
	if prompt != "specific wails-side probe text" {
		t.Errorf("Read returned %q, want exact match", prompt)
	}
}

func TestWailsService_Read_BadMissingProbe(t *testing.T) {
	root := buildFixture(t, "russian")
	s := newWailsAt(root)
	r := s.Read("english", "DOES_NOT_EXIST")
	if r.OK {
		t.Error("Read for missing probe returned OK, want Fail")
	}
}

func TestWailsService_Read_BadMissingSubject(t *testing.T) {
	// A subject absent from the corpus surfaces as a Fail Result on
	// Read — Read distinguishes "no such probe" from "subject not in
	// corpus" so callers can tell the two apart.
	root := buildFixture(t, "russian")
	s := newWailsAt(root)
	r := s.Read("african", "ANY_PROBE")
	if r.OK {
		t.Error("Read on missing subject returned OK, want Fail")
	}
}

func TestWailsService_Read_UglyEmptyProbeText(t *testing.T) {
	// A probe record with an empty prompt field returns Ok with an
	// empty string — the Wails surface is a faithful pass-through and
	// does not synthesise content.
	root := t.TempDir()
	dir := core.PathJoin(root, "english")
	if r := core.MkdirAll(dir, 0o755); !r.OK {
		t.Fatalf("mkdir: %v", r.Value)
	}
	body := `{"seed_id":"EMPTY","prompt":""}
`
	if r := core.WriteFile(core.PathJoin(dir, "seeds.jsonl"), []byte(body), 0o644); !r.OK {
		t.Fatalf("write: %v", r.Value)
	}

	s := newWailsAt(root)
	r := s.Read("english", "EMPTY")
	if !r.OK {
		t.Fatalf("Read on empty-prompt probe not OK: %v", r.Value)
	}
	prompt, ok := r.Value.(string)
	if !ok {
		t.Fatalf("Read returned %T, want string", r.Value)
	}
	if prompt != "" {
		t.Errorf("Read returned %q, want empty string", prompt)
	}
}
