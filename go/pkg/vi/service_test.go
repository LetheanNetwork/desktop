// SPDX-Licence-Identifier: EUPL-1.2

package vi_test

import (
	core "dappco.re/go"
	"dappco.re/go/config"
	"dappco.re/go/orm"

	"dappco.re/lthn/desktop/pkg/auth"
	"dappco.re/lthn/desktop/pkg/vi"
)

// newServiceCore wires config + orm + the vi schema. No queue
// service — Service.OnStart short-circuits cleanly when queue isn't
// registered, which we exercise in TestService_OnStart_Bad.
//
// Stamps TierOperator with subject "test-operator" via the auth
// sidecar so the Mantis #1750 Require gates on Catalogue / Sites /
// Repos / Activity pass through. Tests that want to exercise a
// different tier (e.g. TierInternal floor for deny-path coverage)
// re-stamp via auth.SetCaller / auth.ClearCaller after construction.
func newServiceCore(t *core.T) *core.Core {
	t.Helper()
	c := core.New(
		core.WithName("config", config.NewConfigServiceWith(config.ServiceOptions{
			Path: t.TempDir() + "/lthn.json",
		})),
	)
	core.RequireTrue(t, orm.Register(c).OK)
	mem := orm.NewMemium()
	core.RequireTrue(t, orm.Mount(c, "default", mem).OK)
	for _, schema := range vi.Schemas() {
		core.RequireTrue(t, orm.RegisterSchema(c, schema).OK)
		mem.RegisterTable(schema.Name, schema)
	}
	core.RequireTrue(t, c.ServiceStartup(core.Background(), nil).OK)
	auth.SetCaller(c, auth.CallerIdentity{
		Tier:    auth.TierOperator,
		Subject: "test-operator",
		Source:  "vi_test",
	})
	t.Cleanup(func() {
		auth.ClearCaller(c)
		_ = c.ServiceShutdown(core.Background())
	})
	return c
}

// TestService_Sites_Good — with one persisted probe per catalogue
// entry, Sites() returns one SiteStatus per probed domain in
// catalogue order, with Stack populated from the catalogue.
func TestService_Sites_Good(t *core.T) {
	c := newServiceCore(t)
	svc := vi.NewService(c)
	t0 := core.Now().UTC()

	// Override the catalogue to a small known set.
	cfg, ok := core.ServiceFor[*config.Service](c, "config")
	core.RequireTrue(t, ok)
	core.RequireTrue(t, cfg.Set("vi.sites", []map[string]any{
		{"url": "https://lthn.ai", "stack": "Lethean · CIC"},
		{"url": "https://host.uk.com", "stack": "Lethean · SaaS"},
	}).OK)

	// Seed one probe per site.
	ok1 := vi.SiteProbe{
		ID: "p-1", Domain: "lthn.ai", URL: "https://lthn.ai",
		StatusCode: 200, LatencyMs: 42, OK: true,
		CheckedAt: t0,
	}
	bad := vi.SiteProbe{
		ID: "p-2", Domain: "host.uk.com", URL: "https://host.uk.com",
		StatusCode: 503, LatencyMs: 120, OK: false, LastError: "bad gateway",
		CheckedAt: t0,
	}
	core.RequireTrue(t, orm.Insert(c, &ok1).OK)
	core.RequireTrue(t, orm.Insert(c, &bad).OK)

	r := svc.Sites()
	core.RequireTrue(t, r.OK)
	out := r.Value.(vi.SitesOutput)
	core.AssertEqual(t, 2, out.Scanned)
	core.AssertLen(t, out.Sites, 2)

	core.AssertEqual(t, "lthn.ai", out.Sites[0].Domain)
	core.AssertEqual(t, "Lethean · CIC", out.Sites[0].Stack)
	core.AssertEqual(t, "green", out.Sites[0].Status)
	core.AssertEqual(t, 42, out.Sites[0].Response)

	core.AssertEqual(t, "host.uk.com", out.Sites[1].Domain)
	core.AssertEqual(t, "red", out.Sites[1].Status)
	core.AssertEqual(t, "bad gateway", out.Sites[1].Err)
}

// TestService_Sites_Bad — nil service + service-with-nil-core both
// return a Sites envelope rather than panicking, so the Wails
// binding always has a stable JSON shape to surface.
func TestService_Sites_Bad(t *core.T) {
	var nilSvc *vi.Service
	r := nilSvc.Sites()
	core.RequireTrue(t, r.OK)
	out := r.Value.(vi.SitesOutput)
	core.AssertLen(t, out.Sites, 0)

	svc := vi.NewService(nil)
	r = svc.Sites()
	core.RequireTrue(t, r.OK)
}

// TestService_Sites_Ugly — catalogued sites with no probe row yet
// are skipped from the response (not surfaced as empty cards). The
// "Scanned" count reflects the catalogue size, NOT the response
// length, so the UI can render "watching 4 sites, results from 2".
func TestService_Sites_Ugly(t *core.T) {
	c := newServiceCore(t)
	svc := vi.NewService(c)
	cfg, ok := core.ServiceFor[*config.Service](c, "config")
	core.RequireTrue(t, ok)
	core.RequireTrue(t, cfg.Set("vi.sites", []map[string]any{
		{"url": "https://probed.example"},
		{"url": "https://never-probed.example"},
	}).OK)

	probe := vi.SiteProbe{
		ID: "p-only", Domain: "probed.example", URL: "https://probed.example",
		StatusCode: 200, LatencyMs: 1, OK: true,
		CheckedAt: core.Now().UTC(),
	}
	core.RequireTrue(t, orm.Insert(c, &probe).OK)

	r := svc.Sites()
	core.RequireTrue(t, r.OK)
	out := r.Value.(vi.SitesOutput)
	core.AssertEqual(t, 2, out.Scanned) // catalogue size
	core.AssertLen(t, out.Sites, 1)     // only the one with probe data
	core.AssertEqual(t, "probed.example", out.Sites[0].Domain)
}

// TestService_Catalogue_Good — Catalogue() returns the live config-
// resolved catalogue (defaults when nothing's configured).
func TestService_Catalogue_Good(t *core.T) {
	c := newServiceCore(t)
	svc := vi.NewService(c)

	r := svc.Catalogue()
	core.RequireTrue(t, r.OK)
	cat := r.Value.([]vi.SiteCatalogue)
	core.AssertEqual(t, 4, len(cat)) // defaults
}

// TestService_Catalogue_Bad — nil service returns an empty
// catalogue rather than panicking.
func TestService_Catalogue_Bad(t *core.T) {
	var nilSvc *vi.Service
	r := nilSvc.Catalogue()
	core.RequireTrue(t, r.OK)
	cat := r.Value.([]vi.SiteCatalogue)
	core.AssertLen(t, cat, 0)
}

// TestService_Catalogue_Ugly — flipping the config between calls
// changes the response without restarting the service (live-reload
// discipline matches the runner package).
func TestService_Catalogue_Ugly(t *core.T) {
	c := newServiceCore(t)
	svc := vi.NewService(c)
	cfg, ok := core.ServiceFor[*config.Service](c, "config")
	core.RequireTrue(t, ok)

	before := svc.Catalogue().Value.([]vi.SiteCatalogue)
	core.AssertEqual(t, 4, len(before))

	core.RequireTrue(t, cfg.Set("vi.sites", []map[string]any{
		{"url": "https://only.example"},
	}).OK)
	after := svc.Catalogue().Value.([]vi.SiteCatalogue)
	core.AssertEqual(t, 1, len(after))
	core.AssertEqual(t, "https://only.example", after[0].URL)
}

// TestService_ServiceName_Good — the Wails binding namespace is
// stable; renaming would break the generated frontend bindings.
func TestService_ServiceName_Good(t *core.T) {
	svc := vi.NewService(nil)
	core.AssertEqual(t, "Vi", svc.ServiceName())
}

// TestService_Register_Good — Register returns a Result whose Value
// is a *Service so the registry's ServiceFor[*vi.Service] resolves.
func TestService_Register_Good(t *core.T) {
	c := newServiceCore(t)
	r := vi.Register(c)
	core.RequireTrue(t, r.OK)
	_, ok := r.Value.(*vi.Service)
	core.AssertTrue(t, ok)
}

// TestService_Activity_Good — with one persisted PR snapshot per
// catalogue entry, Activity() returns one ActivityEntry per open
// PR, newest-first by checked_at (and limited to the watched
// catalogue — out-of-catalogue rows don't surface).
func TestService_Activity_Good(t *core.T) {
	c := newServiceCore(t)
	svc := vi.NewService(c)
	t0 := core.Now().UTC()

	// Override the watched repos to a small known set.
	cfg, ok := core.ServiceFor[*config.Service](c, "config")
	core.RequireTrue(t, ok)
	core.RequireTrue(t, cfg.Set("vi.repos", []map[string]any{
		{"provider": "forge", "owner": "lthn", "name": "desktop"},
		{"provider": "github", "owner": "LetheanNetwork", "name": "desktop"},
	}).OK)

	rows := []vi.PRActivity{
		{ID: "pra-f1", Provider: "forge", Owner: "lthn", Repo: "desktop",
			PRNumber: 7, Title: "Lethean forge PR", Author: "snider", State: "open",
			URL:      "https://forge.lthn.sh/lthn/desktop/pulls/7",
			OpenedAt: t0.Add(-24 * core.Hour), UpdatedAt: t0, CheckedAt: t0},
		{ID: "pra-g1", Provider: "github", Owner: "LetheanNetwork", Repo: "desktop",
			PRNumber: 1, Title: "GitHub mirror PR", Author: "external", State: "open",
			URL:      "https://github.com/LetheanNetwork/desktop/pull/1",
			OpenedAt: t0.Add(-2 * core.Hour), UpdatedAt: t0, CheckedAt: t0},
	}
	for i := range rows {
		core.RequireTrue(t, orm.Insert(c, &rows[i]).OK)
	}

	r := svc.Activity()
	core.RequireTrue(t, r.OK)
	out := r.Value.(vi.ActivityOutput)
	core.AssertEqual(t, 2, out.Scanned)
	core.AssertLen(t, out.Items, 2)

	// First entry has the canonical fields shaped + URL preserved.
	first := out.Items[0]
	core.AssertEqual(t, "pr", first.Kind)
	core.AssertTrue(t, first.Provider == "forge" || first.Provider == "github")
	core.AssertTrue(t, first.URL != "")
	core.AssertTrue(t, first.Repo != "")
	core.AssertTrue(t, first.UpdatedAt != "")
}

// TestService_Activity_Bad — nil service + service-with-nil-core
// both return a stable Activity envelope rather than panicking, so
// the Wails binding always has a JSON shape to surface.
func TestService_Activity_Bad(t *core.T) {
	var nilSvc *vi.Service
	r := nilSvc.Activity()
	core.RequireTrue(t, r.OK)
	out := r.Value.(vi.ActivityOutput)
	core.AssertLen(t, out.Items, 0)

	svc := vi.NewService(nil)
	r = svc.Activity()
	core.RequireTrue(t, r.OK)
}

// TestService_Activity_Ugly — closed PRs are filtered out; only
// open PRs surface to the Activity panel. Scanned reflects the
// catalogue size regardless of how many open rows landed.
func TestService_Activity_Ugly(t *core.T) {
	c := newServiceCore(t)
	svc := vi.NewService(c)
	cfg, ok := core.ServiceFor[*config.Service](c, "config")
	core.RequireTrue(t, ok)
	core.RequireTrue(t, cfg.Set("vi.repos", []map[string]any{
		{"provider": "forge", "owner": "lthn", "name": "desktop"},
	}).OK)

	t0 := core.Now().UTC()
	open := vi.PRActivity{
		ID: "pra-o", Provider: "forge", Owner: "lthn", Repo: "desktop",
		PRNumber: 1, Title: "open", State: "open", CheckedAt: t0,
	}
	closed := vi.PRActivity{
		ID: "pra-c", Provider: "forge", Owner: "lthn", Repo: "desktop",
		PRNumber: 2, Title: "closed", State: "closed", CheckedAt: t0,
	}
	core.RequireTrue(t, orm.Insert(c, &open).OK)
	core.RequireTrue(t, orm.Insert(c, &closed).OK)

	r := svc.Activity()
	core.RequireTrue(t, r.OK)
	out := r.Value.(vi.ActivityOutput)
	core.AssertEqual(t, 1, out.Scanned)
	core.AssertLen(t, out.Items, 1)
	core.AssertEqual(t, 1, out.Items[0].Number)
}

// TestService_Repos_Good — Repos() returns the live config-
// resolved catalogue (defaults when nothing's configured).
func TestService_Repos_Good(t *core.T) {
	c := newServiceCore(t)
	svc := vi.NewService(c)

	r := svc.Repos()
	core.RequireTrue(t, r.OK)
	repos := r.Value.([]vi.PRRepo)
	core.AssertEqual(t, 1, len(repos)) // default catalogue
	core.AssertEqual(t, "lthn", repos[0].Owner)
}

// TestService_Repos_Bad — nil service returns an empty list rather
// than panicking.
func TestService_Repos_Bad(t *core.T) {
	var nilSvc *vi.Service
	r := nilSvc.Repos()
	core.RequireTrue(t, r.OK)
	repos := r.Value.([]vi.PRRepo)
	core.AssertLen(t, repos, 0)
}

// TestService_Repos_Ugly — flipping vi.repos between calls is
// reflected without restart (live-reload discipline mirrors the
// sites catalogue).
func TestService_Repos_Ugly(t *core.T) {
	c := newServiceCore(t)
	svc := vi.NewService(c)
	cfg, ok := core.ServiceFor[*config.Service](c, "config")
	core.RequireTrue(t, ok)

	before := svc.Repos().Value.([]vi.PRRepo)
	core.AssertEqual(t, 1, len(before))

	core.RequireTrue(t, cfg.Set("vi.repos", []map[string]any{
		{"provider": "forge", "owner": "lthn", "name": "ai"},
		{"provider": "github", "owner": "openai", "name": "openai-python"},
	}).OK)
	after := svc.Repos().Value.([]vi.PRRepo)
	core.AssertEqual(t, 2, len(after))
}
