// SPDX-Licence-Identifier: EUPL-1.2

// Package telemetry reports live power consumption and memory usage
// for the lthn process and its loaded models. Drives the watts +
// memory readouts in the tray popover and live-telemetry window.
//
// macOS source options (decision pending — see RFC.first-release.md §7):
//   - `powermetrics` subprocess tail (rich data, needs sudo)
//   - `IOReport` framework (no sudo, less detail)
//   - small XPC helper (best of both, more build complexity)
//
// Usage example:
//
//	c := core.New()
//	t := telemetry.NewService(telemetry.Options{})
//	if r := t.Register(c); !r.OK {
//		return r
//	}
package telemetry

import (
	core "dappco.re/go"
)

// Reading is a single telemetry sample.
type Reading struct {
	WattsActive float64 // package power during inference
	WattsIdle   float64 // package power at rest
	MemoryMB    float64 // RSS of the process
}

// Options configures the telemetry service.
type Options struct {
	// Source selects the platform telemetry backend
	// ("powermetrics" | "ioreport" | "xpc"). Empty = platform default.
	Source string
}

// Service polls the platform telemetry source at a configured cadence
// and emits Reading values to subscribers (runner signals, frontend).
type Service struct {
	opts Options
}

// NewService constructs the telemetry service.
//
// Usage example:
//
//	t := telemetry.NewService(telemetry.Options{Source: "ioreport"})
//	t.Register(c)
func NewService(opts Options) *Service {
	return &Service{opts: opts}
}

// Register wires the telemetry service into the Core container.
// Pattern per Mantis #1336 canonical Service.go.
//
// Usage example:
//
//	if r := svc.Register(c); !r.OK {
//		return r
//	}
func (s *Service) Register(c *core.Core) core.Result {
	// TODO when telemetry source is selected: wire
	//   - telemetry.sample action (one-shot read)
	//   - telemetry.subscribe action (returns a stream)
	//   - signal exports consumed by runner + frontend
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
