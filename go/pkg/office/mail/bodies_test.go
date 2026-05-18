// SPDX-Licence-Identifier: EUPL-1.2

// bodies_test.go — Cerberus #9 Concern 3.A / Mantis #1556 contract
// tests for per-message body writes via FetchBody. The two tests
// named in the Cerberus #9 brief are mandatory:
//
//   - TestMail_BodyWrite_FirstWriteIsUnconditional_Good — confirms
//     that the IfMtime zero-time first-write path is reached and
//     produces a real on-disk artefact under the bodies/ folder.
//   - TestMail_BodyWrite_RewriteIdempotentOnSameContent_Good —
//     confirms a second write of identical bytes against the same
//     thread-id is a no-op (or substrate-OK) and does not corrupt
//     the cached file.
//
// Also exercises the surface-level guards: locked session, invalid
// folder slug, invalid thread id.

package mail

import (
	"testing"

	core "dappco.re/go"
)

// TestMail_BodyWrite_FirstWriteIsUnconditional_Good — the first
// FetchBody call writes the body bytes to disk via
// paths.AtomicWriteWithVersion with IfMtime zero, producing the
// canonical bodies/<thread-id>.eml artefact. Confirms the at-rest
// substrate cascade adoption (Cerberus #9 Concern 3.A) is honoured
// at the per-message boundary.
//
// Driven by an injected fetchBodyFromIMAP override so the test
// doesn't need a real IMAP server. The substrate write path is the
// load-bearing thing under test; the IMAP wire is the surface that
// follows when §4.2 lazy-fetch lands.
func TestMail_BodyWrite_FirstWriteIsUnconditional_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	c := core.New()
	svc := NewService(c)
	ap := newTestAccountProvider(t)
	svc.SetAccountService(ap)

	const want = "Subject: hi\r\n\r\nhello world"
	svc.bodyFetchOverride = func(_ FetchBodyInput) ([]byte, error) {
		return []byte(want), nil
	}

	r := svc.FetchBody(FetchBodyInput{
		AccountName: "personal",
		FolderSlug:  "inbox",
		ThreadID:    "abc123",
	})
	if !r.OK {
		t.Fatalf("FetchBody first call failed: %s", r.Error())
	}
	out, ok := r.Value.(FetchBodyOutput)
	if !ok {
		t.Fatalf("FetchBody returned non-FetchBodyOutput value")
	}
	if out.Plain != "hello world" {
		t.Errorf("Plain = %q; want %q", out.Plain, "hello world")
	}

	// File MUST exist on disk under bodies/.
	pathR := bodyFilePath("inbox", "abc123")
	if !pathR.OK {
		t.Fatalf("bodyFilePath: %s", pathR.Error())
	}
	statR := core.Stat(pathR.Value.(string))
	if !statR.OK {
		t.Fatalf("body file not on disk after first FetchBody: %s", pathR.Value.(string))
	}

	// On-disk bytes match what fetchBodyFromIMAP returned (the substrate
	// writes the raw RFC822 bytes verbatim).
	readR := core.ReadFile(pathR.Value.(string))
	if !readR.OK {
		t.Fatalf("read cached body: %s", readR.Error())
	}
	if got := string(readR.Value.([]byte)); got != want {
		t.Errorf("on-disk body = %q; want %q", got, want)
	}
}

// TestMail_BodyWrite_RewriteIdempotentOnSameContent_Good — the
// second FetchBody call for a cached thread-id serves from disk and
// does NOT trigger a re-write (cache hit). Confirms the cache path
// returns the same Plain output without invoking the bodyFetchOverride
// a second time.
func TestMail_BodyWrite_RewriteIdempotentOnSameContent_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	c := core.New()
	svc := NewService(c)
	ap := newTestAccountProvider(t)
	svc.SetAccountService(ap)

	const want = "From: foo\r\n\r\nsecond call must hit cache"
	calls := 0
	svc.bodyFetchOverride = func(_ FetchBodyInput) ([]byte, error) {
		calls++
		return []byte(want), nil
	}

	// First call → IMAP fetch + on-disk write.
	in := FetchBodyInput{
		AccountName: "personal",
		FolderSlug:  "inbox",
		ThreadID:    "xyz789",
	}
	if r := svc.FetchBody(in); !r.OK {
		t.Fatalf("first FetchBody failed: %s", r.Error())
	}
	if calls != 1 {
		t.Fatalf("after first call: bodyFetchOverride invoked %d times; want 1", calls)
	}

	// Second call → cache hit, fetchBodyFromIMAP MUST NOT be invoked.
	r2 := svc.FetchBody(in)
	if !r2.OK {
		t.Fatalf("second FetchBody failed: %s", r2.Error())
	}
	if calls != 1 {
		t.Errorf("after second call: bodyFetchOverride invoked %d times; want 1 (cache miss)", calls)
	}
	out2, _ := r2.Value.(FetchBodyOutput)
	if out2.Plain != "second call must hit cache" {
		t.Errorf("Plain on cache hit = %q; want %q", out2.Plain, "second call must hit cache")
	}
}

// TestMail_FetchBody_InvalidInputs_Bad — surface-level guards:
// empty account name, invalid folder slug, invalid thread id. Each
// MUST reject BEFORE any disk side-effect.
func TestMail_FetchBody_InvalidInputs_Bad(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	c := core.New()
	svc := NewService(c)
	ap := newTestAccountProvider(t)
	svc.SetAccountService(ap)

	cases := []struct {
		name string
		in   FetchBodyInput
		want string
	}{
		{
			name: "empty_account",
			in:   FetchBodyInput{AccountName: "", FolderSlug: "inbox", ThreadID: "abc"},
			want: "account_name is required",
		},
		{
			name: "invalid_folder_slug",
			in:   FetchBodyInput{AccountName: "personal", FolderSlug: "../etc/passwd", ThreadID: "abc"},
			want: "invalid folder slug",
		},
		{
			name: "invalid_thread_id",
			in:   FetchBodyInput{AccountName: "personal", FolderSlug: "inbox", ThreadID: "../escape"},
			want: "invalid thread id",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := svc.FetchBody(tc.in)
			if r.OK {
				t.Fatalf("FetchBody(%v) succeeded; want Fail", tc.in)
			}
			if !core.Contains(r.Error(), tc.want) {
				t.Errorf("error = %q; want substring %q", r.Error(), tc.want)
			}
		})
	}
}

// TestMail_FetchBody_LockedSession_Bad — FetchBody returns
// mail.session.locked when the session is paused, mirroring
// FetchOnce. No disk side-effect.
func TestMail_FetchBody_LockedSession_Bad(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	c := core.New()
	svc := NewService(c)
	ap := newTestAccountProvider(t)
	svc.SetAccountService(ap)
	svc.paused.Store(true)

	r := svc.FetchBody(FetchBodyInput{
		AccountName: "personal",
		FolderSlug:  "inbox",
		ThreadID:    "abc",
	})
	if r.OK {
		t.Fatal("FetchBody MUST fail when session is locked")
	}
	if !core.Contains(r.Error(), "mail.session.locked") &&
		!core.Contains(r.Error(), "sign in") {
		t.Errorf("error = %q; want session.locked", r.Error())
	}
}
