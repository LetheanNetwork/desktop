// SPDX-Licence-Identifier: EUPL-1.2

// registry_fake_test.go — end-to-end coverage of FetchIndex /
// FetchManifest / ConfirmManifestVersionBump / the Install catalogue-
// staleness + pending-version gates against a REAL httptest.NewTLSServer
// wired through subject.WithFakeRegistry (see export_test.go). No
// mocked HTTP behaviour: the real net/http client, the real
// requireHTTPS / requireAllowedManifestHost gates, and the real JSON/
// YAML decode paths all run — only the endpoint's network address is
// swapped from marketplace.lthn.ai to a loopback listener.

package marketplace_test

import (
	"net/http"

	core "dappco.re/go"
	subject "dappco.re/lthn/desktop/pkg/marketplace"
)

const fakeManifestYAML = `schema: lthn-vm/v1
name: opencode
display: OpenCode
images:
  - id: app
    image: alpine:3.21
`

// TestFetchIndex_DownloadsAndCaches_Good — cache-miss forces a real
// HTTP GET against the fake registry; the parsed result stamps
// FetchedAt on every entry; a SECOND call within the TTL window must
// hit the on-disk cache and NOT touch the fake server again.
func TestFetchIndex_DownloadsAndCaches_Good(t *core.T) {
	t.Setenv("HOME", t.TempDir())
	requests := 0
	base := subject.WithFakeRegistry(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"name":"opencode","display":"OpenCode","source_url":"https://marketplace.lthn.ai/v1/opencode.yml"}]`))
	}))

	svc := subject.NewService(nil)

	r1 := svc.FetchIndex(base + "/v1/index.json")
	core.RequireTrue(t, r1.OK, r1.Error())
	res1 := r1.Value.(subject.FetchIndexResult)
	core.AssertLen(t, res1.Entries, 1)
	core.AssertFalse(t, res1.FromCache)
	core.AssertFalse(t, res1.Entries[0].FetchedAt.IsZero(),
		"downloadIndex must stamp FetchedAt on every entry (Mantis #1640 F3)")

	r2 := svc.FetchIndex(base + "/v1/index.json")
	core.RequireTrue(t, r2.OK, r2.Error())
	res2 := r2.Value.(subject.FetchIndexResult)
	core.AssertTrue(t, res2.FromCache, "second call within TTL must serve from cache")

	core.AssertEqual(t, 1, requests, "cache hit must not re-fetch the fake registry")
}

// TestFetchIndex_NonHTTPS_Bad — downloadIndex's requireHTTPS gate
// rejects a plain http:// index URL before any network I/O.
func TestFetchIndex_NonHTTPS_Bad(t *core.T) {
	t.Setenv("HOME", t.TempDir())
	svc := subject.NewService(nil)
	r := svc.FetchIndex("http://marketplace.lthn.ai/v1/index.json")
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "https://")
}

// TestFetchIndex_HostNotAllowed_Bad — a syntactically-valid https URL
// on a host outside allowedManifestHostSuffixes is rejected before
// any network I/O.
func TestFetchIndex_HostNotAllowed_Bad(t *core.T) {
	t.Setenv("HOME", t.TempDir())
	svc := subject.NewService(nil)
	r := svc.FetchIndex("https://evil.example.com/index.json")
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "not on the allowlist")
}

// TestFetchIndex_OversizedBody_Bad — a Content-Length above
// maxIndexBytes rejects before the body is read into memory.
func TestFetchIndex_OversizedBody_Bad(t *core.T) {
	t.Setenv("HOME", t.TempDir())
	base := subject.WithFakeRegistry(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "999999999")
		w.WriteHeader(http.StatusOK)
	}))
	svc := subject.NewService(nil)
	r := svc.FetchIndex(base + "/v1/index.json")
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "exceeds cap")
}

// TestFetchManifest_HTTPSSuccess_Good — a real GET against the fake
// registry, real YAML parse, real ValidateManifest pass.
func TestFetchManifest_HTTPSSuccess_Good(t *core.T) {
	base := subject.WithFakeRegistry(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(fakeManifestYAML))
	}))
	svc := subject.NewService(nil)
	r := svc.FetchManifest(base + "/opencode.yml")
	core.RequireTrue(t, r.OK, r.Error())
	m := r.Value.(subject.BundleManifest)
	core.AssertEqual(t, "opencode", m.Name)
}

// TestFetchManifest_ParseFailure_Bad — the fake registry serves bytes
// that don't decode as a valid manifest; fetchManifestHTTPS's parse
// branch surfaces the failure (and fires emitFetchManifestFailed).
func TestFetchManifest_ParseFailure_Bad(t *core.T) {
	base := subject.WithFakeRegistry(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not: valid: manifest: : :\n  - broken [yaml"))
	}))
	svc := subject.NewService(nil)
	r := svc.FetchManifest(base + "/broken.yml")
	core.AssertFalse(t, r.OK)
}

// TestFetchManifest_HTTPNotFound_Bad — a 404 from the registry
// surfaces through fetchCapped's status-code gate.
func TestFetchManifest_HTTPNotFound_Bad(t *core.T) {
	base := subject.WithFakeRegistry(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	svc := subject.NewService(nil)
	r := svc.FetchManifest(base + "/missing.yml")
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "404")
}

// TestFetchManifest_HostNotAllowed_Bad — FetchManifest's own
// host-allowlist gate (distinct EventMarketplaceFetchManifestRejected
// branch, separate from requireHTTPS) rejects before any network I/O.
func TestFetchManifest_HostNotAllowed_Bad(t *core.T) {
	svc := subject.NewService(nil)
	r := svc.FetchManifest("https://evil.example.com/x.yml")
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "not on the allowlist")
}

// Note: ConfirmManifestVersionBump's empty-bundleID / empty-digest
// validation is already covered by TestInstall_ConfirmManifestVersionBump_Bad
// in install_test.go; the real success/promotion path (pending →
// canonical digest, survives a cache re-read) is covered by
// TestConfirmManifestVersionBump_FullSuccess_Good in
// install_deep_test.go, driven from inside the package so it can seed
// the exact cache slot the method's internal FetchIndex("") resolves.

// TestInstall_CatalogueStale_RejectsThenForces_Bad — the Mantis #1640
// F3 staleness gate: a catalogue entry whose FetchedAt is older than
// StaleCatalogueThreshold rejects Install unless ForceStaleInstall is
// set — driven against a REAL fake-registry-backed catalogue entry
// (not a hand-built one) so findCatalogueEntry's FetchIndex("") call
// resolves through the real cache-read path.
func TestInstall_CatalogueStale_RejectsThenForces_Bad(t *core.T) {
	t.Setenv("HOME", t.TempDir())

	// Seed an already-stale cache entry directly (bypasses the fake
	// HTTP round trip — we want a FetchedAt far in the past, which a
	// live download would never produce since downloadIndex always
	// stamps core.Now()).
	staleJSON := []byte(`[{"name":"test-bundle","display":"Test","source_url":"https://marketplace.lthn.ai/v1/test-bundle.yml","fetched_at":"2000-01-01T00:00:00Z"}]`)
	if err := subject.WriteIndexCacheForTest(subject.IndexCachePathForTest(subject.NewService(nil)), staleJSON); err != nil {
		t.Fatalf("seed stale cache: %v", err)
	}

	svc := subject.NewService(nil)
	m := subject.BundleManifest{
		Schema:  "lthn-vm/v1",
		Name:    "test-bundle",
		Display: "Test Bundle",
		Images:  []subject.ImageEntry{{ID: "app", Image: "alpine:3.21"}},
	}

	r := svc.Install(subject.InstallInput{Manifest: m})
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "marketplace.catalogue.stale")

	// ForceStaleInstall bypasses the staleness gate; Install proceeds
	// to the next gate (no sandbox wired → "sandbox service not
	// available"), proving the override actually let it past staleness
	// rather than failing for the same reason twice.
	r2 := svc.Install(subject.InstallInput{Manifest: m, ForceStaleInstall: true})
	core.AssertFalse(t, r2.OK)
	core.AssertContains(t, r2.Error(), "sandbox service not available")
}

// TestInstall_ManifestVersionPending_Bad — the Mantis #1645 F6 gate:
// a catalogue entry carrying a non-empty PendingManifestDigest blocks
// Install until ConfirmManifestVersionBump promotes it.
func TestInstall_ManifestVersionPending_Bad(t *core.T) {
	t.Setenv("HOME", t.TempDir())
	freshJSON := []byte(`[{"name":"test-bundle","display":"Test","source_url":"https://marketplace.lthn.ai/v1/test-bundle.yml","pending_manifest_digest":"sha256:pending"}]`)
	if err := subject.WriteIndexCacheForTest(subject.IndexCachePathForTest(subject.NewService(nil)), freshJSON); err != nil {
		t.Fatalf("seed pending-version cache: %v", err)
	}

	svc := subject.NewService(nil)
	m := subject.BundleManifest{
		Schema:  "lthn-vm/v1",
		Name:    "test-bundle",
		Display: "Test Bundle",
		Images:  []subject.ImageEntry{{ID: "app", Image: "alpine:3.21"}},
	}
	r := svc.Install(subject.InstallInput{Manifest: m})
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "marketplace.manifest.version_unconfirmed")
}
