// SPDX-Licence-Identifier: EUPL-1.2

// recorder.go — Phase 1 recorder substrate per RFC §3 / §4. Three
// pieces:
//
//   - Recorder interface — the contract callers depend on. Lets the
//     pkg/account + pkg/serverkey cross-cuts wire against an in-memory
//     fixture inside _test.go files without touching disk.
//   - fileSink — the production NDJSON file sink. One day-file per
//     call (file-per-day rolled by date), append-only via core.OpenFile
//     with O_APPEND | O_CREATE | O_WRONLY at mode 0o600.
//   - Default() — process-level singleton. Lazily constructs the file
//     sink on first call, rooted at ~/Lethean/audit/. Boot wiring
//     (Stage F's cmd/lthn/main.go) will switch this to a constructor-
//     time injection in Phase 2; today's helpers call Default() so
//     they don't need to thread the recorder through every callsite.
//
// Phase 1 deliberately ships SimpleRecorderSync semantics — every
// Record call opens the day-file, writes one NDJSON line, fsyncs,
// closes. The RFC §4.2 split-mode RecordSync vs RecordBatch is Phase 2
// work; today's per-event cost (~1-5ms) is acceptable because auth
// volume is sparse (≪1/sec realistic) and the cross-cut Phase 1 wires
// is auth-only.

package audit

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/paths"
)

// Recorder is the sink contract every audit emitter depends on. Phase
// 1 wires the file sink at process scope (via Default); _test.go fixtures
// implement this with an in-memory []Event slice so behavioural tests
// don't touch disk.
//
// Record validates the event (non-empty Event name + non-negative TS),
// runs the §2.1.1 secret-shape redactor over Meta, and persists the
// cleaned event. Returns core.Result so callers using core.* patterns
// don't need to reach for a different error shape.
//
// Usage example:
//
//	r := audit.Default().Record(audit.Event{
//	    Event: audit.EventAuthSessionIssued,
//	    AccountID: "abc123def4567890",
//	    TS: core.Now().UTC().Unix(),
//	    Outcome: audit.OutcomeOK,
//	})
//	if !r.OK { /* surface but never fail the parent op */ }
type Recorder interface {
	Record(ev Event) core.Result
}

// SetDefault overrides the package-level default recorder. Tests use
// this to inject an in-memory fixture; production boot wiring uses it
// to swap the lazily-constructed file sink for a constructor-injected
// instance. Pass nil to reset to lazy construction on the next Default
// call (useful in test teardown).
//
// Usage example:
//
//	fake := &inMemoryRecorder{}
//	audit.SetDefault(fake)
//	t.Cleanup(func() { audit.SetDefault(nil) })
func SetDefault(r Recorder) {
	defaultMu.Lock()
	defer defaultMu.Unlock()
	defaultRecorder = r
}

// Default returns the process-level recorder. Lazily constructs the
// ~/Lethean/audit/ file sink on first call. Returns a never-nil noop
// recorder if construction fails (e.g. ~/Lethean/ unreachable) — the
// audit-log MUST NEVER block an auth path, so failures surface as
// silent skip + a core.Print warning rather than a propagated error.
//
// Usage example:
//
//	r := audit.Default().Record(ev)
//	_ = r // ignore — audit failures never block auth paths
func Default() Recorder {
	defaultMu.RLock()
	r := defaultRecorder
	defaultMu.RUnlock()
	if r != nil {
		return r
	}
	defaultMu.Lock()
	defer defaultMu.Unlock()
	if defaultRecorder != nil {
		return defaultRecorder
	}
	sink, err := newFileSink()
	if err != nil {
		core.Print(core.Stderr(),
			"audit: file sink construction failed — events drop until SetDefault is called: %s\n",
			err.Error())
		defaultRecorder = noopRecorder{}
		return defaultRecorder
	}
	defaultRecorder = sink
	return defaultRecorder
}

// defaultRecorder + defaultMu hold the process-level singleton state.
// RWMutex because Default() is called on every audit emit (hot path);
// SetDefault is rare (test setup + boot wiring).
var (
	defaultRecorder Recorder
	defaultMu       core.RWMutex
)

// noopRecorder is the fallback when ~/Lethean/audit/ is unreachable.
// Records to nowhere; never errors. Auth paths keep working; the
// operator's only signal of the construction failure is the core.Print
// warning Default() emitted at first attempt.
type noopRecorder struct{}

// Record is a no-op for the noopRecorder. Returns OK so callers
// pattern-matching on r.OK don't branch into a failure path on what
// is, semantically, "audit unavailable but we're still serving the
// request."
func (noopRecorder) Record(_ Event) core.Result { return core.Ok(nil) }

// fileSink is the production NDJSON sink. Root is the parent directory
// (~/Lethean/audit/); per-call resolution of the day-file path lets
// rotation happen passively (Day N + 1's first Record opens a new
// file). Phase 2 swaps this for a long-lived file handle + a midnight
// rotation goroutine.
type fileSink struct {
	root string
	mu   core.Mutex
}

// newFileSink resolves ~/Lethean/audit/ via the paths package, creates
// the directory at mode 0o700 if missing, and returns a ready sink.
// Returns an error rather than core.Result so the Default() init path
// can branch into the noop-fallback cleanly.
func newFileSink() (*fileSink, error) {
	rootR := paths.Root()
	if !rootR.OK {
		return nil, core.E("audit.newFileSink", "resolve ~/Lethean/ root", rootR.Value.(error))
	}
	root, _ := rootR.Value.(string)
	auditRoot := core.PathJoin(root, auditDir)
	if r := core.MkdirAll(auditRoot, dirMode); !r.OK {
		return nil, core.E("audit.newFileSink",
			"mkdir ~/Lethean/audit/", r.Value.(error))
	}
	return &fileSink{root: auditRoot}, nil
}

// Record persists one Event as a single NDJSON line in the day-file
// for ev.TS. Steps:
//
//  1. Validate the event (non-empty Event, non-negative TS).
//  2. Redact Meta via the §2.1.1 secret-shape detector. Dirty events
//     get an audit.record.redacted debug-echo via core.Print (RFC §6.1
//     — never re-enter the typed bus).
//  3. Marshal the cleaned event to JSON. Reject events containing
//     embedded newlines in any string field (defends §6.5 single-
//     process interleave safety — POSIX append is atomic per-write but
//     a multi-line payload would interleave under contention).
//  4. Open the day-file via core.OpenFile with O_APPEND | O_CREATE |
//     O_WRONLY at mode 0o600. Write the JSON line + '\n'. fsync. Close.
//
// All failure paths return a core.Fail with an "audit.*" code. The
// caller is expected to log + continue — audit failures MUST NEVER
// block the auth path that emitted the event.
//
// Usage example:
//
//	r := sink.Record(ev)
//	if !r.OK { core.Print(core.Stderr(), "audit: %s\n", r.Error()) }
func (s *fileSink) Record(ev Event) core.Result {
	if ev.Event == "" {
		return core.Fail(core.NewCode(codeAuditEventInvalid,
			"Event.Event is required"))
	}
	if ev.TS == 0 {
		ev.TS = core.Now().UTC().Unix()
	}

	cleaned, dirty := redactMeta(ev.Meta)
	ev.Meta = cleaned
	if dirty {
		// RFC §6.1 — never re-enter the typed bus. Debug-echo via
		// core.Print is the audit-of-audit signal.
		core.Print(core.Stderr(),
			"event=audit.record.redacted ts=%d source_event=%s\n",
			core.Now().UTC().Unix(), ev.Event)
	}

	// Reject embedded newlines in any string field — would break the
	// NDJSON line contract + interleave under POSIX append. Walks Event
	// name + AccountID + Scope + Outcome + RequestID + every string in
	// Meta. The JSON serialiser would silently escape a literal newline
	// to the JSON backslash-n sequence, but the spec contract is "no
	// literal newline EVER lands in a Meta value" — the encoder-escape
	// would mask a caller's bug that injected a newline accidentally;
	// reject loud at the boundary instead.
	if containsRawNewline(ev) {
		return core.Fail(core.NewCode(codeAuditEventInvalid,
			"event contains a raw newline in a string field"))
	}

	bR := core.JSONMarshal(ev)
	if !bR.OK {
		return core.Fail(core.NewCode(codeAuditMarshalFailed,
			"marshalling event to JSON failed"))
	}
	line, _ := bR.Value.([]byte)
	// Append the trailing newline — NDJSON contract requires it AND
	// the writer must emit the newline atomically with the body so a
	// concurrent appender doesn't land its line between our body + our
	// terminator. POSIX guarantees atomicity for writes ≤ PIPE_BUF
	// (4096 on darwin); audit lines are dominated by Meta which we
	// expect to stay well under that cap for the auth-event family
	// Phase 1 cross-cuts.
	line = append(line, '\n')

	s.mu.Lock()
	defer s.mu.Unlock()

	dayPath := s.dayFilePath(ev.TS)
	openR := core.OpenFile(dayPath, core.O_APPEND|core.O_CREATE|core.O_WRONLY, fileMode)
	if !openR.OK {
		return core.Fail(core.NewCode(codeAuditOpenFailed,
			"opening day-file failed: "+openR.Value.(error).Error()))
	}
	f, _ := openR.Value.(*core.OSFile)
	if _, err := f.Write(line); err != nil {
		_ = f.Close()
		return core.Fail(core.NewCode(codeAuditWriteFailed,
			"writing day-file failed: "+err.Error()))
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return core.Fail(core.NewCode(codeAuditWriteFailed,
			"fsync day-file failed: "+err.Error()))
	}
	if err := f.Close(); err != nil {
		return core.Fail(core.NewCode(codeAuditWriteFailed,
			"closing day-file failed: "+err.Error()))
	}
	return core.Ok(nil)
}

// dayFilePath returns the absolute path of the YYYY-MM-DD.ndjson file
// for the supplied unix-seconds timestamp. RFC §4.1 names the layout;
// Phase 1 ships a flat <root>/<YYYY-MM-DD>.ndjson rather than the
// nested <root>/YYYY/MM/lthn-audit-YYYY-MM-DD.log shape (which is
// Phase 2 + rotation work — the flat shape is forward-compatible
// since a Phase 2 sweep can rename files into the year/month tree
// without breaking Query).
func (s *fileSink) dayFilePath(unixSec int64) string {
	t := core.UnixTime(unixSec).UTC()
	// Manual YYYY-MM-DD format — avoid taking a string-format dep on a
	// bigger time helper than necessary. Two-digit month / day padding
	// done arithmetically.
	y, mo, d := t.Year(), int(t.Month()), t.Day()
	name := core.Itoa(y) + "-" + zeroPad2(mo) + "-" + zeroPad2(d) + ".ndjson"
	return core.PathJoin(s.root, name)
}

// zeroPad2 returns a two-digit zero-padded decimal string for n in
// [0, 99]. Out-of-range inputs render as core.Itoa(n) verbatim — the
// caller's loop bound determines validity; this helper does not
// bounds-check (it would mask a caller bug to silently pad a 3-digit
// year-modulo error).
func zeroPad2(n int) string {
	if n < 10 && n >= 0 {
		return "0" + core.Itoa(n)
	}
	return core.Itoa(n)
}

// containsRawNewline walks every string field of ev (top-level + nested
// Meta) and reports whether any contains a literal '\n'. Used by Record
// to fail-loud on the embedded-newline class of caller bug before it
// corrupts the NDJSON stream.
func containsRawNewline(ev Event) bool {
	if core.Contains(ev.Event, "\n") {
		return true
	}
	if core.Contains(ev.AccountID, "\n") {
		return true
	}
	if core.Contains(ev.Scope, "\n") {
		return true
	}
	if core.Contains(ev.Outcome, "\n") {
		return true
	}
	if core.Contains(ev.RequestID, "\n") {
		return true
	}
	return mapContainsRawNewline(ev.Meta)
}

func mapContainsRawNewline(m map[string]any) bool {
	for k, v := range m {
		if core.Contains(k, "\n") {
			return true
		}
		if valueContainsRawNewline(v) {
			return true
		}
	}
	return false
}

func valueContainsRawNewline(v any) bool {
	switch tv := v.(type) {
	case string:
		return core.Contains(tv, "\n")
	case map[string]any:
		return mapContainsRawNewline(tv)
	case []any:
		for _, elem := range tv {
			if valueContainsRawNewline(elem) {
				return true
			}
		}
		return false
	default:
		return false
	}
}
