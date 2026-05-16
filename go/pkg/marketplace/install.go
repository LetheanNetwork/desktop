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
	"dappco.re/lthn/desktop/pkg/plugin"
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
	// Permissions is the JSON-encoded []Permission the bundle declared
	// in its manifest.permissions block. Snapshot at install time;
	// pkg/gateway uses this to gate plugin requests at the API
	// boundary (per RFC.marketplace.md §7a). Stored as JSON-encoded
	// string so the orm column shape stays simple.
	Permissions string `json:"permissions,omitempty"`
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
		b.String("permissions")
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
		vols, vr := s.resolveVolumes(img)
		if !vr.OK {
			lastErr = vr.Error()
			continue
		}
		spawnIn := sandbox.SpawnLongInput{
			Image:   img.Image,
			Command: s.resolveImageCommand(img),
			Env:     s.resolveImageEnv(img, env),
			Volumes: vols,
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
		// Snapshot the manifest's permissions block as JSON so
		// pkg/gateway can read them back via CheckPermission without
		// re-fetching the manifest. Empty when the manifest declares
		// no permissions (allowed shape).
		Permissions: encodePermissions(m.Permissions),
	}
	if s.core != nil {
		_ = orm.Of[InstalledBundle](s.core).Save(&rec)
	}

	// Five-pillar plugin registration — wire the manifest's plugin:
	// block into pkg/plugin's Core action registry. Best-effort; a
	// plugin-block failure doesn't fail the install (the sandboxes
	// already landed). Uninstall calls plugin.UnregisterBundle.
	_ = plugin.RegisterBundle(s.core, m.Name, manifestToPluginInput(m))

	// Plugin-view registry — populate the source-of-truth for the
	// frontend descriptor lookup, CSP frame-src allowlist + §5.1
	// postMessage origin verification per RFC.plugin-views §2 + §3.3
	// + §7.2. Best-effort: a port-collision Fail Result here doesn't
	// fail the install (the sandboxes already landed); the affected
	// view simply doesn't register so the frontend falls back per §6.
	registerPluginViews(m)

	// Broadcast — frontend renders status pills + future MCP/agent
	// consumers react. Before is zero (first install, no prior state).
	fireBundleChanged(s.core, PhaseInstalled, rec, InstalledBundle{})

	if lastErr != "" {
		return core.Fail(core.E(installBundleOp, "one or more images failed to start: "+lastErr, nil))
	}

	return core.Ok(InstallOutput{
		BundleID:   m.Name,
		SandboxIDs: sandboxIDs,
	})
}

// manifestToPluginInput translates a BundleManifest's plugin: block
// + images block into the plugin-package-local BundleInput shape.
// The two type-trees are deliberately separated to avoid an import
// cycle (pkg/plugin can't import pkg/marketplace because
// marketplace/wails.go imports plugin for the legacy binary-plugin
// host).
func manifestToPluginInput(m BundleManifest) plugin.BundleInput {
	in := plugin.BundleInput{
		Exposes: map[string]string{},
	}
	for _, img := range m.Images {
		if img.Expose != nil && img.Expose.Route != "" {
			in.Exposes[img.ID] = img.Expose.Route
		}
	}
	if m.Plugin == nil {
		return in
	}
	for _, r := range m.Plugin.Routes {
		in.Routes = append(in.Routes, plugin.BundleRouteEntry{
			Title:            r.Title,
			Icon:             r.Icon,
			Group:            r.Group,
			Target:           r.Target,
			OpenAfterInstall: r.OpenAfterInstall,
		})
	}
	for _, c := range m.Plugin.Commands {
		in.Commands = append(in.Commands, plugin.BundleCommandEntry{
			ID:    c.ID,
			Title: c.Title,
			Runs:  c.Runs,
		})
	}
	for _, s := range m.Plugin.Settings {
		in.Settings = append(in.Settings, plugin.BundleSettingEntry{
			Key:     s.Key,
			Type:    s.Type,
			Prompt:  s.Prompt,
			Default: s.Default,
		})
	}
	return in
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
		vols, vr := s.resolveVolumes(img)
		if !vr.OK {
			// Skip images with rejected volumes — same defence as
			// Install. The launch path is best-effort; an attacker-
			// crafted volume name in a stored manifest doesn't make
			// the whole launch fail, just the offending image.
			continue
		}
		spawnIn := sandbox.SpawnLongInput{
			Image:   img.Image,
			Command: s.resolveImageCommand(img),
			Volumes: vols,
		}
		if img.Expose != nil {
			spawnIn.ExposedPort = img.Expose.Port
		}
		_ = sbSvc.SpawnLong(spawnIn)
	}

	before := recR.Value.(InstalledBundle)
	rec := before
	rec.Status = BundleStatusRunning
	if s.core != nil {
		_ = orm.Of[InstalledBundle](s.core).Save(&rec)
	}
	fireBundleChanged(s.core, PhaseLaunched, rec, before)

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
		before := recR.Value.(InstalledBundle)
		rec := before
		rec.Status = BundleStatusStopped
		if s.core != nil {
			_ = orm.Of[InstalledBundle](s.core).Save(&rec)
		}
		fireBundleChanged(s.core, PhaseStopped, rec, before)
	}

	return core.Ok(nil)
}

// Uninstall stops all sandboxes and removes the durable record.
// Does NOT delete persistent volumes — caller prompts the user first
// per RFC.marketplace.md §5.4.
//
// Ordering — plugin-views RFC §2.1 (Cerberus HIGH-4):
//
//  1. Drop the plugin from the live plugin-view registry
//     (PluginViewRegistry.Drop) — stops accepting postMessage and
//     drops capability grants for the doomed plugin BEFORE any
//     in-flight message can be honoured.
//  2. CSP frame-src allowlist mutation falls out of (1) — the next
//     HTTP response the cspMiddleware renders sees the narrowed
//     registry.
//  3. Emit PluginUninstalled on the IPC bus so subscribers (frontend
//     descriptor table, future MCP listeners) drop their entries.
//  4. Tear down sandboxes (existing Stop call) + drop five-pillar
//     plugin entries + remove the orm record.
//  5. Broadcast the BundleChanged PhaseUninstalled event so existing
//     listeners stay informed.
//
// Usage example:
//
//	r := svc.Uninstall("opencode")
//	if r.OK { /* bundle record removed */ }
func (s *Service) Uninstall(bundleID string) core.Result {
	if core.Trim(bundleID) == "" {
		return core.Fail(core.E(uninstallBundleOp, "bundle id is required", nil))
	}

	// Capture the pre-uninstall state before Stop flips status. The
	// broadcast at the end carries this as Bundle so subscribers see
	// the final shape; Before carries whatever Stop saved.
	var before InstalledBundle
	if recR := s.findInstalledBundle(bundleID); recR.OK {
		before = recR.Value.(InstalledBundle)
	}

	// Step 1 + 2 + 3 — drop from plugin-view registry first so
	// postMessage handlers, CSP allowlist + capability table all see
	// the narrowed state BEFORE the iframe is torn down. The view
	// registry holds the descriptors the frontend mounts against +
	// the per-port CSP allowlist the cspMiddleware reads on every
	// response. Plugin code derived from bundle id — pluginCodeOf
	// resolves the BundleManifest.Plugin.Code → fall back to Name.
	pluginCode := s.resolvePluginCode(bundleID)
	if pluginCode != "" {
		ViewRegistry.Drop(pluginCode)
		firePluginUninstalled(s.core, pluginCode)
	}

	// Step 4 — stop all running sandboxes.
	_ = s.Stop(bundleID)

	// Drop five-pillar plugin entries before the orm record so a
	// later "list installed" doesn't surface ghost route/command
	// names. plugin.UnregisterBundle is idempotent + handles unknown
	// bundle ids cleanly.
	if s.core != nil {
		_ = plugin.UnregisterBundle(s.core, bundleID)
		rec := InstalledBundle{BundleID: bundleID}
		_ = orm.Of[InstalledBundle](s.core).Delete(&rec)
	}

	// Step 5 — Broadcast BundleChanged. Bundle carries the captured
	// pre-uninstall state so subscribers know what was removed;
	// Before stays zero since the orm record is gone by the time the
	// broadcast fires.
	fireBundleChanged(s.core, PhaseUninstalled, before, InstalledBundle{})

	return core.Ok(nil)
}

// registerPluginViews materialises the manifest's PluginView block
// into resolved PluginViewDescriptors + adds them to the live
// ViewRegistry. Per RFC.plugin-views §2 + §3.3 + §5.1 + §7.2 this
// is the source-of-truth update for:
//
//   - the frontend descriptor lookup (GetViewDescriptor)
//   - the CSP frame-src per-port allowlist (cspMiddleware)
//   - the postMessage inbound origin verification (PluginCodeForOrigin)
//   - the §7.2 origin-uniqueness invariant (port collision reject)
//
// Iframe sources of the form ${expose.<id>.route} are NOT resolved
// here — the frontend mounts against http://127.0.0.1:<port> which
// the descriptor carries explicitly (LoopbackPort + LoopbackOrigin).
// The Source field stays as the manifest's resolved route string so
// the iframe URL can reference plugin-internal paths.
func registerPluginViews(m BundleManifest) {
	if m.Plugin == nil || len(m.Plugin.Views) == 0 {
		return
	}
	code := pluginCodeOf(m)
	if code == "" {
		return
	}
	// Build expose id → port + route lookup so iframe view sources
	// can resolve the per-image expose block at install time.
	type exposeBinding struct {
		port  int
		route string
	}
	bindings := map[string]exposeBinding{}
	for _, img := range m.Images {
		if img.Expose != nil {
			bindings[img.ID] = exposeBinding{
				port:  img.Expose.Port,
				route: img.Expose.Route,
			}
		}
	}
	for _, v := range m.Plugin.Views {
		desc := PluginViewDescriptor{
			ID:           v.ID,
			Label:        v.Label,
			Icon:         iconOrDefault(v.Icon),
			Group:        "plugin",
			Kind:         v.Kind,
			Source:       v.Source,
			PluginCode:   code,
			Capabilities: append([]string{}, v.Capabilities...),
		}
		if v.Kind == PluginViewKindIframe {
			exposeID := exposeIDFromSource(v.Source)
			if b, ok := bindings[exposeID]; ok {
				desc.LoopbackPort = b.port
				desc.LoopbackOrigin = core.Sprintf("http://127.0.0.1:%d", b.port)
				if b.route != "" {
					desc.Source = b.route
				}
			}
		}
		_ = ViewRegistry.Add(code, desc)
	}
}

// iconOrDefault returns the manifest-declared icon or "fa-cube"
// when the manifest omitted it (per §2 optional-field default).
func iconOrDefault(icon string) string {
	if core.Trim(icon) == "" {
		return "fa-cube"
	}
	return icon
}

// exposeIDFromSource extracts `<id>` from a `${expose.<id>.route}`
// placeholder string. Returns "" when the source isn't shaped like
// the placeholder (caller treats as no binding).
func exposeIDFromSource(src string) string {
	src = core.Trim(src)
	const prefix = "${expose."
	const suffix = ".route}"
	if !core.HasPrefix(src, prefix) || !core.HasSuffix(src, suffix) {
		return ""
	}
	return src[len(prefix) : len(src)-len(suffix)]
}

// resolvePluginCode returns the plugin code for an installed bundle.
// Loads the manifest from the bundle's config directory; falls back
// to the bundle id when the manifest can't be read (bundle predates
// the views feature OR config path missing).
func (s *Service) resolvePluginCode(bundleID string) string {
	configPath := s.bundleConfigPath(bundleID)
	manifestPath := core.JoinPath(configPath, "manifest.yml")
	readR := core.ReadFile(manifestPath)
	if !readR.OK {
		return core.Trim(bundleID)
	}
	raw, _ := readR.Value.([]byte)
	mR := ParseManifestBytes(raw)
	if !mR.OK {
		return core.Trim(bundleID)
	}
	return pluginCodeOf(mR.Value.(BundleManifest))
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
//
// Cerberus Mantis #1431 — every Persist name flows through
// sandbox.IsValidVolumeName to refuse host-path masquerade as a named
// volume (e.g. `/var/run/docker.sock` would otherwise become a
// host-path bind-mount enabling a docker-socket container escape).
// Returns the volumes plus an error Result; the install loop fails
// the affected image install cleanly when validation rejects.
func (s *Service) resolveVolumes(img ImageEntry) ([]sandbox.LongVolumeMount, core.Result) {
	out := make([]sandbox.LongVolumeMount, 0, len(img.Volumes))
	for _, v := range img.Volumes {
		if !sandbox.IsValidVolumeName(v.Persist) {
			return nil, core.Fail(core.E("marketplace.resolveVolumes",
				core.Concat("invalid volume name (must be alphanumeric + [_.-] only, no paths): ",
					v.Persist), nil))
		}
		// Cerberus Mantis #1446 — gate the container-side path too.
		// `Container: "/data:ro,bind"` would inject mount options
		// into the docker -v argument vector.
		if !sandbox.IsValidContainerPath(v.Container) {
			return nil, core.Fail(core.E("marketplace.resolveVolumes",
				core.Concat("invalid container path (must be absolute, no : , whitespace): ",
					v.Container), nil))
		}
		out = append(out, sandbox.LongVolumeMount{
			Name:      v.Persist,
			Container: v.Container,
		})
	}
	return out, core.Ok(nil)
}

// substituteTokens replaces ${env.KEY} tokens in v with the
// corresponding value from env. Tokens with no match are preserved as-is.
func substituteTokens(v string, env map[string]string) string {
	for k, val := range env {
		v = core.Replace(v, "${env."+k+"}", val)
	}
	return v
}

// encodePermissions JSON-encodes the manifest permissions block for
// persistence on InstalledBundle.Permissions. Empty input round-trips
// as "" (not "null") so the orm column stays clean for bundles that
// declare no permissions.
//
// Usage example:
//
//	rec.Permissions = encodePermissions(manifest.Permissions)
func encodePermissions(perms []Permission) string {
	if len(perms) == 0 {
		return ""
	}
	return core.JSONMarshalString(perms)
}

// DecodePermissions parses an InstalledBundle.Permissions string back
// into the typed slice. Returns nil for empty / invalid input so
// callers (pkg/gateway.CheckPermission) can treat absence and parse
// failure identically: no permissions = nothing allowed.
//
// Usage example:
//
//	perms := marketplace.DecodePermissions(rec.Permissions)
//	for _, p := range perms { /* … */ }
func DecodePermissions(encoded string) []Permission {
	if encoded == "" {
		return nil
	}
	var out []Permission
	if r := core.JSONUnmarshalString(encoded, &out); !r.OK {
		return nil
	}
	return out
}
