// SPDX-Licence-Identifier: EUPL-1.2

// service_lifecycle_test.go — Register / PausePolling / ResumePolling
// / StartPolling + the relativeWhen future-timestamp branch. None of
// these are exercised anywhere else in the package.

package mail

import (
	"testing"
	"time"

	core "dappco.re/go"
)

// TestRegister_Good — Register wires a *Service into the Core
// container and returns it via core.Ok.
func TestRegister_Good(t *testing.T) {
	c := core.New()
	r := Register(c)
	if !r.OK {
		t.Fatalf("Register: %s", r.Error())
	}
	svc, ok := r.Value.(*Service)
	if !ok || svc == nil {
		t.Fatalf("Register did not return a *Service")
	}
	if svc.ServiceName() != "Mail" {
		t.Errorf("ServiceName = %q, want Mail", svc.ServiceName())
	}
}

// TestPausePolling_Good — flips paused, fires EventSessionLocked with
// the deferred-poll count in the message.
func TestPausePolling_Good(t *testing.T) {
	c := core.New()
	svc := NewService(c)
	svc.deferredPolls = 3

	var got MailEvent
	Subscribe(c, func(_ *core.Core, ev MailEvent) { got = ev })

	r := svc.PausePolling(c)
	if !r.OK {
		t.Fatalf("PausePolling: %s", r.Error())
	}
	if !svc.paused.Load() {
		t.Error("expected paused=true after PausePolling")
	}
	if got.Kind != EventSessionLocked {
		t.Errorf("expected EventSessionLocked, got %q", got.Kind)
	}
	if !core.Contains(got.Error, "3 folders queued") {
		t.Errorf("expected deferred count in message, got %q", got.Error)
	}
}

// TestResumePolling_Good — flips paused off, resets deferredPolls.
func TestResumePolling_Good(t *testing.T) {
	c := core.New()
	svc := NewService(c)
	svc.paused.Store(true)
	svc.deferredPolls = 5

	r := svc.ResumePolling(c)
	if !r.OK {
		t.Fatalf("ResumePolling: %s", r.Error())
	}
	if svc.paused.Load() {
		t.Error("expected paused=false after ResumePolling")
	}
	if svc.deferredPolls != 0 {
		t.Errorf("expected deferredPolls reset to 0, got %d", svc.deferredPolls)
	}
}

// TestStartPolling_Good — currently a no-op that always returns Ok;
// pinned so a future real implementation can't silently start
// failing this entrypoint.
func TestStartPolling_Good(t *testing.T) {
	c := core.New()
	svc := NewService(c)
	r := svc.StartPolling(c)
	if !r.OK {
		t.Fatalf("StartPolling: %s", r.Error())
	}
}

// TestRelativeWhen_FutureTimestamp_Ugly — a timestamp AFTER now (diff
// < 0) still resolves via the abs()-then-bucket logic rather than
// underflowing to a negative bucket.
func TestRelativeWhen_FutureTimestamp_Ugly(t *testing.T) {
	now := core.Now()
	future := now.Add(2 * time.Minute)
	got := relativeWhen(future, now)
	if got != "now" {
		t.Fatalf("expected 'now' for a near-future timestamp, got %q", got)
	}
}
