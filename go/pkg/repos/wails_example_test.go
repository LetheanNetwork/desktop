// SPDX-Licence-Identifier: EUPL-1.2

package repos_test

import (
	core "dappco.re/go"
	subject "dappco.re/lthn/desktop/pkg/repos"
)

func ExampleService_Status() {
	ref := (*subject.Service).Status
	_ = core.Sprintf("%T", ref)
}

// TestWails_Service_Status_* — real behavioural tests, live in
// wails_test.go. (Previously this file paired the Example with a
// same-named Test that only formatted a method VALUE via %T and
// never invoked it — see wails_test.go's doc comment for the full
// mechanism writeup.)
