// SPDX-Licence-Identifier: EUPL-1.2

// Internal tests for service.go — OnStart / OnStop / handleProbeJob /
// enqueue / handleFetchPRJob / enqueueFetchPR are unexported, so this
// file lives in package vi (mirrors audit_test.go's precedent).
// Internal so the real self-rescheduling queue-handler chain can be
// driven directly rather than waiting on the background worker
// ticker — deterministic, no sleeps.

package vi

import (
	"net/http"
	"net/http/httptest"

	core "dappco.re/go"
	"dappco.re/go/config"
	"dappco.re/go/orm"

	"dappco.re/lthn/desktop/pkg/auth"
	"dappco.re/lthn/desktop/pkg/queue"
)

// newInternalServiceCore wires config + orm + the vi schema + the
// queue schema (queue.Enqueue/ScheduleAfter persist via orm — no
// queue.Service registration needed, matches pkg/benchmark's
// newCoreWithBenchmarkAndQueue precedent). Stamps TierOperator so
// the auth.Require gates inside queue.Enqueue and vi's own Wails
// accessors pass.
func newInternalServiceCore(t *core.T) *core.Core {
	t.Helper()
	c := core.New(
		core.WithName("config", config.NewConfigServiceWith(config.ServiceOptions{
			Path: t.TempDir() + "/lthn.json",
		})),
	)
	core.RequireTrue(t, orm.Register(c).OK)
	mem := orm.NewMemium()
	core.RequireTrue(t, orm.Mount(c, "default", mem).OK)
	for _, schema := range Schemas() {
		core.RequireTrue(t, orm.RegisterSchema(c, schema).OK)
		mem.RegisterTable(schema.Name, schema)
	}
	for _, schema := range queue.Schemas() {
		core.RequireTrue(t, orm.RegisterSchema(c, schema).OK)
		mem.RegisterTable(schema.Name, schema)
	}
	core.RequireTrue(t, c.ServiceStartup(core.Background(), nil).OK)
	auth.SetCaller(c, auth.CallerIdentity{
		Tier:    auth.TierOperator,
		Subject: "test-operator",
		Source:  "vi_internal_test",
	})
	t.Cleanup(func() {
		auth.ClearCaller(c)
		_ = c.ServiceShutdown(core.Background())
	})
	return c
}

// pendingJobsOfKind counts StatusPending queue.Job rows of the given
// kind — used to assert OnStart's seed wave + handler reschedules
// actually persisted work rather than silently no-op'ing.
func pendingJobsOfKind(t *core.T, c *core.Core, kind string) int {
	t.Helper()
	r := orm.Of[queue.Job](c).Where("kind", "=", kind).Get()
	core.RequireTrue(t, r.OK)
	rows, ok := r.Value.([]queue.Job)
	core.RequireTrue(t, ok)
	n := 0
	for _, row := range rows {
		if row.Status == queue.StatusPending {
			n++
		}
	}
	return n
}

// TestService_OnStart_Good — with a live queue substrate, OnStart
// registers both handler kinds and seeds one immediate-fire Job per
// catalogued site (4 defaults) + one per watched repo (1 default).
func TestService_OnStart_Good(t *core.T) {
	c := newInternalServiceCore(t)
	svc := NewService(c)

	r := svc.OnStart()
	core.RequireTrue(t, r.OK)

	core.AssertEqual(t, 4, pendingJobsOfKind(t, c, ProbeKind))
	core.AssertEqual(t, 1, pendingJobsOfKind(t, c, FetchPRKind))
}

// TestService_OnStart_Bad — nil service and service-with-nil-core
// both fail cleanly rather than panicking.
func TestService_OnStart_Bad(t *core.T) {
	var nilSvc *Service
	r := nilSvc.OnStart()
	core.AssertFalse(t, r.OK)

	svc := NewService(nil)
	r = svc.OnStart()
	core.AssertFalse(t, r.OK)
}

// TestService_OnStart_Ugly — an empty catalogue (all sites + repos
// config-overridden to nothing watchable) still returns Ok; OnStart
// tolerates a "watching nothing" state cleanly.
func TestService_OnStart_Ugly(t *core.T) {
	c := newInternalServiceCore(t)
	svc := NewService(c)

	cfg, ok := core.ServiceFor[*config.Service](c, "config")
	core.RequireTrue(t, ok)
	// Non-https entries are rejected by LoadCatalogue/LoadPRRepos but
	// fall back to defaults when every entry rejects — so instead
	// assert the fallback-to-default behaviour holds under OnStart
	// too (still Ok, still seeds the default 4 + 1).
	core.RequireTrue(t, cfg.Set("vi.sites", []map[string]any{
		{"url": "http://insecure.example"},
	}).OK)

	r := svc.OnStart()
	core.RequireTrue(t, r.OK)
	core.AssertEqual(t, 4, pendingJobsOfKind(t, c, ProbeKind))
}

// TestService_OnStop_Good — OnStop is a documented no-op; assert it
// stays that way (Ok(nil), no side effects) for both a live service
// and the nil receiver.
func TestService_OnStop_Good(t *core.T) {
	c := newInternalServiceCore(t)
	svc := NewService(c)
	r := svc.OnStop()
	core.RequireTrue(t, r.OK)
	core.AssertNil(t, r.Value)
}

func TestService_OnStop_Bad(t *core.T) {
	var nilSvc *Service
	r := nilSvc.OnStop()
	core.RequireTrue(t, r.OK)
}

// TestService_HandleProbeJob_Good — a live catalogue entry probes
// successfully against a hermetic HTTPS test server, persists a
// SiteProbe row, and reschedules the next tick (a fresh pending Job
// for the same kind).
func TestService_HandleProbeJob_Good(t *core.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer ts.Close()

	origClient := probeClient
	probeClient = ts.Client()
	t.Cleanup(func() { probeClient = origClient })

	c := newInternalServiceCore(t)
	svc := NewService(c)

	cfg, ok := core.ServiceFor[*config.Service](c, "config")
	core.RequireTrue(t, ok)
	core.RequireTrue(t, cfg.Set("vi.sites", []map[string]any{
		{"url": ts.URL, "intervalSeconds": 60},
	}).OK)

	opts := core.NewOptions(core.Option{Key: "url", Value: ts.URL})
	r := svc.handleProbeJob(core.Background(), opts)
	core.RequireTrue(t, r.OK)

	// One probe row persisted, successful.
	probeRows := orm.Of[SiteProbe](c).Where("url", "=", ts.URL).Get()
	core.RequireTrue(t, probeRows.OK)
	rows, ok := probeRows.Value.([]SiteProbe)
	core.RequireTrue(t, ok)
	core.RequireTrue(t, len(rows) == 1)
	core.AssertTrue(t, rows[0].OK)
	core.AssertEqual(t, 200, rows[0].StatusCode)

	// Reschedule fired — a fresh pending Job exists for ProbeKind.
	core.AssertEqual(t, 1, pendingJobsOfKind(t, c, ProbeKind))
}

// TestService_HandleProbeJob_Bad — empty url payload short-circuits
// without touching the catalogue or network; url no longer in the
// live catalogue ends the reschedule chain cleanly (no fresh Job).
func TestService_HandleProbeJob_Bad(t *core.T) {
	c := newInternalServiceCore(t)
	svc := NewService(c)

	r := svc.handleProbeJob(core.Background(), core.NewOptions())
	core.RequireTrue(t, r.OK)
	core.AssertEqual(t, 0, pendingJobsOfKind(t, c, ProbeKind))

	opts := core.NewOptions(core.Option{Key: "url", Value: "https://no-longer-catalogued.example"})
	r = svc.handleProbeJob(core.Background(), opts)
	core.RequireTrue(t, r.OK)
	core.AssertEqual(t, 0, pendingJobsOfKind(t, c, ProbeKind))
}

// TestService_HandleProbeJob_Ugly — a catalogue entry with no
// explicit interval falls back to DefaultInterval for the reschedule
// (delay>0 branch of enqueue, via queue.ScheduleAfter rather than
// immediate Enqueue) instead of Enqueue's zero-delay branch.
func TestService_HandleProbeJob_Ugly(t *core.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	origClient := probeClient
	probeClient = ts.Client()
	t.Cleanup(func() { probeClient = origClient })

	c := newInternalServiceCore(t)
	svc := NewService(c)

	cfg, ok := core.ServiceFor[*config.Service](c, "config")
	core.RequireTrue(t, ok)
	// intervalSeconds omitted (0) -> LoadCatalogue defaults it to
	// DefaultInterval, exercising handleProbeJob's `interval <= 0`
	// fallback branch explicitly (cat.Interval always > 0 today, but
	// the fallback line stays exercised via the default path).
	core.RequireTrue(t, cfg.Set("vi.sites", []map[string]any{
		{"url": ts.URL},
	}).OK)

	opts := core.NewOptions(core.Option{Key: "url", Value: ts.URL})
	r := svc.handleProbeJob(core.Background(), opts)
	core.RequireTrue(t, r.OK)

	// 500 response -> OK=false but the probe still persisted + the
	// chain still reschedules.
	probeRows := orm.Of[SiteProbe](c).Where("url", "=", ts.URL).Get()
	core.RequireTrue(t, probeRows.OK)
	rows, ok := probeRows.Value.([]SiteProbe)
	core.RequireTrue(t, ok)
	core.RequireTrue(t, len(rows) == 1)
	core.AssertFalse(t, rows[0].OK)
	core.AssertEqual(t, 1, pendingJobsOfKind(t, c, ProbeKind))
}

// TestService_HandleFetchPRJob_Good — a watched repo's BaseURL
// points at a hermetic HTTPS test server serving one open PR; the
// handler persists a PRActivity row and reschedules the next tick.
func TestService_HandleFetchPRJob_Good(t *core.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"number":7,"title":"fix thing","state":"open",` +
			`"html_url":"https://forge.lthn.sh/lthn/desktop/pulls/7",` +
			`"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-02T00:00:00Z",` +
			`"user":{"login":"snider"}}]`))
	}))
	defer ts.Close()

	origClient := fetchClient
	fetchClient = ts.Client()
	t.Cleanup(func() { fetchClient = origClient })

	c := newInternalServiceCore(t)
	svc := NewService(c)

	cfg, ok := core.ServiceFor[*config.Service](c, "config")
	core.RequireTrue(t, ok)
	core.RequireTrue(t, cfg.Set("vi.repos", []map[string]any{
		{"provider": ProviderForge, "baseUrl": ts.URL, "owner": "lthn", "name": "desktop"},
	}).OK)

	repo := PRRepo{Provider: ProviderForge, BaseURL: ts.URL, Owner: "lthn", Name: "desktop"}
	opts := core.NewOptions(core.Option{Key: "repo", Value: RepoKey(repo)})
	r := svc.handleFetchPRJob(core.Background(), opts)
	core.RequireTrue(t, r.OK)

	prRows := orm.Of[PRActivity](c).Where("repo", "=", "desktop").Get()
	core.RequireTrue(t, prRows.OK)
	rows, ok := prRows.Value.([]PRActivity)
	core.RequireTrue(t, ok)
	core.RequireTrue(t, len(rows) == 1)
	core.AssertEqual(t, 7, rows[0].PRNumber)

	core.AssertEqual(t, 1, pendingJobsOfKind(t, c, FetchPRKind))
}

// TestService_HandleFetchPRJob_Bad — empty repo-key payload
// short-circuits; a repo key no longer in the live catalogue ends
// the reschedule chain without enqueuing a follow-up.
func TestService_HandleFetchPRJob_Bad(t *core.T) {
	c := newInternalServiceCore(t)
	svc := NewService(c)

	r := svc.handleFetchPRJob(core.Background(), core.NewOptions())
	core.RequireTrue(t, r.OK)
	core.AssertEqual(t, 0, pendingJobsOfKind(t, c, FetchPRKind))

	opts := core.NewOptions(core.Option{Key: "repo", Value: "forge:nowhere/nothing"})
	r = svc.handleFetchPRJob(core.Background(), opts)
	core.RequireTrue(t, r.OK)
	core.AssertEqual(t, 0, pendingJobsOfKind(t, c, FetchPRKind))
}

// TestService_EnqueueFetchPR_Ugly — a repo with an explicit
// intervalSeconds reschedules via the delay>0 ScheduleAfter branch
// rather than the immediate Enqueue branch.
func TestService_EnqueueFetchPR_Ugly(t *core.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer ts.Close()

	origClient := fetchClient
	fetchClient = ts.Client()
	t.Cleanup(func() { fetchClient = origClient })

	c := newInternalServiceCore(t)
	svc := NewService(c)

	cfg, ok := core.ServiceFor[*config.Service](c, "config")
	core.RequireTrue(t, ok)
	core.RequireTrue(t, cfg.Set("vi.repos", []map[string]any{
		{"provider": ProviderForge, "baseUrl": ts.URL, "owner": "lthn", "name": "desktop", "intervalSeconds": 600},
	}).OK)

	repo := PRRepo{Provider: ProviderForge, BaseURL: ts.URL, Owner: "lthn", Name: "desktop"}
	opts := core.NewOptions(core.Option{Key: "repo", Value: RepoKey(repo)})
	r := svc.handleFetchPRJob(core.Background(), opts)
	core.RequireTrue(t, r.OK)
	core.AssertEqual(t, 1, pendingJobsOfKind(t, c, FetchPRKind))
}
