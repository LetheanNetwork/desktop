// SPDX-Licence-Identifier: EUPL-1.2

// Package runner is the "talk" surface of the lthn binary — the
// boundary the rest of the binary speaks to when it wants a model to
// answer a prompt. The implementation wraps dappco.re/go/ai's
// ProviderRouter so the same surface can route across local engines
// (go-mlx, future go-rocm), external endpoints (OpenAI-compatible,
// Anthropic, Ollama), and self-hosted Lethean network nodes.
//
// Architectural choice (Snider 2026-05-12): the runner consumes the
// CONSUMER surface (go-ai), not the DRIVER surface (go-mlx). Keeps
// the binary portable to hardware that isn't Apple Silicon. Driver
// selection happens via the routes the runner is constructed with —
// not a compile-time decision.
//
// Usage example:
//
//	c := core.New()
//	r := runner.NewService(runner.Options{})
//	if rr := r.Register(c); !rr.OK {
//		return rr
//	}
//	reply := r.Generate("hello")
package runner

import (

	core "dappco.re/go"
	"dappco.re/go/ai/ai"
	"dappco.re/go/inference"
)

// Status is the runner's lifecycle state.
type Status string

// Lifecycle states surfaced to the frontend as signals.
const (
	StatusIdle       Status = "idle"
	StatusLoading    Status = "loading"
	StatusReady      Status = "ready"
	StatusGenerating Status = "generating"
	StatusError      Status = "error"
)

// Options configures the runner at construction time.
type Options struct {
	// Routes is the ordered fallback list passed to ai.ProviderRouter.
	// Empty list = the runner serves the echo stub (useful pre-wiring).
	Routes []ai.ProviderRoute

	// ModelDir is the directory the runner scans for local model
	// snapshots. Defaults to ~/Lethean/conf/models/ per the
	// no-hidden-user-bloat rule.
	ModelDir string
}

// Service is the runner subsystem. Holds the ai.ProviderRouter (when
// configured) and the lifecycle state the frontend observes.
type Service struct {
	opts    Options
	router  *ai.ProviderRouter
	core    *core.Core
	dynamic []ai.ProviderRoute
}

// NewService constructs the runner with the canonical Mantis #1336
// shape. When opts.Routes is non-empty the runner builds a real
// ai.ProviderRouter; otherwise it serves the echo stub so the binary
// is still useful pre-wiring.
//
// Usage example:
//
//	r := runner.NewService(runner.Options{Routes: routes})
//	r.Register(c)
func NewService(opts Options) *Service {
	s := &Service{opts: opts}
	if len(opts.Routes) > 0 {
		built := ai.NewProviderRouter(opts.Routes...)
		if built.OK {
			if router, ok := built.Value.(*ai.ProviderRouter); ok {
				s.router = router
			}
		}
	}
	return s
}

// Register wires the runner into the Core container. Today this is a
// no-op (the runner is driven through Generate / Models calls from the
// HTTP server and CLI), but the canonical Mantis #1336 entry stays for
// future action wiring (e.g. exposing runner.status as a core action).
//
// Usage example:
//
//	if r := s.Register(c); !r.OK {
//		return r
//	}
func (s *Service) Register(c *core.Core) core.Result {
	return core.Ok(nil)
}

// SetDynamicRoutes replaces the runner's dynamic-route set and
// rebuilds the ai.ProviderRouter against the merged list of static
// (opts.Routes) + dynamic. Use for provider sources whose lifetime
// is decoupled from process startup — opencode sandboxes start /
// stop at runtime, and each transition triggers a SetDynamicRoutes
// call from the cmdServe wire-up.
//
// Empty routes argument clears the dynamic set, leaving only the
// static routes from construction. nil + nil router state is
// allowed (the runner falls back to the echo stub).
//
// Usage example:
//
//	runnerSvc.SetDynamicRoutes(opencodeSvc.Routes())
func (s *Service) SetDynamicRoutes(routes []ai.ProviderRoute) core.Result {
	if s == nil {
		return core.Fail(core.E("runner.SetDynamicRoutes", "service is nil", nil))
	}
	s.dynamic = routes
	merged := make([]ai.ProviderRoute, 0, len(s.opts.Routes)+len(routes))
	merged = append(merged, s.opts.Routes...)
	merged = append(merged, routes...)
	if len(merged) == 0 {
		s.router = nil
		return core.Ok(nil)
	}
	built := ai.NewProviderRouter(merged...)
	if !built.OK {
		return built
	}
	if router, ok := built.Value.(*ai.ProviderRouter); ok {
		s.router = router
	}
	return core.Ok(nil)
}

// Generate returns the assistant reply for prompt. Implements the
// server.Runner contract. When no ai.ProviderRouter is configured the
// runner returns an echo stub so the binary is still useful.
//
// Usage example:
//
//	reply := s.Generate("hello")
//	if reply.OK { core.Println(reply.Value) }
func (s *Service) Generate(prompt string) core.Result {
	if s.router == nil {
		return core.Ok(core.Concat("[lthn stub] received: ", prompt))
	}
	resp := s.router.Chat(core.Background(), ai.ProviderChatRequest{
		Prompt: prompt,
	})
	if !resp.OK {
		return resp
	}
	chat, ok := resp.Value.(*ai.ProviderChatResponse)
	if !ok || chat == nil {
		return core.Ok("")
	}
	return core.Ok(chat.Text)
}

// Chat is the messages-array variant of Generate. Routes a full
// chat-completion request through ai.ProviderRouter.
//
// Usage example:
//
//	reply := s.Chat([]inference.Message{
//		{Role: "user", Content: "ping"},
//	})
func (s *Service) Chat(messages []inference.Message) core.Result {
	if s.router == nil {
		last := ""
		for i := len(messages) - 1; i >= 0; i-- {
			if messages[i].Role == "user" {
				last = messages[i].Content
				break
			}
		}
		return core.Ok(core.Concat("[lthn stub] received: ", last))
	}
	resp := s.router.Chat(core.Background(), ai.ProviderChatRequest{
		Messages: messages,
	})
	if !resp.OK {
		return resp
	}
	chat, ok := resp.Value.(*ai.ProviderChatResponse)
	if !ok || chat == nil {
		return core.Ok("")
	}
	return core.Ok(chat.Text)
}

// Models returns the list of model identifiers exposed by the runner.
// Implements the server.Runner contract. Today the names come from the
// configured ai.ProviderRoute order; local-model scanning of
// ~/Lethean/conf/models/ lands when the driver layer is wired.
//
// Usage example:
//
//	ids := s.Models()
//	if ids.OK { _ = ids.Value.([]string) }
func (s *Service) Models() core.Result {
	if s.router == nil {
		return core.Ok([]string{})
	}
	routes := s.router.Providers()
	out := make([]string, 0, len(routes))
	for _, r := range routes {
		if r.Name != "" {
			out = append(out, r.Name)
			continue
		}
		if r.ModelID != "" {
			out = append(out, r.ModelID)
		}
	}
	return core.Ok(out)
}

// Register constructs a default runner Service and wires it into the
// Core container. The Mantis #1336 one-shot canonical entry.
//
// Usage example:
//
//	if r := runner.Register(c); !r.OK {
//		return r
//	}
func Register(c *core.Core) core.Result {
	return NewService(Options{}).Register(c)
}
