// SPDX-Licence-Identifier: EUPL-1.2

// Real tests for apikey.go's exported surface (GenerateOrLoad, Rotate,
// Mask). The pre-existing apikey_example_test.go only takes method
// VALUES via reflection (core.Sprintf("%T", ref)) and never calls
// them — that's why the package sat at 0.0% coverage despite having
// "passing" tests. These tests actually invoke the functions against
// a real config.Service backed by a t.TempDir() config file.

package apikey_test

import (
	core "dappco.re/go"
	"dappco.re/go/config"
	subject "dappco.re/lthn/desktop/pkg/apikey"
)

// apikeyFixture builds a Core with a real config.Service rooted at a
// fresh temp file, started so cfg.config is non-nil (config.Service
// methods return "config not loaded" until OnStartup runs).
func apikeyFixture(t *core.T) *core.Core {
	t.Helper()
	c := core.New(core.WithName(
		"config",
		config.NewConfigServiceWith(config.ServiceOptions{
			Path: core.PathJoin(t.TempDir(), "lthn.yaml"),
		}),
	))
	core.RequireTrue(t, c.ServiceStartup(core.Background(), nil).OK)
	t.Cleanup(func() { _ = c.ServiceShutdown(core.Background()) })
	return c
}

func TestApikey_GenerateOrLoad_Good_FreshKeyGeneratedAndPersisted(t *core.T) {
	c := apikeyFixture(t)

	r := subject.GenerateOrLoad(c)
	core.RequireTrue(t, r.OK, r.Error())
	key, ok := r.Value.(string)
	core.RequireTrue(t, ok)
	core.AssertTrue(t, core.HasPrefix(key, "sk-lthn-"))
	// 16 entropy bytes hex-encoded = 32 chars, plus the "sk-lthn-" prefix.
	core.AssertEqual(t, len("sk-lthn-")+32, len(key))

	// Second call must return the SAME key — idempotent across launches.
	r2 := subject.GenerateOrLoad(c)
	core.RequireTrue(t, r2.OK, r2.Error())
	core.AssertEqual(t, key, r2.Value.(string))
}

func TestApikey_GenerateOrLoad_Bad_NoConfigServiceRegistered(t *core.T) {
	c := core.New() // no "config" service wired
	r := subject.GenerateOrLoad(c)
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "config service not registered")
}

func TestApikey_GenerateOrLoad_Ugly_ExistingEmptyValueRegenerates(t *core.T) {
	c := apikeyFixture(t)
	cfg, ok := core.ServiceFor[*config.Service](c, "config")
	core.RequireTrue(t, ok)
	// An explicitly empty stored value must NOT short-circuit as "found" —
	// GenerateOrLoad checks existing != "" before trusting it.
	core.RequireTrue(t, cfg.Set(subject.ConfigKey, "").OK)

	r := subject.GenerateOrLoad(c)
	core.RequireTrue(t, r.OK, r.Error())
	key := r.Value.(string)
	core.AssertTrue(t, core.HasPrefix(key, "sk-lthn-"))
	core.AssertNotEqual(t, "", key)
}

func TestApikey_Rotate_Good_ReplacesPersistedKey(t *core.T) {
	c := apikeyFixture(t)

	first := subject.GenerateOrLoad(c)
	core.RequireTrue(t, first.OK, first.Error())

	r := subject.Rotate(c)
	core.RequireTrue(t, r.OK, r.Error())
	rotated := r.Value.(string)
	core.AssertNotEqual(t, first.Value.(string), rotated)
	core.AssertTrue(t, core.HasPrefix(rotated, "sk-lthn-"))

	// GenerateOrLoad after Rotate must now return the rotated value —
	// proof the new key was actually committed to config, not just
	// returned in-memory.
	after := subject.GenerateOrLoad(c)
	core.RequireTrue(t, after.OK, after.Error())
	core.AssertEqual(t, rotated, after.Value.(string))
}

func TestApikey_Rotate_Bad_NoConfigServiceRegistered(t *core.T) {
	c := core.New()
	r := subject.Rotate(c)
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "config service not registered")
}

func TestApikey_Mask_Good_TypicalKey(t *core.T) {
	got := subject.Mask("sk-lthn-0011223344556677889aabbccddeeff")
	core.AssertTrue(t, core.HasPrefix(got, "sk-lthn-0011"))
	core.AssertTrue(t, core.Contains(got, "•"))
	core.AssertTrue(t, core.HasSuffix(got, "eeff"))
	// The raw key itself must never appear inside the masked form.
	core.AssertFalse(t, core.Contains(got, "223344556677889aabbccdd"))
}

func TestApikey_Mask_Bad_EmptyReturnsEmpty(t *core.T) {
	core.AssertEqual(t, "", subject.Mask(""))
}

func TestApikey_Mask_Ugly_ShortKeyReturnedUnchanged(t *core.T) {
	// len < 12 short-circuits to the raw value.
	core.AssertEqual(t, "sk-lthn-", subject.Mask("sk-lthn-"))
	core.AssertEqual(t, "short", subject.Mask("short"))
}

// TestApikey_GenerateOrLoad_Ugly_CommitFailsOnReadOnlyDir drives the
// generateAndStore error path that neither the Good nor Bad case above
// reaches: cfg.Set succeeds (in-memory only) but cfg.Commit fails
// because the config directory itself is not writable. Real fault
// injection via chmod, restored in cleanup so t.TempDir()'s own
// removal doesn't trip over the permission change.
func TestApikey_GenerateOrLoad_Ugly_CommitFailsOnReadOnlyDir(t *core.T) {
	dir := t.TempDir()
	core.RequireTrue(t, core.Chmod(dir, 0o500).OK)
	t.Cleanup(func() { _ = core.Chmod(dir, 0o755) })

	c := core.New(core.WithName(
		"config",
		config.NewConfigServiceWith(config.ServiceOptions{
			Path: core.PathJoin(dir, "lthn.yaml"),
		}),
	))
	core.RequireTrue(t, c.ServiceStartup(core.Background(), nil).OK)
	t.Cleanup(func() { _ = c.ServiceShutdown(core.Background()) })

	r := subject.GenerateOrLoad(c)
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "failed to save config")
}

func TestApikey_Mask_Ugly_NarrowMidReturnsUnchanged(t *core.T) {
	// keyPrefix(8) + head(4) + tail(4) = 16; anything shorter than
	// mid==4 extra chars (i.e. len < 20) but >= 12 hits the "mid < 4"
	// guard and returns the key unmasked rather than producing a
	// negative-length bullet run.
	key := "sk-lthn-abcdefgh" // len 16: mid = 16-8-4-4 = 0 < 4
	core.AssertEqual(t, key, subject.Mask(key))
}
