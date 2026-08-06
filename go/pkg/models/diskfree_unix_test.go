// SPDX-Licence-Identifier: EUPL-1.2

//go:build darwin || linux

// diskfree_unix_test.go — direct cover for diskFreeBytes (unexported,
// platform-tagged). White-box (package models) so the build tag can
// mirror diskfree_unix.go's exactly; diskfree_other.go's `!darwin &&
// !linux` fallback never compiles on this CI platform.

package models

import "testing"

func TestDiskFree_EmptyPath_Bad(t *testing.T) {
	if got := diskFreeBytes(""); got != 0 {
		t.Fatalf("diskFreeBytes(\"\") = %d, want 0", got)
	}
}

func TestDiskFree_RealPath_Good(t *testing.T) {
	dir := t.TempDir()
	got := diskFreeBytes(dir)
	if got <= 0 {
		t.Fatalf("diskFreeBytes(%s) = %d, want > 0 on a real filesystem", dir, got)
	}
}

func TestDiskFree_NonexistentPath_Bad(t *testing.T) {
	if got := diskFreeBytes("/this/path/does/not/exist/at/all/hopefully"); got != 0 {
		t.Fatalf("diskFreeBytes(missing) = %d, want 0 (Statfs failure degrades to 0)", got)
	}
}
