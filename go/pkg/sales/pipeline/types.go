// SPDX-Licence-Identifier: EUPL-1.2

// Package pipeline is the lthn-side sales pipeline service. Derives the
// Kanban pipeline view from deal files at ~/Lethean/sales/deals/{id}.md
// — deals own their stage; this package is a read-time rollup.
//
// Wire shapes match the PipelineColumn + Deal interfaces consumed by the
// <lthn-view-pipeline> Lit element in the Sales role view.
//
// Usage example (Wails):
//
//	r := pipelineSvc.List(pipeline.ListInput{})
//	if r.OK { out := r.Value.(pipeline.ListOutput) }
package pipeline

// PipelineDeal is the JSON wire type for a deal card inside a column.
// Field names match the inline Deal interface in
// frontend/src/lit/views/sales/pipeline.ts.
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
// frontend/src/lit/views/sales/pipeline.ts exactly.
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
type MoveInput struct {
	// DealID is the deal slug to move.
	DealID string `json:"dealId"`
	// ToStage is the target stage identifier.
	ToStage string `json:"toStage"`
}
