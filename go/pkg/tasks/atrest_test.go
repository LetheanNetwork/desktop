// SPDX-Licence-Identifier: EUPL-1.2

// Stage E at-rest sealing tests for tasks user-prompt fields —
// Mantis #1727 / Cerberus #64 F-4. Exercises the round-trip
// seal/unseal path with a real PGP keypair across Issue.Description
// + Note.Body, plus the locked-account legacy-plaintext fallback +
// the queryable-without-unlock state-column property.

package tasks_test

import (
	"sync"

	core "dappco.re/go"
	"dappco.re/go/orm"

	"dappco.re/lthn/desktop/pkg/account"
	"dappco.re/lthn/desktop/pkg/tasks"

	"github.com/Snider/Enchantrix/pkg/crypt/std/pgp"
)

// tasksTestAccountSurface mirrors the sessions test stub — narrow
// live-read surface tasks consumes from the wired account service.
// Registered under the canonical "account" core name.
type tasksTestAccountSurface struct {
	unlocked []string
	pub      []byte
	priv     []byte
}

func (s *tasksTestAccountSurface) UnlockedAccountIDs() []string { return s.unlocked }
func (s *tasksTestAccountSurface) PublicKeyFor(id string) ([]byte, bool) {
	if id == "" || s.pub == nil {
		return nil, false
	}
	return s.pub, true
}
func (s *tasksTestAccountSurface) PrivateKeyFor(id string) (*account.PrivateKeyHandle, bool) {
	if id == "" || s.priv == nil {
		return nil, false
	}
	return account.NewPrivateKeyHandleForTest(append([]byte(nil), s.priv...)), true
}

var (
	tasksAtrestKeyOnce sync.Once
	tasksAtrestKeyPub  []byte
	tasksAtrestKeyPriv []byte
)

func tasksAtrestKeyPair(t *core.T) (pub, priv []byte) {
	t.Helper()
	tasksAtrestKeyOnce.Do(func() {
		svc := pgp.NewService()
		p, k, err := svc.GenerateKeyPair("Tasks", "tasks-test@lthn.local", "test")
		if err != nil {
			t.Fatalf("generate test key pair: %v", err)
		}
		tasksAtrestKeyPub = p
		tasksAtrestKeyPriv = k
	})
	return tasksAtrestKeyPub, tasksAtrestKeyPriv
}

// sealedTasksCoreFixture builds a Core with orm + an unlocked
// account stub so the at-rest seal/unseal path engages by default.
func sealedTasksCoreFixture(t *core.T) *core.Core {
	t.Helper()
	pub, priv := tasksAtrestKeyPair(t)
	c := core.New(
		core.WithName("account", func(c *core.Core) core.Result {
			return c.RegisterService("account", &tasksTestAccountSurface{
				unlocked: []string{"acct-tasks-sealed"},
				pub:      pub,
				priv:     priv,
			})
		}),
	)
	core.RequireTrue(t, orm.Register(c).OK)
	mem := orm.NewMemium()
	core.RequireTrue(t, orm.Mount(c, "default", mem).OK)
	for _, schema := range tasks.Schemas() {
		core.RequireTrue(t, orm.RegisterSchema(c, schema).OK)
		mem.RegisterTable(schema.Name, schema)
	}
	core.RequireTrue(t, c.ServiceStartup(core.Background(), nil).OK)
	t.Cleanup(func() { _ = c.ServiceShutdown(core.Background()) })
	return c
}

// TestTasks_Description_AtRest_Good — Issue.Description is sealed
// in the DuckDB column on insert + decrypted on Get/List under the
// unlocked account. The round-trip preserves the human-readable
// string byte-for-byte.
func TestTasks_Description_AtRest_Good(t *core.T) {
	c := sealedTasksCoreFixture(t)
	r := tasks.Create(c, tasks.CreateInput{
		Project:     "ide",
		Summary:     "sealed-issue test",
		Description: "highly confidential description prompt",
	})
	core.RequireTrue(t, r.OK)
	issue, _, ok := orm.Detail[tasks.Issue](r)
	core.RequireTrue(t, ok)

	// Get round-trip unseals.
	got := tasks.Get(c, issue.ID)
	core.RequireTrue(t, got.OK)
	gotIssue, _, ok := orm.Detail[tasks.Issue](got)
	core.RequireTrue(t, ok)
	core.AssertEqual(t, "highly confidential description prompt", gotIssue.Description)

	// Raw orm read MUST see the sealed envelope (not the plaintext).
	raw := orm.Of[tasks.Issue](c).Find(issue.ID)
	core.RequireTrue(t, raw.OK)
	rawIssue, _, ok := orm.Detail[tasks.Issue](raw)
	core.RequireTrue(t, ok)
	core.AssertTrue(t, core.HasPrefix(rawIssue.Description, `{"sealed":`),
		"raw orm read must see sealed envelope, got: "+rawIssue.Description)
}

// TestTasks_State_QueryableWithoutUnlock_Good — orchestration
// fields (project, state, severity) stay plain so Where()-filtered
// queries continue to hit the right rows even when the account is
// locked. Pins the "List() works without unlock" property the brief
// requires.
func TestTasks_State_QueryableWithoutUnlock_Good(t *core.T) {
	c := sealedTasksCoreFixture(t)
	a := tasks.Create(c, tasks.CreateInput{Project: "ide", Summary: "a", Description: "sealed-a"})
	core.RequireTrue(t, a.OK)
	b := tasks.Create(c, tasks.CreateInput{Project: "ofm", Summary: "b", Description: "sealed-b"})
	core.RequireTrue(t, b.OK)

	// State + project columns drive the WHERE — those must remain
	// queryable regardless of unlock-state. Description is empty in
	// the result rows whose unseal fails closed, but the count + the
	// metadata rounds-trip cleanly.
	r := tasks.List(c, tasks.ListFilter{Project: "ide", State: tasks.StateOpen})
	core.RequireTrue(t, r.OK)
	issues, ok := r.Value.([]tasks.Issue)
	core.RequireTrue(t, ok)
	core.AssertEqual(t, 1, len(issues))
	core.AssertEqual(t, "ide", issues[0].Project)
	core.AssertEqual(t, tasks.StateOpen, issues[0].State)
}

// TestTasks_Note_AtRest_Good — Note.Body is sealed on insert and
// decrypted on ListNotes. Mirrors the Issue.Description property
// for the comment surface.
func TestTasks_Note_AtRest_Good(t *core.T) {
	c := sealedTasksCoreFixture(t)
	issue := tasks.Create(c, tasks.CreateInput{Project: "ide", Summary: "parent"}).Value.(tasks.Issue)

	n := tasks.AddNote(c, issue.ID, "Landed at SHA abc123 — confidential", "cladius")
	core.RequireTrue(t, n.OK)

	notes := tasks.ListNotes(c, issue.ID).Value.([]tasks.Note)
	core.AssertLen(t, notes, 1)
	core.AssertEqual(t, "Landed at SHA abc123 — confidential", notes[0].Body)

	// Raw orm read sees the sealed envelope.
	raw := orm.Of[tasks.Note](c).Where("issue_id", "=", issue.ID).Get()
	core.RequireTrue(t, raw.OK)
	rawNotes := raw.Value.([]tasks.Note)
	core.AssertLen(t, rawNotes, 1)
	core.AssertTrue(t, core.HasPrefix(rawNotes[0].Body, `{"sealed":`),
		"raw orm read must see sealed Note.Body envelope")
}
