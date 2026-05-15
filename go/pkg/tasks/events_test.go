// SPDX-Licence-Identifier: EUPL-1.2

package tasks_test

import (
	core "dappco.re/go"
	"dappco.re/go/orm"
	"dappco.re/lthn/desktop/pkg/tasks"
)

// newEventsCore is a test-local Core with orm wired and the tasks
// schemas registered. Mirrors newTestCore in api_test.go but kept
// separate so events tests can run in isolation.
func newEventsCore(t *core.T) *core.Core {
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

// TestEvents_Subscribe_Created — Create fires KindCreated with the
// new Issue and zero-value Before.
func TestEvents_Subscribe_Created(t *core.T) {
	c := newEventsCore(t)

	var got []tasks.IssueChanged
	var mu core.Mutex
	tasks.Subscribe(c, func(_ *core.Core, ev tasks.IssueChanged) {
		mu.Lock()
		got = append(got, ev)
		mu.Unlock()
	})

	r := tasks.Create(c, tasks.CreateInput{
		Project: "lthn", Summary: "first event",
	})
	core.RequireTrue(t, r.OK)

	mu.Lock()
	defer mu.Unlock()
	core.AssertLen(t, got, 1)
	ev := got[0]
	core.AssertEqual(t, tasks.KindCreated, ev.Kind)
	core.AssertEqual(t, "first event", ev.Issue.Summary)
	core.AssertEqual(t, "", ev.Before.ID)
}

// TestEvents_Update_FiresUpdated — Update with a non-closing state
// change fires KindUpdated with both Before and Issue populated.
func TestEvents_Update_FiresUpdated(t *core.T) {
	c := newEventsCore(t)

	r := tasks.Create(c, tasks.CreateInput{Project: "lthn", Summary: "u"})
	core.RequireTrue(t, r.OK)
	created := r.Value.(tasks.Issue)

	var got []tasks.IssueChanged
	var mu core.Mutex
	tasks.Subscribe(c, func(_ *core.Core, ev tasks.IssueChanged) {
		mu.Lock()
		got = append(got, ev)
		mu.Unlock()
	})

	r = tasks.Update(c, created.ID, tasks.UpdateInput{
		State: tasks.StateInProgress,
	})
	core.RequireTrue(t, r.OK)

	mu.Lock()
	defer mu.Unlock()
	core.AssertLen(t, got, 1)
	ev := got[0]
	core.AssertEqual(t, tasks.KindUpdated, ev.Kind)
	core.AssertEqual(t, tasks.StateOpen, ev.Before.State)
	core.AssertEqual(t, tasks.StateInProgress, ev.Issue.State)
}

// TestEvents_Close_FiresClosed — a state transition to StateDone
// fires KindClosed (not KindUpdated).
func TestEvents_Close_FiresClosed(t *core.T) {
	c := newEventsCore(t)

	r := tasks.Create(c, tasks.CreateInput{Project: "lthn", Summary: "c"})
	created := r.Value.(tasks.Issue)

	var seen string
	tasks.Subscribe(c, func(_ *core.Core, ev tasks.IssueChanged) {
		seen = ev.Kind
	})

	core.RequireTrue(t, tasks.Close(c, created.ID, "fixed").OK)
	core.AssertEqual(t, tasks.KindClosed, seen)
}

// TestEvents_AddNote_FiresNoted — appending a Note fires KindNoted
// with the parent Issue snapshot AND the new Note populated.
func TestEvents_AddNote_FiresNoted(t *core.T) {
	c := newEventsCore(t)

	r := tasks.Create(c, tasks.CreateInput{Project: "lthn", Summary: "n"})
	parent := r.Value.(tasks.Issue)

	var got tasks.IssueChanged
	tasks.Subscribe(c, func(_ *core.Core, ev tasks.IssueChanged) {
		if ev.Kind == tasks.KindNoted {
			got = ev
		}
	})

	core.RequireTrue(t, tasks.AddNote(c, parent.ID, "a comment", "tester").OK)
	core.AssertEqual(t, tasks.KindNoted, got.Kind)
	core.AssertEqual(t, "a comment", got.Note.Body)
	core.AssertEqual(t, parent.ID, got.Issue.ID)
}

// TestEvents_MultipleSubscribers — every registered listener receives
// every event; one bad listener doesn't take the cascade down.
func TestEvents_MultipleSubscribers(t *core.T) {
	c := newEventsCore(t)

	var aCount, bCount int
	var mu core.Mutex
	tasks.Subscribe(c, func(*core.Core, tasks.IssueChanged) {
		mu.Lock()
		aCount++
		mu.Unlock()
	})
	tasks.Subscribe(c, func(*core.Core, tasks.IssueChanged) {
		// Panic to confirm Core's recover keeps the cascade alive.
		panic("simulated bad subscriber")
	})
	tasks.Subscribe(c, func(*core.Core, tasks.IssueChanged) {
		mu.Lock()
		bCount++
		mu.Unlock()
	})

	core.RequireTrue(t, tasks.Create(c, tasks.CreateInput{Project: "lthn", Summary: "m"}).OK)
	// Allow microscopic time for any goroutine fan-out (Core's
	// broadcast is sync today but defensive).
	core.Sleep(5 * core.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	core.AssertEqual(t, 1, aCount)
	core.AssertEqual(t, 1, bCount)
}
