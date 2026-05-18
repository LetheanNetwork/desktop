// SPDX-Licence-Identifier: EUPL-1.2

// errorcode.go — bounded-keyspace audit error_code + error_scope
// substrate per plans/code/lthn/desktop/audit/RFC.error-code-cascade.md
// §1 (CLEAN-FOR-IMPL, v2 — Cerberus #62 DREAD folded; Mantis #1720
// dual-key shape). The cluster-wide promise: the result of ErrorCode
// and ErrorScope are the ONLY values that may be assigned to audit
// Event Meta["error_code"] / Meta["error_scope"]. Type-system
// enforcement of the bounded-keyspace contract — not docstring
// discipline.
//
// Canonical codespace registry — the closed set of canonical Lethean
// codespace strings (r.Code()) that the substrate may surface in
// Meta["error_code"]. Adding a new codespace is a reserved-schema
// change: register here in the same commit that introduces the
// emitter, so the forensic walker, chip-filter UI, and log scrapers
// learn the new code with the schema bump and the bounded-vocabulary
// promise stays type-anchored rather than docstring-anchored.
//
// Per-package canonical codespace prefixes (Mantis #1720 v2 registry):
//
//   - "ai.*"            — AI provider router / model invocation
//                          (e.g. "ai.openai.provider", "ai.vllm.timeout")
//   - "marketplace.*"   — pkg/marketplace install / launch / fetch
//                          (e.g. "marketplace.install", "marketplace.fetch_manifest")
//   - "sandbox.*"       — pkg/sandbox spawn / kill / long-run lifecycle
//                          (e.g. "sandbox.spawn", "sandbox.long.run")
//   - "process.*"       — pkg/process run / start / kill
//                          (e.g. "process.run", "process.start")
//   - "downloader.*"    — pkg/downloader fetch / verify / resolve
//                          (e.g. "downloader.fetch", "downloader.digest_mismatch")
//   - "gateway.*"       — pkg/gateway dispatch
//                          (e.g. "gateway.dispatch")
//   - "runner.*"        — pkg/runner inference orchestration
//                          (e.g. "runner.generate", "runner.chat")
//   - "image.*"         — pkg/imagetrust typed-enum specialisation
//                          (e.g. "image_empty", "image_imds" — non-dotted,
//                          legacy shape preserved per RFC §4.2)
//   - "unknown_error"   — substrate fallback (FOLD-1 canon, no prefix)
//
// New top-level prefix (e.g. "queue.*", "sessions.*") requires:
//   1. Registration in this comment block.
//   2. Mantis ticket recording the schema bump.
//   3. Mirror update in frontend/src/lit/obs/audit-constants.ts so the
//      chip-filter UI surfaces the new keyspace.
//
// Two-key shape (Mantis #1720 ADD-1, RFC §7 Q-4 v2):
//
//   - ErrorCode returns the canonical Lethean codespace (e.g.
//     "ai.openai.provider") — "WHY the failure occurred" — sourced from
//     r.Code() or *core.Err.Operation or "unknown_error" fallback.
//   - ErrorScope returns the operation scope (*core.Err.Operation only)
//     — "WHAT failed at the package / operation boundary" (e.g.
//     "marketplace.install"). Returns "" when no scope is available.
//
// The walker disambiguates by reading BOTH keys: error_code answers
// the categorical question, error_scope answers the boundary question.
// v1 flattened these into a single Meta["error_code"] which forced the
// auditor to guess whether a string was canonical-code or scope-name.
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

// ErrorScope derives the operation-scope literal from a failed
// core.Result — the "WHAT failed at the package / operation boundary"
// half of the Mantis #1720 v2 dual-key shape. ErrorScope returns
// (*core.Err).Operation when the underlying error wraps a *core.Err,
// or "" otherwise. The function NEVER returns r.Code() — that branch
// is owned by ErrorCode and emitting the codespace as a scope would
// conflate the two semantic axes the dual-key shape exists to keep
// apart.
//
// Resolution order (RFC §7 Q-4 v2 / Mantis #1720):
//
//  1. r.OK == true → "" (no error, no scope).
//  2. (*core.Err).Operation if r.Value wraps a *core.Err and
//     Operation is non-empty (e.g. "marketplace.install").
//  3. "" — no operation scope available (bare error or non-*core.Err
//     value).
//
// Co-emit both keys per the canonical dual-key shape:
//
//	_ = audit.Default().Record(audit.Event{
//	    Event:   audit.EventMarketplaceInstallFailed,
//	    Outcome: audit.OutcomeFailed,
//	    Meta: map[string]any{
//	        "error_code":  audit.ErrorCode(r),
//	        "error_scope": audit.ErrorScope(r),
//	    },
//	})
//
// The forensic walker reads BOTH keys: error_code answers "WHY"
// (canonical code), error_scope answers "WHAT" (operation boundary).
// Either may be "" — error_code is "" only on OK Results, error_scope
// is "" whenever no *core.Err is wrapped (e.g. bare errors.New value).
//
// The same prose-leak protection that ErrorCode pins applies here:
// only the Operation literal is surfaced — never the *core.Err.Message
// field, never any underlying cause prose. The bounded-keyspace
// promise is enforced by the resolution order, not by docstring.
func ErrorScope(r core.Result) string {
	if r.OK {
		return ""
	}
	if err, ok := r.Value.(error); ok {
		var e *core.Err
		if core.As(err, &e) && e.Operation != "" {
			return e.Operation
		}
	}
	return ""
}
