// SPDX-Licence-Identifier: EUPL-1.2

// Tests for the runtime data firewall. AX-10 G/B/U triads per public
// function, plus end-to-end gin.HandlerFunc smoke tests with httptest
// for the boundary behaviour the spec mandates.
//
// Hermeticity: each test wires its own *core.Core with orm + Memium +
// the InstalledBundle schema; no shared state between tests.

package gateway_test

import (
	"net/http/httptest"
	"strings"

	core "dappco.re/go"
	"dappco.re/go/orm"
	"github.com/gin-gonic/gin"

	"dappco.re/lthn/desktop/pkg/gateway"
	"dappco.re/lthn/desktop/pkg/marketplace"
)

// newGatewayCore is the test-local Core wired with orm + Memium +
// the InstalledBundle schema. Mirrors the helpers in pkg/queue +
// pkg/tasks tests.
func newGatewayCore(t *core.T) *core.Core {
	t.Helper()
	c := core.New()
	core.RequireTrue(t, orm.Register(c).OK)
	mem := orm.NewMemium()
	core.RequireTrue(t, orm.Mount(c, "default", mem).OK)
	schema := marketplace.InstalledBundle{}.Schema()
	core.RequireTrue(t, orm.RegisterSchema(c, schema).OK)
	mem.RegisterTable(schema.Name, schema)
	core.RequireTrue(t, c.ServiceStartup(core.Background(), nil).OK)
	t.Cleanup(func() { _ = c.ServiceShutdown(core.Background()) })
	return c
}

// seedBundle persists an InstalledBundle with the supplied permissions.
// Permissions are JSON-encoded via the marketplace package's helper so
// the test mirrors the install-time path exactly.
func seedBundle(t *core.T, c *core.Core, id string, perms []marketplace.Permission) {
	t.Helper()
	rec := marketplace.InstalledBundle{
		BundleID:       id,
		Display:        id,
		ManifestSchema: "lthn-vm/v1",
		Status:         "running",
		ConfigPath:     "/tmp/" + id,
		InstalledAt:    core.Now(),
	}
	if len(perms) > 0 {
		rec.Permissions = core.JSONMarshalString(perms)
	}
	core.RequireTrue(t, orm.Save(c, &rec).OK)
}

// ─── RegisterScope ─────────────────────────────────────────────────────

func TestGateway_RegisterScope_Good(t *core.T) {
	c := newGatewayCore(t)
	svc := gateway.NewService(c)
	r := gateway.RegisterScope(svc, "custom.thing", "read",
		func(*core.Core, string, *gin.Context) core.Result { return core.Ok("ok") })
	core.AssertTrue(t, r.OK)
}

func TestGateway_RegisterScope_Bad(t *core.T) {
	c := newGatewayCore(t)
	svc := gateway.NewService(c)
	noop := func(*core.Core, string, *gin.Context) core.Result { return core.Ok(nil) }

	core.AssertFalse(t, gateway.RegisterScope(nil, "x", "read", noop).OK)
	core.AssertFalse(t, gateway.RegisterScope(svc, "", "read", noop).OK)
	core.AssertFalse(t, gateway.RegisterScope(svc, "x", "", noop).OK)
	core.AssertFalse(t, gateway.RegisterScope(svc, "x", "read", nil).OK)
}

func TestGateway_RegisterScope_Ugly(t *core.T) {
	c := newGatewayCore(t)
	svc := gateway.NewService(c)
	first := func(*core.Core, string, *gin.Context) core.Result { return core.Ok("first") }
	second := func(*core.Core, string, *gin.Context) core.Result { return core.Ok("second") }
	core.RequireTrue(t, gateway.RegisterScope(svc, "x", "read", first).OK)
	core.RequireTrue(t, gateway.RegisterScope(svc, "x", "read", second).OK)
	// Last write wins — no panic, no double-registration error;
	// matches Core's c.Action semantics.
}

// ─── CheckPermission ───────────────────────────────────────────────────

func TestGateway_CheckPermission_Good(t *core.T) {
	c := newGatewayCore(t)
	seedBundle(t, c, "opencode", []marketplace.Permission{
		{Scope: "project.metadata", Mode: "read", Reason: "for the demo"},
	})
	r := gateway.CheckPermission(c, "opencode", "project.metadata", "read")
	core.AssertTrue(t, r.OK)
}

func TestGateway_CheckPermission_Bad(t *core.T) {
	c := newGatewayCore(t)

	// nil core
	core.AssertFalse(t, gateway.CheckPermission(nil, "opencode", "x", "read").OK)

	// empty bundle id
	core.AssertFalse(t, gateway.CheckPermission(c, "", "x", "read").OK)

	// unknown bundle (not installed)
	core.AssertFalse(t, gateway.CheckPermission(c, "ghost", "x", "read").OK)

	// permission not declared
	seedBundle(t, c, "opencode", []marketplace.Permission{
		{Scope: "project.metadata", Mode: "read"},
	})
	r := gateway.CheckPermission(c, "opencode", "service.notify", "invoke")
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "permission not declared")
}

func TestGateway_CheckPermission_Ugly(t *core.T) {
	c := newGatewayCore(t)
	// Bundle declared multiple permissions; CheckPermission picks the
	// matching scope+mode pair regardless of order or count.
	seedBundle(t, c, "opencode", []marketplace.Permission{
		{Scope: "project.metadata", Mode: "read"},
		{Scope: "service.notify", Mode: "invoke"},
		{Scope: "project.files", Mode: "read"},
	})
	core.AssertTrue(t, gateway.CheckPermission(c, "opencode", "project.metadata", "read").OK)
	core.AssertTrue(t, gateway.CheckPermission(c, "opencode", "service.notify", "invoke").OK)
	core.AssertTrue(t, gateway.CheckPermission(c, "opencode", "project.files", "read").OK)
	// Mode mismatch on a declared scope still rejects — write isn't read.
	core.AssertFalse(t, gateway.CheckPermission(c, "opencode", "project.metadata", "write").OK)
}

// ─── HTTP Handle (gin.HandlerFunc) ─────────────────────────────────────

// newTestRouter builds a gin engine wired with the gateway's route
// group. Returns the *Service + the engine; tests fire httptest
// requests against engine.ServeHTTP.
func newTestRouter(t *core.T, c *core.Core) (*gateway.Service, *gin.Engine) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	svc := gateway.NewService(c)
	r := gin.New()
	r.POST("/v1/api/gateway/:scope/:mode", svc.Handle)
	return svc, r
}

func TestGateway_Handle_Good(t *core.T) {
	c := newGatewayCore(t)
	seedBundle(t, c, "opencode", []marketplace.Permission{
		{Scope: "project.metadata", Mode: "read"},
	})
	_, r := newTestRouter(t, c)

	req := httptest.NewRequest("POST", "/v1/api/gateway/project.metadata/read", nil)
	req.Header.Set("Bundle-ID", "opencode")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	core.AssertEqual(t, core.StatusOK, w.Code)
	core.AssertContains(t, w.Body.String(), "projects")
	core.AssertContains(t, w.Body.String(), "count")
}

func TestGateway_Handle_Bad_MissingBundleID(t *core.T) {
	c := newGatewayCore(t)
	_, r := newTestRouter(t, c)

	req := httptest.NewRequest("POST", "/v1/api/gateway/project.metadata/read", nil)
	// Intentionally no Bundle-ID header.
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	core.AssertEqual(t, core.StatusUnauthorized, w.Code)
	core.AssertContains(t, w.Body.String(), "missing-bundle-id")
}

func TestGateway_Handle_Bad_PermissionDenied(t *core.T) {
	c := newGatewayCore(t)
	// Bundle exists but doesn't declare the requested scope.
	seedBundle(t, c, "opencode", []marketplace.Permission{
		{Scope: "service.notify", Mode: "invoke"},
	})
	_, r := newTestRouter(t, c)

	req := httptest.NewRequest("POST", "/v1/api/gateway/project.metadata/read", nil)
	req.Header.Set("Bundle-ID", "opencode")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	core.AssertEqual(t, core.StatusForbidden, w.Code)
	core.AssertContains(t, w.Body.String(), "permission-denied")
}

func TestGateway_Handle_Bad_UnknownBundle(t *core.T) {
	c := newGatewayCore(t)
	_, r := newTestRouter(t, c)

	req := httptest.NewRequest("POST", "/v1/api/gateway/project.metadata/read", nil)
	req.Header.Set("Bundle-ID", "ghost")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Unknown bundle = CheckPermission fail = 403 with permission-denied.
	core.AssertEqual(t, core.StatusForbidden, w.Code)
}

func TestGateway_Handle_Bad_UnregisteredScope(t *core.T) {
	c := newGatewayCore(t)
	seedBundle(t, c, "opencode", []marketplace.Permission{
		{Scope: "nope.unknown", Mode: "read"},
	})
	_, r := newTestRouter(t, c)

	req := httptest.NewRequest("POST", "/v1/api/gateway/nope.unknown/read", nil)
	req.Header.Set("Bundle-ID", "opencode")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Scope not in handler registry = 501 not implemented; takes
	// precedence over permission check (no point gating an unknown
	// scope).
	core.AssertEqual(t, core.StatusNotImplemented, w.Code)
	core.AssertContains(t, w.Body.String(), "scope-unavailable")
}

func TestGateway_Handle_Ugly(t *core.T) {
	c := newGatewayCore(t)
	seedBundle(t, c, "opencode", []marketplace.Permission{
		{Scope: "service.notify", Mode: "invoke"},
	})
	_, r := newTestRouter(t, c)

	// service.notify:invoke now dispatches via the notification.send
	// action when given a valid body. An empty POST has no body to
	// decode → handler returns the typed validation 400 (gateway wraps
	// as handler-failed). Verifies the gateway propagates handler-
	// shaped validation errors with a stable message, not the legacy
	// 501 stub.
	req := httptest.NewRequest("POST", "/v1/api/gateway/service.notify/invoke", nil)
	req.Header.Set("Bundle-ID", "opencode")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	core.AssertEqual(t, core.StatusBadRequest, w.Code)
	core.AssertContains(t, w.Body.String(), "handler-failed")
	core.AssertContains(t, w.Body.String(), "invalid notification body")
}

func TestGateway_Handle_ServiceNotify_MissingTitle(t *core.T) {
	c := newGatewayCore(t)
	seedBundle(t, c, "opencode", []marketplace.Permission{
		{Scope: "service.notify", Mode: "invoke"},
	})
	_, r := newTestRouter(t, c)

	// Body decodes cleanly but title is empty — handler rejects with
	// "title is required". Anchors the validation gate so a future
	// refactor that drops the check flips this test.
	body := strings.NewReader(`{"message":"hi"}`)
	req := httptest.NewRequest("POST", "/v1/api/gateway/service.notify/invoke", body)
	req.Header.Set("Bundle-ID", "opencode")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	core.AssertEqual(t, core.StatusBadRequest, w.Code)
	core.AssertContains(t, w.Body.String(), "title is required")
}

func TestGateway_Handle_ServiceNotify_TitleByteCap(t *core.T) {
	c := newGatewayCore(t)
	seedBundle(t, c, "opencode", []marketplace.Permission{
		{Scope: "service.notify", Mode: "invoke"},
	})
	_, r := newTestRouter(t, c)

	// 257-byte title exceeds the 256-byte cap. Cerberus anchor:
	// drops hostile-label DoS where a bundle ships a 1MB title that
	// crashes the OS notification renderer.
	huge := strings.Repeat("x", 257)
	body := strings.NewReader(`{"title":"` + huge + `","message":"hi"}`)
	req := httptest.NewRequest("POST", "/v1/api/gateway/service.notify/invoke", body)
	req.Header.Set("Bundle-ID", "opencode")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	core.AssertEqual(t, core.StatusBadRequest, w.Code)
	core.AssertContains(t, w.Body.String(), "title exceeds byte cap")
}

// ─── Service constructor ───────────────────────────────────────────────

func TestGateway_NewService_Good(t *core.T) {
	c := newGatewayCore(t)
	svc := gateway.NewService(c)
	core.AssertNotNil(t, svc)
}

func TestGateway_NewService_Bad(t *core.T) {
	// nil Core — constructor doesn't reject (matches the marketplace
	// constructor convention); failure surface is at first request
	// when CheckPermission tries to query orm against a nil core.
	svc := gateway.NewService(nil)
	core.AssertNotNil(t, svc)
}

func TestGateway_NewService_Ugly(t *core.T) {
	c := newGatewayCore(t)
	// Two services on the same Core have independent handler registries.
	a := gateway.NewService(c)
	b := gateway.NewService(c)
	core.AssertNotEqual(t, core.Sprintf("%p", a), core.Sprintf("%p", b))
	// Registering on one doesn't affect the other.
	r := gateway.RegisterScope(a, "only.on.a", "read",
		func(*core.Core, string, *gin.Context) core.Result { return core.Ok(nil) })
	core.AssertTrue(t, r.OK)
}

// ─── NewRoutes / coreapi.RouteGroup ─────────────────────────────────────

func TestGateway_NewRoutes_Good(t *core.T) {
	c := newGatewayCore(t)
	svc := gateway.NewService(c)
	routes := gateway.NewRoutes(svc)

	core.AssertNotNil(t, routes)
	core.AssertEqual(t, "gateway", routes.Name())
	core.AssertEqual(t, "", routes.BasePath())

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	routes.RegisterRoutes(&engine.RouterGroup)

	seedBundle(t, c, "opencode", []marketplace.Permission{
		{Scope: "project.metadata", Mode: "read"},
	})
	req := httptest.NewRequest("POST", "/v1/api/gateway/project.metadata/read", nil)
	req.Header.Set("Bundle-ID", "opencode")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	core.AssertEqual(t, core.StatusOK, w.Code)
	core.AssertContains(t, w.Body.String(), "projects")
}

// ─── serviceNotifyInvoke byte caps + success path ───────────────────────

func TestGateway_Handle_ServiceNotify_MessageByteCap(t *core.T) {
	c := newGatewayCore(t)
	seedBundle(t, c, "opencode", []marketplace.Permission{
		{Scope: "service.notify", Mode: "invoke"},
	})
	_, r := newTestRouter(t, c)

	huge := strings.Repeat("x", 4097)
	body := strings.NewReader(`{"title":"hi","message":"` + huge + `"}`)
	req := httptest.NewRequest("POST", "/v1/api/gateway/service.notify/invoke", body)
	req.Header.Set("Bundle-ID", "opencode")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	core.AssertEqual(t, core.StatusBadRequest, w.Code)
	core.AssertContains(t, w.Body.String(), "message exceeds byte cap")
}

func TestGateway_Handle_ServiceNotify_SubtitleByteCap(t *core.T) {
	c := newGatewayCore(t)
	seedBundle(t, c, "opencode", []marketplace.Permission{
		{Scope: "service.notify", Mode: "invoke"},
	})
	_, r := newTestRouter(t, c)

	huge := strings.Repeat("x", 257)
	body := strings.NewReader(`{"title":"hi","message":"hi","subtitle":"` + huge + `"}`)
	req := httptest.NewRequest("POST", "/v1/api/gateway/service.notify/invoke", body)
	req.Header.Set("Bundle-ID", "opencode")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	core.AssertEqual(t, core.StatusBadRequest, w.Code)
	core.AssertContains(t, w.Body.String(), "subtitle exceeds byte cap")
}

func TestGateway_Handle_ServiceNotify_GoodDispatchesAndStampsID(t *core.T) {
	c := newGatewayCore(t)
	seedBundle(t, c, "opencode", []marketplace.Permission{
		{Scope: "service.notify", Mode: "invoke"},
	})
	dispatched := false
	c.Action("notification.send", func(_ core.Context, options core.Options) core.Result {
		task := options.Get("task")
		core.RequireTrue(t, task.OK)
		dispatched = true
		return core.Ok(nil)
	})
	_, r := newTestRouter(t, c)

	body := strings.NewReader(`{"title":"hi","message":"there"}`)
	req := httptest.NewRequest("POST", "/v1/api/gateway/service.notify/invoke", body)
	req.Header.Set("Bundle-ID", "opencode")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	core.AssertEqual(t, core.StatusOK, w.Code)
	core.AssertTrue(t, dispatched)
	// No id supplied → the handler stamps "<bundleID>:<generated>".
	core.AssertContains(t, w.Body.String(), `"id":"opencode:`)
}

// ─── Register (factory) ────────────────────────────────────────────────

func TestGateway_Register_Good(t *core.T) {
	c := newGatewayCore(t)
	r := gateway.Register(c)
	core.AssertTrue(t, r.OK)
	svc, ok := r.Value.(*gateway.Service)
	core.RequireTrue(t, ok)
	core.AssertNotNil(t, svc)
}
