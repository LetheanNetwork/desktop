// SPDX-Licence-Identifier: EUPL-1.2

package serverkey

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/audit"
	"forge.lthn.ai/Snider/Enchantrix/pkg/crypt/std/pgp"
)

// IssueBootstrapToken mints a fresh detached-PGP-signature token with
// the default account.create scope. Thin wrapper around
// IssueBootstrapTokenForScope retained for callers that pre-date the
// Stage E.B multi-scope split.
//
// Usage example:
//
//	r := svc.IssueBootstrapToken()
//	if r.OK { out := r.Value.(BootstrapTokenOutput); _ = out.Token }
func (s *Service) IssueBootstrapToken() core.Result {
	return s.IssueBootstrapTokenForScope(scopeAccountCreate)
}

// IssueBootstrapTokenForScope mints a fresh detached-PGP-signature
// token over a canonical JSON header for the requested scope. The
// token shape is
//
//	LTHN-BOOT-1.<base64url-header>.<base64url-signature>
//
// where header = canonicalise({"iat": <now>, "exp": <now+60>,
// "scope": "<wantScope>", "nonce": "<hex>"}). issuerTTL of 60s
// is intentional — combined with the verifier's 120s ceiling +
// per-mint nonce, the replay window stays narrow while still
// permitting the user a sane amount of time to enter their
// passphrase + retry on error (Cerberus #1462).
//
// The token MUST live ONLY in the closure scope of the frontend
// handler that consumes it (Cerberus #1465). Never persist to
// localStorage / cookie / IndexedDB / module-level cache.
//
// scope MUST be one of the bootstrapAllowedScopes set
// (account.create or account.unlock). An unknown scope is rejected
// with "auth.bootstrap.scope" so a typo never silently mints an
// unverifiable token.
//
// Usage example:
//
//	r := svc.IssueBootstrapTokenForScope("account.unlock")
//	if r.OK { out := r.Value.(BootstrapTokenOutput); _ = out.Token }
func (s *Service) IssueBootstrapTokenForScope(scope string) core.Result {
	if !bootstrapAllowedScopes[scope] {
		return core.Fail(core.NewCode("auth.bootstrap.scope",
			"requested scope is not in the bootstrap-token allow-list"))
	}
	s.mu.RLock()
	priv := s.privateKey
	s.mu.RUnlock()
	if len(priv) == 0 {
		return core.Fail(core.E("serverkey.IssueBootstrapToken",
			"server key not bootstrapped — call Bootstrap() first", nil))
	}

	nonceR := core.RandomBytes(nonceSize)
	if !nonceR.OK {
		return core.Fail(core.E("serverkey.IssueBootstrapToken", "generate nonce", nonceR.Value.(error)))
	}
	nonce := core.HexEncode(nonceR.Value.([]byte))

	now := core.Now().UTC().Unix()
	exp := now + issuerTTL

	header := map[string]any{
		"iat":   now,
		"exp":   exp,
		"scope": scope,
		"nonce": nonce,
	}
	canon, canonR := canonicalise(header)
	if !canonR.OK {
		return canonR
	}

	pgpSvc := pgp.NewService()
	sig, err := pgpSvc.Sign(priv, canon)
	if err != nil {
		return core.Fail(core.E("serverkey.IssueBootstrapToken", "sign header", err))
	}

	token := tokenPrefix + core.Base64URLEncode(canon) + "." + core.Base64URLEncode(sig)
	return core.Ok(BootstrapTokenOutput{
		Token:     token,
		ExpiresAt: exp,
	})
}

// VerifyBootstrapToken implements the Verifier interface for
// pkg/server's bootstrap-auth middleware. Returns OK iff:
//
//  1. token has the LTHN-BOOT-1. prefix + two base64url-encoded parts
//  2. header decodes to a map with iat / exp / scope / nonce fields
//  3. scope equals wantScope (Cerberus #1467 — path/scope lockstep)
//  4. iat <= now + clockSkewTolerance (5s slack — Cerberus #1462)
//  5. exp > now (token not expired)
//  6. now - iat <= verifierTTL (120s ceiling — Cerberus #1462)
//  7. nonce not present in consumed set (Cerberus #1463)
//  8. PGP signature verifies against the in-memory server public key
//
// On OK the nonce is added to the consumed set BEFORE return (so a
// caller can't drive a replay by tight-looping the same token between
// VerifyBootstrapToken and the endpoint handler firing).
//
// On any failure returns core.Fail with a code from "auth.bootstrap.*"
// — the middleware shapes the error envelope without leaking which
// check tripped to the caller (timing oracle hardening).
//
// Usage example:
//
//	r := svc.VerifyBootstrapToken(token, "account.create")
//	if !r.OK { /* 401 */ }
func (s *Service) VerifyBootstrapToken(token, wantScope string) core.Result {
	s.mu.RLock()
	pub := s.publicKey
	s.mu.RUnlock()
	if len(pub) == 0 {
		return core.Fail(core.E("serverkey.VerifyBootstrapToken",
			"server key not bootstrapped — call Bootstrap() first", nil))
	}

	if !core.HasPrefix(token, tokenPrefix) {
		return core.Fail(core.NewCode("auth.bootstrap.format", "token prefix invalid"))
	}
	body := token[len(tokenPrefix):]
	parts := core.Split(body, ".")
	if len(parts) != 2 {
		return core.Fail(core.NewCode("auth.bootstrap.format", "token must have exactly two segments"))
	}
	headerR := core.Base64URLDecode(parts[0])
	if !headerR.OK {
		return core.Fail(core.NewCode("auth.bootstrap.format", "header decode failed"))
	}
	headerBytes, _ := headerR.Value.([]byte)
	sigR := core.Base64URLDecode(parts[1])
	if !sigR.OK {
		return core.Fail(core.NewCode("auth.bootstrap.format", "signature decode failed"))
	}
	sigBytes, _ := sigR.Value.([]byte)

	var header map[string]any
	if r := core.JSONUnmarshal(headerBytes, &header); !r.OK {
		return core.Fail(core.NewCode("auth.bootstrap.format", "header is not valid JSON"))
	}

	// Re-canonicalise the parsed header — the signature was computed
	// over the canonical bytes, not the raw bytes the caller sent
	// (Cerberus #1469). Mismatch between parsed canon vs raw bytes
	// would let an attacker submit a tampered key-order while
	// claiming a valid signature; canonicalising forces the verifier
	// to use the same input the issuer signed.
	canon, canonR := canonicalise(header)
	if !canonR.OK {
		return canonR
	}

	scope, _ := header["scope"].(string)
	if scope != wantScope {
		return core.Fail(core.NewCode("auth.bootstrap.scope", "token scope does not match expected scope"))
	}

	iat, ok := numericClaim(header, "iat")
	if !ok {
		return core.Fail(core.NewCode("auth.bootstrap.format", "iat claim missing or non-numeric"))
	}
	exp, ok := numericClaim(header, "exp")
	if !ok {
		return core.Fail(core.NewCode("auth.bootstrap.format", "exp claim missing or non-numeric"))
	}
	nonce, _ := header["nonce"].(string)
	if nonce == "" {
		return core.Fail(core.NewCode("auth.bootstrap.format", "nonce missing or empty"))
	}

	now := core.Now().UTC().Unix()
	if iat > now+clockSkewTolerance {
		return core.Fail(core.NewCode("auth.bootstrap.ttl", "iat in the future beyond clock-skew tolerance"))
	}
	if exp <= now {
		return core.Fail(core.NewCode("auth.bootstrap.ttl", "token expired"))
	}
	if now-iat > verifierTTL {
		return core.Fail(core.NewCode("auth.bootstrap.ttl", "token age exceeds verifier ceiling"))
	}

	// Nonce replay defence (Cerberus #1463). Keyed by HMAC so a
	// memory-read attacker doesn't learn the nonces in flight.
	key := s.nonceKey(nonce)
	s.mu.Lock()
	s.evictExpiredNoncesLocked(now)
	if _, replay := s.consumedNonces[key]; replay {
		s.mu.Unlock()
		return core.Fail(core.NewCode("auth.bootstrap.replay", "nonce already consumed"))
	}
	s.mu.Unlock()

	// Verify the PGP signature against the canonical bytes — NOT the
	// raw header bytes the caller submitted.
	pgpSvc := pgp.NewService()
	if err := pgpSvc.Verify(pub, canon, sigBytes); err != nil {
		return core.Fail(core.NewCode("auth.bootstrap.signature", "signature verification failed"))
	}

	// Mark the nonce consumed. Re-take the lock; evict any newcomers
	// who slipped between unlock + relock (the re-check below makes
	// the operation idempotent — even if two goroutines race past the
	// check above, the second to land sees an existing entry and
	// refuses the token).
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, replay := s.consumedNonces[key]; replay {
		return core.Fail(core.NewCode("auth.bootstrap.replay", "nonce already consumed"))
	}
	// Expire the nonce after the verifier ceiling — anything older
	// would already be rejected on TTL grounds, so we don't need to
	// remember it forever.
	s.consumedNonces[key] = core.UnixTime(now + verifierTTL)
	return core.Ok(nil)
}

// IssueSessionToken mints a fresh LTHN-SESS-1. detached-PGP-signature
// token authorising the supplied account_id for the issuer-TTL window
// per RFC §3 / §6.
//
//	LTHN-SESS-1.<base64url(canonical-header)>.<base64url(signature)>
//
// where header = canonicalise({"iat": <now>, "exp": <now+900>,
// "scope": "session", "account_id": "<hex>", "nonce": "<hex>"}).
// 15-minute issuer TTL + 30-minute verifier ceiling (sessionIssuerTTL
// + sessionVerifierTTL) calibrate to the desk-leave threshold, NOT
// the user's task duration (RFC §6).
//
// The token MUST live only in module-scope frontend state via
// api-fetch.ts:setSessionToken — never localStorage / sessionStorage
// / cookie / IndexedDB / module cache (RFC §3.2 — same closure-only
// discipline Cerberus #1465 demanded for bootstrap-tokens).
//
// Per RFC §3.1 the nonce IS added to consumedSessionNonces (a per-
// issue-uniqueness gate at mint time) but VerifySessionToken does
// NOT burn the nonce — session tokens are consumed N times.
//
// Usage example:
//
//	r := svc.IssueSessionToken("abc123def4567890")
//	if r.OK {
//	    out := r.Value.(SessionTokenOutput)
//	    _ = out.Token  // LTHN-SESS-1.…
//	}
func (s *Service) IssueSessionToken(accountID string) core.Result {
	if accountID == "" {
		return core.Fail(core.NewCode("auth.session.account_id",
			"account_id is required to mint a session token"))
	}
	s.mu.RLock()
	priv := s.privateKey
	s.mu.RUnlock()
	if len(priv) == 0 {
		return core.Fail(core.E("serverkey.IssueSessionToken",
			"server key not bootstrapped — call Bootstrap() first", nil))
	}

	nonceR := core.RandomBytes(nonceSize)
	if !nonceR.OK {
		return core.Fail(core.E("serverkey.IssueSessionToken", "generate nonce", nonceR.Value.(error)))
	}
	nonce := core.HexEncode(nonceR.Value.([]byte))

	now := core.Now().UTC().Unix()
	exp := now + sessionIssuerTTL

	header := map[string]any{
		"iat":        now,
		"exp":        exp,
		"scope":      scopeSession,
		"account_id": accountID,
		"nonce":      nonce,
	}
	canon, canonR := canonicalise(header)
	if !canonR.OK {
		return canonR
	}

	pgpSvc := pgp.NewService()
	sig, err := pgpSvc.Sign(priv, canon)
	if err != nil {
		return core.Fail(core.E("serverkey.IssueSessionToken", "sign header", err))
	}

	// Per-issue nonce uniqueness — defends against a buggy mint loop
	// emitting the same nonce twice. Verify does NOT burn (RFC §3.1).
	key := s.nonceKey(nonce)
	s.mu.Lock()
	s.evictExpiredSessionNoncesLocked(now)
	if _, replay := s.consumedSessionNonces[key]; replay {
		s.mu.Unlock()
		return core.Fail(core.NewCode("auth.session.replay",
			"session-token nonce collision detected at issue time"))
	}
	s.consumedSessionNonces[key] = core.UnixTime(now + sessionVerifierTTL)
	s.mu.Unlock()

	// RFC §6 M4 — audit-log preparation. Event names are reserved
	// schema; Stage F log-tailing relies on the literal names.
	core.Print(core.Stderr(),
		"event=auth.session.issued account_id=%s ts=%d exp=%d\n",
		accountID, now, exp)
	// Stage F.B Phase 1 — typed audit.Recorder emission alongside the
	// human-debug core.Print line (RFC.stage-f §1 dual-emit shape).
	// Recorder write failure is ignored — audit failures MUST NEVER
	// block the auth path. The session-token bytes themselves are
	// NEVER passed to the recorder (only the iat/exp/scope envelope).
	_ = audit.Default().Record(audit.Event{
		Event:     audit.EventAuthSessionIssued,
		AccountID: accountID,
		TS:        now,
		Exp:       exp,
		Scope:     scopeSession,
		Outcome:   audit.OutcomeOK,
	})

	token := sessionTokenPrefix + core.Base64URLEncode(canon) + "." + core.Base64URLEncode(sig)
	return core.Ok(SessionTokenOutput{
		Token:     token,
		ExpiresAt: exp,
		AccountID: accountID,
	})
}

// VerifySessionToken implements the session-tier half of the
// Verifier interface per RFC §3.1 + §4. Returns OK with
// SessionVerifyOutput on success:
//
//  1. token has the LTHN-SESS-1. prefix + two base64url-encoded parts
//  2. header decodes to a map with iat / exp / scope / account_id / nonce
//  3. scope equals "session" (Cerberus #1467 — bootstrap tokens MUST
//     NOT satisfy a session-tier gate, and vice versa)
//  4. iat <= now + clockSkewTolerance
//  5. exp > now (token not expired)
//  6. now - iat <= sessionVerifierTTL (30min ceiling — defends a
//     future spec change that lengthens `exp` without re-reviewing)
//  7. account_id present and non-empty
//  8. PGP signature verifies against the in-memory server public key
//
// Per RFC §3.1 the nonce is NOT added to a consumed-set on verify —
// session tokens are consumed N times per session (once per
// apiFetch call). Replay defence relies on TTL + closure-only
// frontend storage.
//
// On failure returns core.Fail with a code from "auth.session.*".
// Failures emit auth.session.verify_failed audit events per RFC §6
// M4 with a `reason` field so Stage F's log-tailer can categorise
// without parsing prose error messages.
//
// Usage example:
//
//	r := svc.VerifySessionToken(token)
//	if !r.OK { /* 401 */ }
//	out := r.Value.(SessionVerifyOutput)
//	_ = out.AccountID
func (s *Service) VerifySessionToken(token string) core.Result {
	s.mu.RLock()
	pub := s.publicKey
	s.mu.RUnlock()
	if len(pub) == 0 {
		return core.Fail(core.E("serverkey.VerifySessionToken",
			"server key not bootstrapped — call Bootstrap() first", nil))
	}

	if !core.HasPrefix(token, sessionTokenPrefix) {
		s.auditSessionVerifyFailed("", "prefix")
		return core.Fail(core.NewCode("auth.session.format", "token prefix invalid"))
	}
	body := token[len(sessionTokenPrefix):]
	parts := core.Split(body, ".")
	if len(parts) != 2 {
		s.auditSessionVerifyFailed("", "unknown")
		return core.Fail(core.NewCode("auth.session.format", "token must have exactly two segments"))
	}
	headerR := core.Base64URLDecode(parts[0])
	if !headerR.OK {
		s.auditSessionVerifyFailed("", "unknown")
		return core.Fail(core.NewCode("auth.session.format", "header decode failed"))
	}
	headerBytes, _ := headerR.Value.([]byte)
	sigR := core.Base64URLDecode(parts[1])
	if !sigR.OK {
		s.auditSessionVerifyFailed("", "unknown")
		return core.Fail(core.NewCode("auth.session.format", "signature decode failed"))
	}
	sigBytes, _ := sigR.Value.([]byte)

	var header map[string]any
	if r := core.JSONUnmarshal(headerBytes, &header); !r.OK {
		s.auditSessionVerifyFailed("", "unknown")
		return core.Fail(core.NewCode("auth.session.format", "header is not valid JSON"))
	}

	// Re-canonicalise the parsed header BEFORE checking the signature —
	// same Cerberus #1469 discipline as bootstrap-token verify. Forces
	// the verifier to sign-check the issuer's bytes, not the caller's.
	canon, canonR := canonicalise(header)
	if !canonR.OK {
		return canonR
	}

	scope, _ := header["scope"].(string)
	if scope != scopeSession {
		accountID, _ := header["account_id"].(string)
		s.auditSessionVerifyFailed(accountID, "scope")
		return core.Fail(core.NewCode("auth.session.scope", "token scope is not session"))
	}

	iat, ok := numericClaim(header, "iat")
	if !ok {
		s.auditSessionVerifyFailed("", "unknown")
		return core.Fail(core.NewCode("auth.session.format", "iat claim missing or non-numeric"))
	}
	exp, ok := numericClaim(header, "exp")
	if !ok {
		s.auditSessionVerifyFailed("", "unknown")
		return core.Fail(core.NewCode("auth.session.format", "exp claim missing or non-numeric"))
	}
	accountID, _ := header["account_id"].(string)
	if accountID == "" {
		s.auditSessionVerifyFailed("", "unknown")
		return core.Fail(core.NewCode("auth.session.format", "account_id missing or empty"))
	}

	now := core.Now().UTC().Unix()
	if iat > now+clockSkewTolerance {
		s.auditSessionVerifyFailed(accountID, "expired")
		return core.Fail(core.NewCode("auth.session.ttl", "iat in the future beyond clock-skew tolerance"))
	}
	if exp <= now {
		s.auditSessionVerifyFailed(accountID, "expired")
		return core.Fail(core.NewCode("auth.session.ttl", "token expired"))
	}
	if now-iat > sessionVerifierTTL {
		s.auditSessionVerifyFailed(accountID, "expired")
		return core.Fail(core.NewCode("auth.session.ttl", "token age exceeds verifier ceiling"))
	}

	// Signature verification LAST so the timing-signal of "valid
	// signature but expired token" is no worse than "invalid
	// signature, never decode further" — both reach this branch in
	// constant-ish time.
	pgpSvc := pgp.NewService()
	if err := pgpSvc.Verify(pub, canon, sigBytes); err != nil {
		s.auditSessionVerifyFailed(accountID, "signature")
		return core.Fail(core.NewCode("auth.session.signature", "signature verification failed"))
	}

	return core.Ok(SessionVerifyOutput{AccountID: accountID})
}

// auditSessionVerifyFailed emits the auth.session.verify_failed
// audit event per RFC §6 M4 with the documented structured fields.
// Reserved-schema: changing field names after Stage E ships breaks
// the Stage F log-tailing contract.
//
// reason MUST be a member of sessionVerifyFailedReasons. Cerberus
// DREAD ADD-MED-2 — the RFC §6 M4 enumeration is the canonical
// set; any reason outside it normalises to "unknown" so the log-
// tailer keeps a closed-world view + Stage F's grok parser doesn't
// need to learn new categories per Stage F log-tailing contract.
func (s *Service) auditSessionVerifyFailed(accountID, reason string) {
	if !sessionVerifyFailedReasons[reason] {
		reason = "unknown"
	}
	now := core.Now().UTC().Unix()
	core.Print(core.Stderr(),
		"event=auth.session.verify_failed account_id=%s ts=%d reason=%s\n",
		accountID, now, reason)
	// Stage F.B Phase 1 — typed audit.Recorder emission alongside the
	// human-debug core.Print line. `reason` lands in Meta so the
	// Operations-panel pivot stays the same closed enumeration the
	// log-tailer would have parsed (RFC.stage-f §6 M4 reserved fields).
	_ = audit.Default().Record(audit.Event{
		Event:     audit.EventAuthSessionVerifyFailed,
		AccountID: accountID,
		TS:        now,
		Scope:     scopeSession,
		Outcome:   audit.OutcomeDenied,
		Meta: map[string]any{
			"reason": reason,
		},
	})
}

// sessionVerifyFailedReasons is the closed set of reasons RFC §6 M4
// reserves for the auth.session.verify_failed audit event. Any
// caller-supplied reason outside this set normalises to "unknown"
// in auditSessionVerifyFailed so Stage F's log-tailer always sees
// one of these literals.
//
// Adding a member here is a reserved-schema change — Stage F's
// grok parser MUST learn the new category in lockstep.
var sessionVerifyFailedReasons = map[string]bool{
	"prefix":    true,
	"signature": true,
	"expired":   true,
	"scope":     true,
	"unknown":   true,
}

// evictExpiredSessionNoncesLocked removes consumedSessionNonces
// entries whose stored expiry is past `now`. Caller MUST hold s.mu
// (write lock). Mirrors evictExpiredNoncesLocked's contract; kept
// separate so the bootstrap and session nonce-sets can have
// different TTLs / eviction cadence without one starving the other.
func (s *Service) evictExpiredSessionNoncesLocked(nowUnix int64) {
	for k, expiresAt := range s.consumedSessionNonces {
		if expiresAt.Unix() <= nowUnix {
			delete(s.consumedSessionNonces, k)
		}
	}
}

// nonceKey returns hex(HMAC-SHA256(processSalt, raw nonce bytes)).
// Used as the consumed-nonce-set key per Cerberus #1463 so a memory
// reader can't lift raw nonces from the map.
func (s *Service) nonceKey(nonce string) string {
	mac := core.HMAC("sha256", s.processSalt, []byte(nonce))
	if !mac.OK {
		// HMAC only fails on unsupported algorithm — sha256 always
		// works. Defensive fallback: a salted SHA-256 still scrambles
		// the input, so we don't degrade to raw nonce.
		sum := core.SHA256HexString(string(s.processSalt) + ":" + nonce)
		return sum
	}
	return core.HexEncode(mac.Value.([]byte))
}

// evictExpiredNoncesLocked removes consumed-nonce entries whose stored
// expiry is past `now`. Caller MUST hold s.mu (write lock). Keeps the
// map size bounded — a long-lived process minting tokens at 1/sec
// would otherwise accumulate ~120 entries steady-state, but a buggy
// caller minting at 1000/sec without verifying would balloon the map
// without bound.
func (s *Service) evictExpiredNoncesLocked(nowUnix int64) {
	for k, expiresAt := range s.consumedNonces {
		if expiresAt.Unix() <= nowUnix {
			delete(s.consumedNonces, k)
		}
	}
}

// numericClaim coerces an iat/exp value (which JSON unmarshal returns
// as float64 for untyped maps) into int64. Returns (0, false) for any
// non-numeric value so callers can branch on validity.
func numericClaim(header map[string]any, key string) (int64, bool) {
	v, ok := header[key]
	if !ok || v == nil {
		return 0, false
	}
	switch n := v.(type) {
	case float64:
		return int64(n), true
	case int:
		return int64(n), true
	case int64:
		return n, true
	}
	return 0, false
}

// canonicalise serialises a map into RFC-8785-style canonical JSON:
// sorted keys, no insignificant whitespace, UTF-8, compact JSON
// numbers. The output bytes are what the signer signs AND what the
// verifier re-derives before checking the signature (Cerberus #1469).
//
// Implementation: JSON-marshal each top-level value individually
// (CoreGO's json wraps encoding/json which preserves insertion order
// for map[string]any, not key order), then assemble with sorted keys.
// Today's tokens use a fixed flat schema (iat / exp / scope / nonce);
// the canon helper still walks generically so future scope additions
// don't need a code change.
//
// Usage example:
//
//	canon, r := canonicalise(map[string]any{
//	    "exp": 1700000060, "iat": 1700000000,
//	    "nonce": "abc", "scope": "account.create",
//	})
//	if r.OK { _ = canon }
func canonicalise(header map[string]any) ([]byte, core.Result) {
	keys := make([]string, 0, len(header))
	for k := range header {
		keys = append(keys, k)
	}
	core.SliceSort(keys)

	// Build the canonical form manually so we control whitespace +
	// key order. Each value is marshalled with the standard CoreGO
	// JSON encoder; for the primitives we use in tokens (int64,
	// string) the encoder produces canonical output verbatim.
	out := []byte("{")
	for i, k := range keys {
		if i > 0 {
			out = append(out, ',')
		}
		// Marshal the key as a JSON string (handles escapes uniformly).
		kJSON := core.JSONMarshal(k)
		if !kJSON.OK {
			return nil, core.Fail(core.E("serverkey.canonicalise", "marshal key", kJSON.Value.(error)))
		}
		out = append(out, kJSON.Value.([]byte)...)
		out = append(out, ':')
		vJSON := core.JSONMarshal(header[k])
		if !vJSON.OK {
			return nil, core.Fail(core.E("serverkey.canonicalise", "marshal value", vJSON.Value.(error)))
		}
		out = append(out, vJSON.Value.([]byte)...)
	}
	out = append(out, '}')
	return out, core.Ok(nil)
}

// encryptPrivate symmetrically wraps the armoured PGP private key with
// the passphrase via Enchantrix's SymmetricallyEncrypt. Returned bytes
// are the encrypted envelope (NOT armoured a second time — the PGP
// symmetric output is already a self-describing message).
func encryptPrivate(svc *pgp.Service, passphrase, privatePlaintext []byte) ([]byte, int, error) {
	ct, err := svc.SymmetricallyEncrypt(passphrase, privatePlaintext)
	if err != nil {
		return nil, 0, err
	}
	return ct, len(ct), nil
}

// marshalServerKey assembles the on-disk blob shape:
//
//	# lthn server-key v1
//	-----BEGIN LTHN PUBLIC KEY-----
//	<armoured pub bytes>
//	-----END LTHN PUBLIC KEY-----
//	-----BEGIN LTHN PRIVATE KEY (sym-encrypted)-----
//	<base64 cipher bytes>
//	-----END LTHN PRIVATE KEY-----
//
// The frame markers let future loaders unambiguously split the two
// segments without parsing the inner PGP armouring.
func marshalServerKey(pub, sealedPriv []byte) []byte {
	const (
		hdr      = "# lthn server-key v1\n"
		pubOpen  = "-----BEGIN LTHN PUBLIC KEY-----\n"
		pubClose = "\n-----END LTHN PUBLIC KEY-----\n"
		prvOpen  = "-----BEGIN LTHN PRIVATE KEY (sym-encrypted)-----\n"
		prvClose = "\n-----END LTHN PRIVATE KEY-----\n"
	)
	out := []byte(hdr)
	out = append(out, pubOpen...)
	out = append(out, pub...)
	out = append(out, pubClose...)
	out = append(out, prvOpen...)
	out = append(out, []byte(core.Base64URLEncode(sealedPriv))...)
	out = append(out, prvClose...)
	return out
}

// loadServerKey parses the on-disk blob and returns (publicArmoured,
// privateArmoured) where private is the decrypted PGP armoured block.
// Mode verification happens in the caller (Bootstrap) BEFORE this
// runs — loadServerKey is pure parse + decrypt.
func (s *Service) loadServerKey(path string, passphrase []byte) ([]byte, []byte, core.Result) {
	readR := core.ReadFile(path)
	if !readR.OK {
		return nil, nil, core.Fail(core.E("serverkey.loadServerKey", "read key file", readR.Value.(error)))
	}
	blob, _ := readR.Value.([]byte)

	const (
		pubOpen  = "-----BEGIN LTHN PUBLIC KEY-----\n"
		pubClose = "\n-----END LTHN PUBLIC KEY-----\n"
		prvOpen  = "-----BEGIN LTHN PRIVATE KEY (sym-encrypted)-----\n"
		prvClose = "\n-----END LTHN PRIVATE KEY-----\n"
	)
	bs := string(blob)

	pubStart := indexOf(bs, pubOpen)
	if pubStart < 0 {
		return nil, nil, core.Fail(core.NewError("serverkey.loadServerKey: missing public-key block"))
	}
	pubEnd := indexOf(bs, pubClose)
	if pubEnd < 0 || pubEnd <= pubStart {
		return nil, nil, core.Fail(core.NewError("serverkey.loadServerKey: malformed public-key block"))
	}
	pub := []byte(bs[pubStart+len(pubOpen) : pubEnd])

	prvStart := indexOf(bs, prvOpen)
	if prvStart < 0 {
		return nil, nil, core.Fail(core.NewError("serverkey.loadServerKey: missing private-key block"))
	}
	prvEnd := indexOf(bs[prvStart:], prvClose)
	if prvEnd < 0 {
		return nil, nil, core.Fail(core.NewError("serverkey.loadServerKey: malformed private-key block"))
	}
	prvB64 := bs[prvStart+len(prvOpen) : prvStart+prvEnd]
	sealedR := core.Base64URLDecode(prvB64)
	if !sealedR.OK {
		return nil, nil, core.Fail(core.E("serverkey.loadServerKey", "decode sealed private", sealedR.Value.(error)))
	}
	sealed, _ := sealedR.Value.([]byte)

	pgpSvc := pgp.NewService()
	plain, err := pgpSvc.SymmetricallyDecrypt(passphrase, sealed)
	if err != nil {
		return nil, nil, core.Fail(core.E("serverkey.loadServerKey", "decrypt private", err))
	}
	return pub, plain, core.Ok(nil)
}

// atomicWrite writes data to tmp + fsync + rename(tmp, path) so a
// crash mid-write doesn't leave a half-written sensitive file.
// Cerberus #1460 — partial state must not be reachable.
func atomicWrite(path string, data []byte, mode core.FileMode) core.Result {
	tmp := path + ".tmp"
	openR := core.OpenFile(tmp, core.O_CREATE|core.O_WRONLY|core.O_TRUNC, mode)
	if !openR.OK {
		return core.Fail(core.E("serverkey.atomicWrite", "open temp", openR.Value.(error)))
	}
	f, _ := openR.Value.(*core.OSFile)
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = core.Remove(tmp)
		return core.Fail(core.E("serverkey.atomicWrite", "write temp", err))
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = core.Remove(tmp)
		return core.Fail(core.E("serverkey.atomicWrite", "fsync temp", err))
	}
	if err := f.Close(); err != nil {
		_ = core.Remove(tmp)
		return core.Fail(core.E("serverkey.atomicWrite", "close temp", err))
	}
	if r := core.Rename(tmp, path); !r.OK {
		_ = core.Remove(tmp)
		return core.Fail(core.E("serverkey.atomicWrite", "rename temp", r.Value.(error)))
	}
	return core.Ok(nil)
}

// indexOf returns the byte offset of substr in s, or -1 when absent.
// Local helper so we don't add `strings` to the banned-import list.
// (core.Contains exists but no exported IndexOf today — file as a
// minor CoreGO export gap if needed long-term.)
func indexOf(s, substr string) int {
	n := len(substr)
	if n == 0 {
		return 0
	}
	if n > len(s) {
		return -1
	}
	for i := 0; i+n <= len(s); i++ {
		if s[i:i+n] == substr {
			return i
		}
	}
	return -1
}
