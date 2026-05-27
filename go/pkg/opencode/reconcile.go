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
//
// Adoption gate (Mantis #1599 BLOCK / Cerberus #22): name-prefix
// alone is forgeable — any user on the same docker daemon can spawn
// `docker run --name lthn-opencode-evil ...` and have us front it
// with the per-install bearer header, redirecting upstream proxy
// traffic to attacker-controlled code. Reconcile now gates on the
// "lthn.opencode.install_id" docker label set at spawn-time matching
// THIS install's identifier. Pre-label containers (from earlier
// builds) are left behind with a warning audit event — user repairs
// via `lthn opencode repair` or a manual `docker rm` of orphans.

package opencode

import (

	core "dappco.re/go"
	"dappco.re/go/orm"

	"dappco.re/lthn/desktop/pkg/audit"
)

// EventOpencodeSandboxAdopted is the audit event emitted once per
// container Reconcile successfully adopts. Used by auditors looking
// for surprising adoption events (e.g. the same install_id appearing
// from a process that wasn't our last `lthn serve`).
//
// Meta keys:
//
//	sandbox_id  — the opencode sandbox identifier (post-prefix-trim)
//	container   — the full docker container name
//	install_id  — our install_id (also the label value matched)
//	host_port   — the host-side port mapped to the container
const EventOpencodeSandboxAdopted = "opencode.sandbox.adopted"

// EventOpencodeSandboxAdoptionDenied is the audit event emitted
// once per container Reconcile saw but refused to adopt because the
// install_id label did not match (or was absent). Reason values:
//
//	"missing_label"   — pre-#1599 container with no install_id label
//	"label_mismatch"  — different install_id (sibling install or
//	                    forged container)
//
// Meta keys:
//
//	container         — the full docker container name
//	reason            — one of the values above
//	expected_install  — our install_id (the value Reconcile required)
//	saw_install       — the install_id we found on the container, or
//	                    "" when reason=missing_label
const EventOpencodeSandboxAdoptionDenied = "opencode.sandbox.adoption_denied"

// reconcileLine is the parsed view of one line of the
// docker-ps output Reconcile consumes. Pure data — the
// adoption gate operates on this shape so it can be unit-tested
// without spinning up docker.
type reconcileLine struct {
	Name      string
	Ports     string
	InstallID string // value of the InstallIDLabel; "" when unlabelled
}

// reconcileVerdict is one row's worth of post-gate decision. Pure
// data — produced by classifyReconcile from a docker-ps line + the
// expected install_id; consumed by adoptFromOutput (Save + proxy
// register + audit-emit) and emitDenials (audit-emit only).
//
// Status values:
//
//	"adopt"           — gate passed; row is safe to adopt
//	"missing_label"   — name-prefix match, no install_id label set
//	"label_mismatch"  — name-prefix match, install_id differs
//	"skip"            — name-prefix didn't match (alien container)
//	"bad_port"        — gate would have passed but Ports unparseable
type reconcileVerdict struct {
	Line     reconcileLine
	SandboxID string // post-prefix-trim ID; empty for skip/bad rows
	HostPort  int    // 0 unless Status=="adopt"
	Status    string
}

const (
	verdictAdopt          = "adopt"
	verdictMissingLabel   = "missing_label"
	verdictLabelMismatch  = "label_mismatch"
	verdictSkip           = "skip"
	verdictBadPort        = "bad_port"
)

// classifyReconcile is the pure adoption-gate decision. Given one
// parsed docker-ps line plus the expected install_id, returns the
// verdict (no I/O, no audit, no orm). Centralising the gate here
// keeps the security-critical logic in one place that the test
// matrix in reconcile_test.go can exhaust without docker.
func classifyReconcile(line reconcileLine, expectedInstallID string) reconcileVerdict {
	if !core.HasPrefix(line.Name, containerPrefix) {
		return reconcileVerdict{Line: line, Status: verdictSkip}
	}
	if line.InstallID == "" {
		return reconcileVerdict{Line: line, Status: verdictMissingLabel}
	}
	if line.InstallID != expectedInstallID {
		return reconcileVerdict{Line: line, Status: verdictLabelMismatch}
	}
	id := core.TrimPrefix(line.Name, containerPrefix)
	hostPort := parseHostPort(line.Ports)
	if hostPort == 0 {
		return reconcileVerdict{Line: line, SandboxID: id, Status: verdictBadPort}
	}
	return reconcileVerdict{Line: line, SandboxID: id, HostPort: hostPort, Status: verdictAdopt}
}

// Reconcile lists running containers whose name matches the
// lthn-opencode- prefix and re-registers each in the orm + proxy,
// but only when the container also carries the per-install
// adoption-gate label (Mantis #1599). Returns the number of
// containers recovered.
//
// Safe to call at any point; existing orm records with matching
// ids are overwritten in place (Save is upsert-shaped). Containers
// that don't match the prefix OR don't carry our install_id label
// are ignored — the latter case emits an
// EventOpencodeSandboxAdoptionDenied audit event so divergence is
// observable.
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

	idR := s.InstallID()
	if !idR.OK {
		return idR
	}
	installID, _ := idR.Value.(string)
	if installID == "" {
		return core.Fail(core.E("opencode.Reconcile", "install_id is empty", nil))
	}

	// docker ps --filter name=lthn-opencode- --filter label=<key>=<id>
	// --format "{{.Names}}\t{{.Ports}}\t{{.Label "<key>"}}" gives us
	// the data Reconcile needs in one shot. We pass BOTH a name
	// filter AND a label filter so docker itself rejects the bulk of
	// mismatches; the per-record check below is defence-in-depth in
	// case any future docker version returns rows that don't
	// honour the server-side filter (e.g. via --format injection).
	ctx, cancel := core.WithTimeout(core.Background(), 5*core.Second)
	defer cancel()
	runR := ps.Run(ctx, s.runtime(),
		"ps",
		"--filter", "name="+containerPrefix,
		"--filter", "label="+InstallIDLabel+"="+installID,
		"--format", "{{.Names}}\t{{.Ports}}\t{{.Label \""+InstallIDLabel+"\"}}",
	)
	if !runR.OK {
		return runR
	}
	out, _ := runR.Value.(string)

	// Also list unlabelled / mismatched containers so we can emit a
	// denial audit per-pre-label-era container — observability without
	// risk: we never adopt them, just record they exist. Failure is
	// non-fatal; the primary adoption pass is the load-bearing path.
	deniedCtx, deniedCancel := core.WithTimeout(core.Background(), 5*core.Second)
	defer deniedCancel()
	deniedR := ps.Run(deniedCtx, s.runtime(),
		"ps",
		"--filter", "name="+containerPrefix,
		"--format", "{{.Names}}\t{{.Ports}}\t{{.Label \""+InstallIDLabel+"\"}}",
	)
	deniedOut := ""
	if deniedR.OK {
		deniedOut, _ = deniedR.Value.(string)
	}

	authHeader := s.authHeader()
	recovered := s.adoptFromOutput(out, installID, authHeader)
	s.emitDenials(deniedOut, installID)

	if recovered > 0 {
		// Notify subscribers (runner) — the route table needs to
		// pick up the recovered sandboxes' providers.
		s.fireSandboxChange()
	}
	return core.Ok(recovered)
}

// adoptFromOutput walks the FILTERED docker-ps output, runs the
// pure gate, and adopts every "adopt" verdict. Returns the count
// adopted. Audit emit is best-effort — failures MUST NEVER block
// reconcile.
func (s *Service) adoptFromOutput(out, expectedInstallID, authHeader string) int {
	recovered := 0
	for _, line := range parseReconcileLines(out) {
		v := classifyReconcile(line, expectedInstallID)
		if v.Status != verdictAdopt {
			continue
		}

		sb := Sandbox{
			ID:        v.SandboxID,
			Image:     s.image(),
			HostPort:  v.HostPort,
			Status:    StatusRunning,
			CreatedAt: core.Now(),
		}
		if r := orm.Of[Sandbox](s.Core()).Save(&sb); !r.OK {
			// Sibling-pattern to opencode.Stop.save_failed — a Save
			// failure here means the container exists on the runtime
			// but isn't tracked in the orm, so the GUI won't surface
			// it and the user thinks reconcile lost their sandbox.
			// Log loud so audit / activity can correlate the drift
			// with the failed adoption; the loop continues to give
			// other sandboxes a chance.
			core.Warn("opencode.reconcile.save_failed",
				"id", v.SandboxID, "error", r.Error())
			continue
		}
		s.proxy.Set(v.SandboxID, core.Sprintf("http://127.0.0.1:%d", v.HostPort), authHeader)
		// Auto-subscribe — no-op when no emitter is installed.
		_, _ = s.Subscribe(v.SandboxID)
		recovered++

		_ = audit.Default().Record(audit.Event{
			Event:   EventOpencodeSandboxAdopted,
			TS:      core.Now().UTC().Unix(),
			Scope:   "opencode.sandbox.reconcile",
			Outcome: audit.OutcomeOK,
			Meta: map[string]any{
				"sandbox_id": v.SandboxID,
				"container":  v.Line.Name,
				"install_id": expectedInstallID,
				"host_port":  v.HostPort,
			},
		})
	}
	return recovered
}

// emitDenials walks the UNFILTERED docker-ps output and emits one
// EventOpencodeSandboxAdoptionDenied per container Reconcile saw
// but did not adopt because the install_id label was missing or
// mismatched. Pure side-effect — no return value because audit
// failures are non-fatal (see redact.go discipline).
func (s *Service) emitDenials(out, expectedInstallID string) {
	for _, line := range parseReconcileLines(out) {
		v := classifyReconcile(line, expectedInstallID)
		var reason string
		switch v.Status {
		case verdictMissingLabel:
			reason = "missing_label"
		case verdictLabelMismatch:
			reason = "label_mismatch"
		default:
			continue
		}
		_ = audit.Default().Record(audit.Event{
			Event:   EventOpencodeSandboxAdoptionDenied,
			TS:      core.Now().UTC().Unix(),
			Scope:   "opencode.sandbox.reconcile",
			Outcome: audit.OutcomeDenied,
			Meta: map[string]any{
				"container":        v.Line.Name,
				"reason":           reason,
				"expected_install": expectedInstallID,
				"saw_install":      v.Line.InstallID,
			},
		})
	}
}

// parseReconcileLines turns the raw `docker ps --format` output into
// a slice of reconcileLine. Pure — no I/O, no audit, no orm. Skips
// blank lines and rows that don't have the expected 3 tab-separated
// columns (defensive; a future docker --format change must not crash
// the boot path).
//
// Per-line trimming uses TrimRight(\r) only — a full TrimSpace would
// strip the trailing TAB on rows whose InstallID column is empty
// (e.g. pre-#1599 legacy containers), collapsing the row from 3 tab
// fields to 2 and dropping it. We need those rows: emitDenials must
// see them to emit a missing_label denial event.
func parseReconcileLines(out string) []reconcileLine {
	var lines []reconcileLine
	for _, raw := range core.Split(core.Trim(out), "\n") {
		raw = core.TrimRight(raw, "\r")
		if raw == "" {
			continue
		}
		parts := core.SplitN(raw, "\t", 3)
		if len(parts) != 3 {
			continue
		}
		lines = append(lines, reconcileLine{
			Name:      core.Trim(parts[0]),
			Ports:     core.Trim(parts[1]),
			InstallID: core.Trim(parts[2]),
		})
	}
	return lines
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
