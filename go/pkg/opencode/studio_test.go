// SPDX-Licence-Identifier: EUPL-1.2

// studio_test.go — coverage for studio.go. IsStudioInstalled() checks
// REAL host state (core.Stat on /Applications/OpenCode.app on darwin)
// with no production seam to fake it — see the note above
// TestIsStudioInstalled_ReflectsHostState_Good. OpenStudio's darwin
// happy path shells to a REAL `open -a OpenCode`; we deliberately
// never construct a Service that could reach that branch (see
// TestOpenStudio_NeverLaunchesRealApp_Bad).

package opencode

import (
	"runtime"
	"testing"

	core "dappco.re/go"
)

// TestIsStudioInstalled_ReflectsHostState_Good — asserts consistency
// with the underlying primitive rather than a fixed answer, so the
// test passes on any host regardless of whether OpenCode.app happens
// to be installed.
func TestIsStudioInstalled_ReflectsHostState_Good(t *testing.T) {
	svc := &Service{}
	got := svc.IsStudioInstalled()
	if runtime.GOOS == "darwin" {
		want := core.Stat(studioMacPath).OK
		if got != want {
			t.Errorf("IsStudioInstalled() = %v; want %v (core.Stat(%q).OK)", got, want, studioMacPath)
		}
		return
	}
	if got {
		t.Errorf("IsStudioInstalled() on %s = true; want false (no desktop app shipped)", runtime.GOOS)
	}
}

// TestOpenStudio_NilService_Bad
func TestOpenStudio_NilService_Bad(t *testing.T) {
	var svc *Service
	r := svc.OpenStudio()
	if r.OK {
		t.Fatalf("OpenStudio on a nil Service returned OK; want Fail")
	}
}

// TestOpenStudio_NeverLaunchesRealApp_Bad — OpenStudio against a bare
// &Service{} can NEVER reach the real `open -a OpenCode` exec: if the
// app isn't installed (common on CI), IsStudioInstalled() is false and
// OpenStudio fails at the first guard; if it IS installed (true on
// this dev box), a bare Service's proc() is nil, so the SECOND guard
// fires instead. Either way ps.Run never executes. We assert the
// response is never a launch-succeeded shape.
func TestOpenStudio_NeverLaunchesRealApp_Bad(t *testing.T) {
	svc := &Service{}
	r := svc.OpenStudio()
	if r.OK {
		t.Fatalf("OpenStudio against a bare Service returned OK — this would mean a real app launch fired")
	}
}
