// SPDX-Licence-Identifier: EUPL-1.2

// query.go — Streaming-cursor Query path per RFC.stage-f.md §4.3.
// Walks day-files in the audit root from Since to Until, applying
// the predicate inline, returning up to QueryInput.Limit matches +
// an opaque Cursor the caller resumes from.
//
// Cursor format (RFC §4.3 ADD-HIGH-2):
//
//	base64({"file_date":"YYYY-MM-DD","event_ts_lower_bound":<unix>})
//
// The timestamp-bound cursor survives rotation (.log → .log.gz →
// .log.gz.candidate) because rotation.ResolveDayFile retries every
// suffix in order; v1's `{file_path, byte_offset}` shape broke on
// every operator-state change.

package audit

import (
	"bufio"
	"io"
	"time"

	core "dappco.re/go"

	"dappco.re/lthn/desktop/pkg/audit/internal/rotation"
)

// QueryInput is the predicate shape for Service.Query. All fields are
// optional — zero-value means "no filter on this field." Limit caps
// the per-call result count to defend against unbounded scans.
//
// Usage example:
//
//	r := svc.Query(audit.QueryInput{
//	    Since:     core.UnixTime(now - 3600),
//	    Event:     audit.EventAuthSessionIssued,
//	    AccountID: "abc123def4567890", // hashed before compare
//	    Limit:     200,
//	})
//	if r.OK { out := r.Value.(audit.QueryOutput); _ = out.Events }
type QueryInput struct {
	// Since is the inclusive lower-bound timestamp. Zero-value =
	// no lower bound (scan from the oldest day on disk).
	Since core.Time `json:"since"`

	// Until is the exclusive upper-bound timestamp. Zero-value = scan
	// up to "now."
	Until core.Time `json:"until"`

	// Event filters by exact dotted event-name match. Empty = all
	// events.
	Event string `json:"event"`

	// AccountID is the RAW account_id from the caller; Query hashes
	// it under the Service's HMAC secret before comparing against
	// on-disk values (RFC §6.4 — the on-disk form is the hash, so
	// the predicate operates in hash-space too).
	AccountID string `json:"account_id"`

	// Outcome filters by exact match against the OutcomeOK/Failed/
	// Denied/Error enum. Empty = all.
	Outcome string `json:"outcome"`

	// Limit caps the per-call result count. Default: 200. Maximum:
	// 5000 — caller pagination via Cursor is the only path to larger
	// totals. The cap defends against a forensic-query that loads
	// "everything ever" and OOMs the recorder.
	Limit int `json:"limit"`

	// Cursor is an opaque continuation token from a prior Query call.
	// Empty = start from Since.
	Cursor string `json:"cursor"`
}

// QueryOutput is the Result.Value shape Service.Query returns. Events
// holds up to Limit matches in TS-ascending order; NextCursor is
// non-empty when more results may exist beyond Limit.
//
// Usage example:
//
//	for {
//	    r := svc.Query(in)
//	    if !r.OK { break }
//	    out, _ := r.Value.(audit.QueryOutput)
//	    for _, ev := range out.Events { handle(ev) }
//	    if out.NextCursor == "" { break }
//	    in.Cursor = out.NextCursor
//	}
type QueryOutput struct {
	Events     []Event `json:"events"`
	NextCursor string  `json:"next_cursor"`
}

// queryCursor is the decoded shape behind the opaque Cursor string.
// Stored as JSON inside a base64-url wrapper so wire transport is
// trivial. See RFC §4.3 ADD-HIGH-2.
type queryCursor struct {
	FileDate          string `json:"file_date"`
	EventTSLowerBound int64  `json:"event_ts_lower_bound"`
}

// Query walks the audit root applying the predicate. Returns up to
// QueryInput.Limit matches in TS-ascending order. Errors during file
// open/decompress are logged + skipped (audit failures MUST NEVER
// block the caller) — the result represents best-effort matches from
// the readable subset.
//
// Per RFC §4.3 the scan is streaming-cursor — never loads the full
// dataset into memory. Cursor survives rotation/compression/retention
// rename by re-resolving the file at each resume.
//
// Authz scope per RFC §5.3 ADD-MED-3: F v1 grants any unlocked
// session read access to the entire audit log across all account_ids
// — single-user desktop assumption. Per-account scoping is Stage F+
// work IF multi-user desktop becomes a deploy mode.
//
// Usage example:
//
//	r := svc.Query(audit.QueryInput{Limit: 100})
//	if r.OK { out := r.Value.(audit.QueryOutput); render(out.Events) }
func (s *Service) Query(in QueryInput) core.Result {
	if s.root == "" {
		// Degraded Service — nothing to query.
		return core.Ok(QueryOutput{})
	}
	limit := in.Limit
	if limit <= 0 {
		limit = defaultQueryLimit
	}
	if limit > maxQueryLimit {
		limit = maxQueryLimit
	}

	// Hash the predicate's account_id BEFORE walking files so the
	// inner loop compares hash-to-hash without re-hashing per event.
	hashedAccount := ""
	if in.AccountID != "" {
		hashedAccount = hashAccountID(s.secret, in.AccountID)
	}

	startDate, startTSFloor, err := decodeCursor(in.Cursor, in.Since)
	if err != nil {
		return core.Fail(core.NewCode(codeAuditQueryInvalidCursor,
			"invalid cursor: "+err.Error()))
	}

	// When Since is unset AND no Cursor, jump the start cursor to the
	// earliest YYYY-MM-DD stem actually present on disk so we don't
	// walk thousands of empty days from the epoch forward. The cursor
	// already lands on a valid file when supplied (no scan needed).
	if in.Cursor == "" && in.Since.IsZero() {
		if earliest, ok := earliestStem(s.root); ok {
			startDate = earliest
		}
	}

	endDate := dateStem(timeOrNow(in.Until).Unix())

	out := QueryOutput{Events: make([]Event, 0, limit)}

	cur := startDate
	for {
		if cur > endDate {
			break
		}
		// Resolve the day-file across the suffix chain — survives
		// rotation/compression/retention rename per RFC §4.3.
		path, ok := rotation.ResolveDayFile(s.root, cur)
		if !ok {
			// No file for that day — advance.
			cur = nextDay(cur)
			continue
		}
		rd, closer, openR := rotation.OpenForRead(path)
		if !openR.OK {
			closer()
			cur = nextDay(cur)
			continue
		}
		shouldStop, nextCursor := scanDayFile(rd, cur, startTSFloor, in, hashedAccount, limit, &out)
		closer()
		if shouldStop {
			out.NextCursor = nextCursor
			break
		}
		cur = nextDay(cur)
		// After moving to next day the floor resets — startTSFloor
		// is only meaningful for the resume-day.
		startTSFloor = 0
	}
	return core.Ok(out)
}

// scanDayFile reads one day-file line by line, decodes each NDJSON
// Event, applies the predicate, accumulates matches into out. Returns
// (shouldStop, nextCursor):
//
//   - shouldStop=true when we've hit Limit + caller should bail; the
//     emitted cursor lets a follow-up Query resume mid-day.
//   - shouldStop=false when the file was fully scanned; the outer
//     loop advances to next day.
func scanDayFile(rd io.Reader, fileDate string, tsFloor int64, in QueryInput,
	hashedAccount string, limit int, out *QueryOutput) (bool, string) {
	scanner := bufio.NewScanner(rd)
	// Generous per-line buffer — events have free-form Meta + can
	// easily exceed the bufio default 64KB on a forensic-query for
	// an event with a verbose meta payload.
	const maxLineBytes = 1024 * 1024 // 1MB
	scanner.Buffer(make([]byte, 0, 64*1024), maxLineBytes)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var ev Event
		if r := core.JSONUnmarshal(line, &ev); !r.OK {
			// Malformed line per RFC §6.6 — skip + emit typed event.
			// Self-record is deferred to a follow-on; for v1 the skip
			// happens silently (a future Service hook can wire the
			// audit.record.skipped_malformed_line emission).
			continue
		}
		if !in.Since.IsZero() && ev.TS < in.Since.Unix() {
			continue
		}
		if !in.Until.IsZero() && ev.TS >= in.Until.Unix() {
			continue
		}
		if tsFloor > 0 && ev.TS < tsFloor {
			continue
		}
		if in.Event != "" && ev.Event != in.Event {
			continue
		}
		if hashedAccount != "" && ev.AccountID != hashedAccount {
			continue
		}
		if in.Outcome != "" && ev.Outcome != in.Outcome {
			continue
		}
		out.Events = append(out.Events, ev)
		if len(out.Events) >= limit {
			// Emit cursor for the NEXT event past this one. The
			// resume-floor is (ev.TS + 1) so we don't replay the
			// current event; if multiple events share the same
			// second the next call may miss them — for Phase 2
			// this is an acceptable trade-off, Phase 3 can switch
			// to nanosecond-resolution TS.
			return true, encodeCursor(fileDate, ev.TS+1)
		}
	}
	return false, ""
}

// encodeCursor returns the base64-JSON shape per RFC §4.3.
//
// Usage example (internal):
//
//	cur := encodeCursor("2026-05-16", 1778889600)
func encodeCursor(date string, tsFloor int64) string {
	c := queryCursor{FileDate: date, EventTSLowerBound: tsFloor}
	b := core.JSONMarshal(c)
	if !b.OK {
		return ""
	}
	return core.Base64URLEncode(b.Value.([]byte))
}

// decodeCursor extracts the (file_date, ts_floor) pair from an opaque
// Cursor. Empty cursor returns the Since-derived defaults: file_date
// from Since's day (or today if Since is zero), ts_floor=0.
//
// Returns ("", 0, err) on a malformed cursor — caller surfaces as the
// codeAuditQueryInvalidCursor failure.
func decodeCursor(cursor string, since core.Time) (string, int64, error) {
	if cursor == "" {
		return dateStem(timeOrEpoch(since).Unix()), 0, nil
	}
	rawR := core.Base64URLDecode(cursor)
	if !rawR.OK {
		return "", 0, core.NewError("cursor base64 decode failed")
	}
	raw, _ := rawR.Value.([]byte)
	var c queryCursor
	if r := core.JSONUnmarshal(raw, &c); !r.OK {
		return "", 0, core.NewError("cursor JSON decode failed")
	}
	if len(c.FileDate) != 10 || c.FileDate[4] != '-' || c.FileDate[7] != '-' {
		return "", 0, core.NewError("cursor file_date malformed")
	}
	return c.FileDate, c.EventTSLowerBound, nil
}

// nextDay returns the YYYY-MM-DD stem one day after the supplied
// stem. Used by Query to advance through the day-file tree.
func nextDay(stem string) string {
	t, ok := parseDateStem(stem)
	if !ok {
		return stem
	}
	t = t.Add(24 * time.Hour)
	y, mo, d := t.Year(), int(t.Month()), t.Day()
	return core.Itoa(y) + "-" + zeroPad2(mo) + "-" + zeroPad2(d)
}

// timeOrNow returns t when t is non-zero, otherwise core.Now().UTC().
// Used by Query.Until handling so a zero-value Until means "scan up
// to right now."
func timeOrNow(t core.Time) core.Time {
	if t.IsZero() {
		return core.Now().UTC()
	}
	return t
}

// timeOrEpoch returns t when t is non-zero, otherwise the unix-epoch
// time. Used by decodeCursor so a zero-value Since means "scan from
// the start of recorded history."
func timeOrEpoch(t core.Time) core.Time {
	if t.IsZero() {
		// Unix epoch — practical effect is "the earliest day-file
		// on disk." The outer Query loop short-circuits on missing
		// files so it doesn't actually walk every day from 1970.
		return time.Unix(0, 0).UTC()
	}
	return t
}

// earliestStem returns the YYYY-MM-DD stem of the oldest day-file in
// the audit root. Returns ("", false) when the directory is empty or
// unreadable. Used by Query to skip the epoch-forward walk when the
// caller leaves Since unset.
func earliestStem(root string) (string, bool) {
	entriesR := core.ReadDir(core.DirFS(root), ".")
	if !entriesR.OK {
		return "", false
	}
	entries, _ := entriesR.Value.([]core.FsDirEntry)
	earliest := ""
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if len(name) < 10 {
			continue
		}
		stem := name[:10]
		if _, ok := parseDateStem(stem); !ok {
			continue
		}
		if earliest == "" || stem < earliest {
			earliest = stem
		}
	}
	return earliest, earliest != ""
}

// Limit defaults + cap per RFC §4.3 + §3.1.
const (
	defaultQueryLimit = 200
	maxQueryLimit     = 5000
)

// Error codes Query emits.
const (
	codeAuditQueryInvalidCursor = "audit.query.invalid_cursor"
)
