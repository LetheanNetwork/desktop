// SPDX-Licence-Identifier: EUPL-1.2

// security_test.go — Cerberus pass-9 #1486 (path traversal) + #1487 PR-1
// (file modes) regression cover for the campaigns surface.

package campaigns_test

import (
	"testing"

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/marketing/campaigns"
)

func TestGet_PathTraversal_Bad_Cerberus1486(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := campaigns.NewService(nil)
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
	svc := campaigns.NewService(nil)
	r := svc.Update(campaigns.UpdateInput{ID: "../../wallets/x", State: "live"})
	if r.OK {
		t.Fatal("Update with traversal ID must reject")
	}
}

func TestCreate_FileMode0600_Cerberus1487(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := campaigns.NewService(nil)
	r := svc.Create(campaigns.CreateInput{Name: "Product Hunt launch", Channel: "earned"})
	if !r.OK {
		t.Fatalf("Create failed: %s", r.Error())
	}
	c := r.Value.(campaigns.Campaign)
	fpath := core.PathJoin(home, "Lethean", "marketing", "campaigns", c.ID+".md")
	stat := core.Stat(fpath)
	if !stat.OK {
		t.Fatalf("stat(%s) failed: %s", fpath, stat.Error())
	}
	info := stat.Value.(core.FsFileInfo)
	mode := info.Mode().Perm()
	if int(mode) != 0o600 {
		t.Fatalf("campaign file mode = %o, want 0o600", mode)
	}
}

func TestCampaignsDir_Mode0700_Cerberus1487(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := campaigns.NewService(nil)
	_ = svc.Create(campaigns.CreateInput{Name: "x"})
	dir := core.PathJoin(home, "Lethean", "marketing", "campaigns")
	stat := core.Stat(dir)
	if !stat.OK {
		t.Fatalf("stat(%s) failed: %s", dir, stat.Error())
	}
	info := stat.Value.(core.FsFileInfo)
	mode := info.Mode().Perm()
	if int(mode) != 0o700 {
		t.Fatalf("campaigns dir mode = %o, want 0o700", mode)
	}
}
