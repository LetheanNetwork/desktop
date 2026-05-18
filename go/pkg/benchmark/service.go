// SPDX-Licence-Identifier: EUPL-1.2

package benchmark

import (
	core "dappco.re/go"
)

// Options configures the benchmark Service. All fields optional; the
// substrate ships sensible defaults so the canonical entry shape
// (`benchmark.NewService(benchmark.Options{}).Register(c)`) works
// without ceremony.
type Options struct {
	// Reserved for future configuration. Kept as a struct now so
	// adding fields later (e.g. DefaultBencher, MaxHistorySize)
	// does not break callers.
}

// Service is the benchmark substrate's Wails-facing entry point. It
// composes:
//
//   - the Bencher registry (Register / Lookup / List)
//   - the DuckDB-backed Run storage (Record / Get / History)
//   - the dispatch helper (Bench — looks up a registered Bencher,
//     calls its Bench method, persists the returned Run)
//
// It does NOT execute inference itself; that's a Bencher's job. The
// Service guarantees registration uniqueness, validates Kind values,
// and persists results so cross-bencher / cross-model analytics work
// regardless of which adapter produced the row.
//
// Lifecycle: NewService(opts) → Register(c). Register stamps the
// Wails-Service handle into Core (under name "Benchmark") and wires
// the Core action bus entry "benchmark.history" so non-Wails callers
// (CLI, MCP, future agents) can read history through Core actions
// without importing this package directly.
//
// Usage example:
//
//	svc := benchmark.NewService(benchmark.Options{})
//	if r := svc.Register(c); !r.OK { return r }
//	svc.RegisterBencher(myBencher)
//	r := svc.History(benchmark.Filter{Limit: 50})
type Service struct {
	core *core.Core
	opts Options
	reg  *bencherRegistry
}

// NewService constructs an unregistered Service. Call Register(c)
// after construction to wire it into Core.
//
// Usage example:
//
//	svc := benchmark.NewService(benchmark.Options{})
func NewService(opts Options) *Service {
	return &Service{
		opts: opts,
		reg:  newBencherRegistry(),
	}
}

// Register wires the Service onto the Core container per the
// canonical Mantis #1336 Service.go pattern. Stores the *core.Core
// reference so Wails-bound methods can dispatch to package-level
// orm helpers without each one needing the Core injected.
//
// The "benchmark.history" Core action is registered so non-Wails
// callers (CLI surfaces, MCP tool catalogue, future agents) can pull
// history through the action bus without importing benchmark.
//
// Usage example:
//
//	if r := svc.Register(c); !r.OK {
//	    return r
//	}
func (s *Service) Register(c *core.Core) core.Result {
	if c == nil {
		return core.Fail(core.E("benchmark.Register", "core is nil", nil))
	}
	s.core = c
	c.Action("benchmark.history", func(_ core.Context, _ core.Options) core.Result {
		return s.History(Filter{})
	})
	return core.Ok(nil)
}

// Register constructs a default Service and wires it into Core in
// one shot. The canonical entry point per Mantis #1336 for callers
// that don't need to retain the *Service handle.
//
// Usage example:
//
//	if r := benchmark.Register(c); !r.OK {
//	    return r
//	}
func Register(c *core.Core) core.Result {
	return NewService(Options{}).Register(c)
}

// ----- Bencher registry surface ----------------------------------------

// RegisterBencher adds a Bencher to the substrate. Rejects nil,
// empty-name, invalid-Kind, and duplicate-name registrations so two
// adapters cannot silently claim the same Run.Bencher identity.
//
// Usage example:
//
//	if r := svc.RegisterBencher(myBencher); !r.OK { return r }
func (s *Service) RegisterBencher(b Bencher) core.Result {
	return s.reg.register(b)
}

// UnregisterBencher removes a previously-registered Bencher by name.
// Idempotent: removing an unregistered name returns Ok. Used by the
// openaibench Settings UI Remove + reconfigure flows (delete-then-add
// as cheap Update). Once removed, EnqueueBench against that name
// fails clean and the picker drops the entry on the next refresh.
//
// Usage example:
//
//	if r := svc.UnregisterBencher("openai-compat:nim"); !r.OK { return r }
func (s *Service) UnregisterBencher(name string) core.Result {
	return s.reg.unregister(name)
}

// ListBenchers returns the metadata view of every registered Bencher
// in registration order. Frontend renders a picker / source filter
// against this; the runtime Bencher refs stay inside the substrate.
//
// Usage example:
//
//	r := svc.ListBenchers()
//	if r.OK { infos := r.Value.([]benchmark.BencherInfo); _ = infos }
func (s *Service) ListBenchers() core.Result {
	return core.Ok(s.reg.infos())
}

// ModelsForBencher returns the model IDs the named Bencher currently
// serves. Delegates to Bencher.Models. Lets the frontend populate a
// model picker without consumer packages needing direct adapter
// imports — same uniform surface across ollama / openai-compat /
// future llama-bench / lthn-mlx.
//
// Returns Fail when the name is not registered. Bencher.Models may
// itself return Fail when its discovery endpoint is unreachable; the
// substrate passes that through unchanged so callers see the real
// failure mode.
//
// Usage example:
//
//	r := svc.ModelsForBencher("ollama")
//	if r.OK { ids := r.Value.([]string); _ = ids }
func (s *Service) ModelsForBencher(bencherName string) core.Result {
	if bencherName == "" {
		return core.Fail(core.E("benchmark.ModelsForBencher", "bencher name is required", nil))
	}
	lookup := s.reg.bencher(bencherName)
	if !lookup.OK {
		return lookup
	}
	b := lookup.Value.(Bencher)
	ctx := core.Background()
	if s.core != nil {
		ctx = s.core.Context()
	}
	return b.Models(ctx)
}

// ----- Run storage surface ---------------------------------------------

// Record persists a Run produced outside the substrate's dispatch
// path. Callers that build a Run by hand (e.g. importing historical
// data, or a Bencher that wants to wrap several internal runs into
// one substrate-visible record) use this directly. Bencher
// implementations called via Bench should NOT call Record — the
// substrate handles persistence on their behalf.
//
// Auto-fills empty ID + zero Timestamp. Run.Bencher is the caller's
// responsibility and is NOT overwritten — Record is the import
// surface and must preserve historical attribution.
//
// Usage example:
//
//	r := svc.Record(run)
//	if r.OK { stored := r.Value.(benchmark.Run); _ = stored }
func (s *Service) Record(run Run) core.Result {
	if s.core == nil {
		return core.Fail(core.E("benchmark.Record", "service not registered (call Register first)", nil))
	}
	return insertRun(s.core, run)
}

// Get returns the Run identified by id. Fail when not found.
//
// Usage example:
//
//	r := svc.Get("01ABC...")
//	if r.OK { run := r.Value.(benchmark.Run); _ = run }
func (s *Service) Get(id string) core.Result {
	if s.core == nil {
		return core.Fail(core.E("benchmark.Get", "service not registered", nil))
	}
	if id == "" {
		return core.Fail(core.E("benchmark.Get", "id is required", nil))
	}
	return getRun(s.core, id)
}

// History returns Runs matching the filter, newest first. Empty
// filter returns the recent-history window (capped at the substrate's
// defaultListLimit). The Wails surface fronts this method as
// History(filter); the Core action "benchmark.history" reads it with
// an empty filter for CLI / MCP consumers.
//
// Usage example:
//
//	r := svc.History(benchmark.Filter{Model: "gemma-4-e2b", Limit: 50})
//	if r.OK { runs := r.Value.([]benchmark.Run); _ = runs }
func (s *Service) History(filter Filter) core.Result {
	if s.core == nil {
		return core.Fail(core.E("benchmark.History", "service not registered", nil))
	}
	return listRuns(s.core, filter)
}

// Compare diffs two Runs by id, returning a Diff value with both
// records embedded plus per-metric deltas (B - A). Fails fast if
// either id is missing.
//
// Usage example:
//
//	r := svc.Compare("01A...", "01B...")
//	if r.OK { d := r.Value.(benchmark.Diff); _ = d.TgDelta }
func (s *Service) Compare(idA, idB string) core.Result {
	if s.core == nil {
		return core.Fail(core.E("benchmark.Compare", "service not registered", nil))
	}
	if idA == "" || idB == "" {
		return core.Fail(core.E("benchmark.Compare", "both ids required", nil))
	}
	ra := getRun(s.core, idA)
	if !ra.OK {
		return ra
	}
	rb := getRun(s.core, idB)
	if !rb.OK {
		return rb
	}
	runA := ra.Value.(Run)
	runB := rb.Value.(Run)
	return core.Ok(Diff{
		IDA:            idA,
		IDB:            idB,
		RunA:           runA,
		RunB:           runB,
		PpDelta:        runB.PpTokSec - runA.PpTokSec,
		TgDelta:        runB.TgTokSec - runA.TgTokSec,
		PeakWattsDelta: runB.PeakWatts - runA.PeakWatts,
		PeakMemMBDelta: runB.PeakMemMB - runA.PeakMemMB,
	})
}

// ----- Dispatch -------------------------------------------------------

// Bench dispatches a Bench request to a registered Bencher and
// records the resulting Run. The substrate:
//
//  1. Looks up the named bencher in the registry; fail if missing.
//  2. Calls Bencher.CanBench(req); fail if false.
//  3. Calls Bencher.Bench(c, req) to get the result.
//  4. Overwrites Run.Bencher with bencher.Name() so adapters
//     cannot lie about attribution.
//  5. Persists via insertRun, which auto-fills ID + Timestamp.
//
// The persisted Run is returned via Result.Value so the caller can
// use it immediately (UI refresh, follow-up queries) without a
// second round-trip.
//
// Usage example:
//
//	req := benchmark.Bench{Model: "gemma-4-e2b", Prompt: stdPrompt, Ctx: 2048}
//	r := svc.Bench(c.Context(), req, "lthn-mlx")
//	if r.OK { run := r.Value.(benchmark.Run); _ = run }
func (s *Service) Bench(ctx core.Context, req Bench, bencherName string) core.Result {
	if s.core == nil {
		return core.Fail(core.E("benchmark.Bench", "service not registered", nil))
	}
	if bencherName == "" {
		return core.Fail(core.E("benchmark.Bench", "bencher name is required", nil))
	}
	lookup := s.reg.bencher(bencherName)
	if !lookup.OK {
		return lookup
	}
	b := lookup.Value.(Bencher)
	if !b.CanBench(req) {
		return core.Fail(core.E("benchmark.Bench", core.Concat("bencher ", bencherName, " cannot service request"), nil))
	}
	br := b.Bench(ctx, req)
	if !br.OK {
		return br
	}
	run, ok := br.Value.(Run)
	if !ok {
		return core.Fail(core.E("benchmark.Bench", core.Concat("bencher ", bencherName, " did not return Run"), nil))
	}
	run.Bencher = b.Name() // ENFORCE attribution; adapters cannot spoof
	return insertRun(s.core, run)
}

// ----- Wails3 Service shape -------------------------------------------
//
// Implements application.Service so Wails generates a TS binding
// at frontend/bindings/dappco.re/lthn/desktop/pkg/benchmark/service.ts.

// ServiceName labels the binding namespace exposed to JS.
func (s *Service) ServiceName() string { return "Benchmark" }

// ServiceStartup runs at app boot. No-op today; reserved for
// future hooks (cache warm, schema migration trigger, etc.).
func (s *Service) ServiceStartup(_ core.Context, _ any) core.Result {
	return core.Ok(nil)
}

// ServiceShutdown runs at app exit. No-op today.
func (s *Service) ServiceShutdown() core.Result { return core.Ok(nil) }
