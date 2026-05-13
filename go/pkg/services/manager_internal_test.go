// SPDX-Licence-Identifier: EUPL-1.2

package services

import core "dappco.re/go"

const (
	testServeLabel   = "ai.lthn.serve"
	testBinPath      = "/bin/lthn"
	testLocalBinPath = "/usr/local/bin/lthn"
)

func TestManager_launchAgentPlist_Good(t *core.T) {
	entry := Entry{
		Name:        "serve",
		DisplayName: "Lethean Desktop API",
		Description: "API service",
		Arguments:   []string{"serve"},
	}

	plist := launchAgentPlist(entry, testServeLabel, testLocalBinPath)

	core.AssertContains(t, plist, "<string>"+testServeLabel+"</string>")
	core.AssertContains(t, plist, "<string>"+testLocalBinPath+"</string>")
	core.AssertContains(t, plist, "<string>serve</string>")
	core.AssertContains(t, plist, "<key>RunAtLoad</key>")
}

func TestManager_launchAgentPlist_Bad_EscapesValues(t *core.T) {
	entry := Entry{
		Name:      "serve",
		Arguments: []string{`serve&trace`},
	}

	plist := launchAgentPlist(entry, "ai.lthn.<serve>", `/tmp/lthn"bin`)

	core.AssertContains(t, plist, "<string>ai.lthn.&lt;serve&gt;</string>")
	core.AssertContains(t, plist, "<string>/tmp/lthn&#34;bin</string>")
	core.AssertContains(t, plist, "<string>serve&amp;trace</string>")
}

func TestManager_launchAgentPlist_Ugly_EmptyArguments(t *core.T) {
	entry := Entry{Name: "serve"}

	plist := launchAgentPlist(entry, testServeLabel, testBinPath)

	core.AssertContains(t, plist, "<array>")
	core.AssertContains(t, plist, "<string>"+testBinPath+"</string>")
}

func TestManager_systemdUnit_Good(t *core.T) {
	entry := Entry{
		Description: "Lethean Desktop API",
		Arguments:   []string{"serve"},
	}

	unit := systemdUnit(entry, testServeLabel, testLocalBinPath)

	core.AssertContains(t, unit, "Description=Lethean Desktop API")
	core.AssertContains(t, unit, "ExecStart="+testLocalBinPath+" serve")
	core.AssertContains(t, unit, "Restart=always")
}

func TestManager_systemdUnit_Bad_EmptyDescription(t *core.T) {
	entry := Entry{Arguments: []string{"tray"}}

	unit := systemdUnit(entry, "ai.lthn.tray", testBinPath)

	core.AssertContains(t, unit, "Description=")
	core.AssertContains(t, unit, "ExecStart="+testBinPath+" tray")
}

func TestManager_systemdUnit_Ugly_EmptyArguments(t *core.T) {
	entry := Entry{Description: "Lethean"}

	unit := systemdUnit(entry, testServeLabel, testBinPath)

	core.AssertContains(t, unit, "ExecStart="+testBinPath)
	core.AssertContains(t, unit, "# Label="+testServeLabel)
}
