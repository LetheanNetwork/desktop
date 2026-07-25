// SPDX-Licence-Identifier: EUPL-1.2

package main

import (
	core "dappco.re/go"
)

func readMCPWiringFixture(t *core.T, path string) string {
	t.Helper()

	r := core.ReadFile(path)
	core.AssertTrue(t, r.OK)
	contents, ok := r.Value.([]byte)
	core.AssertTrue(t, ok)
	return string(contents)
}

func TestWailsMCPDevWiring_Good_BuildTag(t *core.T) {
	taskfile := readMCPWiringFixture(t, "../../../Taskfile.yml")
	config := readMCPWiringFixture(t, "../../../build/config.yml")

	core.AssertContains(t, taskfile, `EXTRA_TAGS: "mcp"`)
	core.AssertContains(t, config, "- cmd: env EXTRA_TAGS=mcp wails3 task build\n")
	core.AssertContains(t, config, "- cmd: env LTHN_DEV=1 LTHN_WAILS_WS_LISTEN=127.0.0.1:9199 LTHN_WAILS_WS_URL=ws://localhost:9199/wails/ws wails3 task run\n")
}

func TestWailsMCPDevWiring_Bad_LegacyBridgeInactive(t *core.T) {
	app := readMCPWiringFixture(t, "app.go")
	desktop := readMCPWiringFixture(t, "../../pkg/desktop/desktop.go")
	subsystems := readMCPWiringFixture(t, "../../pkg/desktop/subsystems.go")
	mainTS := readMCPWiringFixture(t, "../../../frontend-ng/src/main.ts")

	core.AssertFalse(t, core.Contains(app, `"dappco.re/lthn/desktop/pkg/bridge"`))
	core.AssertFalse(t, core.Contains(app, `core.WithName("bridge"`))
	core.AssertFalse(t, core.Contains(desktop, `"dappco.re/lthn/desktop/pkg/bridge"`))
	core.AssertFalse(t, core.Contains(desktop, "gui.Bind(bridgeSvc)"))
	core.AssertFalse(t, core.Contains(subsystems, "bridge.RegisterWebMCPTools"))
	core.AssertFalse(t, core.Contains(mainTS, "import './wails-bridge'"))
}

func TestWailsMCPDevWiring_Ugly_LegacyBridgeRetained(t *core.T) {
	goBridge := readMCPWiringFixture(t, "../../pkg/bridge/bridge.go")
	webBridge := readMCPWiringFixture(t, "../../../frontend-ng/src/wails-bridge.ts")

	core.AssertContains(t, goBridge, "package bridge")
	core.AssertContains(t, goBridge, "func RegisterService")
	core.AssertContains(t, webBridge, "function installBridge")
	core.AssertContains(t, webBridge, "function installWebMcpBridge")
}
