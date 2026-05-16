// SPDX-Licence-Identifier: EUPL-1.2

// Registry fetch + local cache for the lthn-vm bundle catalogue.
//
// FetchIndex downloads marketplace.lthn.ai/v1/index.json (or the configured
// override URL), caches the result to ~/Lethean/conf/marketplace/index.json,
// and returns the parsed catalogue. Subsequent calls within the TTL window
// return the cached copy.
//
// FetchManifest downloads and parses one bundle manifest by its source URL.
// https:// source URLs are supported in v1; oci:// and git+https:// return
// a "not supported in v1" error and are forward-arc placeholders.
//
// Spec: plans/project/lthn/desktop/RFC.marketplace.md §3.
package marketplace

import (
	core "dappco.re/go"
)

const (
	fetchIndexOp    = "marketplace.FetchIndex"
	fetchManifestOp = "marketplace.FetchManifest"

	// defaultIndexURL is the curated catalogue endpoint.
	defaultIndexURL = "https://marketplace.lthn.ai/v1/index.json"

	// indexCacheTTL is how long a locally cached index is considered fresh.
	// Matches the "every 24h" cadence from the spec.
	indexCacheTTL = 24 * core.Hour

	// indexCacheFileName is the local file name for the cached index.
	indexCacheFileName = "index.json"

	// Cerberus Mantis #1433 — size caps on the two registry fetch
	// surfaces. Without these, a malicious mirror could stream
	// unbounded bytes regardless of Content-Length, exhausting RAM
	// (ReadAll into memory; the catalogue path doesn't stream to
	// disk). Cap shape mirrors the downloader hardening (#1425).
	//
	// 4 MiB for the index — the curated catalogue today has dozens
	// of entries; thousands of entries would still fit in 4 MiB at
	// JSON-density. 256 KiB for a single manifest — a manifest is
	// metadata + envrefs + image refs, never large.
	maxIndexBytes    = 4 << 20  // 4 MiB
	maxManifestBytes = 256 << 10 // 256 KiB

	// maxRegistryRedirects matches the downloader's explicit-cap
	// pattern. Default Go client is 10; we match for auditability.
	maxRegistryRedirects = 10
)

// httpsOnlyClient is the registry's HTTP client. Cerberus Mantis
// #1433 — three guarantees:
//
//   - CheckRedirect rejects any non-https redirect target. The plain
//     CheckRedirect+AllowedSource shape from the downloader doesn't
//     directly apply (the marketplace registry isn't host-allowlisted
//     today), so we enforce protocol instead — the original URL must
//     also be https, and every hop must stay https.
//   - Redirect chain capped at maxRegistryRedirects.
//   - The default transport's TLS settings apply — no MitM-friendly
//     InsecureSkipVerify anywhere in this package.
//
// Pinning specific CAs / fingerprints is Mantis #1428 (TLS pinning
// deferred to marketplace manifest), tracked separately.
var httpsOnlyClient = &core.HTTPClient{
	CheckRedirect: func(req *core.Request, via []*core.Request) error {
		if len(via) >= maxRegistryRedirects {
			return core.NewError("marketplace: stopped after " +
				core.Sprintf("%d", maxRegistryRedirects) + " redirects")
		}
		if req.URL.Scheme != "https" {
			return core.NewError("marketplace: refusing non-https redirect to " +
				req.URL.String())
		}
		return nil
	},
}

// requireHTTPS rejects URLs whose scheme isn't https. Cerberus
// Mantis #1433 — the original FetchManifest accepted both http:// and
// https://, opening a downgrade vector for any catalogue that
// declared http:// source URLs (catalogue itself is fetched over
// https today, but per-entry source URLs are caller-supplied).
func requireHTTPS(op, rawURL string) core.Result {
	if !core.HasPrefix(rawURL, "https://") {
		return core.Fail(core.E(op,
			"only https:// URLs accepted (refusing "+rawURL+")", nil))
	}
	return core.Ok(nil)
}

// fetchCapped performs a GET via httpsOnlyClient and ReadAll's the
// body bounded at maxBytes. Returns (raw bytes, response status,
// Result.OK). A response that exceeds the cap fails with a clear
// message rather than truncating silently.
//
// Pattern mirrors pkg/downloader's two-stage gate: Content-Length
// pre-check + LimitReader at cap+1 with post-read overflow check.
func fetchCapped(op, url string, maxBytes int64) (raw []byte, status int, result core.Result) {
	reqR := core.NewHTTPRequest("GET", url, nil)
	if !reqR.OK {
		return nil, 0, reqR
	}
	req := reqR.Value.(*core.Request)
	resp, err := httpsOnlyClient.Do(req)
	if err != nil {
		if resp != nil {
			_ = resp.Body.Close()
		}
		return nil, 0, core.Fail(core.E(op, "GET failed: "+url, err))
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, resp.StatusCode, core.Fail(core.E(op,
			core.Sprintf("HTTP %d from %s", resp.StatusCode, url), nil))
	}
	if resp.ContentLength > maxBytes {
		return nil, resp.StatusCode, core.Fail(core.E(op,
			core.Sprintf("Content-Length %d exceeds cap %d",
				resp.ContentLength, maxBytes), nil))
	}

	bounded := core.LimitReader(resp.Body, maxBytes+1)
	readR := core.ReadAll(bounded)
	if !readR.OK {
		return nil, resp.StatusCode, core.Fail(core.E(op, "response read failed", nil))
	}
	raw, _ = readR.Value.([]byte)
	if int64(len(raw)) > maxBytes {
		return nil, resp.StatusCode, core.Fail(core.E(op,
			core.Sprintf("response body exceeded %d byte cap", maxBytes), nil))
	}
	return raw, resp.StatusCode, core.Ok(nil)
}

// CatalogueEntry is one record in the marketplace index.
// Matches the shape at marketplace.lthn.ai/v1/index.json.
type CatalogueEntry struct {
	// Name is the stable identifier (must be lowercase alphanumeric + dash).
	Name string `json:"name"`
	// Display is the user-facing label.
	Display string `json:"display"`
	// Description is the short one-liner shown on the catalogue card.
	Description string `json:"description"`
	// Category drives navigation grouping (e.g. "ai-agents", "databases").
	Category string `json:"category"`
	// Icon is the Font Awesome class string (e.g. "fa-robot").
	Icon string `json:"icon,omitempty"`
	// Homepage is the upstream project URL shown on the detail view.
	Homepage string `json:"homepage,omitempty"`
	// License is the SPDX identifier (e.g. "MIT", "GPL-2.0").
	License string `json:"license,omitempty"`
	// SourceURL is where the per-bundle manifest is fetched from.
	// Supported protocols: "https://", "oci://", "git+https://".
	SourceURL string `json:"source_url"`
	// RequiresVersion is the minimum lthn-desktop version needed.
	// Empty means any version.
	RequiresVersion string `json:"requires_version,omitempty"`
	// LastUpdated is the ISO-8601 date the registry entry was last changed.
	LastUpdated string `json:"last_updated,omitempty"`
}

// FetchIndexResult is the parsed catalogue returned by FetchIndex.
type FetchIndexResult struct {
	// Entries is the full catalogue list.
	Entries []CatalogueEntry `json:"entries"`
	// CachedAt is when the local cache was written. Zero if fetched live.
	CachedAt core.Time `json:"cached_at"`
	// FromCache is true when the result came from the local cache.
	FromCache bool `json:"from_cache"`
}

// FetchIndex downloads (or reads from cache) the marketplace catalogue.
// indexURL overrides the default endpoint; pass "" to use the default.
//
// Usage example:
//
//	r := svc.FetchIndex("")
//	if r.OK { res := r.Value.(marketplace.FetchIndexResult) }
func (s *Service) FetchIndex(indexURL string) core.Result {
	if core.Trim(indexURL) == "" {
		indexURL = defaultIndexURL
	}

	cachePath := s.indexCachePath()
	if r := s.readIndexCache(cachePath); r.OK {
		return r
	}

	return s.downloadIndex(indexURL, cachePath)
}

// FetchManifest fetches and parses a bundle manifest from the given source URL.
// https:// URLs are supported in v1. oci:// and git+https:// are forward-arc
// placeholders that return a clear "not supported in v1" error.
//
// Usage example:
//
//	r := svc.FetchManifest("https://marketplace.lthn.ai/v1/opencode.yml")
//	if r.OK { m := r.Value.(marketplace.BundleManifest) }
func (s *Service) FetchManifest(sourceURL string) core.Result {
	if core.Trim(sourceURL) == "" {
		return core.Fail(core.E(fetchManifestOp, "source url is required", nil))
	}

	switch {
	case core.HasPrefix(sourceURL, "https://"):
		return s.fetchManifestHTTPS(sourceURL)
	case core.HasPrefix(sourceURL, "http://"):
		// Cerberus #1433 — http:// is a downgrade vector; reject
		// loudly rather than silently upgrading.
		return core.Fail(core.E(fetchManifestOp,
			"refusing plaintext http:// source URL (use https://): "+sourceURL, nil))
	case core.HasPrefix(sourceURL, "oci://"):
		return core.Fail(core.E(fetchManifestOp,
			"oci:// source URLs are not supported in v1 — use https:// instead", nil))
	case core.HasPrefix(sourceURL, "git+https://"):
		return core.Fail(core.E(fetchManifestOp,
			"git+https:// source URLs are not supported in v1 — use https:// instead", nil))
	default:
		return core.Fail(core.E(fetchManifestOp,
			"unsupported source URL protocol: "+sourceURL, nil))
	}
}

// SearchCatalogue filters a FetchIndexResult by query + category.
// Mirrors the in-memory search function for the live catalogue.
//
// Usage example:
//
//	r := svc.FetchIndex("")
//	results := marketplace.SearchCatalogue(r.Value.(marketplace.FetchIndexResult), "ollama", "")
func SearchCatalogue(index FetchIndexResult, query, category string) []CatalogueEntry {
	queryLower := core.Lower(core.Trim(query))
	categoryLower := core.Lower(core.Trim(category))
	out := make([]CatalogueEntry, 0, len(index.Entries))
	for _, e := range index.Entries {
		if categoryLower != "" && core.Lower(e.Category) != categoryLower {
			continue
		}
		if queryLower != "" {
			hay := core.Lower(e.Name + " " + e.Display + " " + e.Category + " " + e.Description)
			if !core.Contains(hay, queryLower) {
				continue
			}
		}
		out = append(out, e)
	}
	return out
}

// indexCachePath returns the local path for the cached index file.
func (s *Service) indexCachePath() string {
	homeR := core.UserHomeDir()
	if homeR.OK {
		return core.PathJoin(homeR.Value.(string), "Lethean", "conf", "marketplace", indexCacheFileName)
	}
	return core.PathJoin("/tmp", "lthn-marketplace", indexCacheFileName)
}

// readIndexCache returns the cached index if the cache file exists and is
// younger than indexCacheTTL. Returns a Fail result when the cache is absent
// or stale — caller should downloadIndex instead.
func (s *Service) readIndexCache(cachePath string) core.Result {
	statR := core.Stat(cachePath)
	if !statR.OK {
		return core.Fail(core.E(fetchIndexOp, "cache miss: file absent", nil))
	}
	info := statR.Value.(core.FsFileInfo)
	age := core.Since(info.ModTime())
	if age > indexCacheTTL {
		return core.Fail(core.E(fetchIndexOp, "cache stale", nil))
	}

	readR := core.ReadFile(cachePath)
	if !readR.OK {
		return core.Fail(core.E(fetchIndexOp, "cache read failed", nil))
	}
	raw, _ := readR.Value.([]byte)

	var entries []CatalogueEntry
	if r := core.JSONUnmarshal(raw, &entries); !r.OK {
		return core.Fail(core.E(fetchIndexOp, "cache parse failed", nil))
	}

	return core.Ok(FetchIndexResult{
		Entries:   entries,
		CachedAt:  info.ModTime(),
		FromCache: true,
	})
}

// downloadIndex fetches the index from indexURL, writes the cache, and returns
// the parsed result. Cerberus #1433 — fetch is now https-only with redirect
// re-validation + a 4 MiB body cap.
func (s *Service) downloadIndex(indexURL, cachePath string) core.Result {
	if r := requireHTTPS(fetchIndexOp, indexURL); !r.OK {
		return r
	}
	raw, _, r := fetchCapped(fetchIndexOp, indexURL, maxIndexBytes)
	if !r.OK {
		return r
	}

	var entries []CatalogueEntry
	if r := core.JSONUnmarshal(raw, &entries); !r.OK {
		return core.Fail(core.E(fetchIndexOp, "index parse failed", nil))
	}

	// Write cache — best-effort; a failed write doesn't break the response.
	cacheDir := core.PathDir(cachePath)
	_ = core.MkdirAll(cacheDir, 0o755)
	_ = core.WriteFile(cachePath, raw, 0o644)

	return core.Ok(FetchIndexResult{
		Entries:   entries,
		FromCache: false,
	})
}

// fetchManifestHTTPS downloads a manifest YAML from an https:// URL and
// parses it. Cerberus #1433 — https-only, redirect re-validated, 256 KiB cap.
func (s *Service) fetchManifestHTTPS(url string) core.Result {
	if r := requireHTTPS(fetchManifestOp, url); !r.OK {
		return r
	}
	raw, _, r := fetchCapped(fetchManifestOp, url, maxManifestBytes)
	if !r.OK {
		return r
	}
	return ParseManifestBytes(raw)
}
