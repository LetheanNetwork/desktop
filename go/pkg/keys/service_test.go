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

// Production wires Register via core.WithName(...); RegisterService fires
// post-factory inside WithName. Direct-call testing of Register must use
// WithName to model the production lifecycle (Mantis #1457).
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

// --- GetOrCreate ---

func TestService_Service_GetOrCreate_Good(t *core.T) {
	svc := homeFixture(t)
	// Fresh install — generate fires and key is persisted.
	calls := 0
	r := svc.GetOrCreate("new-key", func() ([]byte, error) {
		calls++
		return []byte("generated-secret"), nil
	})
	core.AssertTrue(t, r.OK, "GetOrCreate must succeed on fresh ref")
	core.AssertEqual(t, "generated-secret", string(r.Value.([]byte)))
	core.AssertEqual(t, 1, calls, "generate must be called exactly once")

	// Second call — reloads from disk, generate is NOT called again.
	r2 := svc.GetOrCreate("new-key", func() ([]byte, error) {
		calls++
		return []byte("should-not-appear"), nil
	})
	core.AssertTrue(t, r2.OK)
	core.AssertEqual(t, "generated-secret", string(r2.Value.([]byte)))
	core.AssertEqual(t, 1, calls, "generate must not fire when key already persisted")
}

func TestService_Service_GetOrCreate_Bad(t *core.T) {
	svc := homeFixture(t)
	// generate() returning an error must propagate as Fail.
	r := svc.GetOrCreate("failing-key", func() ([]byte, error) {
		return nil, core.NewError("deliberate generate failure")
	})
	core.AssertFalse(t, r.OK, "GetOrCreate must Fail when generate errors")

	// Empty ref must Fail (delegates to Put / Get path-validation).
	r2 := svc.GetOrCreate("", func() ([]byte, error) {
		return []byte("x"), nil
	})
	core.AssertFalse(t, r2.OK, "GetOrCreate must Fail on empty ref")
}

func TestService_Service_GetOrCreate_Ugly(t *core.T) {
	svc := homeFixture(t)
	// Pre-existing key written via Put must be returned without calling generate.
	core.AssertTrue(t, svc.Put("pre-existing", []byte("original")).OK)
	calls := 0
	r := svc.GetOrCreate("pre-existing", func() ([]byte, error) {
		calls++
		return []byte("override"), nil
	})
	core.AssertTrue(t, r.OK)
	core.AssertEqual(t, "original", string(r.Value.([]byte)), "pre-existing value must win")
	core.AssertEqual(t, 0, calls, "generate must not fire when key already exists")
}

// --- SingleInstanceKey ---

func TestService_Service_SingleInstanceKey_Good(t *core.T) {
	svc := homeFixture(t)
	// First boot — key is generated, returned as [32]byte.
	r := svc.SingleInstanceKey()
	core.AssertTrue(t, r.OK, "SingleInstanceKey must succeed on fresh install")
	key, ok := r.Value.([32]byte)
	core.AssertTrue(t, ok, "SingleInstanceKey Value must be [32]byte")
	// Key must not be the zero value.
	var zero [32]byte
	core.AssertNotEqual(t, zero, key, "generated key must not be all-zero")
}

func TestService_Service_SingleInstanceKey_Bad(t *core.T) {
	// Wrong-length stored blob must Fail.
	svc := homeFixture(t)
	// Write a short blob directly to the ref so ensureMaster
	// and Put/Get succeed but the length check inside
	// SingleInstanceKey rejects it.
	core.AssertTrue(t, svc.Put("single-instance", []byte("tooshort")).OK)
	r := svc.SingleInstanceKey()
	core.AssertFalse(t, r.OK, "SingleInstanceKey must Fail when stored blob has wrong length")
}

func TestService_Service_SingleInstanceKey_Ugly(t *core.T) {
	svc := homeFixture(t)
	// First call generates; subsequent calls return the same key.
	r1 := svc.SingleInstanceKey()
	core.AssertTrue(t, r1.OK)
	key1 := r1.Value.([32]byte)

	r2 := svc.SingleInstanceKey()
	core.AssertTrue(t, r2.OK)
	key2 := r2.Value.([32]byte)

	core.AssertEqual(t, key1, key2, "SingleInstanceKey must return same key on repeat calls")
}

// --- Mantis #1624 — KEK-gate on post-unlock PGP-derived KEK ---

// kekFromBytes is the test-side analogue of cmd/lthn/app.go's
// production provider closure — derives a 32-byte KEK from a fake
// "private key" via core.HKDF using the same canonical salt/info
// pair production code uses (keys.KEKHKDFSalt / keys.KEKHKDFInfo).
// Returned by the helper Provider used in the tests below.
func kekFromBytes(t *core.T, priv []byte) []byte {
	t.Helper()
	r := core.HKDF("sha256", priv,
		[]byte(keys.KEKHKDFSalt), []byte(keys.KEKHKDFInfo), 32)
	core.AssertTrue(t, r.OK, "HKDF derive must succeed")
	return r.Value.([]byte)
}

// TestKeys_KEKGate_PreUnlock_FallsBack_Good — no provider wired
// (or provider reports unlocked=false), master persists/reads via
// the legacy raw-32-byte path. Headless `lthn serve` boot scenario.
func TestKeys_KEKGate_PreUnlock_FallsBack_Good(t *core.T) {
	svc := homeFixture(t)
	// No KEK provider wired — same scenario as headless boot.
	core.AssertTrue(t, svc.Put("k", []byte("legacy-path")).OK)
	r := svc.Get("k")
	core.AssertTrue(t, r.OK)
	core.AssertEqual(t, "legacy-path", string(r.Value.([]byte)))

	// Explicit nil provider — equivalent to "not wired".
	svc.SetKEKProvider(nil)
	core.AssertTrue(t, svc.Put("k2", []byte("still-legacy")).OK)
	r2 := svc.Get("k2")
	core.AssertTrue(t, r2.OK)
	core.AssertEqual(t, "still-legacy", string(r2.Value.([]byte)))

	// Provider returns (_, false) — same fallback.
	svc.SetKEKProvider(func() ([]byte, bool) { return nil, false })
	core.AssertTrue(t, svc.Put("k3", []byte("locked")).OK)
	r3 := svc.Get("k3")
	core.AssertTrue(t, r3.OK)
	core.AssertEqual(t, "locked", string(r3.Value.([]byte)))
}

// TestKeys_KEKGate_PostUnlock_DerivesViaPGP_Good — provider returns
// a KEK derived via HKDF from fake private-key bytes; round-trip
// Put/Get works and the on-disk master is the 60-byte wrapped form.
func TestKeys_KEKGate_PostUnlock_DerivesViaPGP_Good(t *core.T) {
	t.Setenv("HOME", t.TempDir())
	r := keys.New()
	core.AssertTrue(t, r.OK)
	svc := r.Value.(*keys.Service)

	fakePriv := []byte("fake-pgp-private-key-bytes-for-test-only")
	svc.SetKEKProvider(func() ([]byte, bool) {
		return kekFromBytes(t, fakePriv), true
	})

	core.AssertTrue(t, svc.Put("openai", []byte("sk-secret")).OK)
	got := svc.Get("openai")
	core.AssertTrue(t, got.OK)
	core.AssertEqual(t, "sk-secret", string(got.Value.([]byte)))

	// Verify on-disk master is the 60-byte wrapped envelope, not raw 32.
	home := core.Getenv("HOME")
	masterPath := core.PathJoin(home, "Lethean", "data", "keys", ".master")
	blobR := core.ReadFile(masterPath)
	core.AssertTrue(t, blobR.OK)
	blob := blobR.Value.([]byte)
	core.AssertEqual(t, 72, len(blob),
		"post-unlock master must be KEK-wrapped (24 XChaCha20 nonce + 32 ct + 16 tag)")
}

// TestKeys_KEKGate_LockTransition_RotatesMaster_Ugly — start
// unlocked, write a wrapped master, then flip provider to return
// (_, false). Subsequent writes still succeed (cache holds the
// unwrapped master) BUT a fresh Service constructed against the
// same on-disk state Fails to read because the wrapped envelope
// needs the KEK that's no longer available.
func TestKeys_KEKGate_LockTransition_RotatesMaster_Ugly(t *core.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	r := keys.New()
	core.AssertTrue(t, r.OK)
	svc := r.Value.(*keys.Service)

	fakePriv := []byte("session-priv-bytes")
	unlocked := true
	svc.SetKEKProvider(func() ([]byte, bool) {
		if !unlocked {
			return nil, false
		}
		return kekFromBytes(t, fakePriv), true
	})

	// Establish wrapped master + a stored secret while unlocked.
	core.AssertTrue(t, svc.Put("api", []byte("first")).OK)
	core.AssertTrue(t, svc.Get("api").OK)

	// Flip to locked — subsequent writes in the SAME process still
	// succeed because the master is cached + the next master write
	// path falls back gracefully (no rotation back-to-legacy fires).
	unlocked = false
	core.AssertTrue(t, svc.Put("api2", []byte("second")).OK,
		"cached master keeps in-process operations working post-lock")

	// A fresh Service (simulates next process boot post-lock) MUST
	// Fail to ensureMaster — the on-disk master is wrapped and the
	// KEK provider isn't wired, so the legacy path can't decode it.
	r2 := keys.New()
	core.AssertTrue(t, r2.OK)
	svc2 := r2.Value.(*keys.Service)
	// No provider wired — represents the locked state of a fresh boot.
	getR := svc2.Get("api")
	core.AssertFalse(t, getR.OK,
		"fresh Service with wrapped master + no KEK provider must Fail")
}

// TestKeys_KEKGate_MixedFormat_LegacyMasterDecrypts_Good — install
// has an existing raw 32-byte master + a secret stored against it
// (the "installed user" scenario). After SetKEKProvider lands, the
// legacy master reads unchanged, an existing secret still decrypts,
// AND the NEXT Put migrates the master in-place to the wrapped form.
func TestKeys_KEKGate_MixedFormat_LegacyMasterDecrypts_Good(t *core.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Pre-KEK install — write a secret while no provider is wired so
	// the master file lands in the legacy 32-byte raw form.
	r1 := keys.New()
	core.AssertTrue(t, r1.OK)
	preSvc := r1.Value.(*keys.Service)
	core.AssertTrue(t, preSvc.Put("legacy", []byte("pre-kek-secret")).OK)

	masterPath := core.PathJoin(home, "Lethean", "data", "keys", ".master")
	preBlobR := core.ReadFile(masterPath)
	core.AssertTrue(t, preBlobR.OK)
	core.AssertEqual(t, 32, len(preBlobR.Value.([]byte)),
		"pre-KEK master must be raw 32 bytes")

	// Now the same install upgrades — wire a KEK provider and
	// construct a fresh Service against the existing on-disk state.
	r2 := keys.New()
	core.AssertTrue(t, r2.OK)
	postSvc := r2.Value.(*keys.Service)
	fakePriv := []byte("upgrade-time-priv-bytes")
	postSvc.SetKEKProvider(func() ([]byte, bool) {
		return kekFromBytes(t, fakePriv), true
	})

	// The legacy secret MUST still decrypt — the read-path accepts
	// the 32-byte legacy master without going through the KEK.
	got := postSvc.Get("legacy")
	core.AssertTrue(t, got.OK, "legacy secret must keep decrypting post-KEK-wire")
	core.AssertEqual(t, "pre-kek-secret", string(got.Value.([]byte)))

	// Next Put triggers the lazy migration: master gets re-persisted
	// in the 60-byte wrapped form. Read it from disk to confirm.
	core.AssertTrue(t, postSvc.Put("new", []byte("post-kek-secret")).OK)
	postBlobR := core.ReadFile(masterPath)
	core.AssertTrue(t, postBlobR.OK)
	core.AssertEqual(t, 72, len(postBlobR.Value.([]byte)),
		"first write post-KEK-wire must migrate master to wrapped form")

	// And the legacy secret + the new secret BOTH still decrypt —
	// the master plaintext is unchanged across the format rotation.
	core.AssertEqual(t, "pre-kek-secret", string(postSvc.Get("legacy").Value.([]byte)))
	core.AssertEqual(t, "post-kek-secret", string(postSvc.Get("new").Value.([]byte)))

	// A fresh Service post-migration MUST require the provider to
	// read the now-wrapped master.
	r3 := keys.New()
	core.AssertTrue(t, r3.OK)
	freshSvc := r3.Value.(*keys.Service)
	freshSvc.SetKEKProvider(func() ([]byte, bool) {
		return kekFromBytes(t, fakePriv), true
	})
	core.AssertTrue(t, freshSvc.Get("legacy").OK,
		"post-migration fresh Service with KEK reads legacy secret")
}

// TestKeys_KEKGate_RoundTrip_Good — provider → write → read returns
// the same bytes. The minimal end-to-end happy-path that pins the
// contract: SetKEKProvider doesn't break the existing Put/Get
// contract; ciphertext on disk for the *secret* is sealed under the
// master (not under the KEK directly), and decrypt round-trips.
func TestKeys_KEKGate_RoundTrip_Good(t *core.T) {
	svc := homeFixture(t)
	svc.SetKEKProvider(func() ([]byte, bool) {
		return kekFromBytes(t, []byte("round-trip-priv")), true
	})
	payload := []byte("the quick brown fox jumps over the lazy dog")
	core.AssertTrue(t, svc.Put("brown-fox", payload).OK)
	r := svc.Get("brown-fox")
	core.AssertTrue(t, r.OK)
	core.AssertEqual(t, string(payload), string(r.Value.([]byte)))
}
