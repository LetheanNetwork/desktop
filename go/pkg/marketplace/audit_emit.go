// SPDX-Licence-Identifier: EUPL-1.2

// audit_emit.go — typed pkg/audit emit helpers for the marketplace
// lifecycle quartet (Install / Uninstall / Launch / Stop) +
// FetchManifest + ValidateManifest. Cerberus #54 C4 (Mantis #1692) —
// Repudiation gap close. Marketplace operations previously emitted
// ZERO audit events; bundle install + plugin-view registration + CSP
// allowlist mutation were forensically silent.
//
// Helper functions wrap audit.Default().Record so the call-site at
// each lifecycle entry-point stays single-line. The helper enforces
// the Cerberus #1465 closure-only-scope discipline at the boundary:
// only the safe meta keys (bundle_id / plugin_code / domain_only /
// image_count / error_code / reason) are accepted; raw manifest /
// env / image-ref / fetch-URL-path bytes never reach the substrate.
//
// Failed helpers take a core.Result and resolve Meta["error_code"]
// through audit.ErrorCode(r) — bounded *core.Err.Operation literal
// rather than raw err.Error() prose. This is the W5 cascade adoption
// per RFC.error-code-cascade.md §3 / Mantis #1715 / Cerberus #61 C-3:
// caller-controlled bytes (URLs, manifest names, error wrap strings)
// can no longer reach the audit substrate via the helper boundary.
// Bare-string sites construct core.Fail(core.E(op, msg, nil)) before
// emit so the single boundary type holds across the whole package.
//
// Usage example (internal):
//
//	emitInstallRequested(bundleID, pluginCode)
//	emitInstallSucceeded(bundleID, pluginCode, len(sandboxIDs))
//	if !r.OK { emitInstallFailed(bundleID, r) }

package marketplace

import (
	core "dappco.re/go"

	"dappco.re/lthn/desktop/pkg/audit"
)

// marketplaceScope is the Event.Scope literal stamped on every typed
// audit row emitted from this package. Closed-set per RFC.stage-f.md §2;
// the Operations panel's facet chrome learns this literal in the same
// commit a new scope ships.
const marketplaceScope = "marketplace"

// reasonHostNotAllowed is the closed-set Meta["reason"] literal stamped
// on EventMarketplaceFetchManifestRejected rows when the host-allowlist
// gate (requireAllowedManifestHost) rejects the caller-supplied URL.
// Reserved literal so a downstream chip-filter UI / forensic walker can
// categorise without re-running the gate logic.
const reasonHostNotAllowed = "host_not_allowed"

// emitInstallRequested fires the Requested row at the top of
// Install — BEFORE the sandbox SpawnLong loop or orm.Save so a crash
// mid-call still leaves the operator intent in the audit substrate.
//
// Usage example (internal):
//
//	emitInstallRequested(m.Name, pluginCodeOf(m))
func emitInstallRequested(bundleID, pluginCode string) {
	_ = audit.Default().Record(audit.Event{
		Event:   audit.EventMarketplaceInstallRequested,
		TS:      core.Now().UTC().Unix(),
		Scope:   marketplaceScope,
		Outcome: audit.OutcomeOK,
		Meta: map[string]any{
			"bundle_id":   bundleID,
			"plugin_code": pluginCode,
		},
	})
}

// emitInstallSucceeded fires the Succeeded row at the end of Install
// when the full pipeline (validate / spawn / persist / register) lands
// without error. imageCount is the spawned-image tally surfaced via
// InstallOutput.SandboxIDs.
//
// Usage example (internal):
//
//	emitInstallSucceeded(m.Name, pluginCodeOf(m), len(sandboxIDs))
func emitInstallSucceeded(bundleID, pluginCode string, imageCount int) {
	_ = audit.Default().Record(audit.Event{
		Event:   audit.EventMarketplaceInstallSucceeded,
		TS:      core.Now().UTC().Unix(),
		Scope:   marketplaceScope,
		Outcome: audit.OutcomeOK,
		Meta: map[string]any{
			"bundle_id":   bundleID,
			"plugin_code": pluginCode,
			"image_count": imageCount,
		},
	})
}

// emitInstallFailed fires the Failed row on any non-OK Install Result
// (validation, sandbox-spawn, orm-save, rollback). bundleID may be
// empty when ValidateManifest rejected the input before Name resolved.
//
// r is the failed core.Result whose stable, bounded error_code is
// derived through audit.ErrorCode per W5 of the audit error-code
// cascade RFC. Bare-string sites wrap as core.Fail(core.E(op, msg,
// nil)) before calling — single boundary type keeps Meta["error_code"]
// inside the closed keyspace and forbids raw prose leakage via
// err.Error().
//
// Usage example (internal):
//
//	if !r.OK { emitInstallFailed(m.Name, r); return r }
//	fail := core.Fail(core.E(installBundleOp, "sandbox service not available", nil))
//	emitInstallFailed(m.Name, fail); return fail
func emitInstallFailed(bundleID string, r core.Result) {
	_ = audit.Default().Record(audit.Event{
		Event:   audit.EventMarketplaceInstallFailed,
		TS:      core.Now().UTC().Unix(),
		Scope:   marketplaceScope,
		Outcome: audit.OutcomeFailed,
		Meta: map[string]any{
			"bundle_id":  bundleID,
			"error_code": audit.ErrorCode(r),
		},
	})
}

// emitUninstallRequested fires the Requested row at the top of
// Uninstall — BEFORE the plugin-view registry drop / sandbox teardown
// / orm.Delete so the operator intent survives a mid-uninstall crash.
//
// Usage example (internal):
//
//	emitUninstallRequested(bundleID, s.resolvePluginCode(bundleID))
func emitUninstallRequested(bundleID, pluginCode string) {
	_ = audit.Default().Record(audit.Event{
		Event:   audit.EventMarketplaceUninstallRequested,
		TS:      core.Now().UTC().Unix(),
		Scope:   marketplaceScope,
		Outcome: audit.OutcomeOK,
		Meta: map[string]any{
			"bundle_id":   bundleID,
			"plugin_code": pluginCode,
		},
	})
}

// emitUninstallSucceeded fires the Succeeded row at the end of
// Uninstall when the multi-step teardown lands without error.
//
// Usage example (internal):
//
//	emitUninstallSucceeded(bundleID, pluginCode)
func emitUninstallSucceeded(bundleID, pluginCode string) {
	_ = audit.Default().Record(audit.Event{
		Event:   audit.EventMarketplaceUninstallSucceeded,
		TS:      core.Now().UTC().Unix(),
		Scope:   marketplaceScope,
		Outcome: audit.OutcomeOK,
		Meta: map[string]any{
			"bundle_id":   bundleID,
			"plugin_code": pluginCode,
		},
	})
}

// emitUninstallFailed fires the Failed row on any non-OK Uninstall
// Result (empty id, orm delete failure post-sandbox-teardown). See
// emitInstallFailed for the W5 audit.ErrorCode(r) contract.
//
// Usage example (internal):
//
//	if !r.OK { emitUninstallFailed(bundleID, r); return r }
func emitUninstallFailed(bundleID string, r core.Result) {
	_ = audit.Default().Record(audit.Event{
		Event:   audit.EventMarketplaceUninstallFailed,
		TS:      core.Now().UTC().Unix(),
		Scope:   marketplaceScope,
		Outcome: audit.OutcomeFailed,
		Meta: map[string]any{
			"bundle_id":  bundleID,
			"error_code": audit.ErrorCode(r),
		},
	})
}

// emitLaunchRequested fires the Requested row at the top of Launch
// BEFORE the manifest read / sandbox re-spawn loop. Distinguishes
// Install-time spawn from post-restart re-spawn in audit history.
//
// Usage example (internal):
//
//	emitLaunchRequested(bundleID)
func emitLaunchRequested(bundleID string) {
	_ = audit.Default().Record(audit.Event{
		Event:   audit.EventMarketplaceLaunchRequested,
		TS:      core.Now().UTC().Unix(),
		Scope:   marketplaceScope,
		Outcome: audit.OutcomeOK,
		Meta: map[string]any{
			"bundle_id": bundleID,
		},
	})
}

// emitLaunchSucceeded fires the Succeeded row at the end of Launch
// when every image re-spawn + orm.Save lands. imageCount is the
// re-spawned-image tally.
//
// Usage example (internal):
//
//	emitLaunchSucceeded(bundleID, len(launchedIDs))
func emitLaunchSucceeded(bundleID string, imageCount int) {
	_ = audit.Default().Record(audit.Event{
		Event:   audit.EventMarketplaceLaunchSucceeded,
		TS:      core.Now().UTC().Unix(),
		Scope:   marketplaceScope,
		Outcome: audit.OutcomeOK,
		Meta: map[string]any{
			"bundle_id":   bundleID,
			"image_count": imageCount,
		},
	})
}

// emitLaunchFailed fires the Failed row on any non-OK Launch Result
// (empty id, bundle not installed, manifest missing, orm save failure).
// See emitInstallFailed for the W5 audit.ErrorCode(r) contract.
//
// Usage example (internal):
//
//	if !r.OK { emitLaunchFailed(bundleID, r); return r }
func emitLaunchFailed(bundleID string, r core.Result) {
	_ = audit.Default().Record(audit.Event{
		Event:   audit.EventMarketplaceLaunchFailed,
		TS:      core.Now().UTC().Unix(),
		Scope:   marketplaceScope,
		Outcome: audit.OutcomeFailed,
		Meta: map[string]any{
			"bundle_id":  bundleID,
			"error_code": audit.ErrorCode(r),
		},
	})
}

// emitStopRequested fires the Requested row at the top of Stop BEFORE
// the sandbox Kill loop + orm Status flip.
//
// Usage example (internal):
//
//	emitStopRequested(bundleID)
func emitStopRequested(bundleID string) {
	_ = audit.Default().Record(audit.Event{
		Event:   audit.EventMarketplaceStopRequested,
		TS:      core.Now().UTC().Unix(),
		Scope:   marketplaceScope,
		Outcome: audit.OutcomeOK,
		Meta: map[string]any{
			"bundle_id": bundleID,
		},
	})
}

// emitStopSucceeded fires the Succeeded row at the end of Stop.
//
// Usage example (internal):
//
//	emitStopSucceeded(bundleID)
func emitStopSucceeded(bundleID string) {
	_ = audit.Default().Record(audit.Event{
		Event:   audit.EventMarketplaceStopSucceeded,
		TS:      core.Now().UTC().Unix(),
		Scope:   marketplaceScope,
		Outcome: audit.OutcomeOK,
		Meta: map[string]any{
			"bundle_id": bundleID,
		},
	})
}

// emitStopFailed fires the Failed row on any non-OK Stop Result.
// See emitInstallFailed for the W5 audit.ErrorCode(r) contract.
//
// Usage example (internal):
//
//	if !r.OK { emitStopFailed(bundleID, r); return r }
func emitStopFailed(bundleID string, r core.Result) {
	_ = audit.Default().Record(audit.Event{
		Event:   audit.EventMarketplaceStopFailed,
		TS:      core.Now().UTC().Unix(),
		Scope:   marketplaceScope,
		Outcome: audit.OutcomeFailed,
		Meta: map[string]any{
			"bundle_id":  bundleID,
			"error_code": audit.ErrorCode(r),
		},
	})
}

// emitFetchManifestRequested fires the Requested row at the top of
// FetchManifest BEFORE the host-allowlist gate + HTTP GET. Meta carries
// the URL hostname only (no path / query / fragment) per the brief's
// SECURITY-NOTE escape valve — manifest URLs occasionally embed agent
// identifiers in query strings; recording host-only keeps the row
// forensic without leaking path-traversal evidence attempts.
//
// Usage example (internal):
//
//	emitFetchManifestRequested(sourceURL)
func emitFetchManifestRequested(sourceURL string) {
	_ = audit.Default().Record(audit.Event{
		Event:   audit.EventMarketplaceFetchManifestRequested,
		TS:      core.Now().UTC().Unix(),
		Scope:   marketplaceScope,
		Outcome: audit.OutcomeOK,
		Meta: map[string]any{
			"domain_only": domainOnly(sourceURL),
		},
	})
}

// emitFetchManifestSucceeded fires the Succeeded row at the end of
// FetchManifest when fetch + parse + validate all land. bundleID is
// the resolved manifest.Name post-parse.
//
// Usage example (internal):
//
//	emitFetchManifestSucceeded(sourceURL, manifest.Name)
func emitFetchManifestSucceeded(sourceURL, bundleID string) {
	_ = audit.Default().Record(audit.Event{
		Event:   audit.EventMarketplaceFetchManifestSucceeded,
		TS:      core.Now().UTC().Unix(),
		Scope:   marketplaceScope,
		Outcome: audit.OutcomeOK,
		Meta: map[string]any{
			"domain_only": domainOnly(sourceURL),
			"bundle_id":   bundleID,
		},
	})
}

// emitFetchManifestFailed fires the Failed row on any non-OK
// FetchManifest Result EXCEPT the host-allowlist rejection (which has
// its own Rejected event so the auditor can distinguish policy-block
// from network/parse failure). See emitInstallFailed for the W5
// audit.ErrorCode(r) contract.
//
// Usage example (internal):
//
//	if !r.OK { emitFetchManifestFailed(sourceURL, r); return r }
func emitFetchManifestFailed(sourceURL string, r core.Result) {
	_ = audit.Default().Record(audit.Event{
		Event:   audit.EventMarketplaceFetchManifestFailed,
		TS:      core.Now().UTC().Unix(),
		Scope:   marketplaceScope,
		Outcome: audit.OutcomeFailed,
		Meta: map[string]any{
			"domain_only": domainOnly(sourceURL),
			"error_code":  audit.ErrorCode(r),
		},
	})
}

// emitFetchManifestRejected fires when the host-allowlist gate refuses
// the caller-supplied URL. Sibling of EventSandboxVolumeRejected — the
// Rejected row distinguishes policy-block from runtime/parse failure.
//
// Usage example (internal):
//
//	if !isAllowedManifestHost(url) {
//	    emitFetchManifestRejected(url)
//	    return ...
//	}
func emitFetchManifestRejected(sourceURL string) {
	_ = audit.Default().Record(audit.Event{
		Event:   audit.EventMarketplaceFetchManifestRejected,
		TS:      core.Now().UTC().Unix(),
		Scope:   marketplaceScope,
		Outcome: audit.OutcomeDenied,
		Meta: map[string]any{
			"domain_only": domainOnly(sourceURL),
			"reason":      reasonHostNotAllowed,
		},
	})
}

// emitValidateManifestFailed fires the Failed row when ValidateManifest
// rejects a parsed BundleManifest (bad name, unsupported schema,
// malformed plugin block, invalid view custom-element tag, yaml-bomb,
// imagetrust rejection, etc). bundleID is the manifest.Name (may be
// empty when the rejection was on Name itself).
//
// NOTE: the success path emits NO Succeeded row — every Install /
// FetchManifest call validates as part of its own pipeline; emitting
// Succeeded per validate would flood the substrate with duplicate
// install-OK signals. The Install pipeline's InstallRequested subsumes
// the validate-OK case.
//
// Usage example (internal):
//
//	if !r.OK { emitValidateManifestFailed(m.Name, r); return r }
//
// See emitInstallFailed for the W5 audit.ErrorCode(r) contract.
func emitValidateManifestFailed(bundleID string, r core.Result) {
	_ = audit.Default().Record(audit.Event{
		Event:   audit.EventMarketplaceValidateManifestFailed,
		TS:      core.Now().UTC().Unix(),
		Scope:   marketplaceScope,
		Outcome: audit.OutcomeFailed,
		Meta: map[string]any{
			"bundle_id":  bundleID,
			"error_code": audit.ErrorCode(r),
		},
	})
}

// emitTrustedKeyMutated records an EventMarketplaceTrustedKeyMutated
// row for the DREAD F4 / F1-cluster trust-store mutation surface
// (Mantis #1644 / #1647). Outcome is OutcomeDenied when reason is
// reasonDuplicate (the DREAD N1 load-time reject path); every other
// reason is OutcomeOK (the authorised-mutation path).
//
// Meta keys mirror the pkg/audit/types.go const-block contract:
// name, old_fingerprint, new_fingerprint, reason, operator_confirmed.
// fingerprints are SHA256(pubkey) first 16 hex chars per signing.go's
// fingerprintFrom — short enough to scan but unique enough to
// distinguish keys forensically.
//
// Usage example (internal):
//
//	emitTrustedKeyMutated("lthn-official", "", "ab12cd...", reasonAdd, true)
func emitTrustedKeyMutated(name, oldFp, newFp, reason string, operatorConfirmed bool) {
	outcome := audit.OutcomeOK
	if reason == reasonDuplicate {
		outcome = audit.OutcomeDenied
	}
	_ = audit.Default().Record(audit.Event{
		Event:   audit.EventMarketplaceTrustedKeyMutated,
		TS:      core.Now().UTC().Unix(),
		Scope:   marketplaceScope,
		Outcome: outcome,
		Meta: map[string]any{
			"name":               name,
			"old_fingerprint":    oldFp,
			"new_fingerprint":    newFp,
			"reason":             reason,
			"operator_confirmed": operatorConfirmed,
		},
	})
}

// domainOnly returns the URL hostname only — no scheme, no path, no
// query, no fragment. Returns "" for malformed input; the audit emit
// helpers tolerate the empty value (a downstream forensic walker
// distinguishes empty-domain-only as "URL was malformed enough to
// reject pre-parse" vs the populated case).
//
// Mirrors pkg/process's command_hash discipline at the URL boundary:
// the audit surface records the categorical fact (which host) without
// the sensitive specifics (which path / which query string).
//
// Usage example (internal):
//
//	host := domainOnly("https://forge.lthn.sh/x/y?token=secret")
//	// host == "forge.lthn.sh"
func domainOnly(rawURL string) string {
	if core.Trim(rawURL) == "" {
		return ""
	}
	r := core.URLParse(rawURL)
	if !r.OK {
		return ""
	}
	u, _ := r.Value.(*core.URL)
	if u == nil {
		return ""
	}
	return core.Lower(u.Hostname())
}
