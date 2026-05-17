// SPDX-Licence-Identifier: EUPL-1.2

// Tests for the encrypted-at-rest keys service after the tier
// partition (RFC.stage-e-keys-partition v3 — Mantis #1625). AX-7
// triplet per public symbol named Test<File>_<Receiver>_<Method>_<Variant>
// for methods, Test<File>_<Symbol>_<Variant> for free funcs.
// Each test reroutes $HOME so the real ~/Lethean/data/keys/ is
// never touched.

package keys_test

import (
	"sync"

	core "dappco.re/go"
	"dappco.re/go/io/sigil"
	"dappco.re/lthn/desktop/pkg/audit"
	"dappco.re/lthn/desktop/pkg/keys"
)

// fixedKEK is a deterministic 32-byte KEK used by the per-tier
// fixtures. Production KEKs derive via HKDF; tests use a constant
// so the on-disk wrapped masters are reproducible across runs.
var fixedTier0KEK = bytesOf(0x42)
var fixedTier1KEK = bytesOf(0x77)

func bytesOf(b byte) []byte {
	out := make([]byte, 32)
	for i := range out {
		out[i] = b
	}
	return out
}

// tier0Fixture constructs a keys.Service under a temp HOME with
// the tier-0 KEK provider wired (deterministic 0x42-fill key).
// Use when the test exercises tier-0 ops or SingleInstanceKey.
func tier0Fixture(t *core.T) *keys.Service {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	r := keys.New()
	core.AssertTrue(t, r.OK, "keys.New must succeed under temp HOME")
	svc := r.Value.(*keys.Service)
	svc.SetKEKProviderTier0(func() ([]byte, bool) { return fixedTier0KEK, true })
	return svc
}

// tier1Fixture extends tier0Fixture by also wiring the tier-1 KEK
// provider — the common shape for tier-1 ops and round-trip tests.
func tier1Fixture(t *core.T) *keys.Service {
	t.Helper()
	svc := tier0Fixture(t)
	svc.SetKEKProvider(func() ([]byte, bool) { return fixedTier1KEK, true })
	return svc
}

// homeFixture is a bare Service over a temp HOME — no KEK
// providers wired. Use only for tests that probe the
// no-provider-wired failure path.
func homeFixture(t *core.T) *keys.Service {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	r := keys.New()
	core.AssertTrue(t, r.OK, "keys.New must succeed under temp HOME")
	return r.Value.(*keys.Service)
}

// --- New ---

func TestService_New_Good(t *core.T) {
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
	c := core.New(core.WithName("keys", keys.Register))
	got := c.Service("keys")
	core.AssertTrue(t, got.OK, "keys service discoverable after WithName")
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

// --- PutTier0 / GetTier0 round-trip ---

func TestService_Service_PutTier0_Good(t *core.T) {
	svc := tier0Fixture(t)
	r := svc.PutTier0("single-instance", []byte("ipc-key"))
	core.AssertTrue(t, r.OK)
	got := svc.GetTier0("single-instance")
	core.AssertTrue(t, got.OK)
	core.AssertEqual(t, "ipc-key", string(got.Value.([]byte)))
}

func TestService_Service_PutTier0_Bad(t *core.T) {
	svc := tier0Fixture(t)
	core.AssertFalse(t, svc.PutTier0("", []byte("x")).OK)
	core.AssertFalse(t, svc.PutTier0("../escape", []byte("x")).OK)
	core.AssertFalse(t, svc.PutTier0("nested/ref", []byte("x")).OK)
	core.AssertFalse(t, svc.PutTier0(".sneak", []byte("x")).OK)
}

func TestService_Service_PutTier0_Ugly(t *core.T) {
	svc := tier0Fixture(t)
	// Without tier-0 KEK provider wired, PutTier0 Fails clearly.
	bare := homeFixture(t)
	core.AssertFalse(t, bare.PutTier0("x", []byte("x")).OK)
	// Overwrite is silent — second Put replaces first.
	core.AssertTrue(t, svc.PutTier0("x", []byte("first")).OK)
	core.AssertTrue(t, svc.PutTier0("x", []byte("second")).OK)
	got := svc.GetTier0("x")
	core.AssertTrue(t, got.OK)
	core.AssertEqual(t, "second", string(got.Value.([]byte)))
}

// --- PutTier1 / GetTier1 round-trip ---

func TestService_Service_PutTier1_Good(t *core.T) {
	svc := tier1Fixture(t)
	r := svc.PutTier1("openai-default", []byte("sk-abc123"))
	core.AssertTrue(t, r.OK)
	got := svc.GetTier1("openai-default")
	core.AssertTrue(t, got.OK)
	core.AssertEqual(t, "sk-abc123", string(got.Value.([]byte)))
}

func TestService_Service_PutTier1_Bad(t *core.T) {
	svc := tier1Fixture(t)
	core.AssertFalse(t, svc.PutTier1("", []byte("x")).OK)
	core.AssertFalse(t, svc.PutTier1("../escape", []byte("x")).OK)
	core.AssertFalse(t, svc.PutTier1("nested/ref", []byte("x")).OK)
	core.AssertFalse(t, svc.PutTier1(".sneak", []byte("x")).OK)
}

func TestService_Service_PutTier1_Ugly(t *core.T) {
	svc := tier1Fixture(t)
	// Without tier-1 KEK provider wired, PutTier1 Fails clearly.
	bare := tier0Fixture(t)
	core.AssertFalse(t, bare.PutTier1("x", []byte("x")).OK)
	// Empty plaintext is legal.
	core.AssertTrue(t, svc.PutTier1("y", []byte{}).OK)
	r := svc.GetTier1("y")
	core.AssertTrue(t, r.OK)
	core.AssertEqual(t, 0, len(r.Value.([]byte)))
}

// --- GetTier0 ---

func TestService_Service_GetTier0_Good(t *core.T) {
	svc := tier0Fixture(t)
	core.AssertTrue(t, svc.PutTier0("k", []byte("secret")).OK)
	r := svc.GetTier0("k")
	core.AssertTrue(t, r.OK)
	core.AssertEqual(t, "secret", string(r.Value.([]byte)))
}

func TestService_Service_GetTier0_Bad(t *core.T) {
	svc := tier0Fixture(t)
	core.AssertFalse(t, svc.GetTier0("never-stored").OK)
	core.AssertFalse(t, svc.GetTier0("").OK)
	core.AssertFalse(t, svc.GetTier0("../escape").OK)
}

func TestService_Service_GetTier0_Ugly(t *core.T) {
	svc := tier0Fixture(t)
	core.AssertTrue(t, svc.PutTier0("k", []byte("payload")).OK)
	first := svc.GetTier0("k")
	core.AssertTrue(t, first.OK)
	second := svc.GetTier0("k")
	core.AssertTrue(t, second.OK)
	core.AssertEqual(t, string(first.Value.([]byte)), string(second.Value.([]byte)))
}

// --- GetTier1 ---

func TestService_Service_GetTier1_Good(t *core.T) {
	svc := tier1Fixture(t)
	core.AssertTrue(t, svc.PutTier1("k", []byte("secret")).OK)
	r := svc.GetTier1("k")
	core.AssertTrue(t, r.OK)
	core.AssertEqual(t, "secret", string(r.Value.([]byte)))
}

func TestService_Service_GetTier1_Bad(t *core.T) {
	svc := tier1Fixture(t)
	core.AssertFalse(t, svc.GetTier1("never-stored").OK)
	core.AssertFalse(t, svc.GetTier1("").OK)
	core.AssertFalse(t, svc.GetTier1("../escape").OK)
}

func TestService_Service_GetTier1_Ugly(t *core.T) {
	svc := tier1Fixture(t)
	core.AssertTrue(t, svc.PutTier1("k", []byte("payload")).OK)
	first := svc.GetTier1("k")
	core.AssertTrue(t, first.OK)
	second := svc.GetTier1("k")
	core.AssertTrue(t, second.OK)
	core.AssertEqual(t, string(first.Value.([]byte)), string(second.Value.([]byte)))
}

// --- DeleteTier0 / DeleteTier1 ---

func TestService_Service_DeleteTier0_Good(t *core.T) {
	svc := tier0Fixture(t)
	core.AssertTrue(t, svc.PutTier0("k", []byte("x")).OK)
	core.AssertTrue(t, svc.DeleteTier0("k").OK)
	core.AssertFalse(t, svc.GetTier0("k").OK, "Get after Delete must Fail")
}

func TestService_Service_DeleteTier0_Bad(t *core.T) {
	svc := tier0Fixture(t)
	core.AssertFalse(t, svc.DeleteTier0("").OK)
	core.AssertFalse(t, svc.DeleteTier0("../escape").OK)
}

func TestService_Service_DeleteTier0_Ugly(t *core.T) {
	svc := tier0Fixture(t)
	core.AssertTrue(t, svc.DeleteTier0("not-here").OK)
	core.AssertTrue(t, svc.PutTier0("k", []byte("x")).OK)
	core.AssertTrue(t, svc.DeleteTier0("k").OK)
	core.AssertTrue(t, svc.DeleteTier0("k").OK, "second delete on same ref idempotent")
}

func TestService_Service_DeleteTier1_Good(t *core.T) {
	svc := tier1Fixture(t)
	core.AssertTrue(t, svc.PutTier1("k", []byte("x")).OK)
	core.AssertTrue(t, svc.DeleteTier1("k").OK)
	core.AssertFalse(t, svc.GetTier1("k").OK)
}

func TestService_Service_DeleteTier1_Bad(t *core.T) {
	svc := tier1Fixture(t)
	core.AssertFalse(t, svc.DeleteTier1("").OK)
	core.AssertFalse(t, svc.DeleteTier1("../escape").OK)
}

func TestService_Service_DeleteTier1_Ugly(t *core.T) {
	svc := tier1Fixture(t)
	core.AssertTrue(t, svc.DeleteTier1("not-here").OK)
	core.AssertTrue(t, svc.PutTier1("k", []byte("x")).OK)
	core.AssertTrue(t, svc.DeleteTier1("k").OK)
	core.AssertTrue(t, svc.DeleteTier1("k").OK)
}

// --- ListTier0 / ListTier1 ---

func TestService_Service_ListTier0_Good(t *core.T) {
	svc := tier0Fixture(t)
	core.AssertTrue(t, svc.PutTier0("alpha", []byte("a")).OK)
	core.AssertTrue(t, svc.PutTier0("beta", []byte("b")).OK)
	r := svc.ListTier0()
	core.AssertTrue(t, r.OK)
	refs := r.Value.([]string)
	core.AssertEqual(t, 2, len(refs))
}

func TestService_Service_ListTier0_Bad(t *core.T) {
	svc := tier0Fixture(t)
	r := svc.ListTier0()
	core.AssertTrue(t, r.OK)
	core.AssertEqual(t, 0, len(r.Value.([]string)))
}

func TestService_Service_ListTier0_Ugly(t *core.T) {
	// Tier-0 list must not surface .master / .master-tier0 /
	// .master-tier1 / tier-1 blobs in the same dir.
	svc := tier1Fixture(t)
	core.AssertTrue(t, svc.PutTier0("k0", []byte("x")).OK)
	core.AssertTrue(t, svc.PutTier1("k1", []byte("y")).OK)
	r := svc.ListTier0()
	core.AssertTrue(t, r.OK)
	refs := r.Value.([]string)
	core.AssertEqual(t, 1, len(refs), "ListTier0 must not surface tier-1 blobs")
	core.AssertEqual(t, "k0", refs[0])
}

func TestService_Service_ListTier1_Good(t *core.T) {
	svc := tier1Fixture(t)
	core.AssertTrue(t, svc.PutTier1("alpha", []byte("a")).OK)
	core.AssertTrue(t, svc.PutTier1("beta", []byte("b")).OK)
	r := svc.ListTier1()
	core.AssertTrue(t, r.OK)
	refs := r.Value.([]string)
	core.AssertEqual(t, 2, len(refs))
}

func TestService_Service_ListTier1_Bad(t *core.T) {
	svc := tier1Fixture(t)
	r := svc.ListTier1()
	core.AssertTrue(t, r.OK)
	core.AssertEqual(t, 0, len(r.Value.([]string)))
}

func TestService_Service_ListTier1_Ugly(t *core.T) {
	svc := tier1Fixture(t)
	core.AssertTrue(t, svc.PutTier0("k0", []byte("x")).OK)
	core.AssertTrue(t, svc.PutTier1("k1", []byte("y")).OK)
	r := svc.ListTier1()
	core.AssertTrue(t, r.OK)
	refs := r.Value.([]string)
	core.AssertEqual(t, 1, len(refs), "ListTier1 must not surface tier-0 blobs")
	core.AssertEqual(t, "k1", refs[0])
}

// --- HasTier0 / HasTier1 ---

func TestService_Service_HasTier0_Good(t *core.T) {
	svc := tier0Fixture(t)
	core.AssertTrue(t, svc.PutTier0("k", []byte("x")).OK)
	r := svc.HasTier0("k")
	core.AssertTrue(t, r.OK)
	core.AssertTrue(t, r.Value.(bool))
}

func TestService_Service_HasTier0_Bad(t *core.T) {
	svc := tier0Fixture(t)
	core.AssertFalse(t, svc.HasTier0("").OK)
	core.AssertFalse(t, svc.HasTier0("../escape").OK)
}

func TestService_Service_HasTier0_Ugly(t *core.T) {
	svc := tier0Fixture(t)
	r := svc.HasTier0("missing")
	core.AssertTrue(t, r.OK)
	core.AssertFalse(t, r.Value.(bool))
}

func TestService_Service_HasTier1_Good(t *core.T) {
	svc := tier1Fixture(t)
	core.AssertTrue(t, svc.PutTier1("k", []byte("x")).OK)
	r := svc.HasTier1("k")
	core.AssertTrue(t, r.OK)
	core.AssertTrue(t, r.Value.(bool))
}

func TestService_Service_HasTier1_Bad(t *core.T) {
	svc := tier1Fixture(t)
	core.AssertFalse(t, svc.HasTier1("").OK)
	core.AssertFalse(t, svc.HasTier1("../escape").OK)
}

func TestService_Service_HasTier1_Ugly(t *core.T) {
	svc := tier1Fixture(t)
	r := svc.HasTier1("missing")
	core.AssertTrue(t, r.OK)
	core.AssertFalse(t, r.Value.(bool))
}

// --- GetOrCreateTier0 / GetOrCreateTier1 ---

func TestService_Service_GetOrCreateTier0_Good(t *core.T) {
	svc := tier0Fixture(t)
	calls := 0
	r := svc.GetOrCreateTier0("new-key", func() ([]byte, error) {
		calls++
		return []byte("generated-secret"), nil
	})
	core.AssertTrue(t, r.OK)
	core.AssertEqual(t, "generated-secret", string(r.Value.([]byte)))
	core.AssertEqual(t, 1, calls)
	r2 := svc.GetOrCreateTier0("new-key", func() ([]byte, error) {
		calls++
		return []byte("should-not-appear"), nil
	})
	core.AssertTrue(t, r2.OK)
	core.AssertEqual(t, "generated-secret", string(r2.Value.([]byte)))
	core.AssertEqual(t, 1, calls, "generate must not fire when key already persisted")
}

func TestService_Service_GetOrCreateTier0_Bad(t *core.T) {
	svc := tier0Fixture(t)
	r := svc.GetOrCreateTier0("failing-key", func() ([]byte, error) {
		return nil, core.NewError("deliberate generate failure")
	})
	core.AssertFalse(t, r.OK)
	r2 := svc.GetOrCreateTier0("", func() ([]byte, error) {
		return []byte("x"), nil
	})
	core.AssertFalse(t, r2.OK)
}

func TestService_Service_GetOrCreateTier0_Ugly(t *core.T) {
	svc := tier0Fixture(t)
	core.AssertTrue(t, svc.PutTier0("pre-existing", []byte("original")).OK)
	calls := 0
	r := svc.GetOrCreateTier0("pre-existing", func() ([]byte, error) {
		calls++
		return []byte("override"), nil
	})
	core.AssertTrue(t, r.OK)
	core.AssertEqual(t, "original", string(r.Value.([]byte)))
	core.AssertEqual(t, 0, calls)
}

func TestService_Service_GetOrCreateTier1_Good(t *core.T) {
	svc := tier1Fixture(t)
	calls := 0
	r := svc.GetOrCreateTier1("new-key", func() ([]byte, error) {
		calls++
		return []byte("generated"), nil
	})
	core.AssertTrue(t, r.OK)
	core.AssertEqual(t, "generated", string(r.Value.([]byte)))
	core.AssertEqual(t, 1, calls)
	r2 := svc.GetOrCreateTier1("new-key", func() ([]byte, error) {
		calls++
		return []byte("should-not-appear"), nil
	})
	core.AssertTrue(t, r2.OK)
	core.AssertEqual(t, "generated", string(r2.Value.([]byte)))
	core.AssertEqual(t, 1, calls)
}

func TestService_Service_GetOrCreateTier1_Bad(t *core.T) {
	svc := tier1Fixture(t)
	r := svc.GetOrCreateTier1("failing-key", func() ([]byte, error) {
		return nil, core.NewError("deliberate generate failure")
	})
	core.AssertFalse(t, r.OK)
	r2 := svc.GetOrCreateTier1("", func() ([]byte, error) {
		return []byte("x"), nil
	})
	core.AssertFalse(t, r2.OK)
}

func TestService_Service_GetOrCreateTier1_Ugly(t *core.T) {
	svc := tier1Fixture(t)
	core.AssertTrue(t, svc.PutTier1("pre-existing", []byte("original")).OK)
	calls := 0
	r := svc.GetOrCreateTier1("pre-existing", func() ([]byte, error) {
		calls++
		return []byte("override"), nil
	})
	core.AssertTrue(t, r.OK)
	core.AssertEqual(t, "original", string(r.Value.([]byte)))
	core.AssertEqual(t, 0, calls)
}

// --- ServiceName / ServiceStartup / ServiceShutdown ---

func TestService_Service_ServiceName_Good(t *core.T) {
	svc := tier0Fixture(t)
	core.AssertEqual(t, "Keys", svc.ServiceName())
}

func TestService_Service_ServiceName_Bad(t *core.T) {
	var svc *keys.Service
	core.AssertNotPanics(t, func() { _ = svc.ServiceName() })
	zero := &keys.Service{}
	core.AssertEqual(t, "Keys", zero.ServiceName())
}

func TestService_Service_ServiceName_Ugly(t *core.T) {
	svc := tier0Fixture(t)
	first := svc.ServiceName()
	second := svc.ServiceName()
	core.AssertEqual(t, first, second)
}

func TestService_Service_ServiceStartup_Good(t *core.T) {
	svc := tier0Fixture(t)
	r := svc.ServiceStartup(core.Background(), nil)
	core.AssertTrue(t, r.OK)
}

func TestService_Service_ServiceStartup_Bad(t *core.T) {
	svc := tier0Fixture(t)
	core.AssertTrue(t, svc.ServiceStartup(core.Background(), struct{}{}).OK)
	core.AssertTrue(t, svc.ServiceStartup(core.Background(), "nonsense").OK)
	core.AssertTrue(t, svc.ServiceStartup(core.Background(), 42).OK)
}

func TestService_Service_ServiceStartup_Ugly(t *core.T) {
	svc := tier0Fixture(t)
	core.AssertTrue(t, svc.ServiceStartup(core.Background(), nil).OK)
	core.AssertTrue(t, svc.ServiceStartup(core.Background(), nil).OK)
}

func TestService_Service_ServiceShutdown_Good(t *core.T) {
	svc := tier1Fixture(t)
	core.AssertTrue(t, svc.PutTier1("k", []byte("x")).OK)
	r := svc.ServiceShutdown()
	core.AssertTrue(t, r.OK)
}

func TestService_Service_ServiceShutdown_Bad(t *core.T) {
	svc := tier1Fixture(t)
	core.AssertTrue(t, svc.ServiceShutdown().OK)
	core.AssertTrue(t, svc.PutTier1("k", []byte("x")).OK, "ops re-load masters after shutdown")
}

func TestService_Service_ServiceShutdown_Ugly(t *core.T) {
	svc := tier1Fixture(t)
	core.AssertTrue(t, svc.PutTier1("k", []byte("x")).OK)
	core.AssertTrue(t, svc.ServiceShutdown().OK)
	core.AssertTrue(t, svc.ServiceShutdown().OK)
	r := svc.GetTier1("k")
	core.AssertTrue(t, r.OK)
}

// --- WPutTier1 / WListTier1 / WHasTier1 / WDeleteTier1 ---

func TestService_Service_WPutTier1_Good(t *core.T) {
	svc := tier1Fixture(t)
	r := svc.WPutTier1("openai-default", "sk-abc")
	core.AssertTrue(t, r.OK)
	got := svc.GetTier1("openai-default")
	core.AssertTrue(t, got.OK)
	core.AssertEqual(t, "sk-abc", string(got.Value.([]byte)))
}

func TestService_Service_WPutTier1_Bad(t *core.T) {
	svc := tier1Fixture(t)
	core.AssertFalse(t, svc.WPutTier1("", "x").OK)
	core.AssertFalse(t, svc.WPutTier1("../escape", "x").OK)
}

func TestService_Service_WPutTier1_Ugly(t *core.T) {
	svc := tier1Fixture(t)
	core.AssertTrue(t, svc.WPutTier1("k", "first").OK)
	core.AssertTrue(t, svc.WPutTier1("k", "second").OK)
	got := svc.GetTier1("k")
	core.AssertTrue(t, got.OK)
	core.AssertEqual(t, "second", string(got.Value.([]byte)))
}

func TestService_Service_WListTier1_Good(t *core.T) {
	svc := tier1Fixture(t)
	core.AssertTrue(t, svc.WPutTier1("k1", "x").OK)
	core.AssertTrue(t, svc.WPutTier1("k2", "y").OK)
	r := svc.WListTier1()
	core.AssertTrue(t, r.OK)
	core.AssertEqual(t, 2, len(r.Value.([]string)))
}

func TestService_Service_WListTier1_Bad(t *core.T) {
	svc := tier1Fixture(t)
	r := svc.WListTier1()
	core.AssertTrue(t, r.OK)
	core.AssertEqual(t, 0, len(r.Value.([]string)))
}

func TestService_Service_WListTier1_Ugly(t *core.T) {
	svc := tier1Fixture(t)
	core.AssertTrue(t, svc.WPutTier1("k", "x").OK)
	first := svc.WListTier1()
	second := svc.WListTier1()
	core.AssertTrue(t, first.OK && second.OK)
	core.AssertEqual(t, len(first.Value.([]string)), len(second.Value.([]string)))
}

func TestService_Service_WHasTier1_Good(t *core.T) {
	svc := tier1Fixture(t)
	core.AssertTrue(t, svc.WPutTier1("k", "x").OK)
	r := svc.WHasTier1("k")
	core.AssertTrue(t, r.OK && r.Value.(bool))
}

func TestService_Service_WHasTier1_Bad(t *core.T) {
	svc := tier1Fixture(t)
	core.AssertFalse(t, svc.WHasTier1("").OK)
	core.AssertFalse(t, svc.WHasTier1("../escape").OK)
	core.AssertFalse(t, svc.WHasTier1(".master").OK)
}

func TestService_Service_WHasTier1_Ugly(t *core.T) {
	svc := tier1Fixture(t)
	r := svc.WHasTier1("missing")
	core.AssertTrue(t, r.OK)
	core.AssertFalse(t, r.Value.(bool))
}

func TestService_Service_WDeleteTier1_Good(t *core.T) {
	svc := tier1Fixture(t)
	core.AssertTrue(t, svc.WPutTier1("k", "x").OK)
	core.AssertTrue(t, svc.WDeleteTier1("k").OK)
	core.AssertFalse(t, svc.GetTier1("k").OK)
}

func TestService_Service_WDeleteTier1_Bad(t *core.T) {
	svc := tier1Fixture(t)
	core.AssertFalse(t, svc.WDeleteTier1("").OK)
	core.AssertFalse(t, svc.WDeleteTier1("../escape").OK)
}

func TestService_Service_WDeleteTier1_Ugly(t *core.T) {
	svc := tier1Fixture(t)
	core.AssertTrue(t, svc.WDeleteTier1("not-here").OK)
	core.AssertTrue(t, svc.WPutTier1("k", "x").OK)
	core.AssertTrue(t, svc.WDeleteTier1("k").OK)
	core.AssertTrue(t, svc.WDeleteTier1("k").OK)
}

// --- SingleInstanceKey (tier-0) ---

func TestService_Service_SingleInstanceKey_Good(t *core.T) {
	svc := tier0Fixture(t)
	r := svc.SingleInstanceKey()
	core.AssertTrue(t, r.OK)
	key, ok := r.Value.([32]byte)
	core.AssertTrue(t, ok)
	var zero [32]byte
	core.AssertNotEqual(t, zero, key)
}

func TestService_Service_SingleInstanceKey_Bad(t *core.T) {
	svc := tier0Fixture(t)
	// Pre-seed a wrong-length blob in tier-0 to force the
	// length-check failure inside SingleInstanceKey.
	core.AssertTrue(t, svc.PutTier0("single-instance", []byte("tooshort")).OK)
	r := svc.SingleInstanceKey()
	core.AssertFalse(t, r.OK)
}

func TestService_Service_SingleInstanceKey_Ugly(t *core.T) {
	svc := tier0Fixture(t)
	r1 := svc.SingleInstanceKey()
	core.AssertTrue(t, r1.OK)
	key1 := r1.Value.([32]byte)
	r2 := svc.SingleInstanceKey()
	core.AssertTrue(t, r2.OK)
	key2 := r2.Value.([32]byte)
	core.AssertEqual(t, key1, key2)
}

// --- Cross-tier reachability (RFC §4.5 — Q1-B disjoint by construction) ---

// TestKeys_CrossTier_GetTier1_WhenOnlyT0FileExists_Good — a
// tier-0 blob at openai-default.t0.aead does NOT satisfy a
// GetTier1("openai-default") read because tier-1 reads
// openai-default.t1.aead exclusively. RFC §4.5 enforcement.
func TestKeys_CrossTier_GetTier1_WhenOnlyT0FileExists_Good(t *core.T) {
	svc := tier1Fixture(t)
	core.AssertTrue(t, svc.PutTier0("openai-default", []byte("tier0-value")).OK)
	// Tier-1 doesn't see the tier-0 blob.
	r := svc.GetTier1("openai-default")
	core.AssertFalse(t, r.OK, "GetTier1 must Fail when only the .t0.aead file exists")
	hasR := svc.HasTier1("openai-default")
	core.AssertTrue(t, hasR.OK)
	core.AssertFalse(t, hasR.Value.(bool), "HasTier1 must report false when only .t0.aead exists")
}

// TestKeys_CrossTier_PutTier0_WritesT0Suffix_Good — verifies the
// on-disk filename ends `.t0.aead`.
func TestKeys_CrossTier_PutTier0_WritesT0Suffix_Good(t *core.T) {
	svc := tier0Fixture(t)
	core.AssertTrue(t, svc.PutTier0("alpha", []byte("v")).OK)
	home := core.Getenv("HOME")
	expected := core.PathJoin(home, "Lethean", "data", "keys", "alpha.t0.aead")
	statR := core.Stat(expected)
	core.AssertTrue(t, statR.OK, "tier-0 blob must land at .t0.aead suffix")
}

// TestKeys_CrossTier_PutTier1_WritesT1Suffix_Good — verifies the
// on-disk filename ends `.t1.aead`.
func TestKeys_CrossTier_PutTier1_WritesT1Suffix_Good(t *core.T) {
	svc := tier1Fixture(t)
	core.AssertTrue(t, svc.PutTier1("alpha", []byte("v")).OK)
	home := core.Getenv("HOME")
	expected := core.PathJoin(home, "Lethean", "data", "keys", "alpha.t1.aead")
	statR := core.Stat(expected)
	core.AssertTrue(t, statR.OK, "tier-1 blob must land at .t1.aead suffix")
}

// TestKeys_CrossTier_PutTier0AndTier1_SameRef_TwoFiles_Good —
// the same ref in both tiers produces two distinct on-disk files;
// neither shadows the other.
func TestKeys_CrossTier_PutTier0AndTier1_SameRef_TwoFiles_Good(t *core.T) {
	svc := tier1Fixture(t)
	core.AssertTrue(t, svc.PutTier0("same", []byte("t0")).OK)
	core.AssertTrue(t, svc.PutTier1("same", []byte("t1")).OK)
	got0 := svc.GetTier0("same")
	got1 := svc.GetTier1("same")
	core.AssertTrue(t, got0.OK && got1.OK)
	core.AssertEqual(t, "t0", string(got0.Value.([]byte)))
	core.AssertEqual(t, "t1", string(got1.Value.([]byte)))
}

// --- ADD-K3 validator (`\.t[0-9]\.aead$` rejection) ---

func TestKeys_RefValidator_RejectsTierSuffix_Good(t *core.T) {
	svc := tier1Fixture(t)
	// Tier-1 PutTier1 with a ref ending in .t0.aead / .t1.aead /
	// .t9.aead MUST Fail.
	for _, bad := range []string{"foo.t0.aead", "bar.t1.aead", "baz.t9.aead"} {
		r := svc.PutTier1(bad, []byte("x"))
		core.AssertFalse(t, r.OK, "PutTier1 must reject ref="+bad)
	}
}

func TestKeys_RefValidator_RejectsTierSuffix_Bad(t *core.T) {
	svc := tier0Fixture(t)
	// Same rule for tier-0.
	for _, bad := range []string{"foo.t0.aead", "x.t5.aead"} {
		r := svc.PutTier0(bad, []byte("x"))
		core.AssertFalse(t, r.OK, "PutTier0 must reject ref="+bad)
	}
}

func TestKeys_RefValidator_RejectsTierSuffix_Ugly(t *core.T) {
	svc := tier0Fixture(t)
	// Edge cases: refs that LOOK suffixed but aren't — must pass.
	// "foo.txt" doesn't end .aead, "foo.tt.aead" has two letters
	// after dot, "foo.taead" is short of the .tN. shape.
	core.AssertTrue(t, svc.PutTier0("foo.txt", []byte("x")).OK)
	// "alpha.aead" ends .aead but no tier digit — currently allowed
	// (no ADD-K3 hit). The ref then resolves on-disk to
	// "alpha.aead.t0.aead" which is well-formed.
	core.AssertTrue(t, svc.PutTier0("alpha.aead", []byte("x")).OK)
}

// TestKeys_KeyPath_NULByte_Rejected_Bad — refs containing embedded
// NUL bytes are rejected at the keyPath validator (defensive against
// C-string truncation path-injection primitives). Cerberus #67 F-3
// / Mantis #1738.
func TestKeys_KeyPath_NULByte_Rejected_Bad(t *core.T) {
	svc := tier0Fixture(t)
	// Embedded NUL anywhere in the ref MUST Fail — middle, trailing,
	// leading-after-letter shapes all caught by the same gate.
	for _, bad := range []string{"valid\x00malicious", "trail\x00", "a\x00b\x00c"} {
		r := svc.PutTier0(bad, []byte("x"))
		core.AssertFalse(t, r.OK, "PutTier0 must reject ref with NUL byte")
	}
}

// --- KEK provider rotation + invalidation ---

// TestKeys_KEKProviderTier1_Rotation_InvalidatesCache_Good — set
// provider A, write, swap provider, re-read with the new provider
// from a fresh Service. The wrapped master under A is unreadable
// under B → Get fails.
func TestKeys_KEKProviderTier1_Rotation_InvalidatesCache_Good(t *core.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	r := keys.New()
	core.AssertTrue(t, r.OK)
	svc := r.Value.(*keys.Service)

	kekA := bytesOf(0xAA)
	svc.SetKEKProvider(func() ([]byte, bool) { return kekA, true })
	core.AssertTrue(t, svc.PutTier1("k", []byte("under-A")).OK)

	// Fresh Service over same on-disk state, wire kekB — read must
	// fail (different KEK can't unwrap).
	svc2 := keys.New().Value.(*keys.Service)
	kekB := bytesOf(0xBB)
	svc2.SetKEKProvider(func() ([]byte, bool) { return kekB, true })
	core.AssertFalse(t, svc2.GetTier1("k").OK, "fresh Service under different KEK must Fail to read tier-1 master")
}

// TestKeys_KEKProviderTier1_Reload_Good — same KEK after restart
// works.
func TestKeys_KEKProviderTier1_Reload_Good(t *core.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	r := keys.New()
	core.AssertTrue(t, r.OK)
	svc := r.Value.(*keys.Service)
	svc.SetKEKProvider(func() ([]byte, bool) { return fixedTier1KEK, true })
	core.AssertTrue(t, svc.PutTier1("k", []byte("payload")).OK)

	svc2 := keys.New().Value.(*keys.Service)
	svc2.SetKEKProvider(func() ([]byte, bool) { return fixedTier1KEK, true })
	got := svc2.GetTier1("k")
	core.AssertTrue(t, got.OK)
	core.AssertEqual(t, "payload", string(got.Value.([]byte)))
}

// TestKeys_NoProviderWired_Tier1Fails_Good — without
// SetKEKProvider, tier-1 ops Fail with a clear error.
func TestKeys_NoProviderWired_Tier1Fails_Good(t *core.T) {
	svc := tier0Fixture(t) // tier-0 wired, tier-1 not
	r := svc.PutTier1("k", []byte("x"))
	core.AssertFalse(t, r.OK)
}

// TestKeys_NoProviderWired_Tier0Fails_Good — without
// SetKEKProviderTier0, tier-0 ops Fail.
func TestKeys_NoProviderWired_Tier0Fails_Good(t *core.T) {
	svc := homeFixture(t) // neither wired
	r := svc.PutTier0("single-instance", []byte("x"))
	core.AssertFalse(t, r.OK)
}

// --- Tier-0 master format (KEK-wrapped, 72 bytes) ---

// TestKeys_Tier0Master_KEKWrappedOnDisk_Good — after PutTier0,
// the .master-tier0 on disk is the 72-byte KEK-wrapped envelope.
func TestKeys_Tier0Master_KEKWrappedOnDisk_Good(t *core.T) {
	svc := tier0Fixture(t)
	core.AssertTrue(t, svc.PutTier0("k", []byte("x")).OK)
	home := core.Getenv("HOME")
	masterPath := core.PathJoin(home, "Lethean", "data", "keys", ".master-tier0")
	blobR := core.ReadFile(masterPath)
	core.AssertTrue(t, blobR.OK)
	core.AssertEqual(t, 72, len(blobR.Value.([]byte)),
		"tier-0 master must be KEK-wrapped (24 nonce + 32 ct + 16 tag)")
}

// TestKeys_Tier1Master_KEKWrappedOnDisk_Good — after PutTier1,
// the .master-tier1 on disk is the 72-byte KEK-wrapped envelope.
func TestKeys_Tier1Master_KEKWrappedOnDisk_Good(t *core.T) {
	svc := tier1Fixture(t)
	core.AssertTrue(t, svc.PutTier1("k", []byte("x")).OK)
	home := core.Getenv("HOME")
	masterPath := core.PathJoin(home, "Lethean", "data", "keys", ".master-tier1")
	blobR := core.ReadFile(masterPath)
	core.AssertTrue(t, blobR.OK)
	core.AssertEqual(t, 72, len(blobR.Value.([]byte)),
		"tier-1 master must be KEK-wrapped (24 nonce + 32 ct + 16 tag)")
}

// --- Legacy install detection + migration (#1638 Step 0b) ---

// seedLegacyInstall creates a pre-partition (.master raw 32B +
// single-instance.aead sealed under it) install under the given
// home dir. Returns the legacy master bytes so the caller can
// assert downstream behaviour.
func seedLegacyInstall(t *core.T, home string) []byte {
	t.Helper()
	dir := core.PathJoin(home, "Lethean", "data", "keys")
	core.AssertTrue(t, core.MkdirAll(dir, 0o700).OK)

	// Generate a fixed-ish legacy master so tests are reproducible.
	legacy := bytesOf(0x10)
	masterPath := core.PathJoin(dir, ".master")
	core.AssertTrue(t, core.WriteFile(masterPath, legacy, 0o600).OK)

	// Seal a single-instance key under the legacy master, sealed
	// in the legacy single-instance.aead.
	sealedKey := make([]byte, 32)
	for i := range sealedKey {
		sealedKey[i] = 0x99
	}
	cipherSigil, err := sigil.NewChaChaPolySigil(legacy, nil)
	core.AssertTrue(t, err == nil)
	blob, err := cipherSigil.In(sealedKey)
	core.AssertTrue(t, err == nil)
	siPath := core.PathJoin(dir, "single-instance.aead")
	core.AssertTrue(t, core.WriteFile(siPath, blob, 0o600).OK)
	return legacy
}

// TestService_HeadlessLegacyInstall_PreservesSingleInstance —
// RFC §6 / Mantis #1638. Pre-#1624 32B raw .master +
// single-instance.aead present at boot, no tier-1 KEK provider
// ever wired. Step 0b sub-sequence fires: tier-0 master
// generated, single-instance.aead decrypted under legacy raw
// master, re-encrypted under tier-0 master, written to
// single-instance.t0.aead, legacy file deleted. Wails IPC
// continuity assertion.
func TestService_HeadlessLegacyInstall_PreservesSingleInstance(t *core.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	seedLegacyInstall(t, home)

	// Wire tier-0 KEK provider (e.g. cmd/lthn/app.go boot path).
	// Do NOT wire tier-1 — headless install.
	r := keys.New()
	core.AssertTrue(t, r.OK)
	svc := r.Value.(*keys.Service)
	svc.SetKEKProviderTier0(func() ([]byte, bool) { return fixedTier0KEK, true })

	// First tier-0 op (SingleInstanceKey) triggers Step 0b.
	keyR := svc.SingleInstanceKey()
	core.AssertTrue(t, keyR.OK, "SingleInstanceKey must succeed on legacy install")
	key := keyR.Value.([32]byte)
	// Returned key MUST be the original sealedKey value (0x99 fill).
	var want [32]byte
	for i := range want {
		want[i] = 0x99
	}
	core.AssertEqual(t, want, key, "SingleInstanceKey must return the pre-migration sealed bytes (IPC continuity)")

	// On-disk: tier-0 master present + tier-0 single-instance
	// present + legacy single-instance.aead removed. Legacy
	// .master retained for tier-1 migration.
	dir := core.PathJoin(home, "Lethean", "data", "keys")
	core.AssertTrue(t, core.Stat(core.PathJoin(dir, ".master-tier0")).OK, ".master-tier0 must be persisted")
	core.AssertTrue(t, core.Stat(core.PathJoin(dir, "single-instance.t0.aead")).OK, "single-instance.t0.aead must be written")
	core.AssertFalse(t, core.Stat(core.PathJoin(dir, "single-instance.aead")).OK, "legacy single-instance.aead must be removed")
	core.AssertTrue(t, core.Stat(core.PathJoin(dir, ".master")).OK, "legacy .master must be retained for tier-1 migration")

	// Tier-1 ops must Fail (no tier-1 KEK wired) with a clear error.
	core.AssertFalse(t, svc.PutTier1("openai", []byte("x")).OK)
}

// TestService_HeadlessPost1624Install_DefersTier0Migration —
// RFC §6 / Mantis #1638 sibling. 72B KEK-wrapped legacy .master +
// no tier-1 KEK provider. Step 0b detects legacy, skips
// sub-sequence; SingleInstanceKey returns the no_legacy_unlock
// error; on subsequent SetKEKProvider(tier1), Step 3 + Step 3e
// complete the migration.
func TestService_HeadlessPost1624Install_DefersTier0Migration(t *core.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := core.PathJoin(home, "Lethean", "data", "keys")
	core.AssertTrue(t, core.MkdirAll(dir, 0o700).OK)

	// Build a 72B KEK-wrapped legacy .master (post-#1624 form)
	// under the same KEK that we'll later wire as tier-1.
	legacyMaster := bytesOf(0x10)
	tier1KEK := fixedTier1KEK
	tier1Sigil, err := sigil.NewChaChaPolySigil(tier1KEK, nil)
	core.AssertTrue(t, err == nil)
	wrapped, err := tier1Sigil.In(legacyMaster)
	core.AssertTrue(t, err == nil)
	core.AssertEqual(t, 72, len(wrapped))
	core.AssertTrue(t, core.WriteFile(core.PathJoin(dir, ".master"), wrapped, 0o600).OK)

	// Seal a single-instance key under the legacy master.
	sealedKey := bytesOf(0x99)
	siSigil, err := sigil.NewChaChaPolySigil(legacyMaster, nil)
	core.AssertTrue(t, err == nil)
	siBlob, err := siSigil.In(sealedKey)
	core.AssertTrue(t, err == nil)
	core.AssertTrue(t, core.WriteFile(core.PathJoin(dir, "single-instance.aead"), siBlob, 0o600).OK)

	// Boot — tier-0 wired, tier-1 NOT wired.
	r := keys.New()
	core.AssertTrue(t, r.OK)
	svc := r.Value.(*keys.Service)
	svc.SetKEKProviderTier0(func() ([]byte, bool) { return fixedTier0KEK, true })

	// Step 0b detects legacy but skips sub-sequence; tier-0 ops
	// Fail with no_legacy_unlock.
	keyR := svc.SingleInstanceKey()
	core.AssertFalse(t, keyR.OK, "SingleInstanceKey must Fail pre-tier-1-unlock for 72B-wrapped legacy install")
	core.AssertFalse(t, core.Stat(core.PathJoin(dir, "single-instance.t0.aead")).OK)

	// Unlock — wire tier-1 KEK provider. Step 3 fires: legacy
	// .master is unwrapped under tier-1 KEK, re-wrapped to
	// .master-tier1, single-instance migrated to .t0.aead.
	svc.SetKEKProvider(func() ([]byte, bool) { return tier1KEK, true })
	keyR2 := svc.SingleInstanceKey()
	core.AssertTrue(t, keyR2.OK, "SingleInstanceKey must succeed after tier-1 unlock completes migration")
	var want [32]byte
	for i := range want {
		want[i] = 0x99
	}
	core.AssertEqual(t, want, keyR2.Value.([32]byte))

	// On-disk end state: legacy .master + .aead removed, tier
	// substrate present.
	core.AssertFalse(t, core.Stat(core.PathJoin(dir, ".master")).OK)
	core.AssertFalse(t, core.Stat(core.PathJoin(dir, "single-instance.aead")).OK)
	core.AssertTrue(t, core.Stat(core.PathJoin(dir, ".master-tier0")).OK)
	core.AssertTrue(t, core.Stat(core.PathJoin(dir, ".master-tier1")).OK)
	core.AssertTrue(t, core.Stat(core.PathJoin(dir, "single-instance.t0.aead")).OK)
}

// TestService_Migrate_FromLegacyRawMaster_OnFirstUnlock_Good —
// pre-#1624 raw .master + tier-1 .aead provider creds. First
// SetKEKProvider triggers the full Step 3 migration.
func TestService_Migrate_FromLegacyRawMaster_OnFirstUnlock_Good(t *core.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := core.PathJoin(home, "Lethean", "data", "keys")
	core.AssertTrue(t, core.MkdirAll(dir, 0o700).OK)

	legacy := bytesOf(0x10)
	core.AssertTrue(t, core.WriteFile(core.PathJoin(dir, ".master"), legacy, 0o600).OK)

	// Seal a tier-1 cred ("openai") under legacy master.
	legacySigil, err := sigil.NewChaChaPolySigil(legacy, nil)
	core.AssertTrue(t, err == nil)
	credBlob, err := legacySigil.In([]byte("sk-legacy"))
	core.AssertTrue(t, err == nil)
	core.AssertTrue(t, core.WriteFile(core.PathJoin(dir, "openai.aead"), credBlob, 0o600).OK)

	// Boot — wire both providers; tier-1 SetKEKProvider triggers
	// the full migration.
	r := keys.New()
	core.AssertTrue(t, r.OK)
	svc := r.Value.(*keys.Service)
	svc.SetKEKProviderTier0(func() ([]byte, bool) { return fixedTier0KEK, true })
	svc.SetKEKProvider(func() ([]byte, bool) { return legacy, true })

	// openai cred must read post-migration (rename to .t1.aead
	// preserved the legacy crypto, KEK over the same key bytes).
	got := svc.GetTier1("openai")
	core.AssertTrue(t, got.OK, "tier-1 read of migrated cred must succeed")
	core.AssertEqual(t, "sk-legacy", string(got.Value.([]byte)))

	// On-disk: .master removed, openai.aead renamed to .t1.aead.
	core.AssertFalse(t, core.Stat(core.PathJoin(dir, ".master")).OK)
	core.AssertFalse(t, core.Stat(core.PathJoin(dir, "openai.aead")).OK)
	core.AssertTrue(t, core.Stat(core.PathJoin(dir, "openai.t1.aead")).OK)
	core.AssertTrue(t, core.Stat(core.PathJoin(dir, ".master-tier1")).OK)
}

// TestService_Migrate_BothFilesPresent_RefusesProceed_Good —
// RFC §4.4 Step 2 corruption signal. Both .master and
// .master-tier1 present → migration refuses; tier-1 reads continue
// to require a clean state.
func TestService_Migrate_BothFilesPresent_RefusesProceed_Good(t *core.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := core.PathJoin(home, "Lethean", "data", "keys")
	core.AssertTrue(t, core.MkdirAll(dir, 0o700).OK)
	core.AssertTrue(t, core.WriteFile(core.PathJoin(dir, ".master"), bytesOf(0x10), 0o600).OK)
	// Plant a well-formed wrapped .master-tier1 under
	// fixedTier1KEK so the corruption check fires (both present).
	tier1Sigil, err := sigil.NewChaChaPolySigil(fixedTier1KEK, nil)
	core.AssertTrue(t, err == nil)
	wrapped, err := tier1Sigil.In(bytesOf(0x20))
	core.AssertTrue(t, err == nil)
	core.AssertTrue(t, core.WriteFile(core.PathJoin(dir, ".master-tier1"), wrapped, 0o600).OK)

	r := keys.New()
	core.AssertTrue(t, r.OK)
	svc := r.Value.(*keys.Service)
	svc.SetKEKProvider(func() ([]byte, bool) { return fixedTier1KEK, true })
	// Migration declines (warn logged) — tier-1 master DOES load
	// from .master-tier1 because corruption-detect doesn't tear
	// down a valid file. The .master file isn't touched.
	core.AssertTrue(t, core.Stat(core.PathJoin(dir, ".master")).OK, "legacy .master must remain after refused migration")
}

// TestService_Migrate_Idempotent_Good — second SetKEKProvider
// after a successful migration is a no-op.
func TestService_Migrate_Idempotent_Good(t *core.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := core.PathJoin(home, "Lethean", "data", "keys")
	core.AssertTrue(t, core.MkdirAll(dir, 0o700).OK)
	legacy := bytesOf(0x10)
	core.AssertTrue(t, core.WriteFile(core.PathJoin(dir, ".master"), legacy, 0o600).OK)

	r := keys.New()
	core.AssertTrue(t, r.OK)
	svc := r.Value.(*keys.Service)
	svc.SetKEKProvider(func() ([]byte, bool) { return legacy, true })
	// First write post-migration.
	core.AssertTrue(t, svc.PutTier1("k", []byte("v")).OK)
	// Second SetKEKProvider call — must not re-migrate.
	svc.SetKEKProvider(func() ([]byte, bool) { return legacy, true })
	got := svc.GetTier1("k")
	core.AssertTrue(t, got.OK)
	core.AssertEqual(t, "v", string(got.Value.([]byte)))
}

// --- Concurrent Put under migration (RFC §6 Q2 reliability) ---

// TestService_ConcurrentPutTier1_NoTornState_Ugly — parallel
// PutTier1 calls must all succeed without producing torn state.
// Serialised by s.mu internally.
func TestService_ConcurrentPutTier1_NoTornState_Ugly(t *core.T) {
	svc := tier1Fixture(t)
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			ref := core.Sprintf("ref-%d", id)
			core.AssertTrue(t, svc.PutTier1(ref, []byte(core.Sprintf("value-%d", id))).OK)
		}(i)
	}
	wg.Wait()
	r := svc.ListTier1()
	core.AssertTrue(t, r.OK)
	core.AssertEqual(t, 16, len(r.Value.([]string)))
}

// --- Mantis #1625 S2 / Cerberus #39: Step 3e defers when tier-0
//     master not loaded so legacy single-instance.aead doesn't
//     get orphaned by Step 3g deleting legacy .master.

// TestService_Migrate_Tier0NotLoaded_DefersSingleInstance_Good —
// 72B-wrapped legacy install + legacy single-instance.aead + tier-1
// KEK provider wired BEFORE tier-0 KEK provider. First
// migrateTier1Locked call (triggered by SetKEKProvider tier-1) must
// early-return at Step 3e: latch UNSET, single-instance.aead
// RETAINED, legacy .master RETAINED. Second call (after tier-0 KEK
// wires) must complete: latch SET, single-instance.t0.aead written,
// single-instance.aead removed, legacy .master removed.
func TestService_Migrate_Tier0NotLoaded_DefersSingleInstance_Good(t *core.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := core.PathJoin(home, "Lethean", "data", "keys")
	core.AssertTrue(t, core.MkdirAll(dir, 0o700).OK)

	// Build a 72B KEK-wrapped legacy .master (post-#1624 form)
	// under the KEK we'll later wire as tier-1.
	legacyMaster := bytesOf(0x10)
	tier1KEK := fixedTier1KEK
	tier1Sigil, err := sigil.NewChaChaPolySigil(tier1KEK, nil)
	core.AssertTrue(t, err == nil)
	wrapped, err := tier1Sigil.In(legacyMaster)
	core.AssertTrue(t, err == nil)
	core.AssertEqual(t, 72, len(wrapped))
	core.AssertTrue(t, core.WriteFile(core.PathJoin(dir, ".master"), wrapped, 0o600).OK)

	// Seal a single-instance key under the legacy master.
	sealedKey := bytesOf(0x99)
	siSigil, err := sigil.NewChaChaPolySigil(legacyMaster, nil)
	core.AssertTrue(t, err == nil)
	siBlob, err := siSigil.In(sealedKey)
	core.AssertTrue(t, err == nil)
	core.AssertTrue(t, core.WriteFile(core.PathJoin(dir, "single-instance.aead"), siBlob, 0o600).OK)

	// Boot — tier-1 wired FIRST, tier-0 NOT wired. (Real-world
	// trigger: pre-E.K.C wire order; substrate must defend against
	// it regardless.)
	r := keys.New()
	core.AssertTrue(t, r.OK)
	svc := r.Value.(*keys.Service)
	svc.SetKEKProvider(func() ([]byte, bool) { return tier1KEK, true })

	// First migrateTier1Locked fired by SetKEKProvider. Step 3e
	// must early-return: tier-0 master absent, single-instance.aead
	// present.
	//
	// On-disk invariants:
	//   - .master RETAINED (Step 3g did not fire)
	//   - single-instance.aead RETAINED (Step 3e did not re-seal)
	//   - .master-tier1 NOT written (Step 3d defers with the rest)
	//   - .master-tier0 NOT written (no tier-0 KEK provider)
	//   - single-instance.t0.aead NOT written
	core.AssertTrue(t, core.Stat(core.PathJoin(dir, ".master")).OK,
		"legacy .master must be retained when Step 3e defers")
	core.AssertTrue(t, core.Stat(core.PathJoin(dir, "single-instance.aead")).OK,
		"legacy single-instance.aead must be retained when tier-0 not loaded")
	core.AssertFalse(t, core.Stat(core.PathJoin(dir, ".master-tier0")).OK,
		".master-tier0 must NOT be written without tier-0 KEK provider")
	core.AssertFalse(t, core.Stat(core.PathJoin(dir, "single-instance.t0.aead")).OK,
		"single-instance.t0.aead must NOT be written without tier-0 master")

	// Latch UNSET assertion via observable behaviour: wiring
	// tier-0 KEK now MUST re-trigger migration on next
	// SetKEKProvider call. If the latch had been set, the second
	// SetKEKProvider would skip migrateTier1Locked entirely and
	// the legacy files would persist forever.
	svc.SetKEKProviderTier0(func() ([]byte, bool) { return fixedTier0KEK, true })
	// Re-fire migration by calling SetKEKProvider again with the
	// same tier-1 KEK. The migrationComplete latch guards this:
	// if it were already set, this would short-circuit.
	svc.SetKEKProvider(func() ([]byte, bool) { return tier1KEK, true })

	// Now the full Step 3 sequence must have completed.
	keyR := svc.SingleInstanceKey()
	core.AssertTrue(t, keyR.OK,
		"SingleInstanceKey must succeed after tier-0 wires and migration completes")
	var want [32]byte
	for i := range want {
		want[i] = 0x99
	}
	core.AssertEqual(t, want, keyR.Value.([32]byte),
		"recovered SI key must match pre-migration sealed bytes (IPC continuity)")

	// On-disk end state: legacy gone, tier substrate present.
	core.AssertFalse(t, core.Stat(core.PathJoin(dir, ".master")).OK,
		"legacy .master must be removed once migration completes")
	core.AssertFalse(t, core.Stat(core.PathJoin(dir, "single-instance.aead")).OK,
		"legacy single-instance.aead must be removed once migration completes")
	core.AssertTrue(t, core.Stat(core.PathJoin(dir, ".master-tier0")).OK,
		".master-tier0 must be written by Step 3c")
	core.AssertTrue(t, core.Stat(core.PathJoin(dir, ".master-tier1")).OK,
		".master-tier1 must be written by Step 3d")
	core.AssertTrue(t, core.Stat(core.PathJoin(dir, "single-instance.t0.aead")).OK,
		"single-instance.t0.aead must be written by Step 3e")
}

// TestService_Migrate_Tier0OnDiskNoProvider_Defers_NoHalfState —
// Mantis #1625 Cerberus #40: tighten 3b.5 — file-presence on disk
// (.master-tier0) does NOT imply we can unwrap it without a live
// tier-0 KEK provider. Step 3c only writes-on-absence (it does not
// load-on-presence), so a file-only fixture would leave
// s.tier0Master empty while Step 3d still wrote .master-tier1 and
// Step 3g still deleted legacy .master — bricking the install on
// next boot (legacyStat.OK && tier1Stat.OK → corruption-refuse).
//
// Setup: 72B-wrapped legacy .master + legacy single-instance.aead +
// pre-existing .master-tier0 on disk (wrapped under tier-0 KEK) +
// tier-1 KEK provider wired + tier-0 KEK provider NOT wired.
//
// Assert: Step 3b.5 fires defer pre-write — NO .master-tier1 written,
// legacy .master + single-instance.aead retained, .master-tier0 left
// byte-identical (Step 3c never rewrote). Latch unset.
//
// NB Cerberus #40 SECURITY-NOTE (surfaced separately): the retry
// path (second SetKEKProvider after tier-0 wires) does NOT complete
// in this on-disk-file-no-provider shape — Step 3c's write-on-
// absence path is skipped because the file is already there, so
// s.tier0Master stays empty and Step 3e's defensive defer catches.
// The tighten makes the FIRST-call state strictly safer (no
// half-state, no brick), but a full fix needs an additional
// load-on-presence path in Step 3c. Test scope here is the 3b.5
// guard only — first-call safety. Retry-success path is the
// follow-on ticket.
func TestService_Migrate_Tier0OnDiskNoProvider_Defers_NoHalfState(t *core.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := core.PathJoin(home, "Lethean", "data", "keys")
	core.AssertTrue(t, core.MkdirAll(dir, 0o700).OK)

	// Build a 72B KEK-wrapped legacy .master (post-#1624 form)
	// under the KEK we'll later wire as tier-1.
	legacyMaster := bytesOf(0x10)
	tier1KEK := fixedTier1KEK
	tier1Sigil, err := sigil.NewChaChaPolySigil(tier1KEK, nil)
	core.AssertTrue(t, err == nil)
	wrapped, err := tier1Sigil.In(legacyMaster)
	core.AssertTrue(t, err == nil)
	core.AssertEqual(t, 72, len(wrapped))
	core.AssertTrue(t, core.WriteFile(core.PathJoin(dir, ".master"), wrapped, 0o600).OK)

	// Seal a single-instance key under the legacy master.
	sealedKey := bytesOf(0x99)
	siSigil, err := sigil.NewChaChaPolySigil(legacyMaster, nil)
	core.AssertTrue(t, err == nil)
	siBlob, err := siSigil.In(sealedKey)
	core.AssertTrue(t, err == nil)
	core.AssertTrue(t, core.WriteFile(core.PathJoin(dir, "single-instance.aead"), siBlob, 0o600).OK)

	// Pre-seed .master-tier0 on disk, wrapped under the tier-0 KEK
	// (so the file is real + cryptographically valid), but DO NOT
	// wire a tier-0 KEK provider — simulates the dangerous shape
	// Cerberus #40 named: file-present, KEK absent.
	tier0Master := bytesOf(0x20)
	tier0KEK := fixedTier0KEK
	tier0Sigil, err := sigil.NewChaChaPolySigil(tier0KEK, nil)
	core.AssertTrue(t, err == nil)
	tier0Wrapped, err := tier0Sigil.In(tier0Master)
	core.AssertTrue(t, err == nil)
	core.AssertEqual(t, 72, len(tier0Wrapped))
	tier0Path := core.PathJoin(dir, ".master-tier0")
	core.AssertTrue(t, core.WriteFile(tier0Path, tier0Wrapped, 0o600).OK)

	// Snapshot the on-disk tier-0 master bytes so we can assert it
	// wasn't rewritten by a half-completed Step 3c during the
	// deferred-call window.
	tier0Pre := core.ReadFile(tier0Path)
	core.AssertTrue(t, tier0Pre.OK)

	// Boot — tier-1 wired, tier-0 NOT wired. Pre-#40 guard would
	// have proceeded (tier0OnDisk=true short-circuited the defer);
	// post-#40 guard must defer because tier-0 provider isn't live.
	r := keys.New()
	core.AssertTrue(t, r.OK)
	svc := r.Value.(*keys.Service)
	svc.SetKEKProvider(func() ([]byte, bool) { return tier1KEK, true })

	// Step 3b.5 must have fired pre-write defer.
	//   - .master retained
	//   - single-instance.aead retained
	//   - .master-tier1 NOT written (no half-state)
	//   - single-instance.t0.aead NOT written
	//   - .master-tier0 byte-identical to seed (Step 3c never ran)
	core.AssertTrue(t, core.Stat(core.PathJoin(dir, ".master")).OK,
		"legacy .master must be retained when 3b.5 defers under file-on-disk-no-provider")
	core.AssertTrue(t, core.Stat(core.PathJoin(dir, "single-instance.aead")).OK,
		"legacy single-instance.aead must be retained when 3b.5 defers")
	core.AssertFalse(t, core.Stat(core.PathJoin(dir, ".master-tier1")).OK,
		".master-tier1 must NOT be written when 3b.5 defers — would brick on next boot")
	core.AssertFalse(t, core.Stat(core.PathJoin(dir, "single-instance.t0.aead")).OK,
		"single-instance.t0.aead must NOT be written when 3b.5 defers")
	tier0Post := core.ReadFile(tier0Path)
	core.AssertTrue(t, tier0Post.OK)
	core.AssertEqual(t, tier0Pre.Value.([]byte), tier0Post.Value.([]byte),
		".master-tier0 must be byte-identical to seed — Step 3c must not have rewritten it")
}

// TestService_Migrate_Tier0OnDiskNoProvider_RetrySuccess_Good —
// Mantis #1653 (Cerberus C#19 deferred from #1625): close the retry
// path that the #40 tighten left dangling. Setup mirrors the #40
// fixture (legacy .master + legacy single-instance.aead + on-disk
// .master-tier0 + tier-1 KEK live + tier-0 KEK NOT live). First
// SetKEKProvider defers per 3b.5 (covered by the #40 test). Then we
// wire the tier-0 KEK provider and re-fire SetKEKProvider — the
// load-on-presence branch added to Step 3c MUST unwrap the on-disk
// .master-tier0 so Steps 3d-3g can complete cleanly.
//
// Assert: post-retry, .master-tier1 is written, .master is removed,
// single-instance.t0.aead is written (legacy SI re-sealed under
// tier-0 master), and SingleInstanceKey() returns the originally
// sealed 32B payload — proves the in-memory tier-0 master matches
// the on-disk one and the re-seal used the correct key.
func TestService_Migrate_Tier0OnDiskNoProvider_RetrySuccess_Good(t *core.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := core.PathJoin(home, "Lethean", "data", "keys")
	core.AssertTrue(t, core.MkdirAll(dir, 0o700).OK)

	// Same fixture shape as the #40 defer test.
	legacyMaster := bytesOf(0x10)
	tier1KEK := fixedTier1KEK
	tier1Sigil, err := sigil.NewChaChaPolySigil(tier1KEK, nil)
	core.AssertTrue(t, err == nil)
	wrapped, err := tier1Sigil.In(legacyMaster)
	core.AssertTrue(t, err == nil)
	core.AssertTrue(t, core.WriteFile(core.PathJoin(dir, ".master"), wrapped, 0o600).OK)

	// Seal a known SI payload under the legacy master so we can
	// round-trip-verify after the migration completes.
	sealedKey := bytesOf(0x99)
	siSigil, err := sigil.NewChaChaPolySigil(legacyMaster, nil)
	core.AssertTrue(t, err == nil)
	siBlob, err := siSigil.In(sealedKey)
	core.AssertTrue(t, err == nil)
	core.AssertTrue(t, core.WriteFile(core.PathJoin(dir, "single-instance.aead"), siBlob, 0o600).OK)

	// Pre-seed .master-tier0 wrapped under the tier-0 KEK we'll wire
	// on the second SetKEKProvider call. The retry path MUST unwrap
	// this byte-for-byte and use the resulting master to re-seal SI.
	tier0Master := bytesOf(0x20)
	tier0KEK := fixedTier0KEK
	tier0Sigil, err := sigil.NewChaChaPolySigil(tier0KEK, nil)
	core.AssertTrue(t, err == nil)
	tier0Wrapped, err := tier0Sigil.In(tier0Master)
	core.AssertTrue(t, err == nil)
	tier0Path := core.PathJoin(dir, ".master-tier0")
	core.AssertTrue(t, core.WriteFile(tier0Path, tier0Wrapped, 0o600).OK)
	tier0Pre := core.ReadFile(tier0Path)
	core.AssertTrue(t, tier0Pre.OK)

	// First boot — tier-1 wired, tier-0 NOT wired. 3b.5 must defer.
	r := keys.New()
	core.AssertTrue(t, r.OK)
	svc := r.Value.(*keys.Service)
	svc.SetKEKProvider(func() ([]byte, bool) { return tier1KEK, true })

	// Confirm the defer fired (precondition for the retry path).
	core.AssertTrue(t, core.Stat(core.PathJoin(dir, ".master")).OK,
		"precondition: 3b.5 must have deferred — legacy .master retained")
	core.AssertFalse(t, core.Stat(core.PathJoin(dir, ".master-tier1")).OK,
		"precondition: 3b.5 must have deferred — .master-tier1 not written")

	// Retry: wire tier-0 KEK, then re-fire SetKEKProvider to trigger
	// migrateTier1Locked again. Step 3c load-on-presence MUST now
	// unwrap .master-tier0 so Steps 3d-3g complete.
	svc.SetKEKProviderTier0(func() ([]byte, bool) { return tier0KEK, true })
	svc.SetKEKProvider(func() ([]byte, bool) { return tier1KEK, true })

	// Post-retry assertions: full migration complete.
	core.AssertFalse(t, core.Stat(core.PathJoin(dir, ".master")).OK,
		"legacy .master must be removed once retry completes Step 3g")
	core.AssertTrue(t, core.Stat(core.PathJoin(dir, ".master-tier1")).OK,
		".master-tier1 must be written after retry succeeds")
	core.AssertFalse(t, core.Stat(core.PathJoin(dir, "single-instance.aead")).OK,
		"legacy single-instance.aead must be removed after Step 3e re-seal")
	core.AssertTrue(t, core.Stat(core.PathJoin(dir, "single-instance.t0.aead")).OK,
		"single-instance.t0.aead must be written under tier-0 master")

	// .master-tier0 byte-identical — Step 3c must NOT have rewritten
	// it on the load-on-presence path (file was already there).
	tier0Post := core.ReadFile(tier0Path)
	core.AssertTrue(t, tier0Post.OK)
	core.AssertEqual(t, tier0Pre.Value.([]byte), tier0Post.Value.([]byte),
		".master-tier0 must be byte-identical to seed — load-on-presence must not rewrite")

	// Round-trip via SingleInstanceKey: proves the in-memory tier-0
	// master loaded by Step 3c matches the on-disk wrapped master,
	// AND that Step 3e re-sealed SI using that same master (so the
	// new .t0.aead unwraps to the original sealed payload). This is
	// the C#19 prescription: SEEDED bytes round-trip after retry.
	siR := svc.SingleInstanceKey()
	core.AssertTrue(t, siR.OK, "SingleInstanceKey must succeed after retry-success")
	gotKey := siR.Value.([32]byte)
	var wantKey [32]byte
	copy(wantKey[:], sealedKey)
	core.AssertEqual(t, wantKey, gotKey,
		"SingleInstanceKey must return the originally sealed payload — proves tier-0 master loaded correctly and SI re-seal used it")
}

// --- Audit emit tests (Mantis #1763 / Cerberus #77 F-1) ---

// keysRecordingRecorder captures every audit.Event the keys service
// emits during a test. Mirror of the pkg/runner / pkg/account fixture
// shape; the goroutine-safe variant guards against any future
// concurrent emit path (today every emit is synchronous, but the
// guard keeps the fixture forward-safe).
type keysRecordingRecorder struct {
	mu     sync.Mutex
	events []audit.Event
}

func (r *keysRecordingRecorder) Record(ev audit.Event) core.Result {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, ev)
	return core.Ok(nil)
}

func (r *keysRecordingRecorder) snapshot() []audit.Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]audit.Event, len(r.events))
	copy(out, r.events)
	return out
}

// installKeysRecorder wires a keysRecordingRecorder as audit.Default
// and restores the prior default at test end.
//
// Usage example:
//
//	rec := installKeysRecorder(t)
//	... drive service ...
//	events := rec.snapshot()
func installKeysRecorder(t *core.T) *keysRecordingRecorder {
	t.Helper()
	rec := &keysRecordingRecorder{}
	audit.SetDefault(rec)
	t.Cleanup(func() { audit.SetDefault(nil) })
	return rec
}

// findKeysEvent returns the most-recent event whose Event field
// matches name, or nil if none. Used by the emit-shape tests below.
func findKeysEvent(events []audit.Event, name string) *audit.Event {
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Event == name {
			return &events[i]
		}
	}
	return nil
}

// TestKeys_PutTier1_EmitsAudit_Good — PutTier1 first-write fires
// EventKeysTier1Stored with source="internal" and the canonical Meta
// shape (ref / kind / tier / source); overwrite fires the Replaced
// variant. Mantis #1763 / Cerberus #77 F-1.
func TestKeys_PutTier1_EmitsAudit_Good(t *core.T) {
	rec := installKeysRecorder(t)
	svc := tier1Fixture(t)

	// First write — Stored.
	r := svc.PutTier1("openai-default", []byte("sk-do-not-leak-1763"))
	core.AssertTrue(t, r.OK)

	stored := findKeysEvent(rec.snapshot(), audit.EventKeysTier1Stored)
	core.AssertTrue(t, stored != nil,
		"EventKeysTier1Stored must fire on first-write PutTier1")
	core.AssertEqual(t, audit.OutcomeOK, stored.Outcome)
	core.AssertEqual(t, "keys", stored.Scope)
	core.AssertEqual(t, "openai-default", stored.Meta["ref"])
	core.AssertEqual(t, "tier1", stored.Meta["kind"])
	core.AssertEqual(t, "tier1", stored.Meta["tier"])
	core.AssertEqual(t, "internal", stored.Meta["source"])

	// Overwrite — Replaced.
	r2 := svc.PutTier1("openai-default", []byte("sk-do-not-leak-rotated"))
	core.AssertTrue(t, r2.OK)
	replaced := findKeysEvent(rec.snapshot(), audit.EventKeysTier1Replaced)
	core.AssertTrue(t, replaced != nil,
		"EventKeysTier1Replaced must fire on overwrite PutTier1")
	core.AssertEqual(t, audit.OutcomeOK, replaced.Outcome)
	core.AssertEqual(t, "openai-default", replaced.Meta["ref"])
	core.AssertEqual(t, "internal", replaced.Meta["source"])
}

// TestKeys_DeleteTier1_EmitsAudit_Good — DeleteTier1 against a present
// ref fires EventKeysTier1Deleted with source="internal" and the
// canonical Meta shape; the idempotent no-op path (file absent) emits
// nothing. Mantis #1763 / Cerberus #77 F-1.
func TestKeys_DeleteTier1_EmitsAudit_Good(t *core.T) {
	rec := installKeysRecorder(t)
	svc := tier1Fixture(t)

	// Seed a ref to delete.
	core.AssertTrue(t, svc.PutTier1("openai-default", []byte("sk-seed")).OK)
	preCount := len(rec.snapshot())

	// Idempotent no-op against an absent ref — no emit.
	r := svc.DeleteTier1("never-existed")
	core.AssertTrue(t, r.OK)
	idempotentSnap := rec.snapshot()
	core.AssertEqual(t, preCount, len(idempotentSnap),
		"idempotent no-op DeleteTier1 must NOT emit an audit row")

	// Actual removal — Deleted.
	core.AssertTrue(t, svc.DeleteTier1("openai-default").OK)
	deleted := findKeysEvent(rec.snapshot(), audit.EventKeysTier1Deleted)
	core.AssertTrue(t, deleted != nil,
		"EventKeysTier1Deleted must fire on actual at-rest removal")
	core.AssertEqual(t, audit.OutcomeOK, deleted.Outcome)
	core.AssertEqual(t, "keys", deleted.Scope)
	core.AssertEqual(t, "openai-default", deleted.Meta["ref"])
	core.AssertEqual(t, "tier1", deleted.Meta["kind"])
	core.AssertEqual(t, "tier1", deleted.Meta["tier"])
	core.AssertEqual(t, "internal", deleted.Meta["source"])
}

// TestKeys_AuditMeta_NoKeyBytes_Bad — PII canary: drives PutTier0 /
// PutTier1 / WPutTier1 / DeleteTier1 with a recognisable plaintext
// sentinel, then asserts the sentinel never appears in any audit row
// Meta string value (Cerberus #1465 closure-only-scope discipline).
// Mantis #1763 / Cerberus #77 F-1.
func TestKeys_AuditMeta_NoKeyBytes_Bad(t *core.T) {
	rec := installKeysRecorder(t)
	svc := tier1Fixture(t)

	const sentinel = "sk-canary-do-not-persist-1763"

	core.AssertTrue(t, svc.PutTier0("single-instance", []byte(sentinel)).OK)
	core.AssertTrue(t, svc.PutTier1("openai-default", []byte(sentinel)).OK)
	core.AssertTrue(t, svc.WPutTier1("anthropic-default", sentinel).OK)
	core.AssertTrue(t, svc.DeleteTier1("openai-default").OK)
	core.AssertTrue(t, svc.DeleteTier0("single-instance").OK)

	events := rec.snapshot()
	core.AssertTrue(t, len(events) > 0, "must have emitted at least one audit row")
	for i := range events {
		for k, v := range events[i].Meta {
			vs, ok := v.(string)
			if !ok {
				continue
			}
			core.AssertFalse(t, core.Contains(vs, sentinel),
				"event["+events[i].Event+"].Meta["+k+"] must not echo plaintext credential")
		}
	}
}

// TestKeys_WPutTier1_EmitsAudit_Good — Wails-surface WPutTier1 fires
// EventKeysTier1Stored with source="wails_input", distinguishing the
// renderer-reachable call path from the Go-internal PutTier1 path.
// WDeleteTier1 mirrors the same source discipline. Mantis #1763 /
// Cerberus #77 F-1.
func TestKeys_WPutTier1_EmitsAudit_Good(t *core.T) {
	rec := installKeysRecorder(t)
	svc := tier1Fixture(t)

	core.AssertTrue(t, svc.WPutTier1("openai-default", "sk-wails-shape").OK)
	stored := findKeysEvent(rec.snapshot(), audit.EventKeysTier1Stored)
	core.AssertTrue(t, stored != nil,
		"EventKeysTier1Stored must fire from WPutTier1")
	core.AssertEqual(t, "wails_input", stored.Meta["source"],
		"Wails-surface call must carry source=wails_input")
	core.AssertEqual(t, "openai-default", stored.Meta["ref"])
	core.AssertEqual(t, "tier1", stored.Meta["kind"])

	// WDeleteTier1 mirrors the same source discrimination.
	core.AssertTrue(t, svc.WDeleteTier1("openai-default").OK)
	deleted := findKeysEvent(rec.snapshot(), audit.EventKeysTier1Deleted)
	core.AssertTrue(t, deleted != nil,
		"EventKeysTier1Deleted must fire from WDeleteTier1")
	core.AssertEqual(t, "wails_input", deleted.Meta["source"])
}
