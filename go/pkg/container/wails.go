// SPDX-Licence-Identifier: EUPL-1.2

// Wails surface for the container service. Methods are bound on
// the *Service receiver and exposed to the frontend via wails3
// generate bindings — the TS shape is in
// frontend/bindings/dappco.re/lthn/desktop/pkg/container/service.

package container

import (
	"context"
	"time"

	core "dappco.re/go"
)

// Detection is the shape returned to the frontend by Detect.
// Available is the convenience subset filtered to runtimes the
// host actually has installed; Runtimes is the full list so the
// UI can render cards for everything (greyed out when missing).
type Detection struct {
	Runtimes  []Runtime `json:"runtimes"`
	Available []Runtime `json:"available"`
}

// ListOutput is the shape returned by List — Containers across the
// requested runtime (or all when runtime == "").
type ListOutput struct {
	Containers []Container `json:"containers"`
	Count      int         `json:"count"`
}

// LogsOutput is the shape returned by Logs — last N lines of stdout
// + stderr for the named container.
type LogsOutput struct {
	ID      string `json:"id"`
	Runtime string `json:"runtime"`
	Logs    string `json:"logs"`
}

// Detect probes every known runtime and returns the full list +
// the available subset. Force is accepted for API parity with
// core/ide's cached version, but lthn doesn't cache today — every
// call re-probes.
func (s *Service) Detect(force bool) (Detection, error) {
	_ = force
	runtimes := s.detectAll()
	available := make([]Runtime, 0, len(runtimes))
	for _, r := range runtimes {
		if r.Available {
			available = append(available, r)
		}
	}
	return Detection{Runtimes: runtimes, Available: available}, nil
}

// List enumerates running containers. When runtime is empty the
// service queries every runtime it knows how to talk to; otherwise
// only the named one.
func (s *Service) List(runtime string) (ListOutput, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	out := ListOutput{}
	if runtime == "" || runtime == "docker" {
		out.Containers = append(out.Containers, s.listDocker(ctx)...)
	}
	if runtime == "" || runtime == "podman" {
		out.Containers = append(out.Containers, s.listPodman(ctx)...)
	}
	out.Count = len(out.Containers)
	return out, nil
}

// Logs streams the last N lines for the named container via the
// runtime's native logs command. Bounded — for live tailing the
// frontend should poll. tail<=0 defaults to 200 lines.
func (s *Service) Logs(id, runtime string, tail int) (LogsOutput, error) {
	if id == "" {
		return LogsOutput{}, core.E("container.Logs", "id is required", nil)
	}
	if runtime == "" {
		runtime = "docker"
	}
	if tail <= 0 {
		tail = 200
	}
	ps := s.proc()
	if ps == nil {
		return LogsOutput{}, core.E("container.Logs", "process service unavailable", nil)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tailArg := core.Itoa(tail)
	var r core.Result
	switch runtime {
	case "docker":
		r = ps.Run(ctx, "docker", "logs", "--tail", tailArg, id)
	case "podman":
		r = ps.Run(ctx, "podman", "logs", "--tail", tailArg, id)
	default:
		return LogsOutput{}, core.E("container.Logs", "unsupported runtime: "+runtime, nil)
	}
	if !r.OK {
		return LogsOutput{}, core.E("container.Logs", r.Error(), nil)
	}
	logs, _ := r.Value.(string)
	return LogsOutput{ID: id, Runtime: runtime, Logs: logs}, nil
}
