// SPDX-Licence-Identifier: EUPL-1.2

// Package calibrate is the desktop's adapter to the lthn-mlx profiling
// CLI — the "Calibrate this machine" surface. The whole tuning backend
// already lives in lthn-mlx (`discover` / `bench` / `auto-tune`, every
// one emitting `--json`); this package shells out to it and parses the
// reports so the GUI can offer non-experts an unattended calibration:
// trade ~minutes of machine time for an expert-tuned local-model setup
// they never have to understand. See frontend/APP-CONTRACT.md +
// project_calibration_runner_for_dummies.md.
//
// Direct shell-out via dappco.re/go/process, same shape as pkg/git —
// no HTTP, just `lthn-mlx <subcommand> [--json]` per call. (pkg/lemma
// is the HTTP admin client to a *running* serve; calibration is
// one-shot CLI ops, a different surface.)
//
// Unit 1 (this file) is Discover — the fast, no-model machine probe
// that opens any calibration. Bench (Bencher adapter) + the chained
// auto-tune sweep land in follow-on units.
package calibrate

import (
	core "dappco.re/go"
	"dappco.re/go/process"
)

const (
	discoverOp = "calibrate.Discover"

	// binaryName is the lthn-mlx CLI resolved on PATH when no explicit
	// or known-location binary is found.
	binaryName = "lthn-mlx"

	// envBinaryOverride lets a dev / packaged build point at a specific
	// lthn-mlx binary (e.g. the bundled one inside the .app Resources)
	// without relying on PATH.
	envBinaryOverride = "LTHN_MLX_BIN"
)

// Service owns the calibration surface. Holds a *core.Core ref so it
// can resolve the process service at call time (boot-order safe, same
// pattern as pkg/git).
//
// Usage example:
//
//	svc := calibrate.NewService(c)
//	r := svc.Discover()
type Service struct {
	core *core.Core

	// mu guards job — the single in-flight calibration. A machine
	// calibrates one model at a time; concurrent Calibrate() calls are
	// rejected while a job runs.
	mu  core.Mutex
	job *calibrateJob
}

// NewService constructs the calibration surface against a Core container.
// Wired via application.NewService(calibrate.NewService(c)) in
// pkg/desktop/desktop.go (follow-on unit).
//
// Usage example:
//
//	svc := calibrate.NewService(c)
func NewService(c *core.Core) *Service { return &Service{core: c} }

// Register constructs the calibration service for Core registration.
//
// Usage example:
//
//	core.New(core.WithService(calibrate.Register))
func Register(c *core.Core) core.Result {
	return core.Ok(NewService(c))
}

// resolveBinary resolves the lthn-mlx CLI. Order: explicit override
// (LTHN_MLX_BIN) → known install location (~/Lethean/bin) → dev build
// (~/Code/core/go-mlx/bin) → bare name for PATH resolution by
// process.Run. Bundling the binary into the .app and pointing the
// override at it is the packaged path (Mantis #98).
func resolveBinary() string {
	if p := core.Trim(core.Env(envBinaryOverride)); p != "" {
		return p
	}
	home := core.Env("HOME")
	candidates := []string{
		core.PathJoin(home, "Lethean", "bin", binaryName),
		core.PathJoin(home, "Code", "core", "go-mlx", "bin", binaryName),
	}
	for _, candidate := range candidates {
		if core.Stat(candidate).OK {
			return candidate
		}
	}
	return binaryName
}

// Discover runs `lthn-mlx discover --json` and returns the parsed
// MachineReport — the machine's inference backend, memory envelope,
// and supported capabilities. No model is loaded; the probe is fast
// (~1s). This is step one of any calibration: know the hardware before
// tuning to it.
//
// Fails with a clear "process service unavailable" / "lthn-mlx not
// reachable" Result the UI can surface (e.g. "install lthn-mlx to
// calibrate") rather than crashing — the binary may be absent on a
// fresh install before bundling lands.
//
// Usage example:
//
//	r := svc.Discover()
//	if r.OK {
//	    rep := r.Value.(calibrate.MachineReport)
//	    canTune := rep.Supports("runtime.autotune")
//	}
func (s *Service) Discover() core.Result {
	if s == nil || s.core == nil {
		return core.Fail(core.E(discoverOp, "core not bound", nil))
	}
	proc, ok := core.ServiceFor[*process.Service](s.core, "process")
	if !ok || proc == nil {
		return core.Fail(core.E(discoverOp, "process service unavailable", nil))
	}
	r := proc.Run(core.Background(), resolveBinary(), "discover", "--json")
	out, _ := r.Value.(string)
	if !r.OK {
		return core.Fail(core.E(discoverOp, r.Error(), nil))
	}
	report := ParseDiscoverReport(out)
	if !report.OK {
		return report
	}
	return report
}

// ParseDiscoverReport parses the JSON stdout of `lthn-mlx discover
// --json` into a MachineReport. Split from Discover so the parse is
// unit-testable without spawning the binary, and reusable if the report
// ever arrives over a different transport.
//
// Usage example:
//
//	r := calibrate.ParseDiscoverReport(stdout)
//	if r.OK { rep := r.Value.(calibrate.MachineReport); _ = rep }
func ParseDiscoverReport(stdout string) core.Result {
	const op = "calibrate.ParseDiscoverReport"
	if core.Trim(stdout) == "" {
		return core.Fail(core.E(op, "empty discover report", nil))
	}
	var report MachineReport
	if pr := core.JSONUnmarshalString(stdout, &report); !pr.OK {
		return core.Fail(core.E(op, pr.Error(), nil))
	}
	return core.Ok(report)
}
