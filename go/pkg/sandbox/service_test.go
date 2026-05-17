// SPDX-Licence-Identifier: EUPL-1.2

package sandbox

import (
	core "dappco.re/go"
)

// TestSandbox_Service_InstallID_IdempotentAcrossCalls_Good — the
// first call generates + persists a hex-encoded 16-byte identifier;
// every subsequent call on the same lthn install returns the same
// value. Mirrors opencode's per-install identifier contract — the
// label that SpawnLong stamps must be stable so a future reconcile
// pass can gate "which surviving containers do I trust to adopt".
//
// The kv layer is a process-singleton (core.Once), so HOME is set
// before the first call and any sibling test that exercises kv()
// would bind the same store path. There's only one InstallID
// caller today (this test); if a second sandbox test needs kv()
// state isolation later, it should fork a subprocess.
func TestSandbox_Service_InstallID_IdempotentAcrossCalls_Good(t *core.T) {
	t.Setenv("HOME", t.TempDir())

	svc := newTestService(Options{})
	core.AssertNotNil(t, svc)

	r1 := NewSpawnPort(svc).InstallID()
	core.AssertTrue(t, r1.OK)
	id1, ok := r1.Value.(string)
	core.AssertTrue(t, ok)
	// Hex-encoded 16 bytes = 32 chars.
	core.AssertEqual(t, 32, len(id1))

	r2 := NewSpawnPort(svc).InstallID()
	core.AssertTrue(t, r2.OK)
	id2, ok := r2.Value.(string)
	core.AssertTrue(t, ok)
	core.AssertEqual(t, id1, id2)

	// And a third call, just to confirm steady-state.
	r3 := NewSpawnPort(svc).InstallID()
	core.AssertTrue(t, r3.OK)
	core.AssertEqual(t, id1, r3.Value.(string))
}
