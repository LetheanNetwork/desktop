// SPDX-Licence-Identifier: EUPL-1.2

// dread_test.go — assertions per the Cerberus DREAD findings folded
// into RFC v2. Each test names the specific Mantis number so a
// future auditor can re-trace the security rationale from the test.

package serverkey_test

import (
	"os"
	"testing"

	core "dappco.re/go"
	subject "dappco.re/lthn/desktop/pkg/serverkey"
)

// --- Cerberus #1463 — nonce keying is HMAC(processSalt, nonce), not raw. ---

func TestServerkey_ConsumedNonceKeying_Cerberus1463(t *core.T) {
	_ = homeFixture(t)
	svc := subject.NewService(nil)
	core.AssertTrue(t, svc.Bootstrap().OK)

	out := svc.IssueBootstrapToken().Value.(subject.BootstrapTokenOutput)
	// Decode header to lift the raw nonce.
	parts := core.Split(out.Token[len("LTHN-BOOT-1."):], ".")
	core.AssertEqual(t, 2, len(parts))
	headerR := core.Base64URLDecode(parts[0])
	core.AssertTrue(t, headerR.OK)
	var header map[string]any
	core.AssertTrue(t, core.JSONUnmarshal(headerR.Value.([]byte), &header).OK)
	rawNonce, _ := header["nonce"].(string)
	core.AssertTrue(t, rawNonce != "", "header should carry a nonce")

	// Verify the token — the nonce moves into the consumed set.
	core.AssertTrue(t, svc.VerifyBootstrapToken(out.Token, "account.create").OK)

	// Inspect the consumed set: the key MUST be the HMAC-derived
	// digest, NOT the raw nonce string. Cerberus #1463 — a memory-
	// reader must not be able to lift in-flight nonces from the map.
	consumed := svc.ExportedConsumedNonces()
	wantKey := svc.ExportedNonceKey(rawNonce)
	_, found := consumed[wantKey]
	core.AssertTrue(t, found, "consumed-nonce map must hold HMAC(salt, nonce) key")
	_, foundRaw := consumed[rawNonce]
	core.AssertTrue(t, !foundRaw, "consumed-nonce map must NOT hold raw nonce key")
}

// --- Cerberus #1469 — canonicalise produces sorted-key, no-whitespace JSON. ---

func TestServerkey_Canonicalise_SortedKeys_Cerberus1469(t *core.T) {
	// Two equivalent maps in different insertion orders must produce
	// IDENTICAL canonical bytes. CoreGO map iteration is deterministic
	// per process but not across processes — this test asserts the
	// canonicaliser sorts keys regardless of input order.
	header1 := map[string]any{
		"scope": "account.create",
		"iat":   int64(1700000000),
		"exp":   int64(1700000060),
		"nonce": "abcdef",
	}
	header2 := map[string]any{
		"nonce": "abcdef",
		"exp":   int64(1700000060),
		"scope": "account.create",
		"iat":   int64(1700000000),
	}
	c1, r1 := subject.ExportedCanonicalise(header1)
	core.AssertTrue(t, r1.OK)
	c2, r2 := subject.ExportedCanonicalise(header2)
	core.AssertTrue(t, r2.OK)
	core.AssertEqual(t, string(c1), string(c2))

	// And the canonical output starts with the alphabetically-first
	// key — `exp` comes before `iat`/`nonce`/`scope`.
	core.AssertTrue(t, core.HasPrefix(string(c1), `{"exp":`),
		"canonical JSON must begin with alphabetically-first key (exp)")
}

func TestServerkey_Canonicalise_NoWhitespace_Cerberus1469(t *core.T) {
	header := map[string]any{
		"a": "x",
		"b": int64(42),
	}
	c, r := subject.ExportedCanonicalise(header)
	core.AssertTrue(t, r.OK)
	// No spaces, no newlines, no tabs.
	for _, b := range c {
		if b == ' ' || b == '\t' || b == '\n' || b == '\r' {
			t.Fatalf("canonical output must contain no insignificant whitespace, got byte %d", b)
		}
	}
}

// --- Cerberus #1466 — single-instance gate paths (both branches). ---

func TestServerkey_BootstrapLock_CoreGUIPath_Cerberus1466(t *core.T) {
	// When singleInstanceCheck returns true (CoreGUI already
	// serialised us), the file lock path is skipped — Bootstrap
	// must still succeed.
	_ = homeFixture(t)
	svc := subject.NewService(nil)
	svc.ExportedSetSingleInstanceCheck(func() bool { return true })
	r := svc.Bootstrap()
	core.AssertTrue(t, r.OK, "Bootstrap must succeed when CoreGUI gate confirms single-instance")
}

func TestServerkey_BootstrapLock_FlockPath_Cerberus1466(t *core.T) {
	// Default (no singleInstanceCheck) takes the file-lock branch.
	// Bootstrap must still succeed; this exercises the lock-acquire
	// path explicitly.
	_ = homeFixture(t)
	svc := subject.NewService(nil)
	svc.ExportedSetSingleInstanceCheck(func() bool { return false })
	r := svc.Bootstrap()
	core.AssertTrue(t, r.OK, "Bootstrap must succeed via flock fallback")
}

// --- Cerberus #1462 — TTL ceilings. ---

func TestServerkey_TokenTTL_IssuerExp_Cerberus1462(t *core.T) {
	// Issuer stamps exp = iat + 60. Round-trip the token + decode
	// the header to inspect the claims directly.
	_ = homeFixture(t)
	svc := subject.NewService(nil)
	core.AssertTrue(t, svc.Bootstrap().OK)

	out := svc.IssueBootstrapToken().Value.(subject.BootstrapTokenOutput)
	parts := core.Split(out.Token[len("LTHN-BOOT-1."):], ".")
	headerR := core.Base64URLDecode(parts[0])
	core.AssertTrue(t, headerR.OK)
	var header map[string]any
	core.AssertTrue(t, core.JSONUnmarshal(headerR.Value.([]byte), &header).OK)

	iat, _ := header["iat"].(float64)
	exp, _ := header["exp"].(float64)
	delta := int64(exp - iat)
	core.AssertEqual(t, int64(60), delta)
	core.AssertEqual(t, int64(exp), out.ExpiresAt)
}

// --- Cerberus #1470 — no CWD anywhere in the package source. ---

func TestServerkey_NoCWDInSource_Cerberus1470(t *core.T) {
	// Grep the production source files (NOT tests) for any CWD-style
	// dependency: core.Getwd, os.Getwd, /proc/<pid>/cwd literals.
	// The passphrase MUST derive from .seed, never from CWD.
	files := []string{
		"types.go",
		"service.go",
		"token.go",
		"wails.go",
	}
	for _, name := range files {
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		body := string(data)
		for _, marker := range []string{
			"core.Getwd",
			"os.Getwd",
			"/proc/self/cwd",
		} {
			if contains(body, marker) {
				t.Fatalf("Cerberus #1470 violation: %s contains forbidden marker %q", name, marker)
			}
		}
	}
}

// contains is a local substring check so the grep test stays free of
// banned strings imports. Faithful to strings.Contains semantics.
func contains(s, sub string) bool {
	if sub == "" {
		return true
	}
	if len(sub) > len(s) {
		return false
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// silence the testing import (already required for *core.T = *testing.T).
var _ = testing.Short
