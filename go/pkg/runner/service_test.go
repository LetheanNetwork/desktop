// SPDX-Licence-Identifier: EUPL-1.2

// Tests for the runner subsystem. The router-backed path needs network
// access for real provider backends, so these tests exercise the
// stub-fallback path (Routes empty) — same code paths plus the
// configuration knobs that select between them.

package runner_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	core "dappco.re/go"
	"dappco.re/go/config"
	"dappco.re/go/inference"

	"dappco.re/lthn/desktop/pkg/runner"
)

func TestRunner_NewService_Good_EmptyOptions(t *core.T) {
	s := runner.NewService(runner.Options{})
	core.AssertNotNil(t, s)
	core.AssertTrue(t, s.Models().OK)
}

func TestRunner_Service_Register_Good(t *core.T) {
	c := core.New()
	s := runner.NewService(runner.Options{})
	r := s.Register(c)
	core.AssertTrue(t, r.OK, "Register should succeed even without routes")
}

func TestRunner_Register_Good(t *core.T) {
	c := core.New()
	r := runner.Register(c)
	core.AssertTrue(t, r.OK)
	routes := runner.LoadRoutesFromCore(c)
	core.AssertLen(t, routes, 0)
}

func TestRunner_Generate_Good_Stub(t *core.T) {
	s := runner.NewService(runner.Options{})
	r := s.Generate("hello")
	core.AssertTrue(t, r.OK)
	out := r.Value.(string)
	core.AssertTrue(t, strings.Contains(out, "hello"), "stub echoes the input")
	core.AssertTrue(t, strings.Contains(out, "lthn stub"), "stub identifies itself")
}

func TestRunner_Chat_Good_StubFindsLastUserMessage(t *core.T) {
	s := runner.NewService(runner.Options{})
	r := s.Chat([]inference.Message{
		{Role: "system", Content: "be brief"},
		{Role: "user", Content: "first"},
		{Role: "assistant", Content: "ok"},
		{Role: "user", Content: "second"},
	})
	core.AssertTrue(t, r.OK)
	out := r.Value.(string)
	core.AssertTrue(t, strings.Contains(out, "second"), "stub picks the most recent user message")
}

func TestRunner_Chat_Good_StubNoUserMessage(t *core.T) {
	s := runner.NewService(runner.Options{})
	r := s.Chat([]inference.Message{
		{Role: "system", Content: "be brief"},
		{Role: "assistant", Content: "ack"},
	})
	core.AssertTrue(t, r.OK, "stub should not Fail when no user message — empty echo is fine")
}

func TestRunner_Models_Good_StubEmptyList(t *core.T) {
	s := runner.NewService(runner.Options{})
	r := s.Models()
	core.AssertTrue(t, r.OK)
	models := r.Value.([]string)
	core.AssertLen(t, models, 0, "stub serves zero models")
}

func TestRunner_LoadRoutesFromCore_Bad_NilCore(t *core.T) {
	routes := runner.LoadRoutesFromCore(nil)
	core.AssertLen(t, routes, 0)
	core.AssertNil(t, routes)
}

func TestRunner_LoadRoutesFromCore_Bad_NoConfigService(t *core.T) {
	c := core.New()
	routes := runner.LoadRoutesFromCore(c)
	core.AssertLen(t, routes, 0, "no config service → no routes")
}

func TestRunner_NewServiceFromCore_Good_NoConfig(t *core.T) {
	c := core.New()
	s := runner.NewServiceFromCore(c)
	core.AssertNotNil(t, s)
	// Falls back to stub since no config service was registered.
	r := s.Generate("ping")
	core.AssertTrue(t, r.OK)
	core.AssertTrue(t, strings.Contains(r.Value.(string), "lthn stub"))
}

// configCoreWithRoutes builds a Core with the config service registered
// + the given YAML written to a temp file as the config source.
// Exercises the routes:* parsing + buildRoute() construction path.
func configCoreWithRoutes(t *core.T, yaml string) *core.Core {
	t.Helper()
	tmp := t.TempDir()
	cfgFile := filepath.Join(tmp, "lthn.yaml")
	core.AssertNoError(t, os.WriteFile(cfgFile, []byte(yaml), 0o644))

	c := core.New(
		core.WithName("config", config.NewConfigServiceWith(config.ServiceOptions{
			Path:      cfgFile,
			EnvPrefix: "LTHN_TEST",
		})),
	)
	core.AssertTrue(t, c.ServiceStartup(context.Background(), nil).OK)
	return c
}

func TestRunner_LoadRoutesFromCore_Good_BuildsOpenAIRoute(t *core.T) {
	yaml := `routes:
  ollama:
    kind: openai
    base_url: http://localhost:11434/v1
    model: llama3.2:3b
  openai:
    kind: openai
    base_url: https://api.openai.com/v1
    api_key: sk-fake
    model: gpt-4o-mini
`
	c := configCoreWithRoutes(t, yaml)
	routes := runner.LoadRoutesFromCore(c)
	core.AssertEqual(t, 2, len(routes), "both routes should build")

	byName := map[string]bool{}
	for _, r := range routes {
		byName[r.Name] = true
		core.AssertNotEqual(t, "", r.ModelID, "ModelID should be set")
		core.AssertNotNil(t, r.Model, "Model should be constructed")
		core.AssertEqual(t, "openai", r.Labels["kind"])
	}
	core.AssertTrue(t, byName["ollama"], "ollama route built")
	core.AssertTrue(t, byName["openai"], "openai route built")
}

func TestRunner_LoadRoutesFromCore_Good_DefaultKindIsOpenAI(t *core.T) {
	// Empty kind defaults to "openai" per buildRoute's switch.
	yaml := `routes:
  local:
    base_url: http://localhost:11434/v1
    model: gemma-4-e2b
`
	c := configCoreWithRoutes(t, yaml)
	routes := runner.LoadRoutesFromCore(c)
	core.AssertEqual(t, 1, len(routes))
	core.AssertEqual(t, "local", routes[0].Name)
}

func TestRunner_LoadRoutesFromCore_Good_UnknownKindSkipped(t *core.T) {
	yaml := `routes:
  weird:
    kind: not-a-real-kind
    model: m
  ok:
    kind: openai
    base_url: http://localhost:11434/v1
    model: llama
`
	c := configCoreWithRoutes(t, yaml)
	routes := runner.LoadRoutesFromCore(c)
	core.AssertEqual(t, 1, len(routes), "unknown kind silently dropped")
	core.AssertEqual(t, "ok", routes[0].Name)
}

// With a router configured, Generate / Chat hit the network path. We
// can't drive a real provider, but we can verify NewService DID
// construct a router from the routes (s.router != nil) by observing
// that Models() now returns the configured route names instead of
// the stub's empty list.
func TestRunner_NewServiceFromCore_Good_RouterConstructed(t *core.T) {
	yaml := `routes:
  ollama:
    kind: openai
    base_url: http://localhost:11434/v1
    model: llama3.2:3b
`
	c := configCoreWithRoutes(t, yaml)
	s := runner.NewServiceFromCore(c)
	core.AssertNotNil(t, s)

	r := s.Models()
	core.AssertTrue(t, r.OK)
	models := r.Value.([]string)
	core.AssertGreater(t, len(models), 0, "router-backed runner should report configured route names")
	core.AssertContains(t, models, "ollama")
}
