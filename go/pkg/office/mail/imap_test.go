// SPDX-Licence-Identifier: EUPL-1.2

package mail

import (
	"testing"

	core "dappco.re/go"
)

// TestFetchOnce_LockedSession_Bad — FetchOnce returns mail.session.locked when
// session is paused (§6 — no IMAP connection attempted).
func TestFetchOnce_LockedSession_Bad(t *testing.T) {
	c := core.New()
	svc := NewService(c)
	ap := newTestAccountProvider(t)
	svc.SetAccountService(ap)
	svc.paused.Store(true)

	r := svc.FetchOnce(FetchOnceInput{AccountName: "personal", FolderSlug: "inbox"})
	if r.OK {
		t.Fatal("expected FetchOnce to fail when session is locked")
	}
	if !core.Contains(r.Error(), "mail.session.locked") && !core.Contains(r.Error(), "sign in") {
		t.Errorf("unexpected error (want session.locked): %s", r.Error())
	}
}

// TestPollingLoop_ExponentialBackoff_Ugly — verify backoff table shape:
// 30s/1m/2m/5m/10m/10m (capped per §2.3).
func TestPollingLoop_ExponentialBackoff_Ugly(t *testing.T) {
	c := core.New()
	svc := NewService(c)
	key := "test/inbox"

	expected := []core.Duration{
		30 * core.Second,
		core.Minute,
		2 * core.Minute,
		5 * core.Minute,
		10 * core.Minute,
		10 * core.Minute, // capped — further calls return same
	}

	for i, want := range expected {
		got := svc.backoffDelay(key)
		if got != want {
			t.Errorf("backoff[%d]: got %v, want %v", i, got, want)
		}
	}
}

// TestResetBackoff_Good — after successful connect, backoff resets to 30s.
func TestResetBackoff_Good(t *testing.T) {
	c := core.New()
	svc := NewService(c)
	key := "test/inbox"

	// Advance through a few entries.
	_ = svc.backoffDelay(key)
	_ = svc.backoffDelay(key)
	_ = svc.backoffDelay(key)

	svc.resetBackoff(key)

	got := svc.backoffDelay(key)
	if got != 30*core.Second {
		t.Errorf("after reset, expected 30s, got %v", got)
	}
}

// TestUIDValidityChange_PreserveAndResync_Ugly — when UIDValidity changes:
// threads.md renamed to .preresync-{old}, EventResyncStarted+Completed fire.
func TestUIDValidityChange_PreserveAndResync_Ugly(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	c := core.New()
	svc := NewService(c)

	var events []string
	Subscribe(c, func(_ *core.Core, ev MailEvent) {
		events = append(events, ev.Kind)
	})

	// Write a dummy threads.md so Rename has something to work with.
	folderSlug := "inbox"
	threadsR := threadsFilePath(folderSlug)
	if threadsR.OK {
		threadsPath := threadsR.Value.(string)
		_ = core.WriteFile(threadsPath, []byte("---\nid: abc\n"), 0o600)
	}

	err := svc.handleUIDValidityRotation("personal", folderSlug, 12345678)
	if err != nil {
		t.Fatalf("handleUIDValidityRotation: %v", err)
	}

	// Must have fired ResyncStarted + ResyncCompleted in order.
	if len(events) < 2 {
		t.Fatalf("expected ≥2 events, got %d: %v", len(events), events)
	}
	if events[0] != EventResyncStarted {
		t.Errorf("event[0]: got %q, want %q", events[0], EventResyncStarted)
	}
	found := false
	for _, ev := range events {
		if ev == EventResyncCompleted {
			found = true
		}
	}
	if !found {
		t.Errorf("EventResyncCompleted not fired; events: %v", events)
	}
}

// TestStateFile_AccountFolderIsolation_Ugly — two accounts polling the same
// folder slug don't cross-contaminate state files.
func TestStateFile_AccountFolderIsolation_Ugly(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	c := core.New()
	svc := NewService(c)

	stateA := &FolderState{UIDValidity: 1111, LastUIDSeen: 42, LastFetchTS: core.Now()}
	stateB := &FolderState{UIDValidity: 2222, LastUIDSeen: 99, LastFetchTS: core.Now()}

	if err := svc.saveFolderState("account-a", "inbox", stateA); err != nil {
		t.Fatalf("saveFolderState account-a: %v", err)
	}
	if err := svc.saveFolderState("account-b", "inbox", stateB); err != nil {
		t.Fatalf("saveFolderState account-b: %v", err)
	}

	gotA, err := svc.loadFolderState("account-a", "inbox")
	if err != nil {
		t.Fatalf("loadFolderState account-a: %v", err)
	}
	gotB, err := svc.loadFolderState("account-b", "inbox")
	if err != nil {
		t.Fatalf("loadFolderState account-b: %v", err)
	}

	if gotA.UIDValidity != 1111 {
		t.Errorf("account-a UIDValidity: got %d, want 1111", gotA.UIDValidity)
	}
	if gotB.UIDValidity != 2222 {
		t.Errorf("account-b UIDValidity: got %d, want 2222", gotB.UIDValidity)
	}
	if gotA.LastUIDSeen != 42 {
		t.Errorf("account-a LastUIDSeen: got %d, want 42", gotA.LastUIDSeen)
	}
	if gotB.LastUIDSeen != 99 {
		t.Errorf("account-b LastUIDSeen: got %d, want 99", gotB.LastUIDSeen)
	}
}

// TestSessionLocked_NoSilentFallthrough_Ugly — every read-gated public method,
// while locked, returns an explicit lock result (§6 HIGH-mail-2).
func TestSessionLocked_NoSilentFallthrough_Ugly(t *testing.T) {
	c := core.New()
	svc := NewService(c)
	ap := newTestAccountProvider(t)
	svc.SetAccountService(ap)
	svc.paused.Store(true)

	table := []struct {
		name string
		call func() core.Result
	}{
		{"FetchOnce", func() core.Result {
			return svc.FetchOnce(FetchOnceInput{AccountName: "x", FolderSlug: "inbox"})
		}},
		{"ListAccounts", func() core.Result {
			return svc.ListAccounts()
		}},
		{"RemoveAccount", func() core.Result {
			return svc.RemoveAccount("personal")
		}},
	}

	for _, tc := range table {
		r := tc.call()
		if r.OK {
			t.Errorf("%s: expected failure when locked, got OK", tc.name)
			continue
		}
		if !core.Contains(r.Error(), "mail.session.locked") && !core.Contains(r.Error(), "sign in") {
			t.Errorf("%s: expected session.locked error, got: %s", tc.name, r.Error())
		}
	}
}
