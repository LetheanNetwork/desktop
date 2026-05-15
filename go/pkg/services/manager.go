// SPDX-Licence-Identifier: EUPL-1.2

package services

import (
	"runtime"

	core "dappco.re/go"
	"dappco.re/go/process"
)

// labelPrefix is prepended to the entry name to form the OS service
// label (com.lethean.<name>). Per the bundle-id `ai.lthn.desktop`
// memory, the user-facing identifier root is `ai.lthn` —
// "ai.lthn.serve" is the launchctl label.
const labelPrefix = "ai.lthn."

const (
	installLaunchAgentOp       = "services.installLaunchAgent"
	installSystemdUserUnitOp   = "services.installSystemdUserUnit"
	uninstallSystemdUserUnitOp = "services.uninstallSystemdUserUnit"
)

type controller struct {
	entry Entry
	label string
	exe   string
}

// controllerFor returns the native-service controller data for entry,
// configured to run THIS binary with entry.Arguments.
func controllerFor(entry Entry) core.Result {
	args := core.Args()
	if len(args) == 0 {
		return core.Fail(core.E("services.controllerFor", "process argv is empty", nil))
	}
	exe := args[0]
	if r := core.PathAbs(exe); r.OK {
		exe = r.Value.(string)
	}
	if exe == "" {
		return core.Fail(core.E("services.controllerFor", "executable path is empty", nil))
	}
	return core.Ok(&controller{
		entry: entry,
		label: labelPrefix + entry.Name,
		exe:   exe,
	})
}

func (ctl *controller) install() core.Result {
	switch runtime.GOOS {
	case "darwin":
		return ctl.installLaunchAgent()
	case "linux":
		return ctl.installSystemdUserUnit()
	default:
		return unsupportedServiceManager("Install")
	}
}

func (ctl *controller) uninstall() core.Result {
	switch runtime.GOOS {
	case "darwin":
		return ctl.uninstallLaunchAgent()
	case "linux":
		return ctl.uninstallSystemdUserUnit()
	default:
		return unsupportedServiceManager("Uninstall")
	}
}

func (ctl *controller) start() core.Result {
	switch runtime.GOOS {
	case "darwin":
		return ctl.launchctl("kickstart", "-k", ctl.launchctlTarget())
	case "linux":
		return ctl.systemctl("start", ctl.systemdUnitName())
	default:
		return unsupportedServiceManager("Start")
	}
}

func (ctl *controller) stop() core.Result {
	switch runtime.GOOS {
	case "darwin":
		return ctl.launchctl("kill", "TERM", ctl.launchctlTarget())
	case "linux":
		return ctl.systemctl("stop", ctl.systemdUnitName())
	default:
		return unsupportedServiceManager("Stop")
	}
}

func (ctl *controller) restart() core.Result {
	switch runtime.GOOS {
	case "darwin":
		return ctl.launchctl("kickstart", "-k", ctl.launchctlTarget())
	case "linux":
		return ctl.systemctl("restart", ctl.systemdUnitName())
	default:
		return unsupportedServiceManager("Restart")
	}
}

func (ctl *controller) status() core.Result {
	switch runtime.GOOS {
	case "darwin":
		return ctl.launchctlStatus()
	case "linux":
		return ctl.systemdStatus()
	default:
		return unsupportedServiceManager("Status")
	}
}

func unsupportedServiceManager(operation string) core.Result {
	return core.Fail(core.E(
		"services."+operation,
		core.Sprintf("service manager unsupported on %s", runtime.GOOS),
		nil,
	))
}

func (ctl *controller) launchAgentPath() core.Result {
	home := core.UserHomeDir()
	if !home.OK {
		return core.Fail(core.E("services.launchAgentPath", "resolve home directory", home.Value.(error)))
	}
	return core.Ok(core.PathJoin(
		home.Value.(string),
		"Library",
		"LaunchAgents",
		ctl.label+".plist",
	))
}

func (ctl *controller) launchctlDomain() core.Result {
	current := core.UserCurrent()
	if !current.OK {
		return core.Fail(core.E("services.launchctlDomain", "resolve current user", current.Value.(error)))
	}
	user := current.Value.(*core.User)
	return core.Ok(core.Concat("gui/", user.Uid))
}

func (ctl *controller) launchctlTarget() string {
	domain := ctl.launchctlDomain()
	if !domain.OK {
		return ctl.label
	}
	return core.Concat(domain.Value.(string), "/", ctl.label)
}

func (ctl *controller) launchctl(args ...string) core.Result {
	return process.Run(core.Background(), "launchctl", args...)
}

func (ctl *controller) installLaunchAgent() core.Result {
	path := ctl.launchAgentPath()
	if !path.OK {
		return path
	}
	plistPath := path.Value.(string)
	if r := core.MkdirAll(core.PathDir(plistPath), 0o755); !r.OK {
		return core.Fail(core.E(installLaunchAgentOp, "create LaunchAgents directory", r.Value.(error)))
	}
	if r := core.WriteFile(plistPath, []byte(launchAgentPlist(ctl.entry, ctl.label, ctl.exe)), 0o644); !r.OK {
		return core.Fail(core.E(installLaunchAgentOp, "write launch agent plist", r.Value.(error)))
	}
	domain := ctl.launchctlDomain()
	if !domain.OK {
		return domain
	}
	if r := ctl.launchctl("bootstrap", domain.Value.(string), plistPath); !r.OK && !core.Contains(r.Error(), "already") {
		return core.Fail(core.E(installLaunchAgentOp, "bootstrap launch agent", r.Value.(error)))
	}
	return core.Ok(nil)
}

func (ctl *controller) uninstallLaunchAgent() core.Result {
	path := ctl.launchAgentPath()
	if !path.OK {
		return path
	}
	plistPath := path.Value.(string)
	domain := ctl.launchctlDomain()
	if !domain.OK {
		return domain
	}
	if r := ctl.launchctl("bootout", domain.Value.(string), plistPath); !r.OK && !serviceManagerMissing(r) {
		return core.Fail(core.E("services.uninstallLaunchAgent", "bootout launch agent", r.Value.(error)))
	}
	if r := core.Remove(plistPath); !r.OK {
		if err, ok := r.Value.(error); ok && core.IsNotExist(err) {
			return core.Ok(nil)
		}
		return core.Fail(core.E("services.uninstallLaunchAgent", "remove launch agent plist", r.Value.(error)))
	}
	return core.Ok(nil)
}

func (ctl *controller) launchctlStatus() core.Result {
	r := ctl.launchctl("print", ctl.launchctlTarget())
	if r.OK {
		output := r.Value.(string)
		if core.Contains(output, "state = running") || core.Contains(output, "active count =") {
			return core.Ok("running")
		}
		if core.Contains(output, "state = waiting") || core.Contains(output, "state = exited") {
			return core.Ok("stopped")
		}
		return core.Ok("unknown")
	}
	path := ctl.launchAgentPath()
	if !path.OK {
		return path
	}
	stat := core.Stat(path.Value.(string))
	if !stat.OK {
		if err, ok := stat.Value.(error); ok && core.IsNotExist(err) {
			return core.Ok("not_installed")
		}
		return core.Fail(core.E("services.launchctlStatus", "stat launch agent plist", stat.Value.(error)))
	}
	return core.Ok("stopped")
}

func (ctl *controller) systemdUnitDir() core.Result {
	config := core.UserConfigDir()
	if !config.OK {
		return core.Fail(core.E("services.systemdUnitDir", "resolve user config directory", config.Value.(error)))
	}
	return core.Ok(core.PathJoin(config.Value.(string), "systemd", "user"))
}

func (ctl *controller) systemdUnitPath() core.Result {
	dir := ctl.systemdUnitDir()
	if !dir.OK {
		return dir
	}
	return core.Ok(core.PathJoin(dir.Value.(string), ctl.systemdUnitName()))
}

func (ctl *controller) systemdUnitName() string {
	return ctl.label + ".service"
}

func (ctl *controller) systemctl(args ...string) core.Result {
	full := append([]string{"--user"}, args...)
	return process.Run(core.Background(), "systemctl", full...)
}

func (ctl *controller) installSystemdUserUnit() core.Result {
	dir := ctl.systemdUnitDir()
	if !dir.OK {
		return dir
	}
	if r := core.MkdirAll(dir.Value.(string), 0o755); !r.OK {
		return core.Fail(core.E(installSystemdUserUnitOp, "create systemd user directory", r.Value.(error)))
	}
	path := ctl.systemdUnitPath()
	if !path.OK {
		return path
	}
	if r := core.WriteFile(path.Value.(string), []byte(systemdUnit(ctl.entry, ctl.label, ctl.exe)), 0o644); !r.OK {
		return core.Fail(core.E(installSystemdUserUnitOp, "write systemd unit", r.Value.(error)))
	}
	if r := ctl.systemctl("daemon-reload"); !r.OK {
		return core.Fail(core.E(installSystemdUserUnitOp, "reload systemd user units", r.Value.(error)))
	}
	if r := ctl.systemctl("enable", "--now", ctl.systemdUnitName()); !r.OK {
		return core.Fail(core.E(installSystemdUserUnitOp, "enable systemd unit", r.Value.(error)))
	}
	return core.Ok(nil)
}

func (ctl *controller) uninstallSystemdUserUnit() core.Result {
	if r := ctl.systemctl("disable", "--now", ctl.systemdUnitName()); !r.OK && !serviceManagerMissing(r) {
		return core.Fail(core.E(uninstallSystemdUserUnitOp, "disable systemd unit", r.Value.(error)))
	}
	path := ctl.systemdUnitPath()
	if !path.OK {
		return path
	}
	if r := core.Remove(path.Value.(string)); !r.OK {
		if err, ok := r.Value.(error); ok && core.IsNotExist(err) {
			return core.Ok(nil)
		}
		return core.Fail(core.E(uninstallSystemdUserUnitOp, "remove systemd unit", r.Value.(error)))
	}
	if r := ctl.systemctl("daemon-reload"); !r.OK {
		return core.Fail(core.E(uninstallSystemdUserUnitOp, "reload systemd user units", r.Value.(error)))
	}
	return core.Ok(nil)
}

func (ctl *controller) systemdStatus() core.Result {
	r := ctl.systemctl("is-active", ctl.systemdUnitName())
	if r.OK && core.Trim(r.Value.(string)) == "active" {
		return core.Ok("running")
	}
	path := ctl.systemdUnitPath()
	if !path.OK {
		return path
	}
	stat := core.Stat(path.Value.(string))
	if !stat.OK {
		if err, ok := stat.Value.(error); ok && core.IsNotExist(err) {
			return core.Ok("not_installed")
		}
		return core.Fail(core.E("services.systemdStatus", "stat systemd unit", stat.Value.(error)))
	}
	return core.Ok("stopped")
}

func launchAgentPlist(entry Entry, label, exe string) string {
	args := []string{plistString(exe)}
	for _, arg := range entry.Arguments {
		args = append(args, plistString(arg))
	}
	return core.Concat(
		`<?xml version="1.0" encoding="UTF-8"?>`+"\n",
		`<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">`+"\n",
		`<plist version="1.0">`+"\n",
		"<dict>\n",
		"\t<key>Label</key>\n",
		"\t", plistString(label), "\n",
		"\t<key>ProgramArguments</key>\n",
		"\t<array>\n",
		"\t\t", core.Join("\n\t\t", args...), "\n",
		"\t</array>\n",
		"\t<key>RunAtLoad</key>\n",
		"\t<true/>\n",
		"\t<key>KeepAlive</key>\n",
		"\t<true/>\n",
		"</dict>\n",
		"</plist>\n",
	)
}

func plistString(value string) string {
	return core.Concat("<string>", core.HTMLEscape(value), "</string>")
}

func systemdUnit(entry Entry, label, exe string) string {
	args := append([]string{exe}, entry.Arguments...)
	return core.Concat(
		"[Unit]\n",
		"Description=", entry.Description, "\n\n",
		"[Service]\n",
		"Type=simple\n",
		"ExecStart=", core.Join(" ", args...), "\n",
		"Restart=always\n\n",
		"[Install]\n",
		"WantedBy=default.target\n",
		"# Label=", label, "\n",
	)
}

func serviceManagerMissing(result core.Result) bool {
	msg := result.Error()
	return core.Contains(msg, "No such file") ||
		core.Contains(msg, "not found") ||
		core.Contains(msg, "No such process") ||
		core.Contains(msg, "Could not find domain")
}

// Install registers the service with the OS service manager. On
// macOS this writes ~/Library/LaunchAgents/ai.lthn.<name>.plist and
// loads it. On Linux ~/.config/systemd/user/ai.lthn.<name>.service.
// Idempotent at the OS level — calling twice is fine.
//
// Usage example:
//
//	r := services.Install("serve")
func Install(name string) core.Result {
	r := Lookup(name)
	if !r.OK {
		return r
	}
	entry := r.Value.(Entry)
	ctl := controllerFor(entry)
	if !ctl.OK {
		return ctl
	}
	return ctl.Value.(*controller).install()
}

// Uninstall removes the service definition from the OS service
// manager. Stops first if running.
//
// Usage example:
//
//	r := services.Uninstall("serve")
func Uninstall(name string) core.Result {
	r := Lookup(name)
	if !r.OK {
		return r
	}
	entry := r.Value.(Entry)
	ctl := controllerFor(entry)
	if !ctl.OK {
		return ctl
	}
	return ctl.Value.(*controller).uninstall()
}

// Start asks the OS service manager to start the service. The
// service must be installed first.
//
// Usage example:
//
//	r := services.Start("serve")
func Start(name string) core.Result {
	r := Lookup(name)
	if !r.OK {
		return r
	}
	entry := r.Value.(Entry)
	ctl := controllerFor(entry)
	if !ctl.OK {
		return ctl
	}
	return ctl.Value.(*controller).start()
}

// Stop asks the OS service manager to stop the service. No-op when
// already stopped.
//
// Usage example:
//
//	r := services.Stop("serve")
func Stop(name string) core.Result {
	r := Lookup(name)
	if !r.OK {
		return r
	}
	entry := r.Value.(Entry)
	ctl := controllerFor(entry)
	if !ctl.OK {
		return ctl
	}
	return ctl.Value.(*controller).stop()
}

// Restart stops then starts the service. Convenience for the common
// "reload config" path.
//
// Usage example:
//
//	r := services.Restart("serve")
func Restart(name string) core.Result {
	r := Lookup(name)
	if !r.OK {
		return r
	}
	entry := r.Value.(Entry)
	ctl := controllerFor(entry)
	if !ctl.OK {
		return ctl
	}
	return ctl.Value.(*controller).restart()
}

// Status returns the OS service-manager view of the service —
// running / stopped / unknown.
//
// Usage example:
//
//	r := services.Status("serve")
//	if r.OK { state := r.Value.(string); _ = state }
func Status(name string) core.Result {
	r := Lookup(name)
	if !r.OK {
		return r
	}
	entry := r.Value.(Entry)
	ctl := controllerFor(entry)
	if !ctl.OK {
		return ctl
	}
	return ctl.Value.(*controller).status()
}
