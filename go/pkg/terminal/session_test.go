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
		subscribers: make(map[uint64]TerminalSubscriber),
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
	_, unsub := sess.Subscribe(func(chunk []byte) {
		mu.Lock()
		buf = append(buf, chunk...)
		mu.Unlock()
	})
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
