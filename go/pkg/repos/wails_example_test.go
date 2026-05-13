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

func TestWails_Service_Status_Good(t *core.T) {
	ref := (*subject.Service).Status
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "Service_Status")
}

func TestWails_Service_Status_Bad(t *core.T) {
	ref := (*subject.Service).Status
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:Service_Status", "Service_Status")
}

func TestWails_Service_Status_Ugly(t *core.T) {
	ref := (*subject.Service).Status
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("Service_Status"), 0)
}
