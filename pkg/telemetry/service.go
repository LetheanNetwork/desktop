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
//	c := core.New()
//	telemetry.Register(c)
package telemetry

// Reading is a single telemetry sample.
type Reading struct {
	WattsActive float64 // package power during inference
	WattsIdle   float64 // package power at rest
	MemoryMB    float64 // RSS of the process
}

// Service polls the platform telemetry source at a configured cadence
// and emits Reading values to subscribers (runner signals, frontend).
type Service struct {
	// Fields:
	//   - source ("powermetrics" | "ioreport" | "xpc")
	//   - sampling interval
	//   - subscriber channels / event bus
}

// New constructs the telemetry service.
//
//	s := telemetry.New()
//	s.Register(c)
func New() *Service { return &Service{} }

// Register wires the telemetry service into the Core container.
// Pattern per Mantis #1336 canonical Service.go.
func (s *Service) Register() error {
	// Wires:
	//   - telemetry.sample action (one-shot read)
	//   - telemetry.subscribe action (returns a stream)
	//   - signal exports consumed by runner + frontend
	return nil
}
