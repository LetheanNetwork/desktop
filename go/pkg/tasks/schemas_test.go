// SPDX-License-Identifier: EUPL-1.2

package tasks_test

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/tasks"
)

func TestSchemas_Schemas_Good(t *core.T) {
	schemas := tasks.Schemas()
	core.AssertLen(t, schemas, 2)
	core.AssertEqual(t, "tasks_issues", schemas[0].Name)
}

func TestSchemas_Schemas_Bad(t *core.T) {
	schemas := tasks.Schemas()
	core.AssertNotEqual(t, schemas[0].Name, schemas[1].Name)
}

func TestSchemas_Schemas_Ugly(t *core.T) {
	schemas := tasks.Schemas()
	for _, schema := range schemas {
		core.AssertNotEmpty(t, schema.PK)
		core.AssertNotEmpty(t, schema.Fields)
	}
}
