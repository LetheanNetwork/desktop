// SPDX-Licence-Identifier: EUPL-1.2

// Tests for the Cerberus #54 C4 (Mantis #1692) Repudiation gap close:
// Install / Uninstall / Launch / Stop / FetchManifest / ValidateManifest
// on pkg/marketplace.Service now emit typed audit.EventMarketplace*
// rows via audit.Default() so the auditor sees lifecycle decisions
// instead of forensically silent bundle ops. Mirror of
// pkg/process/audit_test.go (H#193 Cerberus #50) — the
// recordingRecorder shape is duplicated rather than shared because
// each test package's SetDefault is process-global and the fixture
// must not leak across packages running in parallel.
//
// The Install / Launch / Uninstall / Stop tests drive each entrypoint
// against a marketplace service with no sandbox or orm wired so the
// dispatch fails fast at sandbox-not-available / empty-id / bundle-
// not-installed. That drives the Requested + Failed lifecycle pair
// end-to-end without spinning a real container runtime.

package marketplace_test

import (
	"sync"

	core "dappco.re/go"

	"dappco.re/lthn/desktop/pkg/audit"
	subject "dappco.re/lthn/desktop/pkg/marketplace"
)

// marketplaceScope mirrors the package-internal const stamped on every
// typed audit row from this package — kept here as a literal so the
// black-box test can assert on it without re-exporting the const.
const marketplaceScope = "marketplace"

// recordingRecorder captures every audit.Event the marketplace service
// emits during a test. Mirror of the same-named fixture in
// pkg/process/audit_test.go (H#193) and pkg/sandbox/audit_test.go (H#188).
type recordingRecorder struct {
	mu     sync.Mutex
	events []audit.Event
}

func (r *recordingRecorder) Record(ev audit.Event) core.Result {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, ev)
	return core.Ok(nil)
}

func (r *recordingRecorder) snapshot() []audit.Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]audit.Event, len(r.events))
	copy(out, r.events)
	return out
}

// filterByName returns only events whose Event field matches name.
func (r *recordingRecorder) filterByName(name string) []audit.Event {
	all := r.snapshot()
	out := make([]audit.Event, 0, len(all))
	for _, ev := range all {
		if ev.Event == name {
			out = append(out, ev)
		}
	}
	return out
}

// installAuditRecorder wires a recordingRecorder as audit.Default and
// restores the prior default at test end.
func installAuditRecorder(t *core.T) *recordingRecorder {
	t.Helper()
	rec := &recordingRecorder{}
	audit.SetDefault(rec)
	t.Cleanup(func() { audit.SetDefault(nil) })
	return rec
}

// TestMarketplace_Install_EmitsAuditTrail_Good drives Install against a
// marketplace service with no sandbox wired so the dispatch fails fast
// at sandbox-not-available. Asserts the Requested + Failed lifecycle
// pair lands on the audit substrate with the expected Meta shape.
func TestMarketplace_Install_EmitsAuditTrail_Good(t *core.T) {
	rec := installAuditRecorder(t)
	svc := newTestMarketplaceService()

	r := svc.Install(subject.InstallInput{
		Manifest: minimalInstallManifest,
		Env:      map[string]string{},
	})
	core.AssertFalse(t, r.OK,
		"Install against a service with no sandbox must fail")

	requested := rec.filterByName("marketplace.install.requested")
	core.AssertEqual(t, 1, len(requested), "Install must emit exactly one Requested")
	core.AssertEqual(t, marketplaceScope, requested[0].Scope)
	core.AssertEqual(t, audit.OutcomeOK, requested[0].Outcome)
	core.AssertEqual(t, "test-bundle", requested[0].Meta["bundle_id"])
	core.AssertEqual(t, "test-bundle", requested[0].Meta["plugin_code"])

	failed := rec.filterByName("marketplace.install.failed")
	core.AssertEqual(t, 1, len(failed), "sandbox-not-available must emit Failed")
	core.AssertEqual(t, audit.OutcomeFailed, failed[0].Outcome)
	core.AssertEqual(t, "test-bundle", failed[0].Meta["bundle_id"])
	_, hasErrCode := failed[0].Meta["error_code"]
	core.AssertTrue(t, hasErrCode, "Failed Meta must carry error_code key")

	// Defensive: zero Succeeded rows on a failure path.
	succeeded := rec.filterByName("marketplace.install.succeeded")
	core.AssertEqual(t, 0, len(succeeded), "Failed path must not emit Succeeded")
}

// TestMarketplace_Install_InvalidManifest_EmitsValidatePair drives
// Install with a manifest that fails ValidateManifest (missing Name)
// to confirm BOTH the InstallFailed AND the sibling
// ValidateManifestFailed row land. Forensic walker uses TS proximity
// to pair the two.
func TestMarketplace_Install_InvalidManifest_EmitsValidatePair(t *core.T) {
	rec := installAuditRecorder(t)
	svc := newTestMarketplaceService()

	bad := subject.BundleManifest{
		Schema: "lthn-vm/v1",
		Images: []subject.ImageEntry{{ID: "app", Image: "alpine:3.21"}},
	}
	r := svc.Install(subject.InstallInput{Manifest: bad})
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "invalid manifest")

	validateFailed := rec.filterByName("marketplace.validate_manifest.failed")
	core.AssertEqual(t, 1, len(validateFailed),
		"missing-name manifest must emit ValidateManifestFailed")
	core.AssertEqual(t, audit.OutcomeFailed, validateFailed[0].Outcome)
	core.AssertEqual(t, "", validateFailed[0].Meta["bundle_id"],
		"bundle_id is empty when Name is the rejection")

	installFailed := rec.filterByName("marketplace.install.failed")
	core.AssertEqual(t, 1, len(installFailed),
		"validate-rejection cascades to InstallFailed sibling")
}

// TestMarketplace_Uninstall_EmitsAuditTrail_Good drives Uninstall with
// an empty bundleID to confirm the bad-input path still emits Requested
// + Failed. The Requested fires BEFORE the empty-id check so an auditor
// catches caller misbehaviour.
func TestMarketplace_Uninstall_EmitsAuditTrail_Good(t *core.T) {
	rec := installAuditRecorder(t)
	svc := newTestMarketplaceService()

	r := svc.Uninstall("")
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "bundle id is required")

	requested := rec.filterByName("marketplace.uninstall.requested")
	core.AssertEqual(t, 1, len(requested), "Uninstall must emit Requested")
	core.AssertEqual(t, marketplaceScope, requested[0].Scope)
	core.AssertEqual(t, audit.OutcomeOK, requested[0].Outcome)

	failed := rec.filterByName("marketplace.uninstall.failed")
	core.AssertEqual(t, 1, len(failed), "empty-id must emit Failed")
	core.AssertEqual(t, audit.OutcomeFailed, failed[0].Outcome)
	_, hasErrCode := failed[0].Meta["error_code"]
	core.AssertTrue(t, hasErrCode)
}

// TestMarketplace_Launch_EmitsAuditTrail_Good drives Launch against
// an empty-id and asserts the bad-input path still emits Requested.
// The Requested fires BEFORE the empty-id check so an auditor catches
// caller misbehaviour.
func TestMarketplace_Launch_EmitsAuditTrail_Good(t *core.T) {
	rec := installAuditRecorder(t)
	svc := newTestMarketplaceService()

	r := svc.Launch("")
	core.AssertFalse(t, r.OK)

	requested := rec.filterByName("marketplace.launch.requested")
	core.AssertEqual(t, 1, len(requested), "Launch must emit Requested")
	core.AssertEqual(t, marketplaceScope, requested[0].Scope)
	core.AssertEqual(t, audit.OutcomeOK, requested[0].Outcome)

	failed := rec.filterByName("marketplace.launch.failed")
	core.AssertEqual(t, 1, len(failed), "empty-id must emit Failed")
	core.AssertEqual(t, audit.OutcomeFailed, failed[0].Outcome)

	// Defensive: zero Succeeded rows on a failure path.
	succeeded := rec.filterByName("marketplace.launch.succeeded")
	core.AssertEqual(t, 0, len(succeeded))
}

// TestMarketplace_Stop_EmitsAuditTrail_Good drives Stop against an
// unknown-bundle (Stop on unknown returns OK — idempotent). Confirms
// the Requested + Succeeded pair lands.
func TestMarketplace_Stop_EmitsAuditTrail_Good(t *core.T) {
	rec := installAuditRecorder(t)
	svc := newTestMarketplaceService()

	r := svc.Stop("nonexistent-bundle")
	core.AssertTrue(t, r.OK,
		"Stop on unknown bundle is idempotent (returns OK)")

	requested := rec.filterByName("marketplace.stop.requested")
	core.AssertEqual(t, 1, len(requested), "Stop must emit Requested")
	core.AssertEqual(t, marketplaceScope, requested[0].Scope)
	core.AssertEqual(t, "nonexistent-bundle", requested[0].Meta["bundle_id"])

	succeeded := rec.filterByName("marketplace.stop.succeeded")
	core.AssertEqual(t, 1, len(succeeded), "OK Stop emits Succeeded")
	core.AssertEqual(t, audit.OutcomeOK, succeeded[0].Outcome)

	failed := rec.filterByName("marketplace.stop.failed")
	core.AssertEqual(t, 0, len(failed), "OK path must not emit Failed")
}

// TestMarketplace_FetchManifest_EmitsAuditTrail_Good drives
// FetchManifest with an unsupported-protocol URL so the dispatch fails
// at the protocol switch. Asserts the Requested + Failed pair lands
// with domain_only Meta (not the raw URL).
func TestMarketplace_FetchManifest_EmitsAuditTrail_Good(t *core.T) {
	rec := installAuditRecorder(t)
	svc := newTestMarketplaceService()

	r := svc.FetchManifest("oci://example.com/some/path")
	core.AssertFalse(t, r.OK)

	requested := rec.filterByName("marketplace.fetch_manifest.requested")
	core.AssertEqual(t, 1, len(requested), "FetchManifest must emit Requested")
	core.AssertEqual(t, marketplaceScope, requested[0].Scope)
	core.AssertEqual(t, "example.com", requested[0].Meta["domain_only"],
		"domain_only Meta records hostname only — no path, no scheme")

	failed := rec.filterByName("marketplace.fetch_manifest.failed")
	core.AssertEqual(t, 1, len(failed), "oci:// must emit Failed")
	core.AssertEqual(t, audit.OutcomeFailed, failed[0].Outcome)
	core.AssertEqual(t, "example.com", failed[0].Meta["domain_only"])
	_, hasErrCode := failed[0].Meta["error_code"]
	core.AssertTrue(t, hasErrCode)
}

// TestMarketplace_FetchManifest_HostNotAllowed_EmitsRejected drives
// FetchManifest against an https:// URL whose host is NOT on the
// allowlist. Confirms the Rejected event fires (NOT Failed) so the
// auditor can distinguish "policy refused" from runtime/parse failure.
func TestMarketplace_FetchManifest_HostNotAllowed_EmitsRejected(t *core.T) {
	rec := installAuditRecorder(t)
	svc := newTestMarketplaceService()

	r := svc.FetchManifest("https://hostile.example.com/manifest.yml")
	core.AssertFalse(t, r.OK)

	requested := rec.filterByName("marketplace.fetch_manifest.requested")
	core.AssertEqual(t, 1, len(requested))

	rejected := rec.filterByName("marketplace.fetch_manifest.rejected")
	core.AssertEqual(t, 1, len(rejected),
		"host-allowlist gate must emit Rejected (not Failed)")
	core.AssertEqual(t, audit.OutcomeDenied, rejected[0].Outcome)
	core.AssertEqual(t, "hostile.example.com", rejected[0].Meta["domain_only"])
	core.AssertEqual(t, "host_not_allowed", rejected[0].Meta["reason"])

	// Defensive: Rejected is distinct from Failed — no Failed row
	// when the host-gate is what blocked.
	failed := rec.filterByName("marketplace.fetch_manifest.failed")
	core.AssertEqual(t, 0, len(failed),
		"host-rejection must NOT also emit Failed (distinct events)")
}

// TestMarketplace_ValidateManifest_StandaloneEmitsFailed drives
// ValidateManifest directly (not via Install) to confirm the wrapper
// emits the ValidateManifestFailed row even when standalone callers
// (parseManifestBytes / external tooling) reject a hostile manifest.
func TestMarketplace_ValidateManifest_StandaloneEmitsFailed(t *core.T) {
	rec := installAuditRecorder(t)

	bad := subject.BundleManifest{
		Schema: "wrong-schema",
		Name:   "test-bundle",
	}
	r := subject.ValidateManifest(bad)
	core.AssertFalse(t, r.OK)

	failed := rec.filterByName("marketplace.validate_manifest.failed")
	core.AssertEqual(t, 1, len(failed),
		"standalone ValidateManifest call must emit Failed on rejection")
	core.AssertEqual(t, audit.OutcomeFailed, failed[0].Outcome)
	core.AssertEqual(t, "test-bundle", failed[0].Meta["bundle_id"])

	// Defensive: NO Succeeded row on the OK path (per audit_emit.go
	// rationale — Install pipeline's InstallRequested subsumes the
	// validate-OK signal).
	okMani := subject.BundleManifest{
		Schema:  "lthn-vm/v1",
		Name:    "ok-bundle",
		Display: "OK",
		Images:  []subject.ImageEntry{{ID: "app", Image: "alpine:3.21"}},
	}
	okR := subject.ValidateManifest(okMani)
	core.AssertTrue(t, okR.OK)
	// snapshot length unchanged on OK
	allFailed := rec.filterByName("marketplace.validate_manifest.failed")
	core.AssertEqual(t, 1, len(allFailed),
		"OK validate must not emit a new Failed row")
}

// TestMarketplace_AuditMeta_NoRawSecrets_Bad asserts the Cerberus
// #1465 closure-only-scope discipline at the marketplace lifecycle
// boundary: no Meta payload across Install / Uninstall / Launch /
// Stop / FetchManifest carries raw env values / manifest body bytes /
// URL paths-with-query-strings. The redactor in pkg/audit also
// enforces this at the sink layer; the emit-site MUST NOT supply
// these in the first place (defence in depth).
func TestMarketplace_AuditMeta_NoRawSecrets_Bad(t *core.T) {
	rec := installAuditRecorder(t)
	svc := newTestMarketplaceService()

	const sensitiveEnv = "SUPERSECRETENVDONOTPERSIST"
	const sensitivePath = "SUPERSECRETPATHDONOTPERSIST"
	const sensitiveQuery = "SUPERSECRETQUERYDONOTPERSIST"

	// Drive Install — sandbox-not-available reject; we just want the
	// Meta shape on the Requested + Failed pair against a manifest
	// whose env declares the sensitive value.
	manifestWithSecret := subject.BundleManifest{
		Schema:  "lthn-vm/v1",
		Name:    "test-bundle",
		Display: "Test",
		Images:  []subject.ImageEntry{{ID: "app", Image: "alpine:3.21"}},
		Env: []subject.EnvEntry{
			{Key: "SECRET", Default: sensitiveEnv},
		},
	}
	_ = svc.Install(subject.InstallInput{
		Manifest: manifestWithSecret,
		Env:      map[string]string{"SECRET": sensitiveEnv},
	})

	// Drive FetchManifest with a URL carrying secrets in path + query.
	urlWithSecrets := "https://hostile.example.com/" + sensitivePath +
		"?token=" + sensitiveQuery
	_ = svc.FetchManifest(urlWithSecrets)

	// Drive Uninstall / Launch / Stop on ids that embed the marker.
	_ = svc.Uninstall("id-with-" + sensitiveEnv)
	_ = svc.Launch("id-with-" + sensitiveEnv)
	_ = svc.Stop("id-with-" + sensitiveEnv)

	for _, ev := range rec.snapshot() {
		// Top-level fields must not carry the sensitive bytes.
		core.AssertFalse(t, core.Contains(ev.Event, sensitiveEnv))
		core.AssertFalse(t, core.Contains(ev.Event, sensitivePath))
		core.AssertFalse(t, core.Contains(ev.Event, sensitiveQuery))
		core.AssertFalse(t, core.Contains(ev.Scope, sensitiveEnv))

		// Walk Meta string values. bundle_id / plugin_code legitimately
		// carry caller-supplied identifiers (Uninstall/Launch/Stop) —
		// id content is not a secret per se (a future redactor pass MAY
		// hash these). But error_code / domain_only / reason fields
		// must never echo the sensitive bytes.
		for k, v := range ev.Meta {
			if k == "bundle_id" || k == "plugin_code" {
				continue
			}
			if s, ok := v.(string); ok {
				core.AssertFalse(t, core.Contains(s, sensitiveEnv),
					"Meta["+k+"] must not echo env bytes (event="+ev.Event+")")
				core.AssertFalse(t, core.Contains(s, sensitivePath),
					"Meta["+k+"] must not echo URL-path bytes (event="+ev.Event+")")
				core.AssertFalse(t, core.Contains(s, sensitiveQuery),
					"Meta["+k+"] must not echo URL-query bytes (event="+ev.Event+")")
			}
		}
		// Reserved smuggle-surface canaries — the emit-site is the only
		// place to enforce these so an audit reader can trust the schema.
		_, hasEnv := ev.Meta["env"]
		core.AssertFalse(t, hasEnv, "Meta must never carry an `env` key")
		_, hasManifest := ev.Meta["manifest"]
		core.AssertFalse(t, hasManifest, "Meta must never carry a raw `manifest` key")
		_, hasURL := ev.Meta["url"]
		core.AssertFalse(t, hasURL,
			"Meta must never carry a raw `url` key (only domain_only)")
		_, hasPath := ev.Meta["path"]
		core.AssertFalse(t, hasPath, "Meta must never carry a raw `path` key")
		_, hasQuery := ev.Meta["query"]
		core.AssertFalse(t, hasQuery, "Meta must never carry a raw `query` key")
	}
}
