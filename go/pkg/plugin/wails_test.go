// SPDX-Licence-Identifier: EUPL-1.2

// Internal-package tests for wails.go — Install, Remove, Start, Stop
// (re-exercised at the Install/Remove level, not just runtime_test.go's
// direct startPlugin/stopPlugin calls), List, Status. package plugin so
// setup/assertions can reach s.state directly.
//
// Install() always ends by calling s.Start(), which spawns the
// just-written binary via process.Service. Every Install test here
// writes deliberately NON-executable payload bytes ("not-a-real-binary"
// plain text) as the plugin "binary" — writePlugin sets the exec bit
// (0o755) on whatever bytes it's given, but content that isn't a valid
// executable format still makes the OS refuse to run it (ENOEXEC), so
// cmd.Start() fails synchronously exactly like runtime_test.go's
// spawn-fault-injection cases. That means every Install test below
// exercises manifest build + validate, the LocalPath/fetch branch,
// checksum verification, the pre-existing-plugin stop, writePlugin, and
// the call into Start — everything except the final `core.Ok(...)`
// success return, which requires a real running plugin process
// (the same structural exec boundary documented in runtime_test.go).
package plugin

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"os"

	core "dappco.re/go"
	"dappco.re/go/process"
)

// chmodPath is the test-only stand-in for os.Chmod (banned in
// production per AX-6 — real I/O goes through core.Fs()/c.Process()).
func chmodPath(path string, mode os.FileMode) bool {
	return os.Chmod(path, mode) == nil
}

// httpHandlerWritingBytes returns an http.HandlerFunc that writes body
// verbatim — the loopback fixture's response payload.
func httpHandlerWritingBytes(body []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(body)
	}
}

// ─── Install ─────────────────────────────────────────────────────────────

func TestWails_Install_Bad_ValidationFails(t *core.T) {
	svc := newTestService(t, core.New())
	r := svc.Install(InstallInput{}) // empty code -> manifest.validate() fails
	core.AssertFalse(t, r.OK)
}

func TestWails_Install_Bad_MissingBinarySource(t *core.T) {
	svc := newTestService(t, core.New())
	r := svc.Install(InstallInput{Code: "opencode", Name: "OpenCode"})
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "binary_url or local_path required")
}

func TestWails_Install_Bad_LocalPathRejected(t *core.T) {
	svc := newTestService(t, core.New())
	r := svc.Install(InstallInput{
		Code: "opencode", Name: "OpenCode",
		LocalPath: "/etc/passwd",
		Checksum:  "sha256:0000000000000000000000000000000000000000000000000000000000000000",
	})
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "local_path rejected")
}

func TestWails_Install_Bad_ChecksumMismatch(t *core.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	src := tmp + "/Downloads/fake.bin"
	core.RequireTrue(t, core.MkdirAll(tmp+"/Downloads", 0o755).OK)
	core.RequireTrue(t, core.WriteFile(src, []byte("not-a-real-binary"), 0o644).OK)

	svc := newTestService(t, core.New())
	r := svc.Install(InstallInput{
		Code: "opencode", Name: "OpenCode",
		LocalPath: src,
		Checksum:  "sha256:0000000000000000000000000000000000000000000000000000000000000000",
	})
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "checksum")
}

// TestWails_Install_Bad_LocalPathWriteThenStartFails covers the
// LocalPath branch end to end: gate passes, ReadFile succeeds, checksum
// matches, any pre-existing instance is stopped (no-op — nothing
// running yet), writePlugin lays the plugin out on disk, then Start
// fails because "not-a-real-binary" isn't an executable format.
func TestWails_Install_Bad_LocalPathWriteThenStartFails(t *core.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	src := tmp + "/Downloads/fake.bin"
	core.RequireTrue(t, core.MkdirAll(tmp+"/Downloads", 0o755).OK)
	payload := []byte("not-a-real-binary")
	core.RequireTrue(t, core.WriteFile(src, payload, 0o644).OK)
	sum := sha256.Sum256(payload)
	checksum := "sha256:" + hex.EncodeToString(sum[:])

	c := core.New(core.WithService(process.Register))
	svc := newTestService(t, c)
	r := svc.Install(InstallInput{
		Code: "opencode", Name: "OpenCode", Namespace: "opencode",
		LocalPath: src, Checksum: checksum,
	})
	core.AssertFalse(t, r.OK, "fake payload can't execute — Start fails")

	// writePlugin's tail DID land before Start ran.
	written := core.ReadFile(tmp + "/Lethean/conf/plugins/opencode/bin/opencode")
	core.RequireTrue(t, written.OK)
	core.AssertEqual(t, "not-a-real-binary", string(written.Value.([]byte)))
}

// TestWails_Install_Bad_FetchedBinaryWriteThenStartFails covers the
// BinaryURL/fetch branch end to end using the loopback GitHub transport
// fixture from install_test.go.
func TestWails_Install_Bad_FetchedBinaryWriteThenStartFails(t *core.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	payload := []byte("not-a-real-binary")
	withLoopbackGithubTransport(t, httpHandlerWritingBytes(payload))
	sum := sha256.Sum256(payload)
	checksum := "sha256:" + hex.EncodeToString(sum[:])

	c := core.New(core.WithService(process.Register))
	svc := newTestService(t, c)
	r := svc.Install(InstallInput{
		Code: "opencode", Name: "OpenCode", Namespace: "opencode",
		BinaryURL: fakeGithubBinaryURL, Checksum: checksum,
	})
	core.AssertFalse(t, r.OK)

	written := core.ReadFile(tmp + "/Lethean/conf/plugins/opencode/bin/opencode")
	core.RequireTrue(t, written.OK)
}

// ─── Remove ──────────────────────────────────────────────────────────────

func TestWails_Remove_Good_DeletesInstalledPlugin(t *core.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	dir := tmp + "/Lethean/conf/plugins/opencode"
	core.RequireTrue(t, core.MkdirAll(dir, 0o755).OK)
	core.RequireTrue(t, core.WriteFile(dir+"/plugin.json", []byte("{}"), 0o644).OK)

	svc := newTestService(t, core.New())
	r := svc.Remove("opencode")
	core.RequireTrue(t, r.OK)

	listing := core.ReadDir(core.DirFS(tmp+"/Lethean/conf/plugins"), ".")
	if listing.OK {
		for _, e := range listing.Value.([]core.FsDirEntry) {
			core.AssertNotEqual(t, "opencode", e.Name())
		}
	}
	_, tracked := svc.state["opencode"]
	core.AssertFalse(t, tracked)
}

func TestWails_Remove_Good_UntrackedAndNeverInstalledIsNoOp(t *core.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	svc := newTestService(t, core.New())
	r := svc.Remove("never-installed")
	core.AssertTrue(t, r.OK)
}

// TestWails_Remove_Bad_DeleteFails is real fault injection: the parent
// directory's write bit is stripped so unlinking the child fails.
func TestWails_Remove_Bad_DeleteFails(t *core.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	root := tmp + "/Lethean/conf/plugins"
	dir := root + "/opencode"
	core.RequireTrue(t, core.MkdirAll(dir, 0o755).OK)
	core.RequireTrue(t, core.WriteFile(dir+"/plugin.json", []byte("{}"), 0o644).OK)
	core.RequireTrue(t, chmodPath(root, 0o500))
	t.Cleanup(func() { _ = chmodPath(root, 0o755) })

	svc := newTestService(t, core.New())
	r := svc.Remove("opencode")
	core.AssertFalse(t, r.OK)
}

// ─── List ────────────────────────────────────────────────────────────────

func TestWails_List_Good_EmptyWhenNothingInstalled(t *core.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	svc := newTestService(t, core.New())
	r := svc.List()
	core.RequireTrue(t, r.OK)
	core.AssertEqual(t, 0, len(r.Value.(ListOutput).Plugins))
}

func TestWails_List_Good_ListsInstalledPlugins(t *core.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	dir := tmp + "/Lethean/conf/plugins/opencode"
	core.RequireTrue(t, core.MkdirAll(dir, 0o755).OK)
	core.RequireTrue(t, saveManifest(dir, Manifest{Code: "opencode", Name: "OpenCode", Binary: "bin/opencode"}).OK)

	svc := newTestService(t, core.New())
	r := svc.List()
	core.RequireTrue(t, r.OK)
	plugins := r.Value.(ListOutput).Plugins
	core.RequireTrue(t, len(plugins) == 1)
	core.AssertEqual(t, "opencode", plugins[0].Code)
	core.AssertEqual(t, dir, plugins[0].Dir)
}

// ─── Status ──────────────────────────────────────────────────────────────

func TestWails_Status_Good_UnknownReturnsStoppedStub(t *core.T) {
	svc := newTestService(t, core.New())
	r := svc.Status("never-started")
	core.RequireTrue(t, r.OK)
	core.AssertEqual(t, "stopped", r.Value.(Status).State)
}

func TestWails_Status_Good_TrackedReturnsSnapshot(t *core.T) {
	svc := newTestService(t, core.New())
	svc.state["x"] = &pluginState{state: "running", port: 4321, pid: 77}
	r := svc.Status("x")
	core.RequireTrue(t, r.OK)
	st := r.Value.(Status)
	core.AssertEqual(t, "running", st.State)
	core.AssertEqual(t, 4321, st.Port)
	core.AssertEqual(t, 77, st.PID)
}
