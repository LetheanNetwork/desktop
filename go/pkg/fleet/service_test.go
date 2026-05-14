// SPDX-Licence-Identifier: EUPL-1.2

// Tests for the Fleet service. AX-7 triplet (Good/Bad/Ugly) per
// public symbol, named per the v0.9.0 audit:
//   - Free funcs (New, Register): Test<File>_<Symbol>_<Variant>
//   - Methods on *Service:        Test<File>_<Receiver>_<Method>_<Variant>
// Each test reroutes $HOME to t.TempDir() so the real ~/Lethean/ is
// never touched.

package fleet_test

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/fleet"
)

// homeFixture rebinds $HOME to a t-scoped temp dir + returns the
// fresh Service. Cleanup closes the DB so the file handle drops
// before the dir is removed.
func homeFixture(t *core.T) *fleet.Service {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	r := fleet.New()
	core.AssertTrue(t, r.OK, "fleet.New must succeed under temp HOME")
	svc := r.Value.(*fleet.Service)
	t.Cleanup(func() {
		closeR := svc.Close()
		core.AssertTrue(t, closeR.OK, "fleet.Service.Close must succeed in cleanup")
	})
	return svc
}

// --- New (free function) ---

func TestService_New_Good(t *core.T) {
	svc := homeFixture(t)
	core.AssertNotNil(t, svc, "fleet.New value must be non-nil")
	core.AssertEqual(t, "Fleet", svc.ServiceName(), "service name reflects the package")
}

func TestService_New_Bad(t *core.T) {
	// $HOME points at a regular file → paths.DataDir() fails because
	// MkdirAll cannot turn a file into a directory.
	tmp := t.TempDir()
	blocker := core.PathJoin(tmp, "not-a-dir")
	core.AssertTrue(t, core.WriteFile(blocker, []byte("x"), 0o644).OK)
	t.Setenv("HOME", blocker)
	r := fleet.New()
	core.AssertFalse(t, r.OK, "fleet.New must Fail when HOME is a regular file")
}

func TestService_New_Ugly(t *core.T) {
	// Two New() calls against the same $HOME both succeed — DuckDB
	// CREATE TABLE IF NOT EXISTS makes the schema apply idempotent.
	t.Setenv("HOME", t.TempDir())
	r1 := fleet.New()
	core.AssertTrue(t, r1.OK)
	svc1 := r1.Value.(*fleet.Service)
	core.AssertTrue(t, svc1.Close().OK)
	r2 := fleet.New()
	core.AssertTrue(t, r2.OK)
	svc2 := r2.Value.(*fleet.Service)
	core.AssertTrue(t, svc2.Close().OK)
}

// --- Register (free function) ---

func TestService_Register_Good(t *core.T) {
	t.Setenv("HOME", t.TempDir())
	c := core.New()
	r := fleet.Register(c)
	core.AssertTrue(t, r.OK)
	got := c.Service("fleet")
	core.AssertTrue(t, got.OK, "fleet service must be discoverable on Core after Register")
}

func TestService_Register_Bad(t *core.T) {
	tmp := t.TempDir()
	blocker := core.PathJoin(tmp, "not-a-dir")
	core.AssertTrue(t, core.WriteFile(blocker, []byte("x"), 0o644).OK)
	t.Setenv("HOME", blocker)
	c := core.New()
	r := fleet.Register(c)
	core.AssertFalse(t, r.OK, "Register must Fail when HOME is a regular file")
}

func TestService_Register_Ugly(t *core.T) {
	t.Setenv("HOME", t.TempDir())
	c := core.New()
	core.AssertTrue(t, fleet.Register(c).OK)
	// Second register against the same Core — Core.RegisterService
	// may reject duplicate names; behaviour is documented downstream.
	// Both outcomes acceptable; we just guard against panic.
	core.AssertNotPanics(t, func() { _ = fleet.Register(c) })
}

// --- Close (method on *Service) ---

func TestService_Service_Close_Good(t *core.T) {
	svc := homeFixture(t)
	r := svc.Close()
	core.AssertTrue(t, r.OK, "first Close must succeed")
	core.AssertNotNil(t, svc, "service handle remains addressable after Close")
}

func TestService_Service_Close_Bad(t *core.T) {
	t.Setenv("HOME", t.TempDir())
	r := fleet.New()
	core.AssertTrue(t, r.OK)
	svc := r.Value.(*fleet.Service)
	core.AssertTrue(t, svc.Close().OK)
	// Methods after Close must return Fail rather than panic.
	core.AssertFalse(t, svc.Machines().OK, "Machines after Close must Fail")
	core.AssertFalse(t, svc.Queue(5).OK, "Queue after Close must Fail")
}

func TestService_Service_Close_Ugly(t *core.T) {
	t.Setenv("HOME", t.TempDir())
	r := fleet.New()
	core.AssertTrue(t, r.OK)
	svc := r.Value.(*fleet.Service)
	core.AssertTrue(t, svc.Close().OK)
	core.AssertTrue(t, svc.Close().OK, "double-close must be idempotent")
	core.AssertTrue(t, svc.Close().OK, "triple-close still safe")
}

// --- Machines (method on *Service) ---

func TestService_Service_Machines_Good(t *core.T) {
	svc := homeFixture(t)
	r := svc.Machines()
	core.AssertTrue(t, r.OK)
	rows := r.Value.([]fleet.Machine)
	core.AssertEqual(t, 0, len(rows))
}

func TestService_Service_Machines_Bad(t *core.T) {
	t.Setenv("HOME", t.TempDir())
	r := fleet.New()
	core.AssertTrue(t, r.OK)
	svc := r.Value.(*fleet.Service)
	core.AssertTrue(t, svc.Close().OK)
	core.AssertFalse(t, svc.Machines().OK)
}

func TestService_Service_Machines_Ugly(t *core.T) {
	svc := homeFixture(t)
	core.AssertTrue(t, svc.UpsertMachine(fleet.Machine{ID: "z", Name: "zelda", Status: "online"}).OK)
	core.AssertTrue(t, svc.UpsertMachine(fleet.Machine{ID: "a", Name: "alpha", Status: "online"}).OK)
	core.AssertTrue(t, svc.UpsertMachine(fleet.Machine{ID: "self", Name: "my-mac", Status: "online", IsSelf: true}).OK)
	r := svc.Machines()
	core.AssertTrue(t, r.OK)
	rows := r.Value.([]fleet.Machine)
	core.AssertEqual(t, 3, len(rows))
	core.AssertEqual(t, "my-mac", rows[0].Name, "self machine must sort first")
	core.AssertEqual(t, "alpha", rows[1].Name)
	core.AssertEqual(t, "zelda", rows[2].Name)
}

// --- Queue (method on *Service) ---

func TestService_Service_Queue_Good(t *core.T) {
	svc := homeFixture(t)
	r := svc.Queue(10)
	core.AssertTrue(t, r.OK)
	rows := r.Value.([]fleet.QueueRow)
	core.AssertEqual(t, 0, len(rows))
}

func TestService_Service_Queue_Bad(t *core.T) {
	t.Setenv("HOME", t.TempDir())
	r := fleet.New()
	core.AssertTrue(t, r.OK)
	svc := r.Value.(*fleet.Service)
	core.AssertTrue(t, svc.Close().OK)
	core.AssertFalse(t, svc.Queue(5).OK)
}

func TestService_Service_Queue_Ugly(t *core.T) {
	svc := homeFixture(t)
	// limit=0 must not error; query just omits the LIMIT clause.
	r := svc.Queue(0)
	core.AssertTrue(t, r.OK)
	r2 := svc.Queue(-5)
	core.AssertTrue(t, r2.OK, "negative limits treated same as zero — no cap")
}

// --- RoutingRules (method on *Service) ---

func TestService_Service_RoutingRules_Good(t *core.T) {
	svc := homeFixture(t)
	r := svc.RoutingRules()
	core.AssertTrue(t, r.OK)
	rules := r.Value.([]fleet.RoutingRule)
	core.AssertEqual(t, 0, len(rules))
}

func TestService_Service_RoutingRules_Bad(t *core.T) {
	t.Setenv("HOME", t.TempDir())
	r := fleet.New()
	core.AssertTrue(t, r.OK)
	svc := r.Value.(*fleet.Service)
	core.AssertTrue(t, svc.Close().OK)
	core.AssertFalse(t, svc.RoutingRules().OK)
}

func TestService_Service_RoutingRules_Ugly(t *core.T) {
	svc := homeFixture(t)
	core.AssertTrue(t, svc.RoutingRules().OK)
	core.AssertTrue(t, svc.RoutingRules().OK, "repeated query must be idempotent")
}

// --- UpsertMachine (method on *Service) ---

func TestService_Service_UpsertMachine_Good(t *core.T) {
	svc := homeFixture(t)
	core.AssertTrue(t, svc.UpsertMachine(fleet.Machine{
		ID: "this-mac", Name: "this Mac", Arch: "M3 Pro · 36 GB",
		Status: "online · loaded", Model: "gemma-4-e2b",
		LoadPct: 32, TPS: 47.2, IsSelf: true,
	}).OK)
	r := svc.Machines()
	core.AssertTrue(t, r.OK)
	rows := r.Value.([]fleet.Machine)
	core.AssertEqual(t, 1, len(rows))
	core.AssertEqual(t, "this Mac", rows[0].Name)
	core.AssertTrue(t, rows[0].IsSelf)
	core.AssertTrue(t, rows[0].CreatedAt > 0)
}

func TestService_Service_UpsertMachine_Bad(t *core.T) {
	t.Setenv("HOME", t.TempDir())
	r := fleet.New()
	core.AssertTrue(t, r.OK)
	svc := r.Value.(*fleet.Service)
	core.AssertTrue(t, svc.Close().OK)
	core.AssertFalse(t, svc.UpsertMachine(fleet.Machine{ID: "x", Name: "x"}).OK)
}

func TestService_Service_UpsertMachine_Ugly(t *core.T) {
	svc := homeFixture(t)
	core.AssertTrue(t, svc.UpsertMachine(fleet.Machine{ID: "x", Name: "first", Status: "offline"}).OK)
	core.AssertTrue(t, svc.UpsertMachine(fleet.Machine{ID: "x", Name: "second", Status: "online"}).OK)
	r := svc.Machines()
	core.AssertTrue(t, r.OK)
	rows := r.Value.([]fleet.Machine)
	core.AssertEqual(t, 1, len(rows), "upsert collapses to a single row")
	core.AssertEqual(t, "second", rows[0].Name)
	core.AssertEqual(t, "online", rows[0].Status)
}

// --- ServiceName / ServiceStartup / ServiceShutdown (Wails lifecycle) ---

func TestService_Service_ServiceName_Good(t *core.T) {
	svc := homeFixture(t)
	name := svc.ServiceName()
	core.AssertEqual(t, "Fleet", name)
}

func TestService_Service_ServiceName_Bad(t *core.T) {
	// Defensive: ServiceName on nil receiver MUST not panic. The
	// pointer-receiver method returns a literal and never deref's
	// the receiver, so it's safe even with a nil pointer.
	var svc *fleet.Service
	core.AssertNotPanics(t, func() { _ = svc.ServiceName() })
	// Confirm same call against a non-nil zero-value Service also
	// returns the package literal without exploding.
	zero := &fleet.Service{}
	core.AssertEqual(t, "Fleet", zero.ServiceName())
}

func TestService_Service_ServiceName_Ugly(t *core.T) {
	svc := homeFixture(t)
	first := svc.ServiceName()
	second := svc.ServiceName()
	core.AssertEqual(t, first, second)
	core.AssertEqual(t, "Fleet", first)
}

func TestService_Service_ServiceStartup_Good(t *core.T) {
	svc := homeFixture(t)
	r := svc.ServiceStartup(core.Background(), nil)
	core.AssertTrue(t, r.OK)
}

func TestService_Service_ServiceStartup_Bad(t *core.T) {
	// ServiceStartup is a no-op in our shape — Init() already ran in
	// New(). After Close it should still not panic and should still
	// report OK (it doesn't touch the DB).
	t.Setenv("HOME", t.TempDir())
	r := fleet.New()
	core.AssertTrue(t, r.OK)
	svc := r.Value.(*fleet.Service)
	core.AssertTrue(t, svc.Close().OK)
	sr := svc.ServiceStartup(core.Background(), nil)
	core.AssertTrue(t, sr.OK, "ServiceStartup is a no-op; safe after Close")
}

func TestService_Service_ServiceStartup_Ugly(t *core.T) {
	svc := homeFixture(t)
	r := svc.ServiceStartup(core.Background(), nil)
	core.AssertTrue(t, r.OK)
	// any opts type should be accepted (signature is `any`).
	r2 := svc.ServiceStartup(core.Background(), struct{}{})
	core.AssertTrue(t, r2.OK)
}

func TestService_Service_ServiceShutdown_Good(t *core.T) {
	t.Setenv("HOME", t.TempDir())
	r := fleet.New()
	core.AssertTrue(t, r.OK)
	svc := r.Value.(*fleet.Service)
	sr := svc.ServiceShutdown()
	core.AssertTrue(t, sr.OK, "ServiceShutdown closes the DB; first call must succeed")
}

func TestService_Service_ServiceShutdown_Bad(t *core.T) {
	// Second shutdown should still report OK (idempotent close).
	t.Setenv("HOME", t.TempDir())
	r := fleet.New()
	core.AssertTrue(t, r.OK)
	svc := r.Value.(*fleet.Service)
	core.AssertTrue(t, svc.ServiceShutdown().OK)
	core.AssertTrue(t, svc.ServiceShutdown().OK, "second shutdown must be safe")
}

func TestService_Service_ServiceShutdown_Ugly(t *core.T) {
	t.Setenv("HOME", t.TempDir())
	r := fleet.New()
	core.AssertTrue(t, r.OK)
	svc := r.Value.(*fleet.Service)
	mr := svc.Machines()
	core.AssertTrue(t, mr.OK)
	sr := svc.ServiceShutdown()
	core.AssertTrue(t, sr.OK)
}
