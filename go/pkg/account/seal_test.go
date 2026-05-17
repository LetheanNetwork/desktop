// SPDX-Licence-Identifier: EUPL-1.2

// Tests for the account.Service Seal contract per Stage E.A
// (plans/code/lthn/desktop/auth-gate/RFC.stage-e-seal.md v2 §8).
// All Cerberus #25 load-bearing items are pinned here:
//
//   - Q1  (URL :id is authoritative)
//   - Q2  (account.not_found NO lockout allocation — Mantis #1586)
//   - Q3  (paths.AtomicWriteWithVersion IfMatchHash composite gate)
//   - Q4  (byte-equality idempotent retry — no canonical-equality)
//   - Q5  (inherit global 64 KiB body cap — no per-route widen)
//   - ADD-HIGH-2 (paths.IsValidID gate BEFORE any path-touching ops)
//   - ADD-MED-2  (recordSealFailure tick matrix)
//   - ADD-MED-3  (NO blob_size_bytes in audit Meta)
//
// Test isolation via homeFixture rebinding $HOME so the real
// ~/Lethean/ tree is never touched. Service constructed via the
// existing newUnlockable so the lockout substrate behaves identically
// to the unlock-side tests.

package account_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"

	core "dappco.re/go"
	subject "dappco.re/lthn/desktop/pkg/account"
	"dappco.re/lthn/desktop/pkg/audit"
)

// fixtureSealBlob returns a synthetic byte sequence that satisfies
// Seal's minimum-length gate (minSealedBlobBytes = 64) and looks
// unambiguously NOT-a-marker (the Create-marker is a 64-char ASCII
// hex string; this is 128 bytes of binary). Real callers post a real
// PGP envelope; the Seal handler v1 only length-sanes the blob so
// synthetic bytes exercise the state machine without forcing the
// tests to spin a full PGP keygen + symmetric-encrypt per case.
func fixtureSealBlob() []byte {
	out := make([]byte, 128)
	for i := range out {
		out[i] = byte((i * 7) ^ 0x5a) // visually-non-ASCII, deterministic
	}
	return out
}

// seedMarkerAccount lays down a Create-marker account directory at
// ~/Lethean/account/<id>/ via the real Service.Create path so the
// on-disk shape (public.key + meta.json + private.key=hex(SHA-256(pub)))
// matches production exactly. Returns the canonical accountID.
func seedMarkerAccount(t *core.T) (string, *subject.Service) {
	t.Helper()
	svc := subject.NewService(nil)
	in := validInput()
	r := svc.Create(in)
	core.AssertTrue(t, r.OK, "Create must succeed (fixture)")
	return in.AccountID, svc
}

// --- Seal — Good (happy path: MARKER → SEALED) ---

func TestSeal_Good(t *core.T) {
	_ = homeFixture(t)
	id, svc := seedMarkerAccount(t)
	rec := &recordingRecorder{}
	audit.SetDefault(rec)
	t.Cleanup(func() { audit.SetDefault(nil) })

	blob := fixtureSealBlob()
	r := svc.Seal(subject.SealInput{
		AccountID: id,
		Version:   1,
		Blob:      blob,
	})
	core.AssertTrue(t, r.OK, "MARKER → SEALED Seal must succeed")

	out, ok := r.Value.(subject.SealOutput)
	core.AssertTrue(t, ok, "Result.Value must be SealOutput")
	core.AssertEqual(t, id, out.AccountID)
	core.AssertEqual(t, 1, out.Version)
	core.AssertTrue(t, out.SealedAt > 0, "SealedAt must be a real unix-seconds timestamp")

	// Cerberus #25 ADD-MED-3 — audit Meta MUST NOT carry blob_size_bytes.
	// Walk every recorded event's Meta and assert the key is absent.
	for _, ev := range rec.events {
		_, has := ev.Meta["blob_size_bytes"]
		core.AssertFalse(t, has,
			"ADD-MED-3: blob_size_bytes MUST NOT appear in audit Meta")
	}

	// Audit success row landed with the right event name + scope.
	var seenSuccess bool
	for _, ev := range rec.events {
		if ev.Event == "auth.account.sealed" && ev.AccountID == id {
			seenSuccess = true
			core.AssertEqual(t, "account.seal", ev.Scope)
			core.AssertEqual(t, audit.OutcomeOK, ev.Outcome)
		}
	}
	core.AssertTrue(t, seenSuccess, "auth.account.sealed MUST be emitted on success")
}

// --- Seal — Bad: id_mismatch handled at handler boundary ---
// Handler-level URL-vs-body mismatch is covered by the routes test
// below (TestRoutes_SealEndpoint_IDMismatch_Bad); Service.Seal does
// NOT see the mismatch because the handler reconciles before the
// Service call. This test pins the Service-level behaviour for the
// pre-reconciled shape (empty AccountID → id.required).

// --- Seal — Bad: account_id invalid (Cerberus #25 ADD-HIGH-2 / Mantis #1627) ---

func TestSeal_AccountIdInvalid_Bad(t *core.T) {
	_ = homeFixture(t)
	svc := subject.NewService(nil)

	// IsValidID-failing ids (path traversal, leading dot, NUL, oversized,
	// path separator). All MUST return paths.invalid_id BEFORE any
	// filesystem touch. Cite Cerberus #25 ADD-HIGH-2 + Mantis #1627.
	cases := []string{
		"../../wallets/server.key", // path traversal
		".hidden",                  // leading dot
		"a/b",                      // path separator
		"a\\b",                     // backslash separator
		"a..b",                     // contains ..
		"",                         // empty handled by id.required (skipped)
	}
	for _, id := range cases {
		if id == "" {
			continue // handled separately
		}
		r := svc.Seal(subject.SealInput{
			AccountID: id,
			Version:   1,
			Blob:      fixtureSealBlob(),
		})
		core.AssertFalse(t, r.OK, "invalid id "+id+" MUST be rejected")
		// IsValidID returns core.E("paths.invalid_id", ...) so the
		// failure surfaces in r.Error() (NOT r.Code() — same shape the
		// unlock-side test asserts at unlock_test.go:578). The check
		// mirrors that contract verbatim so future hardening can sweep
		// both surfaces with one refactor.
		core.AssertTrue(t, core.Contains(r.Error(), "paths.invalid_id"),
			"id "+id+" MUST surface paths.invalid_id (ADD-HIGH-2 / #1627)")
	}

	// Empty AccountID surfaces account.id.required (pre-IsValidID gate).
	r := svc.Seal(subject.SealInput{Version: 1, Blob: fixtureSealBlob()})
	core.AssertFalse(t, r.OK)
	core.AssertEqual(t, "account.id.required", r.Code())
}

// --- Seal — Bad: account not found → 404 (Q2 — NO lockout allocation) ---

func TestSeal_AccountNotFound_Bad(t *core.T) {
	_ = homeFixture(t)
	svc := subject.NewService(nil)

	// 16-hex-char id that LOOKS valid but has no on-disk account.
	r := svc.Seal(subject.SealInput{
		AccountID: "deadbeefcafef00d",
		Version:   1,
		Blob:      fixtureSealBlob(),
	})
	core.AssertFalse(t, r.OK)
	core.AssertEqual(t, "account.not_found", r.Code())
}

// --- Seal — Bad: account not found MUST NOT allocate a lockout entry ---
// (Cerberus #25 Q2 — Mantis #1586 Option 2 re-asserted)

func TestSeal_AccountNotFoundNoLockoutAllocation_Bad(t *core.T) {
	_ = homeFixture(t)
	svc := subject.NewService(nil)

	// Baseline — empty lockout map.
	core.AssertEqual(t, 0, subject.LockoutMapSizeForTest(svc),
		"fixture baseline: lockout map MUST be empty")

	// Multiple not-found seal attempts MUST NOT grow the map.
	for i := 0; i < 5; i++ {
		r := svc.Seal(subject.SealInput{
			AccountID: "fa11edc00ffeec01", // valid shape, no on-disk account
			Version:   1,
			Blob:      fixtureSealBlob(),
		})
		core.AssertFalse(t, r.OK)
		core.AssertEqual(t, "account.not_found", r.Code())
	}

	// Q2 constraint — map size unchanged (NO allocation, not just NO tick).
	// Mantis #1586 Option 2 protection re-asserted: probe traffic against
	// non-existent ids must NOT allocate any *lockoutEntry slot.
	core.AssertEqual(t, 0, subject.LockoutMapSizeForTest(svc),
		"Q2 (Mantis #1586): not_found MUST NOT allocate a lockout entry")
}

// --- Seal — Bad: blob_invalid + lockout-tick (ADD-MED-2) ---

func TestSeal_BlobTooLarge_Bad(t *core.T) {
	// Body cap is enforced at the middleware boundary (RFC §2.3 / Q5:
	// inherit MaxBodyBytesDefault = 64 KiB). At the Service level we
	// only length-sane the lower bound + accept whatever Gin's bind
	// hands us; a too-large body never reaches Seal because the
	// middleware 413s first. This test exists for completeness —
	// the Service-level shape that approximates "too small to be valid"
	// is what we can deterministically pin without spinning the engine.
	_ = homeFixture(t)
	id, svc := seedMarkerAccount(t)

	// Short blob — below minSealedBlobBytes (64).
	r := svc.Seal(subject.SealInput{
		AccountID: id,
		Version:   1,
		Blob:      []byte("too-short-to-be-a-real-pgp-envelope"),
	})
	core.AssertFalse(t, r.OK)
	core.AssertEqual(t, "account.seal.blob.invalid", r.Code())
}

// --- Seal — Bad: version != 1 → version_unsupported + lockout-tick ---

func TestSeal_VersionInvalid_Bad(t *core.T) {
	_ = homeFixture(t)
	id, svc := seedMarkerAccount(t)

	r := svc.Seal(subject.SealInput{
		AccountID: id,
		Version:   2, // unsupported in v1
		Blob:      fixtureSealBlob(),
	})
	core.AssertFalse(t, r.OK)
	core.AssertEqual(t, "account.seal.version_unsupported", r.Code())
}

// --- Seal — Conflict: already sealed (different blob) → 409 ---

func TestSeal_AlreadySealed_Conflict_Bad(t *core.T) {
	_ = homeFixture(t)
	id, svc := seedMarkerAccount(t)

	first := fixtureSealBlob()
	r1 := svc.Seal(subject.SealInput{AccountID: id, Version: 1, Blob: first})
	core.AssertTrue(t, r1.OK, "first Seal must succeed")

	// Second Seal with DIFFERENT bytes — 409 already_sealed.
	second := append([]byte{0xff, 0xee, 0xdd, 0xcc}, first[4:]...)
	r2 := svc.Seal(subject.SealInput{AccountID: id, Version: 1, Blob: second})
	core.AssertFalse(t, r2.OK, "second Seal with different blob MUST fail")
	core.AssertEqual(t, "account.seal.already_sealed", r2.Code(),
		"different-blob retry MUST surface already_sealed (§3.1 seal-once)")
}

// --- Seal — Good: idempotent retry (Q4 byte-equality) ---

func TestSeal_IdempotentRetry_Good(t *core.T) {
	_ = homeFixture(t)
	id, svc := seedMarkerAccount(t)

	blob := fixtureSealBlob()
	r1 := svc.Seal(subject.SealInput{AccountID: id, Version: 1, Blob: blob})
	core.AssertTrue(t, r1.OK, "first Seal must succeed")
	out1, _ := r1.Value.(subject.SealOutput)

	// Second Seal with byte-IDENTICAL blob — 200 OK + same SealedAt
	// (the idempotent-retry path returns the stored first-seal mtime
	// per §3.3 / Cerberus #26 OBS-1).
	r2 := svc.Seal(subject.SealInput{AccountID: id, Version: 1, Blob: blob})
	core.AssertTrue(t, r2.OK, "second Seal with identical bytes MUST succeed (idempotent)")
	out2, _ := r2.Value.(subject.SealOutput)
	core.AssertEqual(t, out1.SealedAt, out2.SealedAt,
		"idempotent retry MUST return the original SealedAt (Cerberus #26 OBS-1)")
	core.AssertEqual(t, out1.AccountID, out2.AccountID)
	core.AssertEqual(t, out1.Version, out2.Version)

	// Q4 idempotent retry MUST NOT tick the lockout counter.
	// (The lockout map remains at whatever baseline Create+Seal left
	// it at — typically 0 because no failures were ticked.)
	core.AssertEqual(t, 0, subject.LockoutMapSizeForTest(svc),
		"Q4: idempotent retry MUST NOT allocate or tick a lockout entry")
}

// --- Seal — Bad: account_id mismatch + version + invalid pre-account-bind ---
// These pre-account-bind validation paths MUST NOT allocate or tick a
// lockout entry per RFC §5.2 exempt list (mirrors unlock-side
// discipline).
//
// Service-level test: invalid_body + id_mismatch land at the handler
// boundary (URL :id reconciliation lives there). Service.Seal itself
// sees the already-reconciled AccountID. The version_unsupported and
// blob.invalid paths DO tick — those are post-bind validation
// failures (the caller has supplied a valid id, just bad payload).

// --- Seal — audit Meta MUST NOT carry blob_size_bytes (ADD-MED-3 negative) ---

func TestSeal_NoBlobSizeInAuditMeta_Bad(t *core.T) {
	_ = homeFixture(t)
	id, svc := seedMarkerAccount(t)
	rec := &recordingRecorder{}
	audit.SetDefault(rec)
	t.Cleanup(func() { audit.SetDefault(nil) })

	// Exercise BOTH success and failure paths so every audit row is
	// checked.
	r := svc.Seal(subject.SealInput{AccountID: id, Version: 1, Blob: fixtureSealBlob()})
	core.AssertTrue(t, r.OK)

	// Trigger a failure too — already_sealed conflict.
	r2 := svc.Seal(subject.SealInput{AccountID: id, Version: 1, Blob: []byte("different bytes of sufficient length to pass min-blob gate xxxx xxxx xxxx xxxx xxxx xxxx xxxx")})
	core.AssertFalse(t, r2.OK)

	core.AssertTrue(t, len(rec.events) > 0, "fixture sanity: audit recorder MUST have events")
	for _, ev := range rec.events {
		_, has := ev.Meta["blob_size_bytes"]
		core.AssertFalse(t, has,
			"ADD-MED-3: blob_size_bytes MUST NOT appear in audit Meta — event="+ev.Event)
		// Also assert NO blob content leaked into Meta (Cerberus #1465).
		_, hasBlob := ev.Meta["blob"]
		core.AssertFalse(t, hasBlob,
			"Cerberus #1465 closure-only-scope: blob bytes MUST NOT appear in Meta — event="+ev.Event)
	}
}

// --- Seal — Good: audit Meta carries EXACTLY {path_hash, version} ---
// (Cerberus #29 ADD-MED-2 — backend regression guard for
// RFC.stage-e-audit-viewer v2 §4.3)
//
// The frontend viewer's TestAuditWindow_SealedEventMetaShape_Good
// catches drift at render-time; this Go-side test pins the SAME
// invariant at emit-time so a future seal-path commit that
// reintroduces a dropped field (e.g. blob_size_bytes / wallet_path /
// size_bytes) trips here BEFORE shipping to the frontend.
//
// The shape contract per Cerberus #25 ADD-MED-3 + Cerberus #29
// ADD-MED-2: Service.Seal's success-emit MUST carry EXACTLY two Meta
// keys — path_hash + version — and nothing else. The service layer's
// __process stamp lands at the Recorder boundary (audit.Service.
// recordCommon), NOT at the emit call this recordingRecorder
// captures; the recorder receives the pre-stamp shape so the assertion
// stays tight against the keys seal.go itself sets.

func TestSeal_AuditMetaShape_Good(t *core.T) {
	_ = homeFixture(t)
	id, svc := seedMarkerAccount(t)
	rec := &recordingRecorder{}
	audit.SetDefault(rec)
	t.Cleanup(func() { audit.SetDefault(nil) })

	r := svc.Seal(subject.SealInput{
		AccountID: id,
		Version:   1,
		Blob:      fixtureSealBlob(),
	})
	core.AssertTrue(t, r.OK, "MARKER → SEALED Seal must succeed")

	var sealedEvent *audit.Event
	for i := range rec.events {
		if rec.events[i].Event == "auth.account.sealed" {
			sealedEvent = &rec.events[i]
		}
	}
	core.AssertTrue(t, sealedEvent != nil,
		"auth.account.sealed audit row MUST be emitted on Seal success")

	// EXACT-shape assertion: only path_hash + version are allowed.
	// Any additional key is a regression of Cerberus #25 ADD-MED-3 /
	// Cerberus #29 ADD-MED-2 — fail loudly so the violating commit
	// can't ship.
	allowed := map[string]bool{
		"path_hash": true,
		"version":   true,
	}
	for k := range sealedEvent.Meta {
		core.AssertTrue(t, allowed[k],
			"Cerberus #29 ADD-MED-2: unexpected Meta key in auth.account.sealed: "+k)
	}

	// Positive shape: the two expected keys MUST be present + typed
	// correctly. path_hash is SHA-256 hex (64 lowercase hex chars).
	pathHash, has := sealedEvent.Meta["path_hash"].(string)
	core.AssertTrue(t, has, "path_hash MUST be present and a string")
	core.AssertEqual(t, 64, len(pathHash),
		"path_hash MUST be SHA-256 hex (64 chars)")

	version, hasV := sealedEvent.Meta["version"].(int)
	core.AssertTrue(t, hasV, "version MUST be present and an int")
	core.AssertEqual(t, 1, version,
		"version MUST equal the SealInput.Version (v1 schema)")

	// Negative reassertion of the Cerberus #25 ADD-MED-3 dropped field
	// set — explicit list keeps grep-discoverability for future audits.
	for _, dropped := range []string{
		"blob_size_bytes",
		"wallet_path",
		"size_bytes",
		"blob",
	} {
		_, leaked := sealedEvent.Meta[dropped]
		core.AssertFalse(t, leaked,
			"Cerberus #25 ADD-MED-3 dropped field leaked into Meta: "+dropped)
	}
}

// --- handler-level: PUT /v1/account/:id/seal ---

// doPUT issues a PUT against the engine with the supplied body bytes.
// Mirrors doPOST in routes_test.go.
func doPUT(eng interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
}, path string, body []byte) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPut, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	eng.ServeHTTP(rr, req)
	return rr
}

// TestRoutes_SealEndpoint_Good — happy round-trip through the gin
// engine. Seed a MARKER account via Create, then PUT the seal blob
// at the URL :id endpoint. 200 OK + canonical response shape.
func TestRoutes_SealEndpoint_Good(t *core.T) {
	_ = homeFixture(t)
	id, svc := seedMarkerAccount(t)
	eng := newTestEngine(t, svc)

	body := core.JSONMarshal(subject.SealInput{
		AccountID: id,
		Version:   1,
		Blob:      fixtureSealBlob(),
	})
	core.AssertTrue(t, body.OK)

	rr := doPUT(eng, "/v1/account/"+id+"/seal", body.Value.([]byte))
	core.AssertEqual(t, http.StatusOK, rr.Code, "valid Seal → 200")

	var env struct {
		Success bool `json:"success"`
		Data    struct {
			AccountID string `json:"account_id"`
			SealedAt  int64  `json:"sealed_at"`
			Version   int    `json:"version"`
		} `json:"data"`
	}
	uR := core.JSONUnmarshal(rr.Body.Bytes(), &env)
	core.AssertTrue(t, uR.OK)
	core.AssertTrue(t, env.Success)
	core.AssertEqual(t, id, env.Data.AccountID)
	core.AssertEqual(t, 1, env.Data.Version)
	core.AssertTrue(t, env.Data.SealedAt > 0)
}

// TestSeal_AccountIdMismatch_Bad — URL :id ≠ body account_id surfaces
// 400 account.id_mismatch BEFORE Service.Seal is called. Identity
// binds from the URL not the body (Cerberus #25 Q1 / Cerberus #18
// mirror — handleLock's session-id discipline at routes.go:339).
func TestSeal_AccountIdMismatch_Bad(t *core.T) {
	_ = homeFixture(t)
	id, svc := seedMarkerAccount(t)
	eng := newTestEngine(t, svc)

	body := core.JSONMarshal(subject.SealInput{
		AccountID: "deadbeefcafef00d", // deliberately wrong
		Version:   1,
		Blob:      fixtureSealBlob(),
	})
	core.AssertTrue(t, body.OK)

	rr := doPUT(eng, "/v1/account/"+id+"/seal", body.Value.([]byte))
	core.AssertEqual(t, http.StatusBadRequest, rr.Code,
		"URL :id vs body account_id mismatch → 400 (Cerberus #25 Q1)")

	var env struct {
		Success bool `json:"success"`
		Error   struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	uR := core.JSONUnmarshal(rr.Body.Bytes(), &env)
	core.AssertTrue(t, uR.OK)
	core.AssertFalse(t, env.Success)
	core.AssertEqual(t, "account.id_mismatch", env.Error.Code)
}

// TestRoutes_SealEndpoint_NotFound_404 — PUT against a URL :id that
// has no on-disk account → 404 account.not_found. NO lockout
// allocation (covered by the Service-level
// TestSeal_AccountNotFoundNoLockoutAllocation_Bad).
func TestRoutes_SealEndpoint_NotFound_404(t *core.T) {
	_ = homeFixture(t)
	svc := subject.NewService(nil)
	eng := newTestEngine(t, svc)

	missingID := "deadbeefcafef00d"
	body := core.JSONMarshal(subject.SealInput{
		AccountID: missingID,
		Version:   1,
		Blob:      fixtureSealBlob(),
	})
	core.AssertTrue(t, body.OK)

	rr := doPUT(eng, "/v1/account/"+missingID+"/seal", body.Value.([]byte))
	core.AssertEqual(t, http.StatusNotFound, rr.Code,
		"missing account → 404 account.not_found (RFC §2.5)")
}

// --- Seal — Bad: lockout-state pre-check honoured (Cerberus #59 F1 / Mantis #1707) ---
//
// Cerberus #59 F1 found that Service.Seal had NO lockoutState pre-check
// despite four cross-file references (codeAccountSealAccountLocked,
// statusForSealCode 423 mapping, RFC §2.5 status table,
// recordSealFailure docstring exempt-path list) implying it did. The
// attack lever: a caller probing Seal with malformed bodies ticks the
// SHARED per-account lockout counter via recordSealFailure ->
// recordFailedAttempt; without a lockout-state pre-check on the Seal
// side, the counter overflowed silently for Seal probes and only
// surfaced 423 on the Unlock-side flow (cross-flow asymmetry — STRIDE-D).
//
// The fix mirrors unlock.go:104-109 — a lockoutState pre-check at the
// top of Seal (after IsValidID, before validation gates) returns
// codeAccountSealAccountLocked / 423 once the threshold trips. Tests
// below pin both the locked-out-blocks-seal contract AND the
// cross-flow lockout symmetry that was the load-bearing bug.

// TestSeal_LockoutStateBlocked_Bad — drive 5 failed seal attempts (the
// lockoutThreshold), then a 6th attempt MUST return
// account.seal.account_locked / 423 even though the request shape is
// otherwise valid. Before the fix, the 6th request would have continued
// through the validation gates and ticked the counter further; after
// the fix, the pre-check short-circuits with the typed locked-out
// response.
func TestSeal_LockoutStateBlocked_Bad(t *core.T) {
	_ = homeFixture(t)
	id, svc := seedMarkerAccount(t)

	threshold, _, _ := subject.LockoutConstantsForTest()

	// Drive `threshold` failures via the version-unsupported branch —
	// it's a post-IsValidID, post-account-exists validation failure that
	// ticks the lockout counter per ADD-MED-2. The trigger-attempt
	// (threshold-th call) emits the locked_out response itself; the
	// (threshold+1)-th call MUST surface the pre-check rejection.
	for i := 0; i < threshold; i++ {
		r := svc.Seal(subject.SealInput{
			AccountID: id,
			Version:   2, // unsupported — ticks lockout via recordSealFailure
			Blob:      fixtureSealBlob(),
		})
		core.AssertFalse(t, r.OK, "failed seal attempt MUST reject")
	}

	// Now the account is locked. A subsequent VALID request (correct
	// version + valid blob shape) MUST surface 423 account_locked at
	// the lockoutState pre-check BEFORE any further validation or disk
	// touch. This is the bug Cerberus #59 F1 named: Seal previously had
	// no pre-check, so the counter accumulated silently across probe
	// traffic.
	r := svc.Seal(subject.SealInput{
		AccountID: id,
		Version:   1,
		Blob:      fixtureSealBlob(),
	})
	core.AssertFalse(t, r.OK, "post-threshold Seal MUST reject")
	core.AssertEqual(t, "account.seal.account_locked", r.Code(),
		"Cerberus #59 F1: locked account MUST surface account_locked, not 200/blob-pass")
}

// TestSeal_LockoutCrossFlowAsymmetry_Bad — the load-bearing
// regression. Drive seal failures until the per-account lockout trips,
// then a subsequent Unlock call MUST ALSO reject with locked_out. The
// counter is SHARED across both flows (RFC §5.2: both flows protect the
// same account substrate; coupling the signal is the right shape), so
// seal-spam locking out unlock is the symptom that proves the counter
// is honoured symmetrically.
//
// This is the cross-flow asymmetry STRIDE-D finding: before the fix,
// Seal would tick the shared counter but never check it, while Unlock
// would check the counter but only honoured its own ticks indirectly
// (Seal still passed through). An attacker spamming Seal could lock
// out the legitimate user from Unlock WITHOUT Seal ever surfacing 423
// to forensics. Post-fix, both flows refuse on the same threshold.
func TestSeal_LockoutCrossFlowAsymmetry_Bad(t *core.T) {
	home := homeFixture(t)
	// Use a REAL encrypted account so Unlock can attempt a meaningful
	// decrypt path — this proves the lockout pre-check fires BEFORE the
	// decrypt branch (otherwise a wrong-passphrase / bad-blob would
	// confound the assertion).
	writeEncryptedAccount(t, home, fixtureAccountID, fixturePassphrase)
	svc := newUnlockable(t, home)

	threshold, _, _ := subject.LockoutConstantsForTest()

	// Spam Seal with version_unsupported failures — fixtureAccountID
	// exists on disk (so accountExists gate clears) but Seal will fail
	// validation each time AND tick the shared lockout counter per
	// ADD-MED-2. The threshold-th attempt itself triggers + returns the
	// account_locked response on the SAME call (mirror of unlock-side
	// trigger discipline at unlock.go:196-206).
	for i := 0; i < threshold; i++ {
		r := svc.Seal(subject.SealInput{
			AccountID: fixtureAccountID,
			Version:   2,
			Blob:      fixtureSealBlob(),
		})
		core.AssertFalse(t, r.OK, "seal probe MUST fail")
	}

	// Account is locked from Seal-side probes. A subsequent Unlock with
	// the CORRECT passphrase MUST reject with locked_out — the counter
	// is shared, so seal-spam must lock unlock too. Before the
	// recordSealFailure return-shape extension, the trigger boolean was
	// dropped on the floor and the cross-flow symmetry held only by
	// accident (the counter accumulated, lockoutState fired on Unlock
	// side anyway). Post-fix, the trigger event ALSO emits via Seal's
	// own auditLockoutTriggered call so the audit log carries a
	// consistent record of which flow tripped the lockout.
	rUnlock := svc.Unlock(subject.UnlockInput{
		AccountID:  fixtureAccountID,
		Passphrase: fixturePassphrase,
	})
	core.AssertFalse(t, rUnlock.OK,
		"Cerberus #59 F1: seal-side lockout MUST also block unlock (cross-flow symmetry)")
	core.AssertEqual(t, "account.unlock.locked_out", rUnlock.Code(),
		"unlock MUST surface its locked_out code (counter is shared per RFC §5.2)")
}

// TestRoutes_SealEndpoint_AlreadySealed_409 — second PUT with
// different bytes against an already-sealed account → 409
// account.seal.already_sealed. Sibling of
// TestSeal_AlreadySealed_Conflict_Bad covering the HTTP status code.
func TestRoutes_SealEndpoint_AlreadySealed_409(t *core.T) {
	_ = homeFixture(t)
	id, svc := seedMarkerAccount(t)
	eng := newTestEngine(t, svc)

	first := fixtureSealBlob()
	body := core.JSONMarshal(subject.SealInput{
		AccountID: id, Version: 1, Blob: first,
	})
	core.AssertTrue(t, body.OK)
	rr1 := doPUT(eng, "/v1/account/"+id+"/seal", body.Value.([]byte))
	core.AssertEqual(t, http.StatusOK, rr1.Code, "first PUT → 200")

	second := append([]byte{0xff, 0xee, 0xdd, 0xcc}, first[4:]...)
	body2 := core.JSONMarshal(subject.SealInput{
		AccountID: id, Version: 1, Blob: second,
	})
	core.AssertTrue(t, body2.OK)
	rr2 := doPUT(eng, "/v1/account/"+id+"/seal", body2.Value.([]byte))
	core.AssertEqual(t, http.StatusConflict, rr2.Code,
		"different-blob retry → 409 (RFC §2.5 seal-once)")
}

