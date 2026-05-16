// SPDX-Licence-Identifier: EUPL-1.2

// Package audience is the lthn-side subscriber segment service. Manages
// audience segments at ~/Lethean/marketing/audience/{slug}.md — each
// segment carries a name, count, weekly growth label, and source tag.
//
// Wire shapes match the Segment interface consumed by the
// <lthn-view-audience> Lit element in the Marketing role view.
//
// Usage example (Wails):
//
//	r := audienceSvc.List(audience.ListInput{})
//	if r.OK { out := r.Value.(audience.ListOutput) }
package audience

// Segment is the JSON wire type for a single subscriber segment row.
// Field names match the Segment interface in
// frontend/src/lit/views/marketing/audience.ts.
//
// Usage example:
//
//	seg := audience.Segment{
//	    ID: "local-ai-developers", Name: "Local-AI developers",
//	    N: 4892, Growth: "+62 / w", Src: "signup-tagged",
//	}
type Segment struct {
	// ID is the segment slug (filename key).
	ID string `json:"id"`
	// Name is the segment display name.
	Name string `json:"name"`
	// N is the subscriber count.
	N int `json:"n"`
	// Growth is the pre-formatted weekly growth label ("+184 / w").
	Growth string `json:"growth"`
	// Src is the source tag: "all"|"signup-tagged"|"sales-tagged"|"manual"|"telemetry · opt-in".
	Src string `json:"src"`
	// Spark is the comma-separated weekly count series for the sparkline.
	// Only populated for the "all" aggregate segment.
	Spark string `json:"spark,omitempty"`

	// Version is the monotonic optimistic-lock version (Cascade W2,
	// RFC §B.3 row 6). Stamped by writeSegment; 0 on legacy files
	// predating the cutover, >=1 after first write through
	// paths.AtomicWriteWithVersion. Internal-only — `json:"-"` keeps
	// it out of the Wails wire shape consumed by the frontend.
	Version int `json:"-"`
}

// ListInput drives the List method.
//
// Usage example:
//
//	r := svc.List(audience.ListInput{})
type ListInput struct{}

// ListOutput is the List response envelope.
//
// Usage example:
//
//	out := r.Value.(audience.ListOutput)
//	for _, seg := range out.Segments { _ = seg.Name }
type ListOutput struct {
	// Segments is the ordered list of segments ("all" first, then others).
	Segments []Segment `json:"segments"`
	// TotalN is the total subscriber count (from "all" segment or sum of others).
	TotalN int `json:"totalN"`
	// TotalGrowth is the growth label for the aggregate segment.
	TotalGrowth string `json:"totalGrowth"`
	// Spark is the weekly count series for the hero sparkline.
	Spark string `json:"spark"`
}

// CreateInput drives the Create method.
//
// Usage example:
//
//	r := svc.Create(audience.CreateInput{Name: "Local-AI developers", Src: "signup-tagged"})
type CreateInput struct {
	// Name is the segment display name. Required.
	Name string `json:"name"`
	// N is the initial subscriber count. Defaults to 0.
	N int `json:"n,omitempty"`
	// Growth is the weekly growth label. Defaults to "+0 / w".
	Growth string `json:"growth,omitempty"`
	// Src is the source tag. Required.
	Src string `json:"src"`
	// Spark is the optional weekly series (aggregate only).
	Spark string `json:"spark,omitempty"`
}

// UpdateInput drives the Update method. All fields except ID are optional patches.
//
// Usage example:
//
//	r := svc.Update(audience.UpdateInput{ID: "local-ai-developers", N: 5000})
type UpdateInput struct {
	// ID is the segment slug. Required.
	ID string `json:"id"`
	// N, Growth, Src, Spark — optional patch fields.
	N      int    `json:"n,omitempty"`
	Growth string `json:"growth,omitempty"`
	Src    string `json:"src,omitempty"`
	Spark  string `json:"spark,omitempty"`
}
