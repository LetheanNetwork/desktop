// SPDX-Licence-Identifier: EUPL-1.2

// At-rest sealing tests for Job.Payload — Mantis #1727 / Cerberus
// #64 F-4. Exercises the round-trip seal/unseal path with a real
// PGP keypair across Enqueue → List → Get + the queryable-without-
// unlock orchestration-field property.

package queue_test

import (
	"sync"

	core "dappco.re/go"
	"dappco.re/go/orm"

	"dappco.re/lthn/desktop/pkg/account"
	"dappco.re/lthn/desktop/pkg/queue"

	"forge.lthn.ai/Snider/Enchantrix/pkg/crypt/std/pgp"
)

// queueTestAccountSurface mirrors the tasks test stub — narrow
// live-read surface queue consumes from the wired account service.
type queueTestAccountSurface struct {
	unlocked []string
	pub      []byte
	priv     []byte
}

func (s *queueTestAccountSurface) UnlockedAccountIDs() []string { return s.unlocked }
func (s *queueTestAccountSurface) PublicKeyFor(id string) ([]byte, bool) {
	if id == "" || s.pub == nil {
		return nil, false
	}
	return s.pub, true
}
func (s *queueTestAccountSurface) PrivateKeyFor(id string) (*account.PrivateKeyHandle, bool) {
	if id == "" || s.priv == nil {
		return nil, false
	}
	return account.NewPrivateKeyHandleForTest(append([]byte(nil), s.priv...)), true
}

var (
	queueAtrestKeyOnce sync.Once
	queueAtrestKeyPub  []byte
	queueAtrestKeyPriv []byte
)

func queueAtrestKeyPair(t *core.T) (pub, priv []byte) {
	t.Helper()
	queueAtrestKeyOnce.Do(func() {
		svc := pgp.NewService()
		p, k, err := svc.GenerateKeyPair("Queue", "queue-test@lthn.local", "test")
		if err != nil {
			t.Fatalf("generate test key pair: %v", err)
		}
		queueAtrestKeyPub = p
		queueAtrestKeyPriv = k
	})
	return queueAtrestKeyPub, queueAtrestKeyPriv
}

func sealedQueueCoreFixture(t *core.T) *core.Core {
	t.Helper()
	pub, priv := queueAtrestKeyPair(t)
	c := core.New(
		core.WithName("account", func(c *core.Core) core.Result {
			return c.RegisterService("account", &queueTestAccountSurface{
				unlocked: []string{"acct-queue-sealed"},
				pub:      pub,
				priv:     priv,
			})
		}),
	)
	core.RequireTrue(t, orm.Register(c).OK)
	mem := orm.NewMemium()
	core.RequireTrue(t, orm.Mount(c, "default", mem).OK)
	for _, schema := range queue.Schemas() {
		core.RequireTrue(t, orm.RegisterSchema(c, schema).OK)
		mem.RegisterTable(schema.Name, schema)
	}
	core.RequireTrue(t, c.ServiceStartup(core.Background(), nil).OK)
	t.Cleanup(func() { _ = c.ServiceShutdown(core.Background()) })
	return c
}

// TestQueue_Payload_AtRest_Good — Enqueue seals the JSON Payload
// in the DuckDB column; Get + List unseal at the read boundary.
// The orchestration columns (kind, status, project) round-trip
// plaintext so Where()-filtered queries continue to work without
// unlock.
func TestQueue_Payload_AtRest_Good(t *core.T) {
	c := sealedQueueCoreFixture(t)
	r := queue.Enqueue(c, "lint", core.NewOptions(
		core.Option{Key: "path", Value: "/highly/confidential/prompt/path"},
		core.Option{Key: "owner", Value: "snider"},
	))
	core.RequireTrue(t, r.OK)
	job, ok := r.Value.(queue.Job)
	core.RequireTrue(t, ok)
	// The returned Job carries plaintext (broadcast shape preserved).
	core.AssertTrue(t, core.Contains(job.Payload, "/highly/confidential/prompt/path"),
		"returned Job.Payload should be plaintext for listeners")

	// Get round-trip — unseals the at-rest value.
	got := queue.Get(c, job.ID)
	core.RequireTrue(t, got.OK)
	gotJob, ok := got.Value.(queue.Job)
	core.RequireTrue(t, ok)
	core.AssertTrue(t, core.Contains(gotJob.Payload, "/highly/confidential/prompt/path"))

	// Raw orm read MUST see the sealed envelope (not plaintext).
	raw := orm.Of[queue.Job](c).Find(job.ID)
	core.RequireTrue(t, raw.OK)
	rawJob, _, ok := orm.Detail[queue.Job](raw)
	core.RequireTrue(t, ok)
	core.AssertTrue(t, core.HasPrefix(rawJob.Payload, `{"sealed":`),
		"raw orm read must see sealed envelope, got: "+rawJob.Payload)

	// Orchestration columns survive plain.
	core.AssertEqual(t, "lint", rawJob.Kind)
	core.AssertEqual(t, queue.StatusPending, rawJob.Status)
}

// TestQueue_List_OrchestrationFiltersStayWorking_Good — List with
// a Kind filter still hits the right rows when Payloads are sealed.
func TestQueue_List_OrchestrationFiltersStayWorking_Good(t *core.T) {
	c := sealedQueueCoreFixture(t)
	core.RequireTrue(t, queue.Enqueue(c, "lint", core.NewOptions(
		core.Option{Key: "path", Value: "/a"},
	)).OK)
	core.RequireTrue(t, queue.Enqueue(c, "fetch", core.NewOptions(
		core.Option{Key: "url", Value: "https://x"},
	)).OK)

	r := queue.List(c, queue.ListFilter{Kind: "lint"})
	core.RequireTrue(t, r.OK)
	jobs, ok := r.Value.([]queue.Job)
	core.RequireTrue(t, ok)
	core.AssertEqual(t, 1, len(jobs))
	core.AssertEqual(t, "lint", jobs[0].Kind)
	core.AssertTrue(t, core.Contains(jobs[0].Payload, "/a"))
}
