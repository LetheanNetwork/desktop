// SPDX-Licence-Identifier: EUPL-1.2

// Format / claim / TTL edge cases for VerifyBootstrapToken and
// VerifySessionToken that the happy-path + tamper tests elsewhere
// never reach. Tokens are forged directly via
// ExportedBuildBootstrapToken / ExportedBuildSessionToken
// (export_test.go) — every check exercised here runs BEFORE the PGP
// signature is verified, so no valid signature is required; the
// signature bytes are deliberately garbage.

package serverkey_test

import (
	core "dappco.re/go"
	subject "dappco.re/lthn/desktop/pkg/serverkey"
)

func bootstrappedForge(t *core.T) *subject.Service {
	t.Helper()
	_ = homeFixture(t)
	svc := subject.NewService(nil)
	core.AssertTrue(t, svc.Bootstrap().OK, "fixture Bootstrap should succeed")
	return svc
}

// --- VerifyBootstrapToken ---

func TestToken_VerifyBootstrapToken_Bad_HeaderNotBase64(t *core.T) {
	svc := bootstrappedForge(t)
	token := "LTHN-BOOT-1." + "not@@valid$$base64" + "." + "sig"
	r := svc.VerifyBootstrapToken(token, "account.create")
	core.AssertTrue(t, !r.OK, "malformed base64 header segment must be rejected")
}

func TestToken_VerifyBootstrapToken_Bad_SignatureNotBase64(t *core.T) {
	svc := bootstrappedForge(t)
	validHeader := core.Base64URLEncode([]byte(`{"scope":"account.create"}`))
	token := "LTHN-BOOT-1." + validHeader + "." + "not@@valid$$base64"
	r := svc.VerifyBootstrapToken(token, "account.create")
	core.AssertTrue(t, !r.OK, "malformed base64 signature segment must be rejected")
}

func TestToken_VerifyBootstrapToken_Bad_WrongSegmentCount(t *core.T) {
	svc := bootstrappedForge(t)
	token := "LTHN-BOOT-1." + "onlyonesegmentnodot"
	r := svc.VerifyBootstrapToken(token, "account.create")
	core.AssertTrue(t, !r.OK, "a token body without exactly two dot-separated segments must be rejected")
}

func TestToken_VerifyBootstrapToken_Bad_HeaderNotJSON(t *core.T) {
	svc := bootstrappedForge(t)
	notJSON := core.Base64URLEncode([]byte("this is not json"))
	token := "LTHN-BOOT-1." + notJSON + ".c2ln"
	r := svc.VerifyBootstrapToken(token, "account.create")
	core.AssertTrue(t, !r.OK, "a header that decodes but isn't valid JSON must be rejected")
}

func TestToken_VerifyBootstrapToken_Bad_MissingIat(t *core.T) {
	svc := bootstrappedForge(t)
	now := core.Now().UTC().Unix()
	header := map[string]any{
		"exp":   now + 60,
		"scope": "account.create",
		"nonce": "abc123",
	}
	forged := subject.ExportedBuildBootstrapToken(header, []byte("garbage-sig"))
	r := svc.VerifyBootstrapToken(forged, "account.create")
	core.AssertTrue(t, !r.OK, "a header without an iat claim must be rejected")
}

func TestToken_VerifyBootstrapToken_Bad_MissingExp(t *core.T) {
	svc := bootstrappedForge(t)
	now := core.Now().UTC().Unix()
	header := map[string]any{
		"iat":   now,
		"scope": "account.create",
		"nonce": "abc123",
	}
	forged := subject.ExportedBuildBootstrapToken(header, []byte("garbage-sig"))
	r := svc.VerifyBootstrapToken(forged, "account.create")
	core.AssertTrue(t, !r.OK, "a header without an exp claim must be rejected")
}

func TestToken_VerifyBootstrapToken_Bad_MissingNonce(t *core.T) {
	svc := bootstrappedForge(t)
	now := core.Now().UTC().Unix()
	header := map[string]any{
		"iat":   now,
		"exp":   now + 60,
		"scope": "account.create",
	}
	forged := subject.ExportedBuildBootstrapToken(header, []byte("garbage-sig"))
	r := svc.VerifyBootstrapToken(forged, "account.create")
	core.AssertTrue(t, !r.OK, "a header without a nonce claim must be rejected")
}

func TestToken_VerifyBootstrapToken_Bad_Expired(t *core.T) {
	svc := bootstrappedForge(t)
	now := core.Now().UTC().Unix()
	header := map[string]any{
		"iat":   now - 100,
		"exp":   now - 10, // already past — but well within the verifier TTL ceiling
		"scope": "account.create",
		"nonce": "abc123",
	}
	forged := subject.ExportedBuildBootstrapToken(header, []byte("garbage-sig"))
	r := svc.VerifyBootstrapToken(forged, "account.create")
	core.AssertTrue(t, !r.OK, "a token whose exp has already passed must be rejected")
}

// --- VerifySessionToken ---

func TestToken_VerifySessionToken_Bad_PrefixInvalid(t *core.T) {
	svc := bootstrappedForge(t)
	r := svc.VerifySessionToken("NOT-THE-RIGHT-PREFIX.abc.def")
	core.AssertTrue(t, !r.OK, "a token without the LTHN-SESS-1. prefix must be rejected")
}

func TestToken_VerifySessionToken_Bad_WrongSegmentCount(t *core.T) {
	svc := bootstrappedForge(t)
	r := svc.VerifySessionToken("LTHN-SESS-1." + "onlyonesegmentnodot")
	core.AssertTrue(t, !r.OK, "a token body without exactly two dot-separated segments must be rejected")
}

func TestToken_VerifySessionToken_Bad_HeaderNotBase64(t *core.T) {
	svc := bootstrappedForge(t)
	r := svc.VerifySessionToken("LTHN-SESS-1." + "not@@valid$$base64" + ".sig")
	core.AssertTrue(t, !r.OK, "malformed base64 header segment must be rejected")
}

func TestToken_VerifySessionToken_Bad_ScopeMismatch(t *core.T) {
	svc := bootstrappedForge(t)
	header := map[string]any{"scope": "not-session"}
	forged := subject.ExportedBuildSessionToken(header, []byte("garbage-sig"))
	r := svc.VerifySessionToken(forged)
	core.AssertTrue(t, !r.OK, "a header whose scope isn't \"session\" must be rejected")
}

func TestToken_VerifySessionToken_Bad_MissingIat(t *core.T) {
	svc := bootstrappedForge(t)
	now := core.Now().UTC().Unix()
	header := map[string]any{
		"scope":      "session",
		"exp":        now + 900,
		"account_id": "acct-1",
	}
	forged := subject.ExportedBuildSessionToken(header, []byte("garbage-sig"))
	r := svc.VerifySessionToken(forged)
	core.AssertTrue(t, !r.OK, "a session header without an iat claim must be rejected")
}

func TestToken_VerifySessionToken_Bad_MissingExp(t *core.T) {
	svc := bootstrappedForge(t)
	now := core.Now().UTC().Unix()
	header := map[string]any{
		"scope":      "session",
		"iat":        now,
		"account_id": "acct-1",
	}
	forged := subject.ExportedBuildSessionToken(header, []byte("garbage-sig"))
	r := svc.VerifySessionToken(forged)
	core.AssertTrue(t, !r.OK, "a session header without an exp claim must be rejected")
}

func TestToken_VerifySessionToken_Bad_MissingAccountID(t *core.T) {
	svc := bootstrappedForge(t)
	now := core.Now().UTC().Unix()
	header := map[string]any{
		"scope": "session",
		"iat":   now,
		"exp":   now + 900,
	}
	forged := subject.ExportedBuildSessionToken(header, []byte("garbage-sig"))
	r := svc.VerifySessionToken(forged)
	core.AssertTrue(t, !r.OK, "a session header without an account_id claim must be rejected")
}

func TestToken_VerifySessionToken_Bad_IatInFuture(t *core.T) {
	svc := bootstrappedForge(t)
	now := core.Now().UTC().Unix()
	header := map[string]any{
		"scope":      "session",
		"iat":        now + 3600, // far beyond the 5s clock-skew tolerance
		"exp":        now + 9999,
		"account_id": "acct-1",
	}
	forged := subject.ExportedBuildSessionToken(header, []byte("garbage-sig"))
	r := svc.VerifySessionToken(forged)
	core.AssertTrue(t, !r.OK, "a session token with iat far in the future must be rejected")
}

func TestToken_VerifySessionToken_Bad_Expired(t *core.T) {
	svc := bootstrappedForge(t)
	now := core.Now().UTC().Unix()
	header := map[string]any{
		"scope":      "session",
		"iat":        now - 100,
		"exp":        now - 10, // already past, but within the verifier ceiling
		"account_id": "acct-1",
	}
	forged := subject.ExportedBuildSessionToken(header, []byte("garbage-sig"))
	r := svc.VerifySessionToken(forged)
	core.AssertTrue(t, !r.OK, "a session token whose exp has already passed must be rejected")
}

func TestToken_VerifySessionToken_Bad_AgeExceedsVerifierCeiling(t *core.T) {
	// exp deliberately set far in the future so the "exp <= now" check
	// does NOT fire — only the "now - iat > sessionVerifierTTL"
	// absolute-ceiling check should trip.
	svc := bootstrappedForge(t)
	now := core.Now().UTC().Unix()
	header := map[string]any{
		"scope":      "session",
		"iat":        now - 10000,
		"exp":        now + 10000,
		"account_id": "acct-1",
	}
	forged := subject.ExportedBuildSessionToken(header, []byte("garbage-sig"))
	r := svc.VerifySessionToken(forged)
	core.AssertTrue(t, !r.OK, "a session token older than the verifier ceiling must be rejected even with exp in the future")
}
