// SPDX-Licence-Identifier: EUPL-1.2

package sessions_test

import (
	core "dappco.re/go"
	"dappco.re/go/inference"
	"dappco.re/lthn/desktop/pkg/sessions"
)

// withTempLetheanHome re-points HOME so paths.Root() resolves under
// a writable per-test directory. Returns the workspace root path
// (i.e. <tmp>/Lethean) so callers can compose under-root export paths.
//
// Mantis #1420 — used by the Export/ExportAll path-token tests so the
// boundary-layer validator sees a predictable, test-owned root rather
// than the developer's real ~/Lethean/.
func withTempLetheanHome(t *core.T) string {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	return core.PathJoin(tmp, "Lethean")
}

// Mantis #1418 — Cerberus M2 2026-05-16. Race tests for the per-
// session mutex on WailsService. Run with `go test -race` to detect
// data races; without -race the test still asserts the manifest
// counter matches the actual message-log length after concurrent
// writes — the invariant the lock protects.

// TestWails_Append_RaceFreeConcurrent — 100 concurrent Append on the
// same session id must produce manifest.Messages == 100 and the
// message log must hold 100 entries. Without the per-id mutex, the
// readManifest → mutate → writeManifest RMW window drops updates.
func TestWails_Append_RaceFreeConcurrent(t *core.T) {
	c := coreFixture(t)
	svc := sessions.NewWailsService(c)
	id := svc.Create("race-target").Value.(string)

	const N = 100
	done := make(chan struct{}, N)
	for i := 0; i < N; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			svc.Append(id, "user", "m")
		}()
	}
	for i := 0; i < N; i++ {
		<-done
	}

	// Manifest counter must match the actual log length.
	infos := svc.List().Value.([]sessions.SessionInfo)
	var target sessions.SessionInfo
	for _, info := range infos {
		if info.ID == id {
			target = info
			break
		}
	}
	core.AssertEqual(t, N, target.Messages,
		"manifest.Messages must equal the number of Append calls")

	// Cross-check the actual message log.
	read := svc.Read(id)
	core.AssertTrue(t, read.OK)
	msgs := read.Value.([]inference.Message)
	core.AssertEqual(t, N, len(msgs), "message log length must match Append count")
}

// TestWails_SetTags_RaceFreeConcurrent — 50 concurrent SetTags +
// Append alternating on the same id. Final state: manifest reads
// without panic, tags slice is well-formed, message log matches the
// number of Appends. Without the lock, an interleaved SetTags +
// Append corrupts info.Messages vs the actual log.
func TestWails_SetTags_RaceFreeConcurrent(t *core.T) {
	c := coreFixture(t)
	svc := sessions.NewWailsService(c)
	id := svc.Create("tag-race").Value.(string)

	const N = 50
	done := make(chan struct{}, N*2)
	for i := 0; i < N; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			svc.SetTags(id, []string{"a", "b"})
		}()
		go func() {
			defer func() { done <- struct{}{} }()
			svc.Append(id, "user", "x")
		}()
	}
	for i := 0; i < N*2; i++ {
		<-done
	}

	infos := svc.List().Value.([]sessions.SessionInfo)
	var target sessions.SessionInfo
	for _, info := range infos {
		if info.ID == id {
			target = info
			break
		}
	}
	// Tags should be the canonical set [a, b] after every SetTags
	// (idempotent) — proves SetTags didn't half-write.
	core.AssertEqual(t, 2, len(target.Tags))
	core.AssertEqual(t, "a", target.Tags[0])
	core.AssertEqual(t, "b", target.Tags[1])
	// Counter ↔ log invariant.
	core.AssertEqual(t, N, target.Messages)
	msgs := svc.Read(id).Value.([]inference.Message)
	core.AssertEqual(t, N, len(msgs))
}

// TestWails_RMW_DifferentSessionsUncontended — verifies the
// per-id mutex doesn't serialise unrelated sessions. Two sessions
// each take 100 Appends; total wall-clock should be roughly
// max(per-session) not sum-of-sessions. We don't measure timing
// (flaky on CI) — instead we assert that BOTH session counters
// reach N, proving no cross-session deadlock + correct final state.
func TestWails_RMW_DifferentSessionsUncontended(t *core.T) {
	c := coreFixture(t)
	svc := sessions.NewWailsService(c)
	idA := svc.Create("A").Value.(string)
	idB := svc.Create("B").Value.(string)

	const N = 100
	done := make(chan struct{}, N*2)
	for i := 0; i < N; i++ {
		go func() { defer func() { done <- struct{}{} }(); svc.Append(idA, "user", "a") }()
		go func() { defer func() { done <- struct{}{} }(); svc.Append(idB, "user", "b") }()
	}
	for i := 0; i < N*2; i++ {
		<-done
	}

	infos := svc.List().Value.([]sessions.SessionInfo)
	got := map[string]int{}
	for _, info := range infos {
		got[info.ID] = info.Messages
	}
	core.AssertEqual(t, N, got[idA], "session A counter ok")
	core.AssertEqual(t, N, got[idB], "session B counter ok")
}

// Mantis #1420 — Cerberus M4 2026-05-17. Boundary-layer path
// enforcement on WailsService.Export + ExportAll. Without these gates
// a WebView caller (extension / iframe / future plugin tier) could
// pass an arbitrary host path and turn the export verbs into an
// arbitrary-write primitive. UI convention (Dialogs.SaveFile starts
// under ~/Lethean/) is not a security boundary — the backend has to
// enforce it.

// TestExport_GoodPath_Good — a path that resolves under ~/Lethean/
// (test-controlled tmp HOME) is accepted and the markdown lands on disk.
func TestExport_GoodPath_Good(t *core.T) {
	root := withTempLetheanHome(t)
	c := coreFixture(t)
	svc := sessions.NewWailsService(c)
	id := svc.Create("good-path").Value.(string)
	core.AssertTrue(t, svc.Append(id, "user", "hi").OK)

	out := core.PathJoin(root, "exports", "good.md")
	core.AssertTrue(t, core.MkdirAll(core.PathJoin(root, "exports"), 0o755).OK,
		"must be able to create the under-root parent dir")

	r := svc.Export(id, out)
	core.AssertTrue(t, r.OK, "Export under ~/Lethean/ must succeed")

	read := core.ReadFile(out)
	core.AssertTrue(t, read.OK, "exported file must exist on disk")
	body := string(read.Value.([]byte))
	core.AssertContains(t, body, "# good-path")
	core.AssertContains(t, body, "hi")
}

// TestExport_PathOutsideAllowed_Bad — an absolute path outside the
// workspace root is rejected before any file I/O occurs. Code is
// "sessions.export.path_invalid" so the lit toast can render a
// uniform message regardless of which sub-clause tripped.
func TestExport_PathOutsideAllowed_Bad(t *core.T) {
	_ = withTempLetheanHome(t)
	c := coreFixture(t)
	svc := sessions.NewWailsService(c)
	id := svc.Create("attacker-target").Value.(string)

	// /tmp is absolute + clean but lives OUTSIDE ~/Lethean/ — exactly
	// the arbitrary-write primitive the M4 finding describes.
	escape := core.PathJoin(t.TempDir(), "pwn.md")

	r := svc.Export(id, escape)
	core.AssertFalse(t, r.OK, "Export outside ~/Lethean/ must reject")
	err, _ := r.Value.(error)
	core.AssertNotNil(t, err, "rejection must carry a typed error")
	core.AssertContains(t, err.Error(), "sessions.export.path_invalid",
		"rejection must surface the path_invalid code")

	// And the same shape on ExportAll — the dir argument has the same
	// surface area as the single-file Export path.
	rAll := svc.ExportAll(escape)
	core.AssertFalse(t, rAll.OK, "ExportAll outside ~/Lethean/ must reject")
	errAll, _ := rAll.Value.(error)
	core.AssertNotNil(t, errAll)
	core.AssertContains(t, errAll.Error(), "sessions.export.path_invalid")
}

// TestExport_RelativePath_Bad — a relative path (no leading slash) is
// rejected. Defends against a WebView caller that forgets to anchor
// the picker output and lets the backend silently write to cwd
// (whatever that is for the running lthn process, usually opaque).
func TestExport_RelativePath_Bad(t *core.T) {
	_ = withTempLetheanHome(t)
	c := coreFixture(t)
	svc := sessions.NewWailsService(c)
	id := svc.Create("rel-target").Value.(string)

	r := svc.Export(id, "relative/path/out.md")
	core.AssertFalse(t, r.OK, "Export with relative path must reject")
	err, _ := r.Value.(error)
	core.AssertNotNil(t, err)
	core.AssertContains(t, err.Error(), "sessions.export.path_invalid")

	rAll := svc.ExportAll("relative/dir")
	core.AssertFalse(t, rAll.OK, "ExportAll with relative dir must reject")
	errAll, _ := rAll.Value.(error)
	core.AssertNotNil(t, errAll)
	core.AssertContains(t, errAll.Error(), "sessions.export.path_invalid")
}
