// SPDX-Licence-Identifier: EUPL-1.2

package runner

import (
	core "dappco.re/go"
	"dappco.re/go/inference/agent/ai"
)

// routerForSelector snapshots the live router and optionally narrows it to
// routes matching an exact route name, model id, or provider_id label.
// Empty selector returns the existing router unchanged, preserving its
// configured fallback order.
func (s *Service) routerForSelector(selector string) core.Result {
	if s == nil {
		return core.Fail(core.E("runner.routerForSelector", "service is nil", nil))
	}
	s.routerMu.RLock()
	router := s.router
	s.routerMu.RUnlock()

	selector = core.Trim(selector)
	if selector == "" {
		return core.Ok(router)
	}
	if router == nil {
		return core.Fail(core.E("runner.routerForSelector",
			"provider route not found: "+selector, nil))
	}

	matches := matchingProviderRoutes(router.Providers(), selector)
	if len(matches) == 0 {
		return core.Fail(core.E("runner.routerForSelector",
			"provider route not found: "+selector, nil))
	}
	built := ai.NewProviderRouter(matches...)
	if !built.OK {
		cause, _ := built.Value.(error)
		return core.Fail(core.E("runner.routerForSelector",
			"build selected provider route: "+built.Error(), cause))
	}
	return built
}

func (s *Service) liveProviderRoutes() []ai.ProviderRoute {
	if s == nil {
		return nil
	}
	s.routerMu.RLock()
	router := s.router
	s.routerMu.RUnlock()
	if router == nil {
		return nil
	}
	return router.Providers()
}

func matchingProviderRoutes(routes []ai.ProviderRoute, selector string) []ai.ProviderRoute {
	selector = core.Trim(selector)
	if selector == "" {
		return append([]ai.ProviderRoute(nil), routes...)
	}
	out := make([]ai.ProviderRoute, 0, 1)
	for _, route := range routes {
		if providerRouteMatches(route, selector) {
			out = append(out, route)
		}
	}
	return out
}

func providerRouteMatches(route ai.ProviderRoute, selector string) bool {
	if route.Name == selector || route.ModelID == selector {
		return true
	}
	if route.Labels != nil && route.Labels["provider_id"] == selector {
		return true
	}
	// OpenCode route names are opencode:<provider>/<model>. Accepting
	// opencode:<provider> lets callers select that provider's live model
	// routes while exact route names still choose one model.
	return core.HasPrefix(route.Name, selector+"/") &&
		core.HasPrefix(selector, "opencode:")
}
