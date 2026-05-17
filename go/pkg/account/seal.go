// SPDX-Licence-Identifier: EUPL-1.2

// seal.go — Stage E.A per plans/code/lthn/desktop/auth-gate/RFC.stage-e-seal.md v2.
// Implements PUT /v1/account/:id/seal — the marker-replacement endpoint
// that closes the seal-step gap left dangling by the legacy create+seal
// flow (Provision's one-shot keygen+seal does NOT use this path; it
// writes the sealed envelope as the LEAF in one atomic call).
//
// State machine (RFC §3.1):
//
//	NONEXISTENT ──Create──▶ MARKER ──Seal──▶ SEALED ──Unlock──▶ UNLOCKED
//
// Seal MUST refuse every transition except MARKER → SEALED. Concrete
// rejections (RFC §3.1):
//
//   - NONEXISTENT → 404 account.not_found    (NO lockout-tick — Q2)
//   - SEALED + identical blob → 200 OK       (idempotent retry — Q4)
//   - SEALED + different blob → 409 already_sealed
//
// Mechanism (Cerberus #25 Q3 RULING — no primitive extension):
//
//   - paths.ReadVersion pre-check classifies current state
//   - paths.AtomicWriteWithVersion(WriteInput{IfMatchHash: sha256(marker_bytes)})
//     atomic gate inside the primitive's WithFileLock — no outer lock
//     (#1578 deadlock pattern explicitly avoided)
//
// Auth tier (RFC §2.2):
//
//   - bootstrap-tier, scope account.seal
//   - lockstep entries: serverkey.bootstrapAllowedScopes +
//     server.BootstrapPathScopes (Cerberus #1467)
//   - path lookup via c.FullPath() substrate — Mantis #1626 (d4fdcca)
//
// Cerberus #25 load-bearing items folded as code:
//
//   - ADD-HIGH-2: paths.IsValidID gate BEFORE any path-touching ops
//     (Mantis #1627). Mirrors Unlock/Lock/PrivateKeyFor/PublicKeyFor.
//   - ADD-MED-2: recordSealFailure helper ticks the unlock-side
//     per-account lockout counter on blob.required, blob.invalid,
//     version_unsupported, write_failed. NOT on not_found, id.required,
//     id_mismatch, already_sealed, account_locked.
//   - ADD-MED-3: blob_size_bytes DROPPED from audit Meta.
//   - Q2: not_found MUST NOT allocate a lockout entry (re-asserts
//     Mantis #1586 Option 2 — accountExists-gated allocation).

package account

import (
	"net/http"

	core "dappco.re/go"
	coreapi "dappco.re/go/api"
	"dappco.re/lthn/desktop/pkg/audit"
	"dappco.re/lthn/desktop/pkg/paths"
	"github.com/gin-gonic/gin"
)

// minSealedBlobBytes is the lower bound on a valid PGP-symmetric
// envelope. A real Ed25519 PGP-symmetric envelope is ~512 bytes
// (S2K + IV + key packet + SEIPD with AES-256-CFB of ~96 B private-key
// DER); 64 bytes is the theoretical floor for a degenerate one-packet
// envelope. Anything shorter is structurally impossible — fail fast
// before reaching the at-rest mode-verify primitive.
//
// Cerberus #25 §2.3: deep PGP structural validation is deferred; v1
// length-sanes only. Future Stage Y / E.A.v3 dispatches MAY tighten
// to a real PGP-parse gate but the round-trip Unlock path already
// catches structural corruption via distinguishDecrypt.
const minSealedBlobBytes = 64

// Seal replaces the Create-time marker at
// ~/Lethean/account/<input.AccountID>/private.key with a user-encrypted
// PGP envelope. Seal-once — see file-level docstring.
//
// Cerberus #25 ADD-HIGH-2 — paths.IsValidID is the first gate after
// the cheap shape checks. Without it, a malicious id like
// "../../wallets/server.key" would let the handler write attacker-chosen
// paths via core.PathJoin (HIGH precedent: Mantis #1573).
//
// Cerberus #25 Q3 — the composite gate inside AtomicWriteWithVersion
// is the only file-lock holder. The handler's pre-check via ReadVersion
// runs OUTSIDE the lock; on CodeVersionStale the handler re-runs the
// pre-check so the now-current state is classified correctly. NO outer
// WithFileLock (Mantis #1578 deadlock pattern).
//
// Usage example:
//
//	r := svc.Seal(account.SealInput{
//	    AccountID: "abc123def4567890",
//	    Version:   1,
//	    Blob:      pgpEnvelopeBytes,
//	})
//	if r.OK { out := r.Value.(account.SealOutput); _ = out.SealedAt }
func (s *Service) Seal(input SealInput) core.Result {
	// Pre-account-bind validation — no lockout-tick on these paths.
	if input.AccountID == "" {
		return core.Fail(core.NewCode(codeAccountIDRequired, "account_id is required"))
	}
	// Cerberus #25 ADD-HIGH-2 — id shape gate BEFORE any path concat.
	// Mantis #1627 done-criterion. Matches Unlock (unlock.go:81),
	// Lock (unlock.go:274), PrivateKeyFor (unlock.go:881), PublicKeyFor
	// (unlock.go:915).
	if err := paths.IsValidID(input.AccountID); err != nil {
		return core.Fail(err)
	}
	// Version single-tier enforcement (RFC §2.3). Ticks lockout — a
	// caller submitting version=2 is either misconfigured or probing.
	if input.Version != 1 {
		s.recordSealFailure(input.AccountID, "version_unsupported")
		s.auditSealFailed(input.AccountID, "version_unsupported", input.RequestID)
		return core.Fail(core.NewCode(codeAccountSealVersionUnsupported,
			"only version=1 sealed envelopes are accepted"))
	}
	// Blob length-sane gate (RFC §2.3). Ticks lockout.
	if len(input.Blob) == 0 {
		s.recordSealFailure(input.AccountID, "blob_required")
		s.auditSealFailed(input.AccountID, "blob_required", input.RequestID)
		return core.Fail(core.NewCode(codeAccountSealBlobRequired,
			"blob is required"))
	}
	if len(input.Blob) < minSealedBlobBytes {
		s.recordSealFailure(input.AccountID, "blob_invalid")
		s.auditSealFailed(input.AccountID, "blob_invalid", input.RequestID)
		return core.Fail(core.NewCode(codeAccountSealBlobInvalid,
			"blob is shorter than the minimum valid PGP-symmetric envelope"))
	}

	rootR := paths.Root()
	if !rootR.OK {
		return rootR
	}
	root, _ := rootR.Value.(string)
	privatePath := core.PathJoin(root, accountDir, input.AccountID, privateKeyFile)
	publicPath := core.PathJoin(root, accountDir, input.AccountID, publicKeyFile)

	// Pre-check via ReadVersion (RFC §3.3 step 2). Classifies current
	// state for the §3.1 transition rules. The primitive's ReadVersion
	// returns Mtime.IsZero() for a missing file — used as the
	// NONEXISTENT signal here.
	curR := paths.ReadVersion(privatePath)
	if !curR.OK {
		// Read error — surface as write_failed so the operator sees
		// the disk-touch failed without leaking which step.
		s.recordSealFailure(input.AccountID, "write_failed")
		s.auditSealFailed(input.AccountID, "write_failed", input.RequestID)
		return core.Fail(core.NewCode(codeAccountSealWriteFailed,
			"reading current private.key state failed: "+curR.Error()))
	}
	cur, _ := curR.Value.(paths.ReadOutput)
	if cur.Mtime.IsZero() {
		// NONEXISTENT — Q2 RULED: NO lockout-tick + NO lockout
		// allocation. Mantis #1586 Option 2 protection re-asserted.
		s.auditSealFailed(input.AccountID, "not_found", input.RequestID)
		return core.Fail(core.NewCode(codeAccountNotFound,
			"no account exists at this id; call /v1/account/create first"))
	}

	// Read the public key to compute the expected marker bytes for
	// IfMatchHash. The Create-marker is hex(SHA-256(public_key)) — 64
	// ASCII chars. isCreateMarker (unlock.go:999) is the existing
	// classifier; we reproduce the marker-bytes computation here so the
	// IfMatchHash gate has something to compare against.
	pubR := core.ReadFile(publicPath)
	if !pubR.OK {
		// public.key missing while private.key is present is a
		// structurally-impossible state under the Cerberus #1471 leaf
		// invariant (Create writes public.key BEFORE private.key). Treat
		// as write_failed so the operator triages the directory.
		s.recordSealFailure(input.AccountID, "write_failed")
		s.auditSealFailed(input.AccountID, "write_failed", input.RequestID)
		return core.Fail(core.NewCode(codeAccountSealWriteFailed,
			"reading public.key failed: "+pubR.Error()))
	}
	pubBytes, _ := pubR.Value.([]byte)
	expectedMarker := []byte(core.SHA256HexString(string(pubBytes)))

	// Classify cur.Body against the expected marker:
	//   - bytes match expectedMarker → MARKER state, proceed to seal
	//   - bytes == input.Blob        → SEALED idempotent retry, return 200 OK
	//   - bytes ≠ marker AND ≠ blob  → SEALED with different content, 409
	if !bytesEqual(cur.Body, expectedMarker) {
		// SEALED state — distinguish idempotent retry from real conflict.
		if bytesEqual(cur.Body, input.Blob) {
			// Idempotent retry per Q4 byte-equality. NO lockout-tick.
			// NO audit-as-failure. Stored sealed_at = current mtime per
			// Cerberus #26 OBS-1.
			return core.Ok(SealOutput{
				AccountID: input.AccountID,
				SealedAt:  cur.Mtime.Unix(),
				Version:   input.Version,
			})
		}
		// Genuine seal-once conflict. NO lockout-tick (legitimate
		// non-malicious caller may have re-encrypted; ADD-MED-2 lockout
		// applies only to validation-failure paths, not state-conflict).
		s.auditSealFailed(input.AccountID, "already_sealed", input.RequestID)
		return core.Fail(core.NewCode(codeAccountSealAlreadySealed,
			"account is already sealed; refusing to overwrite"))
	}

	// MARKER state — proceed to AtomicWriteWithVersion with the
	// IfMatchHash composite gate (Cerberus #25 Q3 mechanism). The
	// hash MUST match the marker bytes we just classified; concurrent
	// writers see CodeVersionStale and the second caller re-enters
	// the pre-check, correctly returning 200/409 per the now-current
	// state.
	writeR := paths.AtomicWriteWithVersion(privatePath, paths.WriteInput{
		Body:        input.Blob,
		IfMatchHash: core.SHA256Hex(expectedMarker),
	})
	if !writeR.OK {
		// CodeVersionStale = concurrent seal won the race. Re-run the
		// classification once to pick the right response shape. We do
		// NOT loop indefinitely — the second pass either lands a real
		// 200/409 or surfaces the same write_failed for a structural
		// failure (e.g. the racing writer also stale → escalate).
		if writeR.Code() == paths.CodeVersionStale {
			cur2R := paths.ReadVersion(privatePath)
			if cur2R.OK {
				cur2, _ := cur2R.Value.(paths.ReadOutput)
				if !cur2.Mtime.IsZero() && !bytesEqual(cur2.Body, expectedMarker) {
					if bytesEqual(cur2.Body, input.Blob) {
						return core.Ok(SealOutput{
							AccountID: input.AccountID,
							SealedAt:  cur2.Mtime.Unix(),
							Version:   input.Version,
						})
					}
					s.auditSealFailed(input.AccountID, "already_sealed", input.RequestID)
					return core.Fail(core.NewCode(codeAccountSealAlreadySealed,
						"account is already sealed; refusing to overwrite"))
				}
			}
		}
		s.recordSealFailure(input.AccountID, "write_failed")
		s.auditSealFailed(input.AccountID, "write_failed", input.RequestID)
		return core.Fail(core.NewCode(codeAccountSealWriteFailed,
			"writing sealed private.key failed: "+writeR.Error()))
	}
	out, _ := writeR.Value.(paths.WriteOutput)

	// Success — emit audit. Cerberus #26 OBS-1: sealed_at comes from
	// the post-write Mtime so the response matches what
	// paths.ReadVersion(private.key).Mtime returns on subsequent
	// idempotent retries (seal-once invariant makes mtime the
	// canonical first-seal-time).
	now := out.Mtime.Unix()
	s.auditSealSucceeded(input.AccountID, input.Version, root, input.RequestID, now)

	return core.Ok(SealOutput{
		AccountID: input.AccountID,
		SealedAt:  now,
		Version:   input.Version,
	})
}

// recordSealFailure ticks the unlock-side per-account lockout counter
// on validation-failure seal paths per Cerberus #25 ADD-MED-2. Mirrors
// recordFailedAttempt (unlock.go:607) but exposed as a Service-scoped
// shim so the tick-discipline lives in one place and the test substrate
// has a single spy point.
//
// Exempt paths (NO call to this helper — load-bearing):
//
//   - account.id.required / invalid_body / id_mismatch — pre-account-bind
//   - account.not_found — Q2 constraint, NO allocation
//   - account.seal.already_sealed — legitimate retry path
//   - account.seal.account_locked — already locked
//
// Reuses the existing recordFailedAttempt machinery so the per-account
// lockout window + cooldown stay coherent across unlock failures and
// seal failures (RFC §5.2: both flows protect the same account
// substrate; coupling the signal is the right shape).
//
// Usage example:
//
//	s.recordSealFailure(input.AccountID, "blob_invalid")
//	s.auditSealFailed(input.AccountID, "blob_invalid", input.RequestID)
//	return core.Fail(core.NewCode(codeAccountSealBlobInvalid, "..."))
func (s *Service) recordSealFailure(accountID, reason string) {
	now := core.Now().UTC().Unix()
	_, _, _ = s.recordFailedAttempt(accountID, now)
	_ = reason // reason is captured by the sibling auditSealFailed call
}

// auditSealSucceeded emits the auth.account.sealed audit row + the
// human-debug core.Print echo per Stage F.B Phase 1 dual-emit shape
// (Stage E.B and Stage X.B mirrors). Recorder failure ignored — audit
// failures MUST NEVER block the auth path that emitted the event.
//
// References audit.EventAuthAccountSealed for the typed event-name
// constant so the parity-grep + Stage F log-tailer have a single
// surface to bind against (Mantis #1631 H#141 follow-up — replaced the
// prior literal-string emit). The core.Print debug line still carries
// the literal for grep-friendly stderr output; the on-disk shape is
// driven by the constant.
//
// Cerberus #25 ADD-MED-3: blob_size_bytes is DROPPED from Meta. The
// forensic use-case is unnamed; default-omit per Cerberus discipline.
// Cerberus #1465 closure-only-scope: the canonical encrypted blob is
// NEVER in Meta; the SHA-256 hex of the account directory path is the
// audit join-key.
//
// Usage example:
//
//	s.auditSealSucceeded(input.AccountID, input.Version, root, input.RequestID, now)
func (s *Service) auditSealSucceeded(accountID string, version int, root, requestID string, ts int64) {
	dir := core.PathJoin(root, accountDir, accountID)
	pathHash := core.SHA256HexString(dir)
	core.Print(core.Stderr(),
		"event=auth.account.sealed account_id=%s ts=%d version=%d\n",
		accountID, ts, version)
	_ = audit.Default().Record(audit.Event{
		Event:     audit.EventAuthAccountSealed,
		AccountID: accountID,
		TS:        ts,
		Scope:     "account.seal",
		Outcome:   audit.OutcomeOK,
		RequestID: requestID,
		Meta: map[string]any{
			"path_hash": pathHash,
			"version":   version,
		},
	})
}

// auditSealFailed emits auth.account.seal_failed with the reason code.
// Lockout-tick (when applicable per ADD-MED-2) MUST happen via
// recordSealFailure BEFORE this call so the audit row reflects the
// freshly-decremented counter; the call order is asserted by the test
// substrate via stub.
//
// References audit.EventAuthAccountSealFailed for the typed event-name
// constant — sibling of auditSealSucceeded per the Mantis #1631 H#141
// follow-up that landed the typed surface.
//
// Usage example:
//
//	s.recordSealFailure(input.AccountID, "write_failed")
//	s.auditSealFailed(input.AccountID, "write_failed", input.RequestID)
func (s *Service) auditSealFailed(accountID, reason, requestID string) {
	now := core.Now().UTC().Unix()
	core.Print(core.Stderr(),
		"event=auth.account.seal_failed account_id=%s ts=%d reason=%s\n",
		accountID, now, reason)
	_ = audit.Default().Record(audit.Event{
		Event:     audit.EventAuthAccountSealFailed,
		AccountID: accountID,
		TS:        now,
		Scope:     "account.seal",
		Outcome:   audit.OutcomeFailed,
		RequestID: requestID,
		Meta: map[string]any{
			"reason": reason,
		},
	})
}

// handleSeal is the gin handler for PUT /v1/account/:id/seal. Reads
// SealInput, enforces the URL-vs-body id-match invariant (Cerberus #25
// Q1 — authoritative identity binds from the URL not the body, mirrors
// handleLock's session-id discipline), and shapes the HTTP response.
//
// Auth: bootstrap-auth middleware verifies LTHN-BOOT-1.* token with
// scope=account.seal BEFORE this handler runs. By the time the handler
// lands the nonce is already burned (replay defence holds on partial
// failure per Cerberus #1460 (d)).
//
// Cerberus #1626 substrate (commit d4fdcca) — the BootstrapPathScopes
// lookup uses c.FullPath() (gin's registered route pattern
// "/v1/account/:id/seal") not c.Request.URL.Path; this is why the
// parametrised route works.
//
// HTTP status mapping per RFC.stage-e-seal.md §2.5:
//
//   - account.invalid_body              → 400 Bad Request
//   - account.id.required               → 400 Bad Request
//   - account.id_mismatch               → 400 Bad Request
//   - account.seal.blob.required        → 400 Bad Request
//   - account.seal.blob.invalid         → 400 Bad Request
//   - account.seal.version_unsupported  → 400 Bad Request
//   - account.not_found                 → 404 Not Found
//   - account.seal.already_sealed       → 409 Conflict
//   - account.seal.account_locked       → 423 Locked
//   - account.seal.write_failed         → 500 Internal Server Error
func (g *routesProvider) handleSeal(c *gin.Context) {
	var input SealInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, coreapi.Fail(codeAccountInvalidBody, err.Error()))
		return
	}

	// Cerberus #25 Q1 — authoritative account_id binds from the URL,
	// not the body. The body field MAY be present for client-side typo
	// protection but MUST equal the URL when supplied. Empty body
	// account_id is filled from the URL; mismatch surfaces id_mismatch.
	// Mirrors handleLock's body-vs-session id discipline (routes.go:339).
	urlID := c.Param("id")
	if urlID == "" {
		c.JSON(http.StatusBadRequest,
			coreapi.Fail(codeAccountIDRequired, "account_id is required in URL"))
		return
	}
	if input.AccountID != "" && input.AccountID != urlID {
		c.JSON(http.StatusBadRequest,
			coreapi.Fail(codeAccountIDMismatch,
				"body account_id does not match URL :id"))
		return
	}
	// Canonical source — overwrite body with URL-bound id before
	// handing off to Service.Seal.
	input.AccountID = urlID

	// Cerberus #1524 — server generates the audit RequestID. Caller's
	// body field is dropped to prevent forensic deniability; the
	// server-side UUID v4 is echoed in the X-Request-Id header so the
	// legitimate caller can correlate to the audit log. Mirrors
	// handleCreate / handleUnlock / handleProvision discipline.
	srvReqID := serverRequestID()
	input.RequestID = srvReqID
	c.Header("X-Request-Id", srvReqID)

	r := g.svc.Seal(input)
	if r.OK {
		out, _ := r.Value.(SealOutput)
		c.JSON(http.StatusOK, coreapi.OK(out))
		return
	}

	code := r.Code()
	if code == "" {
		code = codeAccountSealWriteFailed
	}
	c.JSON(statusForSealCode(code), coreapi.Fail(code, r.Error()))
}

// statusForSealCode maps a "account.seal.*" / "account.*" failure code
// emitted by Service.Seal to its HTTP status per RFC §2.5. Kept
// separate from the package-wide statusForCode so the seal-specific
// codes (already_sealed → 409, account_locked → 423, not_found → 404)
// can land without disturbing the create/unlock/provision/lock
// mappings (which already use statusForCode).
//
// Unknown codes default to 500 so an unmapped failure surfaces as a
// server error rather than masquerading as a client error.
//
// Usage example:
//
//	c.JSON(statusForSealCode("account.seal.already_sealed"), coreapi.Fail(...))
func statusForSealCode(code string) int {
	switch code {
	case codeAccountInvalidBody,
		codeAccountIDRequired,
		codeAccountIDMismatch,
		codeAccountSealBlobRequired,
		codeAccountSealBlobInvalid,
		codeAccountSealVersionUnsupported,
		"paths.invalid_id":
		return http.StatusBadRequest
	case codeAccountNotFound:
		return http.StatusNotFound
	case codeAccountSealAlreadySealed:
		return http.StatusConflict
	case codeAccountSealAccountLocked:
		return http.StatusLocked
	}
	return http.StatusInternalServerError
}

// bytesEqual is a tiny local shim that names the comparison so the
// caller's intent is grep-able. core.* doesn't ship a bytes.Equal
// alias today; the loop body is trivial enough to inline without
// pulling the stdlib bytes import (which would violate AX-6 in this
// package's go vet/lint discipline if the package doesn't already
// import bytes — unlock.go does, but seal.go shouldn't grow the
// dependency just for one call).
//
// Usage example:
//
//	if bytesEqual(cur.Body, expectedMarker) { ... }
func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
