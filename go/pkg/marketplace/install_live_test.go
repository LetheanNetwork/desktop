// SPDX-Licence-Identifier: EUPL-1.2

// install_live_test.go — Install / Launch / Stop / Status driven
// against a REAL sandbox.Service (github.com/.../pkg/sandbox,
// registered for real, no fake/mock substituted) + a REAL orm.Memium-
// backed InstalledBundle table. Mirrors pkg/bridge/sandbox_test.go's
// crib: "the container runtime is not present in CI so we assert the
// gate didn't short-circuit — any later failure shape is allowed."
// SpawnLong genuinely fails here because no docker/podman/apple
// container binary is on the test PATH; that failure is the real
// exec boundary, not a mock, and every line of Install/Launch logic
// BEFORE and AROUND that boundary (staleness/collision/digest gates,
// resolveEnv, persistManifest, orm.Save, plugin-view registration,
// event broadcast) runs for real.

package marketplace

import (
	"testing"

	core "dappco.re/go"
	"dappco.re/go/orm"
	"dappco.re/lthn/desktop/pkg/sandbox"
)

// newLiveMarketplaceCore builds a Core with a real (docker-absent)
// sandbox.Service registered under "sandbox" + a real orm.Memium
// backing the InstalledBundle table — everything Install/Launch/Stop
// need to run their full bodies hermetically.
func newLiveMarketplaceCore(t *testing.T) *core.Core {
	t.Helper()
	c := core.New(core.WithService(sandbox.Register))
	if r := orm.Register(c); !r.OK {
		t.Fatalf("orm.Register: %s", r.Error())
	}
	med := orm.NewMemium()
	if r := orm.Mount(c, "default", med); !r.OK {
		t.Fatalf("orm.Mount: %s", r.Error())
	}
	var rec InstalledBundle
	schema := rec.Schema()
	if r := orm.RegisterSchema(c, schema); !r.OK {
		t.Fatalf("orm.RegisterSchema: %s", r.Error())
	}
	med.RegisterTable(schema.Name, schema)
	if r := c.ServiceStartup(core.Background(), nil); !r.OK {
		t.Fatalf("ServiceStartup: %s", r.Error())
	}
	t.Cleanup(func() { _ = c.ServiceShutdown(core.Background()) })
	return c
}

func liveTestManifest(name string) BundleManifest {
	return BundleManifest{
		Schema:  manifestSchema,
		Name:    name,
		Display: "Live Test Bundle",
		Images: []ImageEntry{
			{ID: "app", Image: "alpine:3.21"},
		},
	}
}

// TestInstall_RealSandboxNoRuntime_SpawnFailsCleanly_Ugly — with a
// real sandbox.Service registered and no container runtime present,
// Install runs the full pipeline (collision gate, staleness gate —
// no catalogue entry so skipped, digest-verify skip since no @sha256
// ref, resolveEnv, MkdirAll, persistManifest, spawn loop) and lands
// on the "one or more images failed to start" branch — the real exec
// boundary, not a mock.
func TestInstall_RealSandboxNoRuntime_SpawnFailsCleanly_Ugly(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	c := newLiveMarketplaceCore(t)
	svc := NewService(c)

	m := liveTestManifest("live-bundle-a")
	r := svc.Install(InstallInput{Manifest: m})
	if r.OK {
		t.Fatal("expected Install to fail — no container runtime is present in this test environment")
	}
	if !core.Contains(r.Error(), "one or more images failed to start") {
		t.Errorf("expected the spawn-loop failure branch, got: %s", r.Error())
	}

	// The manifest MUST have been persisted to disk (Mantis #1689)
	// even though the spawn failed — Launch's manifest.yml read
	// depends on this regardless of spawn outcome.
	configPath := svc.bundleConfigPath("live-bundle-a")
	manifestPath := core.JoinPath(configPath, "manifest.yml")
	if statR := core.Stat(manifestPath); !statR.OK {
		t.Errorf("expected manifest.yml to be persisted despite spawn failure")
	}

	// The InstalledBundle record MUST have been saved with
	// BundleStatusFailed (orm.Save succeeds against the real Memium
	// even though the spawn loop failed).
	recR := orm.Of[InstalledBundle](c).Find("live-bundle-a")
	if !recR.OK {
		t.Fatalf("expected InstalledBundle record to be saved despite spawn failure: %s", recR.Error())
	}
	rec := recR.Value.(InstalledBundle)
	if rec.Status != BundleStatusFailed {
		t.Errorf("expected status=failed, got %q", rec.Status)
	}
}

// TestInstall_OrmNotWired_Bad — sandbox registered but orm never
// mounted: Install reaches the orm.Save call, it fails, and the
// rollback-then-fail branch (Mantis #1693) fires. Distinct fault
// injection from the "no runtime" test above — this fails BEFORE the
// spawn-loop's lastErr check even runs its course, at the orm layer.
func TestInstall_OrmNotWired_Bad(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	c := core.New(core.WithService(sandbox.Register)) // orm never registered
	svc := NewService(c)

	m := liveTestManifest("live-bundle-b")
	r := svc.Install(InstallInput{Manifest: m})
	if r.OK {
		t.Fatal("expected Install to fail when orm isn't wired")
	}
	if !core.Contains(r.Error(), "orm save failed") {
		t.Errorf("expected an orm-save failure, got: %s", r.Error())
	}
}

// TestLaunch_RealSandboxNoRuntime_StillSucceeds_Good — Launch's spawn
// loop is best-effort (unlike Install's lastErr tracking): even
// though every SpawnLong call fails (no runtime), Launch still
// updates the record to Running and returns Ok. Exercises the FULL
// Launch body including the manifest.yml read-back, orm.Save success,
// and the PhaseLaunched broadcast.
func TestLaunch_RealSandboxNoRuntime_StillSucceeds_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	c := newLiveMarketplaceCore(t)
	svc := NewService(c)

	m := liveTestManifest("live-bundle-c")
	installR := svc.Install(InstallInput{Manifest: m})
	if installR.OK {
		t.Fatal("setup: expected Install to fail on spawn (no runtime) as in the sibling test")
	}

	var events []BundleChanged
	Subscribe(c, func(_ *core.Core, ev BundleChanged) { events = append(events, ev) })

	r := svc.Launch("live-bundle-c")
	if !r.OK {
		t.Fatalf("Launch: %s", r.Error())
	}

	recR := orm.Of[InstalledBundle](c).Find("live-bundle-c")
	if !recR.OK {
		t.Fatalf("findInstalledBundle after Launch: %s", recR.Error())
	}
	rec := recR.Value.(InstalledBundle)
	if rec.Status != BundleStatusRunning {
		t.Errorf("expected status=running after Launch, got %q", rec.Status)
	}

	found := false
	for _, ev := range events {
		if ev.Phase == PhaseLaunched {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a PhaseLaunched BundleChanged broadcast")
	}
}

// TestLaunch_ManifestMissing_Bad — an orm record exists (Install's
// manifest-write step never ran) but manifest.yml is absent on disk.
func TestLaunch_ManifestMissing_Bad(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	c := newLiveMarketplaceCore(t)
	svc := NewService(c)

	rec := InstalledBundle{
		BundleID:       "orphan-bundle",
		Display:        "Orphan",
		ManifestSchema: manifestSchema,
		Status:         BundleStatusIdle,
		ConfigPath:     svc.bundleConfigPath("orphan-bundle"),
		InstalledAt:    core.Now(),
	}
	if r := orm.Of[InstalledBundle](c).Save(&rec); !r.OK {
		t.Fatalf("seed InstalledBundle: %s", r.Error())
	}

	r := svc.Launch("orphan-bundle")
	if r.OK {
		t.Fatal("expected Launch to fail when manifest.yml is missing")
	}
	if !core.Contains(r.Error(), "manifest not found") {
		t.Errorf("unexpected error: %s", r.Error())
	}
}

// TestLaunch_ManifestParseFailure_Bad — manifest.yml exists but is
// not valid YAML.
func TestLaunch_ManifestParseFailure_Bad(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	c := newLiveMarketplaceCore(t)
	svc := NewService(c)

	rec := InstalledBundle{
		BundleID:       "bad-manifest-bundle",
		Display:        "Bad Manifest",
		ManifestSchema: manifestSchema,
		Status:         BundleStatusIdle,
		ConfigPath:     svc.bundleConfigPath("bad-manifest-bundle"),
		InstalledAt:    core.Now(),
	}
	if r := orm.Of[InstalledBundle](c).Save(&rec); !r.OK {
		t.Fatalf("seed InstalledBundle: %s", r.Error())
	}
	configPath := svc.bundleConfigPath("bad-manifest-bundle")
	if r := core.MkdirAll(configPath, 0o755); !r.OK {
		t.Fatalf("mkdir configPath: %s", r.Error())
	}
	manifestPath := core.JoinPath(configPath, "manifest.yml")
	if r := core.WriteFile(manifestPath, []byte(": : : broken [yaml"), 0o600); !r.OK {
		t.Fatalf("write broken manifest.yml: %s", r.Error())
	}

	r := svc.Launch("bad-manifest-bundle")
	if r.OK {
		t.Fatal("expected Launch to fail parsing a broken manifest.yml")
	}
	if !core.Contains(r.Error(), "manifest parse failed") {
		t.Errorf("unexpected error: %s", r.Error())
	}
}

// TestStatus_RealSandbox_Good — Status resolves handles via the real
// (empty) sandbox handle list without needing a live container.
func TestStatus_RealSandbox_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	c := newLiveMarketplaceCore(t)
	svc := NewService(c)

	rec := InstalledBundle{
		BundleID:       "status-bundle",
		Display:        "Status Bundle",
		ManifestSchema: manifestSchema,
		Status:         BundleStatusRunning,
		ConfigPath:     svc.bundleConfigPath("status-bundle"),
		InstalledAt:    core.Now(),
	}
	if r := orm.Of[InstalledBundle](c).Save(&rec); !r.OK {
		t.Fatalf("seed InstalledBundle: %s", r.Error())
	}

	r := svc.Status("status-bundle")
	if !r.OK {
		t.Fatalf("Status: %s", r.Error())
	}
	out := r.Value.(BundleStatusOutput)
	if out.BundleID != "status-bundle" {
		t.Errorf("BundleID = %q, want status-bundle", out.BundleID)
	}
	if len(out.Handles) != 0 {
		t.Errorf("expected zero live handles (nothing was ever spawned), got %d", len(out.Handles))
	}
}

// TestCheckPluginCodeCollision_BundleCollision_Bad — a second bundle
// declaring the SAME effective plugin code as an already-installed
// bundle is rejected; re-installing the SAME bundle id is allowed.
func TestCheckPluginCodeCollision_BundleCollision_Bad(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	c := newLiveMarketplaceCore(t)
	svc := NewService(c)

	existing := InstalledBundle{
		BundleID:       "bundle-one",
		Display:        "Bundle One",
		ManifestSchema: manifestSchema,
		Status:         BundleStatusRunning,
		ConfigPath:     svc.bundleConfigPath("bundle-one"),
		InstalledAt:    core.Now(),
	}
	if r := orm.Of[InstalledBundle](c).Save(&existing); !r.OK {
		t.Fatalf("seed existing bundle: %s", r.Error())
	}
	// resolvePluginCode("bundle-one") falls back to "bundle-one" since
	// no manifest.yml is on disk for it.

	// A different bundle id claiming the SAME effective code collides.
	m := BundleManifest{
		Schema: manifestSchema, Name: "bundle-two",
		Plugin: &PluginBlock{Code: "bundle-one"},
	}
	r := svc.checkPluginCodeCollision(m)
	if r.OK {
		t.Fatal("expected a collision reject for a duplicate effective plugin code")
	}

	// Re-installing bundle-one itself (same bundle id) is NOT a collision.
	same := BundleManifest{Schema: manifestSchema, Name: "bundle-one"}
	r2 := svc.checkPluginCodeCollision(same)
	if !r2.OK {
		t.Errorf("expected re-install of the same bundle id to be allowed, got: %s", r2.Error())
	}
}

// TestCheckPluginCodeCollision_EmptyCode_Good — an empty effective
// plugin code (blank manifest Name AND no Plugin.Code) skips the gate
// entirely.
func TestCheckPluginCodeCollision_EmptyCode_Good(t *testing.T) {
	svc := &Service{}
	r := svc.checkPluginCodeCollision(BundleManifest{})
	if !r.OK {
		t.Errorf("expected an empty effective code to skip the gate, got: %s", r.Error())
	}
}

// --- pure / low-level Install helpers ---

func TestResolveEnv_MergesSuppliedAndDefaults_Good(t *testing.T) {
	svc := &Service{}
	m := BundleManifest{Env: []EnvEntry{
		{Key: "A", Default: "default-a"},
		{Key: "B", Default: "default-b"},
	}}
	got := svc.resolveEnv(m, map[string]string{"A": "supplied-a"})
	if got["A"] != "supplied-a" {
		t.Errorf("A = %q, want supplied-a", got["A"])
	}
	if got["B"] != "default-b" {
		t.Errorf("B = %q, want default-b", got["B"])
	}
}

func TestResolveImageEnv_SubstitutesTokens_Good(t *testing.T) {
	svc := &Service{}
	img := ImageEntry{Env: map[string]string{"PASS": "${env.SERVER_PASSWORD}"}}
	got := svc.resolveImageEnv(img, map[string]string{"SERVER_PASSWORD": "hunter2"})
	if got["PASS"] != "hunter2" {
		t.Errorf("PASS = %q, want hunter2", got["PASS"])
	}
}

func TestSubstituteTokens_NoMatchPreserved_Ugly(t *testing.T) {
	got := substituteTokens("${env.UNKNOWN}", map[string]string{"OTHER": "x"})
	if got != "${env.UNKNOWN}" {
		t.Errorf("got %q, want token preserved as-is", got)
	}
}

func TestResolveImageCommand_ReturnsImageRef_Good(t *testing.T) {
	svc := &Service{}
	got := svc.resolveImageCommand(ImageEntry{Image: "alpine:3.21"})
	if got != "alpine:3.21" {
		t.Errorf("got %q, want alpine:3.21", got)
	}
}

func TestResolveVolumes_InvalidVolumeName_Bad(t *testing.T) {
	svc := &Service{}
	_, r := svc.resolveVolumes(ImageEntry{
		Volumes: []VolumeMount{{Container: "/data", Persist: "/etc/passwd"}},
	})
	if r.OK {
		t.Fatal("expected resolveVolumes to reject a path-shaped volume name")
	}
}

func TestResolveVolumes_InvalidContainerPath_Bad(t *testing.T) {
	svc := &Service{}
	_, r := svc.resolveVolumes(ImageEntry{
		Volumes: []VolumeMount{{Container: "/data:ro,bind", Persist: "good-name"}},
	})
	if r.OK {
		t.Fatal("expected resolveVolumes to reject an option-injecting container path")
	}
}

func TestResolveVolumes_Valid_Good(t *testing.T) {
	svc := &Service{}
	got, r := svc.resolveVolumes(ImageEntry{
		Volumes: []VolumeMount{{Container: "/data", Persist: "app-data"}},
	})
	if !r.OK {
		t.Fatalf("resolveVolumes: %s", r.Error())
	}
	if len(got) != 1 || got[0].Name != "app-data" || got[0].Container != "/data" {
		t.Errorf("unexpected volumes: %+v", got)
	}
}

func TestResolveSandboxInstallID_NilPort_Good(t *testing.T) {
	svc := &Service{}
	if got := svc.resolveSandboxInstallID(nil); got != "" {
		t.Errorf("expected empty install id for a nil SpawnPort, got %q", got)
	}
}

func TestEncodeDecodePermissions_RoundTrip_Good(t *testing.T) {
	perms := []Permission{{Scope: "files", Mode: "read", Reason: "x"}}
	encoded := encodePermissions(perms)
	if encoded == "" {
		t.Fatal("expected non-empty encoding for a non-empty permission set")
	}
	decoded := DecodePermissions(encoded)
	if len(decoded) != 1 || decoded[0].Scope != "files" || decoded[0].Mode != "read" {
		t.Errorf("unexpected round-trip: %+v", decoded)
	}
}

func TestEncodePermissions_Empty_Good(t *testing.T) {
	if got := encodePermissions(nil); got != "" {
		t.Errorf("expected empty string for no permissions, got %q", got)
	}
}

func TestDecodePermissions_EmptyAndInvalid_Bad(t *testing.T) {
	if got := DecodePermissions(""); got != nil {
		t.Errorf("expected nil for empty input, got %+v", got)
	}
	if got := DecodePermissions("not json"); got != nil {
		t.Errorf("expected nil for invalid JSON, got %+v", got)
	}
}

func TestDiffNewPermissions_Ugly(t *testing.T) {
	perms := []Permission{
		{Scope: "files", Mode: "read"},
		{Scope: "process", Mode: "spawn"},
	}
	prev := map[string]bool{"files:read": true}
	got := diffNewPermissions(perms, prev)
	if len(got) != 1 || got[0] != "process:spawn" {
		t.Errorf("expected only the new scope, got %v", got)
	}

	// All-granted → nil, not an empty non-nil slice.
	allGranted := map[string]bool{"files:read": true, "process:spawn": true}
	if got := diffNewPermissions(perms, allGranted); got != nil {
		t.Errorf("expected nil when everything is already granted, got %v", got)
	}

	if got := diffNewPermissions(nil, prev); got != nil {
		t.Errorf("expected nil for an empty permissions manifest, got %v", got)
	}
}

func TestExpectedDigestFromRef_Ugly(t *testing.T) {
	cases := map[string]string{
		"alpine:3.21":                              "",
		"alpine@sha256:abc123":                     "sha256:abc123",
		"registry.example.com/img@sha256:deadbeef": "sha256:deadbeef",
	}
	for ref, want := range cases {
		if got := expectedDigestFromRef(ref); got != want {
			t.Errorf("expectedDigestFromRef(%q) = %q, want %q", ref, got, want)
		}
	}
}

func TestIconOrDefault_Ugly(t *testing.T) {
	if got := iconOrDefault(""); got != "fa-cube" {
		t.Errorf("got %q, want fa-cube", got)
	}
	if got := iconOrDefault("fa-terminal"); got != "fa-terminal" {
		t.Errorf("got %q, want fa-terminal", got)
	}
}

func TestExposeIDFromSource_Ugly(t *testing.T) {
	if got := exposeIDFromSource("${expose.app.route}"); got != "app" {
		t.Errorf("got %q, want app", got)
	}
	if got := exposeIDFromSource("not-a-placeholder"); got != "" {
		t.Errorf("expected empty for a non-placeholder source, got %q", got)
	}
}

func TestManifestToPluginInput_Ugly(t *testing.T) {
	m := BundleManifest{
		Images: []ImageEntry{{ID: "app", Expose: &ExposeBlock{Route: "/app"}}},
		Plugin: &PluginBlock{
			Routes:   []RouteEntry{{Title: "App", Target: "${expose.app.route}"}},
			Commands: []CommandEntry{{ID: "run", Title: "Run", Runs: "route:${expose.app.route}"}},
			Settings: []SettingEntry{{Key: "k", Type: "string", Prompt: "p"}},
		},
	}
	in := manifestToPluginInput(m)
	if in.Exposes["app"] != "/app" {
		t.Errorf("expected Exposes[app]=/app, got %q", in.Exposes["app"])
	}
	if len(in.Routes) != 1 || len(in.Commands) != 1 || len(in.Settings) != 1 {
		t.Errorf("expected routes/commands/settings to carry through, got %+v", in)
	}
}

func TestManifestToPluginInput_NoPluginBlock_Good(t *testing.T) {
	m := BundleManifest{Images: []ImageEntry{{ID: "app"}}}
	in := manifestToPluginInput(m)
	if len(in.Routes) != 0 || len(in.Commands) != 0 || len(in.Settings) != 0 {
		t.Errorf("expected all-empty when Plugin block is nil, got %+v", in)
	}
}

func TestIsInstalled_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	c := newLiveMarketplaceCore(t)
	svc := NewService(c)

	if svc.IsInstalled("nope") {
		t.Error("expected IsInstalled to be false when nothing is installed")
	}
	if svc.IsInstalled("") {
		t.Error("expected IsInstalled(\"\") to be false")
	}

	rec := InstalledBundle{
		BundleID:       "installed-bundle",
		Display:        "Installed Bundle",
		ManifestSchema: manifestSchema,
		Status:         BundleStatusRunning,
		ConfigPath:     svc.bundleConfigPath("installed-bundle"),
		InstalledAt:    core.Now(),
	}
	if r := orm.Of[InstalledBundle](c).Save(&rec); !r.OK {
		t.Fatalf("seed: %s", r.Error())
	}
	if !svc.IsInstalled("installed-bundle") {
		t.Error("expected IsInstalled to be true once a matching bundle is saved (fallback to bundle id)")
	}
}

func TestFindInstalledBundle_NilCore_Bad(t *testing.T) {
	svc := &Service{}
	r := svc.findInstalledBundle("x")
	if r.OK {
		t.Fatal("expected findInstalledBundle to fail with a nil core")
	}
}
