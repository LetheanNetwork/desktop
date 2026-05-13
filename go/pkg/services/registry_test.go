// SPDX-Licence-Identifier: EUPL-1.2

// Tests for the OS-service registry. The Registry / Lookup / Names
// surface is pure (no system calls) — those get full unit coverage.
//
// Install / Start / Stop / Uninstall / Restart / Status all delegate
// to kardianos/service, which writes to ~/Library/LaunchAgents on
// macOS and ~/.config/systemd/user/ on Linux. Unit-testing those
// success paths would mutate the developer's real service manager,
// so the tests here only cover:
//   - the Lookup-fails branch (defensive guard on every public verb)
//   - controller construction via Status against a never-installed
//     entry, which exercises the kardianos.Service factory without
//     touching the on-disk state
//
// Full coverage of the success paths belongs to the integration test
// suite that runs against a disposable container or VM where
// modifying launchd / systemd has no blast radius. This file's
// ceiling without that infrastructure is around 50% statement
// coverage — accepted trade-off per Snider's "70% proves the code
// works, anything higher is an agent's job" rule.

package services_test

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/services"
)

func TestServices_Registry_Good_HasKnownEntries(t *core.T) {
	entries := services.Registry()
	core.AssertGreater(t, len(entries), 0, "registry must not be empty")

	byName := map[string]services.Entry{}
	for _, e := range entries {
		byName[e.Name] = e
	}
	core.AssertContains(t, byName, "serve")
	core.AssertContains(t, byName, "tray")
}

func TestServices_Registry_Good_EntryShape(t *core.T) {
	entries := services.Registry()
	for _, e := range entries {
		core.AssertNotEmpty(t, e.Name, "Entry.Name must be set")
		core.AssertNotEmpty(t, e.DisplayName, "Entry.DisplayName must be set")
		core.AssertNotEmpty(t, e.Description, "Entry.Description must be set")
		core.AssertNotEmpty(t, e.Arguments, "Entry.Arguments must be set")
	}
}

func TestServices_Lookup_Good(t *core.T) {
	r := services.Lookup("serve")
	core.AssertTrue(t, r.OK)
	entry := r.Value.(services.Entry)
	core.AssertEqual(t, "serve", entry.Name)
	core.AssertNotEmpty(t, entry.DisplayName)
	core.AssertContains(t, entry.Arguments, "serve")
}

func TestServices_Lookup_Bad_Unknown(t *core.T) {
	r := services.Lookup("not-a-real-service")
	core.AssertFalse(t, r.OK)
	core.AssertNotEmpty(t, r.Error())
}

func TestServices_Names_Good(t *core.T) {
	names := services.Names()
	core.AssertGreater(t, len(names), 0)
	joined := core.Join(",", names...)
	core.AssertContains(t, joined, "serve")
	core.AssertContains(t, joined, "tray")
}

// All lifecycle verbs hit the same Lookup-fails branch when given an
// unknown name. Test once per verb so each public entrypoint's
// defensive path is covered.

func TestServices_Install_Bad_UnknownName(t *core.T) {
	r := services.Install("not-real")
	core.AssertFalse(t, r.OK)
	core.AssertNotEmpty(t, r.Error())
}

func TestServices_Uninstall_Bad_UnknownName(t *core.T) {
	r := services.Uninstall("not-real")
	core.AssertFalse(t, r.OK)
	core.AssertNotEmpty(t, r.Error())
}

func TestServices_Start_Bad_UnknownName(t *core.T) {
	r := services.Start("not-real")
	core.AssertFalse(t, r.OK)
	core.AssertNotEmpty(t, r.Error())
}

func TestServices_Stop_Bad_UnknownName(t *core.T) {
	r := services.Stop("not-real")
	core.AssertFalse(t, r.OK)
	core.AssertNotEmpty(t, r.Error())
}

func TestServices_Restart_Bad_UnknownName(t *core.T) {
	r := services.Restart("not-real")
	core.AssertFalse(t, r.OK)
	core.AssertNotEmpty(t, r.Error())
}

func TestServices_Status_Bad_UnknownName(t *core.T) {
	r := services.Status("not-real")
	core.AssertFalse(t, r.OK)
	core.AssertNotEmpty(t, r.Error())
}

// Status against a known but never-installed service returns
// "not_installed" — exercises the controller-construction path plus
// the nativeservice.ErrNotInstalled branch.
func TestServices_Status_Good_NotInstalled(t *core.T) {
	// Use the canonical 'tray' entry — kardianos can construct the
	// controller from the running test binary, then return
	// not_installed since we haven't called Install.
	r := services.Status("tray")
	if !r.OK {
		// Some CI environments (containers without user launchd /
		// systemd) can't build a controller at all — acceptable; skip.
		t.Skipf("Status check requires a usable service manager: %v", r.Value)
		return
	}
	state := r.Value.(string)
	core.AssertContains(t, []string{"not_installed", "stopped", "unknown"}, state,
		"Status of an uninstalled service should report a known state")
}
