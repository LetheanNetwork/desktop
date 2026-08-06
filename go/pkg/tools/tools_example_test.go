// SPDX-Licence-Identifier: EUPL-1.2

package tools_test

import (
	core "dappco.re/go"
	subject "dappco.re/lthn/desktop/pkg/tools"
)

func ExampleNewWailsService() {
	ref := subject.NewWailsService
	_ = core.Sprintf("%T", ref)
}

func ExampleWailsService_ServiceName() {
	ref := (*subject.WailsService).ServiceName
	_ = core.Sprintf("%T", ref)
}

func ExampleWailsService_ServiceStartup() {
	ref := (*subject.WailsService).ServiceStartup
	_ = core.Sprintf("%T", ref)
}

func ExampleWailsService_ServiceShutdown() {
	ref := (*subject.WailsService).ServiceShutdown
	_ = core.Sprintf("%T", ref)
}

func ExampleWailsService_List() {
	ref := (*subject.WailsService).List
	_ = core.Sprintf("%T", ref)
}

func ExampleRegister() {
	ref := subject.Register
	_ = core.Sprintf("%T", ref)
}

// TestTools_* — real behavioural tests, live in tools_test.go.
// (Previously this file paired each Example with a same-named Test
// that only formatted a method VALUE via %T and never invoked it —
// see pkg/git's wails_test.go doc comment for the full mechanism
// writeup.)
