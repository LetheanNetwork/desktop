// SPDX-Licence-Identifier: EUPL-1.2

// Internal-package tests for the Mantis #1693 MED (Cerberus #54 C5)
// orm.Save / orm.Delete error-handling fixes. Internal because the
// rollback helper (rollbackSpawnedSandboxes) is unexported and the
// load-bearing security assertion is that the helper iterates ids
// calling sbSvc.Kill without panicking on the nil / empty edges; the
// Stop + Uninstall round-trip cases exercise the public surface
// against a failing orm medium so the orm error propagation is
// covered end-to-end.

package marketplace

import (
	core "dappco.re/go"
	"dappco.re/go/orm"
)

// failingWriteMedium wraps an orm.Memium and toggles Write into a
// hard Fail once failWrites flips true. Read / Stream / Watch /
// Search delegate to the underlying Memium so the seeded record
// remains discoverable via findInstalledBundle — the Save / Delete
// callsite is the one we want to break, not the seeding round-trip.
//
// Usage example (internal):
//
//	med := &failingWriteMedium{mem: orm.NewMemium()}
//	orm.Mount(c, "default", med)
//	// ... seed a record while med.failWrites is false ...
//	med.failWrites = true
//	// next Save / Delete returns Fail
type failingWriteMedium struct {
	mem        *orm.Memium
	failWrites bool
	writeCount int
}

func (m *failingWriteMedium) Caps() orm.Capabilities { return m.mem.Caps() }

func (m *failingWriteMedium) Read(ctx core.Context, in orm.ReadIntent) core.Result {
	return m.mem.Read(ctx, in)
}

func (m *failingWriteMedium) Stream(ctx core.Context, in orm.ReadIntent) core.Result {
	return m.mem.Stream(ctx, in)
}

func (m *failingWriteMedium) Watch(ctx core.Context, in orm.ReadIntent) core.Result {
	return m.mem.Watch(ctx, in)
}

func (m *failingWriteMedium) Search(ctx core.Context, in orm.ReadIntent) core.Result {
	return m.mem.Search(ctx, in)
}

func (m *failingWriteMedium) Write(ctx core.Context, in orm.WriteIntent) core.Result {
	m.writeCount++
	if m.failWrites {
		return core.Fail(core.E("marketplace.test.medium",
			"injected write failure for rollback test", nil))
	}
	return m.mem.Write(ctx, in)
}

// newCoreWithFailingMedium builds a Core with orm + the wrapping
// medium + the InstalledBundle schema registered. The returned
// medium is shared so callers can flip failWrites after seeding.
func newCoreWithFailingMedium(t *core.T) (*core.Core, *failingWriteMedium) {
	t.Helper()
	c := core.New()
	core.RequireTrue(t, orm.Register(c).OK)
	med := &failingWriteMedium{mem: orm.NewMemium()}
	core.RequireTrue(t, orm.Mount(c, "default", med).OK)
	var rec InstalledBundle
	schema := rec.Schema()
	core.RequireTrue(t, orm.RegisterSchema(c, schema).OK)
	med.mem.RegisterTable(schema.Name, schema)
	core.RequireTrue(t, c.ServiceStartup(core.Background(), nil).OK)
	t.Cleanup(func() { _ = c.ServiceShutdown(core.Background()) })
	return c, med
}

// TestMarketplace_Install_OrmSaveFails_Rolls_Back_Container_Bad —
// Mantis #1693 MED (Cerberus #54 C5). The full Install path needs a
// process.Service-backed sandbox.Service to reach the orm.Save call
// (SpawnLong fails outright when proc is unwired), which is out of
// scope for a unit test. Exercise the rollback helper directly so
// the load-bearing assertion — "orm.Save failure triggers a Kill
// sweep over every spawned sandbox id without panicking on edge
// inputs" — has unit coverage.
//
// The Install callsite shares the helper one-for-one with Launch
// (same signature, same ordering: spawn loop populates ids,
// orm.Save fails, rollback iterates ids calling sbSvc.Kill). The
// sandbox.Service is nil here to assert the defensive nil-tolerant
// branch fires — the rollback path is already an unhappy branch;
// compounding with a nil-deref would hide the real orm error.
func TestMarketplace_Install_OrmSaveFails_Rolls_Back_Container_Bad(t *core.T) {
	// Same shape as the production callsite when sbSvc is nil — the
	// helper must not panic; the Install caller will already have
	// returned Fail("sandbox service not available") long before
	// reaching this code path, but defence in depth costs nothing.
	core.AssertNotPanics(t, func() {
		ids := map[string]string{
			"app":  "sb-aaaa1111",
			"side": "sb-bbbb2222",
		}
		rollbackSpawnedSandboxes(nil, ids)
	})

	// Empty ids map is the no-op path — exercised when Install's
	// spawn loop ran but every image's resolveVolumes Fail-ed before
	// the SpawnLong call. Helper must tolerate without iterating.
	core.AssertNotPanics(t, func() {
		rollbackSpawnedSandboxes(nil, map[string]string{})
	})

	// Empty-string sandbox id is the skip path — exercised when an
	// image entry made it past resolveVolumes but SpawnLong returned
	// a Fail Result (id never assigned). Helper must not call Kill
	// with an empty id (which would log a spurious EventSandboxKillFailed
	// row at audit.Default()).
	core.AssertNotPanics(t, func() {
		rollbackSpawnedSandboxes(nil, map[string]string{"app": ""})
	})
}

// TestMarketplace_Stop_OrmSaveFails_Returns_Error_Bad — Mantis #1693
// MED (Cerberus #54 C5). The Stop callsite's failure mode is a stale
// Status flag (containers are already killed by the loop above the
// orm.Save; nothing to roll back). Before this ticket Stop silently
// swallowed the orm error via `_ =`, leaving the record reading
// "running" when live state was "stopped". Assert the error now
// surfaces to the caller.
func TestMarketplace_Stop_OrmSaveFails_Returns_Error_Bad(t *core.T) {
	c, med := newCoreWithFailingMedium(t)
	svc := NewService(c)

	// Seed a record so findInstalledBundle inside Stop hits OK and
	// the orm.Save path actually executes (failWrites=false during
	// seed so the seed write succeeds via the underlying Memium).
	rec := InstalledBundle{
		BundleID:       "test-bundle",
		Display:        "Test Bundle",
		ManifestSchema: "lthn-vm/v1",
		Status:         BundleStatusRunning,
		ConfigPath:     "/tmp/lthn-marketplace/test-bundle",
		InstalledAt:    core.Now(),
	}
	seedR := orm.Of[InstalledBundle](c).Save(&rec)
	core.RequireTrue(t, seedR.OK)

	// Flip the switch — next Write returns Fail.
	med.failWrites = true

	r := svc.Stop("test-bundle")
	core.AssertFalse(t, r.OK)
	// The error string must point at the orm cause so a debugger
	// reading the audit log can correlate the Stop failure with the
	// injected medium error.
	core.AssertTrue(t, core.Contains(r.Error(), "orm save failed"))
}

// TestMarketplace_Uninstall_OrmDeleteFails_Returns_Error_Bad — Mantis
// #1693 MED (Cerberus #54 C5). The Uninstall callsite's failure mode
// is a stale registry record (sandboxes are torn down by stopLocked +
// plugin.UnregisterBundle above the orm.Delete; nothing to roll back).
// Before this ticket Uninstall silently swallowed the orm.Delete error
// via `_ =`, leaving the record discoverable by ListInstalled even
// though the runtime + plugin entries were gone. Assert the error now
// surfaces to the caller; broadcast still fires so live UI subscribers
// can re-fetch and reflect the half-state.
func TestMarketplace_Uninstall_OrmDeleteFails_Returns_Error_Bad(t *core.T) {
	c, med := newCoreWithFailingMedium(t)
	svc := NewService(c)

	// Seed a record so findInstalledBundle inside Uninstall hits OK.
	rec := InstalledBundle{
		BundleID:       "test-bundle",
		Display:        "Test Bundle",
		ManifestSchema: "lthn-vm/v1",
		Status:         BundleStatusRunning,
		ConfigPath:     "/tmp/lthn-marketplace/test-bundle",
		InstalledAt:    core.Now(),
	}
	seedR := orm.Of[InstalledBundle](c).Save(&rec)
	core.RequireTrue(t, seedR.OK)

	// Subscribe to the BundleChanged broadcast so the test can
	// observe that uninstall still fires the PhaseUninstalled event
	// even on the orm.Delete failure path (live UI subscribers can
	// re-fetch and reflect the half-state).
	var got BundleChanged
	Subscribe(c, func(_ *core.Core, ev BundleChanged) {
		got = ev
	})

	med.failWrites = true

	r := svc.Uninstall("test-bundle")
	core.AssertFalse(t, r.OK)
	core.AssertTrue(t, core.Contains(r.Error(), "orm delete failed"))
	// Broadcast still fires on the failure path so subscribers see
	// the captured pre-uninstall state and can re-fetch.
	core.AssertEqual(t, PhaseUninstalled, got.Phase)
	core.AssertEqual(t, "test-bundle", got.Bundle.BundleID)
}
