// SPDX-Licence-Identifier: EUPL-1.2

package content_test

import (
	"testing"

	"dappco.re/lthn/desktop/pkg/marketing/content"
)

func TestListInput_Fields_Good(t *testing.T) {
	in := content.ListInput{Col: "draft"}
	_ = in.Col
}

func TestListOutput_Fields_Good(t *testing.T) {
	out := content.ListOutput{
		Columns:       []content.ContentColumn{},
		TotalInFlight: 0,
		DueToday:      0,
	}
	_ = out.Columns
	_ = out.TotalInFlight
	_ = out.DueToday
}

func TestCreateInput_Fields_Good(t *testing.T) {
	in := content.CreateInput{
		T:    "v0.2 release notes",
		Col:  "draft",
		Who:  "you",
		Due:  "today",
		Body: "outline here",
	}
	_ = in.T
	_ = in.Col
	_ = in.Who
	_ = in.Due
	_ = in.Body
}

func TestUpdateInput_Fields_Good(t *testing.T) {
	in := content.UpdateInput{
		ID:   "v02-release-notes-20260516",
		Col:  "review",
		Who:  "you → Mei",
		Due:  "",
		When: "just now",
		Body: "notes",
	}
	_ = in.ID
	_ = in.Col
	_ = in.Who
	_ = in.Due
	_ = in.When
	_ = in.Body
}

func TestContentItem_Fields_Good(t *testing.T) {
	item := content.ContentItem{
		ID:   "v02-release-notes-20260516",
		T:    "v0.2 release notes",
		Who:  "you",
		When: "3 d ago",
		Due:  "today",
		Col:  "draft",
		Body: "outline",
	}
	_ = item.ID
	_ = item.T
	_ = item.Who
	_ = item.When
	_ = item.Due
	_ = item.Col
	_ = item.Body
}

func TestContentColumn_Fields_Good(t *testing.T) {
	col := content.ContentColumn{
		ID:    "draft",
		Label: "Drafting",
		Items: []content.ContentItem{},
	}
	_ = col.ID
	_ = col.Label
	_ = col.Items
}
