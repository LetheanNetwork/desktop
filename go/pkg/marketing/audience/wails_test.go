// SPDX-Licence-Identifier: EUPL-1.2

package audience_test

import (
	"testing"

	"dappco.re/lthn/desktop/pkg/marketing/audience"
)

func TestListInput_Fields_Good(t *testing.T) {
	in := audience.ListInput{}
	_ = in
}

func TestListOutput_Fields_Good(t *testing.T) {
	out := audience.ListOutput{
		Segments:    []audience.Segment{},
		TotalN:      0,
		TotalGrowth: "+0 / w",
		Spark:       "",
	}
	_ = out.Segments
	_ = out.TotalN
	_ = out.TotalGrowth
	_ = out.Spark
}

func TestCreateInput_Fields_Good(t *testing.T) {
	in := audience.CreateInput{
		Name:   "Local-AI developers",
		N:      4892,
		Growth: "+62 / w",
		Src:    "signup-tagged",
		Spark:  "284,302,318",
	}
	_ = in.Name
	_ = in.N
	_ = in.Growth
	_ = in.Src
	_ = in.Spark
}

func TestUpdateInput_Fields_Good(t *testing.T) {
	in := audience.UpdateInput{
		ID:     "local-ai-developers",
		N:      5000,
		Growth: "+80 / w",
		Src:    "signup-tagged",
		Spark:  "284,302,318",
	}
	_ = in.ID
	_ = in.N
	_ = in.Growth
	_ = in.Src
	_ = in.Spark
}

func TestSegment_Fields_Good(t *testing.T) {
	seg := audience.Segment{
		ID:     "all-subscribers",
		Name:   "All subscribers",
		N:      8214,
		Growth: "+184 / w",
		Src:    "all",
		Spark:  "284,302,318",
	}
	_ = seg.ID
	_ = seg.Name
	_ = seg.N
	_ = seg.Growth
	_ = seg.Src
	_ = seg.Spark
}
