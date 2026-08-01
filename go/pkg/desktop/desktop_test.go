// SPDX-Licence-Identifier: EUPL-1.2

// Tests for the desktop service. AX-7 triplet per public symbol:
// Test<File>_<Receiver>_<Method>_<Variant>. Each test avoids starting
// the Wails event loop — desktop.Run requires an active NSApp on macOS.
// The single-instance key path is exercised via the pkg/keys surface
// (the integration-level guarantee is that desktop.Run calls
// SingleInstanceKey and fails fast when Keys is nil + key unavailable).

package desktop_test

import (
	core "dappco.re/go"
	"dappco.re/go/crypt/keys"
	"dappco.re/lthn/desktop/pkg/keysvc"
)

// keysFixture constructs a keys.Service under a temp HOME and
// wires a deterministic tier-0 KEK provider so SingleInstanceKey
// (tier-0 partition per RFC.stage-e-keys-partition v3 / Mantis
// #1625) can encrypt/decrypt without a real .seed substrate. The
// fixture supplies a fixed 32-byte KEK; production wiring is the
// HKDF(.seed, "lthn-keys-kek", "tier0-v1", 32) closure in
// cmd/lthn/app.go (E.K.C lane).
func keysFixture(t *core.T) *keys.Service {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	r := keysvc.New()
	core.AssertTrue(t, r.OK, "keys.New must succeed under temp HOME")
	svc := r.Value.(*keys.Service)
	tier0KEK := make([]byte, 32)
	for i := range tier0KEK {
		tier0KEK[i] = 0x42
	}
	svc.SetKEKProviderTier0(func() ([]byte, bool) { return tier0KEK, true })
	return svc
}

// --- Single-instance key round-trip (pkg/keys integration) ---

// TestDesktop_SingleInstanceKey_Fresh confirms that a fresh install
// generates a non-zero key and persists it.
func TestDesktop_SingleInstanceKey_Fresh(t *core.T) {
	svc := keysFixture(t)
	r := svc.SingleInstanceKey()
	core.AssertTrue(t, r.OK, "SingleInstanceKey must succeed on first call")
	key, ok := r.Value.([32]byte)
	core.AssertTrue(t, ok, "Value must be [32]byte")
	var zero [32]byte
	core.AssertNotEqual(t, zero, key, "generated key must not be all-zero")

	// Persisted — HasTier0("single-instance") must be true. The
	// single-instance Wails IPC key lives in the tier-0 partition
	// per RFC.stage-e-keys-partition v3 (Mantis #1625).
	hasR := svc.HasTier0("single-instance")
	core.AssertTrue(t, hasR.OK)
	core.AssertTrue(t, hasR.Value.(bool), "single-instance blob must be persisted after generation")
}

// TestDesktop_SingleInstanceKey_Reload confirms that a second boot
// reloads the same key without regenerating.
func TestDesktop_SingleInstanceKey_Reload(t *core.T) {
	svc := keysFixture(t)

	// Simulate first boot.
	r1 := svc.SingleInstanceKey()
	core.AssertTrue(t, r1.OK)
	key1 := r1.Value.([32]byte)

	// Simulate second boot — construct a fresh Service over the
	// same temp HOME. The tier-0 master + single-instance.t0.aead
	// are already on disk; re-wire the tier-0 KEK provider with
	// the SAME fixed KEK so the second Service can unwrap the
	// existing tier-0 master.
	svc2 := keysvc.New().Value.(*keys.Service)
	tier0KEK := make([]byte, 32)
	for i := range tier0KEK {
		tier0KEK[i] = 0x42
	}
	svc2.SetKEKProviderTier0(func() ([]byte, bool) { return tier0KEK, true })
	r2 := svc2.SingleInstanceKey()
	core.AssertTrue(t, r2.OK, "SingleInstanceKey must succeed on reload")
	key2 := r2.Value.([32]byte)

	core.AssertEqual(t, key1, key2, "key must be identical across reboots")
}
