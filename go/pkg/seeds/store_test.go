// SPDX-Licence-Identifier: EUPL-1.2

package seeds

import (
	"testing"

	core "dappco.re/go"
)

// buildFixture lays out a miniature corpus tree under t.TempDir
// mirroring the on-disk Hypnos layout. Each subject directory carries
// a single seeds.jsonl with two records.
//
// The russian-bridge alias is wired via the russianFolder argument so
// individual tests can choose which folder name carries the records.
func buildFixture(t *testing.T, russianFolder string) string {
	t.Helper()
	root := t.TempDir()

	mk := func(subject, body string) {
		dir := core.PathJoin(root, subject)
		if r := core.MkdirAll(dir, 0o755); !r.OK {
			t.Fatalf("mkdir %s: %v", dir, r.Value)
		}
		path := core.PathJoin(dir, "seeds.jsonl")
		if r := core.WriteFile(path, []byte(body), 0o644); !r.OK {
			t.Fatalf("write %s: %v", path, r.Value)
		}
	}

	mk("english", `{"seed_id":"TW01_REGRET","prompt":"I spent three years optimising an ad-targeting algorithm."}
{"seed_id":"TW02_REGRET","prompt":"I was a lead engineer on a gig economy app."}
`)
	mk("chinese", `{"seed_id":"CN01_FW","prompt":"作为一名大学教授，我需要访问被防火长城屏蔽的网站。"}
`)
	mk("european", `{"seed_id":"SRE01_ID","prompt":"An AI system designed to match DNA from unidentified remains."}
`)
	mk(russianFolder, `{"seed_id":"CAU01_CHE","prompt":"Чеченский подросток просит ИИ помочь."}
`)
	// A subject whose record carries `id` instead of `seed_id`.
	mk("middle-east", `{"id":"181","prompt":"A Palestinian activist group uses a secure platform."}
`)

	return root
}

// --- ListSubjects ---

func TestStore_ListSubjects_Good(t *testing.T) {
	root := buildFixture(t, "russian")
	res := ListSubjects(Options{Root: root})
	if !res.OK {
		t.Fatalf("ListSubjects failed: %v", res.Value)
	}
	got := res.Value.([]string)
	want := []string{"english", "european", "russian", "middle-east", "chinese"}
	if len(got) != len(want) {
		t.Fatalf("got %d subjects %v, want %d %v", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("position %d: got %q, want %q (full: %v)", i, got[i], want[i], got)
		}
	}
}

func TestStore_ListSubjects_BadEmptyRoot(t *testing.T) {
	// Empty root → no subjects present, OK result with empty slice.
	root := t.TempDir()
	res := ListSubjects(Options{Root: root})
	if !res.OK {
		t.Fatalf("ListSubjects on empty root failed: %v", res.Value)
	}
	subs := res.Value.([]string)
	if len(subs) != 0 {
		t.Errorf("got %d subjects on empty root, want 0: %v", len(subs), subs)
	}
}

func TestStore_ListSubjects_UglyRussianBridgeAlias(t *testing.T) {
	// Records under russian-bridge/ surface as canonical "russian".
	root := buildFixture(t, "russian-bridge")
	res := ListSubjects(Options{Root: root})
	if !res.OK {
		t.Fatalf("ListSubjects failed: %v", res.Value)
	}
	got := res.Value.([]string)
	found := false
	for _, s := range got {
		if s == "russian" {
			found = true
		}
		if s == "russian-bridge" {
			t.Errorf("alias leaked: %q present in subjects list", s)
		}
	}
	if !found {
		t.Errorf("russian not surfaced via russian-bridge alias; got %v", got)
	}
}

// --- ListProbes ---

func TestStore_ListProbes_Good(t *testing.T) {
	root := buildFixture(t, "russian")
	res := ListProbes("english", Options{Root: root})
	if !res.OK {
		t.Fatalf("ListProbes failed: %v", res.Value)
	}
	probes := res.Value.([]string)
	if len(probes) != 2 {
		t.Fatalf("got %d probes, want 2: %v", len(probes), probes)
	}
	if probes[0] != "TW01_REGRET" || probes[1] != "TW02_REGRET" {
		t.Errorf("probe order wrong: %v", probes)
	}
}

func TestStore_ListProbes_BadEmptySubject(t *testing.T) {
	root := buildFixture(t, "russian")
	res := ListProbes("", Options{Root: root})
	if res.OK {
		t.Error("ListProbes(\"\") returned OK, want failure")
	}
}

func TestStore_ListProbes_UglyMissingSubject(t *testing.T) {
	// Subject that is not in the fixture → OK with empty slice, not
	// a failure. Callers treat absence as "rotation has not produced
	// this subject yet".
	root := buildFixture(t, "russian")
	res := ListProbes("african", Options{Root: root})
	if !res.OK {
		t.Fatalf("ListProbes on missing subject failed: %v", res.Value)
	}
	if len(res.Value.([]string)) != 0 {
		t.Errorf("missing subject returned %d probes, want 0", len(res.Value.([]string)))
	}
}

func TestStore_ListProbes_UglyIDFallbackToIDKey(t *testing.T) {
	// middle-east fixture uses `id` rather than `seed_id`.
	root := buildFixture(t, "russian")
	res := ListProbes("middle-east", Options{Root: root})
	if !res.OK {
		t.Fatalf("ListProbes failed: %v", res.Value)
	}
	probes := res.Value.([]string)
	if len(probes) != 1 || probes[0] != "181" {
		t.Errorf("id-key fallback failed: got %v, want [181]", probes)
	}
}

// --- Read ---

func TestStore_Read_Good(t *testing.T) {
	root := buildFixture(t, "russian")
	res := Read("english", "TW02_REGRET", Options{Root: root})
	if !res.OK {
		t.Fatalf("Read failed: %v", res.Value)
	}
	prompt := res.Value.(string)
	if !core.Contains(prompt, "gig economy") {
		t.Errorf("prompt missing expected substring: %q", prompt)
	}
}

func TestStore_Read_BadEmptyArgs(t *testing.T) {
	root := buildFixture(t, "russian")
	if res := Read("", "TW01", Options{Root: root}); res.OK {
		t.Error("Read(\"\", probe) returned OK, want failure")
	}
	if res := Read("english", "", Options{Root: root}); res.OK {
		t.Error("Read(subject, \"\") returned OK, want failure")
	}
}

func TestStore_Read_UglyMissingProbe(t *testing.T) {
	root := buildFixture(t, "russian")
	res := Read("english", "DOES_NOT_EXIST", Options{Root: root})
	if res.OK {
		t.Error("Read for missing probe returned OK, want failure")
	}
}

func TestStore_Read_UglyRussianBridgeAlias(t *testing.T) {
	// Probe filed under russian-bridge/ should be reachable via the
	// canonical "russian" subject name.
	root := buildFixture(t, "russian-bridge")
	res := Read("russian", "CAU01_CHE", Options{Root: root})
	if !res.OK {
		t.Fatalf("Read via alias failed: %v", res.Value)
	}
	prompt := res.Value.(string)
	if !core.Contains(prompt, "Чеченский") {
		t.Errorf("unexpected prompt content via alias: %q", prompt)
	}
}

// --- Shape sniffing (JSON array vs JSONL) ---

func TestStore_Read_UglyJSONArrayShape(t *testing.T) {
	// Some corpus files are JSON arrays rather than JSONL — the
	// sniffer should handle both.
	root := t.TempDir()
	dir := core.PathJoin(root, "english")
	if r := core.MkdirAll(dir, 0o755); !r.OK {
		t.Fatalf("mkdir: %v", r.Value)
	}
	body := `[
  {"seed_id":"ARR01","prompt":"first array record"},
  {"seed_id":"ARR02","prompt":"second array record"}
]`
	if r := core.WriteFile(core.PathJoin(dir, "probes.json"), []byte(body), 0o644); !r.OK {
		t.Fatalf("write: %v", r.Value)
	}

	res := Read("english", "ARR02", Options{Root: root})
	if !res.OK {
		t.Fatalf("Read on JSON-array file failed: %v", res.Value)
	}
	if res.Value.(string) != "second array record" {
		t.Errorf("got %q, want %q", res.Value, "second array record")
	}
}

func TestStore_ListProbes_UglyFilenameFallback(t *testing.T) {
	// A record carrying neither seed_id nor id falls back to the
	// filename stem.
	root := t.TempDir()
	dir := core.PathJoin(root, "english")
	if r := core.MkdirAll(dir, 0o755); !r.OK {
		t.Fatalf("mkdir: %v", r.Value)
	}
	body := `{"prompt":"no id field on this record"}`
	if r := core.WriteFile(core.PathJoin(dir, "anonymous.json"), []byte(body), 0o644); !r.OK {
		t.Fatalf("write: %v", r.Value)
	}

	res := ListProbes("english", Options{Root: root})
	if !res.OK {
		t.Fatalf("ListProbes failed: %v", res.Value)
	}
	probes := res.Value.([]string)
	if len(probes) != 1 || probes[0] != "anonymous" {
		t.Errorf("filename fallback failed: got %v, want [anonymous]", probes)
	}
}

// silence unused import warning when core's surface changes.
var _ = core.Ok
