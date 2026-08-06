// SPDX-Licence-Identifier: EUPL-1.2

// process.go tests. The nil-service guard (procSvc returning nil) is
// exercised against a bare/zero Service; the real behaviour is
// exercised against a genuinely registered dappco.re/go/process
// Service — a real (but short-lived, non-networked, no fixed-port)
// subprocess per test, same as the process package's own test suite
// does. /bin/echo, /bin/sleep and cat are used because they exist on
// every POSIX dev/CI box this repo targets.

package bridge

import (
	core "dappco.re/go"
	"dappco.re/go/process"
)

func processHarness(t *core.T) *Service {
	t.Helper()
	c := core.New()
	r := process.NewService(process.Options{})(c)
	core.AssertTrue(t, r.OK)
	ps := r.Value.(*process.Service)
	core.AssertTrue(t, ps.OnStartup(core.Background()).OK)
	core.AssertTrue(t, c.RegisterService("process", ps).OK)
	return &Service{ServiceRuntime: core.NewServiceRuntime[Options](c, Options{}), port: 9999}
}

// ─── procSvc nil-guards ─────────────────────────────────────────────

func TestProcess_ProcSvc_Bad_NilService(t *core.T) {
	var s *Service
	core.AssertNil(t, s.procSvc())
}

func TestProcess_ProcSvc_Bad_NoServiceRuntime(t *core.T) {
	s := &Service{}
	core.AssertNil(t, s.procSvc())
}

func TestProcess_ProcSvc_Bad_ServiceNotRegistered(t *core.T) {
	c := core.New()
	s := &Service{ServiceRuntime: core.NewServiceRuntime[Options](c, Options{}), port: 9999}
	core.AssertNil(t, s.procSvc())
}

// ─── toolProcessStart ───────────────────────────────────────────────

func TestProcess_ToolProcessStart_Good(t *core.T) {
	s := processHarness(t)
	resp := s.toolProcessStart(core.Background(), map[string]any{
		"command": "/bin/echo", "args": []any{"hello"},
	})
	core.AssertEqual(t, true, resp["ok"])
	id, ok := resp["id"].(string)
	core.AssertTrue(t, ok)
	core.AssertNotEmpty(t, id)
}

func TestProcess_ToolProcessStart_Bad_ServiceUnavailable(t *core.T) {
	s := &Service{}
	resp := s.toolProcessStart(core.Background(), map[string]any{"command": "/bin/echo"})
	core.AssertEqual(t, false, resp["ok"])
	core.AssertEqual(t, processServiceUnavailable, resp["error"])
}

func TestProcess_ToolProcessStart_Bad_MissingCommandParam(t *core.T) {
	s := processHarness(t)
	resp := s.toolProcessStart(core.Background(), map[string]any{})
	core.AssertEqual(t, false, resp["ok"])
	core.AssertEqual(t, "command param required", resp["error"])
}

func TestProcess_ToolProcessStart_Ugly_CommandNotFound(t *core.T) {
	s := processHarness(t)
	resp := s.toolProcessStart(core.Background(), map[string]any{"command": "definitely-not-a-real-binary-xyz"})
	core.AssertEqual(t, false, resp["ok"])
	core.AssertNotEmpty(t, resp["error"])
}

// ─── toolProcessKill / toolProcessStop ──────────────────────────────

func TestProcess_ToolProcessKill_Good(t *core.T) {
	s := processHarness(t)
	start := s.toolProcessStart(core.Background(), map[string]any{"command": "/bin/sleep", "args": []any{"5"}})
	core.AssertEqual(t, true, start["ok"])
	id := start["id"].(string)

	resp := s.toolProcessKill(map[string]any{"id": id})
	core.AssertEqual(t, true, resp["ok"])
	core.AssertEqual(t, id, resp["killed"])
}

func TestProcess_ToolProcessKill_Bad_ServiceUnavailable(t *core.T) {
	s := &Service{}
	resp := s.toolProcessKill(map[string]any{"id": "x"})
	core.AssertEqual(t, false, resp["ok"])
	core.AssertEqual(t, processServiceUnavailable, resp["error"])
}

func TestProcess_ToolProcessKill_Bad_MissingIDParam(t *core.T) {
	s := processHarness(t)
	resp := s.toolProcessKill(map[string]any{})
	core.AssertEqual(t, false, resp["ok"])
	core.AssertEqual(t, idParamRequired, resp["error"])
}

func TestProcess_ToolProcessKill_Ugly_UnknownID(t *core.T) {
	s := processHarness(t)
	resp := s.toolProcessKill(map[string]any{"id": "no-such-process"})
	core.AssertEqual(t, false, resp["ok"])
	core.AssertNotEmpty(t, resp["error"])
}

func TestProcess_ToolProcessStop_Good_IsKillAlias(t *core.T) {
	s := processHarness(t)
	start := s.toolProcessStart(core.Background(), map[string]any{"command": "/bin/sleep", "args": []any{"5"}})
	id := start["id"].(string)
	resp := s.toolProcessStop(map[string]any{"id": id})
	core.AssertEqual(t, true, resp["ok"])
	core.AssertEqual(t, id, resp["killed"])
}

// ─── toolProcessList ────────────────────────────────────────────────

func TestProcess_ToolProcessList_Good(t *core.T) {
	s := processHarness(t)
	start := s.toolProcessStart(core.Background(), map[string]any{"command": "/bin/echo", "args": []any{"x"}})
	core.AssertEqual(t, true, start["ok"])

	resp := s.toolProcessList()
	core.AssertEqual(t, true, resp["ok"])
	count, ok := resp["count"].(int)
	core.AssertTrue(t, ok)
	core.AssertGreaterOrEqual(t, count, 1)
}

func TestProcess_ToolProcessList_Bad_ServiceUnavailable(t *core.T) {
	s := &Service{}
	resp := s.toolProcessList()
	core.AssertEqual(t, false, resp["ok"])
	core.AssertEqual(t, processServiceUnavailable, resp["error"])
}

// ─── toolProcessOutput ──────────────────────────────────────────────

func TestProcess_ToolProcessOutput_Good(t *core.T) {
	s := processHarness(t)
	start := s.toolProcessStart(core.Background(), map[string]any{"command": "/bin/echo", "args": []any{"captured"}})
	id := start["id"].(string)

	resp := s.toolProcessOutput(map[string]any{"id": id})
	core.AssertEqual(t, true, resp["ok"])
	_, ok := resp["value"].(string)
	core.AssertTrue(t, ok)
}

func TestProcess_ToolProcessOutput_Bad_ServiceUnavailable(t *core.T) {
	s := &Service{}
	resp := s.toolProcessOutput(map[string]any{"id": "x"})
	core.AssertEqual(t, false, resp["ok"])
	core.AssertEqual(t, processServiceUnavailable, resp["error"])
}

func TestProcess_ToolProcessOutput_Bad_MissingIDParam(t *core.T) {
	s := processHarness(t)
	resp := s.toolProcessOutput(map[string]any{})
	core.AssertEqual(t, false, resp["ok"])
	core.AssertEqual(t, idParamRequired, resp["error"])
}

func TestProcess_ToolProcessOutput_Ugly_UnknownID(t *core.T) {
	s := processHarness(t)
	resp := s.toolProcessOutput(map[string]any{"id": "no-such-id"})
	core.AssertEqual(t, false, resp["ok"])
	core.AssertNotEmpty(t, resp["error"])
}

// ─── toolProcessInput ───────────────────────────────────────────────

func TestProcess_ToolProcessInput_Good(t *core.T) {
	s := processHarness(t)
	start := s.toolProcessStart(core.Background(), map[string]any{"command": "/bin/cat"})
	core.AssertEqual(t, true, start["ok"])
	id := start["id"].(string)
	t.Cleanup(func() { s.toolProcessKill(map[string]any{"id": id}) })

	resp := s.toolProcessInput(map[string]any{"id": id, "input": "ping\n"})
	core.AssertEqual(t, true, resp["ok"])
	core.AssertEqual(t, 5, resp["bytes"])
}

func TestProcess_ToolProcessInput_Bad_ServiceUnavailable(t *core.T) {
	s := &Service{}
	resp := s.toolProcessInput(map[string]any{"id": "x", "input": "y"})
	core.AssertEqual(t, false, resp["ok"])
	core.AssertEqual(t, processServiceUnavailable, resp["error"])
}

func TestProcess_ToolProcessInput_Bad_MissingIDParam(t *core.T) {
	s := processHarness(t)
	resp := s.toolProcessInput(map[string]any{"input": "y"})
	core.AssertEqual(t, false, resp["ok"])
	core.AssertEqual(t, idParamRequired, resp["error"])
}

func TestProcess_ToolProcessInput_Ugly_ProcessAlreadyExited(t *core.T) {
	s := processHarness(t)
	start := s.toolProcessStart(core.Background(), map[string]any{"command": "/bin/echo", "args": []any{"done"}})
	id := start["id"].(string)

	// Real fault injection: give the short-lived echo a moment to exit,
	// then try to write to its (closed) stdin.
	for i := 0; i < 50; i++ {
		list := s.toolProcessList()
		found := false
		if procs, ok := list["value"].([]map[string]any); ok {
			for _, p := range procs {
				if p["id"] == id && p["running"] == true {
					found = true
				}
			}
		}
		if !found {
			break
		}
		core.Sleep(20 * core.Millisecond)
	}
	resp := s.toolProcessInput(map[string]any{"id": id, "input": "too late"})
	core.AssertEqual(t, false, resp["ok"])
}

// ─── stringSliceParam ───────────────────────────────────────────────

func TestProcess_StringSliceParam_Good_AnySlice(t *core.T) {
	out := stringSliceParam(map[string]any{"args": []any{"a", "b", ""}}, "args")
	core.AssertEqual(t, []string{"a", "b"}, out, "empty-string elements must be dropped")
}

func TestProcess_StringSliceParam_Good_StringSlice(t *core.T) {
	out := stringSliceParam(map[string]any{"args": []string{"x", "y"}}, "args")
	core.AssertEqual(t, []string{"x", "y"}, out)
}

func TestProcess_StringSliceParam_Bad_MissingKey(t *core.T) {
	out := stringSliceParam(map[string]any{}, "args")
	core.AssertNil(t, out)
}

func TestProcess_StringSliceParam_Ugly_WrongType(t *core.T) {
	out := stringSliceParam(map[string]any{"args": "not-a-slice"}, "args")
	core.AssertNil(t, out)
}
