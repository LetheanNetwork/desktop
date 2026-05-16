// SPDX-Licence-Identifier: EUPL-1.2

// Example_* functions double as TS-binding inspiration: each shows
// the Service method signature an Angular/Lit caller will see after
// `task generate:bindings` regenerates the frontend bindings tree.

package tasks_test

import (
	core "dappco.re/go"
	subject "dappco.re/lthn/desktop/pkg/tasks"
)

func ExampleService_List() {
	ref := (*subject.Service).List
	_ = core.Sprintf("%T", ref)
}

func ExampleService_Get() {
	ref := (*subject.Service).Get
	_ = core.Sprintf("%T", ref)
}

func ExampleService_Create() {
	ref := (*subject.Service).Create
	_ = core.Sprintf("%T", ref)
}

func ExampleService_Update() {
	ref := (*subject.Service).Update
	_ = core.Sprintf("%T", ref)
}

func ExampleService_AddNote() {
	ref := (*subject.Service).AddNote
	_ = core.Sprintf("%T", ref)
}

func ExampleNewService() {
	c := core.New()
	svc := subject.NewService(c)
	_ = svc
}
