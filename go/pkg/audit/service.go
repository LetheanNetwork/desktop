// SPDX-Licence-Identifier: EUPL-1.2

// service.go — Phase 2 long-lived audit Service per RFC.stage-f.md §3.
// Wraps the Phase 1 file-per-event substrate with:
//
//   - Long-lived day-file handle held by the Service for the lifetime
//     of the active local day. Per-Record cost drops from
//     ~OpenFile+Write+Sync+Close to Write+(maybe-Sync).
//   - Dual-mode Record split per RFC §4.2:
//       * RecordSync (auth.*) — fsyncs every event, ≤50ms p99.
//       * RecordBatch (cascade) — writes immediately, fsyncs at the
//         page boundary (200ms OR 100 events, whichever first).
//   - Rotation goroutine — wakes every Options.RotationCheckInterval
//     (default 60s) to (a) swap handle on day-boundary crossing, (b)
//     gzip files older than CompressOlderThan, (c) rename files older
//     than KeepDays to `.log.gz.candidate`.
//   - Rotation-barrier (Cerberus #1711 MED-1) — every Record call
//     compares the current local day against the handle's day; a
//     drift triggers immediate handle swap before write. Defends
//     against the sleep-wake case where the background goroutine is
//     paused while a still-running emitter would otherwise write to
//     the stale day-file.
//
// Service implements Recorder so existing emit-sites (pkg/account,
// pkg/serverkey) keep working unchanged via audit.Default(). Boot
// wiring in cmd/lthn/app.go constructs the Service explicitly + calls
// audit.SetDefault(svc) to swap the lazy file sink for the configured
// instance.

package audit

import (
	"time"

	core "dappco.re/go"

	"dappco.re/lthn/desktop/pkg/audit/internal/rotation"
	"dappco.re/lthn/desktop/pkg/paths"
)

// audit file naming layout (Mantis #1651 size-rotation):
//   - Sequenced day-files: <YYYY-MM-DD>.<NNN>.log  e.g. 2026-05-17.000.log
//   - First file of each day starts at sequence 000; size-rotation
//     opens .001, .002, ... as the threshold trips. NNN is fixed-width
//     three-digit so lexical sort matches arrival order.
//   - Rotation interfaces (rotation.Compress, rotation.Candidate) treat
//     the `<date>.<NNN>.log` stem as one unit — every sequenced file
//     rotates independently to its own .log.gz/.log.gz.candidate.

// Service is the long-lived audit recorder per RFC §3. Construct via
// audit.New; call audit.SetDefault(svc) so existing audit.Default()
// callers route through the configured instance. One Service per
// process is the production discipline (Mantis #1711's lthn-mlx case
// is a SEPARATE Service in a SEPARATE binary).
//
// Usage example (boot):
//
//	svc := audit.New(c, audit.Options{
//	    AuditSecret:  serverkeySvc.AuditHMACSecret(),
//	    ProcessLabel: "lthn",
//	})
//	audit.SetDefault(svc)
//
// Usage example (emitter):
//
//	_ = audit.Default().Record(audit.Event{Event: ..., Outcome: audit.OutcomeOK})
type Service struct {
	core *core.Core
	opts Options
	root string

	// secret is the resolved HMAC key for §6.4 account_id hashing.
	// Either Options.AuditSecret verbatim OR a process-local random
	// fallback (with the "non-persistent" warning logged at New-time).
	// Guarded by secretMu so SetSecret (Mantis #1522 boot-order fix) can
	// swap the random fallback for the serverkey-derived HMAC after
	// serverkey.Bootstrap lands ~/Lethean/wallets/.seed.
	secret   []byte
	secretMu core.RWMutex

	// noopCounter increments per dropped event when the active sink
	// is a noopRecorder (i.e. root resolution failed). Re-emits a
	// core.Print warning every Options.NoopDropWarningEvery hits
	// (Cerberus #1711 MED-2).
	noopCounter int64

	// state guards the live file handle + the in-process batching
	// buffer state. mu (writer-side) is held for every Record call
	// and the rotation goroutine's handle-swap; the dedicated
	// rotation interval is short enough (default 60s) that lock
	// contention with the writer goroutine stays well below auth-
	// emit latency budgets.
	mu          core.Mutex
	currentFile *core.OSFile
	currentDate string // YYYY-MM-DD of the handle's day
	currentSeq  int    // NNN sequence within the day (Mantis #1651 size-rot)
	currentPath string // absolute path of the handle's file
	currentSize int64  // bytes written through the live handle

	// prevChain is the hex SHA-256 digest of the previous row's chain
	// link (Mantis #1651 tamper-resistance). Seeded from the existing
	// file's tail on handle-open; empty string at genesis. Guarded by
	// s.mu so concurrent Record() callers see a coherent sequence.
	prevChain string

	// batched tracks how many RecordBatch writes have happened since
	// the last fsync; reset to 0 on every Sync. The rotation loop
	// also force-flushes at BatchFlushInterval.
	batched int

	// stop signals the rotation goroutine to exit. Closed by Close().
	stop chan struct{}
}

// New constructs an audit Service. Resolves the root directory (Options.Root
// or ~/Lethean/audit/ via paths.Root), validates the HMAC secret (Options.AuditSecret
// or random fallback), and spawns the rotation goroutine. Returns a
// never-nil *Service — if root-resolution fails the returned Service is
// "degraded" and every Record() drops into the noop path (the operator
// sees a core.Print warning at construction + counter-throttled warnings
// thereafter).
//
// The returned Service does NOT auto-register as audit.Default() — the
// caller is expected to call audit.SetDefault(svc) explicitly so
// degraded states surface loud rather than silent-swap.
//
// Usage example:
//
//	svc := audit.New(c, audit.Options{AuditSecret: secret})
//	audit.SetDefault(svc)
//	defer svc.Close()
func New(c *core.Core, in Options) *Service {
	opts := resolved(in)
	root := opts.Root
	if root == "" {
		rootR := paths.Root()
		if !rootR.OK {
			core.Print(core.Stderr(),
				"audit: paths.Root failed — audit will drop events: %s\n",
				rootR.Error())
			return &Service{core: c, opts: opts, stop: make(chan struct{})}
		}
		base, _ := rootR.Value.(string)
		root = core.PathJoin(base, auditDir)
	}
	if r := core.MkdirAll(root, dirMode); !r.OK {
		core.Print(core.Stderr(),
			"audit: mkdir %s failed — audit will drop events: %s\n",
			root, r.Error())
		return &Service{core: c, opts: opts, stop: make(chan struct{})}
	}

	secret := opts.AuditSecret
	if len(secret) == 0 {
		randR := core.RandomBytes(32)
		if randR.OK {
			secret, _ = randR.Value.([]byte)
		}
		core.Print(core.Stderr(),
			"audit: AuditSecret was not supplied — using a process-local random fallback; account_id-keyed Query results will NOT survive a process restart\n")
	}

	s := &Service{
		core:   c,
		opts:   opts,
		root:   root,
		secret: secret,
		stop:   make(chan struct{}),
	}
	// Spawn the rotation goroutine. Uses c.Go when c is non-nil so
	// the supervised-goroutine machinery (panic recovery + shutdown
	// signalling) wraps it; falls back to a bare go statement when
	// the caller passed nil core (test fixtures).
	if c != nil {
		c.Go(s.rotationLoop)
	} else {
		go s.rotationLoop()
	}
	return s
}

// SetSecret swaps the HMAC secret used for §6.4 account_id hashing.
// Designed for the Mantis #1522 boot-order fix: cmd/lthn/app.go
// constructs the audit Service immediately after core.New (with
// Options{} → process-local random fallback) so events from earliest
// service OnStart hooks land in the file sink rather than the
// noopRecorder. Once serverkey.Bootstrap lands ~/Lethean/wallets/.seed,
// the boot wire calls SetSecret(serverkeySvc.AuditHMACSecret()) so the
// at-rest account_id hashing matches the canonical HKDF derivation for
// the rest of the process lifetime.
//
// Passing an empty []byte is a no-op — the existing secret stays in
// place (callers MUST NOT use SetSecret to clear the secret; that would
// regress to the random-fallback warning state without the warning).
//
// Safe for concurrent use against in-flight Record calls — the secret
// is snapshotted under secretMu.RLock at the start of every recordCommon.
//
// Usage example (boot wire):
//
//	auditSvc := audit.New(c, audit.Options{}) // random fallback secret
//	audit.SetDefault(auditSvc)
//	// ... serverkey.Bootstrap runs ...
//	auditSvc.SetSecret(serverkeySvc.AuditHMACSecret())
func (s *Service) SetSecret(secret []byte) {
	if len(secret) == 0 {
		return
	}
	s.secretMu.Lock()
	s.secret = secret
	s.secretMu.Unlock()
}

// Record satisfies Recorder. Routes auth.* events through the sync
// path; everything else takes the batched cascade path. The sync/batch
// split mirrors RFC §4.2 — auth volume is sparse + load-bearing for
// forensic timeline reconstruction, so we pay the fsync latency
// per-event; cascade volume can burst high (queue + tasks under load)
// so we coalesce.
//
// Usage example:
//
//	_ = svc.Record(audit.Event{Event: audit.EventAuthSessionIssued, ...})
func (s *Service) Record(ev Event) core.Result {
	if isAuthEvent(ev.Event) {
		return s.RecordSync(ev)
	}
	return s.RecordBatch(ev)
}

// RecordSync persists an Event with an immediate fsync. Used for
// auth.* events per RFC §4.2 — preserves Phase 1's posture where every
// auth-emit's bytes are durable on disk before the caller returns.
//
// Usage example:
//
//	_ = svc.RecordSync(audit.Event{Event: audit.EventAuthSessionIssued, ...})
func (s *Service) RecordSync(ev Event) core.Result {
	return s.recordCommon(ev, true)
}

// RecordBatch persists an Event without an immediate fsync. The bytes
// are written to disk immediately (POSIX append is atomic per-write so
// there is no read-tearing risk for downstream tailers), but the
// kernel may delay flushing to the underlying device until either:
//
//   - BatchFlushInterval elapses (default 200ms) — the rotation loop
//     calls Sync() once a flush is due.
//   - BatchFlushEventCount RecordBatch calls have piled up since the
//     last flush (default 100) — Sync() fires inline.
//
// Use for high-volume cascade events (tasks/queue/pipeline/incidents)
// where per-event fsync would block the broadcast goroutine.
//
// Usage example:
//
//	_ = svc.RecordBatch(audit.Event{Event: "queue.enqueued", ...})
func (s *Service) RecordBatch(ev Event) core.Result {
	return s.recordCommon(ev, false)
}

// recordCommon is the shared write path. Steps:
//
//  1. Stamp default fields (TS, ProcessLabel into Meta).
//  2. Hash account_id under the HMAC secret (RFC §6.4).
//  3. Validate (non-empty Event, no embedded newlines).
//  4. Redact Meta via the §2.1.1 detector.
//  5. Marshal JSON, append newline.
//  6. Acquire the writer lock, ensure the day-handle is current
//     (rotation-barrier per Cerberus #1711 MED-1), write.
//  7. Sync immediately (sync=true) OR conditionally on count
//     threshold (sync=false).
func (s *Service) recordCommon(ev Event, sync bool) core.Result {
	if s.root == "" {
		// Degraded Service — paths.Root failed at New-time. Drop with
		// counter-throttled warning per Cerberus #1711 MED-2.
		s.dropWithWarning(ev)
		return core.Ok(nil)
	}
	if ev.Event == "" {
		return core.Fail(core.NewCode(codeAuditEventInvalid,
			"Event.Event is required"))
	}
	if !isValidOutcome(ev.Outcome) {
		return core.Fail(core.NewCode(codeAuditEventInvalid,
			"Event.Outcome must be one of ok|failed|denied|error"))
	}
	if ev.TS == 0 {
		ev.TS = core.Now().UTC().Unix()
	}

	// Stamp process discriminator into Meta (RFC §6.5 lthn-mlx
	// plumbing). Re-uses any caller-supplied Meta map by copy-on-write
	// so the emit-site's Meta stays untouched.
	ev.Meta = stampProcessLabel(ev.Meta, s.opts.ProcessLabel)

	// Hash account_id under the HMAC secret BEFORE any field walk so
	// downstream paths (redaction, serialisation, query) all see the
	// hashed form on disk (RFC §6.4). Snapshot the secret under
	// secretMu so a concurrent SetSecret (boot-order swap from random
	// fallback to serverkey-derived HMAC, Mantis #1522) sees a stable
	// value for this event.
	if ev.AccountID != "" {
		s.secretMu.RLock()
		secret := s.secret
		s.secretMu.RUnlock()
		ev.AccountID = hashAccountID(secret, ev.AccountID)
	}

	// §2.1.1 secret-shape detector + RFC §6.5 newline guard run in
	// the same Meta walk pass per Cerberus #1711 LOW-2 consolidation.
	cleaned, dirty, badNewline := walkMetaForCleanAndNewline(ev.Meta)
	ev.Meta = cleaned
	if badNewline {
		return core.Fail(core.NewCode(codeAuditEventInvalid,
			"event contains a raw newline in a string field"))
	}
	if dirty {
		core.Print(core.Stderr(),
			"event=audit.record.redacted ts=%d source_event=%s\n",
			core.Now().UTC().Unix(), ev.Event)
	}
	// Top-level string fields (Event/Scope/Outcome/RequestID) still
	// need the newline check — they're not in Meta. AccountID is
	// already a hex hash by this point so it can't carry newlines.
	if core.Contains(ev.Event, "\n") || core.Contains(ev.Scope, "\n") ||
		core.Contains(ev.Outcome, "\n") || core.Contains(ev.RequestID, "\n") {
		return core.Fail(core.NewCode(codeAuditEventInvalid,
			"event contains a raw newline in a string field"))
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Ensure the active handle is for the right day + within the
	// size budget. Size-rotation runs INSIDE the locked region so
	// no concurrent writer can land bytes that would push the file
	// past the threshold while we're deciding to rotate.
	if r := s.ensureCurrentHandleLocked(ev.TS); !r.OK {
		return r
	}

	// Compute the chain digest BEFORE marshal-with-chain so the
	// digest stored in __chain reflects the canonical "without
	// chain" bytes a verifier will reconstruct. Stamp the digest
	// into Meta then marshal the final line.
	canonical, canonR := canonicalWithoutChain(ev)
	if !canonR.OK {
		return core.Fail(core.NewCode(codeAuditMarshalFailed,
			"canonical pre-marshal failed: "+canonR.Error()))
	}
	digest := computeChain(s.prevChain, canonical)
	if ev.Meta == nil {
		ev.Meta = map[string]any{chainKey: digest}
	} else {
		// Copy-on-write so caller's Meta map isn't mutated with our
		// recorder-managed key.
		out := make(map[string]any, len(ev.Meta)+1)
		for k, v := range ev.Meta {
			out[k] = v
		}
		out[chainKey] = digest
		ev.Meta = out
	}

	bR := core.JSONMarshal(ev)
	if !bR.OK {
		return core.Fail(core.NewCode(codeAuditMarshalFailed,
			"marshalling event to JSON failed"))
	}
	line, _ := bR.Value.([]byte)
	line = append(line, '\n')

	// Cross-process advisory lock (Mantis #1651 concurrent-write).
	// Held only across the write+sync critical section so the per-
	// event lock-acquire cost (~1-2ms via paths.WithFileLock
	// sentinel) is bounded.
	writeR := s.lockedWrite(line, sync)
	if !writeR.OK {
		return writeR
	}
	// Only advance prevChain AFTER the bytes are safely on disk —
	// a failed write must not poison the next event's link.
	s.prevChain = digest
	s.currentSize += int64(len(line))
	return core.Ok(nil)
}

// lockedWrite executes the actual file write inside a cross-process
// advisory lock (paths.WithFileLock). Caller MUST hold s.mu. The
// LockTimeout=0 escape valve skips the file-lock entirely — used by
// tests that want to verify in-process locking without paying the
// sentinel-file cost.
func (s *Service) lockedWrite(line []byte, sync bool) core.Result {
	doWrite := func() core.Result {
		if _, err := s.currentFile.Write(line); err != nil {
			return core.Fail(core.NewCode(codeAuditWriteFailed,
				"writing day-file failed: "+err.Error()))
		}
		if sync {
			if err := s.currentFile.Sync(); err != nil {
				return core.Fail(core.NewCode(codeAuditWriteFailed,
					"fsync day-file failed: "+err.Error()))
			}
			s.batched = 0
			return core.Ok(nil)
		}
		s.batched++
		if s.batched >= s.opts.BatchFlushEventCount {
			if err := s.currentFile.Sync(); err != nil {
				return core.Fail(core.NewCode(codeAuditWriteFailed,
					"fsync day-file failed: "+err.Error()))
			}
			s.batched = 0
		}
		return core.Ok(nil)
	}
	if s.opts.LockTimeout <= 0 {
		return doWrite()
	}
	return paths.WithFileLock(s.currentPath, s.opts.LockTimeout, doWrite)
}

// ensureCurrentHandleLocked verifies the live file handle is for the
// day implied by `tsUnix` (local time). On day-drift (or on first
// call) it closes the current handle + opens the new day's file.
// Caller MUST hold s.mu.
//
// This is the rotation-barrier per Cerberus #1711 MED-1: the
// background rotation loop ticks at 60s + can sleep through a wake-up
// (laptop closed at 23:58, opened at 00:30), so write callers MUST
// check the current day themselves rather than trusting the loop has
// done it.
func (s *Service) ensureCurrentHandleLocked(tsUnix int64) core.Result {
	wantDate := dateStem(tsUnix)
	dayDrift := s.currentFile != nil && s.currentDate != wantDate
	sizeOverflow := s.currentFile != nil &&
		s.opts.RotateAtSize > 0 &&
		s.currentSize >= s.opts.RotateAtSize
	if s.currentFile != nil && !dayDrift && !sizeOverflow {
		return core.Ok(nil)
	}
	if s.currentFile != nil {
		_ = s.currentFile.Sync()
		_ = s.currentFile.Close()
		s.currentFile = nil
		s.batched = 0
	}
	// Day-drift resets the sequence counter; size-overflow bumps it.
	if dayDrift || s.currentDate == "" {
		s.currentSeq = nextSequenceForDay(s.root, wantDate)
		s.prevChain = ""
	} else if sizeOverflow {
		s.currentSeq++
		s.prevChain = ""
	}
	s.currentSize = 0
	s.currentDate = wantDate
	path := core.PathJoin(s.root, sequencedFileName(wantDate, s.currentSeq))
	openR := core.OpenFile(path, core.O_APPEND|core.O_CREATE|core.O_WRONLY, fileMode)
	if !openR.OK {
		return core.Fail(core.NewCode(codeAuditOpenFailed,
			"opening day-file failed: "+openR.Value.(error).Error()))
	}
	f, _ := openR.Value.(*core.OSFile)
	s.currentFile = f
	s.currentPath = path
	// If we just re-opened a previously written file (process restart
	// mid-day, or rotation gap), seed currentSize + prevChain from the
	// existing content so the new event lands at the correct byte
	// position AND continues the existing chain.
	if st := core.Stat(path); st.OK {
		if fi, ok := st.Value.(core.FsFileInfo); ok {
			s.currentSize = fi.Size()
		}
	}
	if lastR, _ := readLastChainFromFile(path); lastR != "" {
		s.prevChain = lastR
	}
	return core.Ok(nil)
}

// sequencedFileName returns the basename for the given day stem +
// sequence number. Layout:
//
//   - seq == 0 → "<YYYY-MM-DD>.log" (the legacy unsequenced shape;
//     keeps every consumer that resolves by date+suffix compatible
//     with the new size-rotation code path).
//   - seq >= 1 → "<YYYY-MM-DD>.NNN.log" (rotated sibling files,
//     created when the active handle crosses RotateAtSize). NNN is
//     zero-padded to three digits so a lexical sort matches arrival
//     order.
//
// Mantis #1651 — keeping seq=0 unsequenced means existing tests +
// the rotation.ResolveDayFile single-file resolver keep working for
// every install whose audit volume stays under the rotation
// threshold (the common case for desktop auth traffic).
func sequencedFileName(dateStem string, seq int) string {
	if seq <= 0 {
		return dateStem + rotation.LogSuffix
	}
	return dateStem + "." + threeDigit(seq) + rotation.LogSuffix
}

// threeDigit returns the zero-padded three-digit decimal for n in
// [0, 999]. Out-of-range inputs return the natural decimal — the
// caller's rotation loop bounds this; we don't bounds-check here so
// a four-digit-rotation-day stays grep-able rather than silently
// rolling back to "000".
func threeDigit(n int) string {
	if n < 0 {
		return core.Itoa(n)
	}
	if n < 10 {
		return "00" + core.Itoa(n)
	}
	if n < 100 {
		return "0" + core.Itoa(n)
	}
	return core.Itoa(n)
}

// nextSequenceForDay scans root for sequenced files matching the
// given dateStem and returns the next available sequence number.
// Used on day-drift (fresh day starts at 0 = unsequenced .log) and
// on Service.New (process restart in mid-day must continue the
// existing sequence so concurrent re-opens don't append into
// possibly-partial-but-untrusted prior content). Returns 0 when no
// existing file matches.
func nextSequenceForDay(root, dateStem string) int {
	entriesR := core.ReadDir(core.DirFS(root), ".")
	if !entriesR.OK {
		return 0
	}
	entries, _ := entriesR.Value.([]core.FsDirEntry)
	highest := -1
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		// Accept unsequenced "<date>.log" / "<date>.log.gz" / candidate
		// as seq=0, and sequenced "<date>.NNN.{log,log.gz,candidate}"
		// as seq=NNN.
		if name == dateStem+rotation.LogSuffix ||
			name == dateStem+rotation.CompressedSuffix ||
			name == dateStem+rotation.CandidateSuffix {
			if highest < 0 {
				highest = 0
			}
			continue
		}
		prefix := dateStem + "."
		if !core.HasPrefix(name, prefix) {
			continue
		}
		stem := name[len(prefix):]
		dot := -1
		for i := 0; i < len(stem); i++ {
			if stem[i] == '.' {
				dot = i
				break
			}
		}
		if dot < 0 {
			continue
		}
		numStr := stem[:dot]
		nR := core.Atoi(numStr)
		if !nR.OK {
			continue
		}
		n, _ := nR.Value.(int)
		if n > highest {
			highest = n
		}
	}
	// Return highest+1; first file of a fresh day is 0 (unsequenced).
	return highest + 1
}

// Register is the canonical core.WithName-compatible factory. Wired
// in cmd/lthn/app.go so the service is reachable via
// core.ServiceFor[*Service](c, "audit") — pkg/server's RoutesProvider
// auto-discovery picks up the /v1/audit/events surface from
// RouteGroups().
//
// Constructed with Options{} defaults — the boot path that needs to
// pass an HMAC secret or non-default Root SHOULD use audit.New
// explicitly and call audit.SetDefault(svc) instead of Register, then
// register the Service under "audit" via c.RegisterService.
//
// Usage example:
//
//	core.WithName("audit", audit.Register)
func Register(c *core.Core) core.Result {
	svc := New(c, Options{})
	SetDefault(svc)
	return core.Ok(svc)
}

// ServiceName labels the binding namespace. Wails bindings would land
// at frontend/bindings/.../audit/ if any were added (no Wails surface
// today — the consumer is the REST endpoint + frontend fetch).
func (s *Service) ServiceName() string { return "Audit" }

// ServiceStartup is the Wails3 lifecycle hook. No-op — rotation goroutine
// spawned in New() so all moving parts are already live by the time
// the Core dispatches Startup hooks.
func (s *Service) ServiceStartup(_ core.Context, _ any) core.Result {
	return core.Ok(nil)
}

// ServiceShutdown flushes + closes the live day-file handle via
// Close. Boot wiring SHOULD let the canonical Core shutdown path
// drive this; manual Close is reserved for tests / sub-binary
// teardown (lthn-mlx exit).
func (s *Service) ServiceShutdown() core.Result {
	return s.Close()
}

// Close synchronously shuts the rotation goroutine, fsyncs the live
// handle, and closes it. Safe to call multiple times — second + later
// calls are no-ops.
//
// Boot wiring SHOULD call Close from a ServiceShutdown hook so the
// final cascade-batched events flush before the process exits.
//
// Usage example:
//
//	defer svc.Close()
func (s *Service) Close() core.Result {
	if s.stop != nil {
		// Closing twice would panic — guard with a select on the
		// already-closed channel.
		select {
		case <-s.stop:
			// already closed; nothing to do
		default:
			close(s.stop)
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.currentFile != nil {
		_ = s.currentFile.Sync()
		_ = s.currentFile.Close()
		s.currentFile = nil
	}
	return core.Ok(nil)
}

// rotationLoop is the background rotation + retention worker. Wakes
// every Options.RotationCheckInterval (default 60s) and:
//
//  1. Force-flushes the batched-write counter if BatchFlushInterval
//     has passed since the last write (so a low-rate cascade stream
//     still hits disk within 200ms).
//  2. Swaps the handle on day-boundary crossing (the rotation barrier
//     in ensureCurrentHandleLocked also covers this; the loop catches
//     the case where no writes happened across midnight).
//  3. Compresses uncompressed `.log` files older than
//     CompressOlderThan days into `.log.gz`.
//  4. Renames `.log.gz` files older than KeepDays into
//     `.log.gz.candidate` (the persistent "logged-once" retention
//     marker per RFC §4.2).
//
// Exits cleanly on s.stop closure OR c.IsShutdown returning true.
func (s *Service) rotationLoop() {
	if s.root == "" {
		// Degraded Service — nothing to rotate.
		return
	}
	tick := s.opts.RotationCheckInterval
	if tick <= 0 {
		tick = defaultRotationCheckInterval
	}
	for {
		select {
		case <-s.stop:
			return
		case <-core.After(tick):
		}
		if s.core != nil && s.core.IsShutdown() {
			return
		}
		s.tickFlush()
		s.tickRotateAndRetain()
	}
}

// tickFlush forces an fsync on the live handle if there are
// unflushed batched writes. Mirrors the Cerberus #1711 ADD-CRIT-1
// 200ms-or-100-events flush contract from RFC §4.2.
func (s *Service) tickFlush() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.currentFile == nil || s.batched == 0 {
		return
	}
	_ = s.currentFile.Sync()
	s.batched = 0
}

// tickRotateAndRetain scans the audit root for compressible /
// retention-overdue files. Throttled to one operation per tick per
// the calm-tenant ethos (RFC §4.2 — "throttled (one compression per
// check tick)").
func (s *Service) tickRotateAndRetain() {
	entriesR := core.ReadDir(core.DirFS(s.root), ".")
	if !entriesR.OK {
		return
	}
	entries, _ := entriesR.Value.([]core.FsDirEntry)
	now := core.Now().UTC()
	// One compression OR one rename per tick. Sweep first for
	// retention candidates (cheap rename), then for compressible
	// files (more expensive — actually streams + gzips).
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !core.HasSuffix(name, rotation.CompressedSuffix) {
			continue
		}
		if core.HasSuffix(name, rotation.CandidateSuffix) {
			continue
		}
		ageDays := fileAgeDays(name, now)
		if s.opts.KeepDays > 0 && ageDays >= s.opts.KeepDays {
			fullPath := core.PathJoin(s.root, name)
			if _, r := rotation.Candidate(fullPath); r.OK {
				core.Print(core.Stderr(),
					"event=audit.retention.candidate file=%s age_days=%d\n",
					name, ageDays)
				return // throttled — one operation per tick
			}
		}
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !core.HasSuffix(name, rotation.LogSuffix) {
			continue
		}
		ageDays := fileAgeDays(name, now)
		if s.opts.CompressOlderThan <= 0 || ageDays < s.opts.CompressOlderThan {
			continue
		}
		fullPath := core.PathJoin(s.root, name)
		// Don't compress the current day's file — even if today
		// somehow exceeds the threshold (shouldn't happen at
		// default=7d but a misconfiguration to 0 would race the
		// active handle). currentDate guard is taken under s.mu.
		s.mu.Lock()
		isCurrent := s.currentDate != "" &&
			core.HasPrefix(name, s.currentDate)
		s.mu.Unlock()
		if isCurrent {
			continue
		}
		if _, r := rotation.Compress(fullPath, fileMode); r.OK {
			core.Print(core.Stderr(),
				"event=audit.rotation.compressed file=%s\n", name)
			return
		}
	}
}

// dropWithWarning logs the drop + bumps the noop counter. Re-emits a
// "still dropping" warning every NoopDropWarningEvery hits so the
// operator sees a recurring signal that audit is degraded rather than
// a single startup line that scrolls off the buffer.
func (s *Service) dropWithWarning(ev Event) {
	s.mu.Lock()
	s.noopCounter++
	c := s.noopCounter
	s.mu.Unlock()
	threshold := int64(s.opts.NoopDropWarningEvery)
	if threshold <= 0 {
		return
	}
	if c%threshold == 0 {
		core.Print(core.Stderr(),
			"audit: %d events dropped since recorder degradation (last event=%s)\n",
			c, ev.Event)
	}
}

// fileAgeDays returns approx age in whole days inferred from the file
// name's embedded YYYY-MM-DD stem. Filenames that don't match the
// expected prefix return 0 so they aren't mis-classified as ancient
// + retention-flagged in error.
func fileAgeDays(name string, now core.Time) int {
	if len(name) < 10 {
		return 0
	}
	stem := name[:10]
	t, ok := parseDateStem(stem)
	if !ok {
		return 0
	}
	delta := now.Sub(t)
	days := int(delta.Hours() / 24)
	if days < 0 {
		return 0
	}
	return days
}

// parseDateStem returns the UTC midnight of YYYY-MM-DD. Returns (zero,
// false) for any non-conforming input.
func parseDateStem(stem string) (core.Time, bool) {
	if len(stem) != 10 || stem[4] != '-' || stem[7] != '-' {
		return core.Time{}, false
	}
	yR := core.Atoi(stem[0:4])
	mR := core.Atoi(stem[5:7])
	dR := core.Atoi(stem[8:10])
	if !yR.OK || !mR.OK || !dR.OK {
		return core.Time{}, false
	}
	y, _ := yR.Value.(int)
	mo, _ := mR.Value.(int)
	d, _ := dR.Value.(int)
	// time package is stdlib; core.Time is a type-alias for time.Time
	// so this stays compatible with the rest of the audit package's
	// core.Time-typed signatures. Banned-import list excludes "time"
	// because it has no security-CVE rationale to wrap.
	return time.Date(y, time.Month(mo), d, 0, 0, 0, 0, time.UTC), true
}

// dateStem returns the YYYY-MM-DD stem for a unix-seconds timestamp.
// Used to derive both the active day-file basename and the cursor
// `file_date` value (RFC §4.3).
func dateStem(unixSec int64) string {
	t := core.UnixTime(unixSec).UTC()
	y, mo, d := t.Year(), int(t.Month()), t.Day()
	return core.Itoa(y) + "-" + zeroPad2(mo) + "-" + zeroPad2(d)
}

// stampProcessLabel returns a Meta map with the __process key set to
// label. Copy-on-write — only allocates a new map when label is
// non-empty AND the value differs from what's already there.
func stampProcessLabel(meta map[string]any, label string) map[string]any {
	if label == "" {
		return meta
	}
	if meta == nil {
		return map[string]any{metaKeyProcess: label}
	}
	if existing, ok := meta[metaKeyProcess].(string); ok && existing == label {
		return meta
	}
	out := make(map[string]any, len(meta)+1)
	for k, v := range meta {
		out[k] = v
	}
	out[metaKeyProcess] = label
	return out
}

// isAuthEvent is the prefix-test that routes events into RecordSync
// vs RecordBatch. RFC §4.2 — all auth.* events MUST be sync; cascade
// events (tasks/queue/pipeline/incidents) batch.
func isAuthEvent(name string) bool {
	return core.HasPrefix(name, "auth.")
}

// isValidOutcome enforces the closed-set Outcome enum from RFC §2
// per Cerberus #1711 LOW-1. Empty outcome rejects.
func isValidOutcome(o string) bool {
	switch o {
	case OutcomeOK, OutcomeFailed, OutcomeDenied, OutcomeError:
		return true
	}
	return false
}
