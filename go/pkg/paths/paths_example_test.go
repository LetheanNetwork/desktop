// SPDX-Licence-Identifier: EUPL-1.2

package paths_test

import (
	core "dappco.re/go"
	subject "dappco.re/lthn/desktop/pkg/paths"
)

func ExampleRoot() {
	ref := subject.Root
	_ = core.Sprintf("%T", ref)
}

func ExampleConfDir() {
	ref := subject.ConfDir
	_ = core.Sprintf("%T", ref)
}

func ExampleDataDir() {
	ref := subject.DataDir
	_ = core.Sprintf("%T", ref)
}

func ExampleWalletsDir() {
	ref := subject.WalletsDir
	_ = core.Sprintf("%T", ref)
}

func ExampleCliDir() {
	ref := subject.CliDir
	_ = core.Sprintf("%T", ref)
}

func ExampleModelsDir() {
	ref := subject.ModelsDir
	_ = core.Sprintf("%T", ref)
}

func ExampleConfigFile() {
	ref := subject.ConfigFile
	_ = core.Sprintf("%T", ref)
}

func ExampleStoreDB() {
	ref := subject.StoreDB
	_ = core.Sprintf("%T", ref)
}

func ExampleWorkspaceDir() {
	ref := subject.WorkspaceDir
	_ = core.Sprintf("%T", ref)
}

func TestPaths_Root_Bad(t *core.T) {
	ref := subject.Root
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:Root", "Root")
}

func TestPaths_Root_Ugly(t *core.T) {
	ref := subject.Root
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("Root"), 0)
}

func TestPaths_ConfDir_Bad(t *core.T) {
	ref := subject.ConfDir
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:ConfDir", "ConfDir")
}

func TestPaths_ConfDir_Ugly(t *core.T) {
	ref := subject.ConfDir
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("ConfDir"), 0)
}

func TestPaths_DataDir_Bad(t *core.T) {
	ref := subject.DataDir
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:DataDir", "DataDir")
}

func TestPaths_DataDir_Ugly(t *core.T) {
	ref := subject.DataDir
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("DataDir"), 0)
}

func TestPaths_WalletsDir_Bad(t *core.T) {
	ref := subject.WalletsDir
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:WalletsDir", "WalletsDir")
}

func TestPaths_WalletsDir_Ugly(t *core.T) {
	ref := subject.WalletsDir
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("WalletsDir"), 0)
}

func TestPaths_CliDir_Bad(t *core.T) {
	ref := subject.CliDir
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:CliDir", "CliDir")
}

func TestPaths_CliDir_Ugly(t *core.T) {
	ref := subject.CliDir
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("CliDir"), 0)
}

func TestPaths_ModelsDir_Bad(t *core.T) {
	ref := subject.ModelsDir
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:ModelsDir", "ModelsDir")
}

func TestPaths_ModelsDir_Ugly(t *core.T) {
	ref := subject.ModelsDir
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("ModelsDir"), 0)
}

func TestPaths_ConfigFile_Bad(t *core.T) {
	ref := subject.ConfigFile
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:ConfigFile", "ConfigFile")
}

func TestPaths_ConfigFile_Ugly(t *core.T) {
	ref := subject.ConfigFile
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("ConfigFile"), 0)
}

func TestPaths_StoreDB_Bad(t *core.T) {
	ref := subject.StoreDB
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:StoreDB", "StoreDB")
}

func TestPaths_StoreDB_Ugly(t *core.T) {
	ref := subject.StoreDB
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("StoreDB"), 0)
}

func TestPaths_WorkspaceDir_Bad(t *core.T) {
	ref := subject.WorkspaceDir
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:WorkspaceDir", "WorkspaceDir")
}

func TestPaths_WorkspaceDir_Ugly(t *core.T) {
	ref := subject.WorkspaceDir
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("WorkspaceDir"), 0)
}
