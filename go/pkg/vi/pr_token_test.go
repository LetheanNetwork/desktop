// SPDX-Licence-Identifier: EUPL-1.2

package vi_test

import (
	core "dappco.re/go"
	"dappco.re/go/config"

	"dappco.re/lthn/desktop/pkg/keys"
	"dappco.re/lthn/desktop/pkg/vi"
)

// tokenFixture wires config + keys services with a deterministic
// tier-1 KEK so MigrateLegacyTokens + LoadToken exercise the same
// substrate the production wire walks (cmd/lthn/app.go).
//
// Returns the Core and the live keys service; HOME is redirected
// to t.TempDir() so the tier-1 on-disk partition is per-test
// isolated.
func tokenFixture(t *core.T) (*core.Core, *keys.Service) {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	cfgFile := core.PathJoin(tmp, "lthn.yaml")
	core.AssertTrue(t, core.WriteFile(cfgFile, []byte("{}\n"), 0o644).OK)

	c := core.New(
		core.WithName("config", config.NewConfigServiceWith(config.ServiceOptions{
			Path:      cfgFile,
			EnvPrefix: "LTHN_TEST_VI_TOKEN",
		})),
		core.WithName("keys", keys.Register),
	)
	core.AssertTrue(t, c.ServiceStartup(core.Background(), nil).OK)
	t.Cleanup(func() { _ = c.ServiceShutdown(core.Background()) })

	keysSvc, _ := core.ServiceFor[*keys.Service](c, "keys")
	core.RequireTrue(t, keysSvc != nil)
	keysSvc.SetKEKProviderTier0(func() ([]byte, bool) {
		k := make([]byte, 32)
		for i := range k {
			k[i] = 0x42
		}
		return k, true
	})
	keysSvc.SetKEKProvider(func() ([]byte, bool) {
		k := make([]byte, 32)
		for i := range k {
			k[i] = 0x77
		}
		return k, true
	})
	keysSvc.SetCore(c)
	return c, keysSvc
}

// TestVi_LoadToken_FromKeys_Good — when a token is sealed in tier-1
// under the canonical `vi-<provider>` ref, LoadToken resolves via
// keys.GetTier1 (NOT via the legacy config literal).
func TestVi_LoadToken_FromKeys_Good(t *core.T) {
	c, keysSvc := tokenFixture(t)
	core.RequireTrue(t, keysSvc.PutTier1(vi.TokenKeysRef("forge"), []byte("tok_sealed_in_tier1")).OK)

	got := vi.LoadToken(c, "forge")
	core.AssertEqual(t, "tok_sealed_in_tier1", got)
}

// TestVi_LoadToken_FromKeys_LegacyFallback_Good — when no tier-1
// ref exists, LoadToken falls back to the legacy config literal so
// an account-locked first launch still serves PR-fetches.
func TestVi_LoadToken_FromKeys_LegacyFallback_Good(t *core.T) {
	c, _ := tokenFixture(t)
	cfg, ok := core.ServiceFor[*config.Service](c, "config")
	core.RequireTrue(t, ok)
	core.RequireTrue(t, cfg.Set(vi.TokenConfigKey("forge"), "tok_legacy_config").OK)

	got := vi.LoadToken(c, "forge")
	core.AssertEqual(t, "tok_legacy_config", got)
}

// TestVi_TokenMigration_FromConfig_Good — MigrateLegacyTokens lifts
// a config literal into tier-1, blanks the config key, and returns
// the migrated count. Post-migration LoadToken resolves via tier-1.
func TestVi_TokenMigration_FromConfig_Good(t *core.T) {
	c, keysSvc := tokenFixture(t)
	cfg, ok := core.ServiceFor[*config.Service](c, "config")
	core.RequireTrue(t, ok)
	core.RequireTrue(t, cfg.Set(vi.TokenConfigKey("forge"), "tok_pre_migration").OK)
	core.RequireTrue(t, cfg.Set(vi.TokenConfigKey("github"), "ghp_pre_migration").OK)

	migrated := vi.MigrateLegacyTokens(c)
	core.AssertEqual(t, 2, migrated)

	// Tier-1 substrate now carries the ciphertext.
	for _, p := range []string{"forge", "github"} {
		r := keysSvc.GetTier1(vi.TokenKeysRef(p))
		core.RequireTrue(t, r.OK)
	}

	// Config literal blanked.
	var got string
	_ = cfg.Get(vi.TokenConfigKey("forge"), &got)
	core.AssertEqual(t, "", got)

	// LoadToken now resolves via tier-1.
	core.AssertEqual(t, "tok_pre_migration", vi.LoadToken(c, "forge"))
	core.AssertEqual(t, "ghp_pre_migration", vi.LoadToken(c, "github"))

	// Idempotent — second call is a no-op.
	core.AssertEqual(t, 0, vi.MigrateLegacyTokens(c))
}

// TestVi_LoadToken_Bad — nil core, empty provider, missing config
// service, missing key all return "" cleanly.
func TestVi_LoadToken_Bad(t *core.T) {
	core.AssertEqual(t, "", vi.LoadToken(nil, "forge"))
	core.AssertEqual(t, "", vi.LoadToken(nil, ""))

	c := newServiceCore(t)
	core.AssertEqual(t, "", vi.LoadToken(c, ""))             // empty provider
	core.AssertEqual(t, "", vi.LoadToken(c, "forge"))        // unset key
	core.AssertEqual(t, "", vi.LoadToken(c, "neverexisted")) // unknown provider
}

// TestVi_TokenConfigKey_Good — config key shape is stable.
func TestVi_TokenConfigKey_Good(t *core.T) {
	core.AssertEqual(t, "vi.tokens.forge", vi.TokenConfigKey("forge"))
	core.AssertEqual(t, "vi.tokens.github", vi.TokenConfigKey("github"))
	core.AssertEqual(t, "vi-forge", vi.TokenKeysRef("forge"))
	core.AssertEqual(t, "vi-github", vi.TokenKeysRef("github"))
}

// TestVi_MaskToken_Good — masking leaks first 4 + last 4 only.
func TestVi_MaskToken_Good(t *core.T) {
	masked := vi.MaskToken("ghp_0123456789abcdef0123456789abcdef0123")
	core.AssertTrue(t, len(masked) > 0)
	core.AssertTrue(t, core.HasPrefix(masked, "ghp_"))
	core.AssertTrue(t, core.HasSuffix(masked, "0123"))
	core.AssertFalse(t, core.Contains(masked, "abcdef"))
}

// TestVi_MaskToken_Bad — empty + ultra-short tokens don't panic.
func TestVi_MaskToken_Bad(t *core.T) {
	core.AssertEqual(t, "", vi.MaskToken(""))
	core.AssertEqual(t, "abc", vi.MaskToken("abc"))
	core.AssertEqual(t, "12345678", vi.MaskToken("12345678"))
}

// TestVi_MaskToken_Ugly — exact length math.
func TestVi_MaskToken_Ugly(t *core.T) {
	got := vi.MaskToken("123456789012")
	core.AssertEqual(t, "1234••••9012", got)
}
