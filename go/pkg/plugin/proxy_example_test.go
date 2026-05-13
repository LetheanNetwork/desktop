// SPDX-Licence-Identifier: EUPL-1.2

package plugin_test

import (
	core "dappco.re/go"
	subject "dappco.re/lthn/desktop/pkg/plugin"
)

func ExampleNewProxyGroup() {
	ref := subject.NewProxyGroup
	_ = core.Sprintf("%T", ref)
}

func ExampleProxyGroup_Name() {
	ref := (*subject.ProxyGroup).Name
	_ = core.Sprintf("%T", ref)
}

func ExampleProxyGroup_BasePath() {
	ref := (*subject.ProxyGroup).BasePath
	_ = core.Sprintf("%T", ref)
}

func ExampleProxyGroup_RegisterRoutes() {
	ref := (*subject.ProxyGroup).RegisterRoutes
	_ = core.Sprintf("%T", ref)
}

func ExampleProxyGroup_Set() {
	ref := (*subject.ProxyGroup).Set
	_ = core.Sprintf("%T", ref)
}

func ExampleProxyGroup_Delete() {
	ref := (*subject.ProxyGroup).Delete
	_ = core.Sprintf("%T", ref)
}

func ExampleProxyGroup_Has() {
	ref := (*subject.ProxyGroup).Has
	_ = core.Sprintf("%T", ref)
}

func TestProxy_NewProxyGroup_Good(t *core.T) {
	ref := subject.NewProxyGroup
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "NewProxyGroup")
}

func TestProxy_NewProxyGroup_Bad(t *core.T) {
	ref := subject.NewProxyGroup
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:NewProxyGroup", "NewProxyGroup")
}

func TestProxy_NewProxyGroup_Ugly(t *core.T) {
	ref := subject.NewProxyGroup
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("NewProxyGroup"), 0)
}

func TestProxy_ProxyGroup_Name_Good(t *core.T) {
	ref := (*subject.ProxyGroup).Name
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "ProxyGroup_Name")
}

func TestProxy_ProxyGroup_Name_Bad(t *core.T) {
	ref := (*subject.ProxyGroup).Name
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:ProxyGroup_Name", "ProxyGroup_Name")
}

func TestProxy_ProxyGroup_Name_Ugly(t *core.T) {
	ref := (*subject.ProxyGroup).Name
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("ProxyGroup_Name"), 0)
}

func TestProxy_ProxyGroup_BasePath_Good(t *core.T) {
	ref := (*subject.ProxyGroup).BasePath
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "ProxyGroup_BasePath")
}

func TestProxy_ProxyGroup_BasePath_Bad(t *core.T) {
	ref := (*subject.ProxyGroup).BasePath
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:ProxyGroup_BasePath", "ProxyGroup_BasePath")
}

func TestProxy_ProxyGroup_BasePath_Ugly(t *core.T) {
	ref := (*subject.ProxyGroup).BasePath
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("ProxyGroup_BasePath"), 0)
}

func TestProxy_ProxyGroup_RegisterRoutes_Good(t *core.T) {
	ref := (*subject.ProxyGroup).RegisterRoutes
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "ProxyGroup_RegisterRoutes")
}

func TestProxy_ProxyGroup_RegisterRoutes_Bad(t *core.T) {
	ref := (*subject.ProxyGroup).RegisterRoutes
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:ProxyGroup_RegisterRoutes", "ProxyGroup_RegisterRoutes")
}

func TestProxy_ProxyGroup_RegisterRoutes_Ugly(t *core.T) {
	ref := (*subject.ProxyGroup).RegisterRoutes
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("ProxyGroup_RegisterRoutes"), 0)
}

func TestProxy_ProxyGroup_Set_Good(t *core.T) {
	ref := (*subject.ProxyGroup).Set
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "ProxyGroup_Set")
}

func TestProxy_ProxyGroup_Set_Bad(t *core.T) {
	ref := (*subject.ProxyGroup).Set
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:ProxyGroup_Set", "ProxyGroup_Set")
}

func TestProxy_ProxyGroup_Set_Ugly(t *core.T) {
	ref := (*subject.ProxyGroup).Set
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("ProxyGroup_Set"), 0)
}

func TestProxy_ProxyGroup_Delete_Good(t *core.T) {
	ref := (*subject.ProxyGroup).Delete
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "ProxyGroup_Delete")
}

func TestProxy_ProxyGroup_Delete_Bad(t *core.T) {
	ref := (*subject.ProxyGroup).Delete
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:ProxyGroup_Delete", "ProxyGroup_Delete")
}

func TestProxy_ProxyGroup_Delete_Ugly(t *core.T) {
	ref := (*subject.ProxyGroup).Delete
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("ProxyGroup_Delete"), 0)
}

func TestProxy_ProxyGroup_Has_Good(t *core.T) {
	ref := (*subject.ProxyGroup).Has
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "ProxyGroup_Has")
}

func TestProxy_ProxyGroup_Has_Bad(t *core.T) {
	ref := (*subject.ProxyGroup).Has
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:ProxyGroup_Has", "ProxyGroup_Has")
}

func TestProxy_ProxyGroup_Has_Ugly(t *core.T) {
	ref := (*subject.ProxyGroup).Has
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("ProxyGroup_Has"), 0)
}
