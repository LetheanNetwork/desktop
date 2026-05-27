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
	"context"

	core "dappco.re/go"
	"dappco.re/go/ai/ai"
	"dappco.re/go/inference"

	"dappco.re/lthn/desktop/pkg/audit"
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
//
// Concurrency: Cerberus #45 / Mantis #1656 — routerMu guards router +
// dynamic against the race between renderer-driven Generate / Chat
// reads (via WGenerate / WChat Wails bindings) and the lifecycle
// writer that calls ApplyDynamicRoutes when opencode sandboxes start
// or stop. The bare router pointer was being swapped while inference
// requests were dereferencing it.
type Service struct {
	opts     Options
	routerMu core.RWMutex
	router   *ai.ProviderRouter
	core     *core.Core
	dynamic  []ai.ProviderRoute
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

// ApplyDynamicRoutes replaces the runner's dynamic-route set and
// rebuilds the ai.ProviderRouter against the merged list of static
// (opts.Routes) + dynamic. Use for provider sources whose lifetime
// is decoupled from process startup — opencode sandboxes start /
// stop at runtime, and each transition triggers an ApplyDynamicRoutes
// call from the cmdServe wire-up.
//
// Empty routes argument clears the dynamic set, leaving only the
// static routes from construction. nil + nil router state is
// allowed (the runner falls back to the echo stub).
//
// Cerberus #45 / Mantis #1656 — package-level function (not method)
// so Wails3's method-set reflection never binds this onto the
// renderer surface. Untrusted frontend code cannot trigger a
// router-pointer swap mid-Generate / mid-Chat. The exclusive write
// lock pairs with the RLock in Generate / Chat / Models so the
// router pointer + dynamic slice mutate atomically.
//
// Usage example:
//
//	runner.ApplyDynamicRoutes(runnerSvc, opencodeSvc.Routes())
func ApplyDynamicRoutes(s *Service, routes []ai.ProviderRoute) core.Result {
	if s == nil {
		return core.Fail(core.E("runner.ApplyDynamicRoutes", "service is nil", nil))
	}
	merged := make([]ai.ProviderRoute, 0, len(s.opts.Routes)+len(routes))
	merged = append(merged, s.opts.Routes...)
	merged = append(merged, routes...)
	s.routerMu.Lock()
	defer s.routerMu.Unlock()
	s.dynamic = routes
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
// Background-context shim for callers without an HTTP request scope
// (Action-bus, CLI, Wails). HTTP handlers should use GenerateCtx so
// upstream provider RTT cancels when the client disconnects (Cerberus
// #60 F-2 / Mantis #1711 — STRIDE-D self-DoS via abandoned-request
// quota burn).
//
// Usage example:
//
//	reply := s.Generate("hello")
//	if reply.OK { core.Println(reply.Value) }
func (s *Service) Generate(prompt string) core.Result {
	return s.GenerateCtx(core.Background(), prompt)
}

// GenerateCtx is the ctx-aware variant of Generate. Plumbed through
// pkg/server/handlers.go from gin's c.Request.Context() so a client
// disconnect (browser tab close, fetch abort) cancels the upstream
// router.Chat call instead of letting it run to completion and burn
// quota against the Requested audit row.
//
// Cerberus #60 F-2 / Mantis #1711: ratelimit.NewWithSQLite counts every
// Requested row against quota; before ctx-plumbing, abandoned mid-call
// requests still completed upstream + still counted. Now ctx cancel
// propagates into ai.ProviderRouter.Chat → underlying provider HTTP
// client → cancels the in-flight request.
//
// Cerberus #45 / Mantis #1656 — snapshots the router pointer under
// RLock then releases before the (potentially long) network call so
// ApplyDynamicRoutes writes aren't blocked for the inference RTT.
//
// Audit emit shape unchanged from H#179: Requested commits BEFORE the
// network call (so cancellation still leaves a forensic-correlatable
// row); Failed / Completed commits after.
//
// Usage example:
//
//	reply := s.GenerateCtx(c.Request.Context(), "hello")
//	if reply.OK { core.Println(reply.Value) }
func (s *Service) GenerateCtx(ctx context.Context, prompt string) core.Result {
	s.routerMu.RLock()
	router := s.router
	s.routerMu.RUnlock()
	if router == nil {
		return core.Ok(core.Concat("[lthn stub] received: ", prompt))
	}
	// Cerberus #45 / Mantis #1658 — Shape A audit emit at the egress
	// boundary per H#179 surfacing. The Requested row commits BEFORE
	// the network call so a crash OR client-disconnect cancellation
	// mid-call still leaves the request in the audit substrate. Provider/
	// model identifiers from the first configured route (router fallback
	// head); the post-router.Chat Completed emit upgrades to the
	// actually-selected provider/model. Audit emits happen AFTER the
	// RLock release — audit is observability, not router state; H#178
	// mutex semantics untouched.
	provider, model := routerHead(router)
	started := core.Now()
	_ = audit.Default().Record(audit.Event{
		Event:   audit.EventInferenceGenerateRequested,
		TS:      started.Unix(),
		Scope:   "inference",
		Outcome: audit.OutcomeOK,
		Meta: map[string]any{
			"provider":  provider,
			"model":     model,
			"msg_count": 1,
		},
	})
	resp := router.Chat(ctx, ai.ProviderChatRequest{
		Prompt: prompt,
	})
	if !resp.OK {
		_ = audit.Default().Record(audit.Event{
			Event:   audit.EventInferenceGenerateFailed,
			TS:      core.Now().Unix(),
			Scope:   "inference",
			Outcome: audit.OutcomeFailed,
			Meta: map[string]any{
				"provider":              provider,
				"model":                 model,
				audit.MetaKeyErrorCode:  audit.ErrorCode(resp),
				audit.MetaKeyErrorScope: audit.ErrorScope(resp),
			},
		})
		return resp
	}
	chat, ok := resp.Value.(ai.ProviderChatResponse)
	if !ok {
		// Router contract drift — resp.OK was true but the Value
		// payload isn't the documented ai.ProviderChatResponse type.
		// Previously this branch emitted a fraudulent Completed/OK
		// audit row with tokens=0 and returned Ok("") — the chat
		// surface then either persisted a blank reply (fixed in
		// 9a70bdf) or saw a misleading "successful but empty" row
		// in the audit ledger with no signal that the router was
		// the actual fault. Fail loud so the operator can chase
		// the real bug (router refactor, provider mock drift, etc.)
		// instead of suspecting the model.
		_ = audit.Default().Record(audit.Event{
			Event:   audit.EventInferenceGenerateFailed,
			TS:      core.Now().Unix(),
			Scope:   "inference",
			Outcome: audit.OutcomeFailed,
			Meta: map[string]any{
				"provider":              provider,
				"model":                 model,
				audit.MetaKeyErrorCode:  "runner.unexpected_response_type",
				audit.MetaKeyErrorScope: "runner",
				"latency_ms":            core.Since(started).Milliseconds(),
			},
		})
		return core.Fail(core.E("runner.GenerateCtx",
			"router returned unexpected response type", nil))
	}
	selectedProvider := chat.Provider
	if selectedProvider == "" {
		selectedProvider = provider
	}
	selectedModel := chat.ModelID
	if selectedModel == "" {
		selectedModel = model
	}
	_ = audit.Default().Record(audit.Event{
		Event:   audit.EventInferenceGenerateCompleted,
		TS:      core.Now().Unix(),
		Scope:   "inference",
		Outcome: audit.OutcomeOK,
		Meta: map[string]any{
			"provider":   selectedProvider,
			"model":      selectedModel,
			"tokens":     chat.Metrics.GeneratedTokens,
			"latency_ms": core.Since(started).Milliseconds(),
		},
	})
	return core.Ok(chat.Text)
}

// Chat is the messages-array variant of Generate. Background-context
// shim for callers without an HTTP request scope (Action-bus, CLI,
// Wails). HTTP handlers should use ChatCtx so upstream provider RTT
// cancels on client disconnect (Cerberus #60 F-2 / Mantis #1711).
//
// Usage example:
//
//	reply := s.Chat([]inference.Message{
//		{Role: "user", Content: "ping"},
//	})
func (s *Service) Chat(messages []inference.Message) core.Result {
	return s.ChatCtx(core.Background(), messages)
}

// ChatCtx is the ctx-aware variant of Chat. Plumbed through
// pkg/server/handlers.go from gin's c.Request.Context() so a client
// disconnect cancels the upstream router.Chat call instead of letting
// it run to completion and burn quota against the Requested audit row.
//
// Cerberus #60 F-2 / Mantis #1711: see GenerateCtx doc-comment for the
// full STRIDE-D self-DoS rationale.
//
// Usage example:
//
//	reply := s.ChatCtx(c.Request.Context(), []inference.Message{
//		{Role: "user", Content: "ping"},
//	})
func (s *Service) ChatCtx(ctx context.Context, messages []inference.Message) core.Result {
	s.routerMu.RLock()
	router := s.router
	s.routerMu.RUnlock()
	if router == nil {
		last := ""
		for i := len(messages) - 1; i >= 0; i-- {
			if messages[i].Role == "user" {
				last = messages[i].Content
				break
			}
		}
		return core.Ok(core.Concat("[lthn stub] received: ", last))
	}
	// Cerberus #45 / Mantis #1658 — Shape A audit emit at the egress
	// boundary per H#179 surfacing. Messages-array variant of Generate.
	// Mutex semantics from H#178 af2fba9 untouched; emits run AFTER
	// RLock release.
	provider, model := routerHead(router)
	started := core.Now()
	_ = audit.Default().Record(audit.Event{
		Event:   audit.EventInferenceChatRequested,
		TS:      started.Unix(),
		Scope:   "inference",
		Outcome: audit.OutcomeOK,
		Meta: map[string]any{
			"provider":  provider,
			"model":     model,
			"msg_count": len(messages),
		},
	})
	resp := router.Chat(ctx, ai.ProviderChatRequest{
		Messages: messages,
	})
	if !resp.OK {
		_ = audit.Default().Record(audit.Event{
			Event:   audit.EventInferenceChatFailed,
			TS:      core.Now().Unix(),
			Scope:   "inference",
			Outcome: audit.OutcomeFailed,
			Meta: map[string]any{
				"provider":              provider,
				"model":                 model,
				audit.MetaKeyErrorCode:  audit.ErrorCode(resp),
				audit.MetaKeyErrorScope: audit.ErrorScope(resp),
			},
		})
		return resp
	}
	chat, ok := resp.Value.(ai.ProviderChatResponse)
	if !ok {
		// See GenerateCtx for the rationale — same fail-loud
		// substitution for the previous Completed/OK + Ok("")
		// fraudulent-audit pattern.
		_ = audit.Default().Record(audit.Event{
			Event:   audit.EventInferenceChatFailed,
			TS:      core.Now().Unix(),
			Scope:   "inference",
			Outcome: audit.OutcomeFailed,
			Meta: map[string]any{
				"provider":              provider,
				"model":                 model,
				audit.MetaKeyErrorCode:  "runner.unexpected_response_type",
				audit.MetaKeyErrorScope: "runner",
				"latency_ms":            core.Since(started).Milliseconds(),
			},
		})
		return core.Fail(core.E("runner.ChatCtx",
			"router returned unexpected response type", nil))
	}
	selectedProvider := chat.Provider
	if selectedProvider == "" {
		selectedProvider = provider
	}
	selectedModel := chat.ModelID
	if selectedModel == "" {
		selectedModel = model
	}
	_ = audit.Default().Record(audit.Event{
		Event:   audit.EventInferenceChatCompleted,
		TS:      core.Now().Unix(),
		Scope:   "inference",
		Outcome: audit.OutcomeOK,
		Meta: map[string]any{
			"provider":   selectedProvider,
			"model":      selectedModel,
			"tokens":     chat.Metrics.GeneratedTokens,
			"latency_ms": core.Since(started).Milliseconds(),
		},
	})
	return core.Ok(chat.Text)
}

// routerHead returns the (provider, model) identifiers of the first
// configured route on the router — the head of the fallback chain that
// will be tried first. Used to stamp Requested + Failed audit events
// with the route-the-router-was-going-to-try; Completed events stamp
// with the actually-selected route from ProviderChatResponse.
func routerHead(router *ai.ProviderRouter) (provider, model string) {
	if router == nil {
		return "", ""
	}
	routes := router.Providers()
	if len(routes) == 0 {
		return "", ""
	}
	return routes[0].Name, routes[0].ModelID
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
	s.routerMu.RLock()
	router := s.router
	s.routerMu.RUnlock()
	if router == nil {
		return core.Ok([]string{})
	}
	routes := router.Providers()
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
	return core.Ok(NewServiceFromCore(c))
}
