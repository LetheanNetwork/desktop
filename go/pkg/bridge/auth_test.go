// SPDX-Licence-Identifier: EUPL-1.2

// Cerberus #1423 / Mantis 2026-05-16 — auth.go unit tests.
//
// Coverage:
//   - constantTimeEqual: equal / different-length / single-bit
//   - originAllowed: empty / "null" / set
//   - loadOrGenerateToken: persists on first call, returns same on
//     second, mode is 0o600, regenerates after manual file removal
//
// requireAuth integration testing lives at the HTTP layer (calling
// /mcp/info on a started Service with + without the header); covered
// in the bridge_test.go file alongside the rest of the wiring.

package bridge

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/paths"
)

func homeFixture(t *core.T) string {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	return tmp
}

func TestBridge_ConstantTimeEqual_Match(t *core.T) {
	core.AssertTrue(t, constantTimeEqual("abc123", "abc123"))
}

func TestBridge_ConstantTimeEqual_DifferentLength(t *core.T) {
	core.AssertFalse(t, constantTimeEqual("abc", "abc123"))
	core.AssertFalse(t, constantTimeEqual("", "x"))
}

func TestBridge_ConstantTimeEqual_SingleByteFlip(t *core.T) {
	core.AssertFalse(t, constantTimeEqual("abc123", "abc124"))
}

func TestBridge_ConstantTimeEqual_BothEmpty(t *core.T) {
	core.AssertTrue(t, constantTimeEqual("", ""))
}

func TestBridge_OriginAllowed_Empty(t *core.T) {
	core.AssertTrue(t, originAllowed(""))
}

func TestBridge_OriginAllowed_Null(t *core.T) {
	core.AssertTrue(t, originAllowed("null"))
}

func TestBridge_OriginAllowed_Whitespace(t *core.T) {
	core.AssertTrue(t, originAllowed("   "))
}

func TestBridge_OriginAllowed_HttpsSet(t *core.T) {
	core.AssertFalse(t, originAllowed("https://attacker.example"))
}

func TestBridge_OriginAllowed_HttpSet(t *core.T) {
	core.AssertFalse(t, originAllowed("http://localhost:9879"))
}

func TestBridge_OriginAllowed_FileSet(t *core.T) {
	// file:// origins from a bookmark / saved page are still
	// "set" — DNS rebind doesn't reach via file: but better safe.
	core.AssertFalse(t, originAllowed("file://"))
}

func TestBridge_LoadOrGenerateToken_FirstCall(t *core.T) {
	homeFixture(t)
	r := loadOrGenerateToken()
	core.AssertTrue(t, r.OK, "first call must succeed")
	tok, _ := r.Value.(string)
	core.AssertTrue(t, len(tok) >= TokenByteLength,
		"generated token must be at least TokenByteLength chars")

	// File exists at the right path with the right mode.
	conf := paths.ConfDir().Value.(string)
	stat := core.Stat(core.PathJoin(conf, TokenFileName))
	core.AssertTrue(t, stat.OK)
	info := stat.Value.(core.FsFileInfo)
	core.AssertEqual(t, 0o600, int(info.Mode().Perm()),
		"token file must be mode 0o600")
}

func TestBridge_LoadOrGenerateToken_SecondCallReturnsSame(t *core.T) {
	homeFixture(t)
	a := loadOrGenerateToken().Value.(string)
	b := loadOrGenerateToken().Value.(string)
	core.AssertEqual(t, a, b,
		"second call must return the persisted token, not regenerate")
}

func TestBridge_LoadOrGenerateToken_RegeneratesAfterRemoval(t *core.T) {
	homeFixture(t)
	a := loadOrGenerateToken().Value.(string)

	conf := paths.ConfDir().Value.(string)
	core.AssertTrue(t, core.Remove(core.PathJoin(conf, TokenFileName)).OK)

	b := loadOrGenerateToken().Value.(string)
	core.AssertNotEqual(t, a, b,
		"removing the token file must produce a fresh token on next call")
}

func TestBridge_LoadOrGenerateToken_EmptyFileRegenerates(t *core.T) {
	homeFixture(t)
	// Pre-plant an empty token file (simulates corrupt state).
	conf := paths.ConfDir().Value.(string)
	file := core.PathJoin(conf, TokenFileName)
	core.AssertTrue(t, core.WriteFile(file, []byte(""), 0o600).OK)

	r := loadOrGenerateToken()
	core.AssertTrue(t, r.OK)
	tok, _ := r.Value.(string)
	core.AssertTrue(t, len(tok) >= TokenByteLength,
		"empty token file must trigger regeneration")
}

// requireAuth bearer-prefix tests — Cerberus H#9-verify F4 (Mantis
// #1537). RFC 7235 §2.1 specifies HTTP auth scheme names are case-
// insensitive; the prior strict "Bearer " HasPrefix check rejected
// lowercase callers ("bearer abc") that some HTTP libraries emit
// verbatim. The fix lowers the scheme bytes for comparison while
// preserving the token tail case.

// authFixture wires up a minimal Service with a known bearer token
// installed, so requireAuth can be invoked directly without a live
// HTTP server. Returns the service + the token string.
func authFixture(t *core.T, tok string) *Service {
	t.Helper()
	s := &Service{}
	s.tokenMu.Lock()
	s.token = tok
	s.tokenMu.Unlock()
	return s
}

func TestRequireAuth_AcceptsLowercaseBearer_Good(t *core.T) {
	s := authFixture(t, "tok-abc-123")
	called := false
	handler := s.requireAuth(func(w core.ResponseWriter, r *core.Request) {
		called = true
		w.WriteHeader(200)
	})
	req := core.NewHTTPTestRequest(core.MethodGet, "/mcp/info", nil)
	req.Header.Set("Authorization", "bearer tok-abc-123")
	rec := core.NewHTTPTestRecorder()
	handler(rec, req)
	core.AssertEqual(t, 200, rec.Code, "lowercase 'bearer' must be accepted (RFC 7235)")
	core.AssertTrue(t, called, "next handler must run on valid lowercase bearer")
}

func TestRequireAuth_AcceptsUppercaseBearer_Good(t *core.T) {
	s := authFixture(t, "tok-abc-123")
	handler := s.requireAuth(func(w core.ResponseWriter, _ *core.Request) {
		w.WriteHeader(200)
	})
	req := core.NewHTTPTestRequest(core.MethodGet, "/mcp/info", nil)
	req.Header.Set("Authorization", "BEARER tok-abc-123")
	rec := core.NewHTTPTestRecorder()
	handler(rec, req)
	core.AssertEqual(t, 200, rec.Code, "uppercase 'BEARER' must be accepted (RFC 7235)")
}

func TestRequireAuth_AcceptsCanonicalBearer_Good(t *core.T) {
	s := authFixture(t, "tok-abc-123")
	handler := s.requireAuth(func(w core.ResponseWriter, _ *core.Request) {
		w.WriteHeader(200)
	})
	req := core.NewHTTPTestRequest(core.MethodGet, "/mcp/info", nil)
	req.Header.Set("Authorization", "Bearer tok-abc-123")
	rec := core.NewHTTPTestRecorder()
	handler(rec, req)
	core.AssertEqual(t, 200, rec.Code, "canonical 'Bearer' must still work after F4 fix")
}

func TestRequireAuth_RejectsMissingHeader_Bad(t *core.T) {
	s := authFixture(t, "tok-abc-123")
	handler := s.requireAuth(func(w core.ResponseWriter, _ *core.Request) {
		w.WriteHeader(200)
	})
	req := core.NewHTTPTestRequest(core.MethodGet, "/mcp/info", nil)
	rec := core.NewHTTPTestRecorder()
	handler(rec, req)
	core.AssertEqual(t, 401, rec.Code, "missing Authorization header must 401")
}

func TestRequireAuth_RejectsNonBearerScheme_Bad(t *core.T) {
	s := authFixture(t, "tok-abc-123")
	handler := s.requireAuth(func(w core.ResponseWriter, _ *core.Request) {
		w.WriteHeader(200)
	})
	req := core.NewHTTPTestRequest(core.MethodGet, "/mcp/info", nil)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	rec := core.NewHTTPTestRecorder()
	handler(rec, req)
	core.AssertEqual(t, 401, rec.Code, "non-bearer scheme (Basic) must 401")
}

func TestRequireAuth_RejectsBadTokenCaseSensitiveTail_Ugly(t *core.T) {
	// The scheme is case-insensitive; the token tail is NOT.
	s := authFixture(t, "tok-abc-123")
	handler := s.requireAuth(func(w core.ResponseWriter, _ *core.Request) {
		w.WriteHeader(200)
	})
	req := core.NewHTTPTestRequest(core.MethodGet, "/mcp/info", nil)
	req.Header.Set("Authorization", "bearer TOK-ABC-123")
	rec := core.NewHTTPTestRecorder()
	handler(rec, req)
	core.AssertEqual(t, 401, rec.Code, "token-tail comparison must stay case-sensitive")
}

func TestRequireAuth_RejectsShortHeader_Ugly(t *core.T) {
	// "Bear" — shorter than the prefix length; must not panic.
	s := authFixture(t, "tok-abc-123")
	handler := s.requireAuth(func(w core.ResponseWriter, _ *core.Request) {
		w.WriteHeader(200)
	})
	req := core.NewHTTPTestRequest(core.MethodGet, "/mcp/info", nil)
	req.Header.Set("Authorization", "Bear")
	rec := core.NewHTTPTestRecorder()
	handler(rec, req)
	core.AssertEqual(t, 401, rec.Code, "header shorter than 'bearer ' must 401 cleanly (no panic)")
}

// requireOrigin tests — Cerberus H#9-verify F1 (Mantis #1534).
// /internal/* endpoints can't enforce bearer auth (the in-WebView
// JS shim has no access to the bridge token and we deliberately do
// not leak it into the eval closure), so we apply Origin-only auth
// as defence-in-depth alongside each endpoint's own capability
// (eval-reply's per-call reqID with 2^128 keyspace).

func TestRequireOrigin_AllowsEmptyOrigin_Good(t *core.T) {
	// The in-process WebView posts WITHOUT Origin (null/file:
	// origins omit it on same-origin fetches), so the middleware
	// must pass empty-Origin requests through.
	s := authFixture(t, "tok-ignored")
	called := false
	handler := s.requireOrigin(func(w core.ResponseWriter, _ *core.Request) {
		called = true
		w.WriteHeader(200)
	})
	req := core.NewHTTPTestRequest(core.MethodPost, "/internal/eval-reply", nil)
	rec := core.NewHTTPTestRecorder()
	handler(rec, req)
	core.AssertEqual(t, 200, rec.Code, "WebView shim (no Origin) must pass through")
	core.AssertTrue(t, called, "next handler must run when Origin is absent")
}

func TestRequireOrigin_AllowsNullOrigin_Good(t *core.T) {
	// Some embedded WebViews report Origin: "null"; treat as safe.
	s := authFixture(t, "tok-ignored")
	handler := s.requireOrigin(func(w core.ResponseWriter, _ *core.Request) {
		w.WriteHeader(200)
	})
	req := core.NewHTTPTestRequest(core.MethodPost, "/internal/eval-reply", nil)
	req.Header.Set("Origin", "null")
	rec := core.NewHTTPTestRecorder()
	handler(rec, req)
	core.AssertEqual(t, 200, rec.Code, "'null' Origin must pass through")
}

func TestRequireOrigin_RejectsBrowserOrigin_Bad(t *core.T) {
	// DNS-rebind primitive: attacker.example resolved to 127.0.0.1,
	// browser sets Origin: https://attacker.example. Must be 403'd.
	s := authFixture(t, "tok-ignored")
	called := false
	handler := s.requireOrigin(func(w core.ResponseWriter, _ *core.Request) {
		called = true
		w.WriteHeader(200)
	})
	req := core.NewHTTPTestRequest(core.MethodPost, "/internal/eval-reply", nil)
	req.Header.Set("Origin", "https://attacker.example")
	rec := core.NewHTTPTestRecorder()
	handler(rec, req)
	core.AssertEqual(t, 403, rec.Code, "browser-Origin requests must 403 (DNS-rebind defence)")
	core.AssertFalse(t, called, "next handler must NOT run when Origin check fails")
}

func TestRequireOrigin_RejectsLocalhostOrigin_Bad(t *core.T) {
	// Even http://localhost:9879 set as Origin by a browser fetch
	// is the rebind primitive shape — reject.
	s := authFixture(t, "tok-ignored")
	handler := s.requireOrigin(func(w core.ResponseWriter, _ *core.Request) {
		w.WriteHeader(200)
	})
	req := core.NewHTTPTestRequest(core.MethodPost, "/internal/eval-reply", nil)
	req.Header.Set("Origin", "http://localhost:9879")
	rec := core.NewHTTPTestRecorder()
	handler(rec, req)
	core.AssertEqual(t, 403, rec.Code, "any non-empty Origin must 403 — the WebView shim never sets one")
}

func TestRequireOrigin_DoesNotRequireBearer_Good(t *core.T) {
	// Critical contract: requireOrigin must NEVER consult the
	// Authorization header. The WebView shim has no token to send.
	s := authFixture(t, "tok-real")
	called := false
	handler := s.requireOrigin(func(w core.ResponseWriter, _ *core.Request) {
		called = true
		w.WriteHeader(200)
	})
	req := core.NewHTTPTestRequest(core.MethodPost, "/internal/eval-reply", nil)
	// Deliberately no Authorization header — and no Origin either.
	rec := core.NewHTTPTestRecorder()
	handler(rec, req)
	core.AssertEqual(t, 200, rec.Code,
		"requireOrigin must not require a bearer token (the WebView shim has none)")
	core.AssertTrue(t, called)
}
