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

// TestEvents_Subscribe_Good — a single subscriber receives every
// IssueChanged broadcast, populated with the Issue snapshot AFTER
// the change. Covers the canonical create path: KindCreated event
// with zero-value Before.
func TestEvents_Subscribe_Good(t *core.T) {
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

// TestEvents_Subscribe_Bad — Subscribe with a nil Core or nil
// callback is a defensive no-op (returns early without panicking
// or registering anything). Test asserts AssertNotPanics across
// both nil paths.
func TestEvents_Subscribe_Bad(t *core.T) {
	core.AssertNotPanics(t, func() {
		tasks.Subscribe(nil, func(*core.Core, tasks.IssueChanged) {})
	})
	c := newEventsCore(t)
	core.AssertNotPanics(t, func() {
		tasks.Subscribe(c, nil)
	})
}

// TestEvents_Subscribe_Ugly — fan-out covers the four Kind variants
// (KindCreated / KindUpdated / KindClosed / KindNoted), exercises
// multiple registered subscribers, and proves Core's panic-recover
// keeps the cascade alive when one subscriber panics.
func TestEvents_Subscribe_Ugly(t *core.T) {
	c := newEventsCore(t)

	var seen []string
	var mu core.Mutex
	record := func(_ *core.Core, ev tasks.IssueChanged) {
		mu.Lock()
		seen = append(seen, ev.Kind)
		mu.Unlock()
	}

	tasks.Subscribe(c, record)
	tasks.Subscribe(c, func(*core.Core, tasks.IssueChanged) {
		panic("simulated bad subscriber — Core's recover keeps cascade alive")
	})
	var aCount int
	tasks.Subscribe(c, func(*core.Core, tasks.IssueChanged) {
		mu.Lock()
		aCount++
		mu.Unlock()
	})

	// Cover every Kind by walking an Issue through its lifecycle.
	r := tasks.Create(c, tasks.CreateInput{Project: "lthn", Summary: "u"})
	core.RequireTrue(t, r.OK)
	created := r.Value.(tasks.Issue)

	core.RequireTrue(t, tasks.Update(c, created.ID, tasks.UpdateInput{
		State: tasks.StateInProgress,
	}).OK)
	core.RequireTrue(t, tasks.AddNote(c, created.ID, "comment", "tester").OK)
	core.RequireTrue(t, tasks.Close(c, created.ID, "fixed").OK)

	// Tiny pause for any goroutine fan-out (Core broadcast is sync today
	// but defensive). Then assert four distinct Kinds reached the recorder.
	core.Sleep(5 * core.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	core.AssertLen(t, seen, 4)
	core.AssertEqual(t, tasks.KindCreated, seen[0])
	core.AssertEqual(t, tasks.KindUpdated, seen[1])
	core.AssertEqual(t, tasks.KindNoted, seen[2])
	core.AssertEqual(t, tasks.KindClosed, seen[3])
	// Healthy subscriber count matches event count — panic in the
	// middle subscriber doesn't take the third one down.
	core.AssertEqual(t, 4, aCount)
}
