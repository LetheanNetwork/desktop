// SPDX-Licence-Identifier: EUPL-1.2

package lint_test

import (
	core "dappco.re/go"
	subject "dappco.re/lthn/desktop/pkg/lint"
)

func ExampleNewService() {
	ref := subject.NewService
	_ = core.Sprintf("%T", ref)
}

func ExampleRegister() {
	ref := subject.Register
	_ = core.Sprintf("%T", ref)
}

// TestLint_NewService_* and TestLint_Register_* — real behavioural
// tests, live in lint_test.go alongside the shared harness they need.
// (Previously this file paired each Example with a same-named Test
// that only formatted a method VALUE via %T and never invoked it —
// see wails_test.go's doc comment for the full mechanism writeup.)
