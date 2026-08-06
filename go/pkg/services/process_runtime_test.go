// SPDX-Licence-Identifier: EUPL-1.2

// process_runtime_test.go covers namedProcessRuntime (the production
// ProcessRuntime implementation wired in service.go's Register — every
// other test in this package exercises the fakeProcessRuntime double
// instead, per service_test.go, leaving the real adapter's nil-guards
// and delegation entirely untested) and emptyWorkingDirectoryResolver.

package services

import (
	"testing"

	core "dappco.re/go"
	coreio "dappco.re/go/io"
	coreprocess "dappco.re/go/process"
)

// newWiredProcessRuntime boots a real *coreprocess.Service through a
// minimal Core container (mirrors registration_test.go's fixture) and
// returns it wrapped as the production ProcessRuntime adapter.
func newWiredProcessRuntime(t *testing.T) *namedProcessRuntime {
	t.Helper()
	c := core.New(
		core.WithName("process", coreprocess.NewService(coreprocess.Options{})),
		core.WithName("io", func(c *core.Core) core.Result {
			return core.Ok(&coreio.Service{
				ServiceRuntime: core.NewServiceRuntime(c, coreio.IOConfig{}),
				Medium:         coreio.NewMemoryMedium(),
			})
		}),
	)
	started := c.ServiceStartup(core.Background(), nil)
	if !started.OK {
		t.Fatalf("ServiceStartup: %s", started.Error())
	}
	t.Cleanup(func() { _ = c.ServiceShutdown(core.Background()) })

	svc, ok := core.ServiceFor[*coreprocess.Service](c, "process")
	if !ok || svc == nil {
		t.Fatal("process service not resolvable from Core")
	}
	return &namedProcessRuntime{service: svc}
}

// ---- nil-guards: nil *namedProcessRuntime receiver ------------------------

func TestNamedProcessRuntime_StartWithOptions_Bad_NilRuntime(t *testing.T) {
	var runtime *namedProcessRuntime
	r := runtime.StartWithOptions(core.Background(), coreprocess.RunOptions{Command: "x"})
	if r.OK {
		t.Fatal("expected Fail on a nil *namedProcessRuntime receiver")
	}
	if !core.Contains(r.Error(), "unavailable") {
		t.Fatalf("expected 'unavailable' in error, got %q", r.Error())
	}
}

func TestNamedProcessRuntime_Get_Bad_NilRuntime(t *testing.T) {
	var runtime *namedProcessRuntime
	r := runtime.Get("any-id")
	if r.OK {
		t.Fatal("expected Fail on a nil *namedProcessRuntime receiver")
	}
}

func TestNamedProcessRuntime_Signal_Bad_NilRuntime(t *testing.T) {
	var runtime *namedProcessRuntime
	r := runtime.Signal("any-id", SignalTerminate)
	if r.OK {
		t.Fatal("expected Fail on a nil *namedProcessRuntime receiver")
	}
}

func TestNamedProcessRuntime_Kill_Bad_NilRuntime(t *testing.T) {
	var runtime *namedProcessRuntime
	r := runtime.Kill("any-id")
	if r.OK {
		t.Fatal("expected Fail on a nil *namedProcessRuntime receiver")
	}
}

// ---- nil-guards: non-nil runtime, nil wrapped service ---------------------

func TestNamedProcessRuntime_StartWithOptions_Bad_NilService(t *testing.T) {
	runtime := &namedProcessRuntime{}
	r := runtime.StartWithOptions(core.Background(), coreprocess.RunOptions{Command: "x"})
	if r.OK {
		t.Fatal("expected Fail when the wrapped process service is nil")
	}
}

func TestNamedProcessRuntime_Get_Bad_NilService(t *testing.T) {
	runtime := &namedProcessRuntime{}
	r := runtime.Get("any-id")
	if r.OK {
		t.Fatal("expected Fail when the wrapped process service is nil")
	}
}

func TestNamedProcessRuntime_Signal_Bad_NilService(t *testing.T) {
	runtime := &namedProcessRuntime{}
	r := runtime.Signal("any-id", SignalTerminate)
	if r.OK {
		t.Fatal("expected Fail when the wrapped process service is nil")
	}
}

func TestNamedProcessRuntime_Kill_Bad_NilService(t *testing.T) {
	runtime := &namedProcessRuntime{}
	r := runtime.Kill("any-id")
	if r.OK {
		t.Fatal("expected Fail when the wrapped process service is nil")
	}
}

// ---- delegation: real wrapped service, fixture (nonexistent) command ------

// TestNamedProcessRuntime_StartWithOptions_Good_Delegates points the
// command at a fixture path that does not exist — the spawn itself
// fails (no hermetic-rule-violating real long-running binary is ever
// launched), but the call proves StartWithOptions delegates to the
// wrapped *coreprocess.Service rather than short-circuiting.
func TestNamedProcessRuntime_StartWithOptions_Good_Delegates(t *testing.T) {
	runtime := newWiredProcessRuntime(t)
	r := runtime.StartWithOptions(core.Background(), coreprocess.RunOptions{
		Command: "/nonexistent/lthn-fixture-binary-does-not-exist",
	})
	// The fixture binary does not exist, so the underlying spawn is
	// expected to fail — what matters here is that the call reached
	// the wrapped service (a nil-guard short-circuit would also
	// return !OK, but with a distinct "unavailable" message).
	if r.OK {
		t.Fatal("expected the nonexistent fixture binary to fail to spawn")
	}
	if core.Contains(r.Error(), "unavailable") {
		t.Fatalf("expected delegation to the wrapped service, got the nil-guard message: %q", r.Error())
	}
}

func TestNamedProcessRuntime_Get_Good_Delegates(t *testing.T) {
	runtime := newWiredProcessRuntime(t)
	r := runtime.Get("no-such-process-id")
	if r.OK {
		t.Fatal("expected Get to fail for an unknown process id")
	}
	if core.Contains(r.Error(), "unavailable") {
		t.Fatalf("expected delegation to the wrapped service, got the nil-guard message: %q", r.Error())
	}
}

func TestNamedProcessRuntime_Signal_Good_Delegates(t *testing.T) {
	runtime := newWiredProcessRuntime(t)
	r := runtime.Signal("no-such-process-id", SignalTerminate)
	if r.OK {
		t.Fatal("expected Signal to fail for an unknown process id")
	}
	if core.Contains(r.Error(), "unavailable") {
		t.Fatalf("expected delegation to the wrapped service, got the nil-guard message: %q", r.Error())
	}
}

func TestNamedProcessRuntime_Kill_Good_Delegates(t *testing.T) {
	runtime := newWiredProcessRuntime(t)
	r := runtime.Kill("no-such-process-id")
	if r.OK {
		t.Fatal("expected Kill to fail for an unknown process id")
	}
	if core.Contains(r.Error(), "unavailable") {
		t.Fatalf("expected delegation to the wrapped service, got the nil-guard message: %q", r.Error())
	}
}

// ---- emptyWorkingDirectoryResolver -----------------------------------------

func TestEmptyWorkingDirectoryResolver_Resolve_Good_Empty(t *testing.T) {
	var resolver emptyWorkingDirectoryResolver
	r := resolver.Resolve(WorkingDirectory{})
	if !r.OK {
		t.Fatalf("Resolve(empty): %s", r.Error())
	}
	if r.Value != "" {
		t.Fatalf("Resolve(empty).Value = %v, want empty string", r.Value)
	}
}

func TestEmptyWorkingDirectoryResolver_Resolve_Bad_NonEmpty(t *testing.T) {
	var resolver emptyWorkingDirectoryResolver
	r := resolver.Resolve(WorkingDirectory{MountID: "provider-mount"})
	if r.OK {
		t.Fatal("expected Resolve to fail for a non-empty WorkingDirectory")
	}
	failure, ok := r.Value.(*Failure)
	if !ok {
		t.Fatalf("Resolve(non-empty).Value type = %T, want *Failure", r.Value)
	}
	if failure.Code != ErrorWorkingDirectoryUnsupported {
		t.Fatalf("Failure.Code = %q, want %q", failure.Code, ErrorWorkingDirectoryUnsupported)
	}
}
