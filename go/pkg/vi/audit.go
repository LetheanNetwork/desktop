// SPDX-Licence-Identifier: EUPL-1.2

// audit.go — typed audit emit helpers for the Vi PR-fetch + probe
// surfaces. 9th audit-cluster adoption per Cerberus #72 F-2 (Mantis
// #1749) — sibling shape of the closed clusters across runner /
// sandbox / process / marketplace / gateway / downloader / queue /
// sessions.
//
// PII / secret-shape discipline (Cerberus #1465 closure-only-scope):
//
//   - The PAT bytes returned by LoadToken are NEVER recorded — only
//     the binary "token_set" observation.
//   - The full upstream URL (which may carry pagination cursors or,
//     in the rejected case, a hostile `javascript:` payload) is
//     NEVER recorded — the (provider, owner, name) tuple plus
//     pr_number is structurally equivalent for a forensic walker.
//   - Raw error strings from doFetch / doProbe never reach Meta —
//     audit.ErrorCode(r) derives a bounded canon at the boundary.

package vi

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/audit"
)

// viScope is the Event.Scope literal stamped on every typed audit
// row emitted from this package — 9th audit-cluster adoption per
// Cerberus #72 F-2 (Mantis #1749). The Operations panel's facet
// chrome learns this literal in the same commit a new scope ships.
const viScope = "vi"

// Closed-set reason literals for EventViPRFetchRejected. Keep in
// lockstep with the docstring in pkg/audit/types.go and the chip-
// filter UI in <lthn-audit-viewer>.
const (
	rejectReasonUnsafeURL = "unsafe_url"
)

// emitPRFetchRequested fires the Requested row at the top of Fetch
// after the surface guards but BEFORE the LoadToken call. Bytes the
// caller controls (PAT, raw URL, query) never reach Meta — only the
// closed-set (provider, owner, name) tuple.
//
// Usage example (internal):
//
//	emitPRFetchRequested(repo, tokenSet)
func emitPRFetchRequested(repo PRRepo, tokenSet bool) {
	_ = audit.Default().Record(audit.Event{
		Event:   audit.EventViPRFetchRequested,
		TS:      core.Now().UTC().Unix(),
		Scope:   viScope,
		Outcome: audit.OutcomeOK,
		Meta: map[string]any{
			"provider":   repo.Provider,
			"repo_owner": repo.Owner,
			"repo_name":  repo.Name,
			"token_set":  tokenSet,
		},
	})
}

// emitPRTokenLoaded fires immediately after LoadToken returns. Single
// event — LoadToken's outcome at the audit substrate is binary (set
// vs not-set); the token bytes are NEVER recorded.
//
// Usage example (internal):
//
//	emitPRTokenLoaded(repo.Provider, token != "")
func emitPRTokenLoaded(provider string, tokenSet bool) {
	_ = audit.Default().Record(audit.Event{
		Event:   audit.EventViPRTokenLoaded,
		TS:      core.Now().UTC().Unix(),
		Scope:   viScope,
		Outcome: audit.OutcomeOK,
		Meta: map[string]any{
			"provider":  provider,
			"token_set": tokenSet,
		},
	})
}

// emitPRFetchSucceeded fires when Fetch completes the upstream round-
// trip + per-row persistence loop with a non-error status. The
// 401 / 403 / other 4xx paths route here too — the upstream
// responded, the auditor pairs http_status to derive intent.
//
// Usage example (internal):
//
//	emitPRFetchSucceeded(repo, status, inserted)
func emitPRFetchSucceeded(repo PRRepo, httpStatus, prCount int) {
	_ = audit.Default().Record(audit.Event{
		Event:   audit.EventViPRFetchSucceeded,
		TS:      core.Now().UTC().Unix(),
		Scope:   viScope,
		Outcome: audit.OutcomeOK,
		Meta: map[string]any{
			"provider":    repo.Provider,
			"repo_owner":  repo.Owner,
			"repo_name":   repo.Name,
			"http_status": httpStatus,
			"pr_count":    prCount,
		},
	})
}

// emitPRFetchFailed fires when doFetch returns a transport-side
// error so the row loop is skipped. error_code is bounded via
// audit.ErrorCode(r); the raw doFetch error string never reaches
// Meta.
//
// Usage example (internal):
//
//	emitPRFetchFailed(repo, status, core.Fail(core.E("vi.Fetch", err, nil)))
func emitPRFetchFailed(repo PRRepo, httpStatus int, r core.Result) {
	_ = audit.Default().Record(audit.Event{
		Event:   audit.EventViPRFetchFailed,
		TS:      core.Now().UTC().Unix(),
		Scope:   viScope,
		Outcome: audit.OutcomeFailed,
		Meta: map[string]any{
			"provider":    repo.Provider,
			"repo_owner":  repo.Owner,
			"repo_name":   repo.Name,
			"http_status": httpStatus,
			"error_code":  audit.ErrorCode(r),
		},
	})
}

// emitPRFetchRejected fires per-row when Fetch drops an upstream PR
// at the isSafePRURL gate. Reason is the closed-set literal; the
// dropped URL bytes are NEVER in Meta (the URL itself is the hostile
// payload — recording it would persist the attack string into the
// long-lived forensic substrate).
//
// Usage example (internal):
//
//	emitPRFetchRejected(repo, pr.Number, rejectReasonUnsafeURL)
func emitPRFetchRejected(repo PRRepo, prNumber int, reason string) {
	_ = audit.Default().Record(audit.Event{
		Event:   audit.EventViPRFetchRejected,
		TS:      core.Now().UTC().Unix(),
		Scope:   viScope,
		Outcome: audit.OutcomeDenied,
		Meta: map[string]any{
			"provider":   repo.Provider,
			"repo_owner": repo.Owner,
			"repo_name":  repo.Name,
			"pr_number":  prNumber,
			"reason":     reason,
		},
	})
}

// emitProbeRequested fires at the top of Probe after the surface
// guards but BEFORE the network round-trip. domain is the host only
// (no scheme / path) per DomainOf.
//
// Usage example (internal):
//
//	emitProbeRequested(domain)
func emitProbeRequested(domain string) {
	_ = audit.Default().Record(audit.Event{
		Event:   audit.EventViProbeRequested,
		TS:      core.Now().UTC().Unix(),
		Scope:   viScope,
		Outcome: audit.OutcomeOK,
		Meta: map[string]any{
			"domain": domain,
		},
	})
}

// emitProbeSucceeded fires when probeClient.Do returns without
// transport error. 4xx / 5xx upstream statuses route here too — the
// audit row records "the probe ran"; the OK derivation lives on
// the persisted SiteProbe row.
//
// Usage example (internal):
//
//	emitProbeSucceeded(domain, statusCode, latencyMs)
func emitProbeSucceeded(domain string, httpStatus, latencyMs int) {
	_ = audit.Default().Record(audit.Event{
		Event:   audit.EventViProbeSucceeded,
		TS:      core.Now().UTC().Unix(),
		Scope:   viScope,
		Outcome: audit.OutcomeOK,
		Meta: map[string]any{
			"domain":      domain,
			"http_status": httpStatus,
			"latency_ms":  latencyMs,
		},
	})
}

// emitProbeFailed fires when Probe hits a surface-guard rejection,
// a transport error from probeClient.Do, or an orm.Insert failure
// at the persistence bottom. error_code is bounded via
// audit.ErrorCode(r); raw doProbe error string never reaches Meta.
//
// Usage example (internal):
//
//	emitProbeFailed(domain, r)
func emitProbeFailed(domain string, r core.Result) {
	_ = audit.Default().Record(audit.Event{
		Event:   audit.EventViProbeFailed,
		TS:      core.Now().UTC().Unix(),
		Scope:   viScope,
		Outcome: audit.OutcomeFailed,
		Meta: map[string]any{
			"domain":     domain,
			"error_code": audit.ErrorCode(r),
		},
	})
}
