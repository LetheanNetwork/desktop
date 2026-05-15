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

	core "dappco.re/go"
	"dappco.re/go/inference"
	"dappco.re/go/store"

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
