// SPDX-Licence-Identifier: EUPL-1.2

// notify.go — Mantis #1488 user-notify half. Terminal-stage moves
// (won / lost) MUST be visible to the operator so a prompt-injection
// that DID land — or any legitimate close — announces itself rather
// than silently flipping the forecast. The handler in wails.go fires
// the typed PipelineDealTerminalEvent below on the Core ACTION bus
// alongside the generic PipelineMovedEvent; the app-shell toast
// subscriber filters on this narrower stream for the explicit pop.
//
// Backend-only scope per the Mantis #1488 SECURITY-NOTE escape valve
// — the frontend toast surface lands as a separate ticket. The typed
// event IS the contract; the toast is the consumer.

package pipeline

import core "dappco.re/go"

// EventPipelineDealTerminal is broadcast on the Core ACTION bus when
// a deal lands in a terminal stage (won or lost). Cerberus pass-9
// HIGH (Mantis #1488) prescribed an explicit user-notify for terminal
// transitions; this is the named-action half of the contract for
// subscribers that only care about close events.
//
// Usage example:
//
//	c.Subscribe(pipeline.EventPipelineDealTerminal, fn)
const EventPipelineDealTerminal = "sales.pipeline.deal_terminal"

// PipelineDealTerminalEvent is the Core ACTION bus payload for a deal
// landing in a terminal stage. Fired ALONGSIDE the PipelineMovedEvent
// (not instead of) — terminal moves emit BOTH so generic audit
// subscribers and notify-only subscribers can wire independently.
//
// Usage example:
//
//	c.RegisterAction(func(c *core.Core, msg core.Message) core.Result {
//	    if ev, ok := msg.(pipeline.PipelineDealTerminalEvent); ok {
//	        // surface a toast / tray ping for ev.ToStage ∈ {won, lost}
//	    }
//	    return core.Result{OK: true}
//	})
type PipelineDealTerminalEvent struct {
	// EventName is the event constant.
	EventName string `json:"event"`
	// DealID is the deal that reached the terminal stage.
	DealID string `json:"dealId"`
	// FromStage is the stage the deal left.
	FromStage string `json:"fromStage"`
	// ToStage is the terminal stage — always "won" or "lost".
	ToStage string `json:"toStage"`
	// Force reflects whether the move bypassed IsLegalTransition via
	// MoveInput.Force.
	Force bool `json:"force,omitempty"`
	// At is the event timestamp.
	At core.Time `json:"at"`
}

// fireTerminal publishes a terminal-stage event on the Core ACTION
// bus for moves that land in "won" or "lost". The app-shell toast
// subscriber filters on this typed payload to surface a visible pop
// without filtering every PipelineMovedEvent.
//
// Usage example:
//
//	s.fireTerminal("202605-DEAL-001", "close", "won", false)
func (s *Service) fireTerminal(dealID, fromStage, toStage string, force bool) {
	if s == nil || s.core == nil {
		return
	}
	s.core.ACTION(PipelineDealTerminalEvent{
		EventName: EventPipelineDealTerminal,
		DealID:    dealID,
		FromStage: fromStage,
		ToStage:   toStage,
		Force:     force,
		At:        core.Now().UTC(),
	})
}
