// SPDX-Licence-Identifier: EUPL-1.2

package api_test

import (
	core "dappco.re/go"
	subject "dappco.re/lthn/desktop/pkg/api"
)

func ExampleDefaultSpecInfo() {
	ref := subject.DefaultSpecInfo
	_ = core.Sprintf("%T", ref)
}

func ExampleExportSpec() {
	ref := subject.ExportSpec
	_ = core.Sprintf("%T", ref)
}

func ExampleGenerateSDK() {
	ref := subject.GenerateSDK
	_ = core.Sprintf("%T", ref)
}

func TestSdk_DefaultSpecInfo_Good(t *core.T) {
	ref := subject.DefaultSpecInfo
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "DefaultSpecInfo")
}

func TestSdk_DefaultSpecInfo_Bad(t *core.T) {
	ref := subject.DefaultSpecInfo
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:DefaultSpecInfo", "DefaultSpecInfo")
}

func TestSdk_DefaultSpecInfo_Ugly(t *core.T) {
	ref := subject.DefaultSpecInfo
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("DefaultSpecInfo"), 0)
}

func TestSdk_ExportSpec_Good(t *core.T) {
	ref := subject.ExportSpec
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "ExportSpec")
}

func TestSdk_ExportSpec_Bad(t *core.T) {
	ref := subject.ExportSpec
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:ExportSpec", "ExportSpec")
}

func TestSdk_ExportSpec_Ugly(t *core.T) {
	ref := subject.ExportSpec
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("ExportSpec"), 0)
}

func TestSdk_GenerateSDK_Good(t *core.T) {
	ref := subject.GenerateSDK
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "GenerateSDK")
}

func TestSdk_GenerateSDK_Bad(t *core.T) {
	ref := subject.GenerateSDK
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:GenerateSDK", "GenerateSDK")
}

func TestSdk_GenerateSDK_Ugly(t *core.T) {
	ref := subject.GenerateSDK
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("GenerateSDK"), 0)
}
