// SPDX-Licence-Identifier: EUPL-1.2

// Package container is the lthn-side container-runtime surface.
// Detects which container runtimes are installed on this host
// (Docker, Podman, LinuxKit, QEMU, Apple Container.framework) and
// lists running containers across them.
//
// Ported from core/ide's container_bridge.go + containersbridge.go.
// The MCP-tool indirection there existed for AI-tool dual-mount;
// lthn goes direct because the Wails GUI is the only consumer today.
// Probes shell out via dappco.re/go/process so the user's PATH +
// installed binaries are honoured. v2 may swap to direct imports of
// dappco.re/go/container if/when that lib lands in our go.work.

package container

import (
	"context"
	"time"

	core "dappco.re/go"
	"dappco.re/go/process"
)

// Service owns the container surface. Holds *core.Core for late
// resolution of the process service (boot order).
type Service struct {
	core *core.Core
}

// NewService constructs the container surface against a Core
// container. Wired via application.NewService(container.NewService(c))
// in pkg/desktop/desktop.go.
func NewService(c *core.Core) *Service { return &Service{core: c} }

// proc resolves the process service at call time. Returns nil when
// the service isn't registered.
func (s *Service) proc() *process.Service {
	if s == nil || s.core == nil {
		return nil
	}
	ps, _ := core.ServiceFor[*process.Service](s.core, "process")
	return ps
}

// Runtime describes one container runtime + its capability set.
// Mirrors core/go-container's ContainerRuntime shape.
type Runtime struct {
	Name                string `json:"name"`
	Available           bool   `json:"available"`
	Version             string `json:"version,omitempty"`
	Path                string `json:"path,omitempty"`
	Description         string `json:"description"`
	HasGPU              bool   `json:"has_gpu"`
	HasNetworkIsolation bool   `json:"has_network_isolation"`
	HasVolumeMounts     bool   `json:"has_volume_mounts"`
	HasEncryption       bool   `json:"has_encryption"`
	HardwareIsolated    bool   `json:"hardware_isolated"`
	SubSecondStart      bool   `json:"sub_second_start"`
}

// Container mirrors core/go-container's Container struct.
type Container struct {
	ID      string `json:"id"`
	Name    string `json:"name,omitempty"`
	Image   string `json:"image"`
	Status  string `json:"status"`
	Runtime string `json:"runtime"`
	Created string `json:"created,omitempty"`
}

// quickVersion runs `cmd flag` with a short timeout and returns the
// first line of output. Used by every probe — version capture must
// be cheap and bounded.
func (s *Service) quickVersion(cmd, flag string) string {
	ps := s.proc()
	if ps == nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	r := ps.Run(ctx, cmd, flag)
	if !r.OK {
		return ""
	}
	out, _ := r.Value.(string)
	first := core.SplitN(out, "\n", 2)[0]
	return core.Trim(first)
}

// probeRuntime resolves a runtime by binary name and fills in the
// version + path slots when found. Caller seeds Description +
// capability flags.
func (s *Service) probeRuntime(bin, label, versionFlag string, info Runtime) Runtime {
	if found := (core.App{}).Find(bin, label); found.OK {
		info.Available = true
		if app, ok := found.Value.(*core.App); ok && app != nil {
			info.Path = app.Path
		}
		info.Version = s.quickVersion(bin, versionFlag)
	}
	return info
}

// detectAll probes every known runtime and returns them in a
// stable order (so the UI doesn't shuffle on re-scan).
func (s *Service) detectAll() []Runtime {
	return []Runtime{
		s.probeRuntime("docker", "docker", "--version", Runtime{
			Name:                "docker",
			Description:         "Docker — OCI containers via Docker daemon",
			HasNetworkIsolation: true,
			HasVolumeMounts:     true,
		}),
		s.probeRuntime("podman", "podman", "--version", Runtime{
			Name:                "podman",
			Description:         "Podman — daemonless OCI containers (rootless capable)",
			HasNetworkIsolation: true,
			HasVolumeMounts:     true,
		}),
		s.probeRuntime("linuxkit", "linuxkit", "version", Runtime{
			Name:                "linuxkit",
			Description:         "LinuxKit — image builder for portable Linux VMs",
			HasNetworkIsolation: true,
			HasVolumeMounts:     true,
			HardwareIsolated:    true,
		}),
		s.probeRuntime("qemu-system-x86_64", "QEMU", "--version", Runtime{
			Name:                "qemu",
			Description:         "QEMU — full machine emulator + virtualiser",
			HasGPU:              true,
			HasNetworkIsolation: true,
			HasVolumeMounts:     true,
			HardwareIsolated:    true,
		}),
		s.probeRuntime("container", "Apple Container", "--version", Runtime{
			Name:                "apple",
			Description:         "Apple Native Containers — macOS 15.4+ Container.framework",
			HasNetworkIsolation: true,
			HasVolumeMounts:     true,
			HardwareIsolated:    true,
			HasEncryption:       true,
			SubSecondStart:      true,
		}),
	}
}

// listDocker returns running Docker containers via `docker ps`.
// Empty slice on any failure — partial-host situations are
// expected (daemon stopped, no socket, etc.) and shouldn't error
// the whole panel.
func (s *Service) listDocker(ctx context.Context) []Container {
	ps := s.proc()
	if ps == nil {
		return nil
	}
	c, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	r := ps.Run(c, "docker", "ps", "--format", "{{json .}}", "--no-trunc")
	if !r.OK {
		return nil
	}
	out, _ := r.Value.(string)
	var containers []Container
	for _, line := range core.Split(out, "\n") {
		if core.Trim(line) == "" {
			continue
		}
		var raw struct {
			ID      string `json:"ID"`
			Names   string `json:"Names"`
			Image   string `json:"Image"`
			Status  string `json:"Status"`
			Created string `json:"CreatedAt"`
		}
		if result := core.JSONUnmarshalString(line, &raw); !result.OK {
			continue
		}
		containers = append(containers, Container{
			ID:      shortID(raw.ID, 12),
			Name:    raw.Names,
			Image:   raw.Image,
			Status:  raw.Status,
			Runtime: "docker",
			Created: raw.Created,
		})
	}
	return containers
}

// listPodman returns running Podman containers via `podman ps`.
func (s *Service) listPodman(ctx context.Context) []Container {
	ps := s.proc()
	if ps == nil {
		return nil
	}
	c, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	r := ps.Run(c, "podman", "ps", "--format", "json")
	if !r.OK {
		return nil
	}
	out, _ := r.Value.(string)
	var raws []struct {
		ID     string   `json:"Id"`
		Names  []string `json:"Names"`
		Image  string   `json:"Image"`
		Status string   `json:"State"`
	}
	if result := core.JSONUnmarshalString(out, &raws); !result.OK {
		return nil
	}
	containers := make([]Container, 0, len(raws))
	for _, raw := range raws {
		name := ""
		if len(raw.Names) > 0 {
			name = raw.Names[0]
		}
		containers = append(containers, Container{
			ID:      shortID(raw.ID, 12),
			Name:    name,
			Image:   raw.Image,
			Status:  raw.Status,
			Runtime: "podman",
		})
	}
	return containers
}

// shortID truncates a container/image ID to the conventional 12
// chars so the UI doesn't get crowded by 64-char sha256s.
func shortID(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
