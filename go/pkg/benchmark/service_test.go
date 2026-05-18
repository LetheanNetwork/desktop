// SPDX-Licence-Identifier: EUPL-1.2

package benchmark_test

import (
	core "dappco.re/go"
	"dappco.re/go/orm"
	"dappco.re/lthn/desktop/pkg/benchmark"
)

// newTestCore returns a *core.Core with orm registered + a fresh
// Memium mounted under "default" + the benchmark schema registered.
// Tests use this to exercise the substrate end-to-end without
// DuckDB on the filesystem.
func newTestCore(t *core.T) *core.Core {
	t.Helper()
	c := core.New()
	core.RequireTrue(t, orm.Register(c).OK)
	mem := orm.NewMemium()
	core.RequireTrue(t, orm.Mount(c, "default", mem).OK)
	for _, schema := range benchmark.Schemas() {
		core.RequireTrue(t, orm.RegisterSchema(c, schema).OK)
		mem.RegisterTable(schema.Name, schema)
	}
	return c
}

func newRegisteredService(t *core.T) *benchmark.Service {
	t.Helper()
	c := newTestCore(t)
	svc := benchmark.NewService(benchmark.Options{})
	core.RequireTrue(t, svc.Register(c).OK)
	return svc
}

// Bencher registry --------------------------------------------------

func TestRegisterBencher_RoundTrip(t *core.T) {
	svc := newRegisteredService(t)
	fx := &benchmark.FixtureBencher{BencherName: "fix-a", BencherKind: benchmark.KindLocal}
	core.RequireTrue(t, svc.RegisterBencher(fx).OK)

	r := svc.ListBenchers()
	core.RequireTrue(t, r.OK)
	infos := r.Value.([]benchmark.BencherInfo)
	core.AssertEqual(t, 1, len(infos))
	core.AssertEqual(t, "fix-a", infos[0].Name)
	core.AssertEqual(t, benchmark.KindLocal, infos[0].Kind)
}

func TestRegisterBencher_RejectsDuplicate(t *core.T) {
	svc := newRegisteredService(t)
	fx := &benchmark.FixtureBencher{BencherName: "dup", BencherKind: benchmark.KindLocal}
	core.RequireTrue(t, svc.RegisterBencher(fx).OK)
	core.AssertFalse(t, svc.RegisterBencher(&benchmark.FixtureBencher{BencherName: "dup", BencherKind: benchmark.KindRemoteHTTP}).OK)
}

func TestListBenchers_OrderIsRegistration(t *core.T) {
	svc := newRegisteredService(t)
	for _, n := range []string{"a", "b", "c"} {
		core.RequireTrue(t, svc.RegisterBencher(&benchmark.FixtureBencher{BencherName: n, BencherKind: benchmark.KindLocal}).OK)
	}
	r := svc.ListBenchers()
	infos := r.Value.([]benchmark.BencherInfo)
	core.AssertEqual(t, "a", infos[0].Name)
	core.AssertEqual(t, "b", infos[1].Name)
	core.AssertEqual(t, "c", infos[2].Name)
}

// Storage round-trip -------------------------------------------------

func TestRecord_AutoFillsIDAndTimestamp(t *core.T) {
	svc := newRegisteredService(t)
	r := svc.Record(benchmark.Run{
		Bencher: "manual", Model: "gemma-4-e2b", Ctx: 2048,
		PpTokSec: 4800, TgTokSec: 47.2,
		PromptLen: 1024, OutputLen: 256,
	})
	core.RequireTrue(t, r.OK)
	stored := r.Value.(benchmark.Run)
	core.AssertNotEqual(t, "", stored.ID)
	core.AssertFalse(t, stored.Timestamp.IsZero())
	core.AssertEqual(t, "manual", stored.Bencher)
}

func TestRecord_PreservesExplicitID(t *core.T) {
	svc := newRegisteredService(t)
	r := svc.Record(benchmark.Run{
		ID: "fixed-id-x", Bencher: "m", Model: "x", Ctx: 1024,
		PpTokSec: 100, TgTokSec: 10, PromptLen: 512, OutputLen: 128,
	})
	core.RequireTrue(t, r.OK)
	core.AssertEqual(t, "fixed-id-x", r.Value.(benchmark.Run).ID)

	// Round-trip via Get
	g := svc.Get("fixed-id-x")
	core.RequireTrue(t, g.OK)
	core.AssertEqual(t, "fixed-id-x", g.Value.(benchmark.Run).ID)
}

func TestRecord_ExtraRoundTripsThroughJSON(t *core.T) {
	svc := newRegisteredService(t)
	extra := map[string]any{
		"billed_cost":  0.0042,
		"region":       "eu-west-1",
		"gpu":          "H100",
		"queue_ms":     45,
	}
	r := svc.Record(benchmark.Run{
		Bencher: "remote", Model: "llama-3-70b", Ctx: 8192,
		PpTokSec: 8500, TgTokSec: 130, PromptLen: 6400, OutputLen: 256,
		Endpoint: "https://api.example/v1",
		Extra:    extra,
	})
	core.RequireTrue(t, r.OK)
	stored := r.Value.(benchmark.Run)

	g := svc.Get(stored.ID)
	core.RequireTrue(t, g.OK)
	got := g.Value.(benchmark.Run)
	core.AssertEqual(t, "eu-west-1", got.Extra["region"])
	// JSON Unmarshal of numeric → float64; tolerate
	if v, ok := got.Extra["queue_ms"].(float64); !ok || v != 45 {
		t.Errorf("Extra[queue_ms]: want 45 (float64), got %v (%T)", got.Extra["queue_ms"], got.Extra["queue_ms"])
	}
}

func TestGet_MissingFails(t *core.T) {
	svc := newRegisteredService(t)
	core.AssertFalse(t, svc.Get("nonexistent").OK)
}

func TestGet_EmptyIDFails(t *core.T) {
	svc := newRegisteredService(t)
	core.AssertFalse(t, svc.Get("").OK)
}

func TestHistory_NewestFirst(t *core.T) {
	svc := newRegisteredService(t)
	now := core.Now().UTC()
	for i, ts := range []core.Time{now.Add(-3 * core.Hour), now.Add(-2 * core.Hour), now.Add(-1 * core.Hour)} {
		core.RequireTrue(t, svc.Record(benchmark.Run{
			ID: core.Sprintf("r%d", i), Timestamp: ts,
			Bencher: "x", Model: "m", Ctx: 1024,
			PpTokSec: 100, TgTokSec: 10, PromptLen: 512, OutputLen: 128,
		}).OK)
	}
	r := svc.History(benchmark.Filter{Limit: 10})
	core.RequireTrue(t, r.OK)
	runs := r.Value.([]benchmark.Run)
	core.AssertEqual(t, 3, len(runs))
	core.AssertEqual(t, "r2", runs[0].ID) // newest
	core.AssertEqual(t, "r0", runs[2].ID) // oldest
}

func TestHistory_FilterByBencher(t *core.T) {
	svc := newRegisteredService(t)
	for _, name := range []string{"a", "a", "b"} {
		core.RequireTrue(t, svc.Record(benchmark.Run{
			Bencher: name, Model: "m", Ctx: 1024,
			PpTokSec: 100, TgTokSec: 10, PromptLen: 512, OutputLen: 128,
		}).OK)
	}
	r := svc.History(benchmark.Filter{Bencher: "a"})
	runs := r.Value.([]benchmark.Run)
	core.AssertEqual(t, 2, len(runs))
	for _, run := range runs {
		core.AssertEqual(t, "a", run.Bencher)
	}
}

func TestHistory_FilterByModelAndCtxRange(t *core.T) {
	svc := newRegisteredService(t)
	cases := []struct {
		model string
		ctx   int
	}{
		{"gemma-4-e2b", 1024},
		{"gemma-4-e2b", 2048},
		{"gemma-4-e2b", 8192},
		{"llama-3-3b", 2048},
	}
	for _, ctx := range cases {
		core.RequireTrue(t, svc.Record(benchmark.Run{
			Bencher: "x", Model: ctx.model, Ctx: ctx.ctx,
			PpTokSec: 1, TgTokSec: 1, PromptLen: 1, OutputLen: 1,
		}).OK)
	}
	r := svc.History(benchmark.Filter{Model: "gemma-4-e2b", MinCtx: 2048, MaxCtx: 4096})
	runs := r.Value.([]benchmark.Run)
	core.AssertEqual(t, 1, len(runs))
	core.AssertEqual(t, 2048, runs[0].Ctx)
}

// Dispatch -----------------------------------------------------------

func TestBench_DispatchesAndPersists(t *core.T) {
	svc := newRegisteredService(t)
	fx := &benchmark.FixtureBencher{
		BencherName: "fix",
		BencherKind: benchmark.KindLocal,
		Canned: []benchmark.Run{{PpTokSec: 4800, TgTokSec: 47.2, PromptLen: 1024, OutputLen: 256}},
	}
	core.RequireTrue(t, svc.RegisterBencher(fx).OK)

	r := svc.Bench(core.Background(), benchmark.Bench{Model: "gemma-4-e2b", Ctx: 2048}, "fix")
	core.RequireTrue(t, r.OK)
	run := r.Value.(benchmark.Run)
	core.AssertEqual(t, "fix", run.Bencher) // substrate stamped attribution
	core.AssertEqual(t, "gemma-4-e2b", run.Model)
	core.AssertEqual(t, 2048, run.Ctx)
	core.AssertNotEqual(t, "", run.ID)
	core.AssertFalse(t, run.Timestamp.IsZero())

	// Persisted in History.
	h := svc.History(benchmark.Filter{})
	core.AssertEqual(t, 1, len(h.Value.([]benchmark.Run)))
}

func TestBench_RejectsUnknownBencher(t *core.T) {
	svc := newRegisteredService(t)
	core.AssertFalse(t, svc.Bench(core.Background(), benchmark.Bench{Model: "m"}, "missing").OK)
}

// rejectorBencher always says it CanBench=false.
type rejectorBencher struct{}

func (rejectorBencher) Name() string                  { return "rejector" }
func (rejectorBencher) Kind() benchmark.Kind          { return benchmark.KindLocal }
func (rejectorBencher) CanBench(_ benchmark.Bench) bool { return false }
func (rejectorBencher) Models(_ core.Context) core.Result {
	return core.Ok([]string{})
}
func (rejectorBencher) Bench(_ core.Context, _ benchmark.Bench) core.Result {
	return core.Fail(core.E("rejector.Bench", "should not be reached", nil))
}

func TestBench_RespectsCanBenchFalse(t *core.T) {
	svc := newRegisteredService(t)
	core.RequireTrue(t, svc.RegisterBencher(rejectorBencher{}).OK)
	r := svc.Bench(core.Background(), benchmark.Bench{}, "rejector")
	core.AssertFalse(t, r.OK)
}

// Compare ------------------------------------------------------------

func TestCompare_ReturnsDeltas(t *core.T) {
	svc := newRegisteredService(t)
	a := svc.Record(benchmark.Run{
		ID: "A", Bencher: "x", Model: "m", Ctx: 2048,
		PpTokSec: 4000, TgTokSec: 40, PeakWatts: 8.0, PeakMemMB: 2000,
		PromptLen: 1024, OutputLen: 256,
	})
	core.RequireTrue(t, a.OK)
	b := svc.Record(benchmark.Run{
		ID: "B", Bencher: "x", Model: "m", Ctx: 2048,
		PpTokSec: 4500, TgTokSec: 45, PeakWatts: 9.0, PeakMemMB: 2200,
		PromptLen: 1024, OutputLen: 256,
	})
	core.RequireTrue(t, b.OK)

	r := svc.Compare("A", "B")
	core.RequireTrue(t, r.OK)
	d := r.Value.(benchmark.Diff)
	core.AssertEqual(t, 500.0, d.PpDelta)
	core.AssertEqual(t, 5.0, d.TgDelta)
	core.AssertEqual(t, 1.0, d.PeakWattsDelta)
	core.AssertEqual(t, 200.0, d.PeakMemMBDelta)
}

func TestCompare_MissingFails(t *core.T) {
	svc := newRegisteredService(t)
	core.AssertFalse(t, svc.Compare("missing", "also-missing").OK)
}

// Service-not-registered guards --------------------------------------

func TestUnregisteredService_Fails(t *core.T) {
	svc := benchmark.NewService(benchmark.Options{})
	core.AssertFalse(t, svc.Record(benchmark.Run{}).OK)
	core.AssertFalse(t, svc.Get("x").OK)
	core.AssertFalse(t, svc.History(benchmark.Filter{}).OK)
	core.AssertFalse(t, svc.Compare("a", "b").OK)
	core.AssertFalse(t, svc.Bench(core.Background(), benchmark.Bench{}, "x").OK)
}

// Models surface ------------------------------------------------------

func TestModelsForBencher_FromCannedRuns(t *core.T) {
	svc := newRegisteredService(t)
	fx := &benchmark.FixtureBencher{
		BencherName: "fix",
		BencherKind: benchmark.KindLocal,
		Canned: []benchmark.Run{
			{Model: "gemma-4-e2b"},
			{Model: "llama-3"},
			{Model: "gemma-4-e2b"}, // dup — should dedup
		},
	}
	core.RequireTrue(t, svc.RegisterBencher(fx).OK)
	r := svc.ModelsForBencher("fix")
	core.RequireTrue(t, r.OK)
	ids := r.Value.([]string)
	core.AssertEqual(t, 2, len(ids))
	core.AssertEqual(t, "gemma-4-e2b", ids[0])
	core.AssertEqual(t, "llama-3", ids[1])
}

func TestModelsForBencher_FromModelList(t *core.T) {
	svc := newRegisteredService(t)
	fx := &benchmark.FixtureBencher{
		BencherName: "fix",
		BencherKind: benchmark.KindLocal,
		ModelList:   []string{"explicit-a", "explicit-b"},
		Canned:      []benchmark.Run{{Model: "from-canned"}},
	}
	core.RequireTrue(t, svc.RegisterBencher(fx).OK)
	r := svc.ModelsForBencher("fix")
	core.RequireTrue(t, r.OK)
	ids := r.Value.([]string)
	// ModelList takes precedence; canned-derived list ignored.
	core.AssertEqual(t, 2, len(ids))
	core.AssertEqual(t, "explicit-a", ids[0])
}

func TestModelsForBencher_UnknownBencherFails(t *core.T) {
	svc := newRegisteredService(t)
	r := svc.ModelsForBencher("does-not-exist")
	core.AssertFalse(t, r.OK)
}

func TestModelsForBencher_EmptyNameFails(t *core.T) {
	svc := newRegisteredService(t)
	r := svc.ModelsForBencher("")
	core.AssertFalse(t, r.OK)
}

// Wails Service interface --------------------------------------------

func TestWailsServiceShape(t *core.T) {
	svc := benchmark.NewService(benchmark.Options{})
	core.AssertEqual(t, "Benchmark", svc.ServiceName())
	core.RequireTrue(t, svc.ServiceStartup(core.Background(), nil).OK)
	core.RequireTrue(t, svc.ServiceShutdown().OK)
}
