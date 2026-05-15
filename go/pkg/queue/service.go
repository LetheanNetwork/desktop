// SPDX-Licence-Identifier: EUPL-1.2

// Service — queue's Core integration. Owns the worker goroutine
// lifecycle. OnStart spawns a single worker via c.waitGroup.Go;
// OnShutdown's responsibility is moot because the worker self-exits
// on c.shutdown.Load() and Core's ServiceShutdown drains the
// waitGroup before returning.

package queue

import (

	core "dappco.re/go"
)

// Options configures the queue Service at construction.
type Options struct {
	// PollInterval is how often the worker scans for ready jobs.
	// Smaller = lower latency from Enqueue → execution; larger =
	// fewer wakeups when the queue is idle. Default 1s.
	PollInterval core.Duration
}

// Service runs the queue worker loop. Embeds *core.ServiceRuntime[Options]
// for the canonical service-pattern (Mantis #1336).
type Service struct {
	*core.ServiceRuntime[Options]
}

// NewService is the canonical Core service factory. Pattern mirrors
// pkg/opencode and pkg/repos.
//
// Usage example:
//
//	core.WithName("queue", queue.NewService(queue.Options{
//	    PollInterval: core.Second,
//	}))
func NewService(opts Options) func(*core.Core) core.Result {
	return func(c *core.Core) core.Result {
		if opts.PollInterval <= 0 {
			opts.PollInterval = core.Second
		}
		svc := &Service{
			ServiceRuntime: core.NewServiceRuntime(c, opts),
		}
		return core.Ok(svc)
	}
}

// Register constructs the queue service for Core registration.
//
// Usage example:
//
//	core.New(core.WithName("queue", queue.Register))
func Register(c *core.Core) core.Result {
	return NewService(Options{})(c)
}

// ServiceName labels the binding namespace exposed to JS.
func (s *Service) ServiceName() string { return "Queue" }

// OnStart spawns the single worker goroutine. Worker self-exits
// when c.Context() cancels (Core's ServiceShutdown calls cancel
// before draining services).
//
// Note: Core's tracked-goroutine waitGroup is private to the core
// package, so this worker runs as a plain `go func` rather than
// being drained by ServiceShutdown. In-flight jobs at exit may end
// up orphaned (left in StatusRunning); reconcile-on-boot promotes
// them back to StatusPending — see Reconcile in worker.go (TBD #367
// follow-up). Public c.Go(fn) helper would close this gap cleanly;
// flagging as a CoreGO ergonomics gap.
//
// Multiple workers (per-kind concurrency caps) is a v2 polish that
// uses c.Lock("queue.kind.<name>").TryLock() — see worker.go's
// dispatch comment for the integration point.
func (s *Service) OnStart() core.Result {
	if s == nil || s.Core() == nil {
		return core.Fail(core.E("queue.OnStart", "service or core is nil", nil))
	}
	c := s.Core()
	interval := s.Options().PollInterval
	go runWorker(c, interval)
	return core.Ok(nil)
}

// OnStop is a no-op — Core's ServiceShutdown already flips
// c.shutdown.Load() (which the worker checks each tick) and
// drains c.waitGroup before returning. We don't duplicate
// that here.
func (s *Service) OnStop() core.Result {
	return core.Ok(nil)
}
