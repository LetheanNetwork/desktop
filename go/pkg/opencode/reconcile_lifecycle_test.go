// SPDX-Licence-Identifier: EUPL-1.2

// reconcile_lifecycle_test.go — coverage for Reconcile() and
// adoptFromOutput() (reconcile.go), the two Service-methods the
// existing reconcile_test.go's pure-function suite (classifyReconcile
// / parseReconcileLines / parseHostPort / emitDenials) deliberately
// left untouched. `docker ps` is faked via Options.Runtime — the fake
// script inspects argv to tell the filtered (adoption) call from the
// unfiltered (denial-sweep) call apart, exactly mirroring the two
// distinct ps.Run invocations Reconcile makes.

package opencode

import (
	"testing"

	core "dappco.re/go"

	"dappco.re/lthn/desktop/pkg/audit"
)

// dockerPSScript builds a fake-runtime script that answers `ps
// --filter name=... --filter label=...` (the adoption-gate call, argv
// contains "label=") with adoptOut, and the label-less denial-sweep
// call with denyOut. Any other subcommand exits 0 with empty output.
func dockerPSScript(adoptOut, denyOut string) string {
	return `
case "$*" in
  *label=*)
    cat <<'EOF'
` + adoptOut + `
EOF
    ;;
  *)
    cat <<'EOF'
` + denyOut + `
EOF
    ;;
esac
exit 0
`
}

// TestReconcile_AdoptsMatchingContainer_Good — a single container
// whose label matches our install_id is adopted: orm row saved,
// proxy target registered, recovered count == 1, one Adopted audit
// event.
func TestReconcile_AdoptsMatchingContainer_Good(t *testing.T) {
	rec := &fakeAuditRecorder{}
	audit.SetDefault(rec)
	t.Cleanup(func() { audit.SetDefault(nil) })

	// Two-pass: resolve installID first (InstallID() only touches the
	// kv() singleton, not any Service field, so a throwaway zero-value
	// Service call is safe), THEN bake it into the fake `docker ps`
	// script, THEN construct the real Service under test.
	resetKV(t)
	idR := (&Service{}).InstallID()
	if !idR.OK {
		t.Fatalf("pre-resolve InstallID failed: %s", idR.Error())
	}
	installID, _ := idR.Value.(string)

	line := "lthn-opencode-oc-recon-1\t127.0.0.1:51900->4096/tcp\t" + installID
	rt := fakeRuntime(t, dockerPSScript(line, ""))

	c := newTestCore(t)
	r := NewService(Options{Runtime: rt})(c)
	if !r.OK {
		t.Fatalf("NewService failed: %s", r.Error())
	}
	svc := r.Value.(*Service)

	result := svc.Reconcile()
	if !result.OK {
		t.Fatalf("Reconcile failed: %s", result.Error())
	}
	recovered, _ := result.Value.(int)
	if recovered != 1 {
		t.Fatalf("Reconcile recovered = %d; want 1", recovered)
	}

	inspectR := svc.Inspect("oc-recon-1")
	if !inspectR.OK {
		t.Fatalf("Inspect after Reconcile failed: %s", inspectR.Error())
	}
	sb, _ := inspectR.Value.(Sandbox)
	if sb.HostPort != 51900 {
		t.Errorf("adopted HostPort = %d; want 51900", sb.HostPort)
	}
	if !svc.proxy.Has("oc-recon-1") {
		t.Errorf("adopted sandbox not registered in the proxy group")
	}

	events := rec.snapshot()
	found := false
	for _, ev := range events {
		if ev.Event == EventOpencodeSandboxAdopted {
			found = true
			if ev.Meta["sandbox_id"] != "oc-recon-1" {
				t.Errorf("adopted event Meta.sandbox_id = %v; want oc-recon-1", ev.Meta["sandbox_id"])
			}
		}
	}
	if !found {
		t.Errorf("no %s event emitted; events=%+v", EventOpencodeSandboxAdopted, events)
	}
}

// TestReconcile_MixedAdoptAndDeny_Ugly — one adoptable container plus
// one label-mismatched container in the SAME docker-ps universe:
// Reconcile adopts the matching one and emits a denial for the other.
func TestReconcile_MixedAdoptAndDeny_Ugly(t *testing.T) {
	rec := &fakeAuditRecorder{}
	audit.SetDefault(rec)
	t.Cleanup(func() { audit.SetDefault(nil) })

	resetKV(t)
	idR := (&Service{}).InstallID()
	if !idR.OK {
		t.Fatalf("pre-resolve InstallID failed: %s", idR.Error())
	}
	installID, _ := idR.Value.(string)

	allOut := "" +
		"lthn-opencode-oc-good\t127.0.0.1:51901->4096/tcp\t" + installID + "\n" +
		"lthn-opencode-oc-evil\t127.0.0.1:51902->4096/tcp\tattacker-install\n"
	adoptOut := "lthn-opencode-oc-good\t127.0.0.1:51901->4096/tcp\t" + installID
	rt := fakeRuntime(t, dockerPSScript(adoptOut, allOut))

	c := newTestCore(t)
	r := NewService(Options{Runtime: rt})(c)
	if !r.OK {
		t.Fatalf("NewService failed: %s", r.Error())
	}
	svc := r.Value.(*Service)

	result := svc.Reconcile()
	if !result.OK {
		t.Fatalf("Reconcile failed: %s", result.Error())
	}
	recovered, _ := result.Value.(int)
	if recovered != 1 {
		t.Fatalf("Reconcile recovered = %d; want 1 (only oc-good matches)", recovered)
	}

	events := rec.snapshot()
	var adopted, denied int
	for _, ev := range events {
		switch ev.Event {
		case EventOpencodeSandboxAdopted:
			adopted++
		case EventOpencodeSandboxAdoptionDenied:
			denied++
			if ev.Meta["container"] != "lthn-opencode-oc-evil" {
				t.Errorf("denial event container = %v; want lthn-opencode-oc-evil", ev.Meta["container"])
			}
		}
	}
	if adopted != 1 || denied != 1 {
		t.Fatalf("adopted=%d denied=%d; want 1 and 1 (events=%+v)", adopted, denied, events)
	}
}

// TestReconcile_NoContainers_Good — empty docker-ps output on both
// calls: Reconcile succeeds with recovered=0 and emits nothing.
func TestReconcile_NoContainers_Good(t *testing.T) {
	rt := fakeRuntime(t, dockerPSScript("", ""))
	svc := newTestService(t, Options{Runtime: rt})

	r := svc.Reconcile()
	if !r.OK {
		t.Fatalf("Reconcile failed: %s", r.Error())
	}
	if recovered, _ := r.Value.(int); recovered != 0 {
		t.Errorf("recovered = %d; want 0", recovered)
	}
}

// TestReconcile_ProcessUnavailable_Bad — a bare Service has no
// process.Service backing.
func TestReconcile_ProcessUnavailable_Bad(t *testing.T) {
	svc := &Service{}
	r := svc.Reconcile()
	if r.OK {
		t.Fatalf("Reconcile on a bare Service returned OK; want Fail")
	}
	if !core.Contains(r.Error(), "process service unavailable") {
		t.Errorf("error = %q; want 'process service unavailable'", r.Error())
	}
}

// TestReconcile_PSCommandFails_Bad — the runtime's `ps` invocation
// itself fails (non-zero exit); Reconcile surfaces the failure rather
// than silently reporting zero recovered.
func TestReconcile_PSCommandFails_Bad(t *testing.T) {
	rt := fakeRuntime(t, "exit 1")
	svc := newTestService(t, Options{Runtime: rt})
	r := svc.Reconcile()
	if r.OK {
		t.Fatalf("Reconcile with a failing `ps` returned OK; want Fail")
	}
}

// TestAdoptFromOutput_SaveFails_Bad — orm Save fails (no Medium
// mounted); adoptFromOutput must skip the row (continue) rather than
// panic, and report 0 recovered.
func TestAdoptFromOutput_SaveFails_Bad(t *testing.T) {
	resetKV(t)
	c := newTestCoreNoORM(t)
	r := NewService(Options{})(c)
	if !r.OK {
		t.Fatalf("NewService failed: %s", r.Error())
	}
	svc := r.Value.(*Service)

	idR := svc.InstallID()
	installID, _ := idR.Value.(string)
	out := "lthn-opencode-oc-nosave\t127.0.0.1:51903->4096/tcp\t" + installID

	recovered := svc.adoptFromOutput(out, installID, "")
	if recovered != 0 {
		t.Errorf("adoptFromOutput with no orm Medium recovered = %d; want 0", recovered)
	}
}

// TestAdoptFromOutput_BadPortSkipped_Ugly — a row that passes the
// label gate but carries an unparseable Ports column must be skipped
// (verdictBadPort), not adopted.
func TestAdoptFromOutput_BadPortSkipped_Ugly(t *testing.T) {
	svc := newTestService(t, Options{})
	idR := svc.InstallID()
	installID, _ := idR.Value.(string)
	out := "lthn-opencode-oc-badport\tno-arrow-here\t" + installID

	recovered := svc.adoptFromOutput(out, installID, "")
	if recovered != 0 {
		t.Errorf("adoptFromOutput with an unparseable port recovered = %d; want 0", recovered)
	}
	r := svc.Inspect("oc-badport")
	if r.OK {
		t.Errorf("bad-port row should never have been saved to the orm")
	}
}

// TestAdoptFromOutput_EmptyOutput_Good — no lines to walk.
func TestAdoptFromOutput_EmptyOutput_Good(t *testing.T) {
	svc := newTestService(t, Options{})
	if got := svc.adoptFromOutput("", "any-install", ""); got != 0 {
		t.Errorf("adoptFromOutput('') = %d; want 0", got)
	}
}
