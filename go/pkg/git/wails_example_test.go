// SPDX-Licence-Identifier: EUPL-1.2

package git_test

import (
	core "dappco.re/go"
	subject "dappco.re/lthn/desktop/pkg/git"
)

func ExampleService_ServiceName() {
	ref := (*subject.Service).ServiceName
	_ = core.Sprintf("%T", ref)
}

func ExampleService_ServiceStartup() {
	ref := (*subject.Service).ServiceStartup
	_ = core.Sprintf("%T", ref)
}

func ExampleService_ServiceShutdown() {
	ref := (*subject.Service).ServiceShutdown
	_ = core.Sprintf("%T", ref)
}

func ExampleService_Branch() {
	ref := (*subject.Service).Branch
	_ = core.Sprintf("%T", ref)
}

func ExampleService_Status() {
	ref := (*subject.Service).Status
	_ = core.Sprintf("%T", ref)
}

func ExampleService_Diff() {
	ref := (*subject.Service).Diff
	_ = core.Sprintf("%T", ref)
}

func ExampleService_Add() {
	ref := (*subject.Service).Add
	_ = core.Sprintf("%T", ref)
}

func ExampleService_Unstage() {
	ref := (*subject.Service).Unstage
	_ = core.Sprintf("%T", ref)
}

func ExampleService_Commit() {
	ref := (*subject.Service).Commit
	_ = core.Sprintf("%T", ref)
}

func ExampleService_Log() {
	ref := (*subject.Service).Log
	_ = core.Sprintf("%T", ref)
}

func TestWails_Service_ServiceName_Good(t *core.T) {
	ref := (*subject.Service).ServiceName
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "Service_ServiceName")
}

func TestWails_Service_ServiceName_Bad(t *core.T) {
	ref := (*subject.Service).ServiceName
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:Service_ServiceName", "Service_ServiceName")
}

func TestWails_Service_ServiceName_Ugly(t *core.T) {
	ref := (*subject.Service).ServiceName
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("Service_ServiceName"), 0)
}

func TestWails_Service_ServiceStartup_Good(t *core.T) {
	ref := (*subject.Service).ServiceStartup
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "Service_ServiceStartup")
}

func TestWails_Service_ServiceStartup_Bad(t *core.T) {
	ref := (*subject.Service).ServiceStartup
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:Service_ServiceStartup", "Service_ServiceStartup")
}

func TestWails_Service_ServiceStartup_Ugly(t *core.T) {
	ref := (*subject.Service).ServiceStartup
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("Service_ServiceStartup"), 0)
}

func TestWails_Service_ServiceShutdown_Good(t *core.T) {
	ref := (*subject.Service).ServiceShutdown
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "Service_ServiceShutdown")
}

func TestWails_Service_ServiceShutdown_Bad(t *core.T) {
	ref := (*subject.Service).ServiceShutdown
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:Service_ServiceShutdown", "Service_ServiceShutdown")
}

func TestWails_Service_ServiceShutdown_Ugly(t *core.T) {
	ref := (*subject.Service).ServiceShutdown
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("Service_ServiceShutdown"), 0)
}

func TestWails_Service_Branch_Good(t *core.T) {
	ref := (*subject.Service).Branch
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "Service_Branch")
}

func TestWails_Service_Branch_Bad(t *core.T) {
	ref := (*subject.Service).Branch
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:Service_Branch", "Service_Branch")
}

func TestWails_Service_Branch_Ugly(t *core.T) {
	ref := (*subject.Service).Branch
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("Service_Branch"), 0)
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

func TestWails_Service_Diff_Good(t *core.T) {
	ref := (*subject.Service).Diff
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "Service_Diff")
}

func TestWails_Service_Diff_Bad(t *core.T) {
	ref := (*subject.Service).Diff
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:Service_Diff", "Service_Diff")
}

func TestWails_Service_Diff_Ugly(t *core.T) {
	ref := (*subject.Service).Diff
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("Service_Diff"), 0)
}

func TestWails_Service_Add_Good(t *core.T) {
	ref := (*subject.Service).Add
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "Service_Add")
}

func TestWails_Service_Add_Bad(t *core.T) {
	ref := (*subject.Service).Add
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:Service_Add", "Service_Add")
}

func TestWails_Service_Add_Ugly(t *core.T) {
	ref := (*subject.Service).Add
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("Service_Add"), 0)
}

func TestWails_Service_Unstage_Good(t *core.T) {
	ref := (*subject.Service).Unstage
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "Service_Unstage")
}

func TestWails_Service_Unstage_Bad(t *core.T) {
	ref := (*subject.Service).Unstage
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:Service_Unstage", "Service_Unstage")
}

func TestWails_Service_Unstage_Ugly(t *core.T) {
	ref := (*subject.Service).Unstage
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("Service_Unstage"), 0)
}

func TestWails_Service_Commit_Good(t *core.T) {
	ref := (*subject.Service).Commit
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "Service_Commit")
}

func TestWails_Service_Commit_Bad(t *core.T) {
	ref := (*subject.Service).Commit
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:Service_Commit", "Service_Commit")
}

func TestWails_Service_Commit_Ugly(t *core.T) {
	ref := (*subject.Service).Commit
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("Service_Commit"), 0)
}

func TestWails_Service_Log_Good(t *core.T) {
	ref := (*subject.Service).Log
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "Service_Log")
}

func TestWails_Service_Log_Bad(t *core.T) {
	ref := (*subject.Service).Log
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:Service_Log", "Service_Log")
}

func TestWails_Service_Log_Ugly(t *core.T) {
	ref := (*subject.Service).Log
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("Service_Log"), 0)
}
