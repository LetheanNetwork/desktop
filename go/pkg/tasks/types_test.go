// SPDX-License-Identifier: EUPL-1.2

package tasks

import (
	core "dappco.re/go"
)

func TestTypesIssueSchema(t *core.T) {
	schema := Issue{}.Schema()
	core.AssertEqual(t, "tasks_issues", schema.Name)
	core.AssertNotEmpty(t, schema.Fields)
}

func TestTypesNoteSchema(t *core.T) {
	schema := Note{}.Schema()
	core.AssertEqual(t, "tasks_notes", schema.Name)
	core.AssertLen(t, schema.PK, 1)
	core.AssertEqual(t, "id", schema.PK[0])
}

func TestTypesIssueStateConstants(t *core.T) {
	issue := Issue{State: StateOpen, CreatedAt: core.Now(), UpdatedAt: core.Now()}
	core.AssertEqual(t, "open", issue.State)
	core.AssertFalse(t, issue.CreatedAt.IsZero())
	core.AssertFalse(t, issue.UpdatedAt.IsZero())
}
