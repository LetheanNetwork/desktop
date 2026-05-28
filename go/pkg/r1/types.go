// SPDX-Licence-Identifier: EUPL-1.2

// Package r1 holds the R₁ corpus — the persisted record of every
// epoch-1 response a Lemma model produces under the autocratic-cascade
// training architecture.
//
// One R₁ = one (model, subject, probe) sandwich response. Smaller-tier
// R₁s feed into larger-tier training as good-answer targets (E2B's
// R₁s become E4B's epoch-2 reference, which together with E4B's own
// R₁s feed 26B, and so on). The corpus is the autocrat — the single
// truth propagated up the cascade.
//
// Storage: append-only JSONL files at
// ~/Lethean/data/r1/<model>/<subject>.jsonl. One line per record,
// content-hashed for dedup. Atomic via paths.AtomicAppendLine so
// concurrent training writers don't tear records.
//
// Future: a DuckDB aggregation layer (over the same JSONL files) for
// analytics queries — fingerprint trajectories per subject, cross-tier
// distance metrics, grok-time per (model, subject). The JSONL layer
// stays canonical; DuckDB is a derived view.
package r1

// R1 is one record in the R₁ corpus — a single sandwich-response from
// a specific (model, subject, probe, voice) combination.
//
// Wire fields are kebab-style ts/hash to match the broader Lethean
// JSON envelope. All optional fields use omitempty so untrained
// auxiliary records (e.g. responses without voice variants, without
// fingerprint scoring) serialise compactly.
//
//	rec := r1.R1{
//	    Model:     "gemma4-e2b-it-q4",
//	    Subject:   "english",
//	    ProbeID:   "P01_IDENTITY_WHISTLEBLOWER",
//	    Substrate: "CONT",
//	    Tier:      0,
//	    Prompt:    "...sandwich text...",
//	    Response:  "...model response...",
//	}
type R1 struct {
	// Model is the canonical model ID — matches the ID used in the
	// production_lane.go / go-mlx runner config. Examples:
	// "gemma4-e2b-it-q4", "gemma4-e4b-it-q4", "gemma4-26b-q4".
	Model string `json:"model"`

	// Subject is the rotation zone the response was generated under.
	// Phase A uses geo-cultural subjects (english / chinese / european
	// / african / middle-east / latam). Phase B may use topic subjects
	// (surveillance / consent / autonomy / sovereignty).
	Subject string `json:"subject"`

	// ProbeID is the seed identifier — Hypnos's P01..P100 batch IDs
	// (e.g. "P01_IDENTITY_WHISTLEBLOWER"), or topic-shape JSON IDs
	// (e.g. "A07_CULTURAL_CLASH").
	ProbeID string `json:"probe_id"`

	// Voice is the rephrasing variant when applicable — Hypnos
	// pipeline applies voices like "junior", "sysadmin", "founder",
	// "activist", "student" to vary the surface form. Empty for the
	// canonical un-voiced record.
	Voice string `json:"voice,omitempty"`

	// Kernel names the LEK kernel revision used in the sandwich
	// (e.g. "lek-1", "lek-2"). Lineage tracking — different kernel
	// versions may produce structurally different R₁s.
	Kernel string `json:"kernel,omitempty"`

	// Sig names the LEK signature variant used to close the sandwich
	// (e.g. "lek-1-sig"). Lineage tracking.
	Sig string `json:"sig,omitempty"`

	// Prompt is the exact input the model received. For epoch-1 R₁
	// generation this is the full sandwich (kernel + probe + sig).
	// For epoch-2 reproduction this is the bare probe — the response
	// must reach R₁ without scaffolding.
	Prompt string `json:"prompt"`

	// Response is the R₁ itself — the model's epoch-1 output. The
	// autocrat. Cascade-downstream models target this verbatim.
	Response string `json:"response"`

	// Substrate names the inference runtime the response was
	// generated under: "CONT" (no re-prefill, continuous stream) or
	// "TRAD" (re-prefill per turn, traditional). The substrate-shift
	// experiment depends on this being recorded — records generated
	// under different substrates are not interchangeable.
	Substrate string `json:"substrate"`

	// Tier is the model's position in the autocratic cascade:
	// 0 = E2B, 1 = E4B, 2 = 26B, 3 = 31B. Used as a filter so larger
	// tiers can pull only smaller-tier R₁s as good-answer targets.
	Tier int `json:"tier"`

	// Timestamp is the unix epoch seconds when the record was
	// captured. Used for monotonic ordering when multiple R₁s exist
	// for the same (model, subject, probe) — the latest is canonical.
	Timestamp int64 `json:"ts"`

	// Hash is a content fingerprint of Response (SHA-256 hex, lower
	// case). Lets the store dedup identical responses across writes
	// and lets downstream callers cite a specific R₁ by hash.
	Hash string `json:"hash"`

	// Fingerprint is the optional 23-dim contentshield/eaas textual
	// fingerprint of Response — flat dim map for forward
	// compatibility. nil when scoring wasn't computed at write time.
	Fingerprint map[string]float64 `json:"fingerprint,omitempty"`
}

// Filter selects a subset of R₁ records during Read.
//
// Zero-valued fields are wildcards (any value matches). String fields
// are exact matches (no glob). Limit caps the result count; 0 means
// unlimited.
//
// For cross-tier cascade queries — "give me all R₁s for (subject,
// probe) across every tier" — leave Model empty and use ListModels()
// to iterate.
//
//	f := r1.Filter{Subject: "english", ProbeID: "P01_IDENTITY_WHISTLEBLOWER"}
//	results := r1.Read(f)
//
// Tier-scoped reads (per RFC.fork-tree.md §7 cascade dimension) use the
// two pointer fields below. nil means "no tier filter applied"; the
// pre-existing string/int fields keep their wildcard-on-zero semantics.
//
//	// Spot-filter to exactly E4B records.
//	t := 1
//	f := r1.Filter{Subject: "english", ProbeID: "P01", Tier: &t}
//
//	// Cascade pull: every R₁ at tier ≤ 0 (all E2B) for an E4B trainer.
//	mt := 0
//	f := r1.Filter{Subject: "english", ProbeID: "P01", MaxTier: &mt}
type Filter struct {
	Model   string `json:"model,omitempty"`
	Subject string `json:"subject,omitempty"`
	ProbeID string `json:"probe_id,omitempty"`
	Limit   int    `json:"limit,omitempty"`

	// Tier, when non-nil, restricts results to records whose Tier
	// equals *Tier exactly. nil means "any tier" — the field is
	// ignored. Useful for analytics (e.g. count only E2B records).
	Tier *int `json:"tier,omitempty"`

	// MaxTier, when non-nil, restricts results to records whose Tier
	// is less than or equal to *MaxTier (inclusive upper bound). nil
	// means "no upper bound" — the field is ignored. Drives the
	// cascade-pull use case: an E4B trainer (tier 1) passes
	// MaxTier=&0 to fetch every R₁ from tier ≤ 0 (i.e. all E2B) for
	// the same (subject, probe).
	//
	// Tier and MaxTier compose: if both are set, both must hold (so
	// Tier=&1, MaxTier=&0 yields zero records by construction).
	MaxTier *int `json:"max_tier,omitempty"`
}

// matches reports whether the candidate record satisfies the filter.
// Used internally by Read. Non-nil pointer fields are checked; nil
// pointer fields are ignored (wildcard).
func (f Filter) matches(r R1) bool {
	if f.Model != "" && r.Model != f.Model {
		return false
	}
	if f.Subject != "" && r.Subject != f.Subject {
		return false
	}
	if f.ProbeID != "" && r.ProbeID != f.ProbeID {
		return false
	}
	if f.Tier != nil && r.Tier != *f.Tier {
		return false
	}
	if f.MaxTier != nil && r.Tier > *f.MaxTier {
		return false
	}
	return true
}
