// SPDX-Licence-Identifier: EUPL-1.2

// tui_guard_test.go — coverage for OpenTUI / OpenTUIInApp's
// early-return guard clauses ONLY (nil service, empty id, sandbox not
// found/not running). The platform-branch bodies (darwin → real
// `osascript`; OpenTUIInApp → terminal.Spawn(["opencode","attach",…]))
// are a DELIBERATE LEAVE-OUT: both osascript and a real `opencode`
// binary are present on this dev machine's $PATH
// (/usr/bin/osascript, /opt/homebrew/bin/opencode), and neither is
// reachable through the package's Options.Runtime seam (OpenTUI calls
// ps.Run(ctx, "osascript", …) with a hardcoded command name, and
// OpenTUIInApp calls terminal.Spawn directly, bypassing process.Service
// entirely). Exercising either success path would pop a real Terminal
// window / spawn a real PTY running the real opencode CLI — exactly
// what the house rules forbid ("never invoke a real opencode/external
// tool"). The shellQuote / appleScriptQuote / cmdArgvQuote pure
// helpers that feed those commands ARE fully covered in tui_test.go.

package opencode

import "testing"

func TestOpenTUI_NilService_Bad(t *testing.T) {
	var svc *Service
	r := svc.OpenTUI("oc-x")
	if r.OK {
		t.Fatalf("OpenTUI on a nil Service returned OK; want Fail")
	}
}

func TestOpenTUI_EmptyID_Bad(t *testing.T) {
	svc := &Service{}
	r := svc.OpenTUI("   ")
	if r.OK {
		t.Fatalf("OpenTUI('') returned OK; want Fail")
	}
}

func TestOpenTUI_SandboxNotFound_Bad(t *testing.T) {
	svc := newTestService(t, Options{})
	r := svc.OpenTUI("oc-does-not-exist")
	if r.OK {
		t.Fatalf("OpenTUI against a missing sandbox returned OK; want Fail")
	}
}

func TestOpenTUI_SandboxNotRunning_Bad(t *testing.T) {
	svc := newTestService(t, Options{})
	sb := Sandbox{ID: "oc-stopped", Image: svc.image(), HostPort: 1, Status: StatusStopped}
	if r := seedSandboxDirect(t, svc, sb); !r.OK {
		t.Fatalf("seed failed: %s", r.Error())
	}
	r := svc.OpenTUI("oc-stopped")
	if r.OK {
		t.Fatalf("OpenTUI against a stopped sandbox returned OK; want Fail")
	}
}

func TestOpenTUIInApp_NilService_Bad(t *testing.T) {
	var svc *Service
	r := svc.OpenTUIInApp("oc-x")
	if r.OK {
		t.Fatalf("OpenTUIInApp on a nil Service returned OK; want Fail")
	}
}

func TestOpenTUIInApp_EmptyID_Bad(t *testing.T) {
	svc := &Service{}
	r := svc.OpenTUIInApp("   ")
	if r.OK {
		t.Fatalf("OpenTUIInApp('') returned OK; want Fail")
	}
}

func TestOpenTUIInApp_SandboxNotFound_Bad(t *testing.T) {
	svc := newTestService(t, Options{})
	r := svc.OpenTUIInApp("oc-does-not-exist")
	if r.OK {
		t.Fatalf("OpenTUIInApp against a missing sandbox returned OK; want Fail")
	}
}

func TestOpenTUIInApp_SandboxNotRunning_Bad(t *testing.T) {
	svc := newTestService(t, Options{})
	sb := Sandbox{ID: "oc-stopped-2", Image: svc.image(), HostPort: 1, Status: StatusStopped}
	if r := seedSandboxDirect(t, svc, sb); !r.OK {
		t.Fatalf("seed failed: %s", r.Error())
	}
	r := svc.OpenTUIInApp("oc-stopped-2")
	if r.OK {
		t.Fatalf("OpenTUIInApp against a stopped sandbox returned OK; want Fail")
	}
}
