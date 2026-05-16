// SPDX-Licence-Identifier: EUPL-1.2

package vi_test

import (
	core "dappco.re/go"
	"dappco.re/go/orm"
	"dappco.re/lthn/desktop/pkg/vi"
)

// newProbeCore builds a Core with orm + memium + the vi schema
// registered. No config service — Probe + LatestByDomain don't
// need it.
func newProbeCore(t *core.T) *core.Core {
	t.Helper()
	c := core.New()
	core.RequireTrue(t, orm.Register(c).OK)
	mem := orm.NewMemium()
	core.RequireTrue(t, orm.Mount(c, "default", mem).OK)
	for _, schema := range vi.Schemas() {
		core.RequireTrue(t, orm.RegisterSchema(c, schema).OK)
		mem.RegisterTable(schema.Name, schema)
	}
	core.RequireTrue(t, c.ServiceStartup(core.Background(), nil).OK)
	t.Cleanup(func() { _ = c.ServiceShutdown(core.Background()) })
	return c
}

// TestProbe_Probe_Bad — non-https URL is rejected at the probe
// surface (no network round-trip, no orm insert).
func TestProbe_Probe_Bad(t *core.T) {
	c := newProbeCore(t)
	r := vi.Probe(c, vi.SiteCatalogue{URL: "http://insecure.example"})
	core.AssertFalse(t, r.OK)

	// Nil core rejected outright.
	r = vi.Probe(nil, vi.SiteCatalogue{URL: "https://lthn.ai"})
	core.AssertFalse(t, r.OK)
}

// TestProbe_LatestByDomain_Good — given multiple SiteProbe rows for
// the same domain, LatestByDomain returns the newest, in catalogue
// order across distinct domains.
func TestProbe_LatestByDomain_Good(t *core.T) {
	c := newProbeCore(t)
	t0 := core.Now().UTC()

	// Insert older + newer rows for lthn.ai, only one for host.uk.com.
	older := vi.SiteProbe{
		ID: "p-older-1", Domain: "lthn.ai", URL: "https://lthn.ai",
		StatusCode: 200, LatencyMs: 100, OK: true,
		CheckedAt: t0.Add(-2 * core.Minute),
	}
	newer := vi.SiteProbe{
		ID: "p-newer-1", Domain: "lthn.ai", URL: "https://lthn.ai",
		StatusCode: 503, LatencyMs: 250, OK: false, LastError: "bad gateway",
		CheckedAt: t0,
	}
	host := vi.SiteProbe{
		ID: "p-host-1", Domain: "host.uk.com", URL: "https://host.uk.com",
		StatusCode: 200, LatencyMs: 80, OK: true,
		CheckedAt: t0,
	}
	core.RequireTrue(t, orm.Insert(c, &older).OK)
	core.RequireTrue(t, orm.Insert(c, &newer).OK)
	core.RequireTrue(t, orm.Insert(c, &host).OK)

	catalogue := []vi.SiteCatalogue{
		{URL: "https://lthn.ai"},
		{URL: "https://host.uk.com"},
	}
	rows := vi.LatestByDomain(c, catalogue)
	core.AssertLen(t, rows, 2)

	// Newest lthn.ai row wins.
	core.AssertEqual(t, "p-newer-1", rows[0].ID)
	core.AssertEqual(t, 503, rows[0].StatusCode)
	core.AssertFalse(t, rows[0].OK)

	// host.uk.com is the second catalogue entry — appears second.
	core.AssertEqual(t, "p-host-1", rows[1].ID)
}

// TestProbe_LatestByDomain_Bad — empty inputs return nil-safe empty
// slices. A catalogue entry with no probe rows yet is silently
// skipped so partial Vi pipelines (first-run, just-added site)
// render cleanly.
func TestProbe_LatestByDomain_Bad(t *core.T) {
	c := newProbeCore(t)

	core.AssertNil(t, vi.LatestByDomain(nil, nil))
	core.AssertNil(t, vi.LatestByDomain(c, nil))

	// Catalogue entry with no probe yet → no row in output.
	rows := vi.LatestByDomain(c, []vi.SiteCatalogue{
		{URL: "https://never-probed.example"},
	})
	core.AssertLen(t, rows, 0)
}

// TestProbe_LatestByDomain_Ugly — catalogue order is honoured even
// when SiteProbe rows arrived out-of-order, and duplicate URLs in
// the catalogue surface the same probe twice (caller responsibility
// to dedup — we don't silently drop user-declared intent).
func TestProbe_LatestByDomain_Ugly(t *core.T) {
	c := newProbeCore(t)
	t0 := core.Now().UTC()

	p := vi.SiteProbe{
		ID: "p-1", Domain: "lthn.ai", URL: "https://lthn.ai",
		StatusCode: 200, LatencyMs: 50, OK: true,
		CheckedAt: t0,
	}
	core.RequireTrue(t, orm.Insert(c, &p).OK)

	rows := vi.LatestByDomain(c, []vi.SiteCatalogue{
		{URL: "https://lthn.ai"},
		{URL: "https://lthn.ai/duplicate"}, // same domain
	})
	core.AssertLen(t, rows, 2)
	core.AssertEqual(t, rows[0].ID, rows[1].ID) // same underlying probe
}
