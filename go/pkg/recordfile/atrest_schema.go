// SPDX-Licence-Identifier: EUPL-1.2

// atrest_schema.go — per-surface header-field whitelist + bucketing
// helpers for the at-rest substrate (§2.4 of
// RFC.stage-e-encrypt-at-rest v2).
//
// The substrate enforces "encrypt-everything-except-listed-keys" by
// asking the consumer for a HeaderSchema that names exactly the
// header keys it wants present and the validator+bucketer for each
// one. The encode pipeline:
//
//  1. Consumer's HeaderSchema.HeaderFor(rec) returns map[string]any
//     of candidate header values.
//  2. Substrate intersects against the schema's AllowedKeys + invokes
//     each validator. Any key not whitelisted, or any value failing
//     its validator, returns typed
//     "recordfile.atrest.schema.bucketed_field_violation".
//  3. Substrate adds the structural fields (schema, id, version,
//     account.*, surface, body.checksum, header.mac, created_at,
//     updated_at) — consumers MUST NOT supply these.
//
// Bucketing helpers (MonthBucket, LogSizeBucket, HashTag) are exported
// so per-writer schema definitions reuse the canonical implementations
// and so SchemaViolation tests can pin the byte shape.
//
// Usage example (sales/deals consumer):
//
//	schema := recordfile.HeaderSchema[DealRecord]{
//	    Surface: recordfile.SurfaceSalesDeals,
//	    AllowedKeys: map[string]recordfile.FieldValidator{
//	        "stage":         recordfile.ValidateEnum(recordfile.SalesDealsStages),
//	        "close.target":  recordfile.ValidateMonthBucket,
//	    },
//	    HeaderFor: func(r DealRecord) map[string]any {
//	        return map[string]any{
//	            "stage":        string(r.Stage),
//	            "close.target": recordfile.MonthBucket(r.CloseTarget),
//	        }
//	    },
//	}

package recordfile

import (
	core "dappco.re/go"
)

// Surface is the canonical surface enum carried in the at-rest header.
// Each value MUST match the path-segment in ~/Lethean/<surface>/ so
// header-field MAC + body cross-checks remain grep-stable.
type Surface string

// Canonical surface values per RFC §2.4 per-field MUST table.
//
//	hdr := map[string]any{"surface": string(recordfile.SurfaceSalesDeals)}
const (
	SurfaceSalesDeals          Surface = "sales.deals"
	SurfaceIncidents           Surface = "incidents"
	SurfaceRunbooks            Surface = "runbooks"
	SurfaceMarketingCampaigns  Surface = "marketing.campaigns"
	SurfaceMarketingAudience   Surface = "marketing.audience"
	SurfaceMarketingSocial     Surface = "marketing.social"
	SurfaceMarketingContent    Surface = "marketing.content"
)

// SchemaVersion is the canonical version string carried in every
// at-rest header. Decode rejects with
// "recordfile.atrest.schema_unsupported" on mismatch and with
// "recordfile.atrest.schema_missing" when absent (Cerberus lean-7
// amendment, RFC §2.7).
const SchemaVersion = "lthn.atrest.v1"

// MagicLTHN is the workspace-wide Trix magic-number per RFC §2.3
// (Q1 CONFIRM). Encode + Decode pin this value so the on-disk
// format-fingerprint stays uniform across surfaces.
const MagicLTHN = "LTHN"

// MaxPayloadBytes caps the encrypted payload at 1 MiB (RFC §2.3 +
// §1.5 T5 defence). Enforced at BOTH encode and decode so a
// deliberately-malformed file cannot DOS the read path via allocation
// pressure.
const MaxPayloadBytes = 1 * 1024 * 1024

// FieldValidator validates a single header-value at encode time.
// Returns nil when v conforms; returns a *core.Err with Code
// "recordfile.atrest.schema.bucketed_field_violation" otherwise.
//
//	v := recordfile.ValidateEnum(recordfile.SalesDealsStages)("propose")
type FieldValidator func(v any) error

// HeaderSchema couples a surface enum, the per-surface allowed-key
// map, and a function that projects T into the candidate header.
//
// Substrate-level invariants (RFC §5.2) — fingerprint, MAC, body
// checksum, schema, id, version, account.*, created_at, updated_at,
// surface — are written by the substrate; the schema's HeaderFor MUST
// NOT populate them or the substrate returns typed
// "recordfile.atrest.schema.reserved_key" (encoded in AllowedKeys
// enforcement below — reserved keys are simply not allowed in the
// HeaderFor return map).
type HeaderSchema[T any] struct {
	// Surface pins the on-disk surface label written into the header.
	Surface Surface

	// AllowedKeys names every searchable / list-view header key this
	// surface is willing to expose plus the validator that gates
	// each one. Free-form keys outside this map are rejected; a
	// missing key in HeaderFor's output is allowed (optional field).
	AllowedKeys map[string]FieldValidator

	// HeaderFor projects rec into a map of candidate header values
	// keyed by the same names AllowedKeys uses. The substrate adds
	// reserved structural keys on top.
	HeaderFor func(rec T) map[string]any

	// IDFor returns the record id used as the file basename
	// (<id>.lthn) AND embedded into the header `id` field.
	IDFor func(rec T) string

	// VersionFor returns the monotonic record version embedded in the
	// header `version` field (used for the AtomicWriteWithVersion
	// IfVersion gate when the substrate writes the file).
	VersionFor func(rec T) int64
}

// reservedHeaderKeys names every header key written by the substrate
// itself. HeaderFor MUST NOT include any of these; the substrate
// returns "recordfile.atrest.schema.reserved_key" at encode time when
// it does.
//
// Kept package-private — callers don't need to know the exact list,
// only that the substrate owns these fields.
var reservedHeaderKeys = map[string]struct{}{
	"schema":                    {},
	"id":                        {},
	"version":                   {},
	"account":                   {},
	"surface":                   {},
	"body.checksum":             {},
	"header.mac":                {},
	"created_at":                {},
	"updated_at":                {},
}

// validateConsumerHeader runs schema.AllowedKeys + reserved-keys
// gates against a candidate header. Reserved-key collisions or
// disallowed keys return typed errors before any encrypt work.
func validateConsumerHeader(allowed map[string]FieldValidator, cand map[string]any) error {
	for k, v := range cand {
		if _, isReserved := reservedHeaderKeys[k]; isReserved {
			return core.NewCode("recordfile.atrest.schema.reserved_key",
				core.Sprintf("header key %q is reserved by the at-rest substrate; remove from HeaderFor", k))
		}
		validator, ok := allowed[k]
		if !ok {
			return core.NewCode("recordfile.atrest.schema.bucketed_field_violation",
				core.Sprintf("header key %q is not in this surface's AllowedKeys whitelist", k))
		}
		if validator != nil {
			if err := validator(v); err != nil {
				return err
			}
		}
	}
	return nil
}

// --- Canonical enums per RFC §2.4 ---

// SalesDealsStages is the closed enum for sales/deals.stage per RFC
// §2.4. Free-form stage values are rejected at encode.
var SalesDealsStages = []string{
	"qual", "engage", "propose", "negotiate",
	"closed_won", "closed_lost", "on_hold",
}

// IncidentsStatuses is the closed enum for incidents.status per RFC
// §2.4. (severity stays free-form per the table CONFIRM row.)
var IncidentsStatuses = []string{"open", "closed"}

// MarketingAudienceSizeBuckets is the closed log-bucket enum for
// marketing/audience.size per RFC §2.4. Raw integer audience sizes
// MUST be projected through LogSizeBucket before reaching the header.
var MarketingAudienceSizeBuckets = []string{
	"<1k", "1-10k", "10-100k", "100k+",
}

// --- Validators ---

// ValidateEnum returns a FieldValidator that accepts only strings
// drawn from members. Anything else returns
// "recordfile.atrest.schema.bucketed_field_violation".
//
//	v := recordfile.ValidateEnum([]string{"open","closed"})("triaged")
//	// → bucketed_field_violation
func ValidateEnum(members []string) FieldValidator {
	allowed := make(map[string]struct{}, len(members))
	for _, m := range members {
		allowed[m] = struct{}{}
	}
	return func(v any) error {
		s, ok := v.(string)
		if !ok {
			return core.NewCode("recordfile.atrest.schema.bucketed_field_violation",
				core.Sprintf("enum field requires string, got %T", v))
		}
		if _, ok := allowed[s]; !ok {
			return core.NewCode("recordfile.atrest.schema.bucketed_field_violation",
				core.Sprintf("value %q is not in the enum allowlist", s))
		}
		return nil
	}
}

// ValidateMonthBucket accepts only strings of the form YYYY-MM.
// Day-granularity dates leak too much when joined with surface metadata
// (RFC §1.1) so the header carries the month bucket only; the exact
// date lives in the encrypted body.
//
//	err := recordfile.ValidateMonthBucket("2026-05")        // nil
//	err = recordfile.ValidateMonthBucket("2026-05-17")      // bucketed_field_violation
func ValidateMonthBucket(v any) error {
	s, ok := v.(string)
	if !ok {
		return core.NewCode("recordfile.atrest.schema.bucketed_field_violation",
			core.Sprintf("month bucket requires string, got %T", v))
	}
	if !isYYYYMM(s) {
		return core.NewCode("recordfile.atrest.schema.bucketed_field_violation",
			core.Sprintf("value %q is not a YYYY-MM month bucket", s))
	}
	return nil
}

// ValidateHashTagList accepts only []any (or []string) where every
// element is a 16-hex-char string — the SHA-256 hash-tag truncation
// produced by HashTag. Plaintext tags MUST be hashed before reaching
// the header so List operations compare hashes without learning
// vocabulary.
//
//	hashes := []any{recordfile.HashTag("ops")}
//	err := recordfile.ValidateHashTagList(hashes)  // nil
func ValidateHashTagList(v any) error {
	xs, ok := v.([]any)
	if !ok {
		return core.NewCode("recordfile.atrest.schema.bucketed_field_violation",
			core.Sprintf("hash-tag list requires []any, got %T", v))
	}
	for i, x := range xs {
		s, ok := x.(string)
		if !ok {
			return core.NewCode("recordfile.atrest.schema.bucketed_field_violation",
				core.Sprintf("hash-tag list element %d requires string, got %T", i, x))
		}
		if len(s) != 16 || !isLowerHex(s) {
			return core.NewCode("recordfile.atrest.schema.bucketed_field_violation",
				core.Sprintf("hash-tag list element %d %q is not a 16-hex-char SHA-256 truncation", i, s))
		}
	}
	return nil
}

// ValidateString accepts any string. Used for whitelisted free-form
// header keys (e.g. incidents.severity, marketing.campaigns.platform
// in the RFC §2.4 CONFIRM rows).
//
//	err := recordfile.ValidateString("p2")  // nil
func ValidateString(v any) error {
	if _, ok := v.(string); !ok {
		return core.NewCode("recordfile.atrest.schema.bucketed_field_violation",
			core.Sprintf("string field required, got %T", v))
	}
	return nil
}

// --- Bucketing helpers ---

// MonthBucket truncates a Unix-seconds timestamp to YYYY-MM (UTC).
//
//	bucket := recordfile.MonthBucket(1747440000)  // → "2026-05"
func MonthBucket(unixSeconds int64) string {
	t := core.UnixTime(unixSeconds).UTC()
	return core.Sprintf("%04d-%02d", t.Year(), int(t.Month()))
}

// LogSizeBucket maps an audience-size integer onto the canonical
// log-bucket enum per RFC §2.4.
//
//	b := recordfile.LogSizeBucket(2500)  // → "1-10k"
func LogSizeBucket(n int64) string {
	switch {
	case n < 1_000:
		return "<1k"
	case n < 10_000:
		return "1-10k"
	case n < 100_000:
		return "10-100k"
	default:
		return "100k+"
	}
}

// HashTag returns the first 16 lowercase hex chars of SHA-256(tag).
// Used to project free-form runbook tags into header hash-tags so
// List operations compare hashes without revealing the vocabulary
// (RFC §2.4).
//
//	h := recordfile.HashTag("ops")  // → "2c1743a391305..."[:16]
func HashTag(tag string) string {
	return core.SHA256HexString(tag)[:16]
}

// --- Internal predicates ---

// isYYYYMM reports whether s matches strict YYYY-MM format.
// Implementation avoids importing strings/regexp; the substrate's
// AX-6 banned-import discipline rules out time.Parse-style parsing
// for a one-shot syntactic check.
func isYYYYMM(s string) bool {
	if len(s) != 7 {
		return false
	}
	if s[4] != '-' {
		return false
	}
	for i := 0; i < 4; i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	if s[5] < '0' || s[5] > '1' {
		return false
	}
	if s[6] < '0' || s[6] > '9' {
		return false
	}
	month := (int(s[5]-'0') * 10) + int(s[6]-'0')
	return month >= 1 && month <= 12
}

// isLowerHex reports whether s consists only of [0-9a-f] characters.
func isLowerHex(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}
