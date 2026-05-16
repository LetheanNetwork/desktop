// SPDX-Licence-Identifier: EUPL-1.2

package pipeline

import core "dappco.re/go"

// EventPipelineMoved is broadcast on the Core ACTION bus when a deal is
// moved between stages via MoveDeal().
//
// Usage example:
//
//	c.Subscribe(pipeline.EventPipelineMoved, func(ctx core.Context, opts core.Options) core.Result {
//	    dealId := opts.String("dealId")
//	    return core.Ok(nil)
//	})
const EventPipelineMoved = "sales.pipeline.moved"

// PipelineMovedEvent is the Core ACTION bus payload for a deal stage move.
//
// Usage example:
//
//	c.RegisterAction(func(c *core.Core, msg core.Message) core.Result {
//	    if ev, ok := msg.(pipeline.PipelineMovedEvent); ok {
//	        _ = ev.DealID
//	        _ = ev.ToStage
//	    }
//	    return core.Result{OK: true}
//	})
type PipelineMovedEvent struct {
	// EventName is the event constant.
	EventName string `json:"event"`
	// DealID is the moved deal's slug.
	DealID string `json:"dealId"`
	// FromStage is the stage the deal was in before the move.
	FromStage string `json:"fromStage"`
	// ToStage is the stage the deal moved to.
	ToStage string `json:"toStage"`
	// At is the event timestamp.
	At core.Time `json:"at"`
}
