// SPDX-Licence-Identifier: EUPL-1.2

// export_test.go — package-internal test seams for time-mocked
// stale-prune coverage of the per-account_id lockout map (Mantis #1596
// LOW, forward-arc from #1586). The lockout helpers are package-private
// for production hygiene; this file lives in package account (NOT
// account_test) so it can reach the unexported state, then exports the
// minimum surface unlock_test.go (package account_test) needs to drive
// the window-expiry assertions.
//
// The helpers MUST NOT change production behaviour — they wrap existing
// internals with a fixed-clock parameter so tests can pin "now" without
// patching core.Now() (which is the SECURITY-NOTE the H#54 sibling
// TestUnlock_RecordFailedAttempt_StaleEntriesPruned_Good called out as
// future-work).
//
// Naming convention: every export carries the `ForTest` suffix so
// `grep -n ForTest pkg/account/` surfaces every test-only seam in one
// pass — keeps reviewer attention on the boundary.

package account

// PruneStaleLockoutsForTest exposes the package-private
// pruneStaleLockoutsLocked walker to external tests so they can drive
// the stale-prune path with a fixed nowUnix. Acquires s.mu (write lock)
// internally so callers don't have to know about the *Locked naming.
//
// Mirrors the production call-site shape: recordFailedAttempt holds the
// write lock and invokes pruneStaleLockoutsLocked; this helper takes
// the same lock + invokes the same function with a test-supplied
// timestamp, so what we observe is exactly the production path's prune
// behaviour at the chosen wall-clock.
//
// Usage example (from unlock_test.go):
//
//	subject.PruneStaleLockoutsForTest(svc, futureNow)
//	core.AssertEqual(t, 0, subject.LockoutMapSizeForTest(svc))
func PruneStaleLockoutsForTest(s *Service, nowUnix int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneStaleLockoutsLocked(nowUnix)
}

// SeedLockoutEntryForTest plants a *lockoutEntry directly into the
// per-account lockout map, bypassing recordFailedAttempt's accountExists
// gate (#1586 Option 2). Used by the time-mocked prune tests to set up
// a deterministic fixture: N entries with chosen attempt timestamps +
// chosen unlockAt, advance the mocked clock past the window, then call
// PruneStaleLockoutsForTest to assert which entries drop.
//
// Bypassing accountExists is the right shape for these tests because
// they verify the PRUNE LOGIC in isolation — the gate is already
// covered by the Option-2 tests in unlock_test.go and exercising it
// here would mean writing N real on-disk account fixtures just to
// satisfy a Stat() check the prune logic doesn't itself care about.
//
// attempts is taken by value (slice copy) so the caller's backing
// array stays independent of the map's stored entry.
//
// Usage example (from unlock_test.go):
//
//	subject.SeedLockoutEntryForTest(svc, "abc", []int64{t1, t2}, 0)
func SeedLockoutEntryForTest(s *Service, accountID string, attempts []int64, unlockAt int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lockouts == nil {
		s.lockouts = map[string]*lockoutEntry{}
	}
	cp := make([]int64, len(attempts))
	copy(cp, attempts)
	s.lockouts[accountID] = &lockoutEntry{
		attempts: cp,
		unlockAt: unlockAt,
	}
}

// LockoutMapSizeForTest returns the current length of the per-account
// lockout map. Reads under the RLock so the test can call it
// concurrently with sibling test goroutines (-race coverage).
//
// Usage example (from unlock_test.go):
//
//	core.AssertEqual(t, 5, subject.LockoutMapSizeForTest(svc))
func LockoutMapSizeForTest(s *Service) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.lockouts)
}

// LockoutHasEntryForTest reports whether the per-account lockout map
// currently holds an entry for the supplied account_id. Used by the
// prune tests to assert specific entries dropped (or were preserved)
// without exposing the *lockoutEntry pointer.
//
// Usage example (from unlock_test.go):
//
//	core.AssertFalse(t, subject.LockoutHasEntryForTest(svc, "stale-id"))
func LockoutHasEntryForTest(s *Service, accountID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.lockouts[accountID]
	return ok
}

// LockoutConstantsForTest exposes the package-private lockout policy
// constants so tests can construct fixture timestamps RELATIVE to the
// configured window/cooldown instead of hard-coding 300/60 — when the
// constants are tuned, the tests follow automatically.
//
// Returns (threshold, windowSeconds, cooldownSeconds) in the same order
// the constants are declared in types.go.
//
// Usage example (from unlock_test.go):
//
//	_, window, _ := subject.LockoutConstantsForTest()
//	staleTS := baseNow - window - 1
func LockoutConstantsForTest() (threshold int, windowSeconds int64, cooldownSeconds int64) {
	return lockoutThreshold, lockoutWindowSeconds, lockoutCooldownSeconds
}

// SeedUnlockedForTest plants raw bytes directly into the in-memory
// unlocked-private-key map, bypassing the real Unlock decrypt path.
// Used by ServiceShutdown's state-clearing test so it doesn't have to
// pay a real PGP keygen + iterated-S2K decrypt (seconds of wall-clock
// per call across this package's already-heavy suite) just to prove a
// trivial "shutdown nils the maps" behaviour that has nothing to do
// with the decrypt path itself.
//
// Usage example (from service_test.go):
//
//	subject.SeedUnlockedForTest(svc, "abc123", []byte("fixture-priv"))
func SeedUnlockedForTest(s *Service, accountID string, priv []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.unlocked == nil {
		s.unlocked = map[string][]byte{}
	}
	cp := make([]byte, len(priv))
	copy(cp, priv)
	s.unlocked[accountID] = cp
}
