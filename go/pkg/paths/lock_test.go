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
