// SPDX-Licence-Identifier: EUPL-1.2

// Wails surface for the audience service. Methods are bound on the
// *Service receiver and exposed to the frontend via wails3 generate
// bindings — the TS shape lands at
// frontend/bindings/dappco.re/lthn/desktop/pkg/marketing/audience/service.

package audience

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/paths"
)

// List returns all audience segments, with the "all" segment first.
//
// Usage example:
//
//	r := svc.List(audience.ListInput{})
//	if r.OK { out := r.Value.(audience.ListOutput) }
func (s *Service) List(_ ListInput) core.Result {
	segs, err := loadSegments()
	if err != nil {
		return core.Fail(core.E("audience.List", "scan failed", err))
	}

	if segs == nil {
		segs = []Segment{}
	}

	totalN := 0
	totalGrowth := ""
	spark := ""

	// Prefer explicit "all" segment for totals; fall back to sum.
	hasAll := false
	sumN := 0
	for _, seg := range segs {
		if seg.Src == "all" || seg.Name == "All subscribers" {
			totalN = seg.N
			totalGrowth = seg.Growth
			spark = seg.Spark
			hasAll = true
		} else {
			sumN += seg.N
		}
	}
	if !hasAll {
		totalN = sumN
	}

	return core.Ok(ListOutput{
		Segments:    segs,
		TotalN:      totalN,
		TotalGrowth: totalGrowth,
		Spark:       spark,
	})
}

// Get returns a single segment by ID.
//
// Usage example:
//
//	r := svc.Get("local-ai-developers")
//	if r.OK { seg := r.Value.(audience.Segment) }
func (s *Service) Get(id string) core.Result {
	if err := paths.IsValidID(id); err != nil {
		return core.Fail(err)
	}
	dirR := audienceDir()
	if !dirR.OK {
		return core.Fail(core.E("audience.Get", dirR.Error(), nil))
	}
	raw := core.ReadFile(core.PathJoin(dirR.Value.(string), id+".md"))
	if !raw.OK {
		return core.Fail(core.E("audience.Get", "not found: "+id, nil))
	}
	seg, err := parseSegment(raw.Value.([]byte))
	if err != nil {
		return core.Fail(core.E("audience.Get", "parse failed", err))
	}
	return core.Ok(seg)
}

// Create creates a new audience segment record.
//
// Usage example:
//
//	r := svc.Create(audience.CreateInput{Name: "Local-AI developers", Src: "signup-tagged"})
//	if r.OK { seg := r.Value.(audience.Segment) }
func (s *Service) Create(input CreateInput) core.Result {
	if input.Name == "" {
		return core.Fail(core.E("audience.Create", "name is required", nil))
	}
	if input.Src == "" {
		return core.Fail(core.E("audience.Create", "src is required", nil))
	}
	dirR := audienceDir()
	if !dirR.OK {
		return core.Fail(core.E("audience.Create", dirR.Error(), nil))
	}

	growth := input.Growth
	if growth == "" {
		growth = "+0 / w"
	}

	id := slugifyAudience(input.Name)
	if id == "" {
		id = "segment"
	}
	// Cerberus #1486: slugifyAudience is defensive but not a path-safe
	// guarantee — validate before the slug propagates into writeSegment.
	if err := paths.IsValidID(id); err != nil {
		return core.Fail(err)
	}

	seg := Segment{
		ID:     id,
		Name:   input.Name,
		N:      input.N,
		Growth: growth,
		Src:    input.Src,
		Spark:  input.Spark,
	}

	if r := writeSegment(dirR.Value.(string), seg); !r.OK {
		return core.Fail(core.E("audience.Create", "write failed", nil))
	}
	s.fireAudienceEvent(EventAudienceCreated, id, seg.N)
	return core.Ok(seg)
}

// Update applies patch fields to an existing segment record.
//
// Usage example:
//
//	r := svc.Update(audience.UpdateInput{ID: "local-ai-developers", N: 5000})
//	if r.OK { seg := r.Value.(audience.Segment) }
func (s *Service) Update(input UpdateInput) core.Result {
	if err := paths.IsValidID(input.ID); err != nil {
		return core.Fail(err)
	}
	dirR := audienceDir()
	if !dirR.OK {
		return core.Fail(core.E("audience.Update", dirR.Error(), nil))
	}

	raw := core.ReadFile(core.PathJoin(dirR.Value.(string), input.ID+".md"))
	if !raw.OK {
		return core.Fail(core.E("audience.Update", "not found: "+input.ID, nil))
	}
	seg, err := parseSegment(raw.Value.([]byte))
	if err != nil {
		return core.Fail(core.E("audience.Update", "parse failed", err))
	}

	// Patch non-zero fields.
	if input.N != 0 {
		seg.N = input.N
	}
	if input.Growth != "" {
		seg.Growth = input.Growth
	}
	if input.Src != "" {
		seg.Src = input.Src
	}
	if input.Spark != "" {
		seg.Spark = input.Spark
	}

	if r := writeSegment(dirR.Value.(string), seg); !r.OK {
		return core.Fail(core.E("audience.Update", "write failed", nil))
	}
	s.fireAudienceEvent(EventAudienceUpdated, seg.ID, seg.N)
	return core.Ok(seg)
}
