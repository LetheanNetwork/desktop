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
)

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
	case core.HasPrefix(sourceURL, "https://") || core.HasPrefix(sourceURL, "http://"):
		return s.fetchManifestHTTPS(sourceURL)
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
// the parsed result.
func (s *Service) downloadIndex(indexURL, cachePath string) core.Result {
	getR := core.HTTPGet(indexURL)
	if !getR.OK {
		return core.Fail(core.E(fetchIndexOp, "HTTP fetch failed: "+indexURL, nil))
	}
	resp := getR.Value.(*core.Response)
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return core.Fail(core.E(fetchIndexOp,
			core.Sprintf("HTTP %d from %s", resp.StatusCode, indexURL), nil))
	}

	readR := core.ReadAll(resp.Body)
	if !readR.OK {
		return core.Fail(core.E(fetchIndexOp, "response read failed", nil))
	}
	raw, _ := readR.Value.([]byte)

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
// parses it.
func (s *Service) fetchManifestHTTPS(url string) core.Result {
	getR := core.HTTPGet(url)
	if !getR.OK {
		return core.Fail(core.E(fetchManifestOp, "HTTP fetch failed: "+url, nil))
	}
	resp := getR.Value.(*core.Response)
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return core.Fail(core.E(fetchManifestOp,
			core.Sprintf("HTTP %d from %s", resp.StatusCode, url), nil))
	}

	readR := core.ReadAll(resp.Body)
	if !readR.OK {
		return core.Fail(core.E(fetchManifestOp, "response read failed", nil))
	}
	raw, _ := readR.Value.([]byte)

	return ParseManifestBytes(raw)
}
