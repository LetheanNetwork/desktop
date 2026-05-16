// SPDX-Licence-Identifier: EUPL-1.2

package deals

import core "dappco.re/go"

// EventDealCreated is broadcast on the Core ACTION bus when a new deal
// is created via Create().
//
// Usage example:
//
//	c.Subscribe(deals.EventDealCreated, func(ctx core.Context, opts core.Options) core.Result {
//	    id := opts.String("dealId")
//	    return core.Ok(nil)
//	})
const EventDealCreated = "sales.deals.created"

// EventDealStageChanged is broadcast when UpdateStage() transitions a
// deal's pipeline stage.
//
// Usage example:
//
//	c.Subscribe(deals.EventDealStageChanged, func(ctx core.Context, opts core.Options) core.Result {
//	    stage := opts.String("stage")
//	    return core.Ok(nil)
//	})
const EventDealStageChanged = "sales.deals.stage_changed"

// DealEvent is the Core ACTION bus payload for deal lifecycle changes.
//
// Usage example:
//
//	c.RegisterAction(func(c *core.Core, msg core.Message) core.Result {
//	    if ev, ok := msg.(deals.DealEvent); ok {
//	        _ = ev.DealID
//	        _ = ev.Stage
//	    }
//	    return core.Result{OK: true}
//	})
type DealEvent struct {
	// EventName is the event constant that triggered this message.
	EventName string `json:"event"`
	// DealID is the deal slug.
	DealID string `json:"dealId"`
	// Stage is the current stage after the change.
	Stage string `json:"stage"`
	// Customer is the deal's counterparty name.
	Customer string `json:"customer"`
	// At is the event timestamp.
	At core.Time `json:"at"`
}
