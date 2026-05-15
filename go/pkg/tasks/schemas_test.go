// SPDX-License-Identifier: EUPL-1.2

package tasks_test

import (
	"testing"

	"dappco.re/lthn/desktop/pkg/tasks"
)

func TestSchemas_Schemas_Good(t *testing.T) {
	schemas := tasks.Schemas()
	if len(schemas) != 2 {
		t.Fatalf("Schemas: expected issue and note schemas, got %d", len(schemas))
	}
	if schemas[0].Name != "tasks_issues" {
		t.Fatalf("Schemas: first schema should be tasks_issues, got %q", schemas[0].Name)
	}
}

func TestSchemas_Schemas_Bad(t *testing.T) {
	schemas := tasks.Schemas()
	if schemas[0].Name == schemas[1].Name {
		t.Fatalf("Schemas: issue and note schemas must have distinct names: %#v", schemas)
	}
}

func TestSchemas_Schemas_Ugly(t *testing.T) {
	schemas := tasks.Schemas()
	for _, schema := range schemas {
		if len(schema.PK) == 0 || len(schema.Fields) == 0 {
			t.Fatalf("Schemas: schema missing pk or fields: %#v", schema)
		}
	}
}
