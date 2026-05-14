// SPDX-Licence-Identifier: EUPL-1.2

// OPENCODE_SERVER_PASSWORD lifecycle — per RFC.opencode.md §4.3 +
// §7. One random password per lthn install (NOT per sandbox) — used
// as the env var on every spawned opencode-serve container AND as
// the credential lthn's own client calls send via HTTP Basic Auth.
//
// OpenCode-serve enforces auth when OPENCODE_SERVER_PASSWORD is set:
// the username defaults to "opencode" (override with
// OPENCODE_SERVER_USERNAME env, which lthn doesn't change), the
// password is the env value. Header format is the standard HTTP
// Basic — `Authorization: Basic base64("opencode:<pw>")`.
//
// Why one password per install (not per sandbox):
//   - It's a host-isolation control, not a per-tenant secret. The
//     threat model is "user-on-the-same-host shouldn't be able to
//     drive an unauthenticated opencode-serve", not "sandboxes
//     should be isolated from each other" (Docker network isolates
//     them on 127.0.0.1:<port> bound by us).
//   - Simpler reverse-proxy injection — one header, all sandboxes.

package opencode

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"net/http"

	core "dappco.re/go"
	goiostore "dappco.re/go/io/store"
)

const (
	// serverAuthStoreGroup is the DuckDB group under which the
	// per-install password lives.
	serverAuthStoreGroup = "opencode.server"
	// serverAuthPasswordKey is the key inside the group.
	serverAuthPasswordKey = "password"
	// serverAuthUsername matches opencode-serve's default — the
	// upstream's OPENCODE_SERVER_USERNAME defaults to "opencode" and
	// we don't override it (one less knob to keep in sync).
	serverAuthUsername = "opencode"
	// serverPasswordBytes is the random source width — 24 bytes
	// becomes a 48-char hex string. 192 bits of entropy is plenty
	// for a local-only auth secret.
	serverPasswordBytes = 24
)

// ServerPassword returns the persisted OPENCODE_SERVER_PASSWORD,
// generating + storing a new one on first call. Idempotent —
// subsequent calls return the same value.
//
// Usage example:
//
//	r := svc.ServerPassword()
//	if r.OK { pw := r.Value.(string); _ = pw }
func (s *Service) ServerPassword() core.Result {
	st, r := kv()
	if !r.OK {
		return r
	}
	if existing, err := st.Get(serverAuthStoreGroup, serverAuthPasswordKey); err == nil && existing != "" {
		return core.Ok(existing)
	} else if err != nil && !core.Is(err, goiostore.NotFoundError) {
		return core.Fail(err)
	}
	// First call ever — generate + persist.
	buf := make([]byte, serverPasswordBytes)
	if _, err := rand.Read(buf); err != nil {
		return core.Fail(core.E("opencode.ServerPassword", "rand read failed", err))
	}
	pw := hex.EncodeToString(buf)
	if err := st.Set(serverAuthStoreGroup, serverAuthPasswordKey, pw); err != nil {
		return core.Fail(err)
	}
	return core.Ok(pw)
}

// authHeader returns the HTTP Basic Auth header value lthn uses
// when calling opencode-serve. Format: "Basic base64(user:pw)".
//
// Returns empty string when password retrieval fails. Callers
// must handle empty (skip injection) so a transient KV failure
// doesn't bork the proxy.
func (s *Service) authHeader() string {
	r := s.ServerPassword()
	if !r.OK {
		return ""
	}
	pw, _ := r.Value.(string)
	if pw == "" {
		return ""
	}
	raw := serverAuthUsername + ":" + pw
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(raw))
}

// applyAuth sets the Authorization header on a request from the
// persisted server password. No-op when the header is empty.
func (s *Service) applyAuth(r *http.Request) {
	if h := s.authHeader(); h != "" {
		r.Header.Set("Authorization", h)
	}
}
