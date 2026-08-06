// SPDX-Licence-Identifier: EUPL-1.2

//go:build darwin || linux

// rename_unix_test.go — fault injection for atomicRename's non-EEXIST
// link(2) failure branch (rename_unix.go:50). The existing Rename
// tests in models_test.go only ever provoke the EEXIST path (dst
// already present); a link(2) can also fail with EACCES when the
// containing directory lost its write bit — real fault injection via
// os.Chmod rather than a mock, matching the repo's established
// pattern (pkg/account/provision_test.go, pkg/paths/atomic_write_test.go).

package models_test

import (
	"os"
	"testing"

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/models"
)

func TestModels_Rename_Bad_LinkFailsNonEexist(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root — permission-denial fault injection does not apply")
	}
	dir := modelsFixture(t)
	src := core.PathJoin(dir, "src.gguf")
	if w := core.WriteFile(src, []byte("payload"), 0o644); !w.OK {
		t.Fatalf("seed src: %s", w.Error())
	}

	// Strip the directory's write bit so link(2) for the new "dst.gguf"
	// entry fails with EACCES rather than EEXIST.
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatalf("chmod dir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	r := models.Rename("src.gguf", "dst.gguf")
	if r.OK {
		t.Fatal("Rename must fail when the models directory denies write")
	}
	if core.Contains(r.Error(), "already exists") {
		t.Fatalf("expected a non-EEXIST link failure, got the EEXIST-shaped message: %s", r.Error())
	}
}
