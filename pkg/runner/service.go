// SPDX-Licence-Identifier: EUPL-1.2

// Package runner adapts go-mlx's inference engine into the lthn
// runtime surface. Exposes Start/Stop/Generate plus signals consumed
// by the tray popover and chat window.
//
//	c := core.New()
//	runner.Register(c)
//
// Signals contract is the one in RFC.first-release.md §4.7:
//
//	type RunnerService interface {
//	    ModelName() Signal[string]
//	    Status() Signal[RunnerStatus] // idle | loading | ready | generating | error
//	    TokensPerSec() Signal[float64]
//	    Power() Signal[PowerReading]  // { active, idle }
//	    MemoryMB() Signal[float64]
//	    AirplaneOk() Signal[bool]
//	    ErrorMessage() Signal[string]
//
//	    Start(ctx) error
//	    Stop(ctx) error
//	    Generate(ctx, prompt, onToken) error
//	}
package runner

// Status is the runner's lifecycle state.
type Status string

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

// Service is the runner subsystem. Wraps go-mlx (Apple Metal),
// go-rocm (HIP — v0.2 trajectory), and eventually go-inference's
// state primitive for warm-resume.
type Service struct {
	// Fields:
	//   - mlx engine handle (dappco.re/go/mlx)
	//   - state primitive handle (dappco.re/go/inference/state)
	//   - active model path + metadata
	//   - active generation context (cancellable)
}

// New constructs the runner service.
//
//	s := runner.New()
//	s.Register(c)
func New() *Service { return &Service{} }

// Register wires the runner service into the Core container.
// Pattern per Mantis #1336 canonical Service.go.
func (s *Service) Register() error {
	// Wires:
	//   - runner.start / runner.stop / runner.generate actions
	//   - signal exports (status, tok/s, model, error) consumed
	//     by frontend Lit windows via Wails bindings
	//   - KV state snapshot/restore hooks (warm-resume path)
	return nil
}
