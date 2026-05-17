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
	"dappco.re/lthn/desktop/pkg/audit"
	"dappco.re/lthn/desktop/pkg/paths"
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

	// ForceStaleInstall opt-ins to bypassing the catalogue staleness
	// gate (Mantis #1640 HIGH F3). When the catalogue entry's FetchedAt
	// is older than StaleCatalogueThreshold, Install REJECTS with
	// `marketplace.catalogue.stale` unless this flag is true. The
	// frontend's "Catalogue is N hours stale, install anyway?" prompt
	// is the canonical operator confirmation surface; programmatic
	// callers that assert true without UI evidence still leave a row
	// in audit history via emitStaleCatalogueInstallAttempt.
	ForceStaleInstall bool

	// ExpectedManifestDigest, when non-empty, is the manifest body
	// digest the caller wants to assert at install time. Compared
	// against the CatalogueEntry.ManifestDigest if known — mismatch
	// rejects with `marketplace.manifest.version_unconfirmed`. Empty
	// keeps backwards-compat for v1 install callers.
	ExpectedManifestDigest string

	// PrevGrantedPermissions is the per-bundle map of previously-granted
	// (scope:mode → granted_at_iso8601) entries. On Install (re-install
	// / update path), the manifest's declared Permissions are diffed
	// against this map; any scope:mode in the new manifest NOT in
	// PrevGrantedPermissions sets InstallOutput.RequiresReConsent=true
	// + RequiresReConsentScopes carries the NEW scope:mode entries the
	// caller must surface for re-confirmation (Mantis #1641 MED F5).
	// First-install path passes nil; the UI never prompts on first
	// install because the manifest's permissions modal already gathers
	// the consent.
	PrevGrantedPermissions map[string]bool
}

// InstallOutput is what Install returns on success.
type InstallOutput struct {
	// BundleID is the stable identifier (= manifest.Name).
	BundleID string `json:"bundle_id"`
	// SandboxIDs maps image id → the sandbox.ContainerHandle.SandboxID
	// assigned during this install. Useful for reverse-proxy wiring.
	SandboxIDs map[string]string `json:"sandbox_ids"`

	// RequiresReConsent is true when the manifest declares NEW
	// scope:mode permissions that were not in
	// InstallInput.PrevGrantedPermissions (Mantis #1641 MED F5). The
	// frontend surfaces a per-permission re-consent modal; the install
	// itself has ALREADY landed at the point this flag is true — the
	// flag signals "show the operator what they're now granting" rather
	// than gating the install on the consent.
	RequiresReConsent bool `json:"requires_re_consent,omitempty"`

	// RequiresReConsentScopes is the closed-set list of scope:mode
	// entries the operator needs to re-confirm. Empty unless
	// RequiresReConsent is true. Shape matches the
	// "scope:mode" join used in the gateway dispatch surface
	// (e.g. "files:read", "process:spawn").
	RequiresReConsentScopes []string `json:"requires_re_consent_scopes,omitempty"`
}

// BundleStatusOutput is what Status returns.
type BundleStatusOutput struct {
	BundleID string `json:"bundle_id"`
	Display  string `json:"display"`
	Status   string `json:"status"`
	// Handles is the per-image sandbox handle snapshot.
	Handles []sandbox.ContainerHandle `json:"handles,omitempty"`
}

// sandboxPort resolves the sandbox substrate via the SpawnPort
// closure-adapter. Mantis #1664 Phase B (Cerberus #55 ADD-2) — Spawn /
// SpawnLong / Kill / ListHandles / GetHandle / InstallID on
// *sandbox.Service are now Wails-table shims that reject with
// ErrTierGoOnly; substrate access from in-Go consumers MUST route
// through the typed port. The lazy-resolver shape preserves the
// pre-Phase-B `if svc == nil { ... }` guard pattern (NewSpawnPort
// returns nil for nil svc). D-3b adjudication — no composition-root
// refactor; port resolved at callsite.
func (s *Service) sandboxPort() sandbox.SpawnPort {
	if s == nil || s.core == nil {
		return nil
	}
	svc, _ := core.ServiceFor[*sandbox.Service](s.core, "sandbox")
	return sandbox.NewSpawnPort(svc)
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
	// Cerberus #54 C4 (Mantis #1692) — Requested row commits BEFORE
	// ValidateManifest so a forensic walker can correlate caller intent
	// against rejection class. bundle_id may be empty here if the
	// manifest's Name field is itself the rejection — that's the safe
	// shape (empty bundle_id paired with InstallFailed + a sibling
	// ValidateManifestFailed row is the auditor's signal).
	emitInstallRequested(input.Manifest.Name, pluginCodeOf(input.Manifest))

	if r := ValidateManifest(input.Manifest); !r.OK {
		// ValidateManifest already emitted EventMarketplaceValidateManifestFailed
		// at its own boundary — the Install pipeline only wraps with the
		// InstallFailed sibling so a forensic walker can pair the two
		// rows via TS proximity.
		fail := core.Fail(core.E(installBundleOp, "invalid manifest", nil))
		emitInstallFailed(input.Manifest.Name, fail)
		return fail
	}

	m := input.Manifest

	// Mantis #1583 MED — single-flight per bundleID. Two concurrent
	// Install / Launch / Stop / Uninstall calls for the same bundle
	// used to race on the orm record + sandbox handles, producing
	// orphaned containers or split state. Acquire BEFORE the duplicate-
	// code check (#1580) so the check + record write inside the
	// critical section can't be raced by a sibling install for the
	// same id. Different bundle ids remain concurrent.
	lock := s.bundleMutex(m.Name)
	lock.Lock()
	defer lock.Unlock()

	// Mantis #1580 HIGH — Install-time plugin-code uniqueness gate.
	// A marketplace bundle's plugin.code must not collide with an
	// existing binary plugin OR an already-installed marketplace
	// bundle's plugin code, else postMessage / proxy / view-registry
	// dispatch becomes ambiguous (whose code is this?). Reject loudly
	// rather than overwriting.
	if r := s.checkPluginCodeCollision(m); !r.OK {
		emitInstallFailed(m.Name, r)
		return r
	}

	// Mantis #1640 HIGH F3 — catalogue staleness gate. If the
	// FetchedAt evidence is older than StaleCatalogueThreshold and the
	// caller has NOT asserted ForceStaleInstall, refuse install with a
	// typed reject. UX prompt "Catalogue is N hours stale, install
	// anyway?" surfaces the override option.
	entry := s.findCatalogueEntry(m.Name)
	if entry != nil && !entry.FetchedAt.IsZero() {
		age := core.Since(entry.FetchedAt)
		if age > StaleCatalogueThreshold {
			emitStaleCatalogueInstallAttempt(m.Name, input.ForceStaleInstall, age)
			if !input.ForceStaleInstall {
				fail := core.Fail(core.E(installBundleOp,
					codeStaleCatalogueInstall+": catalogue entry for "+m.Name+
						" is stale (age exceeds "+core.Sprintf("%s", StaleCatalogueThreshold)+
						" threshold); set ForceStaleInstall=true to override", nil))
				emitInstallFailed(m.Name, fail)
				return fail
			}
		}
	}

	// Mantis #1645 MED F6 — manifest version-bump confirmation gate.
	// If the catalogue records a PendingManifestDigest (upstream
	// released a new manifest at the same source URL without the
	// operator having confirmed the bump), refuse install until
	// ConfirmManifestVersionBump promotes Pending → ManifestDigest.
	// Split-brain on legit version bumps is the failure mode this
	// closes: silently installing a new version under the same tag
	// would defeat the whole signing chain.
	if entry != nil && core.Trim(entry.PendingManifestDigest) != "" {
		emitManifestVersionPending(m.Name,
			core.Trim(entry.ManifestDigest),
			core.Trim(entry.PendingManifestDigest))
		fail := core.Fail(core.E(installBundleOp,
			codeManifestVersionUnconfirmed+": catalogue carries an unconfirmed pending version for "+
				m.Name+"; call ConfirmManifestVersionBump first", nil))
		emitInstallFailed(m.Name, fail)
		return fail
	}

	// Mantis #1645 MED F6 follow-on — caller-supplied expected manifest
	// digest cross-check. When the InstallInput passes an explicit
	// expected digest AND the catalogue entry has a canonical
	// ManifestDigest, mismatch is a hard reject (the version of the
	// manifest the operator authorised is not the version on disk).
	if entry != nil && core.Trim(input.ExpectedManifestDigest) != "" &&
		core.Trim(entry.ManifestDigest) != "" &&
		core.Trim(input.ExpectedManifestDigest) != core.Trim(entry.ManifestDigest) {
		fail := core.Fail(core.E(installBundleOp,
			codeManifestVersionUnconfirmed+
				": caller-asserted expected manifest digest does not match catalogue canonical digest", nil))
		emitInstallFailed(m.Name, fail)
		return fail
	}

	sbPort := s.sandboxPort()
	if sbPort == nil {
		fail := core.Fail(core.E(installBundleOp, "sandbox service not available", nil))
		emitInstallFailed(m.Name, fail)
		return fail
	}

	// Mantis #1639 HIGH F2 — image digest verify gate. For every image
	// entry whose manifest declares an Expected digest, re-verify via
	// sandbox.InspectImage BEFORE SpawnLong. Without this gate the
	// runtime delegates digest verification to docker pull which
	// happens lazily and asymmetrically (cached layers skip
	// re-verification). The gate fires AFTER staleness +
	// version-confirmation so the audit history shows the rejection
	// class clearly when multiple gates would fail.
	if sbSvc, _ := core.ServiceFor[*sandbox.Service](s.core, "sandbox"); sbSvc != nil {
		if r := s.verifyImageDigests(sbSvc, m); !r.OK {
			emitInstallFailed(m.Name, r)
			return r
		}
	}

	env := s.resolveEnv(m, input.Env)

	configPath := s.bundleConfigPath(m.Name)
	if r := core.MkdirAll(configPath, 0o755); !r.OK {
		fail := core.Fail(core.E(installBundleOp, "config dir creation failed", nil))
		emitInstallFailed(m.Name, fail)
		return fail
	}

	// Mantis #1689 HIGH (Cerberus #54 C1) — persist the validated
	// manifest to manifest.yml under configPath. Without this the
	// Launch + Uninstall.resolvePluginCode paths both fall through
	// their core.ReadFile fallback (Launch fails outright with
	// "manifest not found"; resolvePluginCode silently returns the
	// bundle id, leaving stale ViewRegistry entries when
	// plugin.code != bundle.Name on uninstall — descriptors + CSP
	// frame-src allowlist + postMessage origin table retain dead
	// entries past uninstall).
	if r := s.persistManifest(configPath, m); !r.OK {
		emitInstallFailed(m.Name, r)
		return r
	}

	// Mantis #1670 — stamp install + bundle identity on every
	// long-running container so a future reconcile pass can gate
	// adoption (same shape as opencode's install-id labelling,
	// Mantis #1599 Cerberus #22). Resolve installID once outside
	// the loop so every image entry shares the same value; on KV
	// failure fall through with "" (SpawnLong omits the label
	// for an empty value — no regression vs the pre-#1670 world).
	installID := s.resolveSandboxInstallID(sbPort)

	sandboxIDs := map[string]string{}
	var lastErr string

	for _, img := range m.Images {
		vols, vr := s.resolveVolumes(img)
		if !vr.OK {
			lastErr = vr.Error()
			continue
		}
		spawnIn := buildInstallSpawnInput(buildInstallSpawnInputArgs{
			Img:       img,
			Command:   s.resolveImageCommand(img),
			Env:       s.resolveImageEnv(img, env),
			Volumes:   vols,
			InstallID: installID,
			BundleID:  m.Name,
		})

		r := sbPort.SpawnLong(spawnIn)
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
		// Mantis #1693 MED (Cerberus #54 C5) — orm.Save was previously
		// silently swallowed via `_ =`. On failure the sandboxes
		// spawned above became orphans (no durable record so neither
		// Uninstall nor a future reconcile could find them). Roll back
		// every spawned handle via Kill so the OS state matches the
		// (now-rejected) record state, then return the orm error.
		if r := orm.Of[InstalledBundle](s.core).Save(&rec); !r.OK {
			rollbackSpawnedSandboxes(sbPort, sandboxIDs)
			fail := core.Fail(core.E(installBundleOp,
				"orm save failed — rolled back spawned containers: "+r.Error(), nil))
			emitInstallFailed(m.Name, fail)
			return fail
		}
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
		fail := core.Fail(core.E(installBundleOp, "one or more images failed to start: "+lastErr, nil))
		emitInstallFailed(m.Name, fail)
		return fail
	}

	// Mantis #1641 MED F5 — per-scope re-consent diff. Compare the
	// manifest's declared scope:mode permissions against
	// InstallInput.PrevGrantedPermissions; any new entry is surfaced on
	// InstallOutput.RequiresReConsent / RequiresReConsentScopes so the
	// frontend can prompt with a per-permission modal. First-install
	// path passes nil for PrevGrantedPermissions and never triggers the
	// flag (the manifest's permissions modal already gathers the
	// initial consent).
	reConsentScopes := diffNewPermissions(m.Permissions, input.PrevGrantedPermissions)
	requiresReConsent := false
	if len(input.PrevGrantedPermissions) > 0 && len(reConsentScopes) > 0 {
		requiresReConsent = true
		emitPermissionDiffRequiresReConsent(m.Name, reConsentScopes)
	}

	emitInstallSucceeded(m.Name, pluginCodeOf(m), len(sandboxIDs))
	return core.Ok(InstallOutput{
		BundleID:                m.Name,
		SandboxIDs:              sandboxIDs,
		RequiresReConsent:       requiresReConsent,
		RequiresReConsentScopes: reConsentScopes,
	})
}

// verifyImageDigests Mantis #1639 HIGH F2 — pre-spawn digest gate.
// For every image entry in the manifest, call sandbox.InspectImage(ref)
// and compare against the manifest's expected digest (when declared).
// Returns the typed reject Result on first mismatch / unverifiable so
// the audit history pins the offending image. v1 manifests without
// per-image expected digests fall through permissively — the gate is
// strict only when the manifest carries the evidence.
//
// The expected digest field is parsed out of ImageEntry's image ref
// when present in the OCI `image@sha256:...` form. Future bundles
// will carry a dedicated `digest:` field on each image entry; until
// then, the embedded `@sha256:` is the convention.
//
// Usage example (internal):
//
//	if r := s.verifyImageDigests(sbSvc, m); !r.OK { return r }
func (s *Service) verifyImageDigests(sbSvc *sandbox.Service, m BundleManifest) core.Result {
	for _, img := range m.Images {
		expected := expectedDigestFromRef(img.Image)
		if expected == "" {
			// No embedded digest evidence on this ref — fall through.
			// Future bundle.image entries with an explicit `digest:`
			// field will route through here without code change.
			continue
		}
		actual, r := sbSvc.InspectImage(img.Image)
		if !r.OK {
			// Surface as digest_unverifiable — degraded host, not
			// active tampering. Pattern-match on the typed prefix
			// sandbox stamps so the auditor can distinguish docker-
			// missing from inspect-failed from parse-failed.
			emitImageDigestUnverifiable(m.Name, img.Image, r.Error())
			return core.Fail(core.E(installBundleOp,
				codeImageDigestUnverifiable+": cannot verify digest for "+img.Image+
					": "+r.Error(), nil))
		}
		if actual != expected {
			emitImageDigestMismatch(m.Name, img.Image, expected, actual)
			return core.Fail(core.E(installBundleOp,
				codeImageDigestMismatch+": image "+img.Image+
					" expected "+expected+" got "+actual, nil))
		}
	}
	return core.Ok(nil)
}

// expectedDigestFromRef extracts the SHA256 digest from an
// `image-name@sha256:hex` reference. Returns "" when the ref carries
// no digest suffix — the install gate treats that as "no evidence,
// fall through permissively".
//
// Usage example (internal):
//
//	d := expectedDigestFromRef("docker.io/lib/alpine@sha256:abc")
//	// d == "sha256:abc"
func expectedDigestFromRef(ref string) string {
	ref = core.Trim(ref)
	const sep = "@sha256:"
	idx := core.Index(ref, sep)
	if idx < 0 {
		return ""
	}
	digest := ref[idx+1:] // strip the leading "@" — keep "sha256:..."
	if core.Trim(digest) == "" {
		return ""
	}
	return digest
}

// diffNewPermissions returns the closed-set list of scope:mode
// strings declared in manifest.Permissions that are NOT present in
// prevGranted. Returns nil when manifest declares no permissions or
// every declared permission is already granted. Order follows the
// manifest's declaration order so the operator-facing modal renders
// stable.
//
// Usage example (internal):
//
//	newScopes := diffNewPermissions(m.Permissions, prev)
//	if len(newScopes) > 0 { /* prompt for re-consent */ }
func diffNewPermissions(perms []Permission, prevGranted map[string]bool) []string {
	if len(perms) == 0 {
		return nil
	}
	out := make([]string, 0, len(perms))
	for _, p := range perms {
		key := p.Scope + ":" + p.Mode
		if !prevGranted[key] {
			out = append(out, key)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// emitStaleCatalogueInstallAttempt records the F3 override-attempted
// audit row. Event const sourced from pkg/audit/types.go per Mantis
// #1746 housekeeping — the Wave 2 inline-literal carve-out closed
// once the audit/types.go allowlist gate opened.
//
// Usage example (internal):
//
//	emitStaleCatalogueInstallAttempt(bundleID, input.ForceStaleInstall, age)
func emitStaleCatalogueInstallAttempt(bundleID string, override bool, age core.Duration) {
	_ = audit.Default().Record(audit.Event{
		Event:   audit.EventMarketplaceCatalogueStaleInstallAttempt,
		TS:      core.Now().UTC().Unix(),
		Scope:   marketplaceScope,
		Outcome: audit.OutcomeOK,
		Meta: map[string]any{
			"bundle_id":  bundleID,
			"override":   override,
			"age_string": core.Sprintf("%s", age),
		},
	})
}

// emitManifestVersionPending records the F6 "operator hit Install but
// version is unconfirmed" audit row. Distinct from confirmed/rolled-
// back so an auditor can reconstruct the operator's decision sequence.
func emitManifestVersionPending(bundleID, oldDigest, pendingDigest string) {
	_ = audit.Default().Record(audit.Event{
		Event:   audit.EventMarketplaceManifestVersionPending,
		TS:      core.Now().UTC().Unix(),
		Scope:   marketplaceScope,
		Outcome: audit.OutcomeDenied,
		Meta: map[string]any{
			"bundle_id":      bundleID,
			"old_digest":     oldDigest,
			"pending_digest": pendingDigest,
		},
	})
}

// emitManifestVersionBumpTransition records the F6 promotion event:
// operator confirmed Pending → Canonical, OR a rollback drops Pending
// back to the prior canonical. transition is one of "confirmed" /
// "rolled_back" / "auto_promoted" (forward-arc).
func emitManifestVersionBumpTransition(bundleID, oldDigest, newDigest, transition string) {
	_ = audit.Default().Record(audit.Event{
		Event:   audit.EventMarketplaceManifestVersionTransition,
		TS:      core.Now().UTC().Unix(),
		Scope:   marketplaceScope,
		Outcome: audit.OutcomeOK,
		Meta: map[string]any{
			"bundle_id":  bundleID,
			"old_digest": oldDigest,
			"new_digest": newDigest,
			"transition": transition,
		},
	})
}

// emitImageDigestMismatch records the F2 hard-reject row. Both
// expected + actual digests land in Meta so an auditor can correlate
// the registry's served digest against what the manifest demanded.
func emitImageDigestMismatch(bundleID, imageRef, expected, actual string) {
	_ = audit.Default().Record(audit.Event{
		Event:   audit.EventMarketplaceImageDigestMismatch,
		TS:      core.Now().UTC().Unix(),
		Scope:   marketplaceScope,
		Outcome: audit.OutcomeDenied,
		Meta: map[string]any{
			"bundle_id":       bundleID,
			"image_ref":       imageRef,
			"expected_digest": expected,
			"actual_digest":   actual,
		},
	})
}

// emitImageDigestUnverifiable records the F2 degraded-verify row. The
// reason carries the typed prefix sandbox stamped so the auditor can
// distinguish docker-missing from inspect-failed from parse-failed.
func emitImageDigestUnverifiable(bundleID, imageRef, reason string) {
	_ = audit.Default().Record(audit.Event{
		Event:   audit.EventMarketplaceImageDigestUnverifiable,
		TS:      core.Now().UTC().Unix(),
		Scope:   marketplaceScope,
		Outcome: audit.OutcomeFailed,
		Meta: map[string]any{
			"bundle_id": bundleID,
			"image_ref": imageRef,
			"reason":    reason,
		},
	})
}

// emitPermissionDiffRequiresReConsent records the F5 "manifest update
// adds permissions" row. scopes is the closed-set scope:mode list the
// frontend will surface in the re-consent modal.
func emitPermissionDiffRequiresReConsent(bundleID string, scopes []string) {
	_ = audit.Default().Record(audit.Event{
		Event:   audit.EventMarketplacePermissionDiffRequiresReConsent,
		TS:      core.Now().UTC().Unix(),
		Scope:   marketplaceScope,
		Outcome: audit.OutcomeOK,
		Meta: map[string]any{
			"bundle_id":  bundleID,
			"new_scopes": scopes,
		},
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
	// Cerberus #54 C4 (Mantis #1692) — Requested row commits BEFORE
	// the empty-id check so an audit row captures every invocation,
	// even the bad-input ones a forensic walker correlates against
	// caller misbehaviour.
	emitLaunchRequested(bundleID)

	if core.Trim(bundleID) == "" {
		fail := core.Fail(core.E(launchBundleOp, "bundle id is required", nil))
		emitLaunchFailed(bundleID, fail)
		return fail
	}

	// Mantis #1583 MED — single-flight per bundleID.
	lock := s.bundleMutex(bundleID)
	lock.Lock()
	defer lock.Unlock()

	recR := s.findInstalledBundle(bundleID)
	if !recR.OK {
		// Bounded *core.Err.Operation literal (no caller-supplied bytes
		// echoed) so the NoRawSecrets canary stays clean — bundle_id is
		// already in the sibling Requested row's Meta for forensic pairing.
		fail := core.Fail(core.E(launchBundleOp, "bundle not installed: "+bundleID, nil))
		emitLaunchFailed(bundleID, fail)
		return fail
	}

	configPath := s.bundleConfigPath(bundleID)
	manifestPath := core.JoinPath(configPath, "manifest.yml")
	readR := core.ReadFile(manifestPath)
	if !readR.OK {
		fail := core.Fail(core.E(launchBundleOp, "manifest not found at: "+manifestPath, nil))
		emitLaunchFailed(bundleID, fail)
		return fail
	}
	raw, _ := readR.Value.([]byte)
	mR := ParseManifestBytes(raw)
	if !mR.OK {
		fail := core.Fail(core.E(launchBundleOp, "manifest parse failed", nil))
		emitLaunchFailed(bundleID, fail)
		return fail
	}
	m := mR.Value.(BundleManifest)

	sbPort := s.sandboxPort()
	if sbPort == nil {
		fail := core.Fail(core.E(launchBundleOp, "sandbox service not available", nil))
		emitLaunchFailed(bundleID, fail)
		return fail
	}

	// Mantis #1670 — same install/bundle stamping as the Install
	// loop. Resolve installID once outside the loop so every
	// image entry shares the same value.
	installID := s.resolveSandboxInstallID(sbPort)

	// Track sandbox handles spawned in this Launch call so a downstream
	// orm.Save failure can roll them back (Mantis #1693 MED / Cerberus
	// #54 C5). Keyed by image id; values are the sandbox.ContainerHandle
	// returned by SpawnLong so Kill can target them precisely.
	launchedIDs := map[string]string{}
	for _, img := range m.Images {
		vols, vr := s.resolveVolumes(img)
		if !vr.OK {
			// Skip images with rejected volumes — same defence as
			// Install. The launch path is best-effort; an attacker-
			// crafted volume name in a stored manifest doesn't make
			// the whole launch fail, just the offending image.
			continue
		}
		spawnIn := buildInstallSpawnInput(buildInstallSpawnInputArgs{
			Img:       img,
			Command:   s.resolveImageCommand(img),
			Volumes:   vols,
			InstallID: installID,
			BundleID:  m.Name,
		})
		spawnR := sbPort.SpawnLong(spawnIn)
		if spawnR.OK {
			if h, ok := spawnR.Value.(sandbox.ContainerHandle); ok {
				launchedIDs[img.ID] = h.SandboxID
			}
		}
	}

	before := recR.Value.(InstalledBundle)
	rec := before
	rec.Status = BundleStatusRunning
	if s.core != nil {
		// Mantis #1693 MED (Cerberus #54 C5) — orm.Save was previously
		// silently swallowed. On failure the freshly-spawned sandboxes
		// from the loop above became orphans (no Status flip + stale
		// "stopped" record means a follow-up Stop call wouldn't touch
		// the live containers). Roll back the just-spawned handles via
		// Kill and surface the orm error to the caller.
		if r := orm.Of[InstalledBundle](s.core).Save(&rec); !r.OK {
			rollbackSpawnedSandboxes(sbPort, launchedIDs)
			fail := core.Fail(core.E(launchBundleOp,
				"orm save failed — rolled back spawned containers: "+r.Error(), nil))
			emitLaunchFailed(bundleID, fail)
			return fail
		}
	}
	fireBundleChanged(s.core, PhaseLaunched, rec, before)

	emitLaunchSucceeded(bundleID, len(launchedIDs))
	return core.Ok(nil)
}

// Stop kills all running sandboxes for a bundle and marks it stopped.
//
// Usage example:
//
//	r := svc.Stop("opencode")
//	if r.OK { /* bundle is stopped */ }
func (s *Service) Stop(bundleID string) core.Result {
	// Cerberus #54 C4 (Mantis #1692) — Requested row commits at the
	// public-entry boundary so the in-package stopLocked (re-entrant
	// from Uninstall) doesn't double-fire. emitStopSucceeded/Failed
	// pair fires inside this method against the stopLocked Result.
	emitStopRequested(bundleID)

	if core.Trim(bundleID) == "" {
		fail := core.Fail(core.E(stopBundleOp, "bundle id is required", nil))
		emitStopFailed(bundleID, fail)
		return fail
	}

	// Mantis #1583 MED — single-flight per bundleID.
	lock := s.bundleMutex(bundleID)
	lock.Lock()
	defer lock.Unlock()

	r := s.stopLocked(bundleID)
	if !r.OK {
		emitStopFailed(bundleID, r)
		return r
	}
	emitStopSucceeded(bundleID)
	return r
}

// stopLocked is the lock-held body of Stop. Split so Uninstall (which
// also acquires bundleMutex(bundleID)) can run the same teardown
// without recursive-lock deadlock. Mantis #1583.
func (s *Service) stopLocked(bundleID string) core.Result {
	sbPort := s.sandboxPort()
	if sbPort != nil {
		listR := sbPort.ListHandles()
		if listR.OK {
			handles := listR.Value.([]sandbox.ContainerHandle)
			for _, h := range handles {
				if h.BundleID == bundleID {
					_ = sbPort.Kill(h.SandboxID)
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
			// Mantis #1693 MED (Cerberus #54 C5) — orm.Save was
			// previously silently swallowed. Sandboxes are already
			// killed above so there's nothing to roll back; the
			// failure mode here is a stale Status flag (record still
			// reads "running" when the live state is "stopped"). Return
			// the error rather than masking it as Ok — callers can then
			// retry the Status flip or alert.
			if r := orm.Of[InstalledBundle](s.core).Save(&rec); !r.OK {
				return core.Fail(core.E(stopBundleOp,
					"orm save failed — record may show stale Status: "+r.Error(), nil))
			}
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
	// Cerberus #54 C4 (Mantis #1692) — Requested row commits BEFORE
	// the empty-id check + plugin-view registry drop / sandbox
	// teardown / orm.Delete so the operator intent survives a
	// mid-uninstall crash. plugin_code resolves via the manifest-on-
	// disk lookup; falls back to bundleID when the manifest is missing.
	emitUninstallRequested(bundleID, s.resolvePluginCode(bundleID))

	if core.Trim(bundleID) == "" {
		fail := core.Fail(core.E(uninstallBundleOp, "bundle id is required", nil))
		emitUninstallFailed(bundleID, fail)
		return fail
	}

	// Mantis #1583 MED — single-flight per bundleID. Acquired ONCE
	// at the top; the internal Stop call is routed through stopLocked
	// to avoid the recursive-lock deadlock.
	lock := s.bundleMutex(bundleID)
	lock.Lock()
	defer lock.Unlock()

	// Capture the pre-uninstall state before Stop flips status. The
	// broadcast at the end carries this as Bundle so subscribers see
	// the final shape; Before carries whatever Stop saved.
	// recordExists guards the orm.Delete below — Mantis #1693 MED
	// (Cerberus #54 C5). Before the surface change the Delete error
	// was silently swallowed via `_ =` so Delete-on-nothing returned
	// the same OK shape as Delete-on-real-record. Now that Delete
	// surfaces its error, a defensive "uninstall a bundle that was
	// never installed" call (covered by TestEvents_Uninstall_Fires)
	// must NOT hit Delete at all — the no-op is "nothing to remove",
	// not "removal failed".
	var before InstalledBundle
	recordExists := false
	if recR := s.findInstalledBundle(bundleID); recR.OK {
		before = recR.Value.(InstalledBundle)
		recordExists = true
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

	// Step 4 — stop all running sandboxes. Use stopLocked (not Stop)
	// because we already hold bundleMutex(bundleID); calling Stop
	// would deadlock on re-acquire (Mantis #1583).
	_ = s.stopLocked(bundleID)

	// Drop five-pillar plugin entries before the orm record so a
	// later "list installed" doesn't surface ghost route/command
	// names. plugin.UnregisterBundle is idempotent + handles unknown
	// bundle ids cleanly.
	if s.core != nil {
		_ = plugin.UnregisterBundle(s.core, bundleID)
		// Mantis #1693 MED (Cerberus #54 C5) — orm.Delete was
		// previously silently swallowed. Sandboxes are already torn
		// down above so there's nothing to roll back; the failure
		// mode here is a stale registry record (subsequent list-
		// installed surfaces a ghost bundle whose runtime is gone).
		// Surface the error so the caller can retry the delete or
		// flag the orphan record for reconcile; broadcast still fires
		// so live UI subscribers can re-fetch.
		//
		// Gated on recordExists so the defensive "uninstall a bundle
		// that was never installed" call doesn't hit Delete on a
		// table that has no row to remove (returning a misleading
		// Fail when the desired end-state — "no such record" — is
		// already achieved).
		if recordExists {
			rec := InstalledBundle{BundleID: bundleID}
			if r := orm.Of[InstalledBundle](s.core).Delete(&rec); !r.OK {
				fireBundleChanged(s.core, PhaseUninstalled, before, InstalledBundle{})
				fail := core.Fail(core.E(uninstallBundleOp,
					"orm delete failed — registry record may be stale: "+r.Error(), nil))
				emitUninstallFailed(bundleID, fail)
				return fail
			}
		}
	}

	// Step 5 — Broadcast BundleChanged. Bundle carries the captured
	// pre-uninstall state so subscribers know what was removed;
	// Before stays zero since the orm record is gone by the time the
	// broadcast fires.
	fireBundleChanged(s.core, PhaseUninstalled, before, InstalledBundle{})

	// plugin_code captured at the end uses the resolved value before
	// the orm.Delete dropped the record (resolvePluginCode reads from
	// disk so the post-delete lookup would still work, but using the
	// before snapshot is cheaper + carries the exact value an auditor
	// would correlate with the matching Requested row).
	emitUninstallSucceeded(bundleID, s.resolvePluginCode(bundleID))
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

// rollbackSpawnedSandboxes Kills every sandbox handle in ids when an
// orm.Save fails AFTER the spawn loop has landed live containers. The
// alternative — silently dropping the orm error — leaves an orphan
// container with no managed handle (no record in InstalledBundle so
// Uninstall + ListHandles-driven reconcile can't find it). Mantis
// #1693 MED (Cerberus #54 C5).
//
// Best-effort: each Kill is a separate Result; one Kill failing
// doesn't stop the rest. sbSvc.Kill already emits the
// EventSandboxKillRequested / Succeeded / Failed audit chain so the
// rollback is traceable from audit history without a new event type.
// nil sbSvc is tolerated (defensive — the orm.Save failure path is
// already an unhappy branch; don't compound it with a nil-deref).
//
// Usage example (internal):
//
//	if r := orm.Of[InstalledBundle](s.core).Save(&rec); !r.OK {
//	    rollbackSpawnedSandboxes(sbSvc, sandboxIDs)
//	    return core.Fail(...)
//	}
func rollbackSpawnedSandboxes(sbPort sandbox.SpawnPort, ids map[string]string) {
	if sbPort == nil {
		return
	}
	for _, id := range ids {
		if id == "" {
			continue
		}
		_ = sbPort.Kill(id)
	}
}

// persistManifest writes the validated manifest to <configPath>/manifest.yml
// via paths.AtomicWriteWithVersion. Mantis #1689 HIGH (Cerberus #54 C1):
// before this step landed, Install left the config dir empty so Launch's
// ReadFile failed outright AND resolvePluginCode silently fell back to
// the bundle id — leaving stale ViewRegistry/CSP/postMessage entries when
// plugin.code != bundle.Name on uninstall.
//
// Round-trip is covered by manifest_test.go (MarshalManifest +
// ParseManifestBytes are inverses on a validated BundleManifest).
// AtomicWriteWithVersion gives tmp+fsync+rename torn-write safety + a
// 0o600 perm baseline (paths.writeFileMode). Empty WriteInput predicates
// = unconditional first-write, which is the install-time shape (every
// install rewrites the manifest from the validated input).
//
// Usage example (internal):
//
//	if r := s.persistManifest(configPath, m); !r.OK { return r }
func (s *Service) persistManifest(configPath string, m BundleManifest) core.Result {
	yamlR := MarshalManifest(m)
	if !yamlR.OK {
		return core.Fail(core.E(installBundleOp, "manifest serialise failed", nil))
	}
	manifestPath := core.JoinPath(configPath, "manifest.yml")
	r := paths.AtomicWriteWithVersion(manifestPath, paths.WriteInput{
		Body: yamlR.Value.([]byte),
	})
	if !r.OK {
		return core.Fail(core.E(installBundleOp, "manifest write failed: "+r.Error(), nil))
	}
	return core.Ok(nil)
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
	sbPort := s.sandboxPort()
	if sbPort != nil {
		listR := sbPort.ListHandles()
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

// IsInstalled reports whether the given plugin code resolves to an
// installed marketplace bundle. Used by cmd/lthn/main.go as the
// marketplace-tier branch of the composite PluginInstalledChecker
// (Mantis #1581): pkg/plugin owns the binary-plugin tier via
// (*plugin.Service).ProxyGroup().Has; marketplace owns the bundle
// tier via this method. Together they ensure the
// /v1/plugin-view/capability-grant 404 gate covers both install
// surfaces.
//
// A bundle's effective plugin code is the explicit plugin.code from
// its on-disk manifest (when present) falling back to the bundle id
// — same rule as pluginCodeOf used by checkPluginCodeCollision, so
// the "this is a real installed plugin" check is symmetric across
// install-time uniqueness and runtime capability-grant gating.
//
// Best-effort by design: when ListInstalled fails (orm outage,
// schema not yet migrated) the method returns false rather than
// failing loud — the audit row still emits, only the plugin_id
// gate is degraded. Empty code returns false.
//
// Usage example:
//
//	if svc.IsInstalled("opencode") { /* bundle present */ }
func (s *Service) IsInstalled(code string) bool {
	code = core.Trim(code)
	if code == "" {
		return false
	}
	listR := s.ListInstalled()
	if !listR.OK {
		return false
	}
	bundles, ok := listR.Value.([]InstalledBundle)
	if !ok {
		return false
	}
	for _, b := range bundles {
		if s.resolvePluginCode(b.BundleID) == code {
			return true
		}
	}
	return false
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

// buildInstallSpawnInputArgs is the parameter bag for
// buildInstallSpawnInput. Keeping the call shape as a struct (vs.
// positional args) future-proofs the wiring as more SpawnLongInput
// fields gain bundle-side defaults — adding a field doesn't ripple
// through every caller's argument order.
type buildInstallSpawnInputArgs struct {
	Img       ImageEntry
	Command   string
	Env       map[string]string
	Volumes   []sandbox.LongVolumeMount
	InstallID string
	BundleID  string
}

// buildInstallSpawnInput constructs the sandbox.SpawnLongInput the
// Install + Launch loops hand to sandbox.SpawnLong, stamping the
// per-install + per-bundle identifiers (Mantis #1670 / H#187
// follow-on for #1665).
//
// Extracted so the wiring is testable without spinning up the
// sandbox service — the test asserts both identifier fields land
// on the spawn input verbatim regardless of which loop builds it.
//
// Usage example:
//
//	spawnIn := buildInstallSpawnInput(buildInstallSpawnInputArgs{
//	    Img:       img,
//	    Command:   s.resolveImageCommand(img),
//	    Env:       s.resolveImageEnv(img, env),
//	    Volumes:   vols,
//	    InstallID: installID,
//	    BundleID:  m.Name,
//	})
func buildInstallSpawnInput(args buildInstallSpawnInputArgs) sandbox.SpawnLongInput {
	spawnIn := sandbox.SpawnLongInput{
		Image:     args.Img.Image,
		Command:   args.Command,
		Env:       args.Env,
		Volumes:   args.Volumes,
		InstallID: args.InstallID,
		BundleID:  args.BundleID,
	}
	if args.Img.Expose != nil {
		spawnIn.ExposedPort = args.Img.Expose.Port
	}
	return spawnIn
}

// resolveSandboxInstallID asks the sandbox service for its
// persisted per-install identifier; on KV failure (e.g. HOME not
// resolvable in a hardened test environment) the install/launch
// loops fall through with "" so SpawnLong omits the label rather
// than failing the whole bundle install.
//
// The degradation matches the pre-#1670 world — labels were
// always empty before, so a transient KV failure can't be a worse
// outcome than the baseline behaviour everyone has been running.
func (s *Service) resolveSandboxInstallID(sbPort sandbox.SpawnPort) string {
	if sbPort == nil {
		return ""
	}
	r := sbPort.InstallID()
	if !r.OK {
		return ""
	}
	id, _ := r.Value.(string)
	return id
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

// checkPluginCodeCollision enforces the Mantis #1580 HIGH Install-
// time uniqueness gate. A marketplace bundle's plugin.code MUST NOT
// collide with:
//
//   - an existing binary-plugin code installed under
//     ~/Lethean/conf/plugins/<code>/plugin.json (resolved via the
//     pkg/plugin service's List surface when registered);
//   - an already-installed marketplace bundle whose effective plugin
//     code (pluginCodeOf) matches this install's effective code AND
//     whose bundle id differs (re-installing the SAME bundle id is a
//     legitimate update path, not a collision).
//
// Reject with typed code paths.CodePluginCodeCollision so HTTP
// envelope construction + frontend toast routing can pattern-match.
//
// Best-effort by design: when pkg/plugin is not registered (binary-
// plugin host absent), the binary-side check is skipped — the
// marketplace surface still gates against its own installed bundles
// via the orm scan.
func (s *Service) checkPluginCodeCollision(m BundleManifest) core.Result {
	code := pluginCodeOf(m)
	if code == "" {
		return core.Ok(nil)
	}

	// (a) Binary-plugin collision via pkg/plugin.Service.List().
	if s.core != nil {
		if host, ok := core.ServiceFor[*plugin.Service](s.core, "plugin"); ok && host != nil {
			listR := host.List()
			if listR.OK {
				if out, ok := listR.Value.(plugin.ListOutput); ok {
					for _, p := range out.Plugins {
						if p.Code == code {
							return core.Fail(core.E(paths.CodePluginCodeCollision,
								"plugin code already taken by an installed binary plugin: "+code, nil))
						}
					}
				}
			}
		}
	}

	// (b) Marketplace-bundle collision via orm scan.
	if s.core != nil {
		listR := orm.Of[InstalledBundle](s.core).Get()
		if listR.OK {
			if bundles, ok := listR.Value.([]InstalledBundle); ok {
				for _, ib := range bundles {
					if ib.BundleID == m.Name {
						// Same bundle id — re-install / update path, NOT
						// a collision. The bundle owns its own code.
						continue
					}
					existing := s.resolvePluginCode(ib.BundleID)
					if existing == code {
						return core.Fail(core.E(paths.CodePluginCodeCollision,
							"plugin code already taken by installed bundle "+ib.BundleID+": "+code, nil))
					}
				}
			}
		}
	}

	return core.Ok(nil)
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
