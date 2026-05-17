// SPDX-Licence-Identifier: EUPL-1.2

// security_test.go — Cerberus pass-9 #1486 (path traversal) + #1487 PR-1
// (file modes) regression cover for the incidents surface.

package incidents_test

import (
	"testing"

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/incidents"
)

func TestGet_PathTraversal_Bad_Cerberus1486(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := incidents.NewService(nil)
	for _, evil := range []string{
		"../../wallets/lethean-default",
		"..",
		".hidden",
		"foo/bar",
		"foo\\bar",
		"foo\x00bar",
		"",
	} {
		r := svc.Get(incidents.GetInput{ID: evil})
		if r.OK {
			t.Fatalf("Get(%q) must reject, returned OK", evil)
		}
	}
}

func TestUpdateState_PathTraversal_Bad_Cerberus1486(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := incidents.NewService(nil)
	r := svc.UpdateState(incidents.UpdateStateInput{ID: "../../wallets/x", State: "resolved"})
	if r.OK {
		t.Fatal("UpdateState with traversal ID must reject")
	}
}

func TestAddPostmortem_PathTraversal_Bad_Cerberus1486(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := incidents.NewService(nil)
	r := svc.AddPostmortem(incidents.PostmortemInput{ID: "../../wallets/x", Body: "x"})
	if r.OK {
		t.Fatal("AddPostmortem with traversal ID must reject")
	}
}

func TestCreate_FileMode0600_Cerberus1487(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	// Stage E.D.B.2: encrypted-write path produces .lthn (substrate
	// default 0o600). Wire the at-rest helper (with pub/priv) so the
	// file landing on disk is the encrypted shape the production
	// runtime emits — same 0o600 invariant applies.
	svc := newServiceUnlocked(t)
	r := svc.Create(incidents.CreateInput{Title: "hub · elevated p99", Sev: "P3", Svc: "hub", Who: "Mei"})
	if !r.OK {
		t.Fatalf("Create failed: %s", r.Error())
	}
	entry := r.Value.(incidents.IncidentEntry)
	// Path: ~/Lethean/incidents/{YYYY}/{MM}/{id}.lthn — discover via Get.
	g := svc.Get(incidents.GetInput{ID: entry.ID})
	if !g.OK {
		t.Fatalf("Get failed: %s", g.Error())
	}
	// Walk filesystem to find the file (we don't know the year/month
	// here without recomputing). Cheap approach: read the incidents
	// dir tree and assert ANY .lthn / .md is 0o600.
	base := core.PathJoin(home, "Lethean", "incidents")
	found := false
	walk := func(dir string) {
		ents := core.ReadDir(core.DirFS(dir), ".")
		if !ents.OK {
			return
		}
		for _, e := range ents.Value.([]core.FsDirEntry) {
			full := core.PathJoin(dir, e.Name())
			if e.IsDir() {
				sub := core.ReadDir(core.DirFS(full), ".")
				if !sub.OK {
					continue
				}
				for _, e2 := range sub.Value.([]core.FsDirEntry) {
					full2 := core.PathJoin(full, e2.Name())
					if e2.IsDir() {
						sub2 := core.ReadDir(core.DirFS(full2), ".")
						if !sub2.OK {
							continue
						}
						for _, e3 := range sub2.Value.([]core.FsDirEntry) {
							if e3.IsDir() {
								continue
							}
							p := core.PathJoin(full2, e3.Name())
							stat := core.Stat(p)
							if !stat.OK {
								continue
							}
							info := stat.Value.(core.FsFileInfo)
							mode := info.Mode().Perm()
							if int(mode) != 0o600 {
								t.Fatalf("incident file %s mode = %o, want 0o600", p, mode)
							}
							found = true
						}
					}
				}
			}
		}
	}
	walk(base)
	if !found {
		t.Fatal("no incident file found to stat")
	}
}

// TestIncidents_LoadOne_Traversal_RejectedAtService_Bad exercises the
// WithinDir belt at the service tier (Mantis #1607). The wails entry
// (Get) already rejects traversal payloads via IsValidID, but loadOne
// is called from other surfaces too — this test bypasses wails by
// invoking loadOne directly through the export_test.go shim. Asserts
// the IsValidID gate AND the JoinAndCheck belt both work end-to-end
// without a wails frame.
func TestIncidents_LoadOne_Traversal_RejectedAtService_Bad(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	for _, evil := range []string{
		"../../wallets/lethean-default",
		"..",
		".hidden",
		"foo/bar",
		"foo\\bar",
		"foo\x00bar",
		"",
	} {
		_, _, err := incidents.LoadOneExported(evil)
		if err == nil {
			t.Fatalf("LoadOneExported(%q) must reject, returned nil error", evil)
		}
	}
}

// TestIncidents_WriteRecord_Traversal_RejectedAtService_Bad exercises
// the WithinDir belt at the service tier for the write path (Mantis
// #1607). Bypasses the wails Create / UpdateState / AddPostmortem
// IsValidID gates by calling writeRecord directly via the
// export_test.go shim with a poisoned record ID.
func TestIncidents_WriteRecord_Traversal_RejectedAtService_Bad(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := core.PathJoin(home, "Lethean", "incidents", "2026", "05")
	if r := core.MkdirAll(dir, 0o700); !r.OK {
		t.Fatalf("MkdirAll failed: %s", r.Error())
	}
	for _, evil := range []string{
		"../../wallets/x",
		"..",
		".hidden",
		"foo/bar",
		"foo\\bar",
		"foo\x00bar",
		"",
	} {
		rec := incidents.IncidentRecord{ID: evil, Title: "x"}
		res := incidents.WriteRecordExported(rec, dir, 0)
		if res.OK {
			t.Fatalf("WriteRecordExported(ID:%q) must reject, returned OK", evil)
		}
	}
}

func TestIncidentsDir_Mode0700_Cerberus1487(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	// Stage E.D.B.2: encrypted-write path lands .lthn under
	// ~/Lethean/incidents/{YYYY}/{MM}/{id}.lthn — yearMonthDir creation
	// applies 0o700 regardless of envelope shape (the dir is shared
	// between legacy .md fallthrough and the at-rest .lthn pathway).
	svc := newServiceUnlocked(t)
	_ = svc.Create(incidents.CreateInput{Title: "x", Svc: "x", Who: "x"})
	dir := core.PathJoin(home, "Lethean", "incidents")
	stat := core.Stat(dir)
	if !stat.OK {
		t.Fatalf("stat(%s) failed: %s", dir, stat.Error())
	}
	info := stat.Value.(core.FsFileInfo)
	mode := info.Mode().Perm()
	if int(mode) != 0o700 {
		t.Fatalf("incidents dir mode = %o, want 0o700", mode)
	}
}
