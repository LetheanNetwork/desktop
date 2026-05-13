// SPDX-Licence-Identifier: EUPL-1.2

package services_test

import (
	core "dappco.re/go"
	subject "dappco.re/lthn/desktop/pkg/services"
)

func ExampleNewWailsService() {
	ref := subject.NewWailsService
	_ = core.Sprintf("%T", ref)
}

func ExampleWailsService_ServiceName() {
	ref := (*subject.WailsService).ServiceName
	_ = core.Sprintf("%T", ref)
}

func ExampleWailsService_ServiceStartup() {
	ref := (*subject.WailsService).ServiceStartup
	_ = core.Sprintf("%T", ref)
}

func ExampleWailsService_ServiceShutdown() {
	ref := (*subject.WailsService).ServiceShutdown
	_ = core.Sprintf("%T", ref)
}

func ExampleWailsService_Registry() {
	ref := (*subject.WailsService).Registry
	_ = core.Sprintf("%T", ref)
}

func ExampleWailsService_Install() {
	ref := (*subject.WailsService).Install
	_ = core.Sprintf("%T", ref)
}

func ExampleWailsService_Uninstall() {
	ref := (*subject.WailsService).Uninstall
	_ = core.Sprintf("%T", ref)
}

func ExampleWailsService_Start() {
	ref := (*subject.WailsService).Start
	_ = core.Sprintf("%T", ref)
}

func ExampleWailsService_Stop() {
	ref := (*subject.WailsService).Stop
	_ = core.Sprintf("%T", ref)
}

func ExampleWailsService_Restart() {
	ref := (*subject.WailsService).Restart
	_ = core.Sprintf("%T", ref)
}

func ExampleWailsService_Status() {
	ref := (*subject.WailsService).Status
	_ = core.Sprintf("%T", ref)
}

func TestWails_NewWailsService_Good(t *core.T) {
	ref := subject.NewWailsService
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "NewWailsService")
}

func TestWails_NewWailsService_Bad(t *core.T) {
	ref := subject.NewWailsService
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:NewWailsService", "NewWailsService")
}

func TestWails_NewWailsService_Ugly(t *core.T) {
	ref := subject.NewWailsService
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("NewWailsService"), 0)
}

func TestWails_WailsService_ServiceName_Good(t *core.T) {
	ref := (*subject.WailsService).ServiceName
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "WailsService_ServiceName")
}

func TestWails_WailsService_ServiceName_Bad(t *core.T) {
	ref := (*subject.WailsService).ServiceName
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:WailsService_ServiceName", "WailsService_ServiceName")
}

func TestWails_WailsService_ServiceName_Ugly(t *core.T) {
	ref := (*subject.WailsService).ServiceName
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("WailsService_ServiceName"), 0)
}

func TestWails_WailsService_ServiceStartup_Good(t *core.T) {
	ref := (*subject.WailsService).ServiceStartup
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "WailsService_ServiceStartup")
}

func TestWails_WailsService_ServiceStartup_Bad(t *core.T) {
	ref := (*subject.WailsService).ServiceStartup
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:WailsService_ServiceStartup", "WailsService_ServiceStartup")
}

func TestWails_WailsService_ServiceStartup_Ugly(t *core.T) {
	ref := (*subject.WailsService).ServiceStartup
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("WailsService_ServiceStartup"), 0)
}

func TestWails_WailsService_ServiceShutdown_Good(t *core.T) {
	ref := (*subject.WailsService).ServiceShutdown
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "WailsService_ServiceShutdown")
}

func TestWails_WailsService_ServiceShutdown_Bad(t *core.T) {
	ref := (*subject.WailsService).ServiceShutdown
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:WailsService_ServiceShutdown", "WailsService_ServiceShutdown")
}

func TestWails_WailsService_ServiceShutdown_Ugly(t *core.T) {
	ref := (*subject.WailsService).ServiceShutdown
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("WailsService_ServiceShutdown"), 0)
}

func TestWails_WailsService_Registry_Good(t *core.T) {
	ref := (*subject.WailsService).Registry
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "WailsService_Registry")
}

func TestWails_WailsService_Registry_Bad(t *core.T) {
	ref := (*subject.WailsService).Registry
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:WailsService_Registry", "WailsService_Registry")
}

func TestWails_WailsService_Registry_Ugly(t *core.T) {
	ref := (*subject.WailsService).Registry
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("WailsService_Registry"), 0)
}

func TestWails_WailsService_Install_Good(t *core.T) {
	ref := (*subject.WailsService).Install
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "WailsService_Install")
}

func TestWails_WailsService_Install_Bad(t *core.T) {
	ref := (*subject.WailsService).Install
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:WailsService_Install", "WailsService_Install")
}

func TestWails_WailsService_Install_Ugly(t *core.T) {
	ref := (*subject.WailsService).Install
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("WailsService_Install"), 0)
}

func TestWails_WailsService_Uninstall_Good(t *core.T) {
	ref := (*subject.WailsService).Uninstall
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "WailsService_Uninstall")
}

func TestWails_WailsService_Uninstall_Bad(t *core.T) {
	ref := (*subject.WailsService).Uninstall
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:WailsService_Uninstall", "WailsService_Uninstall")
}

func TestWails_WailsService_Uninstall_Ugly(t *core.T) {
	ref := (*subject.WailsService).Uninstall
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("WailsService_Uninstall"), 0)
}

func TestWails_WailsService_Start_Good(t *core.T) {
	ref := (*subject.WailsService).Start
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "WailsService_Start")
}

func TestWails_WailsService_Start_Bad(t *core.T) {
	ref := (*subject.WailsService).Start
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:WailsService_Start", "WailsService_Start")
}

func TestWails_WailsService_Start_Ugly(t *core.T) {
	ref := (*subject.WailsService).Start
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("WailsService_Start"), 0)
}

func TestWails_WailsService_Stop_Good(t *core.T) {
	ref := (*subject.WailsService).Stop
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "WailsService_Stop")
}

func TestWails_WailsService_Stop_Bad(t *core.T) {
	ref := (*subject.WailsService).Stop
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:WailsService_Stop", "WailsService_Stop")
}

func TestWails_WailsService_Stop_Ugly(t *core.T) {
	ref := (*subject.WailsService).Stop
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("WailsService_Stop"), 0)
}

func TestWails_WailsService_Restart_Good(t *core.T) {
	ref := (*subject.WailsService).Restart
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "WailsService_Restart")
}

func TestWails_WailsService_Restart_Bad(t *core.T) {
	ref := (*subject.WailsService).Restart
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:WailsService_Restart", "WailsService_Restart")
}

func TestWails_WailsService_Restart_Ugly(t *core.T) {
	ref := (*subject.WailsService).Restart
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("WailsService_Restart"), 0)
}

func TestWails_WailsService_Status_Good(t *core.T) {
	ref := (*subject.WailsService).Status
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "WailsService_Status")
}

func TestWails_WailsService_Status_Bad(t *core.T) {
	ref := (*subject.WailsService).Status
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:WailsService_Status", "WailsService_Status")
}

func TestWails_WailsService_Status_Ugly(t *core.T) {
	ref := (*subject.WailsService).Status
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("WailsService_Status"), 0)
}
