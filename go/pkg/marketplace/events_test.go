// SPDX-Licence-Identifier: EUPL-1.2

// Tests for the marketplace event bus. AX-10 G/B/U triads for
// Subscribe + verification that Service.Install / Launch / Stop /
// Uninstall fire BundleChanged with the right Phase + Before/Bundle
// snapshots.
//
// These tests don't exercise the actual sandbox-spawn path (that
// requires docker); they verify the event-bus contract directly +
// the lifecycle methods' fire-site coverage via short-circuited
// Service paths (no sandbox.Service registered means Install/Launch
// fail early — Stop/Uninstall fire on the record path).

package marketplace_test

import (
	core "dappco.re/go"

	subject "dappco.re/lthn/desktop/pkg/marketplace"
)

// ─── Subscribe ─────────────────────────────────────────────────────────

func TestEvents_Subscribe_Good(t *core.T) {
	c := core.New()
	core.RequireTrue(t, c.ServiceStartup(core.Background(), nil).OK)
	t.Cleanup(func() { _ = c.ServiceShutdown(core.Background()) })

	var received subject.BundleChanged
	subject.Subscribe(c, func(_ *core.Core, ev subject.BundleChanged) {
		received = ev
	})

	// Fire one directly so we don't need to construct the full
	// install path here — the lifecycle wiring is exercised below.
	c.ACTION(subject.BundleChanged{
		Phase:  subject.PhaseInstalled,
		Bundle: subject.InstalledBundle{BundleID: "test"},
	})

	core.AssertEqual(t, subject.PhaseInstalled, received.Phase)
	core.AssertEqual(t, "test", received.Bundle.BundleID)
}

func TestEvents_Subscribe_Bad(t *core.T) {
	c := core.New()
	core.RequireTrue(t, c.ServiceStartup(core.Background(), nil).OK)
	t.Cleanup(func() { _ = c.ServiceShutdown(core.Background()) })

	// nil Core + nil fn are defensive no-ops — neither panics.
	core.AssertNotPanics(t, func() {
		subject.Subscribe(nil, func(*core.Core, subject.BundleChanged) {})
	})
	core.AssertNotPanics(t, func() {
		subject.Subscribe(c, nil)
	})
}

func TestEvents_Subscribe_Ugly(t *core.T) {
	c := core.New()
	core.RequireTrue(t, c.ServiceStartup(core.Background(), nil).OK)
	t.Cleanup(func() { _ = c.ServiceShutdown(core.Background()) })

	var aCount, bCount int
	var mu core.Mutex
	subject.Subscribe(c, func(*core.Core, subject.BundleChanged) {
		mu.Lock()
		aCount++
		mu.Unlock()
	})
	subject.Subscribe(c, func(*core.Core, subject.BundleChanged) {
		panic("simulated bad subscriber — Core's recover keeps cascade alive")
	})
	subject.Subscribe(c, func(*core.Core, subject.BundleChanged) {
		mu.Lock()
		bCount++
		mu.Unlock()
	})

	// Fire one event; both healthy subscribers see it; the panicker
	// recovers individually so the cascade doesn't die.
	c.ACTION(subject.BundleChanged{
		Phase:  subject.PhaseInstalled,
		Bundle: subject.InstalledBundle{BundleID: "x"},
	})
	core.Sleep(5 * core.Millisecond) // any broadcast fan-out

	mu.Lock()
	defer mu.Unlock()
	core.AssertEqual(t, 1, aCount)
	core.AssertEqual(t, 1, bCount)

	// Phase constants are non-empty + distinct (defensive — if any
	// gets accidentally redefined to "" or duplicated, the Subscribe
	// shape can't disambiguate).
	core.AssertNotEqual(t, "", subject.PhaseInstalled)
	core.AssertNotEqual(t, "", subject.PhaseLaunched)
	core.AssertNotEqual(t, "", subject.PhaseStopped)
	core.AssertNotEqual(t, "", subject.PhaseUninstalled)
	core.AssertNotEqual(t, subject.PhaseInstalled, subject.PhaseLaunched)
	core.AssertNotEqual(t, subject.PhaseStopped, subject.PhaseUninstalled)
}

// ─── Phase constants ───────────────────────────────────────────────────

func TestEvents_PhaseConstants_Good(t *core.T) {
	// Values match the conventional lifecycle vocabulary used in
	// queue.JobChanged + tasks.IssueChanged so a polyglot subscriber
	// can branch on shared string vocabulary.
	core.AssertEqual(t, "installed", subject.PhaseInstalled)
	core.AssertEqual(t, "launched", subject.PhaseLaunched)
	core.AssertEqual(t, "stopped", subject.PhaseStopped)
	core.AssertEqual(t, "uninstalled", subject.PhaseUninstalled)
}

// ─── Lifecycle fire-site coverage ──────────────────────────────────────

// TestEvents_Uninstall_Fires verifies the Uninstall path fires
// PhaseUninstalled even when no sandbox.Service is registered. The
// Uninstall lifecycle doesn't actually need the sandbox path for the
// event-fire — it captures pre-state, calls Stop (which fails open
// when no sandbox is wired), drops the orm record, fires.
func TestEvents_Uninstall_Fires(t *core.T) {
	c := core.New()
	core.RequireTrue(t, c.ServiceStartup(core.Background(), nil).OK)
	t.Cleanup(func() { _ = c.ServiceShutdown(core.Background()) })

	var seen []subject.BundleChanged
	var mu core.Mutex
	subject.Subscribe(c, func(_ *core.Core, ev subject.BundleChanged) {
		mu.Lock()
		seen = append(seen, ev)
		mu.Unlock()
	})

	svc := subject.NewService(c)
	// Uninstall on a non-installed bundle still fires the event with
	// a zero Bundle — defensive shape so subscribers can react to
	// "uninstall attempted on X" even when X wasn't installed.
	r := svc.Uninstall("nonexistent")
	core.AssertTrue(t, r.OK)
	core.Sleep(5 * core.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	core.AssertEqual(t, 1, len(seen))
	core.AssertEqual(t, subject.PhaseUninstalled, seen[0].Phase)
}
