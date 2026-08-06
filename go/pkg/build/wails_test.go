// SPDX-Licence-Identifier: EUPL-1.2

package build_test

import (
	core "dappco.re/go"
	"dappco.re/go/process"
	subject "dappco.re/lthn/desktop/pkg/build"
)

// buildWailsHarness returns a subject.Service backed by a real
// dappco.re/go/process Service registered under the "process" name —
// mirrors pkg/bridge/process_test.go's processHarness. Real (short-lived,
// no fixed-port) subprocesses only: /bin/echo and friends exist on every
// POSIX dev/CI box this repo targets.
func buildWailsHarness(t *core.T) *subject.Service {
	t.Helper()
	c := core.New()
	r := process.NewService(process.Options{})(c)
	core.AssertTrue(t, r.OK)
	ps := r.Value.(*process.Service)
	core.AssertTrue(t, ps.OnStartup(core.Background()).OK)
	core.AssertTrue(t, c.RegisterService("process", ps).OK)
	return subject.NewService(c)
}

func TestWails_Service_ServiceName_Good_ReturnsBuild(t *core.T) {
	svc := subject.NewService(nil)
	core.AssertEqual(t, "Build", svc.ServiceName())
}

func TestWails_Service_ServiceStartup_Good_NoOp(t *core.T) {
	svc := subject.NewService(nil)
	r := svc.ServiceStartup(core.Background(), nil)
	core.AssertTrue(t, r.OK)
}

func TestWails_Service_ServiceShutdown_Good_NoOp(t *core.T) {
	svc := subject.NewService(nil)
	r := svc.ServiceShutdown()
	core.AssertTrue(t, r.OK)
}

// --- Detect ---

func TestWails_Service_Detect_Bad_EmptyPath(t *core.T) {
	svc := subject.NewService(nil)
	r := svc.Detect("   ")
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "path required")
}

func TestWails_Service_Detect_Bad_NonexistentPath(t *core.T) {
	svc := subject.NewService(nil)
	r := svc.Detect("/definitely/not/a/real/path/xyz")
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "not a directory")
}

func TestWails_Service_Detect_Bad_PathIsFile(t *core.T) {
	dir := t.TempDir()
	filePath := core.PathJoin(dir, "plain.txt")
	core.AssertTrue(t, core.WriteFile(filePath, []byte("x"), 0o644).OK)

	svc := subject.NewService(nil)
	r := svc.Detect(filePath)
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "not a directory")
}

func TestWails_Service_Detect_Good_DirectoryDetected(t *core.T) {
	dir := t.TempDir()
	svc := subject.NewService(nil)
	r := svc.Detect(dir)
	core.AssertTrue(t, r.OK)
	d, ok := r.Value.(subject.Detection)
	core.AssertTrue(t, ok)
	core.AssertEqual(t, dir, d.Path)
}

// --- Run ---

func TestWails_Service_Run_Bad_EmptyPath(t *core.T) {
	svc := subject.NewService(nil)
	r := svc.Run("  ", "", nil)
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "path required")
}

func TestWails_Service_Run_Bad_NoCommandDetected(t *core.T) {
	svc := subject.NewService(nil)
	r := svc.Run(t.TempDir(), "", nil)
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "could not detect a build command")
}

func TestWails_Service_Run_Bad_ProcessServiceUnavailable(t *core.T) {
	svc := subject.NewService(core.New())
	r := svc.Run(t.TempDir(), "/bin/echo", []string{"hi"})
	core.AssertFalse(t, r.OK)
}

func TestWails_Service_Run_Good_ExplicitCommand(t *core.T) {
	svc := buildWailsHarness(t)
	r := svc.Run(t.TempDir(), "/bin/echo", []string{"hi"})
	core.AssertTrue(t, r.OK)
	out, ok := r.Value.(subject.RunResult)
	core.AssertTrue(t, ok)
	core.AssertNotEmpty(t, out.ProcessID)
	core.AssertEqual(t, "/bin/echo", out.BuildCommand)
}

// --- ProcessOutput ---

func TestWails_Service_ProcessOutput_Bad_ServiceUnavailable(t *core.T) {
	svc := subject.NewService(core.New())
	r := svc.ProcessOutput("whatever")
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "process service unavailable")
}

func TestWails_Service_ProcessOutput_Bad_NotFound(t *core.T) {
	svc := buildWailsHarness(t)
	r := svc.ProcessOutput("no-such-id")
	core.AssertFalse(t, r.OK)
}

func TestWails_Service_ProcessOutput_Good_ReturnsOutput(t *core.T) {
	svc := buildWailsHarness(t)
	run := svc.Run(t.TempDir(), "/bin/echo", []string{"hi"})
	core.AssertTrue(t, run.OK)
	id := run.Value.(subject.RunResult).ProcessID

	r := svc.ProcessOutput(id)
	core.AssertTrue(t, r.OK)
	_, ok := r.Value.(string)
	core.AssertTrue(t, ok)
}

// --- ProcessKill ---

func TestWails_Service_ProcessKill_Bad_ServiceUnavailable(t *core.T) {
	svc := subject.NewService(core.New())
	r := svc.ProcessKill("whatever")
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "process service unavailable")
}

func TestWails_Service_ProcessKill_Bad_NotFound(t *core.T) {
	svc := buildWailsHarness(t)
	r := svc.ProcessKill("no-such-id")
	core.AssertFalse(t, r.OK)
}

func TestWails_Service_ProcessKill_Good_IdempotentOnFinishedProcess(t *core.T) {
	svc := buildWailsHarness(t)
	run := svc.Run(t.TempDir(), "/bin/sleep", []string{"5"})
	core.AssertTrue(t, run.OK)
	id := run.Value.(subject.RunResult).ProcessID

	r1 := svc.ProcessKill(id)
	core.AssertTrue(t, r1.OK)

	// Idempotent — killing an already-finished process is a no-op success.
	r2 := svc.ProcessKill(id)
	core.AssertTrue(t, r2.OK)
}

// --- ProcessList ---

func TestWails_Service_ProcessList_Bad_ServiceUnavailable(t *core.T) {
	svc := subject.NewService(core.New())
	r := svc.ProcessList()
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "process service unavailable")
}

func TestWails_Service_ProcessList_Good_ReturnsEntries(t *core.T) {
	svc := buildWailsHarness(t)
	run := svc.Run(t.TempDir(), "/bin/echo", []string{"hi"})
	core.AssertTrue(t, run.OK)

	r := svc.ProcessList()
	core.AssertTrue(t, r.OK)
	entries, ok := r.Value.([]subject.ProcessEntry)
	core.AssertTrue(t, ok)
	core.AssertGreater(t, len(entries), 0)
}
