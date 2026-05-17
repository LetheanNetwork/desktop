// SPDX-License-Identifier: EUPL-1.2

package tasks_test

import (
	core "dappco.re/go"
	"dappco.re/go/orm"
	"dappco.re/lthn/desktop/pkg/auth"
	"dappco.re/lthn/desktop/pkg/tasks"
)

// newTestCore returns a *core.Core with orm registered + a fresh Memium
// mounted under "default" + the tasks schemas registered. Tests use this
// to exercise the package end-to-end without any DuckDB / filesystem.
func newTestCore(t *core.T) *core.Core {
	t.Helper()
	c := core.New()
	core.RequireTrue(t, orm.Register(c).OK)
	mem := orm.NewMemium()
	core.RequireTrue(t, orm.Mount(c, "default", mem).OK)
	for _, schema := range tasks.Schemas() {
		core.RequireTrue(t, orm.RegisterSchema(c, schema).OK)
		mem.RegisterTable(schema.Name, schema)
	}
	return c
}

func TestApi_Create_Good(t *core.T) {
	c := newTestCore(t)
	result := tasks.Create(c, tasks.CreateInput{Project: "ide", Summary: "create good"})
	core.RequireTrue(t, result.OK)
	issue, _, ok := orm.Detail[tasks.Issue](result)
	core.RequireTrue(t, ok)
	core.AssertEqual(t, "ide", issue.Project)
}

func TestApi_Create_Bad(t *core.T) {
	c := newTestCore(t)
	result := tasks.Create(c, tasks.CreateInput{Summary: "missing project"})
	core.AssertFalse(t, result.OK)
}

func TestApi_Create_Ugly(t *core.T) {
	c := newTestCore(t)
	result := tasks.Create(c, tasks.CreateInput{Project: "ide", Summary: "defaults"})
	issue, _, ok := orm.Detail[tasks.Issue](result)
	core.RequireTrue(t, ok)
	core.AssertEqual(t, tasks.StateOpen, issue.State)
	core.AssertEqual(t, tasks.PriorityNormal, issue.Priority)
}

func TestApi_Get_Good(t *core.T) {
	c := newTestCore(t)
	created := tasks.Create(c, tasks.CreateInput{Project: "ide", Summary: "get good"})
	issue, _, _ := orm.Detail[tasks.Issue](created)
	result := tasks.Get(c, issue.ID)
	core.AssertTrue(t, result.OK)
}

func TestApi_Get_Bad(t *core.T) {
	c := newTestCore(t)
	result := tasks.Get(c, "missing")
	core.AssertFalse(t, result.OK)
}

func TestApi_Get_Ugly(t *core.T) {
	c := newTestCore(t)
	created := tasks.Create(c, tasks.CreateInput{Project: "ide", Summary: "get ugly"})
	issue, _, _ := orm.Detail[tasks.Issue](created)
	found, _, ok := orm.Detail[tasks.Issue](tasks.Get(c, issue.ID))
	core.RequireTrue(t, ok)
	core.AssertEqual(t, issue.ID, found.ID)
}

func TestApi_List_Good(t *core.T) {
	c := newTestCore(t)
	tasks.Create(c, tasks.CreateInput{Project: "ide", Summary: "one"})
	tasks.Create(c, tasks.CreateInput{Project: "other", Summary: "two"})
	result := tasks.List(c, tasks.ListFilter{Project: "ide"})
	issues, ok := orm.Cast[[]tasks.Issue](result)
	core.RequireTrue(t, ok)
	core.AssertLen(t, issues, 1)
}

func TestApi_List_Bad(t *core.T) {
	c := newTestCore(t)
	result := tasks.List(c, tasks.ListFilter{Project: "none"})
	issues, ok := orm.Cast[[]tasks.Issue](result)
	core.RequireTrue(t, ok)
	core.AssertLen(t, issues, 0)
}

func TestApi_List_Ugly(t *core.T) {
	c := newTestCore(t)
	tasks.Create(c, tasks.CreateInput{Project: "ide", Summary: "offset one"})
	tasks.Create(c, tasks.CreateInput{Project: "ide", Summary: "offset two"})
	result := tasks.List(c, tasks.ListFilter{State: tasks.StateOpen})
	issues, ok := orm.Cast[[]tasks.Issue](result)
	core.RequireTrue(t, ok)
	core.AssertLen(t, issues, 2)
}

func TestApi_Update_Good(t *core.T) {
	c := newTestCore(t)
	created := tasks.Create(c, tasks.CreateInput{Project: "ide", Summary: "update"})
	issue, _, _ := orm.Detail[tasks.Issue](created)
	result := tasks.Update(c, issue.ID, tasks.UpdateInput{State: tasks.StateInProgress})
	updated, _, ok := orm.Detail[tasks.Issue](result)
	core.RequireTrue(t, ok)
	core.AssertEqual(t, tasks.StateInProgress, updated.State)
}

func TestApi_Update_Bad(t *core.T) {
	c := newTestCore(t)
	result := tasks.Update(c, "missing", tasks.UpdateInput{State: tasks.StateDone})
	core.AssertFalse(t, result.OK)
}

func TestApi_Update_Ugly(t *core.T) {
	c := newTestCore(t)
	created := tasks.Create(c, tasks.CreateInput{Project: "ide", Summary: "close through update"})
	issue, _, _ := orm.Detail[tasks.Issue](created)
	result := tasks.Update(c, issue.ID, tasks.UpdateInput{State: tasks.StateDone})
	updated, _, ok := orm.Detail[tasks.Issue](result)
	core.RequireTrue(t, ok)
	core.AssertFalse(t, updated.ClosedAt.IsZero())
}

func TestApi_Close_Good(t *core.T) {
	c := newTestCore(t)
	created := tasks.Create(c, tasks.CreateInput{Project: "ide", Summary: "close"})
	issue, _, _ := orm.Detail[tasks.Issue](created)
	result := tasks.Close(c, issue.ID, "fixed")
	closed, _, ok := orm.Detail[tasks.Issue](result)
	core.RequireTrue(t, ok)
	core.AssertEqual(t, "fixed", closed.Resolution)
}

func TestApi_Close_Bad(t *core.T) {
	c := newTestCore(t)
	result := tasks.Close(c, "missing", "fixed")
	core.AssertFalse(t, result.OK)
}

func TestApi_Close_Ugly(t *core.T) {
	c := newTestCore(t)
	created := tasks.Create(c, tasks.CreateInput{Project: "ide", Summary: "close blank"})
	issue, _, _ := orm.Detail[tasks.Issue](created)
	result := tasks.Close(c, issue.ID, "")
	closed, _, ok := orm.Detail[tasks.Issue](result)
	core.RequireTrue(t, ok)
	core.AssertEqual(t, tasks.StateDone, closed.State)
}

func TestApi_AddNote_Good(t *core.T) {
	c := newTestCore(t)
	result := tasks.AddNote(c, "issue-1", "body", "vi")
	note, _, ok := orm.Detail[tasks.Note](result)
	core.RequireTrue(t, ok)
	core.AssertEqual(t, "issue-1", note.IssueID)
}

func TestApi_AddNote_Bad(t *core.T) {
	c := newTestCore(t)
	result := tasks.AddNote(c, "", "body", "vi")
	core.AssertFalse(t, result.OK)
}

func TestApi_AddNote_Ugly(t *core.T) {
	c := newTestCore(t)
	result := tasks.AddNote(c, "issue-1", "body", "")
	note, _, ok := orm.Detail[tasks.Note](result)
	core.RequireTrue(t, ok)
	core.AssertEqual(t, "", note.Author)
}

func TestApi_ListNotes_Good(t *core.T) {
	c := newTestCore(t)
	tasks.AddNote(c, "issue-1", "first", "vi")
	result := tasks.ListNotes(c, "issue-1")
	notes, ok := orm.Cast[[]tasks.Note](result)
	core.RequireTrue(t, ok)
	core.AssertLen(t, notes, 1)
}

func TestApi_ListNotes_Bad(t *core.T) {
	c := newTestCore(t)
	result := tasks.ListNotes(c, "missing")
	notes, ok := orm.Cast[[]tasks.Note](result)
	core.RequireTrue(t, ok)
	core.AssertLen(t, notes, 0)
}

func TestApi_ListNotes_Ugly(t *core.T) {
	c := newTestCore(t)
	tasks.AddNote(c, "issue-1", "first", "vi")
	tasks.AddNote(c, "issue-1", "second", "you")
	result := tasks.ListNotes(c, "issue-1")
	notes, ok := orm.Cast[[]tasks.Note](result)
	core.RequireTrue(t, ok)
	core.AssertLen(t, notes, 2)
	core.AssertEqual(t, "first", notes[0].Body)
	core.AssertEqual(t, "second", notes[1].Body)
}

// Mantis #1503 — Update must reject caller-supplied State/Severity/
// Priority values that fall outside the canonical closed enum sets in
// types.go. Per-field _Bad tests assert each gate fires independently;
// the _Good test pins that all canonical values still pass.

func TestTasks_Update_InvalidStateRejected_Bad(t *core.T) {
	c := newTestCore(t)
	created := tasks.Create(c, tasks.CreateInput{Project: "ide", Summary: "bad state"})
	issue, _, _ := orm.Detail[tasks.Issue](created)
	result := tasks.Update(c, issue.ID, tasks.UpdateInput{State: "wontfix"})
	core.AssertFalse(t, result.OK)
	// Untouched on disk.
	current, _, _ := orm.Detail[tasks.Issue](tasks.Get(c, issue.ID))
	core.AssertEqual(t, tasks.StateOpen, current.State)
}

func TestTasks_Update_InvalidSeverityRejected_Bad(t *core.T) {
	c := newTestCore(t)
	created := tasks.Create(c, tasks.CreateInput{Project: "ide", Summary: "bad severity"})
	issue, _, _ := orm.Detail[tasks.Issue](created)
	result := tasks.Update(c, issue.ID, tasks.UpdateInput{Severity: "p0"})
	core.AssertFalse(t, result.OK)
	current, _, _ := orm.Detail[tasks.Issue](tasks.Get(c, issue.ID))
	core.AssertEqual(t, tasks.SeverityMinor, current.Severity)
}

func TestTasks_Update_InvalidPriorityRejected_Bad(t *core.T) {
	c := newTestCore(t)
	created := tasks.Create(c, tasks.CreateInput{Project: "ide", Summary: "bad priority"})
	issue, _, _ := orm.Detail[tasks.Issue](created)
	result := tasks.Update(c, issue.ID, tasks.UpdateInput{Priority: "asap"})
	core.AssertFalse(t, result.OK)
	current, _, _ := orm.Detail[tasks.Issue](tasks.Get(c, issue.ID))
	core.AssertEqual(t, tasks.PriorityNormal, current.Priority)
}

// Mantis #1726 (Cerberus #64 F-6) — lifecycle gate: Update must reject
// state hops outside the allowedStateTransitions matrix and must clear
// ClosedAt when an issue is reopened from a terminal state.

// TestTasks_Update_InvalidTransition_RejectsBad asserts that a transition
// out of a terminal state to a non-allowed target is rejected with the
// typed sentinel and that on-disk state is preserved.
func TestTasks_Update_InvalidTransition_RejectsBad(t *core.T) {
	c := newTestCore(t)
	created := tasks.Create(c, tasks.CreateInput{Project: "ide", Summary: "bad hop"})
	issue, _, _ := orm.Detail[tasks.Issue](created)
	// open -> done is legal; cancel from done is NOT in the matrix.
	closeRes := tasks.Update(c, issue.ID, tasks.UpdateInput{State: tasks.StateDone})
	core.RequireTrue(t, closeRes.OK)
	hop := tasks.Update(c, issue.ID, tasks.UpdateInput{State: tasks.StateCancelled})
	core.AssertFalse(t, hop.OK)
	current, _, _ := orm.Detail[tasks.Issue](tasks.Get(c, issue.ID))
	core.AssertEqual(t, tasks.StateDone, current.State)
	core.AssertFalse(t, current.ClosedAt.IsZero())
}

// TestTasks_Update_ReopenClearsClosedAt_Good asserts that moving from a
// terminal state back to open clears the ClosedAt timestamp so the
// forensic record does not survive across the lifecycle break.
func TestTasks_Update_ReopenClearsClosedAt_Good(t *core.T) {
	c := newTestCore(t)
	created := tasks.Create(c, tasks.CreateInput{Project: "ide", Summary: "reopen"})
	issue, _, _ := orm.Detail[tasks.Issue](created)
	closeRes := tasks.Update(c, issue.ID, tasks.UpdateInput{State: tasks.StateDone})
	closed, _, _ := orm.Detail[tasks.Issue](closeRes)
	core.RequireTrue(t, !closed.ClosedAt.IsZero())
	reopenRes := tasks.Update(c, issue.ID, tasks.UpdateInput{State: tasks.StateOpen})
	core.RequireTrue(t, reopenRes.OK)
	reopened, _, _ := orm.Detail[tasks.Issue](reopenRes)
	core.AssertEqual(t, tasks.StateOpen, reopened.State)
	core.AssertTrue(t, reopened.ClosedAt.IsZero())
}

// TestTasks_Update_TransitionMatrix_Ugly walks the full matrix: every
// declared (from -> to) pair is accepted; every undeclared pair is
// rejected. Same-state moves are legal (idempotent updates).
func TestTasks_Update_TransitionMatrix_Ugly(t *core.T) {
	allStates := []string{tasks.StateOpen, tasks.StateInProgress, tasks.StateDone, tasks.StateCancelled}
	for _, from := range allStates {
		for _, to := range allStates {
			c := newTestCore(t)
			created := tasks.Create(c, tasks.CreateInput{Project: "ide", Summary: "walk " + from + "->" + to})
			issue, _, _ := orm.Detail[tasks.Issue](created)
			// Seed the from state via the allowed open -> from hop
			// (open is the universal entry edge).
			if from != tasks.StateOpen {
				seed := tasks.Update(c, issue.ID, tasks.UpdateInput{State: from})
				core.RequireTrue(t, seed.OK)
			}
			res := tasks.Update(c, issue.ID, tasks.UpdateInput{State: to})
			if tasks.IsAllowedStateTransition(from, to) {
				core.AssertTrue(t, res.OK)
			} else {
				core.AssertFalse(t, res.OK)
			}
		}
	}
}

func TestTasks_Update_AllCanonicalEnumsAccepted_Good(t *core.T) {
	c := newTestCore(t)
	for _, state := range []string{tasks.StateOpen, tasks.StateInProgress, tasks.StateDone, tasks.StateCancelled} {
		created := tasks.Create(c, tasks.CreateInput{Project: "ide", Summary: "state " + state})
		issue, _, _ := orm.Detail[tasks.Issue](created)
		result := tasks.Update(c, issue.ID, tasks.UpdateInput{State: state})
		core.AssertTrue(t, result.OK)
	}
	for _, sev := range []string{tasks.SeverityFeature, tasks.SeverityTrivial, tasks.SeverityText, tasks.SeverityTweak, tasks.SeverityMinor, tasks.SeverityMajor, tasks.SeverityCrash, tasks.SeverityBlock} {
		created := tasks.Create(c, tasks.CreateInput{Project: "ide", Summary: "sev " + sev})
		issue, _, _ := orm.Detail[tasks.Issue](created)
		result := tasks.Update(c, issue.ID, tasks.UpdateInput{Severity: sev})
		core.AssertTrue(t, result.OK)
	}
	for _, pri := range []string{tasks.PriorityNone, tasks.PriorityLow, tasks.PriorityNormal, tasks.PriorityHigh, tasks.PriorityUrgent, tasks.PriorityImmediate} {
		created := tasks.Create(c, tasks.CreateInput{Project: "ide", Summary: "pri " + pri})
		issue, _, _ := orm.Detail[tasks.Issue](created)
		result := tasks.Update(c, issue.ID, tasks.UpdateInput{Priority: pri})
		core.AssertTrue(t, result.OK)
	}
}

// --- B2 tier-auth gate + ENFORCE-not-claim ---------------------------------
//
// Mantis #1721 Phase 1 B2 / Mantis #1722 HIGH — pkg/tasks Wails surface
// adopts the auth substrate per RFC.tier-auth-substrate.md v2 §5.1 +
// §5.3. The Wails entry points (Create / Update / AddNote / Get / List)
// now gate caller-tier via auth.Require and OVERWRITE caller-derived
// attribution fields (Reporter / Author) from auth.Caller(c).Subject
// rather than reading them from the input struct (F-pattern A).

// TestTasks_Create_ReporterFromCaller_Good asserts that the Wails
// Service.Create OVERWRITES input.Reporter from auth.Caller(c).Subject
// — a renderer-tier caller claiming Reporter="bob" lands a record with
// Reporter="alice" (the stamped subject). This is the F-pattern A
// worked example: forge attempts at the Wails IPC surface are
// neutralised at the substrate boundary, not after the data lands.
func TestTasks_Create_ReporterFromCaller_Good(t *core.T) {
	c := newTestCore(t)
	auth.SetCaller(c, auth.CallerIdentity{
		Tier:    auth.TierRenderer,
		Subject: "alice",
		Source:  "wails",
	})
	svc := tasks.NewService(c)
	r := svc.Create(tasks.CreateIssueInput{
		Project:  "lthn",
		Summary:  "reporter forgery",
		Reporter: "bob", // hostile claim
	})
	core.RequireTrue(t, r.OK)
	issue, _, ok := orm.Detail[tasks.Issue](r)
	core.RequireTrue(t, ok)
	core.AssertEqual(t, "alice", issue.Reporter)
}

// TestTasks_AddNote_AuthorFromCaller_Good asserts that the Wails
// Service.AddNote OVERWRITES input.Author from auth.Caller(c).Subject
// — a renderer claiming Author="cerberus" lands a Note with
// Author="alice" (the stamped subject). Mirrors the Create gate; this
// is the activity-log forgery prevention (planting false trail entries).
func TestTasks_AddNote_AuthorFromCaller_Good(t *core.T) {
	c := newTestCore(t)
	auth.SetCaller(c, auth.CallerIdentity{
		Tier:    auth.TierRenderer,
		Subject: "alice",
		Source:  "wails",
	})
	svc := tasks.NewService(c)
	r := svc.AddNote(tasks.AddNoteInput{
		IssueID: "issue-x",
		Body:    "false attribution",
		Author:  "cerberus", // hostile claim
	})
	core.RequireTrue(t, r.OK)
	note, _, ok := orm.Detail[tasks.Note](r)
	core.RequireTrue(t, ok)
	core.AssertEqual(t, "alice", note.Author)
}

// TestTasks_Create_InternalTier_Rejected_Bad asserts the Require gate
// rejects callers stamped at the TierInternal floor (the zero-value
// resolution) for write operations. The Wails surface permits ONLY
// {TierOperator, TierRenderer}; intra-Go (TierInternal), cascade
// handlers (TierCascade), and cron-worker (TierCron) lands a visible
// Fail rather than a silent forgery surface.
func TestTasks_Create_InternalTier_Rejected_Bad(t *core.T) {
	c := core.New()
	core.RequireTrue(t, orm.Register(c).OK)
	mem := orm.NewMemium()
	core.RequireTrue(t, orm.Mount(c, "default", mem).OK)
	for _, schema := range tasks.Schemas() {
		core.RequireTrue(t, orm.RegisterSchema(c, schema).OK)
		mem.RegisterTable(schema.Name, schema)
	}
	// NB: no auth.SetCaller — Caller resolves to TierInternal floor.
	svc := tasks.NewService(c)
	r := svc.Create(tasks.CreateIssueInput{
		Project: "lthn",
		Summary: "internal tier should be rejected at Wails surface",
	})
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "tier_not_permitted")
}

// TestTasks_Update_CascadeTier_Rejected_Bad asserts the Update gate
// rejects TierCascade — write operations are operator/renderer-only
// per RFC §5.1 table row. A cascade handler that tried to write to
// tasks via the Wails surface would land this deny path; cascade
// handlers MUST go through the api.go package-level Update which is
// TierInternal-callable, not through the Wails Service shim.
func TestTasks_Update_CascadeTier_Rejected_Bad(t *core.T) {
	c := newTestCore(t)
	created := tasks.Create(c, tasks.CreateInput{Project: "lthn", Summary: "seed"})
	issue, _, _ := orm.Detail[tasks.Issue](created)
	auth.SetCaller(c, auth.CallerIdentity{
		Tier:    auth.TierCascade,
		Subject: "renderer-source-account-id",
		Source:  "queue-worker",
	})
	svc := tasks.NewService(c)
	r := svc.Update(tasks.UpdateIssueInput{ID: issue.ID, State: tasks.StateInProgress})
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "tier_not_permitted")
}

// TestTasks_List_AllowsCascade_Good asserts the read gate is broader
// than the write gate per RFC §5.1: List/Get permit TierCascade so
// audit / agent-prep / PR-watcher subscribers can fetch issue state
// from inside cascade handlers without needing operator elevation.
func TestTasks_List_AllowsCascade_Good(t *core.T) {
	c := newTestCore(t)
	tasks.Create(c, tasks.CreateInput{Project: "lthn", Summary: "for cascade read"})
	auth.SetCaller(c, auth.CallerIdentity{
		Tier:    auth.TierCascade,
		Subject: "renderer-source-account-id",
		Source:  "queue-worker",
	})
	svc := tasks.NewService(c)
	r := svc.List(tasks.ListInput{Project: "lthn"})
	core.RequireTrue(t, r.OK)
	out, ok := r.Value.(tasks.ListOutput)
	core.RequireTrue(t, ok)
	core.AssertEqual(t, 1, out.Count)
}

// TestTasks_AddNote_InternalTier_Rejected_Ugly walks the third write
// surface (Note creation) to confirm the gate is consistent across
// all three Wails entry points that touch persistence. Cascade-only
// audit-trail inspectors that try to plant Notes via the Wails surface
// land in the same deny path Create / Update use.
func TestTasks_AddNote_InternalTier_Rejected_Ugly(t *core.T) {
	c := core.New()
	core.RequireTrue(t, orm.Register(c).OK)
	mem := orm.NewMemium()
	core.RequireTrue(t, orm.Mount(c, "default", mem).OK)
	for _, schema := range tasks.Schemas() {
		core.RequireTrue(t, orm.RegisterSchema(c, schema).OK)
		mem.RegisterTable(schema.Name, schema)
	}
	svc := tasks.NewService(c)
	r := svc.AddNote(tasks.AddNoteInput{IssueID: "issue-x", Body: "internal note"})
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "tier_not_permitted")
}
