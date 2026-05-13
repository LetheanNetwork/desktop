// SPDX-Licence-Identifier: EUPL-1.2

// Package telemetry samples the local lthn process and surfaces the
// readings as a Reading struct. Drives the watts + memory readouts in
// the tray popover and live-telemetry window.
//
// Today: portable subset — RSS via runtime.MemStats, goroutine + CGO
// counts, uptime. macOS power sampling (Active/Idle wattage) needs
// powermetrics or IOReport; that wires later via an XPC helper. The
// Reading shape is stable so callers don't need to change when power
// figures fill in.
//
// Usage example:
//
//	r := telemetry.Sample()
//	if r.OK { reading := r.Value.(telemetry.Reading); _ = reading }
package telemetry

import (
	"context"
	"runtime"
	"time"

	core "dappco.re/go"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// Reading is a single telemetry sample.
type Reading struct {
	// Process metrics — always available.
	HeapAllocMB   float64 `json:"heap_alloc_mb"`
	HeapSysMB     float64 `json:"heap_sys_mb"`
	StackInUseMB  float64 `json:"stack_in_use_mb"`
	NumGoroutines int     `json:"num_goroutines"`
	NumCgoCalls   int64   `json:"num_cgo_calls"`
	UptimeSeconds float64 `json:"uptime_seconds"`
	NumGC         uint32  `json:"num_gc"`
	LastGCPauseMs float64 `json:"last_gc_pause_ms"`

	// Power metrics — zero today, populated when the XPC helper
	// lands. See RFC.first-release.md §7.
	WattsActive float64 `json:"watts_active"`
	WattsIdle   float64 `json:"watts_idle"`
}

// processStart is the time the binary started. Captured at package
// init so Sample().UptimeSeconds is meaningful across the whole
// process lifetime, not per-call.
var processStart = time.Now()

// Sample takes one telemetry reading from the local process. Cheap
// — runtime.ReadMemStats is microseconds. Safe to call frequently.
//
// Usage example:
//
//	r := telemetry.Sample()
//	if r.OK { _ = r.Value.(Reading) }
func Sample() core.Result {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)

	var lastGCPauseNs uint64
	if ms.NumGC > 0 {
		lastGCPauseNs = ms.PauseNs[(ms.NumGC+255)%256]
	}

	return core.Ok(Reading{
		HeapAllocMB:   bytesToMB(ms.HeapAlloc),
		HeapSysMB:     bytesToMB(ms.HeapSys),
		StackInUseMB:  bytesToMB(ms.StackInuse),
		NumGoroutines: runtime.NumGoroutine(),
		NumCgoCalls:   runtime.NumCgoCall(),
		UptimeSeconds: time.Since(processStart).Seconds(),
		NumGC:         ms.NumGC,
		LastGCPauseMs: float64(lastGCPauseNs) / 1e6,
	})
}

func bytesToMB(b uint64) float64 {
	return float64(b) / 1024.0 / 1024.0
}

// Options configures the telemetry service. Kept for future
// powermetrics/IOReport source selection — today no fields used.
type Options struct {
	// Source selects the platform telemetry backend
	// ("powermetrics" | "ioreport" | "xpc"). Empty = portable subset.
	Source string
}

// Service registers telemetry.sample on the Core action bus so other
// services can poll without importing this package directly.
type Service struct {
	opts Options
}

// NewService constructs the telemetry service.
//
// Usage example:
//
//	t := telemetry.NewService(telemetry.Options{})
//	t.Register(c)
func NewService(opts Options) *Service {
	return &Service{opts: opts}
}

// Register wires telemetry.sample onto the Core action bus.
// Pattern per Mantis #1336 canonical Service.go.
//
// Usage example:
//
//	if r := svc.Register(c); !r.OK {
//		return r
//	}
func (s *Service) Register(c *core.Core) core.Result {
	if c == nil {
		return core.Fail(core.E("telemetry.Register", "core is nil", nil))
	}
	c.Action("telemetry.sample", func(_ core.Context, _ core.Options) core.Result {
		return Sample()
	})
	return core.Ok(nil)
}

// Register constructs a default telemetry Service and wires it into
// the Core container. One-shot canonical entry per Mantis #1336.
//
// Usage example:
//
//	if r := telemetry.Register(c); !r.OK {
//		return r
//	}
func Register(c *core.Core) core.Result {
	return NewService(Options{}).Register(c)
}

// ----- Wails3 Service shape ----------------------------------------
//
// Implements application.Service so Wails generates a TS binding
// at frontend/bindings/dappco.re/lthn/desktop/pkg/telemetry/service.ts.
// The Sample method below is what the WebView calls; the package-level
// Sample()  / Core action above stay for non-WebView callers.

// ServiceName labels the binding namespace exposed to JS.
func (s *Service) ServiceName() string { return "Telemetry" }

// ServiceStartup runs at app boot. No-op today; reserved for the
// powermetrics / IOReport / XPC helper handshake when that lands.
func (s *Service) ServiceStartup(_ context.Context, _ application.ServiceOptions) core.Result {
	return core.Ok(nil)
}

// ServiceShutdown runs at app exit. No-op today.
func (s *Service) ServiceShutdown() core.Result { return core.Ok(nil) }

// CurrentSample returns one process telemetry reading in the
// package-level Sample() value. Named CurrentSample to avoid
// name-collision with the package-level Sample function in
// method-set resolution.
//
// Usage example (from TS):
//
//	import { CurrentSample } from "@desktop/telemetry/service";
//	const r = await CurrentSample();
//	console.log(r.heap_alloc_mb);
func (s *Service) CurrentSample() core.Result {
	r := Sample()
	if !r.OK {
		return core.Fail(core.E("telemetry.Service.CurrentSample", "sample telemetry failed", r.Value.(error)))
	}
	reading, ok := r.Value.(Reading)
	if !ok {
		return core.Fail(core.NewError("telemetry: Sample returned unexpected value type"))
	}
	return core.Ok(reading)
}
