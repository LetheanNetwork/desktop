// SPDX-Licence-Identifier: EUPL-1.2

package tasks_test

import (
	core "dappco.re/go"
	"dappco.re/go/orm"
	"dappco.re/lthn/desktop/pkg/auth"
	"dappco.re/lthn/desktop/pkg/tasks"
)

// newServiceCore wires a Service over an in-memory Memium with the
// tasks schemas registered. Mirrors newTestCore in api_test.go so the
// Wails surface gets the same isolated harness.
//
// Stamps TierOperator with subject "test-operator" via the auth
// sidecar so the B2 Require gates in wails.go (Create / Update /
// AddNote / List / Get) pass through. Tests that want to exercise a
// different tier (e.g. TierRenderer for ENFORCE-not-claim forgery
// rejection) re-stamp via auth.SetCaller(c, ...) after constructing
// the service.
func newServiceCore(t *core.T) (*tasks.Service, *core.Core) {
	t.Helper()
	c := core.New()
	core.RequireTrue(t, orm.Register(c).OK)
	mem := orm.NewMemium()
	core.RequireTrue(t, orm.Mount(c, "default", mem).OK)
	for _, schema := range tasks.Schemas() {
		core.RequireTrue(t, orm.RegisterSchema(c, schema).OK)
		mem.RegisterTable(schema.Name, schema)
	}
	auth.SetCaller(c, auth.CallerIdentity{
		Tier:    auth.TierOperator,
		Subject: "test-operator",
		Source:  "tasks_test",
	})
	return tasks.NewService(c), c
}

// --- NewService ---------------------------------------------------------

func TestWails_NewService_Good(t *core.T) {
	svc, _ := newServiceCore(t)
	core.AssertNotNil(t, svc)
}

func TestWails_NewService_Bad(t *core.T) {
	// Nil Core is permitted at construction time — the failure
	// surface is at-call, not at-construct. Documenting that here
	// keeps us honest: NewService(nil) returns a non-nil pointer.
	svc := tasks.NewService(nil)
	core.AssertNotNil(t, svc)
}

func TestWails_NewService_Ugly(t *core.T) {
	// Repeated construction yields independent Services — no shared
	// global state. Each Wails service binding gets its own struct.
	svc1, _ := newServiceCore(t)
	svc2, _ := newServiceCore(t)
	core.AssertNotEqual(t, core.Sprintf("%p", svc1), core.Sprintf("%p", svc2))
}

// --- Register -----------------------------------------------------------

func TestWails_Register_Good(t *core.T) {
	c := core.New()
	r := tasks.Register(c)
	core.AssertTrue(t, r.OK)
	core.AssertNotNil(t, r.Value)
}

func TestWails_Register_Bad(t *core.T) {
	// Register on a nil container — Go would panic on a nil receiver
	// inside core.Ok; we accept that and just confirm the success
	// path doesn't depend on c being non-nil for construction.
	r := tasks.Register(nil)
	core.AssertTrue(t, r.OK)
}

func TestWails_Register_Ugly(t *core.T) {
	// Two registers yield two distinct services — Register is not a
	// singleton helper.
	c := core.New()
	a := tasks.Register(c).Value
	b := tasks.Register(c).Value
	core.AssertNotEqual(t, core.Sprintf("%p", a), core.Sprintf("%p", b))
}

// --- List ---------------------------------------------------------------

func TestWails_List_Good(t *core.T) {
	svc, c := newServiceCore(t)
	tasks.Create(c, tasks.CreateInput{Project: "lthn", Summary: "wire backlog"})
	tasks.Create(c, tasks.CreateInput{Project: "lthn", Summary: "wire sprints"})
	r := svc.List(tasks.ListInput{Project: "lthn"})
	core.RequireTrue(t, r.OK)
	out, ok := r.Value.(tasks.ListOutput)
	core.RequireTrue(t, ok)
	core.AssertEqual(t, 2, out.Count)
	core.AssertLen(t, out.Issues, 2)
}

func TestWails_List_Bad(t *core.T) {
	svc, _ := newServiceCore(t)
	r := svc.List(tasks.ListInput{Project: "no-such-project"})
	core.RequireTrue(t, r.OK)
	out, ok := r.Value.(tasks.ListOutput)
	core.RequireTrue(t, ok)
	core.AssertEqual(t, 0, out.Count)
	core.AssertLen(t, out.Issues, 0)
}

func TestWails_List_Ugly(t *core.T) {
	svc, c := newServiceCore(t)
	tasks.Create(c, tasks.CreateInput{Project: "a", Summary: "one"})
	tasks.Create(c, tasks.CreateInput{Project: "b", Summary: "two"})
	tasks.Create(c, tasks.CreateInput{Project: "c", Summary: "three"})
	// Empty input = no filter; all three projects surface together.
	r := svc.List(tasks.ListInput{})
	out, ok := r.Value.(tasks.ListOutput)
	core.RequireTrue(t, ok)
	core.AssertEqual(t, 3, out.Count)
}

// --- Get ----------------------------------------------------------------

func TestWails_Get_Good(t *core.T) {
	svc, c := newServiceCore(t)
	created, _, _ := orm.Detail[tasks.Issue](tasks.Create(c, tasks.CreateInput{Project: "lthn", Summary: "fetch me"}))
	r := svc.Get(tasks.GetInput{ID: created.ID})
	core.RequireTrue(t, r.OK)
	got, _, ok := orm.Detail[tasks.Issue](r)
	core.RequireTrue(t, ok)
	core.AssertEqual(t, created.ID, got.ID)
}

func TestWails_Get_Bad(t *core.T) {
	svc, _ := newServiceCore(t)
	r := svc.Get(tasks.GetInput{ID: ""})
	core.AssertFalse(t, r.OK)
}

func TestWails_Get_Ugly(t *core.T) {
	svc, _ := newServiceCore(t)
	// ID present but no row → orm-level fail surfaces unchanged.
	r := svc.Get(tasks.GetInput{ID: "ghost-id-no-row"})
	core.AssertFalse(t, r.OK)
}

// --- Create -------------------------------------------------------------

func TestWails_Create_Good(t *core.T) {
	svc, _ := newServiceCore(t)
	r := svc.Create(tasks.CreateIssueInput{Project: "lthn", Summary: "create via wails"})
	core.RequireTrue(t, r.OK)
	issue, _, ok := orm.Detail[tasks.Issue](r)
	core.RequireTrue(t, ok)
	core.AssertEqual(t, "lthn", issue.Project)
	core.AssertEqual(t, tasks.StateOpen, issue.State)
}

func TestWails_Create_Bad(t *core.T) {
	svc, _ := newServiceCore(t)
	r := svc.Create(tasks.CreateIssueInput{Summary: "no project"})
	core.AssertFalse(t, r.OK)
}

func TestWails_Create_Ugly(t *core.T) {
	svc, _ := newServiceCore(t)
	// Optional fields default-applied; severity falls back to minor.
	r := svc.Create(tasks.CreateIssueInput{Project: "lthn", Summary: "defaults"})
	issue, _, ok := orm.Detail[tasks.Issue](r)
	core.RequireTrue(t, ok)
	core.AssertEqual(t, tasks.SeverityMinor, issue.Severity)
}

// --- Update -------------------------------------------------------------

func TestWails_Update_Good(t *core.T) {
	svc, c := newServiceCore(t)
	created, _, _ := orm.Detail[tasks.Issue](tasks.Create(c, tasks.CreateInput{Project: "lthn", Summary: "update me"}))
	r := svc.Update(tasks.UpdateIssueInput{ID: created.ID, State: tasks.StateInProgress})
	updated, _, ok := orm.Detail[tasks.Issue](r)
	core.RequireTrue(t, ok)
	core.AssertEqual(t, tasks.StateInProgress, updated.State)
}

func TestWails_Update_Bad(t *core.T) {
	svc, _ := newServiceCore(t)
	r := svc.Update(tasks.UpdateIssueInput{State: tasks.StateDone})
	core.AssertFalse(t, r.OK)
}

func TestWails_Update_Ugly(t *core.T) {
	svc, c := newServiceCore(t)
	created, _, _ := orm.Detail[tasks.Issue](tasks.Create(c, tasks.CreateInput{Project: "lthn", Summary: "close via wails"}))
	r := svc.Update(tasks.UpdateIssueInput{ID: created.ID, State: tasks.StateDone})
	closed, _, ok := orm.Detail[tasks.Issue](r)
	core.RequireTrue(t, ok)
	core.AssertEqual(t, tasks.StateDone, closed.State)
	core.AssertFalse(t, closed.ClosedAt.IsZero())
}

// --- AddNote ------------------------------------------------------------

func TestWails_AddNote_Good(t *core.T) {
	svc, c := newServiceCore(t)
	parent, _, _ := orm.Detail[tasks.Issue](tasks.Create(c, tasks.CreateInput{Project: "lthn", Summary: "note target"}))
	r := svc.AddNote(tasks.AddNoteInput{IssueID: parent.ID, Body: "first note", Author: "hephaestus"})
	core.RequireTrue(t, r.OK)
	note, _, ok := orm.Detail[tasks.Note](r)
	core.RequireTrue(t, ok)
	core.AssertEqual(t, parent.ID, note.IssueID)
	core.AssertEqual(t, "first note", note.Body)
}

func TestWails_AddNote_Bad(t *core.T) {
	svc, _ := newServiceCore(t)
	r := svc.AddNote(tasks.AddNoteInput{Body: "orphan", Author: "x"})
	core.AssertFalse(t, r.OK)
}

func TestWails_AddNote_Ugly(t *core.T) {
	svc, _ := newServiceCore(t)
	// Empty body — api.go enforces non-empty body, expect fail.
	r := svc.AddNote(tasks.AddNoteInput{IssueID: "id-x", Body: "", Author: "x"})
	core.AssertFalse(t, r.OK)
}

// --- *WailsService Shape (a.i) IPC-entry stamp ---------------------------
//
// RFC.tier-auth-substrate.md v3.1 §4.4 / Cerberus #73 F-1 / Mantis #1755.
// *WailsService wraps *Service; every wrapper method stamps TierRenderer
// onto the auth sidecar at IPC entry, then defers ClearCaller so the
// stamp clears at return. These tests pin the contract from three
// directions: positive (stamp lifts unstamped core to TierRenderer),
// negative (stamp is cleared on return — no leakage), cyclic (sequential
// calls all stamp-and-clear cleanly).

// newUnstampedWailsService wires a *WailsService over an unstamped
// *core.Core (no auth.SetCaller). The IPC-entry stamp inside the wrapper
// is the only source of caller identity — lets the tests observe the
// wrapper's stamp without confusing it with ambient sidecar state.
func newUnstampedWailsService(t *core.T) (*tasks.WailsService, *core.Core) {
	t.Helper()
	c := core.New()
	core.RequireTrue(t, orm.Register(c).OK)
	mem := orm.NewMemium()
	core.RequireTrue(t, orm.Mount(c, "default", mem).OK)
	for _, schema := range tasks.Schemas() {
		core.RequireTrue(t, orm.RegisterSchema(c, schema).OK)
		mem.RegisterTable(schema.Name, schema)
	}
	return tasks.NewWailsService(tasks.NewService(c)), c
}

// TestWailsTasksService_Create_StampsRenderer_Good — the wrapper's
// IPC-entry stamp lifts an unstamped *core.Core to TierRenderer for the
// duration of the call, so the inner *Service.Create's existing Require
// gate passes without any external (HTTP-middleware or test-harness)
// SetCaller. Without the wrapper stamp the gate would deny — proves
// the wrapper is the source of the renderer-tier authority on Wails
// IPC entry. Reporter == "wails" pins the ENFORCE-not-claim chain
// running end-to-end through the wrapper.
func TestWailsTasksService_Create_StampsRenderer_Good(t *core.T) {
	w, _ := newUnstampedWailsService(t)
	r := w.Create(tasks.CreateIssueInput{Project: "lthn", Summary: "renderer-stamped via IPC entry"})
	core.RequireTrue(t, r.OK)
	issue, _, ok := orm.Detail[tasks.Issue](r)
	core.RequireTrue(t, ok)
	core.AssertEqual(t, "lthn", issue.Project)
	// ENFORCE-not-claim — Reporter is overwritten by the inner
	// *Service.Create from auth.Caller(s.core).Subject, which is the
	// wrapper's "wails" stamp when no upstream session has overridden
	// it. Pins the subject the audit trail records for a Wails-IPC
	// create with no logged-in session.
	core.AssertEqual(t, "wails", issue.Reporter)
}

// TestWailsTasksService_Create_StampsRenderer_Bad — the stamp is CLEARED
// on return via defer ClearCaller, so reading the sidecar AFTER the
// wrapper call returns shows TierInternal. Defends against the stamp
// leaking across IPC boundaries into background tasks / cascade workers
// that may run on the same *core.Core after the Wails call returns.
// Additionally proves the bare-substrate (no wrapper) call path: a
// direct *Service.Create on the unstamped core, post-clear, denies as
// expected — the substrate's gate is intact.
func TestWailsTasksService_Create_StampsRenderer_Bad(t *core.T) {
	w, c := newUnstampedWailsService(t)
	r := w.Create(tasks.CreateIssueInput{Project: "lthn", Summary: "first call stamps + clears"})
	core.RequireTrue(t, r.OK)
	// After the deferred ClearCaller, the sidecar slot for c is empty;
	// auth.Caller(c) resolves to the TierInternal floor.
	id := auth.Caller(c)
	core.AssertEqual(t, auth.TierInternal, id.Tier)
	core.AssertEqual(t, "", id.Subject)
	// A direct call into the substrate (bypassing the wrapper) now sees
	// TierInternal and is denied — proves the wrapper's stamp is the
	// only renderer-tier source on this *core.Core.
	bare := w.Substrate().Create(tasks.CreateIssueInput{Project: "lthn", Summary: "no stamp"})
	core.AssertFalse(t, bare.OK)
	core.AssertContains(t, bare.Error(), "tier_not_permitted")
}

// TestWailsTasksService_Create_StampsRenderer_Ugly — sequential calls
// each get their own stamp at entry and clear at return; the sidecar is
// empty at the end. Pins the stamp-and-clear cycle. Also exercises all
// five wrapper methods (List/Get/Create/Update/AddNote) end-to-end so a
// future contributor who forgets stampRenderer on a new method fails
// red here when the substrate gate denies.
func TestWailsTasksService_Create_StampsRenderer_Ugly(t *core.T) {
	w, c := newUnstampedWailsService(t)
	r := w.Create(tasks.CreateIssueInput{Project: "lthn", Summary: "seed"})
	core.RequireTrue(t, r.OK)
	issue, _, _ := orm.Detail[tasks.Issue](r)
	// List
	core.RequireTrue(t, w.List(tasks.ListInput{Project: "lthn"}).OK)
	// Get
	core.RequireTrue(t, w.Get(tasks.GetInput{ID: issue.ID}).OK)
	// Update
	core.RequireTrue(t, w.Update(tasks.UpdateIssueInput{ID: issue.ID, State: tasks.StateInProgress}).OK)
	// AddNote
	core.RequireTrue(t, w.AddNote(tasks.AddNoteInput{IssueID: issue.ID, Body: "via wrapper"}).OK)
	// Sidecar empty post-cycle — defer ClearCaller fires every method.
	id := auth.Caller(c)
	core.AssertEqual(t, auth.TierInternal, id.Tier)
}

// --- *WailsService.Substrate accessor ------------------------------------

// TestWailsTasksService_Substrate_Good — Substrate() returns the wrapped
// *Service pointer (NOT a fresh copy) so in-Go consumers reach the
// substrate directly without re-constructing. Pins the accessor for the
// wrapper-drift exempt-list contract.
func TestWailsTasksService_Substrate_Good(t *core.T) {
	c := core.New()
	svc := tasks.NewService(c)
	w := tasks.NewWailsService(svc)
	core.AssertEqual(t, core.Sprintf("%p", svc), core.Sprintf("%p", w.Substrate()))
}
