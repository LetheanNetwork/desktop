// SPDX-Licence-Identifier: EUPL-1.2

package audit

import (
	"testing"

	core "dappco.re/go"
)

// inMemoryRecorder is a Recorder fixture for tests. Stores every
// Record call's Event in a slice so test cases can assert on counts +
// field shapes without touching disk.
type inMemoryRecorder struct {
	mu     core.Mutex
	events []Event
}

func (r *inMemoryRecorder) Record(ev Event) core.Result {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, ev)
	return core.Ok(nil)
}

func (r *inMemoryRecorder) snapshot() []Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Event, len(r.events))
	copy(out, r.events)
	return out
}

// --- IsSecretShaped --------------------------------------------------

func TestRedact_IsSecretShaped_Good_BootstrapToken(t *testing.T) {
	if !IsSecretShaped("LTHN-BOOT-1.test.fixture.canary.notarealtoken") {
		t.Fatal("LTHN-BOOT-1. prefix should trip the recogniser")
	}
}

func TestRedact_IsSecretShaped_Good_SessionToken(t *testing.T) {
	if !IsSecretShaped("LTHN-SESS-1.test.fixture.canary") {
		t.Fatal("LTHN-SESS-1. prefix should trip the recogniser")
	}
}

func TestRedact_IsSecretShaped_Good_ChallengeToken(t *testing.T) {
	if !IsSecretShaped("LTHN-CHALL-1.test.fixture") {
		t.Fatal("LTHN-CHALL-1. prefix should trip the forward-reserve recogniser")
	}
}

func TestRedact_IsSecretShaped_Good_BearerHeader(t *testing.T) {
	if !IsSecretShaped("Bearer test-token-not-a-real-credential") {
		t.Fatal("Bearer-prefix should trip the recogniser")
	}
	// Case-insensitive prefix per spec.
	if !IsSecretShaped("bearer test-token-not-a-real-credential") {
		t.Fatal("bearer-prefix lower-cased should trip the recogniser")
	}
	if !IsSecretShaped("BEARER test-token-not-a-real-credential") {
		t.Fatal("BEARER-prefix upper-cased should trip the recogniser")
	}
}

func TestRedact_IsSecretShaped_Good_LongBase64(t *testing.T) {
	// 40-char base64-shape — well above the 32-byte floor.
	if !IsSecretShaped("dGVzdC1jYW5hcnktbm90LWEtcmVhbC1zZWNyZXQ=") {
		t.Fatal("40-char base64 should trip the heuristic floor")
	}
}

func TestRedact_IsSecretShaped_Good_LongHex(t *testing.T) {
	// 64-char hex (SHA-256-ish shape) — well above the floor.
	if !IsSecretShaped("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef") {
		t.Fatal("64-char hex should trip the heuristic floor")
	}
}

func TestRedact_IsSecretShaped_Bad_EmptyString(t *testing.T) {
	if IsSecretShaped("") {
		t.Fatal("empty string should never trip the recogniser")
	}
}

func TestRedact_IsSecretShaped_Bad_LegitimatePath(t *testing.T) {
	// Slash + dash characters keep it OUT of the base64 alphabet at
	// short length, AND short length keeps it below the floor.
	if IsSecretShaped("/sales/q2-deal") {
		t.Fatal("legitimate Meta.path value tripped the floor — false-positive")
	}
}

func TestRedact_IsSecretShaped_Bad_ShortIdentifier(t *testing.T) {
	// 16-char hex (account_id shape) is BELOW the 32B floor — wouldn't
	// trip even if it weren't allowlisted. Sanity-check the floor.
	if IsSecretShaped("abc123def4567890") {
		t.Fatal("16-char hex tripped the floor — floor mis-tuned")
	}
}

func TestRedact_IsSecretShaped_Ugly_ShortBearerNotMatched(t *testing.T) {
	// The recogniser is prefix-anchored; "value-bearer-suffix" should
	// NOT trip on a substring "bearer" in the middle.
	if IsSecretShaped("value-bearer-suffix") {
		t.Fatal("middle-of-string 'bearer' tripped — recogniser should be prefix-anchored")
	}
}

// --- redactMeta ------------------------------------------------------

func TestRedact_RedactMeta_Good_NoSecrets(t *testing.T) {
	in := map[string]any{
		"account_id": "abc123def4567890",
		"path":       "/sales/q2-deal",
		"count":      int64(42),
	}
	out, dirty := redactMeta(in)
	if dirty {
		t.Fatal("dirty flag set for clean Meta — false-positive")
	}
	// No copy when nothing was redacted — pointer identity preserved.
	if &out == nil {
		t.Fatal("redactMeta returned nil for non-nil input")
	}
}

func TestRedact_RedactMeta_Ugly_TokenInTopLevel(t *testing.T) {
	in := map[string]any{
		"token":      "LTHN-BOOT-1.test.fixture",
		"account_id": "abc123def4567890",
	}
	out, dirty := redactMeta(in)
	if !dirty {
		t.Fatal("dirty flag NOT set when top-level token leaked")
	}
	if out["token"] != redactionPlaceholder {
		t.Fatalf("token value not redacted; got %q want %q",
			out["token"], redactionPlaceholder)
	}
	// account_id should pass through unchanged (not a token shape).
	if out["account_id"] != "abc123def4567890" {
		t.Fatalf("account_id mutated; got %q", out["account_id"])
	}
	// Copy-on-write — the original input map MUST NOT have been
	// mutated.
	if in["token"] != "LTHN-BOOT-1.test.fixture" {
		t.Fatal("input map mutated — redactMeta must copy-on-write")
	}
}

func TestRedact_RedactMeta_Ugly_NestedTokenInMap(t *testing.T) {
	in := map[string]any{
		"context": map[string]any{
			"deep": "LTHN-SESS-1.nested.canary",
		},
	}
	out, dirty := redactMeta(in)
	if !dirty {
		t.Fatal("dirty flag NOT set when nested token leaked")
	}
	nested, ok := out["context"].(map[string]any)
	if !ok {
		t.Fatal("nested map was lost during redaction")
	}
	if nested["deep"] != redactionPlaceholder {
		t.Fatalf("nested token not redacted; got %q", nested["deep"])
	}
}

func TestRedact_RedactMeta_Ugly_NestedTokenInSlice(t *testing.T) {
	in := map[string]any{
		"recent_tokens": []any{
			"X-Forwarded-For: 10.0.0.1",
			"LTHN-BOOT-1.slice.canary.fixture",
		},
	}
	out, dirty := redactMeta(in)
	if !dirty {
		t.Fatal("dirty flag NOT set when slice-element leaked")
	}
	slice, ok := out["recent_tokens"].([]any)
	if !ok {
		t.Fatal("slice was lost during redaction")
	}
	if slice[0] != "X-Forwarded-For: 10.0.0.1" {
		t.Fatal("clean slice element was mutated")
	}
	if slice[1] != redactionPlaceholder {
		t.Fatalf("token slice element was not redacted; got %q", slice[1])
	}
}

func TestRedact_RedactMeta_Bad_AccountIDAllowlist(t *testing.T) {
	// account_id shape values are hex-like but LIVE under the allowlist
	// — must not trip even when they exceed the floor (Phase 2 HMAC
	// will produce 32-hex-char values that DO exceed the floor).
	in := map[string]any{
		"account_id": "0123456789abcdef0123456789abcdef",
	}
	out, dirty := redactMeta(in)
	if dirty {
		t.Fatal("account_id-keyed long-hex tripped the floor — allowlist not honoured")
	}
	if out["account_id"] != "0123456789abcdef0123456789abcdef" {
		t.Fatal("account_id value mutated")
	}
}

func TestRedact_RedactMeta_Ugly_TokenUnderAccountIDKey(t *testing.T) {
	// Even though account_id is allowlisted from the FLOOR, a literal
	// token under that key MUST still be caught by the explicit
	// recogniser (defence in depth — caller bug pasting the wrong
	// value).
	in := map[string]any{
		"account_id": "LTHN-BOOT-1.test.canary",
	}
	out, dirty := redactMeta(in)
	if !dirty {
		t.Fatal("token under account_id key was NOT redacted — allowlist over-applied")
	}
	if out["account_id"] != redactionPlaceholder {
		t.Fatalf("token under account_id key not replaced; got %q", out["account_id"])
	}
}

// --- fileSink.Record -------------------------------------------------

func newTestSink(t *testing.T) *fileSink {
	t.Helper()
	dir := t.TempDir()
	return &fileSink{root: dir}
}

func TestRecorder_Record_Good_HappyPath(t *testing.T) {
	sink := newTestSink(t)
	ev := Event{
		Event:     EventAuthSessionIssued,
		AccountID: "abc123def4567890",
		TS:        core.Now().UTC().Unix(),
		Outcome:   OutcomeOK,
	}
	r := sink.Record(ev)
	if !r.OK {
		t.Fatalf("Record failed on happy path: %s", r.Error())
	}
	path := sink.dayFilePath(ev.TS)
	rr := core.ReadFile(path)
	if !rr.OK {
		t.Fatalf("day-file unreadable after Record: %s", rr.Error())
	}
	body, _ := rr.Value.([]byte)
	if len(body) == 0 {
		t.Fatal("day-file empty after Record")
	}
	// Ends in newline — NDJSON contract.
	if body[len(body)-1] != '\n' {
		t.Fatal("day-file line missing trailing newline")
	}
}

func TestRecorder_Record_Bad_EmptyEventName(t *testing.T) {
	sink := newTestSink(t)
	r := sink.Record(Event{TS: core.Now().UTC().Unix()})
	if r.OK {
		t.Fatal("Record accepted event with empty Event field — should reject")
	}
}

func TestRecorder_Record_Bad_EmbeddedNewline(t *testing.T) {
	sink := newTestSink(t)
	r := sink.Record(Event{
		Event:   "auth.test.bad",
		Scope:   "line1\nline2",
		TS:      core.Now().UTC().Unix(),
		Outcome: OutcomeError,
	})
	if r.OK {
		t.Fatal("Record accepted event with embedded newline — should reject")
	}
}

func TestRecorder_Record_Bad_EmbeddedNewlineInMeta(t *testing.T) {
	sink := newTestSink(t)
	r := sink.Record(Event{
		Event:   "auth.test.bad",
		TS:      core.Now().UTC().Unix(),
		Outcome: OutcomeError,
		Meta:    map[string]any{"detail": "line1\nline2"},
	})
	if r.OK {
		t.Fatal("Record accepted event with embedded newline in Meta — should reject")
	}
}

func TestRecorder_Record_Ugly_TokenInMetaRedacted(t *testing.T) {
	sink := newTestSink(t)
	ev := Event{
		Event:   "auth.test.leak",
		TS:      core.Now().UTC().Unix(),
		Outcome: OutcomeError,
		Meta: map[string]any{
			"oops_pasted_token": "LTHN-SESS-1.test.canary.fixture",
		},
	}
	r := sink.Record(ev)
	if !r.OK {
		t.Fatalf("Record rejected leaky event when it should have redacted + persisted: %s", r.Error())
	}
	path := sink.dayFilePath(ev.TS)
	rr := core.ReadFile(path)
	if !rr.OK {
		t.Fatalf("day-file unreadable: %s", rr.Error())
	}
	body, _ := rr.Value.([]byte)
	if core.Contains(string(body), "LTHN-SESS-1.test.canary.fixture") {
		t.Fatal("leaked token bytes landed on disk — redactor bypassed")
	}
	if !core.Contains(string(body), redactionPlaceholder) {
		t.Fatal("redaction placeholder missing from persisted line")
	}
}

func TestRecorder_Record_Ugly_MultipleEventsAppend(t *testing.T) {
	sink := newTestSink(t)
	now := core.Now().UTC().Unix()
	for i := 0; i < 5; i++ {
		r := sink.Record(Event{
			Event:   "auth.test.append",
			TS:      now,
			Outcome: OutcomeOK,
		})
		if !r.OK {
			t.Fatalf("Record %d failed: %s", i, r.Error())
		}
	}
	path := sink.dayFilePath(now)
	rr := core.ReadFile(path)
	if !rr.OK {
		t.Fatalf("day-file unreadable: %s", rr.Error())
	}
	body, _ := rr.Value.([]byte)
	count := 0
	for _, b := range body {
		if b == '\n' {
			count++
		}
	}
	if count != 5 {
		t.Fatalf("expected 5 NDJSON lines, got %d", count)
	}
}

// --- Default + SetDefault --------------------------------------------

func TestRecorder_SetDefault_Good_OverrideAndReset(t *testing.T) {
	fake := &inMemoryRecorder{}
	SetDefault(fake)
	t.Cleanup(func() { SetDefault(nil) })

	r := Default().Record(Event{
		Event:   "auth.test.override",
		TS:      core.Now().UTC().Unix(),
		Outcome: OutcomeOK,
	})
	if !r.OK {
		t.Fatalf("Default().Record failed via override: %s", r.Error())
	}
	got := fake.snapshot()
	if len(got) != 1 {
		t.Fatalf("expected 1 event captured by fake, got %d", len(got))
	}
	if got[0].Event != "auth.test.override" {
		t.Fatalf("captured event name mismatch; got %q", got[0].Event)
	}
}

// --- dayFilePath -----------------------------------------------------

func TestRecorder_DayFilePath_Good_DeterministicFormat(t *testing.T) {
	sink := &fileSink{root: "/tmp/auditroot"}
	// 2026-05-16 00:00:00 UTC = 1778889600.
	got := sink.dayFilePath(1778889600)
	want := "/tmp/auditroot/2026-05-16.ndjson"
	if got != want {
		t.Fatalf("dayFilePath mismatch; got %q want %q", got, want)
	}
}

func TestRecorder_DayFilePath_Ugly_SingleDigitMonthDay(t *testing.T) {
	sink := &fileSink{root: "/tmp/auditroot"}
	// 2026-01-09 00:00:00 UTC = 1767916800. Both month + day need
	// zero-padding.
	got := sink.dayFilePath(1767916800)
	want := "/tmp/auditroot/2026-01-09.ndjson"
	if got != want {
		t.Fatalf("dayFilePath mismatch; got %q want %q", got, want)
	}
}
