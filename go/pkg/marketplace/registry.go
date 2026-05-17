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
	"dappco.re/lthn/desktop/pkg/paths"
)

const (
	fetchIndexOp    = "marketplace.FetchIndex"
	fetchManifestOp = "marketplace.FetchManifest"

	// defaultIndexURL is the curated catalogue endpoint.
	defaultIndexURL = "https://marketplace.lthn.ai/v1/index.json"

	// manifestHostNotAllowedMsg is the rejection message used when a
	// FetchManifest source URL targets a host outside
	// allowedManifestHostSuffixes. Stable string so audit consumers /
	// UI surfaces can keyword-match without depending on core.Is
	// classification (no typed-error sentinel today — beta-speed
	// compile-time const per Mantis #1690 brief).
	manifestHostNotAllowedMsg = "manifest host is not on the allowlist"

	// indexCacheTTL is how long a locally cached index is considered fresh.
	// Matches the "every 24h" cadence from the spec.
	indexCacheTTL = 24 * core.Hour

	// indexCacheFileName is the local file name for the cached index.
	indexCacheFileName = "index.json"

	// StaleCatalogueThreshold is the wall-clock window within which a
	// CatalogueEntry's FetchedAt timestamp keeps the entry installable
	// (Mantis #1640 HIGH F3). Beyond this threshold pkg/marketplace.Install
	// rejects with `marketplace.catalogue.stale` unless the caller asserts
	// InstallInput.ForceStaleInstall=true. Matches indexCacheTTL today —
	// "if the curated mirror hasn't been visible in 24h, refuse install
	// without operator confirmation" is the spec shape per the brief.
	//
	// NOT runtime-configurable to forecloses TOCTOU: a malicious actor
	// flipping the threshold between the gate check and the install
	// would defeat the gate. Updates ship as code review. Future config
	// plumbing is a forward-arc concern.
	StaleCatalogueThreshold = 24 * core.Hour

	// codeStaleCatalogueInstall is the typed reject literal returned by
	// Install when FetchedAt > StaleCatalogueThreshold and
	// ForceStaleInstall is false. Stable string so audit consumers /
	// UI surfaces can keyword-match.
	codeStaleCatalogueInstall = "marketplace.catalogue.stale"

	// codeManifestVersionUnconfirmed is the typed reject literal returned
	// by Install when CatalogueEntry.PendingManifestDigest is non-empty
	// and ConfirmManifestVersionBump has not yet promoted Pending →
	// ManifestDigest (Mantis #1645 MED F6).
	codeManifestVersionUnconfirmed = "marketplace.manifest.version_unconfirmed"

	// codeImageDigestMismatch is the typed reject literal returned by
	// Install when sandbox.InspectImage(ref) returns a digest that
	// differs from the manifest's expected digest for the image
	// (Mantis #1639 HIGH F2).
	codeImageDigestMismatch = "image.digest_mismatch"

	// codeImageDigestUnverifiable is the typed reject literal returned by
	// Install when sandbox.InspectImage(ref) cannot resolve a digest at
	// all (docker unavailable, inspect failed, parse failed). Distinct
	// from mismatch so the auditor can distinguish "wrong digest"
	// (active tampering) from "unable to verify" (degraded host).
	codeImageDigestUnverifiable = "image.digest_unverifiable"

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

// allowedManifestHostSuffixes is the compile-time allowlist of OCI /
// HTTPS hosts a caller's manifest URL may target. Cerberus Mantis #1690
// C2 — mirrors the imagetrust.IsAllowedImage shape at the manifest tier
// (and pkg/downloader.allowedHostSuffixes at the model-bytes tier):
// without a host gate, FetchManifest accepts any agent-supplied https
// URL via mcpInstall.SourceURL, turning the marketplace fetch surface
// into a fetch-anywhere primitive for any party that can reach the MCP
// tool.
//
// Suffix-matched against the parsed URL hostname so CDN mirrors under
// the same parent zone (e.g. marketplace.lthn.ai serving a manifest
// referencing v1.marketplace.lthn.ai) work without per-CDN entries.
//
// Big-tent OSS coverage: our own Forge surfaces (homelab + public),
// the canonical marketplace endpoint, and the four community git hosts
// where third-party bundle authors are likely to publish manifest YAML
// (GitHub, GitLab, Codeberg + raw.githubusercontent.com is reachable
// under github.com suffix-match).
//
// NOT runtime-configurable to avoid TOCTOU — a malicious caller flipping
// the list between the gate check and the HTTP fetch would defeat the
// gate. Updates ship as code review. Same compile-time discipline as
// imagetrust.allowedImageRegistries (#1667) and
// downloader.allowedHostSuffixes (#1424).
//
// MUST stay sorted alphabetically AND contain no duplicates — invariant
// asserted in TestMarketplace_AllowedManifestHosts_Sorted_Good.
var allowedManifestHostSuffixes = []string{
	"codeberg.org",
	"forge.lthn.ai",
	"forge.lthn.sh",
	"github.com",
	"githubusercontent.com",
	"gitlab.com",
	"marketplace.lthn.ai",
}

// isAllowedManifestHost reports whether rawURL's hostname matches a
// suffix on allowedManifestHostSuffixes. Returns false for empty,
// malformed, or non-https URLs — callers should require https
// separately so the typed error class stays distinct from the
// host-allowlist rejection.
//
// Usage example:
//
//	if !isAllowedManifestHost(url) {
//	    return core.Fail(core.E(fetchManifestOp,
//	        manifestHostNotAllowedMsg+": "+url, nil))
//	}
func isAllowedManifestHost(rawURL string) bool {
	if rawURL == "" {
		return false
	}
	r := core.URLParse(rawURL)
	if !r.OK {
		return false
	}
	u, _ := r.Value.(*core.URL)
	if u == nil {
		return false
	}
	host := core.Lower(u.Hostname())
	if host == "" {
		return false
	}
	for _, suffix := range allowedManifestHostSuffixes {
		if host == suffix || core.HasSuffix(host, "."+suffix) {
			return true
		}
	}
	return false
}

// requireAllowedManifestHost wraps isAllowedManifestHost in the
// core.Result idiom used at the FetchManifest / downloadIndex entry
// boundary. Fail message carries the rejected URL so audit + UI
// surfaces can show the operator which host was blocked.
func requireAllowedManifestHost(op, rawURL string) core.Result {
	if !isAllowedManifestHost(rawURL) {
		return core.Fail(core.E(op,
			manifestHostNotAllowedMsg+": "+rawURL, nil))
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
	raw, _, status, result = fetchCappedWithETag(op, url, maxBytes)
	return
}

// fetchCappedWithETag mirrors fetchCapped and additionally returns the
// ETag header value. Used by the .sig + body pinned-fetch path so the
// caller can require both fetches to land at the same ETag (DREAD v2
// N2 / Mantis #1650 — rotation-race close). Empty ETag is tolerated
// (origin may not stamp one); the caller's rotation-race gate fires
// only when BOTH responses carry a non-empty ETag AND they differ.
func fetchCappedWithETag(op, url string, maxBytes int64) (raw []byte, etag string, status int, result core.Result) {
	return fetchCappedWithETagClient(op, url, maxBytes, httpsOnlyClient)
}

// fetchCappedWithETagClient is the inner form parameterised on the
// HTTP client so unit tests can inject httptest.NewTLSServer().Client()
// — the production path always passes httpsOnlyClient.
func fetchCappedWithETagClient(op, url string, maxBytes int64, client *core.HTTPClient) (raw []byte, etag string, status int, result core.Result) {
	reqR := core.NewHTTPRequest("GET", url, nil)
	if !reqR.OK {
		return nil, "", 0, reqR
	}
	req := reqR.Value.(*core.Request)
	resp, err := client.Do(req)
	if err != nil {
		if resp != nil {
			_ = resp.Body.Close()
		}
		return nil, "", 0, core.Fail(core.E(op, "GET failed: "+url, err))
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, "", resp.StatusCode, core.Fail(core.E(op,
			core.Sprintf("HTTP %d from %s", resp.StatusCode, url), nil))
	}
	if resp.ContentLength > maxBytes {
		return nil, "", resp.StatusCode, core.Fail(core.E(op,
			core.Sprintf("Content-Length %d exceeds cap %d",
				resp.ContentLength, maxBytes), nil))
	}

	bounded := core.LimitReader(resp.Body, maxBytes+1)
	readR := core.ReadAll(bounded)
	if !readR.OK {
		return nil, "", resp.StatusCode, core.Fail(core.E(op, "response read failed", nil))
	}
	// core.ReadAll's Value is a string per pkg/external/go/io.go contract;
	// convert to bytes here so callers (parsers, hashers) get the expected
	// shape. The legacy cast-to-[]byte form silently swallowed the body and
	// was the source of the empty-body diagnosis during Wave 1 fetch-pin
	// test bring-up.
	rawStr, _ := readR.Value.(string)
	raw = []byte(rawStr)
	if int64(len(raw)) > maxBytes {
		return nil, "", resp.StatusCode, core.Fail(core.E(op,
			core.Sprintf("response body exceeded %d byte cap", maxBytes), nil))
	}
	etag = resp.Header.Get("ETag")
	return raw, etag, resp.StatusCode, core.Ok(nil)
}

// FetchManifestSignedResult bundles the body + signature pair from
// fetchManifestWithSig along with the matched ETag for forensic
// diagnosis. Used by the install pipeline to assert ETag-pinned
// fetch arrival before invoking Verify.
type FetchManifestSignedResult struct {
	Body      []byte
	Signature []byte
	ETag      string
}

// fetchManifestWithSig fetches `<url>` and `<url>.sig` from the same
// origin and asserts both responses arrive with the same ETag (when
// the origin stamps one). DREAD v2 N2 HIGH (Mantis #1650) — without
// this gate, a mirror that briefly rotates the body between the .sig
// fetch and the body fetch can defeat the signature check.
//
// Returns sigRotationRaceReason on Result.Error() when both fetches
// land with non-empty ETags that disagree. When either side returns
// an empty ETag (origin doesn't stamp one), the gate is informational
// only — the verify chain still asserts cryptographic correctness, but
// the rotation-race detection can't fire from missing wire evidence.
//
// Both fetches are bounded at maxManifestBytes (256 KiB) since the
// `.sig` file is structurally tiny (64 bytes for raw ed25519) and the
// manifest body is the cap-defining surface.
//
// Usage example:
//
//	r := fetchManifestWithSig(fetchManifestOp, "https://m.lthn.ai/x.yml")
//	if r.OK { sr := r.Value.(FetchManifestSignedResult) }
func fetchManifestWithSig(op, url string) core.Result {
	return fetchManifestWithSigClient(op, url, httpsOnlyClient)
}

// fetchManifestWithSigClient is the inner form parameterised on the
// HTTP client so unit tests can drive the rotation-race gate against
// an httptest.NewTLSServer().Client() without ripping the production
// httpsOnlyClient discipline out of the call path.
func fetchManifestWithSigClient(op, url string, client *core.HTTPClient) core.Result {
	bodyRaw, bodyETag, _, bodyResult := fetchCappedWithETagClient(op, url, maxManifestBytes, client)
	if !bodyResult.OK {
		return bodyResult
	}
	sigURL := url + ".sig"
	sigRaw, sigETag, _, sigResult := fetchCappedWithETagClient(op, sigURL, maxManifestBytes, client)
	if !sigResult.OK {
		return sigResult
	}
	if bodyETag != "" && sigETag != "" && bodyETag != sigETag {
		return core.Fail(core.E(op,
			sigRotationRaceReason+": body ETag "+bodyETag+
				" != .sig ETag "+sigETag, nil))
	}
	return core.Ok(FetchManifestSignedResult{
		Body:      bodyRaw,
		Signature: sigRaw,
		ETag:      bodyETag,
	})
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
	// Signature is the optional detached ed25519 signature over the
	// CatalogueEntry's CBOR canonical encoding (CatalogueEntry with
	// Signature nil) — new in v1.1 per Mantis #1637 DREAD F1. v1
	// entries decode cleanly with both Signature and KeyID empty;
	// the verify gate enforces presence only when a trusted-keys
	// store is configured at runtime.
	Signature []byte `json:"signature,omitempty"`
	// KeyID names which entry in trusted_keys.json verified Signature.
	// Capped at maxTrustedKeyIDChars (Mantis #1648 DREAD v2 N3).
	KeyID string `json:"key_id,omitempty"`

	// FetchedAt records when the registry last refreshed THIS entry from
	// the curated index endpoint. Used by pkg/marketplace.Install's
	// staleness gate (Mantis #1640 HIGH F3): if Since(FetchedAt) >
	// StaleCatalogueThreshold and InstallInput.ForceStaleInstall is
	// false, Install rejects with `marketplace.catalogue.stale` to
	// prevent "I'm patched" confidence under attack where a hostile
	// mirror withholds updates to keep operators installing known-bad
	// versions. Stamped at writeIndexCache time from FetchIndexResult.
	// Zero value treated as "fetched in the unbounded past" — defaults
	// the gate to TRIPPED unless ForceStaleInstall set.
	FetchedAt core.Time `json:"fetched_at,omitempty"`

	// PendingManifestDigest is the proposed-but-unconfirmed manifest
	// digest for this entry's NEXT version bump (Mantis #1645 MED F6).
	// When the upstream publisher rotates a manifest at the SAME source
	// URL, the registry refresh stamps the new digest here; Install
	// REJECTS with `marketplace.manifest.version_unconfirmed` until an
	// operator-confirmed call to (*Service).ConfirmManifestVersionBump
	// promotes Pending → ManifestDigest. Empty (zero string) when no
	// pending bump exists — the canonical happy-path shape.
	PendingManifestDigest string `json:"pending_manifest_digest,omitempty"`

	// ManifestDigest is the catalogue-canonical expected digest of the
	// fetched bundle manifest body (sha256 over the YAML bytes BEFORE
	// signature stripping). Compared against the locally-recomputed
	// digest after FetchManifest as the F6 split-brain anchor.
	// Empty for v1 entries that predate #1645; the gate is informational
	// (no reject) when empty.
	ManifestDigest string `json:"manifest_digest,omitempty"`
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
	// Cerberus #54 C4 (Mantis #1692) — Requested row commits at the
	// top so a forensic walker captures every fetch attempt regardless
	// of the protocol-switch outcome. The Succeeded / Failed / Rejected
	// emit happens inside fetchManifestHTTPS (the only path that
	// reaches the host-allowlist gate + network); the other switch
	// arms emit Failed inline.
	emitFetchManifestRequested(sourceURL)

	if core.Trim(sourceURL) == "" {
		fail := core.Fail(core.E(fetchManifestOp, "source url is required", nil))
		emitFetchManifestFailed(sourceURL, fail)
		return fail
	}

	switch {
	case core.HasPrefix(sourceURL, "https://"):
		return s.fetchManifestHTTPS(sourceURL)
	case core.HasPrefix(sourceURL, "http://"):
		// Cerberus #1433 — http:// is a downgrade vector; reject
		// loudly rather than silently upgrading.
		fail := core.Fail(core.E(fetchManifestOp,
			"refusing plaintext http:// source URL (use https://): "+sourceURL, nil))
		emitFetchManifestFailed(sourceURL, fail)
		return fail
	case core.HasPrefix(sourceURL, "oci://"):
		fail := core.Fail(core.E(fetchManifestOp,
			"oci:// source URLs are not supported in v1 — use https:// instead", nil))
		emitFetchManifestFailed(sourceURL, fail)
		return fail
	case core.HasPrefix(sourceURL, "git+https://"):
		fail := core.Fail(core.E(fetchManifestOp,
			"git+https:// source URLs are not supported in v1 — use https:// instead", nil))
		emitFetchManifestFailed(sourceURL, fail)
		return fail
	default:
		fail := core.Fail(core.E(fetchManifestOp,
			"unsupported source URL protocol: "+sourceURL, nil))
		emitFetchManifestFailed(sourceURL, fail)
		return fail
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
//
// Mantis #1582 — the cache body may now carry the optional version
// frontmatter stamped by writeIndexCache via paths.AtomicWriteWithVersion.
// stripCacheFrontmatter peels it before JSON unmarshal so legacy
// pure-JSON caches (pre-#1582) continue to parse cleanly — lazy
// migration per RFC §3.2.
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
	raw = stripCacheFrontmatter(raw)

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

// indexCacheVersion is the current frontmatter version stamped on
// new writes (Mantis #1582). Bump on schema-breaking cache shape
// changes; readers handle prior versions via lazy migration in
// stripCacheFrontmatter.
const indexCacheVersion = 1

// composeCacheBody prepends YAML frontmatter (---\nversion: N\n---\n)
// to the JSON cache body so paths.AtomicWriteWithVersion can read
// the version via parseFrontmatterVersion + the IfVersion gate works
// as documented. Body shape is wire-stable across the cache TTL —
// readers that don't know about the frontmatter (legacy pure-JSON)
// still parse OK because stripCacheFrontmatter is the strip surface.
//
// Usage example:
//
//	body := composeCacheBody(jsonBytes, indexCacheVersion)
//	r := paths.AtomicWriteWithVersion(cachePath, paths.WriteInput{Body: body})
func composeCacheBody(jsonBody []byte, version int) []byte {
	header := "---\nversion: " + core.Sprintf("%d", version) + "\n---\n"
	out := make([]byte, 0, len(header)+len(jsonBody))
	out = append(out, []byte(header)...)
	out = append(out, jsonBody...)
	return out
}

// stripCacheFrontmatter removes a leading "---\n...---\n" YAML
// frontmatter block from raw if present, otherwise returns raw
// unchanged. Mantis #1582 — readers tolerate both shapes during the
// lazy-migration window so a pre-#1582 cache on disk parses without
// a rewrite roundtrip.
func stripCacheFrontmatter(raw []byte) []byte {
	const open = "---\n"
	if len(raw) < len(open) {
		return raw
	}
	for i := 0; i < len(open); i++ {
		if raw[i] != open[i] {
			return raw
		}
	}
	// Find closing "---\n" delimiter after the open.
	rest := raw[len(open):]
	for i := 0; i < len(rest)-3; i++ {
		if rest[i] == '-' && rest[i+1] == '-' && rest[i+2] == '-' {
			if i == 0 || rest[i-1] == '\n' {
				// Skip past --- + the following newline if present.
				end := i + 3
				if end < len(rest) && rest[end] == '\n' {
					end++
				}
				return rest[end:]
			}
		}
	}
	// Malformed frontmatter (no close) — return as-is so JSON parse
	// can surface the real error.
	return raw
}

// writeIndexCache performs the Mantis #1582 atomic write of the
// cache body via paths.AtomicWriteWithVersion. The cache is a
// single-writer surface today (only downloadIndex writes) but the
// atomic primitive still buys us tmp+fsync+rename torn-write safety
// + frontmatter-stamped version so future cascade adopters can
// IfVersion-gate without a schema migration.
//
// On stale conflict (IfVersion mismatch), wraps the VersionStale
// envelope as paths.ConflictEnvelope with code
// "marketplace.index_cache.conflict" so the surface matches the
// cascade convention.
func writeIndexCache(cachePath string, jsonBody []byte) core.Result {
	cacheDir := core.PathDir(cachePath)
	_ = core.MkdirAll(cacheDir, 0o755)

	body := composeCacheBody(jsonBody, indexCacheVersion)
	r := paths.AtomicWriteWithVersion(cachePath, paths.WriteInput{
		Body: body,
	})
	if !r.OK {
		if stale, ok := paths.VersionStaleFromError(r.Value); ok {
			return core.Fail(paths.NewConflictEnvelope(
				"marketplace.index_cache.conflict", stale))
		}
		return r
	}
	return r
}

// downloadIndex fetches the index from indexURL, writes the cache, and returns
// the parsed result. Cerberus #1433 — fetch is now https-only with redirect
// re-validation + a 4 MiB body cap.
func (s *Service) downloadIndex(indexURL, cachePath string) core.Result {
	if r := requireHTTPS(fetchIndexOp, indexURL); !r.OK {
		return r
	}
	if r := requireAllowedManifestHost(fetchIndexOp, indexURL); !r.OK {
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

	// Mantis #1640 HIGH F3 — stamp FetchedAt on every entry so Install's
	// staleness gate has wall-clock evidence to evaluate. We stamp the
	// in-memory copy AND re-marshal before cache write so the cached
	// shape carries the same evidence (and the next readIndexCache
	// surfaces a populated FetchedAt rather than the zero default).
	now := core.Now()
	for i := range entries {
		entries[i].FetchedAt = now
	}
	stampedRaw := raw
	if r := core.JSONMarshal(entries); r.OK {
		if b, ok := r.Value.([]byte); ok && len(b) > 0 {
			stampedRaw = b
		}
	}

	// Write cache via paths.AtomicWriteWithVersion (Mantis #1582 —
	// cascade-adoption). Best-effort on the network response side: a
	// failed cache write doesn't break the live result. tmp+fsync+
	// rename atomicity prevents torn cache files; frontmatter
	// version stamp opens the IfVersion gate for future adopters.
	_ = writeIndexCache(cachePath, stampedRaw)

	return core.Ok(FetchIndexResult{
		Entries:   entries,
		FromCache: false,
	})
}

// findCatalogueEntry looks up one entry from the catalogue by bundle
// name. Used by Install's staleness + manifest-version-confirmation
// gates so the typed reject can stamp the FetchedAt /
// PendingManifestDigest evidence at the rejection boundary.
//
// Returns nil when the catalogue is unreachable, has no entry for
// name, or the FetchIndex call itself fails — callers treat missing
// entries as "no catalogue evidence; fall through to permissive
// shape" rather than blocking install on a network failure.
//
// Usage example (internal):
//
//	entry := s.findCatalogueEntry(m.Name)
//	if entry != nil && time.Since(entry.FetchedAt) > StaleCatalogueThreshold {
//	    return core.Fail(...)
//	}
func (s *Service) findCatalogueEntry(name string) *CatalogueEntry {
	name = core.Trim(name)
	if name == "" {
		return nil
	}
	r := s.FetchIndex("")
	if !r.OK {
		return nil
	}
	idx, ok := r.Value.(FetchIndexResult)
	if !ok {
		return nil
	}
	for i := range idx.Entries {
		if idx.Entries[i].Name == name {
			return &idx.Entries[i]
		}
	}
	return nil
}

// ConfirmManifestVersionBump promotes a CatalogueEntry's
// PendingManifestDigest into the canonical ManifestDigest field after
// operator confirmation (Mantis #1645 MED F6). expectedNewDigest must
// match the entry's current PendingManifestDigest exactly — the
// operator UI displays the pending value and asks for explicit
// "yes, I confirm this is the version I want to allow" before this
// method fires.
//
// On match: writes the updated catalogue back through the cache layer
// + emits `marketplace.manifest.version.confirmed` audit row.
//
// On mismatch: rejects with `marketplace.manifest.version_mismatch` —
// the operator was looking at a stale-by-now confirmation prompt
// (another refresh landed a different pending). The UI must re-fetch
// and re-confirm.
//
// On missing entry / missing pending: rejects with
// `marketplace.manifest.version_no_pending` — no transition to make.
//
// Renderer-callable via WConfirmManifestVersionBump per the tier-auth
// substrate (requires TierOperator) — but the substrate-tier method
// itself is the public API in this Wave; the W-binding lands as a
// follow-up tier-routing ticket.
//
// Usage example:
//
//	r := svc.ConfirmManifestVersionBump("opencode", "sha256:abc123…")
//	if !r.OK { /* operator must re-fetch + re-confirm */ }
func (s *Service) ConfirmManifestVersionBump(bundleID, expectedNewDigest string) core.Result {
	bundleID = core.Trim(bundleID)
	expectedNewDigest = core.Trim(expectedNewDigest)
	if bundleID == "" {
		return core.Fail(core.E("marketplace.ConfirmManifestVersionBump",
			"bundle id is required", nil))
	}
	if expectedNewDigest == "" {
		return core.Fail(core.E("marketplace.ConfirmManifestVersionBump",
			"expected new digest is required", nil))
	}
	r := s.FetchIndex("")
	if !r.OK {
		return r
	}
	idx, ok := r.Value.(FetchIndexResult)
	if !ok {
		return core.Fail(core.E("marketplace.ConfirmManifestVersionBump",
			"catalogue not available", nil))
	}
	pos := -1
	for i := range idx.Entries {
		if idx.Entries[i].Name == bundleID {
			pos = i
			break
		}
	}
	if pos < 0 {
		return core.Fail(core.E("marketplace.ConfirmManifestVersionBump",
			"marketplace.manifest.version_no_pending: bundle not in catalogue: "+bundleID, nil))
	}
	pending := core.Trim(idx.Entries[pos].PendingManifestDigest)
	if pending == "" {
		return core.Fail(core.E("marketplace.ConfirmManifestVersionBump",
			"marketplace.manifest.version_no_pending: no pending version bump for "+bundleID, nil))
	}
	if pending != expectedNewDigest {
		return core.Fail(core.E("marketplace.ConfirmManifestVersionBump",
			"marketplace.manifest.version_mismatch: operator-confirmed digest does not match current pending", nil))
	}
	prior := core.Trim(idx.Entries[pos].ManifestDigest)
	idx.Entries[pos].ManifestDigest = pending
	idx.Entries[pos].PendingManifestDigest = ""
	idx.Entries[pos].FetchedAt = core.Now()
	// Re-marshal + atomic-rewrite the cache so the promotion survives a
	// process restart. Best-effort: a cache-write failure surfaces but
	// the in-memory FetchIndex consumers next call will rebuild from
	// network anyway.
	body := core.JSONMarshal(idx.Entries)
	if body.OK {
		if b, ok := body.Value.([]byte); ok {
			_ = writeIndexCache(s.indexCachePath(), b)
		}
	}
	emitManifestVersionBumpTransition(bundleID, prior, pending, "confirmed")
	return core.Ok(idx.Entries[pos])
}

// fetchManifestHTTPS downloads a manifest YAML from an https:// URL and
// parses it. Cerberus #1433 — https-only, redirect re-validated, 256 KiB cap.
// Cerberus #1690 C2 — host-allowlist gated BEFORE any network I/O so
// FetchManifest can never be turned into a fetch-anywhere primitive by
// an agent supplying a hostile https URL via marketplace_install.
func (s *Service) fetchManifestHTTPS(url string) core.Result {
	if r := requireHTTPS(fetchManifestOp, url); !r.OK {
		emitFetchManifestFailed(url, r)
		return r
	}
	// Cerberus #54 C4 — distinct Rejected event for the host-allowlist
	// gate keeps "policy refused" rows separate from network/parse
	// failures in audit history. Mirror of EventSandboxVolumeRejected.
	if r := requireAllowedManifestHost(fetchManifestOp, url); !r.OK {
		emitFetchManifestRejected(url)
		return r
	}
	raw, _, r := fetchCapped(fetchManifestOp, url, maxManifestBytes)
	if !r.OK {
		emitFetchManifestFailed(url, r)
		return r
	}
	parsed := ParseManifestBytes(raw)
	if !parsed.OK {
		emitFetchManifestFailed(url, parsed)
		return parsed
	}
	m, _ := parsed.Value.(BundleManifest)
	emitFetchManifestSucceeded(url, m.Name)
	return parsed
}
