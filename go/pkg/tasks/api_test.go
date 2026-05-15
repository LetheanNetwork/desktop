// SPDX-License-Identifier: EUPL-1.2

package tasks_test

import (
	core "dappco.re/go"
	"dappco.re/go/orm"
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

func TestTasks_Create_Good_PopulatesDefaults(t *core.T) {
	c := newTestCore(t)

	r := tasks.Create(c, tasks.CreateInput{
		Project: "ide",
		Summary: "wire tasks panel",
	})
	core.RequireTrue(t, r.OK)
	issue, _, ok := orm.Detail[tasks.Issue](r)
	core.RequireTrue(t, ok)
	core.AssertNotEmpty(t, issue.ID)
	core.AssertEqual(t, tasks.StateOpen, issue.State)
	core.AssertEqual(t, tasks.SeverityMinor, issue.Severity)
	core.AssertEqual(t, tasks.PriorityNormal, issue.Priority)
	core.AssertFalse(t, issue.CreatedAt.IsZero())
}

func TestTasks_Create_Bad_MissingProject(t *core.T) {
	c := newTestCore(t)
	r := tasks.Create(c, tasks.CreateInput{Summary: "no project"})
	core.AssertFalse(t, r.OK)
}

func TestTasks_Create_Bad_MissingSummary(t *core.T) {
	c := newTestCore(t)
	r := tasks.Create(c, tasks.CreateInput{Project: "ide"})
	core.AssertFalse(t, r.OK)
}

func TestTasks_Get_Good_RoundTrip(t *core.T) {
	c := newTestCore(t)
	created := tasks.Create(c, tasks.CreateInput{Project: "ide", Summary: "round trip"})
	core.RequireTrue(t, created.OK)
	createdIssue, _, _ := orm.Detail[tasks.Issue](created)

	r := tasks.Get(c, createdIssue.ID)
	core.RequireTrue(t, r.OK)
	issue, _, ok := orm.Detail[tasks.Issue](r)
	core.RequireTrue(t, ok)
	core.AssertEqual(t, "round trip", issue.Summary)
}

func TestTasks_Get_Bad_NotFound(t *core.T) {
	c := newTestCore(t)
	r := tasks.Get(c, "missing-id")
	core.AssertFalse(t, r.OK)
}

func TestTasks_List_Good_FiltersByProject(t *core.T) {
	c := newTestCore(t)
	tasks.Create(c, tasks.CreateInput{Project: "ide", Summary: "ide one"})
	tasks.Create(c, tasks.CreateInput{Project: "ide", Summary: "ide two"})
	tasks.Create(c, tasks.CreateInput{Project: "store", Summary: "store one"})

	r := tasks.List(c, tasks.ListFilter{Project: "ide"})
	core.RequireTrue(t, r.OK)
	issues, ok := orm.Cast[[]tasks.Issue](r)
	core.RequireTrue(t, ok)
	core.AssertLen(t, issues, 2)
	for _, issue := range issues {
		core.AssertEqual(t, "ide", issue.Project)
	}
}

func TestTasks_Update_Good_StateTransitionSetsClosedAt(t *core.T) {
	c := newTestCore(t)
	created := tasks.Create(c, tasks.CreateInput{Project: "ide", Summary: "to be closed"})
	core.RequireTrue(t, created.OK)
	createdIssue, _, _ := orm.Detail[tasks.Issue](created)

	r := tasks.Update(c, createdIssue.ID, tasks.UpdateInput{State: tasks.StateDone})
	core.RequireTrue(t, r.OK)
	updated, _, _ := orm.Detail[tasks.Issue](r)
	core.AssertEqual(t, tasks.StateDone, updated.State)
	core.AssertFalse(t, updated.ClosedAt.IsZero())
}

func TestTasks_Close_Good_SetsResolution(t *core.T) {
	c := newTestCore(t)
	created := tasks.Create(c, tasks.CreateInput{Project: "ide", Summary: "to close"})
	core.RequireTrue(t, created.OK)
	createdIssue, _, _ := orm.Detail[tasks.Issue](created)

	r := tasks.Close(c, createdIssue.ID, "fixed")
	core.RequireTrue(t, r.OK)
	closed, _, _ := orm.Detail[tasks.Issue](r)
	core.AssertEqual(t, tasks.StateDone, closed.State)
	core.AssertEqual(t, "fixed", closed.Resolution)
}

func TestTasks_AddNote_Good_PersistsAndLists(t *core.T) {
	c := newTestCore(t)
	created := tasks.Create(c, tasks.CreateInput{Project: "ide", Summary: "note me"})
	createdIssue, _, _ := orm.Detail[tasks.Issue](created)

	core.RequireTrue(t, tasks.AddNote(c, createdIssue.ID, "first comment", "cladius").OK)
	core.RequireTrue(t, tasks.AddNote(c, createdIssue.ID, "second comment", "snider").OK)

	r := tasks.ListNotes(c, createdIssue.ID)
	core.RequireTrue(t, r.OK)
	notes, ok := orm.Cast[[]tasks.Note](r)
	core.RequireTrue(t, ok)
	core.AssertLen(t, notes, 2)
}

func TestTasks_AddNote_Bad_EmptyBody(t *core.T) {
	c := newTestCore(t)
	r := tasks.AddNote(c, "any-id", "", "cladius")
	core.AssertFalse(t, r.OK)
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
