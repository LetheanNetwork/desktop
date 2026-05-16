// SPDX-Licence-Identifier: EUPL-1.2

package campaigns

import core "dappco.re/go"

// EventCampaignCreated is broadcast on the Core ACTION bus when a campaign
// is created via Create().
//
// Usage example:
//
//	c.Subscribe(campaigns.EventCampaignCreated, func(ctx core.Context, opts core.Options) core.Result {
//	    id := opts.String("campaignId")
//	    return core.Ok(nil)
//	})
const EventCampaignCreated = "marketing.campaigns.created"

// EventCampaignUpdated is broadcast on the Core ACTION bus when a campaign
// is updated via Update().
const EventCampaignUpdated = "marketing.campaigns.updated"

// CampaignEvent is the Core ACTION bus payload for a campaign state change.
//
// Usage example:
//
//	c.RegisterAction(func(c *core.Core, msg core.Message) core.Result {
//	    if ev, ok := msg.(campaigns.CampaignEvent); ok {
//	        _ = ev.CampaignID
//	    }
//	    return core.Result{OK: true}
//	})
type CampaignEvent struct {
	// EventName is the event constant.
	EventName string `json:"event"`
	// CampaignID is the campaign slug.
	CampaignID string `json:"campaignId"`
	// At is the event timestamp.
	At core.Time `json:"at"`
}
