// SPDX-Licence-Identifier: EUPL-1.2

// Package runner adapts go-mlx's inference engine into the lthn
// runtime surface. Exposes start/stop/generate plus signals consumed
// by the tray popover and chat window.
//
// Usage example:
//
//	c := core.New()
//	r := runner.NewService(runner.Options{})
//	if rr := r.Register(c); !rr.OK {
//		return rr
//	}
package runner

import (
	core "dappco.re/go"
)

// Status is the runner's lifecycle state.
type Status string

// Lifecycle states exposed as signals to the frontend.
const (
	StatusIdle       Status = "idle"
	StatusLoading    Status = "loading"
	StatusReady      Status = "ready"
	StatusGenerating Status = "generating"
	StatusError      Status = "error"
)

// PowerReading is a snapshot from the telemetry subsystem, threaded
// through the runner so frontend signals see one source.
type PowerReading struct {
	Active float64 // watts during inference
	Idle   float64 // watts at rest
}

// Options configures the runner service at construction time.
type Options struct {
	// ModelDir is the directory the runner scans for models.
	// Defaults to ~/Lethean/conf/models/ per the no-hidden-bloat rule.
	ModelDir string
}

// Service is the runner subsystem. Wraps go-mlx (Apple Metal),
// go-rocm (HIP — v0.2 trajectory), and the state primitive for
// warm-resume.
type Service struct {
	opts Options
}

// NewService constructs the runner service.
//
// Usage example:
//
//	r := runner.NewService(runner.Options{ModelDir: "~/Lethean/conf/models/"})
//	r.Register(c)
func NewService(opts Options) *Service {
	return &Service{opts: opts}
}

// Register wires the runner service into the Core container. Pattern
// per Mantis #1336 canonical Service.go.
//
// Usage example:
//
//	if r := svc.Register(c); !r.OK {
//		return r
//	}
func (s *Service) Register(c *core.Core) core.Result {
	// TODO when go-mlx is integrable: wire
	//   - runner.start / runner.stop / runner.generate actions
	//   - signal exports (status, tok/s, model, error) consumed by
	//     frontend Lit windows via Wails bindings
	//   - KV state snapshot/restore hooks (warm-resume path)
	return core.Ok(nil)
}

// Register constructs a default runner Service and wires it into the
// Core container. The one-shot canonical entry per Mantis #1336.
//
// Usage example:
//
//	if r := runner.Register(c); !r.OK {
//		return r
//	}
func Register(c *core.Core) core.Result {
	return NewService(Options{}).Register(c)
}
