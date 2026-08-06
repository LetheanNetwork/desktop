// SPDX-Licence-Identifier: EUPL-1.2

package terminal

import (
	"sync"
	"testing"
	"time"
)

// memSession builds a PTY-less session with a small ring so the buffer logic is
// testable without spawning a shell or allocating 10MB.
func memSession(ringCap int) *TerminalSession {
	return &TerminalSession{
		ID:          "test",
		Host:        "local",
		Shell:       "/bin/sh",
		ring:        make([]byte, ringCap),
		subscribers: make(map[uint64]OutputSubscriber),
		done:        make(chan struct{}),
	}
}

func TestTerminalSession_Ring_Good(t *testing.T) {
	s := memSession(16)
	s.appendRing([]byte("hello"))
	if got := string(s.ringSnapshot()); got != "hello" {
		t.Errorf("ring snapshot = %q, want %q", got, "hello")
	}
}

func TestTerminalSession_Ring_Bad(t *testing.T) {
	// Nothing written → snapshot is nil (a fresh subscriber gets no scrollback).
	if snap := memSession(16).ringSnapshot(); snap != nil {
		t.Errorf("empty ring snapshot = %q, want nil", snap)
	}
	// A zero-capacity ring must not panic on append (defensive guard).
	memSession(0).appendRing([]byte("x"))
}

func TestTerminalSession_Ring_Ugly(t *testing.T) {
	// Write more than the ring holds: the snapshot is the last ringCap bytes in
	// chronological order, not garbage from the wrap arithmetic.
	s := memSession(8)
	s.appendRing([]byte("abcdefghij")) // 10 bytes into an 8-byte ring
	if !s.wrapped {
		t.Fatal("ring should be marked wrapped after overflow")
	}
	if got := string(s.ringSnapshot()); got != "cdefghij" {
		t.Errorf("wrapped snapshot = %q, want %q", got, "cdefghij")
	}
}

func TestTerminalSession_Cursor_GoodLiveMonotonic(t *testing.T) {
	s := memSession(16)
	var got []OutputChunk

	replay, unsubscribe, err := s.SubscribeFrom(0, func(chunk OutputChunk) {
		got = append(got, chunk)
	})
	if err != nil {
		t.Fatalf("subscribe from current cursor: %v", err)
	}
	defer unsubscribe()
	if len(replay.Data) != 0 || replay.Start != 0 || replay.End != 0 || replay.Reset {
		t.Fatalf("fresh replay = %#v, want empty cursor zero", replay)
	}

	s.publish([]byte("abc"))
	s.publish([]byte("de"))

	if len(got) != 2 {
		t.Fatalf("live chunks = %d, want 2", len(got))
	}
	assertOutputChunk(t, got[0], 0, 3, "abc", false)
	assertOutputChunk(t, got[1], 3, 5, "de", false)
}

func TestTerminalSession_Cursor_GoodRetainedReplay(t *testing.T) {
	s := memSession(8)
	s.appendRing([]byte("abcdef"))

	replay, unsubscribe, err := s.SubscribeFrom(2, func(OutputChunk) {})
	if err != nil {
		t.Fatalf("subscribe from retained cursor: %v", err)
	}
	defer unsubscribe()

	assertOutputChunk(t, replay, 2, 6, "cdef", false)
}

func TestTerminalSession_Cursor_BadFutureCursor(t *testing.T) {
	s := memSession(8)
	s.appendRing([]byte("abc"))

	if _, unsubscribe, err := s.SubscribeFrom(4, func(OutputChunk) {}); err == nil {
		unsubscribe()
		t.Fatal("future cursor should be rejected")
	}
	if len(s.subscribers) != 0 {
		t.Fatalf("future cursor registered %d subscribers, want none", len(s.subscribers))
	}
}

func TestTerminalSession_Cursor_UglyExpiredCursorResets(t *testing.T) {
	s := memSession(8)
	s.appendRing([]byte("abcdefghij"))

	replay, unsubscribe, err := s.SubscribeFrom(1, func(OutputChunk) {})
	if err != nil {
		t.Fatalf("subscribe from expired cursor: %v", err)
	}
	defer unsubscribe()

	assertOutputChunk(t, replay, 2, 10, "cdefghij", true)
}

func TestTerminalSession_Cursor_CurrentHasNoReplay(t *testing.T) {
	s := memSession(8)
	s.appendRing([]byte("abc"))

	replay, unsubscribe, err := s.SubscribeFrom(3, func(OutputChunk) {})
	if err != nil {
		t.Fatalf("subscribe from current cursor: %v", err)
	}
	defer unsubscribe()

	assertOutputChunk(t, replay, 3, 3, "", false)
}

func TestTerminalSession_Attach_ReplacesSubscriberAtomically(t *testing.T) {
	s := memSession(8)
	var first, second []OutputChunk

	_, firstID, err := s.subscribeFrom(0, nil, func(chunk OutputChunk) {
		first = append(first, chunk)
	})
	if err != nil {
		t.Fatalf("first subscribe: %v", err)
	}
	s.publish([]byte("a"))

	_, secondID, err := s.subscribeFrom(1, &firstID, func(chunk OutputChunk) {
		second = append(second, chunk)
	})
	if err != nil {
		t.Fatalf("replacement subscribe: %v", err)
	}
	if secondID == firstID {
		t.Fatalf("replacement subscriber id = %d, want a new id", secondID)
	}

	s.publish([]byte("b"))

	if len(first) != 1 {
		t.Fatalf("replaced subscriber received %d chunks, want 1", len(first))
	}
	if len(second) != 1 {
		t.Fatalf("replacement subscriber received %d chunks, want 1", len(second))
	}
	assertOutputChunk(t, first[0], 0, 1, "a", false)
	assertOutputChunk(t, second[0], 1, 2, "b", false)
}

func TestTerminalSession_Resize_NoopOnZero(t *testing.T) {
	// Zero dimensions and a nil PTY must both be silent no-ops, not panics.
	s := memSession(16)
	s.Resize(0, 0)
	s.Resize(80, 24) // f == nil → returns early
}

func TestTerminalSession_NewSessionID_Unique(t *testing.T) {
	a, b := newSessionID(), newSessionID()
	if a == b {
		t.Errorf("two session IDs collided: %q", a)
	}
	if len(a) != 16 {
		t.Errorf("session ID = %q (len %d), want 16 hex chars", a, len(a))
	}
}

// TestTerminalSession_Echo_Good is the real-PTY integration smoke: spawn a
// shell, subscribe, write a command, and confirm its output streams back.
// Skips (not fails) if a PTY can't be allocated in the test environment.
func TestTerminalSession_Echo_Good(t *testing.T) {
	sess, err := NewTerminalSession(SessionOptions{Shell: "/bin/sh", Term: "xterm-256color", Cols: 80, Rows: 24})
	if err != nil {
		t.Skipf("open terminal session: %v", err)
	}
	go func() { _ = sess.Run() }()
	t.Cleanup(sess.Close)

	var mu sync.Mutex
	var buf []byte
	_, unsub, err := sess.SubscribeFrom(0, func(chunk OutputChunk) {
		mu.Lock()
		buf = append(buf, chunk.Data...)
		mu.Unlock()
	})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer unsub()

	if _, err := sess.Write([]byte("echo lthn_marker_42\n")); err != nil {
		t.Fatalf("write to PTY: %v", err)
	}

	deadline := time.After(3 * time.Second)
	for {
		mu.Lock()
		seen := contains(buf, "lthn_marker_42")
		mu.Unlock()
		if seen {
			return // output streamed back through the subscriber — pass
		}
		select {
		case <-deadline:
			t.Fatal("did not observe command echo within 3s")
		case <-time.After(20 * time.Millisecond):
		}
	}
}

func TestTerminalSession_Subscribe_Good(t *testing.T) {
	s := memSession(8)
	s.appendRing([]byte("ab"))
	var mu sync.Mutex
	var received []byte

	replay, unsubscribe := s.Subscribe(func(chunk []byte) {
		mu.Lock()
		received = append(received, chunk...)
		mu.Unlock()
	})
	if string(replay) != "ab" {
		t.Fatalf("replay = %q, want %q", replay, "ab")
	}

	s.publish([]byte("c"))
	mu.Lock()
	got := string(received)
	mu.Unlock()
	if got != "c" {
		t.Fatalf("live via Subscribe = %q, want %q", got, "c")
	}

	unsubscribe()
	s.publish([]byte("d"))
	mu.Lock()
	got = string(received)
	mu.Unlock()
	if got != "c" {
		t.Fatalf("received after unsubscribe = %q, want unchanged %q", got, "c")
	}
}

func assertOutputChunk(t *testing.T, got OutputChunk, start, end uint64, data string, reset bool) {
	t.Helper()
	if got.Start != start || got.End != end || string(got.Data) != data || got.Reset != reset {
		t.Errorf(
			"output chunk = {start:%d end:%d data:%q reset:%t}, want {start:%d end:%d data:%q reset:%t}",
			got.Start,
			got.End,
			string(got.Data),
			got.Reset,
			start,
			end,
			data,
			reset,
		)
	}
}

func contains(haystack []byte, needle string) bool {
	return len(needle) > 0 && indexOf(string(haystack), needle) >= 0
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
