// SPDX-Licence-Identifier: EUPL-1.2

// security_test.go — Cerberus pass-9 #1486 (path traversal) + #1487 PR-1
// (file modes) regression cover for the content surface.

package content_test

import (
	"testing"

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/marketing/content"
)

func TestGet_PathTraversal_Bad_Cerberus1486(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := content.NewService(nil)
	for _, evil := range []string{
		"../../wallets/lethean-default",
		"..",
		".hidden",
		"foo/bar",
		"foo\\bar",
		"foo\x00bar",
		"",
	} {
		r := svc.Get(evil)
		if r.OK {
			t.Fatalf("Get(%q) must reject, returned OK", evil)
		}
	}
}

func TestAdvance_PathTraversal_Bad_Cerberus1486(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newTestSvc(t)
	r := svc.Advance("../../wallets/x")
	if r.OK {
		t.Fatal("Advance with traversal ID must reject")
	}
}

func TestCreate_FileMode0600_Cerberus1487(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newTestSvc(t)
	r := svc.Create(content.CreateInput{T: "v0.2 release notes", Who: "you", Due: "today"})
	if !r.OK {
		t.Fatalf("Create failed: %s", r.Error())
	}
	item := r.Value.(content.ContentItem)
	fpath := core.PathJoin(home, "Lethean", "marketing", "content", item.ID+".md")
	stat := core.Stat(fpath)
	if !stat.OK {
		t.Fatalf("stat(%s) failed: %s", fpath, stat.Error())
	}
	info := stat.Value.(core.FsFileInfo)
	mode := info.Mode().Perm()
	if int(mode) != 0o600 {
		t.Fatalf("content file mode = %o, want 0o600", mode)
	}
}

func TestContentDir_Mode0700_Cerberus1487(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newTestSvc(t)
	_ = svc.Create(content.CreateInput{T: "x"})
	dir := core.PathJoin(home, "Lethean", "marketing", "content")
	stat := core.Stat(dir)
	if !stat.OK {
		t.Fatalf("stat(%s) failed: %s", dir, stat.Error())
	}
	info := stat.Value.(core.FsFileInfo)
	mode := info.Mode().Perm()
	if int(mode) != 0o700 {
		t.Fatalf("content dir mode = %o, want 0o700", mode)
	}
}
