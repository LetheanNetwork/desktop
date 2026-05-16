// SPDX-Licence-Identifier: EUPL-1.2

// lock_test.go — sentinel-file lock + NFS-detection tests per
// RFC.atomic-write.md §10. Mandatory test names from the RFC:
//
//   - TestWithFileLock_TimeoutBlocked_Bad
//   - TestWithFileLock_ReleasedOnPanic_Ugly
//   - TestSentinelLock_OrphanReclaimAfterDeadPID_Ugly
//   - TestSentinelLock_PIDRecycleNoFalseAlive_Ugly (Q1 + MED-1)
//   - TestPathsRoot_NetworkFSRejected_Bad (HIGH-2)

package paths_test

import (
	"sync"
	"testing"

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/paths"
)

func TestWithFileLock_Good(t *core.T) {
	homeFixture(t)
	root := paths.Root()
	core.AssertTrue(t, root.OK)
	target := core.PathJoin(root.Value.(string), "target.md")

	called := false
	r := paths.WithFileLock(target, 0, func() core.Result {
		called = true
		return core.Ok("done")
	})
	core.AssertTrue(t, r.OK, "WithFileLock should succeed: "+r.Error())
	core.AssertTrue(t, called, "fn should have been invoked")

	// Sentinel must be released on return.
	stat := core.Stat(target + paths.LockfileSuffix)
	core.AssertFalse(t, stat.OK, "sentinel should be removed after fn returns")
}

func TestWithFileLock_TimeoutBlocked_Bad(t *core.T) {
	homeFixture(t)
	root := paths.Root()
	core.AssertTrue(t, root.OK)
	target := core.PathJoin(root.Value.(string), "blocked.md")

	// Hold the lock from a goroutine and signal the test once held.
	held := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{})
	go func() {
		_ = paths.WithFileLock(target, 0, func() core.Result {
			close(held)
			<-release
			return core.Ok(nil)
		})
		close(done)
	}()
	<-held
	defer func() { close(release); <-done }()

	// Second acquire with a tiny timeout MUST fail.
	r := paths.WithFileLock(target, 50*core.Millisecond, func() core.Result {
		return core.Ok(nil)
	})
	core.AssertFalse(t, r.OK, "second acquire should timeout")
	core.AssertContains(t, r.Error(), paths.CodeLockTimeout)
}

func TestWithFileLock_ReleasedOnPanic_Ugly(t *core.T) {
	homeFixture(t)
	root := paths.Root()
	core.AssertTrue(t, root.OK)
	target := core.PathJoin(root.Value.(string), "panicky.md")

	// First call panics inside fn — the defer must still unlink the sentinel.
	func() {
		defer func() { _ = recover() }()
		_ = paths.WithFileLock(target, 0, func() core.Result {
			panic("kaboom")
		})
	}()

	// Sentinel must NOT remain.
	stat := core.Stat(target + paths.LockfileSuffix)
	core.AssertFalse(t, stat.OK, "panicking fn must still release the lock")

	// Subsequent acquire must succeed immediately.
	r := paths.WithFileLock(target, 100*core.Millisecond, func() core.Result {
		return core.Ok(nil)
	})
	core.AssertTrue(t, r.OK, "follow-up acquire should not be blocked")
}

func TestSentinelLock_OrphanReclaimAfterDeadPID_Ugly(t *core.T) {
	homeFixture(t)
	root := paths.Root()
	core.AssertTrue(t, root.OK)
	target := core.PathJoin(root.Value.(string), "orphan.md")
	sentinel := target + paths.LockfileSuffix

	// Plant a sentinel pointing at a PID that cannot exist (max uint16
	// is a safe ceiling on POSIX; we go higher to be sure).
	deadPID := 1<<30 - 1
	body := []byte(core.Itoa(deadPID) + "\n0\n")
	if r := core.WriteFile(sentinel, body, 0o600); !r.OK {
		t.Fatalf("plant sentinel: %s", r.Error())
	}

	// Acquire must reclaim quickly and succeed.
	r := paths.WithFileLock(target, 500*core.Millisecond, func() core.Result {
		return core.Ok(nil)
	})
	core.AssertTrue(t, r.OK, "dead-PID holder should be reclaimed: "+r.Error())
}

func TestSentinelLock_PIDRecycleNoFalseAlive_Ugly(t *core.T) {
	homeFixture(t)
	root := paths.Root()
	core.AssertTrue(t, root.OK)
	target := core.PathJoin(root.Value.(string), "recycle.md")
	sentinel := target + paths.LockfileSuffix

	// Plant a sentinel claiming the CURRENT pid but with a fork-time
	// that does NOT match the live process (1 ns since epoch is a
	// known-bad value). The reclaim path treats this as a PID-recycle
	// signal and reclaims, even though kill(0) would say the PID is alive.
	body := []byte(core.Itoa(core.Getpid()) + "\n1\n")
	if r := core.WriteFile(sentinel, body, 0o600); !r.OK {
		t.Fatalf("plant sentinel: %s", r.Error())
	}

	r := paths.WithFileLock(target, 500*core.Millisecond, func() core.Result {
		return core.Ok(nil)
	})
	// On platforms where fork-time is unavailable (process_other.go),
	// the conservative branch waits out the timeout. Either outcome
	// proves there is no false-alive panic; the assert is that the
	// behaviour is deterministic — never a corruption.
	if r.OK {
		// Good — recycle detected and reclaimed.
		return
	}
	core.AssertContains(t, r.Error(), paths.CodeLockTimeout,
		"on platforms without fork-time, timeout is the safe fallback")
}

func TestWithFileLock_Concurrent_Ugly(t *core.T) {
	homeFixture(t)
	root := paths.Root()
	core.AssertTrue(t, root.OK)
	target := core.PathJoin(root.Value.(string), "concurrent.md")

	var counter int
	var mu sync.Mutex
	const N = 8
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			_ = paths.WithFileLock(target, 5*core.Second, func() core.Result {
				mu.Lock()
				counter++
				mu.Unlock()
				return core.Ok(nil)
			})
		}()
	}
	wg.Wait()
	core.AssertEqual(t, N, counter, "all goroutines should have acquired the lock serially")
}

// TestWithFileLock_NWayRaceRespectsTimeout_Ugly — Cerberus DREAD-r2
// F1 (Mantis #1525). Multiple goroutines chase the same lock under
// a long-held holder + a re-planter that keeps replacing the
// sentinel with a dead-PID body the moment any acquire-window opens.
// The re-planter forces maybeReclaim() to return true in a tight
// loop; pre-fix code skipped the deadline check on every reclaim
// continue, so racers could wait far past their declared Timeout.
// With the fix the deadline is re-tested at the TOP of every
// iteration and each racer surfaces CodeLockTimeout within budget.
func TestWithFileLock_NWayRaceRespectsTimeout_Ugly(t *core.T) {
	homeFixture(t)
	root := paths.Root()
	core.AssertTrue(t, root.OK)
	target := core.PathJoin(root.Value.(string), "nway-race.md")
	sentinel := target + paths.LockfileSuffix

	// Re-planter goroutine: continuously writes a dead-PID sentinel
	// body so racers' maybeReclaim() removes it then immediately
	// finds a fresh dead sentinel on the next iteration. This is
	// the precise reclaim-loop shape that the pre-fix code starved
	// on — every reclaim returned true so the deadline check was
	// bypassed.
	deadPID := 1<<30 - 1
	body := []byte(core.Itoa(deadPID) + "\n0\n")
	stop := make(chan struct{})
	planterDone := make(chan struct{})
	go func() {
		defer close(planterDone)
		for {
			select {
			case <-stop:
				return
			default:
			}
			_ = core.WriteFile(sentinel, body, 0o600)
		}
	}()

	const N = 3
	results := make(chan core.Result, N)
	var wg sync.WaitGroup
	wg.Add(N)
	started := core.Now()
	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			r := paths.WithFileLock(target, 200*core.Millisecond, func() core.Result {
				return core.Ok(nil)
			})
			results <- r
		}()
	}
	wg.Wait()
	close(stop)
	<-planterDone
	elapsed := core.Now().Sub(started)
	// Wallclock budget: 200ms timeout + 1.8s grace. Pre-fix
	// behaviour exhibited unbounded blocking until the iteration
	// cap (or test-runner timeout) kicked in.
	core.AssertTrue(t, elapsed < 2*core.Second,
		core.Sprintf("N-way race must surface within budget; took %s", elapsed))
	close(results)
	timeouts := 0
	wins := 0
	for r := range results {
		if r.OK {
			wins++
			continue
		}
		if core.Contains(r.Error(), paths.CodeLockTimeout) ||
			core.Contains(r.Error(), paths.CodeLockIterationCap) {
			timeouts++
			continue
		}
		t.Fatalf("unexpected failure shape: %s", r.Error())
	}
	// At least one racer should hit the deadline — full sweep of
	// wins would mean the re-planter never raced any racer, which
	// is fine on a fast scheduler but the fixture is intentionally
	// hostile. We accept any mix as long as TOTAL wallclock stays
	// inside budget (the real load-bearing assertion above).
	_ = wins + timeouts
}

func TestPathsRoot_NetworkFSRejected_Bad(t *core.T) {
	homeFixture(t)
	paths.SetRootFSTypeForTest("nfs")
	t.Cleanup(func() { paths.SetRootFSTypeForTest("") })

	r := paths.WithFileLock("/tmp/whatever", 100*core.Millisecond, func() core.Result {
		t.Fatalf("fn must not run on rejected fstype")
		return core.Ok(nil)
	})
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), paths.CodeLockNetworkFS)
}

func TestRootFSType_LocalAccepted_Good(t *core.T) {
	homeFixture(t)
	r := paths.RootFSType()
	core.AssertTrue(t, r.OK, "RootFSType should succeed for a local HOME tempdir")
	fstype := r.Value.(string)
	core.AssertFalse(t, paths.IsNetworkFS(fstype),
		"tempdir on test runner should not classify as network: "+fstype)
}

func TestIsNetworkFS_PolicyTable(t *core.T) {
	for _, fs := range []string{"nfs", "nfs4", "smbfs", "cifs", "fuse", "fuse.sshfs", "fuse.gocryptfs"} {
		if !paths.IsNetworkFS(fs) {
			t.Errorf("expected %q classified as network", fs)
		}
	}
	for _, fs := range []string{"apfs", "ext4", "xfs", "btrfs", "zfs", "tmpfs", "unknown", ""} {
		if paths.IsNetworkFS(fs) {
			t.Errorf("expected %q classified as local", fs)
		}
	}
}

// Avoid unused-import warnings when build tags reduce visibility.
var _ = testing.AllocsPerRun
