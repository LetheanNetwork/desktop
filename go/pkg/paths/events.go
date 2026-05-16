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
// Usage example:
//
//	paths.SubscribeLockEvents(func(ev paths.LockEvent) {
//	    log.Printf("paths event: %+v", ev)
//	})
type LockEvent struct {
	Kind     string    `json:"kind"`
	PathHash string    `json:"path_hash"`
	Caller   string    `json:"caller,omitempty"`
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
// Usage example:
//
//	mode := paths.AuditModeForPath(fpath)
//	if mode == paths.AuditModeSync { ... }
func AuditModeForPath(p string) AuditMode {
	rel := workspaceRel(p)
	if core.HasPrefix(rel, "wallets/") {
		return AuditModeSync
	}
	if core.HasPrefix(rel, "account/") {
		return AuditModeSync
	}
	if rel == "office/mail/_accounts.enc" {
		return AuditModeSync
	}
	return AuditModeBatch
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

// emitLockEvent fans an event to in-process subscribers and the
// audit recorder. Called by emitWriteSucceeded / emitVersionStale
// (defined in init() below — overrides the no-op stubs in
// atomic_write.go).
func emitLockEvent(ev LockEvent) {
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
}
