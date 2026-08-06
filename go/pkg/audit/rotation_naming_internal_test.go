// SPDX-Licence-Identifier: EUPL-1.2

// rotation_naming_internal_test.go — covers the sequenced-file naming
// helpers (threeDigit / nextSequenceForDay), the tamper-chain seed
// reader (readLastChainFromFile), and routes.go's since/until/limit
// query-param parsing branches — all pure or file-local, none
// previously exercised outside their happy-path callers.

package audit

import (
	"net/http"
	"testing"

	core "dappco.re/go"
)

// TestThreeDigit_Good — the zero-padded common range.
func TestThreeDigit_Good(t *testing.T) {
	core.AssertEqual(t, "000", threeDigit(0))
	core.AssertEqual(t, "007", threeDigit(7))
	core.AssertEqual(t, "042", threeDigit(42))
	core.AssertEqual(t, "099", threeDigit(99))
	core.AssertEqual(t, "100", threeDigit(100))
	core.AssertEqual(t, "999", threeDigit(999))
}

// TestThreeDigit_Bad — a negative input (shouldn't happen given the
// caller's own bounds, but the function is defensively total) falls
// back to the natural decimal rather than panicking or truncating.
func TestThreeDigit_Bad(t *testing.T) {
	core.AssertEqual(t, "-1", threeDigit(-1))
}

// TestThreeDigit_Ugly — a four-digit rotation day (pathological, more
// than 1000 size-rotations in one calendar day) stays grep-able as
// the natural decimal rather than silently truncating back to "000".
func TestThreeDigit_Ugly(t *testing.T) {
	core.AssertEqual(t, "1000", threeDigit(1000))
}

// TestNextSequenceForDay_Good — with a run of sequenced day-files on
// disk, the next available sequence is highest+1.
func TestNextSequenceForDay_Good(t *testing.T) {
	root := t.TempDir()
	stem := "2026-03-01"
	for _, name := range []string{
		stem + ".log",     // seq 0 (unsequenced)
		stem + ".001.log", // seq 1
		stem + ".003.log", // seq 3 -- highest
		"2026-03-02.log",  // different day -- must not count
	} {
		core.RequireTrue(t, core.WriteFile(core.PathJoin(root, name), []byte("x"), 0o600).OK)
	}
	core.AssertEqual(t, 4, nextSequenceForDay(root, stem))
}

// TestNextSequenceForDay_Bad — an unreadable/missing root directory
// degrades to 0 rather than erroring the caller.
func TestNextSequenceForDay_Bad(t *testing.T) {
	core.AssertEqual(t, 0, nextSequenceForDay(core.PathJoin(t.TempDir(), "does-not-exist"), "2026-03-01"))
}

// TestNextSequenceForDay_Ugly — an empty day (no matching files at
// all) returns 0, the unsequenced starting point.
func TestNextSequenceForDay_Ugly(t *testing.T) {
	root := t.TempDir()
	core.RequireTrue(t, core.WriteFile(core.PathJoin(root, "unrelated.txt"), []byte("x"), 0o600).OK)
	core.AssertEqual(t, 0, nextSequenceForDay(root, "2026-03-01"))
}

// TestReadLastChainFromFile_Good — after several real Record calls
// against a live Service, readLastChainFromFile on the resulting
// day-file returns the same prevChain the Service is currently
// holding (the most recent row's __chain value).
func TestReadLastChainFromFile_Good(t *testing.T) {
	svc := newTestService(t)
	for i := 0; i < 3; i++ {
		r := svc.RecordSync(Event{Event: "test.chain.seed", TS: core.Now().UTC().Unix(), Outcome: OutcomeOK})
		core.RequireTrue(t, r.OK)
	}
	svc.mu.Lock()
	wantChain := svc.prevChain
	path := svc.currentPath
	svc.mu.Unlock()
	core.RequireTrue(t, wantChain != "")

	got, r := readLastChainFromFile(path)
	core.RequireTrue(t, r.OK)
	core.AssertEqual(t, wantChain, got)
}

// TestReadLastChainFromFile_Bad — a missing file is genesis (empty
// prevChain, no error) — NOT a failure condition.
func TestReadLastChainFromFile_Bad(t *testing.T) {
	got, r := readLastChainFromFile(core.PathJoin(t.TempDir(), "never-written.log"))
	core.RequireTrue(t, r.OK)
	core.AssertEqual(t, "", got)
}

// TestReadLastChainFromFile_Ugly — an existing but empty file is also
// genesis, distinct from the missing-file case but the same result
// shape.
func TestReadLastChainFromFile_Ugly(t *testing.T) {
	path := core.PathJoin(t.TempDir(), "empty.log")
	core.RequireTrue(t, core.WriteFile(path, []byte{}, 0o600).OK)
	got, r := readLastChainFromFile(path)
	core.RequireTrue(t, r.OK)
	core.AssertEqual(t, "", got)
}

// TestParseEventsQuery_SinceUntilLimit_Good — well-formed since /
// until / limit query params all parse through to Query without
// rejection (200, not 400).
func TestParseEventsQuery_SinceUntilLimit_Good(t *testing.T) {
	svc := newTestService(t)
	eng := newRoutesEngine(t, svc)

	rr := doGET(eng, "/v1/audit/events?since=1700000000&until=1800000000&limit=10")
	core.AssertEqual(t, http.StatusOK, rr.Code)
}

// TestHashAccountID_Bad — the exported wrapper's two documented
// empty-input guards (empty accountID, empty secret) both short-
// circuit to "" without ever reaching the internal hasher.
func TestHashAccountID_Bad(t *testing.T) {
	core.AssertEqual(t, "", HashAccountID([]byte("a-secret"), ""))
	core.AssertEqual(t, "", HashAccountID(nil, "acct-123"))
}

// TestFileSinkRecord_RawNewlineRejected_Bad — an embedded newline
// nested inside Meta (both directly and inside a nested map/slice)
// is rejected loud rather than silently JSON-escaped, exercising
// containsRawNewline's Meta walk + mapContainsRawNewline +
// valueContainsRawNewline's map/slice recursion arms.
func TestFileSinkRecord_RawNewlineRejected_Bad(t *testing.T) {
	sink := &fileSink{root: t.TempDir()}

	cases := []struct {
		name string
		meta map[string]any
	}{
		{"direct", map[string]any{"note": "line1\nline2"}},
		{"nested-map", map[string]any{"outer": map[string]any{"inner": "a\nb"}}},
		{"nested-slice", map[string]any{"list": []any{"clean", "dirty\nvalue"}}},
		{"dirty-key", map[string]any{"bad\nkey": "value"}},
	}
	for _, tc := range cases {
		r := sink.Record(Event{
			Event: "test.newline." + tc.name,
			TS:    core.Now().UTC().Unix(),
			Meta:  tc.meta,
		})
		core.AssertFalse(t, r.OK)
		core.AssertContains(t, r.Error(), "raw newline")
	}
}

// TestParseEventsQuery_SinceUntilLimit_Bad — a malformed since /
// until / limit each independently 400 with the invalid_param code
// rather than silently falling back to "no filter."
func TestParseEventsQuery_SinceUntilLimit_Bad(t *testing.T) {
	svc := newTestService(t)
	eng := newRoutesEngine(t, svc)

	for _, qs := range []string{
		"since=not-a-number",
		"until=not-a-number",
		"limit=not-a-number",
	} {
		rr := doGET(eng, "/v1/audit/events?"+qs)
		core.AssertEqual(t, http.StatusBadRequest, rr.Code)
	}
}
