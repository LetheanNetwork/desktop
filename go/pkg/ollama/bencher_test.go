// SPDX-Licence-Identifier: EUPL-1.2

package ollama_test

import (
	core "dappco.re/go"
	"dappco.re/go/orm"

	"dappco.re/lthn/desktop/pkg/benchmark"
	"dappco.re/lthn/desktop/pkg/ollama"
)

// newTestCoreWithBenchmark mirrors pkg/benchmark/service_test.go's
// helper: registers orm + mounts in-memory Memium + registers the
// benchmark schemas. Local to this test file to avoid an export from
// the benchmark package just for tests.
func newTestCoreWithBenchmark(t *core.T) *core.Core {
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

// fakeOllama returns a httptest server that serves the two endpoints
// the bencher cares about: /api/tags (CanBench) and /api/generate
// (Bench). Tests pass a custom Handler so they can shape the
// response per scenario.
func fakeOllama(handler core.HandlerFunc) *core.HTTPTestServer {
	return core.NewHTTPTestServer(core.HandlerFunc(handler))
}

// tagsBody is the canned response /api/tags returns when the
// daemon advertises one model. Mirrors ollama's wire shape closely
// enough that the bencher's CanBench can parse it.
const tagsBody = `{"models":[{"name":"llama3","size":1234,"digest":"abc"}]}`

// generateBody is the canned /api/generate response. eval_count +
// eval_duration produce a tg of (256 / (4s)) = 64 tok/s; prompt_eval
// produces 512 / 1s = 512 tok/s.
const generateBody = `{
  "model": "llama3",
  "response": "hello",
  "total_duration": 5000000000,
  "load_duration": 100000000,
  "prompt_eval_count": 512,
  "prompt_eval_duration": 1000000000,
  "eval_count": 256,
  "eval_duration": 4000000000
}`

func TestNewBencher_AppliesDefaults(t *core.T) {
	b := ollama.NewBencher(ollama.Options{})
	core.AssertEqual(t, ollama.DefaultName, b.Name())
	core.AssertEqual(t, benchmark.KindRemoteHTTP, b.Kind())
}

func TestNewBencher_HonoursName(t *core.T) {
	b := ollama.NewBencher(ollama.Options{Name: "ollama-laptop"})
	core.AssertEqual(t, "ollama-laptop", b.Name())
}

func TestDescribe_Default(t *core.T) {
	b := ollama.NewBencher(ollama.Options{Endpoint: "http://example:11434"})
	d, ok := any(b).(interface{ Describe() string })
	core.RequireTrue(t, ok)
	got := d.Describe()
	core.RequireTrue(t, got != "")
	if got != "Ollama daemon at http://example:11434" {
		t.Errorf("Describe default: %q", got)
	}
}

func TestDescribe_Custom(t *core.T) {
	b := ollama.NewBencher(ollama.Options{Description: "lab box"})
	d := any(b).(interface{ Describe() string })
	core.AssertEqual(t, "lab box", d.Describe())
}

func TestCanBench_ListedModel(t *core.T) {
	srv := fakeOllama(func(w core.ResponseWriter, r *core.Request) {
		if r.URL.Path == "/api/tags" {
			_, _ = w.Write([]byte(tagsBody))
			return
		}
		w.WriteHeader(500)
	})
	defer srv.Close()
	b := ollama.NewBencher(ollama.Options{Endpoint: srv.URL})
	core.AssertTrue(t, b.CanBench(benchmark.Bench{Model: "llama3"}))
}

func TestCanBench_UnknownModel(t *core.T) {
	srv := fakeOllama(func(w core.ResponseWriter, r *core.Request) {
		if r.URL.Path == "/api/tags" {
			_, _ = w.Write([]byte(tagsBody))
			return
		}
		w.WriteHeader(500)
	})
	defer srv.Close()
	b := ollama.NewBencher(ollama.Options{Endpoint: srv.URL})
	core.AssertFalse(t, b.CanBench(benchmark.Bench{Model: "mistral"}))
}

func TestCanBench_DaemonDownSoftFails(t *core.T) {
	b := ollama.NewBencher(ollama.Options{Endpoint: "http://127.0.0.1:1"}) // refused
	// Soft-fail to true so the Bench call surfaces the real error.
	core.AssertTrue(t, b.CanBench(benchmark.Bench{Model: "anything"}))
}

func TestCanBench_EmptyModelRejects(t *core.T) {
	b := ollama.NewBencher(ollama.Options{})
	core.AssertFalse(t, b.CanBench(benchmark.Bench{Model: ""}))
}

func TestBench_HappyPath(t *core.T) {
	var gotMethod, gotPath, gotContentType string
	srv := fakeOllama(func(w core.ResponseWriter, r *core.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotContentType = r.Header.Get("Content-Type")
		_, _ = w.Write([]byte(generateBody))
	})
	defer srv.Close()
	b := ollama.NewBencher(ollama.Options{Endpoint: srv.URL})
	r := b.Bench(core.Background(), benchmark.Bench{Model: "llama3", Prompt: "hi", Ctx: 2048, MaxOutput: 256})
	core.RequireTrue(t, r.OK)
	run := r.Value.(benchmark.Run)
	core.AssertEqual(t, "POST", gotMethod)
	core.AssertEqual(t, "/api/generate", gotPath)
	core.AssertEqual(t, "application/json", gotContentType)
	core.AssertEqual(t, "llama3", run.Model)
	core.AssertEqual(t, 2048, run.Ctx)
	core.AssertEqual(t, 512, run.PromptLen)
	core.AssertEqual(t, 256, run.OutputLen)
	core.AssertEqual(t, 512.0, run.PpTokSec)
	core.AssertEqual(t, 64.0, run.TgTokSec)
	core.AssertEqual(t, srv.URL, run.Endpoint)
	core.AssertEqual(t, "llama3", run.Extra["ollama_model"])
}

func TestBench_HTTPErrorBubbles(t *core.T) {
	srv := fakeOllama(func(w core.ResponseWriter, _ *core.Request) {
		w.WriteHeader(500)
		_, _ = w.Write([]byte("model is busy"))
	})
	defer srv.Close()
	b := ollama.NewBencher(ollama.Options{Endpoint: srv.URL})
	r := b.Bench(core.Background(), benchmark.Bench{Model: "llama3", Ctx: 1024})
	core.AssertFalse(t, r.OK)
}

func TestBench_EmptyModelFails(t *core.T) {
	b := ollama.NewBencher(ollama.Options{})
	r := b.Bench(core.Background(), benchmark.Bench{Model: ""})
	core.AssertFalse(t, r.OK)
}

func TestBench_DivisionByZeroIsZeroNotNaN(t *core.T) {
	zeroDurations := `{"model":"llama3","response":"","total_duration":0,"load_duration":0,"prompt_eval_count":0,"prompt_eval_duration":0,"eval_count":0,"eval_duration":0}`
	srv := fakeOllama(func(w core.ResponseWriter, _ *core.Request) {
		_, _ = w.Write([]byte(zeroDurations))
	})
	defer srv.Close()
	b := ollama.NewBencher(ollama.Options{Endpoint: srv.URL})
	r := b.Bench(core.Background(), benchmark.Bench{Model: "llama3"})
	core.RequireTrue(t, r.OK)
	run := r.Value.(benchmark.Run)
	core.AssertEqual(t, 0.0, run.PpTokSec)
	core.AssertEqual(t, 0.0, run.TgTokSec)
}

func TestBench_RegistersWithSubstrate(t *core.T) {
	// End-to-end: register through the substrate + ensure dispatch
	// produces a stored Run with the right Bencher attribution.
	c := newTestCoreWithBenchmark(t)
	svc := benchmark.NewService(benchmark.Options{})
	core.RequireTrue(t, svc.Register(c).OK)

	srv := fakeOllama(func(w core.ResponseWriter, r *core.Request) {
		switch r.URL.Path {
		case "/api/tags":
			_, _ = w.Write([]byte(tagsBody))
		case "/api/generate":
			_, _ = w.Write([]byte(generateBody))
		default:
			w.WriteHeader(404)
		}
	})
	defer srv.Close()
	b := ollama.NewBencher(ollama.Options{Endpoint: srv.URL, Name: "ollama-test"})
	core.RequireTrue(t, svc.RegisterBencher(b).OK)

	r := svc.Bench(core.Background(), benchmark.Bench{Model: "llama3", Ctx: 1024}, "ollama-test")
	if !r.OK {
		t.Fatalf("svc.Bench: %v", r.Value)
	}
	run := r.Value.(benchmark.Run)
	core.AssertEqual(t, "ollama-test", run.Bencher) // substrate stamped
	core.AssertEqual(t, 512.0, run.PpTokSec)        // came from the fake
}
