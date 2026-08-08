// SPDX-Licence-Identifier: EUPL-1.2

package deploys_test

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/deploys"
)

// ExampleService_List shows how to list all recent deploys.
func ExampleService_List() {
	c := core.New()
	svc := deploys.NewService(c)

	r := svc.List(deploys.ListInput{Limit: 10})
	if !r.OK {
		return
	}
	out := r.Value.(deploys.ListOutput)
	for _, row := range out.History {
		_ = row.Env     // "production"
		_ = row.Outcome // "success"
	}
	for _, env := range out.Envs {
		_ = env.Name   // "production"
		_ = env.Health // "ok"
	}
}

// ExampleService_Get shows how to fetch a single deploy record by ID.
func ExampleService_Get() {
	c := core.New()
	svc := deploys.NewService(c)

	r := svc.Get(deploys.GetInput{ID: "deploy-20260516-1432"})
	if !r.OK {
		return
	}
	out := r.Value.(deploys.GetOutput)
	_ = out.Record.Env // "preview"
	_ = out.Record.By  // "Tobi"
	_ = out.Notes      // "Deploy went smoothly."
}

// ExampleService_Create shows how to persist a new deploy record.
func ExampleService_Create() {
	c := core.New()
	svc := deploys.NewService(c)

	r := svc.Create(deploys.CreateInput{
		Env:     "preview",
		By:      "Tobi",
		Commit:  "b8e034",
		Outcome: "success",
		Dur:     "58s",
		Notes:   "All checks passed.",
	})
	if !r.OK {
		return
	}
	out := r.Value.(deploys.CreateOutput)
	_ = out.ID // "deploy-20260516-1432"
}
