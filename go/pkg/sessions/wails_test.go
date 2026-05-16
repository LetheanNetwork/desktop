// SPDX-Licence-Identifier: EUPL-1.2

package sessions_test

import (
	core "dappco.re/go"
	"dappco.re/go/inference"
	"dappco.re/lthn/desktop/pkg/sessions"
)

// Mantis #1418 — Cerberus M2 2026-05-16. Race tests for the per-
// session mutex on WailsService. Run with `go test -race` to detect
// data races; without -race the test still asserts the manifest
// counter matches the actual message-log length after concurrent
// writes — the invariant the lock protects.

// TestWails_Append_RaceFreeConcurrent — 100 concurrent Append on the
// same session id must produce manifest.Messages == 100 and the
// message log must hold 100 entries. Without the per-id mutex, the
// readManifest → mutate → writeManifest RMW window drops updates.
func TestWails_Append_RaceFreeConcurrent(t *core.T) {
	c := coreFixture(t)
	svc := sessions.NewWailsService(c)
	id := svc.Create("race-target").Value.(string)

	const N = 100
	done := make(chan struct{}, N)
	for i := 0; i < N; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			svc.Append(id, "user", "m")
		}()
	}
	for i := 0; i < N; i++ {
		<-done
	}

	// Manifest counter must match the actual log length.
	infos := svc.List().Value.([]sessions.SessionInfo)
	var target sessions.SessionInfo
	for _, info := range infos {
		if info.ID == id {
			target = info
			break
		}
	}
	core.AssertEqual(t, N, target.Messages,
		"manifest.Messages must equal the number of Append calls")

	// Cross-check the actual message log.
	read := svc.Read(id)
	core.AssertTrue(t, read.OK)
	msgs := read.Value.([]inference.Message)
	core.AssertEqual(t, N, len(msgs), "message log length must match Append count")
}

// TestWails_SetTags_RaceFreeConcurrent — 50 concurrent SetTags +
// Append alternating on the same id. Final state: manifest reads
// without panic, tags slice is well-formed, message log matches the
// number of Appends. Without the lock, an interleaved SetTags +
// Append corrupts info.Messages vs the actual log.
func TestWails_SetTags_RaceFreeConcurrent(t *core.T) {
	c := coreFixture(t)
	svc := sessions.NewWailsService(c)
	id := svc.Create("tag-race").Value.(string)

	const N = 50
	done := make(chan struct{}, N*2)
	for i := 0; i < N; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			svc.SetTags(id, []string{"a", "b"})
		}()
		go func() {
			defer func() { done <- struct{}{} }()
			svc.Append(id, "user", "x")
		}()
	}
	for i := 0; i < N*2; i++ {
		<-done
	}

	infos := svc.List().Value.([]sessions.SessionInfo)
	var target sessions.SessionInfo
	for _, info := range infos {
		if info.ID == id {
			target = info
			break
		}
	}
	// Tags should be the canonical set [a, b] after every SetTags
	// (idempotent) — proves SetTags didn't half-write.
	core.AssertEqual(t, 2, len(target.Tags))
	core.AssertEqual(t, "a", target.Tags[0])
	core.AssertEqual(t, "b", target.Tags[1])
	// Counter ↔ log invariant.
	core.AssertEqual(t, N, target.Messages)
	msgs := svc.Read(id).Value.([]inference.Message)
	core.AssertEqual(t, N, len(msgs))
}

// TestWails_RMW_DifferentSessionsUncontended — verifies the
// per-id mutex doesn't serialise unrelated sessions. Two sessions
// each take 100 Appends; total wall-clock should be roughly
// max(per-session) not sum-of-sessions. We don't measure timing
// (flaky on CI) — instead we assert that BOTH session counters
// reach N, proving no cross-session deadlock + correct final state.
func TestWails_RMW_DifferentSessionsUncontended(t *core.T) {
	c := coreFixture(t)
	svc := sessions.NewWailsService(c)
	idA := svc.Create("A").Value.(string)
	idB := svc.Create("B").Value.(string)

	const N = 100
	done := make(chan struct{}, N*2)
	for i := 0; i < N; i++ {
		go func() { defer func() { done <- struct{}{} }(); svc.Append(idA, "user", "a") }()
		go func() { defer func() { done <- struct{}{} }(); svc.Append(idB, "user", "b") }()
	}
	for i := 0; i < N*2; i++ {
		<-done
	}

	infos := svc.List().Value.([]sessions.SessionInfo)
	got := map[string]int{}
	for _, info := range infos {
		got[info.ID] = info.Messages
	}
	core.AssertEqual(t, N, got[idA], "session A counter ok")
	core.AssertEqual(t, N, got[idB], "session B counter ok")
}
