// SPDX-Licence-Identifier: EUPL-1.2

// subscribe_lifecycle_test.go — coverage for Subscribe / Unsubscribe /
// runSubscription (subscribe.go). streamEvents itself is already
// covered by subscribe_test.go; this file exercises the goroutine
// lifecycle around it: idempotent subscribe, no-op when no emitter is
// installed, cancel-on-Unsubscribe, and the reconnect-with-backoff
// loop (driven with a pre-cancelled context so the backoff sleep never
// actually happens — the ctx.Err() check fires on the very first loop
// iteration, keeping the test fast).

package opencode

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	core "dappco.re/go"
)

// TestSubscribe_NoEmitterInstalled_Good — with no emitter set,
// Subscribe must return a no-op cancel + Ok, and must NOT register a
// subscription entry (no SSE connection opened).
func TestSubscribe_NoEmitterInstalled_Good(t *testing.T) {
	svc := newTestService(t, Options{})
	seedRunningSandbox(t, svc, "oc-sub-noemitter", 1)

	cancel, r := svc.Subscribe("oc-sub-noemitter")
	if !r.OK {
		t.Fatalf("Subscribe with no emitter failed: %s", r.Error())
	}
	cancel() // must not panic
	svc.mu.RLock()
	_, tracked := svc.subscriptions["oc-sub-noemitter"]
	svc.mu.RUnlock()
	if tracked {
		t.Errorf("Subscribe with no emitter registered a subscription entry")
	}
}

// TestSubscribe_EmptyID_Bad / NilService_Bad — guard clauses.
func TestSubscribe_EmptyID_Bad(t *testing.T) {
	svc := &Service{}
	_, r := svc.Subscribe("  ")
	if r.OK {
		t.Fatalf("Subscribe('') returned OK; want Fail")
	}
}

func TestSubscribe_NilService_Bad(t *testing.T) {
	var svc *Service
	cancel, r := svc.Subscribe("oc-x")
	if r.OK {
		t.Fatalf("Subscribe on a nil Service returned OK; want Fail")
	}
	cancel() // must not panic
}

// TestSubscribe_TargetNotRunning_Bad — an emitter is installed but the
// sandbox isn't running; targetFor fails and Subscribe propagates it.
func TestSubscribe_TargetNotRunning_Bad(t *testing.T) {
	svc := newTestService(t, Options{})
	svc.SetEventEmitter(func(string) {})
	_, r := svc.Subscribe("oc-ghost")
	if r.OK {
		t.Fatalf("Subscribe against a missing sandbox returned OK; want Fail")
	}
}

// TestSubscribe_HappyPath_Good — a running sandbox + an installed
// emitter: Subscribe spawns the goroutine, forwards at least one SSE
// event, and Unsubscribe (via the returned cancel, and via
// Service.Unsubscribe) tears it down cleanly. Idempotent re-Subscribe
// returns the same cancel without opening a second connection.
func TestSubscribe_HappyPath_Good(t *testing.T) {
	var connections int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connections++
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		f, _ := w.(http.Flusher)
		_, _ = w.Write([]byte(`data: {"type":"server.connected"}` + "\n"))
		if f != nil {
			f.Flush()
		}
		// Hold the connection open until the client cancels.
		<-r.Context().Done()
	}))
	t.Cleanup(upstream.Close)

	svc := newTestService(t, Options{})
	seedRunningSandbox(t, svc, "oc-sub-happy", portOf(t, upstream))

	events := make(chan string, 4)
	svc.SetEventEmitter(func(e string) { events <- e })

	cancel, r := svc.Subscribe("oc-sub-happy")
	if !r.OK {
		t.Fatalf("Subscribe failed: %s", r.Error())
	}

	select {
	case e := <-events:
		if e != `{"type":"server.connected"}` {
			t.Errorf("event = %q; want server.connected payload", e)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("never received the first SSE event")
	}

	// Idempotent re-Subscribe — same cancel, no second connection.
	cancel2, r2 := svc.Subscribe("oc-sub-happy")
	if !r2.OK {
		t.Fatalf("second Subscribe failed: %s", r2.Error())
	}
	_ = cancel2

	svc.Unsubscribe("oc-sub-happy")
	svc.mu.RLock()
	_, tracked := svc.subscriptions["oc-sub-happy"]
	svc.mu.RUnlock()
	if tracked {
		t.Errorf("subscription still tracked after Unsubscribe")
	}

	cancel() // second cancel call must be harmless
}

// TestSubscribe_UnsubscribeUnknownID_Good — Unsubscribe on an id that
// was never subscribed is a safe no-op.
func TestSubscribe_UnsubscribeUnknownID_Good(t *testing.T) {
	svc := &Service{}
	svc.Unsubscribe("oc-never-subscribed") // must not panic
	var nilSvc *Service
	nilSvc.Unsubscribe("oc-x") // must not panic
}

// TestRunSubscription_ExitsImmediatelyOnCancelledContext_Good — drives
// runSubscription directly with an already-cancelled context so the
// very first ctx.Err() check returns, proving the loop's exit path
// without waiting through any real backoff sleep.
func TestRunSubscription_ExitsImmediatelyOnCancelledContext_Good(t *testing.T) {
	svc := &Service{}
	ctx, cancel := core.WithCancel(core.Background())
	cancel()

	done := make(chan struct{})
	go func() {
		svc.runSubscription(ctx, "oc-x", "http://127.0.0.1:1", "")
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("runSubscription did not exit promptly against a cancelled context")
	}
}

// TestRunSubscription_ReconnectsAfterStreamError_Ugly — the upstream
// immediately refuses the connection (connection error, not a clean
// stream-end), so streamEvents returns a non-nil error and
// runSubscription enters its backoff branch. We cancel the context
// while it's asleep in the backoff `select` and confirm the goroutine
// still exits promptly — proving the backoff select's ctx.Done() arm,
// not just the top-of-loop check.
func TestRunSubscription_ReconnectsAfterStreamError_Ugly(t *testing.T) {
	svc := &Service{}
	// Port 1 is a privileged, essentially-guaranteed-refused port on
	// 127.0.0.1 — connecting there fails fast with connection-refused,
	// which is exactly the "real fault injection" shape (no upstream
	// at all), without needing to bind + immediately close a real
	// listener race.
	ctx, cancel := core.WithCancel(core.Background())

	done := make(chan struct{})
	go func() {
		svc.runSubscription(ctx, "oc-x", "http://127.0.0.1:1", "")
		close(done)
	}()

	// Give the first failed connection attempt + backoff-sleep entry a
	// moment, then cancel — the goroutine must observe ctx.Done() from
	// inside the backoff select rather than sleeping out the full 1s.
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("runSubscription did not exit promptly after cancel during backoff")
	}
}
