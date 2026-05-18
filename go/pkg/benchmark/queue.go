// SPDX-Licence-Identifier: EUPL-1.2

package benchmark

import (
	core "dappco.re/go"
	"dappco.re/go/orm"

	"dappco.re/lthn/desktop/pkg/auth"
	"dappco.re/lthn/desktop/pkg/queue"
)

// KindBenchmarkGPU is the queue kind name registered for benchmark
// dispatch. Per [[design_cooperative_task_queue]]: capture-greedy /
// execute-throttled — concurrent EnqueueBench calls land immediately
// (no GPU contention at the enqueue site), the queue's single worker
// processes them sequentially so two benchmarks never share the GPU.
//
// One canonical kind for now; future work may split (e.g. "benchmark.cpu"
// for remote-HTTP benchers that don't touch local GPU, letting them
// run concurrently with a local lthn-mlx benchmark).
const KindBenchmarkGPU = "benchmark.gpu"

// payload field names — kept as constants so the enqueuer + handler
// can't drift on the wire shape without a compile-time break.
const (
	payloadBencherName = "bencher_name"
	payloadModel       = "model"
	payloadPrompt      = "prompt"
	payloadCtx         = "ctx"
	payloadMaxOutput   = "max_output"
)

// BenchCompleted is the Core action emitted when the queue handler
// finishes a benchmark — whether it succeeded or failed. Subscribers
// (pkg/desktop's event bridge → frontend's Wails Events.On) use this
// to refresh History without polling.
//
// JobID is intentionally absent today: the queue worker doesn't stamp
// the JobID onto the dispatch context, and benchmark.gpu kind is
// serialised by-construction (single-worker / one-at-a-time) so the
// frontend doesn't need per-job correlation — every completion means
// "refresh History". When the queue grows multi-worker per-class
// support, JobID can land here without breaking subscribers.
//
// Usage example (subscriber side):
//
//	benchmark.SubscribeCompleted(c, func(_ *core.Core, ev benchmark.BenchCompleted) {
//	    emitCoreEvent(c, "benchmark:completed", ev)
//	})
type BenchCompleted struct {
	RunID   string    `json:"run_id,omitempty"`
	Bencher string    `json:"bencher"`
	Model   string    `json:"model"`
	OK      bool      `json:"ok"`
	Error   string    `json:"error,omitempty"`
	At      core.Time `json:"at"`
}

// SubscribeCompleted wires fn to be called for every BenchCompleted
// fired on the Core IPC bus. Used by pkg/desktop to bridge to a
// Wails event the WebView listens for.
//
// Usage example:
//
//	benchmark.SubscribeCompleted(c, func(_ *core.Core, ev benchmark.BenchCompleted) {
//	    emitCoreEvent(c, "benchmark:completed", ev)
//	})
func SubscribeCompleted(c *core.Core, fn func(*core.Core, BenchCompleted)) {
	if c == nil || fn == nil {
		return
	}
	c.RegisterAction(func(c *core.Core, msg core.Message) core.Result {
		if ev, ok := msg.(BenchCompleted); ok {
			fn(c, ev)
		}
		return core.Result{OK: true}
	})
}

// RegisterQueueHandler installs the "benchmark.gpu" handler on the
// queue substrate. The handler reads the request payload, calls
// Service.Bench (the synchronous direct-dispatch surface), persists
// the Run, and fires BenchCompleted on the Core IPC bus.
//
// PermittedTiers per RFC v3.1 §5.1: renderer (the user clicking Run
// via Wails), operator (CLI), internal (intra-Go callers / tests),
// cascade (future automation that re-runs benchmarks on cadence).
//
// Usage example (boot wire):
//
//	benchmarkSvc := benchmark.NewService(benchmark.Options{})
//	_ = benchmarkSvc.Register(c)
//	_ = benchmark.RegisterQueueHandler(c, benchmarkSvc)
func RegisterQueueHandler(c *core.Core, svc *Service) core.Result {
	if c == nil {
		return core.Fail(core.E("benchmark.RegisterQueueHandler", "core is nil", nil))
	}
	if svc == nil {
		return core.Fail(core.E("benchmark.RegisterQueueHandler", "service is nil", nil))
	}
	return queue.RegisterKind(c, queue.HandlerOptions{
		Kind:        KindBenchmarkGPU,
		Description: "Benchmark inference run — serialised per-GPU",
		PermittedTiers: []auth.CallerTier{
			auth.TierRenderer,
			auth.TierOperator,
			auth.TierInternal,
			auth.TierCascade,
		},
		Handler: func(ctx core.Context, opts core.Options) core.Result {
			req := Bench{
				Model:     opts.String(payloadModel),
				Prompt:    opts.String(payloadPrompt),
				Ctx:       opts.Int(payloadCtx),
				MaxOutput: opts.Int(payloadMaxOutput),
			}
			bencherName := opts.String(payloadBencherName)
			r := svc.Bench(ctx, req, bencherName)
			// Fire completion on the Core IPC bus regardless of success
			// so subscribers can update UI state in both branches.
			ev := BenchCompleted{
				Bencher: bencherName,
				Model:   req.Model,
				At:      core.Now().UTC(),
				OK:      r.OK,
			}
			if r.OK {
				if run, ok := r.Value.(Run); ok {
					ev.RunID = run.ID
				}
			} else {
				ev.Error = r.Error()
			}
			c.ACTION(ev)
			return r
		},
	})
}

// EnqueueBench enqueues a benchmark run for asynchronous dispatch via
// the queue substrate. Returns the Job record (whose ID the frontend
// uses to correlate with the BenchCompleted event). Renderer-tier
// callers (the WebView Run button) land here; the worker stamps
// TierCascade on the dispatch context before calling the handler.
//
// This is the Wails-facing dispatch surface. In-Go callers + tests
// that want synchronous Bench should call Service.Bench directly.
//
// Usage example (TS):
//
//	const job = await EnqueueBench({ Model: "llama3", Prompt: txt, Ctx: 2048 }, "ollama");
//	// Subscribe to "benchmark:completed" Wails event for job.ID
func (s *Service) EnqueueBench(req Bench, bencherName string) core.Result {
	if s.core == nil {
		return core.Fail(core.E("benchmark.EnqueueBench", "service not registered", nil))
	}
	if bencherName == "" {
		return core.Fail(core.E("benchmark.EnqueueBench", "bencher name is required", nil))
	}
	if req.Model == "" {
		return core.Fail(core.E("benchmark.EnqueueBench", "model is required", nil))
	}
	payload := core.NewOptions(
		core.Option{Key: payloadBencherName, Value: bencherName},
		core.Option{Key: payloadModel, Value: req.Model},
		core.Option{Key: payloadPrompt, Value: req.Prompt},
		core.Option{Key: payloadCtx, Value: req.Ctx},
		core.Option{Key: payloadMaxOutput, Value: req.MaxOutput},
	)
	return queue.Enqueue(s.core, KindBenchmarkGPU, payload)
}

// PendingBenchCount returns the number of pending + running benchmark
// jobs across all callers. Frontend uses this to render "N pending"
// status and gate the Run button across windows / agents that share
// the same Core.
//
// Returns 0 with OK=true when the queue is empty OR the orm bridge
// fails (the failure mode is "we don't know" — collapsing to 0 is
// the safer UX choice than reporting a fake number).
//
// Usage example:
//
//	r := svc.PendingBenchCount()
//	if r.OK { n := r.Value.(int64); _ = n }
func (s *Service) PendingBenchCount() core.Result {
	if s.core == nil {
		return core.Fail(core.E("benchmark.PendingBenchCount", "service not registered", nil))
	}
	pending := orm.Of[queue.Job](s.core).
		Where("kind", "=", KindBenchmarkGPU).
		Where("status", "=", queue.StatusPending).
		Count()
	running := orm.Of[queue.Job](s.core).
		Where("kind", "=", KindBenchmarkGPU).
		Where("status", "=", queue.StatusRunning).
		Count()
	var total int64
	if pending.OK {
		if v, ok := pending.Value.(int64); ok {
			total += v
		}
	}
	if running.OK {
		if v, ok := running.Value.(int64); ok {
			total += v
		}
	}
	return core.Ok(total)
}
