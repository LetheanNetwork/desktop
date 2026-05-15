// SPDX-Licence-Identifier: EUPL-1.2

package opencode

import (
	"net/http/httptest"

	core "dappco.re/go"
)

// TestSubscribe_streamEvents_Good — happy path: emitter receives every
// "data:" line; comments + id lines are skipped; clean EOF returns nil.
func TestSubscribe_streamEvents_Good(t *core.T) {
	server := httptest.NewServer(core.HandlerFunc(func(w core.ResponseWriter, r *core.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(core.StatusOK)
		f, _ := w.(core.Flusher)
		_, _ = w.Write([]byte(": comment-line-should-be-ignored\n"))
		_, _ = w.Write([]byte("id: 123\n"))
		_, _ = w.Write([]byte(`data: {"type":"server.connected"}` + "\n"))
		_, _ = w.Write([]byte(`data: {"type":"session.created","id":"ses_1"}` + "\n"))
		_, _ = w.Write([]byte(`data:    {"type":"message.part"}` + "\n")) // leading whitespace
		if f != nil {
			f.Flush()
		}
	}))
	defer server.Close()

	svc := &Service{}
	var got []string
	var mu core.Mutex
	svc.SetEventEmitter(func(e string) {
		mu.Lock()
		got = append(got, e)
		mu.Unlock()
	})

	ctx, cancel := core.WithTimeout(core.Background(), 2*core.Second)
	defer cancel()
	if err := svc.streamEvents(ctx, server.URL, ""); err != nil {
		t.Fatalf("streamEvents err: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(got) != 3 {
		t.Fatalf("got %d events, want 3: %v", len(got), got)
	}
	for i, want := range []string{
		`{"type":"server.connected"}`,
		`{"type":"session.created","id":"ses_1"}`,
		`{"type":"message.part"}`,
	} {
		if got[i] != want {
			t.Errorf("event %d: got %q want %q", i, got[i], want)
		}
	}
}

// TestSubscribe_streamEvents_Bad — 4xx upstream surfaces as an error.
func TestSubscribe_streamEvents_Bad(t *core.T) {
	server := httptest.NewServer(core.HandlerFunc(func(w core.ResponseWriter, r *core.Request) {
		w.WriteHeader(core.StatusUnauthorized)
	}))
	defer server.Close()

	svc := &Service{}
	svc.SetEventEmitter(func(string) {})
	ctx, cancel := core.WithTimeout(core.Background(), 2*core.Second)
	defer cancel()
	err := svc.streamEvents(ctx, server.URL, "Basic deadbeef")
	if err == nil {
		t.Fatalf("expected error on 401")
	}
	if !core.Contains(err.Error(), "401") {
		t.Errorf("want 401 in error, got %v", err)
	}
}

// TestSubscribe_streamEvents_Ugly — context cancellation mid-stream
// terminates promptly without panic.
func TestSubscribe_streamEvents_Ugly(t *core.T) {
	server := httptest.NewServer(core.HandlerFunc(func(w core.ResponseWriter, r *core.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(core.StatusOK)
		f, _ := w.(core.Flusher)
		// Slow drip — emit one event then sleep past test timeout.
		_, _ = w.Write([]byte(`data: {"x":1}` + "\n"))
		if f != nil {
			f.Flush()
		}
		core.Sleep(5 * core.Second)
	}))
	defer server.Close()

	svc := &Service{}
	got := make(chan string, 4)
	svc.SetEventEmitter(func(e string) { got <- e })

	ctx, cancel := core.WithCancel(core.Background())
	done := make(chan error, 1)
	go func() {
		done <- svc.streamEvents(ctx, server.URL, "")
	}()

	// Wait for the first event, then cancel.
	select {
	case <-got:
	case <-core.After(2 * core.Second):
		cancel()
		<-done
		t.Fatalf("never received first event")
	}
	cancel()

	select {
	case <-done:
	case <-core.After(2 * core.Second):
		t.Fatalf("cancellation didn't terminate streamEvents promptly")
	}
}
