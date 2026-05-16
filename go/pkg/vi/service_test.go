// SPDX-Licence-Identifier: EUPL-1.2

package vi_test

import (
	core "dappco.re/go"
	"dappco.re/go/config"
	"dappco.re/go/orm"
	"dappco.re/lthn/desktop/pkg/vi"
)

// newServiceCore wires config + orm + the vi schema. No queue
// service — Service.OnStart short-circuits cleanly when queue isn't
// registered, which we exercise in TestService_OnStart_Bad.
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
	t.Cleanup(func() { _ = c.ServiceShutdown(core.Background()) })
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
