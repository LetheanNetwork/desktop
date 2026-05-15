// SPDX-License-Identifier: EUPL-1.2

// Package tasks owns the local-first task system in core/ide. Backed by
// dappco.re/go/orm, schemas registered into the IDE's orm Service at
// boot. Storage flips between in-memory Memium (default) and DuckDB
// (persistent) via the existing /data backend picker.
//
// Why local-first: every agent that automates work on GitHub eventually
// trips anti-bot heuristics. The fix is not throttling — it is not
// depending on a venue with anti-bot heuristics for the primary state.
// Local-canonical here; optional gateway sync to lthn.sh for
// team enrichment arrives later.
//
// Connects-the-loop: each task created by Cladius becomes structured
// training signal for Lemma later (every task is a Cladius decision
// captured as data).
package tasks

import (
	"time"

	"dappco.re/go/orm"
)

// Issue is the primary task record.
//
// Usage example:
//
//	issue := tasks.Issue{Project: "ide", Summary: "wire tasks panel"}
type Issue struct {
	ID            string
	Project       string
	Summary       string
	Description   string
	State         string
	Severity      string
	Priority      string
	Assignee      string
	Reporter      string
	Version       string
	TargetVersion string
	FixedIn       string
	Resolution    string
	CreatedAt     time.Time
	UpdatedAt     time.Time
	ClosedAt      time.Time
}

// Schema declares the orm shape for Issue. Registered at IDE boot.
//
// Usage example:
//
//	orm.RegisterSchema(c, tasks.Issue{}.Schema())
func (Issue) Schema() orm.Schema {
	return orm.Define(func(b *orm.Builder) {
		b.Name("tasks_issues")
		b.PK("id")
		b.String("id").NotNull()
		b.String("project").NotNull()
		b.String("summary").NotNull()
		b.String("description")
		b.String("state").NotNull()
		b.String("severity")
		b.String("priority")
		b.String("assignee")
		b.String("reporter")
		b.String("version")
		b.String("target_version")
		b.String("fixed_in")
		b.String("resolution")
		b.Time("created_at").NotNull()
		b.Time("updated_at").NotNull()
		b.Time("closed_at")
		b.Index("project")
		b.Index("state")
		b.Index("assignee")
	})
}

// Note is a comment / activity log entry on an issue.
//
// Usage example:
//
//	note := tasks.Note{IssueID: "01ABC", Body: "Landed at SHA abc123", Author: "cladius"}
type Note struct {
	ID        string
	IssueID   string
	Body      string
	Author    string
	CreatedAt time.Time
}

// Schema declares the orm shape for Note.
//
// Usage example:
//
//	orm.RegisterSchema(c, tasks.Note{}.Schema())
func (Note) Schema() orm.Schema {
	return orm.Define(func(b *orm.Builder) {
		b.Name("tasks_notes")
		b.PK("id")
		b.String("id").NotNull()
		b.String("issue_id").NotNull()
		b.String("body").NotNull()
		b.String("author")
		b.Time("created_at").NotNull()
		b.Index("issue_id")
	})
}

// Canonical state values for Issue.State.
const (
	StateOpen       = "open"
	StateInProgress = "in_progress"
	StateDone       = "done"
	StateCancelled  = "cancelled"
)

// Canonical severity values (matches Mantis).
const (
	SeverityFeature = "feature"
	SeverityTrivial = "trivial"
	SeverityText    = "text"
	SeverityTweak   = "tweak"
	SeverityMinor   = "minor"
	SeverityMajor   = "major"
	SeverityCrash   = "crash"
	SeverityBlock   = "block"
)

// Canonical priority values (matches Mantis).
const (
	PriorityNone      = "none"
	PriorityLow       = "low"
	PriorityNormal    = "normal"
	PriorityHigh      = "high"
	PriorityUrgent    = "urgent"
	PriorityImmediate = "immediate"
)

// CreateInput captures the fields a caller must / may supply when
// creating an issue. Required: Project, Summary. Optional defaults
// applied by Create.
//
// Usage example:
//
//	in := tasks.CreateInput{Project: "ide", Summary: "ship tasks v0"}
type CreateInput struct {
	Project       string
	Summary       string
	Description   string
	Severity      string
	Priority      string
	Assignee      string
	Reporter      string
	Version       string
	TargetVersion string
}

// UpdateInput captures partial-field updates for an existing issue.
// Empty strings mean "no change"; Description uses a pointer to
// distinguish "clear to empty" from "no change".
//
// Usage example:
//
//	in := tasks.UpdateInput{State: tasks.StateInProgress}
type UpdateInput struct {
	Summary       string
	Description   *string
	State         string
	Severity      string
	Priority      string
	Assignee      string
	Version       string
	TargetVersion string
	FixedIn       string
	Resolution    string
}

// ListFilter narrows a List query. Empty fields = no constraint.
//
// Usage example:
//
//	f := tasks.ListFilter{Project: "ide", State: tasks.StateOpen}
type ListFilter struct {
	Project  string
	State    string
	Assignee string
	Reporter string
	Limit    int
	Offset   int
}
