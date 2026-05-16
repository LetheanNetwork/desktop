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
