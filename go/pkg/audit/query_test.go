// SPDX-Licence-Identifier: EUPL-1.2

// query_test.go — Phase 2.3 Query tests per RFC.stage-f.md §10
// mandatory test list:
//
//   - TestService_Query_Good — predicate match round-trips.
//   - TestService_Query_Bad — invalid cursor → invalid_cursor code.
//   - TestQuery_AccountIDHashing_Ugly — record events for accounts
//     A + B; raw AccountID=A returns ONLY A's events; disk inspection
//     shows ONLY hashes (RFC §6.4).
//   - TestQuery_StreamingCursor_AcrossRotation_Ugly — cursor C
//     resumes correctly after .log → .log.gz rename chain (RFC §4.3).

package audit

import (
	"testing"
	"time"

	core "dappco.re/go"

	"dappco.re/lthn/desktop/pkg/audit/internal/rotation"
)

func TestService_Query_Good_PredicateMatch(t *testing.T) {
	svc := newTestService(t)
	now := core.Now().UTC().Unix()
	mustRecord(t, svc, Event{Event: EventAuthSessionIssued, TS: now, Outcome: OutcomeOK})
	mustRecord(t, svc, Event{Event: EventAuthUnlockFailed, TS: now, Outcome: OutcomeFailed})

	r := svc.Query(QueryInput{Event: EventAuthSessionIssued})
	if !r.OK {
		t.Fatalf("Query failed: %s", r.Error())
	}
	out, _ := r.Value.(QueryOutput)
	if len(out.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(out.Events))
	}
	if out.Events[0].Event != EventAuthSessionIssued {
		t.Fatalf("got %q, want %q", out.Events[0].Event, EventAuthSessionIssued)
	}
}

func TestService_Query_Good_NoFilterReturnsAll(t *testing.T) {
	svc := newTestService(t)
	now := core.Now().UTC().Unix()
	for i := 0; i < 5; i++ {
		mustRecord(t, svc, Event{Event: EventAuthSessionIssued, TS: now, Outcome: OutcomeOK})
	}
	r := svc.Query(QueryInput{})
	if !r.OK {
		t.Fatalf("Query failed: %s", r.Error())
	}
	out, _ := r.Value.(QueryOutput)
	if len(out.Events) != 5 {
		t.Fatalf("expected 5 events, got %d", len(out.Events))
	}
}

func TestService_Query_Good_OutcomeFilter(t *testing.T) {
	svc := newTestService(t)
	now := core.Now().UTC().Unix()
	mustRecord(t, svc, Event{Event: EventAuthSessionIssued, TS: now, Outcome: OutcomeOK})
	mustRecord(t, svc, Event{Event: EventAuthUnlockFailed, TS: now, Outcome: OutcomeFailed})

	r := svc.Query(QueryInput{Outcome: OutcomeFailed})
	if !r.OK {
		t.Fatalf("Query failed: %s", r.Error())
	}
	out, _ := r.Value.(QueryOutput)
	if len(out.Events) != 1 {
		t.Fatalf("expected 1 event with Outcome=failed, got %d", len(out.Events))
	}
}

func TestService_Query_Bad_InvalidCursor(t *testing.T) {
	svc := newTestService(t)
	r := svc.Query(QueryInput{Cursor: "this-is-not-base64-json"})
	if r.OK {
		t.Fatal("Query accepted obviously-malformed cursor — should reject")
	}
	if r.Code() != codeAuditQueryInvalidCursor {
		t.Fatalf("got code=%s, want %s", r.Code(), codeAuditQueryInvalidCursor)
	}
}

func TestQuery_AccountIDHashing_Ugly(t *testing.T) {
	// RFC §6.4 — record events for accounts A + B; raw query for A
	// returns ONLY A's events; disk inspection shows ONLY hashes
	// (no raw account_id substring anywhere in the .log file).
	svc := newTestService(t)
	now := core.Now().UTC().Unix()
	rawA := "account-A-canary-id"
	rawB := "account-B-canary-id"

	mustRecord(t, svc, Event{
		Event: EventAuthSessionIssued, TS: now, AccountID: rawA, Outcome: OutcomeOK,
	})
	mustRecord(t, svc, Event{
		Event: EventAuthSessionIssued, TS: now, AccountID: rawB, Outcome: OutcomeOK,
	})

	// (a) Predicate compare uses hash — query by raw A returns A only.
	r := svc.Query(QueryInput{AccountID: rawA})
	if !r.OK {
		t.Fatalf("Query failed: %s", r.Error())
	}
	out, _ := r.Value.(QueryOutput)
	if len(out.Events) != 1 {
		t.Fatalf("expected 1 event for account A, got %d", len(out.Events))
	}
	// The returned event carries the HASHED account_id (Query reads
	// the on-disk form verbatim — there's no reverse-hash lookup).
	wantHash := HashAccountID(svc.secret, rawA)
	if out.Events[0].AccountID != wantHash {
		t.Fatalf("returned account_id %q is not the hashed form %q",
			out.Events[0].AccountID, wantHash)
	}

	// (b) Disk inspection — neither raw A nor raw B appears.
	body := readDayFile(t, svc, now)
	if core.Contains(string(body), rawA) {
		t.Fatal("raw account_id A leaked to disk")
	}
	if core.Contains(string(body), rawB) {
		t.Fatal("raw account_id B leaked to disk")
	}
	// Both hashes should be present.
	if !core.Contains(string(body), HashAccountID(svc.secret, rawA)) {
		t.Fatal("hashed account_id A not found on disk")
	}
	if !core.Contains(string(body), HashAccountID(svc.secret, rawB)) {
		t.Fatal("hashed account_id B not found on disk")
	}
}

func TestQuery_StreamingCursor_AcrossRotation_Ugly(t *testing.T) {
	// RFC §4.3 ADD-HIGH-2 — cursor C from query at T1 resumes
	// correctly at T2 after .log → .log.gz rename. Tests the
	// rotation.ResolveDayFile suffix chain.
	svc := newTestService(t)
	now := core.Now().UTC().Unix()

	// Record 10 events; limit query to 5 to force a cursor emission.
	for i := 0; i < 10; i++ {
		mustRecord(t, svc, Event{
			Event:   "queue.enqueued", // cascade path — written, then we'll force flush via Close+reopen
			TS:      now + int64(i),
			Outcome: OutcomeOK,
		})
	}
	// Force flush by Close + replace.
	_ = svc.Close()
	svc2 := New(nil, Options{
		Root:                  svc.root,
		AuditSecret:           svc.secret,
		RotationCheckInterval: time.Hour,
	})
	t.Cleanup(func() { _ = svc2.Close() })

	r := svc2.Query(QueryInput{Limit: 5})
	if !r.OK {
		t.Fatalf("first Query failed: %s", r.Error())
	}
	out, _ := r.Value.(QueryOutput)
	if len(out.Events) != 5 {
		t.Fatalf("first page expected 5 events, got %d", len(out.Events))
	}
	if out.NextCursor == "" {
		t.Fatal("first page missing NextCursor — second page unreachable")
	}

	// Simulate rotation: rename the day-file .log → .log.gz manually
	// (mirrors what the background loop would do at the
	// CompressOlderThan boundary). Use the internal rotation.Compress
	// helper so the gzip wrapping matches what rotation would produce.
	logPath := core.PathJoin(svc.root, dateStem(now)+rotation.LogSuffix)
	if _, cr := rotation.Compress(logPath, fileMode); !cr.OK {
		t.Fatalf("simulated rotation failed: %s", cr.Error())
	}

	// Resume with the cursor — must see the remaining 5 events even
	// though the file is now .log.gz.
	r2 := svc2.Query(QueryInput{Limit: 5, Cursor: out.NextCursor})
	if !r2.OK {
		t.Fatalf("resume Query failed: %s", r2.Error())
	}
	out2, _ := r2.Value.(QueryOutput)
	if len(out2.Events) != 5 {
		t.Fatalf("resume page expected 5 events, got %d", len(out2.Events))
	}

	// Combined the two pages should cover all 10 events with no
	// duplicates.
	seen := map[int64]bool{}
	for _, ev := range out.Events {
		seen[ev.TS] = true
	}
	for _, ev := range out2.Events {
		if seen[ev.TS] {
			t.Fatalf("event TS %d appeared in both pages — cursor resume duplicated", ev.TS)
		}
		seen[ev.TS] = true
	}
	if len(seen) != 10 {
		t.Fatalf("expected 10 unique events across pages, got %d", len(seen))
	}
}

func mustRecord(t *testing.T, svc *Service, ev Event) {
	t.Helper()
	r := svc.Record(ev)
	if !r.OK {
		t.Fatalf("Record failed: %s", r.Error())
	}
}

// --- Cerberus #29 ADD-MED-3 — default range cap + wall-clock deadline ---
// (RFC.stage-e-audit-viewer v2 §5.7 + §8.4)

// writeDayFileRaw lays down a synthetic day-file at YYYY-MM-DD.log
// inside the service's audit root carrying the supplied events as
// NDJSON. Used to fabricate days OLDER than the rotation goroutine
// would naturally produce in a test run — exercises the default-clamp
// + cursor-resume code paths without waiting real days.
func writeDayFileRaw(t *testing.T, svc *Service, stem string, events []Event) {
	t.Helper()
	path := core.PathJoin(svc.root, stem+".log")
	var body []byte
	for _, ev := range events {
		// Stamp the hash + process discriminator so the on-disk shape
		// matches what Service.Record would have produced.
		if ev.AccountID != "" {
			ev.AccountID = hashAccountID(svc.secret, ev.AccountID)
		}
		if ev.Meta == nil {
			ev.Meta = map[string]any{}
		}
		if _, has := ev.Meta[metaKeyProcess]; !has {
			ev.Meta[metaKeyProcess] = svc.opts.ProcessLabel
		}
		bR := core.JSONMarshal(ev)
		if !bR.OK {
			t.Fatalf("marshal: %s", bR.Error())
		}
		line, _ := bR.Value.([]byte)
		body = append(body, line...)
		body = append(body, '\n')
	}
	if r := core.WriteFile(path, body, fileMode); !r.OK {
		t.Fatalf("write %s: %s", path, r.Error())
	}
}

// stemDaysAgo returns the YYYY-MM-DD stem `n` days before today UTC.
func stemDaysAgo(n int) string {
	t := core.Now().UTC().Add(-time.Duration(n) * 24 * time.Hour)
	return dateStem(t.Unix())
}

func TestQuery_DefaultRangeClampsTo30d_Good(t *testing.T) {
	// Cerberus #29 ADD-MED-3 — Query with Since=0 + Cursor="" against a
	// disk holding day-files from 90 days back returns events ONLY from
	// the last 30 days. Old-day events MUST NOT appear in the result.
	svc := newTestService(t)
	now := core.Now().UTC().Unix()

	// Far-past day (90 days ago) — outside the default 30d cap.
	writeDayFileRaw(t, svc, stemDaysAgo(90), []Event{{
		Event:   EventAuthSessionIssued,
		TS:      now - 90*24*3600,
		Outcome: OutcomeOK,
		Meta:    map[string]any{"canary": "far-past"},
	}})
	// Recent day (5 days ago) — inside the default 30d cap.
	writeDayFileRaw(t, svc, stemDaysAgo(5), []Event{{
		Event:   EventAuthSessionIssued,
		TS:      now - 5*24*3600,
		Outcome: OutcomeOK,
		Meta:    map[string]any{"canary": "recent"},
	}})

	r := svc.Query(QueryInput{})
	if !r.OK {
		t.Fatalf("Query failed: %s", r.Error())
	}
	out, _ := r.Value.(QueryOutput)
	for _, ev := range out.Events {
		if marker, _ := ev.Meta["canary"].(string); marker == "far-past" {
			t.Fatalf("ADD-MED-3: 90d-old event leaked past the default 30d clamp")
		}
	}
	if len(out.Events) == 0 {
		t.Fatal("expected the 5d-old event in result; default clamp dropped too much")
	}
}

func TestQuery_ExplicitSinceOverridesClamp_Good(t *testing.T) {
	// Cerberus #29 ADD-MED-3 — explicit Since reaches BEYOND the default
	// 30d clamp. Operator opt-in for deeper forensic queries works.
	svc := newTestService(t)
	now := core.Now().UTC().Unix()

	writeDayFileRaw(t, svc, stemDaysAgo(60), []Event{{
		Event:   EventAuthSessionIssued,
		TS:      now - 60*24*3600,
		Outcome: OutcomeOK,
		Meta:    map[string]any{"canary": "60d"},
	}})

	r := svc.Query(QueryInput{
		Since: core.Now().UTC().Add(-90 * 24 * time.Hour),
	})
	if !r.OK {
		t.Fatalf("Query failed: %s", r.Error())
	}
	out, _ := r.Value.(QueryOutput)
	found := false
	for _, ev := range out.Events {
		if marker, _ := ev.Meta["canary"].(string); marker == "60d" {
			found = true
		}
	}
	if !found {
		t.Fatal("explicit Since=90d-ago MUST surface the 60d-old event; default clamp wrongly applied")
	}
}

func TestQuery_CursorOverridesClamp_Good(t *testing.T) {
	// Cerberus #29 ADD-MED-3 — cursor-resume bypasses the default clamp.
	// A cursor encoding a 45d-old file_date MUST resume there, not
	// snap forward to the 30d boundary.
	svc := newTestService(t)
	now := core.Now().UTC().Unix()

	farStem := stemDaysAgo(45)
	writeDayFileRaw(t, svc, farStem, []Event{{
		Event:   EventAuthSessionIssued,
		TS:      now - 45*24*3600,
		Outcome: OutcomeOK,
		Meta:    map[string]any{"canary": "45d"},
	}})

	// Build a cursor pointing at the 45d file with no per-day floor.
	cur := encodeCursor(farStem, 0)
	r := svc.Query(QueryInput{Cursor: cur})
	if !r.OK {
		t.Fatalf("Query failed: %s", r.Error())
	}
	out, _ := r.Value.(QueryOutput)
	found := false
	for _, ev := range out.Events {
		if marker, _ := ev.Meta["canary"].(string); marker == "45d" {
			found = true
		}
	}
	if !found {
		t.Fatal("cursor-resume MUST bypass the default clamp; 45d-old event was wrongly skipped")
	}
}

func TestQuery_DeadlineReturnsPartialCursor_Ugly(t *testing.T) {
	// Cerberus #29 ADD-MED-3 — wall-clock deadline. Lay down enough
	// day-files that the per-day-boundary deadline check trips before
	// the walker completes. Helper returns OK with
	// QueryOutput.DeadlineExceeded=true + NextCursor encoding the
	// resume position.
	svc := newTestService(t)
	now := core.Now().UTC().Unix()

	// 5 day-files spread across the clamp window so the walker has
	// real work to do; we shorten the deadline via the test hook to
	// fire mid-walk.
	for i := 1; i <= 5; i++ {
		stem := stemDaysAgo(i)
		writeDayFileRaw(t, svc, stem, []Event{{
			Event:   EventAuthSessionIssued,
			TS:      now - int64(i)*24*3600,
			Outcome: OutcomeOK,
		}})
	}

	// Force the deadline to fire immediately by overriding now() via a
	// near-zero deadline. We can't monkeypatch core.Now, so the test
	// uses the package-internal queryWithDeadline test seam — see
	// queryDeadlineForTest below.
	out, hit := queryForTestWithDeadline(svc, QueryInput{}, 1)
	if !hit {
		t.Fatal("expected DeadlineExceeded flag to be set with an effectively-zero deadline")
	}
	if out.NextCursor == "" {
		t.Fatal("deadline-fire MUST emit a NextCursor so callers can resume")
	}
}

// queryForTestWithDeadline mirrors Service.Query but with a caller-
// supplied deadline override. Lives in the test file so production
// code stays free of test seams.
func queryForTestWithDeadline(s *Service, in QueryInput, deadlineNs int64) (QueryOutput, bool) {
	// Temporarily swap the package-level deadline override hook used by
	// Service.Query. Test-only seam — production calls always read
	// defaultQueryDeadline.
	prev := queryDeadlineOverride
	queryDeadlineOverride = time.Duration(deadlineNs)
	defer func() { queryDeadlineOverride = prev }()
	r := s.Query(in)
	if !r.OK {
		return QueryOutput{}, false
	}
	out, _ := r.Value.(QueryOutput)
	return out, out.DeadlineExceeded
}
