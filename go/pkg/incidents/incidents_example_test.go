// SPDX-Licence-Identifier: EUPL-1.2

package incidents_test

import (
	subject "dappco.re/lthn/desktop/pkg/incidents"
)

// ExampleNewService documents the registration pattern used in app.go.
func ExampleNewService() {
	// core.New(core.WithName("incidents", incidents.Register))
	_ = subject.NewService(nil)
}

// ExampleService_List shows how a Wails frontend consumes the List method.
func ExampleService_List() {
	svc := subject.NewService(nil)
	r := svc.List(subject.ListInput{State: "investigating", Limit: 20})
	if r.OK {
		out := r.Value.(subject.ListOutput)
		_ = out.Total
	}
}
