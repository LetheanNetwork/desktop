// SPDX-Licence-Identifier: EUPL-1.2

// Fetch — one-shot poll of a single watched repo for open pull
// requests. Hits the provider's REST API with a bounded timeout,
// parses the JSON response, and persists one PRActivity row per
// open PR.
//
// Why GET-list and not webhooks: webhooks need a public ingress
// point (lthn-desktop is local-first; no inbound port). Polling at
// 5min/repo handles ~10 watched repos under GitHub's 5000 req/hr
// authenticated cap with vast headroom. Webhook upgrade is a
// future arc once the marketplace serves a public-bridge bundle.
//
// Why httpsOnlyClient (mirrors marketplace.registry.go + pkg/vi/
// probe.go): the Forge BaseURL is config-supplied and might redirect.
// Refusing http downgrades aligns with Cerberus #1433.
//
// Body cap (256 KiB): matches the marketplace manifest cap shape.
// A page of 30 PRs from Forgejo/GitHub is comfortably <50 KiB; a
// pathological response gets refused rather than blowing memory.

package vi

import (
	core "dappco.re/go"
	"dappco.re/go/orm"
)

// FetchTimeout is the per-request ceiling. 15 seconds tolerates
// slow upstream responses (forge.lthn.sh under load, GitHub during
// incidents) without pinning the worker indefinitely.
const FetchTimeout = 15 * core.Second

// MaxPRBodyBytes is the response-body cap. 256 KiB matches
// marketplace's manifest cap shape — a page of 30 PRs is well
// under 50 KiB so this is purely a runaway guard.
const MaxPRBodyBytes = 256 * 1024

// MaxPRPageSize caps how many PRs we ask for per page. 50 is
// well over any repo we sensibly watch + under both providers'
// page-size ceilings (Forgejo: 50 default, GitHub: 100 max).
const MaxPRPageSize = 50

// MaxRedirects bounds the redirect chain — same convention as
// pkg/marketplace.maxRegistryRedirects (5).
const MaxRedirects = 5

// fetchClient is the shared HTTP client — single instance so
// connection pools survive across fetch ticks. CheckRedirect
// refuses any downgrade to http; chain capped at MaxRedirects.
var fetchClient = &core.HTTPClient{
	Timeout: FetchTimeout,
	CheckRedirect: func(req *core.Request, via []*core.Request) error {
		if len(via) >= MaxRedirects {
			return core.NewError("vi: stopped after 5 PR-fetch redirects")
		}
		if req.URL.Scheme != "https" {
			return core.NewError("vi: refusing non-https redirect to " + req.URL.String())
		}
		return nil
	},
}

// upstreamPR mirrors the subset of the upstream JSON we read.
// Forgejo + GitHub share the same field names + shape for the
// open-PR list — `number`, `title`, `state`, `html_url`,
// `created_at`, `updated_at`, and a nested `user` with a `login`
// string. The two backends diverge in many other fields, but the
// surface we read is uniform.
type upstreamPR struct {
	Number    int        `json:"number"`
	Title     string     `json:"title"`
	State     string     `json:"state"`
	HTMLURL   string     `json:"html_url"`
	CreatedAt string     `json:"created_at"`
	UpdatedAt string     `json:"updated_at"`
	User      upstreamPU `json:"user"`
}

// upstreamPU is the nested `user` object — only the `login` field
// matters for activity rendering.
type upstreamPU struct {
	Login string `json:"login"`
}

// Fetch polls one watched repo for its open PRs and persists each
// one as a PRActivity row. Returns the number of rows inserted on
// success; an error Result on transport / parse / persist failure.
//
// Empty token is fine for both providers — public repos respond
// to anonymous GETs, just under tighter rate limits. 401/403
// responses log a warn + return Ok(0) so a single misconfigured
// repo doesn't tank the polling chain.
//
// Usage example:
//
//	r := vi.Fetch(c, vi.PRRepo{Provider: "forge", Owner: "lthn", Name: "desktop"})
//	if r.OK { count, _ := r.Value.(int); _ = count }
func Fetch(c *core.Core, repo PRRepo) core.Result {
	if c == nil {
		return core.Fail(core.E("vi.Fetch", "core is nil", nil))
	}
	if repo.Owner == "" || repo.Name == "" {
		return core.Fail(core.E("vi.Fetch", "repo owner+name required", nil))
	}
	url := buildListURL(repo)
	if url == "" {
		return core.Fail(core.E("vi.Fetch", "unsupported provider: "+repo.Provider, nil))
	}

	token := LoadToken(c, repo.Provider)
	emitPRTokenLoaded(repo.Provider, token != "")
	emitPRFetchRequested(repo, token != "")

	prs, status, err := doFetch(url, repo.Provider, token)
	if err != "" {
		core.Warn("vi: PR fetch failed",
			"repo", RepoKey(repo),
			"status", status,
			"err", err)
		emitPRFetchFailed(repo, status,
			core.Fail(core.E("vi.Fetch", "doFetch transport error", nil)))
		return core.Ok(0)
	}
	// 401 / 403 — token missing or wrong scope. Don't crash; the
	// next tick re-reads config so a fix takes effect without a
	// restart. Log the masked token state so operators can debug.
	if status == 401 || status == 403 {
		core.Warn("vi: PR fetch unauthorised",
			"repo", RepoKey(repo),
			"status", status,
			"tokenSet", token != "")
		return core.Ok(0)
	}
	if status >= 400 {
		core.Warn("vi: PR fetch HTTP error",
			"repo", RepoKey(repo),
			"status", status)
		return core.Ok(0)
	}

	checked := core.Now().UTC()
	inserted := 0
	for _, pr := range prs {
		// Cerberus pass-6 HIGH — refuse non-https HTMLURL at fetch-
		// time. A compromised or malicious upstream forge can return
		// `javascript:fetch(...)` in html_url; that URL gets persisted,
		// rendered as an Angular href, and one click runs JS in the same
		// origin as every Wails binding (full RCE on the host).
		// Skip rows whose URL isn't https:// rather than store-and-
		// pray. The frontend will also gate at render-time (belt +
		// braces).
		if !isSafePRURL(pr.HTMLURL) {
			core.Warn("vi: dropping PR row with unsafe URL",
				"repo", RepoKey(repo),
				"pr", pr.Number,
				"url", pr.HTMLURL)
			emitPRFetchRejected(repo, pr.Number, rejectReasonUnsafeURL)
			continue
		}
		row := PRActivity{
			ID:        newPRActivityID(),
			Provider:  repo.Provider,
			Owner:     repo.Owner,
			Repo:      repo.Name,
			PRNumber:  pr.Number,
			Title:     pr.Title,
			Author:    pr.User.Login,
			State:     pr.State,
			OpenedAt:  parseTime(pr.CreatedAt),
			UpdatedAt: parseTime(pr.UpdatedAt),
			URL:       pr.HTMLURL,
			CheckedAt: checked,
		}
		if r := orm.Insert(c, &row); !r.OK {
			// Persist failure on one PR shouldn't halt the rest —
			// continue with a warn so a row-level orm hiccup
			// doesn't block the whole tick.
			core.Warn("vi: PR row insert failed",
				"repo", RepoKey(repo),
				"pr", pr.Number,
				"err", r.Error())
			continue
		}
		inserted++
	}
	emitPRFetchSucceeded(repo, status, inserted)
	return core.Ok(inserted)
}

// isSafePRURL gates the upstream-supplied html_url before persistence.
// Cerberus pass-6 — a compromised or malicious upstream forge can
// return `javascript:fetch(...)` in html_url, which renders as an Angular
// `<a href=...>` in the activity window and one click runs JS in
// the same origin as every Wails binding (full RCE).
//
// Today only https:// is acceptable. http:// is rejected too because
// (a) the rest of the marketplace + downloader hardening (#1424,
// #1433) is https-only, (b) anything we link from the Activity slot
// should be transport-secured by the time the user can click it.
func isSafePRURL(u string) bool {
	return core.HasPrefix(u, "https://")
}

// buildListURL composes the open-PR list URL for the given repo.
// Both Forgejo + GitHub speak the same shape for this endpoint,
// only the path prefix differs.
//
//	Forgejo: <baseURL>/api/v1/repos/{owner}/{repo}/pulls?state=open
//	GitHub:  https://api.github.com/repos/{owner}/{repo}/pulls?state=open
func buildListURL(repo PRRepo) string {
	base := repo.BaseURL
	switch repo.Provider {
	case ProviderForge:
		if base == "" {
			base = defaultForgeBaseURL
		}
		return base + "/api/v1/repos/" + repo.Owner + "/" + repo.Name +
			"/pulls?state=open&limit=" + core.Sprintf("%d", MaxPRPageSize)
	case ProviderGitHub:
		// GitHub's pulls endpoint accepts `per_page` not `limit`.
		return defaultGitHubBaseURL + "/repos/" + repo.Owner + "/" + repo.Name +
			"/pulls?state=open&per_page=" + core.Sprintf("%d", MaxPRPageSize)
	default:
		return ""
	}
}

// doFetch performs the HTTPS GET, applies the auth header, reads
// the bounded body, and unmarshals into []upstreamPR.
//
// Returns (prs, statusCode, errMsg). On transport error errMsg is
// non-empty and the caller logs + skips the tick. On HTTP error
// statusCode >= 400 the caller decides whether to surface as
// warn/skip or hard-error.
func doFetch(url, provider, token string) ([]upstreamPR, int, string) {
	if !core.HasPrefix(url, "https://") {
		return nil, 0, "url must start with https://"
	}
	reqR := core.NewHTTPRequest(core.MethodGet, url, nil)
	if !reqR.OK {
		return nil, 0, "build request: " + reqR.Error()
	}
	req := reqR.Value.(*core.Request)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "lthn-desktop-vi/1 (+https://lthn.ai)")

	// Auth: both providers accept "Authorization: token <pat>" for
	// personal access tokens — Forgejo standard + GitHub's
	// classic-token shape. GitHub also accepts "Bearer" for
	// fine-grained PATs; "token" works for both classic and
	// fine-grained so it's the safer universal.
	if token != "" {
		req.Header.Set("Authorization", "token "+token)
	}
	// GitHub recommends a versioned Accept header — pin to the
	// 2022-11-28 REST API version so a future GitHub upgrade
	// doesn't shift the response shape under us.
	if provider == ProviderGitHub {
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	}

	resp, err := fetchClient.Do(req)
	if err != nil {
		if resp != nil {
			_ = resp.Body.Close()
		}
		return nil, 0, err.Error()
	}
	defer resp.Body.Close()

	// 4xx / 5xx — surface status to caller, don't try to parse a
	// likely-not-JSON error body.
	if resp.StatusCode >= 400 {
		bounded := core.LimitReader(resp.Body, MaxPRBodyBytes)
		_ = core.ReadAll(bounded)
		return nil, resp.StatusCode, ""
	}
	if resp.ContentLength > int64(MaxPRBodyBytes) {
		return nil, resp.StatusCode, core.Sprintf(
			"Content-Length %d exceeds cap %d", resp.ContentLength, MaxPRBodyBytes)
	}

	bounded := core.LimitReader(resp.Body, int64(MaxPRBodyBytes)+1)
	readR := core.ReadAll(bounded)
	if !readR.OK {
		return nil, resp.StatusCode, "response read failed"
	}
	// core.ReadAll's Result.Value is `string` (per pkg/external/go/io.go),
	// NOT []byte — the legacy `Value.([]byte)` cast silently returned nil
	// here, so the cap-check below always saw len=0 and the subsequent
	// JSONUnmarshal silently parsed an empty payload as `null`, masking
	// real upstream payloads. Wave 1 ReadAll sweep — fix the cast.
	rawStr, _ := readR.Value.(string)
	raw := []byte(rawStr)
	if int64(len(raw)) > int64(MaxPRBodyBytes) {
		return nil, resp.StatusCode, core.Sprintf(
			"response body exceeded %d byte cap", MaxPRBodyBytes)
	}

	var prs []upstreamPR
	if r := core.JSONUnmarshal(raw, &prs); !r.OK {
		return nil, resp.StatusCode, "JSON parse failed: " + r.Error()
	}
	return prs, resp.StatusCode, ""
}

// LatestPerPR returns the newest PRActivity row per
// (provider, owner, repo, pr_number) for the supplied catalogue,
// ordered by UpdatedAt descending (most-recently-touched first).
// Closed/merged PRs are filtered out — only "open" rows surface.
//
// Bounded at `limit` rows so a runaway PR-heavy repo doesn't dump
// hundreds of rows on the Wails surface. limit <= 0 falls back to
// DefaultActivityLimit.
//
// Usage example:
//
//	rows := vi.LatestPerPR(c, catalogue, 50)
//	for _, row := range rows { _ = row.Title }
func LatestPerPR(c *core.Core, catalogue []PRRepo, limit int) []PRActivity {
	if c == nil || len(catalogue) == 0 {
		return nil
	}
	if limit <= 0 {
		limit = DefaultActivityLimit
	}

	// Build a (provider, owner, repo) → bool filter so the in-loop
	// per-(repo, pr_number) rollup only scans the user's watched
	// set rather than every row in the table.
	watched := make(map[string]bool, len(catalogue))
	for _, r := range catalogue {
		watched[r.Provider+":"+r.Owner+"/"+r.Name] = true
	}

	// Pull recent rows ordered newest first; the latest-per-PR
	// dedup happens in-loop so older entries for the same PR drop
	// off naturally. We over-fetch (10x the limit) because dedup
	// can collapse many rows to one and we want at least `limit`
	// distinct PRs out the other side.
	r := orm.Of[PRActivity](c).
		Order("checked_at", "desc").
		Limit(limit * 10).
		Get()
	if !r.OK {
		return nil
	}
	rows, ok := r.Value.([]PRActivity)
	if !ok || len(rows) == 0 {
		return nil
	}

	seen := make(map[string]bool, limit)
	out := make([]PRActivity, 0, limit)
	for _, row := range rows {
		key := row.Provider + ":" + row.Owner + "/" + row.Repo
		if !watched[key] {
			continue
		}
		if row.State != "" && row.State != "open" {
			continue
		}
		prKey := key + "#" + core.Sprintf("%d", row.PRNumber)
		if seen[prKey] {
			continue
		}
		seen[prKey] = true
		out = append(out, row)
		if len(out) >= limit {
			break
		}
	}
	return out
}

// DefaultActivityLimit caps how many rows Activity() returns when
// the caller doesn't override. 50 is enough to render a
// scroll-anywhere panel without flooding the frontend with PRs
// from a single firehose repo.
const DefaultActivityLimit = 50

// parseTime turns an ISO-8601 / RFC3339 string into a core.Time,
// returning the zero time on parse failure. Both Forgejo + GitHub
// emit RFC3339; the fallback keeps the row inserted even if
// upstream returns a malformed timestamp.
func parseTime(s string) core.Time {
	if s == "" {
		return core.Time{}
	}
	r := core.TimeParse(core.TimeRFC3339, s)
	if !r.OK {
		return core.Time{}
	}
	t, ok := r.Value.(core.Time)
	if !ok {
		return core.Time{}
	}
	return t
}

// newPRActivityID generates the PRActivity primary key. "pra-"
// prefix mirrors pkg/queue's "j-" + pkg/vi probe's "p-"
// grep-friendly convention. Falls back to nanosecond timestamp on
// crypto/rand failure so inserts never abort for ID reasons.
func newPRActivityID() string {
	r := core.RandomString(12)
	if !r.OK {
		return core.Sprintf("pra-%d", core.Now().UnixNano())
	}
	return "pra-" + r.Value.(string)
}
