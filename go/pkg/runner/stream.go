// SPDX-Licence-Identifier: EUPL-1.2

package runner

import (
	"context"

	core "dappco.re/go"
	"dappco.re/go/inference"
	"dappco.re/go/inference/agent/ai"
	"dappco.re/go/render/display/webkit"

	"dappco.re/lthn/desktop/pkg/audit"
)

const (
	eventChatDelta  = "runner:chat:delta"
	eventChatDone   = "runner:chat:done"
	eventChatFailed = "runner:chat:failed"

	maxStreamCallIDBytes = 128
)

// WChatStream streams one assistant turn over the renderer event bus while
// also returning the final ChatReply to the binding caller. Each delta, done,
// and failed event carries call_id so concurrent webview requests cannot
// consume one another's tokens. Empty route preserves the current fallback
// order; a non-empty route/provider selector narrows the request.
func (s *Service) WChatStream(
	callID string,
	messages []inference.Message,
	route string,
) core.Result {
	if core.Trim(callID) == "" {
		return core.Fail(core.E("runner.Service.WChatStream",
			"call id is required", nil))
	}
	if len(callID) > maxStreamCallIDBytes {
		return core.Fail(core.E("runner.Service.WChatStream",
			core.Sprintf("call id exceeds %d byte cap", maxStreamCallIDBytes), nil))
	}
	if r := validateChatMessages("runner.Service.WChatStream", messages); !r.OK {
		reportStreamSideEffect("failed event", s.emitChatFailure(callID, r))
		return r
	}

	routerR := s.routerForSelector(route)
	if !routerR.OK {
		reportStreamSideEffect("failed event", s.emitChatFailure(callID, routerR))
		return routerR
	}
	router, _ := routerR.Value.(*ai.ProviderRouter)
	if router == nil {
		text := core.Concat("[lthn stub] received: ", lastUserMessage(messages))
		if r := s.emitChatEvent(eventChatDelta, map[string]any{
			"call_id":  callID,
			"delta":    text,
			"provider": "",
			"model":    "",
		}); !r.OK {
			return r
		}
		if r := s.emitChatEvent(eventChatDone, map[string]any{
			"call_id":   callID,
			"text":      text,
			"warn_user": false,
			"provider":  "",
			"model":     "",
		}); !r.OK {
			return r
		}
		return core.Ok(ChatReply{Text: text})
	}

	return s.streamRouterChat(core.Background(), callID, messages, router)
}

func (s *Service) streamRouterChat(
	ctx context.Context,
	callID string,
	messages []inference.Message,
	router *ai.ProviderRouter,
) core.Result {
	provider, model := routerHead(router)
	started := core.Now()

	welfareState := s.applyWelfare(ctx, messages, router, provider, model)
	if welfareState.synthetic != "" {
		return s.completeSyntheticStream(callID, welfareState.synthetic)
	}
	messages = welfareState.messages

	reportStreamSideEffect("requested audit", audit.Default().Record(audit.Event{
		Event:   audit.EventInferenceChatRequested,
		TS:      started.Unix(),
		Scope:   "inference",
		Outcome: audit.OutcomeOK,
		Meta: map[string]any{
			"provider":  provider,
			"model":     model,
			"msg_count": len(messages),
		},
	}))

	// lastFailure defaults to the zero Result and is only pre-seeded with
	// the "all providers failed" sentinel when the loop below has no
	// iterations to run it in (an empty provider list) — every other exit
	// path reassigns it before use. This keeps the sentinel's core.E/
	// core.Fail construction off the success path, which is the
	// overwhelming common case and would otherwise pay for a Result that
	// gets thrown away unread on every successful chat turn.
	providers := router.Providers()
	var lastFailure core.Result
	if len(providers) == 0 {
		lastFailure = core.Fail(core.E("runner.Service.WChatStream",
			"all providers failed", nil))
	}
	for _, providerRoute := range providers {
		if err := ctx.Err(); err != nil {
			lastFailure = core.Fail(core.E("runner.Service.WChatStream",
				"request cancelled", err))
			break
		}

		builder := core.NewBuilder()
		emitted := false
		eventFailed := core.Result{}
		eventDeliveryFailed := false
		for token := range providerRoute.Model.Chat(ctx, messages) {
			if token.Text == "" {
				continue
			}
			emitted = true
			builder.WriteString(token.Text)
			eventFailed = s.emitChatEvent(eventChatDelta, map[string]any{
				"call_id":  callID,
				"delta":    token.Text,
				"provider": providerRoute.Name,
				"model":    providerRoute.ModelID,
			})
			if !eventFailed.OK {
				eventDeliveryFailed = true
				break
			}
		}
		if eventDeliveryFailed {
			lastFailure = eventFailed
			break
		}

		modelResult := providerRoute.Model.Err()
		if !modelResult.OK {
			lastFailure = modelResult
			// Once a provider has emitted deltas they are visible to the
			// renderer and cannot safely be retracted before falling back.
			if emitted {
				break
			}
			continue
		}

		text := builder.String()
		reportStreamSideEffect("completed audit", audit.Default().Record(audit.Event{
			Event:   audit.EventInferenceChatCompleted,
			TS:      core.Now().Unix(),
			Scope:   "inference",
			Outcome: audit.OutcomeOK,
			Meta: map[string]any{
				"provider":   providerRoute.Name,
				"model":      providerRoute.ModelID,
				"tokens":     providerRoute.Model.Metrics().GeneratedTokens,
				"latency_ms": core.Since(started).Milliseconds(),
			},
		}))
		done := s.emitChatEvent(eventChatDone, map[string]any{
			"call_id":   callID,
			"text":      text,
			"warn_user": welfareState.warnUser,
			"provider":  providerRoute.Name,
			"model":     providerRoute.ModelID,
		})
		if !done.OK {
			return done
		}
		return core.Ok(ChatReply{Text: text, WarnUser: welfareState.warnUser})
	}

	s.auditStreamFailure(started.UnixMilli(), provider, model, lastFailure)
	reportStreamSideEffect("failed event", s.emitChatFailure(callID, lastFailure))
	cause, _ := lastFailure.Value.(error)
	return core.Fail(core.E("runner.Service.WChatStream",
		"chat stream failed: "+lastFailure.Error(), cause))
}

func (s *Service) completeSyntheticStream(callID string, text string) core.Result {
	if r := s.emitChatEvent(eventChatDelta, map[string]any{
		"call_id": callID,
		"delta":   text,
	}); !r.OK {
		return r
	}
	if r := s.emitChatEvent(eventChatDone, map[string]any{
		"call_id":   callID,
		"text":      text,
		"warn_user": false,
	}); !r.OK {
		return r
	}
	return core.Ok(ChatReply{Text: text})
}

func (s *Service) emitChatEvent(name string, data map[string]any) core.Result {
	if s != nil && s.eventEmitter != nil {
		return s.eventEmitter(name, data)
	}
	if s == nil {
		return core.Fail(core.E("runner.Service.emitChatEvent",
			"service is nil", nil))
	}
	return webkit.EmitEvent(s.core, name, data)
}

func (s *Service) emitChatFailure(callID string, failure core.Result) core.Result {
	return s.emitChatEvent(eventChatFailed, map[string]any{
		"call_id": callID,
		"error":   failure.Error(),
	})
}

func (s *Service) auditStreamFailure(
	startedMillis int64,
	provider string,
	model string,
	failure core.Result,
) {
	reportStreamSideEffect("failed audit", audit.Default().Record(audit.Event{
		Event:   audit.EventInferenceChatFailed,
		TS:      core.Now().Unix(),
		Scope:   "inference",
		Outcome: audit.OutcomeFailed,
		Meta: map[string]any{
			"provider":              provider,
			"model":                 model,
			audit.MetaKeyErrorCode:  audit.ErrorCode(failure),
			audit.MetaKeyErrorScope: audit.ErrorScope(failure),
			"latency_ms":            core.Now().UnixMilli() - startedMillis,
		},
	}))
}

func reportStreamSideEffect(name string, result core.Result) {
	if result.OK {
		return
	}
	core.Print(core.Stderr(), "runner stream %s failed: %s\n", name, result.Error())
}

func lastUserMessage(messages []inference.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			return messages[i].Content
		}
	}
	return ""
}
