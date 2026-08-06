// SPDX-Licence-Identifier: EUPL-1.2

// Real tests for wails.go's Wails-bound surface (Detect / List /
// Logs). Companion to container_test.go's diagnosis: the pre-
// existing wails_example_test.go only reflects on method values and
// never invokes them. These tests drive the real methods against
// fake docker/podman binaries on PATH plus real fault injection
// (non-zero exits, garbage JSON, unsupported runtime names).

package container

import (
	"os"
	"testing"

	core "dappco.re/go"
)

func TestWails_Service_Detect_Good_ReturnsAvailableSubset(t *testing.T) {
	withFakeRuntimesOnPath(t)
	svc := newServiceWithProcess(t)

	r := svc.Detect(false)
	if !r.OK {
		t.Fatalf("Detect: want OK, got fail: %v", r.Error())
	}
	det, ok := r.Value.(Detection)
	if !ok {
		t.Fatalf("Detect value is %T, want Detection", r.Value)
	}
	if len(det.Runtimes) != 5 {
		t.Errorf("Runtimes len = %d, want 5", len(det.Runtimes))
	}
	if len(det.Available) != 2 {
		t.Errorf("Available len = %d, want 2 (docker+podman fakes)", len(det.Available))
	}
	for _, r := range det.Available {
		if !r.Available {
			t.Errorf("entry %q in Available list has Available=false", r.Name)
		}
	}
}

func TestWails_Service_Detect_Bad_ForceParamIgnoredButAccepted(t *testing.T) {
	svc := newServiceWithProcess(t)
	rFalse := svc.Detect(false)
	rTrue := svc.Detect(true)
	if !rFalse.OK || !rTrue.OK {
		t.Fatalf("Detect(false)/Detect(true) both want OK")
	}
	// force is accepted for API parity but not honoured — both calls
	// must produce the same shape (no caching difference to observe).
	df := rFalse.Value.(Detection)
	dt := rTrue.Value.(Detection)
	if len(df.Runtimes) != len(dt.Runtimes) {
		t.Errorf("Detect(false) vs Detect(true) runtime count differs: %d vs %d", len(df.Runtimes), len(dt.Runtimes))
	}
}

func TestWails_Service_List_Good_CombinesDockerAndPodman(t *testing.T) {
	withFakeRuntimesOnPath(t)
	svc := newServiceWithProcess(t)

	r := svc.List("")
	if !r.OK {
		t.Fatalf("List: want OK, got fail: %v", r.Error())
	}
	out, ok := r.Value.(ListOutput)
	if !ok {
		t.Fatalf("List value is %T, want ListOutput", r.Value)
	}
	if out.Count != 2 {
		t.Fatalf("Count = %d, want 2 (one docker + one podman container)", out.Count)
	}
	if len(out.Containers) != 2 {
		t.Fatalf("len(Containers) = %d, want 2", len(out.Containers))
	}
	var sawDocker, sawPodman bool
	for _, c := range out.Containers {
		switch c.Runtime {
		case "docker":
			sawDocker = true
			if c.ID != "abc123456789" { // shortID truncates to 12
				t.Errorf("docker container ID = %q, want 12-char shortID", c.ID)
			}
			if c.Name != "web1" || c.Image != "nginx:latest" {
				t.Errorf("docker container fields wrong: %+v", c)
			}
		case "podman":
			sawPodman = true
			if c.ID != "deadbeef1234" {
				t.Errorf("podman container ID = %q, want 12-char shortID", c.ID)
			}
			if c.Name != "pod1" || c.Image != "alpine:3.21" {
				t.Errorf("podman container fields wrong: %+v", c)
			}
		}
	}
	if !sawDocker || !sawPodman {
		t.Errorf("expected both runtimes represented: sawDocker=%v sawPodman=%v", sawDocker, sawPodman)
	}
}

func TestWails_Service_List_Bad_FiltersToSingleRuntime(t *testing.T) {
	withFakeRuntimesOnPath(t)
	svc := newServiceWithProcess(t)

	r := svc.List("docker")
	if !r.OK {
		t.Fatalf("List(docker): want OK, got fail: %v", r.Error())
	}
	out := r.Value.(ListOutput)
	if out.Count != 1 {
		t.Fatalf("Count = %d, want 1 (docker only)", out.Count)
	}
	if out.Containers[0].Runtime != "docker" {
		t.Errorf("Runtime = %q, want docker", out.Containers[0].Runtime)
	}
}

func TestWails_Service_List_Ugly_NoProcessServiceReturnsEmpty(t *testing.T) {
	svc := &Service{} // no process.Service wired
	r := svc.List("")
	if !r.OK {
		t.Fatalf("List: want OK even with no process service, got fail: %v", r.Error())
	}
	out := r.Value.(ListOutput)
	if out.Count != 0 || len(out.Containers) != 0 {
		t.Errorf("List with no process service: Count=%d len(Containers)=%d, want both 0", out.Count, len(out.Containers))
	}
}

func TestWails_Service_List_Ugly_GarbageJSONLineSkipped(t *testing.T) {
	dir := t.TempDir()
	script := "#!/bin/sh\n" +
		"if [ \"$1\" = \"ps\" ]; then\n" +
		"  echo 'not json at all'\n" +
		"  echo '{\"ID\":\"good1234567890\",\"Names\":\"ok\",\"Image\":\"alpine\",\"Status\":\"Up\",\"CreatedAt\":\"now\"}'\n" +
		"  exit 0\n" +
		"fi\n" +
		"exit 1\n"
	writeFakeBinary(t, dir, "docker", script)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	svc := newServiceWithProcess(t)
	r := svc.List("docker")
	if !r.OK {
		t.Fatalf("List: want OK, got fail: %v", r.Error())
	}
	out := r.Value.(ListOutput)
	if out.Count != 1 {
		t.Fatalf("Count = %d, want 1 (garbage line skipped, good line kept)", out.Count)
	}
	if out.Containers[0].Name != "ok" {
		t.Errorf("Containers[0].Name = %q, want ok", out.Containers[0].Name)
	}
}

func TestWails_Service_Logs_Good_Docker(t *testing.T) {
	withFakeRuntimesOnPath(t)
	svc := newServiceWithProcess(t)

	r := svc.Logs("cid1", "docker", 50)
	if !r.OK {
		t.Fatalf("Logs: want OK, got fail: %v", r.Error())
	}
	out := r.Value.(LogsOutput)
	if out.ID != "cid1" || out.Runtime != "docker" {
		t.Errorf("LogsOutput = %+v, want ID=cid1 Runtime=docker", out)
	}
	if out.Logs == "" {
		t.Error("Logs is empty, want the fake docker log lines")
	}
}

func TestWails_Service_Logs_Good_PodmanDefaultsTail(t *testing.T) {
	withFakeRuntimesOnPath(t)
	svc := newServiceWithProcess(t)

	// tail<=0 must default to 200; runtime="" must default to docker,
	// so pass "podman" explicitly here to prove the podman arm too.
	r := svc.Logs("cid2", "podman", 0)
	if !r.OK {
		t.Fatalf("Logs: want OK, got fail: %v", r.Error())
	}
	out := r.Value.(LogsOutput)
	if out.Runtime != "podman" {
		t.Errorf("Runtime = %q, want podman", out.Runtime)
	}
}

func TestWails_Service_Logs_Good_EmptyRuntimeDefaultsDocker(t *testing.T) {
	withFakeRuntimesOnPath(t)
	svc := newServiceWithProcess(t)

	r := svc.Logs("cid3", "", 10)
	if !r.OK {
		t.Fatalf("Logs: want OK, got fail: %v", r.Error())
	}
	out := r.Value.(LogsOutput)
	if out.Runtime != "docker" {
		t.Errorf("Runtime = %q, want docker (default)", out.Runtime)
	}
}

func TestWails_Service_Logs_Bad_EmptyID(t *testing.T) {
	svc := newServiceWithProcess(t)
	r := svc.Logs("", "docker", 10)
	if r.OK {
		t.Fatal("Logs(empty id): want fail, got OK")
	}
	if !core.Contains(r.Error(), "id is required") {
		t.Errorf("error = %q, want it to mention id is required", r.Error())
	}
}

func TestWails_Service_Logs_Bad_UnsupportedRuntime(t *testing.T) {
	svc := newServiceWithProcess(t)
	r := svc.Logs("cid", "lthn-fake-runtime", 10)
	if r.OK {
		t.Fatal("Logs(unsupported runtime): want fail, got OK")
	}
	if !core.Contains(r.Error(), "unsupported runtime") {
		t.Errorf("error = %q, want it to mention unsupported runtime", r.Error())
	}
}

func TestWails_Service_Logs_Ugly_NoProcessService(t *testing.T) {
	svc := &Service{}
	r := svc.Logs("cid", "docker", 10)
	if r.OK {
		t.Fatal("Logs with no process service: want fail, got OK")
	}
	if !core.Contains(r.Error(), "process service unavailable") {
		t.Errorf("error = %q, want it to mention process service unavailable", r.Error())
	}
}

func TestWails_Service_Logs_Ugly_ProcessRunFails(t *testing.T) {
	dir := t.TempDir()
	script := "#!/bin/sh\necho 'no such container' 1>&2\nexit 1\n"
	writeFakeBinary(t, dir, "docker", script)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	svc := newServiceWithProcess(t)
	r := svc.Logs("missing-cid", "docker", 10)
	if r.OK {
		t.Fatal("Logs against a failing runtime: want fail, got OK")
	}
	if !core.Contains(r.Error(), "container.Logs") {
		t.Errorf("error = %q, want it to carry the container.Logs op prefix", r.Error())
	}
}
