// SPDX-Licence-Identifier: EUPL-1.2

// errorcode.go — bounded-keyspace audit error_code substrate per
// plans/code/lthn/desktop/audit/RFC.error-code-cascade.md §1
// (CLEAN-FOR-IMPL, v2 — Cerberus #62 DREAD folded). The cluster-wide
// promise: the result of ErrorCode is the ONLY value that may be
// assigned to audit Event Meta["error_code"]. Type-system enforcement
// of the bounded-keyspace contract — not docstring discipline.
//
// Motivation — Cerberus #1710 / Mantis #1718 cluster verdict: upstream
// providers (OpenAI / vLLM / docker / process stderr / HTTP error
// bodies) format errors as raw prose that routinely echoes caller-
// controlled input (URLs, file paths, prompts, Authorization bytes).
// Assigning r.Error() / err.Error() directly to Meta["error_code"]
// turns the audit substrate into a STRIDE-I leak surface and bypasses
// the pkg/audit secret-shape redactor (which is token-shaped, not
// prose-shaped).
//
// Wave 1 ships the substrate alone — NO callers migrate in this
// commit. The seven-wave cascade per RFC §3 sequences W2 process ->
// W3 sandbox (+ cause_error sweep, Mantis #1719) -> W4 downloader ->
// W5 marketplace -> W6 gateway -> W7 runner (replaces local helper
// at pkg/runner/service.go:427 + migrates "provider_error" literal
// to canon "unknown_error" per FOLD-1).
//
// Mirrors:
//
//   - pkg/runner/service.go:427-441 — canonical local helper (H#218
//     close of Cerberus #1710 F-1); W7 deletes after substrate adopt.
//   - pkg/imagetrust/types.go:76 — typed-enum specialisation for the
//     image-ref validator. Stays distinct (different signature, typed
//     sentinel switch). sandbox.go:163 hybrid pattern (try imagetrust
//     first, fall back to audit.ErrorCode) is the canonical
//     composition shape — see ErrorCode docstring Usage example.

package audit

import (
	core "dappco.re/go"
)

// codeUnknown is the cluster-wide canonical fallback literal returned
// when neither r.Code() nor r.Value's *core.Err.Operation is populated.
// Per RFC §2.1 FOLD-1: substrate owns the canonical name "unknown_error".
// W7 migrates the runner-local "provider_error" literal to this canon
// in the same commit that deletes pkg/runner.errorCode().
//
// Adding a new fallback literal is a reserved-schema change — it forks
// canon by package, which is exactly the failure mode the substrate
// exists to prevent. Any future shape that needs richer classification
// belongs in v2 dual-key ErrorCode/ErrorScope (forward-arc, Mantis
// #1720) — not a sibling fallback string.
const codeUnknown = "unknown_error"

// ErrorCode derives a stable, bounded audit error_code from a failed
// core.Result. The function NEVER returns the raw r.Error() prose
// because that string routinely echoes caller-controlled input.
//
// Resolution order (RFC §1.1):
//
//  1. r.OK == true → "" (no error, no code).
//  2. r.Code() if non-empty (the canonical Lethean codespace, e.g.
//     "ai.openai.provider").
//  3. (*core.Err).Operation if r.Value wraps a *core.Err and Operation
//     is non-empty (e.g. "marketplace.install").
//  4. "unknown_error" — cluster-wide canonical fallback (FOLD-1).
//
// Bare-error sites wrap at the call site per RFC Q-3 AFFIRM:
//
//	emitFetchFailed(url, core.Fail(err))   // not ErrorCodeFromErr(err)
//
// Single boundary type (core.Result) is the substrate's value
// proposition — overloading by two helpers fragments the type-system
// anchor the FOLD-1 fallback canon relies on.
//
// Composition shape for typed-error-aware sites (e.g. sandbox.go:163
// per RFC §4.2):
//
//	code := imagetrust.ErrorCode(typedErr)
//	if code == "image_invalid" {                  // generic bucket
//	    code = audit.ErrorCode(core.Fail(typedErr))
//	}
//	_ = audit.Default().Record(audit.Event{
//	    Meta: map[string]any{"error_code": code},
//	})
//
// Standard usage example:
//
//	if !r.OK {
//	    _ = audit.Default().Record(audit.Event{
//	        Event:   audit.EventMarketplaceInstallFailed,
//	        Outcome: audit.OutcomeFailed,
//	        Meta:    map[string]any{"error_code": audit.ErrorCode(r)},
//	    })
//	}
func ErrorCode(r core.Result) string {
	if r.OK {
		return ""
	}
	if code := r.Code(); code != "" {
		return code
	}
	if err, ok := r.Value.(error); ok {
		var e *core.Err
		if core.As(err, &e) && e.Operation != "" {
			return e.Operation
		}
	}
	return codeUnknown
}
