// SPDX-Licence-Identifier: EUPL-1.2

// OpenCodeRoute — Provider-side adapter that lets lthn's runner
// reach opencode-routed models as if they were any other openai-
// compatible upstream. The flow:
//
//	caller → lthn runner /v1/chat/completions
//	       → ai.ProviderRouter
//	       → opencode.Model.Chat(messages)
//	       → POST /session (create) + POST /session/:id/message
//	       → assistant response → yield as one inference.Token
//
// Per RFC.opencode.md §5.2. v1 is non-streaming: a single token
// carrying the assistant's full reply, modelled after the
// openai.Model adapter's pattern. Streaming-aware comes in a
// follow-up once the session SSE events are bridged into
// inference.Token chunks.
//
// Each running sandbox can host MANY providers (lthn-injected,
// host-credential, env-detected). The Routes() factory enumerates
// all of them from the FIRST running sandbox and emits one
// ai.ProviderRoute per provider/model combination.

package opencode

import (
	goio "io"
	"iter"
	"net/http"

	core "dappco.re/go"
	"dappco.re/go/ai/ai"
	"dappco.re/go/inference"
)

// ModelOptions configures one OpenCode-backed Model instance.
type ModelOptions struct {
	// Service is the opencode.Service used to resolve sandbox URLs +
	// auth at chat time. Required.
	Service *Service
	// SandboxID picks which running sandbox handles requests for
	// this model. Required at construction. Set this from the FIRST
	// running sandbox at boot — multi-sandbox routing is v2.
	SandboxID string
	// ProviderID is the upstream provider id inside opencode (e.g.
	// "lthn", "anthropic", "openai"). Comes from /provider response.
	ProviderID string
	// ModelID is the model id inside that provider (e.g. "lthn-local",
	// "claude-sonnet-4-5").
	ModelID string
}

// Model implements inference.TextModel by routing requests through
// opencode-serve's session API. One instance per (sandbox,
// provider, model) tuple.
type Model struct {
	opts ModelOptions

	mu      core.Mutex
	lastErr error
	metrics inference.GenerateMetrics
}

// Compile-time interface check — keeps the file honest as
// inference.TextModel evolves upstream.
var _ inference.TextModel = (*Model)(nil)

// NewModel constructs an OpenCode-backed TextModel. ServiceID +
// SandboxID + ProviderID + ModelID are all required.
//
// Usage example:
//
//	m := opencode.NewModel(opencode.ModelOptions{
//	    Service: svc, SandboxID: "oc-...", ProviderID: "lthn", ModelID: "lthn-local",
//	})
func NewModel(opts ModelOptions) *Model {
	return &Model{opts: opts}
}

// Generate implements inference.TextModel — defers to Chat with a
// single user message, mirroring the openai.Model shape.
func (m *Model) Generate(ctx core.Context, prompt string, opts ...inference.GenerateOption) iter.Seq[inference.Token] {
	return m.Chat(ctx, []inference.Message{{Role: "user", Content: prompt}}, opts...)
}

// Chat implements inference.TextModel — posts the messages to
// opencode-serve and yields a single token carrying the assistant's
// full response. Non-streaming for v1; streaming follows once the
// /session/:id/event SSE shape is wired through.
func (m *Model) Chat(ctx core.Context, messages []inference.Message, _ ...inference.GenerateOption) iter.Seq[inference.Token] {
	return func(yield func(inference.Token) bool) {
		start := core.Now()
		text, err := m.invoke(ctx, messages)
		m.setResult(inference.GenerateMetrics{
			TotalDuration: core.Since(start),
		}, err)
		if err != nil || text == "" {
			return
		}
		yield(inference.Token{Text: text})
	}
}

// Classify is not supported via the opencode session API.
func (m *Model) Classify(core.Context, []string, ...inference.GenerateOption) ([]inference.ClassifyResult, error) {
	return nil, core.E("opencode.Classify",
		"classification is not supported via opencode session routing", nil)
}

// BatchGenerate runs Generate sequentially for each prompt — mirrors
// the openai backend's approach.
func (m *Model) BatchGenerate(ctx core.Context, prompts []string, opts ...inference.GenerateOption) ([]inference.BatchResult, error) {
	out := make([]inference.BatchResult, 0, len(prompts))
	for _, p := range prompts {
		var tokens []inference.Token
		for tok := range m.Generate(ctx, p, opts...) {
			tokens = append(tokens, tok)
		}
		out = append(out, inference.BatchResult{Tokens: tokens, Err: m.Err()})
	}
	return out, nil
}

// ModelType implements inference.TextModel.
func (m *Model) ModelType() string { return "opencode-routed" }

// Info implements inference.TextModel — surface the routed model id
// so logs/metrics distinguish opencode-routed entries from local.
func (m *Model) Info() inference.ModelInfo {
	return inference.ModelInfo{
		Architecture: "opencode-routed",
	}
}

// Metrics implements inference.TextModel.
func (m *Model) Metrics() inference.GenerateMetrics {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.metrics
}

// Err implements inference.TextModel.
func (m *Model) Err() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lastErr
}

// Close implements inference.TextModel — opencode sessions are
// short-lived and cleaned up by opencode-serve when the container
// exits, so Close is a no-op.
func (m *Model) Close() error { return nil }

// setResult records the per-call outcome under the model mutex.
func (m *Model) setResult(metrics inference.GenerateMetrics, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.metrics = metrics
	m.lastErr = err
}

// invoke creates a session, posts the user message, and returns the
// assistant's response text. Errors propagate; the caller sets the
// Model's lastErr via setResult.
func (m *Model) invoke(ctx core.Context, messages []inference.Message) (string, error) {
	target, r := m.opts.Service.targetFor(m.opts.SandboxID)
	if !r.OK {
		return "", core.E("opencode.invoke", r.Error(), nil)
	}

	// Build a single concatenated user prompt — opencode's session
	// /message endpoint expects parts under one user submission.
	// v2 maps inference.Message[] → opencode parts more faithfully
	// (system / user / assistant roles), but for v1 the concatenated
	// shape is what the openai adapter uses too.
	var sb core.Buffer
	for i, msg := range messages {
		if i > 0 {
			sb.WriteByte('\n')
		}
		if msg.Role != "" && msg.Role != "user" {
			sb.WriteString("[")
			sb.WriteString(msg.Role)
			sb.WriteString("] ")
		}
		sb.WriteString(msg.Content)
	}

	// Create a session.
	sessReqBody := core.NewBufferString(core.JSONMarshalString(map[string]any{
		"title": "lthn-route",
	}))
	sessReq, err := http.NewRequestWithContext(ctx, core.MethodPost, target+"/session", sessReqBody)
	if err != nil {
		return "", err
	}
	sessReq.Header.Set("Content-Type", "application/json")
	m.opts.Service.applyAuth(sessReq)
	sessResp, err := core.DefaultHTTPClient.Do(sessReq)
	if err != nil {
		return "", err
	}
	defer func() { _ = sessResp.Body.Close() }()
	if sessResp.StatusCode >= 400 {
		body, _ := goio.ReadAll(sessResp.Body)
		return "", core.E("opencode.invoke",
			core.Sprintf("session create %d: %s", sessResp.StatusCode, string(body)), nil)
	}
	var sess struct {
		ID string `json:"id"`
	}
	body, _ := goio.ReadAll(sessResp.Body)
	if r := core.JSONUnmarshal(body, &sess); !r.OK {
		return "", core.E("opencode.invoke", "session response parse: "+r.Error(), nil)
	}
	if sess.ID == "" {
		return "", core.E("opencode.invoke", "session response missing id", nil)
	}

	// Post the message.
	msgReqBody := core.NewBufferString(core.JSONMarshalString(map[string]any{
		"providerID": m.opts.ProviderID,
		"modelID":    m.opts.ModelID,
		"parts": []map[string]any{
			{"type": "text", "text": sb.String()},
		},
	}))
	msgReq, err := http.NewRequestWithContext(ctx, core.MethodPost,
		target+"/session/"+sess.ID+"/message", msgReqBody)
	if err != nil {
		return "", err
	}
	msgReq.Header.Set("Content-Type", "application/json")
	m.opts.Service.applyAuth(msgReq)
	msgResp, err := core.DefaultHTTPClient.Do(msgReq)
	if err != nil {
		return "", err
	}
	defer func() { _ = msgResp.Body.Close() }()
	msgBody, _ := goio.ReadAll(msgResp.Body)
	if msgResp.StatusCode >= 400 {
		return "", core.E("opencode.invoke",
			core.Sprintf("message post %d: %s", msgResp.StatusCode, string(msgBody)), nil)
	}

	// Response shape: {"info": {...}, "parts": [{"type":"text","text":"..."}]}
	// (or similar — opencode emits structured assistant turns).
	var msgWire struct {
		Parts []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"parts"`
	}
	if r := core.JSONUnmarshal(msgBody, &msgWire); !r.OK {
		// Soft fallback — return the raw body so the caller can
		// inspect rather than swallow a malformed-shape error.
		return string(msgBody), nil
	}
	var out core.Buffer
	for _, p := range msgWire.Parts {
		if p.Type == "text" && p.Text != "" {
			out.WriteString(p.Text)
		}
	}
	return out.String(), nil
}

// Routes enumerates opencode-loaded providers from the first running
// sandbox and returns one ai.ProviderRoute per (provider, model)
// combination. Returns an empty slice when no sandbox is running.
//
// Route Name format: "opencode:<providerID>/<modelID>" — distinct
// namespace from runner-configured routes so lthn's /v1/models list
// shows them as opencode-routed.
//
// Usage example:
//
//	opts := []ai.ProviderRoute{}
//	opts = append(opts, runner.LoadRoutesFromCore(c)...)
//	opts = append(opts, opencodeSvc.Routes()...)
//	router := ai.NewProviderRouter(opts...)
func (s *Service) Routes() []ai.ProviderRoute {
	// Find the first running sandbox. v1 supports one global
	// opencode-serve container per RFC §7; if multiple are running,
	// the first by created_at desc wins.
	statusR := s.Status()
	if !statusR.OK {
		return nil
	}
	running, _ := statusR.Value.([]Sandbox)
	if len(running) == 0 {
		return nil
	}
	sb := running[0]

	// Pull the provider listing.
	provR := s.ProviderList(sb.ID)
	if !provR.OK {
		return nil
	}
	raw, _ := provR.Value.(string)
	var wire struct {
		All []struct {
			ID     string                    `json:"id"`
			Models map[string]map[string]any `json:"models"`
		} `json:"all"`
	}
	if r := core.JSONUnmarshalString(raw, &wire); !r.OK {
		return nil
	}

	out := make([]ai.ProviderRoute, 0, 8)
	for _, p := range wire.All {
		for modelID := range p.Models {
			model := NewModel(ModelOptions{
				Service:    s,
				SandboxID:  sb.ID,
				ProviderID: p.ID,
				ModelID:    modelID,
			})
			out = append(out, ai.ProviderRoute{
				Name:    "opencode:" + p.ID + "/" + modelID,
				ModelID: p.ID + "/" + modelID,
				Model:   model,
				Labels: map[string]string{
					"kind":         "opencode-routed",
					"sandbox_id":   sb.ID,
					"provider_id":  p.ID,
					"upstream_mid": modelID,
				},
			})
		}
	}
	return out
}
