// SPDX-Licence-Identifier: EUPL-1.2

// wails_methods_test.go — coverage for the WailsService thin-wrapper
// surface in wails.go. Each W* method is a one-line "service not
// bound" guard plus a delegation to the underlying Service; the guard
// is exercised via a nil-svc WailsService, and the delegation via a
// fully-wired one built through the newTestService harness.

package opencode

import (
	"net/http"
	"net/http/httptest"
	"testing"

	core "dappco.re/go"
	"dappco.re/go/orm"
)

// TestWailsService_LifecycleHooks_Good — ServiceName/ServiceStartup/
// ServiceShutdown are static and nil-safe.
func TestWailsService_LifecycleHooks_Good(t *testing.T) {
	w := NewWailsService(&Service{})
	if w.ServiceName() != "OpenCodeWails" {
		t.Errorf("ServiceName() = %q; want OpenCodeWails", w.ServiceName())
	}
	if r := w.ServiceStartup(core.Background(), nil); !r.OK {
		t.Errorf("ServiceStartup failed: %s", r.Error())
	}
	if r := w.ServiceShutdown(); !r.OK {
		t.Errorf("ServiceShutdown failed: %s", r.Error())
	}
}

// TestWailsService_UnboundGuards_Bad — every W* method on a
// WailsService with a nil svc (or a nil *WailsService itself) must
// fail gracefully rather than panic.
func TestWailsService_UnboundGuards_Bad(t *testing.T) {
	unbound := &WailsService{}
	var nilW *WailsService

	calls := []struct {
		name string
		fn   func() core.Result
	}{
		{"WStart", func() core.Result { return unbound.WStart("") }},
		{"WStop", func() core.Result { return unbound.WStop("x") }},
		{"WStatus", func() core.Result { return unbound.WStatus() }},
		{"WInspect", func() core.Result { return unbound.WInspect("x") }},
		{"WListProfiles", func() core.Result { return unbound.WListProfiles() }},
		{"WGetProfile", func() core.Result { return unbound.WGetProfile("x") }},
		{"WSaveProfile", func() core.Result { return unbound.WSaveProfile(Profile{Name: "x"}) }},
		{"WDeleteProfile", func() core.Result { return unbound.WDeleteProfile("x") }},
		{"WWebURL", func() core.Result { return unbound.WWebURL("x") }},
		{"WOpenWebWindow", func() core.Result { return unbound.WOpenWebWindow("x") }},
		{"WImportFromHost", func() core.Result { return unbound.WImportFromHost() }},
		{"WListImports", func() core.Result { return unbound.WListImports() }},
		{"WUpgradeWithConsent", func() core.Result { return unbound.WUpgradeWithConsent(UpgradeInput{}) }},
		{"WOpenStudio", func() core.Result { return unbound.WOpenStudio() }},
		{"WOpenTUI", func() core.Result { return unbound.WOpenTUI("x") }},
		{"WOpenTUIInApp", func() core.Result { return unbound.WOpenTUIInApp("x") }},
		{"WEnable", func() core.Result { return unbound.WEnable("") }},
		{"WDisable", func() core.Result { return unbound.WDisable() }},
		{"WProviderList", func() core.Result { return unbound.WProviderList("x") }},
		{"WMergeHostConfig", func() core.Result { return unbound.WMergeHostConfig(MergeHostConfigOptions{}) }},
		{"nil.WStart", func() core.Result { return nilW.WStart("") }},
	}
	for _, c := range calls {
		if r := c.fn(); r.OK {
			t.Errorf("%s on an unbound WailsService returned OK; want Fail", c.name)
		}
	}

	// WIsStudioInstalled / WIsEnabled degrade to Ok(false) rather than
	// Fail when unbound — pin that documented shape.
	if r := unbound.WIsStudioInstalled(); !r.OK || r.Value != false {
		t.Errorf("unbound WIsStudioInstalled() = %+v; want Ok(false)", r)
	}
	if r := unbound.WIsEnabled(); !r.OK || r.Value != false {
		t.Errorf("unbound WIsEnabled() = %+v; want Ok(false)", r)
	}
}

// TestWailsService_BoundDelegation_Good — a fully-wired WailsService
// threads through to the real Service for the KV/orm-backed methods
// (no docker required for these).
func TestWailsService_BoundDelegation_Good(t *testing.T) {
	svc := newTestService(t, Options{})
	w := NewWailsService(svc)

	if r := w.WListProfiles(); !r.OK {
		t.Errorf("WListProfiles failed: %s", r.Error())
	}
	if r := w.WGetProfile(DefaultProfile); !r.OK {
		t.Errorf("WGetProfile(default) failed: %s", r.Error())
	}
	if r := w.WSaveProfile(Profile{Name: "wails-saved"}); !r.OK {
		t.Errorf("WSaveProfile failed: %s", r.Error())
	}
	if r := w.WDeleteProfile("wails-saved"); !r.OK {
		t.Errorf("WDeleteProfile failed: %s", r.Error())
	}
	if r := w.WIsEnabled(); !r.OK {
		t.Errorf("WIsEnabled failed: %s", r.Error())
	}
	if r := w.WMergeHostConfig(MergeHostConfigOptions{}); !r.OK {
		t.Errorf("WMergeHostConfig failed: %s", r.Error())
	}
	if r := w.WListImports(); !r.OK {
		t.Errorf("WListImports failed: %s", r.Error())
	}
	if r := w.WUpgradeWithConsent(UpgradeInput{}); r.OK {
		t.Errorf("WUpgradeWithConsent(zero input) returned OK; want gate refusal")
	}
	// WIsStudioInstalled reflects real host state — just prove it
	// doesn't error and matches the underlying Service call.
	if r := w.WIsStudioInstalled(); !r.OK {
		t.Errorf("WIsStudioInstalled failed: %s", r.Error())
	} else if r.Value != svc.IsStudioInstalled() {
		t.Errorf("WIsStudioInstalled() = %v; want %v", r.Value, svc.IsStudioInstalled())
	}

	// WStatus / WInspect / WStop / WStart / WWebURL / WOpenWebWindow /
	// WProviderList / WOpenTUI / WOpenTUIInApp need a sandbox; use the
	// full Start/Stop harness plus a directly-seeded record for the
	// read-only ones.
	if r := w.WStatus(); !r.OK {
		t.Errorf("WStatus failed: %s", r.Error())
	}

	fake := newFakeOpencodeServe(t)
	seedRunningSandbox(t, svc, "oc-wails-1", portOf(t, fake.Server))

	if r := w.WInspect("oc-wails-1"); !r.OK {
		t.Errorf("WInspect failed: %s", r.Error())
	}
	if r := w.WWebURL("oc-wails-1"); !r.OK {
		t.Errorf("WWebURL failed: %s", r.Error())
	}

	providerSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"all":[]}`))
	}))
	t.Cleanup(providerSrv.Close)
	seedRunningSandbox(t, svc, "oc-wails-providers", portOf(t, providerSrv))
	if r := w.WProviderList("oc-wails-providers"); !r.OK {
		t.Errorf("WProviderList failed: %s", r.Error())
	}
	// OpenTUI/OpenTUIInApp guard-only (never reaches real exec — see
	// tui_guard_test.go for the full leave-out rationale). A running
	// sandbox with a real ServerPassword still fails downstream at the
	// real terminal-spawn call; we only assert it doesn't panic and
	// stop before asserting OK/Fail either way is acceptable here would
	// risk masking a real spawn, so we instead call these against a
	// NOT-running id, which fails at the Inspect gate before any exec.
	if r := w.WOpenTUI("oc-does-not-exist"); r.OK {
		t.Errorf("WOpenTUI against an unknown sandbox returned OK; want Fail")
	}
	if r := w.WOpenTUIInApp("oc-does-not-exist"); r.OK {
		t.Errorf("WOpenTUIInApp against an unknown sandbox returned OK; want Fail")
	}
	if r := w.WOpenStudio(); r.OK {
		// Only assert no panic; installed-state is host-dependent and
		// the real launch is out of scope (see studio tests).
		_ = r
	}
}

// TestWailsService_WListImportedProviders_Good — success path against
// a real seeded ImportedProvider row, proving the redaction survives
// the real ListImportedProviders round-trip (not just the local
// reimplementation covered elsewhere in this package).
func TestWailsService_WListImportedProviders_Good(t *testing.T) {
	svc := newTestService(t, Options{})
	rec := ImportedProvider{ID: "host:openai", Source: SourceOpenCodeHost, ProviderID: "openai", AuthKey: "sk-openai-secret-0123456789ab"}
	if r := orm.Of[ImportedProvider](svc.Core()).Save(&rec); !r.OK {
		t.Fatalf("seed failed: %s", r.Error())
	}
	w := NewWailsService(svc)
	r := w.WListImportedProviders()
	if !r.OK {
		t.Fatalf("WListImportedProviders failed: %s", r.Error())
	}
	views, ok := r.Value.([]ProviderView)
	if !ok || len(views) != 1 {
		t.Fatalf("WListImportedProviders value = %#v; want 1 ProviderView", r.Value)
	}
	if !views[0].Present || views[0].Masked == "" {
		t.Errorf("view = %+v; want Present=true + non-empty Masked", views[0])
	}
}

// TestWailsService_StartStop_Good — WStart / WStop full round trip
// through the real harness (fake docker + pinned health-check port).
func TestWailsService_StartStop_Good(t *testing.T) {
	fake := newFakeOpencodeServe(t)
	pinPortAllocation(t, fake.Server)
	rt := fakeRuntime(t, "exit 0")
	svc := newTestService(t, Options{Runtime: rt})
	w := NewWailsService(svc)

	startR := w.WStart("")
	if !startR.OK {
		t.Fatalf("WStart failed: %s", startR.Error())
	}
	id, _ := startR.Value.(string)

	stopR := w.WStop(id)
	if !stopR.OK {
		t.Fatalf("WStop failed: %s", stopR.Error())
	}
}

// TestWailsService_WOpenWebWindow_Good — registers the fake
// "window.open" action so the success path runs without a real Wails
// window.
func TestWailsService_WOpenWebWindow_Good(t *testing.T) {
	svc := newTestService(t, Options{})
	svc.Core().Action("window.open", func(_ core.Context, _ core.Options) core.Result {
		return core.Ok(nil)
	})
	seedRunningSandbox(t, svc, "oc-wails-webwin", 51826)
	w := NewWailsService(svc)
	if r := w.WOpenWebWindow("oc-wails-webwin"); !r.OK {
		t.Errorf("WOpenWebWindow failed: %s", r.Error())
	}
}

// TestWailsService_WImportFromHost_ProcessUnavailable_Bad —
// deliberately does NOT exercise the real ImportFromHost success path
// (it shells to a real "opencode serve" with no Options.Runtime seam
// — see import_host_test.go's leave-out note). Only the guard is
// covered here.
func TestWailsService_WImportFromHost_ProcessUnavailable_Bad(t *testing.T) {
	w := NewWailsService(&Service{})
	if r := w.WImportFromHost(); r.OK {
		t.Errorf("WImportFromHost on a bare Service returned OK; want Fail")
	}
}
