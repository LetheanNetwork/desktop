// SPDX-Licence-Identifier: EUPL-1.2

// Token loader — provider auth tokens for the PR fetcher. Tokens
// live under `vi.tokens.<provider>` in the Core config service:
//
//	{
//	  "vi": {
//	    "tokens": {
//	      "forge":  "<forgejo personal access token>",
//	      "github": "<github personal access token>"
//	    }
//	  }
//	}
//
// Same shape runner uses for `routes.<name>.api_key` — config-backed,
// loaded fresh per fetch so a token rotation takes effect on the
// next tick without restart.
//
// Tokens are NEVER logged in full — Mask() is the only safe form
// for any operator-facing surface (logs / panel hints / etc.).
// Mirrors pkg/apikey.Mask shape so the masking convention is uniform
// across the binary.

package vi

import (
	core "dappco.re/go"
	"dappco.re/go/config"
)

// TokenConfigKey returns the config dot-path a provider's token
// persists under. Stable across launches.
//
// Usage example:
//
//	key := vi.TokenConfigKey("forge")  // "vi.tokens.forge"
func TokenConfigKey(provider string) string {
	return "vi.tokens." + provider
}

// LoadToken returns the configured personal-access token for the
// named provider, or "" when no token is set. Callers MUST handle
// empty cleanly — anonymous fetches are valid (GitHub rate-limits
// hard at 60/hr/IP; Forgejo public repos work without auth).
//
// Usage example:
//
//	tok := vi.LoadToken(c, "forge")
//	if tok == "" { /* skip authed fetch */ }
func LoadToken(c *core.Core, provider string) string {
	if c == nil || provider == "" {
		return ""
	}
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
