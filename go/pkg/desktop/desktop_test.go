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
	"dappco.re/lthn/desktop/pkg/keys"
)

// keysFixture constructs a keys.Service under a temp HOME.
func keysFixture(t *core.T) *keys.Service {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	r := keys.New()
	core.AssertTrue(t, r.OK, "keys.New must succeed under temp HOME")
	return r.Value.(*keys.Service)
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

	// Persisted — Has("single-instance") must be true.
	hasR := svc.Has("single-instance")
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

	// Simulate second boot — construct a fresh Service over the same
	// temp HOME. The master + single-instance.aead are already on disk.
	svc2 := keys.New().Value.(*keys.Service)
	r2 := svc2.SingleInstanceKey()
	core.AssertTrue(t, r2.OK, "SingleInstanceKey must succeed on reload")
	key2 := r2.Value.([32]byte)

	core.AssertEqual(t, key1, key2, "key must be identical across reboots")
}
