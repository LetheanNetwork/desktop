// SPDX-Licence-Identifier: EUPL-1.2

// enable_lifecycle_test.go — coverage for enable.go's persisted-flag +
// spawn/stop-sweep methods (IsEnabled / Enable / Disable / setEnabled
// / readEnabledFlag).

package opencode

import "testing"

func TestIsEnabled_DefaultsFalse_Good(t *testing.T) {
	resetKV(t)
	svc := &Service{}
	if svc.IsEnabled() {
		t.Errorf("IsEnabled() on a fresh store = true; want false")
	}
}

func TestIsEnabled_KVUnavailable_Bad(t *testing.T) {
	resetKV(t)
	breakKV(t)
	svc := &Service{}
	if svc.IsEnabled() {
		t.Errorf("IsEnabled() with kv unavailable = true; want false (fail-closed)")
	}
}

func TestSetEnabled_RoundTrips_Good(t *testing.T) {
	resetKV(t)
	svc := &Service{}
	if r := svc.setEnabled(true); !r.OK {
		t.Fatalf("setEnabled(true) failed: %s", r.Error())
	}
	if !svc.IsEnabled() {
		t.Errorf("IsEnabled() after setEnabled(true) = false; want true")
	}
	if r := svc.setEnabled(false); !r.OK {
		t.Fatalf("setEnabled(false) failed: %s", r.Error())
	}
	if svc.IsEnabled() {
		t.Errorf("IsEnabled() after setEnabled(false) = true; want false")
	}
}

func TestSetEnabled_KVUnavailable_Bad(t *testing.T) {
	resetKV(t)
	breakKV(t)
	svc := &Service{}
	if r := svc.setEnabled(true); r.OK {
		t.Fatalf("setEnabled with kv unavailable returned OK; want Fail")
	}
}

// TestReadEnabledFlag_MissingKey_Good / Present_Good — the defensive
// lookup helper distinguishes "never enabled" (no key) from an
// explicit stored value.
func TestReadEnabledFlag_MissingKey_Good(t *testing.T) {
	resetKV(t)
	svc := &Service{}
	raw, ok := svc.readEnabledFlag()
	if ok {
		t.Errorf("readEnabledFlag() on a fresh store: ok = true; want false")
	}
	if raw != "" {
		t.Errorf("readEnabledFlag() raw = %q; want empty", raw)
	}
}

func TestReadEnabledFlag_Present_Good(t *testing.T) {
	resetKV(t)
	svc := &Service{}
	if r := svc.setEnabled(true); !r.OK {
		t.Fatalf("setEnabled failed: %s", r.Error())
	}
	raw, ok := svc.readEnabledFlag()
	if !ok {
		t.Fatalf("readEnabledFlag() ok = false; want true after setEnabled")
	}
	if raw != enabledTrue {
		t.Errorf("readEnabledFlag() raw = %q; want %q", raw, enabledTrue)
	}
}

func TestReadEnabledFlag_KVUnavailable_Bad(t *testing.T) {
	resetKV(t)
	breakKV(t)
	svc := &Service{}
	raw, ok := svc.readEnabledFlag()
	if ok || raw != "" {
		t.Errorf("readEnabledFlag() with kv unavailable = (%q, %v); want (\"\", false)", raw, ok)
	}
}

// TestEnable_SpawnsWhenNoneRunning_Good — full Enable() round trip:
// persists the flag AND spawns a sandbox via the real Start() path.
func TestEnable_SpawnsWhenNoneRunning_Good(t *testing.T) {
	fake := newFakeOpencodeServe(t)
	pinPortAllocation(t, fake.Server)
	rt := fakeRuntime(t, "exit 0")
	svc := newTestService(t, Options{Runtime: rt})

	r := svc.Enable("")
	if !r.OK {
		t.Fatalf("Enable failed: %s", r.Error())
	}
	id, _ := r.Value.(string)
	if id == "" {
		t.Fatalf("Enable returned empty sandbox id")
	}
	if !svc.IsEnabled() {
		t.Errorf("IsEnabled() after Enable = false; want true")
	}
}

// TestEnable_IdempotentWhenAlreadyRunning_Good — a second Enable()
// call with a sandbox already running short-circuits to the existing
// id without spawning a second container (proven by the fake runtime
// never being asked to run a SECOND container — Reconcile-adjacent
// concern; here we just assert the returned id is stable).
func TestEnable_IdempotentWhenAlreadyRunning_Good(t *testing.T) {
	fake := newFakeOpencodeServe(t)
	pinPortAllocation(t, fake.Server)
	rt := fakeRuntime(t, "exit 0")
	svc := newTestService(t, Options{Runtime: rt})

	first := svc.Enable("")
	if !first.OK {
		t.Fatalf("first Enable failed: %s", first.Error())
	}
	firstID, _ := first.Value.(string)

	second := svc.Enable("")
	if !second.OK {
		t.Fatalf("second Enable failed: %s", second.Error())
	}
	secondID, _ := second.Value.(string)
	if secondID != firstID {
		t.Errorf("second Enable id = %q; want the same running sandbox id %q", secondID, firstID)
	}
}

// TestEnable_KVUnavailable_Bad — setEnabled fails before any spawn
// attempt.
func TestEnable_KVUnavailable_Bad(t *testing.T) {
	resetKV(t)
	breakKV(t)
	svc := &Service{}
	r := svc.Enable("")
	if r.OK {
		t.Fatalf("Enable with kv unavailable returned OK; want Fail")
	}
}

// TestDisable_StopsRunningSandboxes_Good — Disable persists the flag
// AND stops every currently-running sandbox.
func TestDisable_StopsRunningSandboxes_Good(t *testing.T) {
	fake := newFakeOpencodeServe(t)
	pinPortAllocation(t, fake.Server)
	rt := fakeRuntime(t, "exit 0")
	svc := newTestService(t, Options{Runtime: rt})

	startR := svc.Start("")
	if !startR.OK {
		t.Fatalf("Start failed: %s", startR.Error())
	}
	id, _ := startR.Value.(string)

	r := svc.Disable()
	if !r.OK {
		t.Fatalf("Disable failed: %s", r.Error())
	}
	if svc.IsEnabled() {
		t.Errorf("IsEnabled() after Disable = true; want false")
	}
	inspectR := svc.Inspect(id)
	if !inspectR.OK {
		t.Fatalf("Inspect after Disable failed: %s", inspectR.Error())
	}
	sb, _ := inspectR.Value.(Sandbox)
	if sb.Status != StatusStopped {
		t.Errorf("sandbox status after Disable = %q; want %q", sb.Status, StatusStopped)
	}
}

// TestDisable_NothingRunning_Good — idempotent no-op path.
func TestDisable_NothingRunning_Good(t *testing.T) {
	svc := newTestService(t, Options{})
	r := svc.Disable()
	if !r.OK {
		t.Fatalf("Disable with nothing running failed: %s", r.Error())
	}
}

// TestDisable_KVUnavailable_Bad — setEnabled fails before the status
// sweep.
func TestDisable_KVUnavailable_Bad(t *testing.T) {
	resetKV(t)
	breakKV(t)
	svc := &Service{}
	r := svc.Disable()
	if r.OK {
		t.Fatalf("Disable with kv unavailable returned OK; want Fail")
	}
}
