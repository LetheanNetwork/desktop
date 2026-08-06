// SPDX-Licence-Identifier: EUPL-1.2

// wails_live_test.go — behavioural coverage for wails.go (Search /
// Info / Installed / InstallPlugin / Remove), previously 0% covered.
// Installed/InstallPlugin/Remove are driven against a REAL
// pkg/plugin.Service (registered for real, not faked) — List() scans
// an empty ~/Lethean/conf/plugins/ (hermetic, no I/O beyond a local
// temp HOME) and Install()/Remove() run their real validation +
// error paths without ever reaching the network (the fixture
// catalogue entries carry no BinaryURL, so plugin.Install fails at
// its own "binary_url or local_path required" gate — a real,
// hermetic fault injection, not a mock).

package marketplace_test

import (
	"testing"

	core "dappco.re/go"
	subject "dappco.re/lthn/desktop/pkg/marketplace"
	"dappco.re/lthn/desktop/pkg/plugin"
)

func newLiveWailsCore(t *testing.T) *core.Core {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	return core.New(core.WithService(plugin.Register))
}

// TestWailsSearch_EmptyQuery_Good — Search wraps the in-memory
// catalogue's search() with no filtering.
func TestWailsSearch_EmptyQuery_Good(t *testing.T) {
	svc := subject.NewService(nil)
	r := svc.Search("", "")
	if !r.OK {
		t.Fatalf("Search: %s", r.Error())
	}
	out := r.Value.(subject.SearchOutput)
	if len(out.Packages) == 0 {
		t.Fatal("expected the fixture catalogue to return at least one package")
	}
}

// TestWailsSearch_CategoryFilter_Good
func TestWailsSearch_CategoryFilter_Good(t *testing.T) {
	svc := subject.NewService(nil)
	r := svc.Search("", "agents")
	if !r.OK {
		t.Fatalf("Search: %s", r.Error())
	}
	out := r.Value.(subject.SearchOutput)
	for _, p := range out.Packages {
		if p.Category != "agents" {
			t.Errorf("expected only agents category, got %q", p.Category)
		}
	}
}

// TestWailsInfo_EmptyCode_Bad
func TestWailsInfo_EmptyCode_Bad(t *testing.T) {
	svc := subject.NewService(nil)
	r := svc.Info("")
	if r.OK {
		t.Fatal("expected Info to reject an empty code")
	}
	if !core.Contains(r.Error(), "code is required") {
		t.Errorf("unexpected error: %s", r.Error())
	}
}

// TestWailsInfo_NotFound_Bad
func TestWailsInfo_NotFound_Bad(t *testing.T) {
	svc := subject.NewService(nil)
	r := svc.Info("nonexistent-plugin-code")
	if r.OK {
		t.Fatal("expected Info to fail for an unknown code")
	}
	if !core.Contains(r.Error(), "plugin not found") {
		t.Errorf("unexpected error: %s", r.Error())
	}
}

// TestWailsInfo_Found_Good
func TestWailsInfo_Found_Good(t *testing.T) {
	svc := subject.NewService(nil)
	r := svc.Info("coreagent")
	if !r.OK {
		t.Fatalf("Info: %s", r.Error())
	}
	out := r.Value.(subject.InfoOutput)
	if out.Package.Code != "coreagent" {
		t.Errorf("expected coreagent, got %q", out.Package.Code)
	}
}

// TestWailsInstalled_NoPluginHost_Good — no "plugin" service
// registered → empty list, not an error.
//
// Uses a real (non-nil) *core.Core with nothing registered rather
// than NewService(nil) — Installed/InstallPlugin/Remove call
// core.ServiceFor(s.core, "plugin") with NO nil-guard on s.core
// (unlike every other method in this package, which either checks
// s.core != nil explicitly or routes through a nil-safe helper like
// sandboxPort()). NewService(nil) here would panic inside
// core.ServiceFor rather than returning the intended "not
// registered" Fail. That gap is a real latent bug this test run
// surfaced — flagged for separate triage rather than patched here
// (fixing it is a behaviour change beyond this task's smallest-safe-
// seam mandate). Testing against an empty-but-real Core exercises
// the identical !ok branch these three methods gate on, without
// tripping the nil-core crash.
func TestWailsInstalled_NoPluginHost_Good(t *testing.T) {
	svc := subject.NewService(core.New())
	r := svc.Installed()
	if !r.OK {
		t.Fatalf("Installed: %s", r.Error())
	}
	out := r.Value.(subject.InstalledOutput)
	if len(out.Packages) != 0 {
		t.Errorf("expected empty package list, got %d", len(out.Packages))
	}
}

// TestWailsInstalled_RealPluginHost_Good — a real plugin.Service with
// nothing installed on disk yet returns an empty (not nil) list.
func TestWailsInstalled_RealPluginHost_Good(t *testing.T) {
	c := newLiveWailsCore(t)
	svc := subject.NewService(c)
	r := svc.Installed()
	if !r.OK {
		t.Fatalf("Installed: %s", r.Error())
	}
	out := r.Value.(subject.InstalledOutput)
	if out.Packages == nil {
		t.Error("expected an empty slice, not nil")
	}
}

// TestWailsInstallPlugin_EmptyCode_Bad
func TestWailsInstallPlugin_EmptyCode_Bad(t *testing.T) {
	svc := subject.NewService(nil)
	r := svc.InstallPlugin("")
	if r.OK {
		t.Fatal("expected InstallPlugin to reject an empty code")
	}
	if !core.Contains(r.Error(), "code is required") {
		t.Errorf("unexpected error: %s", r.Error())
	}
}

// TestWailsInstallPlugin_NotInCatalogue_Bad
func TestWailsInstallPlugin_NotInCatalogue_Bad(t *testing.T) {
	svc := subject.NewService(nil)
	r := svc.InstallPlugin("nonexistent")
	if r.OK {
		t.Fatal("expected InstallPlugin to fail for a code outside the catalogue")
	}
	if !core.Contains(r.Error(), "plugin not found in catalogue") {
		t.Errorf("unexpected error: %s", r.Error())
	}
}

// TestWailsInstallPlugin_NoPluginHost_Bad — catalogue hit, but no
// "plugin" service registered. Uses a real empty Core — see the
// nil-core-panic note on TestWailsInstalled_NoPluginHost_Good.
func TestWailsInstallPlugin_NoPluginHost_Bad(t *testing.T) {
	svc := subject.NewService(core.New())
	r := svc.InstallPlugin("coreagent")
	if r.OK {
		t.Fatal("expected InstallPlugin to fail when the plugin host isn't wired")
	}
	if !core.Contains(r.Error(), "plugin host not registered") {
		t.Errorf("unexpected error: %s", r.Error())
	}
}

// TestWailsInstallPlugin_RealHostNoBinaryURL_Bad — real plugin host,
// real catalogue entry, but the fixture entry carries no BinaryURL —
// plugin.Install's own validation gate fails cleanly, hermetically,
// with no network access attempted.
func TestWailsInstallPlugin_RealHostNoBinaryURL_Bad(t *testing.T) {
	c := newLiveWailsCore(t)
	svc := subject.NewService(c)
	r := svc.InstallPlugin("coreagent")
	if r.OK {
		t.Fatal("expected InstallPlugin to fail — the fixture entry has no BinaryURL")
	}
}

// TestWailsRemove_EmptyCode_Bad
func TestWailsRemove_EmptyCode_Bad(t *testing.T) {
	svc := subject.NewService(nil)
	r := svc.Remove("")
	if r.OK {
		t.Fatal("expected Remove to reject an empty code")
	}
	if !core.Contains(r.Error(), "code is required") {
		t.Errorf("unexpected error: %s", r.Error())
	}
}

// TestWailsRemove_NoPluginHost_Bad — real empty Core; see the
// nil-core-panic note above.
func TestWailsRemove_NoPluginHost_Bad(t *testing.T) {
	svc := subject.NewService(core.New())
	r := svc.Remove("coreagent")
	if r.OK {
		t.Fatal("expected Remove to fail when the plugin host isn't wired")
	}
	if !core.Contains(r.Error(), "plugin host not registered") {
		t.Errorf("unexpected error: %s", r.Error())
	}
}

// TestWailsRemove_RealHost_Good — removing a plugin that was never
// installed is a clean no-op (stopPlugin on an unknown code is a
// no-op; removePlugin on a missing dir is tolerated).
func TestWailsRemove_RealHost_Good(t *testing.T) {
	c := newLiveWailsCore(t)
	svc := subject.NewService(c)
	_ = svc.Remove("never-installed-code")
	// Either OK (idempotent no-op) or a clean Fail is an acceptable
	// real outcome here — the load-bearing assertion is that Remove
	// reached the real plugin host and didn't panic.
}
