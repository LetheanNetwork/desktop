// SPDX-Licence-Identifier: EUPL-1.2

// control_misc_test.go — closes the remaining ControlGroup handler
// gaps not covered by control_handlers_test.go: the trivial
// coreapi.RouteGroup accessors, the error branches that need a
// no-ORM or kv-broken Service, the Good response-shape assertions for
// inspect / profileGet (previously only their 404 paths were driven),
// and the real upgrade success path through the HTTP handler.

package opencode

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	core "dappco.re/go"
	"github.com/gin-gonic/gin"
)

func TestControlGroup_NameBasePathRegisterRoutes_Good(t *testing.T) {
	svc := newTestService(t, Options{})
	g := NewControlGroup(svc)
	if g.Name() != "opencode" {
		t.Errorf("Name() = %q; want opencode", g.Name())
	}
	if g.BasePath() != "/v1/api/opencode" {
		t.Errorf("BasePath() = %q; want /v1/api/opencode", g.BasePath())
	}
	gin.SetMode(gin.TestMode)
	e := gin.New()
	rg := e.Group(g.BasePath())
	g.RegisterRoutes(rg) // must not panic
}

// --- list / stop / inspect / profileList / profileGet Good+Bad --------

func TestControlGroup_List_NoORM_Bad(t *testing.T) {
	resetKV(t)
	c := newTestCoreNoORM(t)
	r := NewService(Options{})(c)
	if !r.OK {
		t.Fatalf("NewService failed: %s", r.Error())
	}
	g := NewControlGroup(r.Value.(*Service))
	e := newGinEngine(func(e *gin.Engine) { e.GET("/sandbox", g.list) })
	req := httptest.NewRequest(core.MethodGet, "/sandbox", nil)
	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d; want 500, body=%s", w.Code, w.Body.String())
	}
}

func TestControlGroup_Stop_ProcessUnavailable_Bad(t *testing.T) {
	g := NewControlGroup(&Service{})
	e := newGinEngine(func(e *gin.Engine) { e.DELETE("/sandbox/:id", g.stop) })
	req := httptest.NewRequest(core.MethodDelete, "/sandbox/oc-x", nil)
	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d; want 500, body=%s", w.Code, w.Body.String())
	}
}

func TestControlGroup_Inspect_Good(t *testing.T) {
	g, svc := newTestControlGroup(t, Options{})
	fake := newFakeOpencodeServe(t)
	seedRunningSandbox(t, svc, "oc-inspect-good", portOf(t, fake.Server))

	e := newGinEngine(func(e *gin.Engine) { e.GET("/sandbox/:id", g.inspect) })
	req := httptest.NewRequest(core.MethodGet, "/sandbox/oc-inspect-good", nil)
	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200, body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "oc-inspect-good") {
		t.Errorf("body missing sandbox id: %s", w.Body.String())
	}
}

func TestControlGroup_ProfileList_KVUnavailable_Bad(t *testing.T) {
	svc := newTestService(t, Options{})
	breakKV(t)
	g := NewControlGroup(svc)
	e := newGinEngine(func(e *gin.Engine) { e.GET("/profile", g.profileList) })
	req := httptest.NewRequest(core.MethodGet, "/profile", nil)
	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d; want 500, body=%s", w.Code, w.Body.String())
	}
}

func TestControlGroup_ProfileGet_Good(t *testing.T) {
	g, _ := newTestControlGroup(t, Options{})
	e := newGinEngine(func(e *gin.Engine) { e.GET("/profile/:name", g.profileGet) })
	req := httptest.NewRequest(core.MethodGet, "/profile/default", nil)
	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200, body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"name":"default"`) {
		t.Errorf("body missing default profile: %s", w.Body.String())
	}
}

// --- listImports / listImportedProviders no-ORM Bad --------------------

func newControlGroupNoORM(t *testing.T, opts Options) *ControlGroup {
	t.Helper()
	resetKV(t)
	c := newTestCoreNoORM(t)
	r := NewService(opts)(c)
	if !r.OK {
		t.Fatalf("NewService failed: %s", r.Error())
	}
	return NewControlGroup(r.Value.(*Service))
}

func TestControlGroup_ListImports_NoORM_Bad(t *testing.T) {
	g := newControlGroupNoORM(t, Options{})
	e := newGinEngine(func(e *gin.Engine) { e.GET("/imports", g.listImports) })
	req := httptest.NewRequest(core.MethodGet, "/imports", nil)
	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d; want 500, body=%s", w.Code, w.Body.String())
	}
}

func TestControlGroup_ListImportedProviders_NoORM_Bad(t *testing.T) {
	g := newControlGroupNoORM(t, Options{})
	e := newGinEngine(func(e *gin.Engine) { e.GET("/imports/providers", g.listImportedProviders) })
	req := httptest.NewRequest(core.MethodGet, "/imports/providers", nil)
	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d; want 500, body=%s", w.Code, w.Body.String())
	}
}

// --- enable / disable kv-unavailable Bad --------------------------------

func TestControlGroup_Enable_KVUnavailable_Bad(t *testing.T) {
	svc := newTestService(t, Options{})
	breakKV(t)
	g := NewControlGroup(svc)
	e := newGinEngine(func(e *gin.Engine) { e.POST("/enable", g.enable) })
	req := httptest.NewRequest(core.MethodPost, "/enable", strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d; want 500, body=%s", w.Code, w.Body.String())
	}
}

func TestControlGroup_Disable_KVUnavailable_Bad(t *testing.T) {
	svc := newTestService(t, Options{})
	breakKV(t)
	g := NewControlGroup(svc)
	e := newGinEngine(func(e *gin.Engine) { e.POST("/disable", g.disable) })
	req := httptest.NewRequest(core.MethodPost, "/disable", nil)
	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d; want 500, body=%s", w.Code, w.Body.String())
	}
}

// --- hostConfigMerge generic (non-conflict) error -----------------------

func TestControlGroup_HostConfigMerge_UnknownProfile_Bad(t *testing.T) {
	g, _ := newTestControlGroup(t, Options{})
	e := newGinEngine(func(e *gin.Engine) { e.POST("/host-config", g.hostConfigMerge) })
	req := httptest.NewRequest(core.MethodPost, "/host-config", strings.NewReader(`{"profile":"does-not-exist"}`))
	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d; want 500, body=%s", w.Code, w.Body.String())
	}
}

// --- upgrade success through the real HTTP handler ----------------------

func TestControlGroup_Upgrade_Good(t *testing.T) {
	out := "Digest: " + pullTestDigest + "\nStatus: Downloaded newer image for lthn/dev:latest\n"
	rt := fakeRuntime(t, dockerPullScript(out))
	g, _ := newTestControlGroup(t, Options{Runtime: rt})

	e := newGinEngine(func(e *gin.Engine) { e.POST("/upgrade", g.upgrade) })
	body := `{"confirmed_by_user":true,"image_digest":"` + pullTestDigest + `"}`
	req := httptest.NewRequest(core.MethodPost, "/upgrade", strings.NewReader(body))
	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200, body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"updated":true`) {
		t.Errorf("body missing updated:true: %s", w.Body.String())
	}
}
