// SPDX-Licence-Identifier: EUPL-1.2

// Wails3 Service shape for the runner package — extends the
// existing *Service with lifecycle hooks plus WebView-friendly
// wrappers. The package's core.Result-returning Generate / Chat /
// Models methods stay on the type for Action-bus and CLI callers.

package runner

import (

	core "dappco.re/go"
	"dappco.re/go/config"
	"dappco.re/go/inference"
)

// RouteView is the lean read-only shape Settings → Runner consumes.
// Mirrors RouteConfig minus APIKey — credentials never cross the
// Wails binding boundary, even on localhost.
type RouteView struct {
	Name    string `json:"name"`
	Kind    string `json:"kind"`
	BaseURL string `json:"base_url"`
	Model   string `json:"model"`
}

// ServiceName / Startup / Shutdown — Wails3 lifecycle. Service was
// already constructed as a Core service; Wails just wraps the same
// instance through application.NewService.

func (s *Service) ServiceName() string { return "Runner" }
func (s *Service) ServiceStartup(_ core.Context, _ any) core.Result {
	return core.Ok(nil)
}
func (s *Service) ServiceShutdown() core.Result { return core.Ok(nil) }

// WGenerate is the WebView-binding-friendly Generate. Returns the
// assistant reply as a plain string, or an error if the router
// failed. The unprefixed Generate(prompt) core.Result is preserved
// for Action-bus and CLI callers — see service.go.
//
// Usage example (from TS):
//
//	import { WGenerate } from "@desktop/runner/service";
//	const reply = await WGenerate("hello");
func (s *Service) WGenerate(prompt string) core.Result {
	r := s.Generate(prompt)
	if !r.OK {
		return core.Fail(core.E("runner.Service.WGenerate", "generate failed", r.Value.(error)))
	}
	text, _ := r.Value.(string)
	return core.Ok(text)
}

// WChat is the WebView-binding-friendly Chat — full message
// history in, assistant reply out.
func (s *Service) WChat(messages []inference.Message) core.Result {
	r := s.Chat(messages)
	if !r.OK {
		return core.Fail(core.E("runner.Service.WChat", "chat failed", r.Value.(error)))
	}
	text, _ := r.Value.(string)
	return core.Ok(text)
}

// WModels is the WebView-binding-friendly Models — returns the
// list of configured route names.
func (s *Service) WModels() core.Result {
	r := s.Models()
	if !r.OK {
		return core.Fail(core.E("runner.Service.WModels", "list models failed", r.Value.(error)))
	}
	names, _ := r.Value.([]string)
	return core.Ok(names)
}

// WRoutes returns the configured provider routes for the Settings →
// Runner panel. Re-reads the config service so a config rewrite +
// reload surfaces immediately. Empty slice when no Core ref is set
// (NewService path — runner constructed without config). APIKey is
// never returned.
//
// Usage example (TS):
//
//	import { WRoutes } from "@desktop/runner/service";
//	const routes = await WRoutes();
//	routes.forEach(r => console.log(r.name, r.kind, r.base_url));
func (s *Service) WRoutes() core.Result {
	out := []RouteView{}
	if s == nil || s.core == nil {
		return core.Ok(out)
	}
	cfg, ok := core.ServiceFor[*config.Service](s.core, "config")
	if !ok || cfg == nil {
		return core.Ok(out)
	}
	var raw map[string]RouteConfig
	if r := cfg.Get("routes", &raw); !r.OK {
		return core.Ok(out)
	}
	for name, rc := range raw {
		kind := rc.Kind
		if kind == "" {
			kind = "openai"
		}
		out = append(out, RouteView{
			Name:    name,
			Kind:    kind,
			BaseURL: rc.BaseURL,
			Model:   rc.Model,
		})
	}
	return core.Ok(out)
}
