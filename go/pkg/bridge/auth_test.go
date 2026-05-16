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
