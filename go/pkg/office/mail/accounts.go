// SPDX-Licence-Identifier: EUPL-1.2

// accounts.go — credential storage CRUD for pkg/office/mail.
// Encrypted storage: _accounts.enc (mode 0o600, PGP asymmetric).
// Writes use the user's public key (no unlock required per §1.2).
// Reads use the in-memory private key (unlock required per §1.2).

package mail

import core "dappco.re/go"

// deprecatedProviderHosts maps domain suffix → provider-specific app-password URL.
// When Auth.Kind=="password" for one of these domains, SaveAccount returns
// mail.account.password_auth_deprecated (§1.5 Q2 ruling).
var deprecatedProviderHints = map[string]string{
	"gmail.com":   "https://myaccount.google.com/apppasswords",
	"outlook.com": "https://account.microsoft.com/security",
	"live.com":    "https://account.microsoft.com/security",
	"hotmail.com": "https://account.microsoft.com/security",
	"yahoo.com":   "https://help.yahoo.com/kb/generate-manage-third-party-passwords-sln15241.html",
	"aol.com":     "https://help.aol.com/articles/Create-and-manage-app-password",
}

// SaveAccount persists a new or updated mail account credential to _accounts.enc.
// Writes via Enchantrix pgp.Encrypt(pubKey, …) — does not require unlock.
// Validates deprecated-provider auth kinds per §1.5 Q2 ruling.
//
// Usage example:
//
//	r := svc.SaveAccount(mail.AccountInput{Name: "personal", IMAP: imap, SMTP: smtp, Auth: auth})
//	if !r.OK { /* handle */ }
func (s *Service) SaveAccount(input AccountInput) core.Result {
	if input.Name == "" {
		return core.Fail(core.NewCode("mail.account.name_required", "account name is required"))
	}

	// §1.5 Q2 — reject password auth for deprecated providers.
	if input.Auth.Kind == "password" {
		for domain, hint := range deprecatedProviderHints {
			if domainMatches(input.IMAP.Host, domain) {
				return core.Fail(core.NewCode("mail.account.password_auth_deprecated",
					core.Sprintf("password auth is not supported for %s; use an app password: %s", domain, hint)))
			}
		}
	}
	// §1.5 Q4 — OAuth2 error path for MSA configurations.
	if input.Auth.Kind == "appPassword" {
		for domain := range deprecatedProviders {
			if domainMatches(input.IMAP.Host, domain) {
				// Some MSA configs disable app-passwords entirely.
				// We can't detect this at save time; IMAP connect will fail.
				// Per Q4 we do NOT reject here — fail at connect with
				// mail.account.provider_requires_oauth2 if the server refuses.
				_ = domain
			}
		}
	}
	if input.Auth.Kind == "" {
		return core.Fail(core.NewCode("mail.account.kind_required", "auth.kind is required (appPassword or oauth2)"))
	}

	// Resolve the public key. Must be supplied by the AccountProvider.
	if s.account == nil {
		return core.Fail(core.E("mail.SaveAccount", "account provider not wired (call SetAccountService)", nil))
	}
	// Mantis #1591 Option D — assert exactly one unlocked account.
	// Previous DefaultAccountID() lottery silently mis-bound under
	// multi-unlock (Mantis #1588); the explicit error surfaces the
	// gap instead.
	accountID, idErr := s.singleUnlockedAccount()
	if idErr != nil {
		return core.Fail(idErr)
	}
	pub, ok := s.account.PublicKeyFor(accountID)
	if !ok {
		return core.Fail(core.NewCode("mail.account.no_public_key",
			"could not read user public key — account may not be set up"))
	}

	// Snapshot the current ciphertext hash BEFORE the load-modify-save
	// cycle. Cerberus #9 pre-fire DREAD Concern 2.C: IfMatchHash is
	// over CIPHERTEXT not plaintext — read happens regardless of
	// unlock-state since hashing the encrypted blob never requires the
	// private key. Empty hash + missing file = unconditional first-
	// write (RFC §3.2 MED-2).
	priorHash, _, hashErr := s.readAccountsCiphertextHash()
	if hashErr != nil {
		return core.Fail(core.E("mail.SaveAccount", "read prior hash", hashErr))
	}

	// Load existing accounts (may be locked — decrypt attempt may fail).
	// Writes always succeed without unlock; only reads block on lock.
	// For SaveAccount: if we can't decrypt the existing list (locked), we
	// start from an empty list — the saved cred is ADDED, not replaced.
	// This preserves the "write without unlock" invariant (§1.2).
	// Mantis #1589 / Cerberus #18 — PrivateKeyFor returns a single-use
	// handle; bytes are zeroised inside Use's defer.
	var existing []MailAccount
	handle, hasPriv := s.account.PrivateKeyFor(accountID)
	if hasPriv {
		_ = handle.Use(func(priv []byte) error {
			loaded, err := s.loadAccounts(priv)
			if err != nil {
				core.Warn("mail.SaveAccount: could not decrypt existing accounts, starting fresh", "err", err)
				return nil
			}
			existing = loaded
			return nil
		})
	}

	// Upsert by name.
	found := false
	for i, a := range existing {
		if a.Name == input.Name {
			existing[i] = MailAccount{
				Name: input.Name,
				IMAP: input.IMAP,
				SMTP: input.SMTP,
				Auth: AuthSpec{Kind: input.Auth.Kind, Secret: input.Auth.Secret},
			}
			found = true
			break
		}
	}
	if !found {
		existing = append(existing, MailAccount{
			Name: input.Name,
			IMAP: input.IMAP,
			SMTP: input.SMTP,
			Auth: AuthSpec{Kind: input.Auth.Kind, Secret: input.Auth.Secret},
		})
	}

	return s.saveAccounts(pub, existing, priorHash)
}

// ListAccounts returns all saved accounts with auth.secret zeroed (§1.4).
// Requires unlock — returns mail.session.locked when paused or private key unavailable.
//
// Usage example:
//
//	r := svc.ListAccounts()
//	if r.OK { accounts := r.Value.([]mail.MailAccount) }
func (s *Service) ListAccounts() core.Result {
	if r := s.assertUnlocked(); !r.OK {
		return r
	}
	if s.account == nil {
		return core.Fail(core.E("mail.ListAccounts", "account provider not wired", nil))
	}
	// Mantis #1591 Option D — assert exactly one unlocked account.
	accountID, idErr := s.singleUnlockedAccount()
	if idErr != nil {
		return core.Fail(idErr)
	}
	handle, ok := s.account.PrivateKeyFor(accountID)
	if !ok {
		return s.errLocked()
	}

	// Mantis #1589 / Cerberus #18 — handle zeroises the key bytes on
	// release; capture result + err via outer scope.
	var (
		accounts []MailAccount
		loadErr  error
	)
	_ = handle.Use(func(priv []byte) error {
		accounts, loadErr = s.loadAccounts(priv)
		return loadErr
	})
	if loadErr != nil {
		return core.Fail(core.E("mail.ListAccounts", "decrypt accounts", loadErr))
	}

	// Zero secrets before returning (§1.4).
	for i := range accounts {
		accounts[i].Auth.Secret = ""
	}
	return core.Ok(accounts)
}

// RemoveAccount removes the named account from _accounts.enc.
// Requires unlock to read the existing list before rewriting.
//
// Usage example:
//
//	r := svc.RemoveAccount("personal")
func (s *Service) RemoveAccount(name string) core.Result {
	if name == "" {
		return core.Fail(core.NewCode("mail.account.name_required", "account name is required"))
	}
	if r := s.assertUnlocked(); !r.OK {
		return r
	}
	if s.account == nil {
		return core.Fail(core.E("mail.RemoveAccount", "account provider not wired", nil))
	}
	// Mantis #1591 Option D — assert exactly one unlocked account.
	accountID, idErr := s.singleUnlockedAccount()
	if idErr != nil {
		return core.Fail(idErr)
	}
	handle, ok := s.account.PrivateKeyFor(accountID)
	if !ok {
		return s.errLocked()
	}
	pub, hasPub := s.account.PublicKeyFor(accountID)
	if !hasPub {
		return core.Fail(core.NewCode("mail.account.no_public_key",
			"could not read user public key"))
	}

	// Snapshot ciphertext hash BEFORE the load-modify-save cycle.
	// Cerberus #9 Concern 2.C — ciphertext hash, no plaintext access.
	priorHash, _, hashErr := s.readAccountsCiphertextHash()
	if hashErr != nil {
		return core.Fail(core.E("mail.RemoveAccount", "read prior hash", hashErr))
	}

	// Mantis #1589 / Cerberus #18 — handle zeroises bytes on release;
	// capture decrypted accounts via outer scope.
	var (
		accounts []MailAccount
		loadErr  error
	)
	_ = handle.Use(func(priv []byte) error {
		accounts, loadErr = s.loadAccounts(priv)
		return loadErr
	})
	if loadErr != nil {
		return core.Fail(core.E("mail.RemoveAccount", "decrypt accounts", loadErr))
	}

	filtered := accounts[:0]
	found := false
	for _, a := range accounts {
		if a.Name == name {
			found = true
			continue
		}
		filtered = append(filtered, a)
	}
	if !found {
		return core.Fail(core.NewCode("mail.account.not_found",
			core.Sprintf("account %q not found", name)))
	}

	return s.saveAccounts(pub, filtered, priorHash)
}

// domainMatches reports whether host (IMAP/SMTP hostname) matches or
// ends with the given domain. Handles "imap.gmail.com" → "gmail.com".
func domainMatches(host, domain string) bool {
	if host == domain {
		return true
	}
	suffix := "." + domain
	return len(host) > len(suffix) && host[len(host)-len(suffix):] == suffix
}
