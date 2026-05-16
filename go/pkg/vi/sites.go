// SPDX-Licence-Identifier: EUPL-1.2

// Catalogue — the configured probe targets. Loaded from the Core
// config service under the "vi.sites" key, with a sensible default
// list when the user hasn't overridden anything yet so the very
// first Vi panel render shows real Lethean-family probes instead
// of an empty grid.
//
// Config shape (~/.config/lthn/lthn.json or equivalent):
//
//	{
//	  "vi": {
//	    "sites": [
//	      {"url": "https://lthn.ai",      "name": "Lethean",      "stack": "Lethean · CIC",        "intervalSeconds": 60},
//	      {"url": "https://lthn.sh",      "name": "Lethean Self-host", "stack": "Lethean · Self-host"},
//	      {"url": "https://host.uk.com",  "name": "Host UK",      "stack": "Lethean · SaaS"},
//	      {"url": "https://forge.lthn.sh","name": "Forge (homelab)", "stack": "Lethean · Forgejo"}
//	    ]
//	  }
//	}
//
// `intervalSeconds` is the per-site override. Omit to inherit
// DefaultInterval (60s). When the user provides any "vi.sites"
// block at all, it REPLACES the default catalogue — no merging.
// "I asked for these three" is the contract, not "these three plus
// the defaults".

package vi

import (
	core "dappco.re/go"
	"dappco.re/go/config"
)

// DefaultInterval is the per-site probe cadence when neither the
// site's `intervalSeconds` field nor a global override is set.
// 60s lines up with the user's expectation in the ticket and is
// gentle enough that probing the four default sites won't show up
// in anyone's traffic logs as suspicious.
const DefaultInterval = 60 * core.Second

// MinInterval is the floor — any configured interval below this
// is silently clamped to this floor to keep the probe loop from
// hammering an upstream. 5 seconds is well under any reasonable
// alerting cadence but high enough to not look like abuse.
const MinInterval = 5 * core.Second

// defaultCatalogue is the out-of-the-box probe set. Lines up with
// the four canonical Lethean-family surfaces the user named in the
// ticket — `lthn.ai`, `lthn.sh`, `host.uk.com`, `forge.lthn.sh`.
//
// First-run Vi panel renders these four with real probe results
// (assuming network reachable). The user overrides via the
// `vi.sites` config block.
var defaultCatalogue = []SiteCatalogue{
	{URL: "https://lthn.ai", Name: "Lethean", Stack: "Lethean · CIC", Interval: DefaultInterval},
	{URL: "https://lthn.sh", Name: "Lethean Self-host", Stack: "Lethean · Self-host", Interval: DefaultInterval},
	{URL: "https://host.uk.com", Name: "Host UK", Stack: "Lethean · SaaS", Interval: DefaultInterval},
	{URL: "https://forge.lthn.sh", Name: "Forge (homelab)", Stack: "Lethean · Forgejo", Interval: DefaultInterval},
}

// configSite mirrors the JSON shape under "vi.sites[]". Kept
// separate from SiteCatalogue because the config blob carries
// integer seconds (JSON-friendly) while SiteCatalogue.Interval is
// a core.Duration (Go-friendly). Conversion happens in
// LoadCatalogue.
type configSite struct {
	URL             string `json:"url"`
	Name            string `json:"name"`
	Stack           string `json:"stack"`
	IntervalSeconds int    `json:"intervalSeconds"`
}

// LoadCatalogue returns the configured probe targets, falling back
// to the default catalogue when the config service has no
// `vi.sites` key.
//
// Sites whose URL doesn't start with "https://" are dropped — the
// probe rejects plain http to align with pkg/marketplace's
// httpsOnlyClient discipline (Cerberus #1433). v1 doesn't surface
// these as warnings; a future "config validation" pass can.
//
// Usage example:
//
//	catalogue := vi.LoadCatalogue(c)
//	for _, site := range catalogue { _ = site.URL }
func LoadCatalogue(c *core.Core) []SiteCatalogue {
	if c == nil {
		return clone(defaultCatalogue)
	}
	cfg, ok := core.ServiceFor[*config.Service](c, "config")
	if !ok || cfg == nil {
		return clone(defaultCatalogue)
	}
	var raw []configSite
	r := cfg.Get("vi.sites", &raw)
	if !r.OK || len(raw) == 0 {
		return clone(defaultCatalogue)
	}
	out := make([]SiteCatalogue, 0, len(raw))
	for _, cs := range raw {
		if !core.HasPrefix(cs.URL, "https://") {
			continue
		}
		interval := DefaultInterval
		if cs.IntervalSeconds > 0 {
			interval = core.Duration(cs.IntervalSeconds) * core.Second
		}
		if interval < MinInterval {
			interval = MinInterval
		}
		out = append(out, SiteCatalogue{
			URL:      cs.URL,
			Name:     cs.Name,
			Stack:    cs.Stack,
			Interval: interval,
		})
	}
	if len(out) == 0 {
		// Every entry was rejected (all non-https). Fall back to
		// default rather than ship an empty panel — the user has
		// expressed intent ("watch sites") even if the config got
		// it wrong.
		return clone(defaultCatalogue)
	}
	return out
}

// DefaultCatalogue returns a fresh copy of the out-of-the-box probe
// targets. Exported so tests + the Wails surface can render "what
// would Vi watch if you don't configure anything" without invoking
// LoadCatalogue (which reads the live config service).
//
// Usage example:
//
//	defaults := vi.DefaultCatalogue()
//	core.AssertEqual(t, 4, len(defaults))
func DefaultCatalogue() []SiteCatalogue {
	return clone(defaultCatalogue)
}

// DomainOf strips the scheme + path from a probe URL to produce the
// canonical Domain key used in SiteProbe.Domain + SiteStatus.Domain.
// Falls back to the raw URL when parsing fails so logs always carry
// something identifying.
//
// Usage example:
//
//	d := vi.DomainOf("https://lthn.ai/")   // "lthn.ai"
//	d := vi.DomainOf("https://x.io:8443/y") // "x.io:8443"
func DomainOf(rawURL string) string {
	u := core.URLParse(rawURL)
	if !u.OK {
		return rawURL
	}
	parsed, ok := u.Value.(*core.URL)
	if !ok || parsed == nil || parsed.Host == "" {
		return rawURL
	}
	return parsed.Host
}

// clone returns an independent copy of the catalogue so callers
// can't mutate the package-level default by appending to the
// returned slice.
func clone(src []SiteCatalogue) []SiteCatalogue {
	out := make([]SiteCatalogue, len(src))
	copy(out, src)
	return out
}
