// SPDX-Licence-Identifier: EUPL-1.2

// atrest_schema_test.go — per-field MUST table from RFC §2.4 exercised
// against the validators + bucketing helpers. The grep anchor named
// in the RFC is TestAtRestSchema_HeaderFieldEnums_Bad — that test
// covers every bucketed/enum field row in the table.

package recordfile_test

import (
	"testing"

	"dappco.re/lthn/desktop/pkg/recordfile"
)

// --- Bucketing helpers ----------------------------------------------

// TestAtRestSchema_MonthBucketComputation_Good — date → YYYY-MM
// truncation. Pinned against a known Unix timestamp.
func TestAtRestSchema_MonthBucketComputation_Good(t *testing.T) {
	// 2026-05-17 00:00:00 UTC = 1779494400
	got := recordfile.MonthBucket(1779494400)
	if got != "2026-05" {
		t.Fatalf("MonthBucket mismatch: got %q want %q", got, "2026-05")
	}
	// Jan 1 2026 00:00 UTC = 1767225600
	got = recordfile.MonthBucket(1767225600)
	if got != "2026-01" {
		t.Fatalf("MonthBucket jan mismatch: got %q want %q", got, "2026-01")
	}
}

// TestAtRestSchema_LogBucketComputation_Good — audience size → bucket
// enum.
func TestAtRestSchema_LogBucketComputation_Good(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{0, "<1k"},
		{999, "<1k"},
		{1000, "1-10k"},
		{9999, "1-10k"},
		{10000, "10-100k"},
		{99999, "10-100k"},
		{100000, "100k+"},
		{5_000_000, "100k+"},
	}
	for _, tc := range cases {
		got := recordfile.LogSizeBucket(tc.in)
		if got != tc.want {
			t.Fatalf("LogSizeBucket(%d): got %q want %q", tc.in, got, tc.want)
		}
	}
}

// TestAtRestSchema_HashTagComputation_Good — runbook hash-tag matches
// sha256(tag)[:16] (first 16 hex chars of the SHA-256 hex digest).
func TestAtRestSchema_HashTagComputation_Good(t *testing.T) {
	got := recordfile.HashTag("ops")
	if len(got) != 16 {
		t.Fatalf("HashTag length: got %d want 16", len(got))
	}
	for i := 0; i < len(got); i++ {
		c := got[i]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Fatalf("HashTag non-hex char at %d: %q", i, got)
		}
	}
	// Stability — same input always same output.
	if got != recordfile.HashTag("ops") {
		t.Fatalf("HashTag not stable")
	}
	// Distinct input → distinct hash.
	if recordfile.HashTag("ops") == recordfile.HashTag("dev") {
		t.Fatalf("HashTag collisions between ops and dev — implementation broken")
	}
}

// --- ValidateEnum ----------------------------------------------------

// TestAtRestSchema_ValidateEnum_Good — accept members; reject non-members.
func TestAtRestSchema_ValidateEnum_Good(t *testing.T) {
	v := recordfile.ValidateEnum(recordfile.SalesDealsStages)
	for _, ok := range recordfile.SalesDealsStages {
		if err := v(ok); err != nil {
			t.Fatalf("expected %q to pass, got %v", ok, err)
		}
	}
}

// TestAtRestSchema_ValidateEnum_Bad — reject free-form strings + wrong types.
func TestAtRestSchema_ValidateEnum_Bad(t *testing.T) {
	v := recordfile.ValidateEnum(recordfile.SalesDealsStages)
	bad := []any{"in-progress", "QUAL", "", 42, true, nil}
	for _, b := range bad {
		err := v(b)
		if err == nil {
			t.Fatalf("expected reject for %v, got nil", b)
		}
		// All paths return bucketed_field_violation per RFC §2.4 done-criterion.
		if !errCodeIs(err, "recordfile.atrest.schema.bucketed_field_violation") {
			t.Fatalf("expected bucketed_field_violation for %v, got %v", b, err)
		}
	}
}

// --- ValidateMonthBucket --------------------------------------------

// TestAtRestSchema_ValidateMonthBucket_Good — YYYY-MM passes.
func TestAtRestSchema_ValidateMonthBucket_Good(t *testing.T) {
	ok := []string{"2026-01", "2026-12", "1999-06", "2030-02"}
	for _, s := range ok {
		if err := recordfile.ValidateMonthBucket(s); err != nil {
			t.Fatalf("expected %q to pass, got %v", s, err)
		}
	}
}

// TestAtRestSchema_ValidateMonthBucket_Bad — day-granularity rejected,
// malformed rejected.
func TestAtRestSchema_ValidateMonthBucket_Bad(t *testing.T) {
	bad := []any{"2026-05-17", "26-05", "2026-13", "2026-00", "abcd-ef", "2026/05", 1779494400, nil}
	for _, b := range bad {
		err := recordfile.ValidateMonthBucket(b)
		if err == nil {
			t.Fatalf("expected reject for %v, got nil", b)
		}
		if !errCodeIs(err, "recordfile.atrest.schema.bucketed_field_violation") {
			t.Fatalf("expected bucketed_field_violation for %v, got %v", b, err)
		}
	}
}

// --- ValidateHashTagList --------------------------------------------

// TestAtRestSchema_ValidateHashTagList_Good — list of 16-hex-char strings passes.
func TestAtRestSchema_ValidateHashTagList_Good(t *testing.T) {
	hashes := []any{recordfile.HashTag("ops"), recordfile.HashTag("dev")}
	if err := recordfile.ValidateHashTagList(hashes); err != nil {
		t.Fatalf("expected pass, got %v", err)
	}
}

// TestAtRestSchema_ValidateHashTagList_Bad — plaintext / wrong length / wrong type rejected.
func TestAtRestSchema_ValidateHashTagList_Bad(t *testing.T) {
	bad := []any{
		[]any{"plaintext-tag"},                // wrong length
		[]any{"AABBCCDDEEFF0011"},             // upper hex
		[]any{"deadbeef"},                     // short
		[]any{42},                             // wrong type
		"not-a-list",                          // wrong outer type
	}
	for _, b := range bad {
		err := recordfile.ValidateHashTagList(b)
		if err == nil {
			t.Fatalf("expected reject for %v, got nil", b)
		}
		if !errCodeIs(err, "recordfile.atrest.schema.bucketed_field_violation") {
			t.Fatalf("expected bucketed_field_violation for %v, got %v", b, err)
		}
	}
}

// --- Composite per-surface MUST table (RFC §2.4 grep anchor) -------

// TestAtRestSchema_HeaderFieldEnums_Bad — the load-bearing grep anchor
// from RFC §2.4 "Done-criterion (load-bearing)". For every bucketed
// row in the table, supplying a free-form value MUST trigger the
// typed bucketed_field_violation reject.
func TestAtRestSchema_HeaderFieldEnums_Bad(t *testing.T) {
	type row struct {
		name      string
		validator recordfile.FieldValidator
		freeform  any // a value that MUST be rejected
	}
	rows := []row{
		{
			name:      "sales/deals.stage (enum)",
			validator: recordfile.ValidateEnum(recordfile.SalesDealsStages),
			freeform:  "in-discussion",
		},
		{
			name:      "sales/deals.close.target (month bucket)",
			validator: recordfile.ValidateMonthBucket,
			freeform:  "2026-05-17",
		},
		{
			name:      "incidents.status (enum)",
			validator: recordfile.ValidateEnum(recordfile.IncidentsStatuses),
			freeform:  "RCA pending — customer X",
		},
		{
			name:      "runbooks.tags (hash-tag list)",
			validator: recordfile.ValidateHashTagList,
			freeform:  []any{"plaintext-tag-leaking-vocab"},
		},
		{
			name:      "marketing/campaigns.scheduled.at (month bucket)",
			validator: recordfile.ValidateMonthBucket,
			freeform:  "2026-05-17",
		},
		{
			name:      "marketing/audience.size (log bucket)",
			validator: recordfile.ValidateEnum(recordfile.MarketingAudienceSizeBuckets),
			freeform:  "12345", // raw int as string — must be a bucket label
		},
		{
			name:      "marketing/content.due.at (month bucket)",
			validator: recordfile.ValidateMonthBucket,
			freeform:  "2026-05-17",
		},
	}
	for _, tc := range rows {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.validator(tc.freeform)
			if err == nil {
				t.Fatalf("expected free-form reject, got nil")
			}
			if !errCodeIs(err, "recordfile.atrest.schema.bucketed_field_violation") {
				t.Fatalf("expected bucketed_field_violation, got %v", err)
			}
		})
	}
}

// errCodeIs returns true when err's *core.Err carries the given Code.
// Local helper to keep tests free of core.As ceremony.
func errCodeIs(err error, code string) bool {
	type coder interface{ Error() string }
	_ = coder(err)
	// Use core.ErrorCode via the well-known *core.Err shape.
	type withCode struct {
		Operation string
		Message   string
		Cause     error
		Code      string
	}
	// Avoid importing core just for ErrorCode by string-matching the
	// error formatting — *core.Err.Error() includes "[code]" trailers.
	// More robust: assert through core.ErrorCode at the call sites;
	// done explicitly via a tiny indirection here.
	return errStringContains(err.Error(), "["+code+"]") ||
		errStringContains(err.Error(), code)
}

func errStringContains(s, sub string) bool {
	if len(sub) == 0 || len(s) < len(sub) {
		return len(sub) == 0
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
