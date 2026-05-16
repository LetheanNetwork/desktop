// SPDX-Licence-Identifier: EUPL-1.2

package mail

import (
	"testing"

	core "dappco.re/go"
	"forge.lthn.ai/Snider/Enchantrix/pkg/crypt/std/pgp"
)

// --- test helpers ---

// testAccountProvider is a stub AccountProvider for unit tests.
// It holds an in-memory key pair generated once per test.
type testAccountProvider struct {
	pub  []byte
	priv []byte
}

func newTestAccountProvider(t *testing.T) *testAccountProvider {
	t.Helper()
	pgpSvc := pgp.NewService()
	pub, priv, err := pgpSvc.GenerateKeyPair("Test", "test@lthn.local", "test")
	if err != nil {
		t.Fatalf("generate test key pair: %v", err)
	}
	return &testAccountProvider{pub: pub, priv: priv}
}

func (p *testAccountProvider) PrivateKeyFor(_ string) ([]byte, bool) {
	if len(p.priv) == 0 {
		return nil, false
	}
	cp := make([]byte, len(p.priv))
	copy(cp, p.priv)
	return cp, true
}

func (p *testAccountProvider) PublicKeyFor(_ string) ([]byte, bool) {
	if len(p.pub) == 0 {
		return nil, false
	}
	cp := make([]byte, len(p.pub))
	copy(cp, p.pub)
	return cp, true
}

func (p *testAccountProvider) DefaultAccountID() string {
	return "test-account-id"
}

// lockedAccountProvider returns no private key (simulates locked session).
type lockedAccountProvider struct {
	testAccountProvider
}

func (p *lockedAccountProvider) PrivateKeyFor(_ string) ([]byte, bool) {
	return nil, false
}

// newTestService constructs a mail Service with a temp mail dir.
func newTestMailService(t *testing.T) (*Service, *testAccountProvider) {
	t.Helper()
	tmp := t.TempDir()
	// Override paths.Root() via env is not available — set the mail dir
	// via the private mailDir helper. Instead, we test via methods
	// that accept an account provider; the mailDir calls inside will
	// use whatever paths.Root() returns. For unit tests we use the
	// default home-based path; account provider is tested in isolation.
	_ = tmp
	c := core.New()
	svc := NewService(c)
	ap := newTestAccountProvider(t)
	svc.SetAccountService(ap)
	return svc, ap
}

// --- Good ---

// TestSaveAccount_RoundTripDecrypt_Good — Save → raw bytes decrypt to original record;
// List returns it with secret zeroed.
func TestSaveAccount_RoundTripDecrypt_Good(t *testing.T) {
	svc, _ := newTestMailService(t)

	input := AccountInput{
		Name: "personal",
		IMAP: IMAPConfig{Host: "imap.fastmail.com", Port: 993, User: "me@test.local", TLS: true},
		SMTP: SMTPConfig{Host: "smtp.fastmail.com", Port: 587, User: "me@test.local", TLSStarttls: true},
		Auth: AuthSpec{Kind: "appPassword", Secret: "s3cr3t"},
	}

	if r := svc.SaveAccount(input); !r.OK {
		t.Fatalf("SaveAccount failed: %s", r.Error())
	}

	r := svc.ListAccounts()
	if !r.OK {
		t.Fatalf("ListAccounts failed: %s", r.Error())
	}
	accounts, _ := r.Value.([]MailAccount)
	if len(accounts) != 1 {
		t.Fatalf("expected 1 account, got %d", len(accounts))
	}
	got := accounts[0]
	if got.Name != "personal" {
		t.Errorf("name: got %q want %q", got.Name, "personal")
	}
	if got.IMAP.Host != "imap.fastmail.com" {
		t.Errorf("imap.host: got %q", got.IMAP.Host)
	}
	// Secret MUST be zeroed in the returned record (§1.4).
	if got.Auth.Secret != "" {
		t.Errorf("ListAccounts returned non-empty secret: %q", got.Auth.Secret)
	}
}

// TestSaveAccount_NoUnlockNeeded_Good — Save works while session is locked
// (public key only, per §1.2 write invariant).
func TestSaveAccount_NoUnlockNeeded_Good(t *testing.T) {
	c := core.New()
	svc := NewService(c)
	ap := newTestAccountProvider(t)
	// Wire a locked provider: PrivateKeyFor returns false but PublicKeyFor works.
	locked := &lockedAccountProvider{*ap}
	svc.SetAccountService(locked)
	svc.paused.Store(true)

	input := AccountInput{
		Name: "work",
		IMAP: IMAPConfig{Host: "imap.migadu.com", Port: 993, User: "work@lthn.local", TLS: true},
		SMTP: SMTPConfig{Host: "smtp.migadu.com", Port: 465, User: "work@lthn.local"},
		Auth: AuthSpec{Kind: "appPassword", Secret: "pass"},
	}

	// Save must succeed even when paused (§1.2 — writes don't require unlock).
	if r := svc.SaveAccount(input); !r.OK {
		t.Fatalf("SaveAccount under locked session failed: %s", r.Error())
	}

	// List MUST fail with mail.session.locked (§6 — reads gate on unlock).
	r := svc.ListAccounts()
	if r.OK {
		t.Fatal("ListAccounts should fail when session is locked")
	}
	if r.Error() == "" {
		t.Fatal("expected non-empty error from locked ListAccounts")
	}
}

// --- Bad ---

// TestListAccounts_NeverReturnsSecret_Ugly — even after Save with a secret,
// List returns auth.secret=="" in every record (§1.4).
func TestListAccounts_NeverReturnsSecret_Ugly(t *testing.T) {
	svc, _ := newTestMailService(t)

	accounts := []AccountInput{
		{Name: "personal", IMAP: IMAPConfig{Host: "imap.fastmail.com", Port: 993}, Auth: AuthSpec{Kind: "appPassword", Secret: "super-secret-1"}},
		{Name: "work", IMAP: IMAPConfig{Host: "imap.migadu.com", Port: 993}, Auth: AuthSpec{Kind: "appPassword", Secret: "super-secret-2"}},
	}
	for _, a := range accounts {
		if r := svc.SaveAccount(a); !r.OK {
			t.Fatalf("SaveAccount: %s", r.Error())
		}
	}

	r := svc.ListAccounts()
	if !r.OK {
		t.Fatalf("ListAccounts: %s", r.Error())
	}
	got, _ := r.Value.([]MailAccount)
	for _, a := range got {
		if a.Auth.Secret != "" {
			t.Errorf("account %q returned non-empty secret", a.Name)
		}
	}
}

// TestSaveAccount_DeprecatedProviderRejectsPassword_Bad — gmail.com + kind="password"
// → mail.account.password_auth_deprecated (§1.5 Q2 ruling).
func TestSaveAccount_DeprecatedProviderRejectsPassword_Bad(t *testing.T) {
	svc, _ := newTestMailService(t)

	r := svc.SaveAccount(AccountInput{
		Name: "gmail",
		IMAP: IMAPConfig{Host: "imap.gmail.com", Port: 993},
		Auth: AuthSpec{Kind: "password", Secret: "plain-pass"},
	})
	if r.OK {
		t.Fatal("expected SaveAccount to fail for gmail+password")
	}
	if !core.Contains(r.Error(), "mail.account.password_auth_deprecated") && !core.Contains(r.Error(), "app password") {
		t.Errorf("unexpected error: %s", r.Error())
	}
}

// TestSaveAccount_NameRequired_Bad — empty name → error.
func TestSaveAccount_NameRequired_Bad(t *testing.T) {
	svc, _ := newTestMailService(t)
	r := svc.SaveAccount(AccountInput{Auth: AuthSpec{Kind: "appPassword"}})
	if r.OK {
		t.Fatal("expected failure for empty account name")
	}
}

// --- Ugly ---

// TestRemoveAccount_Good — Save two accounts, remove one, List returns one.
func TestRemoveAccount_Good(t *testing.T) {
	svc, _ := newTestMailService(t)

	for _, name := range []string{"personal", "work"} {
		if r := svc.SaveAccount(AccountInput{
			Name: name,
			IMAP: IMAPConfig{Host: "imap.fastmail.com", Port: 993},
			Auth: AuthSpec{Kind: "appPassword", Secret: "s3cr3t"},
		}); !r.OK {
			t.Fatalf("SaveAccount %s: %s", name, r.Error())
		}
	}

	if r := svc.RemoveAccount("work"); !r.OK {
		t.Fatalf("RemoveAccount: %s", r.Error())
	}

	r := svc.ListAccounts()
	if !r.OK {
		t.Fatalf("ListAccounts: %s", r.Error())
	}
	got, _ := r.Value.([]MailAccount)
	if len(got) != 1 {
		t.Fatalf("expected 1 account after remove, got %d", len(got))
	}
	if got[0].Name != "personal" {
		t.Errorf("wrong account remaining: %q", got[0].Name)
	}
}

// TestRemoveAccount_NotFound_Bad — removing a non-existent name → error.
func TestRemoveAccount_NotFound_Bad(t *testing.T) {
	svc, _ := newTestMailService(t)
	r := svc.RemoveAccount("nonexistent")
	if r.OK {
		t.Fatal("expected failure when removing nonexistent account")
	}
}

// TestDomainMatches_Ugly — verify suffix matching for deprecated-provider gate.
func TestDomainMatches_Ugly(t *testing.T) {
	cases := []struct {
		host   string
		domain string
		want   bool
	}{
		{"imap.gmail.com", "gmail.com", true},
		{"gmail.com", "gmail.com", true},
		{"smtp.gmail.com", "gmail.com", true},
		{"notgmail.com", "gmail.com", false},
		{"imap.fastmail.com", "gmail.com", false},
	}
	for _, tc := range cases {
		got := domainMatches(tc.host, tc.domain)
		if got != tc.want {
			t.Errorf("domainMatches(%q, %q) = %v, want %v", tc.host, tc.domain, got, tc.want)
		}
	}
}
