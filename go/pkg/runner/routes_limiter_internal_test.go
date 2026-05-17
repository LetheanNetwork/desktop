// SPDX-Licence-Identifier: EUPL-1.2

// Internal (same-package) tests for resolveOutboundLimiter — Mantis
// #1660 / Cerberus #45 ADD. The package-level outboundLimiter +
// outboundLimiterOnce pair is process-scoped; reaching into them from
// the runner_test package would need a re-export every test cycle, so
// the assertion lives next to the implementation.
//
// SECURITY-NOTE (Hephaestus surface): ResetOutboundLimiterForTests is
// the test-only seam exported in routes.go alongside the production
// resolveOutboundLimiter; documented test-only contract per AX-10
// (Taskfile path = command path; resetting global state outside test
// paths breaks the once-per-process Limiter sharing contract).

package runner

import (

	core "dappco.re/go"
)

// TestRunner_OutboundLimiter_InitFailure_Bad — when paths.DataDir()
// can't materialise the ~/Lethean/data/ subdir (HOME points at a
// regular file → MkdirAll fails inside the subdir), the resolve helper
// MUST return nil rather than silently propagating a half-built state.
// The fail-loud stderr line is observed by the operator at the boot
// log; this test pins the nil contract that the openai.Backend
// downstream treats as "no quota tracking".
//
// The historical bug returned nil on the SAME path but ALSO did so
// silently with no observable signal AND without serialising
// concurrent first-callers. Three failure modes folded into one
// regression test: serialise the resolve, return the canonical nil,
// emit the diagnostic.
func TestRunner_OutboundLimiter_InitFailure_Bad(t *core.T) {
	// Park the prior process-wide limiter so a previous test in the
	// same go-test binary doesn't poison the assertion.
	ResetOutboundLimiterForTests()
	defer ResetOutboundLimiterForTests()

	tmp := t.TempDir()
	// Make $HOME a regular file rather than a directory — MkdirAll
	// will fail trying to create the Lethean/data/ subtree.
	homeAsFile := core.PathJoin(tmp, "home-as-file")
	core.AssertTrue(t, core.WriteFile(homeAsFile, []byte{}, 0o600).OK,
		"fixture: HOME-as-file must write")
	t.Setenv("HOME", homeAsFile)

	got := resolveOutboundLimiter()
	core.AssertTrue(t, got == nil,
		"resolveOutboundLimiter must return nil when paths.DataDir() cannot resolve")
}

// TestRunner_OutboundLimiter_OnceSerialises_Good — concurrent first-
// callers MUST observe the same Limiter instance (the once-per-process
// contract that the backend sharing relies on). The historical race
// let two simultaneous first-callers each open SQLite, drop one open
// handle on the floor, and accept either-or as the "winning" pointer.
// The Once.Do collapses that to a single attempt. Drives the resolve
// from N goroutines via a WaitGroup-fenced fan-out so the assertion
// reflects post-race-detector state.
func TestRunner_OutboundLimiter_OnceSerialises_Good(t *core.T) {
	ResetOutboundLimiterForTests()
	defer ResetOutboundLimiterForTests()

	// Drive into the failure path (cheap + deterministic) so the test
	// neither requires a writable ~/Lethean/data/ nor opens N stray
	// SQLite handles. The Once contract is independent of the open
	// outcome — the assertion is "first caller's nil wins for everyone".
	tmp := t.TempDir()
	homeAsFile := core.PathJoin(tmp, "home-as-file")
	core.AssertTrue(t, core.WriteFile(homeAsFile, []byte{}, 0o600).OK,
		"fixture: HOME-as-file must write")
	t.Setenv("HOME", homeAsFile)

	const fanOut = 16
	results := make(chan bool, fanOut)
	var wg core.WaitGroup
	for i := 0; i < fanOut; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- (resolveOutboundLimiter() == nil)
		}()
	}
	wg.Wait()
	close(results)

	for r := range results {
		core.AssertTrue(t, r,
			"every concurrent caller must observe the same nil resolve")
	}
}
