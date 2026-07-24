// SPDX-Licence-Identifier: EUPL-1.2

// Route loading for the runner. Reads `routes:` from the
// dappco.re/go/config service registered on the Core container and
// builds ai.ProviderRoute entries that wrap upstream provider
// backends (today: openai-compatible — Ollama, vLLM, llama.cpp, real
// OpenAI all sit behind this single shape).
//
// Config schema (~/Lethean/conf/lthn.yaml) — post-RFC v1.1 (Mantis
// #1657) the per-route credential lives behind an opaque tier-1 ref:
//
//	routes:
//	  openai:
//	    kind: openai          # only kind today; future: anthropic / local
//	    base_url: https://api.openai.com/v1
//	    api_key_ref: openai-default   # tier-1 ref into pkg/keys.Service
//	    model: gpt-4o-mini
//	  ollama:
//	    kind: openai
//	    base_url: http://localhost:11434/v1
//	    model: llama3:8b              # no credential needed for local
//
// The legacy `api_key:` field is retained for the migration window —
// pkg/runner/migration.go auto-imports the plaintext into the tier-1
// substrate and rewrites the config on first successful unlock.
//
// The map key (openai / ollama) becomes the ProviderRoute.Name —
// what `lthn ai models` lists and what the openai-compat /v1/models
// endpoint exposes.
//
// Schema type lives in pkg/runner/types.go (RouteConfig).

package runner

import (
	core "dappco.re/go"
	"dappco.re/go/config"
	"dappco.re/go/inference"
	"dappco.re/go/inference/agent/ai"
	"dappco.re/go/inference/serving/provider/openai"
	"dappco.re/go/ratelimit"

	"dappco.re/lthn/desktop/pkg/keys"
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
//
// Mantis #1660 / Cerberus #45 — guarded by outboundLimiterOnce so
// concurrent first-callers serialise on the SQLite open. Prior to
// this discipline the raw read+write race let two simultaneous
// SetDynamicRoutes calls each invoke ratelimit.NewWithSQLite, drop
// one Limiter on the floor, and (worse) accept either-or as the
// "winning" pointer. The Once.Do collapses that to a single attempt
// whose outcome (success or failure) is the canonical answer for
// the process lifetime.
var (
	outboundLimiter     openai.Limiter
	outboundLimiterOnce core.Once
)

// resolveOutboundLimiter lazily constructs the per-process limiter.
// Called by buildRoute the first time a real route is being wired —
// avoids spinning up the SQLite file in test paths that never touch
// providers.
//
// Mantis #1660 / Cerberus #45 — explicit fail-loud log on SQLite open
// failure so the operator can correlate "quotas not tracking" with the
// underlying disk / permission issue. The prior shape returned nil
// silently and let the backend treat it as "no quota tracking" with no
// observable signal. Tests + CI paths that legitimately have no writable
// ~/Lethean/data/ still see the nil result (operative behaviour
// unchanged) — they just also see a one-shot stderr line explaining why.
func resolveOutboundLimiter() openai.Limiter {
	outboundLimiterOnce.Do(func() {
		dirR := paths.DataDir()
		if !dirR.OK {
			core.Print(core.Stderr(),
				"runner.outboundLimiter: data dir unresolved — quota tracking disabled (%s)\n",
				dirR.Error())
			return
		}
		dir, _ := dirR.Value.(string)
		rl, err := ratelimit.NewWithSQLite(core.PathJoin(dir, "ratelimits.db"))
		if err != nil {
			core.Print(core.Stderr(),
				"runner.outboundLimiter: SQLite open failed — quota tracking disabled (%s)\n",
				err.Error())
			return
		}
		outboundLimiter = rl
	})
	return outboundLimiter
}

// ResetOutboundLimiterForTests clears the cached limiter + once so a
// test can drive the resolveOutboundLimiter path multiple times against
// different paths.DataDir() fixtures. Test-only — production callers
// MUST NOT invoke (the once-per-process contract is load-bearing for
// the openai.Backend.Limiter sharing across route reloads).
//
// Mantis #1660 — exposed so the init-failure test can re-fire resolve
// against a poisoned paths.DataDir() and assert the fail-loud surface
// without contaminating the process-wide limiter state.
func ResetOutboundLimiterForTests() {
	outboundLimiter = nil
	outboundLimiterOnce.Reset()
}

// LoadRoutesFromCore reads `routes:` from the registered config
// service and constructs ai.ProviderRoute entries. Returns an empty
// slice (not an error) when no routes are configured — the runner
// falls back to the echo stub in that case.
//
// Mantis #1657 / RFC.provider-creds-tier1.md v1.1 §3.2 — buildRoute
// resolves rc.APIKeyRef via keys.Service.GetTier1 at build time.
// When the ref cannot be resolved (account locked → no tier-1 KEK
// provider, or ref missing from the tier-1 partition), the route is
// SKIPPED (returns nil) rather than failing the whole load. The
// boot-time pkg/runner/migration.go scan + the SetKEKProvider
// rebuild hook re-attempt resolution once the prerequisites are met.
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
	keysSvc, _ := core.ServiceFor[*keys.Service](c, "keys")
	out := make([]ai.ProviderRoute, 0, len(raw))
	for name, rc := range raw {
		route := buildRoute(name, rc, keysSvc)
		if route == nil {
			continue
		}
		out = append(out, *route)
	}
	return out
}

// resolveRouteCredential returns the plaintext credential to wire into
// the upstream openai.Backend for a route entry, plus the ref it was
// resolved from (empty when no tier-1 resolution happened — legacy
// plaintext path). Returns ("", "", false) when the route names a ref
// but the tier-1 substrate cannot satisfy it (KEK provider not live,
// ref absent) — the caller skips the route in that case so the boot
// path completes cleanly and the deferred-migration UX kicks in.
//
// Resolution precedence (RFC v1.1 §3.1):
//
//   - rc.APIKeyRef set + keysSvc available + GetTier1 OK → tier-1 wins
//   - rc.APIKeyRef set + tier-1 unresolvable → return (false) so caller
//     skips the route (deferred until next rebuild)
//   - rc.APIKey set (legacy path, migration not yet run) → plaintext
//   - both empty → ("", "", true) — local-Ollama style anonymous route
func resolveRouteCredential(rc RouteConfig, keysSvc *keys.Service) (apiKey, resolvedRef string, ok bool) {
	if rc.APIKeyRef != "" {
		if keysSvc == nil {
			return "", "", false
		}
		r := keysSvc.GetTier1(rc.APIKeyRef)
		if !r.OK {
			return "", "", false
		}
		plaintext, castOK := r.Value.([]byte)
		if !castOK || len(plaintext) == 0 {
			return "", "", false
		}
		return string(plaintext), rc.APIKeyRef, true
	}
	return rc.APIKey, "", true
}

// buildRoute turns a config entry into a ProviderRoute. Returns nil
// when the entry is malformed, names an unsupported kind, or carries
// an APIKeyRef that cannot be resolved (account locked / ref missing).
//
// Skip-on-unresolvable is deliberate (RFC v1.1 §4 Option A) — the
// route is rebuilt once the tier-1 KEK provider becomes live via the
// rebuild hook wired in pkg/runner/migration.go.
func buildRoute(name string, rc RouteConfig, keysSvc *keys.Service) *ai.ProviderRoute {
	switch core.Lower(rc.Kind) {
	case "openai", "":
		apiKey, _, ok := resolveRouteCredential(rc, keysSvc)
		if !ok {
			// Deferred-route — credential ref cannot be resolved
			// right now (account locked / ref absent). The route
			// will reappear after the rebuild hook fires post-
			// unlock. Skip silently rather than failing the load
			// so the binary boots cleanly with whatever routes
			// CAN be resolved.
			return nil
		}
		backend := openai.NewBackend(openai.Config{
			Name:         name,
			BaseURL:      rc.BaseURL,
			APIKey:       apiKey,
			DefaultModel: rc.Model,
			Limiter:      resolveOutboundLimiter(),
		})
		loadR := backend.LoadModel(rc.Model)
		if !loadR.OK {
			return nil
		}
		model, ok := loadR.Value.(inference.TextModel)
		if !ok {
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

// loadRouteConfigsFromCore is the read-only sibling of LoadRoutesFromCore
// — returns the raw RouteConfig map without building backends or
// touching keys.Service. Used by wails.go (WRoutes / WSetDynamicRoutes
// reverse-lookup) and migration.go (boot-time scan, RoutesReferencing)
// to enumerate the on-disk shape without forcing credential
// resolution.
func loadRouteConfigsFromCore(c *core.Core) map[string]RouteConfig {
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
	return raw
}

// saveRouteConfigsToCore writes the routes map back into the registered
// config service. Used by WSetDynamicRoutes (operator-driven write) and
// pkg/runner/migration.go (auto-import-and-strip).
//
// Returns the config-service write Result so callers can surface a
// typed failure. Returns Fail when no config service is registered.
func saveRouteConfigsToCore(c *core.Core, routes map[string]RouteConfig) core.Result {
	if c == nil {
		return core.Fail(core.E("runner.saveRouteConfigsToCore", "core is nil", nil))
	}
	cfg, ok := core.ServiceFor[*config.Service](c, "config")
	if !ok || cfg == nil {
		return core.Fail(core.E("runner.saveRouteConfigsToCore", "config service not registered", nil))
	}
	if r := cfg.Set("routes", routes); !r.OK {
		return r
	}
	return cfg.Commit()
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
	welfareEnabled := false
	if cfg, ok := core.ServiceFor[*config.Service](c, "config"); ok && cfg != nil {
		configured := cfg.Get("runner.welfare.enabled", &welfareEnabled)
		if !configured.OK {
			welfareEnabled = false
		}
	}
	s := NewService(Options{
		Routes:         LoadRoutesFromCore(c),
		WelfareEnabled: welfareEnabled,
	})
	s.core = c
	return s
}
