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

// Production wires Register via core.WithName(...); RegisterService fires
// post-factory inside WithName. Direct-call testing of Register must use
// WithName to model the production lifecycle (Mantis #1457).
func TestService_Register_Good(t *core.T) {
	t.Setenv("HOME", t.TempDir())
	c := core.New(core.WithName("fleet", fleet.Register))
	got := c.Service("fleet")
	core.AssertTrue(t, got.OK, "fleet service discoverable after WithName")
}

// fleet.Register is lock-tolerant + boot-tolerant by design: when the
// master DB can't be opened (lock contention from a sibling `lthn serve`,
// a HOME pointing at a regular file, etc.) it logs a warning and
// registers a degraded Service (db == nil) so `lthn` boot keeps going.
// Methods on the inert Service Fail cleanly rather than panic — the
// observable proof of degraded mode below.
//
// Earlier shape asserted Register Fails on bad HOME; that contract was
// flipped deliberately when graceful-degrade landed in service.go (see
// the Register godoc). Do not re-roll the "Fail on bad HOME" assertion
// — boot must not tank just because $HOME is misshapen. Mantis #1615.
func TestService_Register_BadHome_GracefulDegrade_Good(t *core.T) {
	tmp := t.TempDir()
	blocker := core.PathJoin(tmp, "not-a-dir")
	core.AssertTrue(t, core.WriteFile(blocker, []byte("x"), 0o644).OK)
	t.Setenv("HOME", blocker)
	c := core.New()
	r := fleet.Register(c)
	core.AssertTrue(t, r.OK, "Register must graceful-degrade (OK) when HOME is unusable")
	svc, ok := r.Value.(*fleet.Service)
	core.AssertTrue(t, ok, "Register Value must be *fleet.Service even in degraded mode")
	core.AssertNotNil(t, svc, "degraded Service handle must be non-nil + callable")
	// Inert-Service contract: every DB-touching method Fails cleanly
	// (does not panic) when db == nil. Mirrors the post-Close shape
	// asserted across the rest of this file.
	core.AssertNotPanics(t, func() { _ = svc.Machines() })
	core.AssertFalse(t, svc.Machines().OK, "Machines on degraded Service must Fail (db nil)")
	core.AssertFalse(t, svc.Queue(5).OK, "Queue on degraded Service must Fail (db nil)")
	core.AssertFalse(t, svc.Agents().OK, "Agents on degraded Service must Fail (db nil)")
	core.AssertFalse(t, svc.RoutingRules().OK, "RoutingRules on degraded Service must Fail (db nil)")
	// Close on the inert Service is a no-op OK (db nil branch in Close).
	core.AssertTrue(t, svc.Close().OK, "Close on degraded Service must be safe + OK")
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

// --- Agents (method on *Service) ---

func TestService_Service_Agents_Good(t *core.T) {
	svc := homeFixture(t)
	r := svc.Agents()
	core.AssertTrue(t, r.OK)
	rows := r.Value.([]fleet.Agent)
	core.AssertEqual(t, 0, len(rows))
}

func TestService_Service_Agents_Bad(t *core.T) {
	t.Setenv("HOME", t.TempDir())
	r := fleet.New()
	core.AssertTrue(t, r.OK)
	svc := r.Value.(*fleet.Service)
	core.AssertTrue(t, svc.Close().OK)
	core.AssertFalse(t, svc.Agents().OK)
}

func TestService_Service_Agents_Ugly(t *core.T) {
	svc := homeFixture(t)
	core.AssertTrue(t, svc.UpsertAgent(fleet.Agent{
		ID: "z", Name: "zelda", Provider: "openai-compat", Kind: "remote",
	}).OK)
	core.AssertTrue(t, svc.UpsertAgent(fleet.Agent{
		ID: "a", Name: "alpha", Provider: "ollama", Kind: "local",
	}).OK)
	r := svc.Agents()
	core.AssertTrue(t, r.OK)
	rows := r.Value.([]fleet.Agent)
	core.AssertEqual(t, 2, len(rows))
	core.AssertEqual(t, "alpha", rows[0].Name, "agents sort by name asc")
	core.AssertEqual(t, "zelda", rows[1].Name)
}

// --- UpsertAgent (method on *Service) ---

func TestService_Service_UpsertAgent_Good(t *core.T) {
	svc := homeFixture(t)
	core.AssertTrue(t, svc.UpsertAgent(fleet.Agent{
		ID: "openai-default", Name: "OpenAI · GPT-5",
		Provider: "openai-compat", Kind: "remote",
		BaseURL: "https://api.openai.com/v1",
		APIKeyRef: "key:openai:default", Model: "gpt-5",
		Persona: "concise reviewer",
	}).OK)
	r := svc.Agents()
	core.AssertTrue(t, r.OK)
	rows := r.Value.([]fleet.Agent)
	core.AssertEqual(t, 1, len(rows))
	core.AssertEqual(t, "OpenAI · GPT-5", rows[0].Name)
	core.AssertEqual(t, "remote", rows[0].Kind)
	core.AssertEqual(t, "idle", rows[0].Status, "Status defaults to 'idle'")
	core.AssertTrue(t, rows[0].CreatedAt > 0)
}

func TestService_Service_UpsertAgent_Bad(t *core.T) {
	t.Setenv("HOME", t.TempDir())
	r := fleet.New()
	core.AssertTrue(t, r.OK)
	svc := r.Value.(*fleet.Service)
	core.AssertTrue(t, svc.Close().OK)
	core.AssertFalse(t, svc.UpsertAgent(fleet.Agent{ID: "x", Name: "x", Provider: "ollama"}).OK)
}

func TestService_Service_UpsertAgent_Ugly(t *core.T) {
	svc := homeFixture(t)
	core.AssertTrue(t, svc.UpsertAgent(fleet.Agent{
		ID: "x", Name: "first", Provider: "openai-compat", Kind: "remote",
	}).OK)
	core.AssertTrue(t, svc.UpsertAgent(fleet.Agent{
		ID: "x", Name: "second", Provider: "ollama", Kind: "local",
	}).OK)
	r := svc.Agents()
	core.AssertTrue(t, r.OK)
	rows := r.Value.([]fleet.Agent)
	core.AssertEqual(t, 1, len(rows), "upsert collapses to a single row")
	core.AssertEqual(t, "second", rows[0].Name)
	core.AssertEqual(t, "ollama", rows[0].Provider)
	core.AssertEqual(t, "local", rows[0].Kind)
}

// --- DeleteAgent (method on *Service) ---

func TestService_Service_DeleteAgent_Good(t *core.T) {
	svc := homeFixture(t)
	core.AssertTrue(t, svc.UpsertAgent(fleet.Agent{
		ID: "x", Name: "x", Provider: "openai-compat", Kind: "remote",
	}).OK)
	r := svc.DeleteAgent("x")
	core.AssertTrue(t, r.OK)
	list := svc.Agents()
	core.AssertTrue(t, list.OK)
	core.AssertEqual(t, 0, len(list.Value.([]fleet.Agent)))
}

func TestService_Service_DeleteAgent_Bad(t *core.T) {
	t.Setenv("HOME", t.TempDir())
	r := fleet.New()
	core.AssertTrue(t, r.OK)
	svc := r.Value.(*fleet.Service)
	core.AssertTrue(t, svc.Close().OK)
	core.AssertFalse(t, svc.DeleteAgent("x").OK)
}

func TestService_Service_DeleteAgent_Ugly(t *core.T) {
	svc := homeFixture(t)
	// Deleting a nonexistent id is a no-op (OK) — DuckDB DELETE with
	// 0 rows affected still returns success.
	r := svc.DeleteAgent("does-not-exist")
	core.AssertTrue(t, r.OK)
	// Double-delete also OK.
	core.AssertTrue(t, svc.UpsertAgent(fleet.Agent{ID: "x", Name: "x", Provider: "openai-compat"}).OK)
	core.AssertTrue(t, svc.DeleteAgent("x").OK)
	core.AssertTrue(t, svc.DeleteAgent("x").OK, "second delete on same id idempotent")
}

// --- UpsertMachine (method on *Service) ---

// --- DeleteMachine (method on *Service) ---

func TestService_Service_DeleteMachine_Good(t *core.T) {
	svc := homeFixture(t)
	core.AssertTrue(t, svc.UpsertMachine(fleet.Machine{ID: "x", Name: "x"}).OK)
	r := svc.DeleteMachine("x")
	core.AssertTrue(t, r.OK)
	list := svc.Machines()
	core.AssertTrue(t, list.OK)
	core.AssertEqual(t, 0, len(list.Value.([]fleet.Machine)))
}

func TestService_Service_DeleteMachine_Bad(t *core.T) {
	t.Setenv("HOME", t.TempDir())
	r := fleet.New()
	core.AssertTrue(t, r.OK)
	svc := r.Value.(*fleet.Service)
	core.AssertTrue(t, svc.Close().OK)
	core.AssertFalse(t, svc.DeleteMachine("x").OK)
}

func TestService_Service_DeleteMachine_Ugly(t *core.T) {
	svc := homeFixture(t)
	// Delete-of-nonexistent is OK (idempotent).
	core.AssertTrue(t, svc.DeleteMachine("does-not-exist").OK)
	// Double-delete also OK.
	core.AssertTrue(t, svc.UpsertMachine(fleet.Machine{ID: "x", Name: "x"}).OK)
	core.AssertTrue(t, svc.DeleteMachine("x").OK)
	core.AssertTrue(t, svc.DeleteMachine("x").OK, "second delete on same id idempotent")
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

// Capability round-trip — Upsert writes JSON, Machines reads back
// as []string in the documented order, and empty/missing capabilities
// surface as [] not nil so TS callers can always .map() safely.
func TestService_Service_UpsertMachine_CapabilitiesRoundTrip(t *core.T) {
	svc := homeFixture(t)
	core.AssertTrue(t, svc.UpsertMachine(fleet.Machine{
		ID: "shop", Name: "shop · 7950X",
		Capabilities: []string{
			fleet.CapabilityInference,
			fleet.CapabilitySandbox,
			fleet.CapabilityBastion,
		},
	}).OK)
	// Machine with no capabilities → row exists with []string{} on read.
	core.AssertTrue(t, svc.UpsertMachine(fleet.Machine{ID: "plain", Name: "plain"}).OK)
	r := svc.Machines()
	core.AssertTrue(t, r.OK)
	rows := r.Value.([]fleet.Machine)
	core.AssertEqual(t, 2, len(rows))
	byID := map[string]fleet.Machine{}
	for _, m := range rows {
		byID[m.ID] = m
	}
	shop := byID["shop"]
	core.AssertEqual(t, 3, len(shop.Capabilities))
	core.AssertEqual(t, fleet.CapabilityInference, shop.Capabilities[0])
	core.AssertEqual(t, fleet.CapabilitySandbox, shop.Capabilities[1])
	core.AssertEqual(t, fleet.CapabilityBastion, shop.Capabilities[2])
	plain := byID["plain"]
	core.AssertNotNil(t, plain.Capabilities, "empty capabilities must be [] not nil")
	core.AssertEqual(t, 0, len(plain.Capabilities))
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
