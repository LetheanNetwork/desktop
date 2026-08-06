// SPDX-Licence-Identifier: EUPL-1.2

// accounts_extra_test.go — provider-not-wired and no-public-key
// branches across SaveAccount / ListAccounts / RemoveAccount that
// accounts_test.go's happy-path + multi-unlock suites don't reach.

package mail

import (
	"testing"

	core "dappco.re/go"
)

// noPubKeyAccountProvider has a private key but PublicKeyFor always
// fails — simulates an account whose public key can't be read from
// disk (corrupt/missing keyring entry).
type noPubKeyAccountProvider struct {
	testAccountProvider
}

func (p *noPubKeyAccountProvider) PublicKeyFor(_ string) ([]byte, bool) {
	return nil, false
}

// TestSaveAccount_ProviderNotWired_Bad — SetAccountService was never
// called.
func TestSaveAccount_ProviderNotWired_Bad(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := NewService(core.New())
	r := svc.SaveAccount(AccountInput{
		Name: "personal",
		Auth: AuthSpec{Kind: "appPassword", Secret: "x"},
	})
	if r.OK {
		t.Fatal("expected SaveAccount to fail when the account provider isn't wired")
	}
	if !core.Contains(r.Error(), "account provider not wired") {
		t.Errorf("unexpected error: %s", r.Error())
	}
}

// TestSaveAccount_KindRequired_Bad — empty Auth.Kind rejected before
// touching the account provider.
func TestSaveAccount_KindRequired_Bad(t *testing.T) {
	svc, _ := newTestMailService(t)
	r := svc.SaveAccount(AccountInput{Name: "personal"})
	if r.OK {
		t.Fatal("expected SaveAccount to reject an empty auth.kind")
	}
	if !core.Contains(r.Error(), "mail.account.kind_required") {
		t.Errorf("unexpected error: %s", r.Error())
	}
}

// TestSaveAccount_NoPublicKey_Bad — PublicKeyFor fails even though a
// private key exists.
func TestSaveAccount_NoPublicKey_Bad(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	c := core.New()
	svc := NewService(c)
	ap := newTestAccountProvider(t)
	svc.SetAccountService(&noPubKeyAccountProvider{*ap})

	r := svc.SaveAccount(AccountInput{
		Name: "personal",
		Auth: AuthSpec{Kind: "appPassword", Secret: "x"},
	})
	if r.OK {
		t.Fatal("expected SaveAccount to fail when PublicKeyFor fails")
	}
	if !core.Contains(r.Error(), "mail.account.no_public_key") {
		t.Errorf("unexpected error: %s", r.Error())
	}
}

// TestSaveAccount_UpsertExisting_Good — saving the same account name
// twice UPDATES the record in place rather than appending a
// duplicate.
func TestSaveAccount_UpsertExisting_Good(t *testing.T) {
	svc, _ := newTestMailService(t)

	base := AccountInput{
		Name: "personal",
		IMAP: IMAPConfig{Host: "imap.fastmail.com", Port: 993},
		Auth: AuthSpec{Kind: "appPassword", Secret: "first-secret"},
	}
	if r := svc.SaveAccount(base); !r.OK {
		t.Fatalf("first SaveAccount: %s", r.Error())
	}

	updated := base
	updated.IMAP.Host = "imap.migadu.com"
	updated.Auth.Secret = "second-secret"
	if r := svc.SaveAccount(updated); !r.OK {
		t.Fatalf("second SaveAccount (upsert): %s", r.Error())
	}

	r := svc.ListAccounts()
	if !r.OK {
		t.Fatalf("ListAccounts: %s", r.Error())
	}
	accounts, _ := r.Value.([]MailAccount)
	if len(accounts) != 1 {
		t.Fatalf("expected upsert to keep exactly 1 account, got %d", len(accounts))
	}
	if accounts[0].IMAP.Host != "imap.migadu.com" {
		t.Errorf("expected upsert to update IMAP host, got %q", accounts[0].IMAP.Host)
	}
}

// TestListAccounts_ProviderNotWired_Bad — SetAccountService was
// never called (session not "locked", just entirely unwired).
func TestListAccounts_ProviderNotWired_Bad(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := NewService(core.New())
	r := svc.ListAccounts()
	if r.OK {
		t.Fatal("expected ListAccounts to fail when the account provider isn't wired")
	}
	if !core.Contains(r.Error(), "account provider not wired") {
		t.Errorf("unexpected error: %s", r.Error())
	}
}

// TestRemoveAccount_ProviderNotWired_Bad
func TestRemoveAccount_ProviderNotWired_Bad(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := NewService(core.New())
	r := svc.RemoveAccount("personal")
	if r.OK {
		t.Fatal("expected RemoveAccount to fail when the account provider isn't wired")
	}
	if !core.Contains(r.Error(), "account provider not wired") {
		t.Errorf("unexpected error: %s", r.Error())
	}
}

// TestRemoveAccount_NoPublicKey_Bad — PrivateKeyFor succeeds (so the
// method gets past the lock gate) but PublicKeyFor fails — RemoveAccount
// needs the pubkey to re-encrypt the filtered list.
func TestRemoveAccount_NoPublicKey_Bad(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	c := core.New()
	svc := NewService(c)
	ap := newTestAccountProvider(t)
	svc.SetAccountService(ap)
	if r := svc.SaveAccount(AccountInput{
		Name: "personal",
		Auth: AuthSpec{Kind: "appPassword", Secret: "x"},
	}); !r.OK {
		t.Fatalf("SaveAccount setup: %s", r.Error())
	}

	svc.SetAccountService(&noPubKeyAccountProvider{*ap})
	r := svc.RemoveAccount("personal")
	if r.OK {
		t.Fatal("expected RemoveAccount to fail when PublicKeyFor fails")
	}
	if !core.Contains(r.Error(), "mail.account.no_public_key") {
		t.Errorf("unexpected error: %s", r.Error())
	}
}
