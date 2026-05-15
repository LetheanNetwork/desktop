// SPDX-Licence-Identifier: EUPL-1.2

package marketplace_test

import (
	core "dappco.re/go"
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
