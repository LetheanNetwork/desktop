// SPDX-Licence-Identifier: EUPL-1.2

package tasks_test

import (
	"sync"
	"testing"
	"time"

	core "dappco.re/go"
	"dappco.re/go/orm"
	"dappco.re/lthn/desktop/pkg/tasks"
)

// newEventsCore is a test-local Core with orm wired and the tasks
// schemas registered. Mirrors newTestCore in api_test.go but kept
// separate so events tests can run in isolation.
func newEventsCore(t *testing.T) *core.Core {
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

// TestEvents_Subscribe_Created — Create fires KindCreated with the
// new Issue and zero-value Before.
func TestEvents_Subscribe_Created(t *testing.T) {
	c := newEventsCore(t)

	var got []tasks.IssueChanged
	var mu sync.Mutex
	tasks.Subscribe(c, func(_ *core.Core, ev tasks.IssueChanged) {
		mu.Lock()
		got = append(got, ev)
		mu.Unlock()
	})

	r := tasks.Create(c, tasks.CreateInput{
		Project: "lthn", Summary: "first event",
	})
	if !r.OK {
		t.Fatalf("create: %s", r.Error())
	}

	mu.Lock()
	defer mu.Unlock()
	if len(got) != 1 {
		t.Fatalf("got %d events, want 1", len(got))
	}
	ev := got[0]
	if ev.Kind != tasks.KindCreated {
		t.Errorf("kind: got %q, want %q", ev.Kind, tasks.KindCreated)
	}
	if ev.Issue.Summary != "first event" {
		t.Errorf("issue.summary: got %q", ev.Issue.Summary)
	}
	if ev.Before.ID != "" {
		t.Errorf("before should be zero-value on Create, got %+v", ev.Before)
	}
}

// TestEvents_Update_FiresUpdated — Update with a non-closing state
// change fires KindUpdated with both Before and Issue populated.
func TestEvents_Update_FiresUpdated(t *testing.T) {
	c := newEventsCore(t)

	r := tasks.Create(c, tasks.CreateInput{Project: "lthn", Summary: "u"})
	if !r.OK {
		t.Fatalf("create: %s", r.Error())
	}
	created := r.Value.(tasks.Issue)

	var got []tasks.IssueChanged
	var mu sync.Mutex
	tasks.Subscribe(c, func(_ *core.Core, ev tasks.IssueChanged) {
		mu.Lock()
		got = append(got, ev)
		mu.Unlock()
	})

	r = tasks.Update(c, created.ID, tasks.UpdateInput{
		State: tasks.StateInProgress,
	})
	if !r.OK {
		t.Fatalf("update: %s", r.Error())
	}

	mu.Lock()
	defer mu.Unlock()
	if len(got) != 1 {
		t.Fatalf("got %d events, want 1", len(got))
	}
	ev := got[0]
	if ev.Kind != tasks.KindUpdated {
		t.Errorf("kind: got %q, want %q", ev.Kind, tasks.KindUpdated)
	}
	if ev.Before.State != tasks.StateOpen {
		t.Errorf("before.state: got %q, want %q", ev.Before.State, tasks.StateOpen)
	}
	if ev.Issue.State != tasks.StateInProgress {
		t.Errorf("issue.state: got %q, want %q", ev.Issue.State, tasks.StateInProgress)
	}
}

// TestEvents_Close_FiresClosed — a state transition to StateDone
// fires KindClosed (not KindUpdated).
func TestEvents_Close_FiresClosed(t *testing.T) {
	c := newEventsCore(t)

	r := tasks.Create(c, tasks.CreateInput{Project: "lthn", Summary: "c"})
	created := r.Value.(tasks.Issue)

	var seen string
	tasks.Subscribe(c, func(_ *core.Core, ev tasks.IssueChanged) {
		seen = ev.Kind
	})

	if r := tasks.Close(c, created.ID, "fixed"); !r.OK {
		t.Fatalf("close: %s", r.Error())
	}
	if seen != tasks.KindClosed {
		t.Errorf("kind: got %q, want %q", seen, tasks.KindClosed)
	}
}

// TestEvents_AddNote_FiresNoted — appending a Note fires KindNoted
// with the parent Issue snapshot AND the new Note populated.
func TestEvents_AddNote_FiresNoted(t *testing.T) {
	c := newEventsCore(t)

	r := tasks.Create(c, tasks.CreateInput{Project: "lthn", Summary: "n"})
	parent := r.Value.(tasks.Issue)

	var got tasks.IssueChanged
	tasks.Subscribe(c, func(_ *core.Core, ev tasks.IssueChanged) {
		if ev.Kind == tasks.KindNoted {
			got = ev
		}
	})

	if r := tasks.AddNote(c, parent.ID, "a comment", "tester"); !r.OK {
		t.Fatalf("add note: %s", r.Error())
	}
	if got.Kind != tasks.KindNoted {
		t.Fatalf("kind: got %q, want %q", got.Kind, tasks.KindNoted)
	}
	if got.Note.Body != "a comment" {
		t.Errorf("note.body: got %q", got.Note.Body)
	}
	if got.Issue.ID != parent.ID {
		t.Errorf("parent issue not attached: got %q want %q", got.Issue.ID, parent.ID)
	}
}

// TestEvents_MultipleSubscribers — every registered listener receives
// every event; one bad listener doesn't take the cascade down.
func TestEvents_MultipleSubscribers(t *testing.T) {
	c := newEventsCore(t)

	var aCount, bCount int
	var mu sync.Mutex
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

	if r := tasks.Create(c, tasks.CreateInput{Project: "lthn", Summary: "m"}); !r.OK {
		t.Fatalf("create: %s", r.Error())
	}
	// Allow microscopic time for any goroutine fan-out (Core's
	// broadcast is sync today but defensive).
	time.Sleep(5 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if aCount != 1 || bCount != 1 {
		t.Fatalf("each non-panicking listener should fire once; got a=%d b=%d", aCount, bCount)
	}
}
