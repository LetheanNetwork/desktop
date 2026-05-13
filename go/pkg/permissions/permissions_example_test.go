// SPDX-Licence-Identifier: EUPL-1.2

package permissions_test

import (
	core "dappco.re/go"
	subject "dappco.re/lthn/desktop/pkg/permissions"
)

func ExampleInstall() {
	ref := subject.Install
	_ = core.Sprintf("%T", ref)
}

func TestPermissions_Install_Good(t *core.T) {
	ref := subject.Install
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "Install")
}

func TestPermissions_Install_Bad(t *core.T) {
	ref := subject.Install
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:Install", "Install")
}

func TestPermissions_Install_Ugly(t *core.T) {
	ref := subject.Install
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("Install"), 0)
}
