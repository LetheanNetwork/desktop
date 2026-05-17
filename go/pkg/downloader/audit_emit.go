// SPDX-Licence-Identifier: EUPL-1.2

// audit_emit.go — typed pkg/audit emit helpers for the downloader
// FetchVerified supply-chain primary. Cerberus shape-observation
// follow-on to the 5th audit-cluster adoption (gateway H#211, marketplace
// H#208, process H#193, sandbox H#188, runner H#181). FetchVerified is
// the model-weight ingest path mirroring marketplace's FetchManifest —
// both share the host-allowlist + DNS-resolve-private-IP gates, both
// emit the same Requested / Succeeded / Failed / Rejected quartet.
//
// Helper functions wrap audit.Default().Record so the call-site inside
// FetchVerified stays single-line at each branch. The helper enforces
// the Cerberus #1465 closure-only-scope discipline at the boundary:
// only the safe meta keys (source_host / sha256_expected / bytes /
// error_code / reason) are accepted; raw URL path / Authorization
// bytes / response body never reach the substrate.
//
// Usage example (internal):
//
//	emitFetchRequested(url, sha256hex)
//	emitFetchSucceeded(url, written)
//	if !r.OK { emitFetchFailed(url, r.Error()); return r }
//	if !AllowedSource(url) {
//	    emitFetchRejected(url, reasonHostNotAllowed)
//	    return ...
//	}

package downloader

import (
	core "dappco.re/go"

	"dappco.re/lthn/desktop/pkg/audit"
)

// downloaderScope is the Event.Scope literal stamped on every typed
// audit row emitted from this package. Closed-set per RFC.stage-f.md §2;
// the Operations panel's facet chrome learns this literal in the same
// commit a new scope ships.
const downloaderScope = "downloader"

// reasonHostNotAllowed is the closed-set Meta["reason"] literal stamped
// on EventDownloaderFetchRejected rows when AllowedSource refuses the
// caller-supplied URL. Reserved literal so a downstream chip-filter UI /
// forensic walker can categorise without re-running the suffix-match
// gate.
const reasonHostNotAllowed = "host_not_allowed"

// reasonPrivateIPResolved is the closed-set Meta["reason"] literal
// stamped on EventDownloaderFetchRejected rows when
// verifyResolvedIPNotPrivate fires (Cerberus #1429 SSRF defence). Mirrors
// the marketplace.reasonHostNotAllowed shape — each rejection reason
// gets its own reserved literal so the forensic walker can pattern-match
// without running gate logic.
const reasonPrivateIPResolved = "private_ip_resolved"

// emitFetchRequested fires the Requested row at the top of
// FetchVerified — AFTER the URL + name shape gates but BEFORE the
// host-allowlist + DNS-resolve-private-IP gates. The pre-gate position
// lets a forensic walker correlate caller intent (which host) with the
// gate outcome (which policy fired) in the same trace.
//
// Usage example (internal):
//
//	emitFetchRequested(url, sha256hex)
func emitFetchRequested(sourceURL, sha256hex string) {
	_ = audit.Default().Record(audit.Event{
		Event:   audit.EventDownloaderFetchRequested,
		TS:      core.Now().UTC().Unix(),
		Scope:   downloaderScope,
		Outcome: audit.OutcomeOK,
		Meta: map[string]any{
			"source_host":     domainOnly(sourceURL),
			"sha256_expected": sha256hex,
		},
	})
}

// emitFetchSucceeded fires the Succeeded row at the end of FetchVerified
// when the full pipeline (gate / GET / quarantine / digest-verify /
// atomic-promote) lands without error. bytes is the byte count from the
// stream copy — kept for forensic correlation against disk-usage /
// bandwidth accounting.
//
// Usage example (internal):
//
//	emitFetchSucceeded(url, written)
func emitFetchSucceeded(sourceURL string, bytes int64) {
	_ = audit.Default().Record(audit.Event{
		Event:   audit.EventDownloaderFetchSucceeded,
		TS:      core.Now().UTC().Unix(),
		Scope:   downloaderScope,
		Outcome: audit.OutcomeOK,
		Meta: map[string]any{
			"source_host": domainOnly(sourceURL),
			"bytes":       bytes,
		},
	})
}

// emitFetchFailed fires the Failed row on any non-OK FetchVerified
// Result EXCEPT the host-allowlist + private-IP gate rejections (which
// have their own Rejected event so the auditor can distinguish policy-
// block from runtime / network / digest-mismatch failure).
//
// Usage example (internal):
//
//	if !r.OK { emitFetchFailed(url, r.Error()); return r }
func emitFetchFailed(sourceURL, errorCode string) {
	_ = audit.Default().Record(audit.Event{
		Event:   audit.EventDownloaderFetchFailed,
		TS:      core.Now().UTC().Unix(),
		Scope:   downloaderScope,
		Outcome: audit.OutcomeFailed,
		Meta: map[string]any{
			"source_host": domainOnly(sourceURL),
			"error_code":  errorCode,
		},
	})
}

// emitFetchRejected fires when FetchVerified rejects the caller-supplied
// URL at a policy gate. reason MUST be one of the closed-set literals
// declared above (reasonHostNotAllowed / reasonPrivateIPResolved); a
// future gate addition reserves its own literal in the same commit it
// introduces the gate.
//
// Usage example (internal):
//
//	if !AllowedSource(url) {
//	    emitFetchRejected(url, reasonHostNotAllowed)
//	    return core.Fail(...)
//	}
func emitFetchRejected(sourceURL, reason string) {
	_ = audit.Default().Record(audit.Event{
		Event:   audit.EventDownloaderFetchRejected,
		TS:      core.Now().UTC().Unix(),
		Scope:   downloaderScope,
		Outcome: audit.OutcomeDenied,
		Meta: map[string]any{
			"source_host": domainOnly(sourceURL),
			"reason":      reason,
		},
	})
}

// domainOnly returns the lowercase hostname of rawURL with no path /
// query / fragment / port. Mirrors pkg/marketplace.domainOnly verbatim —
// per the SECURITY-NOTE escape valve in the dispatch brief the audit
// surface records the categorical fact (which host) without the
// sensitive specifics (which path / which query string / which token).
//
// Returns "" for empty or unparseable URLs — the audit row keeps the
// empty source_host marker so a forensic walker can distinguish "fetch
// against a malformed URL" from "fetch against allowlisted host".
//
// Usage example (internal):
//
//	host := domainOnly("https://hf.co/foo/resolve/main/x.gguf?token=secret")
//	// host == "hf.co"
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
