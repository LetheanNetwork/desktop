// SPDX-Licence-Identifier: EUPL-1.2

package calibrate

import (
	core "dappco.re/go"
	"dappco.re/go/process"
)

// defaultWorkload is the auto-tune profile used when the caller passes
// an empty / unknown workload — the everyday chat path.
const defaultWorkload = "chat"

// validWorkloads is the closed set lthn-mlx auto-tune accepts. Anything
// else normalises to defaultWorkload so a stale UI value can't error the
// sweep.
var validWorkloads = map[string]struct{}{
	"chat":         {},
	"coding":       {},
	"long_context": {},
	"agent_state":  {},
	"throughput":   {},
	"low_latency":  {},
}

// calibrateJob is the single in-flight calibration: the managed
// auto-tune process plus the request that started it.
type calibrateJob struct {
	proc     *process.Process
	workload string
	model    string
	started  core.Time
}

// CalibrateJob is the descriptor returned by Calibrate when a sweep is
// accepted + started. The frontend then polls CalibrateStatus.
type CalibrateJob struct {
	Workload string    `json:"workload"`
	Model    string    `json:"model"`
	Started  core.Time `json:"started"`
}

// CalibrateResult is the outcome parsed from a finished auto-tune sweep:
// which candidate won, its score (as printed), and the profile file the
// next `lthn-mlx serve` auto-applies. Score is the raw printed string —
// display-only; numeric scoring belongs to the bench/joules pass.
type CalibrateResult struct {
	Workload    string `json:"workload"`
	Model       string `json:"model"`
	SelectedID  string `json:"selected_id"`
	Score       string `json:"score"`
	ProfilePath string `json:"profile_path"`
}

// CalibrateProgress is the pollable state of the current calibration.
// Idle when nothing has run; Running while the sweep is in flight (with
// the accumulated stdout as a live progress feed); Done when finished,
// carrying Result on success or Failed+Error on a non-zero exit.
type CalibrateProgress struct {
	Idle     bool             `json:"idle"`
	Running  bool             `json:"running"`
	Done     bool             `json:"done"`
	Failed   bool             `json:"failed,omitempty"`
	Error    string           `json:"error,omitempty"`
	Workload string           `json:"workload,omitempty"`
	Model    string           `json:"model,omitempty"`
	Output   string           `json:"output,omitempty"`
	Result   *CalibrateResult `json:"result,omitempty"`
}

// normaliseWorkload coerces an arbitrary workload string to a valid one,
// defaulting to chat.
func normaliseWorkload(w string) string {
	w = core.Trim(w)
	if _, ok := validWorkloads[w]; ok {
		return w
	}
	return defaultWorkload
}

// Calibrate starts the unattended "Calibrate this machine" sweep:
// `lthn-mlx auto-tune --workload <w> <model>`. The sweep discovers
// candidate configs, benchmarks each on THIS machine, and writes the
// best-scoring profile to ~/Lethean/data/profiles/ (which `serve`
// auto-applies) — trading minutes of unattended machine time for an
// expert-tuned local-model setup the user never has to understand.
//
// Returns immediately with a CalibrateJob; the sweep runs in the
// managed process service. Poll CalibrateStatus for progress + result.
// Rejects a second sweep while one is running (one machine, one
// calibration at a time).
//
// Usage example:
//
//	r := svc.Calibrate("chat", "/Users/me/models/lemer-lite")
//	// then poll svc.CalibrateStatus() until Done
func (s *Service) Calibrate(workload, model string) core.Result {
	const op = "calibrate.Calibrate"
	if s == nil || s.core == nil {
		return core.Fail(core.E(op, "core not bound", nil))
	}
	if core.Trim(model) == "" {
		return core.Fail(core.E(op, "model path required", nil))
	}
	proc, ok := core.ServiceFor[*process.Service](s.core, "process")
	if !ok || proc == nil {
		return core.Fail(core.E(op, "process service unavailable", nil))
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.job != nil && s.job.proc != nil && s.job.proc.Info().Running {
		return core.Fail(core.E(op, "a calibration is already running", nil))
	}

	wl := normaliseWorkload(workload)
	r := proc.StartWithOptions(core.Background(), process.RunOptions{
		Command: s.binaryPath(),
		Args:    []string{"auto-tune", "--workload", wl, model},
	})
	if !r.OK {
		return core.Fail(core.E(op, r.Error(), nil))
	}
	p, ok := r.Value.(*process.Process)
	if !ok || p == nil {
		return core.Fail(core.E(op, "process service returned non-process", nil))
	}
	s.job = &calibrateJob{proc: p, workload: wl, model: model, started: core.Now()}
	return core.Ok(CalibrateJob{Workload: wl, Model: model, Started: s.job.started})
}

// CalibrateStatus returns the pollable state of the current calibration.
// The frontend calls this on an interval to drive a progress feed during
// the ~minutes sweep, then renders Result when Done.
//
// Usage example:
//
//	r := svc.CalibrateStatus()
//	if r.OK {
//	    p := r.Value.(calibrate.CalibrateProgress)
//	    if p.Done && p.Result != nil { /* show the tuned profile */ }
//	}
func (s *Service) CalibrateStatus() core.Result {
	const op = "calibrate.CalibrateStatus"
	if s == nil || s.core == nil {
		return core.Fail(core.E(op, "core not bound", nil))
	}
	s.mu.Lock()
	job := s.job
	s.mu.Unlock()
	if job == nil || job.proc == nil {
		return core.Ok(CalibrateProgress{Idle: true})
	}
	proc, ok := core.ServiceFor[*process.Service](s.core, "process")
	if !ok || proc == nil {
		return core.Fail(core.E(op, "process service unavailable", nil))
	}

	info := job.proc.Info()
	out, _ := proc.Output(job.proc.ID).Value.(string)
	prog := CalibrateProgress{
		Workload: job.workload,
		Model:    job.model,
		Output:   out,
		Running:  info.Running,
	}
	if info.Running {
		return core.Ok(prog)
	}

	prog.Done = true
	if info.ExitCode != 0 {
		prog.Failed = true
		prog.Error = core.Sprintf("auto-tune exited with code %d", info.ExitCode)
	}
	if res, found := parseCalibrateResult(out); found {
		res.Workload = job.workload
		res.Model = job.model
		prog.Result = &res
	}
	return core.Ok(prog)
}

// parseCalibrateResult extracts the selected candidate + score + saved
// profile path from a finished auto-tune's stdout. The CLI prints:
//
//	selected: <id> (score=0.842)
//	saved:    /Users/.../Lethean/data/profiles/<file>.json
//
// Split out for unit-testability + so a stdout format drift breaks a
// test rather than the live flow. Returns found=false when no "selected:"
// line is present (sweep failed before selection).
func parseCalibrateResult(output string) (CalibrateResult, bool) {
	var res CalibrateResult
	found := false
	for _, raw := range core.Split(output, "\n") {
		line := core.Trim(raw)
		switch {
		case core.HasPrefix(line, "selected:"):
			rest := core.Trim(line[len("selected:"):])
			if idx := core.Index(rest, "(score="); idx >= 0 {
				res.SelectedID = core.Trim(rest[:idx])
				tail := rest[idx+len("(score="):]
				if end := core.Index(tail, ")"); end >= 0 {
					tail = tail[:end]
				}
				res.Score = core.Trim(tail)
			} else {
				res.SelectedID = rest
			}
			found = true
		case core.HasPrefix(line, "saved:"):
			res.ProfilePath = core.Trim(line[len("saved:"):])
		}
	}
	return res, found
}
