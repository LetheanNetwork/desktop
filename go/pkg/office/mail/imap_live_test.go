// SPDX-Licence-Identifier: EUPL-1.2

// imap_live_test.go — end-to-end coverage of fetchFolder / FetchOnce
// / imapConnect / imapConnectWithTLS / imapMsgToRecord against the
// real in-process IMAP server built in imap_fake_test.go. See that
// file's header for the "real service, mock platform" rationale.

package mail

import (
	"crypto/tls"
	"testing"

	core "dappco.re/go"
	imap "github.com/emersion/go-imap/v2"
)

const testIMAPUser = "mailtest@lthn.local"
const testIMAPPass = "s3cr3t-app-password"

func testAccount(name, host string, port int, tlsEnabled bool) *MailAccount {
	return &MailAccount{
		Name: name,
		IMAP: IMAPConfig{Host: host, Port: port, User: testIMAPUser, TLS: tlsEnabled},
		Auth: AuthSpec{Kind: "appPassword", Secret: testIMAPPass},
	}
}

// TestFetchFolder_FullCycle_Good — real SELECT + FETCH against the
// fake server: two messages land in threads.md, state file records
// LastUIDSeen, EventThreadReceived fires per message.
func TestFetchFolder_FullCycle_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	fake := newFakeIMAPServer(t, testIMAPUser, testIMAPPass, true)
	fake.createMailbox(t, "INBOX")
	fake.appendMessage(t, testIMAPUser, testIMAPPass, "inbox",
		[]byte("From: Ada Penley <ada@example.com>\r\n"+
			"To: me@example.com\r\n"+
			"Subject: Re: SOW v2\r\n"+
			"Date: Mon, 01 Jan 2024 10:00:00 +0000\r\n"+
			"Message-Id: <msg-1@example.com>\r\n"+
			"\r\n"+
			"Hello there.\r\n"))
	fake.appendMessage(t, testIMAPUser, testIMAPPass, "inbox",
		[]byte("From: bob@example.com\r\n"+
			"To: me@example.com\r\n"+
			"Subject: Second message\r\n"+
			"Date: Mon, 01 Jan 2024 11:00:00 +0000\r\n"+
			"\r\n"+
			"Second body.\r\n"),
		imap.FlagSeen)

	c := core.New()
	svc := NewService(c)
	ap := newTestAccountProvider(t)
	svc.SetAccountService(ap)
	svc.imapDialOverride = fake.dialOverride()

	host, port := splitFakeAddr(t, fake.addr)
	acct := testAccount("personal", host, port, true)

	var events []string
	Subscribe(c, func(_ *core.Core, ev MailEvent) {
		events = append(events, ev.Kind)
	})

	r := svc.fetchFolder(acct, "inbox")
	if !r.OK {
		t.Fatalf("fetchFolder: %s", r.Error())
	}
	count, _ := r.Value.(int)
	if count != 2 {
		t.Fatalf("expected 2 fetched messages, got %d", count)
	}

	// threads.md must carry both records.
	threadsR := threadsFilePath("inbox")
	if !threadsR.OK {
		t.Fatalf("threadsFilePath: %s", threadsR.Error())
	}
	rawR := core.ReadFile(threadsR.Value.(string))
	if !rawR.OK {
		t.Fatalf("read threads.md: %s", rawR.Error())
	}
	raw, _ := rawR.Value.([]byte)
	recs, err := parseThreads(raw)
	if err != nil {
		t.Fatalf("parseThreads: %v", err)
	}
	if len(recs) != 2 {
		t.Fatalf("expected 2 thread records, got %d: %+v", len(recs), recs)
	}

	// First message: no \Seen flag → Unread true; From resolves to the
	// display name; ID falls back to the real Message-Id.
	var first, second *MailThreadRecord
	for i := range recs {
		if recs[i].ID == "msg-1@example.com" {
			first = &recs[i]
		}
		if recs[i].Subj == "Second message" {
			second = &recs[i]
		}
	}
	if first == nil {
		t.Fatalf("first message record not found: %+v", recs)
	}
	if !first.Unread {
		t.Errorf("first message: expected Unread=true (no \\Seen flag)")
	}
	if first.From != "Ada Penley" {
		t.Errorf("first message From = %q, want %q", first.From, "Ada Penley")
	}
	if second == nil {
		t.Fatalf("second message record not found: %+v", recs)
	}
	if second.Unread {
		t.Errorf("second message: expected Unread=false (\\Seen flag set)")
	}
	if second.From != "bob@example.com" {
		t.Errorf("second message From = %q, want %q (no display name → raw addr)", second.From, "bob@example.com")
	}
	if second.ID == "" {
		t.Errorf("second message ID must fall back to uid-N when Message-Id is absent")
	}

	// State file recorded progress.
	state, err := svc.loadFolderState("personal", "inbox")
	if err != nil {
		t.Fatalf("loadFolderState: %v", err)
	}
	if state.LastUIDSeen != 2 {
		t.Errorf("LastUIDSeen = %d, want 2", state.LastUIDSeen)
	}
	if state.UIDValidity == 0 {
		t.Errorf("UIDValidity must be stamped from the real SELECT response")
	}

	foundThreadEvents := 0
	for _, ev := range events {
		if ev == EventThreadReceived {
			foundThreadEvents++
		}
	}
	if foundThreadEvents != 2 {
		t.Errorf("expected 2 EventThreadReceived, got %d (events: %v)", foundThreadEvents, events)
	}
}

// TestFetchFolder_NothingNew_Good — a second fetchFolder call after
// everything has already been consumed hits the fromUID > toUID
// short-circuit and reports zero without a SELECT/FETCH mismatch.
func TestFetchFolder_NothingNew_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	fake := newFakeIMAPServer(t, testIMAPUser, testIMAPPass, true)
	fake.createMailbox(t, "INBOX")
	fake.appendMessage(t, testIMAPUser, testIMAPPass, "inbox",
		[]byte("From: a@example.com\r\nSubject: only\r\n\r\nbody\r\n"))

	c := core.New()
	svc := NewService(c)
	svc.imapDialOverride = fake.dialOverride()
	host, port := splitFakeAddr(t, fake.addr)
	acct := testAccount("personal", host, port, true)

	first := svc.fetchFolder(acct, "inbox")
	if !first.OK || first.Value.(int) != 1 {
		t.Fatalf("first fetchFolder: ok=%v value=%v err=%s", first.OK, first.Value, first.Error())
	}

	second := svc.fetchFolder(acct, "inbox")
	if !second.OK {
		t.Fatalf("second fetchFolder: %s", second.Error())
	}
	if second.Value.(int) != 0 {
		t.Errorf("second fetchFolder should report 0 new messages, got %v", second.Value)
	}
}

// TestFetchFolder_UIDValidityRotation_Ugly — a stale UIDVALIDITY
// pre-seeded in the state file is detected on the real SELECT
// response, triggers handleUIDValidityRotation from INSIDE
// fetchFolder (not the direct call already covered in imap_test.go),
// and the folder starts fresh (LastUIDSeen resets, both messages
// re-appear as "new").
func TestFetchFolder_UIDValidityRotation_Ugly(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	fake := newFakeIMAPServer(t, testIMAPUser, testIMAPPass, true)
	fake.createMailbox(t, "INBOX")
	fake.appendMessage(t, testIMAPUser, testIMAPPass, "inbox",
		[]byte("From: a@example.com\r\nSubject: one\r\n\r\nbody\r\n"))

	c := core.New()
	svc := NewService(c)
	svc.imapDialOverride = fake.dialOverride()
	host, port := splitFakeAddr(t, fake.addr)
	acct := testAccount("personal", host, port, true)

	var events []string
	Subscribe(c, func(_ *core.Core, ev MailEvent) {
		events = append(events, ev.Kind)
	})

	// Pre-seed a stale state — the real server's UIDVALIDITY will
	// never equal this bogus value.
	stale := &FolderState{UIDValidity: 999999, LastUIDSeen: 500}
	if err := svc.saveFolderState("personal", "inbox", stale); err != nil {
		t.Fatalf("saveFolderState: %v", err)
	}

	r := svc.fetchFolder(acct, "inbox")
	if !r.OK {
		t.Fatalf("fetchFolder: %s", r.Error())
	}
	if r.Value.(int) != 1 {
		t.Fatalf("expected the single message to be re-fetched after rotation, got %v", r.Value)
	}

	found := false
	for _, ev := range events {
		if ev == EventResyncStarted {
			found = true
		}
	}
	if !found {
		t.Errorf("expected EventResyncStarted fired from inside fetchFolder; events: %v", events)
	}

	state, err := svc.loadFolderState("personal", "inbox")
	if err != nil {
		t.Fatalf("loadFolderState: %v", err)
	}
	if state.LastUIDSeen != 1 {
		t.Errorf("post-rotation LastUIDSeen = %d, want 1 (fresh state)", state.LastUIDSeen)
	}
}

// TestFetchFolder_SelectNonexistentFolder_Bad — SELECTing a mailbox
// that was never created on the server surfaces a real IMAP NO
// response through fetchFolder's SELECT error branch.
func TestFetchFolder_SelectNonexistentFolder_Bad(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	fake := newFakeIMAPServer(t, testIMAPUser, testIMAPPass, true)
	// Deliberately no createMailbox call.

	c := core.New()
	svc := NewService(c)
	svc.imapDialOverride = fake.dialOverride()
	host, port := splitFakeAddr(t, fake.addr)
	acct := testAccount("personal", host, port, true)

	r := svc.fetchFolder(acct, "does-not-exist")
	if r.OK {
		t.Fatal("expected fetchFolder to fail SELECTing a nonexistent mailbox")
	}
	if !core.Contains(r.Error(), "SELECT") {
		t.Errorf("expected SELECT-shaped error, got: %s", r.Error())
	}
}

// TestFetchFolder_ConnectFailure_Bad — the dial override itself
// returning an error (fake server refuses login with a wrong
// password) exercises fetchFolder's connect-failure branch,
// including the EventConnectionFailed emission.
func TestFetchFolder_ConnectFailure_Bad(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	fake := newFakeIMAPServer(t, testIMAPUser, testIMAPPass, true)
	fake.createMailbox(t, "INBOX")

	c := core.New()
	svc := NewService(c)
	svc.imapDialOverride = fake.dialOverride()
	host, port := splitFakeAddr(t, fake.addr)
	acct := testAccount("personal", host, port, true)
	acct.Auth.Secret = "wrong-password"

	var events []string
	Subscribe(c, func(_ *core.Core, ev MailEvent) {
		events = append(events, ev.Kind)
	})

	r := svc.fetchFolder(acct, "inbox")
	if r.OK {
		t.Fatal("expected fetchFolder to fail with a bad password")
	}
	found := false
	for _, ev := range events {
		if ev == EventConnectionFailed {
			found = true
		}
	}
	if !found {
		t.Errorf("expected EventConnectionFailed; events: %v", events)
	}
}

// TestFetchOnce_LiveServer_Good — the full public FetchOnce surface
// (account lookup, single-flight lock, decrypt) driven end-to-end
// against the fake server, not just fetchFolder directly.
func TestFetchOnce_LiveServer_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	fake := newFakeIMAPServer(t, testIMAPUser, testIMAPPass, true)
	fake.createMailbox(t, "INBOX")
	fake.appendMessage(t, testIMAPUser, testIMAPPass, "inbox",
		[]byte("From: a@example.com\r\nSubject: hi\r\n\r\nbody\r\n"))

	c := core.New()
	svc := NewService(c)
	ap := newTestAccountProvider(t)
	svc.SetAccountService(ap)
	svc.imapDialOverride = fake.dialOverride()

	host, port := splitFakeAddr(t, fake.addr)
	if r := svc.SaveAccount(AccountInput{
		Name: "personal",
		IMAP: IMAPConfig{Host: host, Port: port, User: testIMAPUser, TLS: true},
		Auth: AuthSpec{Kind: "appPassword", Secret: testIMAPPass},
	}); !r.OK {
		t.Fatalf("SaveAccount: %s", r.Error())
	}

	r := svc.FetchOnce(FetchOnceInput{AccountName: "personal", FolderSlug: "inbox"})
	if !r.OK {
		t.Fatalf("FetchOnce: %s", r.Error())
	}
	if r.Value.(int) != 1 {
		t.Fatalf("expected 1 fetched message via FetchOnce, got %v", r.Value)
	}
}

// TestFetchOnce_AccountNotFound_Bad — FetchOnce for a name absent
// from the saved account list.
func TestFetchOnce_AccountNotFound_Bad(t *testing.T) {
	svc, _ := newTestMailService(t)
	r := svc.FetchOnce(FetchOnceInput{AccountName: "ghost", FolderSlug: "inbox"})
	if r.OK {
		t.Fatal("expected FetchOnce to fail for an unknown account name")
	}
	if !core.Contains(r.Error(), "not found") {
		t.Errorf("expected account-not-found error, got: %s", r.Error())
	}
}

// --- imapConnect / imapConnectWithTLS direct-call coverage ---

// TestImapConnect_ConnectionRefused_Bad — both the TLS and STARTTLS
// dial branches of imapConnect surface a real connection-refused
// error when nothing is listening. imapConnect is called DIRECTLY
// (not via the override seam) so this is the production entrypoint's
// own code, unmodified.
func TestImapConnect_ConnectionRefused_Bad(t *testing.T) {
	svc, _ := newTestMailService(t)

	for _, tlsEnabled := range []bool{true, false} {
		acct := testAccount("x", "127.0.0.1", 1, tlsEnabled) // port 1: nothing listens
		client, err := svc.imapConnect(acct, "x/inbox")
		if err == nil {
			_ = client.Close()
			t.Fatalf("tls=%v: expected connection-refused error, got a live client", tlsEnabled)
		}
	}
}

// TestImapConnect_UntrustedCert_Bad — imapConnect's hard-coded empty
// *imapclient.Options{} means it NEVER trusts a test-local CA — a
// real self-signed-cert rejection, exercising the same code path a
// hostile/misconfigured mail server would trip in production. This
// is why fetchFolder's own tests go through imapDialOverride +
// imapConnectWithTLS(RootCAs: pool) instead of weakening imapConnect.
func TestImapConnect_UntrustedCert_Bad(t *testing.T) {
	svc, _ := newTestMailService(t)
	fake := newFakeIMAPServer(t, testIMAPUser, testIMAPPass, true)
	host, port := splitFakeAddr(t, fake.addr)
	acct := testAccount("x", host, port, true)

	client, err := svc.imapConnect(acct, "x/inbox")
	if err == nil {
		_ = client.Close()
		t.Fatal("expected imapConnect to reject the fake server's untrusted self-signed cert")
	}
}

// TestImapConnectWithTLS_BothBranches_Good — imapConnectWithTLS picks
// DialTLS vs DialStartTLS based on acct.IMAP.TLS, matching whichever
// listener mode the fake server was built with.
func TestImapConnectWithTLS_BothBranches_Good(t *testing.T) {
	t.Run("implicit_tls", func(t *testing.T) {
		fake := newFakeIMAPServer(t, testIMAPUser, testIMAPPass, true)
		host, port := splitFakeAddr(t, fake.addr)
		acct := testAccount("x", host, port, true)
		client, err := imapConnectWithTLS(acct, &tls.Config{RootCAs: fake.trustPool})
		if err != nil {
			t.Fatalf("imapConnectWithTLS (implicit TLS): %v", err)
		}
		_ = client.Close()
	})
	t.Run("starttls", func(t *testing.T) {
		fake := newFakeIMAPServer(t, testIMAPUser, testIMAPPass, false)
		host, port := splitFakeAddr(t, fake.addr)
		acct := testAccount("x", host, port, false)
		client, err := imapConnectWithTLS(acct, &tls.Config{RootCAs: fake.trustPool})
		if err != nil {
			t.Fatalf("imapConnectWithTLS (STARTTLS): %v", err)
		}
		_ = client.Close()
	})
}
