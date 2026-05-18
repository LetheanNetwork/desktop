// SPDX-Licence-Identifier: EUPL-1.2

package benchmark_test

import (
	core "dappco.re/go"
	"dappco.re/go/orm"

	"dappco.re/lthn/desktop/pkg/benchmark"
	"dappco.re/lthn/desktop/pkg/queue"
)

// newCoreWithBenchmarkAndQueue extends newTestCore with queue schemas
// + a Service registration so EnqueueBench / PendingBenchCount have a
// live queue substrate to write into. The worker is NOT spawned —
// tests that need to observe handler-side effects invoke the
// registered Core action manually for deterministic timing.
func newCoreWithBenchmarkAndQueue(t *core.T) *core.Core {
	t.Helper()
	c := core.New()
	core.RequireTrue(t, orm.Register(c).OK)
	mem := orm.NewMemium()
	core.RequireTrue(t, orm.Mount(c, "default", mem).OK)
	for _, schema := range benchmark.Schemas() {
		core.RequireTrue(t, orm.RegisterSchema(c, schema).OK)
		mem.RegisterTable(schema.Name, schema)
	}
	for _, schema := range queue.Schemas() {
		core.RequireTrue(t, orm.RegisterSchema(c, schema).OK)
		mem.RegisterTable(schema.Name, schema)
	}
	return c
}

func TestEnqueueBench_CreatesPendingJob(t *core.T) {
	c := newCoreWithBenchmarkAndQueue(t)
	svc := benchmark.NewService(benchmark.Options{})
	core.RequireTrue(t, svc.Register(c).OK)
	core.RequireTrue(t, benchmark.RegisterQueueHandler(c, svc).OK)
	fx := &benchmark.FixtureBencher{BencherName: "fix", BencherKind: benchmark.KindLocal, ModelList: []string{"m"}}
	core.RequireTrue(t, svc.RegisterBencher(fx).OK)

	r := svc.EnqueueBench(benchmark.Bench{Model: "m", Ctx: 1024}, "fix")
	core.RequireTrue(t, r.OK)
	job, ok := r.Value.(queue.Job)
	core.RequireTrue(t, ok)
	core.AssertEqual(t, benchmark.KindBenchmarkGPU, job.Kind)
	core.AssertEqual(t, queue.StatusPending, job.Status)
	core.AssertNotEqual(t, "", job.ID)
}

func TestEnqueueBench_EmptyBencherFails(t *core.T) {
	c := newCoreWithBenchmarkAndQueue(t)
	svc := benchmark.NewService(benchmark.Options{})
	core.RequireTrue(t, svc.Register(c).OK)
	core.RequireTrue(t, benchmark.RegisterQueueHandler(c, svc).OK)
	r := svc.EnqueueBench(benchmark.Bench{Model: "m"}, "")
	core.AssertFalse(t, r.OK)
}

func TestEnqueueBench_EmptyModelFails(t *core.T) {
	c := newCoreWithBenchmarkAndQueue(t)
	svc := benchmark.NewService(benchmark.Options{})
	core.RequireTrue(t, svc.Register(c).OK)
	core.RequireTrue(t, benchmark.RegisterQueueHandler(c, svc).OK)
	r := svc.EnqueueBench(benchmark.Bench{}, "fix")
	core.AssertFalse(t, r.OK)
}

func TestEnqueueBench_UnregisteredServiceFails(t *core.T) {
	svc := benchmark.NewService(benchmark.Options{})
	r := svc.EnqueueBench(benchmark.Bench{Model: "m"}, "fix")
	core.AssertFalse(t, r.OK)
}

func TestPendingBenchCount_EmptyQueueIsZero(t *core.T) {
	c := newCoreWithBenchmarkAndQueue(t)
	svc := benchmark.NewService(benchmark.Options{})
	core.RequireTrue(t, svc.Register(c).OK)
	core.RequireTrue(t, benchmark.RegisterQueueHandler(c, svc).OK)
	r := svc.PendingBenchCount()
	core.RequireTrue(t, r.OK)
	core.AssertEqual(t, int64(0), r.Value.(int64))
}

func TestPendingBenchCount_OneAfterEnqueue(t *core.T) {
	c := newCoreWithBenchmarkAndQueue(t)
	svc := benchmark.NewService(benchmark.Options{})
	core.RequireTrue(t, svc.Register(c).OK)
	core.RequireTrue(t, benchmark.RegisterQueueHandler(c, svc).OK)
	fx := &benchmark.FixtureBencher{BencherName: "fix", BencherKind: benchmark.KindLocal, ModelList: []string{"m"}}
	core.RequireTrue(t, svc.RegisterBencher(fx).OK)
	core.RequireTrue(t, svc.EnqueueBench(benchmark.Bench{Model: "m"}, "fix").OK)
	r := svc.PendingBenchCount()
	core.RequireTrue(t, r.OK)
	core.AssertEqual(t, int64(1), r.Value.(int64))
}

func TestSubscribeCompleted_FiresOnSuccess(t *core.T) {
	c := newCoreWithBenchmarkAndQueue(t)
	svc := benchmark.NewService(benchmark.Options{})
	core.RequireTrue(t, svc.Register(c).OK)
	core.RequireTrue(t, benchmark.RegisterQueueHandler(c, svc).OK)
	fx := &benchmark.FixtureBencher{
		BencherName: "fix",
		BencherKind: benchmark.KindLocal,
		ModelList:   []string{"m"},
		Canned:      []benchmark.Run{{Model: "m", PpTokSec: 100, TgTokSec: 10, PromptLen: 64, OutputLen: 32}},
	}
	core.RequireTrue(t, svc.RegisterBencher(fx).OK)

	var captured benchmark.BenchCompleted
	var captureCount int
	benchmark.SubscribeCompleted(c, func(_ *core.Core, ev benchmark.BenchCompleted) {
		captured = ev
		captureCount++
	})

	// Invoke the registered handler synchronously via the action bus.
	// Bypasses the worker goroutine so the test deterministic.
	payload := core.NewOptions(
		core.Option{Key: "bencher_name", Value: "fix"},
		core.Option{Key: "model", Value: "m"},
		core.Option{Key: "ctx", Value: 1024},
	)
	r := c.Action("queue.kind.benchmark.gpu").Run(core.Background(), payload)
	core.RequireTrue(t, r.OK)

	core.AssertEqual(t, 1, captureCount)
	core.AssertTrue(t, captured.OK)
	core.AssertEqual(t, "fix", captured.Bencher)
	core.AssertEqual(t, "m", captured.Model)
	core.AssertNotEqual(t, "", captured.RunID)
}

func TestSubscribeCompleted_FiresOnFailure(t *core.T) {
	c := newCoreWithBenchmarkAndQueue(t)
	svc := benchmark.NewService(benchmark.Options{})
	core.RequireTrue(t, svc.Register(c).OK)
	core.RequireTrue(t, benchmark.RegisterQueueHandler(c, svc).OK)

	var captured benchmark.BenchCompleted
	benchmark.SubscribeCompleted(c, func(_ *core.Core, ev benchmark.BenchCompleted) {
		captured = ev
	})

	// No Bencher registered for "missing"; handler invokes Service.Bench,
	// which fails the registry lookup. Event should fire with OK=false.
	payload := core.NewOptions(
		core.Option{Key: "bencher_name", Value: "missing"},
		core.Option{Key: "model", Value: "m"},
		core.Option{Key: "ctx", Value: 1024},
	)
	_ = c.Action("queue.kind.benchmark.gpu").Run(core.Background(), payload)

	core.AssertFalse(t, captured.OK)
	core.AssertEqual(t, "missing", captured.Bencher)
	core.AssertNotEqual(t, "", captured.Error)
}
