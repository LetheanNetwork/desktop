// SPDX-Licence-Identifier: EUPL-1.2

// Package audience is the lthn-side subscriber segment service. Manages
// audience segments at ~/Lethean/marketing/audience/{slug}.lthn — each
// segment carries a name, count, weekly growth label, and source tag.
//
// Stage E.D.B.3 (Mantis #1487 wave 3): the on-disk format is now the
// encrypted Trix envelope (`.lthn`). Legacy `.md` plaintext records
// remain readable via the lazy-migration fallthrough until a write
// promotes them to `.lthn` per RFC §3.1. Header carries the segment
// `size` projected through recordfile.LogSizeBucket (`<1k` | `1-10k` |
// `10-100k` | `100k+`) per RFC §2.4 + Cerberus C#7 Q2 so operators see
// bucketed magnitude while the session is LOCKED without leaking the
// raw subscriber count.
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

// SegmentRecord is the persistence type carried through the at-rest
// substrate as `AtRestWriter[SegmentRecord]`. Stage E.D.B.3 (Mantis
// #1487 wave 3) splits the wire type (Segment, json-tagged) from the
// persistence type (SegmentRecord, yaml-tagged) so the encrypted body
// frontmatter never leaks json key names and the
// recordfile.HeaderSchema[SegmentRecord] generic stays type-pinned.
//
// Per-field MUST per RFC §2.4 marketing/audience row + Cerberus C#7
// Q2 ruling:
//
//   - `name`   → BODY (REJECT in header — PII-adjacent / segment
//     branding can reveal go-to-market intent).
//   - `size`   → HEADER as LOG-BUCKET enum (recordfile.LogSizeBucket
//     of the integer N). Raw integer N MUST NEVER appear in the
//     plaintext header — operators see bucketed magnitude only.
//   - segment criteria + member list → BODY (member identities are PII
//     of the named subscribers; criteria reveals targeting strategy).
//
// All other fields (growth / src / spark) are BODY-only — they're
// either reveal-rate-of-change (growth) or carry targeting / source
// vocabulary (src / spark) that operator-while-locked should not see.
//
// Usage example:
//
//	rec := audience.SegmentRecord{
//	    ID: "local-ai-developers", Name: "Local-AI developers",
//	    N: 4892, Growth: "+62 / w", Src: "signup-tagged",
//	}
type SegmentRecord struct {
	// ID is the canonical segment identifier (matches filename slug).
	ID string `yaml:"id"`

	// Name is the segment display name. BODY-only per RFC §2.4 —
	// MUST NEVER appear in the at-rest header.
	Name string `yaml:"name"`

	// N is the integer subscriber count. BODY-only — the header carries
	// only the LogSizeBucket projection (Cerberus C#7 Q2). Raw N is the
	// sensitive value (precise audience reach), the bucket is the
	// operator-visible-while-locked summary.
	N int `yaml:"n"`

	// Growth is the pre-formatted weekly growth label ("+184 / w").
	// BODY-only — rate-of-change is sensitive (reveals momentum).
	Growth string `yaml:"growth"`

	// Src is the source tag: "all"|"signup-tagged"|"sales-tagged"|
	// "manual"|"telemetry · opt-in". BODY-only — RFC §2.4 names no
	// header key for src, default-body rule applies (also: src reveals
	// targeting strategy / data lineage which operators-while-locked
	// should not have).
	Src string `yaml:"src"`

	// Spark is the comma-separated weekly count series for the
	// sparkline. BODY-only — count series reveals exact magnitudes
	// already covered by the BODY-only N policy.
	Spark string `yaml:"spark"`

	// Version is the monotonic optimistic-lock version (Cascade W2,
	// RFC §B.3 row 6). Stamped by writeSegment; 0 on legacy files
	// predating the cutover, >=1 after first write through
	// paths.AtomicWriteWithVersion / the at-rest substrate.
	//
	// omitempty keeps legacy round-trips clean (Version=0); the first
	// write stamps version=1.
	Version int `yaml:"version,omitempty"`
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
