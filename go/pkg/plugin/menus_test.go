// SPDX-Licence-Identifier: EUPL-1.2

// Internal-package tests for menus.go's Menus(). package plugin so the
// test can populate s.state directly for the Running-flag assertions
// without needing a real process.

package plugin

import core "dappco.re/go"

func TestMenus_Service_Menus_Good_EmptyWhenNoPluginsInstalled(t *core.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	svc := newTestService(t, core.New())
	r := svc.Menus()
	core.RequireTrue(t, r.OK)
	core.AssertEqual(t, 0, len(r.Value.([]MenuEntry)))
}

func TestMenus_Service_Menus_Good_SkipsPluginsWithoutMenuBlock(t *core.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	dir := tmp + "/Lethean/conf/plugins/headless"
	core.RequireTrue(t, core.MkdirAll(dir, 0o755).OK)
	core.RequireTrue(t, saveManifest(dir, Manifest{Code: "headless", Name: "Headless", Binary: "bin/headless"}).OK)

	svc := newTestService(t, core.New())
	r := svc.Menus()
	core.RequireTrue(t, r.OK)
	core.AssertEqual(t, 0, len(r.Value.([]MenuEntry)), "no manifest.menu block -> not surfaced")
}

// TestMenus_Service_Menus_Good_IncludesEntryURLDefaultsLabelAndRunningFlag
// drives the full happy path: a Menu block with no Label (falls back to
// manifest Name), a UI.Entrypoint (populates EntryURL), and a tracked
// running state (Running: true).
func TestMenus_Service_Menus_Good_IncludesEntryURLDefaultsLabelAndRunningFlag(t *core.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	dir := tmp + "/Lethean/conf/plugins/opencode"
	core.RequireTrue(t, core.MkdirAll(dir, 0o755).OK)
	m := Manifest{
		Code: "opencode", Name: "OpenCode", Binary: "bin/opencode", Namespace: "opencode",
		Menu: &Menu{Icon: "fa-robot"}, // Label deliberately empty
		UI:   &UI{Entrypoint: "/index.html"},
	}
	core.RequireTrue(t, saveManifest(dir, m).OK)

	svc := newTestService(t, core.New())
	svc.state["opencode"] = &pluginState{state: "running"}

	r := svc.Menus()
	core.RequireTrue(t, r.OK)
	entries := r.Value.([]MenuEntry)
	core.RequireTrue(t, len(entries) == 1)
	core.AssertEqual(t, "opencode", entries[0].Code)
	core.AssertEqual(t, "OpenCode", entries[0].Label, "falls back to manifest Name when Menu.Label is empty")
	core.AssertEqual(t, "plugin-opencode", entries[0].Surface)
	core.AssertEqual(t, "/v1/api/plugin/opencode/index.html", entries[0].EntryURL)
	core.AssertTrue(t, entries[0].Running)
}

func TestMenus_Service_Menus_Ugly_ExplicitLabelWinsAndNotRunning(t *core.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	dir := tmp + "/Lethean/conf/plugins/opencode"
	core.RequireTrue(t, core.MkdirAll(dir, 0o755).OK)
	m := Manifest{
		Code: "opencode", Name: "OpenCode", Binary: "bin/opencode", Namespace: "opencode",
		Menu: &Menu{Label: "Custom Label"},
		// No UI block -> EntryURL stays empty.
	}
	core.RequireTrue(t, saveManifest(dir, m).OK)

	svc := newTestService(t, core.New()) // never tracked -> not running

	r := svc.Menus()
	entries := r.Value.([]MenuEntry)
	core.RequireTrue(t, len(entries) == 1)
	core.AssertEqual(t, "Custom Label", entries[0].Label)
	core.AssertEqual(t, "", entries[0].EntryURL)
	core.AssertFalse(t, entries[0].Running)
}
