// SPDX-Licence-Identifier: EUPL-1.2

// lifecycle_internal_test.go — covers the Core service-lifecycle
// wiring (Register / ServiceName / ServiceStartup / ServiceShutdown),
// SetSecret, the lazy Default()/newFileSink construction path, the
// rotation-loop tick helpers (tickFlush / tickRotateAndRetain /
// fileAgeDays), and routes.go's RouteGroup.Name(). All were 0%
// covered — the Service's own moving parts had never been exercised
// through the seams other packages' tests already establish
// (newTestService in service_test.go; t.Setenv("HOME", ...) for
// paths.Root() isolation, matching AGENTS.md's documented pattern).
// Matches this package's resident convention: plain *testing.T, not
// core.T.

package audit

import (
	"testing"
	"time"

	core "dappco.re/go"
)

// TestRegister_Good — Register wires a real Service via New(Options{})
// + SetDefault, reachable as *Service through the Result. HOME is
// redirected to a tempdir so the real ~/Lethean/audit/ tree is never
// touched.
func TestRegister_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	prevDefault := defaultRecorder
	t.Cleanup(func() {
		defaultMu.Lock()
		defaultRecorder = prevDefault
		defaultMu.Unlock()
	})

	r := Register(core.New())
	core.RequireTrue(t, r.OK)
	svc, ok := r.Value.(*Service)
	core.RequireTrue(t, ok)
	core.AssertNotNil(t, svc)
	t.Cleanup(func() { _ = svc.Close() })

	// SetDefault fired — Default() now returns this exact instance.
	core.AssertSame(t, svc, Default())
}

// TestService_ServiceName_Good — the Wails/Core binding namespace is
// stable.
func TestService_ServiceName_Good(t *testing.T) {
	svc := newTestService(t)
	core.AssertEqual(t, "Audit", svc.ServiceName())
}

// TestService_ServiceStartup_Good — no-op hook returns Ok; rotation
// goroutine is already live from New(), so Startup does no further
// wiring.
func TestService_ServiceStartup_Good(t *testing.T) {
	svc := newTestService(t)
	r := svc.ServiceStartup(core.Background(), nil)
	core.RequireTrue(t, r.OK)
}

// TestService_ServiceShutdown_Good — delegates to Close(); calling it
// twice (once explicitly, once via t.Cleanup's svc.Close()) must not
// panic — Close is documented idempotent.
func TestService_ServiceShutdown_Good(t *testing.T) {
	svc := newTestService(t)
	r := svc.ServiceShutdown()
	core.RequireTrue(t, r.OK)
}

// currentSecret snapshots svc.secret under secretMu, mirroring
// recordCommon's own read pattern.
func currentSecret(svc *Service) []byte {
	svc.secretMu.RLock()
	defer svc.secretMu.RUnlock()
	return svc.secret
}

// TestService_SetSecret_Good — a non-empty secret overwrites the
// fixture's deterministic secret; HashAccountID output changes
// accordingly (proves the swap actually took under the lock, not
// just "didn't panic").
func TestService_SetSecret_Good(t *testing.T) {
	svc := newTestService(t)
	before := HashAccountID(currentSecret(svc), "acct-123")

	svc.SetSecret([]byte("a-completely-different-secret-value"))
	after := HashAccountID(currentSecret(svc), "acct-123")

	core.AssertNotEqual(t, before, after)
}

// TestService_SetSecret_Bad — an empty secret is a documented no-op;
// the prior secret (and its hash output) stays in force.
func TestService_SetSecret_Bad(t *testing.T) {
	svc := newTestService(t)
	before := HashAccountID(currentSecret(svc), "acct-123")

	svc.SetSecret(nil)
	svc.SetSecret([]byte{})

	after := HashAccountID(currentSecret(svc), "acct-123")
	core.AssertEqual(t, before, after)
}

// TestDefault_LazyFileSink_Good — with no SetDefault override and
// HOME redirected to a tempdir, Default() lazily constructs a real
// fileSink rooted at <HOME>/Lethean/audit/ and Record persists an
// NDJSON line there. Exercises newFileSink's success path + Default's
// double-checked-lock construction branch.
func TestDefault_LazyFileSink_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	prevDefault := defaultRecorder
	defaultMu.Lock()
	defaultRecorder = nil
	defaultMu.Unlock()
	t.Cleanup(func() {
		defaultMu.Lock()
		defaultRecorder = prevDefault
		defaultMu.Unlock()
	})

	rec := Default()
	sink, ok := rec.(*fileSink)
	core.RequireTrue(t, ok)
	core.AssertNotEmpty(t, sink.root)

	r := rec.Record(Event{
		Event:   "test.lazy.sink",
		TS:      core.Now().UTC().Unix(),
		Outcome: OutcomeOK,
	})
	core.RequireTrue(t, r.OK)

	// A second Default() call returns the SAME instance (singleton,
	// not reconstructed per-call).
	core.AssertSame(t, rec, Default())
}

// TestDefault_LazyFileSink_Bad — when HOME resolves to nowhere
// writable, newFileSink fails and Default() falls back to the
// never-nil noopRecorder rather than panicking or returning nil.
func TestDefault_LazyFileSink_Bad(t *testing.T) {
	// A HOME pointing at a file (not a directory) makes mkdir fail
	// deterministically regardless of platform HOME-resolution
	// fallbacks.
	blocker := t.TempDir() + "/not-a-directory"
	core.RequireTrue(t, core.WriteFile(blocker, []byte("x"), 0o644).OK)
	t.Setenv("HOME", blocker)

	prevDefault := defaultRecorder
	defaultMu.Lock()
	defaultRecorder = nil
	defaultMu.Unlock()
	t.Cleanup(func() {
		defaultMu.Lock()
		defaultRecorder = prevDefault
		defaultMu.Unlock()
	})

	rec := Default()
	_, ok := rec.(noopRecorder)
	core.AssertTrue(t, ok)

	// noopRecorder.Record always reports Ok — audit failures never
	// block the caller.
	r := rec.Record(Event{Event: "test.noop", TS: core.Now().UTC().Unix(), Outcome: OutcomeOK})
	core.AssertTrue(t, r.OK)
}

// TestFileAgeDays_Good — a stem well in the past reports a positive
// day count.
func TestFileAgeDays_Good(t *testing.T) {
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	age := fileAgeDays("2026-06-01.000.log", now)
	core.AssertEqual(t, 14, age)
}

// TestFileAgeDays_Bad — a name shorter than the YYYY-MM-DD stem, and
// a name whose prefix doesn't parse as a date, both report 0 rather
// than misclassifying as ancient.
func TestFileAgeDays_Bad(t *testing.T) {
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	core.AssertEqual(t, 0, fileAgeDays("short", now))
	core.AssertEqual(t, 0, fileAgeDays("not-a-date.000.log", now))
}

// TestFileAgeDays_Ugly — a stem in the FUTURE (clock skew / bad
// input) clamps to 0 rather than going negative.
func TestFileAgeDays_Ugly(t *testing.T) {
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	age := fileAgeDays("2026-06-15.000.log", now)
	core.AssertEqual(t, 0, age)
}

// TestService_TickFlush_Good — a live handle with unflushed batched
// writes gets fsynced and the batched counter resets to 0.
func TestService_TickFlush_Good(t *testing.T) {
	svc := newTestService(t)
	r := svc.RecordBatch(Event{Event: "test.batch", TS: core.Now().UTC().Unix(), Outcome: OutcomeOK})
	core.RequireTrue(t, r.OK)

	svc.mu.Lock()
	core.RequireTrue(t, svc.batched > 0)
	svc.mu.Unlock()

	svc.tickFlush()

	svc.mu.Lock()
	core.AssertEqual(t, 0, svc.batched)
	svc.mu.Unlock()
}

// TestService_TickFlush_Bad — no live handle (degraded / not-yet-
// opened Service) is a safe no-op.
func TestService_TickFlush_Bad(t *testing.T) {
	svc := newTestService(t)
	svc.mu.Lock()
	svc.currentFile = nil
	svc.mu.Unlock()
	svc.tickFlush() // must not panic
}

// TestService_TickRotateAndRetain_Good — an empty audit root is a
// clean no-op pass (both sweeps find nothing to do).
func TestService_TickRotateAndRetain_Good(t *testing.T) {
	svc := newTestService(t)
	svc.tickRotateAndRetain() // must not panic; nothing to assert on disk
}

// TestService_TickRotateAndRetain_Ugly — a stale-dated, non-current
// `.log` file older than CompressOlderThan gets compressed to
// `.log.gz` in place.
func TestService_TickRotateAndRetain_Ugly(t *testing.T) {
	svc := newTestService(t)
	svc.opts.CompressOlderThan = 1

	oldName := "2020-01-01.000.log"
	oldPath := core.PathJoin(svc.root, oldName)
	core.RequireTrue(t, core.WriteFile(oldPath, []byte(`{"event":"x"}`+"\n"), 0o600).OK)

	svc.tickRotateAndRetain()

	core.AssertTrue(t, core.Stat(oldPath+".gz").OK)
	core.AssertFalse(t, core.Stat(oldPath).OK)
}

// TestRoutesProvider_Name_Good — RouteGroups() returns exactly one
// provider whose Name()/BasePath() match the documented constants.
func TestRoutesProvider_Name_Good(t *testing.T) {
	svc := newTestService(t)
	groups := svc.RouteGroups()
	core.RequireTrue(t, len(groups) == 1)
	core.AssertEqual(t, GroupName, groups[0].Name())
	core.AssertEqual(t, APIBasePath, groups[0].BasePath())
}
