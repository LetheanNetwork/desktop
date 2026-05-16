// SPDX-Licence-Identifier: EUPL-1.2

// Repo catalogue — the configured PR-watcher targets. Loaded from
// the Core config service under the "vi.repos" key, with a sensible
// default so first launch already surfaces something useful (open
// PRs against the canonical lthn/desktop repo on forge.lthn.sh).
//
// Config shape (~/.config/lthn/lthn.json or equivalent):
//
//	{
//	  "vi": {
//	    "repos": [
//	      {"provider": "forge",  "owner": "lthn", "name": "desktop", "intervalSeconds": 300},
//	      {"provider": "forge",  "baseUrl": "https://forge.lthn.ai", "owner": "lthn", "name": "ai"},
//	      {"provider": "github", "owner": "LetheanNetwork", "name": "desktop"}
//	    ]
//	  }
//	}
//
// `intervalSeconds` is the per-repo override. Omit to inherit
// DefaultPRInterval (300s). When the user provides any "vi.repos"
// block at all, it REPLACES the default catalogue — no merging.
// Same intent contract as vi.sites: "watch exactly these".
//
// Tokens never live in this block — they're stored separately under
// `vi.tokens.<provider>` so a repo list can be shared / version-
// controlled without leaking credentials. See pr_token.go.

package vi

import (
	core "dappco.re/go"
	"dappco.re/go/config"
)

// DefaultPRInterval is the per-repo poll cadence when neither the
// repo's `intervalSeconds` field nor a global override is set.
// 300s (5min) is comfortably under GitHub's 60 req/hr anonymous
// limit even when polling a dozen repos, and well under the 5000
// req/hr authenticated limit. PR state changes are infrequent
// enough that 5min latency is invisible to the operator.
const DefaultPRInterval = 300 * core.Second

// MinPRInterval is the floor — any configured interval below this
// is silently clamped to this floor to defend GitHub's rate limits
// and our own forge from accidental hammering. 30s is well over
// any legitimate polling need.
const MinPRInterval = 30 * core.Second

// ProviderForge is the Forgejo backend name. Default BaseURL is
// https://forge.lthn.sh; other Forgejo instances reuse this
// provider with an explicit BaseURL override.
const ProviderForge = "forge"

// ProviderGitHub is the GitHub.com backend name. BaseURL is fixed
// at https://api.github.com (overriding it is rejected at load —
// "github" means github.com, not arbitrary GHE).
const ProviderGitHub = "github"

// defaultForgeBaseURL is the canonical Forgejo instance we default
// to when a "forge" entry doesn't override BaseURL.
const defaultForgeBaseURL = "https://forge.lthn.sh"

// defaultGitHubBaseURL is the GitHub REST API root. Pinned.
const defaultGitHubBaseURL = "https://api.github.com"

// defaultPRRepos is the out-of-the-box watched-repo set. One
// canonical entry — open PRs against lthn/desktop on the homelab
// forge. The user overrides via the `vi.repos` config block.
//
// We deliberately don't seed any GitHub repos by default — GitHub
// fetches without a token are rate-limited to 60/hr per IP and we'd
// rather not eat that budget on the user's first launch when they
// may not have set a token yet.
var defaultPRRepos = []PRRepo{
	{Provider: ProviderForge, Owner: "lthn", Name: "desktop", Interval: DefaultPRInterval},
}

// configRepo mirrors the JSON shape under "vi.repos[]". Kept
// separate from PRRepo because the config blob carries integer
// seconds (JSON-friendly) while PRRepo.Interval is a core.Duration.
// Conversion happens in LoadPRRepos.
type configRepo struct {
	Provider        string `json:"provider"`
	BaseURL         string `json:"baseUrl"`
	Owner           string `json:"owner"`
	Name            string `json:"name"`
	IntervalSeconds int    `json:"intervalSeconds"`
}

// LoadPRRepos returns the configured PR-watcher targets, falling
// back to the default catalogue when the config service has no
// `vi.repos` key.
//
// Entries with unknown providers, empty owner/name, or (for
// "github") a non-default BaseURL are dropped at load — the
// fetcher would reject them anyway, but rejecting at config-load
// keeps the live catalogue honest and the Wails surface small.
//
// Usage example:
//
//	repos := vi.LoadPRRepos(c)
//	for _, r := range repos { _ = r.Owner + "/" + r.Name }
func LoadPRRepos(c *core.Core) []PRRepo {
	if c == nil {
		return cloneRepos(defaultPRRepos)
	}
	cfg, ok := core.ServiceFor[*config.Service](c, "config")
	if !ok || cfg == nil {
		return cloneRepos(defaultPRRepos)
	}
	var raw []configRepo
	r := cfg.Get("vi.repos", &raw)
	if !r.OK || len(raw) == 0 {
		return cloneRepos(defaultPRRepos)
	}
	out := make([]PRRepo, 0, len(raw))
	for _, cr := range raw {
		if cr.Owner == "" || cr.Name == "" {
			continue
		}
		switch cr.Provider {
		case ProviderForge:
			// BaseURL optional — default to forge.lthn.sh. Any
			// override MUST be https — Forgejo serves the API
			// over TLS; non-https BaseURL is a downgrade vector
			// the same shape as marketplace #1433.
			base := cr.BaseURL
			if base == "" {
				base = defaultForgeBaseURL
			}
			if !core.HasPrefix(base, "https://") {
				continue
			}
			out = append(out, PRRepo{
				Provider: ProviderForge,
				BaseURL:  base,
				Owner:    cr.Owner,
				Name:     cr.Name,
				Interval: intervalFromSeconds(cr.IntervalSeconds),
			})
		case ProviderGitHub:
			// BaseURL override rejected — "github" is github.com.
			// A user wanting GHE can declare "forge" with an
			// explicit BaseURL pointing at their Forgejo /
			// Forgejo-compatible endpoint.
			if cr.BaseURL != "" && cr.BaseURL != defaultGitHubBaseURL {
				continue
			}
			out = append(out, PRRepo{
				Provider: ProviderGitHub,
				BaseURL:  defaultGitHubBaseURL,
				Owner:    cr.Owner,
				Name:     cr.Name,
				Interval: intervalFromSeconds(cr.IntervalSeconds),
			})
		default:
			// Unknown provider — silently dropped. v1 surfaces
			// only the two known backends.
			continue
		}
	}
	if len(out) == 0 {
		// Every entry was rejected. Fall back to the default
		// rather than ship an empty Activity panel — same logic
		// as LoadCatalogue.
		return cloneRepos(defaultPRRepos)
	}
	return out
}

// DefaultPRRepos returns a fresh copy of the out-of-the-box
// watched-repo set. Exported so tests + the Wails surface can
// render "what would Vi watch if you don't configure anything"
// without invoking LoadPRRepos (which reads the live config).
//
// Usage example:
//
//	defaults := vi.DefaultPRRepos()
//	core.AssertEqual(t, 1, len(defaults))
func DefaultPRRepos() []PRRepo {
	return cloneRepos(defaultPRRepos)
}

// RepoKey is the stable "<provider>:<owner>/<name>" identifier for
// a watched repo — used in queue payloads + Activity rollup keys.
//
// Usage example:
//
//	k := vi.RepoKey(vi.PRRepo{Provider: "forge", Owner: "lthn", Name: "desktop"})
//	// → "forge:lthn/desktop"
func RepoKey(r PRRepo) string {
	return r.Provider + ":" + r.Owner + "/" + r.Name
}

// intervalFromSeconds turns the JSON-friendly integer-seconds field
// into a core.Duration, clamping to the floor.
func intervalFromSeconds(secs int) core.Duration {
	interval := DefaultPRInterval
	if secs > 0 {
		interval = core.Duration(secs) * core.Second
	}
	if interval < MinPRInterval {
		interval = MinPRInterval
	}
	return interval
}

// cloneRepos returns an independent copy of the catalogue so callers
// can't mutate the package-level default by appending to the
// returned slice.
func cloneRepos(src []PRRepo) []PRRepo {
	out := make([]PRRepo, len(src))
	copy(out, src)
	return out
}
