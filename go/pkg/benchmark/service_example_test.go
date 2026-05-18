// SPDX-Licence-Identifier: EUPL-1.2

package benchmark_test

import (
	core "dappco.re/go"
	"dappco.re/go/orm"
	"dappco.re/lthn/desktop/pkg/benchmark"
)

// Example shows the canonical boot path: register the service against
// Core, register a Bencher adapter, run a query.
func Example() {
	c := core.New()
	_ = orm.Register(c)
	mem := orm.NewMemium()
	_ = orm.Mount(c, "default", mem)
	for _, schema := range benchmark.Schemas() {
		_ = orm.RegisterSchema(c, schema)
		mem.RegisterTable(schema.Name, schema)
	}

	svc := benchmark.NewService(benchmark.Options{})
	_ = svc.Register(c)
	_ = svc.RegisterBencher(&benchmark.FixtureBencher{
		BencherName: "fixture-local",
		BencherKind: benchmark.KindLocal,
		BencherDesc: "in-process fixture for demos + tests",
	})

	infos := svc.ListBenchers().Value.([]benchmark.BencherInfo)
	core.Println(infos[0].Name, "·", string(infos[0].Kind))
	// Output: fixture-local · local
}

// ExampleService_Bench shows dispatching a request through a
// registered Bencher and reading the persisted Run.
func ExampleService_Bench() {
	c := core.New()
	_ = orm.Register(c)
	mem := orm.NewMemium()
	_ = orm.Mount(c, "default", mem)
	for _, schema := range benchmark.Schemas() {
		_ = orm.RegisterSchema(c, schema)
		mem.RegisterTable(schema.Name, schema)
	}

	svc := benchmark.NewService(benchmark.Options{})
	_ = svc.Register(c)
	_ = svc.RegisterBencher(&benchmark.FixtureBencher{
		BencherName: "fixture-local",
		BencherKind: benchmark.KindLocal,
		Canned: []benchmark.Run{{PpTokSec: 4800, TgTokSec: 47.2, PromptLen: 1024, OutputLen: 256}},
	})

	r := svc.Bench(c.Context(), benchmark.Bench{Model: "gemma-4-e2b", Ctx: 2048}, "fixture-local")
	run := r.Value.(benchmark.Run)
	core.Println(run.Bencher, run.Model, run.TgTokSec)
	// Output: fixture-local gemma-4-e2b 47.2
}

// ExampleService_History shows the History query with a Filter.
func ExampleService_History() {
	c := core.New()
	_ = orm.Register(c)
	mem := orm.NewMemium()
	_ = orm.Mount(c, "default", mem)
	for _, schema := range benchmark.Schemas() {
		_ = orm.RegisterSchema(c, schema)
		mem.RegisterTable(schema.Name, schema)
	}

	svc := benchmark.NewService(benchmark.Options{})
	_ = svc.Register(c)
	_ = svc.Record(benchmark.Run{
		Bencher: "ollama-localhost", Model: "gemma-4-e2b", Ctx: 2048,
		PpTokSec: 3200, TgTokSec: 32, PromptLen: 1024, OutputLen: 256,
	})

	runs := svc.History(benchmark.Filter{Bencher: "ollama-localhost", Limit: 10}).Value.([]benchmark.Run)
	core.Println(len(runs), "·", runs[0].Model)
	// Output: 1 · gemma-4-e2b
}
