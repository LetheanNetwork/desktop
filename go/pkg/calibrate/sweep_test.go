// SPDX-Licence-Identifier: EUPL-1.2

package calibrate

import (
	"testing"
	"time"

	core "dappco.re/go"
	"dappco.re/go/process"
)

// autotuneStdout mirrors a real `lthn-mlx auto-tune` run's stdout (the
// CLI has no --json; we parse these lines).
const autotuneStdout = `auto-tune: model=/Users/me/models/lemer-lite workload=chat machine=sha256:abc123
  planning candidates…
  measuring 4 candidates (this can take several minutes)…
    candidate c1: score=0.512
    candidate c2: score=0.842
    candidate c3: score=0.701

selected: c2 (score=0.842)
saved:    /Users/me/Lethean/data/profiles/lemer-lite-chat.json

run ` + "`lthn-mlx serve /Users/me/models/lemer-lite`" + ` — the profile auto-applies.
`

// TestParseCalibrateResult_Good extracts the winner, score, and saved
// profile path from a complete sweep.
func TestParseCalibrateResult_Good(t *testing.T) {
	res, found := parseCalibrateResult(autotuneStdout)
	if !found {
		t.Fatalf("parseCalibrateResult: found=false, want true")
	}
	if res.SelectedID != "c2" {
		t.Errorf("SelectedID = %q, want c2", res.SelectedID)
	}
	if res.Score != "0.842" {
		t.Errorf("Score = %q, want 0.842", res.Score)
	}
	if res.ProfilePath != "/Users/me/Lethean/data/profiles/lemer-lite-chat.json" {
		t.Errorf("ProfilePath = %q, want the saved profile path", res.ProfilePath)
	}
}

// TestParseCalibrateResult_Bad returns found=false when the sweep
// produced no selection (failed before "selected:").
func TestParseCalibrateResult_Bad(t *testing.T) {
	partial := "auto-tune: model=x workload=chat machine=y\n  planning candidates…\n  no tuning candidates\n"
	if _, found := parseCalibrateResult(partial); found {
		t.Errorf("parseCalibrateResult(no-selection): found=true, want false")
	}
	if _, found := parseCalibrateResult(""); found {
		t.Errorf("parseCalibrateResult(empty): found=true, want false")
	}
}

// TestParseCalibrateResult_Ugly survives a malformed selected line (no
// score / no saved line) without panicking — found is still true, the
// missing fields stay empty.
func TestParseCalibrateResult_Ugly(t *testing.T) {
	res, found := parseCalibrateResult("selected: loneCandidate\n")
	if !found {
		t.Fatalf("parseCalibrateResult(no-score): found=false, want true")
	}
	if res.SelectedID != "loneCandidate" {
		t.Errorf("SelectedID = %q, want loneCandidate", res.SelectedID)
	}
	if res.Score != "" || res.ProfilePath != "" {
		t.Errorf("Score/ProfilePath should be empty for a bare selected line, got %q / %q", res.Score, res.ProfilePath)
	}
}

// TestNormaliseWorkload_Good passes valid workloads through unchanged.
func TestNormaliseWorkload_Good(t *testing.T) {
	for _, w := range []string{"chat", "coding", "long_context", "agent_state", "throughput", "low_latency"} {
		if got := normaliseWorkload(w); got != w {
			t.Errorf("normaliseWorkload(%q) = %q, want unchanged", w, got)
		}
	}
}

// TestNormaliseWorkload_Bad falls back to chat for empty / unknown.
func TestNormaliseWorkload_Bad(t *testing.T) {
	for _, w := range []string{"", "  ", "bogus", "CHAT"} {
		if got := normaliseWorkload(w); got != "chat" {
			t.Errorf("normaliseWorkload(%q) = %q, want chat", w, got)
		}
	}
}

// --- Real tests for Service.Calibrate / Service.CalibrateStatus
// against a fake lthn-mlx binary. See calibrate_test.go for
// hermeticHome / newCalibrateServiceWithProcess / writeFakeLthnMlx —
// the hermetic PATH sandbox is required so resolveBinary() never
// finds the real (219MB) lthn-mlx built at ~/Code/core/go-mlx/bin on
// this dev machine.

const autotuneQuickSuccessScript = "#!/bin/sh\n" +
	"echo 'selected: c1 (score=0.9)'\n" +
	"echo 'saved:    /tmp/lemer-lite-chat.json'\n" +
	"exit 0\n"

// waitCalibrateDone polls CalibrateStatus with a bounded bail counter
// (background-waiters-need-bail-counters) rather than looping forever.
func waitCalibrateDone(t *testing.T, svc *Service) CalibrateProgress {
	t.Helper()
	for i := 0; i < 200; i++ {
		r := svc.CalibrateStatus()
		if !r.OK {
			t.Fatalf("CalibrateStatus: want OK, got fail: %v", r.Error())
		}
		p := r.Value.(CalibrateProgress)
		if p.Done {
			return p
		}
		time.Sleep(15 * time.Millisecond)
	}
	t.Fatal("calibration did not reach Done within the poll budget")
	return CalibrateProgress{}
}

func TestCalibrate_Service_Calibrate_Good_StartsJob(t *testing.T) {
	dir := t.TempDir()
	writeFakeLthnMlx(t, dir, autotuneQuickSuccessScript)
	svc := newCalibrateServiceWithProcess(t, dir)

	r := svc.Calibrate("chat", "/models/lemer-lite")
	if !r.OK {
		t.Fatalf("Calibrate: want OK, got fail: %v", r.Error())
	}
	job := r.Value.(CalibrateJob)
	if job.Workload != "chat" || job.Model != "/models/lemer-lite" {
		t.Errorf("job = %+v, want Workload=chat Model=/models/lemer-lite", job)
	}
}

func TestCalibrate_Service_Calibrate_Bad_NilCore(t *testing.T) {
	svc := &Service{}
	r := svc.Calibrate("chat", "/models/m")
	if r.OK {
		t.Fatal("Calibrate with nil core: want fail, got OK")
	}
	if !core.Contains(r.Error(), "core not bound") {
		t.Errorf("error = %q, want it to mention core not bound", r.Error())
	}
}

func TestCalibrate_Service_Calibrate_Bad_EmptyModel(t *testing.T) {
	hermeticHome(t)
	svc := NewService(core.New())
	r := svc.Calibrate("chat", "")
	if r.OK {
		t.Fatal("Calibrate with empty model: want fail, got OK")
	}
	if !core.Contains(r.Error(), "model path required") {
		t.Errorf("error = %q, want it to mention model path required", r.Error())
	}
}

func TestCalibrate_Service_Calibrate_Bad_NoProcessService(t *testing.T) {
	hermeticHome(t)
	svc := NewService(core.New())
	r := svc.Calibrate("chat", "/models/m")
	if r.OK {
		t.Fatal("Calibrate with no process service: want fail, got OK")
	}
	if !core.Contains(r.Error(), "process service unavailable") {
		t.Errorf("error = %q, want it to mention process service unavailable", r.Error())
	}
}

func TestCalibrate_Service_Calibrate_Bad_AlreadyRunning(t *testing.T) {
	dir := t.TempDir()
	writeFakeLthnMlx(t, dir, "#!/bin/sh\nsleep 1\necho 'selected: c1 (score=0.5)'\nexit 0\n")
	svc := newCalibrateServiceWithProcess(t, dir)

	first := svc.Calibrate("chat", "/models/m")
	if !first.OK {
		t.Fatalf("first Calibrate: want OK, got fail: %v", first.Error())
	}
	second := svc.Calibrate("coding", "/models/other")
	if second.OK {
		t.Fatal("second Calibrate while first is running: want fail, got OK")
	}
	if !core.Contains(second.Error(), "a calibration is already running") {
		t.Errorf("error = %q, want it to mention already running", second.Error())
	}
	// Drain so the background sleep doesn't outlive the test process.
	waitCalibrateDone(t, svc)
}

func TestCalibrate_Service_Calibrate_Ugly_StartFails(t *testing.T) {
	// No fake binary anywhere on PATH — newCalibrateServiceWithProcess("")
	// still guarantees an EMPTY PATH dir, never the ambient host PATH.
	svc := newCalibrateServiceWithProcess(t, "")
	r := svc.Calibrate("chat", "/models/m")
	if r.OK {
		t.Fatal("Calibrate against a missing binary: want fail, got OK")
	}
}

func TestCalibrate_Service_CalibrateStatus_Good_Idle(t *testing.T) {
	hermeticHome(t)
	svc := NewService(core.New())
	r := svc.CalibrateStatus()
	if !r.OK {
		t.Fatalf("CalibrateStatus: want OK, got fail: %v", r.Error())
	}
	p := r.Value.(CalibrateProgress)
	if !p.Idle {
		t.Error("Idle = false, want true when no calibration has ever run")
	}
}

func TestCalibrate_Service_CalibrateStatus_Bad_NilCore(t *testing.T) {
	svc := &Service{}
	r := svc.CalibrateStatus()
	if r.OK {
		t.Fatal("CalibrateStatus with nil core: want fail, got OK")
	}
	if !core.Contains(r.Error(), "core not bound") {
		t.Errorf("error = %q, want it to mention core not bound", r.Error())
	}
}

// TestCalibrate_Service_CalibrateStatus_Ugly_ProcessServiceRemovedAfterJobStarted
// reaches the "job != nil but process service unavailable" branch: a
// real *process.Process is obtained from a process.Service, then
// attached to a Service instance whose Core never registered
// "process" — CalibrateStatus must fail cleanly rather than panic on
// the missing service lookup (job.proc itself is never dereferenced
// before that check).
func TestCalibrate_Service_CalibrateStatus_Ugly_ProcessServiceRemovedAfterJobStarted(t *testing.T) {
	hermeticHome(t)
	procCore := core.New(core.WithName("process", process.NewService(process.Options{})))
	procSvc, ok := core.ServiceFor[*process.Service](procCore, "process")
	if !ok {
		t.Fatal("process service not registered on procCore")
	}
	startR := procSvc.Start(core.Background(), "sh", "-c", "true")
	if !startR.OK {
		t.Fatalf("process.Start: want OK, got fail: %v", startR.Error())
	}
	p := startR.Value.(*process.Process)

	svc := &Service{core: core.New()} // no "process" service on THIS core
	svc.job = &calibrateJob{proc: p, workload: "chat", model: "/models/m"}

	r := svc.CalibrateStatus()
	if r.OK {
		t.Fatal("CalibrateStatus with process service missing: want fail, got OK")
	}
	if !core.Contains(r.Error(), "process service unavailable") {
		t.Errorf("error = %q, want it to mention process service unavailable", r.Error())
	}
}

func TestCalibrate_Service_CalibrateStatus_Good_RunningThenDoneSuccess(t *testing.T) {
	dir := t.TempDir()
	writeFakeLthnMlx(t, dir, "#!/bin/sh\nsleep 0.3\necho 'selected: c2 (score=0.842)'\necho 'saved:    /tmp/lemer-lite-chat.json'\nexit 0\n")
	svc := newCalibrateServiceWithProcess(t, dir)

	startR := svc.Calibrate("chat", "/models/lemer-lite")
	if !startR.OK {
		t.Fatalf("Calibrate: want OK, got fail: %v", startR.Error())
	}

	// Poll once immediately — the fake script sleeps 0.3s, so this
	// should observe Running=true, Done=false at least once.
	immediate := svc.CalibrateStatus()
	if !immediate.OK {
		t.Fatalf("CalibrateStatus: want OK, got fail: %v", immediate.Error())
	}
	prog := immediate.Value.(CalibrateProgress)
	if prog.Workload != "chat" || prog.Model != "/models/lemer-lite" {
		t.Errorf("progress = %+v, want Workload=chat Model=/models/lemer-lite", prog)
	}

	final := waitCalibrateDone(t, svc)
	if !final.Done || final.Failed {
		t.Fatalf("final progress = %+v, want Done=true Failed=false", final)
	}
	if final.Result == nil {
		t.Fatal("Result is nil, want the parsed selected/saved fields")
	}
	if final.Result.SelectedID != "c2" || final.Result.Score != "0.842" {
		t.Errorf("Result = %+v, want SelectedID=c2 Score=0.842", final.Result)
	}
	if final.Result.Workload != "chat" || final.Result.Model != "/models/lemer-lite" {
		t.Errorf("Result Workload/Model = %q/%q, want chat//models/lemer-lite", final.Result.Workload, final.Result.Model)
	}
}

func TestCalibrate_Service_CalibrateStatus_Ugly_DoneFailedNonZeroExit(t *testing.T) {
	dir := t.TempDir()
	writeFakeLthnMlx(t, dir, "#!/bin/sh\necho 'boom' 1>&2\nexit 7\n")
	svc := newCalibrateServiceWithProcess(t, dir)

	startR := svc.Calibrate("chat", "/models/m")
	if !startR.OK {
		t.Fatalf("Calibrate: want OK, got fail: %v", startR.Error())
	}

	final := waitCalibrateDone(t, svc)
	if !final.Done || !final.Failed {
		t.Fatalf("final progress = %+v, want Done=true Failed=true", final)
	}
	if !core.Contains(final.Error, "exited with code 7") {
		t.Errorf("Error = %q, want it to mention exit code 7", final.Error)
	}
}
