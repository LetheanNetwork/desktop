// SPDX-Licence-Identifier: EUPL-1.2

// Enable / Disable — persisted "should opencode-serve be running"
// flag at opencode.serve.enabled in the DuckDB KV. Sibling of
// ServerPassword: same store, same lifecycle. The flag is the
// signal that lets `lthn serve` auto-resume the sandbox on boot
// (RFC.opencode.md §7) without re-prompting the user.
//
// Semantics:
//
//   - Enable persists the flag AND spawns a sandbox if none is
//     running. Idempotent — calling Enable while already running
//     is a no-op (the flag stays true, sandbox stays alive).
//   - Disable persists the flag AND stops any running sandbox.
//     Idempotent — calling Disable while already stopped is fine.
//   - IsEnabled reads the flag. Defaults to false (no key → not
//     enabled) so a fresh install doesn't auto-spawn a container.

package opencode

import (
	core "dappco.re/go"
	goiostore "dappco.re/go/io/store"
)

const (
	// serverEnabledKey lives in the same group as the password —
	// both are per-install "opencode service" settings.
	serverEnabledKey = "enabled"
	// enabledTrue / enabledFalse are the stored values. We use
	// strings rather than booleans so the KV layer stays simple
	// (goiostore.KeyValueStore stores strings).
	enabledTrue  = "true"
	enabledFalse = "false"
)

// IsEnabled returns whether opencode-serve should be running per
// the persisted flag. Defaults to false when the key is missing.
// Persistence errors fall back to false — better to start cold
// than to spawn on a transient KV blip.
//
// Usage example:
//
//	if svc.IsEnabled() { _ = svc.Start(opencode.DefaultProfile) }
func (s *Service) IsEnabled() bool {
	st, r := kv()
	if !r.OK {
		return false
	}
	raw, err := st.Get(serverAuthStoreGroup, serverEnabledKey)
	if err != nil {
		return false
	}
	return raw == enabledTrue
}

// Enable persists `opencode.serve.enabled = true` and spawns a
// sandbox with profileName if none is running. profileName empty
// = DefaultProfile. Returns the sandbox id on success.
//
// Idempotent — if a sandbox is already running, just sets the
// flag and returns its id.
//
// Usage example:
//
//	r := svc.Enable("")
//	if r.OK { id := r.Value.(string); _ = id }
func (s *Service) Enable(profileName string) core.Result {
	if r := s.setEnabled(true); !r.OK {
		return r
	}
	// Already-running short-circuit — returns the existing id.
	if statusR := s.Status(); statusR.OK {
		running, _ := statusR.Value.([]Sandbox)
		if len(running) > 0 {
			return core.Ok(running[0].ID)
		}
	}
	return s.Start(profileName)
}

// Disable persists `opencode.serve.enabled = false` and stops any
// running sandboxes. Idempotent — no-op when nothing is running.
//
// Usage example:
//
//	r := svc.Disable()
//	if r.OK { _ = r }
func (s *Service) Disable() core.Result {
	if r := s.setEnabled(false); !r.OK {
		return r
	}
	statusR := s.Status()
	if !statusR.OK {
		// Setting succeeded; stop-sweep failed only because we
		// couldn't list. Surface as success — the flag is the
		// load-bearing state, container teardown will retry on
		// next boot via auto-resume's negative branch.
		return core.Ok(nil)
	}
	running, _ := statusR.Value.([]Sandbox)
	var firstErr core.Result
	firstErr.OK = true
	for _, sb := range running {
		if r := s.Stop(sb.ID); !r.OK && firstErr.OK {
			firstErr = r
		}
	}
	return firstErr
}

// setEnabled writes the enabled flag. Internal helper used by
// Enable + Disable.
func (s *Service) setEnabled(on bool) core.Result {
	st, r := kv()
	if !r.OK {
		return r
	}
	val := enabledFalse
	if on {
		val = enabledTrue
	}
	if err := st.Set(serverAuthStoreGroup, serverEnabledKey, val); err != nil {
		return core.Fail(err)
	}
	return core.Ok(nil)
}

// readEnabledFlag is a defensive lookup helper that returns the
// raw key state (true / false / missing). Unused today but useful
// when the auto-resume path lands in cmd/lthn — distinguishes
// "never enabled" (no key) from "explicitly disabled" (false).
//
//nolint:unused // future-arc helper for cmd/lthn telemetry.
func (s *Service) readEnabledFlag() (string, bool) {
	st, r := kv()
	if !r.OK {
		return "", false
	}
	raw, err := st.Get(serverAuthStoreGroup, serverEnabledKey)
	if err != nil {
		if core.Is(err, goiostore.NotFoundError) {
			return "", false
		}
		return "", false
	}
	return raw, true
}
