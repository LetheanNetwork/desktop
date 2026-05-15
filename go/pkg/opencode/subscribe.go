// SPDX-Licence-Identifier: EUPL-1.2

// Subscribe — consumes opencode-serve's `GET /global/event` SSE
// stream for a running sandbox and forwards each event to a
// caller-supplied emitter. Per RFC.opencode.md §4.3 + §5.3.
//
// The emitter callback decouples opencode from the Wails app:
// pkg/desktop installs an emitter that bridges to the Wails event
// bus ("opencode:event"); CLI/server modes leave the emitter unset
// so the SSE goroutines never start (no consumer = wasted work).
//
// Lifecycle:
//
//   - Service.SetEventEmitter installs (or clears) the callback.
//   - Service.Subscribe(id) opens the SSE stream for one sandbox
//     + spawns a goroutine that forwards every "data: <json>" line
//     to the emitter. Returns a cancel function that tears the
//     goroutine + connection down.
//   - Service.Start auto-subscribes when an emitter is installed.
//   - Service.Stop cancels the corresponding subscription.
//   - Service.Reconcile auto-subscribes recovered sandboxes.

package opencode

import (
	"bufio"
	"net/http"

	core "dappco.re/go"
)

// EventEmitter is the bridge to the host application's event bus.
// Implementations forward the JSON-encoded event to whichever bus
// the host is wired to (Wails event manager in GUI mode; a no-op
// in CLI mode).
type EventEmitter func(eventJSON string)

// SetEventEmitter installs the emitter callback used by every
// SSE subscriber goroutine. Safe to call before or after any
// Start / Reconcile invocation:
//
//   - If sandboxes are already running, the next Start / Reconcile
//     picks up the new emitter (subscriptions are per-sandbox and
//     created at sandbox-spawn time, not at SetEventEmitter time —
//     we don't backfill).
//   - Setting to nil disables future subscribes but does not cancel
//     in-flight ones (they continue draining; emit becomes a no-op).
//
// Usage example:
//
//	opencodeSvc.SetEventEmitter(func(e string) {
//	    app.Event.Emit("opencode:event", e)
//	})
func (s *Service) SetEventEmitter(emit EventEmitter) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.eventEmitter = emit
	s.mu.Unlock()
}

// emitter returns the currently-installed callback. Used by
// Subscribe goroutines on every event; cheaply re-resolves so a
// late SetEventEmitter takes effect without restarting the stream.
func (s *Service) emitter() EventEmitter {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.eventEmitter
}

// Subscribe opens an SSE stream against the named sandbox's
// /global/event endpoint and forwards each "data:" line to the
// installed emitter. Returns a cancel function that closes the
// stream + tears down the goroutine. Idempotent — calling for an
// already-subscribed id returns the existing cancel function.
//
// No-op when no emitter is installed (returns a cancel that does
// nothing) — saves an SSE connection per sandbox in CLI / serve
// modes where no consumer is wired.
//
// Usage example:
//
//	cancel, r := svc.Subscribe("oc-1735843891234")
//	if r.OK { defer cancel() }
func (s *Service) Subscribe(id string) (func(), core.Result) {
	if s == nil {
		return func() {}, core.Fail(core.E("opencode.Subscribe", "service is nil", nil))
	}
	if core.Trim(id) == "" {
		return func() {}, core.Fail(core.E("opencode.Subscribe", "id is required", nil))
	}
	// Idempotent — return existing cancel if already subscribed.
	s.mu.RLock()
	if cancel, ok := s.subscriptions[id]; ok {
		s.mu.RUnlock()
		return cancel, core.Ok(nil)
	}
	s.mu.RUnlock()

	if s.emitter() == nil {
		// No consumer — skip the SSE connection entirely.
		return func() {}, core.Ok(nil)
	}

	target, r := s.targetFor(id)
	if !r.OK {
		return func() {}, r
	}
	authHeader := s.authHeader()

	ctx, cancel := core.WithCancel(core.Background())
	wrap := func() {
		cancel()
		s.mu.Lock()
		delete(s.subscriptions, id)
		s.mu.Unlock()
	}
	s.mu.Lock()
	if s.subscriptions == nil {
		s.subscriptions = make(map[string]func())
	}
	s.subscriptions[id] = wrap
	s.mu.Unlock()

	go s.runSubscription(ctx, id, target, authHeader)
	return wrap, core.Ok(nil)
}

// Unsubscribe cancels the SSE goroutine for one sandbox. No-op if
// no subscription exists for the given id. Called by Stop.
//
// Usage example:
//
//	svc.Unsubscribe("oc-1735843891234")
func (s *Service) Unsubscribe(id string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	cancel, ok := s.subscriptions[id]
	delete(s.subscriptions, id)
	s.mu.Unlock()
	if ok && cancel != nil {
		cancel()
	}
}

// runSubscription is the goroutine body — reconnects with backoff
// until the context is cancelled. Each "data:" line is forwarded
// to the installed emitter (re-resolved per event so a late
// SetEventEmitter takes effect immediately).
func (s *Service) runSubscription(ctx core.Context, id, target, authHeader string) {
	backoff := 1 * core.Second
	maxBackoff := 30 * core.Second

	for {
		if ctx.Err() != nil {
			return
		}
		if err := s.streamEvents(ctx, target, authHeader); err != nil {
			if ctx.Err() != nil {
				return
			}
			// Connection error — back off + reconnect.
			select {
			case <-ctx.Done():
				return
			case <-core.After(backoff):
			}
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
			continue
		}
		// Stream closed cleanly (opencode-serve sent EOF) — also
		// reconnect, with a short backoff so we don't tight-loop.
		select {
		case <-ctx.Done():
			return
		case <-core.After(500 * core.Millisecond):
		}
		backoff = 1 * core.Second
	}
}

// streamEvents opens one SSE connection + reads until the stream
// ends or ctx fires. Each "data: <json>" line forwards to the
// emitter. Non-data lines (id, retry, comments) are skipped.
func (s *Service) streamEvents(ctx core.Context, target, authHeader string) error {
	req, err := http.NewRequestWithContext(ctx, core.MethodGet, target+"/global/event", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/event-stream")
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	// No timeout on the client — SSE is long-lived. The context
	// is the cancellation lever.
	client := &core.HTTPClient{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		return core.E("opencode.streamEvents",
			core.Sprintf("upstream %d", resp.StatusCode), nil)
	}

	scanner := bufio.NewScanner(resp.Body)
	// Bump buffer so large opencode events don't truncate. 1 MiB
	// is generous; real events are kilobytes.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		line := scanner.Text()
		if !core.HasPrefix(line, "data:") {
			continue
		}
		payload := core.Trim(core.TrimPrefix(line, "data:"))
		if payload == "" {
			continue
		}
		if emit := s.emitter(); emit != nil {
			emit(payload)
		}
	}
	return scanner.Err()
}

