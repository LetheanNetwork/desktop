// SPDX-Licence-Identifier: EUPL-1.2

package runner_test

import (
	core "dappco.re/go"
	"dappco.re/go/inference"
	"dappco.re/go/inference/agent/ai"

	"dappco.re/lthn/desktop/pkg/runner"
)

func providerSelectionRunner() *runner.Service {
	return runner.NewService(runner.Options{Routes: []ai.ProviderRoute{
		{
			Name:    "lem",
			ModelID: "gemma-local",
			Model:   &runnerStubModel{output: "local reply"},
			Labels:  map[string]string{"kind": "local"},
		},
		{
			Name:    "opencode:anthropic/sonnet",
			ModelID: "anthropic/sonnet",
			Model:   &runnerStubModel{output: "external reply"},
			Labels: map[string]string{
				"kind":        "opencode-routed",
				"provider_id": "anthropic",
			},
		},
	}})
}

func TestRunner_WChat_Good_SelectsRequestedRoute(t *core.T) {
	s := providerSelectionRunner()

	r := s.WChat([]inference.Message{{Role: "user", Content: "hello"}},
		"opencode:anthropic/sonnet")

	core.AssertTrue(t, r.OK)
	reply := r.Value.(runner.ChatReply)
	core.AssertEqual(t, "external reply", reply.Text)
	core.AssertFalse(t, reply.WarnUser)
}

func TestRunner_WChat_Bad_UnknownRoute(t *core.T) {
	s := providerSelectionRunner()

	r := s.WChat([]inference.Message{{Role: "user", Content: "hello"}}, "missing")

	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "provider route")
}

func TestRunner_WChat_Ugly_UnspecifiedPreservesCurrentOrder(t *core.T) {
	s := providerSelectionRunner()

	r := s.WChat([]inference.Message{{Role: "user", Content: "hello"}}, "")

	core.AssertTrue(t, r.OK)
	core.AssertEqual(t, "local reply", r.Value.(runner.ChatReply).Text)
}

func TestRunner_WModels_Good_FiltersByProviderOrRoute(t *core.T) {
	s := providerSelectionRunner()

	byRoute := s.WModels("opencode:anthropic/sonnet")
	core.AssertTrue(t, byRoute.OK)
	core.AssertEqual(t, []string{"anthropic/sonnet"}, byRoute.Value.([]string))

	byProvider := s.WModels("anthropic")
	core.AssertTrue(t, byProvider.OK)
	core.AssertEqual(t, []string{"anthropic/sonnet"}, byProvider.Value.([]string))
}

func TestRunner_WModels_Bad_UnknownProvider(t *core.T) {
	r := providerSelectionRunner().WModels("missing")

	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "provider route")
}

func TestRunner_WModels_Ugly_UnspecifiedPreservesCurrentListing(t *core.T) {
	r := providerSelectionRunner().WModels("")

	core.AssertTrue(t, r.OK)
	core.AssertEqual(t,
		[]string{"lem", "opencode:anthropic/sonnet"},
		r.Value.([]string),
	)
}

func TestRunner_WRoutes_Good_IncludesDynamicOpenCodeProviders(t *core.T) {
	s := runner.NewService(runner.Options{Routes: []ai.ProviderRoute{
		{Name: "lem", ModelID: "gemma-local", Model: &runnerStubModel{output: "local"}},
	}})
	r := runner.ApplyDynamicRoutes(s, []ai.ProviderRoute{{
		Name:    "opencode:openai/gpt-5",
		ModelID: "openai/gpt-5",
		Model:   &runnerStubModel{output: "external"},
		Labels:  map[string]string{"kind": "opencode-routed", "provider_id": "openai"},
	}})
	core.AssertTrue(t, r.OK)

	routesR := s.WRoutes()

	core.AssertTrue(t, routesR.OK)
	routes := routesR.Value.([]runner.RouteView)
	found := false
	for _, route := range routes {
		if route.Name == "opencode:openai/gpt-5" {
			found = true
			core.AssertEqual(t, "opencode-routed", route.Kind)
			core.AssertEqual(t, "openai/gpt-5", route.Model)
		}
	}
	core.AssertTrue(t, found, "renderer route listing must include live dynamic providers")
}
