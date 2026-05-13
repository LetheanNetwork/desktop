// SPDX-Licence-Identifier: EUPL-1.2

// Package i18n is the lthn-side locale bundle. It embeds the JSON
// translation files at locales/<lang>.json and exposes a single
// Source() function that hands the embedded FS over as an
// coreI18n.FSSource — drop the result into
// i18n.ServiceOptions.ExtraFS at Core construction time and the
// rest of the app reads translations through c.I18n().Translate(key, args...).
//
// First-rate locales today:
//
//	en        UK English — Snider canon (colour, organisation, centre).
//	          The OG English; en_GB resolves here via the loader's
//	          base-language fallback.
//	en_au     Australian English — meeting-audience polish. Tracks
//	          en for vocabulary (Australia inherits UK spelling),
//	          diverges on greetings (g'day) where it adds warmth.
//	fr        French — full WebView vocabulary translated.
//	zh        Chinese (Simplified) — full WebView vocabulary translated;
//	          zh-CN / zh-TW resolve here via the loader's base-language
//	          fallback. Plural rule is OTHER for all integers.
//
// Add a new locale by dropping en_xx.json (or any BCP-47 tag) in
// locales/ — the embed glob picks it up at compile time, no further
// wiring needed. The FSLoader handles `-` ↔ `_` variants + base-tag
// fallback automatically.
//
// Usage example:
//
//	import lthni18n "dappco.re/lthn/desktop/pkg/i18n"
//	core.New(
//	    core.WithName("i18n", coreI18n.NewCoreService(coreI18n.ServiceOptions{
//	        Language: "en-GB",
//	        Fallback: "en",
//	        ExtraFS:  []coreI18n.FSSource{lthni18n.Source()},
//	    })),
//	)
package i18n

import (
	"embed"

	coreI18n "dappco.re/go/i18n"
)

// localesFS embeds every JSON file under locales/. The glob is
// limited to .json so any future README.md or .gitkeep doesn't
// trip the FSLoader's directory scan.
//
//go:embed locales/*.json
var localesFS embed.FS

// Source returns the lthn locale bundle as a coreI18n.FSSource —
// pass it into ServiceOptions.ExtraFS at Core construction.
//
// The Dir field stays "locales" because FSLoader reads
// "<Dir>/<lang>.json" against the supplied fs.FS, and our embed
// preserves the locales/ prefix on each entry.
//
// Usage example:
//
//	src := lthni18n.Source()
//	opts := coreI18n.ServiceOptions{ExtraFS: []coreI18n.FSSource{src}}
func Source() coreI18n.FSSource {
	return coreI18n.FSSource{
		FS:  localesFS,
		Dir: "locales",
	}
}
