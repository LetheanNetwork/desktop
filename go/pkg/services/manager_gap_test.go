// SPDX-Licence-Identifier: EUPL-1.2

// manager_gap_test.go closes the coverage gap left by
// manager_internal_test.go (which only pins the pure plist/unit
// string builders): the OS service-manager install/uninstall/status
// bodies and the top-level Install/Uninstall/Start/Stop/Restart/Status
// dispatchers were entirely untested. launchctl and systemctl are live
// system-management tools with no dry-run mode, so every test here
// drives the osCommandRun seam (see its doc-comment in manager.go)
// rather than shelling out to the developer's real launchd/systemd
// session — no test in this file ever risks registering a real
// LaunchAgent or systemd unit on the host.

package services

import (
	core "dappco.re/go"
)

// fakeCmdCall records one osCommandRun invocation for assertions on
// call order / arguments.
type fakeCmdCall struct {
	command string
	args    []string
}

// newFakeOSCommand returns a fake osCommandRun that replays results in
// call order (repeating the last result once exhausted), plus the
// slice its calls are recorded into.
func newFakeOSCommand(results ...core.Result) (*[]fakeCmdCall, func(core.Context, string, ...string) core.Result) {
	calls := &[]fakeCmdCall{}
	i := 0
	return calls, func(_ core.Context, command string, args ...string) core.Result {
		*calls = append(*calls, fakeCmdCall{command: command, args: args})
		if len(results) == 0 {
			return core.Ok("")
		}
		if i >= len(results) {
			return results[len(results)-1]
		}
		r := results[i]
		i++
		return r
	}
}

// withFakeOSCommand installs fn as osCommandRun for the duration of
// the test and restores the real process.Run on cleanup.
func withFakeOSCommand(t *core.T, fn func(core.Context, string, ...string) core.Result) {
	t.Helper()
	orig := osCommandRun
	osCommandRun = fn
	t.Cleanup(func() { osCommandRun = orig })
}

func testEntry() Entry {
	return Entry{
		Name:        "serve",
		DisplayName: "Lethean Desktop API",
		Description: "test entry",
		Arguments:   []string{"serve"},
	}
}

func testController(t *core.T) *controller {
	t.Helper()
	r := controllerFor(testEntry())
	core.RequireTrue(t, r.OK, r.Error())
	return r.Value.(*controller)
}

// ---- unsupportedServiceManager / serviceManagerMissing -----------------

func TestManager_UnsupportedServiceManager_Bad(t *core.T) {
	r := unsupportedServiceManager("Install")
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "services.Install")
	core.AssertContains(t, r.Error(), "unsupported")
}

func TestManager_ServiceManagerMissing_Good(t *core.T) {
	for _, msg := range []string{
		"No such file or directory",
		"launchctl: not found",
		"No such process",
		"Could not find domain",
	} {
		got := serviceManagerMissing(core.Fail(core.E("test", msg, nil)))
		core.AssertTrue(t, got, "serviceManagerMissing(%q) should be true", msg)
	}
}

func TestManager_ServiceManagerMissing_Bad(t *core.T) {
	got := serviceManagerMissing(core.Fail(core.E("test", "permission denied", nil)))
	core.AssertFalse(t, got, "serviceManagerMissing(permission denied) should be false")
}

// ---- installLaunchAgent -------------------------------------------------

func TestManager_InstallLaunchAgent_Good(t *core.T) {
	t.Setenv("HOME", t.TempDir())
	ctl := testController(t)
	_, fake := newFakeOSCommand(core.Ok("bootstrap succeeded"))
	withFakeOSCommand(t, fake)

	r := ctl.installLaunchAgent()
	core.RequireTrue(t, r.OK, r.Error())

	pathR := ctl.launchAgentPath()
	core.RequireTrue(t, pathR.OK, pathR.Error())
	stat := core.Stat(pathR.Value.(string))
	core.RequireTrue(t, stat.OK, "expected plist file to exist: %s", stat.Error())
}

func TestManager_InstallLaunchAgent_Bad_BootstrapFails(t *core.T) {
	t.Setenv("HOME", t.TempDir())
	ctl := testController(t)
	_, fake := newFakeOSCommand(core.Fail(core.E("test", "boom", nil)))
	withFakeOSCommand(t, fake)

	r := ctl.installLaunchAgent()
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "bootstrap launch agent")
}

func TestManager_InstallLaunchAgent_Ugly_AlreadyBootstrappedTolerated(t *core.T) {
	t.Setenv("HOME", t.TempDir())
	ctl := testController(t)
	_, fake := newFakeOSCommand(core.Fail(core.E("test", "service already bootstrapped", nil)))
	withFakeOSCommand(t, fake)

	r := ctl.installLaunchAgent()
	core.RequireTrue(t, r.OK, "already-bootstrapped must be tolerated, got: %s", r.Error())
}

// ---- uninstallLaunchAgent ------------------------------------------------

func TestManager_UninstallLaunchAgent_Good(t *core.T) {
	t.Setenv("HOME", t.TempDir())
	ctl := testController(t)
	pathR := ctl.launchAgentPath()
	core.RequireTrue(t, pathR.OK, pathR.Error())
	plistPath := pathR.Value.(string)
	core.RequireTrue(t, core.MkdirAll(core.PathDir(plistPath), 0o755).OK)
	core.RequireTrue(t, core.WriteFile(plistPath, []byte("<plist/>"), 0o644).OK)

	_, fake := newFakeOSCommand(core.Ok("bootout succeeded"))
	withFakeOSCommand(t, fake)

	r := ctl.uninstallLaunchAgent()
	core.RequireTrue(t, r.OK, r.Error())
	stat := core.Stat(plistPath)
	core.AssertFalse(t, stat.OK, "expected plist file to be removed")
}

func TestManager_UninstallLaunchAgent_Bad_BootoutFails(t *core.T) {
	t.Setenv("HOME", t.TempDir())
	ctl := testController(t)
	_, fake := newFakeOSCommand(core.Fail(core.E("test", "permission denied", nil)))
	withFakeOSCommand(t, fake)

	r := ctl.uninstallLaunchAgent()
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "bootout launch agent")
}

func TestManager_UninstallLaunchAgent_Ugly_MissingTolerated_NoPlistFile(t *core.T) {
	t.Setenv("HOME", t.TempDir())
	ctl := testController(t)
	// No plist on disk at all — bootout is tolerated as "missing", and
	// Remove sees IsNotExist, which the code treats as success without
	// ever calling through to the missing-file branch below it.
	_, fake := newFakeOSCommand(core.Fail(core.E("test", "No such process", nil)))
	withFakeOSCommand(t, fake)

	r := ctl.uninstallLaunchAgent()
	core.RequireTrue(t, r.OK, "missing service + missing plist must be tolerated, got: %s", r.Error())
}

// ---- launchctlStatus ------------------------------------------------------

func TestManager_LaunchctlStatus_Good_Running(t *core.T) {
	t.Setenv("HOME", t.TempDir())
	ctl := testController(t)
	_, fake := newFakeOSCommand(core.Ok("state = running\nactive count = 1\n"))
	withFakeOSCommand(t, fake)

	r := ctl.launchctlStatus()
	core.RequireTrue(t, r.OK, r.Error())
	core.AssertEqual(t, "running", r.Value)
}

func TestManager_LaunchctlStatus_Good_Stopped(t *core.T) {
	t.Setenv("HOME", t.TempDir())
	ctl := testController(t)
	_, fake := newFakeOSCommand(core.Ok("state = waiting\n"))
	withFakeOSCommand(t, fake)

	r := ctl.launchctlStatus()
	core.RequireTrue(t, r.OK, r.Error())
	core.AssertEqual(t, "stopped", r.Value)
}

func TestManager_LaunchctlStatus_Ugly_UnknownOutput(t *core.T) {
	t.Setenv("HOME", t.TempDir())
	ctl := testController(t)
	_, fake := newFakeOSCommand(core.Ok("something unparsed\n"))
	withFakeOSCommand(t, fake)

	r := ctl.launchctlStatus()
	core.RequireTrue(t, r.OK, r.Error())
	core.AssertEqual(t, "unknown", r.Value)
}

func TestManager_LaunchctlStatus_Bad_NotInstalled(t *core.T) {
	t.Setenv("HOME", t.TempDir())
	ctl := testController(t)
	_, fake := newFakeOSCommand(core.Fail(core.E("test", "Could not find service", nil)))
	withFakeOSCommand(t, fake)

	r := ctl.launchctlStatus()
	core.RequireTrue(t, r.OK, r.Error())
	core.AssertEqual(t, "not_installed", r.Value)
}

func TestManager_LaunchctlStatus_Bad_PrintFailsPlistExists(t *core.T) {
	t.Setenv("HOME", t.TempDir())
	ctl := testController(t)
	pathR := ctl.launchAgentPath()
	core.RequireTrue(t, pathR.OK, pathR.Error())
	plistPath := pathR.Value.(string)
	core.RequireTrue(t, core.MkdirAll(core.PathDir(plistPath), 0o755).OK)
	core.RequireTrue(t, core.WriteFile(plistPath, []byte("<plist/>"), 0o644).OK)

	_, fake := newFakeOSCommand(core.Fail(core.E("test", "print failed", nil)))
	withFakeOSCommand(t, fake)

	r := ctl.launchctlStatus()
	core.RequireTrue(t, r.OK, r.Error())
	core.AssertEqual(t, "stopped", r.Value)
}

// ---- installSystemdUserUnit ----------------------------------------------

func TestManager_InstallSystemdUserUnit_Good(t *core.T) {
	t.Setenv("HOME", t.TempDir())
	ctl := testController(t)
	_, fake := newFakeOSCommand(core.Ok(""), core.Ok(""))
	withFakeOSCommand(t, fake)

	r := ctl.installSystemdUserUnit()
	core.RequireTrue(t, r.OK, r.Error())

	pathR := ctl.systemdUnitPath()
	core.RequireTrue(t, pathR.OK, pathR.Error())
	stat := core.Stat(pathR.Value.(string))
	core.RequireTrue(t, stat.OK, "expected unit file to exist: %s", stat.Error())
}

func TestManager_InstallSystemdUserUnit_Bad_MkdirFails(t *core.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	ctl := testController(t)

	dirR := ctl.systemdUnitDir()
	core.RequireTrue(t, dirR.OK, dirR.Error())
	// Block the "systemd" parent segment with a plain file so MkdirAll
	// on ".../systemd/user" fails (ENOTDIR).
	blocked := core.PathDir(dirR.Value.(string))
	core.RequireTrue(t, core.MkdirAll(core.PathDir(blocked), 0o755).OK)
	core.RequireTrue(t, core.WriteFile(blocked, []byte("blocked"), 0o644).OK)

	r := ctl.installSystemdUserUnit()
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "create systemd user directory")
}

func TestManager_InstallSystemdUserUnit_Bad_WriteFileFails(t *core.T) {
	t.Setenv("HOME", t.TempDir())
	ctl := testController(t)
	dirR := ctl.systemdUnitDir()
	core.RequireTrue(t, dirR.OK, dirR.Error())
	core.RequireTrue(t, core.MkdirAll(dirR.Value.(string), 0o755).OK)
	pathR := ctl.systemdUnitPath()
	core.RequireTrue(t, pathR.OK, pathR.Error())
	// Pre-occupy the unit file's own path with a directory so WriteFile
	// cannot create the file there.
	core.RequireTrue(t, core.MkdirAll(pathR.Value.(string), 0o755).OK)

	r := ctl.installSystemdUserUnit()
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "write systemd unit")
}

func TestManager_InstallSystemdUserUnit_Bad_ReloadFails(t *core.T) {
	t.Setenv("HOME", t.TempDir())
	ctl := testController(t)
	_, fake := newFakeOSCommand(core.Fail(core.E("test", "reload boom", nil)))
	withFakeOSCommand(t, fake)

	r := ctl.installSystemdUserUnit()
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "reload systemd user units")
}

func TestManager_InstallSystemdUserUnit_Bad_EnableFails(t *core.T) {
	t.Setenv("HOME", t.TempDir())
	ctl := testController(t)
	_, fake := newFakeOSCommand(core.Ok(""), core.Fail(core.E("test", "enable boom", nil)))
	withFakeOSCommand(t, fake)

	r := ctl.installSystemdUserUnit()
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "enable systemd unit")
}

// ---- uninstallSystemdUserUnit --------------------------------------------

func TestManager_UninstallSystemdUserUnit_Good(t *core.T) {
	t.Setenv("HOME", t.TempDir())
	ctl := testController(t)
	pathR := ctl.systemdUnitPath()
	core.RequireTrue(t, pathR.OK, pathR.Error())
	core.RequireTrue(t, core.MkdirAll(core.PathDir(pathR.Value.(string)), 0o755).OK)
	core.RequireTrue(t, core.WriteFile(pathR.Value.(string), []byte("[Unit]\n"), 0o644).OK)

	_, fake := newFakeOSCommand(core.Ok(""), core.Ok(""))
	withFakeOSCommand(t, fake)

	r := ctl.uninstallSystemdUserUnit()
	core.RequireTrue(t, r.OK, r.Error())
	stat := core.Stat(pathR.Value.(string))
	core.AssertFalse(t, stat.OK, "expected unit file to be removed")
}

func TestManager_UninstallSystemdUserUnit_Bad_DisableFails(t *core.T) {
	t.Setenv("HOME", t.TempDir())
	ctl := testController(t)
	_, fake := newFakeOSCommand(core.Fail(core.E("test", "permission denied", nil)))
	withFakeOSCommand(t, fake)

	r := ctl.uninstallSystemdUserUnit()
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "disable systemd unit")
}

func TestManager_UninstallSystemdUserUnit_Ugly_MissingTolerated_NoUnitFile(t *core.T) {
	t.Setenv("HOME", t.TempDir())
	ctl := testController(t)
	_, fake := newFakeOSCommand(core.Fail(core.E("test", "unit not found", nil)))
	withFakeOSCommand(t, fake)

	r := ctl.uninstallSystemdUserUnit()
	core.RequireTrue(t, r.OK, "missing unit must be tolerated, got: %s", r.Error())
}

func TestManager_UninstallSystemdUserUnit_Bad_ReloadFails(t *core.T) {
	t.Setenv("HOME", t.TempDir())
	ctl := testController(t)
	pathR := ctl.systemdUnitPath()
	core.RequireTrue(t, pathR.OK, pathR.Error())
	core.RequireTrue(t, core.MkdirAll(core.PathDir(pathR.Value.(string)), 0o755).OK)
	core.RequireTrue(t, core.WriteFile(pathR.Value.(string), []byte("[Unit]\n"), 0o644).OK)

	_, fake := newFakeOSCommand(core.Ok(""), core.Fail(core.E("test", "reload boom", nil)))
	withFakeOSCommand(t, fake)

	r := ctl.uninstallSystemdUserUnit()
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "reload systemd user units")
}

// ---- systemdStatus ---------------------------------------------------------

func TestManager_SystemdStatus_Good_Running(t *core.T) {
	t.Setenv("HOME", t.TempDir())
	ctl := testController(t)
	_, fake := newFakeOSCommand(core.Ok("active\n"))
	withFakeOSCommand(t, fake)

	r := ctl.systemdStatus()
	core.RequireTrue(t, r.OK, r.Error())
	core.AssertEqual(t, "running", r.Value)
}

func TestManager_SystemdStatus_Bad_NotInstalled(t *core.T) {
	t.Setenv("HOME", t.TempDir())
	ctl := testController(t)
	_, fake := newFakeOSCommand(core.Fail(core.E("test", "inactive", nil)))
	withFakeOSCommand(t, fake)

	r := ctl.systemdStatus()
	core.RequireTrue(t, r.OK, r.Error())
	core.AssertEqual(t, "not_installed", r.Value)
}

func TestManager_SystemdStatus_Ugly_InactiveOutputUnitPresent(t *core.T) {
	t.Setenv("HOME", t.TempDir())
	ctl := testController(t)
	pathR := ctl.systemdUnitPath()
	core.RequireTrue(t, pathR.OK, pathR.Error())
	core.RequireTrue(t, core.MkdirAll(core.PathDir(pathR.Value.(string)), 0o755).OK)
	core.RequireTrue(t, core.WriteFile(pathR.Value.(string), []byte("[Unit]\n"), 0o644).OK)

	_, fake := newFakeOSCommand(core.Ok("inactive\n"))
	withFakeOSCommand(t, fake)

	r := ctl.systemdStatus()
	core.RequireTrue(t, r.OK, r.Error())
	core.AssertEqual(t, "stopped", r.Value)
}

// ---- top-level Install/Uninstall/Start/Stop/Restart/Status --------------

func TestInstall_Good(t *core.T) {
	t.Setenv("HOME", t.TempDir())
	_, fake := newFakeOSCommand(core.Ok(""))
	withFakeOSCommand(t, fake)

	r := Install("serve")
	core.RequireTrue(t, r.OK, r.Error())
}

func TestInstall_Bad_UnknownService(t *core.T) {
	r := Install("does-not-exist")
	core.AssertFalse(t, r.OK)
}

func TestUninstall_Good(t *core.T) {
	t.Setenv("HOME", t.TempDir())
	_, fake := newFakeOSCommand(core.Ok(""))
	withFakeOSCommand(t, fake)

	r := Uninstall("serve")
	core.RequireTrue(t, r.OK, r.Error())
}

func TestUninstall_Bad_UnknownService(t *core.T) {
	r := Uninstall("does-not-exist")
	core.AssertFalse(t, r.OK)
}

func TestStart_Good(t *core.T) {
	t.Setenv("HOME", t.TempDir())
	_, fake := newFakeOSCommand(core.Ok(""))
	withFakeOSCommand(t, fake)

	r := Start("serve")
	core.RequireTrue(t, r.OK, r.Error())
}

func TestStart_Bad_UnknownService(t *core.T) {
	r := Start("does-not-exist")
	core.AssertFalse(t, r.OK)
}

func TestStop_Good(t *core.T) {
	t.Setenv("HOME", t.TempDir())
	_, fake := newFakeOSCommand(core.Ok(""))
	withFakeOSCommand(t, fake)

	r := Stop("serve")
	core.RequireTrue(t, r.OK, r.Error())
}

func TestStop_Bad_UnknownService(t *core.T) {
	r := Stop("does-not-exist")
	core.AssertFalse(t, r.OK)
}

func TestRestart_Good(t *core.T) {
	t.Setenv("HOME", t.TempDir())
	_, fake := newFakeOSCommand(core.Ok(""))
	withFakeOSCommand(t, fake)

	r := Restart("serve")
	core.RequireTrue(t, r.OK, r.Error())
}

func TestRestart_Bad_UnknownService(t *core.T) {
	r := Restart("does-not-exist")
	core.AssertFalse(t, r.OK)
}

func TestStatus_Good(t *core.T) {
	t.Setenv("HOME", t.TempDir())
	_, fake := newFakeOSCommand(core.Ok("state = running\n"))
	withFakeOSCommand(t, fake)

	r := Status("serve")
	core.RequireTrue(t, r.OK, r.Error())
	core.AssertEqual(t, "running", r.Value)
}

func TestStatus_Bad_UnknownService(t *core.T) {
	r := Status("does-not-exist")
	core.AssertFalse(t, r.OK)
}
