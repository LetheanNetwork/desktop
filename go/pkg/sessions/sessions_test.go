// SPDX-Licence-Identifier: EUPL-1.2

// Tests for the session-persistence layer. Sessions composes go-store
// actions through the Core bus, so each test uses an in-memory store
// service registered on a fresh Core — no disk I/O, fast.
//
// nilCore tests verify the defensive guards on the public API don't
// panic; the full-flow tests exercise Create → Append → Read → List
// against an isolated store namespace.

package sessions_test

import (
	"sync"

	core "dappco.re/go"
	"dappco.re/go/inference"
	"dappco.re/go/store"

	"dappco.re/lthn/desktop/pkg/audit"
	"dappco.re/lthn/desktop/pkg/sessions"
)

// coreFixture builds a Core with the store driver wired so every
// sessions.* call can persist + retrieve through the real "store.set"
// / "store.get" / "store.get_all" action surface. Matches the canonical
// bootstrap from cmd/lthn/app.go — core.WithName attaches the service,
// c.ServiceStartup triggers the OnStartup hook that actually registers
// the action handlers.
func coreFixture(t *core.T) *core.Core {
	t.Helper()
	tmp := t.TempDir()
	c := core.New(
		core.WithName("store", store.NewService(store.StoreConfig{
			DatabasePath: tmp + "/sessions-test.db",
		})),
	)
	r := c.ServiceStartup(core.Background(), nil)
	core.AssertTrue(t, r.OK, "ServiceStartup must succeed under tempdir")
	return c
}

func TestSessions_Create_Good(t *core.T) {
	c := coreFixture(t)
	r := sessions.Create(c, "first chat")
	core.AssertTrue(t, r.OK)
	id := r.Value.(string)
	core.AssertNotEmpty(t, id, "Create must return a non-empty id")
}

func TestSessions_Append_Good_Single(t *core.T) {
	c := coreFixture(t)
	cr := sessions.Create(c, "thread")
	core.AssertTrue(t, cr.OK)
	id := cr.Value.(string)

	ar := sessions.Append(c, id, "user", "hello")
	core.AssertTrue(t, ar.OK)

	rr := sessions.Read(c, id)
	core.AssertTrue(t, rr.OK)
	msgs := rr.Value.([]inference.Message)
	core.AssertLen(t, msgs, 1)
	core.AssertEqual(t, "user", msgs[0].Role)
	core.AssertEqual(t, "hello", msgs[0].Content)
}

func TestSessions_Append_Good_Multi(t *core.T) {
	c := coreFixture(t)
	id := sessions.Create(c, "thread").Value.(string)

	core.AssertTrue(t, sessions.Append(c, id, "user", "ping").OK)
	core.AssertTrue(t, sessions.Append(c, id, "assistant", "pong").OK)
	core.AssertTrue(t, sessions.Append(c, id, "user", "?").OK)

	msgs := sessions.Read(c, id).Value.([]inference.Message)
	core.AssertLen(t, msgs, 3)
	core.AssertEqual(t, "assistant", msgs[1].Role)
	core.AssertEqual(t, "?", msgs[2].Content)
}

func TestSessions_List_Good_MultipleSessions(t *core.T) {
	c := coreFixture(t)
	a := sessions.Create(c, "thread A").Value.(string)
	b := sessions.Create(c, "thread B").Value.(string)
	core.AssertNotEqual(t, a, b, "Create must produce distinct ids")

	core.AssertTrue(t, sessions.Append(c, a, "user", "msg-A").OK)

	r := sessions.List(c)
	core.AssertTrue(t, r.OK)
	infos := r.Value.([]sessions.SessionInfo)
	core.AssertLen(t, infos, 2)

	byID := map[string]sessions.SessionInfo{}
	for _, info := range infos {
		byID[info.ID] = info
	}
	core.AssertEqual(t, "thread A", byID[a].Title)
	core.AssertEqual(t, "thread B", byID[b].Title)
	core.AssertEqual(t, 1, byID[a].Messages, "A has one appended message")
	core.AssertEqual(t, 0, byID[b].Messages, "B is still empty")
}

func TestSessions_RecentGenerations_Good(t *core.T) {
	c := coreFixture(t)
	idA := sessions.Create(c, "A").Value.(string)
	idB := sessions.Create(c, "B").Value.(string)

	// A: one ask + reply pair. B: two pairs so we exercise the
	// preceding-user-prompt walk and the per-session multi-yield path.
	core.AssertTrue(t, sessions.Append(c, idA, "user", "first prompt A").OK)
	core.AssertTrue(t, sessions.Append(c, idA, "assistant", "reply A1").OK)
	core.AssertTrue(t, sessions.Append(c, idB, "user", "prompt B1").OK)
	core.AssertTrue(t, sessions.Append(c, idB, "assistant", "reply B1").OK)
	core.AssertTrue(t, sessions.Append(c, idB, "user", "prompt B2").OK)
	core.AssertTrue(t, sessions.Append(c, idB, "assistant", "reply B2").OK)

	r := sessions.RecentGenerations(c, 10)
	core.AssertTrue(t, r.OK)
	gens := r.Value.([]sessions.Generation)
	core.AssertLen(t, gens, 3)

	// Each gen pairs assistant content with preceding user prompt.
	for _, g := range gens {
		core.AssertNotEmpty(t, g.Reply)
		core.AssertNotEmpty(t, g.Prompt)
		core.AssertNotEmpty(t, g.SessionID)
	}

	// Limit truncates.
	r2 := sessions.RecentGenerations(c, 2)
	core.AssertTrue(t, r2.OK)
	core.AssertLen(t, r2.Value.([]sessions.Generation), 2)
}

func TestSessions_RecentGenerations_Bad_NilCore(t *core.T) {
	r := sessions.RecentGenerations(nil, 10)
	core.AssertFalse(t, r.OK)
	core.AssertNotEmpty(t, r.Error())
}

func TestSessions_Create_Bad_NilCore(t *core.T) {
	r := sessions.Create(nil, "thread")
	core.AssertFalse(t, r.OK)
	core.AssertNotEmpty(t, r.Error())
}

func TestSessions_Append_Bad_NilCore(t *core.T) {
	r := sessions.Append(nil, "id", "user", "msg")
	core.AssertFalse(t, r.OK)
	core.AssertNotEmpty(t, r.Error())
}

func TestSessions_Read_Bad_NilCore(t *core.T) {
	r := sessions.Read(nil, "id")
	core.AssertFalse(t, r.OK)
	core.AssertNotEmpty(t, r.Error())
}

func TestSessions_List_Bad_NilCore(t *core.T) {
	r := sessions.List(nil)
	core.AssertFalse(t, r.OK)
	core.AssertNotEmpty(t, r.Error())
}

func TestSessions_Delete_Good(t *core.T) {
	c := coreFixture(t)
	// Plant two sessions so we can assert exactly the one we delete
	// disappears AND the other remains untouched.
	rA := sessions.Create(c, "to be deleted")
	core.AssertTrue(t, rA.OK)
	idA := rA.Value.(string)
	core.AssertTrue(t, sessions.Append(c, idA, "user", "hello").OK)

	rB := sessions.Create(c, "kept")
	core.AssertTrue(t, rB.OK)
	idB := rB.Value.(string)

	delR := sessions.Delete(c, idA)
	core.AssertTrue(t, delR.OK, "Delete must succeed for an existing id")

	// List should now contain only B.
	listR := sessions.List(c)
	core.AssertTrue(t, listR.OK)
	infos := listR.Value.([]sessions.SessionInfo)
	core.AssertLen(t, infos, 1, "exactly one session should remain")
	core.AssertEqual(t, idB, infos[0].ID, "the remaining session must be the unaffected one")

}

func TestSessions_Delete_Good_Idempotent(t *core.T) {
	c := coreFixture(t)
	// Deleting an id that was never created should succeed quietly —
	// matches the underlying store.delete contract.
	r := sessions.Delete(c, "never-existed-id")
	core.AssertTrue(t, r.OK, "Delete must be idempotent on unknown ids")
}

func TestSessions_Delete_Bad_NilCore(t *core.T) {
	r := sessions.Delete(nil, "id")
	core.AssertFalse(t, r.OK)
	core.AssertNotEmpty(t, r.Error())
}

func TestSessions_Delete_Bad_EmptyID(t *core.T) {
	c := coreFixture(t)
	r := sessions.Delete(c, "")
	core.AssertFalse(t, r.OK, "Delete must refuse an empty id")
}

func TestSessions_Rename_Good(t *core.T) {
	c := coreFixture(t)
	cr := sessions.Create(c, "original")
	core.AssertTrue(t, cr.OK)
	id := cr.Value.(string)

	rr := sessions.Rename(c, id, "renamed")
	core.AssertTrue(t, rr.OK, "Rename must succeed for an existing session")

	listR := sessions.List(c)
	core.AssertTrue(t, listR.OK)
	infos := listR.Value.([]sessions.SessionInfo)
	core.AssertLen(t, infos, 1)
	core.AssertEqual(t, "renamed", infos[0].Title, "manifest title must reflect the new value")
}

func TestSessions_Rename_Good_BumpsUpdatedAt(t *core.T) {
	c := coreFixture(t)
	cr := sessions.Create(c, "original")
	core.AssertTrue(t, cr.OK)
	id := cr.Value.(string)

	listR := sessions.List(c)
	core.AssertTrue(t, listR.OK)
	infos := listR.Value.([]sessions.SessionInfo)
	core.AssertLen(t, infos, 1)
	before := infos[0].UpdatedAt

	// UnixNow() resolution is 1s; tests run faster than that so the
	// timestamp won't necessarily advance. Walk forward by 1s manually
	// to guarantee detection. Simulate by sleeping just past 1s.
	core.Sleep(1100 * core.Millisecond)

	rr := sessions.Rename(c, id, "renamed")
	core.AssertTrue(t, rr.OK)

	listR = sessions.List(c)
	core.AssertTrue(t, listR.OK)
	infos = listR.Value.([]sessions.SessionInfo)
	core.AssertLen(t, infos, 1)
	core.AssertTrue(t, infos[0].UpdatedAt >= before, "Rename should bump UpdatedAt")
}

func TestSessions_Rename_Bad_NilCore(t *core.T) {
	r := sessions.Rename(nil, "id", "title")
	core.AssertFalse(t, r.OK)
	core.AssertNotEmpty(t, r.Error())
}

func TestSessions_Rename_Bad_EmptyID(t *core.T) {
	c := coreFixture(t)
	r := sessions.Rename(c, "", "title")
	core.AssertFalse(t, r.OK)
}

func TestSessions_Rename_Bad_EmptyTitle(t *core.T) {
	c := coreFixture(t)
	cr := sessions.Create(c, "first")
	core.AssertTrue(t, cr.OK)
	r := sessions.Rename(c, cr.Value.(string), "")
	core.AssertFalse(t, r.OK, "Rename must refuse empty title")
}

func TestSessions_Rename_Bad_UnknownID(t *core.T) {
	c := coreFixture(t)
	r := sessions.Rename(c, "never-existed-id", "title")
	core.AssertFalse(t, r.OK, "Rename must fail when manifest read fails for the id")
}

func TestSessions_RemoveLast_Good(t *core.T) {
	c := coreFixture(t)
	cr := sessions.Create(c, "chat")
	core.AssertTrue(t, cr.OK)
	id := cr.Value.(string)
	core.AssertTrue(t, sessions.Append(c, id, "user", "hello").OK)
	core.AssertTrue(t, sessions.Append(c, id, "assistant", "hi there").OK)

	r := sessions.RemoveLast(c, id)
	core.AssertTrue(t, r.OK, "RemoveLast must succeed on non-empty session")
	popped := r.Value.(inference.Message)
	core.AssertEqual(t, "assistant", popped.Role)
	core.AssertEqual(t, "hi there", popped.Content)

	// Read should now show only the user turn.
	readR := sessions.Read(c, id)
	core.AssertTrue(t, readR.OK)
	remaining := readR.Value.([]inference.Message)
	core.AssertLen(t, remaining, 1)
	core.AssertEqual(t, "user", remaining[0].Role)

	// Manifest message count should reflect the trim.
	listR := sessions.List(c)
	core.AssertTrue(t, listR.OK)
	infos := listR.Value.([]sessions.SessionInfo)
	core.AssertLen(t, infos, 1)
	core.AssertEqual(t, 1, infos[0].Messages, "manifest count must match remaining message count")
}

func TestSessions_RemoveLast_Bad_EmptySession(t *core.T) {
	c := coreFixture(t)
	cr := sessions.Create(c, "empty")
	core.AssertTrue(t, cr.OK)
	id := cr.Value.(string)

	r := sessions.RemoveLast(c, id)
	core.AssertFalse(t, r.OK, "RemoveLast must refuse an empty session")
}

func TestSessions_RemoveLast_Bad_NilCore(t *core.T) {
	r := sessions.RemoveLast(nil, "id")
	core.AssertFalse(t, r.OK)
	core.AssertNotEmpty(t, r.Error())
}

func TestSessions_RemoveLast_Bad_EmptyID(t *core.T) {
	c := coreFixture(t)
	r := sessions.RemoveLast(c, "")
	core.AssertFalse(t, r.OK)
}

func TestSessions_Export_Good(t *core.T) {
	c := coreFixture(t)
	cr := sessions.Create(c, "Drafting launch")
	core.AssertTrue(t, cr.OK)
	id := cr.Value.(string)
	core.AssertTrue(t, sessions.Append(c, id, "user", "ping").OK)
	core.AssertTrue(t, sessions.Append(c, id, "assistant", "pong").OK)

	path := core.PathJoin(t.TempDir(), "out.md")
	r := sessions.Export(c, id, path)
	core.AssertTrue(t, r.OK, "Export must succeed for a populated session")

	read := core.ReadFile(path)
	core.AssertTrue(t, read.OK)
	body := string(read.Value.([]byte))
	core.AssertContains(t, body, "# Drafting launch")
	core.AssertContains(t, body, "You:")
	core.AssertContains(t, body, "ping")
	core.AssertContains(t, body, "Assistant:")
	core.AssertContains(t, body, "pong")
}

func TestSessions_Export_Good_EmptySession(t *core.T) {
	c := coreFixture(t)
	cr := sessions.Create(c, "empty chat")
	core.AssertTrue(t, cr.OK)
	id := cr.Value.(string)

	path := core.PathJoin(t.TempDir(), "out.md")
	r := sessions.Export(c, id, path)
	core.AssertTrue(t, r.OK, "Export with no messages should still write the header")

	read := core.ReadFile(path)
	core.AssertTrue(t, read.OK)
	body := string(read.Value.([]byte))
	core.AssertContains(t, body, "# empty chat")
}

func TestSessions_Export_Good_UntitledFallback(t *core.T) {
	c := coreFixture(t)
	cr := sessions.Create(c, "") // explicit empty title
	core.AssertTrue(t, cr.OK)
	id := cr.Value.(string)

	path := core.PathJoin(t.TempDir(), "out.md")
	r := sessions.Export(c, id, path)
	core.AssertTrue(t, r.OK)

	read := core.ReadFile(path)
	body := string(read.Value.([]byte))
	core.AssertContains(t, body, "# Untitled conversation")
}

func TestSessions_Export_Bad_NilCore(t *core.T) {
	r := sessions.Export(nil, "id", "/tmp/out.md")
	core.AssertFalse(t, r.OK)
}

func TestSessions_Export_Bad_EmptyID(t *core.T) {
	c := coreFixture(t)
	r := sessions.Export(c, "", "/tmp/out.md")
	core.AssertFalse(t, r.OK)
}

func TestSessions_Export_Bad_EmptyPath(t *core.T) {
	c := coreFixture(t)
	cr := sessions.Create(c, "x")
	core.AssertTrue(t, cr.OK)
	r := sessions.Export(c, cr.Value.(string), "")
	core.AssertFalse(t, r.OK)
}

func TestSessions_Export_Bad_UnknownID(t *core.T) {
	c := coreFixture(t)
	r := sessions.Export(c, "never-existed", core.PathJoin(t.TempDir(), "out.md"))
	core.AssertFalse(t, r.OK, "manifest read failure should propagate")
}

func TestSessions_ClearMessages_Good(t *core.T) {
	c := coreFixture(t)
	cr := sessions.Create(c, "chat")
	core.AssertTrue(t, cr.OK)
	id := cr.Value.(string)
	core.AssertTrue(t, sessions.Append(c, id, "user", "hi").OK)
	core.AssertTrue(t, sessions.Append(c, id, "assistant", "hey").OK)

	r := sessions.ClearMessages(c, id)
	core.AssertTrue(t, r.OK, "ClearMessages must succeed for an existing session")

	// Read must show no messages.
	readR := sessions.Read(c, id)
	core.AssertTrue(t, readR.OK)
	msgs := readR.Value.([]inference.Message)
	core.AssertLen(t, msgs, 0, "Read should return empty after clear")

	// Manifest message count must be 0; session still listed.
	listR := sessions.List(c)
	core.AssertTrue(t, listR.OK)
	infos := listR.Value.([]sessions.SessionInfo)
	core.AssertLen(t, infos, 1)
	core.AssertEqual(t, 0, infos[0].Messages)
	core.AssertEqual(t, "chat", infos[0].Title, "title must survive clear")
}

func TestSessions_ClearMessages_Good_AlreadyEmpty(t *core.T) {
	c := coreFixture(t)
	cr := sessions.Create(c, "empty")
	core.AssertTrue(t, cr.OK)
	id := cr.Value.(string)

	r := sessions.ClearMessages(c, id)
	core.AssertTrue(t, r.OK, "clearing an already-empty session is idempotent")
}

func TestSessions_ClearMessages_Bad_NilCore(t *core.T) {
	r := sessions.ClearMessages(nil, "id")
	core.AssertFalse(t, r.OK)
}

func TestSessions_ClearMessages_Bad_EmptyID(t *core.T) {
	c := coreFixture(t)
	r := sessions.ClearMessages(c, "")
	core.AssertFalse(t, r.OK)
}

func TestSessions_ClearMessages_Bad_UnknownID(t *core.T) {
	c := coreFixture(t)
	r := sessions.ClearMessages(c, "never-existed")
	core.AssertFalse(t, r.OK, "clearing an unknown session must fail rather than create one")
}

// — ExportAll tests — empty catalogue, multi sessions, slug
// normalisation, missing dir auto-created, fail modes.

func TestSessions_ExportAll_Good_EmptyCatalogue(t *core.T) {
	c := coreFixture(t)
	dir := core.PathJoin(t.TempDir(), "out")

	r := sessions.ExportAll(c, dir)
	core.AssertTrue(t, r.OK, "empty catalogue must succeed with zero files")
	core.AssertEqual(t, 0, r.Value.(int))
}

func TestSessions_ExportAll_Good_MultipleSessions(t *core.T) {
	c := coreFixture(t)
	id1 := sessions.Create(c, "first chat").Value.(string)
	id2 := sessions.Create(c, "second chat").Value.(string)
	core.AssertTrue(t, sessions.Append(c, id1, "user", "hello").OK)
	core.AssertTrue(t, sessions.Append(c, id2, "user", "yo").OK)

	dir := core.PathJoin(t.TempDir(), "export")
	r := sessions.ExportAll(c, dir)
	core.AssertTrue(t, r.OK)
	core.AssertEqual(t, 2, r.Value.(int))

	// Both files exist on disk; names use the slugged title + id.
	core.AssertTrue(t, core.Stat(core.PathJoin(dir, "first-chat-"+id1+".md")).OK)
	core.AssertTrue(t, core.Stat(core.PathJoin(dir, "second-chat-"+id2+".md")).OK)
}

func TestSessions_ExportAll_Good_SlugsTitleWithSpecialChars(t *core.T) {
	c := coreFixture(t)
	id := sessions.Create(c, "weird *characters* in title!").Value.(string)
	dir := core.PathJoin(t.TempDir(), "export")

	r := sessions.ExportAll(c, dir)
	core.AssertTrue(t, r.OK)
	// Slug normalises non-alnum runs to single dashes.
	core.AssertTrue(t, core.Stat(core.PathJoin(dir, "weird-characters-in-title-"+id+".md")).OK)
}

func TestSessions_ExportAll_Good_BlankTitle(t *core.T) {
	c := coreFixture(t)
	id := sessions.Create(c, "").Value.(string)
	dir := core.PathJoin(t.TempDir(), "export")

	r := sessions.ExportAll(c, dir)
	core.AssertTrue(t, r.OK)
	// Blank title falls back to "untitled".
	core.AssertTrue(t, core.Stat(core.PathJoin(dir, "untitled-"+id+".md")).OK)
}

func TestSessions_ExportAll_Good_CreatesMissingDir(t *core.T) {
	c := coreFixture(t)
	_ = sessions.Create(c, "x").Value.(string)
	// Multi-level directory that doesn't exist yet.
	dir := core.PathJoin(t.TempDir(), "deeply", "nested", "out")

	r := sessions.ExportAll(c, dir)
	core.AssertTrue(t, r.OK)
	stat := core.Stat(dir)
	core.AssertTrue(t, stat.OK, "ExportAll must create the directory tree")
	info := stat.Value.(core.FsFileInfo)
	core.AssertTrue(t, info.IsDir())
}

// — Cerberus M1 / H2 / H3 / L1 hardening tests — input bounds.

func TestSessions_SetSystemPrompt_Bad_TooLarge(t *core.T) {
	c := coreFixture(t)
	id := sessions.Create(c, "t").Value.(string)
	huge := make([]byte, sessions.SystemPromptMax+1)
	for i := range huge {
		huge[i] = 'x'
	}
	r := sessions.SetSystemPrompt(c, id, string(huge))
	core.AssertFalse(t, r.OK, "prompt past SystemPromptMax must be refused (Cerberus M1)")
}

func TestSessions_SetSystemPrompt_Good_AtCapEdge(t *core.T) {
	c := coreFixture(t)
	id := sessions.Create(c, "t").Value.(string)
	atCap := make([]byte, sessions.SystemPromptMax)
	for i := range atCap {
		atCap[i] = 'y'
	}
	r := sessions.SetSystemPrompt(c, id, string(atCap))
	core.AssertTrue(t, r.OK, "prompt exactly at SystemPromptMax must succeed")
}

func TestSessions_SetTags_Bad_TooMany(t *core.T) {
	c := coreFixture(t)
	id := sessions.Create(c, "t").Value.(string)
	tags := make([]string, sessions.MaxTags+1)
	for i := range tags {
		tags[i] = "tag" + core.Sprintf("%d", i)
	}
	r := sessions.SetTags(c, id, tags)
	core.AssertFalse(t, r.OK, "more than MaxTags tags must be refused (Cerberus M1)")
}

func TestSessions_SetTags_Bad_TagTooLong(t *core.T) {
	c := coreFixture(t)
	id := sessions.Create(c, "t").Value.(string)
	long := make([]byte, sessions.MaxTagLength+1)
	for i := range long {
		long[i] = 'a'
	}
	r := sessions.SetTags(c, id, []string{string(long)})
	core.AssertFalse(t, r.OK, "tag past MaxTagLength must be refused")
}

func TestSessions_SetTags_Good_AtCountCap(t *core.T) {
	c := coreFixture(t)
	id := sessions.Create(c, "t").Value.(string)
	tags := make([]string, sessions.MaxTags)
	for i := range tags {
		tags[i] = core.Sprintf("tag-%d", i)
	}
	r := sessions.SetTags(c, id, tags)
	core.AssertTrue(t, r.OK, "exactly MaxTags must succeed")
}

func TestSessions_Search_Bad_BelowMinLength(t *core.T) {
	c := coreFixture(t)
	id := sessions.Create(c, "anchored").Value.(string)
	core.AssertTrue(t, sessions.Append(c, id, "user", "anchor").OK)

	r := sessions.Search(c, "a")
	core.AssertTrue(t, r.OK, "below-min-length query is OK but returns empty (Cerberus H3)")
	sr := r.Value.(sessions.SearchResult)
	core.AssertLen(t, sr.Hits, 0, "single-char query must return zero hits without scanning")
	core.AssertFalse(t, sr.Truncated, "below-min-length never sets Truncated")
}

func TestSessions_Search_Good_AtMinLengthScans(t *core.T) {
	c := coreFixture(t)
	id := sessions.Create(c, "anchored").Value.(string)
	core.AssertTrue(t, sessions.Append(c, id, "user", "regex anchor").OK)

	r := sessions.Search(c, "an")
	core.AssertTrue(t, r.OK)
	sr := r.Value.(sessions.SearchResult)
	core.AssertLen(t, sr.Hits, 1)
	core.AssertEqual(t, id, sr.Hits[0].ID)
}

func TestSessions_Snippet_AdversarialInputCapped(t *core.T) {
	c := coreFixture(t)
	id := sessions.Create(c, "t").Value.(string)
	// 256 KiB of single-spaces — would O(n²) the collapse loop
	// without the snippetInputCap guard.
	huge := make([]byte, 256*1024)
	for i := range huge {
		huge[i] = ' '
	}
	core.AssertTrue(t, sessions.Append(c, id, "user", string(huge)).OK,
		"adversarial Append must complete in bounded time + not panic")

	infos := sessions.List(c).Value.([]sessions.SessionInfo)
	// Snippet collapses to "" since the trimmed result is blank.
	core.AssertEqual(t, "", infos[0].Snippet)
}

func TestSessions_ExportAll_Bad_NilCore(t *core.T) {
	r := sessions.ExportAll(nil, "/tmp/x")
	core.AssertFalse(t, r.OK)
}

func TestSessions_ExportAll_Bad_EmptyDir(t *core.T) {
	c := coreFixture(t)
	r := sessions.ExportAll(c, "")
	core.AssertFalse(t, r.OK)
}

func TestSessions_Export_Bad_UnwritablePath(t *core.T) {
	c := coreFixture(t)
	cr := sessions.Create(c, "x")
	core.AssertTrue(t, cr.OK)
	// Parent dir doesn't exist → WriteFile fails.
	r := sessions.Export(c, cr.Value.(string), "/nonexistent/path/out.md")
	core.AssertFalse(t, r.OK)
}

func TestSessions_Duplicate_Good(t *core.T) {
	c := coreFixture(t)
	srcID := sessions.Create(c, "original").Value.(string)
	core.AssertTrue(t, sessions.Append(c, srcID, "user", "hello").OK)
	core.AssertTrue(t, sessions.Append(c, srcID, "assistant", "hi there").OK)

	r := sessions.Duplicate(c, srcID)
	core.AssertTrue(t, r.OK, "Duplicate must succeed for an existing session")
	newID := r.Value.(string)
	core.AssertNotEmpty(t, newID)
	core.AssertNotEqual(t, srcID, newID, "Duplicate must produce a fresh id")

	// Both sessions present in the catalogue.
	listR := sessions.List(c)
	core.AssertTrue(t, listR.OK)
	infos := listR.Value.([]sessions.SessionInfo)
	core.AssertLen(t, infos, 2)

	// Find the new manifest and assert its shape.
	var copyInfo sessions.SessionInfo
	for _, info := range infos {
		if info.ID == newID {
			copyInfo = info
			break
		}
	}
	core.AssertEqual(t, "original (copy)", copyInfo.Title, "title must carry the (copy) suffix")
	core.AssertEqual(t, 2, copyInfo.Messages, "message count must match the source")

	// Message content is identical.
	srcMsgs := sessions.Read(c, srcID).Value.([]inference.Message)
	newMsgs := sessions.Read(c, newID).Value.([]inference.Message)
	core.AssertLen(t, newMsgs, len(srcMsgs))
	for i := range srcMsgs {
		core.AssertEqual(t, srcMsgs[i].Role, newMsgs[i].Role)
		core.AssertEqual(t, srcMsgs[i].Content, newMsgs[i].Content)
	}
}

func TestSessions_Duplicate_Good_EmptySession(t *core.T) {
	c := coreFixture(t)
	srcID := sessions.Create(c, "blank").Value.(string)

	r := sessions.Duplicate(c, srcID)
	core.AssertTrue(t, r.OK, "Duplicate of a zero-message session must succeed")
	newID := r.Value.(string)
	msgs := sessions.Read(c, newID).Value.([]inference.Message)
	core.AssertLen(t, msgs, 0, "copy must have zero messages too")
}

func TestSessions_Duplicate_Good_IsolatedFromSource(t *core.T) {
	c := coreFixture(t)
	srcID := sessions.Create(c, "live").Value.(string)
	core.AssertTrue(t, sessions.Append(c, srcID, "user", "first").OK)

	newID := sessions.Duplicate(c, srcID).Value.(string)
	// Appending to the COPY must not touch the SOURCE.
	core.AssertTrue(t, sessions.Append(c, newID, "user", "second").OK)
	srcMsgs := sessions.Read(c, srcID).Value.([]inference.Message)
	newMsgs := sessions.Read(c, newID).Value.([]inference.Message)
	core.AssertLen(t, srcMsgs, 1, "source unaffected by copy appends")
	core.AssertLen(t, newMsgs, 2, "copy has the appended second message")
}

func TestSessions_Duplicate_Bad_NilCore(t *core.T) {
	r := sessions.Duplicate(nil, "x")
	core.AssertFalse(t, r.OK)
}

func TestSessions_Duplicate_Bad_EmptyID(t *core.T) {
	c := coreFixture(t)
	r := sessions.Duplicate(c, "")
	core.AssertFalse(t, r.OK)
}

func TestSessions_Duplicate_Bad_UnknownID(t *core.T) {
	c := coreFixture(t)
	r := sessions.Duplicate(c, "no-such-session")
	core.AssertFalse(t, r.OK, "unknown source must fail rather than create a phantom copy")
}

// — Search tests — title match, content match, both, case-insensitive,
// empty query returns full list.

func TestSessions_Search_Good_TitleMatch(t *core.T) {
	c := coreFixture(t)
	idAlpha := sessions.Create(c, "regex pitfalls").Value.(string)
	idBeta := sessions.Create(c, "weekend plans").Value.(string)
	_ = idBeta

	r := sessions.Search(c, "regex")
	core.AssertTrue(t, r.OK)
	sr := r.Value.(sessions.SearchResult)
	core.AssertLen(t, sr.Hits, 1, "exactly one title match")
	core.AssertEqual(t, idAlpha, sr.Hits[0].ID)
	core.AssertFalse(t, sr.Truncated, "well below cap")
}

func TestSessions_Search_Good_ContentMatch(t *core.T) {
	c := coreFixture(t)
	idAlpha := sessions.Create(c, "unrelated title").Value.(string)
	core.AssertTrue(t, sessions.Append(c, idAlpha, "user", "how do I parse a regex anchor").OK)
	_ = sessions.Create(c, "decoy").Value.(string)

	r := sessions.Search(c, "regex anchor")
	core.AssertTrue(t, r.OK)
	sr := r.Value.(sessions.SearchResult)
	core.AssertLen(t, sr.Hits, 1, "exactly one content match")
	core.AssertEqual(t, idAlpha, sr.Hits[0].ID)
}

func TestSessions_Search_Good_AssistantContentMatch(t *core.T) {
	c := coreFixture(t)
	id := sessions.Create(c, "the meeting").Value.(string)
	core.AssertTrue(t, sessions.Append(c, id, "user", "what is RFC 5988?").OK)
	core.AssertTrue(t, sessions.Append(c, id, "assistant", "RFC 5988 covers Link headers — used for pagination.").OK)

	r := sessions.Search(c, "Link headers")
	core.AssertTrue(t, r.OK)
	sr := r.Value.(sessions.SearchResult)
	core.AssertLen(t, sr.Hits, 1, "assistant-side content must also match")
}

func TestSessions_Search_Good_CaseInsensitive(t *core.T) {
	c := coreFixture(t)
	id := sessions.Create(c, "Mixed-Case Title").Value.(string)
	core.AssertTrue(t, sessions.Append(c, id, "user", "Some MIXED Content Inside").OK)

	for _, query := range []string{"mixed", "MIXED", "MiXeD"} {
		r := sessions.Search(c, query)
		core.AssertTrue(t, r.OK)
		sr := r.Value.(sessions.SearchResult)
		core.AssertLen(t, sr.Hits, 1, "case-insensitive match must find the session for "+query)
	}
}

func TestSessions_Search_Good_TitleAndContentBoth(t *core.T) {
	c := coreFixture(t)
	idTitle := sessions.Create(c, "regex deep dive").Value.(string)
	idContent := sessions.Create(c, "the chat").Value.(string)
	core.AssertTrue(t, sessions.Append(c, idContent, "user", "explain the regex syntax for groups").OK)

	r := sessions.Search(c, "regex")
	core.AssertTrue(t, r.OK)
	sr := r.Value.(sessions.SearchResult)
	core.AssertLen(t, sr.Hits, 2, "title match + content match both surface")
	gotIDs := map[string]bool{}
	for _, h := range sr.Hits {
		gotIDs[h.ID] = true
	}
	core.AssertTrue(t, gotIDs[idTitle])
	core.AssertTrue(t, gotIDs[idContent])
}

func TestSessions_Search_Good_NoMatches(t *core.T) {
	c := coreFixture(t)
	_ = sessions.Create(c, "first").Value.(string)
	_ = sessions.Create(c, "second").Value.(string)

	r := sessions.Search(c, "no-such-needle")
	core.AssertTrue(t, r.OK, "missing matches is success with an empty slice, not failure")
	sr := r.Value.(sessions.SearchResult)
	core.AssertLen(t, sr.Hits, 0)
	core.AssertFalse(t, sr.Truncated)
}

func TestSessions_Search_Good_EmptyQueryReturnsAll(t *core.T) {
	c := coreFixture(t)
	_ = sessions.Create(c, "a").Value.(string)
	_ = sessions.Create(c, "b").Value.(string)
	_ = sessions.Create(c, "c").Value.(string)

	for _, q := range []string{"", "   ", "\t\n"} {
		r := sessions.Search(c, q)
		core.AssertTrue(t, r.OK)
		sr := r.Value.(sessions.SearchResult)
		core.AssertLen(t, sr.Hits, 3, "blank query for "+core.Sprintf("%q", q)+" returns every session")
	}
}

func TestSessions_Search_Bad_NilCore(t *core.T) {
	r := sessions.Search(nil, "x")
	core.AssertFalse(t, r.OK)
}

// — Snippet behaviour — Append populates, RemoveLast recomputes,
// ClearMessages blanks, Duplicate copies, multi-line collapses.

func TestSessions_Snippet_Append_PopulatesManifest(t *core.T) {
	c := coreFixture(t)
	id := sessions.Create(c, "thread").Value.(string)
	// Fresh session has no snippet yet.
	listR := sessions.List(c)
	core.AssertTrue(t, listR.OK)
	infos := listR.Value.([]sessions.SessionInfo)
	core.AssertLen(t, infos, 1)
	core.AssertEqual(t, "", infos[0].Snippet, "new session must start without a snippet")

	core.AssertTrue(t, sessions.Append(c, id, "user", "what is the weather today").OK)
	listR = sessions.List(c)
	infos = listR.Value.([]sessions.SessionInfo)
	core.AssertEqual(t, "what is the weather today", infos[0].Snippet)
}

func TestSessions_Snippet_Append_TruncatesLongContent(t *core.T) {
	c := coreFixture(t)
	id := sessions.Create(c, "long thread").Value.(string)
	long := ""
	for i := 0; i < 200; i++ {
		long += "x"
	}
	core.AssertTrue(t, sessions.Append(c, id, "assistant", long).OK)

	infos := sessions.List(c).Value.([]sessions.SessionInfo)
	core.AssertEqual(t, sessions.SnippetMax, core.RuneCount(infos[0].Snippet),
		"snippet length must cap at SnippetMax runes")
	// Ellipsis sits at the end when truncation happened.
	tail := infos[0].Snippet[len(infos[0].Snippet)-len("…"):]
	core.AssertEqual(t, "…", tail)
}

func TestSessions_Snippet_Append_CollapsesNewlines(t *core.T) {
	c := coreFixture(t)
	id := sessions.Create(c, "multi-line").Value.(string)
	core.AssertTrue(t, sessions.Append(c, id, "user", "line one\nline two\nline three").OK)

	infos := sessions.List(c).Value.([]sessions.SessionInfo)
	core.AssertEqual(t, "line one line two line three", infos[0].Snippet,
		"newlines must collapse to single spaces")
}

func TestSessions_Snippet_Append_TrimsSurroundingSpace(t *core.T) {
	c := coreFixture(t)
	id := sessions.Create(c, "spacey").Value.(string)
	core.AssertTrue(t, sessions.Append(c, id, "user", "  \n  hi there  \n  ").OK)

	infos := sessions.List(c).Value.([]sessions.SessionInfo)
	core.AssertEqual(t, "hi there", infos[0].Snippet)
}

func TestSessions_Snippet_RemoveLast_RecomputesFromNewTail(t *core.T) {
	c := coreFixture(t)
	id := sessions.Create(c, "t").Value.(string)
	core.AssertTrue(t, sessions.Append(c, id, "user", "first message").OK)
	core.AssertTrue(t, sessions.Append(c, id, "assistant", "second message").OK)

	core.AssertEqual(t, "second message", sessions.List(c).Value.([]sessions.SessionInfo)[0].Snippet)

	r := sessions.RemoveLast(c, id)
	core.AssertTrue(t, r.OK)

	// Snippet rolls back to the new tail.
	core.AssertEqual(t, "first message", sessions.List(c).Value.([]sessions.SessionInfo)[0].Snippet)
}

func TestSessions_Snippet_RemoveLast_BlanksWhenEmptied(t *core.T) {
	c := coreFixture(t)
	id := sessions.Create(c, "t").Value.(string)
	core.AssertTrue(t, sessions.Append(c, id, "user", "only message").OK)
	core.AssertTrue(t, sessions.RemoveLast(c, id).OK)

	core.AssertEqual(t, "", sessions.List(c).Value.([]sessions.SessionInfo)[0].Snippet,
		"empty session → blank snippet")
}

func TestSessions_Snippet_ClearMessages_Blanks(t *core.T) {
	c := coreFixture(t)
	id := sessions.Create(c, "t").Value.(string)
	core.AssertTrue(t, sessions.Append(c, id, "user", "something here").OK)
	core.AssertEqual(t, "something here", sessions.List(c).Value.([]sessions.SessionInfo)[0].Snippet)

	core.AssertTrue(t, sessions.ClearMessages(c, id).OK)
	core.AssertEqual(t, "", sessions.List(c).Value.([]sessions.SessionInfo)[0].Snippet,
		"ClearMessages must blank the snippet too")
}

// — SystemPrompt tests — set, list-readback, clear, Duplicate copies,
// fail modes (nil core, empty id, unknown id).

func TestSessions_SetSystemPrompt_Good(t *core.T) {
	c := coreFixture(t)
	id := sessions.Create(c, "t").Value.(string)

	r := sessions.SetSystemPrompt(c, id, "You answer in haiku.")
	core.AssertTrue(t, r.OK)

	infos := sessions.List(c).Value.([]sessions.SessionInfo)
	core.AssertLen(t, infos, 1)
	core.AssertEqual(t, "You answer in haiku.", infos[0].SystemPrompt)
}

func TestSessions_SetSystemPrompt_Good_Empty(t *core.T) {
	c := coreFixture(t)
	id := sessions.Create(c, "t").Value.(string)
	core.AssertTrue(t, sessions.SetSystemPrompt(c, id, "Original prompt").OK)

	// Clear via empty string.
	core.AssertTrue(t, sessions.SetSystemPrompt(c, id, "").OK)
	infos := sessions.List(c).Value.([]sessions.SessionInfo)
	core.AssertEqual(t, "", infos[0].SystemPrompt, "empty prompt clears the field")
}

func TestSessions_SetSystemPrompt_Good_BumpsUpdatedAt(t *core.T) {
	c := coreFixture(t)
	id := sessions.Create(c, "t").Value.(string)
	core.AssertTrue(t, sessions.SetSystemPrompt(c, id, "v1").OK)

	before := sessions.List(c).Value.([]sessions.SessionInfo)[0].UpdatedAt
	// Step the clock so UnixNow's second-grained value changes.
	core.Sleep(1100 * core.Millisecond)
	core.AssertTrue(t, sessions.SetSystemPrompt(c, id, "v2").OK)
	after := sessions.List(c).Value.([]sessions.SessionInfo)[0].UpdatedAt

	core.AssertTrue(t, after > before, "SetSystemPrompt must bump UpdatedAt so the rail reorders")
}

func TestSessions_SetSystemPrompt_Bad_NilCore(t *core.T) {
	r := sessions.SetSystemPrompt(nil, "x", "p")
	core.AssertFalse(t, r.OK)
}

func TestSessions_SetSystemPrompt_Bad_EmptyID(t *core.T) {
	c := coreFixture(t)
	r := sessions.SetSystemPrompt(c, "", "prompt")
	core.AssertFalse(t, r.OK)
}

func TestSessions_SetSystemPrompt_Bad_UnknownID(t *core.T) {
	c := coreFixture(t)
	r := sessions.SetSystemPrompt(c, "no-such-session", "prompt")
	core.AssertFalse(t, r.OK, "unknown session must fail rather than silently create")
}

// — Tags tests — set, normalise (trim/lower/dedupe), clear, Duplicate
// carries, fail modes.

func TestSessions_SetTags_Good(t *core.T) {
	c := coreFixture(t)
	id := sessions.Create(c, "t").Value.(string)

	r := sessions.SetTags(c, id, []string{"work", "ops"})
	core.AssertTrue(t, r.OK)

	infos := sessions.List(c).Value.([]sessions.SessionInfo)
	core.AssertEqual(t, 2, len(infos[0].Tags))
	core.AssertEqual(t, "work", infos[0].Tags[0])
	core.AssertEqual(t, "ops", infos[0].Tags[1])
}

func TestSessions_SetTags_Good_TrimsLowercasesDedupes(t *core.T) {
	c := coreFixture(t)
	id := sessions.Create(c, "t").Value.(string)

	// Mixed: padded whitespace, uppercase, duplicates, blanks.
	r := sessions.SetTags(c, id, []string{"  Work  ", "ops", "WORK", "", "  ", "Ops", "research"})
	core.AssertTrue(t, r.OK)

	tags := sessions.List(c).Value.([]sessions.SessionInfo)[0].Tags
	// Normalised: first occurrence wins. Order: work, ops, research.
	core.AssertEqual(t, 3, len(tags))
	core.AssertEqual(t, "work", tags[0])
	core.AssertEqual(t, "ops", tags[1])
	core.AssertEqual(t, "research", tags[2])
}

func TestSessions_SetTags_Good_ClearWithEmptyInput(t *core.T) {
	c := coreFixture(t)
	id := sessions.Create(c, "t").Value.(string)
	core.AssertTrue(t, sessions.SetTags(c, id, []string{"x", "y"}).OK)

	// nil clears.
	core.AssertTrue(t, sessions.SetTags(c, id, nil).OK)
	tags := sessions.List(c).Value.([]sessions.SessionInfo)[0].Tags
	core.AssertEqual(t, 0, len(tags))

	// []string{} also clears.
	core.AssertTrue(t, sessions.SetTags(c, id, []string{"a"}).OK)
	core.AssertTrue(t, sessions.SetTags(c, id, []string{}).OK)
	tags = sessions.List(c).Value.([]sessions.SessionInfo)[0].Tags
	core.AssertEqual(t, 0, len(tags))
}

func TestSessions_SetTags_Good_OnlyBlanksClears(t *core.T) {
	c := coreFixture(t)
	id := sessions.Create(c, "t").Value.(string)
	// All-blank input is equivalent to clearing.
	r := sessions.SetTags(c, id, []string{"  ", "\t", ""})
	core.AssertTrue(t, r.OK)
	tags := sessions.List(c).Value.([]sessions.SessionInfo)[0].Tags
	core.AssertEqual(t, 0, len(tags))
}

func TestSessions_SetTags_Good_BumpsUpdatedAt(t *core.T) {
	c := coreFixture(t)
	id := sessions.Create(c, "t").Value.(string)
	core.AssertTrue(t, sessions.SetTags(c, id, []string{"v1"}).OK)

	before := sessions.List(c).Value.([]sessions.SessionInfo)[0].UpdatedAt
	core.Sleep(1100 * core.Millisecond)
	core.AssertTrue(t, sessions.SetTags(c, id, []string{"v2"}).OK)
	after := sessions.List(c).Value.([]sessions.SessionInfo)[0].UpdatedAt

	core.AssertTrue(t, after > before, "SetTags must bump UpdatedAt")
}

func TestSessions_SetTags_Bad_NilCore(t *core.T) {
	r := sessions.SetTags(nil, "x", []string{"a"})
	core.AssertFalse(t, r.OK)
}

func TestSessions_SetTags_Bad_EmptyID(t *core.T) {
	c := coreFixture(t)
	r := sessions.SetTags(c, "", []string{"a"})
	core.AssertFalse(t, r.OK)
}

func TestSessions_SetTags_Bad_UnknownID(t *core.T) {
	c := coreFixture(t)
	r := sessions.SetTags(c, "no-such-session", []string{"a"})
	core.AssertFalse(t, r.OK)
}

func TestSessions_Tags_Duplicate_Carries(t *core.T) {
	c := coreFixture(t)
	id := sessions.Create(c, "src").Value.(string)
	core.AssertTrue(t, sessions.SetTags(c, id, []string{"work", "ops"}).OK)

	r := sessions.Duplicate(c, id)
	core.AssertTrue(t, r.OK)
	newID := r.Value.(string)

	infos := sessions.List(c).Value.([]sessions.SessionInfo)
	var copyInfo sessions.SessionInfo
	for _, info := range infos {
		if info.ID == newID {
			copyInfo = info
			break
		}
	}
	core.AssertEqual(t, 2, len(copyInfo.Tags), "Duplicate carries tag set")
	core.AssertEqual(t, "work", copyInfo.Tags[0])
}

func TestSessions_Tags_Duplicate_IsolatedSlice(t *core.T) {
	c := coreFixture(t)
	id := sessions.Create(c, "src").Value.(string)
	core.AssertTrue(t, sessions.SetTags(c, id, []string{"a", "b"}).OK)
	newID := sessions.Duplicate(c, id).Value.(string)

	// Mutating the copy's tags must not touch the source.
	core.AssertTrue(t, sessions.SetTags(c, newID, []string{"a", "b", "c"}).OK)
	srcTags := sessions.List(c).Value.([]sessions.SessionInfo)
	var srcInfo, newInfo sessions.SessionInfo
	for _, info := range srcTags {
		if info.ID == id {
			srcInfo = info
		}
		if info.ID == newID {
			newInfo = info
		}
	}
	core.AssertEqual(t, 2, len(srcInfo.Tags), "source unchanged")
	core.AssertEqual(t, 3, len(newInfo.Tags))
}

func TestSessions_SystemPrompt_Duplicate_Carries(t *core.T) {
	c := coreFixture(t)
	id := sessions.Create(c, "src").Value.(string)
	core.AssertTrue(t, sessions.SetSystemPrompt(c, id, "carry me").OK)

	r := sessions.Duplicate(c, id)
	core.AssertTrue(t, r.OK)
	newID := r.Value.(string)

	infos := sessions.List(c).Value.([]sessions.SessionInfo)
	var copyInfo sessions.SessionInfo
	for _, info := range infos {
		if info.ID == newID {
			copyInfo = info
			break
		}
	}
	core.AssertEqual(t, "carry me", copyInfo.SystemPrompt,
		"Duplicate must carry the system prompt to the copy")
}

func TestSessions_Snippet_Duplicate_Copies(t *core.T) {
	c := coreFixture(t)
	id := sessions.Create(c, "src").Value.(string)
	core.AssertTrue(t, sessions.Append(c, id, "user", "preserved snippet").OK)

	r := sessions.Duplicate(c, id)
	core.AssertTrue(t, r.OK)
	newID := r.Value.(string)

	infos := sessions.List(c).Value.([]sessions.SessionInfo)
	var copyInfo sessions.SessionInfo
	for _, info := range infos {
		if info.ID == newID {
			copyInfo = info
			break
		}
	}
	core.AssertEqual(t, "preserved snippet", copyInfo.Snippet,
		"Duplicate must carry the snippet to the copy")
}

func TestSessions_Search_Good_TitleMatchSkipsMessageRead(t *core.T) {
	// Title hit short-circuits the message read — this test asserts the
	// happy path doesn't accidentally include sessions whose title alone
	// matches AND surfaces them once, not twice (the implementation uses
	// `continue` after a title hit; this guards against future drift).
	c := coreFixture(t)
	id := sessions.Create(c, "alpha bravo").Value.(string)
	core.AssertTrue(t, sessions.Append(c, id, "user", "alpha bravo charlie").OK)

	r := sessions.Search(c, "alpha")
	core.AssertTrue(t, r.OK)
	sr := r.Value.(sessions.SearchResult)
	core.AssertLen(t, sr.Hits, 1, "title hit + content hit on the same session = one entry only")
}

// TestSessions_Search_RespectsMaxScan_Good proves the worst-case bound:
// Search stops after MaxSearchScan sessions and flags the result
// Truncated:true instead of marching the full catalogue per keystroke
// (Mantis #1538 / Cerberus H9-verify F5). Uses SetMaxSearchScanForTest
// to shrink the cap so the test stays fast rather than populating 5500
// sessions.
func TestSessions_Search_RespectsMaxScan_Good(t *core.T) {
	c := coreFixture(t)
	defer sessions.SetMaxSearchScanForTest(50)()

	// Populate 60 sessions whose title + body never match "needle"
	// so the loop has to keep scanning until it hits the cap.
	const total = 60
	for i := 0; i < total; i++ {
		id := sessions.Create(c, core.Sprintf("decoy-%03d", i)).Value.(string)
		core.AssertTrue(t, sessions.Append(c, id, "user", "irrelevant body").OK)
	}

	r := sessions.Search(c, "needle")
	core.AssertTrue(t, r.OK, "Search must succeed even when truncated")
	sr := r.Value.(sessions.SearchResult)
	core.AssertTrue(t, sr.Truncated, "scanning past the cap must flip Truncated:true")
	core.AssertLen(t, sr.Hits, 0, "no decoy matches the query — hits stays empty")
	core.AssertLessOrEqual(t, len(sr.Hits), 50, "hit count bounded by the cap")
}

// TestSessions_Search_RespectsMaxScan_BelowCap proves the inverse:
// a catalogue smaller than the cap MUST return Truncated:false even
// when zero matches are found — the flag means "ran out of scan
// budget", not "found nothing".
func TestSessions_Search_RespectsMaxScan_BelowCap(t *core.T) {
	c := coreFixture(t)
	defer sessions.SetMaxSearchScanForTest(50)()

	for i := 0; i < 10; i++ {
		_ = sessions.Create(c, core.Sprintf("decoy-%03d", i)).Value.(string)
	}

	r := sessions.Search(c, "needle")
	core.AssertTrue(t, r.OK)
	sr := r.Value.(sessions.SearchResult)
	core.AssertFalse(t, sr.Truncated, "scanned every session inside the cap — Truncated stays false")
	core.AssertLen(t, sr.Hits, 0)
}

// sessionsScopeLiteral mirrors the package-internal sessionsScope const
// stamped on every typed audit row from pkg/sessions — duplicated here
// as a string literal so a future rename still fires the test if the
// constant value drifts from the closed-set spec.
const sessionsScopeLiteral = "sessions"

// recordingSessionsRecorder captures every audit.Event the sessions
// package emits during a test. Mirror of the same-named fixture in
// pkg/downloader/audit_test.go + pkg/process/audit_test.go.
type recordingSessionsRecorder struct {
	mu     sync.Mutex
	events []audit.Event
}

func (r *recordingSessionsRecorder) Record(ev audit.Event) core.Result {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, ev)
	return core.Ok(nil)
}

func (r *recordingSessionsRecorder) snapshot() []audit.Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]audit.Event, len(r.events))
	copy(out, r.events)
	return out
}

func (r *recordingSessionsRecorder) filterByName(name string) []audit.Event {
	all := r.snapshot()
	out := make([]audit.Event, 0, len(all))
	for _, ev := range all {
		if ev.Event == name {
			out = append(out, ev)
		}
	}
	return out
}

// installSessionsAuditRecorder wires a recordingSessionsRecorder as
// audit.Default and restores the prior default at test end.
func installSessionsAuditRecorder(t *core.T) *recordingSessionsRecorder {
	t.Helper()
	rec := &recordingSessionsRecorder{}
	audit.SetDefault(rec)
	t.Cleanup(func() { audit.SetDefault(nil) })
	return rec
}

// TestSessions_Create_EmitsAuditTrail_Good — happy-path Create asserts
// the Requested + Succeeded trio (no Failed) lands with the expected
// scope + outcome + Meta. 7th audit-cluster adoption per Cerberus #67
// F-2 (Mantis #1737) — sibling shape of the closed clusters across
// runner / sandbox / process / marketplace / gateway / downloader /
// queue.
func TestSessions_Create_EmitsAuditTrail_Good(t *core.T) {
	c := coreFixture(t)
	rec := installSessionsAuditRecorder(t)

	r := sessions.Create(c, "audit-good")
	core.AssertTrue(t, r.OK, "Create happy path must succeed")
	id := r.Value.(string)

	requested := rec.filterByName(audit.EventSessionsCreateRequested)
	succeeded := rec.filterByName(audit.EventSessionsCreateSucceeded)
	failed := rec.filterByName(audit.EventSessionsCreateFailed)

	core.AssertEqual(t, 1, len(requested),
		"exactly one Requested row on happy path")
	core.AssertEqual(t, 1, len(succeeded),
		"exactly one Succeeded row on happy path")
	core.AssertEqual(t, 0, len(failed),
		"no Failed row on happy path")

	// Scope + Outcome shape per RFC §2.
	core.AssertEqual(t, sessionsScopeLiteral, requested[0].Scope)
	core.AssertEqual(t, audit.OutcomeOK, requested[0].Outcome)
	core.AssertEqual(t, sessionsScopeLiteral, succeeded[0].Scope)
	core.AssertEqual(t, audit.OutcomeOK, succeeded[0].Outcome)

	// Requested Meta — title_len only, no session_id yet (id allocation
	// happens AFTER the Requested emit).
	core.AssertEqual(t, len([]rune("audit-good")), requested[0].Meta["title_len"])

	// Succeeded Meta — session_id + title_len.
	core.AssertEqual(t, id, succeeded[0].Meta["session_id"])
	core.AssertEqual(t, len([]rune("audit-good")), succeeded[0].Meta["title_len"])
}

// TestSessions_Delete_EmitsAuditTrail_Good — happy-path Delete asserts
// the Requested + Succeeded trio (no Failed) lands with the expected
// scope + outcome + Meta. The Requested row commits regardless of
// registry state (idempotent Delete contract).
func TestSessions_Delete_EmitsAuditTrail_Good(t *core.T) {
	c := coreFixture(t)
	id := sessions.Create(c, "to-delete").Value.(string)
	// Install recorder AFTER Create so the Create rows don't bleed
	// into the Delete-only assertions.
	rec := installSessionsAuditRecorder(t)

	r := sessions.Delete(c, id)
	core.AssertTrue(t, r.OK, "Delete happy path must succeed")

	requested := rec.filterByName(audit.EventSessionsDeleteRequested)
	succeeded := rec.filterByName(audit.EventSessionsDeleteSucceeded)
	failed := rec.filterByName(audit.EventSessionsDeleteFailed)

	core.AssertEqual(t, 1, len(requested),
		"exactly one Requested row on happy path")
	core.AssertEqual(t, 1, len(succeeded),
		"exactly one Succeeded row on happy path")
	core.AssertEqual(t, 0, len(failed),
		"no Failed row on happy path")

	core.AssertEqual(t, sessionsScopeLiteral, requested[0].Scope)
	core.AssertEqual(t, audit.OutcomeOK, requested[0].Outcome)
	core.AssertEqual(t, sessionsScopeLiteral, succeeded[0].Scope)
	core.AssertEqual(t, audit.OutcomeOK, succeeded[0].Outcome)

	core.AssertEqual(t, id, requested[0].Meta["session_id"])
	core.AssertEqual(t, id, succeeded[0].Meta["session_id"])
}

// TestSessions_AuditMeta_ErrorCodeBoundedKeyspace_Ugly — W1 substrate
// canon check. Forces a Failed path (Rename on a non-existent session
// surfaces readManifest's "not found" error through the manifest read)
// and asserts Meta["error_code"] is the bounded *core.Err.Operation
// literal, NOT the raw r.Error() prose. Defends the audit.ErrorCode(r)
// substrate at the emit boundary so future regressions that revert the
// helper signature to a raw string fall loud rather than silently
// re-leaking caller-controlled input.
//
// Mirrors pkg/downloader.TestDownloader_AuditMeta_ErrorCodeBoundedKeyspace_Bad —
// each new audit-cluster adoption pins the same contract at its boundary.
func TestSessions_AuditMeta_ErrorCodeBoundedKeyspace_Ugly(t *core.T) {
	c := coreFixture(t)
	rec := installSessionsAuditRecorder(t)

	// Rename a non-existent session id — readManifest returns the
	// "sessions.readManifest: not found" error which routes through
	// emitRenameFailed → audit.ErrorCode(r).
	canaryID := "RENAME-CANARY-NOTAREALID-LTHN"
	r := sessions.Rename(c, canaryID, "new title")
	core.AssertFalse(t, r.OK, "Rename of missing id must fail")

	failed := rec.filterByName(audit.EventSessionsRenameFailed)
	core.AssertEqual(t, 1, len(failed),
		"exactly one Failed row on missing-id path")

	// Bounded-keyspace contract: error_code is a *core.Err.Operation
	// canon (e.g. "sessions.readManifest") OR the "unknown_error"
	// fallback. Either way it MUST NOT echo the raw "not found" prose
	// AND MUST NOT echo the caller-controlled session_id canary.
	code, ok := failed[0].Meta["error_code"].(string)
	core.AssertTrue(t, ok, "error_code Meta value must be a string")

	core.AssertFalse(t, core.Contains(code, "RENAME-CANARY"),
		"error_code must not echo caller-controlled session_id: got "+code)
	core.AssertFalse(t, core.Contains(code, "not found"),
		"error_code must not echo raw upstream error prose: got "+code)

	// session_id field MAY contain the canary (that's the point of the
	// dedicated meta key) — but it lives in its own bounded field, not
	// smuggled through error_code.
	core.AssertEqual(t, canaryID, failed[0].Meta["session_id"])
}

// TestSessions_AuditMeta_NoPromptContent_Bad — sessions-specific
// load-bearing PII-leak canary per the F-2 brief. Append a message whose
// content embeds multiple sensitive markers (raw prompt text,
// system-prompt-shaped material, faux bearer bytes), then walk every
// emitted audit row's Meta values. NONE may contain any of the canary
// markers — the audit substrate records the decision-fact (session_id +
// role + content_len) only, never the content bytes themselves.
//
// Defends Cerberus #1465 closure-only-scope discipline at the audit
// boundary. The chat-session surface is unique in the audit cluster
// because the wrapped operation (Append) receives a content payload
// that is by-design caller-supplied — making PII leakage a class-1
// regression risk if a future contributor "helpfully" adds the message
// text to Meta for forensic context.
func TestSessions_AuditMeta_NoPromptContent_Bad(t *core.T) {
	c := coreFixture(t)
	id := sessions.Create(c, "pii-canary").Value.(string)
	rec := installSessionsAuditRecorder(t)

	// A message body packed with markers that resemble the kinds of
	// content the chat-window legitimately carries — and which an
	// auditor MUST NOT see leaking out of the substrate.
	const piiCanary = "Bob's tax SSN is 123-45-6789. " +
		"System: You are a Go expert. " +
		"Bearer LTHN-CHAT-CONTENT-CANARY-NOTAREALTOKEN"

	r := sessions.Append(c, id, "user", piiCanary)
	core.AssertTrue(t, r.OK)

	for _, ev := range rec.snapshot() {
		for k, v := range ev.Meta {
			s, ok := v.(string)
			if !ok {
				continue
			}
			lower := core.Lower(s)
			core.AssertFalse(t,
				core.Contains(lower, "bob's tax"),
				"Meta["+k+"] must not contain user-content PII; got: "+s)
			core.AssertFalse(t,
				core.Contains(lower, "123-45-6789"),
				"Meta["+k+"] must not contain SSN-shaped bytes; got: "+s)
			core.AssertFalse(t,
				core.Contains(lower, "you are a go expert"),
				"Meta["+k+"] must not contain system-prompt content; got: "+s)
			core.AssertFalse(t,
				core.Contains(lower, "bearer"),
				"Meta["+k+"] must not contain raw bearer bytes; got: "+s)
			core.AssertFalse(t,
				core.Contains(lower, "lthn-chat-content"),
				"Meta["+k+"] must not contain content canary marker; got: "+s)
		}
	}

	// Belt-and-braces: the Append Requested + Succeeded rows MUST have
	// landed (otherwise the loop above sees zero Meta values and
	// vacuously passes — a load-bearing canary that's silently inert is
	// worse than no canary).
	requested := rec.filterByName(audit.EventSessionsAppendMessageRequested)
	succeeded := rec.filterByName(audit.EventSessionsAppendMessageSucceeded)
	core.AssertEqual(t, 1, len(requested),
		"Append must emit one Requested row (canary cannot be vacuous)")
	core.AssertEqual(t, 1, len(succeeded),
		"Append must emit one Succeeded row (canary cannot be vacuous)")

	// The content_len Meta is forensically useful AND must reflect the
	// real byte count — proves the canary actually flowed through Append.
	core.AssertEqual(t, len(piiCanary), requested[0].Meta["content_len"])
	core.AssertEqual(t, len(piiCanary), succeeded[0].Meta["content_len"])
	core.AssertEqual(t, "user", requested[0].Meta["role"])
}
