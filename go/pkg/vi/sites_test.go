// SPDX-Licence-Identifier: EUPL-1.2

package vi_test

import (
	core "dappco.re/go"
	"dappco.re/go/config"
	"dappco.re/lthn/desktop/pkg/vi"
)

// newViCore constructs a Core with config wired (default — no
// vi.sites override). LoadCatalogue falls back to defaults.
func newViCore(t *core.T) *core.Core {
	t.Helper()
	c := core.New(
		core.WithName("config", config.NewConfigServiceWith(config.ServiceOptions{
			Path: t.TempDir() + "/lthn.json",
		})),
	)
	core.RequireTrue(t, c.ServiceStartup(core.Background(), nil).OK)
	t.Cleanup(func() { _ = c.ServiceShutdown(core.Background()) })
	return c
}

// TestSites_LoadCatalogue_Good — with no config override, returns
// the four-entry default catalogue covering the canonical Lethean
// family surfaces.
func TestSites_LoadCatalogue_Good(t *core.T) {
	c := newViCore(t)
	cat := vi.LoadCatalogue(c)
	core.AssertEqual(t, 4, len(cat))

	// Defaults are HTTPS, named, and at the default interval.
	for _, entry := range cat {
		core.AssertTrue(t, core.HasPrefix(entry.URL, "https://"))
		core.AssertNotEmpty(t, entry.Name)
		core.AssertEqual(t, vi.DefaultInterval, entry.Interval)
	}
}

// TestSites_LoadCatalogue_Bad — nil core returns a (still safe)
// clone of the default catalogue so callers can iterate without a
// guard. Empty/missing config returns defaults, not an empty slice.
func TestSites_LoadCatalogue_Bad(t *core.T) {
	cat := vi.LoadCatalogue(nil)
	core.AssertEqual(t, 4, len(cat))

	// Independent copy — mutating the returned slice must not
	// affect the package-level default.
	cat[0].Name = "MUTATED"
	again := vi.LoadCatalogue(nil)
	core.AssertNotEqual(t, "MUTATED", again[0].Name)
}

// TestSites_LoadCatalogue_Ugly — config override REPLACES the
// default catalogue rather than merging. Non-https entries are
// dropped. IntervalSeconds below MinInterval is clamped.
func TestSites_LoadCatalogue_Ugly(t *core.T) {
	c := newViCore(t)
	cfg, ok := core.ServiceFor[*config.Service](c, "config")
	core.RequireTrue(t, ok)

	// Three entries: one good, one http (should be dropped),
	// one with a tiny interval (should be clamped to MinInterval).
	core.RequireTrue(t, cfg.Set("vi.sites", []map[string]any{
		{"url": "https://example.org", "name": "Example", "intervalSeconds": 30},
		{"url": "http://insecure.example", "name": "Insecure"},
		{"url": "https://fast.example", "name": "Fast", "intervalSeconds": 1},
	}).OK)

	cat := vi.LoadCatalogue(c)
	core.AssertEqual(t, 2, len(cat)) // http entry dropped

	core.AssertEqual(t, "https://example.org", cat[0].URL)
	core.AssertEqual(t, 30*core.Second, cat[0].Interval)

	core.AssertEqual(t, "https://fast.example", cat[1].URL)
	core.AssertEqual(t, vi.MinInterval, cat[1].Interval) // clamped up
}

// TestSites_DomainOf_Good — strips scheme + path to leave host.
func TestSites_DomainOf_Good(t *core.T) {
	core.AssertEqual(t, "lthn.ai", vi.DomainOf("https://lthn.ai"))
	core.AssertEqual(t, "lthn.ai", vi.DomainOf("https://lthn.ai/"))
	core.AssertEqual(t, "lthn.ai", vi.DomainOf("https://lthn.ai/dashboard"))
}

// TestSites_DomainOf_Bad — unparseable URL falls back to the raw
// input so logs always carry something identifying.
func TestSites_DomainOf_Bad(t *core.T) {
	core.AssertEqual(t, "::not a url::", vi.DomainOf("::not a url::"))
}

// TestSites_DomainOf_Ugly — non-default port is preserved so
// "lthn.ai" vs "lthn.ai:8443" remain distinct keys in the SiteProbe
// table.
func TestSites_DomainOf_Ugly(t *core.T) {
	core.AssertEqual(t, "x.io:8443", vi.DomainOf("https://x.io:8443/path"))
}

// TestSites_DefaultCatalogue_Good — DefaultCatalogue() returns an
// independent copy, never the package-level slice itself.
func TestSites_DefaultCatalogue_Good(t *core.T) {
	a := vi.DefaultCatalogue()
	b := vi.DefaultCatalogue()
	core.AssertEqual(t, len(a), len(b))

	a[0].Name = "CHANGED"
	core.AssertNotEqual(t, "CHANGED", b[0].Name)
}

// TestSites_Schemas_Good — Schemas() returns the vi_site_probes
// schema (first, Sites came before PR Activity) plus any other
// schemas the package owns; smoke check on PK + indexes for the
// sites entry without asserting total count (the count is owned by
// TestPRFetch_Schemas_Good in pr_fetch_test.go).
func TestSites_Schemas_Good(t *core.T) {
	schemas := vi.Schemas()
	core.AssertTrue(t, len(schemas) >= 1)
	core.AssertEqual(t, "vi_site_probes", schemas[0].Name)
	core.AssertEqual(t, "id", schemas[0].PK[0])
}

// TestSites_Schemas_Bad — Schemas() never returns nil or zero-length;
// callers can iterate without a guard.
func TestSites_Schemas_Bad(t *core.T) {
	schemas := vi.Schemas()
	core.AssertNotEmpty(t, schemas)
	core.AssertNotEqual(t, "queue_jobs", schemas[0].Name)
}

// TestSites_Schemas_Ugly — every field name in the returned Schema
// is unique; defensive against a future refactor adding a duplicate
// builder call.
func TestSites_Schemas_Ugly(t *core.T) {
	schema := vi.Schemas()[0]
	seen := map[string]bool{}
	for _, f := range schema.Fields {
		core.AssertFalse(t, seen[f.Name])
		seen[f.Name] = true
	}
}
