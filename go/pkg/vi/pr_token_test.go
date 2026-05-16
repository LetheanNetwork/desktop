// SPDX-Licence-Identifier: EUPL-1.2

package vi_test

import (
	core "dappco.re/go"
	"dappco.re/go/config"
	"dappco.re/lthn/desktop/pkg/vi"
)

// TestPRToken_LoadToken_Good — a token written under the canonical
// config key round-trips out via LoadToken.
func TestPRToken_LoadToken_Good(t *core.T) {
	c := newServiceCore(t)
	cfg, ok := core.ServiceFor[*config.Service](c, "config")
	core.RequireTrue(t, ok)
	core.RequireTrue(t, cfg.Set("vi.tokens.forge", "tok_abcdef0123456789").OK)

	got := vi.LoadToken(c, "forge")
	core.AssertEqual(t, "tok_abcdef0123456789", got)
}

// TestPRToken_LoadToken_Bad — nil core, empty provider, missing
// config service, missing key all return "" cleanly.
func TestPRToken_LoadToken_Bad(t *core.T) {
	core.AssertEqual(t, "", vi.LoadToken(nil, "forge"))
	core.AssertEqual(t, "", vi.LoadToken(nil, ""))

	c := newServiceCore(t)
	core.AssertEqual(t, "", vi.LoadToken(c, ""))             // empty provider
	core.AssertEqual(t, "", vi.LoadToken(c, "forge"))        // unset key
	core.AssertEqual(t, "", vi.LoadToken(c, "neverexisted")) // unknown provider
}

// TestPRToken_TokenConfigKey_Good — config key shape is stable.
func TestPRToken_TokenConfigKey_Good(t *core.T) {
	core.AssertEqual(t, "vi.tokens.forge", vi.TokenConfigKey("forge"))
	core.AssertEqual(t, "vi.tokens.github", vi.TokenConfigKey("github"))
}

// TestPRToken_MaskToken_Good — masking leaks first 4 + last 4 only.
func TestPRToken_MaskToken_Good(t *core.T) {
	masked := vi.MaskToken("ghp_0123456789abcdef0123456789abcdef0123")
	core.AssertTrue(t, len(masked) > 0)
	core.AssertTrue(t, core.HasPrefix(masked, "ghp_"))
	core.AssertTrue(t, core.HasSuffix(masked, "0123"))
	core.AssertFalse(t, core.Contains(masked, "abcdef"))
}

// TestPRToken_MaskToken_Bad — empty + ultra-short tokens don't
// panic. An empty input returns empty; a too-short token is left
// alone (we can't safely mask it without revealing most of it).
func TestPRToken_MaskToken_Bad(t *core.T) {
	core.AssertEqual(t, "", vi.MaskToken(""))
	core.AssertEqual(t, "abc", vi.MaskToken("abc"))         // shorter than head+tail
	core.AssertEqual(t, "12345678", vi.MaskToken("12345678")) // equal to head+tail
}

// TestPRToken_MaskToken_Ugly — exact length math: a 12-char token
// (head 4 + tail 4 = 8, mid = 4) renders as `1234••••9012`.
func TestPRToken_MaskToken_Ugly(t *core.T) {
	got := vi.MaskToken("123456789012")
	core.AssertEqual(t, "1234••••9012", got)
}
