// SPDX-Licence-Identifier: EUPL-1.2

package vi_test

import (
	core "dappco.re/go"
	"dappco.re/go/orm"
	"dappco.re/lthn/desktop/pkg/vi"
)

// newFetchCore builds a Core with orm + memium + the vi schemas
// registered. No config service — Fetch + LatestPerPR don't need it
// for the persistence-side tests below. Tests that need a token / a
// `vi.repos` override use newServiceCore (defined in service_test.go).
func newFetchCore(t *core.T) *core.Core {
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

// TestPRFetch_Fetch_Bad — invalid inputs (nil core, empty owner)
// fail at the surface without a network round-trip or orm insert.
func TestPRFetch_Fetch_Bad(t *core.T) {
	c := newFetchCore(t)

	// Nil core rejected outright.
	r := vi.Fetch(nil, vi.PRRepo{Provider: "forge", Owner: "lthn", Name: "desktop"})
	core.AssertFalse(t, r.OK)

	// Empty owner+name rejected — we don't want to send an open
	// GET to a malformed URL.
	r = vi.Fetch(c, vi.PRRepo{Provider: "forge"})
	core.AssertFalse(t, r.OK)

	// Unknown provider → URL builder returns "", Fetch fails.
	r = vi.Fetch(c, vi.PRRepo{Provider: "bitbucket", Owner: "x", Name: "y"})
	core.AssertFalse(t, r.OK)
}

// TestPRFetch_LatestPerPR_Good — given multiple PRActivity rows for
// the same PR, LatestPerPR returns the newest snapshot per
// (provider, owner, repo, pr_number), filtered to the supplied
// catalogue and ordered newest-first by checked_at.
func TestPRFetch_LatestPerPR_Good(t *core.T) {
	c := newFetchCore(t)
	t0 := core.Now().UTC()

	// PR #7 on lthn/desktop — two snapshots, the newer wins.
	older := vi.PRActivity{
		ID: "pra-older-1", Provider: "forge", Owner: "lthn", Repo: "desktop",
		PRNumber: 7, Title: "old title", Author: "snider", State: "open",
		URL:       "https://forge.lthn.sh/lthn/desktop/pulls/7",
		CheckedAt: t0.Add(-1 * core.Hour),
	}
	newer := vi.PRActivity{
		ID: "pra-newer-1", Provider: "forge", Owner: "lthn", Repo: "desktop",
		PRNumber: 7, Title: "new title", Author: "snider", State: "open",
		URL:       "https://forge.lthn.sh/lthn/desktop/pulls/7",
		CheckedAt: t0,
	}
	// PR #1 on a different repo, also watched.
	other := vi.PRActivity{
		ID: "pra-other-1", Provider: "github", Owner: "LetheanNetwork", Repo: "desktop",
		PRNumber: 1, Title: "gh pr", Author: "external", State: "open",
		URL:       "https://github.com/LetheanNetwork/desktop/pull/1",
		CheckedAt: t0,
	}
	core.RequireTrue(t, orm.Insert(c, &older).OK)
	core.RequireTrue(t, orm.Insert(c, &newer).OK)
	core.RequireTrue(t, orm.Insert(c, &other).OK)

	catalogue := []vi.PRRepo{
		{Provider: "forge", Owner: "lthn", Name: "desktop"},
		{Provider: "github", Owner: "LetheanNetwork", Name: "desktop"},
	}
	rows := vi.LatestPerPR(c, catalogue, 50)
	core.AssertLen(t, rows, 2)

	// Find the lthn/desktop PR #7 — should be the newer row.
	var found *vi.PRActivity
	for i := range rows {
		if rows[i].PRNumber == 7 {
			found = &rows[i]
			break
		}
	}
	core.AssertTrue(t, found != nil)
	if found != nil {
		core.AssertEqual(t, "pra-newer-1", found.ID)
		core.AssertEqual(t, "new title", found.Title)
	}
}

// TestPRFetch_LatestPerPR_Bad — empty inputs are nil-safe; rows
// for repos NOT in the supplied catalogue are silently filtered.
func TestPRFetch_LatestPerPR_Bad(t *core.T) {
	c := newFetchCore(t)

	core.AssertNil(t, vi.LatestPerPR(nil, nil, 50))
	core.AssertNil(t, vi.LatestPerPR(c, nil, 50))

	// Persist a row for a NON-catalogued repo — must NOT surface.
	row := vi.PRActivity{
		ID: "pra-x", Provider: "forge", Owner: "ghost", Repo: "repo",
		PRNumber: 1, Title: "x", State: "open",
		CheckedAt: core.Now().UTC(),
	}
	core.RequireTrue(t, orm.Insert(c, &row).OK)

	out := vi.LatestPerPR(c, []vi.PRRepo{
		{Provider: "forge", Owner: "lthn", Name: "desktop"},
	}, 50)
	core.AssertLen(t, out, 0)
}

// TestPRFetch_LatestPerPR_Ugly — closed/merged PRs are filtered
// out; only "open" state rows surface to the Activity panel.
// limit=0 falls back to DefaultActivityLimit.
func TestPRFetch_LatestPerPR_Ugly(t *core.T) {
	c := newFetchCore(t)
	t0 := core.Now().UTC()

	open := vi.PRActivity{
		ID: "pra-open", Provider: "forge", Owner: "lthn", Repo: "desktop",
		PRNumber: 1, Title: "open one", State: "open",
		CheckedAt: t0,
	}
	closed := vi.PRActivity{
		ID: "pra-closed", Provider: "forge", Owner: "lthn", Repo: "desktop",
		PRNumber: 2, Title: "closed one", State: "closed",
		CheckedAt: t0,
	}
	core.RequireTrue(t, orm.Insert(c, &open).OK)
	core.RequireTrue(t, orm.Insert(c, &closed).OK)

	catalogue := []vi.PRRepo{
		{Provider: "forge", Owner: "lthn", Name: "desktop"},
	}
	// limit=0 → DefaultActivityLimit fallback.
	rows := vi.LatestPerPR(c, catalogue, 0)
	core.AssertLen(t, rows, 1)
	core.AssertEqual(t, "pra-open", rows[0].ID)
}

// TestPRFetch_Schemas_Good — the vi package exposes both SiteProbe
// AND PRActivity schemas via the canonical Schemas() entry point so
// cmd/lthn/app.go's `vi.Schemas()...` append wires the new table
// without a separate registration call.
func TestPRFetch_Schemas_Good(t *core.T) {
	schemas := vi.Schemas()
	core.AssertEqual(t, 2, len(schemas))

	// Names line up — guards against a future copy-paste shuffle
	// where the slice still has two entries but pointing at the
	// wrong tables.
	names := make(map[string]bool, len(schemas))
	for _, s := range schemas {
		names[s.Name] = true
	}
	core.AssertTrue(t, names["vi_site_probes"])
	core.AssertTrue(t, names["vi_pr_activity"])
}
