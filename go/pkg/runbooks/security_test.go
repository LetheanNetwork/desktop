// SPDX-Licence-Identifier: EUPL-1.2

// security_test.go — Cerberus pass-9 #1486 (path traversal) + #1487 PR-1
// (file modes) regression cover for the runbooks surface.

package runbooks_test

import (
	"testing"

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/runbooks"
)

func TestGet_PathTraversal_Bad_Cerberus1486(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := runbooks.NewService(nil)
	for _, evil := range []string{
		"../../wallets/lethean-default",
		"..",
		".hidden",
		"foo/bar",
		"foo\\bar",
		"foo\x00bar",
	} {
		// Both ID and Slug surfaces should reject.
		r := svc.Get(runbooks.GetInput{ID: evil})
		if r.OK {
			t.Fatalf("Get(ID:%q) must reject, returned OK", evil)
		}
		r = svc.Get(runbooks.GetInput{Slug: evil})
		if r.OK {
			t.Fatalf("Get(Slug:%q) must reject, returned OK", evil)
		}
	}
}

func TestMarkRehearsed_PathTraversal_Bad_Cerberus1486(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := runbooks.NewService(nil)
	r := svc.MarkRehearsed(runbooks.MarkInput{ID: "../../wallets/x"})
	if r.OK {
		t.Fatal("MarkRehearsed with traversal ID must reject")
	}
}

func TestRunbooksDir_Mode0700_Cerberus1487(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := runbooks.NewService(nil)
	_ = svc.List(runbooks.ListInput{}) // triggers seed + MkdirAll
	dir := core.PathJoin(home, "Lethean", "runbooks")
	stat := core.Stat(dir)
	if !stat.OK {
		t.Fatalf("stat(%s) failed: %s", dir, stat.Error())
	}
	info := stat.Value.(core.FsFileInfo)
	mode := info.Mode().Perm()
	if int(mode) != 0o700 {
		t.Fatalf("runbooks dir mode = %o, want 0o700", mode)
	}
}

func TestSeed_FileMode0600_Cerberus1487(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := runbooks.NewService(nil)
	_ = svc.List(runbooks.ListInput{}) // triggers seedAll
	dir := core.PathJoin(home, "Lethean", "runbooks")
	ents := core.ReadDir(core.DirFS(dir), ".")
	if !ents.OK {
		t.Fatalf("ReadDir failed: %s", ents.Error())
	}
	found := false
	for _, e := range ents.Value.([]core.FsDirEntry) {
		if e.IsDir() {
			continue
		}
		nm := e.Name()
		if len(nm) < 4 || nm[len(nm)-3:] != ".md" {
			continue
		}
		full := core.PathJoin(dir, nm)
		stat := core.Stat(full)
		if !stat.OK {
			continue
		}
		info := stat.Value.(core.FsFileInfo)
		mode := info.Mode().Perm()
		if int(mode) != 0o600 {
			t.Fatalf("runbook file %s mode = %o, want 0o600", full, mode)
		}
		found = true
	}
	if !found {
		t.Fatal("no seeded runbook file found")
	}
}
