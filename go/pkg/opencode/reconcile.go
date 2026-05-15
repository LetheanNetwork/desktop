// SPDX-Licence-Identifier: EUPL-1.2

// Reconcile — on serve boot, sweep the host runtime for surviving
// lthn-opencode-* containers and re-register them in the orm +
// reverse-proxy targets map.
//
// Why this exists: the orm is mounted on an in-memory Memium
// (see cmd/lthn/app.go), so the Sandbox table is wiped every
// time `lthn serve` restarts. The containers, however, live on
// the docker daemon — they survive our restarts cleanly. Without
// Reconcile, the auto-resume path would see "no sandboxes running"
// and spawn a duplicate, leaving the surviving container orphaned.
//
// Per RFC.opencode.md §7 "Restart". The contract is "ensure
// container is running", not "spawn fresh every time".

package opencode

import (

	core "dappco.re/go"
	"dappco.re/go/orm"
)

// Reconcile lists running containers whose name matches the
// lthn-opencode- prefix and re-registers each in the orm + proxy.
// Returns the number of containers recovered.
//
// Safe to call at any point; existing orm records with matching
// ids are overwritten in place (Save is upsert-shaped). Containers
// that don't match the prefix are ignored.
//
// Usage example:
//
//	r := svc.Reconcile()
//	if r.OK { n := r.Value.(int); _ = n }
func (s *Service) Reconcile() core.Result {
	ps := s.proc()
	if ps == nil {
		return core.Fail(core.E("opencode.Reconcile", "process service unavailable", nil))
	}

	// docker ps --filter name=lthn-opencode- --format "{{.Names}}\t{{.Ports}}"
	// gives one line per running container, even when the container
	// is in a partially-bound state. We re-derive host ports from
	// the Ports column rather than trusting the port we allocated
	// last time — host bindings stick across runtime restarts.
	ctx, cancel := core.WithTimeout(core.Background(), 5*core.Second)
	defer cancel()
	runR := ps.Run(ctx, s.runtime(),
		"ps",
		"--filter", "name="+containerPrefix,
		"--format", "{{.Names}}\t{{.Ports}}",
	)
	if !runR.OK {
		return runR
	}
	out, _ := runR.Value.(string)
	authHeader := s.authHeader()

	recovered := 0
	for _, line := range core.Split(core.Trim(out), "\n") {
		line = core.Trim(line)
		if line == "" {
			continue
		}
		parts := core.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			continue
		}
		name := core.Trim(parts[0])
		ports := core.Trim(parts[1])
		if !core.HasPrefix(name, containerPrefix) {
			continue
		}
		id := core.TrimPrefix(name, containerPrefix)
		hostPort := parseHostPort(ports)
		if hostPort == 0 {
			continue
		}

		sb := Sandbox{
			ID:        id,
			Image:     s.image(),
			HostPort:  hostPort,
			Status:    StatusRunning,
			CreatedAt: core.Now(),
		}
		if r := orm.Of[Sandbox](s.Core()).Save(&sb); !r.OK {
			continue
		}
		s.proxy.Set(id, core.Sprintf("http://127.0.0.1:%d", hostPort), authHeader)
		// Auto-subscribe — no-op when no emitter is installed.
		_, _ = s.Subscribe(id)
		recovered++
	}

	if recovered > 0 {
		// Notify subscribers (runner) — the route table needs to
		// pick up the recovered sandboxes' providers.
		s.fireSandboxChange()
	}
	return core.Ok(recovered)
}

// parseHostPort extracts the host-side port from a docker Ports
// column like "127.0.0.1:51823->4096/tcp" or
// "0.0.0.0:51823->4096/tcp, [::]:51823->4096/tcp". Returns 0 if
// the format is unrecognised — caller skips reconciliation for
// that container.
func parseHostPort(ports string) int {
	// Pick the first binding — multiple v4/v6 entries are aliases
	// of the same host port.
	first := core.SplitN(ports, ",", 2)[0]
	// "127.0.0.1:51823->4096/tcp" → "127.0.0.1:51823"
	arrow := core.Index(first, "->")
	if arrow < 0 {
		return 0
	}
	hostSide := first[:arrow]
	// Last colon separates host:port.
	colon := core.LastIndex(hostSide, ":")
	if colon < 0 {
		return 0
	}
	portStr := core.Trim(hostSide[colon+1:])
	pr := core.Atoi(portStr)
	if !pr.OK {
		return 0
	}
	return pr.Value.(int)
}
