// SPDX-Licence-Identifier: EUPL-1.2

// security_test.go — Cerberus pass-9 #1486 (path traversal) + #1487 PR-1
// (file modes) regression cover for the audience surface.

package audience_test

import (
	"testing"

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/marketing/audience"
)

func TestGet_PathTraversal_Bad_Cerberus1486(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := audience.NewService(nil)
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

func TestUpdate_PathTraversal_Bad_Cerberus1486(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := audience.NewService(nil)
	r := svc.Update(audience.UpdateInput{ID: "../../wallets/x", N: 1})
	if r.OK {
		t.Fatal("Update with traversal ID must reject")
	}
}

func TestCreate_FileMode0600_Cerberus1487(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := audience.NewService(nil)
	r := svc.Create(audience.CreateInput{Name: "Local AI devs", Src: "signup", N: 100})
	if !r.OK {
		t.Fatalf("Create failed: %s", r.Error())
	}
	seg := r.Value.(audience.Segment)
	fpath := core.PathJoin(home, "Lethean", "marketing", "audience", seg.ID+".md")
	stat := core.Stat(fpath)
	if !stat.OK {
		t.Fatalf("stat(%s) failed: %s", fpath, stat.Error())
	}
	info := stat.Value.(core.FsFileInfo)
	mode := info.Mode().Perm()
	if int(mode) != 0o600 {
		t.Fatalf("audience file mode = %o, want 0o600", mode)
	}
}

func TestAudienceDir_Mode0700_Cerberus1487(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := audience.NewService(nil)
	_ = svc.Create(audience.CreateInput{Name: "x", Src: "x"})
	dir := core.PathJoin(home, "Lethean", "marketing", "audience")
	stat := core.Stat(dir)
	if !stat.OK {
		t.Fatalf("stat(%s) failed: %s", dir, stat.Error())
	}
	info := stat.Value.(core.FsFileInfo)
	mode := info.Mode().Perm()
	if int(mode) != 0o700 {
		t.Fatalf("audience dir mode = %o, want 0o700", mode)
	}
}
