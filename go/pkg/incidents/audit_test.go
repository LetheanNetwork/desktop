// SPDX-Licence-Identifier: EUPL-1.2

// audit_test.go — internal-package cover for AttachAudit +
// mapEventNameToAuditEvent (Stage F.B Phase 2.4 cross-cut). Mirrors
// pkg/tasks/audit_test.go's fakeAuditRecorder pattern (Snider
// 2026-07: "each writer pkg owns its own gate/fixture" — duplicated
// per-pkg rather than shared).

package incidents

import (
	"testing"

	core "dappco.re/go"

	"dappco.re/lthn/desktop/pkg/audit"
)

// fakeAuditRecorder is an in-memory audit.Recorder fixture — stores
// every Record call so tests can assert on the projected event shape
// without touching ~/Lethean/audit/.
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

// TestAudit_MapEventNameToAuditEvent_Good pins the two mapped
// event-names to their reserved audit constants.
func TestAudit_MapEventNameToAuditEvent_Good(t *testing.T) {
	cases := map[string]string{
		EventOpened:       AuditEventCreated,
		EventTransitioned: AuditEventTransitioned,
	}
	for name, want := range cases {
		if got := mapEventNameToAuditEvent(name); got != want {
			t.Errorf("mapEventNameToAuditEvent(%q) = %q, want %q", name, got, want)
		}
	}
}

// TestAudit_MapEventNameToAuditEvent_Bad proves an unmapped/unknown
// event-name (and the empty string) returns "" rather than a stale
// guess — the doc-comment's "future additions don't accidentally land
// in the audit stream" contract.
func TestAudit_MapEventNameToAuditEvent_Bad(t *testing.T) {
	for _, name := range []string{"bogus", "", "incidents.deleted"} {
		if got := mapEventNameToAuditEvent(name); got != "" {
			t.Errorf("mapEventNameToAuditEvent(%q) = %q, want empty", name, got)
		}
	}
}

// TestAudit_AttachAudit_Nil_Bad — AttachAudit(nil) must not panic; it
// is a documented no-op guard.
func TestAudit_AttachAudit_Nil_Bad(t *testing.T) {
	AttachAudit(nil) // must not panic
}

// TestAudit_AttachAudit_Good walks the canonical Opened→Transitioned
// pair and asserts the projected Event carries the mapped name, the
// incident ID/state in Meta, and OutcomeOK.
func TestAudit_AttachAudit_Good(t *testing.T) {
	fake := withFakeAuditRecorder(t)
	c := core.New()
	AttachAudit(c)

	entry := IncidentEntry{ID: "2026-08-INC-001", State: "investigating"}
	c.ACTION(IncidentEvent{EventName: EventOpened, Entry: entry, At: core.Now()})

	if len(fake.events) != 1 {
		t.Fatalf("events = %d, want 1", len(fake.events))
	}
	ev := fake.events[0]
	if ev.Event != AuditEventCreated {
		t.Errorf("Event = %q, want %q", ev.Event, AuditEventCreated)
	}
	if ev.Scope != "incidents.transition" {
		t.Errorf("Scope = %q, want incidents.transition", ev.Scope)
	}
	if ev.Outcome != audit.OutcomeOK {
		t.Errorf("Outcome = %q, want %q", ev.Outcome, audit.OutcomeOK)
	}
	if ev.Meta["incident_id"] != "2026-08-INC-001" {
		t.Errorf("Meta[incident_id] = %v, want 2026-08-INC-001", ev.Meta["incident_id"])
	}
	if ev.Meta["state"] != "investigating" {
		t.Errorf("Meta[state] = %v, want investigating", ev.Meta["state"])
	}

	c.ACTION(IncidentEvent{EventName: EventTransitioned, Entry: entry, At: core.Now()})
	if len(fake.events) != 2 {
		t.Fatalf("events = %d, want 2 after transition", len(fake.events))
	}
	if fake.events[1].Event != AuditEventTransitioned {
		t.Errorf("Event = %q, want %q", fake.events[1].Event, AuditEventTransitioned)
	}
}

// TestAudit_AttachAudit_NonIncidentMessageIgnored_Bad — the subscriber
// must return OK-and-skip for any broadcast message that is not an
// IncidentEvent (the bus is untyped; every handler sees every
// message).
func TestAudit_AttachAudit_NonIncidentMessageIgnored_Bad(t *testing.T) {
	fake := withFakeAuditRecorder(t)
	c := core.New()
	AttachAudit(c)

	type otherMessage struct{ X int }
	c.ACTION(otherMessage{X: 1})

	if len(fake.events) != 0 {
		t.Errorf("events = %d, want 0 for a non-IncidentEvent broadcast", len(fake.events))
	}
}
