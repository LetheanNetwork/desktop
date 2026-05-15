// SPDX-License-Identifier: EUPL-1.2

package tasks

import "dappco.re/go/orm"

// Schemas returns the orm schemas for the tasks subsystem. Called by
// the IDE's orm_init.go during boot so the issues + notes tables get
// registered alongside the demo note schema and any future schemas.
//
// Usage example:
//
//	for _, schema := range tasks.Schemas() {
//	    orm.RegisterSchema(c, schema)
//	}
func Schemas() []orm.Schema {
	return []orm.Schema{
		Issue{}.Schema(),
		Note{}.Schema(),
	}
}
