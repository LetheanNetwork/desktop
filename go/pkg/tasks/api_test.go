// SPDX-License-Identifier: EUPL-1.2

package tasks_test

import (

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/tasks"
	"dappco.re/go/orm"
)

// newTestCore returns a *core.Core with orm registered + a fresh Memium
// mounted under "default" + the tasks schemas registered. Tests use this
// to exercise the package end-to-end without any DuckDB / filesystem.
func newTestCore(t *core.T) *core.Core {
	t.Helper()
	c := core.New()
	if r := orm.Register(c); !r.OK {
		t.Fatalf("orm.Register: %s", r.Error())
	}
	mem := orm.NewMemium()
	if r := orm.Mount(c, "default", mem); !r.OK {
		t.Fatalf("orm.Mount: %s", r.Error())
	}
	for _, schema := range tasks.Schemas() {
		if r := orm.RegisterSchema(c, schema); !r.OK {
			t.Fatalf("orm.RegisterSchema: %s", r.Error())
		}
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
	if !r.OK {
		t.Fatalf("Create: %s", r.Error())
	}
	issue, _, ok := orm.Detail[tasks.Issue](r)
	if !ok {
		t.Fatal("Detail cast failed")
	}
	if issue.ID == "" {
		t.Error("expected non-empty ID")
	}
	if issue.State != tasks.StateOpen {
		t.Errorf("expected state=%q, got %q", tasks.StateOpen, issue.State)
	}
	if issue.Severity != tasks.SeverityMinor {
		t.Errorf("expected severity=%q, got %q", tasks.SeverityMinor, issue.Severity)
	}
	if issue.Priority != tasks.PriorityNormal {
		t.Errorf("expected priority=%q, got %q", tasks.PriorityNormal, issue.Priority)
	}
	if issue.CreatedAt.IsZero() {
		t.Error("expected non-zero CreatedAt")
	}
}

func TestTasks_Create_Bad_MissingProject(t *core.T) {
	c := newTestCore(t)

	r := tasks.Create(c, tasks.CreateInput{Summary: "no project"})
	if r.OK {
		t.Fatal("expected failure when project is empty")
	}
}

func TestTasks_Create_Bad_MissingSummary(t *core.T) {
	c := newTestCore(t)

	r := tasks.Create(c, tasks.CreateInput{Project: "ide"})
	if r.OK {
		t.Fatal("expected failure when summary is empty")
	}
}

func TestTasks_Get_Good_RoundTrip(t *core.T) {
	c := newTestCore(t)
	created := tasks.Create(c, tasks.CreateInput{Project: "ide", Summary: "round trip"})
	if !created.OK {
		t.Fatalf("Create: %s", created.Error())
	}
	createdIssue, _, _ := orm.Detail[tasks.Issue](created)

	r := tasks.Get(c, createdIssue.ID)
	if !r.OK {
		t.Fatalf("Get: %s", r.Error())
	}
	issue, _, ok := orm.Detail[tasks.Issue](r)
	if !ok {
		t.Fatal("Detail cast failed")
	}
	if issue.Summary != "round trip" {
		t.Errorf("expected summary=%q, got %q", "round trip", issue.Summary)
	}
}

func TestTasks_Get_Bad_NotFound(t *core.T) {
	c := newTestCore(t)

	r := tasks.Get(c, "missing-id")
	if r.OK {
		t.Fatal("expected not-found failure")
	}
}

func TestTasks_List_Good_FiltersByProject(t *core.T) {
	c := newTestCore(t)
	tasks.Create(c, tasks.CreateInput{Project: "ide", Summary: "ide one"})
	tasks.Create(c, tasks.CreateInput{Project: "ide", Summary: "ide two"})
	tasks.Create(c, tasks.CreateInput{Project: "store", Summary: "store one"})

	r := tasks.List(c, tasks.ListFilter{Project: "ide"})
	if !r.OK {
		t.Fatalf("List: %s", r.Error())
	}
	issues, ok := orm.Cast[[]tasks.Issue](r)
	if !ok {
		t.Fatal("Cast to []Issue failed")
	}
	if len(issues) != 2 {
		t.Errorf("expected 2 issues, got %d", len(issues))
	}
	for _, issue := range issues {
		if issue.Project != "ide" {
			t.Errorf("expected project=ide, got %q", issue.Project)
		}
	}
}

func TestTasks_Update_Good_StateTransitionSetsClosedAt(t *core.T) {
	c := newTestCore(t)
	created := tasks.Create(c, tasks.CreateInput{Project: "ide", Summary: "to be closed"})
	if !created.OK {
		t.Fatalf("Create: %s", created.Error())
	}
	createdIssue, _, _ := orm.Detail[tasks.Issue](created)

	r := tasks.Update(c, createdIssue.ID, tasks.UpdateInput{State: tasks.StateDone})
	if !r.OK {
		t.Fatalf("Update: %s", r.Error())
	}
	updated, _, _ := orm.Detail[tasks.Issue](r)
	if updated.State != tasks.StateDone {
		t.Errorf("expected state=%q, got %q", tasks.StateDone, updated.State)
	}
	if updated.ClosedAt.IsZero() {
		t.Error("expected ClosedAt set on state=done")
	}
}

func TestTasks_Close_Good_SetsResolution(t *core.T) {
	c := newTestCore(t)
	created := tasks.Create(c, tasks.CreateInput{Project: "ide", Summary: "to close"})
	if !created.OK {
		t.Fatalf("Create: %s", created.Error())
	}
	createdIssue, _, _ := orm.Detail[tasks.Issue](created)

	r := tasks.Close(c, createdIssue.ID, "fixed")
	if !r.OK {
		t.Fatalf("Close: %s", r.Error())
	}
	closed, _, _ := orm.Detail[tasks.Issue](r)
	if closed.State != tasks.StateDone {
		t.Errorf("expected state=done, got %q", closed.State)
	}
	if closed.Resolution != "fixed" {
		t.Errorf("expected resolution=fixed, got %q", closed.Resolution)
	}
}

func TestTasks_AddNote_Good_PersistsAndLists(t *core.T) {
	c := newTestCore(t)
	created := tasks.Create(c, tasks.CreateInput{Project: "ide", Summary: "note me"})
	createdIssue, _, _ := orm.Detail[tasks.Issue](created)

	if r := tasks.AddNote(c, createdIssue.ID, "first comment", "cladius"); !r.OK {
		t.Fatalf("AddNote: %s", r.Error())
	}
	if r := tasks.AddNote(c, createdIssue.ID, "second comment", "snider"); !r.OK {
		t.Fatalf("AddNote: %s", r.Error())
	}

	r := tasks.ListNotes(c, createdIssue.ID)
	if !r.OK {
		t.Fatalf("ListNotes: %s", r.Error())
	}
	notes, ok := orm.Cast[[]tasks.Note](r)
	if !ok {
		t.Fatal("Cast to []Note failed")
	}
	if len(notes) != 2 {
		t.Errorf("expected 2 notes, got %d", len(notes))
	}
}

func TestTasks_AddNote_Bad_EmptyBody(t *core.T) {
	c := newTestCore(t)

	r := tasks.AddNote(c, "any-id", "", "cladius")
	if r.OK {
		t.Fatal("expected failure on empty body")
	}
}

func TestApi_Create_Good(t *core.T) {
	c := newTestCore(t)
	result := tasks.Create(c, tasks.CreateInput{Project: "ide", Summary: "create good"})
	if !result.OK {
		t.Fatalf("Create: %s", result.Error())
	}
	issue, _, ok := orm.Detail[tasks.Issue](result)
	if !ok || issue.Project != "ide" {
		t.Fatalf("Create: unexpected issue %#v", issue)
	}
}

func TestApi_Create_Bad(t *core.T) {
	c := newTestCore(t)
	result := tasks.Create(c, tasks.CreateInput{Summary: "missing project"})
	if result.OK {
		t.Fatal("Create: expected missing project failure")
	}
}

func TestApi_Create_Ugly(t *core.T) {
	c := newTestCore(t)
	result := tasks.Create(c, tasks.CreateInput{Project: "ide", Summary: "defaults"})
	issue, _, ok := orm.Detail[tasks.Issue](result)
	if !ok || issue.State != tasks.StateOpen || issue.Priority != tasks.PriorityNormal {
		t.Fatalf("Create: defaults drifted %#v", issue)
	}
}

func TestApi_Get_Good(t *core.T) {
	c := newTestCore(t)
	created := tasks.Create(c, tasks.CreateInput{Project: "ide", Summary: "get good"})
	issue, _, _ := orm.Detail[tasks.Issue](created)
	result := tasks.Get(c, issue.ID)
	if !result.OK {
		t.Fatalf("Get: %s", result.Error())
	}
}

func TestApi_Get_Bad(t *core.T) {
	c := newTestCore(t)
	result := tasks.Get(c, "missing")
	if result.OK {
		t.Fatal("Get: expected missing issue failure")
	}
}

func TestApi_Get_Ugly(t *core.T) {
	c := newTestCore(t)
	created := tasks.Create(c, tasks.CreateInput{Project: "ide", Summary: "get ugly"})
	issue, _, _ := orm.Detail[tasks.Issue](created)
	found, _, ok := orm.Detail[tasks.Issue](tasks.Get(c, issue.ID))
	if !ok || found.ID != issue.ID {
		t.Fatalf("Get: round-trip ID mismatch %#v", found)
	}
}

func TestApi_List_Good(t *core.T) {
	c := newTestCore(t)
	tasks.Create(c, tasks.CreateInput{Project: "ide", Summary: "one"})
	tasks.Create(c, tasks.CreateInput{Project: "other", Summary: "two"})
	result := tasks.List(c, tasks.ListFilter{Project: "ide"})
	issues, ok := orm.Cast[[]tasks.Issue](result)
	if !ok || len(issues) != 1 {
		t.Fatalf("List: expected one ide issue, got %#v", issues)
	}
}

func TestApi_List_Bad(t *core.T) {
	c := newTestCore(t)
	result := tasks.List(c, tasks.ListFilter{Project: "none"})
	issues, ok := orm.Cast[[]tasks.Issue](result)
	if !ok || len(issues) != 0 {
		t.Fatalf("List: expected empty result, got %#v", issues)
	}
}

func TestApi_List_Ugly(t *core.T) {
	c := newTestCore(t)
	tasks.Create(c, tasks.CreateInput{Project: "ide", Summary: "offset one"})
	tasks.Create(c, tasks.CreateInput{Project: "ide", Summary: "offset two"})
	result := tasks.List(c, tasks.ListFilter{State: tasks.StateOpen})
	issues, ok := orm.Cast[[]tasks.Issue](result)
	if !ok || len(issues) != 2 {
		t.Fatalf("List: state filter expected two open issues, got %#v", issues)
	}
}

func TestApi_Update_Good(t *core.T) {
	c := newTestCore(t)
	created := tasks.Create(c, tasks.CreateInput{Project: "ide", Summary: "update"})
	issue, _, _ := orm.Detail[tasks.Issue](created)
	result := tasks.Update(c, issue.ID, tasks.UpdateInput{State: tasks.StateInProgress})
	updated, _, ok := orm.Detail[tasks.Issue](result)
	if !ok || updated.State != tasks.StateInProgress {
		t.Fatalf("Update: state not changed %#v", updated)
	}
}

func TestApi_Update_Bad(t *core.T) {
	c := newTestCore(t)
	result := tasks.Update(c, "missing", tasks.UpdateInput{State: tasks.StateDone})
	if result.OK {
		t.Fatal("Update: expected missing issue failure")
	}
}

func TestApi_Update_Ugly(t *core.T) {
	c := newTestCore(t)
	created := tasks.Create(c, tasks.CreateInput{Project: "ide", Summary: "close through update"})
	issue, _, _ := orm.Detail[tasks.Issue](created)
	result := tasks.Update(c, issue.ID, tasks.UpdateInput{State: tasks.StateDone})
	updated, _, ok := orm.Detail[tasks.Issue](result)
	if !ok || updated.ClosedAt.IsZero() {
		t.Fatalf("Update: done transition should set ClosedAt %#v", updated)
	}
}

func TestApi_Close_Good(t *core.T) {
	c := newTestCore(t)
	created := tasks.Create(c, tasks.CreateInput{Project: "ide", Summary: "close"})
	issue, _, _ := orm.Detail[tasks.Issue](created)
	result := tasks.Close(c, issue.ID, "fixed")
	closed, _, ok := orm.Detail[tasks.Issue](result)
	if !ok || closed.Resolution != "fixed" {
		t.Fatalf("Close: expected fixed resolution %#v", closed)
	}
}

func TestApi_Close_Bad(t *core.T) {
	c := newTestCore(t)
	result := tasks.Close(c, "missing", "fixed")
	if result.OK {
		t.Fatal("Close: expected missing issue failure")
	}
}

func TestApi_Close_Ugly(t *core.T) {
	c := newTestCore(t)
	created := tasks.Create(c, tasks.CreateInput{Project: "ide", Summary: "close blank"})
	issue, _, _ := orm.Detail[tasks.Issue](created)
	result := tasks.Close(c, issue.ID, "")
	closed, _, ok := orm.Detail[tasks.Issue](result)
	if !ok || closed.State != tasks.StateDone {
		t.Fatalf("Close: should mark done even with blank resolution %#v", closed)
	}
}

func TestApi_AddNote_Good(t *core.T) {
	c := newTestCore(t)
	result := tasks.AddNote(c, "issue-1", "body", "vi")
	note, _, ok := orm.Detail[tasks.Note](result)
	if !ok || note.IssueID != "issue-1" {
		t.Fatalf("AddNote: unexpected note %#v", note)
	}
}

func TestApi_AddNote_Bad(t *core.T) {
	c := newTestCore(t)
	result := tasks.AddNote(c, "", "body", "vi")
	if result.OK {
		t.Fatal("AddNote: expected missing issue failure")
	}
}

func TestApi_AddNote_Ugly(t *core.T) {
	c := newTestCore(t)
	result := tasks.AddNote(c, "issue-1", "body", "")
	note, _, ok := orm.Detail[tasks.Note](result)
	if !ok || note.Author != "" {
		t.Fatalf("AddNote: blank author should be accepted %#v", note)
	}
}

func TestApi_ListNotes_Good(t *core.T) {
	c := newTestCore(t)
	tasks.AddNote(c, "issue-1", "first", "vi")
	result := tasks.ListNotes(c, "issue-1")
	notes, ok := orm.Cast[[]tasks.Note](result)
	if !ok || len(notes) != 1 {
		t.Fatalf("ListNotes: expected one note, got %#v", notes)
	}
}

func TestApi_ListNotes_Bad(t *core.T) {
	c := newTestCore(t)
	result := tasks.ListNotes(c, "missing")
	notes, ok := orm.Cast[[]tasks.Note](result)
	if !ok || len(notes) != 0 {
		t.Fatalf("ListNotes: expected empty result, got %#v", notes)
	}
}

func TestApi_ListNotes_Ugly(t *core.T) {
	c := newTestCore(t)
	tasks.AddNote(c, "issue-1", "first", "vi")
	tasks.AddNote(c, "issue-1", "second", "you")
	result := tasks.ListNotes(c, "issue-1")
	notes, ok := orm.Cast[[]tasks.Note](result)
	if !ok || notes[0].Body != "first" || notes[1].Body != "second" {
		t.Fatalf("ListNotes: expected oldest-first notes, got %#v", notes)
	}
}
