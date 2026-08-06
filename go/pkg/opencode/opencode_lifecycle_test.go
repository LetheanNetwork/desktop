// SPDX-Licence-Identifier: EUPL-1.2

// opencode_lifecycle_test.go — Start / Stop / Inspect / Status /
// waitHealthy / applyProfile coverage. The happy-path tests point a
// real httptest.Server (standing in for opencode-serve inside the
// "container") at the exact host port allocatePort() picks, by
// overriding the package's own pickPortInRange / portProbe test seams
// (already used by opencode_test.go's allocatePort suite) — so
// waitHealthy's real HTTP polling loop hits our fake server instead of
// a real docker-bound port. `docker run` / `docker rm` themselves are
// faked via fakeRuntime (Options.Runtime), never invoking a real
// container runtime.

package opencode

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync"
	"testing"

	core "dappco.re/go"
)

// fakeOpencodeServe starts an httptest.Server answering GET
// /global/health and PATCH /global/config the way opencode-serve
// does. healthStatus / configStatus let callers inject failure
// shapes. Captures the last PATCH body for assertion.
type fakeOpencodeServe struct {
	*httptest.Server
	mu            sync.Mutex
	lastConfigPUT string
	healthStatus  int
	configStatus  int
}

func newFakeOpencodeServe(t *testing.T) *fakeOpencodeServe {
	t.Helper()
	f := &fakeOpencodeServe{healthStatus: http.StatusOK, configStatus: http.StatusOK}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/global/health":
			w.WriteHeader(f.healthStatus)
		case r.URL.Path == "/global/config":
			b, _ := io.ReadAll(r.Body)
			f.mu.Lock()
			f.lastConfigPUT = string(b)
			f.mu.Unlock()
			w.WriteHeader(f.configStatus)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(f.Close)
	return f
}

// pinPortAllocation overrides pickPortInRange / portProbe so
// allocatePort() deterministically returns the port srv is already
// bound to (portProbe would otherwise correctly report it busy).
// Restores both on test cleanup.
func pinPortAllocation(t *testing.T, srv *httptest.Server) int {
	t.Helper()
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse httptest URL: %v", err)
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatalf("parse httptest port: %v", err)
	}
	origPick, origProbe := pickPortInRange, portProbe
	pickPortInRange = func() int { return port }
	portProbe = func(int) error { return nil }
	t.Cleanup(func() {
		pickPortInRange = origPick
		portProbe = origProbe
	})
	return port
}

// TestOpencode_Start_HappyPath_Good — full Start() success path: port
// pinned to a fake opencode-serve, docker run/rm faked via
// Options.Runtime, health check + config PATCH both succeed. Returns
// a non-empty sandbox id; Inspect + Status agree afterwards.
func TestOpencode_Start_HappyPath_Good(t *testing.T) {
	fake := newFakeOpencodeServe(t)
	pinPortAllocation(t, fake.Server)

	rt := fakeRuntime(t, "exit 0")
	svc := newTestService(t, Options{Runtime: rt})

	r := svc.Start("")
	if !r.OK {
		t.Fatalf("Start failed: %s", r.Error())
	}
	id, _ := r.Value.(string)
	if id == "" {
		t.Fatalf("Start returned empty id")
	}

	fake.mu.Lock()
	gotConfig := fake.lastConfigPUT
	fake.mu.Unlock()
	if gotConfig == "" {
		t.Errorf("PATCH /global/config never received a body")
	}

	inspectR := svc.Inspect(id)
	if !inspectR.OK {
		t.Fatalf("Inspect(%s) failed: %s", id, inspectR.Error())
	}
	sb, _ := inspectR.Value.(Sandbox)
	if sb.Status != StatusRunning {
		t.Errorf("Inspect.Status = %q; want %q", sb.Status, StatusRunning)
	}

	statusR := svc.Status()
	if !statusR.OK {
		t.Fatalf("Status failed: %s", statusR.Error())
	}
	running, _ := statusR.Value.([]Sandbox)
	if len(running) != 1 || running[0].ID != id {
		t.Errorf("Status = %+v; want exactly [%s] running", running, id)
	}
}

// TestOpencode_Start_ProfileApplyFails_Ugly — health check succeeds
// but the PATCH /global/config call 4xx's. Start still succeeds (the
// profile-narrowing PATCH is best-effort — a misbehaving apply must
// not strand a healthy sandbox record).
func TestOpencode_Start_ProfileApplyFails_Ugly(t *testing.T) {
	fake := newFakeOpencodeServe(t)
	fake.configStatus = http.StatusBadRequest
	pinPortAllocation(t, fake.Server)

	rt := fakeRuntime(t, "exit 0")
	svc := newTestService(t, Options{Runtime: rt})

	r := svc.Start("")
	if !r.OK {
		t.Fatalf("Start should tolerate a failed profile PATCH, got Fail: %s", r.Error())
	}
}

// TestOpencode_Start_ProcessUnavailable_Bad — a zero-value Service has
// no process.Service backing (proc() returns nil); Start must fail
// fast with the documented error, never panic.
func TestOpencode_Start_ProcessUnavailable_Bad(t *testing.T) {
	svc := &Service{}
	r := svc.Start("")
	if r.OK {
		t.Fatalf("Start on a bare Service returned OK; want Fail")
	}
	if !core.Contains(r.Error(), "process service unavailable") {
		t.Errorf("error = %q; want 'process service unavailable'", r.Error())
	}
}

// TestOpencode_Start_UnknownProfile_Bad — a profile name that was
// never saved must fail at the GetProfile lookup, before any docker
// side effect.
func TestOpencode_Start_UnknownProfile_Bad(t *testing.T) {
	rt := fakeRuntime(t, "echo 'fake runtime should never run' >&2; exit 1")
	svc := newTestService(t, Options{Runtime: rt})

	r := svc.Start("does-not-exist")
	if r.OK {
		t.Fatalf("Start with an unknown profile returned OK; want Fail")
	}
}

// TestOpencode_Start_PortExhausted_Bad — every port probe reports
// busy; Start must surface allocatePort's "port range exhausted"
// failure rather than proceeding to spawn a container.
func TestOpencode_Start_PortExhausted_Bad(t *testing.T) {
	origProbe := portProbe
	portProbe = func(int) error { return core.E("test", "address already in use", nil) }
	t.Cleanup(func() { portProbe = origProbe })

	rt := fakeRuntime(t, "echo 'fake runtime should never run' >&2; exit 1")
	svc := newTestService(t, Options{Runtime: rt})

	r := svc.Start("")
	if r.OK {
		t.Fatalf("Start with all ports busy returned OK; want Fail")
	}
	if !core.Contains(r.Error(), "port range exhausted") {
		t.Errorf("error = %q; want 'port range exhausted'", r.Error())
	}
}

// TestOpencode_Start_SaveFails_Bad — no orm Medium mounted (storage
// backend absent). Start must clean up the just-spawned container
// (best-effort `docker rm -f` via the fake runtime) and surface the
// Save failure rather than leaving an orphaned, untracked container.
func TestOpencode_Start_SaveFails_Bad(t *testing.T) {
	fake := newFakeOpencodeServe(t)
	pinPortAllocation(t, fake.Server)

	rt := fakeRuntime(t, "exit 0")

	resetKV(t)
	c := newTestCoreNoORM(t)
	r := NewService(Options{Runtime: rt})(c)
	if !r.OK {
		t.Fatalf("NewService failed: %s", r.Error())
	}
	svc := r.Value.(*Service)

	startR := svc.Start("")
	if startR.OK {
		t.Fatalf("Start with no orm Medium mounted returned OK; want Fail")
	}
}

// TestOpencode_Start_HealthCheckNeverSucceeds_Bad — opencode-serve
// never returns 200 on /global/health. waitHealthy is driven directly
// with a short timeout (its timeout is a parameter, not hardcoded) so
// this stays fast; Start()'s own 30s-hardcoded call to waitHealthy is
// deliberately NOT exercised end-to-end here — see the file-level
// leave-out note in opencode_test.go's companion coverage report.
func TestOpencode_WaitHealthy_NeverHealthy_Bad(t *testing.T) {
	fake := newFakeOpencodeServe(t)
	fake.healthStatus = http.StatusServiceUnavailable

	r := waitHealthy(fake.Server.URL, "", 900*core.Millisecond)
	if r.OK {
		t.Fatalf("waitHealthy against a permanently-503 server returned OK; want Fail")
	}
	if !core.Contains(r.Error(), "did not become healthy") {
		t.Errorf("error = %q; want 'did not become healthy'", r.Error())
	}
}

// TestOpencode_WaitHealthy_Good — direct unit test of the polling
// primitive: a server that answers 200 immediately is healthy on the
// first probe.
func TestOpencode_WaitHealthy_Good(t *testing.T) {
	fake := newFakeOpencodeServe(t)
	r := waitHealthy(fake.Server.URL, "", 2*core.Second)
	if !r.OK {
		t.Fatalf("waitHealthy failed against a healthy server: %s", r.Error())
	}
}

// TestOpencode_ApplyProfile_Good — direct unit test: a 2xx PATCH
// response is Ok and the wire body is the profile's ToOpenCodeWire
// JSON.
func TestOpencode_ApplyProfile_Good(t *testing.T) {
	fake := newFakeOpencodeServe(t)
	p := DefaultLthnProfile()
	r := applyProfile(fake.Server.URL, "", p)
	if !r.OK {
		t.Fatalf("applyProfile failed: %s", r.Error())
	}
	fake.mu.Lock()
	got := fake.lastConfigPUT
	fake.mu.Unlock()
	if got != p.ToOpenCodeWire() {
		t.Errorf("PATCH body = %q; want %q", got, p.ToOpenCodeWire())
	}
}

// TestOpencode_ApplyProfile_UpstreamError_Bad — a 4xx/5xx response
// from opencode-serve surfaces as Fail carrying the status code + body.
func TestOpencode_ApplyProfile_UpstreamError_Bad(t *testing.T) {
	fake := newFakeOpencodeServe(t)
	fake.configStatus = http.StatusInternalServerError
	r := applyProfile(fake.Server.URL, "", DefaultLthnProfile())
	if r.OK {
		t.Fatalf("applyProfile against a 500 upstream returned OK; want Fail")
	}
	if !core.Contains(r.Error(), "500") {
		t.Errorf("error = %q; want to mention status 500", r.Error())
	}
}

// TestOpencode_Stop_HappyPath_Good — Stop against a previously-Started
// sandbox marks it Stopped, removes the proxy entry, and the fake
// `docker rm -f` runs without error.
func TestOpencode_Stop_HappyPath_Good(t *testing.T) {
	fake := newFakeOpencodeServe(t)
	pinPortAllocation(t, fake.Server)

	rt := fakeRuntime(t, "exit 0")
	svc := newTestService(t, Options{Runtime: rt})

	startR := svc.Start("")
	if !startR.OK {
		t.Fatalf("Start failed: %s", startR.Error())
	}
	id, _ := startR.Value.(string)

	stopR := svc.Stop(id)
	if !stopR.OK {
		t.Fatalf("Stop failed: %s", stopR.Error())
	}

	inspectR := svc.Inspect(id)
	if !inspectR.OK {
		t.Fatalf("Inspect after Stop failed: %s", inspectR.Error())
	}
	sb, _ := inspectR.Value.(Sandbox)
	if sb.Status != StatusStopped {
		t.Errorf("Status after Stop = %q; want %q", sb.Status, StatusStopped)
	}
	if svc.proxy.Has(id) {
		t.Errorf("proxy still has target for %s after Stop", id)
	}
}

// TestOpencode_Stop_EmptyID_Bad — Stop rejects an empty id before any
// process/orm side effect.
func TestOpencode_Stop_EmptyID_Bad(t *testing.T) {
	svc := &Service{}
	r := svc.Stop("  ")
	if r.OK {
		t.Fatalf("Stop('') returned OK; want Fail")
	}
}

// TestOpencode_Stop_ProcessUnavailable_Bad — mirrors Start's guard.
func TestOpencode_Stop_ProcessUnavailable_Bad(t *testing.T) {
	svc := &Service{}
	r := svc.Stop("oc-doesnotexist")
	if r.OK {
		t.Fatalf("Stop on a bare Service returned OK; want Fail")
	}
	if !core.Contains(r.Error(), "process service unavailable") {
		t.Errorf("error = %q; want 'process service unavailable'", r.Error())
	}
}

// TestOpencode_Inspect_EmptyID_Bad / NotFound — id validation + the
// not-found path through the orm.
func TestOpencode_Inspect_EmptyID_Bad(t *testing.T) {
	svc := &Service{}
	r := svc.Inspect(" ")
	if r.OK {
		t.Fatalf("Inspect('') returned OK; want Fail")
	}
}

func TestOpencode_Inspect_NotFound_Bad(t *testing.T) {
	svc := newTestService(t, Options{})
	r := svc.Inspect("oc-never-started")
	if r.OK {
		t.Fatalf("Inspect on an unknown id returned OK; want Fail")
	}
}

// TestOpencode_Status_EmptyWhenNothingRunning_Good — a fresh Service
// with no sandboxes ever started returns an empty (not nil-panic)
// slice.
func TestOpencode_Status_EmptyWhenNothingRunning_Good(t *testing.T) {
	svc := newTestService(t, Options{})
	r := svc.Status()
	if !r.OK {
		t.Fatalf("Status failed: %s", r.Error())
	}
	running, _ := r.Value.([]Sandbox)
	if len(running) != 0 {
		t.Errorf("Status = %+v; want empty", running)
	}
}

// TestOpencode_ServiceAccessors — NewService / Register / ServiceName /
// ProxyGroup / SetOnSandboxChange / fireSandboxChange / runtime / image
// / requireSignature, all in one pass since each is a one-line
// accessor with a nil-safe guard.
func TestOpencode_ServiceAccessors(t *testing.T) {
	svc := newTestService(t, Options{Image: "custom/image:tag", Runtime: "podman", UpgradeRequireSignature: true})
	if svc.ServiceName() != "OpenCode" {
		t.Errorf("ServiceName() = %q; want OpenCode", svc.ServiceName())
	}
	if svc.ProxyGroup() == nil {
		t.Errorf("ProxyGroup() returned nil")
	}
	if got := svc.image(); got != "custom/image:tag" {
		t.Errorf("image() = %q; want custom/image:tag", got)
	}
	if got := svc.runtime(); got != "podman" {
		t.Errorf("runtime() = %q; want podman", got)
	}
	if !svc.requireSignature() {
		t.Errorf("requireSignature() = false; want true")
	}

	fired := 0
	svc.SetOnSandboxChange(func() { fired++ })
	svc.fireSandboxChange()
	if fired != 1 {
		t.Errorf("fireSandboxChange callback fired %d times; want 1", fired)
	}

	// Default runtime/image fall back when unset.
	bare := newTestService(t, Options{})
	if got := bare.runtime(); got != "docker" {
		t.Errorf("default runtime() = %q; want docker", got)
	}
	if got := bare.image(); got != defaultImage {
		t.Errorf("default image() = %q; want %q", got, defaultImage)
	}
	if bare.requireSignature() {
		t.Errorf("default requireSignature() = true; want false")
	}

	// nil-safe guards.
	var nilSvc *Service
	nilSvc.SetOnSandboxChange(func() {})
	nilSvc.fireSandboxChange()
	if nilSvc.proc() != nil {
		t.Errorf("nil Service.proc() should be nil")
	}
	if (&Service{}).requireSignature() {
		t.Errorf("zero Service{}.requireSignature() should be false")
	}
}

// TestOpencode_Register_Good — the Core-registration factory wraps
// NewService(Options{}) verbatim.
func TestOpencode_Register_Good(t *testing.T) {
	resetKV(t)
	c := newTestCore(t)
	r := Register(c)
	if !r.OK {
		t.Fatalf("Register failed: %s", r.Error())
	}
	if _, ok := r.Value.(*Service); !ok {
		t.Fatalf("Register value = %T; want *Service", r.Value)
	}
}
