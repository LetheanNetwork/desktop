// SPDX-Licence-Identifier: EUPL-1.2

package tasks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dappco.re/lthn/desktop/pkg/auth"
)

// fakePath prepends a t.TempDir() containing the named fake-binary
// scripts (see writeScript in detect_test.go) onto PATH, so
// captureCommand("go", ...) / captureCommand("composer", ...) resolve
// to the fixtures rather than any real toolchain on the box. process
// package's lookPath re-resolves PATH on every call (no caching), so
// this is safe to set per-test via t.Setenv.
func fakePath(t *testing.T, bin map[string]string) {
	t.Helper()
	dir := t.TempDir()
	for name, body := range bin {
		writeScript(t, dir, name, body)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestUpdates_ArrowLines(t *testing.T) {
	out := "\n" + // blank
		"go: downloading something\n" + // tool chatter, no arrow
		"dappco.re/go: v0.10.0 → v0.10.3\n" +
		"   golang.org/x/sys: v0.1.0 → v0.2.0   \n" + // indented + trailing space
		"\n"
	got := arrowLines(out)
	if len(got) != 2 {
		t.Fatalf("arrowLines = %v (want 2 update lines)", got)
	}
	if got[0] != "dappco.re/go: v0.10.0 → v0.10.3" || got[1] != "golang.org/x/sys: v0.1.0 → v0.2.0" {
		t.Errorf("arrowLines content = %q", got)
	}
}

func TestUpdates_ArrowLines_NoneOutdated(t *testing.T) {
	// `go list -f` emits a blank line per up-to-date module — all dropped.
	if got := arrowLines("\n\n\n"); len(got) != 0 {
		t.Errorf("arrowLines(blanks) = %v, want empty", got)
	}
}

func TestUpdates_Description(t *testing.T) {
	d := updateDescription("Go", []string{"foo: v1 → v2", "bar: v3 → v4"})
	for _, want := range []string{"2 outdated Go", "Package Update template", "- foo: v1 → v2", "- bar: v3 → v4"} {
		if !strings.Contains(d, want) {
			t.Errorf("updateDescription missing %q in:\n%s", want, d)
		}
	}
}

// --- outdatedGoModules / outdatedComposerPackages — hermetic PATH fixtures ---

func TestUpdates_OutdatedGoModules_Good(t *testing.T) {
	ensureProcess(t)
	fakePath(t, map[string]string{
		"go": "echo \"dappco.re/go: v0.10.0 → v0.10.3\"\n" +
			"echo \"golang.org/x/sys: v0.1.0 → v0.2.0\"\n",
	})
	got := outdatedGoModules(t.TempDir())
	if len(got) != 2 {
		t.Fatalf("outdatedGoModules = %v, want 2 lines", got)
	}
}

func TestUpdates_OutdatedGoModules_NoneOutdated_Bad(t *testing.T) {
	ensureProcess(t)
	fakePath(t, map[string]string{"go": "true\n"})
	got := outdatedGoModules(t.TempDir())
	if len(got) != 0 {
		t.Errorf("outdatedGoModules = %v, want empty", got)
	}
}

func TestUpdates_OutdatedComposerPackages_Good(t *testing.T) {
	ensureProcess(t)
	fakePath(t, map[string]string{
		"composer": `cat <<'JSON'
{"installed":[{"name":"foo/bar","version":"1.0.0","latest":"1.2.0"},{"name":"baz/qux","version":"2.0.0","latest":"2.1.0"}]}
JSON
`,
	})
	got := outdatedComposerPackages(t.TempDir())
	if len(got) != 2 {
		t.Fatalf("outdatedComposerPackages = %v, want 2", got)
	}
	if got[0] != "foo/bar: 1.0.0 → 1.2.0" {
		t.Errorf("line 0 = %q", got[0])
	}
}

func TestUpdates_OutdatedComposerPackages_NoJSON_Bad(t *testing.T) {
	ensureProcess(t)
	fakePath(t, map[string]string{"composer": "echo \"composer not installed or errored\"\n"})
	got := outdatedComposerPackages(t.TempDir())
	if got != nil {
		t.Errorf("outdatedComposerPackages = %v, want nil", got)
	}
}

func TestUpdates_OutdatedComposerPackages_MalformedJSON_Ugly(t *testing.T) {
	ensureProcess(t)
	fakePath(t, map[string]string{"composer": `echo '{"installed": [this is not valid json]}'` + "\n"})
	got := outdatedComposerPackages(t.TempDir())
	if got != nil {
		t.Errorf("outdatedComposerPackages(malformed) = %v, want nil", got)
	}
}

// --- DetectUpdates — full detector flow over both ecosystems ---

// TestUpdates_DetectUpdates_Good runs both ecosystems against fake go +
// composer tools, then re-runs to prove the dedup path.
func TestUpdates_DetectUpdates_Good(t *testing.T) {
	ensureProcess(t)
	c := newDetectCore(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "composer.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	fakePath(t, map[string]string{
		"go":       "echo \"dappco.re/go: v0.10.0 → v0.10.3\"\n",
		"composer": `echo '{"installed":[{"name":"foo/bar","version":"1.0","latest":"1.1"}]}'` + "\n",
	})

	r := DetectUpdates(c, DetectInput{Repo: "x", Path: dir})
	if !r.OK {
		t.Fatalf("DetectUpdates: %s", r.Error())
	}
	out, ok := r.Value.(DetectOutput)
	if !ok {
		t.Fatalf("DetectUpdates value type = %T", r.Value)
	}
	if out.Findings != 2 || out.Created != 2 {
		t.Errorf("DetectUpdates = %+v, want Findings=2 Created=2 (one task per ecosystem)", out)
	}

	r2 := DetectUpdates(c, DetectInput{Repo: "x", Path: dir})
	if !r2.OK {
		t.Fatalf("DetectUpdates (rerun): %s", r2.Error())
	}
	out2, _ := r2.Value.(DetectOutput)
	if out2.Skipped != 2 || out2.Created != 0 {
		t.Errorf("rerun = %+v, want Skipped=2 Created=0 (dedup)", out2)
	}
}

func TestUpdates_DetectUpdates_Bad(t *testing.T) {
	c := newDetectCore(t)
	if r := DetectUpdates(c, DetectInput{}); r.OK {
		t.Error("DetectUpdates with empty repo/path should fail")
	}
	if r := DetectUpdates(c, DetectInput{Repo: "x"}); r.OK {
		t.Error("DetectUpdates with empty path should fail")
	}
}

// TestUpdates_DetectUpdates_Ugly walks the two "nothing to do" / narrowing
// shapes: no manifests present (both ecosystems skipped) and a Lang
// filter that narrows a two-manifest repo down to one ecosystem.
func TestUpdates_DetectUpdates_Ugly(t *testing.T) {
	ensureProcess(t)
	c := newDetectCore(t)

	noManifests := t.TempDir()
	r := DetectUpdates(c, DetectInput{Repo: "bare", Path: noManifests})
	if !r.OK {
		t.Fatalf("DetectUpdates(no manifests): %s", r.Error())
	}
	out, _ := r.Value.(DetectOutput)
	if out.Findings != 0 || out.Created != 0 {
		t.Errorf("DetectUpdates(no manifests) = %+v, want zero", out)
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "composer.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	fakePath(t, map[string]string{
		"go":       "echo \"dappco.re/go: v0.10.0 → v0.10.3\"\n",
		"composer": `echo '{"installed":[{"name":"foo/bar","version":"1.0","latest":"1.1"}]}'` + "\n",
	})
	r2 := DetectUpdates(c, DetectInput{Repo: "narrowed", Path: dir, Lang: "go"})
	if !r2.OK {
		t.Fatalf("DetectUpdates(Lang=go): %s", r2.Error())
	}
	out2, _ := r2.Value.(DetectOutput)
	if out2.Findings != 1 || out2.Created != 1 {
		t.Errorf("DetectUpdates(Lang=go) = %+v, want just the Go ecosystem", out2)
	}
}

// --- Service.DetectUpdates / WailsService.DetectUpdates — tier gate ---

func TestUpdates_ServiceDetectUpdates_Bad(t *testing.T) {
	c := newDetectCore(t)
	svc := NewService(c)
	if r := svc.DetectUpdates(DetectInput{Repo: "x", Path: t.TempDir()}); r.OK {
		t.Error("Service.DetectUpdates should reject an unstamped caller")
	}
}

func TestUpdates_ServiceDetectUpdates_Good(t *testing.T) {
	ensureProcess(t)
	c := newDetectCore(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fakePath(t, map[string]string{"go": "true\n"})
	auth.SetCaller(c, auth.CallerIdentity{Tier: auth.TierOperator, Subject: "op", Source: "test"})
	svc := NewService(c)
	if r := svc.DetectUpdates(DetectInput{Repo: "x", Path: dir}); !r.OK {
		t.Fatalf("Service.DetectUpdates: %s", r.Error())
	}
}

func TestUpdates_WailsServiceDetectUpdates_Ugly(t *testing.T) {
	ensureProcess(t)
	c := newDetectCore(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fakePath(t, map[string]string{"go": "true\n"})
	w := NewWailsService(NewService(c))
	if r := w.DetectUpdates(DetectInput{Repo: "x", Path: dir}); !r.OK {
		t.Fatalf("WailsService.DetectUpdates: %s", r.Error())
	}
}
