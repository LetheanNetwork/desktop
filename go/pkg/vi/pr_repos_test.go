// SPDX-Licence-Identifier: EUPL-1.2

package vi_test

import (
	core "dappco.re/go"
	"dappco.re/go/config"
	"dappco.re/lthn/desktop/pkg/vi"
)

// TestPRRepos_LoadPRRepos_Good — with no `vi.repos` config set,
// LoadPRRepos returns the default catalogue (one entry —
// forge:lthn/desktop).
func TestPRRepos_LoadPRRepos_Good(t *core.T) {
	c := newServiceCore(t)
	repos := vi.LoadPRRepos(c)
	core.AssertLen(t, repos, 1)
	core.AssertEqual(t, vi.ProviderForge, repos[0].Provider)
	core.AssertEqual(t, "lthn", repos[0].Owner)
	core.AssertEqual(t, "desktop", repos[0].Name)
	core.AssertEqual(t, vi.DefaultPRInterval, repos[0].Interval)
}

// TestPRRepos_LoadPRRepos_Bad — nil core + missing-config service
// both fall back to the default catalogue rather than panicking.
func TestPRRepos_LoadPRRepos_Bad(t *core.T) {
	repos := vi.LoadPRRepos(nil)
	core.AssertLen(t, repos, 1)

	// Core without config service registered → defaults.
	c := core.New()
	core.RequireTrue(t, c.ServiceStartup(core.Background(), nil).OK)
	t.Cleanup(func() { _ = c.ServiceShutdown(core.Background()) })
	repos = vi.LoadPRRepos(c)
	core.AssertLen(t, repos, 1)
}

// TestPRRepos_LoadPRRepos_Ugly — exercise the rejection rules:
// missing owner/name, non-https forge BaseURL, github with custom
// BaseURL, unknown provider. All filtered; the surviving entry
// honours its intervalSeconds (clamped at the floor).
func TestPRRepos_LoadPRRepos_Ugly(t *core.T) {
	c := newServiceCore(t)
	cfg, ok := core.ServiceFor[*config.Service](c, "config")
	core.RequireTrue(t, ok)

	core.RequireTrue(t, cfg.Set("vi.repos", []map[string]any{
		// Survives — defaults BaseURL, clamps interval to MinPRInterval.
		{"provider": "forge", "owner": "lthn", "name": "desktop", "intervalSeconds": 5},
		// Dropped — missing owner.
		{"provider": "forge", "name": "no-owner"},
		// Dropped — http BaseURL on forge (downgrade vector).
		{"provider": "forge", "baseUrl": "http://insecure.example", "owner": "x", "name": "y"},
		// Dropped — github with custom BaseURL.
		{"provider": "github", "baseUrl": "https://ghe.example", "owner": "x", "name": "y"},
		// Dropped — unknown provider.
		{"provider": "gitlab", "owner": "x", "name": "y"},
	}).OK)

	repos := vi.LoadPRRepos(c)
	core.AssertLen(t, repos, 1)
	core.AssertEqual(t, "lthn", repos[0].Owner)
	core.AssertEqual(t, vi.MinPRInterval, repos[0].Interval) // clamped
}

// TestPRRepos_DefaultPRRepos_Good — DefaultPRRepos returns a fresh
// copy; mutating the result doesn't poison the package-level default.
func TestPRRepos_DefaultPRRepos_Good(t *core.T) {
	a := vi.DefaultPRRepos()
	core.AssertLen(t, a, 1)
	a[0].Owner = "tampered"
	b := vi.DefaultPRRepos()
	core.AssertEqual(t, "lthn", b[0].Owner)
}

// TestPRRepos_RepoKey_Good — key shape is stable across calls.
func TestPRRepos_RepoKey_Good(t *core.T) {
	k := vi.RepoKey(vi.PRRepo{Provider: "forge", Owner: "lthn", Name: "desktop"})
	core.AssertEqual(t, "forge:lthn/desktop", k)

	k2 := vi.RepoKey(vi.PRRepo{Provider: "github", Owner: "LetheanNetwork", Name: "desktop"})
	core.AssertEqual(t, "github:LetheanNetwork/desktop", k2)
}

// TestPRRepos_LoadPRRepos_AllRejectedFallback — when every config
// entry is invalid, fall back to defaults rather than ship an empty
// panel. Mirrors LoadCatalogue's "user expressed intent" guard.
func TestPRRepos_LoadPRRepos_AllRejectedFallback(t *core.T) {
	c := newServiceCore(t)
	cfg, ok := core.ServiceFor[*config.Service](c, "config")
	core.RequireTrue(t, ok)

	core.RequireTrue(t, cfg.Set("vi.repos", []map[string]any{
		{"provider": "gitlab", "owner": "x", "name": "y"},
		{"provider": "forge"}, // missing owner+name
	}).OK)

	repos := vi.LoadPRRepos(c)
	core.AssertLen(t, repos, 1) // default fallback
	core.AssertEqual(t, "lthn", repos[0].Owner)
}
