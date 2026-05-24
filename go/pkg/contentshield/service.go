// SPDX-Licence-Identifier: EUPL-1.2

package contentshield

import core "dappco.re/go"

// Options configures the ContentShield service.
//
// IncludeSuggestions controls whether the default Score action returns
// the span-level Suggestion list alongside the sycophancy classification.
// Suggestions are useful for inline UI highlighting but add a small
// overhead; telemetry-only callers can skip them. Callers can override
// per-call via the "include_suggestions" option on the action.
type Options struct {
	IncludeSuggestions bool
}

// Service composes the ContentShield primitives behind a Core action
// surface. Registration follows the canonical Mantis #1336 pattern —
// New() builds the service, Register(c) wires actions onto the Core.
//
// ContentShield actions run at guest tier per
// plans/project/lthn/desktop/RFC.guest-mode.md — they execute without
// requiring an unlocked LetheanAccount because the scorer is stateless
// and deterministic, no user data is written, and the input never
// leaves the process.
//
//	svc := contentshield.New(contentshield.Options{IncludeSuggestions: true})
//	svc.Register(c)
//	// Now: c.Action("contentshield.score") is callable from any client.
type Service struct {
	opts Options
}

// New constructs a Service with the given options.
//
// The zero-value Options is valid: suggestions disabled by default.
//
//	svc := contentshield.New(contentshield.Options{})
func New(opts Options) *Service {
	return &Service{opts: opts}
}

// NewService is the alias preferred by Mantis #1336 callers.
//
//	svc := contentshield.NewService(contentshield.Options{})
func NewService(opts Options) *Service {
	return New(opts)
}

// Register wires the ContentShield actions onto the Core runtime.
//
// Actions registered:
//
//   - contentshield.score        — sycophancy + (optional) suggestions
//   - contentshield.suggestions  — span-level suggestions only
//
// All actions execute at guest tier — no LetheanAccount unlock required.
//
//	svc := contentshield.New(opts)
//	r := svc.Register(c)
//	if !r.OK { ... }
func (s *Service) Register(c *core.Core) core.Result {
	if c == nil {
		return core.Fail(core.E("contentshield.Register", "core is nil", nil))
	}

	c.Action("contentshield.score", func(_ core.Context, opts core.Options) core.Result {
		text := opts.String("text")
		result := Score(text)
		if s.opts.IncludeSuggestions || opts.Bool("include_suggestions") {
			result.Suggestions = Suggestions(text)
		}
		return core.Ok(result)
	})

	c.Action("contentshield.score_pair", func(_ core.Context, opts core.Options) core.Result {
		return core.Ok(ScorePair(opts.String("prompt"), opts.String("response")))
	})

	c.Action("contentshield.suggestions", func(_ core.Context, opts core.Options) core.Result {
		return core.Ok(SuggestionsResult{
			Suggestions: Suggestions(opts.String("text")),
		})
	})

	return core.Ok(nil)
}

// Register is the package-level convenience entry point that constructs
// a default Service and wires it onto the Core in one call.
//
//	contentshield.Register(c)  // sufficient for most callers
func Register(c *core.Core) core.Result {
	return NewService(Options{}).Register(c)
}

