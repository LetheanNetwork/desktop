// SPDX-Licence-Identifier: EUPL-1.2

// Tests for the wiring that survives the sealed-key service's
// promotion into go-crypt. Two things have to hold: the event schema
// the two packages now declare separately must stay identical, and a
// credential mutation must still reach ~/Lethean/audit/ with the row it
// always carried. AX-7 triplet naming.

package keysvc_test

import (
	"sync"

	core "dappco.re/go"
	"dappco.re/go/crypt/keys"

	"dappco.re/lthn/desktop/pkg/audit"
	"dappco.re/lthn/desktop/pkg/keysvc"
)

// recordingRecorder captures the audit rows the process recorder
// receives, so a test can assert on what would have been written.
type recordingRecorder struct {
	mu     sync.Mutex
	events []audit.Event
}

func (r *recordingRecorder) Record(ev audit.Event) core.Result {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, ev)
	return core.Ok(nil)
}

func (r *recordingRecorder) find(name string) *audit.Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := len(r.events) - 1; i >= 0; i-- {
		if r.events[i].Event == name {
			return &r.events[i]
		}
	}
	return nil
}

func (r *recordingRecorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.events)
}

// auditedFixture reroutes $HOME, installs a recording process
// recorder, and returns a tier-1-capable service built through the
// application wiring.
func auditedFixture(t *core.T) (*keys.Service, *recordingRecorder) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())

	rec := &recordingRecorder{}
	audit.SetDefault(rec)
	t.Cleanup(func() { audit.SetDefault(nil) })

	r := keysvc.New()
	core.AssertTrue(t, r.OK, "keysvc.New must succeed under temp HOME")
	svc := r.Value.(*keys.Service)
	svc.SetKEKProviderTier0(func() ([]byte, bool) { return make([]byte, 32), true })
	svc.SetKEKProvider(func() ([]byte, bool) { return make([]byte, 32), true })
	return svc, rec
}

// --- Event-schema parity ---

// TestKeysvc_AuditSchema_Parity_Good pins the schema the two packages
// now declare separately. keys/ emits its own literals; pkg/audit keeps
// its constants because the Operations panel and the TypeScript mirror
// decode them. Two declarations of one wire contract only stay honest
// if something fails when they diverge.
func TestKeysvc_AuditSchema_Parity_Good(t *core.T) {
	core.AssertEqual(t, audit.EventKeysTier0Stored, keys.AuditEventTier0Stored)
	core.AssertEqual(t, audit.EventKeysTier0Deleted, keys.AuditEventTier0Deleted)
	core.AssertEqual(t, audit.EventKeysTier1Stored, keys.AuditEventTier1Stored)
	core.AssertEqual(t, audit.EventKeysTier1Replaced, keys.AuditEventTier1Replaced)
	core.AssertEqual(t, audit.EventKeysTier1Deleted, keys.AuditEventTier1Deleted)

	core.AssertEqual(t, audit.OutcomeOK, keys.AuditOutcomeOK)
	core.AssertEqual(t, audit.OutcomeError, keys.AuditOutcomeError)
	core.AssertEqual(t, "keys", keys.AuditScopeKeys)
}

// --- Audit forwarding ---

// TestKeysvc_AuditAdapter_Record_Good drives a real credential
// mutation through the application wiring and asserts the row the
// process recorder receives is the one it received before the
// promotion: same event name, scope, outcome and Meta shape.
func TestKeysvc_AuditAdapter_Record_Good(t *core.T) {
	svc, rec := auditedFixture(t)

	core.AssertTrue(t, svc.PutTier1("openai-default", []byte("sk-do-not-leak")).OK)

	stored := rec.find(audit.EventKeysTier1Stored)
	core.AssertTrue(t, stored != nil,
		"a tier-1 write must still reach the application audit recorder")
	core.AssertEqual(t, audit.OutcomeOK, stored.Outcome)
	core.AssertEqual(t, "keys", stored.Scope)
	core.AssertEqual(t, "openai-default", stored.Meta["ref"])
	core.AssertEqual(t, "tier1", stored.Meta["kind"])
	core.AssertEqual(t, "tier1", stored.Meta["tier"])
	core.AssertEqual(t, "internal", stored.Meta["source"])
	core.AssertTrue(t, stored.TS > 0, "the row must carry an emit timestamp")

	// The Wails-reachable surface keeps its own source discriminator.
	core.AssertTrue(t, svc.WPutTier1("anthropic-default", "sk-wails").OK)
	wails := rec.find(audit.EventKeysTier1Stored)
	core.AssertEqual(t, "wails_input", wails.Meta["source"])

	// Tier-0 is audited too — a pre-unlock write is exactly the one
	// nobody is watching, so its row is the one worth proving.
	core.AssertTrue(t, svc.PutTier0("single-instance", []byte("machine-secret")).OK)
	t0 := rec.find(audit.EventKeysTier0Stored)
	core.AssertTrue(t, t0 != nil, "a tier-0 write must still be audited")
	core.AssertEqual(t, "tier0", t0.Meta["tier"])
}

// TestKeysvc_AuditAdapter_Record_Bad asserts the failure branch still
// records — a rejected ref emits Outcome=error carrying the audit
// package's own error code, not a silent drop.
func TestKeysvc_AuditAdapter_Record_Bad(t *core.T) {
	svc, rec := auditedFixture(t)

	r := svc.DeleteTier1("")
	core.AssertFalse(t, r.OK, "an empty ref must be rejected")

	deleted := rec.find(audit.EventKeysTier1Deleted)
	core.AssertTrue(t, deleted != nil, "a rejected delete must still emit a row")
	core.AssertEqual(t, audit.OutcomeError, deleted.Outcome)
	core.AssertEqual(t, audit.ErrorCode(r), deleted.Meta["error_code"],
		"the row must carry the code pkg/audit derives, not a second opinion")
}

// TestKeysvc_AuditAdapter_Record_Ugly asserts the credential path is
// never blocked by the audit substrate, and that plaintext never
// reaches it.
func TestKeysvc_AuditAdapter_Record_Ugly(t *core.T) {
	svc, rec := auditedFixture(t)

	const sentinel = "sk-canary-must-not-persist"
	core.AssertTrue(t, svc.PutTier1("openai-default", []byte(sentinel)).OK)
	core.AssertTrue(t, svc.DeleteTier1("openai-default").OK)
	core.AssertTrue(t, rec.count() > 0, "rows must have been recorded")

	rec.mu.Lock()
	events := make([]audit.Event, len(rec.events))
	copy(events, rec.events)
	rec.mu.Unlock()

	for i := range events {
		for k, v := range events[i].Meta {
			vs, ok := v.(string)
			if !ok {
				continue
			}
			core.AssertFalse(t, core.Contains(vs, sentinel),
				"event["+events[i].Event+"].Meta["+k+"] must not echo plaintext")
		}
	}
}

// --- Path wiring ---

// TestKeysvc_Options_Dir_Good asserts the injected resolver still puts
// blobs where the application has always kept them.
func TestKeysvc_Options_Dir_Good(t *core.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	r := keysvc.New()
	core.AssertTrue(t, r.OK)
	svc := r.Value.(*keys.Service)
	svc.SetKEKProviderTier0(func() ([]byte, bool) { return make([]byte, 32), true })
	core.AssertTrue(t, svc.PutTier0("single-instance", []byte("machine-secret")).OK)

	blob := core.PathJoin(home, "Lethean", "data", "keys", "single-instance.t0.aead")
	core.AssertTrue(t, core.Stat(blob).OK,
		"tier-0 ciphertext must still land under ~/Lethean/data/keys")
}

// TestKeysvc_Register_Good asserts the registrar is discoverable on a
// Core container under the canonical "keys" name.
func TestKeysvc_Register_Good(t *core.T) {
	t.Setenv("HOME", t.TempDir())
	c := core.New(core.WithName("keys", keysvc.Register))
	core.AssertTrue(t, c.Service("keys").OK,
		"keys service discoverable after WithName")

	svc, _ := core.ServiceFor[*keys.Service](c, "keys")
	core.AssertNotNil(t, svc)
}

// TestKeysvc_Register_Bad asserts a construction failure propagates
// rather than registering a half-built service.
func TestKeysvc_Register_Bad(t *core.T) {
	blocker := core.PathJoin(t.TempDir(), "not-a-dir")
	core.AssertTrue(t, core.WriteFile(blocker, []byte("x"), 0o644).OK)
	t.Setenv("HOME", blocker)

	c := core.New()
	core.AssertFalse(t, keysvc.Register(c).OK,
		"Register must Fail when the data root cannot be created")
}

// TestKeysvc_Register_Ugly asserts repeat registration is safe.
func TestKeysvc_Register_Ugly(t *core.T) {
	t.Setenv("HOME", t.TempDir())
	c := core.New()
	core.AssertTrue(t, keysvc.Register(c).OK)
	core.AssertNotPanics(t, func() { _ = keysvc.Register(c) })
}
