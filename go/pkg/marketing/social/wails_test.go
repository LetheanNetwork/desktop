// SPDX-Licence-Identifier: EUPL-1.2

package social_test

import (
	"testing"

	"dappco.re/lthn/desktop/pkg/marketing/social"
)

func TestListInput_Fields_Good(t *testing.T) {
	in := social.ListInput{Channel: "mastodon", State: "scheduled"}
	_ = in.Channel
	_ = in.State
}

func TestListOutput_Fields_Good(t *testing.T) {
	out := social.ListOutput{
		Posts:          []social.SocialPost{},
		ScheduledCount: 0,
	}
	_ = out.Posts
	_ = out.ScheduledCount
}

func TestCreateInput_Fields_Good(t *testing.T) {
	in := social.CreateInput{
		Ch:     []string{"mastodon", "x"},
		When:   "today · 16:00",
		Text:   "Lethean v0.2 is out.",
		State:  "scheduled",
		Attach: "image",
	}
	_ = in.Ch
	_ = in.When
	_ = in.Text
	_ = in.State
	_ = in.Attach
}

func TestUpdateInput_Fields_Good(t *testing.T) {
	in := social.UpdateInput{
		ID:     "post-20260516",
		Ch:     []string{"mastodon"},
		When:   "tomorrow · 10:00",
		State:  "draft",
		Text:   "Updated.",
		Attach: "image",
	}
	_ = in.ID
	_ = in.Ch
	_ = in.When
	_ = in.State
	_ = in.Text
	_ = in.Attach
}

func TestSocialPost_Fields_Good(t *testing.T) {
	p := social.SocialPost{
		ID:     "post-20260516",
		Ch:     []string{"mastodon", "x"},
		When:   "today · 16:00",
		State:  "scheduled",
		Text:   "Lethean v0.2 is out.",
		Attach: "image",
	}
	_ = p.ID
	_ = p.Ch
	_ = p.When
	_ = p.State
	_ = p.Text
	_ = p.Attach
}
