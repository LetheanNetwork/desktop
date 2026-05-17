// SPDX-Licence-Identifier: EUPL-1.2

// Wails surface for the tasks subsystem. Methods are bound on the
// *Service receiver and exposed to the frontend via wails3 generate
// bindings — the TS shape lands at
// frontend/bindings/dappco.re/lthn/desktop/pkg/tasks/service.
//
// The package-level API (Create/Get/List/Update/AddNote in api.go) is
// the canonical surface; the Service is a thin wrapper that holds the
// *core.Core so each Wails method can dispatch without the frontend
// having to know about Core wiring. Inputs/outputs mirror the
// repos.Service shape — one Input struct per method, returning a
// core.Result whose Value carries the typed response.
//
// Deferred (tracked separately):
//   - Search(query) substring lookup — needs index design.
//   - CalendarBlock primitive — out of scope for this pass.
//   - Extended Issue fields (Estimate, Theme, RICE inputs) — schema
//     migration needed; current pass uses the existing Issue shape.

package tasks

import (

	core "dappco.re/go"
	"dappco.re/go/orm"

	"dappco.re/lthn/desktop/pkg/auth"
)

// Service owns the tasks Wails surface. Wraps *core.Core so the
// bound methods can delegate to the package-level api.go functions
// without leaking Core into the frontend binding.
//
// Wired via application.NewService(tasks.NewService(c)) in
// pkg/desktop/desktop.go (alongside repos/vi/lint/etc.).
type Service struct {
	core *core.Core
}

// NewService constructs the tasks Wails surface against a Core
// container.
//
// Usage example:
//
//	svc := tasks.NewService(c)
func NewService(c *core.Core) *Service {
	return &Service{core: c}
}

// Register constructs the tasks service for Core registration. Mirrors
// the shape used by pkg/repos so the desktop bootstrap can wire tasks
// via core.WithName("tasks", tasks.Register) when/if the service-bus
// path is preferred over direct construction.
//
// Usage example:
//
//	core.New(core.WithName("tasks", tasks.Register))
func Register(c *core.Core) core.Result {
	return core.Ok(NewService(c))
}

// ListInput narrows a List query. Mirrors ListFilter from types.go
// (json tags added for the wails3 TS binding generator). All fields
// optional — empty = no constraint.
type ListInput struct {
	Project  string `json:"project,omitempty"`
	State    string `json:"state,omitempty"`
	Assignee string `json:"assignee,omitempty"`
	Reporter string `json:"reporter,omitempty"`
	Limit    int    `json:"limit,omitempty"`
	Offset   int    `json:"offset,omitempty"`
}

// ListOutput wraps the issues slice plus a count for the UI's
// "X items" header. Keeping the count separate from len() lets the
// shape grow (pagination cursor, etc.) without breaking consumers.
type ListOutput struct {
	Issues []Issue `json:"issues"`
	Count  int     `json:"count"`
}

// List returns issues matching the filter, newest updated_at first.
// Empty input returns every open issue across every project (the
// natural shape for a global backlog view).
//
// Tier gate (RFC §5.1 / table row pkg/tasks read): Require permits
// TierOperator + TierRenderer + TierCascade. Per-account filtering
// remains the per-row concern (separate RFC).
//
// Usage example (TS binding):
//
//	const r = await List({ state: "open" });
//	const out = r.Value as { issues: Issue[]; count: number };
func (s *Service) List(input ListInput) core.Result {
	id, ok := auth.Require(s.core, "tasks.Service.List",
		auth.TierOperator, auth.TierRenderer, auth.TierCascade)
	if !ok {
		return core.Fail(core.E("tasks.Service.List",
			"tasks.tier_not_permitted: "+id.Tier.String(), nil))
	}
	filter := ListFilter{
		Project:  input.Project,
		State:    input.State,
		Assignee: input.Assignee,
		Reporter: input.Reporter,
		Limit:    input.Limit,
		Offset:   input.Offset,
	}
	r := List(s.core, filter)
	if !r.OK {
		return r
	}
	issues, ok := orm.Cast[[]Issue](r)
	if !ok {
		return core.Fail(core.E("tasks.Service.List", "cast issues", nil))
	}
	return core.Ok(ListOutput{Issues: issues, Count: len(issues)})
}

// GetInput identifies a single issue by ID.
type GetInput struct {
	ID string `json:"id"`
}

// Get returns the issue identified by input.ID, or a fail Result if
// the ID is empty / not found.
//
// Tier gate (RFC §5.1): Require permits TierOperator + TierRenderer
// + TierCascade. Per-row ACL deferred to Phase 2.
//
// Usage example (TS binding):
//
//	const r = await Get({ id: "abc123" });
//	const issue = r.Value as Issue;
func (s *Service) Get(input GetInput) core.Result {
	id, ok := auth.Require(s.core, "tasks.Service.Get",
		auth.TierOperator, auth.TierRenderer, auth.TierCascade)
	if !ok {
		return core.Fail(core.E("tasks.Service.Get",
			"tasks.tier_not_permitted: "+id.Tier.String(), nil))
	}
	if input.ID == "" {
		return core.Fail(core.E("tasks.Service.Get", "id is required", nil))
	}
	return Get(s.core, input.ID)
}

// CreateIssueInput captures the fields a caller must / may supply
// when creating an issue. Mirrors CreateInput from types.go with
// json tags for the wails3 TS binding generator.
type CreateIssueInput struct {
	Project       string `json:"project"`
	Summary       string `json:"summary"`
	Description   string `json:"description,omitempty"`
	Severity      string `json:"severity,omitempty"`
	Priority      string `json:"priority,omitempty"`
	Assignee      string `json:"assignee,omitempty"`
	Reporter      string `json:"reporter,omitempty"`
	Version       string `json:"version,omitempty"`
	TargetVersion string `json:"target_version,omitempty"`
}

// Create inserts a new issue and returns the populated record.
// Required: Project + Summary. Defaults applied for State / Severity
// / Priority by the underlying api.go Create.
//
// ENFORCE-not-claim (RFC.tier-auth-substrate.md §5.3 / Mantis #1722):
// Reporter is OVERWRITTEN from auth.Caller(s.core).Subject — the
// input.Reporter field is SILENTLY IGNORED. A hostile renderer can
// supply any string here; the service refuses to attribute records
// based on caller assertion. Assignee remains input-driven (Operator
// legitimately assigns work to others — see §5.3 row).
//
// Tier gate (RFC §5.1 / §10 Q-1 Phase 1 ALLOW): Require permits
// TierOperator + TierRenderer. Other tiers (TierCascade, TierCron,
// TierInternal) reach this surface only through misconfiguration; the
// gate returns a visible Fail rather than silently dropping the
// attempt so the audit trail captures the deny.
//
// Usage example (TS binding):
//
//	const r = await Create({ project: "lthn", summary: "wire backlog" });
//	const issue = r.Value as Issue;
func (s *Service) Create(input CreateIssueInput) core.Result {
	id, ok := auth.Require(s.core, "tasks.Service.Create", auth.TierOperator, auth.TierRenderer)
	if !ok {
		return core.Fail(core.E("tasks.Service.Create",
			"tasks.tier_not_permitted: "+id.Tier.String(), nil))
	}
	return Create(s.core, CreateInput{
		Project:       input.Project,
		Summary:       input.Summary,
		Description:   input.Description,
		Severity:      input.Severity,
		Priority:      input.Priority,
		Assignee:      input.Assignee,
		Reporter:      id.Subject, // ENFORCE-not-claim — input.Reporter ignored
		Version:       input.Version,
		TargetVersion: input.TargetVersion,
	})
}

// UpdateIssueInput captures partial-field updates for an existing
// issue. The frontend supplies the ID alongside the fields; this is
// the Wails-friendly merge of api.go's separate (id, UpdateInput)
// signature. Description uses a pointer to distinguish "clear to
// empty" from "no change" — TS callers pass null vs undefined.
type UpdateIssueInput struct {
	ID            string  `json:"id"`
	Summary       string  `json:"summary,omitempty"`
	Description   *string `json:"description,omitempty"`
	State         string  `json:"state,omitempty"`
	Severity      string  `json:"severity,omitempty"`
	Priority      string  `json:"priority,omitempty"`
	Assignee      string  `json:"assignee,omitempty"`
	Version       string  `json:"version,omitempty"`
	TargetVersion string  `json:"target_version,omitempty"`
	FixedIn       string  `json:"fixed_in,omitempty"`
	Resolution    string  `json:"resolution,omitempty"`
}

// Update applies partial-field changes to the named issue and
// returns the updated record on success. State transitions to
// Done/Cancelled also set ClosedAt (handled by api.go Update).
//
// ENFORCE-not-claim (RFC §5.3 row 321): UpdateIssueInput today carries
// no Editor / Reporter attribution field, so no caller-derived
// overwrite is needed. If a future field lands (e.g. last_edited_by),
// it MUST be sourced from auth.Caller(s.core).Subject following the
// Create / AddNote pattern in this file.
//
// Tier gate (RFC §5.1): Require permits TierOperator + TierRenderer.
// Per-row ACL (renderer touching another account's issue) is tracked
// as a Phase 2 follow-on per RFC §10 B.2 done-criteria.
//
// Usage example (TS binding):
//
//	const r = await Update({ id: "abc123", state: "in_progress" });
//	const updated = r.Value as Issue;
func (s *Service) Update(input UpdateIssueInput) core.Result {
	id, ok := auth.Require(s.core, "tasks.Service.Update", auth.TierOperator, auth.TierRenderer)
	if !ok {
		return core.Fail(core.E("tasks.Service.Update",
			"tasks.tier_not_permitted: "+id.Tier.String(), nil))
	}
	if input.ID == "" {
		return core.Fail(core.E("tasks.Service.Update", "id is required", nil))
	}
	return Update(s.core, input.ID, UpdateInput{
		Summary:       input.Summary,
		Description:   input.Description,
		State:         input.State,
		Severity:      input.Severity,
		Priority:      input.Priority,
		Assignee:      input.Assignee,
		Version:       input.Version,
		TargetVersion: input.TargetVersion,
		FixedIn:       input.FixedIn,
		Resolution:    input.Resolution,
	})
}

// AddNoteInput identifies the parent issue plus the note body +
// author. All three fields required; api.go AddNote fails loudly on
// blanks.
type AddNoteInput struct {
	IssueID string `json:"issue_id"`
	Body    string `json:"body"`
	Author  string `json:"author,omitempty"`
}

// AddNote appends a comment / activity log entry to an issue and
// returns the persisted Note. Broadcasts a KindNoted event so the
// activity feed / audit log / PR-watcher can react.
//
// ENFORCE-not-claim (RFC §5.3 / Mantis #1722): Author is OVERWRITTEN
// from auth.Caller(s.core).Subject — the input.Author field is
// SILENTLY IGNORED. A hostile renderer cannot plant a false audit
// trail by claiming to be another account.
//
// Tier gate (RFC §5.1): Require permits TierOperator + TierRenderer.
//
// Usage example (TS binding):
//
//	const r = await AddNote({ issue_id: "abc123", body: "shipped" });
//	const note = r.Value as Note;
func (s *Service) AddNote(input AddNoteInput) core.Result {
	id, ok := auth.Require(s.core, "tasks.Service.AddNote", auth.TierOperator, auth.TierRenderer)
	if !ok {
		return core.Fail(core.E("tasks.Service.AddNote",
			"tasks.tier_not_permitted: "+id.Tier.String(), nil))
	}
	return AddNote(s.core, input.IssueID, input.Body, id.Subject) // ENFORCE-not-claim — input.Author ignored
}

