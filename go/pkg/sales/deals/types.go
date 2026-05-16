// SPDX-Licence-Identifier: EUPL-1.2

// Package deals is the lthn-side deal record service. Reads, writes, and
// queries Trix-style markdown files from
// ~/Lethean/sales/deals/{deal-id}.md — YAML frontmatter carries the
// searchable header (customer, stage, amount, stakeholders, activity log,
// docs); the body below is free-form notes.
//
// Wire shape matches the Deal interface consumed by the
// <lthn-view-deals> Lit element in the Sales role view.
//
// Deals are also the source-of-truth for pipeline stage — pkg/sales/pipeline
// derives its Kanban view by scanning this directory.
//
// Usage example (Wails):
//
//	r := dealsSvc.List(deals.ListInput{Stage: "engage"})
//	if r.OK { out := r.Value.(deals.ListOutput) }
package deals

import core "dappco.re/go"

// Activity is the JSON wire type for one activity-log entry.
// Field names match the Activity interface in
// frontend/src/lit/views/sales/deals.ts exactly.
//
// Usage example:
//
//	a := deals.Activity{
//	    TS: "today · 14:02", K: "call", Who: "you",
//	    T: "30-min call with Ada Penley",
//	}
type Activity struct {
	// TS is the pre-formatted timestamp ("today · 14:02", "3 d ago").
	TS string `json:"ts"`
	// K is the activity kind: "call" | "email" | "meet".
	K string `json:"k"`
	// Who is the initiator label ("you", "Ada P.").
	Who string `json:"who"`
	// T is the activity body text.
	T string `json:"t"`
}

// DocLink is the JSON wire type for one linked document.
// Field names match DocLink in frontend/src/lit/views/sales/deals.ts.
//
// Usage example:
//
//	d := deals.DocLink{T: "SOW v2 · draft", S: "draft"}
type DocLink struct {
	// T is the document display title ("SOW v2 · draft").
	T string `json:"t"`
	// S is the workflow state: "draft" | "signed" | "shared".
	S string `json:"s"`
}

// Deal is the JSON wire type returned to the Lit frontend.
// Field names match the Deal interface in
// frontend/src/lit/views/sales/deals.ts exactly.
//
// Usage example:
//
//	d := deals.Deal{
//	    Customer: "Heritage Law LLP", Headline: "£24 K · 12-month hosted",
//	    Stage: "Engaging", Probability: "60%", CloseTarget: "14 Jun",
//	}
type Deal struct {
	// Customer is the counterparty name.
	Customer string `json:"customer"`
	// Headline is the pre-formatted deal summary ("£24 K · 12-month hosted").
	Headline string `json:"headline"`
	// Stage is the human-readable current stage label ("Engaging").
	Stage string `json:"stage"`
	// Probability is the pre-formatted probability string ("60%").
	Probability string `json:"probability"`
	// CloseTarget is the human close date ("14 Jun").
	CloseTarget string `json:"closeTarget"`
	// Log is the activity timeline, newest first.
	Log []Activity `json:"log"`
	// Stakeholders is the ordered stakeholder list.
	Stakeholders []string `json:"stakeholders"`
	// Docs is the linked document list.
	Docs []DocLink `json:"docs"`
	// ID is the deal slug — routing only, not rendered.
	ID string `json:"id,omitempty"`
}

// ActivityRecord is the persisted form of an activity log entry.
//
// Usage example:
//
//	a := deals.ActivityRecord{At: core.Now(), K: "call", Who: "you", T: "..."}
type ActivityRecord struct {
	// At is the activity timestamp.
	At core.Time `yaml:"at"`
	// K is the activity kind: call | email | meet.
	K string `yaml:"k"`
	// Who is the initiator label.
	Who string `yaml:"who"`
	// T is the activity body.
	T string `yaml:"t"`
}

// DocRecord is the persisted form of a linked document.
//
// Usage example:
//
//	d := deals.DocRecord{Title: "SOW v2 · draft", State: "draft"}
type DocRecord struct {
	// Title is the document display title.
	Title string `yaml:"title"`
	// State is the workflow state: draft | signed | shared.
	State string `yaml:"state"`
}

// DealRecord is the internal persistence type — the full set of
// fields stored in the YAML frontmatter of each Trix file.
//
// Usage example:
//
//	rec := deals.DealRecord{
//	    ID: "202605-DEAL-001", Customer: "Heritage Law LLP",
//	    AmountPence: 24000, Stage: "engage",
//	    ProbabilityPct: 60, Owner: "Snider",
//	}
type DealRecord struct {
	// ID is the canonical deal identifier (matches filename slug).
	ID string `yaml:"id"`
	// Customer is the counterparty display name.
	Customer string `yaml:"customer"`
	// AmountPence is the deal value in pence (integer, for forecast arithmetic).
	AmountPence int `yaml:"amount_pence"`
	// Stage is the current stage id: qual | engage | propose | close | won | lost.
	Stage string `yaml:"stage"`
	// ProbabilityPct is the close probability 0-100. 0 = use stage default.
	ProbabilityPct int `yaml:"probability_pct"`
	// CloseTarget is the human-readable close date ("14 Jun").
	CloseTarget string `yaml:"close_target"`
	// Owner is the deal owner's name.
	Owner string `yaml:"owner"`
	// Stakeholders is the ordered list of stakeholder display strings.
	Stakeholders []string `yaml:"stakeholders,omitempty"`
	// CreatedAt is the deal creation timestamp.
	CreatedAt core.Time `yaml:"created_at"`
	// UpdatedAt is the last modification timestamp.
	UpdatedAt core.Time `yaml:"updated_at"`
	// Log is the activity timeline (newest first).
	Log []ActivityRecord `yaml:"log,omitempty"`
	// Docs is the linked document list.
	Docs []DocRecord `yaml:"docs,omitempty"`
	// Notes is the markdown body. Not persisted in YAML (yaml:"-").
	Notes string `yaml:"-"`
}

// ListInput drives the List method.
//
// Usage example:
//
//	r := svc.List(deals.ListInput{Stage: "engage", Limit: 20})
type ListInput struct {
	// Stage filters to a specific stage. Empty string = all.
	Stage string `json:"stage,omitempty"`
	// Owner filters to a specific owner. Empty string = all.
	Owner string `json:"owner,omitempty"`
	// Limit caps the result count. Zero defaults to 50.
	Limit int `json:"limit,omitempty"`
}

// ListOutput is the List response envelope.
//
// Usage example:
//
//	out := r.Value.(deals.ListOutput)
//	for _, d := range out.Deals { _ = d.Customer }
type ListOutput struct {
	// Deals is the filtered, sorted list of deals.
	Deals []Deal `json:"deals"`
	// Total is the unfiltered count.
	Total int `json:"total"`
}

// GetInput drives the Get method.
//
// Usage example:
//
//	r := svc.Get(deals.GetInput{ID: "202605-DEAL-001"})
type GetInput struct {
	// ID is the deal slug to retrieve.
	ID string `json:"id"`
}

// CreateInput drives the Create method.
//
// Usage example:
//
//	r := svc.Create(deals.CreateInput{
//	    Customer: "Heritage Law LLP", AmountPence: 24000,
//	    Stage: "engage", Owner: "Snider",
//	})
type CreateInput struct {
	// Customer is the counterparty display name.
	Customer string `json:"customer"`
	// AmountPence is the deal value in pence.
	AmountPence int `json:"amountPence"`
	// Stage is the initial stage id.
	Stage string `json:"stage"`
	// ProbabilityPct is the initial probability 0-100.
	ProbabilityPct int `json:"probabilityPct,omitempty"`
	// CloseTarget is the human-readable close date.
	CloseTarget string `json:"closeTarget,omitempty"`
	// Owner is the deal owner's name.
	Owner string `json:"owner"`
	// Stakeholders is the initial stakeholder list.
	Stakeholders []string `json:"stakeholders,omitempty"`
}

// UpdateStageInput drives the UpdateStage method.
//
// Usage example:
//
//	r := svc.UpdateStage(deals.UpdateStageInput{
//	    ID: "202605-DEAL-001", Stage: "propose",
//	})
type UpdateStageInput struct {
	// ID is the deal slug to transition.
	ID string `json:"id"`
	// Stage is the target stage id.
	Stage string `json:"stage"`
}

// AddActivityInput drives the AddActivity method.
//
// Usage example:
//
//	r := svc.AddActivity(deals.AddActivityInput{
//	    DealID: "202605-DEAL-001", K: "call", Who: "you",
//	    T: "30-min call · privacy posture, no blockers.",
//	})
type AddActivityInput struct {
	// DealID is the deal slug to log against.
	DealID string `json:"dealId"`
	// K is the activity kind: "call" | "email" | "meet".
	K string `json:"k"`
	// Who is the initiator label ("you" or contact name).
	Who string `json:"who"`
	// T is the activity body text.
	T string `json:"t"`
}
