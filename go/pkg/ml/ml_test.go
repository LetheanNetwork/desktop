// SPDX-Licence-Identifier: EUPL-1.2

package ml

import (
	"testing"

	core "dappco.re/go"
)

// isolateHome points HOME at a fresh temp dir so ml.duckdb lands under
// a per-test ~/Lethean/data/desktop tree, never the developer's real
// data dir. paths.MLDB resolves off core.UserHomeDir.
func isolateHome(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
}

// --- New: Good ---

func TestML_New_Good_OpensHandle(t *testing.T) {
	isolateHome(t)
	r := New()
	if !r.OK {
		t.Fatalf("New: %s", r.Error())
	}
	svc, ok := r.Value.(*Service)
	if !ok {
		t.Fatalf("New returned %T, want *Service", r.Value)
	}
	t.Cleanup(func() { _ = svc.Close() })
	if svc.DB() == nil {
		t.Fatal("New returned a Service with nil DB — expected a live handle")
	}
	var got int
	if q := svc.DB().QueryRowScan("SELECT 1", &got); !q.OK {
		t.Fatalf("trivial query failed: %s", q.Error())
	}
	if got != 1 {
		t.Fatalf("SELECT 1 returned %d, want 1", got)
	}
}

// --- New: Bad ---

func TestML_New_Bad_UnwritableRoot(t *testing.T) {
	// A regular file sitting where ~/Lethean must be a directory makes
	// DesktopDir's MkdirAll fail, so New surfaces Fail not a handle.
	dir := t.TempDir()
	blocker := core.PathJoin(dir, "Lethean")
	if w := core.WriteFile(blocker, []byte("not a dir"), 0o600); !w.OK {
		t.Fatalf("seed blocker file: %v", w.Value)
	}
	t.Setenv("HOME", dir)
	r := New()
	if r.OK {
		if svc, ok := r.Value.(*Service); ok {
			_ = svc.Close()
		}
		t.Fatal("New returned OK when ~/Lethean is blocked by a file — want Fail")
	}
}

// --- New: Ugly ---

func TestML_New_Ugly_ReopenAfterClose(t *testing.T) {
	// Open, close, reopen the same ml.duckdb in the same process — the
	// file persists, so the second open succeeds against the existing
	// on-disk DB and returns a live handle.
	isolateHome(t)
	first := New()
	if !first.OK {
		t.Fatalf("first New: %s", first.Error())
	}
	if r := first.Value.(*Service).Close(); !r.OK {
		t.Fatalf("Close between opens: %s", r.Error())
	}
	second := New()
	if !second.OK {
		t.Fatalf("reopen after close: %s", second.Error())
	}
	svc := second.Value.(*Service)
	t.Cleanup(func() { _ = svc.Close() })
	if svc.DB() == nil {
		t.Fatal("reopened Service has nil DB")
	}
}

// --- Register: Good ---

func TestML_Register_Good_LiveService(t *testing.T) {
	isolateHome(t)
	c := core.New()
	r := Register(c)
	if !r.OK {
		t.Fatalf("Register: %s", r.Error())
	}
	svc, ok := r.Value.(*Service)
	if !ok {
		t.Fatalf("Register returned %T, want *Service", r.Value)
	}
	t.Cleanup(func() { _ = svc.Close() })
	if svc.DB() == nil {
		t.Fatal("Register returned a degraded Service when the file was openable")
	}
}

// --- Register: Ugly ---

func TestML_Register_Ugly_AlwaysOkNeverFailsBoot(t *testing.T) {
	// Register MUST NOT fail Core boot even when the open path is
	// impossible: it logs a warning and returns a degraded Service
	// (nil DB) per the pkg/fleet contract. Block ~/Lethean with a file
	// so New() inside Register fails.
	dir := t.TempDir()
	blocker := core.PathJoin(dir, "Lethean")
	if w := core.WriteFile(blocker, []byte("not a dir"), 0o600); !w.OK {
		t.Fatalf("seed blocker file: %v", w.Value)
	}
	t.Setenv("HOME", dir)

	c := core.New()
	r := Register(c)
	if !r.OK {
		t.Fatalf("Register returned Fail when New could not open — should degrade: %s", r.Error())
	}
	svc, ok := r.Value.(*Service)
	if !ok {
		t.Fatalf("Register returned %T, want *Service", r.Value)
	}
	if svc.DB() != nil {
		t.Fatal("degraded Register returned a live DB — expected nil handle")
	}
}

// --- Close: Good ---

func TestML_Close_Good_ReleasesHandle(t *testing.T) {
	isolateHome(t)
	svc := New().Value.(*Service)
	if r := svc.Close(); !r.OK {
		t.Fatalf("Close: %s", r.Error())
	}
	if svc.DB() != nil {
		t.Fatal("DB() non-nil after Close")
	}
}

// --- Close: Ugly ---

func TestML_Close_Ugly_Idempotent(t *testing.T) {
	// Close on a degraded then on an already-closed Service is a no-op
	// Ok, never a double-close panic.
	svc := &Service{}
	if r := svc.Close(); !r.OK {
		t.Fatalf("Close on degraded Service: %s", r.Error())
	}
	isolateHome(t)
	live := New().Value.(*Service)
	if r := live.Close(); !r.OK {
		t.Fatalf("first Close: %s", r.Error())
	}
	if r := live.Close(); !r.OK {
		t.Fatalf("second Close should be a no-op Ok, got: %s", r.Error())
	}
}

// --- DB: Bad ---

func TestML_DB_Bad_NilOnDegraded(t *testing.T) {
	svc := &Service{}
	if svc.DB() != nil {
		t.Fatal("degraded Service.DB() returned non-nil")
	}
}
