// SPDX-Licence-Identifier: EUPL-1.2

// Tests for the encrypted-at-rest keys service. AX-7 triplet per
// public symbol named Test<File>_<Receiver>_<Method>_<Variant> for
// methods, Test<File>_<Symbol>_<Variant> for free funcs. Each test
// reroutes $HOME so the real ~/Lethean/data/keys/ is never touched.

package keys_test

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/keys"
)

func homeFixture(t *core.T) *keys.Service {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	r := keys.New()
	core.AssertTrue(t, r.OK, "keys.New must succeed under temp HOME")
	return r.Value.(*keys.Service)
}

// --- New ---

func TestService_New_Good(t *core.T) {
	// Direct call (not via fixture) so the body references keys.New
	// for the audit's unreferenced-tests check.
	t.Setenv("HOME", t.TempDir())
	r := keys.New()
	core.AssertTrue(t, r.OK)
	svc := r.Value.(*keys.Service)
	core.AssertNotNil(t, svc)
	core.AssertEqual(t, "Keys", svc.ServiceName())
}

func TestService_New_Bad(t *core.T) {
	tmp := t.TempDir()
	blocker := core.PathJoin(tmp, "not-a-dir")
	core.AssertTrue(t, core.WriteFile(blocker, []byte("x"), 0o644).OK)
	t.Setenv("HOME", blocker)
	r := keys.New()
	core.AssertFalse(t, r.OK, "keys.New must Fail when HOME is a regular file")
}

func TestService_New_Ugly(t *core.T) {
	t.Setenv("HOME", t.TempDir())
	r1 := keys.New()
	core.AssertTrue(t, r1.OK)
	r2 := keys.New()
	core.AssertTrue(t, r2.OK)
	core.AssertNotNil(t, r2.Value)
}

// --- Register ---

func TestService_Register_Good(t *core.T) {
	t.Setenv("HOME", t.TempDir())
	c := core.New()
	r := keys.Register(c)
	core.AssertTrue(t, r.OK)
	got := c.Service("keys")
	core.AssertTrue(t, got.OK)
}

func TestService_Register_Bad(t *core.T) {
	tmp := t.TempDir()
	blocker := core.PathJoin(tmp, "not-a-dir")
	core.AssertTrue(t, core.WriteFile(blocker, []byte("x"), 0o644).OK)
	t.Setenv("HOME", blocker)
	c := core.New()
	core.AssertFalse(t, keys.Register(c).OK)
}

func TestService_Register_Ugly(t *core.T) {
	t.Setenv("HOME", t.TempDir())
	c := core.New()
	core.AssertTrue(t, keys.Register(c).OK)
	core.AssertNotPanics(t, func() { _ = keys.Register(c) })
}

// --- Put / Get (round-trip) ---

func TestService_Service_Put_Good(t *core.T) {
	svc := homeFixture(t)
	r := svc.Put("openai-default", []byte("sk-abc123"))
	core.AssertTrue(t, r.OK)
	// Round-trip: Get returns the same plaintext.
	got := svc.Get("openai-default")
	core.AssertTrue(t, got.OK)
	core.AssertEqual(t, "sk-abc123", string(got.Value.([]byte)))
}

func TestService_Service_Put_Bad(t *core.T) {
	svc := homeFixture(t)
	// Empty ref must Fail rather than panic / silently overwrite.
	core.AssertFalse(t, svc.Put("", []byte("x")).OK)
	// Path-separator ref must Fail (anti-traversal).
	core.AssertFalse(t, svc.Put("../escape", []byte("x")).OK)
	core.AssertFalse(t, svc.Put("nested/ref", []byte("x")).OK)
	// Dot-prefixed ref reserved for internal files (.master).
	core.AssertFalse(t, svc.Put(".sneak", []byte("x")).OK)
}

func TestService_Service_Put_Ugly(t *core.T) {
	svc := homeFixture(t)
	// Overwrite is silent — second Put replaces first.
	core.AssertTrue(t, svc.Put("x", []byte("first")).OK)
	core.AssertTrue(t, svc.Put("x", []byte("second")).OK)
	got := svc.Get("x")
	core.AssertTrue(t, got.OK)
	core.AssertEqual(t, "second", string(got.Value.([]byte)))
	// Empty plaintext is legal — represents a zero-byte secret.
	core.AssertTrue(t, svc.Put("y", []byte{}).OK)
	r := svc.Get("y")
	core.AssertTrue(t, r.OK)
	core.AssertEqual(t, 0, len(r.Value.([]byte)))
}

// --- Get ---

func TestService_Service_Get_Good(t *core.T) {
	svc := homeFixture(t)
	core.AssertTrue(t, svc.Put("k", []byte("secret")).OK)
	r := svc.Get("k")
	core.AssertTrue(t, r.OK)
	core.AssertEqual(t, "secret", string(r.Value.([]byte)))
}

func TestService_Service_Get_Bad(t *core.T) {
	svc := homeFixture(t)
	// Missing ref must Fail (no silent empty-string return).
	core.AssertFalse(t, svc.Get("never-stored").OK)
	core.AssertFalse(t, svc.Get("").OK)
	core.AssertFalse(t, svc.Get("../escape").OK)
}

func TestService_Service_Get_Ugly(t *core.T) {
	svc := homeFixture(t)
	// Repeated Get against the same ref must return consistent
	// plaintext — nonces are per-ciphertext, deterministic decrypt.
	core.AssertTrue(t, svc.Put("k", []byte("payload")).OK)
	first := svc.Get("k")
	core.AssertTrue(t, first.OK)
	second := svc.Get("k")
	core.AssertTrue(t, second.OK)
	core.AssertEqual(t, string(first.Value.([]byte)), string(second.Value.([]byte)))
}

// --- Delete ---

func TestService_Service_Delete_Good(t *core.T) {
	svc := homeFixture(t)
	core.AssertTrue(t, svc.Put("k", []byte("x")).OK)
	core.AssertTrue(t, svc.Delete("k").OK)
	core.AssertFalse(t, svc.Get("k").OK, "Get after Delete must Fail")
}

func TestService_Service_Delete_Bad(t *core.T) {
	svc := homeFixture(t)
	core.AssertFalse(t, svc.Delete("").OK)
	core.AssertFalse(t, svc.Delete("../escape").OK)
}

func TestService_Service_Delete_Ugly(t *core.T) {
	svc := homeFixture(t)
	// Delete-of-nonexistent is OK (idempotent).
	core.AssertTrue(t, svc.Delete("not-here").OK)
	core.AssertTrue(t, svc.Put("k", []byte("x")).OK)
	core.AssertTrue(t, svc.Delete("k").OK)
	core.AssertTrue(t, svc.Delete("k").OK, "second delete on same ref idempotent")
}

// --- List ---

func TestService_Service_List_Good(t *core.T) {
	svc := homeFixture(t)
	core.AssertTrue(t, svc.Put("alpha", []byte("a")).OK)
	core.AssertTrue(t, svc.Put("beta", []byte("b")).OK)
	r := svc.List()
	core.AssertTrue(t, r.OK)
	refs := r.Value.([]string)
	core.AssertEqual(t, 2, len(refs))
}

func TestService_Service_List_Bad(t *core.T) {
	svc := homeFixture(t)
	// Empty store → empty list, NOT Fail.
	r := svc.List()
	core.AssertTrue(t, r.OK)
	core.AssertEqual(t, 0, len(r.Value.([]string)))
}

func TestService_Service_List_Ugly(t *core.T) {
	svc := homeFixture(t)
	// .master must not appear in the list — only .aead-suffixed
	// files surface as refs. Force-create the master via a Put.
	core.AssertTrue(t, svc.Put("k", []byte("x")).OK)
	r := svc.List()
	core.AssertTrue(t, r.OK)
	refs := r.Value.([]string)
	core.AssertEqual(t, 1, len(refs), ".master must not surface as a ref")
	core.AssertEqual(t, "k", refs[0])
}

// --- Has ---

func TestService_Service_Has_Good(t *core.T) {
	svc := homeFixture(t)
	core.AssertTrue(t, svc.Put("k", []byte("x")).OK)
	r := svc.Has("k")
	core.AssertTrue(t, r.OK)
	core.AssertTrue(t, r.Value.(bool))
}

func TestService_Service_Has_Bad(t *core.T) {
	svc := homeFixture(t)
	// Empty / traversal refs must Fail rather than report false.
	core.AssertFalse(t, svc.Has("").OK)
	core.AssertFalse(t, svc.Has("../escape").OK)
}

func TestService_Service_Has_Ugly(t *core.T) {
	svc := homeFixture(t)
	// Missing ref → OK with false.
	r := svc.Has("missing")
	core.AssertTrue(t, r.OK)
	core.AssertFalse(t, r.Value.(bool))
}

// --- ServiceName / ServiceStartup / ServiceShutdown ---

func TestService_Service_ServiceName_Good(t *core.T) {
	svc := homeFixture(t)
	name := svc.ServiceName()
	core.AssertEqual(t, "Keys", name)
	core.AssertEqual(t, 4, len(name), "literal length stable")
}

func TestService_Service_ServiceName_Bad(t *core.T) {
	var svc *keys.Service
	core.AssertNotPanics(t, func() { _ = svc.ServiceName() })
	zero := &keys.Service{}
	core.AssertEqual(t, "Keys", zero.ServiceName())
}

func TestService_Service_ServiceName_Ugly(t *core.T) {
	svc := homeFixture(t)
	first := svc.ServiceName()
	second := svc.ServiceName()
	core.AssertEqual(t, first, second)
}

func TestService_Service_ServiceStartup_Good(t *core.T) {
	svc := homeFixture(t)
	r := svc.ServiceStartup(core.Background(), nil)
	core.AssertTrue(t, r.OK)
}

func TestService_Service_ServiceStartup_Bad(t *core.T) {
	svc := homeFixture(t)
	// any opts type accepted — startup is a no-op, never validates.
	core.AssertTrue(t, svc.ServiceStartup(core.Background(), struct{}{}).OK)
	core.AssertTrue(t, svc.ServiceStartup(core.Background(), "nonsense").OK)
	core.AssertTrue(t, svc.ServiceStartup(core.Background(), 42).OK)
}

func TestService_Service_ServiceStartup_Ugly(t *core.T) {
	svc := homeFixture(t)
	core.AssertTrue(t, svc.ServiceStartup(core.Background(), nil).OK)
	core.AssertTrue(t, svc.ServiceStartup(core.Background(), nil).OK, "repeated startup safe")
}

func TestService_Service_ServiceShutdown_Good(t *core.T) {
	svc := homeFixture(t)
	core.AssertTrue(t, svc.Put("k", []byte("x")).OK) // force master load
	r := svc.ServiceShutdown()
	core.AssertTrue(t, r.OK)
}

func TestService_Service_ServiceShutdown_Bad(t *core.T) {
	svc := homeFixture(t)
	// Shutdown without prior master load is still OK (lazy master).
	core.AssertTrue(t, svc.ServiceShutdown().OK)
	// And the next operation still works (lazy re-load).
	core.AssertTrue(t, svc.Put("k", []byte("x")).OK)
}

func TestService_Service_ServiceShutdown_Ugly(t *core.T) {
	svc := homeFixture(t)
	core.AssertTrue(t, svc.Put("k", []byte("x")).OK)
	core.AssertTrue(t, svc.ServiceShutdown().OK)
	core.AssertTrue(t, svc.ServiceShutdown().OK, "double-shutdown safe")
	// After shutdown, Get reloads the master (lazy) and works.
	r := svc.Get("k")
	core.AssertTrue(t, r.OK)
}

// --- W-prefixed Wails wrappers ---

func TestService_Service_WPut_Good(t *core.T) {
	svc := homeFixture(t)
	r := svc.WPut("openai-default", "sk-abc")
	core.AssertTrue(t, r.OK)
	got := svc.Get("openai-default")
	core.AssertTrue(t, got.OK)
	core.AssertEqual(t, "sk-abc", string(got.Value.([]byte)))
}

func TestService_Service_WPut_Bad(t *core.T) {
	svc := homeFixture(t)
	core.AssertFalse(t, svc.WPut("", "x").OK)
	core.AssertFalse(t, svc.WPut("../escape", "x").OK)
}

func TestService_Service_WPut_Ugly(t *core.T) {
	svc := homeFixture(t)
	core.AssertTrue(t, svc.WPut("k", "first").OK)
	core.AssertTrue(t, svc.WPut("k", "second").OK)
	got := svc.Get("k")
	core.AssertTrue(t, got.OK)
	core.AssertEqual(t, "second", string(got.Value.([]byte)))
}

func TestService_Service_WList_Good(t *core.T) {
	svc := homeFixture(t)
	core.AssertTrue(t, svc.WPut("k1", "x").OK)
	core.AssertTrue(t, svc.WPut("k2", "y").OK)
	r := svc.WList()
	core.AssertTrue(t, r.OK)
	core.AssertEqual(t, 2, len(r.Value.([]string)))
}

func TestService_Service_WList_Bad(t *core.T) {
	svc := homeFixture(t)
	r := svc.WList()
	core.AssertTrue(t, r.OK, "empty list is OK, not Fail")
	core.AssertEqual(t, 0, len(r.Value.([]string)))
}

func TestService_Service_WList_Ugly(t *core.T) {
	svc := homeFixture(t)
	core.AssertTrue(t, svc.WPut("k", "x").OK)
	first := svc.WList()
	second := svc.WList()
	core.AssertTrue(t, first.OK && second.OK)
	core.AssertEqual(t, len(first.Value.([]string)), len(second.Value.([]string)))
}

func TestService_Service_WHas_Good(t *core.T) {
	svc := homeFixture(t)
	core.AssertTrue(t, svc.WPut("k", "x").OK)
	r := svc.WHas("k")
	core.AssertTrue(t, r.OK && r.Value.(bool))
}

func TestService_Service_WHas_Bad(t *core.T) {
	svc := homeFixture(t)
	core.AssertFalse(t, svc.WHas("").OK, "empty ref must Fail")
	core.AssertFalse(t, svc.WHas("../escape").OK, "traversal ref must Fail")
	core.AssertFalse(t, svc.WHas(".master").OK, "dot-prefixed ref must Fail")
}

func TestService_Service_WHas_Ugly(t *core.T) {
	svc := homeFixture(t)
	r := svc.WHas("missing")
	core.AssertTrue(t, r.OK)
	core.AssertFalse(t, r.Value.(bool))
}

func TestService_Service_WDelete_Good(t *core.T) {
	svc := homeFixture(t)
	core.AssertTrue(t, svc.WPut("k", "x").OK)
	core.AssertTrue(t, svc.WDelete("k").OK)
	core.AssertFalse(t, svc.Get("k").OK)
}

func TestService_Service_WDelete_Bad(t *core.T) {
	svc := homeFixture(t)
	core.AssertFalse(t, svc.WDelete("").OK, "empty ref must Fail")
	core.AssertFalse(t, svc.WDelete("../escape").OK, "traversal ref must Fail")
}

func TestService_Service_WDelete_Ugly(t *core.T) {
	svc := homeFixture(t)
	core.AssertTrue(t, svc.WDelete("not-here").OK, "delete-of-nonexistent idempotent")
	core.AssertTrue(t, svc.WPut("k", "x").OK)
	core.AssertTrue(t, svc.WDelete("k").OK)
	core.AssertTrue(t, svc.WDelete("k").OK)
}
