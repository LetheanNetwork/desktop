// SPDX-Licence-Identifier: EUPL-1.2

// Package pipeline is the lthn-side sales pipeline service. Derives the
// Kanban pipeline view from deal files at ~/Lethean/sales/deals/{id}.md
// — deals own their stage; this package is a read-time rollup.
//
// Wire shapes match the PipelineColumn + Deal interfaces consumed by the
// Angular pipeline surface in the Sales category.
//
// Usage example (Wails):
//
//	r := pipelineSvc.List(pipeline.ListInput{})
//	if r.OK { out := r.Value.(pipeline.ListOutput) }
package pipeline

// PipelineDeal is the JSON wire type for a deal card inside a column.
// Field names match the inline Deal interface in
// frontend-ng/src/app/desktop/surfaces/sales/pipeline.ts.
//
// Usage example:
//
//	d := pipeline.PipelineDeal{
//	    C: "Heritage Law LLP", V: "£24 K",
//	    T: "GDPR + privilege", ID: "202605-DEAL-001",
//	}
type PipelineDeal struct {
	// C is the customer / counterparty name.
	C string `json:"c"`
	// V is the headline value pre-formatted with currency symbol ("£24 K").
	V string `json:"v"`
	// T is the free-text qualifier: sector / stage detail / blocker.
	T string `json:"t"`
	// ID is the deal slug — used for MoveDeal targeting, not rendered.
	ID string `json:"id,omitempty"`
}

// PipelineColumn is the JSON wire type for one stage column.
// Field names match the PipelineColumn interface in
// frontend-ng/src/app/desktop/surfaces/sales/pipeline.ts exactly.
//
// Usage example:
//
//	col := pipeline.PipelineColumn{
//	    ID: "engage", Label: "Engaging", Value: "£128 K",
//	    Deals: []pipeline.PipelineDeal{},
//	}
type PipelineColumn struct {
	// ID is the stable stage identifier.
	ID string `json:"id"`
	// Label is the human label rendered in the column header.
	Label string `json:"label"`
	// Value is the pre-formatted aggregated value for the stage ("£64 K").
	Value string `json:"value"`
	// Deals is the ordered list of deal cards in this stage.
	Deals []PipelineDeal `json:"deals"`
}

// ListInput drives the List method.
//
// Usage example:
//
//	r := svc.List(pipeline.ListInput{Stage: "engage"})
type ListInput struct {
	// Stage filters to a single stage. Empty = all stages returned.
	Stage string `json:"stage,omitempty"`
}

// ListOutput is the List response envelope.
//
// Usage example:
//
//	out := r.Value.(pipeline.ListOutput)
//	for _, col := range out.Columns { _ = col.Label }
type ListOutput struct {
	// Columns is the ordered list of pipeline stages.
	Columns []PipelineColumn `json:"columns"`
	// TotalValue is the pre-formatted sum across all non-lost stages.
	TotalValue string `json:"totalValue"`
	// TotalDeals is the deal count across all non-lost stages.
	TotalDeals int `json:"totalDeals"`
}

// MoveInput drives the MoveDeal method.
//
// Usage example:
//
//	r := svc.MoveDeal(pipeline.MoveInput{DealID: "202605-DEAL-001", ToStage: "propose"})
//	rForce := svc.MoveDeal(pipeline.MoveInput{DealID: "202605-DEAL-001", ToStage: "qual", Force: true})
type MoveInput struct {
	// DealID is the deal slug to move.
	DealID string `json:"dealId"`
	// ToStage is the target stage identifier.
	ToStage string `json:"toStage"`
	// Force, when true, bypasses the IsLegalTransition gate so the
	// salesperson can perform an exceptional move (e.g. restore a
	// "lost" deal a customer revived, multi-step rewind after a data
	// migration). Forced moves leave a DISTINCT audit shape
	// (outcome="forced") so the Stage F log-tailer can categorise them
	// separately from normal-path moves and surface a pattern of forced
	// moves as a possible misuse signal per Mantis #1488. Frontend
	// MUST treat this flag as an explicit user gesture (confirm-dialog
	// + reason capture) — never auto-set, never default-true.
	Force bool `json:"force,omitempty"`
}

// LegalTransitions enumerates every from-stage → to-stage move
// MoveDeal will accept. Cerberus pass-9 HIGH (Mantis #1488) — a
// transition NOT in this set MUST be rejected with
// transition_illegal so a prompt-injection attack via Vi-driven
// signals (untrusted PR / email content surfaced by Vi) cannot
// silently flip a lead to won/lost, distorting forecast + firing
// downstream automation (Blesta off-boarding, commission triggers).
//
// Graph (mirrors the funnel-direction convention every Sales tool
// the team has used):
//
//	qual    → engage, lost
//	engage  → propose, qual, lost
//	propose → close, engage, lost
//	close   → won, propose, lost
//	won     → (terminal — no transitions out)
//	lost    → (terminal — no transitions out)
//
// Backward moves are permitted up to one stage (engage → qual,
// propose → engage, close → propose) so the salesperson can correct
// a mis-classification without admin intervention. Skip-levels
// (qual → propose, engage → close, etc.) are rejected — they signal
// either prompt injection or process-confusion both worth flagging.
//
// Self-transitions (qual → qual, etc.) are NOT in the map and
// reject as transition_illegal. The MoveDeal handler short-circuits
// these earlier with a "no-op move" path; the map is the back-stop.
//
// Adding an entry here is a security-policy decision — every new
// edge widens the prompt-injection blast radius. New entries
// require Cerberus DREAD review (Mantis ticket trail).
var LegalTransitions = map[string]map[string]bool{
	"qual": {
		"engage": true,
		"lost":   true,
	},
	"engage": {
		"propose": true,
		"qual":    true,
		"lost":    true,
	},
	"propose": {
		"close":  true,
		"engage": true,
		"lost":   true,
	},
	"close": {
		"won":     true,
		"propose": true,
		"lost":    true,
	},
	// won + lost are terminal — no outgoing edges. A "restore"
	// admin verb would land as a separate Mantis ticket + DREAD
	// review (not Stage 5 scope).
}

// IsLegalTransition reports whether a from-stage → to-stage move
// is permitted by LegalTransitions. Empty `from` is treated as
// "new deal" — only the initial stages (qual, engage) are legal
// landing points so deals can't be born at close/won/lost.
//
// Usage example:
//
//	if !pipeline.IsLegalTransition(fromStage, toStage) {
//	    return core.Fail(core.NewCode("transition_illegal", "..."))
//	}
func IsLegalTransition(from, to string) bool {
	if from == "" {
		// New deals — only the entry stages of the funnel.
		return to == "qual" || to == "engage"
	}
	allowed, ok := LegalTransitions[from]
	if !ok {
		// Source is unknown OR terminal (won/lost). Either case
		// MUST reject — no transitions out of terminal stages.
		return false
	}
	return allowed[to]
}
