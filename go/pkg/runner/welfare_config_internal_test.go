// SPDX-Licence-Identifier: EUPL-1.2

package runner

import (
	core "dappco.re/go"
	"dappco.re/go/config"
)

func welfareConfigCore(t *core.T, yaml string) *core.Core {
	t.Helper()
	path := core.PathJoin(t.TempDir(), "lthn.yaml")
	core.AssertTrue(t, core.WriteFile(path, []byte(yaml), 0o600).OK)
	c := core.New(core.WithName("config", config.NewConfigServiceWith(config.ServiceOptions{
		Path:      path,
		EnvPrefix: "LTHN_WELFARE_TEST",
	})))
	core.AssertTrue(t, c.ServiceStartup(core.Background(), nil).OK)
	t.Cleanup(func() { _ = c.ServiceShutdown(core.Background()) })
	return c
}

func TestRunner_NewServiceFromCore_WelfareConfig_Good_DefaultOff(t *core.T) {
	s := NewServiceFromCore(welfareConfigCore(t, "routes: {}\n"))

	core.AssertNil(t, s.welfare,
		"runner welfare must be disabled when runner.welfare.enabled is absent")
}

func TestRunner_NewServiceFromCore_WelfareConfig_Bad_ExplicitFalse(t *core.T) {
	s := NewServiceFromCore(welfareConfigCore(t, "runner:\n  welfare:\n    enabled: false\n"))

	core.AssertNil(t, s.welfare,
		"an explicit false setting must leave the welfare gate disabled")
}

func TestRunner_NewServiceFromCore_WelfareConfig_Ugly_ExplicitOptIn(t *core.T) {
	s := NewServiceFromCore(welfareConfigCore(t, "runner:\n  welfare:\n    enabled: true\n"))

	core.AssertNotNil(t, s.welfare,
		"runner.welfare.enabled=true must attach the opt-in welfare gate")
}
