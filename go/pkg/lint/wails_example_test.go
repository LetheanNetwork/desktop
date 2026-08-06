// SPDX-Licence-Identifier: EUPL-1.2

package lint_test

import (
	core "dappco.re/go"
	subject "dappco.re/lthn/desktop/pkg/lint"
)

func ExampleService_ServiceName() {
	ref := (*subject.Service).ServiceName
	_ = core.Sprintf("%T", ref)
}

func ExampleService_ServiceStartup() {
	ref := (*subject.Service).ServiceStartup
	_ = core.Sprintf("%T", ref)
}

func ExampleService_ServiceShutdown() {
	ref := (*subject.Service).ServiceShutdown
	_ = core.Sprintf("%T", ref)
}

func ExampleService_Run() {
	ref := (*subject.Service).Run
	_ = core.Sprintf("%T", ref)
}

func ExampleService_Catalog() {
	ref := (*subject.Service).Catalog
	_ = core.Sprintf("%T", ref)
}

// TestWails_Service_* — real behavioural tests, live in
// wails_test.go. (Previously this file paired each Example with a
// same-named Test that only formatted a method VALUE via %T and
// never invoked it — see wails_test.go's doc comment for the full
// mechanism writeup.)
