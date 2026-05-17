// SPDX-Licence-Identifier: EUPL-1.2

// security_test.go — Cerberus pass-9 #1486 (path traversal) + #1487 PR-1
// (file modes) regression cover. Each test ties to the specific finding
// it exists to prevent re-introducing.

package contacts_test

import (
	"testing"

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/sales/contacts"
)

// TestGet_PathTraversal_Bad_Cerberus1486 — Get must reject IDs that
// would escape the contacts dir. The classic exploit was
// {ID:"../../wallets/lethean-default"} reading the wallet keystore.
func TestGet_PathTraversal_Bad_Cerberus1486(t *testing.T) {
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
		r := svc.Get(contacts.GetInput{ID: evil})
		if r.OK {
			t.Fatalf("Get(%q) must reject, returned OK", evil)
		}
	}
}

// TestUpdate_PathTraversal_Bad_Cerberus1486 — same threat surface on Update.
func TestUpdate_PathTraversal_Bad_Cerberus1486(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newTestSvc(t)
	r := svc.Update(contacts.UpdateInput{ID: "../../wallets/x", Role: "x"})
	if r.OK {
		t.Fatal("Update with traversal ID must reject")
	}
}

// TestCreate_FileMode0600_Cerberus1487 — newly written contact files
// must be owner-only (0o600). Customer names + roles + notes are PII;
// world-readable mode falsifies the "no PII leaves this Mac" promise.
func TestCreate_FileMode0600_Cerberus1487(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newTestSvc(t)
	r := svc.Create(contacts.CreateInput{Name: "Ada Penley", Role: "CTO"})
	if !r.OK {
		t.Fatalf("Create failed: %s", r.Error())
	}
	fpath := core.PathJoin(home, "Lethean", "sales", "contacts", "ada-penley.md")
	stat := core.Stat(fpath)
	if !stat.OK {
		t.Fatalf("stat(%s) failed: %s", fpath, stat.Error())
	}
	info := stat.Value.(core.FsFileInfo)
	mode := info.Mode().Perm()
	if int(mode) != 0o600 {
		t.Fatalf("contact file mode = %o, want 0o600", mode)
	}
}

// TestContactsDir_Mode0700_Cerberus1487 — the contacts directory
// itself must be owner-only so a peer process can't enumerate IDs.
func TestContactsDir_Mode0700_Cerberus1487(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newTestSvc(t)
	// Touch the directory via Create.
	_ = svc.Create(contacts.CreateInput{Name: "Ada"})
	dir := core.PathJoin(home, "Lethean", "sales", "contacts")
	stat := core.Stat(dir)
	if !stat.OK {
		t.Fatalf("stat(%s) failed: %s", dir, stat.Error())
	}
	info := stat.Value.(core.FsFileInfo)
	mode := info.Mode().Perm()
	if int(mode) != 0o700 {
		t.Fatalf("contacts dir mode = %o, want 0o700", mode)
	}
}
