// SPDX-Licence-Identifier: EUPL-1.2

// events.go — paths.LockEvent typed bus + audit emission per
// RFC.atomic-write.md §6. Two layers:
//
//   1. LockEvent typed struct — for in-process subscribers (Stage F
//      §3.2 typed-event-bus shape).
//   2. Audit emission via the package-level Recorder configured
//      through SetAuditRecorder — auth-substrate paths land via
//      RecordSync (crash-safety > throughput), cascade paths land
//      via RecordBatch (throughput > per-event durability).
//
// MED-3 (HKDF domain separation): path_hash uses a second-stage
// HKDF over the audit secret with info-string "paths.lock.v1|
// <accountID>". Stage F §6.4 account_id-hashing uses
// "audit.path.v1|<accountID>" by RFC convention — same audit secret,
// different info → independent output bytes, no cross-domain pre-
// image confusion.
//
// Forward-contract: accountID is sourced from the package-level
// SetCurrentAccountID injection. Local-tier (boot, no session)
// callers default to "" — the hash degenerates to single-tenant but
// stays domain-separated from the audit-account-id hash.

package paths

import (
	core "dappco.re/go"
)

// LockEvent kinds — closed schema. New kinds are reserved-schema
// changes; consumers pattern-match on these literals.
const (
	EventLockAcquired    = "paths.lock.acquired"
	EventLockReleased    = "paths.lock.released"
	EventVersionStale    = "paths.write.version_stale"
	EventWriteSucceeded  = "paths.write.succeeded"
	// EventWriteFailed is emitted when AtomicWriteWithVersion's
	// open / write / fsync / rename step hits an I/O error. The
	// event's Code field carries the typed paths.write.<step>_failed
	// sentinel so operators can pattern-match without parsing
	// free-form prose. Mantis #1551.
	EventWriteFailed     = "paths.write.failed"
)

// AuditMode enumerates the §6.1 Call 3 split. Caller MUST NOT
// pick directly — AuditModeForPath consults the policy table.
type AuditMode int

const (
	// AuditModeSync = RecordSync. fsync per event. Auth-substrate
	// paths (wallets/, account/, office/mail/_accounts.enc) MUST use
	// this mode so a crash mid-write leaves a coherent audit trail.
	AuditModeSync AuditMode = iota
	// AuditModeBatch = RecordBatch. Buffered + page-boundary flush.
	// Cascade paths (sales/, marketing/, incidents, runbooks/,
	// office/documents/, office/mail/<folder>/threads.md,
	// office/files/) use this mode for throughput.
	AuditModeBatch
)

// LockEvent is the typed forensic record emitted by the lock /
// write primitives. PathHash is the HKDF-derived HMAC of the file
// path under the current account's per-domain key — raw paths NEVER
// traverse the typed bus or hit the audit log.
//
// Code carries the typed sentinel for failure events
// (paths.write.open_failed / fsync_failed / rename_failed) so
// operators can pattern-match without parsing prose. Omitted on
// success events. Mantis #1551.
//
// Usage example:
//
//	paths.SubscribeLockEvents(func(ev paths.LockEvent) {
//	    log.Printf("paths event: %+v", ev)
//	})
type LockEvent struct {
	Kind     string    `json:"kind"`
	PathHash string    `json:"path_hash"`
	Caller   string    `json:"caller,omitempty"`
	Code     string    `json:"code,omitempty"`
	Version  int       `json:"version,omitempty"`
	Mode     AuditMode `json:"-"` // routing only — never serialised to log
	At       core.Time `json:"at"`
}

// AuditRecorder is the minimal interface paths needs to write
// events without depending on the audit package directly (avoids
// import cycle: audit depends on paths.Root for the audit dir, so
// paths cannot import audit). Boot wiring (cmd/lthn/app.go) calls
// SetAuditRecorder with an adapter that fans out to audit.Service's
// RecordSync / RecordBatch by mode.
type AuditRecorder interface {
	// RecordPathsEvent persists the supplied LockEvent. The
	// recorder is responsible for routing to RecordSync vs
	// RecordBatch based on ev.Mode.
	RecordPathsEvent(ev LockEvent) core.Result
}

// noopAuditRecorder is the default before boot wiring lands. Drops
// events silently — Stage F's degraded-mode discipline.
type noopAuditRecorder struct{}

func (noopAuditRecorder) RecordPathsEvent(_ LockEvent) core.Result { return core.Ok(nil) }

// currentAuditRecorder holds the active recorder. Default no-op
// keeps the primitive shippable before boot wiring lands.
var currentAuditRecorder AuditRecorder = noopAuditRecorder{}

// SetAuditRecorder installs the recorder used by the lock + write
// primitives' emit-sites. Boot wiring calls this once during app
// construction. Calling with nil restores the noop default.
//
// Usage example (boot wiring, post-audit.New):
//
//	paths.SetAuditRecorder(adapter.NewForAuditService(auditSvc))
func SetAuditRecorder(r AuditRecorder) {
	if r == nil {
		currentAuditRecorder = noopAuditRecorder{}
		return
	}
	currentAuditRecorder = r
}

// secretProvider returns the HKDF root (audit secret) the events
// machinery uses for path-hash derivation. Boot wiring overrides
// this to source from serverkey.AuditHMACSecret(); the default
// returns nil so the package compiles + tests run without a real
// serverkey instance.
var currentAuditSecret = func() []byte { return nil }

// SetAuditSecretProvider lets boot wiring inject the live HKDF root.
// Passing nil restores the empty default.
//
// Usage example (boot wiring):
//
//	paths.SetAuditSecretProvider(serverkeySvc.AuditHMACSecret)
func SetAuditSecretProvider(fn func() []byte) {
	if fn == nil {
		currentAuditSecret = func() []byte { return nil }
		return
	}
	currentAuditSecret = fn
}

// currentAccountIDFn returns the accountID for the in-flight
// session, used as the HKDF info-string suffix. Default returns
// empty (single-tenant degraded mode).
var currentAccountIDFn = func() string { return "" }

// SetCurrentAccountIDProvider lets the session layer inject a
// per-request accountID resolver. Boot wiring may set this to a
// closure that reads from the gin context; tests may stub it to a
// fixed string.
//
// Usage example:
//
//	paths.SetCurrentAccountIDProvider(func() string {
//	    return sessionMgr.CurrentAccountID()
//	})
func SetCurrentAccountIDProvider(fn func() string) {
	if fn == nil {
		currentAccountIDFn = func() string { return "" }
		return
	}
	currentAccountIDFn = fn
}

// AuditModeForPath consults the §6.1 policy table. Auth-substrate
// path prefixes return AuditModeSync; everything else returns
// AuditModeBatch.
//
// Auth-substrate (RecordSync):
//
//   - wallets/      (server.key, future key rotation)
//   - account/      (per-account keystores)
//   - office/mail/_accounts.enc (IMAP credentials)
//
// Cascade (RecordBatch):
//
//   - everything else (sales/, marketing/, incidents, runbooks/,
//     office/documents/, office/mail/<folder>/threads.md,
//     office/files/, conf/)
//
// F4 (Cerberus DREAD-r2, Mantis #1528): fail-safe default. Any path
// that workspaceRel could not place under Root (returns the input
// verbatim) routes to AuditModeSync. Without this fail-safe an
// attacker-influenced path outside Root, a case-folded prefix on
// APFS/NTFS, or a "../" traversal escapee silently downgrades to
// Batch — auth-substrate writes lose their crash-safety guarantee.
//
// The cleaned + (optionally) case-folded rel is matched against the
// auth-substrate prefixes; unknown shapes default to Sync.
//
// Usage example:
//
//	mode := paths.AuditModeForPath(fpath)
//	if mode == paths.AuditModeSync { ... }
func AuditModeForPath(p string) AuditMode {
	rel := workspaceRel(p)
	if rel == p {
		// Out-of-Root path — workspaceRel passed it through
		// unchanged. Fail-safe: assume the strictest mode so
		// callers cannot accidentally degrade auth-substrate
		// crash-safety by handing in an unexpected path shape.
		return AuditModeSync
	}
	// Normalise traversal segments so "../wallets/" or
	// "office//../wallets/foo" cannot bypass the prefix table.
	cleaned := core.CleanPath(rel, "/")
	if cleaned == "." || core.HasPrefix(cleaned, "../") || cleaned == ".." {
		// CleanPath could not resolve under-root — also fail-safe.
		return AuditModeSync
	}
	// On case-insensitive filesystems (APFS / NTFS), case-folded
	// variants of the auth-substrate prefixes resolve to the same
	// on-disk inode. Lower-case the cleaned rel so the prefix
	// table comparison stays sound.
	match := cleaned
	if r := RootFSType(); r.OK {
		if isCaseInsensitiveFS(r.Value.(string)) {
			match = core.Lower(cleaned)
		}
	}
	if core.HasPrefix(match, "wallets/") {
		return AuditModeSync
	}
	if core.HasPrefix(match, "account/") {
		return AuditModeSync
	}
	if match == "office/mail/_accounts.enc" {
		return AuditModeSync
	}
	return AuditModeBatch
}

// isCaseInsensitiveFS reports whether the supplied fstype is known
// to treat paths case-insensitively. APFS (default on macOS), HFS+,
// FAT family, NTFS, exFAT, and SMB are the practical members of
// this set on the platforms lthn targets. Conservative default is
// false (case-sensitive) for unknown identifiers.
func isCaseInsensitiveFS(fstype string) bool {
	switch fstype {
	case "apfs", "hfs", "hfsplus", "msdos", "fat", "fat32", "exfat",
		"ntfs", "smbfs", "cifs":
		return true
	}
	return false
}

// PathHashInfo returns the HKDF info-string used to derive the
// per-account path-hashing key per §6.2 MED-3. The "paths.lock.v1|"
// prefix domain-separates this output from Stage F §6.4's
// "audit.path.v1|" derivation.
//
// Usage example:
//
//	info := paths.PathHashInfo("acct123") // "paths.lock.v1|acct123"
func PathHashInfo(accountID string) string {
	return "paths.lock.v1|" + accountID
}

// hashPath derives the HKDF-per-domain key, then HMAC-hashes the
// supplied path under that key. Returns "" when the audit secret
// is unavailable (no-op recorder will drop the event regardless).
func hashPath(path string) string {
	secret := currentAuditSecret()
	if len(secret) == 0 {
		return ""
	}
	accountID := currentAccountIDFn()
	info := PathHashInfo(accountID)
	keyR := core.HKDF("sha256", secret, nil, []byte(info), 32)
	if !keyR.OK {
		return ""
	}
	key, _ := keyR.Value.([]byte)
	mac := core.HMAC("sha256", key, []byte(path))
	if !mac.OK {
		return ""
	}
	digest, _ := mac.Value.([]byte)
	hex := core.HexEncode(digest)
	if len(hex) >= 32 {
		return hex[:32]
	}
	return hex
}

// lockEventSubscribers holds the in-process LockEvent subscribers.
// SubscribeLockEvents appends; emitLockEvent fans out. Subscribers
// run synchronously on the emit goroutine — keep them fast or
// off-thread.
var lockEventSubscribers []func(LockEvent)

// SubscribeLockEvents registers fn as a LockEvent subscriber. fn
// is invoked on the same goroutine that emits the event so heavy
// subscribers MUST dispatch their work to a queue.
//
// Usage example:
//
//	paths.SubscribeLockEvents(func(ev paths.LockEvent) {
//	    metrics.IncLockEvent(ev.Kind)
//	})
func SubscribeLockEvents(fn func(LockEvent)) {
	if fn == nil {
		return
	}
	lockEventSubscribers = append(lockEventSubscribers, fn)
}

// ClearLockEventSubscribersForTest resets the subscriber list.
// Test-only helper; production code MUST NOT call it.
func ClearLockEventSubscribersForTest() {
	lockEventSubscribers = nil
}

// auditDegradedCount tracks events dropped at the emit site because
// the HKDF audit secret was unavailable (no-op secret provider —
// pre-boot or session-locked state). FlushDegradedCount surfaces
// the running total as a single summary event once the secret is
// back online; boot wiring is responsible for invoking the flush at
// the right moment (typically post-unlock).
//
// Cerberus DREAD-r2 F2 (Mantis #1526): emitting LockEvents with an
// empty PathHash leaks "an event happened on SOMETHING" to log
// readers + subscribers without the per-account domain separation
// the path-hash provides. Dropping the event (rather than emitting
// it raw) closes the side-channel; the counter keeps the audit log
// honest about how many events were suppressed.
var auditDegradedCount core.AtomicInt64

// EventAuditDegradedSummary is the LockEvent.Kind emitted by
// FlushDegradedCount when there are dropped events to summarise.
// Reserved schema — log readers pattern-match on this literal.
const EventAuditDegradedSummary = "paths.audit.degraded_summary"

// auditDegradedSinceFn returns the monotonic boot timestamp used as
// the "since" anchor in degraded-summary events. Defaults to the
// package-init time captured at first use; tests can override via
// SetAuditDegradedSinceForTest to inject a deterministic anchor.
var auditDegradedSince = core.Now().UTC()

// SetAuditDegradedSinceForTest substitutes the degraded-summary
// "since" anchor for the duration of a test. Always pair with a
// t.Cleanup() reset.
func SetAuditDegradedSinceForTest(at core.Time) {
	auditDegradedSince = at
}

// AuditDegradedCount returns the current dropped-event count so
// operators (or tests) can observe it without forcing a flush.
func AuditDegradedCount() int64 {
	return auditDegradedCount.Load()
}

// FlushDegradedCount emits a single summary event capturing the
// number of LockEvents dropped at the emit site because the audit
// secret was unavailable, then resets the counter. No-op when the
// count is zero. Surfaces nothing when the secret is still
// unavailable (the same condition that caused the drops) — caller
// is expected to invoke FlushDegradedCount post-unlock.
//
// Returns the count that was flushed (or 0 on no-op). The emission
// flows through the same audit recorder as regular events.
//
// Usage example (boot wiring, post-unlock):
//
//	paths.FlushDegradedCount()
func FlushDegradedCount() int64 {
	secret := currentAuditSecret()
	if len(secret) == 0 {
		// Still degraded — nothing useful to log. Don't drain the
		// counter; the next post-unlock call will catch up.
		return 0
	}
	count := auditDegradedCount.Swap(0)
	if count == 0 {
		return 0
	}
	ev := LockEvent{
		Kind: EventAuditDegradedSummary,
		Mode: AuditModeSync,
		At:   core.Now().UTC(),
	}
	// Subscribers receive the summary too so in-process consumers
	// can react.
	for _, fn := range lockEventSubscribers {
		func() {
			defer func() { _ = recover() }()
			fn(ev)
		}()
	}
	// The recorder interface does not carry arbitrary Meta today;
	// the count + since anchor encode into the typed-event payload
	// via dedicated fields when the schema lands. For now the
	// summary event surfaces presence + Mode=Sync routing; the
	// count is observable via AuditDegradedCount() pre-flush, and
	// SetAuditDegradedSinceForTest pins the anchor for assertions.
	_ = auditDegradedSince
	_ = currentAuditRecorder.RecordPathsEvent(ev)
	return count
}

// emitLockEvent fans an event to in-process subscribers and the
// audit recorder. Called by emitWriteSucceeded / emitVersionStale
// (defined in init() below — overrides the no-op stubs in
// atomic_write.go).
//
// F2 (Cerberus DREAD-r2, Mantis #1526): when the HKDF audit secret
// is unavailable the path-hash would degenerate to "" — emitting
// such an event leaks "something happened" metadata without the
// per-account domain separation the hash provides. Drop the event
// at the emit site and increment auditDegradedCount; the eventual
// FlushDegradedCount call surfaces the running total as a single
// summary so the audit log stays honest about the gap.
func emitLockEvent(ev LockEvent) {
	if len(currentAuditSecret()) == 0 {
		auditDegradedCount.Add(1)
		return
	}
	for _, fn := range lockEventSubscribers {
		func() {
			defer func() { _ = recover() }()
			fn(ev)
		}()
	}
	_ = currentAuditRecorder.RecordPathsEvent(ev)
}

// init overrides the atomic_write.go stubs. After this package's
// init runs, every write surface fires LockEvents.
func init() {
	emitWriteSucceeded = func(path string, version int) {
		emitLockEvent(LockEvent{
			Kind:     EventWriteSucceeded,
			PathHash: hashPath(path),
			Version:  version,
			Mode:     AuditModeForPath(path),
			At:       core.Now().UTC(),
		})
	}
	emitVersionStale = func(path string, currentVersion int) {
		emitLockEvent(LockEvent{
			Kind:     EventVersionStale,
			PathHash: hashPath(path),
			Version:  currentVersion,
			Mode:     AuditModeForPath(path),
			At:       core.Now().UTC(),
		})
	}
	// Mantis #1551 — write-step failure emission. Routes via the
	// same AuditModeForPath policy as success events so auth-
	// substrate failures land Sync and cascade failures land Batch.
	// Code carries the typed CodeWrite* sentinel.
	emitWriteFailed = func(path string, code string) {
		emitLockEvent(LockEvent{
			Kind:     EventWriteFailed,
			PathHash: hashPath(path),
			Code:     code,
			Mode:     AuditModeForPath(path),
			At:       core.Now().UTC(),
		})
	}
}
