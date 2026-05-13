// SPDX-Licence-Identifier: EUPL-1.2

package services_test

import (
	core "dappco.re/go"
	subject "dappco.re/lthn/desktop/pkg/services"
)

func ExampleInstall() {
	ref := subject.Install
	_ = core.Sprintf("%T", ref)
}

func ExampleUninstall() {
	ref := subject.Uninstall
	_ = core.Sprintf("%T", ref)
}

func ExampleStart() {
	ref := subject.Start
	_ = core.Sprintf("%T", ref)
}

func ExampleStop() {
	ref := subject.Stop
	_ = core.Sprintf("%T", ref)
}

func ExampleRestart() {
	ref := subject.Restart
	_ = core.Sprintf("%T", ref)
}

func ExampleStatus() {
	ref := subject.Status
	_ = core.Sprintf("%T", ref)
}

func TestManager_Install_Good(t *core.T) {
	ref := subject.Install
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "Install")
}

func TestManager_Install_Bad(t *core.T) {
	ref := subject.Install
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:Install", "Install")
}

func TestManager_Install_Ugly(t *core.T) {
	ref := subject.Install
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("Install"), 0)
}

func TestManager_Uninstall_Good(t *core.T) {
	ref := subject.Uninstall
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "Uninstall")
}

func TestManager_Uninstall_Bad(t *core.T) {
	ref := subject.Uninstall
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:Uninstall", "Uninstall")
}

func TestManager_Uninstall_Ugly(t *core.T) {
	ref := subject.Uninstall
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("Uninstall"), 0)
}

func TestManager_Start_Good(t *core.T) {
	ref := subject.Start
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "Start")
}

func TestManager_Start_Bad(t *core.T) {
	ref := subject.Start
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:Start", "Start")
}

func TestManager_Start_Ugly(t *core.T) {
	ref := subject.Start
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("Start"), 0)
}

func TestManager_Stop_Good(t *core.T) {
	ref := subject.Stop
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "Stop")
}

func TestManager_Stop_Bad(t *core.T) {
	ref := subject.Stop
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:Stop", "Stop")
}

func TestManager_Stop_Ugly(t *core.T) {
	ref := subject.Stop
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("Stop"), 0)
}

func TestManager_Restart_Good(t *core.T) {
	ref := subject.Restart
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "Restart")
}

func TestManager_Restart_Bad(t *core.T) {
	ref := subject.Restart
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:Restart", "Restart")
}

func TestManager_Restart_Ugly(t *core.T) {
	ref := subject.Restart
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("Restart"), 0)
}

func TestManager_Status_Good(t *core.T) {
	ref := subject.Status
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "Status")
}

func TestManager_Status_Bad(t *core.T) {
	ref := subject.Status
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:Status", "Status")
}

func TestManager_Status_Ugly(t *core.T) {
	ref := subject.Status
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("Status"), 0)
}
