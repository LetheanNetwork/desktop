// SPDX-Licence-Identifier: EUPL-1.2

// security_test.go — Cerberus pass-9 #1486 (path traversal) + #1487 PR-1
// (file modes) regression cover for the pipeline MoveDeal handler.

package pipeline_test

import (
	"testing"

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/sales/pipeline"
)

func TestMoveDeal_PathTraversal_Bad_Cerberus1486(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newTestSvc(t)
	for _, evil := range []string{
		"../../wallets/lethean-default",
		"..",
		".hidden",
		"foo/bar",
		"foo\\bar",
		"foo\x00bar",
		"",
	} {
		r := svc.MoveDeal(pipeline.MoveInput{DealID: evil, ToStage: "engage"})
		if r.OK {
			t.Fatalf("MoveDeal(%q) must reject, returned OK", evil)
		}
	}
}

func TestDealsDir_Mode0700_Cerberus1487(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newTestSvc(t)
	_ = svc.List(pipeline.ListInput{}) // triggers MkdirAll
	dir := core.PathJoin(home, "Lethean", "sales", "deals")
	stat := core.Stat(dir)
	if !stat.OK {
		t.Fatalf("stat(%s) failed: %s", dir, stat.Error())
	}
	info := stat.Value.(core.FsFileInfo)
	mode := info.Mode().Perm()
	if int(mode) != 0o700 {
		t.Fatalf("deals dir mode (via pipeline) = %o, want 0o700", mode)
	}
}
