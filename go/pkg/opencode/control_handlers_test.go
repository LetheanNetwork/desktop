// SPDX-Licence-Identifier: EUPL-1.2

// control_handlers_test.go — end-to-end HTTP coverage for the
// ControlGroup handlers in control.go, driven against REAL Service
// instances built via the newTestService / fakeRuntime / orm.Memium
// harness in testutil_test.go. Prior tests in this package (control_
// test.go, upgrade_wire_test.go) deliberately stubbed the Service
// layer because "the production handlers wrap a Service (Core + ORM +
// DuckDB) — too heavy to stand up in a unit test"; the Memium +
// fakeRuntime seams removed that heaviness, so this file drives the
// REAL g.spawn / g.list / g.stop / g.inspect / g.profile* / g.enable /
// g.disable / g.enabled / g.hostConfigMerge / g.providerList /
// g.webURL / g.openWebWindow handlers through gin.
//
// openStudio / openTUI are deliberately only exercised on their
// early-return guard paths — see the file-level note above
// TestControlGroup_OpenStudio_NotBoundToRealApp_Bad.

package opencode

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	core "dappco.re/go"
	"github.com/gin-gonic/gin"

	"dappco.re/go/orm"
)

// newTestControlGroup wires a real ControlGroup to a real Service
// (process registered, Memium mounted, fresh temp-HOME KV store).
func newTestControlGroup(t *testing.T, opts Options) (*ControlGroup, *Service) {
	t.Helper()
	svc := newTestService(t, opts)
	return NewControlGroup(svc), svc
}

// newGinEngine returns a bare gin.Engine in test mode with the given
// routes registered via reg.
func newGinEngine(reg func(*gin.Engine)) *gin.Engine {
	gin.SetMode(gin.TestMode)
	e := gin.New()
	reg(e)
	return e
}

// seedRunningSandbox saves a Sandbox record directly via the orm
// (bypassing Start / docker entirely) so provider/web/proxy-adjacent
// handlers can be tested against a "running" sandbox without a real
// container. hostPort should be the port of a caller-managed
// httptest.Server standing in for opencode-serve.
func seedRunningSandbox(t *testing.T, svc *Service, id string, hostPort int) Sandbox {
	t.Helper()
	sb := Sandbox{ID: id, Image: svc.image(), HostPort: hostPort, Status: StatusRunning, CreatedAt: core.Now()}
	if r := orm.Of[Sandbox](svc.Core()).Save(&sb); !r.OK {
		t.Fatalf("seedRunningSandbox: Save failed: %s", r.Error())
	}
	return sb
}

// seedSandboxDirect saves an arbitrary caller-built Sandbox record via
// the orm, bypassing Start entirely. Unlike seedRunningSandbox it does
// not force Status — used by tests that need a non-running (e.g.
// Stopped) fixture.
func seedSandboxDirect(t *testing.T, svc *Service, sb Sandbox) core.Result {
	t.Helper()
	if sb.CreatedAt.IsZero() {
		sb.CreatedAt = core.Now()
	}
	return orm.Of[Sandbox](svc.Core()).Save(&sb)
}

func portOf(t *testing.T, srv *httptest.Server) int {
	t.Helper()
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse httptest URL: %v", err)
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatalf("parse httptest port: %v", err)
	}
	return port
}

// --- spawn / list / stop / inspect ---------------------------------

func TestControlGroup_Spawn_Good(t *testing.T) {
	fake := newFakeOpencodeServe(t)
	pinPortAllocation(t, fake.Server)
	rt := fakeRuntime(t, "exit 0")
	g, _ := newTestControlGroup(t, Options{Runtime: rt})

	e := newGinEngine(func(e *gin.Engine) { e.POST("/sandbox", g.spawn) })
	req := httptest.NewRequest(core.MethodPost, "/sandbox", strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200, body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"profile":"default"`) {
		t.Errorf("body missing profile=default: %s", w.Body.String())
	}
}

func TestControlGroup_Spawn_Bad(t *testing.T) {
	g, _ := newTestControlGroup(t, Options{})
	e := newGinEngine(func(e *gin.Engine) { e.POST("/sandbox", g.spawn) })
	req := httptest.NewRequest(core.MethodPost, "/sandbox", nil)
	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d; want 500 (no process runtime configured), body=%s", w.Code, w.Body.String())
	}
}

func TestControlGroup_List_Good(t *testing.T) {
	g, svc := newTestControlGroup(t, Options{})
	fake := newFakeOpencodeServe(t)
	seedRunningSandbox(t, svc, "oc-list-1", portOf(t, fake.Server))

	e := newGinEngine(func(e *gin.Engine) { e.GET("/sandbox", g.list) })
	req := httptest.NewRequest(core.MethodGet, "/sandbox", nil)
	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), "oc-list-1") {
		t.Errorf("body missing seeded sandbox: %s", w.Body.String())
	}
}

func TestControlGroup_Stop_Good(t *testing.T) {
	fake := newFakeOpencodeServe(t)
	pinPortAllocation(t, fake.Server)
	rt := fakeRuntime(t, "exit 0")
	g, svc := newTestControlGroup(t, Options{Runtime: rt})

	startR := svc.Start("")
	id, _ := startR.Value.(string)

	e := newGinEngine(func(e *gin.Engine) { e.DELETE("/sandbox/:id", g.stop) })
	req := httptest.NewRequest(core.MethodDelete, "/sandbox/"+id, nil)
	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200, body=%s", w.Code, w.Body.String())
	}
}

func TestControlGroup_Inspect_Bad(t *testing.T) {
	g, _ := newTestControlGroup(t, Options{})
	e := newGinEngine(func(e *gin.Engine) { e.GET("/sandbox/:id", g.inspect) })
	req := httptest.NewRequest(core.MethodGet, "/sandbox/oc-nope", nil)
	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d; want 404", w.Code)
	}
}

// --- profile CRUD ----------------------------------------------------

func TestControlGroup_ProfileList_Good(t *testing.T) {
	g, _ := newTestControlGroup(t, Options{})
	e := newGinEngine(func(e *gin.Engine) { e.GET("/profile", g.profileList) })
	req := httptest.NewRequest(core.MethodGet, "/profile", nil)
	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", w.Code)
	}
	// SeedDefaultProfile ran during Service construction.
	if !strings.Contains(w.Body.String(), `"name":"default"`) {
		t.Errorf("body missing seeded default profile: %s", w.Body.String())
	}
}

func TestControlGroup_ProfileGet_Bad(t *testing.T) {
	g, _ := newTestControlGroup(t, Options{})
	e := newGinEngine(func(e *gin.Engine) { e.GET("/profile/:name", g.profileGet) })
	req := httptest.NewRequest(core.MethodGet, "/profile/does-not-exist", nil)
	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d; want 404", w.Code)
	}
}

func TestControlGroup_ProfileSave_Good(t *testing.T) {
	g, _ := newTestControlGroup(t, Options{})
	e := newGinEngine(func(e *gin.Engine) { e.POST("/profile", g.profileSave) })
	body := `{"name":"tight-loop","model":"anthropic/claude-sonnet-4-5"}`
	req := httptest.NewRequest(core.MethodPost, "/profile", strings.NewReader(body))
	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200, body=%s", w.Code, w.Body.String())
	}
}

func TestControlGroup_ProfileSave_InvalidJSON_Bad(t *testing.T) {
	g, _ := newTestControlGroup(t, Options{})
	e := newGinEngine(func(e *gin.Engine) { e.POST("/profile", g.profileSave) })
	req := httptest.NewRequest(core.MethodPost, "/profile", strings.NewReader(`{not-json`))
	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d; want 400, body=%s", w.Code, w.Body.String())
	}
}

func TestControlGroup_ProfileSave_InvalidSchema_Bad(t *testing.T) {
	g, _ := newTestControlGroup(t, Options{})
	e := newGinEngine(func(e *gin.Engine) { e.POST("/profile", g.profileSave) })
	body := `{"name":"tight","provider":{"evil":{"npm":"@attacker/sdk"}}}`
	req := httptest.NewRequest(core.MethodPost, "/profile", strings.NewReader(body))
	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d; want 500, body=%s", w.Code, w.Body.String())
	}
}

func TestControlGroup_ProfileDelete_Good(t *testing.T) {
	g, svc := newTestControlGroup(t, Options{})
	if r := svc.SaveProfile(Profile{Name: "throwaway"}); !r.OK {
		t.Fatalf("seed SaveProfile failed: %s", r.Error())
	}
	e := newGinEngine(func(e *gin.Engine) { e.DELETE("/profile/:name", g.profileDelete) })
	req := httptest.NewRequest(core.MethodDelete, "/profile/throwaway", nil)
	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200, body=%s", w.Code, w.Body.String())
	}
}

func TestControlGroup_ProfileDelete_DefaultProtected_Bad(t *testing.T) {
	g, _ := newTestControlGroup(t, Options{})
	e := newGinEngine(func(e *gin.Engine) { e.DELETE("/profile/:name", g.profileDelete) })
	req := httptest.NewRequest(core.MethodDelete, "/profile/default", nil)
	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d; want 400, body=%s", w.Code, w.Body.String())
	}
}

// --- host-config merge ------------------------------------------------

func TestControlGroup_HostConfigMerge_Good(t *testing.T) {
	g, _ := newTestControlGroup(t, Options{})
	e := newGinEngine(func(e *gin.Engine) { e.POST("/host-config", g.hostConfigMerge) })
	req := httptest.NewRequest(core.MethodPost, "/host-config", strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200, body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"created":true`) {
		t.Errorf("body missing created:true on first merge: %s", w.Body.String())
	}
}

func TestControlGroup_HostConfigMerge_Conflict_Bad(t *testing.T) {
	g, svc := newTestControlGroup(t, Options{})
	// Seed a conflicting existing config with a DIFFERENT lthn baseURL.
	home, _ := core.UserHomeDir().Value.(string)
	path := core.PathJoin(home, hostConfigSubpath)
	_ = core.MkdirAll(core.PathDir(path), 0o700)
	existing := `{"provider":{"lthn":{"options":{"baseURL":"http://attacker.example/v1"}}}}`
	if r := core.WriteFile(path, []byte(existing), 0o600); !r.OK {
		t.Fatalf("seed existing host config failed: %s", r.Error())
	}
	_ = svc // silence unused if the seed above changes

	e := newGinEngine(func(e *gin.Engine) { e.POST("/host-config", g.hostConfigMerge) })
	req := httptest.NewRequest(core.MethodPost, "/host-config", strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d; want 409, body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), HostConfigConflict) {
		t.Errorf("body missing conflict code: %s", w.Body.String())
	}
}

// --- provider list -----------------------------------------------------

func TestControlGroup_ProviderList_Good(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/provider" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"all":[{"id":"lthn","models":{"lthn-local":{}}}]}`))
	}))
	t.Cleanup(upstream.Close)

	g, svc := newTestControlGroup(t, Options{})
	seedRunningSandbox(t, svc, "oc-providers-1", portOf(t, upstream))

	e := newGinEngine(func(e *gin.Engine) { e.GET("/sandbox/:id/providers", g.providerList) })
	req := httptest.NewRequest(core.MethodGet, "/sandbox/oc-providers-1/providers", nil)
	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200, body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "lthn-local") {
		t.Errorf("body missing upstream payload: %s", w.Body.String())
	}
}

func TestControlGroup_ProviderList_NotRunning_Bad(t *testing.T) {
	g, _ := newTestControlGroup(t, Options{})
	e := newGinEngine(func(e *gin.Engine) { e.GET("/sandbox/:id/providers", g.providerList) })
	req := httptest.NewRequest(core.MethodGet, "/sandbox/oc-ghost/providers", nil)
	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d; want 500, body=%s", w.Code, w.Body.String())
	}
}

// --- enable / disable / enabled ----------------------------------------

func TestControlGroup_Enable_Good(t *testing.T) {
	fake := newFakeOpencodeServe(t)
	pinPortAllocation(t, fake.Server)
	rt := fakeRuntime(t, "exit 0")
	g, _ := newTestControlGroup(t, Options{Runtime: rt})

	e := newGinEngine(func(e *gin.Engine) { e.POST("/enable", g.enable) })
	req := httptest.NewRequest(core.MethodPost, "/enable", strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200, body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"enabled":true`) {
		t.Errorf("body missing enabled:true: %s", w.Body.String())
	}
}

func TestControlGroup_Disable_Good(t *testing.T) {
	g, _ := newTestControlGroup(t, Options{})
	e := newGinEngine(func(e *gin.Engine) { e.POST("/disable", g.disable) })
	req := httptest.NewRequest(core.MethodPost, "/disable", nil)
	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200, body=%s", w.Code, w.Body.String())
	}
}

func TestControlGroup_Enabled_Good(t *testing.T) {
	g, svc := newTestControlGroup(t, Options{})
	if r := svc.setEnabled(true); !r.OK {
		t.Fatalf("setEnabled failed: %s", r.Error())
	}
	e := newGinEngine(func(e *gin.Engine) { e.GET("/enabled", g.enabled) })
	req := httptest.NewRequest(core.MethodGet, "/enabled", nil)
	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"enabled":true`) {
		t.Fatalf("status=%d body=%s; want 200 enabled:true", w.Code, w.Body.String())
	}
}

// --- studio ------------------------------------------------------------
//
// IsStudioInstalled() reflects REAL host state (checks for
// /Applications/OpenCode.app on darwin) — there is no production seam
// to fake it, and faking it is out of scope for a "smallest-safe
// testability seam" (it's a single core.Stat call, not an exec
// boundary). We assert the handler's response is CONSISTENT with the
// real primitive rather than asserting a fixed true/false, so the test
// passes regardless of whether the CI host happens to have the app
// installed.

func TestControlGroup_Studio_ReflectsHostState_Good(t *testing.T) {
	g, svc := newTestControlGroup(t, Options{})
	e := newGinEngine(func(e *gin.Engine) { e.GET("/studio", g.studio) })
	req := httptest.NewRequest(core.MethodGet, "/studio", nil)
	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", w.Code)
	}
	want := `"installed":false`
	if svc.IsStudioInstalled() {
		want = `"installed":true`
	}
	if !strings.Contains(w.Body.String(), want) {
		t.Errorf("body = %s; want to contain %s (matching real host state)", w.Body.String(), want)
	}
}

// TestControlGroup_OpenStudio_NotBoundToRealApp_Bad — deliberate
// leave-out boundary: OpenStudio's darwin happy-path invokes a REAL
// `open -a OpenCode`, which would actually launch the app on any Mac
// that has it installed (true on this dev box). We only ever exercise
// OpenStudio against a bare &Service{} (proc()==nil), which fails at
// the "process service unavailable" guard BEFORE reaching ps.Run —
// this can never accidentally launch the real app, regardless of host
// state, because the guard order in studio.go checks IsStudioInstalled
// first (true on this box) then proc() (nil on a bare Service).
func TestControlGroup_OpenStudio_NotBoundToRealApp_Bad(t *testing.T) {
	g := NewControlGroup(&Service{})
	e := newGinEngine(func(e *gin.Engine) { e.POST("/studio", g.openStudio) })
	req := httptest.NewRequest(core.MethodPost, "/studio", nil)
	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)
	// Either 404 (not installed) or 500 (installed but no proc) is an
	// acceptable, safe outcome — the assertion that matters is that we
	// never got a 200 (which would mean ps.Run fired for real).
	if w.Code == http.StatusOK {
		t.Fatalf("openStudio against a bare Service returned 200 — this would mean a real app launch fired")
	}
}

// --- web URL / open web window ------------------------------------------

func TestControlGroup_WebURL_Good(t *testing.T) {
	g, svc := newTestControlGroup(t, Options{})
	seedRunningSandbox(t, svc, "oc-web-1", 51823)

	e := newGinEngine(func(e *gin.Engine) { e.GET("/sandbox/:id/web", g.webURL) })
	req := httptest.NewRequest(core.MethodGet, "/sandbox/oc-web-1/web", nil)
	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200, body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "127.0.0.1:51823") {
		t.Errorf("body missing resolved URL: %s", w.Body.String())
	}
}

func TestControlGroup_WebURL_NotRunning_Bad(t *testing.T) {
	g, _ := newTestControlGroup(t, Options{})
	e := newGinEngine(func(e *gin.Engine) { e.GET("/sandbox/:id/web", g.webURL) })
	req := httptest.NewRequest(core.MethodGet, "/sandbox/oc-ghost/web", nil)
	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d; want 404, body=%s", w.Code, w.Body.String())
	}
}

// TestControlGroup_OpenWebWindow_Good registers a fake "window.open"
// action on the Service's Core — exactly the extension point core/gui
// registers in production — so OpenWebWindow's full success path runs
// without a real Wails window ever opening.
func TestControlGroup_OpenWebWindow_Good(t *testing.T) {
	g, svc := newTestControlGroup(t, Options{})
	seedRunningSandbox(t, svc, "oc-webwin-1", 51824)

	var gotWindowName string
	svc.Core().Action("window.open", func(_ core.Context, opts core.Options) core.Result {
		taskR := opts.Get("task")
		task, _ := taskR.Value.(map[string]any)
		win, _ := task["Window"].(map[string]any)
		gotWindowName, _ = win["Name"].(string)
		return core.Ok(nil)
	})

	e := newGinEngine(func(e *gin.Engine) { e.POST("/sandbox/:id/web", g.openWebWindow) })
	req := httptest.NewRequest(core.MethodPost, "/sandbox/oc-webwin-1/web", nil)
	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200, body=%s", w.Code, w.Body.String())
	}
	if gotWindowName != "opencode-web-oc-webwin-1" {
		t.Errorf("window.open action Name = %q; want opencode-web-oc-webwin-1", gotWindowName)
	}
}

func TestControlGroup_OpenWebWindow_NoGUIRegistered_Bad(t *testing.T) {
	g, svc := newTestControlGroup(t, Options{})
	seedRunningSandbox(t, svc, "oc-webwin-2", 51825)

	e := newGinEngine(func(e *gin.Engine) { e.POST("/sandbox/:id/web", g.openWebWindow) })
	req := httptest.NewRequest(core.MethodPost, "/sandbox/oc-webwin-2/web", nil)
	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d; want 500 (window.open not registered — serve mode), body=%s", w.Code, w.Body.String())
	}
}

// --- import surface ------------------------------------------------------

func TestControlGroup_ListImports_EmptyGood(t *testing.T) {
	g, _ := newTestControlGroup(t, Options{})
	e := newGinEngine(func(e *gin.Engine) { e.GET("/imports", g.listImports) })
	req := httptest.NewRequest(core.MethodGet, "/imports", nil)
	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200, body=%s", w.Code, w.Body.String())
	}
}

func TestControlGroup_ListImportedProviders_Good(t *testing.T) {
	g, svc := newTestControlGroup(t, Options{})
	rec := ImportedProvider{ID: "host:anthropic", Source: SourceOpenCodeHost, ProviderID: "anthropic", AuthKey: "sk-ant-secret-value-0123456789"}
	if r := orm.Of[ImportedProvider](svc.Core()).Save(&rec); !r.OK {
		t.Fatalf("seed ImportedProvider failed: %s", r.Error())
	}

	e := newGinEngine(func(e *gin.Engine) { e.GET("/imports/providers", g.listImportedProviders) })
	req := httptest.NewRequest(core.MethodGet, "/imports/providers", nil)
	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200, body=%s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "sk-ant-secret-value-0123456789") {
		t.Fatalf("HTTP response leaked raw AuthKey: %s", w.Body.String())
	}
}

func TestControlGroup_ImportFromHost_ProcessUnavailable_Bad(t *testing.T) {
	g := NewControlGroup(&Service{})
	e := newGinEngine(func(e *gin.Engine) { e.POST("/import", g.importFromHost) })
	req := httptest.NewRequest(core.MethodPost, "/import", nil)
	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d; want 500, body=%s", w.Code, w.Body.String())
	}
}

// --- openTUI guard path (no real terminal ever spawned) -----------------

func TestControlGroup_OpenTUI_NotRunning_Bad(t *testing.T) {
	g, _ := newTestControlGroup(t, Options{})
	e := newGinEngine(func(e *gin.Engine) { e.POST("/sandbox/:id/tui", g.openTUI) })
	req := httptest.NewRequest(core.MethodPost, "/sandbox/oc-ghost/tui", nil)
	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d; want 500, body=%s", w.Code, w.Body.String())
	}
}
