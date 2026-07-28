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

	const (
		buildCommand = "node scripts/wails-dev-command.mjs build"
		runCommand   = "node scripts/wails-dev-command.mjs run"
	)

	buildCount, runCount := 0, 0
	buildIndex, runIndex := -1, -1
	buildType, runType := "", ""
	for index, execution := range config.DevMode.Executes {
		switch execution.Command {
		case buildCommand:
			buildCount++
			if buildIndex == -1 {
				buildIndex = index
				buildType = execution.Type
			}
		case runCommand:
			runCount++
			if runIndex == -1 {
				runIndex = index
				runType = execution.Type
			}
		}
	}

	core.AssertEqual(t, 1, buildCount)
	core.AssertEqual(t, 1, runCount)
	core.AssertEqual(t, "blocking", buildType)
	core.AssertEqual(t, "primary", runType)
	core.AssertTrue(t, buildIndex < runIndex)
}

func TestWailsMCPDevWiring_Bad_LegacyBridgeInactive(t *core.T) {
	app := readMCPWiringFixture(t, "app.go")
	desktop := readMCPWiringFixture(t, "../../pkg/desktop/desktop.go")
	subsystems := readMCPWiringFixture(t, "../../pkg/desktop/subsystems.go")
	mainTS := readMCPWiringFixture(t, "../../../frontend/src/main.ts")

	core.AssertFalse(t, core.Contains(app, `"dappco.re/lthn/desktop/pkg/bridge"`))
	core.AssertFalse(t, core.Contains(app, `core.WithName("bridge"`))
	core.AssertFalse(t, core.Contains(desktop, `"dappco.re/lthn/desktop/pkg/bridge"`))
	core.AssertFalse(t, core.Contains(desktop, "gui.Bind(bridgeSvc)"))
	core.AssertFalse(t, core.Contains(subsystems, "bridge.RegisterWebMCPTools"))
	core.AssertFalse(t, core.Contains(mainTS, "import './wails-bridge'"))
}

func TestWailsMCPDevWiring_Ugly_LegacyBridgeRetained(t *core.T) {
	goBridge := readMCPWiringFixture(t, "../../pkg/bridge/bridge.go")
	webBridge := readMCPWiringFixture(t, "../../../frontend/src/wails-bridge.ts")

	core.AssertContains(t, goBridge, "package bridge")
	core.AssertContains(t, goBridge, "func RegisterService")
	core.AssertContains(t, webBridge, "function installBridge")
	core.AssertContains(t, webBridge, "function installWebMcpBridge")
}
