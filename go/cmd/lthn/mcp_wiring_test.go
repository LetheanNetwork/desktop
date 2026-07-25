// SPDX-Licence-Identifier: EUPL-1.2

package main

import (
	core "dappco.re/go"
	"gopkg.in/yaml.v3"
)

type wailsDevelopmentConfig struct {
	DevMode struct {
		Executes []struct {
			Command string `yaml:"cmd"`
			Type    string `yaml:"type"`
		} `yaml:"executes"`
	} `yaml:"dev_mode"`
}

type wailsTaskfile struct {
	Tasks map[string]struct {
		Commands []string          `yaml:"cmds"`
		Env      map[string]string `yaml:"env"`
	} `yaml:"tasks"`
}

func readMCPWiringFixture(t *core.T, path string) string {
	t.Helper()

	r := core.ReadFile(path)
	core.AssertTrue(t, r.OK)
	contents, ok := r.Value.([]byte)
	core.AssertTrue(t, ok)
	return string(contents)
}

func TestWailsMCPDevWiring_Good_CommandContracts(t *core.T) {
	var config wailsDevelopmentConfig
	core.RequireNoError(t, yaml.Unmarshal(
		[]byte(readMCPWiringFixture(t, "../../../build/config.yml")),
		&config,
	))

	executionTypes := make(map[string]string, len(config.DevMode.Executes))
	for _, execution := range config.DevMode.Executes {
		executionTypes[execution.Command] = execution.Type
	}
	core.AssertEqual(t, "blocking", executionTypes["wails3 task common:dev:build:native"])
	core.AssertEqual(t, "primary", executionTypes["wails3 task common:dev:run:native"])

	var taskfile wailsTaskfile
	core.RequireNoError(t, yaml.Unmarshal(
		[]byte(readMCPWiringFixture(t, "../../../build/Taskfile.yml")),
		&taskfile,
	))

	buildTask, ok := taskfile.Tasks["dev:build:native"]
	core.RequireTrue(t, ok)
	core.AssertEqual(t, []string{"wails3 task build"}, buildTask.Commands)
	core.AssertEqual(t, map[string]string{"EXTRA_TAGS": "mcp"}, buildTask.Env)

	runTask, ok := taskfile.Tasks["dev:run:native"]
	core.RequireTrue(t, ok)
	core.AssertEqual(t, []string{"wails3 task run"}, runTask.Commands)
	core.AssertEqual(t, map[string]string{
		"LTHN_DEV":             "1",
		"LTHN_WAILS_WS_LISTEN": "127.0.0.1:9199",
		"LTHN_WAILS_WS_URL":    "ws://localhost:9199/wails/ws",
	}, runTask.Env)
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
