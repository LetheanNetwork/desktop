// SPDX-Licence-Identifier: EUPL-1.2

// redact.go — RFC.stage-f.md §2.1.1 secret-shape detector. Content-
// shape recognisers, NOT path-prefix matching (Cerberus DREAD ADD-
// HIGH-1 ruling — the path-prefix mechanism gives both false-positives
// on legitimate Meta.path values + false-negatives on tokens copied
// into free-form Meta values).
//
// Phase 1 recogniser stack (matches RFC §2.1.1 verbatim):
//
//  1. Token-prefix substring match — "LTHN-BOOT-1." / "LTHN-SESS-1." /
//     "LTHN-CHALL-1." (forward-reserve). Hit if any literal occurs
//     anywhere inside a string value.
//  2. "Bearer " substring (case-insensitive prefix) — catches
//     Authorization header content that leaked into Meta.
//  3. Base64-or-hex of length ≥ 32 — heuristic catch-all for "looks
//     like a secret". Tuned to 32B floor to avoid false-positives on
//     URL paths / short IDs.
//
// Per-key ALLOWLIST: keys named "account_id" MUST NOT trigger the
// base64/hex floor (they are deterministic SHA-256 prefixes — RFC §6.4
// will switch to HMAC-hashed values in a Phase 2 sweep; either shape
// is allowlisted today since both are produced by core.SHA256HexString
// over the supplied input).
//
// Recursive Meta walk — values can be map[string]any / []any; the
// detector walks ALL nested string content, not just top-level keys.

package audit

import (
	core "dappco.re/go"
)

// IsSecretShaped reports whether v looks like a secret per the §2.1.1
// recogniser stack. Exposed for table-tested matching from callers'
// pre-emit validation (a caller building a Meta map for an Event MAY
// pre-check a value rather than relying on the recorder's post-build
// redaction — the recorder still runs the same check at write time
// so the pre-check is purely a callsite-clarity hint).
//
// Returns false for the empty string and for values shorter than the
// 32-byte floor that don't match the explicit token-prefix or Bearer
// recognisers.
//
// Usage example:
//
//	if audit.IsSecretShaped(meta["token"].(string)) {
//	    // never persist the token bytes — store a placeholder instead
//	    meta["token"] = "[REDACTED]"
//	}
func IsSecretShaped(v string) bool {
	if v == "" {
		return false
	}
	// (1) Token-prefix recognisers — RFC §2.1.1 (1). Substring match so
	// a token copied with surrounding context still trips the detector.
	for _, prefix := range tokenPrefixes {
		if core.Contains(v, prefix) {
			return true
		}
	}
	// (2) Bearer header content — RFC §2.1.1 (2). Case-insensitive
	// prefix match. core.HasPrefix is byte-equal; lower-case the first
	// 7 bytes of v for the comparison while leaving the rest untouched
	// so we don't mutate the caller's string.
	if hasCaseInsensitivePrefix(v, "Bearer ") {
		return true
	}
	// (3) Long base64/hex floor — RFC §2.1.1 (3). Whole-string match
	// against the recogniser regex. The compile-once regex is built in
	// init() so the per-call cost is just MatchString.
	if secretFloorRegex != nil && secretFloorRegex.MatchString(v) {
		return true
	}
	return false
}

// tokenPrefixes is the closed set of LTHN-token literals the redactor
// trips on. Forward-reserve "LTHN-CHALL-1." per RFC §2.1.1 so a future
// challenge-response token shape doesn't need a redactor patch on
// landing day.
var tokenPrefixes = []string{
	"LTHN-BOOT-1.",
	"LTHN-SESS-1.",
	"LTHN-CHALL-1.",
}

// secretFloorRegex matches "looks like a secret" — base64url-ish or
// hex characters, length ≥ 32. Anchored to whole-string so a value
// like "/sales/q2-deal" (legitimate path) does NOT trip even though
// individual segments are < 32 bytes.
//
// Compiled once via init so the per-call MatchString is cheap;
// compilation failure leaves secretFloorRegex nil and IsSecretShaped
// falls through to the explicit recognisers (defence-in-depth: the
// token prefix + Bearer checks still defend the primary leakage paths
// even if the heuristic floor is unavailable).
var secretFloorRegex *core.Regexp

func init() {
	// ^[A-Za-z0-9_+/=-]{32,}$ — base64url (A-Za-z0-9_-) + base64
	// standard (+/=) + length floor. Caters to both raw base64 + hex
	// (hex is a strict subset). 32 bytes ≈ 192 bits of entropy — well
	// above the threshold a passphrase/token derivative would land at.
	r := core.Regex(`^[A-Za-z0-9_+/=\-]{32,}$`)
	if r.OK {
		if rx, ok := r.Value.(*core.Regexp); ok {
			secretFloorRegex = rx
		}
	}
}

// hasCaseInsensitivePrefix reports whether s starts with prefix
// ignoring ASCII case. Stays in-package so we don't widen the
// pkg/audit surface for a single internal consumer; core.HasPrefix is
// byte-exact + a Lower-the-whole-string approach would copy needlessly
// for long values that don't match anyway.
func hasCaseInsensitivePrefix(s, prefix string) bool {
	if len(s) < len(prefix) {
		return false
	}
	for i := 0; i < len(prefix); i++ {
		sb := s[i]
		pb := prefix[i]
		if sb >= 'A' && sb <= 'Z' {
			sb += 'a' - 'A'
		}
		if pb >= 'A' && pb <= 'Z' {
			pb += 'a' - 'A'
		}
		if sb != pb {
			return false
		}
	}
	return true
}

// redactionPlaceholder is the literal substituted for any string value
// the detector flags. Kept short + recognisable so operators reading
// raw NDJSON immediately see what happened without parsing.
const redactionPlaceholder = "[REDACTED:contains-token-shape]"

// keyAccountID is the only per-key allowlist Phase 1 honours. Values
// under this key are SHA-256 hex prefixes (and Phase 2 will be HMAC-
// hashed equivalents) — both shapes trip the long-hex floor by
// construction even though they are not secrets.
const keyAccountID = "account_id"

// redactMeta walks v recursively and returns (cleaned, dirty) where
// cleaned is a copy with every secret-shaped string value replaced by
// the placeholder, and dirty reports whether ANY substitution
// happened. Caller uses dirty to decide whether to emit the §2.1
// audit.record.redacted debug-echo via core.Print (RFC §6.1 — never
// re-enter the typed bus).
//
// Allowlist semantics: when descending into a map[string]any the
// caller passes the key it is recursing through (or "" at the top
// level); a recursion into the "account_id" key skips the long-base64
// floor for the direct child string value but STILL applies the
// explicit token-prefix + Bearer recognisers (a token literally pasted
// into Meta["account_id"] is still a leak).
//
// Returns the original map reference unchanged when nothing was
// redacted — saves an alloc on the common "no secrets" path.
//
// Usage example:
//
//	cleaned, dirty := redactMeta(ev.Meta)
//	if dirty {
//	    core.Print(core.Stderr(), "event=audit.record.redacted ts=%d\n",
//	        core.Now().UTC().Unix())
//	}
//	ev.Meta = cleaned
func redactMeta(m map[string]any) (map[string]any, bool) {
	if m == nil {
		return nil, false
	}
	dirty := false
	out := m
	first := true
	for k, v := range m {
		cleaned, sub := redactValue(k, v)
		if sub {
			if first {
				// Copy-on-write — only allocate the cleaned map when at
				// least one substitution actually happens. The first
				// dirty key triggers the copy; subsequent dirty keys
				// land in the already-copied map.
				out = make(map[string]any, len(m))
				for k2, v2 := range m {
					out[k2] = v2
				}
				first = false
			}
			out[k] = cleaned
			dirty = true
		} else if !first {
			// We've already started a copy; keep cleaned (which may be
			// a recursively-cleaned nested value even if no top-level
			// substitution happened for THIS key).
			out[k] = cleaned
		}
	}
	return out, dirty
}

// redactValue recursively cleans v under the given parent key. Returns
// (cleaned, substituted). When v is a string, applies the recogniser
// stack honouring the per-key allowlist; when v is a map/slice,
// recurses; otherwise returns v unchanged.
func redactValue(parentKey string, v any) (any, bool) {
	switch tv := v.(type) {
	case string:
		if parentKey == keyAccountID {
			// Skip the long-base64 floor for account_id values (RFC
			// §2.1.1 per-key allowlist). Still apply explicit token-
			// prefix + Bearer recognisers — a literal token under the
			// account_id key is still a leak we must catch.
			for _, prefix := range tokenPrefixes {
				if core.Contains(tv, prefix) {
					return redactionPlaceholder, true
				}
			}
			if hasCaseInsensitivePrefix(tv, "Bearer ") {
				return redactionPlaceholder, true
			}
			return tv, false
		}
		if IsSecretShaped(tv) {
			return redactionPlaceholder, true
		}
		return tv, false
	case map[string]any:
		cleaned, dirty := redactMeta(tv)
		if dirty {
			return cleaned, true
		}
		return tv, false
	case []any:
		copied := false
		out := tv
		for i, elem := range tv {
			cleaned, sub := redactValue(parentKey, elem)
			if sub {
				if !copied {
					out = make([]any, len(tv))
					copy(out, tv)
					copied = true
				}
				out[i] = cleaned
			}
		}
		return out, copied
	default:
		return v, false
	}
}
