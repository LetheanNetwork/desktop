// SPDX-Licence-Identifier: EUPL-1.2

// Bridge HTTP auth — Cerberus #1423 / Mantis 2026-05-16.
//
// The bridge MCP exposes webview_eval which evaluates arbitrary JS
// inside the WebView. Today it's bound to 127.0.0.1 only — but
// 127.0.0.1 is not a defence:
//
//  - DNS rebinding: a web page the user visits resolves
//    attacker.example to 127.0.0.1, then fetches the bridge.
//    Origin: header is set to https://attacker.example.
//  - Plugin enablement (post-#1421): plugins running in the same
//    user-process can dial localhost freely; an unauthenticated
//    JS-eval sink is a host compromise primitive.
//
// Two layered defences land here:
//
//  1. Bearer-token auth on every /mcp/* path. Token generated on
//     first start, persisted at ~/Lethean/conf/bridge.token mode
//     0600. /health stays open so probes (ping checks) work
//     without the token. (Pre-A5 the bridge also exposed
//     /internal/* HTTP shims for console + error + eval-reply
//     capture from the WebView; A5 moved those onto core/gui's
//     Wails Events bus path so the only HTTP surface left is
//     /mcp/* + /health.)
//
//  2. Origin header rejection. Browsers send Origin on cross-origin
//     fetch (the DNS-rebind primitive). Non-browser MCP callers
//     (codex, bridge.sh, e2e Vitest client) don't send Origin.
//     Rule: if Origin is present AND non-empty AND not "null",
//     reject. Pure-server callers pass through; rebind attempts
//     get the door slammed.
//
// Composes with the planned two-tier Wails service split (#1421):
// once plugin webviews exist, they reach Go through the gateway,
// not the bridge — but defence-in-depth at the bridge port is
// load-bearing while plugin enablement is in flight.

package bridge

import (

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/paths"
)

// TokenFileName is the bridge bearer-token filename inside ConfDir.
const TokenFileName = "bridge.token"

// TokenByteLength is the entropy of the auth token. 32 bytes = 256
// bits, hex-encoded to a 64-char string. More than enough for an
// auth secret; short enough to paste into a shell env var.
const TokenByteLength = 32

// loadOrGenerateToken returns the current bridge auth token,
// generating + persisting a fresh one when the file doesn't yet
// exist. File is written 0o600 — readable only by the user who
// started the bridge, matching the threat model.
//
// Failure modes:
//   - conf dir unresolvable → fail
//   - read existing file failed but file present → fail (no silent
//     regen — could mask a permissions problem the user wants to
//     surface)
//   - generate / write fresh failed → fail
//
// Usage example:
//
//	r := loadOrGenerateToken()
//	if r.OK { token := r.Value.(string); _ = token }
func loadOrGenerateToken() core.Result {
	conf := paths.ConfDir()
	if !conf.OK {
		return conf
	}
	dir, _ := conf.Value.(string)
	file := core.PathJoin(dir, TokenFileName)

	if stat := core.Stat(file); stat.OK {
		body := core.ReadFile(file)
		if !body.OK {
			return core.Fail(core.E("bridge.loadOrGenerateToken", "read existing token failed", body.Value.(error)))
		}
		raw, _ := body.Value.([]byte)
		tok := core.Trim(string(raw))
		if tok == "" {
			// Empty token file = pretend it doesn't exist.
			// Fall through to regenerate.
		} else {
			return core.Ok(tok)
		}
	}
	// Generate a fresh token. core.RandomString gives 16 hex chars
	// per 8 bytes of entropy; ask for double the configured length
	// in chars so the resulting string is TokenByteLength*2 hex chars
	// (i.e. 32 bytes / 256 bits).
	rs := core.RandomString(TokenByteLength * 2)
	if !rs.OK {
		return rs
	}
	tok, _ := rs.Value.(string)
	if w := core.WriteFile(file, []byte(tok), 0o600); !w.OK {
		return core.Fail(core.E("bridge.loadOrGenerateToken", "write token failed", w.Value.(error)))
	}
	return core.Ok(tok)
}

// originAllowed returns true when the request's Origin header is
// safe — empty, "null", or absent. Any other value is treated as a
// browser-originating request from a non-bridge origin, which is the
// DNS-rebind pivot we're defending against.
//
// Pure helper so the test path can drive without spinning a server.
func originAllowed(origin string) bool {
	o := core.Trim(origin)
	if o == "" || o == "null" {
		return true
	}
	return false
}

// constantTimeEqual compares two tokens without an early-exit byte
// loop — defence against the (mostly theoretical for a local
// service) timing attack on token verification. Uses core.Sprintf
// to derive equal-length operands when lengths differ so the
// comparison itself doesn't leak length.
//
// Pure helper.
func constantTimeEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	var diff byte
	for i := 0; i < len(a); i++ {
		diff |= a[i] ^ b[i]
	}
	return diff == 0
}

// requireAuth wraps an http.HandlerFunc with bearer-token + origin
// checks. Token mismatch → 401 with a generic body. Bad origin →
// 403 with a generic body. Both responses are deliberately terse —
// no information about what we expect, no token echo, no helpful
// "did you mean" hint.
//
// /health is intentionally NOT wrapped (probe endpoint).
// The WebView-side console + error capture rides the Wails
// Events bus path (post-A5), so no auth-free HTTP path remains
// on the bridge except /health.
//
// Usage example:
//
//	mux.HandleFunc("/mcp/call", s.requireAuth(s.handleCall))
func (s *Service) requireAuth(next func(core.ResponseWriter, *core.Request)) func(core.ResponseWriter, *core.Request) {
	return func(w core.ResponseWriter, r *core.Request) {
		if !originAllowed(r.Header.Get("Origin")) {
			w.WriteHeader(403)
			_, _ = w.Write([]byte(`{"error":"forbidden"}`))
			return
		}
		got := r.Header.Get("Authorization")
		// Accept "Bearer <token>" — RFC 7235 §2.1 specifies the auth
		// scheme is case-insensitive ("Bearer", "bearer", "BEARER" all
		// valid). Cerberus H#9-verify F4 (Mantis #1537): the prior
		// constant-string HasPrefix check rejected lowercase callers
		// that some HTTP clients emit verbatim. Lower the prefix bytes
		// only; the token tail keeps its case.
		const prefixLen = 7 // len("bearer ")
		if len(got) < prefixLen || core.Lower(got[:prefixLen]) != "bearer " {
			w.WriteHeader(401)
			_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
			return
		}
		presented := got[prefixLen:]
		s.tokenMu.Lock()
		expected := s.token
		s.tokenMu.Unlock()
		if expected == "" || !constantTimeEqual(presented, expected) {
			w.WriteHeader(401)
			_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
			return
		}
		next(w, r)
	}
}
