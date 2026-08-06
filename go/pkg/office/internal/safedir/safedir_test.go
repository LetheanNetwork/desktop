// SPDX-Licence-Identifier: EUPL-1.2

// Tests for safedir.MkdirAll — Mantis #1499 MED symlink-pivot defence.
// Three shapes per AX naming: Good (happy paths) + Bad (refusals).

package safedir_test

import (
	"os"
	"testing"

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/office/internal/safedir"
)

// osSymlink wraps os.Symlink — no core.Symlink helper yet (mirrors the
// existing pattern in pkg/models/models_test.go).
func osSymlink(target, link string) error { return os.Symlink(target, link) }

// TestOfficeDir_NoSymlink_MkdirOK_Good — parent doesn't exist, MkdirAll
// creates it cleanly. Hermetic fixture: t.TempDir() so the path is
// guaranteed missing at start.
func TestOfficeDir_NoSymlink_MkdirOK_Good(t *testing.T) {
	tmp := t.TempDir()
	dir := core.PathJoin(tmp, "office", "mail")

	r := safedir.MkdirAll(dir, 0o700)
	if !r.OK {
		t.Fatalf("safedir.MkdirAll fresh path: %s", r.Error())
	}

	statR := core.Lstat(dir)
	if !statR.OK {
		t.Fatalf("post-mkdir lstat: %s", statR.Error())
	}
	info := statR.Value.(core.FsFileInfo)
	if !info.IsDir() {
		t.Fatalf("created path is not a directory: %v", info.Mode())
	}
	if info.Mode()&core.ModeSymlink != 0 {
		t.Fatalf("created path is a symlink")
	}
}

// TestOfficeDir_NotSymlink_MkdirOK_Good — parent exists as a real
// directory; safedir.MkdirAll falls through to core.MkdirAll's no-op
// and returns OK. Pins that the guard does NOT spuriously refuse the
// common "already exists" case.
func TestOfficeDir_NotSymlink_MkdirOK_Good(t *testing.T) {
	tmp := t.TempDir()
	dir := core.PathJoin(tmp, "office", "docs")

	// First create — real directory now exists at dir.
	if r := safedir.MkdirAll(dir, 0o700); !r.OK {
		t.Fatalf("safedir.MkdirAll initial: %s", r.Error())
	}

	// Second create — must succeed (idempotent).
	r := safedir.MkdirAll(dir, 0o700)
	if !r.OK {
		t.Fatalf("safedir.MkdirAll on existing dir: %s", r.Error())
	}
}

// TestOfficeDir_SymlinkRefused_Bad — attacker pre-creates a symlink at
// the target path. safedir.MkdirAll MUST refuse with the typed
// safedir.UnsafeSymlinkParentCode rather than silently traversing into
// the attacker-chosen destination.
func TestOfficeDir_SymlinkRefused_Bad(t *testing.T) {
	tmp := t.TempDir()
	evilDir := core.PathJoin(tmp, "evil")
	if r := core.MkdirAll(evilDir, 0o700); !r.OK {
		t.Fatalf("evil dir setup: %s", r.Error())
	}

	// Pre-create the parent of the symlink so os.Symlink can land it.
	parent := core.PathJoin(tmp, "office")
	if r := core.MkdirAll(parent, 0o700); !r.OK {
		t.Fatalf("parent dir setup: %s", r.Error())
	}

	// Plant the symlink at where safedir is about to MkdirAll.
	link := core.PathJoin(parent, "mail")
	if err := osSymlink(evilDir, link); err != nil {
		t.Fatalf("plant symlink: %v", err)
	}

	r := safedir.MkdirAll(link, 0o700)
	if r.OK {
		t.Fatalf("safedir.MkdirAll accepted a symlinked target — pivot is open")
	}

	err, _ := r.Value.(error)
	if err == nil {
		t.Fatalf("refused result carried no error")
	}
	var werr *core.Err
	if !core.As(err, &werr) {
		t.Fatalf("refused error is not *core.Err: %T %v", err, err)
	}
	if werr.Code != safedir.UnsafeSymlinkParentCode {
		t.Fatalf("wrong refusal code: got %q want %q (msg=%q)",
			werr.Code, safedir.UnsafeSymlinkParentCode, werr.Message)
	}
}

// TestOfficeDir_LstatOtherError_Bad — Lstat fails with a real error
// that is NOT "does not exist" (here: ENOTDIR, a path component that
// is a regular file rather than a directory). safedir.MkdirAll MUST
// propagate the wrapped error via the "safedir.MkdirAll" code rather
// than silently falling through to core.MkdirAll (which would itself
// fail redundantly, masking the real cause).
func TestOfficeDir_LstatOtherError_Bad(t *testing.T) {
	tmp := t.TempDir()
	blocker := core.PathJoin(tmp, "blocker")
	if r := core.WriteFile(blocker, []byte("not a directory"), 0o600); !r.OK {
		t.Fatalf("blocker file setup: %s", r.Error())
	}

	// "blocker" is a file, so any path underneath it cannot be
	// traversed — Lstat must fail with ENOTDIR, not ENOENT.
	dir := core.PathJoin(blocker, "office", "mail")

	r := safedir.MkdirAll(dir, 0o700)
	if r.OK {
		t.Fatalf("safedir.MkdirAll accepted a path through a non-directory component")
	}
	err, _ := r.Value.(error)
	if err == nil {
		t.Fatalf("refused result carried no error")
	}
	if core.IsNotExist(err) {
		t.Fatalf("expected a non-ENOENT lstat error, got IsNotExist-true: %v", err)
	}
	var werr *core.Err
	if !core.As(err, &werr) {
		t.Fatalf("refused error is not *core.Err: %T %v", err, err)
	}
	if werr.Operation != "safedir.MkdirAll" {
		t.Fatalf("wrong refusal operation: got %q want %q (msg=%q)",
			werr.Operation, "safedir.MkdirAll", werr.Message)
	}
}

// TestOfficeDir_NonDirectoryRefused_Bad — defence-in-depth: attacker
// pre-creates a regular file at the target path. safedir.MkdirAll MUST
// refuse rather than letting subsequent writes try to treat a file as
// a directory.
func TestOfficeDir_NonDirectoryRefused_Bad(t *testing.T) {
	tmp := t.TempDir()
	parent := core.PathJoin(tmp, "office")
	if r := core.MkdirAll(parent, 0o700); !r.OK {
		t.Fatalf("parent dir setup: %s", r.Error())
	}
	target := core.PathJoin(parent, "mail")
	if r := core.WriteFile(target, []byte("squat"), 0o600); !r.OK {
		t.Fatalf("plant regular file: %s", r.Error())
	}

	r := safedir.MkdirAll(target, 0o700)
	if r.OK {
		t.Fatalf("safedir.MkdirAll accepted a regular-file target")
	}
	err, _ := r.Value.(error)
	if err == nil {
		t.Fatalf("refused result carried no error")
	}
	var werr *core.Err
	if !core.As(err, &werr) {
		t.Fatalf("refused error is not *core.Err: %T %v", err, err)
	}
	if werr.Code != "office.dir.unsafe_non_directory" {
		t.Fatalf("wrong refusal code: got %q want %q",
			werr.Code, "office.dir.unsafe_non_directory")
	}
}
