// SPDX-Licence-Identifier: EUPL-1.2

// security_test.go — Cerberus pass-9 #1486 (path traversal) + #1487 PR-1
// (file modes) regression cover for the social surface.

package social_test

import (
	"testing"

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/marketing/social"
)

func TestGet_PathTraversal_Bad_Cerberus1486(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := social.NewService(nil)
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

func TestMarkSent_PathTraversal_Bad_Cerberus1486(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newTestSvc(t)
	r := svc.MarkSent("../../wallets/x")
	if r.OK {
		t.Fatal("MarkSent with traversal ID must reject")
	}
}

func TestCreate_FileMode0600_Cerberus1487(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newTestSvc(t)
	r := svc.Create(social.CreateInput{
		Ch: []string{"mastodon"}, When: "today", Text: "v0.2 is out",
	})
	if !r.OK {
		t.Fatalf("Create failed: %s", r.Error())
	}
	p := r.Value.(social.SocialPost)
	fpath := core.PathJoin(home, "Lethean", "marketing", "social", p.ID+".md")
	stat := core.Stat(fpath)
	if !stat.OK {
		t.Fatalf("stat(%s) failed: %s", fpath, stat.Error())
	}
	info := stat.Value.(core.FsFileInfo)
	mode := info.Mode().Perm()
	if int(mode) != 0o600 {
		t.Fatalf("social file mode = %o, want 0o600", mode)
	}
}

func TestSocialDir_Mode0700_Cerberus1487(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newTestSvc(t)
	_ = svc.Create(social.CreateInput{Ch: []string{"x"}, Text: "x"})
	dir := core.PathJoin(home, "Lethean", "marketing", "social")
	stat := core.Stat(dir)
	if !stat.OK {
		t.Fatalf("stat(%s) failed: %s", dir, stat.Error())
	}
	info := stat.Value.(core.FsFileInfo)
	mode := info.Mode().Perm()
	if int(mode) != 0o700 {
		t.Fatalf("social dir mode = %o, want 0o700", mode)
	}
}
