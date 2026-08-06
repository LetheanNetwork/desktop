// SPDX-Licence-Identifier: EUPL-1.2

// Internal-package tests for plugin.go — the Service lifecycle
// (NewService, Register, OnStartup, OnShutdown), the coreApp / statusFor
// / installRoot / pluginDir helpers, and the per-bundle token registry.
// package plugin (white-box): several of these are unexported, and the
// state-manipulation tests construct pluginState/processHandle values
// directly rather than driving a real process spawn — Cerberus-style
// hermetic testing per the house rule (never execute anything real).
//
// newTestService is the shared Service constructor every other internal
// test file in this package (runtime_test.go, supervisor_test.go,
// wails_test.go, menus_test.go) reuses.

package plugin

import (
	core "dappco.re/go"
)

// newTestService builds a *Service directly against c, bypassing
// core.WithService's registry (NewService doesn't require registration —
// ServiceRuntime.Core() just returns the stored *Core). Callers that
// need s.proc() to resolve register process.Register on c themselves.
func newTestService(t *core.T, c *core.Core) *Service {
	t.Helper()
	r := NewService(Options{})(c)
	core.RequireTrue(t, r.OK)
	svc, ok := r.Value.(*Service)
	core.RequireTrue(t, ok)
	return svc
}

// ─── NewService / Register ──────────────────────────────────────────────

func TestPlugin_NewService_Good_ConstructsEmptyState(t *core.T) {
	c := core.New()
	svc := newTestService(t, c)
	core.AssertNotNil(t, svc.proxy)
	core.AssertEqual(t, 0, len(svc.state))
	core.AssertEqual(t, 0, len(svc.tokens))
}

func TestPlugin_Register_Good(t *core.T) {
	c := core.New()
	r := Register(c)
	core.RequireTrue(t, r.OK)
	_, ok := r.Value.(*Service)
	core.AssertTrue(t, ok)
}

// ─── OnStartup / OnShutdown ─────────────────────────────────────────────

func TestPlugin_Service_OnStartup_Good_NoOp(t *core.T) {
	svc := newTestService(t, core.New())
	r := svc.OnStartup(core.Background())
	core.AssertTrue(t, r.OK)
}

func TestPlugin_Service_OnShutdown_Good_EmptyStateNoOp(t *core.T) {
	svc := newTestService(t, core.New())
	r := svc.OnShutdown(core.Background())
	core.AssertTrue(t, r.OK)
}

// TestPlugin_Service_OnShutdown_Good_StopsTrackedPluginsWithoutRealProcess
// constructs pluginState entries whose processHandle.proc is nil — the
// hermetic seam stopPlugin already exposes (Kill is only called when
// ps2.proc.proc != nil), so OnShutdown's fan-out over every tracked
// plugin is exercised without spawning anything.
func TestPlugin_Service_OnShutdown_Good_StopsTrackedPluginsWithoutRealProcess(t *core.T) {
	svc := newTestService(t, core.New())
	svc.state["a"] = &pluginState{state: "running", proc: &processHandle{port: 1}}
	svc.state["b"] = &pluginState{state: "running", proc: &processHandle{port: 2}}
	r := svc.OnShutdown(core.Background())
	core.RequireTrue(t, r.OK)
	core.AssertEqual(t, "stopped", svc.state["a"].state)
	core.AssertEqual(t, "stopped", svc.state["b"].state)
}

// ─── coreApp ─────────────────────────────────────────────────────────────

func TestPlugin_coreApp_Bad_NilService(t *core.T) {
	var svc *Service
	core.AssertNil(t, svc.coreApp())
}

func TestPlugin_coreApp_Bad_NilServiceRuntime(t *core.T) {
	svc := &Service{}
	core.AssertNil(t, svc.coreApp())
}

func TestPlugin_coreApp_Good(t *core.T) {
	c := core.New()
	svc := newTestService(t, c)
	core.AssertNotNil(t, svc.coreApp())
}

// ─── ProxyGroup ──────────────────────────────────────────────────────────

func TestPlugin_Service_ProxyGroup_Good_ReturnsThePermanentGroup(t *core.T) {
	svc := newTestService(t, core.New())
	core.AssertSame(t, svc.proxy, svc.ProxyGroup())
}

// ─── statusFor ───────────────────────────────────────────────────────────

func TestPlugin_statusFor_Good_UntrackedReturnsStoppedStub(t *core.T) {
	svc := newTestService(t, core.New())
	st := svc.statusFor("nope")
	core.AssertEqual(t, "nope", st.Code)
	core.AssertEqual(t, "stopped", st.State)
	core.AssertEqual(t, 0, st.Port)
}

func TestPlugin_statusFor_Good_TrackedReturnsFullProjection(t *core.T) {
	svc := newTestService(t, core.New())
	now := core.Now()
	svc.state["x"] = &pluginState{
		manifest:  Manifest{Name: "OpenCode", Version: "1.0", Namespace: "oc"},
		state:     "running",
		port:      4321,
		pid:       999,
		startedAt: now,
		lastError: "",
	}
	st := svc.statusFor("x")
	core.AssertEqual(t, "x", st.Code)
	core.AssertEqual(t, "OpenCode", st.Name)
	core.AssertEqual(t, "1.0", st.Version)
	core.AssertEqual(t, "oc", st.Namespace)
	core.AssertEqual(t, "running", st.State)
	core.AssertEqual(t, 4321, st.Port)
	core.AssertEqual(t, 999, st.PID)
}

// ─── installRoot / pluginDir ─────────────────────────────────────────────

func TestPlugin_installRoot_Good(t *core.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	r := installRoot()
	core.RequireTrue(t, r.OK)
	core.AssertEqual(t, tmp+"/Lethean/conf/plugins", r.Value.(string))
}

// TestPlugin_installRoot_Bad_HomeUnavailable — real fault injection:
// os.UserHomeDir() on Unix reads $HOME directly and errors when it's
// unset/empty, so clearing it deterministically triggers the "home dir
// unavailable" branch without touching the real environment.
func TestPlugin_installRoot_Bad_HomeUnavailable(t *core.T) {
	t.Setenv("HOME", "")
	r := installRoot()
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "home dir unavailable")
}

func TestPlugin_pluginDir_Good(t *core.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	r := pluginDir("opencode")
	core.RequireTrue(t, r.OK)
	core.AssertEqual(t, tmp+"/Lethean/conf/plugins/opencode", r.Value.(string))
}

func TestPlugin_pluginDir_Bad_InstallRootFails(t *core.T) {
	t.Setenv("HOME", "")
	r := pluginDir("opencode")
	core.AssertFalse(t, r.OK)
}

// ─── per-bundle token registry (Cerberus #1443) ─────────────────────────

func TestPlugin_registerBundleToken_revokeBundleToken_Good_RoundTrip(t *core.T) {
	svc := newTestService(t, core.New())
	svc.registerBundleToken("opencode", "sekrit")
	code, ok := svc.LookupBundleCode("sekrit")
	core.RequireTrue(t, ok)
	core.AssertEqual(t, "opencode", code)
	svc.revokeBundleToken("sekrit")
	_, ok = svc.LookupBundleCode("sekrit")
	core.AssertFalse(t, ok)
}

func TestPlugin_revokeBundleToken_Ugly_IdempotentOnMissingSecret(t *core.T) {
	svc := newTestService(t, core.New())
	svc.revokeBundleToken("never-registered") // must not panic
	core.AssertEqual(t, 0, len(svc.tokens))
}

func TestPlugin_LookupBundleCode_Bad_EmptySecret(t *core.T) {
	svc := newTestService(t, core.New())
	code, ok := svc.LookupBundleCode("")
	core.AssertFalse(t, ok)
	core.AssertEqual(t, "", code)
}

func TestPlugin_LookupBundleCode_Bad_UnknownSecret(t *core.T) {
	svc := newTestService(t, core.New())
	_, ok := svc.LookupBundleCode("does-not-exist")
	core.AssertFalse(t, ok)
}
