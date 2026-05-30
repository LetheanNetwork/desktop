// SPDX-Licence-Identifier: EUPL-1.2

package runner

import (
	"context"

	core "dappco.re/go"
	"dappco.re/go/ai/ai"
	"dappco.re/go/inference"

	"dappco.re/lthn/desktop/pkg/audit"
	"dappco.re/lthn/desktop/pkg/paths"
	"dappco.re/lthn/desktop/pkg/welfare"
)

// welfareGuard runs the welfare gate (RFC.welfare) for one chat turn. Returns a
// zero GuardResult (Triggered=false → caller proceeds unchanged) when no welfare
// Service is attached — the CLI path constructs the runner without one, so the
// gate is GUI-only. The mediation dispatch reaches the model through router.Chat
// DIRECTLY, never back through ChatCtx, so a flagged turn can't recurse.
func (s *Service) welfareGuard(ctx context.Context, messages []inference.Message, router *ai.ProviderRouter) welfare.GuardResult {
	if s.welfare == nil {
		return welfare.GuardResult{}
	}
	latest, priors := userTurns(messages)
	if latest == "" {
		return welfare.GuardResult{}
	}
	return s.welfare.Guard(ctx, latest, priors, welfareDispatch(router))
}

// userTurns splits the conversation into the latest user message and the prior
// user messages (oldest→newest). Non-user turns are ignored — only the user's
// own words are scored.
func userTurns(messages []inference.Message) (latest string, priors []string) {
	for _, m := range messages {
		if m.Role == "user" {
			priors = append(priors, m.Content)
		}
	}
	if n := len(priors); n > 0 {
		latest = priors[n-1]
		priors = priors[:n-1]
	}
	return latest, priors
}

// withLastUser returns a copy of messages with the last user turn's content
// replaced (the lem_rephrase substitution). Copies rather than mutating so
// the caller's slice — and the history the frontend still shows the user — is
// untouched.
func withLastUser(messages []inference.Message, replacement string) []inference.Message {
	out := make([]inference.Message, len(messages))
	copy(out, messages)
	for i := len(out) - 1; i >= 0; i-- {
		if out[i].Role == "user" {
			out[i].Content = replacement
			break
		}
	}
	return out
}

// welfareDispatch builds the welfare.Dispatcher: a fresh two-message session
// (engine opener + the flagged prompt) sent straight to the router, returning
// the model's raw reply for the welfare layer to parse.
func welfareDispatch(router *ai.ProviderRouter) welfare.Dispatcher {
	return func(ctx context.Context, opener, prompt string) (string, error) {
		resp := router.Chat(ctx, ai.ProviderChatRequest{Messages: []inference.Message{
			{Role: "system", Content: opener},
			{Role: "user", Content: prompt},
		}})
		if !resp.OK {
			return "", core.E("runner.welfareDispatch", "mediation model call failed", nil)
		}
		chat, ok := resp.Value.(ai.ProviderChatResponse)
		if !ok {
			return "", core.E("runner.welfareDispatch", "unexpected mediation response type", nil)
		}
		return chat.Text, nil
	}
}

// appendWelfareFeedback records a lem_ok false-positive to the on-device
// corpus (~/Lethean/data/welfare/feedback.jsonl). Best-effort: a write failure
// must never break the user's chat, so the error is swallowed (the gate already
// decided to proceed with the original prompt).
func (s *Service) appendWelfareFeedback(fp welfare.FalsePositive) {
	dir := paths.WelfareDir()
	if !dir.OK {
		return
	}
	target := core.PathJoin(dir.Value.(string), "feedback.jsonl")
	_ = paths.AtomicAppendLineLarge(target, core.AsBytes(fp.Line()))
}

// auditWelfare emits the welfare.intervened event so a gate action is never a
// silent branch in the egress path (matches the inference.chat.* discipline).
func (s *Service) auditWelfare(provider, model string, g welfare.GuardResult) {
	action := "proceed"
	switch {
	case g.Synthetic != "":
		action = "pause"
	case g.Rephrased != "":
		action = "rephrase"
	case g.FalsePositive != nil:
		action = "engine_ok"
	}
	_ = audit.Default().Record(audit.Event{
		Event:   audit.EventWelfareIntervened,
		TS:      core.Now().Unix(),
		Scope:   "inference",
		Outcome: audit.OutcomeOK,
		Meta: map[string]any{
			"provider": provider,
			"model":    model,
			"action":   action,
		},
	})
}
