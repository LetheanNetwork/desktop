// SPDX-Licence-Identifier: EUPL-1.2

// Route loading for the runner. Reads `routes:` from the
// dappco.re/go/config service registered on the Core container and
// builds ai.ProviderRoute entries that wrap upstream provider
// backends (today: openai-compatible — Ollama, vLLM, llama.cpp, real
// OpenAI all sit behind this single shape).
//
// Config schema (~/Lethean/conf/lthn.yaml):
//
//	routes:
//	  openai:
//	    kind: openai          # only kind today; future: anthropic / local
//	    base_url: https://api.openai.com/v1
//	    api_key: sk-...
//	    model: gpt-4o-mini
//	  ollama:
//	    kind: openai
//	    base_url: http://localhost:11434/v1
//	    model: llama3:8b
//
// The map key (openai / ollama) becomes the ProviderRoute.Name —
// what `lthn ai models` lists and what the openai-compat /v1/models
// endpoint exposes.

package runner

import (
	core "dappco.re/go"
	"dappco.re/go/ai/ai"
	"dappco.re/go/ai/providers/openai"
	"dappco.re/go/config"
	"dappco.re/go/ratelimit"

	"dappco.re/lthn/desktop/pkg/paths"
)

// outboundLimiter is constructed once per process and shared across
// every provider backend so per-provider / per-model quotas survive
// route reloads. SQLite-backed at ~/Lethean/data/ratelimits.db so
// counter state persists across launches.
//
// Nil is a valid value — the openai.Backend treats a nil Limiter
// as "no quota tracking" and the rest of the binary keeps working
// (useful in tests + CI where the path isn't writable).
var outboundLimiter openai.Limiter

// resolveOutboundLimiter lazily constructs the per-process limiter.
// Called by buildRoute the first time a real route is being wired —
// avoids spinning up the SQLite file in test paths that never touch
// providers.
func resolveOutboundLimiter() openai.Limiter {
	if outboundLimiter != nil {
		return outboundLimiter
	}
	dirR := paths.DataDir()
	if !dirR.OK {
		return nil
	}
	dir, _ := dirR.Value.(string)
	rl, err := ratelimit.NewWithSQLite(core.PathJoin(dir, "ratelimits.db"))
	if err != nil {
		return nil
	}
	outboundLimiter = rl
	return outboundLimiter
}

// RouteConfig is one entry in the `routes:` map.
type RouteConfig struct {
	// Kind is the provider adapter — today only "openai" (which covers
	// Ollama, vLLM, llama.cpp's OpenAI-compat endpoint, and real OpenAI).
	Kind string `mapstructure:"kind"`
	// BaseURL is the OpenAI-compatible endpoint base — e.g.
	// "https://api.openai.com/v1" or "http://localhost:11434/v1".
	BaseURL string `mapstructure:"base_url"`
	// APIKey authenticates requests. Optional for local Ollama.
	APIKey string `mapstructure:"api_key"`
	// Model is the upstream model identifier — e.g. "gpt-4o-mini",
	// "llama3:8b". Used as ProviderRoute.ModelID.
	Model string `mapstructure:"model"`
}

// LoadRoutesFromCore reads `routes:` from the registered config
// service and constructs ai.ProviderRoute entries. Returns an empty
// slice (not an error) when no routes are configured — the runner
// falls back to the echo stub in that case.
//
// Usage example:
//
//	routes := runner.LoadRoutesFromCore(c)
func LoadRoutesFromCore(c *core.Core) []ai.ProviderRoute {
	if c == nil {
		return nil
	}
	cfg, ok := core.ServiceFor[*config.Service](c, "config")
	if !ok || cfg == nil {
		return nil
	}
	var raw map[string]RouteConfig
	if r := cfg.Get("routes", &raw); !r.OK {
		return nil
	}
	out := make([]ai.ProviderRoute, 0, len(raw))
	for name, rc := range raw {
		route := buildRoute(name, rc)
		if route == nil {
			continue
		}
		out = append(out, *route)
	}
	return out
}

// buildRoute turns a config entry into a ProviderRoute. Returns nil
// when the entry is malformed or names an unsupported kind — the
// caller skips silently rather than failing the whole load.
func buildRoute(name string, rc RouteConfig) *ai.ProviderRoute {
	switch core.Lower(rc.Kind) {
	case "openai", "":
		backend := openai.NewBackend(openai.Config{
			Name:         name,
			BaseURL:      rc.BaseURL,
			APIKey:       rc.APIKey,
			DefaultModel: rc.Model,
			Limiter:      resolveOutboundLimiter(),
		})
		model, err := backend.LoadModel(rc.Model)
		if err != nil {
			return nil
		}
		return &ai.ProviderRoute{
			Name:    name,
			ModelID: rc.Model,
			Model:   model,
			Labels:  map[string]string{"kind": "openai"},
		}
	default:
		return nil
	}
}

// NewServiceFromCore constructs the runner with routes loaded from
// the config service registered on c. When no routes are configured,
// the runner serves the echo stub.
//
// Usage example:
//
//	c := newAppCore()
//	r := runner.NewServiceFromCore(c)
//	reply := r.Generate("hello")
func NewServiceFromCore(c *core.Core) *Service {
	s := NewService(Options{Routes: LoadRoutesFromCore(c)})
	s.core = c
	return s
}
