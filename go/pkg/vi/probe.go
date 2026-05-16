// SPDX-Licence-Identifier: EUPL-1.2

// Probe — one-shot HTTPS health check. Hits the configured URL with
// a bounded timeout, records latency + status, and persists the
// outcome as a SiteProbe row.
//
// Why GET and not HEAD: many sites (CDNs, reverse proxies in front
// of Laravel/Forgejo/Mattermost) handle GET cleanly but return 405
// or undefined behaviour on HEAD. A small-body GET is the safest
// universal probe; the body is read + discarded with LimitReader so
// a hostile / runaway upstream can't blow our memory.
//
// Why httpsOnlyClient (mirrors marketplace.registry.go): the
// probe set is curated, never user-typed at the probe-fire path,
// but the redirect chain might still land on plain http. Refusing
// the redirect mirrors the Cerberus #1433 discipline applied to
// marketplace registry fetches.

package vi

import (
	core "dappco.re/go"
	"dappco.re/go/orm"
)

// ProbeTimeout is the per-request ceiling. 10 seconds aligns with
// "bandwith of a coffee-shop wifi after a deploy went sideways"
// without pinning the worker goroutine indefinitely. A site that
// takes longer than 10s is recorded as failed (OK=false, Err set).
const ProbeTimeout = 10 * core.Second

// MaxBodyBytes is the cap on the response body we'll actually read.
// We discard the body — only status + latency matter for the
// uptime signal — but the body must drain to allow connection
// reuse. 4 KiB is enough for any sensible 200 OK landing page
// header chunk; truncation past that is fine.
const MaxBodyBytes = 4 * 1024

// MaxErrorBytes truncates the recorded LastError string so a
// pathological transport error (e.g. wrapped TLS chain dump) can't
// bloat the row.
const MaxErrorBytes = 512

// probeClient is the shared HTTP client — single instance so
// connection pools survive across probe ticks. CheckRedirect
// refuses any downgrade to http; we cap at 5 redirects to dodge
// loops without legitimately blocking redirect-heavy hosts.
var probeClient = &core.HTTPClient{
	Timeout: ProbeTimeout,
	CheckRedirect: func(req *core.Request, via []*core.Request) error {
		if len(via) >= 5 {
			return core.NewError("vi: stopped after 5 redirects")
		}
		if req.URL.Scheme != "https" {
			return core.NewError("vi: refusing non-https redirect to " + req.URL.String())
		}
		return nil
	},
}

// Probe runs a single HTTPS GET against `cat.URL`, persists the
// outcome as a SiteProbe row, and returns the persisted record.
// Always returns a row — failed probes are part of the signal, not
// an error condition; the only Result.Fail path is "could not
// persist", which is genuinely broken (orm down / disk full).
//
// Usage example:
//
//	r := vi.Probe(c, vi.SiteCatalogue{URL: "https://lthn.ai"})
//	if r.OK { probe := r.Value.(vi.SiteProbe); _ = probe.OK }
func Probe(c *core.Core, cat SiteCatalogue) core.Result {
	if c == nil {
		return core.Fail(core.E("vi.Probe", "core is nil", nil))
	}
	if !core.HasPrefix(cat.URL, "https://") {
		return core.Fail(core.E("vi.Probe", "url must start with https://", nil))
	}

	domain := DomainOf(cat.URL)
	start := core.Now().UTC()

	statusCode, lastErr := doProbe(cat.URL)
	checked := core.Now().UTC()
	latencyMs := int(checked.Sub(start) / core.Millisecond)

	probe := SiteProbe{
		ID:         newProbeID(),
		Domain:     domain,
		URL:        cat.URL,
		StatusCode: statusCode,
		LatencyMs:  latencyMs,
		OK:         statusCode >= 200 && statusCode < 400,
		LastError:  truncate(lastErr, MaxErrorBytes),
		CheckedAt:  checked,
	}
	if probe.OK {
		probe.LastError = ""
	}
	if r := orm.Insert(c, &probe); !r.OK {
		return r
	}
	return core.Ok(probe)
}

// doProbe performs the HTTPS GET and returns (statusCode, errMsg).
// Transport errors return (0, msg); successful responses (including
// 4xx / 5xx) return (status, "") — the caller derives OK from the
// status range, not from the error field.
func doProbe(url string) (int, string) {
	reqR := core.NewHTTPRequest(core.MethodGet, url, nil)
	if !reqR.OK {
		return 0, "build request: " + reqR.Error()
	}
	req := reqR.Value.(*core.Request)

	resp, err := probeClient.Do(req)
	if err != nil {
		if resp != nil {
			_ = resp.Body.Close()
		}
		return 0, err.Error()
	}
	defer resp.Body.Close()

	// Drain (bounded) so the connection can return to the pool.
	bounded := core.LimitReader(resp.Body, MaxBodyBytes)
	_ = core.ReadAll(bounded)

	return resp.StatusCode, ""
}

// LatestByDomain returns the newest SiteProbe row for each domain
// in the supplied catalogue, in catalogue order. Domains with no
// rows yet are skipped (they'll appear on the next call after the
// first probe fires).
//
// Usage example:
//
//	probes := vi.LatestByDomain(c, catalogue)
//	for _, p := range probes { _ = p.OK }
func LatestByDomain(c *core.Core, catalogue []SiteCatalogue) []SiteProbe {
	if c == nil || len(catalogue) == 0 {
		return nil
	}
	out := make([]SiteProbe, 0, len(catalogue))
	for _, cat := range catalogue {
		domain := DomainOf(cat.URL)
		r := orm.Of[SiteProbe](c).
			Where("domain", "=", domain).
			Order("checked_at", "desc").
			Limit(1).
			Get()
		if !r.OK {
			continue
		}
		rows, ok := r.Value.([]SiteProbe)
		if !ok || len(rows) == 0 {
			continue
		}
		out = append(out, rows[0])
	}
	return out
}

// newProbeID generates the SiteProbe primary key — "p-" prefix for
// grep-friendliness in logs, mirrors pkg/queue's "j-" convention.
// Falls back to nanosecond timestamp on the rare crypto/rand
// failure path so probes never fail to persist for ID reasons.
func newProbeID() string {
	r := core.RandomString(12)
	if !r.OK {
		return core.Sprintf("p-%d", core.Now().UnixNano())
	}
	return "p-" + r.Value.(string)
}

// truncate keeps a string at or below max bytes. Used to cap the
// LastError field so a runaway error chain can't bloat a row.
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}
