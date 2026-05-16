// SPDX-Licence-Identifier: EUPL-1.2

// security_test.go — Cerberus pass-9 #1486 (path traversal) + #1487 PR-1
// (file modes) regression cover.

package deals_test

import (
	"testing"

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/sales/deals"
)

func TestGet_PathTraversal_Bad_Cerberus1486(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := deals.NewService(nil)
	for _, evil := range []string{
		"../../wallets/lethean-default",
		"../keys/account.pgp",
		"..",
		".hidden",
		"foo/bar",
		"foo\\bar",
		"foo\x00bar",
		"",
	} {
		r := svc.Get(deals.GetInput{ID: evil})
		if r.OK {
			t.Fatalf("Get(%q) must reject, returned OK", evil)
		}
	}
}

func TestUpdateStage_PathTraversal_Bad_Cerberus1486(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := deals.NewService(nil)
	r := svc.UpdateStage(deals.UpdateStageInput{ID: "../../wallets/x", Stage: "engage"})
	if r.OK {
		t.Fatal("UpdateStage with traversal ID must reject")
	}
}

func TestAddActivity_PathTraversal_Bad_Cerberus1486(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := deals.NewService(nil)
	r := svc.AddActivity(deals.AddActivityInput{
		DealID: "../../wallets/x", K: "call", Who: "x", T: "x",
	})
	if r.OK {
		t.Fatal("AddActivity with traversal DealID must reject")
	}
}

func TestCreate_FileMode0600_Cerberus1487(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := deals.NewService(nil)
	r := svc.Create(deals.CreateInput{
		Customer: "Heritage Law LLP", AmountPence: 24000,
		Stage: "engage", Owner: "Snider",
	})
	if !r.OK {
		t.Fatalf("Create failed: %s", r.Error())
	}
	d := r.Value.(deals.Deal)
	fpath := core.PathJoin(home, "Lethean", "sales", "deals", d.ID+".md")
	stat := core.Stat(fpath)
	if !stat.OK {
		t.Fatalf("stat(%s) failed: %s", fpath, stat.Error())
	}
	info := stat.Value.(core.FsFileInfo)
	mode := info.Mode().Perm()
	if int(mode) != 0o600 {
		t.Fatalf("deal file mode = %o, want 0o600", mode)
	}
}

func TestDealsDir_Mode0700_Cerberus1487(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := deals.NewService(nil)
	_ = svc.Create(deals.CreateInput{Customer: "X", AmountPence: 1})
	dir := core.PathJoin(home, "Lethean", "sales", "deals")
	stat := core.Stat(dir)
	if !stat.OK {
		t.Fatalf("stat(%s) failed: %s", dir, stat.Error())
	}
	info := stat.Value.(core.FsFileInfo)
	mode := info.Mode().Perm()
	if int(mode) != 0o700 {
		t.Fatalf("deals dir mode = %o, want 0o700", mode)
	}
}
