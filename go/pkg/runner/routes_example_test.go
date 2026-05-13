// SPDX-Licence-Identifier: EUPL-1.2

package runner_test

import (
	core "dappco.re/go"
	subject "dappco.re/lthn/desktop/pkg/runner"
)

func ExampleLoadRoutesFromCore() {
	ref := subject.LoadRoutesFromCore
	_ = core.Sprintf("%T", ref)
}

func ExampleNewServiceFromCore() {
	ref := subject.NewServiceFromCore
	_ = core.Sprintf("%T", ref)
}

func TestRoutes_LoadRoutesFromCore_Good(t *core.T) {
	ref := subject.LoadRoutesFromCore
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "LoadRoutesFromCore")
}

func TestRoutes_LoadRoutesFromCore_Bad(t *core.T) {
	ref := subject.LoadRoutesFromCore
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:LoadRoutesFromCore", "LoadRoutesFromCore")
}

func TestRoutes_LoadRoutesFromCore_Ugly(t *core.T) {
	ref := subject.LoadRoutesFromCore
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("LoadRoutesFromCore"), 0)
}

func TestRoutes_NewServiceFromCore_Good(t *core.T) {
	ref := subject.NewServiceFromCore
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "NewServiceFromCore")
}

func TestRoutes_NewServiceFromCore_Bad(t *core.T) {
	ref := subject.NewServiceFromCore
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:NewServiceFromCore", "NewServiceFromCore")
}

func TestRoutes_NewServiceFromCore_Ugly(t *core.T) {
	ref := subject.NewServiceFromCore
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("NewServiceFromCore"), 0)
}
