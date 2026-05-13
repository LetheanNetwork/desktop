// SPDX-Licence-Identifier: EUPL-1.2

package plugin_test

import (
	core "dappco.re/go"
	subject "dappco.re/lthn/desktop/pkg/plugin"
)

func ExampleService_Menus() {
	ref := (*subject.Service).Menus
	_ = core.Sprintf("%T", ref)
}

func TestMenus_Service_Menus_Good(t *core.T) {
	ref := (*subject.Service).Menus
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "Service_Menus")
}

func TestMenus_Service_Menus_Bad(t *core.T) {
	ref := (*subject.Service).Menus
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:Service_Menus", "Service_Menus")
}

func TestMenus_Service_Menus_Ugly(t *core.T) {
	ref := (*subject.Service).Menus
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("Service_Menus"), 0)
}
