// SPDX-Licence-Identifier: EUPL-1.2

package git_test

import (
	core "dappco.re/go"
	subject "dappco.re/lthn/desktop/pkg/git"
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

func ExampleService_Branch() {
	ref := (*subject.Service).Branch
	_ = core.Sprintf("%T", ref)
}

func ExampleService_Status() {
	ref := (*subject.Service).Status
	_ = core.Sprintf("%T", ref)
}

func ExampleService_Diff() {
	ref := (*subject.Service).Diff
	_ = core.Sprintf("%T", ref)
}

func ExampleService_Add() {
	ref := (*subject.Service).Add
	_ = core.Sprintf("%T", ref)
}

func ExampleService_Unstage() {
	ref := (*subject.Service).Unstage
	_ = core.Sprintf("%T", ref)
}

func ExampleService_Commit() {
	ref := (*subject.Service).Commit
	_ = core.Sprintf("%T", ref)
}

func ExampleService_Log() {
	ref := (*subject.Service).Log
	_ = core.Sprintf("%T", ref)
}

// TestWails_Service_* — real behavioural tests, live in wails_test.go.
// (Previously this file paired each Example with a same-named Test
// that only formatted a method VALUE via %T and never invoked it —
// see wails_test.go's doc comment for the full mechanism writeup.)
