// SPDX-Licence-Identifier: EUPL-1.2

package campaigns_test

import (
	"testing"

	"dappco.re/lthn/desktop/pkg/marketing/campaigns"
)

func TestListInput_Fields_Good(t *testing.T) {
	in := campaigns.ListInput{State: "live"}
	_ = in.State
}

func TestListOutput_Fields_Good(t *testing.T) {
	out := campaigns.ListOutput{
		Campaigns:      []campaigns.Campaign{},
		LiveCount:      0,
		ScheduledCount: 0,
	}
	_ = out.Campaigns
	_ = out.LiveCount
	_ = out.ScheduledCount
}

func TestCreateInput_Fields_Good(t *testing.T) {
	in := campaigns.CreateInput{
		Name:    "Product Hunt launch",
		State:   "draft",
		Reach:   "—",
		Convert: "—",
		Spend:   "£0",
		Channel: "earned",
		Body:    "brief here",
	}
	_ = in.Name
	_ = in.State
	_ = in.Reach
	_ = in.Convert
	_ = in.Spend
	_ = in.Channel
	_ = in.Body
}

func TestUpdateInput_Fields_Good(t *testing.T) {
	in := campaigns.UpdateInput{
		ID:      "v02-launch-20260516",
		State:   "complete",
		Reach:   "42 K",
		Convert: "3.2%",
		Spend:   "£0",
		Channel: "earned",
		Body:    "notes",
	}
	_ = in.ID
	_ = in.State
	_ = in.Reach
	_ = in.Convert
	_ = in.Spend
	_ = in.Channel
	_ = in.Body
}

func TestCampaign_Fields_Good(t *testing.T) {
	c := campaigns.Campaign{
		ID:      "v02-launch-20260516",
		Name:    "v0.2 launch",
		State:   "live",
		Reach:   "42 K",
		Convert: "3.2%",
		Spend:   "£0",
		Channel: "earned",
		Body:    "notes",
	}
	_ = c.ID
	_ = c.Name
	_ = c.State
	_ = c.Reach
	_ = c.Convert
	_ = c.Spend
	_ = c.Channel
	_ = c.Body
}
