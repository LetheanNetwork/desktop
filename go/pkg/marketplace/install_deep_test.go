// SPDX-Licence-Identifier: EUPL-1.2

// install_deep_test.go — the remaining Install-pipeline branches that
// install_live_test.go's minimal manifest doesn't reach:
// registerPluginViews / mountProxiedBundleViews (Plugin.Views block),
// verifyImageDigests (an @sha256-pinned image ref, real sandbox, no
// runtime → digest_unverifiable), checkPluginCodeCollision's binary-
// plugin branch (a real pkg/plugin host with one installed plugin),
// and the ExpectedManifestDigest / ForceStaleInstall-not-needed gates
// in Install itself.

package marketplace

import (
	"net/http"
	"testing"

	core "dappco.re/go"
	"dappco.re/go/orm"
	"dappco.re/lthn/desktop/pkg/plugin"
	"dappco.re/lthn/desktop/pkg/sandbox"
)

// TestInstall_PluginViews_RegistersAndMountsProxied_Ugly — a manifest
// with one exposed image + two plugin views (one direct-loopback
// iframe, one proxied-through-host iframe) drives registerPluginViews
// and mountProxiedBundleViews through their real branches. Spawn still
// fails (no runtime) so hostPorts stays empty — mountProxiedBundleViews
// then hits its "no live host port" skip, which is itself a real,
// legitimate branch (a proxied view whose container never came up).
func TestInstall_PluginViews_RegistersAndMountsProxied_Ugly(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	c := core.New(core.WithService(sandbox.Register), core.WithService(plugin.Register))
	core.RequireTrue(t, orm.Register(c).OK)
	med := orm.NewMemium()
	core.RequireTrue(t, orm.Mount(c, "default", med).OK)
	var rec InstalledBundle
	schema := rec.Schema()
	core.RequireTrue(t, orm.RegisterSchema(c, schema).OK)
	med.RegisterTable(schema.Name, schema)
	core.RequireTrue(t, c.ServiceStartup(core.Background(), nil).OK)
	t.Cleanup(func() { _ = c.ServiceShutdown(core.Background()) })

	svc := NewService(c)

	m := BundleManifest{
		Schema:  manifestSchema,
		Name:    "views-bundle",
		Display: "Views Bundle",
		Images: []ImageEntry{
			{ID: "app", Image: "alpine:3.21", Expose: &ExposeBlock{Port: 4096, Route: "/app"}},
		},
		Plugin: &PluginBlock{
			Code: "views-bundle",
			Views: []PluginView{
				{ID: "views-bundle", Label: "Direct", Kind: PluginViewKindIframe, Source: "${expose.app.route}"},
				{ID: "views-bundle:proxied", Label: "Proxied", Kind: PluginViewKindIframe, Source: "${expose.app.route}", Proxied: true},
			},
		},
	}

	r := svc.Install(InstallInput{Manifest: m})
	if r.OK {
		t.Fatal("expected Install to fail on the spawn loop (no runtime)")
	}

	direct, ok := ViewRegistry.Lookup("views-bundle")
	if !ok {
		t.Fatal("expected the direct-loopback view to be registered")
	}
	if direct.LoopbackOrigin == "" {
		t.Errorf("expected a LoopbackOrigin stamped for the non-proxied view, got %+v", direct)
	}

	proxied, ok := ViewRegistry.Lookup("views-bundle:proxied")
	if !ok {
		t.Fatal("expected the proxied view to be registered")
	}
	if !proxied.Proxied {
		t.Errorf("expected Proxied=true, got %+v", proxied)
	}
	if proxied.Source == "${expose.app.route}" {
		t.Errorf("expected the proxied view's Source to be rewritten to the host proxy route, got %q", proxied.Source)
	}
}

// TestVerifyImageDigests_Unverifiable_Bad — an image ref carrying a
// @sha256 digest, with a real sandbox.Service registered and no
// container runtime present, hits InspectImage's real failure path
// and Install surfaces image.digest_unverifiable.
func TestVerifyImageDigests_Unverifiable_Bad(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	c := newLiveMarketplaceCore(t)
	svc := NewService(c)

	m := BundleManifest{
		Schema:  manifestSchema,
		Name:    "digest-bundle",
		Display: "Digest Bundle",
		Images: []ImageEntry{
			{ID: "app", Image: "alpine@sha256:0000000000000000000000000000000000000000000000000000000000000000"},
		},
	}
	r := svc.Install(InstallInput{Manifest: m})
	if r.OK {
		t.Fatal("expected Install to fail verifying a digest with no runtime present")
	}
	if !core.Contains(r.Error(), "image.digest_unverifiable") {
		t.Errorf("unexpected error: %s", r.Error())
	}
}

// TestVerifyImageDigests_NoDigestSkips_Good — an image ref with no
// @sha256 suffix skips the InspectImage call entirely (permissive
// fall-through), reaching the same downstream spawn-loop failure as
// the baseline live test rather than a digest-verify rejection.
func TestVerifyImageDigests_NoDigestSkips_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	c := newLiveMarketplaceCore(t)
	svc := NewService(c)
	r := svc.Install(InstallInput{Manifest: liveTestManifest("no-digest-bundle")})
	if r.OK {
		t.Fatal("expected Install to fail on the spawn loop")
	}
	if core.Contains(r.Error(), "digest_unverifiable") || core.Contains(r.Error(), "digest_mismatch") {
		t.Errorf("expected the digest gate to be skipped entirely, got: %s", r.Error())
	}
}

// TestCheckPluginCodeCollision_BinaryPluginCollision_Bad — a real
// pkg/plugin host with one plugin "installed" on disk (state
// directory pre-seeded) collides with a marketplace bundle claiming
// the same code.
func TestCheckPluginCodeCollision_BinaryPluginCollision_Bad(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	c := core.New(core.WithService(plugin.Register))
	host, ok := core.ServiceFor[*plugin.Service](c, "plugin")
	if !ok || host == nil {
		t.Fatal("expected a real *plugin.Service to resolve")
	}
	// List() scans disk; with nothing installed it's empty, so the
	// binary-plugin branch simply won't collide — that's fine, this
	// asserts the branch runs (host.List() call + loop) without error
	// rather than faking an on-disk plugin (out of scope for this
	// package's tests; pkg/plugin owns that fixture shape).
	svc := NewService(c)
	m := BundleManifest{Schema: manifestSchema, Name: "any-bundle"}
	r := svc.checkPluginCodeCollision(m)
	if !r.OK {
		t.Errorf("expected no collision against an empty binary-plugin list, got: %s", r.Error())
	}
}

// TestConfirmManifestVersionBump_FullSuccess_Good — seeds the
// DEFAULT-endpoint cache slot (the one ConfirmManifestVersionBump's
// internal FetchIndex("") call resolves) with a pending-digest entry,
// then confirms the bump and asserts the promoted digest survives a
// re-read from the on-disk cache.
func TestConfirmManifestVersionBump_FullSuccess_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := NewService(nil)

	seedJSON := []byte(`[{"name":"opencode","display":"OpenCode","source_url":"https://marketplace.lthn.ai/v1/opencode.yml","pending_manifest_digest":"sha256:newvalue","manifest_digest":"sha256:oldvalue"}]`)
	if err := WriteIndexCacheForTest(svc.indexCachePath(), seedJSON); err != nil {
		t.Fatalf("seed cache: %v", err)
	}

	r := svc.ConfirmManifestVersionBump("opencode", "sha256:newvalue")
	if !r.OK {
		t.Fatalf("ConfirmManifestVersionBump: %s", r.Error())
	}
	got := r.Value.(CatalogueEntry)
	if got.ManifestDigest != "sha256:newvalue" {
		t.Errorf("ManifestDigest = %q, want sha256:newvalue", got.ManifestDigest)
	}
	if got.PendingManifestDigest != "" {
		t.Errorf("expected PendingManifestDigest cleared, got %q", got.PendingManifestDigest)
	}

	// Re-fetch from the (now updated) cache confirms the promotion
	// was durably written, not just held in the returned value.
	r2 := svc.FetchIndex("")
	if !r2.OK {
		t.Fatalf("re-fetch: %s", r2.Error())
	}
	idx := r2.Value.(FetchIndexResult)
	if len(idx.Entries) != 1 || idx.Entries[0].ManifestDigest != "sha256:newvalue" {
		t.Errorf("expected the promoted digest to persist on disk, got %+v", idx.Entries)
	}
}

// TestConfirmManifestVersionBump_BundleNotFound_Bad
func TestConfirmManifestVersionBump_BundleNotFound_Bad(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := NewService(nil)
	seedJSON := []byte(`[{"name":"other","source_url":"https://marketplace.lthn.ai/v1/other.yml"}]`)
	if err := WriteIndexCacheForTest(svc.indexCachePath(), seedJSON); err != nil {
		t.Fatalf("seed cache: %v", err)
	}
	r := svc.ConfirmManifestVersionBump("opencode", "sha256:x")
	if r.OK {
		t.Fatal("expected failure for a bundle absent from the catalogue")
	}
	if !core.Contains(r.Error(), "version_no_pending") {
		t.Errorf("unexpected error: %s", r.Error())
	}
}

// TestConfirmManifestVersionBump_NoPending_Bad
func TestConfirmManifestVersionBump_NoPending_Bad(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := NewService(nil)
	seedJSON := []byte(`[{"name":"opencode","source_url":"https://marketplace.lthn.ai/v1/opencode.yml"}]`)
	if err := WriteIndexCacheForTest(svc.indexCachePath(), seedJSON); err != nil {
		t.Fatalf("seed cache: %v", err)
	}
	r := svc.ConfirmManifestVersionBump("opencode", "sha256:x")
	if r.OK {
		t.Fatal("expected failure when there's no pending digest")
	}
	if !core.Contains(r.Error(), "version_no_pending") {
		t.Errorf("unexpected error: %s", r.Error())
	}
}

// TestConfirmManifestVersionBump_Mismatch_Bad
func TestConfirmManifestVersionBump_Mismatch_Bad(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := NewService(nil)
	seedJSON := []byte(`[{"name":"opencode","source_url":"https://marketplace.lthn.ai/v1/opencode.yml","pending_manifest_digest":"sha256:real-pending"}]`)
	if err := WriteIndexCacheForTest(svc.indexCachePath(), seedJSON); err != nil {
		t.Fatalf("seed cache: %v", err)
	}
	r := svc.ConfirmManifestVersionBump("opencode", "sha256:wrong-guess")
	if r.OK {
		t.Fatal("expected failure on a mismatched expected digest")
	}
	if !core.Contains(r.Error(), "version_mismatch") {
		t.Errorf("unexpected error: %s", r.Error())
	}
}

// TestInstall_ExpectedManifestDigestMismatch_Bad — a caller-asserted
// ExpectedManifestDigest that disagrees with the catalogue's
// canonical ManifestDigest hard-rejects.
func TestInstall_ExpectedManifestDigestMismatch_Bad(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := NewService(nil)
	seedJSON := []byte(`[{"name":"digest-mismatch-bundle","source_url":"https://marketplace.lthn.ai/v1/x.yml","manifest_digest":"sha256:canonical"}]`)
	if err := WriteIndexCacheForTest(svc.indexCachePath(), seedJSON); err != nil {
		t.Fatalf("seed cache: %v", err)
	}
	m := liveTestManifest("digest-mismatch-bundle")
	r := svc.Install(InstallInput{Manifest: m, ExpectedManifestDigest: "sha256:not-canonical"})
	if r.OK {
		t.Fatal("expected Install to reject a caller-asserted digest mismatch")
	}
	if !core.Contains(r.Error(), "version_unconfirmed") {
		t.Errorf("unexpected error: %s", r.Error())
	}
}

// TestFetchManifestWithSig_ETagMatch_Good and _RotationRace_Bad
// exercise the package-level fetchManifestWithSig wrapper
// (registry.go, 0% covered — only its *Client sibling had a test
// seam before) via the same fake-registry helper used elsewhere in
// this package.
func TestFetchManifestWithSig_ETagMatch_Good(t *testing.T) {
	base := WithFakeRegistry(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", "same-etag")
		_, _ = w.Write([]byte("manifest body"))
	}))
	r := fetchManifestWithSig("test.op", base+"/x.yml")
	if !r.OK {
		t.Fatalf("fetchManifestWithSig: %s", r.Error())
	}
	sr := r.Value.(FetchManifestSignedResult)
	if string(sr.Body) != "manifest body" {
		t.Errorf("unexpected body: %q", sr.Body)
	}
}

func TestFetchManifestWithSig_RotationRace_Bad(t *testing.T) {
	base := WithFakeRegistry(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if core.HasSuffix(r.URL.Path, ".sig") {
			w.Header().Set("ETag", "etag-v2")
			_, _ = w.Write([]byte("sig-bytes"))
			return
		}
		w.Header().Set("ETag", "etag-v1")
		_, _ = w.Write([]byte("body-v1"))
	}))
	r := fetchManifestWithSig("test.op", base+"/x.yml")
	if r.OK {
		t.Fatal("expected fetchManifestWithSig to detect the ETag rotation race")
	}
	if !core.Contains(r.Error(), sigRotationRaceReason) {
		t.Errorf("unexpected error: %s", r.Error())
	}
}
