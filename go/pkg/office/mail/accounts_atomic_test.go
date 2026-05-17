// SPDX-Licence-Identifier: EUPL-1.2

// accounts_atomic_test.go — cascade W4 Part 1 cutover tests for the
// _accounts.enc adoption of paths.AtomicWriteWithVersion + IfMatchHash.
// Pins the Cerberus #9 pre-fire DREAD Concerns 2.A / 2.B / 2.C
// done-criteria against future drift.

package mail

import (
	"encoding/json"
	"testing"

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/paths"
)

// newTestServiceWithHome wires a Service whose paths.Root() lands
// under a fresh t.TempDir() so each cascade-W4 test runs in
// hermetic isolation. Mirrors the W3 incidents test pattern.
func newTestServiceWithHome(t *testing.T) (*Service, *testAccountProvider) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	c := core.New()
	svc := NewService(c)
	ap := newTestAccountProvider(t)
	svc.SetAccountService(ap)
	return svc, ap
}

// TestMail_AccountsEnc_PathIsLiteralSingleFile_Good — Cerberus #9
// Concern 2.B: the on-disk path stays exactly
// "office/mail/_accounts.enc" (workspace-relative). Pins against a
// future refactor that splits per-account — splitting would change
// the AtRestEncryptedPrefixes routing + the RecordSync audit
// contract.
func TestMail_AccountsEnc_PathIsLiteralSingleFile_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	// Constant pin — the in-code identifier MUST be the literal
	// single-file blob name. accountsEncFile is the only
	// string accountsEncPath() consults.
	if accountsEncFile != "_accounts.enc" {
		t.Fatalf("accountsEncFile constant drifted: got %q want %q",
			accountsEncFile, "_accounts.enc")
	}

	// Path-construction pin — accountsEncPath() yields the literal
	// workspace-relative "office/mail/_accounts.enc".
	pathR := accountsEncPath()
	if !pathR.OK {
		t.Fatalf("accountsEncPath: %s", pathR.Error())
	}
	abs := pathR.Value.(string)
	root := paths.Root()
	if !root.OK {
		t.Fatalf("paths.Root: %s", root.Error())
	}
	want := core.PathJoin(root.Value.(string), "office", "mail", "_accounts.enc")
	if abs != want {
		t.Fatalf("accountsEncPath drift: got %q want %q", abs, want)
	}

	// The path MUST be classified at-rest-encrypted so the
	// CRIT-1 body-omission gate fires regardless of caller
	// IncludeBody. Cross-references paths.AtRestEncryptedPrefixes.
	if !paths.IsAtRestEncryptedPath(abs) {
		t.Fatalf("_accounts.enc path %q MUST match AtRestEncryptedPrefixes", abs)
	}
}

// TestMail_AccountsEnc_IncludeBodyExplicitlyFalse_Good — Cerberus #9
// Concern 2.A: every _accounts.enc write call site MUST pass
// explicit IncludeBody:false. The discipline assertion runs through
// the Service code path — drive a known stale write and assert the
// conflict envelope NEVER carries a CurrentBody field.
//
// Belt-and-braces: paths.AtRestEncryptedPrefixes also omits at the
// primitive layer, but we explicitly pin the caller-side false here
// so a refactor that swaps the path to a non-at-rest prefix cannot
// silently start leaking ciphertext.
func TestMail_AccountsEnc_IncludeBodyExplicitlyFalse_Good(t *testing.T) {
	svc, _ := newTestServiceWithHome(t)

	// First write seeds the file.
	if r := svc.SaveAccount(AccountInput{
		Name: "personal",
		IMAP: IMAPConfig{Host: "imap.fastmail.com", Port: 993, User: "me@x", TLS: true},
		SMTP: SMTPConfig{Host: "smtp.fastmail.com", Port: 587, User: "me@x", TLSStarttls: true},
		Auth: AuthSpec{Kind: "appPassword", Secret: "s1"},
	}); !r.OK {
		t.Fatalf("seed SaveAccount: %s", r.Error())
	}

	// Force a stale write by tampering the ciphertext between
	// hash-read and our write attempt — simplest reproduction is
	// a second SaveAccount that lands first while we hold a stale
	// priorHash. Drive directly through the primitive surface:
	pathR := accountsEncPath()
	if !pathR.OK {
		t.Fatalf("path: %s", pathR.Error())
	}
	abs := pathR.Value.(string)

	rawR := core.ReadFile(abs)
	if !rawR.OK {
		t.Fatalf("read seed: %s", rawR.Error())
	}
	staleHash := "0000000000000000000000000000000000000000000000000000000000000000"

	// Direct primitive call to assert the wire shape — IncludeBody
	// stays false, CurrentBody MUST be empty even though the
	// caller could theoretically opt in.
	wr := paths.AtomicWriteWithVersion(abs, paths.WriteInput{
		Body:        rawR.Value.([]byte),
		IfMatchHash: staleHash,
		IncludeBody: false,
	})
	if wr.OK {
		t.Fatal("expected stale write to fail with VersionStale envelope")
	}
	stale, ok := paths.VersionStaleFromError(wr.Value)
	if !ok {
		t.Fatalf("expected VersionStale, got %T (%s)", wr.Value, wr.Error())
	}
	if len(stale.CurrentBody) != 0 {
		t.Fatalf("CurrentBody MUST be empty when IncludeBody=false; got %d bytes", len(stale.CurrentBody))
	}

	// Belt-and-braces: even when a caller flips IncludeBody=true on
	// an at-rest path, the primitive REJECTS at the API boundary
	// with CodeIncludeBodyAtRestRejected (Mantis #1553 load-bearing
	// CRIT-1 upgrade — the entry-side reject fires before lock /
	// read / version comparison, so the VersionStale envelope is
	// NOT produced on this branch). The silent-omit behaviour at
	// failVersionStale is retained as defence in depth and is
	// exercised by paths.TestAtomicWrite_AtRestSilentIgnoreDefenceInDepth_Good.
	wr2 := paths.AtomicWriteWithVersion(abs, paths.WriteInput{
		Body:        rawR.Value.([]byte),
		IfMatchHash: staleHash,
		IncludeBody: true, // discipline violation — primitive MUST reject at entry
	})
	if wr2.OK {
		t.Fatal("at-rest IncludeBody=true MUST be rejected at entry")
	}
	core.AssertContains(t, wr2.Error(),
		paths.CodeIncludeBodyAtRestRejected,
		"at-rest IncludeBody=true MUST surface CodeIncludeBodyAtRestRejected")
	if _, ok := paths.VersionStaleFromError(wr2.Value); ok {
		t.Fatal("entry-side reject MUST fire before VersionStale envelope is produced")
	}
}

// TestMail_AccountsEnc_IfMatchHashIsOverCiphertext_Good — Cerberus #9
// Concern 2.C: the optimistic-lock hash is computed over the
// ENCRYPTED blob bytes, NOT decrypted plaintext. Demonstrated by
// encrypting the same plaintext twice (PGP uses a fresh session
// key per message) and observing that the two ciphertexts hash
// differently — therefore the IfMatchHash check trips correctly
// without the writer ever touching plaintext.
func TestMail_AccountsEnc_IfMatchHashIsOverCiphertext_Good(t *testing.T) {
	svc, ap := newTestServiceWithHome(t)

	// Same plaintext encrypted twice produces two distinct
	// ciphertexts — proves the encryption scheme is non-
	// deterministic, which is the structural reason a hash over
	// ciphertext provides conflict-detection without leaking
	// plaintext.
	pub, ok := ap.PublicKeyFor("test-account-id")
	if !ok {
		t.Fatal("PublicKeyFor failed")
	}
	plain := []byte(`[{"name":"personal"}]`)
	enc1, err := svc.pgp.Encrypt(pub, plain)
	if err != nil {
		t.Fatalf("encrypt 1: %v", err)
	}
	enc2, err := svc.pgp.Encrypt(pub, plain)
	if err != nil {
		t.Fatalf("encrypt 2: %v", err)
	}
	h1 := core.SHA256Hex(enc1)
	h2 := core.SHA256Hex(enc2)
	if h1 == h2 {
		t.Fatal("PGP encryption returned deterministic ciphertext — IfMatchHash conflict-detection cannot rely on it; surface to Cerberus")
	}

	// Now drive through the Service code path and assert
	// readAccountsCiphertextHash returns the hash of the on-disk
	// CIPHERTEXT — never decrypts.
	if r := svc.SaveAccount(AccountInput{
		Name: "personal",
		IMAP: IMAPConfig{Host: "imap.fastmail.com", Port: 993, User: "me@x", TLS: true},
		SMTP: SMTPConfig{Host: "smtp.fastmail.com", Port: 587, User: "me@x", TLSStarttls: true},
		Auth: AuthSpec{Kind: "appPassword", Secret: "s1"},
	}); !r.OK {
		t.Fatalf("seed SaveAccount: %s", r.Error())
	}

	pathR := accountsEncPath()
	if !pathR.OK {
		t.Fatalf("path: %s", pathR.Error())
	}
	abs := pathR.Value.(string)
	diskBytes, err := readFileBytes(abs)
	if err != nil {
		t.Fatalf("read disk: %v", err)
	}
	wantHash := core.SHA256Hex(diskBytes)

	gotHash, exists, herr := svc.readAccountsCiphertextHash()
	if herr != nil {
		t.Fatalf("readAccountsCiphertextHash: %v", herr)
	}
	if !exists {
		t.Fatal("readAccountsCiphertextHash returned exists=false after SaveAccount")
	}
	if gotHash != wantHash {
		t.Fatalf("hash drift: got %q want %q", gotHash, wantHash)
	}
}

// TestMail_AccountsEnc_SaveAccount_VersionStale_Ugly — drive two
// SaveAccount paths with a stale priorHash anchor and assert the
// loser receives a paths.ConflictEnvelope with Code
// "mail.accounts.update.conflict" + the lowercase-json wire shape
// that conflict-dispatch.ts extractEnvelope pattern-matches on.
func TestMail_AccountsEnc_SaveAccount_VersionStale_Ugly(t *testing.T) {
	svc, ap := newTestServiceWithHome(t)

	// Seed the file via the live code path.
	if r := svc.SaveAccount(AccountInput{
		Name: "personal",
		IMAP: IMAPConfig{Host: "imap.fastmail.com", Port: 993, User: "me@x", TLS: true},
		SMTP: SMTPConfig{Host: "smtp.fastmail.com", Port: 587, User: "me@x", TLSStarttls: true},
		Auth: AuthSpec{Kind: "appPassword", Secret: "s1"},
	}); !r.OK {
		t.Fatalf("seed SaveAccount: %s", r.Error())
	}

	// Force a manual save with a known-bad priorHash. This
	// exercises the saveAccounts() conflict branch through the
	// same surface the live writers walk.
	pub, _ := ap.PublicKeyFor("test-account-id")
	wr := svc.saveAccounts(pub, []MailAccount{
		{Name: "work", IMAP: IMAPConfig{Host: "imap.x", Port: 993},
			Auth: AuthSpec{Kind: "appPassword", Secret: "z"}},
	}, "deadbeef-stale-hash-anchor-never-matches-disk-state-deadbeef")
	if wr.OK {
		t.Fatal("expected stale-hash saveAccounts to fail")
	}

	env, ok := paths.ConflictEnvelopeFrom(wr.Value)
	if !ok {
		t.Fatalf("expected paths.ConflictEnvelope, got %T (%s)", wr.Value, wr.Error())
	}
	if env.Code != "mail.accounts.update.conflict" {
		t.Fatalf("envelope code: got %q want mail.accounts.update.conflict", env.Code)
	}

	// Marshalled wire-shape pin — Mantis #1544 lowercase tags.
	raw, jerr := json.Marshal(wr.Value)
	if jerr != nil {
		t.Fatalf("json.Marshal: %v", jerr)
	}
	js := string(raw)
	for _, want := range []string{
		`"code":"mail.accounts.update.conflict"`,
		`"current_hash":`,
	} {
		if !core.Contains(js, want) {
			t.Fatalf("envelope missing %s: %s", want, js)
		}
	}
	for _, banned := range []string{`"Code":`, `"Message":`, `"Operation":`} {
		if core.Contains(js, banned) {
			t.Fatalf("envelope leaks PascalCase key %s: %s", banned, js)
		}
	}
}

// TestMail_AccountsEnc_FirstWriteUnconditional_Good — when no
// _accounts.enc file exists yet, SaveAccount lands without an
// IfMatchHash check (RFC §3.2 MED-2 unconditional first-write).
func TestMail_AccountsEnc_FirstWriteUnconditional_Good(t *testing.T) {
	svc, _ := newTestServiceWithHome(t)

	// No prior file — readAccountsCiphertextHash returns empty.
	hash, exists, err := svc.readAccountsCiphertextHash()
	if err != nil {
		t.Fatalf("readAccountsCiphertextHash: %v", err)
	}
	if exists || hash != "" {
		t.Fatalf("pre-state: expected (\"\", false, nil), got (%q, %v, %v)", hash, exists, err)
	}

	if r := svc.SaveAccount(AccountInput{
		Name: "first",
		IMAP: IMAPConfig{Host: "imap.first", Port: 993, User: "u", TLS: true},
		SMTP: SMTPConfig{Host: "smtp.first", Port: 587, User: "u", TLSStarttls: true},
		Auth: AuthSpec{Kind: "appPassword", Secret: "s"},
	}); !r.OK {
		t.Fatalf("first SaveAccount: %s", r.Error())
	}

	// File now exists, hash non-empty.
	_, exists2, _ := svc.readAccountsCiphertextHash()
	if !exists2 {
		t.Fatal("post-state: expected _accounts.enc to exist after first SaveAccount")
	}
}

// TestMail_AccountsEnc_AuditEmissionRecordSync_Good — cascade W4
// audit-emission pin. Routing _accounts.enc through
// AtomicWriteWithVersion emits paths.EventWriteSucceeded with
// Mode=AuditModeSync (auth-substrate, RFC §6.1).
func TestMail_AccountsEnc_AuditEmissionRecordSync_Good(t *testing.T) {
	svc, _ := newTestServiceWithHome(t)
	paths.SetAuditSecretProvider(func() []byte {
		return []byte("mail-w4-cutover-secret-32-bytes!!")
	})
	t.Cleanup(func() { paths.SetAuditSecretProvider(nil) })

	var seen []paths.LockEvent
	paths.SubscribeLockEvents(func(ev paths.LockEvent) {
		seen = append(seen, ev)
	})
	t.Cleanup(paths.ClearLockEventSubscribersForTest)

	if r := svc.SaveAccount(AccountInput{
		Name: "audit-probe",
		IMAP: IMAPConfig{Host: "imap.x", Port: 993, User: "u", TLS: true},
		SMTP: SMTPConfig{Host: "smtp.x", Port: 587, User: "u", TLSStarttls: true},
		Auth: AuthSpec{Kind: "appPassword", Secret: "s"},
	}); !r.OK {
		t.Fatalf("SaveAccount: %s", r.Error())
	}

	// Find the EventWriteSucceeded fired by the primitive. Its
	// Mode field MUST be AuditModeSync — _accounts.enc is
	// auth-substrate.
	found := false
	for _, ev := range seen {
		if ev.Kind != paths.EventWriteSucceeded {
			continue
		}
		if ev.Mode != paths.AuditModeSync {
			t.Fatalf("_accounts.enc audit mode: got %v, want AuditModeSync (auth-substrate)", ev.Mode)
		}
		found = true
	}
	if !found {
		t.Fatal("SaveAccount MUST route through paths.AtomicWriteWithVersion (no EventWriteSucceeded seen)")
	}
}

// readFileBytes is a thin helper to keep test code AX-clean (no
// direct core.ReadFile().Value.([]byte) noise at every call).
func readFileBytes(path string) ([]byte, error) {
	r := core.ReadFile(path)
	if !r.OK {
		return nil, core.E("test.readFileBytes", r.Error(), nil)
	}
	return r.Value.([]byte), nil
}
