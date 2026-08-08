// SPDX-License-Identifier: EUPL-1.2

package tasks

import (
	core "dappco.re/go"
	"dappco.re/go/orm"
)

// idLength is the random-string length of generated IDs. 12 chars from
// core.RandomString gives 62^12 ≈ 3.2e21 keyspace — collisions are
// statistically negligible for a single-machine task store.
const idLength = 12

// newID generates a fresh task / note identifier.
//
// Usage example:
//
//	id := newID()
func newID() string {
	r := core.RandomString(idLength)
	if !r.OK {
		// Fallback: nanosecond timestamp. Sortable, unique enough for
		// rare crypto/rand failure paths.
		return core.Sprintf("T%d", core.Now().UnixNano())
	}
	return r.Value.(string)
}

// Create inserts a new issue and returns the populated record. Required:
// Project + Summary. Defaults applied for State / Severity / Priority.
//
// Usage example:
//
//	r := tasks.Create(c, tasks.CreateInput{Project: "ide", Summary: "wire panel"})
//	if !r.OK { return r }
//	issue, _, _ := orm.Detail[tasks.Issue](r)
func Create(c *core.Core, input CreateInput) core.Result {
	if input.Project == "" {
		return core.Fail(core.E("tasks.Create", "project is required", nil))
	}
	if input.Summary == "" {
		return core.Fail(core.E("tasks.Create", "summary is required", nil))
	}
	now := core.Now().UTC()
	issue := Issue{
		ID:            newID(),
		Project:       input.Project,
		Summary:       input.Summary,
		Description:   SealField(c, input.Description),
		State:         StateOpen,
		Severity:      defaultIfEmpty(input.Severity, SeverityMinor),
		Priority:      defaultIfEmpty(input.Priority, PriorityNormal),
		Assignee:      input.Assignee,
		Reporter:      input.Reporter,
		Version:       input.Version,
		TargetVersion: input.TargetVersion,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if r := orm.Insert(c, &issue); !r.OK {
		return r
	}
	c.ACTION(IssueChanged{
		Kind:  KindCreated,
		Issue: issue,
		At:    now,
	})
	return core.Ok(issue)
}

// Get returns the issue identified by id. Sealed Description fields
// (Mantis #1727) are decrypted via UnsealField when the unlocked
// account state can satisfy the decrypt; legacy plaintext values
// round-trip unchanged.
//
// Usage example:
//
//	r := tasks.Get(c, "01AB...")
//	if !r.OK { return r }
//	issue, _, _ := orm.Detail[tasks.Issue](r)
func Get(c *core.Core, id string) core.Result {
	r := orm.Of[Issue](c).Find(id)
	if !r.OK {
		return r
	}
	issue, _, ok := orm.Detail[Issue](r)
	if !ok {
		return r
	}
	issue.Description = UnsealField(c, issue.Description)
	return core.Ok(issue)
}

// List returns issues matching the filter, newest updated_at first.
//
// Usage example:
//
//	r := tasks.List(c, tasks.ListFilter{Project: "ide", State: tasks.StateOpen})
//	if !r.OK { return r }
//	issues, _ := orm.Cast[[]Issue](r)
func List(c *core.Core, filter ListFilter) core.Result {
	bridge := orm.Of[Issue](c)
	if filter.Project != "" {
		bridge = bridge.Where("project", "=", filter.Project)
	}
	if filter.State != "" {
		bridge = bridge.Where("state", "=", filter.State)
	}
	if filter.Assignee != "" {
		bridge = bridge.Where("assignee", "=", filter.Assignee)
	}
	if filter.Reporter != "" {
		bridge = bridge.Where("reporter", "=", filter.Reporter)
	}
	bridge = bridge.Order("updated_at", "desc")
	if filter.Limit > 0 {
		bridge = bridge.Limit(filter.Limit)
	}
	if filter.Offset > 0 {
		bridge = bridge.Offset(filter.Offset)
	}
	r := bridge.Get()
	if !r.OK {
		return r
	}
	// Sealed Description fields (Mantis #1727) decrypt at the
	// boundary. Orchestration columns (project/state/severity/etc.)
	// stay plain so this Where()-filtered query still hits the right
	// rows; only the user-prompt body needs unsealing.
	issues, ok := r.Value.([]Issue)
	if !ok {
		return r
	}
	for i := range issues {
		issues[i].Description = UnsealField(c, issues[i].Description)
	}
	return core.Ok(issues)
}

// Update applies partial-field changes to the named issue. Returns the
// updated record on success. State transitions to Done/Cancelled also
// set ClosedAt.
//
// Usage example:
//
//	r := tasks.Update(c, "01AB...", tasks.UpdateInput{State: tasks.StateInProgress})
//	if !r.OK { return r }
//	updated, _, _ := orm.Detail[tasks.Issue](r)
func Update(c *core.Core, id string, input UpdateInput) core.Result {
	// Enum gates (Mantis #1503) — caller-supplied State/Severity/Priority
	// must match the canonical closed sets in types.go. Empty strings
	// mean "no change" so they short-circuit. Validate BEFORE the Get
	// round-trip so a bogus enum fails fast and does not consume orm work.
	if input.State != "" && !IsValidState(input.State) {
		return core.Fail(core.E("tasks.Update", "tasks.update.invalid_state: "+input.State, nil))
	}
	if input.Severity != "" && !IsValidSeverity(input.Severity) {
		return core.Fail(core.E("tasks.Update", "tasks.update.invalid_severity: "+input.Severity, nil))
	}
	if input.Priority != "" && !IsValidPriority(input.Priority) {
		return core.Fail(core.E("tasks.Update", "tasks.update.invalid_priority: "+input.Priority, nil))
	}
	r := Get(c, id)
	if !r.OK {
		return r
	}
	issue, _, ok := orm.Detail[Issue](r)
	if !ok {
		return core.Fail(core.E("tasks.Update", "cast issue", nil))
	}
	// Snapshot for the IssueChange.Before payload — listeners may
	// want to compute their own diffs without re-querying orm.
	before := issue
	mutated := false
	if input.Summary != "" {
		issue.Summary = input.Summary
		mutated = true
	}
	if input.Description != nil {
		// Plaintext until the pre-save reseal below. Keeping the
		// in-memory shape unsealed lets the IssueChange.Before /
		// .Issue payload listeners see the human-readable string.
		issue.Description = *input.Description
		mutated = true
	}
	if input.State != "" {
		// Lifecycle gate (Mantis #1726 / Cerberus #64 F-6) — only
		// transitions listed in allowedStateTransitions are honoured;
		// arbitrary State hops were a data-tampering surface
		// (STRIDE-T). Same-state moves are legal (idempotent update).
		if !IsAllowedStateTransition(issue.State, input.State) {
			return core.Fail(core.E(
				"tasks.Update",
				"tasks.state_invalid: "+issue.State+" -> "+input.State,
				nil,
			))
		}
		issue.State = input.State
		mutated = true
		if IsTerminalState(input.State) {
			issue.ClosedAt = core.Now().UTC()
		} else if IsTerminalState(before.State) {
			// Reopen from a terminal state — clear ClosedAt so the
			// forensic timestamp does not survive across the
			// lifecycle break.
			issue.ClosedAt = core.Time{}
		}
	}
	if input.Severity != "" {
		issue.Severity = input.Severity
		mutated = true
	}
	if input.Priority != "" {
		issue.Priority = input.Priority
		mutated = true
	}
	if input.Assignee != "" {
		issue.Assignee = input.Assignee
		mutated = true
	}
	if input.Version != "" {
		issue.Version = input.Version
		mutated = true
	}
	if input.TargetVersion != "" {
		issue.TargetVersion = input.TargetVersion
		mutated = true
	}
	if input.FixedIn != "" {
		issue.FixedIn = input.FixedIn
		mutated = true
	}
	if input.Resolution != "" {
		issue.Resolution = input.Resolution
		mutated = true
	}
	if !mutated {
		return core.Ok(issue)
	}
	now := core.Now().UTC()
	issue.UpdatedAt = now
	// Snapshot the plaintext Description so the Ok value + the
	// IssueChange.Issue payload surface the human-readable string to
	// listeners. SealField the on-disk value just for the orm.Save
	// round-trip; SealField is a no-op when the input is empty or
	// when no account is unlocked, so the legacy plaintext path is
	// preserved on the locked-window first-launch flow.
	plaintextDescription := issue.Description
	issue.Description = SealField(c, plaintextDescription)
	if saveResult := orm.Save(c, &issue); !saveResult.OK {
		return saveResult
	}
	issue.Description = plaintextDescription
	// Closed transitions get the dedicated KindClosed event so
	// listeners (PR-watcher, audit, agent-prep) can subscribe to
	// "issue closed" without diffing every state field. Other
	// updates fire KindUpdated; listeners diff Before vs Issue
	// for whichever fields they care about.
	kind := KindUpdated
	if issue.State != before.State &&
		(issue.State == StateDone || issue.State == StateCancelled) {
		kind = KindClosed
	}
	c.ACTION(IssueChanged{
		Kind:   kind,
		Issue:  issue,
		Before: before,
		At:     now,
	})
	return core.Ok(issue)
}

// Close transitions an issue to Done with the given resolution string.
//
// Usage example:
//
//	r := tasks.Close(c, "01AB...", "fixed")
//	if !r.OK { return r }
func Close(c *core.Core, id, resolution string) core.Result {
	return Update(c, id, UpdateInput{
		State:      StateDone,
		Resolution: resolution,
	})
}

// AddNote appends a comment / activity log entry to an issue.
//
// Usage example:
//
//	r := tasks.AddNote(c, "01AB...", "Landed at SHA abc123", "cladius")
//	if !r.OK { return r }
//	note, _, _ := orm.Detail[tasks.Note](r)
func AddNote(c *core.Core, issueID, body, author string) core.Result {
	if issueID == "" {
		return core.Fail(core.E("tasks.AddNote", "issue_id is required", nil))
	}
	if body == "" {
		return core.Fail(core.E("tasks.AddNote", "body is required", nil))
	}
	now := core.Now().UTC()
	plaintextBody := body
	note := Note{
		ID:        newID(),
		IssueID:   issueID,
		Body:      SealField(c, body),
		Author:    author,
		CreatedAt: now,
	}
	if r := orm.Insert(c, &note); !r.OK {
		return r
	}
	// Restore plaintext Body for the IssueChange payload + the
	// Ok value so listeners + callers see the human-readable
	// string; the at-rest envelope only lives on disk.
	note.Body = plaintextBody
	// Broadcast KindNoted so listeners that aggregate activity
	// (Vi.Activity feed, audit log, PR-watcher commenting) can
	// react. Best-effort fetch of the parent Issue snapshot —
	// failures don't block the note insert.
	parent := Issue{}
	if r := Get(c, issueID); r.OK {
		if i, _, ok := orm.Detail[Issue](r); ok {
			parent = i
		}
	}
	c.ACTION(IssueChanged{
		Kind:  KindNoted,
		Issue: parent,
		Note:  note,
		At:    now,
	})
	return core.Ok(note)
}

// ListNotes returns all notes on an issue, oldest first. Sealed
// Body fields (Mantis #1727) are decrypted via UnsealField at the
// boundary; legacy plaintext values round-trip unchanged.
//
// Usage example:
//
//	r := tasks.ListNotes(c, "01AB...")
//	if !r.OK { return r }
//	notes, _ := orm.Cast[[]Note](r)
func ListNotes(c *core.Core, issueID string) core.Result {
	r := orm.Of[Note](c).Where("issue_id", "=", issueID).Order("created_at", "asc").Get()
	if !r.OK {
		return r
	}
	notes, ok := r.Value.([]Note)
	if !ok {
		return r
	}
	for i := range notes {
		notes[i].Body = UnsealField(c, notes[i].Body)
	}
	return core.Ok(notes)
}

func defaultIfEmpty(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
