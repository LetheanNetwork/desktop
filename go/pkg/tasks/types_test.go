// SPDX-License-Identifier: EUPL-1.2

package tasks

import (
	"testing"

	core "dappco.re/go"
)

func TestTypesIssueSchema(t *testing.T) {
	schema := Issue{}.Schema()
	if schema.Name != "tasks_issues" {
		t.Fatalf("Issue Schema: expected tasks_issues, got %q", schema.Name)
	}
	if len(schema.Fields) == 0 {
		t.Fatal("Issue Schema: expected fields")
	}
}

func TestTypesNoteSchema(t *testing.T) {
	schema := Note{}.Schema()
	if schema.Name != "tasks_notes" {
		t.Fatalf("Note Schema: expected tasks_notes, got %q", schema.Name)
	}
	if len(schema.PK) != 1 || schema.PK[0] != "id" {
		t.Fatalf("Note Schema: expected id primary key, got %#v", schema.PK)
	}
}

func TestTypesIssueStateConstants(t *testing.T) {
	issue := Issue{State: StateOpen, CreatedAt: core.Now(), UpdatedAt: core.Now()}
	if issue.State != "open" {
		t.Fatalf("Issue constants: StateOpen drifted to %q", issue.State)
	}
	if issue.CreatedAt.IsZero() || issue.UpdatedAt.IsZero() {
		t.Fatal("Issue timestamps: expected non-zero test values")
	}
}
