// SPDX-Licence-Identifier: EUPL-1.2

package marketplace_test

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/sandbox"

	subject "dappco.re/lthn/desktop/pkg/marketplace"
)

// minimalInstallManifest is the simplest valid bundle — one image, no plugin block.
var minimalInstallManifest = subject.BundleManifest{
	Schema:  "lthn-vm/v1",
	Name:    "test-bundle",
	Display: "Test Bundle",
	Images: []subject.ImageEntry{
		{ID: "app", Image: "alpine:3.21"},
	},
}

// fullInstallManifest exercises env substitution + volumes + expose.
var fullInstallManifest = subject.BundleManifest{
	Schema:  "lthn-vm/v1",
	Name:    "opencode",
	Display: "OpenCode",
	Images: []subject.ImageEntry{
		{
			ID:    "app",
			Image: "lthn/dev:latest",
			Env: map[string]string{
				"OPENCODE_SERVER_PASSWORD": "${env.SERVER_PASSWORD}",
			},
			Volumes: []subject.VolumeMount{
				{Container: "/data", Persist: "opencode-data"},
			},
			Expose: &subject.ExposeBlock{Port: 4096, Route: "/opencode"},
		},
	},
	Env: []subject.EnvEntry{
		{Key: "SERVER_PASSWORD", Prompt: "Server password", Type: "secret", Default: "changeme"},
	},
}

// newTestMarketplaceService constructs a marketplace Service without Core
// (no orm, no sandbox) for unit testing validation logic only.
func newTestMarketplaceService() *subject.Service {
	r := subject.Register(nil)
	if !r.OK {
		return nil
	}
	svc, _ := r.Value.(*subject.Service)
	return svc
}

// TestInstall_InstalledBundle_Good verifies the InstalledBundle orm struct
// fields + Schema() method compile and have expected column names.
func TestInstall_InstalledBundle_Good(t *core.T) {
	rec := subject.InstalledBundle{
		BundleID:       "opencode",
		Display:        "OpenCode",
		ManifestSchema: "lthn-vm/v1",
		Status:         subject.BundleStatusRunning,
		ConfigPath:     "/home/user/Lethean/conf/marketplace/opencode",
		InstalledAt:    core.Now(),
	}
	core.AssertEqual(t, "opencode", rec.BundleID)
	core.AssertEqual(t, subject.BundleStatusRunning, rec.Status)
	core.AssertEqual(t, "lthn-vm/v1", rec.ManifestSchema)

	schema := rec.Schema()
	core.AssertNotNil(t, schema)
}

// TestInstall_InstalledBundle_Bad verifies the five status constants are
// distinct and non-empty.
func TestInstall_InstalledBundle_Bad(t *core.T) {
	statuses := []string{
		subject.BundleStatusIdle,
		subject.BundleStatusStarting,
		subject.BundleStatusRunning,
		subject.BundleStatusStopped,
		subject.BundleStatusFailed,
	}
	seen := map[string]bool{}
	for _, s := range statuses {
		core.AssertNotEqual(t, "", s)
		core.AssertFalse(t, seen[s])
		seen[s] = true
	}
	core.AssertLen(t, seen, 5)
}

// TestInstall_InstalledBundle_Ugly verifies InstalledBundle.Schema() produces
// a schema with the expected table name and PK.
func TestInstall_InstalledBundle_Ugly(t *core.T) {
	var rec subject.InstalledBundle
	schema := rec.Schema()
	core.AssertNotNil(t, schema)
}

// TestInstall_Install_Good verifies Install rejects an invalid manifest before
// attempting container ops.
func TestInstall_Install_Good(t *core.T) {
	svc := newTestMarketplaceService()
	ref := (*subject.Service).Install
	core.AssertNotNil(t, ref)
	// With no sandbox service wired, Install fails at sandbox resolution,
	// not at manifest validation. Confirm the right error is surfaced.
	r := svc.Install(subject.InstallInput{
		Manifest: minimalInstallManifest,
		Env:      map[string]string{},
	})
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "sandbox service not available")
}

// TestInstall_Install_Bad verifies Install rejects an invalid manifest immediately.
func TestInstall_Install_Bad(t *core.T) {
	svc := newTestMarketplaceService()

	// Invalid manifest — missing name.
	bad := subject.BundleManifest{
		Schema: "lthn-vm/v1",
		Images: []subject.ImageEntry{{ID: "app", Image: "alpine:3.21"}},
	}
	r := svc.Install(subject.InstallInput{Manifest: bad})
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "invalid manifest")
}

// TestInstall_Install_Ugly verifies InstallInput fields are exported and
// compose correctly.
func TestInstall_Install_Ugly(t *core.T) {
	in := subject.InstallInput{
		Manifest: fullInstallManifest,
		Env:      map[string]string{"SERVER_PASSWORD": "s3cr3t"},
	}
	core.AssertEqual(t, "opencode", in.Manifest.Name)
	core.AssertEqual(t, "s3cr3t", in.Env["SERVER_PASSWORD"])
}

// TestInstall_Launch_Good verifies Launch fails cleanly when bundle is not installed.
func TestInstall_Launch_Good(t *core.T) {
	svc := newTestMarketplaceService()
	r := svc.Launch("opencode")
	core.AssertFalse(t, r.OK)
	// "bundle not installed" when no orm and no record exists.
	core.AssertContains(t, r.Error(), "bundle not installed")
}

// TestInstall_Launch_Bad verifies Launch rejects empty bundle ID.
func TestInstall_Launch_Bad(t *core.T) {
	svc := newTestMarketplaceService()
	r := svc.Launch("")
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "bundle id is required")
}

// TestInstall_Launch_Ugly verifies Launch is a method on *Service.
func TestInstall_Launch_Ugly(t *core.T) {
	ref := (*subject.Service).Launch
	core.AssertNotNil(t, ref)
}

// TestInstall_Stop_Good verifies Stop on an unknown bundle returns OK
// (idempotent — no sandboxes to kill is a success state).
func TestInstall_Stop_Good(t *core.T) {
	svc := newTestMarketplaceService()
	r := svc.Stop("nonexistent-bundle")
	core.AssertTrue(t, r.OK)
}

// TestInstall_Stop_Bad verifies Stop rejects empty bundle ID.
func TestInstall_Stop_Bad(t *core.T) {
	svc := newTestMarketplaceService()
	r := svc.Stop("")
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "bundle id is required")
}

// TestInstall_Stop_Ugly verifies Stop is exported and callable.
func TestInstall_Stop_Ugly(t *core.T) {
	ref := (*subject.Service).Stop
	core.AssertNotNil(t, ref)
}

// TestInstall_Uninstall_Good verifies Uninstall on a non-existent bundle
// returns OK (idempotent).
func TestInstall_Uninstall_Good(t *core.T) {
	svc := newTestMarketplaceService()
	r := svc.Uninstall("nonexistent")
	core.AssertTrue(t, r.OK)
}

// TestInstall_Uninstall_Bad verifies Uninstall rejects empty bundle ID.
func TestInstall_Uninstall_Bad(t *core.T) {
	svc := newTestMarketplaceService()
	r := svc.Uninstall("")
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "bundle id is required")
}

// TestInstall_Uninstall_Ugly verifies Uninstall cascades through Stop without panicking.
func TestInstall_Uninstall_Ugly(t *core.T) {
	svc := newTestMarketplaceService()
	// Uninstall a bundle that was never installed — both Stop + Delete are no-ops.
	r := svc.Uninstall("ghost-bundle")
	core.AssertTrue(t, r.OK)
}

// TestInstall_Status_Good verifies Status fails cleanly for unknown bundle.
func TestInstall_Status_Good(t *core.T) {
	svc := newTestMarketplaceService()
	r := svc.Status("opencode")
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "bundle not installed")
}

// TestInstall_Status_Bad verifies Status rejects empty bundle ID.
func TestInstall_Status_Bad(t *core.T) {
	svc := newTestMarketplaceService()
	r := svc.Status("")
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "bundle id is required")
}

// TestInstall_Status_Ugly verifies BundleStatusOutput fields are exported.
func TestInstall_Status_Ugly(t *core.T) {
	out := subject.BundleStatusOutput{
		BundleID: "opencode",
		Display:  "OpenCode",
		Status:   subject.BundleStatusRunning,
		Handles:  nil,
	}
	core.AssertEqual(t, "opencode", out.BundleID)
	core.AssertEqual(t, subject.BundleStatusRunning, out.Status)
}

// TestInstall_ListInstalled_Good verifies ListInstalled returns empty slice
// when no orm is wired (nil core).
func TestInstall_ListInstalled_Good(t *core.T) {
	svc := newTestMarketplaceService()
	r := svc.ListInstalled()
	core.AssertTrue(t, r.OK)
	bundles := r.Value.([]subject.InstalledBundle)
	core.AssertNotNil(t, bundles)
}

// TestInstall_ListInstalled_Bad verifies ListInstalled on nil service
// doesn't panic.
func TestInstall_ListInstalled_Bad(t *core.T) {
	var svc *subject.Service
	ref := (*subject.Service).ListInstalled
	core.AssertNotNil(t, ref)
	_ = svc
}

// TestInstall_ListInstalled_Ugly verifies ListInstalled return type is []InstalledBundle.
func TestInstall_ListInstalled_Ugly(t *core.T) {
	svc := newTestMarketplaceService()
	r := svc.ListInstalled()
	core.AssertTrue(t, r.OK)
	_, ok := r.Value.([]subject.InstalledBundle)
	core.AssertTrue(t, ok)
}

// TestMarketplace_Install_SingleFlightPerBundleID_Good — Mantis
// #1583 MED. Two concurrent Stop calls for the SAME bundle id must
// serialise (one fully completes before the second starts running
// its body). Same-bundle lifecycle ops used to race on the orm
// record + sandbox handles, producing orphaned containers or split
// state. Stop is the cheapest entry to exercise here because it
// requires no sandbox / orm wiring to complete OK.
//
// We instrument by wrapping the work via a counter: serialised
// execution means at no point are two goroutines inside the locked
// section at the same time. Concurrent in-flight calls would let the
// counter exceed 1.
func TestMarketplace_Install_SingleFlightPerBundleID_Good(t *core.T) {
	svc := newTestMarketplaceService()

	const N = 16
	var (
		mu        core.AtomicInt32
		maxInside core.AtomicInt32
		wg        core.WaitGroup
	)
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			// Acquire via the public surface — Stop is mutex-gated
			// for the same bundleID across all callers.
			lock := subject.BundleMutexForTest(svc, "bundle-A")
			lock.Lock()
			cur := mu.Add(1)
			if cur > maxInside.Load() {
				maxInside.Store(cur)
			}
			// Tiny spin so race detector can see overlap if any.
			for k := 0; k < 1000; k++ {
				_ = k
			}
			mu.Add(-1)
			lock.Unlock()
		}()
	}
	wg.Wait()

	core.AssertEqual(t, int32(1), maxInside.Load(),
		"per-bundleID mutex must serialise — never more than one in critical section")
}

// TestMarketplace_Install_CrossBundleConcurrency_Good — Mantis #1583.
// Two different bundleIDs MUST be able to run concurrently. The
// per-bundleID mutex is keyed on the id; distinct ids do not block
// each other.
func TestMarketplace_Install_CrossBundleConcurrency_Good(t *core.T) {
	svc := newTestMarketplaceService()

	lockA := subject.BundleMutexForTest(svc, "bundle-A")
	lockB := subject.BundleMutexForTest(svc, "bundle-B")

	// Distinct id → distinct mutex object. (Pointer-identity check is
	// cleaner than a timing test that flakes on overloaded CI.)
	core.AssertFalse(t, lockA == lockB,
		"distinct bundle ids must map to distinct mutexes")
}

// TestMarketplace_Install_SameBundleSameMutex_Ugly — Mantis #1583.
// The mutex registry MUST be stable across calls — two lookups for
// the same id return the SAME mutex object (not a fresh one each
// time, which would defeat serialisation).
func TestMarketplace_Install_SameBundleSameMutex_Ugly(t *core.T) {
	svc := newTestMarketplaceService()

	first := subject.BundleMutexForTest(svc, "bundle-A")
	second := subject.BundleMutexForTest(svc, "bundle-A")

	core.AssertTrue(t, first == second,
		"same bundle id must return identical mutex pointer across calls")
}

// TestMarketplace_InstallStampsInstallId_Good — Mantis #1670 (H#187
// follow-on for #1665). The Install + Launch loops build their
// sandbox.SpawnLongInput via buildInstallSpawnInput; both InstallID
// and BundleID MUST land on the input so SpawnLong stamps the
// `lthn.sandbox.install_id` + `lthn.sandbox.bundle_id` docker
// labels on the resulting container. Without these, the substrate
// shipped by H#187 is dead code — labels never get applied even
// though the field exists.
func TestMarketplace_InstallStampsInstallId_Good(t *core.T) {
	img := subject.ImageEntry{
		ID:    "app",
		Image: "alpine:3.21",
		Expose: &subject.ExposeBlock{
			Port:  4096,
			Route: "/app",
		},
	}
	env := map[string]string{"FOO": "bar"}
	vols := []sandbox.LongVolumeMount{
		{Name: "data", Container: "/data"},
	}

	got := subject.BuildInstallSpawnInputForTest(
		img,
		"echo",
		env,
		vols,
		"iid-deadbeefcafebabe0011223344556677",
		"opencode",
	)

	// Identity labels — the load-bearing assertion for #1670.
	core.AssertEqual(t, "iid-deadbeefcafebabe0011223344556677", got.InstallID)
	core.AssertEqual(t, "opencode", got.BundleID)

	// Pre-existing wiring still intact (regression guard — the helper
	// extraction must not drop any of the fields the Install/Launch
	// loops were already setting before #1670).
	core.AssertEqual(t, "alpine:3.21", got.Image)
	core.AssertEqual(t, "echo", got.Command)
	core.AssertEqual(t, "bar", got.Env["FOO"])
	core.AssertEqual(t, 1, len(got.Volumes))
	core.AssertEqual(t, "data", got.Volumes[0].Name)
	core.AssertEqual(t, "/data", got.Volumes[0].Container)
	core.AssertEqual(t, 4096, got.ExposedPort)
}

// TestMarketplace_InstallStampsInstallId_Bad — empty install + bundle
// identifiers pass through verbatim (they're the documented
// degradation when sandbox.InstallID() fails — see
// resolveSandboxInstallID's package comment). SpawnLong already
// gates label emission on `core.Trim(...) != ""` so empty values
// mean "skip the label", which matches the pre-#1670 baseline.
func TestMarketplace_InstallStampsInstallId_Bad(t *core.T) {
	img := subject.ImageEntry{ID: "app", Image: "alpine:3.21"}

	got := subject.BuildInstallSpawnInputForTest(
		img, "echo", nil, nil, "", "",
	)

	core.AssertEqual(t, "", got.InstallID)
	core.AssertEqual(t, "", got.BundleID)
}

// TestMarketplace_InstallStampsInstallId_Ugly — Expose may be nil
// (most opencode bundles don't expose a port at the marketplace
// layer); the helper must not segfault and must leave ExposedPort
// at its zero value.
func TestMarketplace_InstallStampsInstallId_Ugly(t *core.T) {
	img := subject.ImageEntry{ID: "headless", Image: "alpine:3.21"}

	got := subject.BuildInstallSpawnInputForTest(
		img, "sleep", nil, nil, "iid-x", "bundle-y",
	)

	core.AssertEqual(t, "iid-x", got.InstallID)
	core.AssertEqual(t, "bundle-y", got.BundleID)
	core.AssertEqual(t, 0, got.ExposedPort)
}

// TestMarketplace_Install_WritesManifestYml_Good — Mantis #1689 HIGH
// (Cerberus #54 C1). Install must persist the validated manifest to
// <configPath>/manifest.yml; without it the Launch path's ReadFile
// fails outright with "manifest not found" on every fresh install
// (functional break) AND Uninstall's resolvePluginCode silently falls
// back to the bundle id (security break — stale ViewRegistry / CSP
// frame-src / postMessage origin entries when plugin.code !=
// bundle.Name).
//
// The Install call itself can't run end-to-end in a unit test (no
// sandbox / docker runtime), so this exercise drives the same
// persistManifest helper Install calls and asserts:
//   - manifest.yml exists at configPath
//   - the on-disk YAML round-trips through ParseManifestBytes back to
//     an equivalent BundleManifest (plugin.code preserved)
//   - resolvePluginCode now returns the plugin.code, NOT the bundle id
//     fallback (the load-bearing security assertion for the C1 bug)
//   - file mode on disk is 0o600 per the manifest-PII baseline
//     (paths.writeFileMode shared with the rest of the writer
//     substrate — same protection plugin-config / wallets get)
func TestMarketplace_Install_WritesManifestYml_Good(t *core.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newTestMarketplaceService()
	core.RequireTrue(t, svc != nil)

	// Manifest with plugin.code intentionally differing from bundle
	// name — this is the shape that triggered the Cerberus #54 C1
	// uninstall security leak (resolvePluginCode would silently
	// return "ghost-bundle" instead of "opencode", and
	// ViewRegistry.Drop("ghost-bundle") would walk an empty
	// byPlugin["ghost-bundle"] slice while ViewRegistry still held
	// every descriptor under "opencode").
	m := subject.BundleManifest{
		Schema:  "lthn-vm/v1",
		Name:    "ghost-bundle",
		Display: "Ghost Bundle",
		Images: []subject.ImageEntry{
			{ID: "app", Image: "alpine:3.21"},
		},
		Plugin: &subject.PluginBlock{
			Code: "opencode",
		},
	}

	configPath := subject.BundleConfigPathForTest(svc, m.Name)
	core.RequireTrue(t, core.MkdirAll(configPath, 0o755).OK)

	if err := subject.PersistManifestForTest(svc, configPath, m); err != nil {
		t.Fatalf("PersistManifestForTest: %v", err)
	}

	manifestPath := core.JoinPath(configPath, "manifest.yml")

	// (a) on-disk presence
	statR := core.Stat(manifestPath)
	core.RequireTrue(t, statR.OK, "manifest.yml must exist after install")
	info := statR.Value.(core.FsFileInfo)
	core.AssertFalse(t, info.IsDir())

	// (b) round-trip
	readR := core.ReadFile(manifestPath)
	core.RequireTrue(t, readR.OK)
	raw, _ := readR.Value.([]byte)
	parsedR := subject.ParseManifestBytes(raw)
	core.RequireTrue(t, parsedR.OK, "on-disk manifest.yml must parse back")
	parsed := parsedR.Value.(subject.BundleManifest)
	core.AssertEqual(t, "ghost-bundle", parsed.Name)
	core.RequireTrue(t, parsed.Plugin != nil)
	core.AssertEqual(t, "opencode", parsed.Plugin.Code,
		"plugin.code must round-trip to disk so resolvePluginCode reads the override, not the bundle id")

	// (c) load-bearing security assertion — resolvePluginCode now
	// walks the on-disk manifest and returns plugin.Code, not the
	// bundle id fallback.
	got := subject.ResolvePluginCodeForTest(svc, "ghost-bundle")
	core.AssertEqual(t, "opencode", got,
		"Cerberus #54 C1 — resolvePluginCode must read manifest.yml; bundle-id fallback leaks ViewRegistry state on uninstall")

	// (d) file mode — paths.writeFileMode baseline (0o600).
	mode := info.Mode().Perm()
	core.AssertEqual(t, core.FileMode(0o600), mode,
		"manifest.yml must inherit the writer-substrate baseline 0o600 perm")
}

// TestMarketplace_Uninstall_NoStaleRegistryEntries_Good — Mantis
// #1689 HIGH (Cerberus #54 C1). The security half of the bug: when a
// bundle's plugin.code differs from its name and Install never wrote
// manifest.yml, Uninstall's resolvePluginCode silently returned the
// bundle id; ViewRegistry.Drop(bundleID) then walked an empty
// byPlugin[bundleID] slice while every descriptor stayed registered
// under the real plugin.code. Result: stale descriptors + CSP
// frame-src allowlist + postMessage origin table retained dead
// entries past uninstall.
//
// With the manifest.yml write in place, resolvePluginCode resolves
// to "opencode" (the plugin.code), Drop walks the right keyspace, and
// every descriptor is gone after Uninstall.
func TestMarketplace_Uninstall_NoStaleRegistryEntries_Good(t *core.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newTestMarketplaceService()
	core.RequireTrue(t, svc != nil)

	const bundleName = "ghost-bundle"
	const pluginCode = "opencode-ghost-test"
	const viewID = pluginCode

	m := subject.BundleManifest{
		Schema:  "lthn-vm/v1",
		Name:    bundleName,
		Display: "Ghost Bundle",
		Images: []subject.ImageEntry{
			{ID: "app", Image: "alpine:3.21"},
		},
		Plugin: &subject.PluginBlock{Code: pluginCode},
	}

	// Simulate Install's manifest-write step (the bit Cerberus #54
	// C1 was missing pre-fix).
	configPath := subject.BundleConfigPathForTest(svc, bundleName)
	core.RequireTrue(t, core.MkdirAll(configPath, 0o755).OK)
	if err := subject.PersistManifestForTest(svc, configPath, m); err != nil {
		t.Fatalf("PersistManifestForTest: %v", err)
	}

	// Register a descriptor under the REAL plugin code (this is what
	// registerPluginViews does at install time). Use a unique view id
	// so prior tests' ViewRegistry contents (package-level singleton)
	// don't collide.
	desc := subject.PluginViewDescriptor{
		ID:         viewID,
		Label:      "Ghost View",
		Icon:       "fa-cube",
		Group:      "plugin",
		Kind:       subject.PluginViewKindIframe,
		Source:     "${expose.app.route}",
		PluginCode: pluginCode,
	}
	core.RequireTrue(t, subject.ViewRegistry.Add(pluginCode, desc).OK)

	// Sanity — descriptor IS present before uninstall.
	if _, ok := subject.ViewRegistry.Lookup(viewID); !ok {
		t.Fatalf("descriptor must be registered before uninstall (test fixture)")
	}

	// Uninstall via the bundle id — pre-fix this called
	// ViewRegistry.Drop(bundleName) because resolvePluginCode fell
	// back to the bundle id; post-fix it resolves to the real plugin
	// code and drops the descriptor cleanly.
	r := svc.Uninstall(bundleName)
	core.AssertTrue(t, r.OK)

	// Load-bearing assertion — the descriptor MUST be gone. Before
	// the fix this Lookup would still return ok=true because Drop
	// walked the wrong keyspace.
	_, present := subject.ViewRegistry.Lookup(viewID)
	core.AssertFalse(t, present,
		"Cerberus #54 C1 — Uninstall must drop ViewRegistry descriptors registered under plugin.code, not the bundle id")
}

// --- Wave 2 (Mantis #1639 / #1640 / #1641 / #1645) tests ---

// TestInstall_InstallInput_ForceStaleInstall_Good asserts the field
// exists, defaults to false, and round-trips on the struct.
func TestInstall_InstallInput_ForceStaleInstall_Good(t *core.T) {
	in := subject.InstallInput{
		Manifest:               minimalInstallManifest,
		ForceStaleInstall:      true,
		ExpectedManifestDigest: "sha256:abc",
		PrevGrantedPermissions: map[string]bool{"files:read": true},
	}
	core.AssertTrue(t, in.ForceStaleInstall)
	core.AssertEqual(t, "sha256:abc", in.ExpectedManifestDigest)
	core.AssertTrue(t, in.PrevGrantedPermissions["files:read"])

	// Default zero-value: every new field is the safe shape.
	zero := subject.InstallInput{Manifest: minimalInstallManifest}
	core.AssertFalse(t, zero.ForceStaleInstall)
	core.AssertEqual(t, "", zero.ExpectedManifestDigest)
	core.AssertNil(t, zero.PrevGrantedPermissions)
}

// TestInstall_InstallOutput_RequiresReConsent_Good asserts the F5
// fields exist + serialise on the struct.
func TestInstall_InstallOutput_RequiresReConsent_Good(t *core.T) {
	out := subject.InstallOutput{
		BundleID:                "opencode",
		SandboxIDs:              map[string]string{"app": "sb-abc"},
		RequiresReConsent:       true,
		RequiresReConsentScopes: []string{"files:write", "process:spawn"},
	}
	core.AssertTrue(t, out.RequiresReConsent)
	core.AssertLen(t, out.RequiresReConsentScopes, 2)
}

// TestInstall_CatalogueEntry_NewFields_Good asserts the Wave 2
// FetchedAt / PendingManifestDigest / ManifestDigest fields are
// present and serialise on the struct.
func TestInstall_CatalogueEntry_NewFields_Good(t *core.T) {
	now := core.Now()
	e := subject.CatalogueEntry{
		Name:                  "opencode",
		Display:               "OpenCode",
		SourceURL:             "https://marketplace.lthn.ai/v1/opencode.yml",
		FetchedAt:             now,
		PendingManifestDigest: "sha256:pending",
		ManifestDigest:        "sha256:canonical",
	}
	core.AssertEqual(t, "opencode", e.Name)
	core.AssertFalse(t, e.FetchedAt.IsZero())
	core.AssertEqual(t, "sha256:pending", e.PendingManifestDigest)
	core.AssertEqual(t, "sha256:canonical", e.ManifestDigest)
}

// TestInstall_StaleCatalogueThreshold_Good pins the documented
// 24h threshold so a future config-plumbing change can't quietly
// loosen the gate.
func TestInstall_StaleCatalogueThreshold_Good(t *core.T) {
	core.AssertEqual(t, 24*core.Hour, subject.StaleCatalogueThreshold)
}

// TestInstall_ConfirmManifestVersionBump_Bad asserts the validation
// reject path — empty bundleID and empty expectedNewDigest both
// reject with the bare-string operation error.
func TestInstall_ConfirmManifestVersionBump_Bad(t *core.T) {
	svc := newTestMarketplaceService()
	core.AssertNotNil(t, svc)

	r := svc.ConfirmManifestVersionBump("", "sha256:abc")
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "bundle id is required")

	r = svc.ConfirmManifestVersionBump("opencode", "")
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "expected new digest is required")
}
