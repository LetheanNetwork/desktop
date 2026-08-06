// SPDX-Licence-Identifier: EUPL-1.2

// Tests for the desktop service. AX-7 triplet per public symbol:
// Test<File>_<Receiver>_<Method>_<Variant>. Each test avoids starting
// the Wails event loop — desktop.Run requires an active NSApp on macOS.
// The single-instance key path is exercised via the pkg/keys surface
// (the integration-level guarantee is that desktop.Run calls
// SingleInstanceKey and fails fast when Keys is nil + key unavailable).

package desktop_test

import (
	"testing/fstest"

	core "dappco.re/go"
	"dappco.re/go/crypt/keys"
	coreio "dappco.re/go/io"
	"dappco.re/lthn/desktop/pkg/connection"
	"dappco.re/lthn/desktop/pkg/desktop"
	"dappco.re/lthn/desktop/pkg/fleet"
	"dappco.re/lthn/desktop/pkg/keysvc"
	"dappco.re/lthn/desktop/pkg/modelruntime"
	"dappco.re/lthn/desktop/pkg/runner"
	"dappco.re/lthn/desktop/pkg/server"
	"dappco.re/lthn/desktop/pkg/services"
)

// keysFixture constructs a keys.Service under a temp HOME and
// wires a deterministic tier-0 KEK provider so SingleInstanceKey
// (tier-0 partition per RFC.stage-e-keys-partition v3 / Mantis
// #1625) can encrypt/decrypt without a real .seed substrate. The
// fixture supplies a fixed 32-byte KEK; production wiring is the
// HKDF(.seed, "lthn-keys-kek", "tier0-v1", 32) closure in
// cmd/lthn/app.go (E.K.C lane).
func keysFixture(t *core.T) *keys.Service {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	r := keysvc.New()
	core.AssertTrue(t, r.OK, "keys.New must succeed under temp HOME")
	svc := r.Value.(*keys.Service)
	tier0KEK := make([]byte, 32)
	for i := range tier0KEK {
		tier0KEK[i] = 0x42
	}
	svc.SetKEKProviderTier0(func() ([]byte, bool) { return tier0KEK, true })
	return svc
}

// --- Single-instance key round-trip (pkg/keys integration) ---

// TestDesktop_SingleInstanceKey_Fresh confirms that a fresh install
// generates a non-zero key and persists it.
func TestDesktop_SingleInstanceKey_Fresh(t *core.T) {
	svc := keysFixture(t)
	r := svc.SingleInstanceKey()
	core.AssertTrue(t, r.OK, "SingleInstanceKey must succeed on first call")
	key, ok := r.Value.([32]byte)
	core.AssertTrue(t, ok, "Value must be [32]byte")
	var zero [32]byte
	core.AssertNotEqual(t, zero, key, "generated key must not be all-zero")

	// Persisted — HasTier0("single-instance") must be true. The
	// single-instance Wails IPC key lives in the tier-0 partition
	// per RFC.stage-e-keys-partition v3 (Mantis #1625).
	hasR := svc.HasTier0("single-instance")
	core.AssertTrue(t, hasR.OK)
	core.AssertTrue(t, hasR.Value.(bool), "single-instance blob must be persisted after generation")
}

// TestDesktop_SingleInstanceKey_Reload confirms that a second boot
// reloads the same key without regenerating.
func TestDesktop_SingleInstanceKey_Reload(t *core.T) {
	svc := keysFixture(t)

	// Simulate first boot.
	r1 := svc.SingleInstanceKey()
	core.AssertTrue(t, r1.OK)
	key1 := r1.Value.([32]byte)

	// Simulate second boot — construct a fresh Service over the
	// same temp HOME. The tier-0 master + single-instance.t0.aead
	// are already on disk; re-wire the tier-0 KEK provider with
	// the SAME fixed KEK so the second Service can unwrap the
	// existing tier-0 master.
	svc2 := keysvc.New().Value.(*keys.Service)
	tier0KEK := make([]byte, 32)
	for i := range tier0KEK {
		tier0KEK[i] = 0x42
	}
	svc2.SetKEKProviderTier0(func() ([]byte, bool) { return tier0KEK, true })
	r2 := svc2.SingleInstanceKey()
	core.AssertTrue(t, r2.OK, "SingleInstanceKey must succeed on reload")
	key2 := r2.Value.([32]byte)

	core.AssertEqual(t, key1, key2, "key must be identical across reboots")
}

// --- Service.Run guard clauses -----------------------------------------
//
// windows_test.go already covers the Options.Server==nil guard. These
// three complete the early-return chain one dependency at a time, each
// stopping before the next guard so none of them reach the real Wails
// construction path (gui.NewService / newGUIRuntime / the blocking
// guiRuntime.Run — see the gui_runtime_test.go note on the same
// boundary).

func TestDesktop_Run_Bad_NoConnection(t *core.T) {
	s := desktop.NewService(desktop.Options{
		Server: server.NewService(server.Options{}),
	})
	r := s.Run()
	core.AssertFalse(t, r.OK, "Run() must Fail when Options.Connection is nil")
	core.AssertContains(t, r.Error(), "Connection is required")
}

func TestDesktop_Run_Bad_NoManagedServices(t *core.T) {
	s := desktop.NewService(desktop.Options{
		Server:     server.NewService(server.Options{}),
		Connection: connection.NewService(connection.Options{}),
		Core:       core.New(),
	})
	r := s.Run()
	core.AssertFalse(t, r.OK, "Run() must Fail when the services manager isn't registered on Core")
	core.AssertContains(t, r.Error(), "managed services are unavailable")
}

func TestDesktop_Run_Bad_NoModelRuntime(t *core.T) {
	manager := services.NewService(services.Options{})
	c := core.New(core.WithName("services", manager.Register))
	s := desktop.NewService(desktop.Options{
		Server:     server.NewService(server.Options{}),
		Connection: connection.NewService(connection.Options{}),
		Core:       c,
	})
	r := s.Run()
	core.AssertFalse(t, r.OK, "Run() must Fail when the model runtime isn't registered on Core")
	core.AssertContains(t, r.Error(), "model runtime is unavailable")
}

// TestDesktop_Run_Ugly_ServiceLockStopsBeforeWailsConstruction drives
// Run() all the way through the ~300-line service-resolution and
// wailsBindings/tray-menu assembly block — every dependency it
// resolves is either genuinely absent (Fleet, Keys, Runner: all
// guarded nil-checks in the source) or resolved via core.ServiceFor's
// own not-found path — and stops it one statement short of ever
// touching Wails. core.WithServiceLock() is the documented Core
// hardening feature (see dappco.re/go's lock.go / the enclave
// contract): once the registry is locked, Run()'s
// `s.opts.Core.RegisterService("gui", s.gui)` fails cleanly with a
// real "registry locked" Result, exactly like it would in a
// security-hardened boot sequence that locks composition before
// calling Run(). That failure fires BEFORE newGUIRuntime — so
// application.New, the Wails window/tray/menu registration, and the
// blocking event loop are never reached; this is the same safe
// boundary gui_runtime_test.go and window_platform_test.go already
// exercise (real *application.App, application.New() never followed
// by Run()), just approached from the Service.Run() side.
func TestDesktop_Run_Ugly_ServiceLockStopsBeforeWailsConstruction(t *core.T) {
	t.Setenv("HOME", t.TempDir())
	lemRoot := t.TempDir()
	manager := services.NewService(services.Options{})
	c := core.New(
		core.WithName("lem-io", coreio.NewService(coreio.IOConfig{Root: lemRoot})),
		core.WithName("services", manager.Register),
		core.WithName("modelruntime", modelruntime.Register),
		core.WithServiceLock(),
	)
	core.RequireTrue(t, c.Service("modelruntime").OK, "modelruntime must have registered before the lock test proceeds")

	s := desktop.NewService(desktop.Options{
		// Wails' asset file server stats the embedded index.html at
		// attachSPA time (not lazily) — a nil Frontend FS panics deep
		// inside wails' assetserver before this test reaches the
		// service-lock boundary it means to exercise.
		Frontend:   fstest.MapFS{"dist/index.html": &fstest.MapFile{Data: []byte("<html></html>")}},
		Server:     server.NewService(server.Options{}),
		Connection: connection.NewService(connection.Options{}),
		Core:       c,
	})
	r := s.Run()

	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "registry locked")
}

// --- Register / RegisterService (core.WithName factories) --------------

func TestDesktop_Register_Good_BindsCoreAndReturnsService(t *core.T) {
	c := core.New()
	r := desktop.Register(c)

	core.RequireTrue(t, r.OK, r.Error())
	_, ok := r.Value.(*desktop.Service)
	core.AssertTrue(t, ok)
}

// TestDesktop_RegisterService_Good_ResolvesEveryOptionalDependencyFromCore
// drives every "if opts.X == nil { resolve from Core }" branch true.
// Service.opts is unexported, so the only externally observable proof
// of resolution is behavioural: Run() against the resulting Service
// must clear the Server/Connection guards (both left nil in the
// Options{} passed to RegisterService) and fail on the next guard
// instead.
func TestDesktop_RegisterService_Good_ResolvesEveryOptionalDependencyFromCore(t *core.T) {
	t.Setenv("HOME", t.TempDir())
	backend := server.NewService(server.Options{})
	runnerSvc := runner.NewService(runner.Options{})
	fleetResult := fleet.New()
	core.RequireTrue(t, fleetResult.OK, fleetResult.Error())
	fleetSvc := fleetResult.Value.(*fleet.Service)
	t.Cleanup(func() { _ = fleetSvc.Close() })
	keysResult := keysvc.New()
	core.RequireTrue(t, keysResult.OK, keysResult.Error())
	keysSvc := keysResult.Value.(*keys.Service)
	connectionSvc := connection.NewService(connection.Options{})

	c := core.New(
		core.WithName("server", func(*core.Core) core.Result { return core.Ok(backend) }),
		core.WithName("runner", func(*core.Core) core.Result { return core.Ok(runnerSvc) }),
		core.WithName("fleet", func(*core.Core) core.Result { return core.Ok(fleetSvc) }),
		core.WithName("keys", func(*core.Core) core.Result { return core.Ok(keysSvc) }),
		core.WithName("connection", func(*core.Core) core.Result { return core.Ok(connectionSvc) }),
	)

	registerResult := desktop.RegisterService(desktop.Options{})(c)
	core.RequireTrue(t, registerResult.OK, registerResult.Error())
	s := registerResult.Value.(*desktop.Service)

	runResult := s.Run()
	core.AssertFalse(t, runResult.OK)
	core.AssertContains(t, runResult.Error(), "managed services are unavailable")
}

// TestDesktop_RegisterService_Ugly_ExplicitServerSkipsCoreDiscovery
// exercises the "already set" skip path — Options.Server is provided,
// so RegisterService's `if opts.Server == nil` branch never fires even
// though no "server" service exists on Core to resolve from.
func TestDesktop_RegisterService_Ugly_ExplicitServerSkipsCoreDiscovery(t *core.T) {
	c := core.New()
	backend := server.NewService(server.Options{})
	r := desktop.RegisterService(desktop.Options{Server: backend})(c)

	core.RequireTrue(t, r.OK, r.Error())
}
