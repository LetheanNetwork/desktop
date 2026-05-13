// SPDX-Licence-Identifier: EUPL-1.2

package api_test

import (
	core "dappco.re/go"
	subject "dappco.re/lthn/desktop/pkg/api"
)

func ExampleNewRunnerGroup() {
	ref := subject.NewRunnerGroup
	_ = core.Sprintf("%T", ref)
}

func ExampleRunnerGroup_Name() {
	ref := (*subject.RunnerGroup).Name
	_ = core.Sprintf("%T", ref)
}

func ExampleRunnerGroup_BasePath() {
	ref := (*subject.RunnerGroup).BasePath
	_ = core.Sprintf("%T", ref)
}

func ExampleRunnerGroup_RegisterRoutes() {
	ref := (*subject.RunnerGroup).RegisterRoutes
	_ = core.Sprintf("%T", ref)
}

func ExampleRunnerGroup_Describe() {
	ref := (*subject.RunnerGroup).Describe
	_ = core.Sprintf("%T", ref)
}

func TestRunnerGroup_NewRunnerGroup_Good(t *core.T) {
	ref := subject.NewRunnerGroup
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "NewRunnerGroup")
}

func TestRunnerGroup_NewRunnerGroup_Bad(t *core.T) {
	ref := subject.NewRunnerGroup
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:NewRunnerGroup", "NewRunnerGroup")
}

func TestRunnerGroup_NewRunnerGroup_Ugly(t *core.T) {
	ref := subject.NewRunnerGroup
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("NewRunnerGroup"), 0)
}

func TestRunnerGroup_RunnerGroup_Name_Good(t *core.T) {
	ref := (*subject.RunnerGroup).Name
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "RunnerGroup_Name")
}

func TestRunnerGroup_RunnerGroup_Name_Bad(t *core.T) {
	ref := (*subject.RunnerGroup).Name
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:RunnerGroup_Name", "RunnerGroup_Name")
}

func TestRunnerGroup_RunnerGroup_Name_Ugly(t *core.T) {
	ref := (*subject.RunnerGroup).Name
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("RunnerGroup_Name"), 0)
}

func TestRunnerGroup_RunnerGroup_BasePath_Good(t *core.T) {
	ref := (*subject.RunnerGroup).BasePath
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "RunnerGroup_BasePath")
}

func TestRunnerGroup_RunnerGroup_BasePath_Bad(t *core.T) {
	ref := (*subject.RunnerGroup).BasePath
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:RunnerGroup_BasePath", "RunnerGroup_BasePath")
}

func TestRunnerGroup_RunnerGroup_BasePath_Ugly(t *core.T) {
	ref := (*subject.RunnerGroup).BasePath
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("RunnerGroup_BasePath"), 0)
}

func TestRunnerGroup_RunnerGroup_RegisterRoutes_Good(t *core.T) {
	ref := (*subject.RunnerGroup).RegisterRoutes
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "RunnerGroup_RegisterRoutes")
}

func TestRunnerGroup_RunnerGroup_RegisterRoutes_Bad(t *core.T) {
	ref := (*subject.RunnerGroup).RegisterRoutes
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:RunnerGroup_RegisterRoutes", "RunnerGroup_RegisterRoutes")
}

func TestRunnerGroup_RunnerGroup_RegisterRoutes_Ugly(t *core.T) {
	ref := (*subject.RunnerGroup).RegisterRoutes
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("RunnerGroup_RegisterRoutes"), 0)
}

func TestRunnerGroup_RunnerGroup_Describe_Good(t *core.T) {
	ref := (*subject.RunnerGroup).Describe
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "RunnerGroup_Describe")
}

func TestRunnerGroup_RunnerGroup_Describe_Bad(t *core.T) {
	ref := (*subject.RunnerGroup).Describe
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:RunnerGroup_Describe", "RunnerGroup_Describe")
}

func TestRunnerGroup_RunnerGroup_Describe_Ugly(t *core.T) {
	ref := (*subject.RunnerGroup).Describe
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("RunnerGroup_Describe"), 0)
}
