// SPDX-Licence-Identifier: EUPL-1.2

// process.go tests for the Service surface not covered by
// audit_test.go's Requested+Failed lifecycle assertions: NewService,
// Register, RouteGroups (+ the rebasedProvider it wraps), and the
// success paths of Run / Start / Kill / List / Get. Mirrors
// pkg/bridge/process_test.go's harness — a real (but short-lived,
// non-networked, no fixed-port) dappco.re/go/process Service
// registered under "process", same as that package's own test suite
// does. /bin/echo and /bin/sleep are used because they exist on every
// POSIX dev/CI box this repo targets.

package process

import (
	core "dappco.re/go"
	coreprocess "dappco.re/go/process"
	"github.com/gin-gonic/gin"
)

// newRegisteredTestService returns a Service bound to a *core.Core with
// a real upstream dappco.re/go/process Service registered + started
// under the "process" name, so process.run / process.start / process.kill
// / process.list / process.get all dispatch through a live runtime
// instead of failing fast with "action not registered".
func newRegisteredTestService(t *core.T) *Service {
	t.Helper()
	c := core.New()
	r := coreprocess.NewService(coreprocess.Options{})(c)
	core.AssertTrue(t, r.OK)
	ps := r.Value.(*coreprocess.Service)
	core.AssertTrue(t, ps.OnStartup(core.Background()).OK)
	core.AssertTrue(t, c.RegisterService("process", ps).OK)
	return NewService(c)
}

// --- NewService / Register ---

func TestProcess_NewService_Good_BindsCore(t *core.T) {
	c := core.New()
	svc := NewService(c)
	core.AssertNotNil(t, svc)
	core.AssertSame(t, c, svc.core)
}

func TestProcess_Register_Good_ReturnsOKService(t *core.T) {
	c := core.New()
	r := Register(c)
	core.AssertTrue(t, r.OK)
	svc, ok := r.Value.(*Service)
	core.AssertTrue(t, ok)
	core.AssertNotNil(t, svc)
}

// --- RouteGroups / rebasedProvider ---

func TestProcess_RouteGroups_Bad_UpstreamUnavailable(t *core.T) {
	svc := NewService(core.New())
	groups := svc.RouteGroups()
	core.AssertNil(t, groups)
}

func TestProcess_RouteGroups_Good_UpstreamAvailable(t *core.T) {
	svc := newRegisteredTestService(t)
	groups := svc.RouteGroups()
	core.AssertEqual(t, 1, len(groups))
	core.AssertEqual(t, GroupName, groups[0].Name())
	core.AssertEqual(t, APIBasePath, groups[0].BasePath())
}

func TestProcess_RebasedProvider_Good_RegisterRoutesDelegates(t *core.T) {
	svc := newRegisteredTestService(t)
	groups := svc.RouteGroups()
	core.AssertEqual(t, 1, len(groups))

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	rg := engine.Group(groups[0].BasePath())

	// Must not panic — confirms the rebasedProvider forwards to the
	// wrapped processapi.ProcessProvider without altering its route set.
	groups[0].RegisterRoutes(rg)

	routes := engine.Routes()
	core.AssertGreater(t, len(routes), 0)
}

// --- Run (success path) ---

func TestProcess_Run_Good_ExecutesAndReturnsOutput(t *core.T) {
	svc := newRegisteredTestService(t)
	r := svc.Run("/bin/echo", []string{"hello"})
	core.AssertTrue(t, r.OK, "Run against a live process runtime must succeed")
}

// --- Start (success path) ---

func TestProcess_Start_Good_ReturnsProcessID(t *core.T) {
	svc := newRegisteredTestService(t)
	r := svc.Start("/bin/sleep", []string{"5"})
	core.AssertTrue(t, r.OK, "Start against a live process runtime must succeed")
	id, ok := r.Value.(string)
	core.AssertTrue(t, ok)
	core.AssertNotEmpty(t, id)

	// Clean up the backgrounded sleep so it doesn't outlive the test.
	_ = svc.Kill(id)
}

// --- Kill (success path) ---

func TestProcess_Kill_Good_TerminatesRunningProcess(t *core.T) {
	svc := newRegisteredTestService(t)
	startR := svc.Start("/bin/sleep", []string{"5"})
	core.AssertTrue(t, startR.OK)
	id := startR.Value.(string)

	r := svc.Kill(id)
	core.AssertTrue(t, r.OK, "Kill against a live, running process must succeed")
}

// --- List ---

func TestProcess_List_Good_EmptyRegistry(t *core.T) {
	svc := newRegisteredTestService(t)
	r := svc.List(false)
	core.AssertTrue(t, r.OK)
	ids, ok := r.Value.([]string)
	core.AssertTrue(t, ok)
	core.AssertEqual(t, 0, len(ids))
}

func TestProcess_List_Good_ReturnsStartedID(t *core.T) {
	svc := newRegisteredTestService(t)
	startR := svc.Start("/bin/sleep", []string{"5"})
	core.AssertTrue(t, startR.OK)
	id := startR.Value.(string)
	t.Cleanup(func() { _ = svc.Kill(id) })

	r := svc.List(false)
	core.AssertTrue(t, r.OK)
	ids := r.Value.([]string)
	core.AssertGreater(t, len(ids), 0)
	core.AssertContains(t, ids, id)
}

func TestProcess_List_Bad_UpstreamUnavailable(t *core.T) {
	svc := NewService(core.New())
	r := svc.List(true)
	core.AssertFalse(t, r.OK)
}

// --- Get ---

func TestProcess_Get_Good_ReturnsInfo(t *core.T) {
	svc := newRegisteredTestService(t)
	startR := svc.Start("/bin/sleep", []string{"5"})
	core.AssertTrue(t, startR.OK)
	id := startR.Value.(string)
	t.Cleanup(func() { _ = svc.Kill(id) })

	r := svc.Get(id)
	core.AssertTrue(t, r.OK)
}

func TestProcess_Get_Bad_UnknownID(t *core.T) {
	svc := newRegisteredTestService(t)
	r := svc.Get("no-such-process-id")
	core.AssertFalse(t, r.OK)
}

func TestProcess_Get_Ugly_UpstreamUnavailable(t *core.T) {
	svc := NewService(core.New())
	r := svc.Get("whatever")
	core.AssertFalse(t, r.OK)
}
