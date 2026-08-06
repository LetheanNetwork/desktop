// SPDX-Licence-Identifier: EUPL-1.2

// wails_service_methods_test.go — real invocations of every
// WailsService wrapper method. wails_example_test.go's
// Test/ExampleWailsService_* functions only take a method VALUE
// (`ref := (*subject.WailsService).ServiceName`) and Sprintf its
// %T — they never call the method, so ServiceName / ServiceStartup /
// ServiceShutdown / RemoveLast / ClearMessages / Rename /
// SetSystemPrompt / Duplicate / Delete / Search / RecentGenerations
// showed 0% coverage despite having "tests" attached to their names.
// This file supplies the missing real Good/Bad/Ugly coverage; the
// existing file is left as-is (out of this task's scope) but the gap
// is worth flagging — those functions were exercising reflection on a
// method value, never the method body.

package sessions_test

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/sessions"
)

// --- Service lifecycle -------------------------------------------------

func TestWailsService_ServiceName_Good(t *core.T) {
	svc := sessions.NewWailsService(nil)
	core.AssertEqual(t, "Sessions", svc.ServiceName())
}

func TestWailsService_ServiceStartup_Good(t *core.T) {
	svc := sessions.NewWailsService(nil)
	r := svc.ServiceStartup(core.Background(), nil)
	core.AssertTrue(t, r.OK, "ServiceStartup is a documented no-op — must always return OK")
}

func TestWailsService_ServiceShutdown_Good(t *core.T) {
	svc := sessions.NewWailsService(nil)
	r := svc.ServiceShutdown()
	core.AssertTrue(t, r.OK, "ServiceShutdown is a documented no-op — must always return OK")
}

// --- RemoveLast ----------------------------------------------------------

func TestWailsService_RemoveLast_Good(t *core.T) {
	c := coreFixture(t)
	svc := sessions.NewWailsService(c)
	id := svc.Create("remove-last").Value.(string)
	core.AssertTrue(t, svc.Append(id, "user", "hello").OK)
	core.AssertTrue(t, svc.Append(id, "assistant", "hi there").OK)

	r := svc.RemoveLast(id)
	core.AssertTrue(t, r.OK, "RemoveLast should succeed on a session with messages")

	read := svc.Read(id)
	core.AssertTrue(t, read.OK)
	core.AssertLen(t, read.Value, 1)
}

func TestWailsService_RemoveLast_Bad_UnknownSession(t *core.T) {
	c := coreFixture(t)
	svc := sessions.NewWailsService(c)
	r := svc.RemoveLast("does-not-exist")
	core.AssertFalse(t, r.OK, "RemoveLast on an unknown session must fail")
}

// --- ClearMessages ---------------------------------------------------

func TestWailsService_ClearMessages_Good(t *core.T) {
	c := coreFixture(t)
	svc := sessions.NewWailsService(c)
	id := svc.Create("clear-me").Value.(string)
	core.AssertTrue(t, svc.Append(id, "user", "hello").OK)

	r := svc.ClearMessages(id)
	core.AssertTrue(t, r.OK, "ClearMessages should succeed")

	read := svc.Read(id)
	core.AssertTrue(t, read.OK)
	core.AssertLen(t, read.Value, 0)
}

func TestWailsService_ClearMessages_Bad_UnknownSession(t *core.T) {
	c := coreFixture(t)
	svc := sessions.NewWailsService(c)
	r := svc.ClearMessages("does-not-exist")
	core.AssertFalse(t, r.OK, "ClearMessages on an unknown session must fail")
}

// --- Rename ------------------------------------------------------------

func TestWailsService_Rename_Good(t *core.T) {
	c := coreFixture(t)
	svc := sessions.NewWailsService(c)
	id := svc.Create("original title").Value.(string)

	r := svc.Rename(id, "new title")
	core.AssertTrue(t, r.OK, "Rename should succeed")

	infos := svc.List().Value.([]sessions.SessionInfo)
	found := false
	for _, info := range infos {
		if info.ID == id {
			found = true
			core.AssertEqual(t, "new title", info.Title)
		}
	}
	core.AssertTrue(t, found, "renamed session must still be listed")
}

func TestWailsService_Rename_Bad_UnknownSession(t *core.T) {
	c := coreFixture(t)
	svc := sessions.NewWailsService(c)
	r := svc.Rename("does-not-exist", "whatever")
	core.AssertFalse(t, r.OK, "Rename on an unknown session must fail")
}

// --- SetSystemPrompt ---------------------------------------------------

func TestWailsService_SetSystemPrompt_Good(t *core.T) {
	c := coreFixture(t)
	svc := sessions.NewWailsService(c)
	id := svc.Create("steered").Value.(string)

	r := svc.SetSystemPrompt(id, "Answer in haiku.")
	core.AssertTrue(t, r.OK, "SetSystemPrompt should succeed")
}

func TestWailsService_SetSystemPrompt_Bad_UnknownSession(t *core.T) {
	c := coreFixture(t)
	svc := sessions.NewWailsService(c)
	r := svc.SetSystemPrompt("does-not-exist", "prompt")
	core.AssertFalse(t, r.OK, "SetSystemPrompt on an unknown session must fail")
}

// --- Duplicate -----------------------------------------------------------

func TestWailsService_Duplicate_Good(t *core.T) {
	c := coreFixture(t)
	svc := sessions.NewWailsService(c)
	id := svc.Create("forkable").Value.(string)
	core.AssertTrue(t, svc.Append(id, "user", "hello").OK)

	r := svc.Duplicate(id)
	core.AssertTrue(t, r.OK, "Duplicate should succeed")
	newID, ok := r.Value.(string)
	core.AssertTrue(t, ok && newID != "" && newID != id, "Duplicate must return a distinct new id")

	read := svc.Read(newID)
	core.AssertTrue(t, read.OK)
	core.AssertLen(t, read.Value, 1)
}

func TestWailsService_Duplicate_Bad_UnknownSession(t *core.T) {
	c := coreFixture(t)
	svc := sessions.NewWailsService(c)
	r := svc.Duplicate("does-not-exist")
	core.AssertFalse(t, r.OK, "Duplicate of an unknown session must fail")
}

// --- Delete --------------------------------------------------------------

func TestWailsService_Delete_Good(t *core.T) {
	c := coreFixture(t)
	svc := sessions.NewWailsService(c)
	id := svc.Create("doomed").Value.(string)

	r := svc.Delete(id)
	core.AssertTrue(t, r.OK, "Delete should succeed")

	infos := svc.List().Value.([]sessions.SessionInfo)
	for _, info := range infos {
		core.AssertTrue(t, info.ID != id, "deleted session must not appear in List")
	}
}

func TestWailsService_Delete_Ugly_IdempotentOnUnknownSession(t *core.T) {
	c := coreFixture(t)
	svc := sessions.NewWailsService(c)
	// Delete is documented idempotent — deleting an unknown id
	// succeeds rather than erroring.
	r := svc.Delete("never-existed")
	core.AssertTrue(t, r.OK, "Delete of an unknown session must succeed (idempotent)")
}

// --- Search --------------------------------------------------------------

func TestWailsService_Search_Good(t *core.T) {
	c := coreFixture(t)
	svc := sessions.NewWailsService(c)
	id := svc.Create("a chat about regex pitfalls").Value.(string)
	core.AssertTrue(t, svc.Append(id, "user", "how do I escape a dot").OK)

	r := svc.Search("regex")
	core.AssertTrue(t, r.OK, "Search should succeed")
	sr, ok := r.Value.(sessions.SearchResult)
	core.AssertTrue(t, ok, "Search must return a SearchResult")
	found := false
	for _, hit := range sr.Hits {
		if hit.ID == id {
			found = true
		}
	}
	core.AssertTrue(t, found, "Search for a title substring must surface the matching session")
}

func TestWailsService_Search_Good_EmptyQueryActsAsList(t *core.T) {
	c := coreFixture(t)
	svc := sessions.NewWailsService(c)
	svc.Create("session one")

	r := svc.Search("")
	core.AssertTrue(t, r.OK, "Search with an empty query should succeed")
	sr := r.Value.(sessions.SearchResult)
	core.AssertGreater(t, len(sr.Hits), 0, "empty-query Search must behave like List")
}

// --- RecentGenerations -------------------------------------------------

func TestWailsService_RecentGenerations_Good(t *core.T) {
	c := coreFixture(t)
	svc := sessions.NewWailsService(c)
	id := svc.Create("gen-session").Value.(string)
	core.AssertTrue(t, svc.Append(id, "user", "hello").OK)
	core.AssertTrue(t, svc.Append(id, "assistant", "hi, how can I help?").OK)

	r := svc.RecentGenerations(10)
	core.AssertTrue(t, r.OK, "RecentGenerations should succeed")
	gens, ok := r.Value.([]sessions.Generation)
	core.AssertTrue(t, ok, "RecentGenerations must return []Generation")
	core.AssertGreater(t, len(gens), 0, "an assistant turn should surface as a generation")
}

func TestWailsService_RecentGenerations_Good_EmptyCatalogue(t *core.T) {
	c := coreFixture(t)
	svc := sessions.NewWailsService(c)
	r := svc.RecentGenerations(10)
	core.AssertTrue(t, r.OK, "RecentGenerations on an empty catalogue should succeed, not fail")
}

// --- ExportAll (already partially covered; adds the failure branch) ---

func TestWailsService_ExportAll_Bad_PathOutsideRoot(t *core.T) {
	withTempLetheanHome(t)
	c := coreFixture(t)
	svc := sessions.NewWailsService(c)
	svc.Create("exportable")

	r := svc.ExportAll("/etc/not-under-lethean")
	core.AssertFalse(t, r.OK, "ExportAll must refuse a directory outside the Lethean workspace")
}

func TestWailsService_ExportAll_Good(t *core.T) {
	root := withTempLetheanHome(t)
	c := coreFixture(t)
	svc := sessions.NewWailsService(c)
	id := svc.Create("exportable").Value.(string)
	core.AssertTrue(t, svc.Append(id, "user", "hello").OK)

	dest := core.PathJoin(root, "exports")
	r := svc.ExportAll(dest)
	core.AssertTrue(t, r.OK, "ExportAll under the Lethean workspace should succeed")
	count, ok := r.Value.(int)
	core.AssertTrue(t, ok, "ExportAll must return an int count")
	core.AssertGreater(t, count, 0, "ExportAll must report at least one exported file")
}
