// SPDX-Licence-Identifier: EUPL-1.2

package opencode

import (
	"errors"
	"testing"

	"dappco.re/lthn/desktop/pkg/audit"
)

// --- allocatePort -------------------------------------------------

// TestAllocatePort_HappyPath_Good — a single probe that returns nil
// must succeed on attempt 1 with no retry/exhausted audit emission.
// Pins the Mantis #1604 fix's first-try shape so a regression to the
// old port-0 idiom (which would always-succeed without probing) is
// caught by the audit-silence assertion.
func TestAllocatePort_HappyPath_Good(t *testing.T) {
	fake := &inMemoryRecorder{}
	audit.SetDefault(fake)
	t.Cleanup(func() { audit.SetDefault(nil) })

	origProbe := portProbe
	origPick := pickPortInRange
	t.Cleanup(func() {
		portProbe = origProbe
		pickPortInRange = origPick
	})
	pickPortInRange = func() int { return 50000 }
	portProbe = func(port int) error { return nil }

	r := allocatePort()
	if !r.OK {
		t.Fatalf("allocatePort failed on free port: %v", r.Error())
	}
	port, ok := r.Value.(int)
	if !ok || port != 50000 {
		t.Fatalf("allocatePort returned %v (%T); want int 50000", r.Value, r.Value)
	}
	if got := len(fake.snapshot()); got != 0 {
		t.Fatalf("happy path emitted %d audit events; want 0", got)
	}
}

// TestAllocatePort_PortInRange_Good — the returned port MUST sit
// inside the IANA dynamic/private range so docker bind targets the
// ephemeral pool the OS itself uses (Cerberus #22 forward-arc note).
// Drives the real portProbe / pickPortInRange against a fresh
// allocation so the live range-math is exercised, not the mock.
func TestAllocatePort_PortInRange_Good(t *testing.T) {
	r := allocatePort()
	if !r.OK {
		t.Fatalf("allocatePort failed on real probe: %v", r.Error())
	}
	port, ok := r.Value.(int)
	if !ok {
		t.Fatalf("allocatePort returned %T; want int", r.Value)
	}
	if port < OpencodeHostPortRangeStart || port > OpencodeHostPortRangeEnd {
		t.Fatalf("port %d outside [%d, %d]",
			port, OpencodeHostPortRangeStart, OpencodeHostPortRangeEnd)
	}
}

// TestAllocatePort_RetryOnEADDRINUSE_Good — the first N probes return
// EADDRINUSE, the (N+1)th returns nil. Allocation must succeed AND
// emit exactly N retry audit events with attempt + port + reason
// metadata. Pins the bounded-tolerance shape that distinguishes this
// fix from a fail-fast or unbounded-loop alternative.
func TestAllocatePort_RetryOnEADDRINUSE_Good(t *testing.T) {
	fake := &inMemoryRecorder{}
	audit.SetDefault(fake)
	t.Cleanup(func() { audit.SetDefault(nil) })

	origProbe := portProbe
	origPick := pickPortInRange
	t.Cleanup(func() {
		portProbe = origProbe
		pickPortInRange = origPick
	})

	calls := 0
	pickPortInRange = func() int {
		calls++
		return 50000 + calls
	}
	portProbe = func(port int) error {
		if calls <= 3 {
			return errors.New("listen tcp 127.0.0.1:X: bind: address already in use")
		}
		return nil
	}

	r := allocatePort()
	if !r.OK {
		t.Fatalf("allocatePort failed after retries: %v", r.Error())
	}
	port, _ := r.Value.(int)
	if port != 50004 {
		t.Fatalf("returned port = %d; want 50004 (succeeded on 4th attempt)", port)
	}
	events := fake.snapshot()
	if len(events) != 3 {
		t.Fatalf("emitted %d audit events; want 3 retries before success", len(events))
	}
	for i, ev := range events {
		if ev.Event != eventOpencodePortRetry {
			t.Errorf("event[%d].Event = %q; want %q", i, ev.Event, eventOpencodePortRetry)
		}
		if ev.Scope != "opencode.port" {
			t.Errorf("event[%d].Scope = %q; want %q", i, ev.Scope, "opencode.port")
		}
		if ev.Outcome != audit.OutcomeError {
			t.Errorf("event[%d].Outcome = %q; want %q", i, ev.Outcome, audit.OutcomeError)
		}
		if got, _ := ev.Meta["attempt"].(int); got != i+1 {
			t.Errorf("event[%d].Meta.attempt = %v; want %d", i, ev.Meta["attempt"], i+1)
		}
		if got, _ := ev.Meta["port"].(int); got != 50001+i {
			t.Errorf("event[%d].Meta.port = %v; want %d", i, ev.Meta["port"], 50001+i)
		}
		if reason, _ := ev.Meta["reason"].(string); reason == "" {
			t.Errorf("event[%d].Meta.reason missing", i)
		}
	}
}

// TestAllocatePort_ExhaustedAfterMax_Bad — every probe returns
// EADDRINUSE; allocation must Fail with the typed
// "opencode.allocatePort" / "port range exhausted" shape AND emit
// exactly OpencodeHostPortRetryMax retry events + one exhausted
// event. Pins the bounded-loop guarantee — without it, a hostile
// adversary could trap the allocator forever.
func TestAllocatePort_ExhaustedAfterMax_Bad(t *testing.T) {
	fake := &inMemoryRecorder{}
	audit.SetDefault(fake)
	t.Cleanup(func() { audit.SetDefault(nil) })

	origProbe := portProbe
	origPick := pickPortInRange
	t.Cleanup(func() {
		portProbe = origProbe
		pickPortInRange = origPick
	})
	pickPortInRange = func() int { return 49999 }
	portProbe = func(port int) error {
		return errors.New("listen tcp 127.0.0.1:X: bind: address already in use")
	}

	r := allocatePort()
	if r.OK {
		t.Fatalf("allocatePort returned OK on all-busy; want Fail")
	}
	if msg := r.Error(); !contains(msg, "port range exhausted") {
		t.Fatalf("error %q missing 'port range exhausted' marker", msg)
	}

	events := fake.snapshot()
	wantRetries := OpencodeHostPortRetryMax
	if len(events) != wantRetries+1 {
		t.Fatalf("emitted %d events; want %d retry + 1 exhausted",
			len(events), wantRetries)
	}
	last := events[len(events)-1]
	if last.Event != eventOpencodePortExhausted {
		t.Errorf("last event = %q; want %q", last.Event, eventOpencodePortExhausted)
	}
	if got, _ := last.Meta["attempts"].(int); got != OpencodeHostPortRetryMax {
		t.Errorf("exhausted.Meta.attempts = %v; want %d",
			last.Meta["attempts"], OpencodeHostPortRetryMax)
	}
	if rng, _ := last.Meta["range"].(string); rng == "" {
		t.Errorf("exhausted.Meta.range missing")
	}
}
