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
// Dual-format aware (RFC.stage-e-encrypt-at-rest v2 §4.1, Wave 3):
// .lthn files project HEADER-ONLY entries (Name / N / Growth / Src /
// Spark empty — the frontend renders an "encrypted" placeholder); .md
// legacy files round-trip the full plaintext entry. Reads-while-
// locked stays open — header MAC verify needs only the public key (no
// unlock required). The aggregate `size` LogSizeBucket lives in the
// .lthn header for operator-visible-while-locked context but is NOT
// projected onto the wire `n` field (wire `n` is raw subscriber count;
// surfacing the bucket as a stand-in would conflate categories).
//
// Usage example:
//
//	r := svc.List(audience.ListInput{})
//	if r.OK { out := r.Value.(audience.ListOutput) }
func (s *Service) List(_ ListInput) core.Result {
	segs, err := s.loadSegments()
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
// Stage E.D.B.3 surface (RFC.stage-e-encrypt-at-rest v2 §3.1, Wave 3):
// .lthn records require an unlocked session to decrypt the body. When
// loadOne reports the typed "audience.session.locked" code (either
// the gate is unwired or no account is unlocked), Get forwards the
// session-locked failure verbatim so the frontend can distinguish
// "encrypted record needs unlock" from a true not-found.
//
// Usage example:
//
//	r := svc.Get("local-ai-developers")
//	if r.OK { seg := r.Value.(audience.Segment) }
func (s *Service) Get(id string) core.Result {
	if err := paths.IsValidID(id); err != nil {
		return core.Fail(err)
	}
	seg, _, err := s.loadOne(id)
	if err != nil {
		// Forward session.locked verbatim so the frontend can render
		// the unlock-prompt distinct from not-found.
		if core.Contains(err.Error(), "audience.session.locked") {
			return core.Fail(err)
		}
		return core.Fail(core.E("audience.Get", "not found: "+id, err))
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
	if fail, ok := s.assertUnlocked("audience.Create"); !ok {
		return fail
	}
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

	// Cascade W2 (RFC §B.3 row 6) — Create is an unconditional first-
	// write (ifVersion=0). writeSegment stamps Version=1 into the
	// marshalled frontmatter. Conflict-path (rare on Create — only
	// fires if another goroutine races on the same slug) returns
	// core.Fail(paths.ConflictEnvelope{...}) directly via writeSegment.
	if wr := s.writeSegment(dirR.Value.(string), seg, 0); !wr.OK {
		return wr
	}
	seg.Version = 1
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
	if fail, ok := s.assertUnlocked("audience.Update"); !ok {
		return fail
	}
	if err := paths.IsValidID(input.ID); err != nil {
		return core.Fail(err)
	}

	seg, dir, err := s.loadOne(input.ID)
	if err != nil {
		return core.Fail(core.E("audience.Update", "not found: "+input.ID, err))
	}

	priorVersion := seg.Version

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

	// Cascade W2 (RFC §B.3 row 6) — IfVersion=priorVersion gates the
	// write under the primitive's optimistic-lock check.
	// priorVersion=0 (legacy file predating cutover) skips the check
	// and stamps Version=1 on the upgrade write. Conflict-path returns
	// core.Fail(paths.ConflictEnvelope{...}) directly via writeSegment
	// (Mantis #1544 gating shape inherited from W1).
	if wr := s.writeSegment(dir, seg, priorVersion); !wr.OK {
		return wr
	}
	seg.Version = priorVersion + 1
	if seg.Version < 1 {
		seg.Version = 1
	}
	s.fireAudienceEvent(EventAudienceUpdated, seg.ID, seg.N)
	return core.Ok(seg)
}
