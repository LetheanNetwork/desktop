// SPDX-Licence-Identifier: EUPL-1.2

// Tests for the embedded locale bundle. Drives the same load path
// the Core service uses at boot — FSLoader against the embed.FS —
// so any "I forgot to include the file in go:embed" regression
// surfaces in CI before it bites the running binary.

package i18n_test

import (
	core "dappco.re/go"
	coreI18n "dappco.re/go/i18n"

	lthni18n "dappco.re/lthn/desktop/pkg/i18n"
)

// newServiceWithSource builds an i18n service rigged to load
// translations from the lthn locale bundle — matches what
// newAppCore does at boot, minus the rest of the Core.
func newServiceWithSource(t *core.T, lang string) {
	src := lthni18n.Source()
	loader := coreI18n.NewFSLoader(src.FS, src.Dir)

	svcResult := coreI18n.New()
	core.AssertTrue(t, svcResult.OK)
	svc := svcResult.Value.(*coreI18n.Service)

	addResult := svc.AddLoader(loader)
	core.AssertTrue(t, addResult.OK)

	setResult := svc.SetLanguage(lang)
	core.AssertTrue(t, setResult.OK)
	svc.SetFallback("en")
	coreI18n.SetDefault(svc)
}

func TestLocales_Source_EnglishLoads(t *core.T) {
	newServiceWithSource(t, "en")
	got := coreI18n.T("cli.welcome.title")
	core.AssertEqual(t, "lthn — Lethean unified binary", got)
}

func TestLocales_Source_EnGBResolvesViaBaseLanguageFallback(t *core.T) {
	// en-GB has no dedicated file — the loader's base-tag fallback
	// resolves it to en.json. This is the Snider canon: UK English
	// IS the OG English, no separate file needed.
	newServiceWithSource(t, "en-GB")
	got := coreI18n.T("cli.welcome.title")
	core.AssertEqual(t, "lthn — Lethean unified binary", got)
}

func TestLocales_Source_EnAULoadsAsDistinctLocale(t *core.T) {
	// en_au.json today carries the same strings as en.json
	// (Australian English inherits UK spelling — no in-scope copy
	// hits the program/programme axis or similar). The file exists
	// so the locale tag is recognised; visible diffs land when we
	// add user-facing copy with words that vary.
	newServiceWithSource(t, "en-AU")
	got := coreI18n.T("cli.welcome.title")
	core.AssertEqual(t, "lthn — Lethean unified binary", got)
}

func TestLocales_Source_MissingKeyReturnsMessageID(t *core.T) {
	// i18n.T's documented fallback when a key isn't in any loaded
	// locale: return the messageID itself. Safe behaviour — never
	// blank, never panics.
	newServiceWithSource(t, "en")
	got := coreI18n.T("definitely.not.a.real.key")
	core.AssertEqual(t, "definitely.not.a.real.key", got)
}

func TestLocales_Source_SubcommandLabelsResolveAcrossLocales(t *core.T) {
	// Sanity sweep — every subcommand label key the CLI uses should
	// resolve to a non-empty string in both locales.
	keys := []string{
		"cli.subcommands.gui",
		"cli.subcommands.tray",
		"cli.subcommands.serve",
		"cli.subcommands.ai",
		"cli.subcommands.config",
		"cli.subcommands.state",
		"cli.subcommands.version",
		"cli.subcommands.help",
	}
	for _, lang := range []string{"en", "en-AU"} {
		newServiceWithSource(t, lang)
		for _, key := range keys {
			got := coreI18n.T(key)
			if got == "" || got == key {
				t.Fatalf("locale %s: key %q did not resolve (got %q)", lang, key, got)
			}
		}
	}
}
