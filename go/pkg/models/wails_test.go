// SPDX-Licence-Identifier: EUPL-1.2

// wails_test.go — real invocations of the WailsService surface.
// wails_example_test.go only asserts that the method VALUES exist
// (reflection over func types via %T) and never calls them, so
// wails.go carried 0% statement coverage despite that file's presence
// — the classic "the test compiles but nothing ran" gap. These tests
// actually call every method against a controlled ~/Lethean/conf/
// models/ fixture.

package models_test

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/models"
)

func TestWailsService_ServiceName_Good(t *core.T) {
	svc := models.NewWailsService()
	core.AssertEqual(t, "Models", svc.ServiceName())
}

func TestWailsService_ServiceStartup_Good(t *core.T) {
	svc := models.NewWailsService()
	r := svc.ServiceStartup(core.Background(), nil)
	core.AssertTrue(t, r.OK)
}

func TestWailsService_ServiceShutdown_Good(t *core.T) {
	svc := models.NewWailsService()
	r := svc.ServiceShutdown()
	core.AssertTrue(t, r.OK)
}

func TestWailsService_List_Good(t *core.T) {
	dir := modelsFixture(t)
	core.AssertTrue(t, core.WriteFile(core.PathJoin(dir, "a.gguf"), []byte("x"), 0o644).OK)

	svc := models.NewWailsService()
	r := svc.List()
	core.AssertTrue(t, r.OK, r.Error())
	entries, ok := r.Value.([]models.Entry)
	core.AssertTrue(t, ok)
	core.AssertLen(t, entries, 1)
}

// TestWailsService_List_Bad_PropagatesUnderlyingFailure — HOME points
// at a regular file so paths.ModelsDir() (via models.List()) fails;
// the WailsService wrapper must surface that as a Fail Result rather
// than panicking on the `r.Value.(error)` assertion.
func TestWailsService_List_Bad_PropagatesUnderlyingFailure(t *core.T) {
	tmp := t.TempDir()
	blocker := core.PathJoin(tmp, "blocker")
	core.AssertTrue(t, core.WriteFile(blocker, []byte("x"), 0o644).OK)
	t.Setenv("HOME", blocker)

	svc := models.NewWailsService()
	r := svc.List()
	core.AssertFalse(t, r.OK, "List() must fail when ModelsDir() fails")
}

func TestWailsService_Delete_Good(t *core.T) {
	dir := modelsFixture(t)
	core.AssertTrue(t, core.WriteFile(core.PathJoin(dir, "gone.gguf"), []byte("x"), 0o644).OK)

	svc := models.NewWailsService()
	r := svc.Delete("gone.gguf")
	core.AssertTrue(t, r.OK, r.Error())
	core.AssertFalse(t, core.Stat(core.PathJoin(dir, "gone.gguf")).OK)
}

func TestWailsService_Delete_Bad_NotFound(t *core.T) {
	modelsFixture(t)
	svc := models.NewWailsService()
	r := svc.Delete("ghost.gguf")
	core.AssertFalse(t, r.OK, "Delete must fail for a missing target")
}

func TestWailsService_DiskFree_Good(t *core.T) {
	modelsFixture(t)
	svc := models.NewWailsService()
	r := svc.DiskFree()
	core.AssertTrue(t, r.OK, r.Error())
	_, ok := r.Value.(int64)
	core.AssertTrue(t, ok, "DiskFree must return an int64")
}

// TestWailsService_DiskFree_Bad_FallsBackToZero — when ModelsDir()
// fails (HOME is a plain file), DiskFree degrades to Ok(0) rather than
// propagating the failure — the WebView's footer slot falls back to
// its design literal instead of surfacing a raw syscall error.
func TestWailsService_DiskFree_Bad_FallsBackToZero(t *core.T) {
	tmp := t.TempDir()
	blocker := core.PathJoin(tmp, "blocker")
	core.AssertTrue(t, core.WriteFile(blocker, []byte("x"), 0o644).OK)
	t.Setenv("HOME", blocker)

	svc := models.NewWailsService()
	r := svc.DiskFree()
	core.AssertTrue(t, r.OK, "DiskFree must degrade to Ok(0), not Fail")
	core.AssertEqual(t, int64(0), r.Value.(int64))
}
