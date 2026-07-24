// SPDX-Licence-Identifier: EUPL-1.2

package desktop

import (
	"sync"

	core "dappco.re/go"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// runtimeEventLimiter suppresses repeated state snapshots and throttles the
// high-volume battery signal. Pulses such as low-memory deliberately bypass
// it.
type runtimeEventLimiter struct {
	mu sync.Mutex

	lastState map[string]string

	batterySeen  bool
	batteryLevel int
	batteryState string
	batteryLow   bool
}

func newRuntimeEventLimiter() *runtimeEventLimiter {
	return &runtimeEventLimiter{
		lastState:    make(map[string]string),
		batteryLevel: -1,
	}
}

func (limiter *runtimeEventLimiter) allowState(name string, data map[string]any) bool {
	if limiter == nil {
		return true
	}
	key := core.JSONMarshalString(data)
	limiter.mu.Lock()
	defer limiter.mu.Unlock()
	if limiter.lastState[name] == key {
		return false
	}
	limiter.lastState[name] = key
	return true
}

func (limiter *runtimeEventLimiter) allowBattery(data map[string]any) bool {
	if limiter == nil {
		return true
	}
	level := batteryPercentage(data)
	state, _ := data["state"].(string)
	low, _ := data["lowPowerMode"].(bool)

	limiter.mu.Lock()
	defer limiter.mu.Unlock()

	report := !limiter.batterySeen ||
		state != limiter.batteryState ||
		low != limiter.batteryLow
	if !report && level >= 0 {
		if level <= 10 || limiter.batteryLevel <= 10 {
			report = level != limiter.batteryLevel
		} else {
			delta := level - limiter.batteryLevel
			if delta < 0 {
				delta = -delta
			}
			report = delta >= 10
		}
	}
	if report {
		limiter.batterySeen = true
		limiter.batteryLevel = level
		limiter.batteryState = state
		limiter.batteryLow = low
	}
	return report
}

func batteryPercentage(data map[string]any) int {
	level, ok := data["level"].(float64)
	if !ok || level < 0 {
		return -1
	}
	return int(level*100 + 0.5)
}

// registerRuntimeSystemEvents adds the Wails common-event layer that is shared
// by desktop and mobile. CoreGUI continues to own window, tray and application
// chrome events; this bridge adds battery, network, lock and low-memory state
// that only the Wails runtime surfaces.
func registerRuntimeSystemEvents(c *core.Core, app *application.App) {
	if c == nil || app == nil {
		return
	}

	limiter := newRuntimeEventLimiter()
	emit := func(name string, data any) {
		if result := emitCoreEvent(c, name, data); !result.OK {
			core.Warn("desktop runtime event forwarding failed", "event", name, "error", result.Error())
		}
	}
	data := func(event *application.ApplicationEvent) map[string]any {
		if event == nil || event.Context().Data() == nil {
			return map[string]any{}
		}
		return event.Context().Data()
	}
	forwardState := func(name string) func(*application.ApplicationEvent) {
		return func(event *application.ApplicationEvent) {
			payload := data(event)
			if limiter.allowState(name, payload) {
				emit(name, payload)
			}
		}
	}

	app.Event.OnApplicationEvent(events.Common.BatteryChanged, func(event *application.ApplicationEvent) {
		payload := data(event)
		if limiter.allowBattery(payload) {
			emit("lthn:system:battery", payload)
		}
	})
	app.Event.OnApplicationEvent(events.Common.NetworkChanged, forwardState("lthn:system:network"))
	app.Event.OnApplicationEvent(events.Common.ThemeChanged, forwardState("lthn:system:theme"))
	app.Event.OnApplicationEvent(events.Common.LowMemory, func(*application.ApplicationEvent) {
		emit("lthn:system:low-memory", map[string]any{})
	})
	app.Event.OnApplicationEvent(events.Common.ScreenLocked, func(*application.ApplicationEvent) {
		emit("lthn:system:lock", map[string]any{"locked": true})
	})
	app.Event.OnApplicationEvent(events.Common.ScreenUnlocked, func(*application.ApplicationEvent) {
		emit("lthn:system:lock", map[string]any{"locked": false})
	})
}
