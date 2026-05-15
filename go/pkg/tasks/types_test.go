// SPDX-License-Identifier: EUPL-1.2

package tasks

import (
	core "dappco.re/go"
)

func TestTypes_IssueSchema_Good(t *core.T) {
	schema := Issue{}.Schema()
	core.AssertEqual(t, "tasks_issues", schema.Name)
	core.AssertNotEmpty(t, schema.Fields)
	core.AssertLen(t, schema.PK, 1)
	core.AssertEqual(t, "id", schema.PK[0])
}

func TestTypes_IssueSchema_Bad(t *core.T) {
	schema := Issue{}.Schema()
	core.AssertNotEqual(t, "tasks_notes", schema.Name)
	core.AssertNotEqual(t, "", schema.Name)
}

func TestTypes_IssueSchema_Ugly(t *core.T) {
	schema := Issue{}.Schema()
	seen := map[string]bool{}
	for _, f := range schema.Fields {
		core.AssertFalse(t, seen[f.Name])
		seen[f.Name] = true
	}
}

func TestTypes_NoteSchema_Good(t *core.T) {
	schema := Note{}.Schema()
	core.AssertEqual(t, "tasks_notes", schema.Name)
	core.AssertLen(t, schema.PK, 1)
	core.AssertEqual(t, "id", schema.PK[0])
}

func TestTypes_NoteSchema_Bad(t *core.T) {
	schema := Note{}.Schema()
	core.AssertNotEqual(t, "tasks_issues", schema.Name)
}

func TestTypes_NoteSchema_Ugly(t *core.T) {
	schema := Note{}.Schema()
	seen := map[string]bool{}
	for _, f := range schema.Fields {
		core.AssertFalse(t, seen[f.Name])
		seen[f.Name] = true
	}
}

func TestTypes_StateConstants_Good(t *core.T) {
	issue := Issue{State: StateOpen, CreatedAt: core.Now(), UpdatedAt: core.Now()}
	core.AssertEqual(t, "open", issue.State)
	core.AssertFalse(t, issue.CreatedAt.IsZero())
	core.AssertFalse(t, issue.UpdatedAt.IsZero())
}

func TestTypes_StateConstants_Bad(t *core.T) {
	core.AssertNotEqual(t, StateOpen, StateDone)
	core.AssertNotEqual(t, StateInProgress, StateCancelled)
}

func TestTypes_StateConstants_Ugly(t *core.T) {
	all := []string{StateOpen, StateInProgress, StateDone, StateCancelled}
	seen := map[string]bool{}
	for _, s := range all {
		core.AssertNotEmpty(t, s)
		core.AssertFalse(t, seen[s])
		seen[s] = true
	}
}
