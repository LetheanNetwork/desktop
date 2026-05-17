// SPDX-Licence-Identifier: EUPL-1.2

// Token loader — provider auth tokens for the PR fetcher.
//
// Tokens live in pkg/keys tier-1 (account-sealed, account-unlock
// required) under refs of shape `vi-<provider>` (e.g. `vi-forge`,
// `vi-github`). The legacy config grammar `vi.tokens.<provider> =
// "<literal>"` is retained for the migration window so existing
// installs read cleanly on first launch.
//
// Migration shape (Mantis #1748 / Stage E at-rest sealing wave):
//
//   - At first launch post-deploy, MigrateLegacyTokens scans the
//     config service for any `vi.tokens.<provider>` literal entries,
//     dispatches each plaintext into the tier-1 substrate under
//     `vi-<provider>`, and blanks the config key. Idempotent: tokens
//     already absent from config are skipped.
//   - Deferred-migration: when tier-1 KEK is not yet live (no
//     account unlocked), PutTier1 fails closed; the literal stays in
//     config and the next call retries.
//   - On every successful migration step, EventViTokenMigrated
//     records the (provider, ref) pair so a forensic walker can
//     correlate "I deleted the plaintext token at T" with "the
//     ciphertext was sealed at T-ε".
//
// Read shape (LoadToken):
//
//   1. Attempt keys.Service.GetTier1("vi-<provider>"). On OK return.
//   2. Fall back to the legacy config literal — kept so an account-
//      locked first launch still serves PR-fetches for the operator
//      who has not yet unlocked. Once the account unlocks and
//      MigrateLegacyTokens drains, the literal disappears and step
//      1 becomes the only resolution path.
//
// Tokens are NEVER logged in full — MaskToken is the only safe form
// for any operator-facing surface (logs / panel hints / etc.).

package vi

import (
	core "dappco.re/go"
	"dappco.re/go/config"

	"dappco.re/lthn/desktop/pkg/audit"
	"dappco.re/lthn/desktop/pkg/keys"
)

// TokenConfigKey returns the legacy config dot-path a provider's
// token persists under during the migration window. Stable across
// launches; reads ALWAYS prefer the tier-1 ref shape (TokenKeysRef)
// so once MigrateLegacyTokens drains this key is empty.
//
// Usage example:
//
//	key := vi.TokenConfigKey("forge")  // "vi.tokens.forge"
func TokenConfigKey(provider string) string {
	return "vi.tokens." + provider
}

// TokenKeysRef returns the pkg/keys tier-1 ref under which a
// provider's PAT is sealed. Stable across launches — once a token
// is migrated this is the only resolution path.
//
// Usage example:
//
//	ref := vi.TokenKeysRef("forge")  // "vi-forge"
func TokenKeysRef(provider string) string {
	return "vi-" + provider
}

// LoadToken returns the configured personal-access token for the
// named provider, or "" when no token is set. Tier-1 (keys.GetTier1)
// is the canonical path; the legacy config literal is the migration-
// window fallback that disappears once MigrateLegacyTokens drains.
//
// Callers MUST handle empty cleanly — anonymous fetches are valid
// (GitHub rate-limits hard at 60/hr/IP; Forgejo public repos work
// without auth).
//
// Usage example:
//
//	tok := vi.LoadToken(c, "forge")
//	if tok == "" { /* skip authed fetch */ }
func LoadToken(c *core.Core, provider string) string {
	if c == nil || provider == "" {
		return ""
	}
	// 1. Canonical path — tier-1 sealed.
	if keysSvc, ok := core.ServiceFor[*keys.Service](c, "keys"); ok && keysSvc != nil {
		r := keysSvc.GetTier1(TokenKeysRef(provider))
		if r.OK {
			if plaintext, ok := r.Value.([]byte); ok && len(plaintext) > 0 {
				return string(plaintext)
			}
		}
	}
	// 2. Legacy fallback — config literal (migration-window only).
	cfg, ok := core.ServiceFor[*config.Service](c, "config")
	if !ok || cfg == nil {
		return ""
	}
	var tok string
	r := cfg.Get(TokenConfigKey(provider), &tok)
	if !r.OK {
		return ""
	}
	return tok
}

// MigrateLegacyTokens scans the config service for `vi.tokens.<p>`
// literal entries, dispatches each plaintext into the tier-1
// substrate under `vi-<p>`, and rewrites the config with the literal
// blanked. Returns the count of successfully migrated providers.
// Idempotent: providers whose config entry is empty are skipped.
//
// Deferred-migration: when tier-1 KEK is not yet live (no account
// unlocked), PutTier1 fails closed; the literal stays put and the
// next call retries. Mirrors runner.MigrateLegacyKeys semantics.
//
// Usage example (in cmd/lthn/app.go after SetKEKProvider fires):
//
//	migrated := vi.MigrateLegacyTokens(c)
//	core.Print(core.Stdout(), "vi: migrated %d legacy tokens\n", migrated)
func MigrateLegacyTokens(c *core.Core) int {
	if c == nil {
		return 0
	}
	cfg, ok := core.ServiceFor[*config.Service](c, "config")
	if !ok || cfg == nil {
		return 0
	}
	keysSvc, ok := core.ServiceFor[*keys.Service](c, "keys")
	if !ok || keysSvc == nil {
		return 0
	}
	// Inspect the vi.tokens subtree as a map. config.Get on the
	// parent dot-path returns the sub-map; an empty/absent subtree
	// short-circuits cleanly.
	var tokens map[string]any
	if r := cfg.Get("vi.tokens", &tokens); !r.OK || len(tokens) == 0 {
		return 0
	}
	migrated := 0
	for provider, raw := range tokens {
		literal, _ := raw.(string)
		if literal == "" {
			continue
		}
		ref := TokenKeysRef(provider)
		if r := keysSvc.PutTier1(ref, []byte(literal)); !r.OK {
			// Tier-1 KEK provider not live; bail without further
			// mutation so the next retry sees the full set queued.
			break
		}
		// Blank the legacy literal post-migration. Per-provider write
		// so a mid-batch failure leaves prior successes durable.
		if r := cfg.Set(TokenConfigKey(provider), ""); !r.OK {
			// PutTier1 already succeeded; emit the audit row anyway —
			// the ciphertext is sealed even if the legacy literal
			// rewrite stalled (operator can clear manually).
		}
		migrated++
		_ = audit.Default().Record(audit.Event{
			Event:   audit.EventViTokenMigrated,
			TS:      core.Now().UTC().Unix(),
			Scope:   viScope,
			Outcome: audit.OutcomeOK,
			Meta: map[string]any{
				"provider": provider,
				"ref":      ref,
			},
		})
	}
	return migrated
}

// MaskToken returns a UI-safe form of a provider token — first 4
// chars + bullets + last 4 chars. Used by panels + logs that want
// to show "a token exists" without leaking it. Returns "" for ""
// so absent-token state renders honestly.
//
// Usage example:
//
//	vi.MaskToken("ghp_0123456789abcdef0123456789abcdef0123")
//	// → "ghp_••••••••••••••••••••••••••••••••••0123"
func MaskToken(token string) string {
	if token == "" {
		return ""
	}
	const headRunes = 4
	const tailRunes = 4
	if len(token) <= headRunes+tailRunes {
		return token
	}
	mid := len(token) - headRunes - tailRunes
	bullets := ""
	for i := 0; i < mid; i++ {
		bullets += "•"
	}
	return token[:headRunes] + bullets + token[len(token)-tailRunes:]
}
