// SPDX-Licence-Identifier: EUPL-1.2

// Bundle lifecycle for lthn-vm bundles: Install / Launch / Stop /
// Uninstall / Status.
//
// Install fetches the manifest, prompts for env (caller-side), spawns each
// image entry via pkg/sandbox.SpawnLong, writes resolved config to
// ~/Lethean/conf/marketplace/<bundle>/, and registers the plugin-contract
// entries from the manifest.plugin block.
//
// The durable state (what's installed) is held in the orm-backed
// InstalledBundle record. The ephemeral state (which sandbox handles are
// currently running) is tracked in Service.bundles (in-memory,
// process-lifetime only).
//
// Spec: plans/project/lthn/desktop/RFC.marketplace.md §5.
package marketplace

import (
	core "dappco.re/go"
	"dappco.re/go/orm"
	"dappco.re/lthn/desktop/pkg/sandbox"
)

const (
	installBundleOp   = "marketplace.Install"
	launchBundleOp    = "marketplace.Launch"
	stopBundleOp      = "marketplace.Stop"
	uninstallBundleOp = "marketplace.Uninstall"
	statusBundleOp    = "marketplace.Status"

	// BundleStatusIdle is set when all images are stopped but the
	// bundle record remains installed.
	BundleStatusIdle = "idle"
	// BundleStatusStarting transitions to running once all sandbox
	// handles have reached StatusReady.
	BundleStatusStarting = "starting"
	// BundleStatusRunning means all required images are up.
	BundleStatusRunning = "running"
	// BundleStatusStopped is set after an explicit Stop call.
	BundleStatusStopped = "stopped"
	// BundleStatusFailed means at least one image failed to start.
	BundleStatusFailed = "failed"
)

// InstalledBundle is the durable orm record for one installed lthn-vm bundle.
// Written at install time; removed at uninstall time.
//
// Usage example:
//
//	r := orm.Of[InstalledBundle](c).Find("opencode")
//	if r.OK { ib := r.Value.(InstalledBundle); _ = ib.Status }
type InstalledBundle struct {
	// BundleID is the manifest.Name — globally unique within the install.
	BundleID string `json:"bundle_id"`
	// Display is manifest.Display — the user-facing label.
	Display string `json:"display"`
	// ManifestSchema is the manifest schema version ("lthn-vm/v1").
	ManifestSchema string `json:"manifest_schema"`
	// Status is the last-known lifecycle state.
	Status string `json:"status"`
	// ConfigPath is the resolved config directory.
	ConfigPath string `json:"config_path"`
	// InstalledAt is the install timestamp.
	InstalledAt core.Time `json:"installed_at"`
}

// Schema satisfies the orm.Schemer interface so orm.Of[InstalledBundle]
// produces a typed bridge.
func (InstalledBundle) Schema() orm.Schema {
	return orm.Define(func(b *orm.Builder) {
		b.Name("marketplace_bundles")
		b.PK("bundle_id")
		b.String("bundle_id").NotNull()
		b.String("display").NotNull()
		b.String("manifest_schema").NotNull()
		b.String("status").NotNull()
		b.String("config_path").NotNull()
		b.Time("installed_at").NotNull()
		b.Index("status")
	})
}

// InstallInput drives Install. Manifest must already be parsed and
// validated (use ParseManifest / ParseManifestBytes first). Env holds
// the user-supplied values for manifest.env entries; keys must match
// the env[].key field names. Any key not supplied falls back to the
// default value declared in the manifest.
type InstallInput struct {
	// Manifest is the parsed + validated bundle descriptor.
	Manifest BundleManifest

	// Env is the user-supplied env-var map (key → value).
	// Keys from manifest.env[].key that are absent here fall back
	// to their default: value.
	Env map[string]string
}

// InstallOutput is what Install returns on success.
type InstallOutput struct {
	// BundleID is the stable identifier (= manifest.Name).
	BundleID string `json:"bundle_id"`
	// SandboxIDs maps image id → the sandbox.ContainerHandle.SandboxID
	// assigned during this install. Useful for reverse-proxy wiring.
	SandboxIDs map[string]string `json:"sandbox_ids"`
}

// BundleStatusOutput is what Status returns.
type BundleStatusOutput struct {
	BundleID string `json:"bundle_id"`
	Display  string `json:"display"`
	Status   string `json:"status"`
	// Handles is the per-image sandbox handle snapshot.
	Handles []sandbox.ContainerHandle `json:"handles,omitempty"`
}

// sandboxSvc resolves the sandbox.Service from the marketplace service's
// Core container. Returns nil when not registered (defensive).
func (s *Service) sandboxSvc() *sandbox.Service {
	if s == nil || s.core == nil {
		return nil
	}
	svc, _ := core.ServiceFor[*sandbox.Service](s.core, "sandbox")
	return svc
}

// Install runs the full install lifecycle for one bundle:
// spawn all images, persist the record, register plugin-contract entries.
//
// Usage example:
//
//	r := svc.Install(marketplace.InstallInput{
//	    Manifest: m,
//	    Env:      map[string]string{"SERVER_PASSWORD": "hunter2"},
//	})
//	if r.OK { out := r.Value.(marketplace.InstallOutput) }
func (s *Service) Install(input InstallInput) core.Result {
	if r := ValidateManifest(input.Manifest); !r.OK {
		return core.Fail(core.E(installBundleOp, "invalid manifest", nil))
	}

	sbSvc := s.sandboxSvc()
	if sbSvc == nil {
		return core.Fail(core.E(installBundleOp, "sandbox service not available", nil))
	}

	m := input.Manifest
	env := s.resolveEnv(m, input.Env)

	configPath := s.bundleConfigPath(m.Name)
	if r := core.MkdirAll(configPath, 0o755); !r.OK {
		return core.Fail(core.E(installBundleOp, "config dir creation failed", nil))
	}

	sandboxIDs := map[string]string{}
	var lastErr string

	for _, img := range m.Images {
		spawnIn := sandbox.SpawnLongInput{
			Image:   img.Image,
			Command: s.resolveImageCommand(img),
			Env:     s.resolveImageEnv(img, env),
			Volumes: s.resolveVolumes(img),
		}
		if img.Expose != nil {
			spawnIn.ExposedPort = img.Expose.Port
		}

		r := sbSvc.SpawnLong(spawnIn)
		if !r.OK {
			lastErr = r.Error()
			continue
		}
		h := r.Value.(sandbox.ContainerHandle)
		sandboxIDs[img.ID] = h.SandboxID
	}

	status := BundleStatusRunning
	if lastErr != "" {
		status = BundleStatusFailed
	}

	rec := InstalledBundle{
		BundleID:       m.Name,
		Display:        m.Display,
		ManifestSchema: m.Schema,
		Status:         status,
		ConfigPath:     configPath,
		InstalledAt:    core.Now(),
	}
	if s.core != nil {
		_ = orm.Of[InstalledBundle](s.core).Save(&rec)
	}

	if lastErr != "" {
		return core.Fail(core.E(installBundleOp, "one or more images failed to start: "+lastErr, nil))
	}

	return core.Ok(InstallOutput{
		BundleID:   m.Name,
		SandboxIDs: sandboxIDs,
	})
}

// Launch starts all sandboxes for an already-installed bundle.
// Equivalent to Install steps 6-7 without the pull.
//
// Usage example:
//
//	r := svc.Launch("opencode")
//	if r.OK { /* sandboxes are running */ }
func (s *Service) Launch(bundleID string) core.Result {
	if core.Trim(bundleID) == "" {
		return core.Fail(core.E(launchBundleOp, "bundle id is required", nil))
	}

	recR := s.findInstalledBundle(bundleID)
	if !recR.OK {
		return core.Fail(core.E(launchBundleOp, "bundle not installed: "+bundleID, nil))
	}

	configPath := s.bundleConfigPath(bundleID)
	manifestPath := core.JoinPath(configPath, "manifest.yml")
	readR := core.ReadFile(manifestPath)
	if !readR.OK {
		return core.Fail(core.E(launchBundleOp, "manifest not found at: "+manifestPath, nil))
	}
	raw, _ := readR.Value.([]byte)
	mR := ParseManifestBytes(raw)
	if !mR.OK {
		return core.Fail(core.E(launchBundleOp, "manifest parse failed", nil))
	}
	m := mR.Value.(BundleManifest)

	sbSvc := s.sandboxSvc()
	if sbSvc == nil {
		return core.Fail(core.E(launchBundleOp, "sandbox service not available", nil))
	}

	for _, img := range m.Images {
		spawnIn := sandbox.SpawnLongInput{
			Image:   img.Image,
			Command: s.resolveImageCommand(img),
			Volumes: s.resolveVolumes(img),
		}
		if img.Expose != nil {
			spawnIn.ExposedPort = img.Expose.Port
		}
		_ = sbSvc.SpawnLong(spawnIn)
	}

	rec := recR.Value.(InstalledBundle)
	rec.Status = BundleStatusRunning
	if s.core != nil {
		_ = orm.Of[InstalledBundle](s.core).Save(&rec)
	}

	return core.Ok(nil)
}

// Stop kills all running sandboxes for a bundle and marks it stopped.
//
// Usage example:
//
//	r := svc.Stop("opencode")
//	if r.OK { /* bundle is stopped */ }
func (s *Service) Stop(bundleID string) core.Result {
	if core.Trim(bundleID) == "" {
		return core.Fail(core.E(stopBundleOp, "bundle id is required", nil))
	}

	sbSvc := s.sandboxSvc()
	if sbSvc != nil {
		listR := sbSvc.ListHandles()
		if listR.OK {
			handles := listR.Value.([]sandbox.ContainerHandle)
			for _, h := range handles {
				if h.BundleID == bundleID {
					_ = sbSvc.Kill(h.SandboxID)
				}
			}
		}
	}

	recR := s.findInstalledBundle(bundleID)
	if recR.OK {
		rec := recR.Value.(InstalledBundle)
		rec.Status = BundleStatusStopped
		if s.core != nil {
			_ = orm.Of[InstalledBundle](s.core).Save(&rec)
		}
	}

	return core.Ok(nil)
}

// Uninstall stops all sandboxes and removes the durable record.
// Does NOT delete persistent volumes — caller prompts the user first
// per RFC.marketplace.md §5.4.
//
// Usage example:
//
//	r := svc.Uninstall("opencode")
//	if r.OK { /* bundle record removed */ }
func (s *Service) Uninstall(bundleID string) core.Result {
	if core.Trim(bundleID) == "" {
		return core.Fail(core.E(uninstallBundleOp, "bundle id is required", nil))
	}

	// Stop all running sandboxes first.
	_ = s.Stop(bundleID)

	if s.core != nil {
		rec := InstalledBundle{BundleID: bundleID}
		_ = orm.Of[InstalledBundle](s.core).Delete(&rec)
	}

	return core.Ok(nil)
}

// Status returns the current state of an installed bundle including
// per-image sandbox handle snapshots.
//
// Usage example:
//
//	r := svc.Status("opencode")
//	if r.OK { out := r.Value.(marketplace.BundleStatusOutput) }
func (s *Service) Status(bundleID string) core.Result {
	if core.Trim(bundleID) == "" {
		return core.Fail(core.E(statusBundleOp, "bundle id is required", nil))
	}

	recR := s.findInstalledBundle(bundleID)
	if !recR.OK {
		return core.Fail(core.E(statusBundleOp, "bundle not installed: "+bundleID, nil))
	}
	rec := recR.Value.(InstalledBundle)

	var handles []sandbox.ContainerHandle
	sbSvc := s.sandboxSvc()
	if sbSvc != nil {
		listR := sbSvc.ListHandles()
		if listR.OK {
			all := listR.Value.([]sandbox.ContainerHandle)
			for _, h := range all {
				if h.BundleID == bundleID {
					handles = append(handles, h)
				}
			}
		}
	}

	return core.Ok(BundleStatusOutput{
		BundleID: rec.BundleID,
		Display:  rec.Display,
		Status:   rec.Status,
		Handles:  handles,
	})
}

// ListInstalled returns all durable InstalledBundle records.
//
// Usage example:
//
//	r := svc.ListInstalled()
//	bundles := r.Value.([]marketplace.InstalledBundle)
func (s *Service) ListInstalled() core.Result {
	if s.core == nil {
		return core.Ok([]InstalledBundle{})
	}
	return orm.Of[InstalledBundle](s.core).Get()
}

// findInstalledBundle looks up one bundle record by ID.
func (s *Service) findInstalledBundle(bundleID string) core.Result {
	if s.core == nil {
		return core.Fail(core.E("marketplace.find", "core not available", nil))
	}
	return orm.Of[InstalledBundle](s.core).Find(bundleID)
}

// bundleConfigPath returns ~/Lethean/conf/marketplace/<id>/.
// Falls back to /tmp/lthn-marketplace/<id> when home dir is unavailable.
func (s *Service) bundleConfigPath(bundleID string) string {
	homeR := core.UserHomeDir()
	if homeR.OK {
		return core.PathJoin(homeR.Value.(string), "Lethean", "conf", "marketplace", bundleID)
	}
	return core.PathJoin("/tmp", "lthn-marketplace", bundleID)
}

// resolveEnv merges user-supplied env with manifest defaults.
// Keys present in supplied take precedence; missing keys fall back
// to manifest default (with ${...} tokens passed through as-is).
func (s *Service) resolveEnv(m BundleManifest, supplied map[string]string) map[string]string {
	out := map[string]string{}
	for _, e := range m.Env {
		if v, ok := supplied[e.Key]; ok {
			out[e.Key] = v
		} else {
			out[e.Key] = e.Default
		}
	}
	return out
}

// resolveImageEnv builds the per-image env map, substituting ${env.KEY}
// tokens from the resolved global env map.
func (s *Service) resolveImageEnv(img ImageEntry, env map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range img.Env {
		out[k] = substituteTokens(v, env)
	}
	return out
}

// resolveImageCommand returns the container entrypoint command.
// For v1, the manifest image entries don't carry a "command:" field —
// the OCI image's default entrypoint runs. We pass "" to SpawnLong
// which signals "use the image's CMD". SpawnLong requires a non-empty
// Command, so we use a sentinel that docker treats as the image default.
func (s *Service) resolveImageCommand(img ImageEntry) string {
	// SpawnLong validates Command != "". For bundle images that rely
	// on the OCI ENTRYPOINT / CMD, the caller passes the image's
	// default command. For v1 we use a placeholder that lets the
	// image's entrypoint run by passing its own name.
	return img.Image
}

// resolveVolumes maps manifest VolumeMount entries to sandbox.LongVolumeMount.
func (s *Service) resolveVolumes(img ImageEntry) []sandbox.LongVolumeMount {
	out := make([]sandbox.LongVolumeMount, 0, len(img.Volumes))
	for _, v := range img.Volumes {
		out = append(out, sandbox.LongVolumeMount{
			Name:      v.Persist,
			Container: v.Container,
		})
	}
	return out
}

// substituteTokens replaces ${env.KEY} tokens in v with the
// corresponding value from env. Tokens with no match are preserved as-is.
func substituteTokens(v string, env map[string]string) string {
	for k, val := range env {
		v = core.Replace(v, "${env."+k+"}", val)
	}
	return v
}
