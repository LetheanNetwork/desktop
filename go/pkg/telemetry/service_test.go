// SPDX-Licence-Identifier: EUPL-1.2

// Tests for the runtime telemetry sampler. Sample() is portable + pure-ish
// (reads runtime.MemStats), so the assertions are shape-based: every
// field has a sane non-negative value and the uptime moves forward
// across successive calls.

package telemetry_test

import (

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/telemetry"
)

func TestTelemetry_Sample_Good_ReadingShape(t *core.T) {
	r := telemetry.Sample()
	core.AssertTrue(t, r.OK)
	rd := r.Value.(telemetry.Reading)

	// Every memory metric should be positive — we're alive and using
	// at least a few KB.
	core.AssertGreater(t, rd.HeapAllocMB, 0.0)
	core.AssertGreater(t, rd.HeapSysMB, 0.0)
	core.AssertGreaterOrEqual(t, rd.StackInUseMB, 0.0)

	// Goroutine + CGO counts are runtime-level — at least one
	// goroutine (the test) is running.
	core.AssertGreaterOrEqual(t, rd.NumGoroutines, 1)
	core.AssertGreaterOrEqual(t, rd.NumCgoCalls, int64(0))

	// Uptime + GC counts non-negative.
	core.AssertGreaterOrEqual(t, rd.UptimeSeconds, 0.0)
	core.AssertGreaterOrEqual(t, int(rd.NumGC), 0)
	core.AssertGreaterOrEqual(t, rd.LastGCPauseMs, 0.0)

	// Power readings are placeholders today — zero is the documented
	// pre-XPC behaviour.
	core.AssertEqual(t, 0.0, rd.WattsActive)
	core.AssertEqual(t, 0.0, rd.WattsIdle)
}

func TestTelemetry_Sample_Good_UptimeMonotonic(t *core.T) {
	r1 := telemetry.Sample().Value.(telemetry.Reading)
	core.Sleep(5 * core.Millisecond)
	r2 := telemetry.Sample().Value.(telemetry.Reading)
	core.AssertGreater(t, r2.UptimeSeconds, r1.UptimeSeconds, "uptime must move forward across samples")
}

func TestTelemetry_NewService_Good_DefaultOptions(t *core.T) {
	svc := telemetry.NewService(telemetry.Options{})
	core.AssertNotNil(t, svc)
	r := svc.Register(core.New())
	core.AssertTrue(t, r.OK)
}

func TestTelemetry_Register_Good(t *core.T) {
	c := core.New()
	core.AssertNotNil(t, c)
	r := telemetry.Register(c)
	core.AssertTrue(t, r.OK, "Register on a fresh Core should succeed")
}

func TestTelemetry_Register_Bad_NilCore(t *core.T) {
	svc := telemetry.NewService(telemetry.Options{})
	r := svc.Register(nil)
	core.AssertFalse(t, r.OK, "Register(nil) must Fail")
}
