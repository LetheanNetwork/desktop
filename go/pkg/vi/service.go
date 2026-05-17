// SPDX-Licence-Identifier: EUPL-1.2

// Service — Vi's Core integration. Owns the probe catalogue, hands
// the per-site work to pkg/queue (single-worker discipline preserved),
// and exposes the resolved Sites() data to the Wails frontend.
//
// Lifecycle:
//   - Register(c)            wires the service into the Core container
//   - OnStart()              registers the queue kind + seeds one Job
//                            per catalogued site (immediate fire);
//                            the handler self-reschedules at the
//                            per-site Interval so the cadence
//                            survives restarts cleanly.
//   - OnStop()               no-op; the queue Service drains workers
//                            on Core shutdown and any in-flight probe
//                            completes within ProbeTimeout (10s).
//
// The handler is registered via queue.RegisterKind under the kind
// "vi-probe-site" — the Payload carries {"url": "..."} which the
// handler resolves back to the live catalogue entry so an interval
// change in config takes effect on the next tick without a restart.

package vi

import (
	core "dappco.re/go"

	"dappco.re/lthn/desktop/pkg/auth"
	"dappco.re/lthn/desktop/pkg/queue"
)

// ProbeKind is the queue.RegisterKind name under which Vi's per-site
// probe handler registers. Exported so a future "vi probe now" CLI
// can enqueue ad-hoc work via queue.Enqueue(c, vi.ProbeKind, ...).
const ProbeKind = "vi-probe-site"

// FetchPRKind is the queue.RegisterKind name under which Vi's
// per-repo PR fetcher registers. Exported so a future
// "vi prs refresh" CLI can enqueue ad-hoc work via
// queue.Enqueue(c, vi.FetchPRKind, ...).
const FetchPRKind = "vi-fetch-prs"

// Service holds the Vi catalogue + Core reference. Per the canonical
// service pattern (Mantis #1336): a tiny struct with a *core.Core
// for late resolution of dependencies (config service, queue
// service, orm).
type Service struct {
	core *core.Core
}

// NewService constructs the Vi service against a Core container.
// Wired via core.WithName("vi", vi.Register) in cmd/lthn/app.go.
//
// Usage example:
//
//	svc := vi.NewService(c)
func NewService(c *core.Core) *Service {
	return &Service{core: c}
}

// Register constructs the Vi service for Core registration. Matches
// the queue / repos / marketplace registration shape.
//
// Usage example:
//
//	core.New(core.WithName("vi", vi.Register))
func Register(c *core.Core) core.Result {
	return core.Ok(NewService(c))
}

// ServiceName labels the binding namespace exposed to JS. The
// Wails-side `Vi.Sites()` / `Vi.Catalogue()` calls resolve under
// this name.
func (s *Service) ServiceName() string { return "Vi" }

// OnStart wires the probe handler into pkg/queue + seeds one Job
// per catalogued site for immediate execution. The handler
// self-reschedules at the per-site Interval after each probe, so
// after the seed wave the cadence is owned by the queue.
//
// Returns Ok even when the catalogue is empty — running with no
// sites is a valid state (the user can populate vi.sites config
// without restarting and the next call to Sites() simply returns
// empty until the first job lands).
//
// Skips wiring when pkg/queue isn't registered (CLI verbs that
// build a partial Core for non-GUI work). The probe handler
// becomes a no-op in that path and Sites() falls back to the
// empty list.
func (s *Service) OnStart() core.Result {
	if s == nil || s.core == nil {
		return core.Fail(core.E("vi.OnStart", "service or core is nil", nil))
	}
	c := s.core

	// Register the probe handler. The handler re-reads the live
	// catalogue each tick so an `intervalSeconds` bump in config
	// takes effect without a restart.
	register := queue.RegisterKind(c, queue.HandlerOptions{
		Kind:        ProbeKind,
		Description: "Vi: probe one configured site for uptime + latency",
		// Vi probe-jobs self-reschedule from within their own handler
		// (TierCascade once the worker stamps the dispatch Context) and
		// are seeded from OnStart under TierInternal. Operator covers
		// human-triggered immediate-probe via the CLI/Wails surface.
		PermittedTiers: []auth.CallerTier{auth.TierOperator, auth.TierCascade, auth.TierInternal},
		Handler: func(ctx core.Context, opts core.Options) core.Result {
			return s.handleProbeJob(ctx, opts)
		},
	})
	if !register.OK {
		// queue not registered yet — that's fine for CLI modes
		// that don't carry the GUI stack. Surface as Ok (with the
		// register error as Value for callers that care) rather
		// than failing service startup.
		return core.Ok(register.Error())
	}

	// Seed: one immediate-fire Job per catalogued site. Each
	// handler reschedules itself at its own interval, so we
	// don't need a separate ticker.
	for _, cat := range LoadCatalogue(c) {
		s.enqueue(cat.URL, 0)
	}

	// PR fetcher — same self-rescheduling shape against
	// FetchPRKind. Defaults to ~5min cadence so we sit well under
	// GitHub's rate ceiling even when watching a dozen repos. The
	// handler ignores register failures the same way the probe
	// handler does — CLI modes that don't run the queue stay
	// usable without PR data.
	prRegister := queue.RegisterKind(c, queue.HandlerOptions{
		Kind:        FetchPRKind,
		Description: "Vi: poll one configured repo for open pull requests",
		// Same posture as ProbeKind — self-rescheduling cadence under
		// TierCascade after the first OnStart-seeded TierInternal hop.
		PermittedTiers: []auth.CallerTier{auth.TierOperator, auth.TierCascade, auth.TierInternal},
		Handler: func(ctx core.Context, opts core.Options) core.Result {
			return s.handleFetchPRJob(ctx, opts)
		},
	})
	if prRegister.OK {
		for _, repo := range LoadPRRepos(c) {
			s.enqueueFetchPR(RepoKey(repo), 0)
		}
	}
	return core.Ok(nil)
}

// OnStop is a no-op — Core's ServiceShutdown drains pkg/queue's
// worker, and any in-flight probe completes within ProbeTimeout.
// We don't need to do anything here.
func (s *Service) OnStop() core.Result {
	return core.Ok(nil)
}

// handleProbeJob is the queue handler body — runs one Probe + then
// reschedules the next probe for the same URL. Always returns Ok so
// the queue marks the job Done; the probe outcome lives in the
// SiteProbe row, not the queue Job's status.
//
// Reschedule discipline: looks up the live catalogue entry by URL
// each tick. When the URL is no longer catalogued (user removed it
// via config), the handler completes WITHOUT enqueuing a follow-up
// — the recurring chain dies cleanly and the SiteProbe history for
// that domain becomes static.
func (s *Service) handleProbeJob(ctx core.Context, opts core.Options) core.Result {
	_ = ctx
	url := opts.String("url")
	if url == "" {
		return core.Ok("vi: no url in payload, skipping")
	}

	// Find the live catalogue entry — interval may have changed
	// since this Job was enqueued.
	var cat SiteCatalogue
	var found bool
	for _, candidate := range LoadCatalogue(s.core) {
		if candidate.URL == url {
			cat = candidate
			found = true
			break
		}
	}
	if !found {
		// URL was removed from config — let the recurring chain
		// die. Returning Ok marks the Job Done; no reschedule.
		return core.Ok("vi: url no longer catalogued, ending chain")
	}

	// Probe — outcome is persisted regardless of Ok/Fail.
	_ = Probe(s.core, cat)

	// Reschedule the next probe. The handler returns Ok before
	// the reschedule fails (which would be an orm-level failure
	// already reported via the Schedule Result) so the queue
	// doesn't try to retry this exact Job.
	interval := cat.Interval
	if interval <= 0 {
		interval = DefaultInterval
	}
	s.enqueue(cat.URL, interval)
	return core.Ok(nil)
}

// enqueue schedules a probe Job for the given URL. delay=0 means
// "as soon as the worker picks it up"; non-zero defers via
// queue.ScheduleAfter.
func (s *Service) enqueue(url string, delay core.Duration) {
	payload := core.NewOptions(core.Option{Key: "url", Value: url})
	if delay <= 0 {
		_ = queue.Enqueue(s.core, ProbeKind, payload)
		return
	}
	_ = queue.ScheduleAfter(s.core, delay, ProbeKind, payload)
}

// Catalogue is the Wails-bound accessor for the configured probe
// targets. Returns the resolved catalogue (config-overridden when
// `vi.sites` is set, defaults otherwise).
//
// Bound to JS as `Vi.Catalogue()` — frontend uses this to render the
// "watching N sites" hint + the per-site interval pill.
//
// Tier gate (RFC §5.1 / §10 Q-1 Phase 1 ALLOW): Require permits
// TierOperator + TierRenderer + TierCascade. Read-only, no secrets in
// output; renderer access is the load-bearing case so the Wails IPC
// entry (via *WailsService.Catalogue stamping TierRenderer) reaches
// here cleanly. nil-guard runs BEFORE the gate so callers passing a
// zero-value *Service still get the stable empty envelope shape.
func (s *Service) Catalogue() core.Result {
	if s == nil || s.core == nil {
		return core.Ok([]SiteCatalogue{})
	}
	id, ok := auth.Require(s.core, "vi.Service.Catalogue",
		auth.TierOperator, auth.TierRenderer, auth.TierCascade)
	if !ok {
		return core.Fail(core.E("vi.Service.Catalogue",
			"vi.tier_not_permitted: "+id.Tier.String(), nil))
	}
	return core.Ok(LoadCatalogue(s.core))
}

// Sites is the Wails-bound accessor for the latest probe results,
// shaped as SitesOutput. Returns one SiteStatus per catalogued
// domain — domains with no probe yet (just-added) are skipped, so
// the response only contains sites Vi has actually seen.
//
// Bound to JS as `Vi.Sites()` — frontend re-polls on its own cadence
// (the wails Events.Emit push from §"Future: real-time push" in
// RFC.vi.md is a separate Mantis follow-up).
func (s *Service) Sites() core.Result {
	if s == nil || s.core == nil {
		return core.Ok(SitesOutput{Sites: []SiteStatus{}})
	}
	id, ok := auth.Require(s.core, "vi.Service.Sites",
		auth.TierOperator, auth.TierRenderer, auth.TierCascade)
	if !ok {
		return core.Fail(core.E("vi.Service.Sites",
			"vi.tier_not_permitted: "+id.Tier.String(), nil))
	}
	catalogue := LoadCatalogue(s.core)
	probes := LatestByDomain(s.core, catalogue)

	// Build a domain → catalogue index so we surface the curated
	// Stack label alongside the probe data without re-walking the
	// catalogue per probe.
	catByDomain := make(map[string]SiteCatalogue, len(catalogue))
	for _, cat := range catalogue {
		catByDomain[DomainOf(cat.URL)] = cat
	}

	sites := make([]SiteStatus, 0, len(probes))
	for _, probe := range probes {
		cat := catByDomain[probe.Domain]
		status := "red"
		if probe.OK {
			status = "green"
		}
		sites = append(sites, SiteStatus{
			Domain:      probe.Domain,
			Stack:       cat.Stack,
			Status:      status,
			Response:    probe.LatencyMs,
			LastChecked: core.TimeFormat(probe.CheckedAt, core.TimeRFC3339),
			Err:         probe.LastError,
		})
	}
	return core.Ok(SitesOutput{
		Sites:   sites,
		Scanned: len(catalogue),
	})
}

// handleFetchPRJob is the queue handler for a single repo's PR
// poll. Mirrors handleProbeJob's shape: re-resolve the live repo
// catalogue (so interval / token changes take effect on next tick
// without restart), perform the fetch, reschedule the next tick.
// Always returns Ok so the queue marks the Job Done — fetch outcome
// lives in PRActivity rows + warn logs, not the queue Job status.
func (s *Service) handleFetchPRJob(ctx core.Context, opts core.Options) core.Result {
	_ = ctx
	key := opts.String("repo")
	if key == "" {
		return core.Ok("vi: no repo key in payload, skipping")
	}

	var repo PRRepo
	var found bool
	for _, candidate := range LoadPRRepos(s.core) {
		if RepoKey(candidate) == key {
			repo = candidate
			found = true
			break
		}
	}
	if !found {
		// Repo removed from config — let the recurring chain die.
		return core.Ok("vi: repo no longer catalogued, ending chain")
	}

	_ = Fetch(s.core, repo)

	interval := repo.Interval
	if interval <= 0 {
		interval = DefaultPRInterval
	}
	s.enqueueFetchPR(RepoKey(repo), interval)
	return core.Ok(nil)
}

// enqueueFetchPR schedules a PR-fetch Job for the given repo key.
// delay=0 means "as soon as the worker picks it up"; non-zero
// defers via queue.ScheduleAfter.
func (s *Service) enqueueFetchPR(repoKey string, delay core.Duration) {
	payload := core.NewOptions(core.Option{Key: "repo", Value: repoKey})
	if delay <= 0 {
		_ = queue.Enqueue(s.core, FetchPRKind, payload)
		return
	}
	_ = queue.ScheduleAfter(s.core, delay, FetchPRKind, payload)
}

// Repos is the Wails-bound accessor for the configured PR-watcher
// targets. Returns the resolved catalogue (config-overridden when
// `vi.repos` is set, defaults otherwise).
//
// Bound to JS as `Vi.Repos()` — frontend uses this to render the
// "watching N repos" hint + per-repo cadence pill.
func (s *Service) Repos() core.Result {
	if s == nil || s.core == nil {
		return core.Ok([]PRRepo{})
	}
	id, ok := auth.Require(s.core, "vi.Service.Repos",
		auth.TierOperator, auth.TierRenderer, auth.TierCascade)
	if !ok {
		return core.Fail(core.E("vi.Service.Repos",
			"vi.tier_not_permitted: "+id.Tier.String(), nil))
	}
	return core.Ok(LoadPRRepos(s.core))
}

// Activity is the Wails-bound accessor for the latest PR activity,
// shaped as ActivityOutput. Returns one ActivityEntry per
// (provider, owner, repo, pr_number) that is currently open,
// ordered newest-first by upstream UpdatedAt.
//
// Bound to JS as `Vi.Activity()` — frontend re-polls on its own
// cadence; real-time push is the §"Future: real-time push"
// follow-up tracked in RFC.vi.md.
func (s *Service) Activity() core.Result {
	if s == nil || s.core == nil {
		return core.Ok(ActivityOutput{Items: []ActivityEntry{}})
	}
	id, ok := auth.Require(s.core, "vi.Service.Activity",
		auth.TierOperator, auth.TierRenderer, auth.TierCascade)
	if !ok {
		return core.Fail(core.E("vi.Service.Activity",
			"vi.tier_not_permitted: "+id.Tier.String(), nil))
	}
	catalogue := LoadPRRepos(s.core)
	rows := LatestPerPR(s.core, catalogue, DefaultActivityLimit)

	items := make([]ActivityEntry, 0, len(rows))
	for _, row := range rows {
		items = append(items, ActivityEntry{
			Kind:      "pr",
			Provider:  row.Provider,
			Repo:      row.Owner + "/" + row.Repo,
			Number:    row.PRNumber,
			Title:     row.Title,
			Author:    row.Author,
			State:     row.State,
			URL:       row.URL,
			OpenedAt:  core.TimeFormat(row.OpenedAt, core.TimeRFC3339),
			UpdatedAt: core.TimeFormat(row.UpdatedAt, core.TimeRFC3339),
			CheckedAt: core.TimeFormat(row.CheckedAt, core.TimeRFC3339),
		})
	}
	return core.Ok(ActivityOutput{
		Items:   items,
		Scanned: len(catalogue),
	})
}
