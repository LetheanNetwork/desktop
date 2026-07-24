// SPDX-Licence-Identifier: EUPL-1.2

package desktop

import core "dappco.re/go"

func TestRuntimeEventLimiter_Good_StateChangesPass(t *core.T) {
	limiter := newRuntimeEventLimiter()
	core.AssertTrue(t, limiter.allowState("network", map[string]any{"online": true}))
	core.AssertTrue(t, limiter.allowState("network", map[string]any{"online": false}))
}

func TestRuntimeEventLimiter_Bad_DuplicateStateDrops(t *core.T) {
	limiter := newRuntimeEventLimiter()
	payload := map[string]any{"online": true, "type": "wifi"}
	core.AssertTrue(t, limiter.allowState("network", payload))
	core.AssertFalse(t, limiter.allowState("network", payload))
}

func TestRuntimeEventLimiter_Ugly_BatteryThresholds(t *core.T) {
	limiter := newRuntimeEventLimiter()
	core.AssertTrue(t, limiter.allowBattery(map[string]any{
		"level": 0.80, "state": "discharging", "lowPowerMode": false,
	}))
	core.AssertFalse(t, limiter.allowBattery(map[string]any{
		"level": 0.75, "state": "discharging", "lowPowerMode": false,
	}))
	core.AssertTrue(t, limiter.allowBattery(map[string]any{
		"level": 0.70, "state": "discharging", "lowPowerMode": false,
	}))
	core.AssertTrue(t, limiter.allowBattery(map[string]any{
		"level": 0.69, "state": "charging", "lowPowerMode": false,
	}))

	low := newRuntimeEventLimiter()
	core.AssertTrue(t, low.allowBattery(map[string]any{"level": 0.10}))
	core.AssertTrue(t, low.allowBattery(map[string]any{"level": 0.09}))
	core.AssertFalse(t, low.allowBattery(map[string]any{"level": 0.09}))
}
