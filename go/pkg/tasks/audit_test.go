// SPDX-Licence-Identifier: EUPL-1.2

package tasks

import (
	"testing"

	core "dappco.re/go"
	"dappco.re/go/orm"

	"dappco.re/lthn/desktop/pkg/audit"
)

// fakeAuditRecorder is an in-memory audit.Recorder fixture (mirrors
// pkg/audit's own inMemoryRecorder test fixture) — stores every Record
// call so tests can assert on the projected event shape without
// touching ~/Lethean/audit/.
type fakeAuditRecorder struct {
	events []audit.Event
}

func (f *fakeAuditRecorder) Record(ev audit.Event) core.Result {
	f.events = append(f.events, ev)
	return core.Ok(nil)
}

// withFakeAuditRecorder installs a fresh fakeAuditRecorder as the
// package-level audit.Default() and registers cleanup to reset it —
// audit.Default() is a process-global singleton (RWMutex-guarded), so
// every AttachAudit test must isolate its own recorder.
func withFakeAuditRecorder(t *testing.T) *fakeAuditRecorder {
	t.Helper()
	fake := &fakeAuditRecorder{}
	audit.SetDefault(fake)
	t.Cleanup(func() { audit.SetDefault(nil) })
	return fake
}

func TestAudit_MapKindToAuditEvent_Good(t *testing.T) {
	cases := map[string]string{
		KindCreated: AuditEventCreated,
		KindUpdated: AuditEventTransitioned,
		KindNoted:   AuditEventTransitioned,
		KindClosed:  AuditEventTransitioned,
	}
	for kind, want := range cases {
		if got := mapKindToAuditEvent(kind); got != want {
			t.Errorf("mapKindToAuditEvent(%q) = %q, want %q", kind, got, want)
		}
	}
}

func TestAudit_MapKindToAuditEvent_Bad(t *testing.T) {
	if got := mapKindToAuditEvent("bogus"); got != "" {
		t.Errorf("mapKindToAuditEvent(bogus) = %q, want empty", got)
	}
	if got := mapKindToAuditEvent(""); got != "" {
		t.Errorf("mapKindToAuditEvent(empty) = %q, want empty", got)
	}
}

// TestAudit_AttachAudit_Good walks the canonical Create path and
// asserts the projected Event carries the mapped name, the Project as
// Scope, and OutcomeOK.
func TestAudit_AttachAudit_Good(t *testing.T) {
	fake := withFakeAuditRecorder(t)
	c := newDetectCore(t)
	AttachAudit(c)

	r := Create(c, CreateInput{Project: "lthn", Summary: "audit me"})
	if !r.OK {
		t.Fatalf("Create: %s", r.Error())
	}
	if len(fake.events) != 1 {
		t.Fatalf("events = %d, want 1", len(fake.events))
	}
	ev := fake.events[0]
	if ev.Event != AuditEventCreated {
		t.Errorf("Event = %q, want %q", ev.Event, AuditEventCreated)
	}
	if ev.Scope != "lthn" {
		t.Errorf("Scope = %q, want lthn", ev.Scope)
	}
	if ev.Outcome != audit.OutcomeOK {
		t.Errorf("Outcome = %q, want %q", ev.Outcome, audit.OutcomeOK)
	}
	if ev.Meta["issue_id"] == "" {
		t.Error("Meta[issue_id] should be populated")
	}
}

// TestAudit_AttachAudit_UnmappedKindSkipped_Bad proves the "" guard in
// AttachAudit's subscriber fires — a Kind absent from
// mapKindToAuditEvent's switch never reaches Record. Every real Kind
// constant IS mapped today, so this exercises the defensive branch via
// a synthetic broadcast rather than a production call path.
func TestAudit_AttachAudit_UnmappedKindSkipped_Bad(t *testing.T) {
	fake := withFakeAuditRecorder(t)
	c := newDetectCore(t)
	AttachAudit(c)

	c.ACTION(IssueChanged{Kind: "bogus-kind", Issue: Issue{Project: "lthn"}, At: core.Now()})
	if len(fake.events) != 0 {
		t.Errorf("events = %d, want 0 for an unmapped Kind", len(fake.events))
	}
}

// TestAudit_AttachAudit_TransitionsAndDoubleAttach_Ugly walks the full
// lifecycle (Create/Update/AddNote/Close — the four Kind variants) and
// pins the documented double-attach behaviour: AttachAudit is not
// idempotent, so calling it twice on the same Core doubles every
// emission (AttachAudit's own doc-comment: "re-attaching multiplies
// emissions").
func TestAudit_AttachAudit_TransitionsAndDoubleAttach_Ugly(t *testing.T) {
	fake := withFakeAuditRecorder(t)
	c := newDetectCore(t)
	AttachAudit(c)
	AttachAudit(c)

	created := Create(c, CreateInput{Project: "lthn", Summary: "walk"})
	issue, _, ok := orm.Detail[Issue](created)
	if !ok {
		t.Fatal("cast created issue")
	}
	if r := Update(c, issue.ID, UpdateInput{State: StateInProgress}); !r.OK {
		t.Fatalf("Update: %s", r.Error())
	}
	if r := AddNote(c, issue.ID, "note", "vi"); !r.OK {
		t.Fatalf("AddNote: %s", r.Error())
	}
	if r := Close(c, issue.ID, "fixed"); !r.OK {
		t.Fatalf("Close: %s", r.Error())
	}

	// 4 lifecycle moments × 2 attachments = 8 records.
	if len(fake.events) != 8 {
		t.Fatalf("events = %d, want 8 (double-attach multiplies)", len(fake.events))
	}
	counts := map[string]int{}
	for _, ev := range fake.events {
		counts[ev.Event]++
	}
	if counts[AuditEventCreated] != 2 {
		t.Errorf("AuditEventCreated count = %d, want 2", counts[AuditEventCreated])
	}
	// KindUpdated + KindNoted + KindClosed all map to Transitioned:
	// 3 lifecycle moments × 2 attachments = 6.
	if counts[AuditEventTransitioned] != 6 {
		t.Errorf("AuditEventTransitioned count = %d, want 6", counts[AuditEventTransitioned])
	}
}
