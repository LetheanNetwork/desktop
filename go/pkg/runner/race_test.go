// SPDX-Licence-Identifier: EUPL-1.2

// Cerberus #45 / Mantis #1656 — race tests for the router pointer.
// Pre-fix: SetDynamicRoutes mutated s.router while Generate / Chat /
// Models dereferenced it from inference goroutines. The fix:
//   1. routerMu (core.RWMutex) guards every router read + write
//   2. SetDynamicRoutes renamed to package-level ApplyDynamicRoutes
//      so Wails3's method-set reflection no longer binds it onto
//      the renderer surface (untrusted frontend can't trigger the
//      mutation path at all)
//
// These tests must be run with `go test -race` to catch any
// regression. Run pattern: `go test -count=10 -race
// ./pkg/runner/...`.

package runner_test

import (
	"reflect"

	core "dappco.re/go"
	"dappco.re/go/inference"
	"dappco.re/go/inference/agent/ai"

	"dappco.re/lthn/desktop/pkg/runner"
)

// TestRunner_ApplyDynamicRoutes_NoRaceWithGenerate_Ugly hammers
// Generate / Chat / Models from N reader goroutines while a writer
// goroutine flips ApplyDynamicRoutes between two route sets. With
// -race, any unsynchronised access to s.router fails the test.
//
// Pre-fix: s.router = router in SetDynamicRoutes races with the
// `if s.router == nil` check in Generate / Chat / Models.
func TestRunner_ApplyDynamicRoutes_NoRaceWithGenerate_Ugly(t *core.T) {
	s := runner.NewService(runner.Options{})

	const readers = 8
	const iters = 200

	done := make(chan struct{}, readers+1)

	// Readers — drive every router-touching surface concurrently.
	for i := 0; i < readers; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			for j := 0; j < iters; j++ {
				_ = s.Generate("probe")
				_ = s.Chat([]inference.Message{
					{Role: "user", Content: "probe"},
				})
				_ = s.Models()
			}
		}()
	}

	// Writer — flip the router between nil + a fake route. The fake
	// route is constructed with a no-op model so NewProviderRouter
	// succeeds without touching the network.
	go func() {
		defer func() { done <- struct{}{} }()
		empty := []ai.ProviderRoute{}
		// We use empty + empty so ApplyDynamicRoutes hits both the
		// "clear router" branch (len(merged)==0 → s.router=nil) and
		// the constructor branch. We don't need a real provider for
		// the race detector to do its job.
		for j := 0; j < iters; j++ {
			_ = runner.ApplyDynamicRoutes(s, empty)
		}
	}()

	for i := 0; i < readers+1; i++ {
		<-done
	}
}

// TestRunner_ApplyDynamicRoutes_NotExposedToWails_Good asserts the
// Cerberus #45 prescription: SetDynamicRoutes is no longer a method
// on *runner.Service, so Wails3's method-set reflection cannot bind
// it onto the renderer surface. Any reintroduction of a public
// mutation method on the Wails-bound type fails this test.
//
// The lint is intentionally narrow — it checks for the literal name
// "SetDynamicRoutes" plus any other method whose name signals
// router mutation. Internal package callers continue to use
// runner.ApplyDynamicRoutes (a package-level function) which Wails
// does not bind.
func TestRunner_ApplyDynamicRoutes_NotExposedToWails_Good(t *core.T) {
	s := runner.NewService(runner.Options{})
	typ := reflect.TypeOf(s)

	bannedMethodNames := []string{
		"SetDynamicRoutes",
		"SetRoutes",
		"ReplaceRoutes",
		"AddRoute",
		"RemoveRoute",
	}
	for i := 0; i < typ.NumMethod(); i++ {
		name := typ.Method(i).Name
		for _, banned := range bannedMethodNames {
			core.AssertFalse(t, name == banned,
				"Wails-bound *runner.Service must not expose router-"+
					"mutation method "+name+" — use package-level "+
					"runner.ApplyDynamicRoutes from Go callers instead")
		}
	}
}
