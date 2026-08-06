// SPDX-Licence-Identifier: EUPL-1.2

// Internal-package tests for runtime.go — proc, pickFreePort,
// waitForHealth, genBundleToken, startPlugin, stopPlugin, resolveToken.
// package plugin (white-box).
//
// Hermetic boundary: every test here either (a) never spawns an OS
// process at all, or (b) drives process.Service.StartWithOptions with a
// Command that CANNOT execute (missing / non-executable / malformed —
// real fault injection per the house rule), so cmd.Start() fails
// synchronously and nothing ever actually runs. waitForHealth is
// exercised against a real httptest.Server on loopback, which the
// house's hermetic list explicitly allows.
//
// startPlugin's post-spawn-success tail (health succeeds, proxy.Set,
// state="running", s.Core().Go(watchProcess)) is NOT covered here — it
// requires a genuinely running OS process with a working process.Done()
// channel (an unexported chan field only process.Service's own
// StartWithOptions can populate), so it is a structural exec boundary,
// not a gap in effort. Documented in the coverage report as a
// deliberate leave-out.

package plugin

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"

	core "dappco.re/go"
	"dappco.re/go/process"
)

// ─── proc ────────────────────────────────────────────────────────────────

func TestRuntime_proc_Bad_NilService(t *core.T) {
	var svc *Service
	core.AssertNil(t, svc.proc())
}

func TestRuntime_proc_Bad_ServiceRuntimeUnset(t *core.T) {
	svc := &Service{}
	core.AssertNil(t, svc.proc())
}

func TestRuntime_proc_Bad_ProcessServiceNotRegistered(t *core.T) {
	svc := newTestService(t, core.New())
	core.AssertNil(t, svc.proc())
}

func TestRuntime_proc_Good_ResolvesRegisteredProcessService(t *core.T) {
	c := core.New(core.WithService(process.Register))
	svc := newTestService(t, c)
	core.AssertNotNil(t, svc.proc())
}

// ─── pickFreePort ────────────────────────────────────────────────────────

func TestRuntime_pickFreePort_Good(t *core.T) {
	r := pickFreePort()
	core.RequireTrue(t, r.OK)
	port, ok := r.Value.(int)
	core.RequireTrue(t, ok)
	core.AssertGreater(t, port, 0)
}

// ─── genBundleToken ──────────────────────────────────────────────────────

func TestRuntime_genBundleToken_Good(t *core.T) {
	r := genBundleToken()
	core.RequireTrue(t, r.OK)
	secret, ok := r.Value.(string)
	core.RequireTrue(t, ok)
	core.AssertEqual(t, 64, len(secret), "32 random bytes hex-encoded is 64 chars")
}

func TestRuntime_genBundleToken_Ugly_SuccessiveCallsDiffer(t *core.T) {
	a := genBundleToken().Value.(string)
	b := genBundleToken().Value.(string)
	core.AssertNotEqual(t, a, b)
}

// ─── waitForHealth ───────────────────────────────────────────────────────

// healthServerPort starts an httptest.Server on loopback and returns its
// numeric port so waitForHealth (which takes a bare int port, not a URL)
// can be pointed at it — never a fixed port, always OS-assigned.
func healthServerPort(t *core.T, handler http.HandlerFunc) int {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	u, err := url.Parse(srv.URL)
	core.RequireTrue(t, err == nil)
	port, err := strconv.Atoi(u.Port())
	core.RequireTrue(t, err == nil, "server URL must carry a numeric port")
	return port
}

func TestRuntime_waitForHealth_Good_RespondsImmediately(t *core.T) {
	port := healthServerPort(t, func(w http.ResponseWriter, r *http.Request) {
		core.AssertEqual(t, "/opencode/health", r.URL.Path)
		w.WriteHeader(http.StatusOK)
	})
	r := waitForHealth(core.Background(), port, "/opencode/health", 2*core.Second)
	core.AssertTrue(t, r.OK)
}

func TestRuntime_waitForHealth_Bad_NeverHealthyTimesOut(t *core.T) {
	port := healthServerPort(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	r := waitForHealth(core.Background(), port, "/health", 250*core.Millisecond)
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "did not become healthy")
}

func TestRuntime_waitForHealth_Ugly_ContextCancelledBailsEarly(t *core.T) {
	ctx, cancel := core.WithCancel(core.Background())
	cancel()
	// Port doesn't even need to be listening — the cancellation check
	// runs before the first HTTP attempt.
	r := waitForHealth(ctx, 1, "/health", 5*core.Second)
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "context cancelled")
}

// ─── stopPlugin ──────────────────────────────────────────────────────────

func TestRuntime_stopPlugin_Good_NoOpWhenUntracked(t *core.T) {
	svc := newTestService(t, core.New())
	r := svc.stopPlugin("never-started")
	core.AssertTrue(t, r.OK)
}

func TestRuntime_stopPlugin_Good_NoOpWhenAlreadyStopped(t *core.T) {
	svc := newTestService(t, core.New())
	svc.state["x"] = &pluginState{state: "running"} // proc is nil -> "already stopped" shape
	r := svc.stopPlugin("x")
	core.RequireTrue(t, r.OK)
	// Early-return path: state deliberately left untouched.
	core.AssertEqual(t, "running", svc.state["x"].state)
}

// TestRuntime_stopPlugin_Good_FullTeardownWithoutRealProcess drives the
// entire "stop a tracked plugin" tail (revoke bundle token, drop the
// proxy mount, flip state to stopped, clear proc) via a pluginState
// whose processHandle wraps a nil *process.Process — stopPlugin's own
// guard (`ps2.proc.proc != nil`) means Kill is never invoked, so this
// needs no OS process at all.
func TestRuntime_stopPlugin_Good_FullTeardownWithoutRealProcess(t *core.T) {
	svc := newTestService(t, core.New())
	svc.state["x"] = &pluginState{
		state:        "running",
		bundleSecret: "sekrit",
		proc:         &processHandle{proc: nil, target: "http://127.0.0.1:1", port: 1},
	}
	svc.tokens["sekrit"] = "x"
	svc.proxy.Set("x", "http://127.0.0.1:1")

	r := svc.stopPlugin("x")
	core.RequireTrue(t, r.OK)
	core.AssertEqual(t, "stopped", svc.state["x"].state)
	core.AssertNil(t, svc.state["x"].proc)
	core.AssertFalse(t, svc.proxy.Has("x"), "proxy mount torn down")
	_, tokenStillThere := svc.tokens["sekrit"]
	core.AssertFalse(t, tokenStillThere, "bundle token revoked")
}

// ─── Start / Stop (exported wrappers) ────────────────────────────────────

func TestRuntime_Start_Good_AlreadyRunningIsNoOp(t *core.T) {
	svc := newTestService(t, core.New())
	svc.state["x"] = &pluginState{state: "running"}
	r := svc.Start("x")
	core.AssertTrue(t, r.OK)
}

func TestRuntime_Start_Bad_ProcessServiceUnavailable(t *core.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	dir := tmp + "/Lethean/conf/plugins/opencode"
	core.RequireTrue(t, core.MkdirAll(dir, 0o755).OK)
	core.RequireTrue(t, saveManifest(dir, Manifest{Code: "opencode", Name: "OpenCode", Binary: "bin/opencode"}).OK)

	svc := newTestService(t, core.New()) // no process.Register
	r := svc.Start("opencode")
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "process service unavailable")
}

func TestRuntime_Stop_Good_UntrackedIsNoOp(t *core.T) {
	svc := newTestService(t, core.New())
	r := svc.Stop("never-started")
	core.AssertTrue(t, r.OK)
}

// ─── startPlugin ─────────────────────────────────────────────────────────

func TestRuntime_startPlugin_Bad_ProcessServiceUnavailable(t *core.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	dir := tmp + "/Lethean/conf/plugins/opencode"
	core.RequireTrue(t, core.MkdirAll(dir, 0o755).OK)
	core.RequireTrue(t, saveManifest(dir, Manifest{Code: "opencode", Name: "OpenCode", Binary: "bin/opencode"}).OK)

	svc := newTestService(t, core.New()) // no process.Register
	r := svc.startPlugin(core.Background(), "opencode", "tok")
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "process service unavailable")
}

func TestRuntime_startPlugin_Bad_PluginDirUnresolvable(t *core.T) {
	t.Setenv("HOME", "")
	c := core.New(core.WithService(process.Register))
	svc := newTestService(t, c)
	r := svc.startPlugin(core.Background(), "opencode", "tok")
	core.AssertFalse(t, r.OK)
}

func TestRuntime_startPlugin_Bad_ManifestMissing(t *core.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	c := core.New(core.WithService(process.Register))
	svc := newTestService(t, c)
	r := svc.startPlugin(core.Background(), "never-installed", "tok")
	core.AssertFalse(t, r.OK)
}

// TestRuntime_startPlugin_Bad_SpawnFailsMissingExecutable is the
// "missing executable" fault-injection case: the manifest declares
// bin/opencode but no such file was ever written, so
// process.Service.StartWithOptions's cmd.Start() fails synchronously
// (ENOENT) — nothing ever executes.
func TestRuntime_startPlugin_Bad_SpawnFailsMissingExecutable(t *core.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	dir := tmp + "/Lethean/conf/plugins/opencode"
	core.RequireTrue(t, core.MkdirAll(dir, 0o755).OK)
	core.RequireTrue(t, saveManifest(dir, Manifest{Code: "opencode", Name: "OpenCode", Binary: "bin/opencode"}).OK)
	// Deliberately no bin/opencode file written.

	c := core.New(core.WithService(process.Register))
	svc := newTestService(t, c)
	r := svc.startPlugin(core.Background(), "opencode", "tok")
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "spawn:")
	core.AssertEqual(t, 0, len(svc.tokens), "bundle token rolled back on spawn failure")
}

// TestRuntime_startPlugin_Bad_SpawnFailsDeniedExecutable is the
// "denied executable" fault-injection case: bin/opencode exists but has
// no execute bit and isn't a valid executable format, so the OS refuses
// to run it — again nothing ever executes.
func TestRuntime_startPlugin_Bad_SpawnFailsDeniedExecutable(t *core.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	dir := tmp + "/Lethean/conf/plugins/opencode"
	core.RequireTrue(t, core.MkdirAll(dir+"/bin", 0o755).OK)
	core.RequireTrue(t, core.WriteFile(dir+"/bin/opencode", []byte("not an executable"), 0o644).OK)
	core.RequireTrue(t, saveManifest(dir, Manifest{Code: "opencode", Name: "OpenCode", Binary: "bin/opencode"}).OK)

	c := core.New(core.WithService(process.Register))
	svc := newTestService(t, c)
	r := svc.startPlugin(core.Background(), "opencode", "tok")
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "spawn:")
}

// ─── resolveToken ────────────────────────────────────────────────────────

// fakeTokenSource satisfies resolveToken's local `tokenSource` interface
// (Reveal() core.Result) structurally — Go's interface satisfaction is
// duck-typed at the call site, so a service registered under "apikey"
// from this test package is resolved by resolveToken exactly like the
// real apikey.Service would be, without pulling that package in.
type fakeTokenSource struct {
	result core.Result
}

func (f *fakeTokenSource) Reveal() core.Result { return f.result }

func TestRuntime_resolveToken_Bad_NoCore(t *core.T) {
	var svc *Service
	core.AssertEqual(t, "", svc.resolveToken())
}

func TestRuntime_resolveToken_Bad_NoTokenServiceRegistered(t *core.T) {
	svc := newTestService(t, core.New())
	core.AssertEqual(t, "", svc.resolveToken())
}

func TestRuntime_resolveToken_Good_ResolvesViaApikeyName(t *core.T) {
	c := core.New(core.WithName("apikey", func(c *core.Core) core.Result {
		return core.Ok(&fakeTokenSource{result: core.Ok("sk-lthn-good")})
	}))
	svc := newTestService(t, c)
	core.AssertEqual(t, "sk-lthn-good", svc.resolveToken())
}

func TestRuntime_resolveToken_Ugly_RevealFailsFallsThroughToEmpty(t *core.T) {
	c := core.New(core.WithName("apikey", func(c *core.Core) core.Result {
		return core.Ok(&fakeTokenSource{result: core.Fail(core.E("x", "no key yet", nil))})
	}))
	svc := newTestService(t, c)
	core.AssertEqual(t, "", svc.resolveToken())
}
