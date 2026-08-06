// SPDX-Licence-Identifier: EUPL-1.2

// bridge.go tests — lifecycle (OnStartup/OnShutdown/Port), the
// RegisterService port-defaulting rule, and subscribeToWebViewEvents'
// window-service discovery poll.
//
// Testability seam: subscribeTimeout/subscribePoll (unexported fields
// added to Service in bridge.go) let a test collapse the production
// 10s/50ms poll window to milliseconds. Without it, exercising the
// "window service never showed up" branch would cost a genuine 10s
// per test run; with it, both the found-immediately and the timeout
// branch are sub-second. No public API changed — RegisterService's
// signature and Options are untouched; the fields are zero-value
// (i.e. "use the production default") on every real construction
// path (RegisterService never sets them).
//
// OnStartup binds "127.0.0.1:0" (OS-assigned ephemeral port) — never
// a fixed port — per house rule.

package bridge

import (
	core "dappco.re/go"
	guiwindow "dappco.re/go/render/display/webkit/pkg/window"
)

// ─── RegisterService port defaulting ────────────────────────────────

func TestBridge_RegisterService_Good_ZeroPortDefaults(t *core.T) {
	c := core.New()
	r := RegisterService(Options{Port: 0})(c)
	core.AssertTrue(t, r.OK)
	svc := r.Value.(*Service)
	core.AssertEqual(t, DefaultPort, svc.Port())
}

func TestBridge_RegisterService_Good_ExplicitPortKept(t *core.T) {
	c := core.New()
	r := RegisterService(Options{Port: 54321})(c)
	core.AssertTrue(t, r.OK)
	svc := r.Value.(*Service)
	core.AssertEqual(t, 54321, svc.Port())
}

// ─── Port() ─────────────────────────────────────────────────────────

func TestBridge_Service_Port_Good_Direct(t *core.T) {
	s := &Service{port: 4321}
	core.AssertEqual(t, 4321, s.Port())
}

// ─── OnStartup / OnShutdown ─────────────────────────────────────────

func TestBridge_Service_OnStartup_Good_BindsEphemeralPortAndPersistsToken(t *core.T) {
	home := homeFixture(t)
	c := core.New()
	s := &Service{
		ServiceRuntime:   core.NewServiceRuntime[Options](c, Options{}),
		port:             0, // "127.0.0.1:0" — OS-assigned ephemeral port, never fixed.
		subscribeTimeout: 30 * core.Millisecond,
		subscribePoll:    5 * core.Millisecond,
	}

	r := s.OnStartup(core.Background())
	core.AssertTrue(t, r.OK)

	s.tokenMu.Lock()
	tok := s.token
	s.tokenMu.Unlock()
	core.AssertTrue(t, len(tok) >= TokenByteLength, "OnStartup must load/generate + hold the bearer token")

	tokenFile := core.PathJoin(home, "Lethean", "conf", TokenFileName)
	core.AssertTrue(t, core.Stat(tokenFile).OK, "OnStartup must persist the token file")

	// Give the ListenAndServe goroutine a moment to actually bind
	// before shutting down, so Shutdown races a live listener rather
	// than a not-yet-started one.
	core.Sleep(20 * core.Millisecond)

	shut := s.OnShutdown(core.Background())
	core.AssertTrue(t, shut.OK)
}

func TestBridge_Service_OnStartup_Bad_TokenDirUnwritable(t *core.T) {
	home := homeFixture(t)
	confDir := core.PathJoin(home, "Lethean", "conf")
	core.AssertTrue(t, core.MkdirAll(confDir, 0o755).OK)
	// Real fault injection: strip write permission on the token's
	// parent directory so the fresh-token WriteFile fails.
	core.AssertTrue(t, core.Chmod(confDir, 0o500).OK)
	t.Cleanup(func() { _ = core.Chmod(confDir, 0o755) })

	c := core.New()
	s := &Service{ServiceRuntime: core.NewServiceRuntime[Options](c, Options{}), port: 0}
	r := s.OnStartup(core.Background())
	core.AssertFalse(t, r.OK, "OnStartup must fail cleanly when the token file cannot be written")
}

func TestBridge_Service_OnShutdown_Good_NilServerIsNoOp(t *core.T) {
	s := &Service{}
	r := s.OnShutdown(core.Background())
	core.AssertTrue(t, r.OK)
}

// ─── subscribeToWebViewEvents ───────────────────────────────────────

// eventBindingPlatform wraps the exported MockPlatform (window/event
// state, WindowInfo plumbing) and adds BindCustomEvent so it also
// satisfies guiwindow.CustomEventBinder — MockPlatform deliberately
// doesn't, so bridge's SubscribeEvent calls would otherwise always
// return false here.
type eventBindingPlatform struct {
	*guiwindow.MockPlatform
	bound map[string]func(data any)
}

func newEventBindingPlatform() *eventBindingPlatform {
	return &eventBindingPlatform{MockPlatform: guiwindow.NewMockPlatform(), bound: map[string]func(data any){}}
}

func (p *eventBindingPlatform) BindCustomEvent(name string, cb func(data any)) {
	p.bound[name] = cb
}

func TestBridge_SubscribeToWebViewEvents_Good_FindsWindowServiceImmediately(t *core.T) {
	c := core.New()
	platform := newEventBindingPlatform()
	rw := guiwindow.Register(platform)(c)
	core.AssertTrue(t, rw.OK)
	winSvc := rw.Value.(*guiwindow.Service)
	core.AssertTrue(t, winSvc.OnStartup(core.Background()).OK)
	core.AssertTrue(t, c.RegisterService("window", winSvc).OK)

	s := &Service{
		ServiceRuntime:   core.NewServiceRuntime[Options](c, Options{}),
		port:             9999,
		subscribeTimeout: 500 * core.Millisecond,
		subscribePoll:    5 * core.Millisecond,
	}
	s.subscribeToWebViewEvents()

	core.AssertNotNil(t, platform.bound[eventConsoleName])
	core.AssertNotNil(t, platform.bound[eventErrorName])
	core.AssertNotNil(t, platform.bound[eventWebMCPToolsChanged])
}

func TestBridge_SubscribeToWebViewEvents_Bad_TimesOutWithNoWindowService(t *core.T) {
	c := core.New()
	s := &Service{
		ServiceRuntime:   core.NewServiceRuntime[Options](c, Options{}),
		port:             9999,
		subscribeTimeout: 30 * core.Millisecond,
		subscribePoll:    5 * core.Millisecond,
	}
	// Must return (not hang) once the shrunk deadline elapses — this
	// call would otherwise block for the production 10s default.
	s.subscribeToWebViewEvents()
}
