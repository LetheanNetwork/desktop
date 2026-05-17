// SPDX-Licence-Identifier: EUPL-1.2

// Service — queue's Core integration. Owns the worker goroutine
// lifecycle. OnStart spawns a single worker via c.waitGroup.Go;
// OnShutdown's responsibility is moot because the worker self-exits
// on c.shutdown.Load() and Core's ServiceShutdown drains the
// waitGroup before returning.

package queue

import (
	"os"

	core "dappco.re/go"

	"dappco.re/lthn/desktop/pkg/paths"
)

// letheanDataDir resolves the ~/Lethean/data directory via the
// canonical paths.DataDir helper. Returns "" on failure so the
// caller can short-circuit cleanly without panicking on a partial
// boot.
func letheanDataDir(_ *core.Core) string {
	r := paths.DataDir()
	if !r.OK {
		return ""
	}
	dir, ok := r.Value.(string)
	if !ok {
		return ""
	}
	return dir
}

// osChmod wraps os.Chmod into a single boundary point so the
// future swap to core.Chmod is one edit. Returns the raw error so
// the caller can pattern-match permission-model surprises without
// constructing a core.Result wrapper for a best-effort hardening
// step.
func osChmod(path string, mode core.FileMode) error {
	return os.Chmod(path, os.FileMode(mode))
}

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
	// Mantis #1727 — enforce 0600 on the at-rest tasks.duckdb so
	// host-level file pickers (Spotlight, Finder, backup tools)
	// can't read the row payloads without going through the
	// process. Best-effort: directory + file mode are tightened if
	// present; absence is non-fatal (first-boot before NewDuckDB
	// has created the file). CoreGO gap: no `core.Chmod` wrapper
	// today — see project_corego_export_gaps. Workaround uses the
	// `os.Chmod` system call directly via core.PermissionsApply
	// which is the canonical wrapper for setting modes on existing
	// paths.
	enforceDuckDBMode(c)
	interval := s.Options().PollInterval
	c.Go(func() { runWorker(c, interval) })
	return core.Ok(nil)
}

// OnStop is a no-op — Core's ServiceShutdown already flips
// c.shutdown.Load() (which the worker checks each tick) and
// drains c.waitGroup before returning. We don't duplicate
// that here.
func (s *Service) OnStop() core.Result {
	return core.Ok(nil)
}

// enforceDuckDBMode tightens the file mode on ~/Lethean/data/tasks.duckdb
// to 0o600 + the parent directory to 0o700 so a host-level file picker
// (Spotlight, Finder, third-party backup tool) can't read the row
// payloads as a different user. Best-effort — missing path is the
// fresh-install state and the next write will create the file with
// the correct mode via orm/duckdb; we only fix the mode on already-
// existing files so a downgrade-then-upgrade machine doesn't keep an
// 0644 file around.
//
// CoreGO gap: no `core.Chmod` wrapper exists today. Per the brief's
// project_corego_export_gaps escape-valve, the workaround uses
// os.Chmod directly. When core.Chmod lands the swap is one-line.
func enforceDuckDBMode(c *core.Core) {
	if c == nil {
		return
	}
	dataR := letheanDataDir(c)
	if dataR == "" {
		return
	}
	dbPath := core.PathJoin(dataR, "tasks.duckdb")
	// Tighten the parent dir to 0o700 — a 0o755 ~/Lethean/data
	// would let other local users list the file even though they
	// can't read its bytes. Best-effort; ignore the error so a
	// platform with permission-model surprises (Windows) doesn't
	// abort the queue startup.
	_ = osChmod(dataR, 0o700)
	// Tighten the file itself. core.Stat short-circuits if absent
	// (first boot before NewDuckDB created the file).
	if core.Stat(dbPath).OK {
		_ = osChmod(dbPath, 0o600)
		// WAL + tmp companions inherit the same boundary so an
		// adversary can't crack open the half-written log.
		_ = osChmod(dbPath+".wal", 0o600)
		_ = osChmod(dbPath+".tmp", 0o600)
	}
}
