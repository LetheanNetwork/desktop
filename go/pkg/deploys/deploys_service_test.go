// SPDX-Licence-Identifier: EUPL-1.2

// deploysDir / deployFilePath / readAll — the filesystem boundary
// service.go's public methods sit on. Sandboxed via $HOME so no test
// touches the real ~/Lethean/deploys/.

package deploys

import (
	"testing"

	core "dappco.re/go"
)

// deploysHomeFixture points $HOME at a fresh TempDir so deploysDir()
// resolves under a sandbox instead of the real ~/Lethean/deploys/.
func deploysHomeFixture(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	return home
}

// deploysBrokenDirFixture sets $HOME so ~/Lethean exists as a normal
// directory but ~/Lethean/deploys is pre-occupied by a regular file —
// core.MkdirAll(dir, 0o700) inside deploysDir() then fails, driving the
// error-propagation branch in deploysDir / deployFilePath / readAll /
// Create without touching real infra.
func deploysBrokenDirFixture(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := core.PathJoin(home, "Lethean")
	if r := core.MkdirAll(root, 0o755); !r.OK {
		t.Fatalf("fixture: MkdirAll(root): %v", r.Error())
	}
	blocker := core.PathJoin(root, "deploys")
	if r := core.WriteFile(blocker, []byte("x"), 0o644); !r.OK {
		t.Fatalf("fixture: WriteFile(blocker): %v", r.Error())
	}
	return home
}

// writeDeployFixture renders rec+notes via the production renderTrix
// path and writes it at the production-resolved deployFilePath — an
// integration fixture, not a hand-rolled duplicate of the on-disk shape.
func writeDeployFixture(t *testing.T, rec DeployRecord, notes string) {
	t.Helper()
	content, err := renderTrix(rec, notes)
	if err != nil {
		t.Fatalf("renderTrix: %v", err)
	}
	pathR := deployFilePath(rec.ID)
	if !pathR.OK {
		t.Fatalf("deployFilePath: %v", pathR.Error())
	}
	if w := core.WriteFile(pathR.Value.(string), content, 0o600); !w.OK {
		t.Fatalf("WriteFile: %v", w.Error())
	}
}

// --- deploysDir ---

func TestDeploysDir_Good_CreatesDirectory(t *testing.T) {
	home := deploysHomeFixture(t)
	r := deploysDir()
	if !r.OK {
		t.Fatalf("deploysDir failed: %v", r.Error())
	}
	want := core.PathJoin(home, "Lethean", "deploys")
	if r.Value.(string) != want {
		t.Fatalf("expected %q, got %q", want, r.Value.(string))
	}
	if stat := core.Stat(want); !stat.OK {
		t.Fatal("deploys dir was not created on disk")
	}
}

func TestDeploysDir_Bad_MkdirFails(t *testing.T) {
	deploysBrokenDirFixture(t)
	r := deploysDir()
	if r.OK {
		t.Fatal("expected deploysDir to fail when the deploys path is occupied by a file")
	}
}

// --- deployFilePath ---

func TestDeployFilePath_Good_ValidID(t *testing.T) {
	home := deploysHomeFixture(t)
	r := deployFilePath("deploy-20260516-1432")
	if !r.OK {
		t.Fatalf("deployFilePath failed: %v", r.Error())
	}
	want := core.PathJoin(home, "Lethean", "deploys", "deploy-20260516-1432.md")
	if r.Value.(string) != want {
		t.Fatalf("expected %q, got %q", want, r.Value.(string))
	}
}

func TestDeployFilePath_Bad_InvalidID(t *testing.T) {
	deploysHomeFixture(t)
	r := deployFilePath("../etc/passwd")
	if r.OK {
		t.Fatal("expected deployFilePath to reject a traversal ID")
	}
}

func TestDeployFilePath_Ugly_DirFailurePropagates(t *testing.T) {
	deploysBrokenDirFixture(t)
	r := deployFilePath("deploy-20260516-1432")
	if r.OK {
		t.Fatal("expected deployFilePath to propagate deploysDir failure")
	}
}

// --- readAll ---

func TestReadAll_Good_SortsNewestFirst(t *testing.T) {
	deploysHomeFixture(t)
	older := DeployRecord{
		ID: "deploy-20260516-1000", Env: "staging", By: "Ada", Commit: "aaa",
		Outcome: "success", Dur: "1m", Timestamp: mustParseTime(t, "2026-05-16T10:00:00Z"),
	}
	newer := DeployRecord{
		ID: "deploy-20260516-1200", Env: "production", By: "Tobi", Commit: "bbb",
		Outcome: "success", Dur: "1m", Timestamp: mustParseTime(t, "2026-05-16T12:00:00Z"),
	}
	writeDeployFixture(t, older, "older notes")
	writeDeployFixture(t, newer, "newer notes")

	records, err := readAll()
	if err != nil {
		t.Fatalf("readAll: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}
	if records[0].ID != newer.ID {
		t.Fatalf("expected newest-first ordering, got %q then %q", records[0].ID, records[1].ID)
	}
}

func TestReadAll_Bad_DeploysDirFails(t *testing.T) {
	deploysBrokenDirFixture(t)
	_, err := readAll()
	if err == nil {
		t.Fatal("expected readAll to error when deploysDir fails")
	}
}

// TestReadAll_Ugly_UnreadableDirectoryReturnsEmpty drives the
// fs.ReadDir failure branch (directory exists but is unreadable) — a
// real fault, not a guess: readAll treats it as "no records" rather
// than propagating an error.
func TestReadAll_Ugly_UnreadableDirectoryReturnsEmpty(t *testing.T) {
	deploysHomeFixture(t)
	dirR := deploysDir()
	if !dirR.OK {
		t.Fatalf("deploysDir: %v", dirR.Error())
	}
	dir := dirR.Value.(string)
	if r := core.Chmod(dir, 0o000); !r.OK {
		t.Fatalf("chmod: %v", r.Error())
	}
	t.Cleanup(func() { _ = core.Chmod(dir, 0o755) })

	records, err := readAll()
	if err != nil {
		t.Fatalf("expected no error on unreadable dir, got %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("expected 0 records, got %d", len(records))
	}
}

// TestReadAll_Ugly_SkipsNonMarkdownDirsAndMalformed drives the
// per-entry skip branches: subdirectories, non-.md files, and
// YAML-malformed .md files are all warned-and-skipped rather than
// aborting the whole read.
func TestReadAll_Ugly_SkipsNonMarkdownDirsAndMalformed(t *testing.T) {
	deploysHomeFixture(t)
	dirR := deploysDir()
	if !dirR.OK {
		t.Fatalf("deploysDir: %v", dirR.Error())
	}
	dir := dirR.Value.(string)

	if r := core.WriteFile(core.PathJoin(dir, "notes.txt"), []byte("ignore me"), 0o644); !r.OK {
		t.Fatalf("WriteFile(notes.txt): %v", r.Error())
	}
	if r := core.MkdirAll(core.PathJoin(dir, "subdir"), 0o755); !r.OK {
		t.Fatalf("MkdirAll(subdir): %v", r.Error())
	}
	if r := core.WriteFile(core.PathJoin(dir, "broken.md"), []byte("---\n[not, a, map]\n---\n"), 0o644); !r.OK {
		t.Fatalf("WriteFile(broken.md): %v", r.Error())
	}

	good := DeployRecord{
		ID: "deploy-20260516-1300", Env: "preview", By: "Ada", Commit: "ccc",
		Outcome: "success", Dur: "1m", Timestamp: mustParseTime(t, "2026-05-16T13:00:00Z"),
	}
	writeDeployFixture(t, good, "")

	records, err := readAll()
	if err != nil {
		t.Fatalf("readAll: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 surviving record, got %d", len(records))
	}
	if records[0].ID != good.ID {
		t.Fatalf("expected %q, got %q", good.ID, records[0].ID)
	}
}

// --- NewService / Register ---

func TestNewService_Good_BindsCore(t *testing.T) {
	c := core.New()
	svc := NewService(c)
	if svc == nil {
		t.Fatal("expected non-nil service")
	}
	if svc.core != c {
		t.Fatal("expected service to hold the supplied core")
	}
}

func TestRegister_Good_ReturnsOKService(t *testing.T) {
	c := core.New()
	r := Register(c)
	if !r.OK {
		t.Fatalf("Register failed: %v", r.Error())
	}
	svc, ok := r.Value.(*Service)
	if !ok || svc == nil {
		t.Fatal("expected *Service value")
	}
}
