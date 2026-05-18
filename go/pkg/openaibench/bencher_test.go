// SPDX-Licence-Identifier: EUPL-1.2

package openaibench_test

import (
	core "dappco.re/go"
	"dappco.re/go/orm"

	"dappco.re/lthn/desktop/pkg/benchmark"
	"dappco.re/lthn/desktop/pkg/openaibench"
)

// fakeOpenAI returns a httptest server serving /v1/models and
// /v1/chat/completions. Handler shapes the response per scenario.
func fakeOpenAI(handler core.HandlerFunc) *core.HTTPTestServer {
	return core.NewHTTPTestServer(core.HandlerFunc(handler))
}

// modelsBody is a canned /v1/models response advertising one model.
const modelsBody = `{
  "object": "list",
  "data": [
    {"id": "gpt-4o-mini", "object": "model"},
    {"id": "meta-llama/Llama-3-8b", "object": "model"}
  ]
}`

// chatBody is a canned /v1/chat/completions response carrying a
// usage block the Bench parser depends on.
const chatBody = `{
  "id": "chatcmpl-abc123",
  "object": "chat.completion",
  "model": "gpt-4o-mini",
  "choices": [
    {"index": 0, "message": {"role": "assistant", "content": "hi"}, "finish_reason": "stop"}
  ],
  "usage": {"prompt_tokens": 200, "completion_tokens": 100, "total_tokens": 300}
}`

// newTestCoreWithBenchmark mirrors pkg/benchmark/service_test.go's
// helper for the substrate-integration test.
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

func TestNewBencher_RejectsMissingName(t *core.T) {
	b := openaibench.NewBencher(openaibench.Options{Endpoint: "http://x/v1"})
	if b != nil {
		t.Fatal("expected nil for missing Name")
	}
}

func TestNewBencher_RejectsMissingEndpoint(t *core.T) {
	b := openaibench.NewBencher(openaibench.Options{Name: "x"})
	if b != nil {
		t.Fatal("expected nil for missing Endpoint")
	}
}

func TestNewBencher_HonoursName(t *core.T) {
	b := openaibench.NewBencher(openaibench.Options{Name: "openai-compat:nim", Endpoint: "http://x/v1"})
	core.AssertEqual(t, "openai-compat:nim", b.Name())
	core.AssertEqual(t, benchmark.KindRemoteHTTP, b.Kind())
}

func TestDescribe_Default(t *core.T) {
	b := openaibench.NewBencher(openaibench.Options{Name: "x", Endpoint: "http://example/v1"})
	d := any(b).(interface{ Describe() string })
	core.AssertEqual(t, "OpenAI-compatible endpoint at http://example/v1", d.Describe())
}

func TestDescribe_Custom(t *core.T) {
	b := openaibench.NewBencher(openaibench.Options{Name: "x", Endpoint: "http://example/v1", Description: "lab box"})
	d := any(b).(interface{ Describe() string })
	core.AssertEqual(t, "lab box", d.Describe())
}

func TestCanBench_ListedModel(t *core.T) {
	srv := fakeOpenAI(func(w core.ResponseWriter, r *core.Request) {
		if r.URL.Path == "/v1/models" {
			_, _ = w.Write([]byte(modelsBody))
			return
		}
		w.WriteHeader(500)
	})
	defer srv.Close()
	b := openaibench.NewBencher(openaibench.Options{Name: "x", Endpoint: srv.URL + "/v1"})
	core.AssertTrue(t, b.CanBench(benchmark.Bench{Model: "gpt-4o-mini"}))
}

func TestCanBench_UnknownModel(t *core.T) {
	srv := fakeOpenAI(func(w core.ResponseWriter, r *core.Request) {
		if r.URL.Path == "/v1/models" {
			_, _ = w.Write([]byte(modelsBody))
			return
		}
		w.WriteHeader(500)
	})
	defer srv.Close()
	b := openaibench.NewBencher(openaibench.Options{Name: "x", Endpoint: srv.URL + "/v1"})
	core.AssertFalse(t, b.CanBench(benchmark.Bench{Model: "claude-opus-4"}))
}

func TestCanBench_EndpointDownSoftFails(t *core.T) {
	b := openaibench.NewBencher(openaibench.Options{Name: "x", Endpoint: "http://127.0.0.1:1/v1"})
	core.AssertTrue(t, b.CanBench(benchmark.Bench{Model: "anything"}))
}

func TestCanBench_EmptyDataSoftFails(t *core.T) {
	// Some endpoints (llama-server) return empty data — soft-fail so
	// Bench surfaces the real error.
	srv := fakeOpenAI(func(w core.ResponseWriter, _ *core.Request) {
		_, _ = w.Write([]byte(`{"object":"list","data":[]}`))
	})
	defer srv.Close()
	b := openaibench.NewBencher(openaibench.Options{Name: "x", Endpoint: srv.URL + "/v1"})
	core.AssertTrue(t, b.CanBench(benchmark.Bench{Model: "anything"}))
}

func TestCanBench_EmptyModelRejects(t *core.T) {
	b := openaibench.NewBencher(openaibench.Options{Name: "x", Endpoint: "http://x/v1"})
	core.AssertFalse(t, b.CanBench(benchmark.Bench{Model: ""}))
}

func TestBench_HappyPath(t *core.T) {
	var gotMethod, gotPath, gotAuth, gotOrg, gotCT string
	srv := fakeOpenAI(func(w core.ResponseWriter, r *core.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotOrg = r.Header.Get("OpenAI-Organization")
		gotCT = r.Header.Get("Content-Type")
		_, _ = w.Write([]byte(chatBody))
	})
	defer srv.Close()
	b := openaibench.NewBencher(openaibench.Options{
		Name:     "x",
		Endpoint: srv.URL + "/v1",
		Bearer:   "sk-test",
		Org:      "org-test",
	})
	r := b.Bench(core.Background(), benchmark.Bench{Model: "gpt-4o-mini", Prompt: "hi", Ctx: 2048, MaxOutput: 256})
	core.RequireTrue(t, r.OK)
	run := r.Value.(benchmark.Run)
	core.AssertEqual(t, "POST", gotMethod)
	core.AssertEqual(t, "/v1/chat/completions", gotPath)
	core.AssertEqual(t, "Bearer sk-test", gotAuth)
	core.AssertEqual(t, "org-test", gotOrg)
	core.AssertEqual(t, "application/json", gotCT)
	core.AssertEqual(t, "gpt-4o-mini", run.Model)
	core.AssertEqual(t, 2048, run.Ctx)
	core.AssertEqual(t, 200, run.PromptLen)
	core.AssertEqual(t, 100, run.OutputLen)
	core.AssertEqual(t, srv.URL+"/v1", run.Endpoint)
	core.AssertEqual(t, "chatcmpl-abc123", run.Extra["response_id"])
	core.AssertEqual(t, "stop", run.Extra["finish_reason"])
	core.AssertTrue(t, run.PpTokSec > 0)
	core.AssertTrue(t, run.TgTokSec > 0)
}

func TestBench_NoBearerOmitsAuthHeader(t *core.T) {
	var gotAuth string
	srv := fakeOpenAI(func(w core.ResponseWriter, r *core.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(chatBody))
	})
	defer srv.Close()
	b := openaibench.NewBencher(openaibench.Options{Name: "x", Endpoint: srv.URL + "/v1"})
	r := b.Bench(core.Background(), benchmark.Bench{Model: "gpt-4o-mini"})
	core.RequireTrue(t, r.OK)
	core.AssertEqual(t, "", gotAuth)
}

func TestBench_HTTPErrorBubbles(t *core.T) {
	srv := fakeOpenAI(func(w core.ResponseWriter, _ *core.Request) {
		w.WriteHeader(401)
		_, _ = w.Write([]byte(`{"error":{"message":"invalid api key"}}`))
	})
	defer srv.Close()
	b := openaibench.NewBencher(openaibench.Options{Name: "x", Endpoint: srv.URL + "/v1"})
	r := b.Bench(core.Background(), benchmark.Bench{Model: "x"})
	core.AssertFalse(t, r.OK)
}

func TestBench_EmptyModelFails(t *core.T) {
	b := openaibench.NewBencher(openaibench.Options{Name: "x", Endpoint: "http://x/v1"})
	r := b.Bench(core.Background(), benchmark.Bench{Model: ""})
	core.AssertFalse(t, r.OK)
}

func TestBench_RegistersWithSubstrate(t *core.T) {
	c := newTestCoreWithBenchmark(t)
	svc := benchmark.NewService(benchmark.Options{})
	core.RequireTrue(t, svc.Register(c).OK)

	srv := fakeOpenAI(func(w core.ResponseWriter, r *core.Request) {
		switch r.URL.Path {
		case "/v1/models":
			_, _ = w.Write([]byte(modelsBody))
		case "/v1/chat/completions":
			_, _ = w.Write([]byte(chatBody))
		default:
			w.WriteHeader(404)
		}
	})
	defer srv.Close()
	b := openaibench.NewBencher(openaibench.Options{
		Name:     "openai-compat:test",
		Endpoint: srv.URL + "/v1",
	})
	core.RequireTrue(t, svc.RegisterBencher(b).OK)
	r := svc.Bench(core.Background(), benchmark.Bench{Model: "gpt-4o-mini", Ctx: 1024}, "openai-compat:test")
	if !r.OK {
		t.Fatalf("svc.Bench: %v", r.Value)
	}
	run := r.Value.(benchmark.Run)
	core.AssertEqual(t, "openai-compat:test", run.Bencher)
	core.AssertEqual(t, 200, run.PromptLen)
	core.AssertEqual(t, 100, run.OutputLen)
}
