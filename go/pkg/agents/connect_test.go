// SPDX-Licence-Identifier: EUPL-1.2

// Internal-package tests for connect.go — hubBearerToken,
// ClaudeConnectRecipe, resolveClaudePath, lastAbsPathLine,
// LaunchClaudeConnected. package agents (white-box).
//
// hubBearerToken needs a real, functioning *keys.Service (it's resolved
// by concrete type via core.ServiceFor[*keys.Service], not an
// interface, so it can't be faked structurally the way runtime_test.go
// faked resolveToken's local interface in pkg/plugin). newTestKeysService
// builds one entirely hermetically: keys.WithDir points it at
// t.TempDir() (never the real ~/Lethean/wallets tree) and a fixed
// 32-byte in-memory KEK stands in for the real seed-derived provider —
// no filesystem outside the temp dir, no network, no real secrets.
//
// LaunchClaudeConnected's "tab" case (terminal.Spawn) and the success
// tail of "window" (proc.Run("open", "-a", "Terminal", ...)) are NOT
// exercised — deliberate leave-outs. "tab" spawns a real PTY running
// whatever resolveClaudePath resolved (which, unresolved, literally
// falls back to the string "claude" — on a machine that has the Claude
// Code CLI installed, that could launch a real nested session).
// "window" shells out to `open -a Terminal`, which pops a real, visible
// Terminal.app window — never acceptable in an automated test. Both are
// exec/UI boundaries, not gaps in effort; the "window" case IS covered
// up to (but not including) that call, via the process-service-
// unavailable branch.
package agents

import (
	"bytes"

	core "dappco.re/go"
	"dappco.re/go/crypt/keys"
)

// fixedTestKEK is a deterministic, hermetic stand-in for the real
// seed-derived tier-0 KEK (which production wires from
// ~/Lethean/wallets/.seed via HKDF). Never used outside t.TempDir()
// fixtures.
var fixedTestKEK = bytes.Repeat([]byte{0x42}, 32)

// newTestKeysService constructs a real *keys.Service backed entirely by
// a temp directory, registers it on c under "keys" (the name
// hubBearerToken looks up), and wires a fixed in-memory tier-0 KEK
// provider so GetOrCreateTier0 can actually run end to end.
func newTestKeysService(t *core.T, c *core.Core) *keys.Service {
	t.Helper()
	tmp := t.TempDir()
	r := keys.Register(c, keys.WithDir(tmp))
	core.RequireTrue(t, r.OK)
	ks, ok := r.Value.(*keys.Service)
	core.RequireTrue(t, ok)
	ks.SetKEKProviderTier0(func() ([]byte, bool) { return fixedTestKEK, true })
	core.RequireTrue(t, c.RegisterService("keys", ks).OK)
	return ks
}

// ─── hubBearerToken ──────────────────────────────────────────────────────

func TestConnect_hubBearerToken_Bad_NilCore(t *core.T) {
	svc := &Service{}
	_, r := svc.hubBearerToken()
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "no core")
}

func TestConnect_hubBearerToken_Bad_KeysServiceUnavailable(t *core.T) {
	svc := &Service{core: core.New()} // no keys service registered
	_, r := svc.hubBearerToken()
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "keys service unavailable")
}

// TestConnect_hubBearerToken_Ugly_EmptyStoredTokenFails pre-seeds an
// empty tier-0 blob directly (bypassing generate — GetOrCreateTier0's
// get-if-exists shortcut returns it as-is) to drive hubBearerToken's
// "hub bearer is empty" defensive branch without depending on the real
// generator ever producing an empty string itself.
func TestConnect_hubBearerToken_Ugly_EmptyStoredTokenFails(t *core.T) {
	c := core.New()
	ks := newTestKeysService(t, c)
	core.RequireTrue(t, ks.PutTier0(connectAuthTokenRef, []byte("")).OK)
	svc := &Service{core: c}

	_, r := svc.hubBearerToken()
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "hub bearer is empty")
}

func TestConnect_hubBearerToken_Good(t *core.T) {
	c := core.New()
	newTestKeysService(t, c)
	svc := &Service{core: c}

	token, r := svc.hubBearerToken()
	core.RequireTrue(t, r.OK)
	core.AssertNotEmpty(t, token)

	// Idempotent — the second call reads the same persisted tier-0 blob
	// back rather than regenerating.
	token2, r2 := svc.hubBearerToken()
	core.RequireTrue(t, r2.OK)
	core.AssertEqual(t, token, token2)
}

// ─── ClaudeConnectRecipe ─────────────────────────────────────────────────

func TestConnect_ClaudeConnectRecipe_Bad_PropagatesTokenFailure(t *core.T) {
	svc := &Service{}
	r := svc.ClaudeConnectRecipe()
	core.AssertFalse(t, r.OK)
}

func TestConnect_ClaudeConnectRecipe_Good(t *core.T) {
	c := core.New()
	newTestKeysService(t, c)
	svc := &Service{core: c}

	r := svc.ClaudeConnectRecipe()
	core.RequireTrue(t, r.OK)
	recipe := r.Value.(ConnectRecipe)
	core.AssertNotEmpty(t, recipe.Token)
	core.AssertEqual(t, DefaultMCPURL+"/mcp", recipe.MCPURL)
	core.AssertEqual(t, 3, len(recipe.InstallCommands))
}

// ─── lastAbsPathLine (pure) ───────────────────────────────────────────────

func TestConnect_lastAbsPathLine_Good_PicksLastAbsoluteLine(t *core.T) {
	out := "some login-shell banner\n/opt/homebrew/bin/claude\n"
	core.AssertEqual(t, "/opt/homebrew/bin/claude", lastAbsPathLine(out))
}

func TestConnect_lastAbsPathLine_Bad_NoAbsoluteLine(t *core.T) {
	core.AssertEqual(t, "", lastAbsPathLine("not found\ncommand not found\n"))
}

func TestConnect_lastAbsPathLine_Ugly_MultipleCandidatesPicksLast(t *core.T) {
	out := "/usr/bin/env\nnoise\n/usr/local/bin/claude\n"
	core.AssertEqual(t, "/usr/local/bin/claude", lastAbsPathLine(out))
}

// ─── resolveClaudePath ────────────────────────────────────────────────────

// TestConnect_resolveClaudePath_Good_FoundUnderHome is the only
// deterministic branch: the first candidate ($HOME/.claude/local/claude)
// is checked before the two hardcoded absolute system paths, so
// planting the fixture there short-circuits before touching real host
// paths that may or may not exist on the machine running the test.
func TestConnect_resolveClaudePath_Good_FoundUnderHome(t *core.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	dir := tmp + "/.claude/local"
	core.RequireTrue(t, core.MkdirAll(dir, 0o755).OK)
	core.RequireTrue(t, core.WriteFile(dir+"/claude", []byte("#!/bin/sh\n"), 0o755).OK)

	svc := &Service{core: core.New()}
	got := svc.resolveClaudePath()
	core.AssertEqual(t, dir+"/claude", got)
}

// ─── LaunchClaudeConnected ────────────────────────────────────────────────

func TestConnect_LaunchClaudeConnected_Bad_PropagatesTokenFailure(t *core.T) {
	svc := &Service{}
	r := svc.LaunchClaudeConnected("tab")
	core.AssertFalse(t, r.OK)
}

func TestConnect_LaunchClaudeConnected_Bad_UnknownTarget(t *core.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	dir := tmp + "/.claude/local"
	core.RequireTrue(t, core.MkdirAll(dir, 0o755).OK)
	core.RequireTrue(t, core.WriteFile(dir+"/claude", []byte("#!/bin/sh\n"), 0o755).OK)

	c := core.New()
	newTestKeysService(t, c)
	svc := &Service{core: c}

	r := svc.LaunchClaudeConnected("carrier-pigeon")
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "unknown target")
}

// TestConnect_LaunchClaudeConnected_Bad_WindowProcessServiceUnavailable
// reaches the "window" case's first guard (proc.Service resolution)
// WITHOUT falling through to the real `open -a Terminal` call — no
// process.Service is registered on c, so ServiceFor fails before the
// script is ever written or Terminal is ever shelled out to.
func TestConnect_LaunchClaudeConnected_Bad_WindowProcessServiceUnavailable(t *core.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	dir := tmp + "/.claude/local"
	core.RequireTrue(t, core.MkdirAll(dir, 0o755).OK)
	core.RequireTrue(t, core.WriteFile(dir+"/claude", []byte("#!/bin/sh\n"), 0o755).OK)

	c := core.New() // deliberately no process.Register
	newTestKeysService(t, c)
	svc := &Service{core: c}

	r := svc.LaunchClaudeConnected("window")
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "process service unavailable")
}
