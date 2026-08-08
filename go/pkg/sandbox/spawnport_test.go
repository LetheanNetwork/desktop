// SPDX-Licence-Identifier: EUPL-1.2

// Tests for SpawnPort + ErrTierGoOnly substrate-tier boundary
// (Mantis #1664 Phase B / Cerberus #55 ADD-2 / RFC v1.1 §7A.4).
//
// Two surfaces in lockstep:
//
//   - Wails-table tier — uppercase *Service.X reject with ErrTierGoOnly
//     and emit audit.EventSandboxSpawnRejected with
//     `reason="tier_go_only"`.
//
//   - Substrate tier — sandbox.NewSpawnPort(svc).X reaches the
//     lowercase package-private implementations. Existing call shape
//     (`if !r.OK { return r }` / `r.Value.(X)`) preserved.

package sandbox

import (
	core "dappco.re/go"

	"dappco.re/lthn/desktop/pkg/audit"
)

// TestSpawnPort_TierGoOnly_SpawnRejected_Bad — calling the uppercase
// Wails shim *Service.Spawn returns ErrTierGoOnly + emits exactly one
// audit.EventSandboxSpawnRejected row with `reason="tier_go_only"`
// and the `surface` Meta keyed to the shim that fired.
func TestSpawnPort_TierGoOnly_SpawnRejected_Bad(t *core.T) {
	rec := installAuditRecorder(t)
	svc := newTestService(Options{})

	r := svc.Spawn(SpawnInput{Image: "lthn/dev:latest", Command: "echo"})
	core.AssertFalse(t, r.OK, "shim Spawn MUST reject")
	cause, ok := r.Value.(error)
	core.AssertTrue(t, ok, "shim error MUST surface as error in Result.Value")
	core.AssertTrue(t, core.Is(cause, ErrTierGoOnly),
		"shim error MUST be ErrTierGoOnly")

	rejected := rec.filterByName(audit.EventSandboxSpawnRejected)
	core.AssertEqual(t, 1, len(rejected),
		"shim MUST emit exactly one Rejected row")
	core.AssertEqual(t, audit.OutcomeDenied, rejected[0].Outcome)
	core.AssertEqual(t, "tier_go_only", rejected[0].Meta["reason"])
	core.AssertEqual(t, "sandbox.Spawn", rejected[0].Meta["surface"])
}

// TestSpawnPort_TierGoOnly_SpawnLongRejected_Bad — same as above for
// the SpawnLong Wails shim. The lifecycle pair (Requested + Failed)
// emitted by the substrate spawnLong MUST NOT fire on the reject path.
func TestSpawnPort_TierGoOnly_SpawnLongRejected_Bad(t *core.T) {
	rec := installAuditRecorder(t)
	svc := newTestService(Options{})

	r := svc.SpawnLong(SpawnLongInput{Image: "lthn/dev:latest", Command: "opencode"})
	core.AssertFalse(t, r.OK)
	cause, _ := r.Value.(error)
	core.AssertTrue(t, core.Is(cause, ErrTierGoOnly))

	rejected := rec.filterByName(audit.EventSandboxSpawnRejected)
	core.AssertEqual(t, 1, len(rejected))
	core.AssertEqual(t, "sandbox.SpawnLong", rejected[0].Meta["surface"])

	// Substrate lifecycle MUST NOT fire on the reject path.
	requested := rec.filterByName(audit.EventSandboxLongRequested)
	core.AssertEqual(t, 0, len(requested),
		"substrate Requested must not fire when shim rejects")
}

// TestSpawnPort_TierGoOnly_KillRejected_Bad — Kill shim emits
// SpawnRejected (not KillRequested) and returns ErrTierGoOnly.
func TestSpawnPort_TierGoOnly_KillRejected_Bad(t *core.T) {
	rec := installAuditRecorder(t)
	svc := newTestService(Options{})

	r := svc.Kill("sb-anything")
	core.AssertFalse(t, r.OK)
	cause, _ := r.Value.(error)
	core.AssertTrue(t, core.Is(cause, ErrTierGoOnly))

	rejected := rec.filterByName(audit.EventSandboxSpawnRejected)
	core.AssertEqual(t, 1, len(rejected))
	core.AssertEqual(t, "sandbox.Kill", rejected[0].Meta["surface"])

	killReq := rec.filterByName(audit.EventSandboxKillRequested)
	core.AssertEqual(t, 0, len(killReq),
		"substrate KillRequested must not fire when shim rejects")
}

// TestSpawnPort_TierGoOnly_ListHandlesRejected_Bad — read-only ListHandles
// shim still rejects + emits (the Wails-table boundary is the security
// property; the substrate listHandles is reachable via SpawnPort only).
func TestSpawnPort_TierGoOnly_ListHandlesRejected_Bad(t *core.T) {
	rec := installAuditRecorder(t)
	svc := newTestService(Options{})

	r := svc.ListHandles()
	core.AssertFalse(t, r.OK)
	cause, _ := r.Value.(error)
	core.AssertTrue(t, core.Is(cause, ErrTierGoOnly))

	rejected := rec.filterByName(audit.EventSandboxSpawnRejected)
	core.AssertEqual(t, 1, len(rejected))
	core.AssertEqual(t, "sandbox.ListHandles", rejected[0].Meta["surface"])
}

// TestSpawnPort_TierGoOnly_GetHandleRejected_Bad — GetHandle shim rejects.
func TestSpawnPort_TierGoOnly_GetHandleRejected_Bad(t *core.T) {
	rec := installAuditRecorder(t)
	svc := newTestService(Options{})

	r := svc.GetHandle("sb-anything")
	core.AssertFalse(t, r.OK)
	cause, _ := r.Value.(error)
	core.AssertTrue(t, core.Is(cause, ErrTierGoOnly))

	rejected := rec.filterByName(audit.EventSandboxSpawnRejected)
	core.AssertEqual(t, 1, len(rejected))
	core.AssertEqual(t, "sandbox.GetHandle", rejected[0].Meta["surface"])
}

// TestSpawnPort_TierGoOnly_InstallIDRejected_Bad — InstallID shim rejects.
func TestSpawnPort_TierGoOnly_InstallIDRejected_Bad(t *core.T) {
	rec := installAuditRecorder(t)
	svc := newTestService(Options{})

	r := svc.InstallID()
	core.AssertFalse(t, r.OK)
	cause, _ := r.Value.(error)
	core.AssertTrue(t, core.Is(cause, ErrTierGoOnly))

	rejected := rec.filterByName(audit.EventSandboxSpawnRejected)
	core.AssertEqual(t, 1, len(rejected))
	core.AssertEqual(t, "sandbox.InstallID", rejected[0].Meta["surface"])
}

// TestSpawnPort_SubstrateAccessible_Good — NewSpawnPort exposes the
// lowercase substrate to in-Go consumers. Calling port.Spawn on a
// fast-fail validation input lands the substrate's Requested + Failed
// audit pair (NOT the Wails-shim Rejected), confirming the port
// adapter routes to the lowercase method.
func TestSpawnPort_SubstrateAccessible_Good(t *core.T) {
	rec := installAuditRecorder(t)
	svc := newTestService(Options{})
	port := NewSpawnPort(svc)
	core.AssertNotNil(t, port)

	// Negative-memory validation reject — exercises the substrate
	// without needing a container runtime.
	r := port.Spawn(SpawnInput{Image: "alpine:3.21", Command: "true", Memory: -1})
	core.AssertFalse(t, r.OK)

	requested := rec.filterByName(audit.EventSandboxSpawnRequested)
	core.AssertEqual(t, 1, len(requested),
		"substrate Requested MUST fire on port.Spawn")

	failed := rec.filterByName(audit.EventSandboxSpawnFailed)
	core.AssertEqual(t, 1, len(failed))

	// And no Rejected row — the port bypasses the Wails-table shim.
	rejected := rec.filterByName(audit.EventSandboxSpawnRejected)
	core.AssertEqual(t, 0, len(rejected),
		"port path MUST NOT emit Rejected — shim was bypassed")
}

// TestSpawnPort_NewSpawnPortNilSvc_Good — defensive nil-check matches
// the lazy-resolver guard pattern consumers rely on
// (`if port == nil { ... }`).
func TestSpawnPort_NewSpawnPortNilSvc_Good(t *core.T) {
	port := NewSpawnPort(nil)
	core.AssertEqual(t, nil, port,
		"NewSpawnPort(nil) MUST return nil — consumers guard on this")
}

// TestSpawnPort_AllShimsEmitRejected_Ugly — table-driven assertion that
// EVERY uppercase Wails-table shim on *Service (Spawn / SpawnLong / Kill
// / ListHandles / GetHandle / InstallID) emits exactly one
// audit.EventSandboxSpawnRejected row with `reason="tier_go_only"` and
// `surface="<sandbox.X>"`, AND returns ErrTierGoOnly.
//
// Closure-confirmation for Mantis #1695 / Cerberus #55 ADD-2 — the
// chosen Shape (b) ships SHIMS that emit Rejected (not REMOVALS). A
// removed method would surface as wails3 method-not-found at dispatch
// and pkg/audit would never see the probe (forensic regression). Per
// RFC v1.1 §10.2 the audit-only-shim IS the chosen path. This table
// pins the closure structurally: any future shim removal that bypasses
// the audit emit will fail this test immediately, surfacing the gap
// before it becomes a forensic blind-spot.
//
// Each call gets its own recorder via t.Run subtests — emits do NOT
// accumulate across rows, so "exactly one Rejected row per call"
// holds per row.
func TestSpawnPort_AllShimsEmitRejected_Ugly(t *core.T) {
	type shimRow struct {
		name    string // subtest name; matches the surface label
		invoke  func(svc *Service) core.Result
		surface string // expected Meta["surface"] literal
	}
	rows := []shimRow{
		{
			name: "Spawn",
			invoke: func(svc *Service) core.Result {
				return svc.Spawn(SpawnInput{Image: "lthn/dev:latest", Command: "echo"})
			},
			surface: "sandbox.Spawn",
		},
		{
			name: "SpawnLong",
			invoke: func(svc *Service) core.Result {
				return svc.SpawnLong(SpawnLongInput{Image: "lthn/dev:latest", Command: "opencode"})
			},
			surface: "sandbox.SpawnLong",
		},
		{
			name:    "Kill",
			invoke:  func(svc *Service) core.Result { return svc.Kill("sb-anything") },
			surface: "sandbox.Kill",
		},
		{
			name:    "ListHandles",
			invoke:  func(svc *Service) core.Result { return svc.ListHandles() },
			surface: "sandbox.ListHandles",
		},
		{
			name:    "GetHandle",
			invoke:  func(svc *Service) core.Result { return svc.GetHandle("sb-anything") },
			surface: "sandbox.GetHandle",
		},
		{
			name:    "InstallID",
			invoke:  func(svc *Service) core.Result { return svc.InstallID() },
			surface: "sandbox.InstallID",
		},
	}

	for _, row := range rows {
		row := row // capture
		t.Run(row.name, func(t *core.T) {
			rec := installAuditRecorder(t)
			svc := newTestService(Options{})

			r := row.invoke(svc)
			core.AssertFalse(t, r.OK,
				"shim "+row.surface+" MUST reject (tier_go_only)")

			cause, ok := r.Value.(error)
			core.AssertTrue(t, ok,
				"shim "+row.surface+" error MUST surface as error in Result.Value")
			core.AssertTrue(t, core.Is(cause, ErrTierGoOnly),
				"shim "+row.surface+" error MUST be ErrTierGoOnly")

			rejected := rec.filterByName(audit.EventSandboxSpawnRejected)
			core.AssertEqual(t, 1, len(rejected),
				"shim "+row.surface+" MUST emit exactly one Rejected row")
			core.AssertEqual(t, audit.OutcomeDenied, rejected[0].Outcome)
			core.AssertEqual(t, "sandbox", rejected[0].Scope)
			core.AssertEqual(t, "tier_go_only", rejected[0].Meta["reason"])
			core.AssertEqual(t, row.surface, rejected[0].Meta["surface"])
		})
	}
}

// TestSandbox_SpawnPort_TierReject_UsesSubstrate_Good — the Wails-shim
// rejection path emits BOTH the legacy EventSandboxSpawnRejected row
// (backward grep) AND the substrate-tier audit.EventTierReject row
// (cluster coherence per RFC.tier-auth-substrate §4.7 / Cerberus #73
// F-4 / Mantis #1758). The substrate row carries op=<surface> +
// caller_tier="renderer" + allowed_tiers="internal" so a forensic
// walker can merge sandbox-shim denies with pkg/auth.Require denies
// under one facet.
func TestSandbox_SpawnPort_TierReject_UsesSubstrate_Good(t *core.T) {
	rec := installAuditRecorder(t)
	svc := newTestService(Options{})

	r := svc.Spawn(SpawnInput{Image: "lthn/dev:latest", Command: "echo"})
	core.AssertFalse(t, r.OK, "shim Spawn MUST reject")

	// Backward-grep: legacy row preserved.
	legacy := rec.filterByName(audit.EventSandboxSpawnRejected)
	core.AssertEqual(t, 1, len(legacy),
		"legacy EventSandboxSpawnRejected MUST still fire (backward grep)")
	core.AssertEqual(t, "tier_go_only", legacy[0].Meta["reason"])

	// Substrate-tier row: new EventTierReject emitted in the same
	// helper for cluster coherence.
	substrate := rec.filterByName(audit.EventTierReject)
	core.AssertEqual(t, 1, len(substrate),
		"substrate EventTierReject MUST fire alongside legacy row")
	core.AssertEqual(t, audit.OutcomeDenied, substrate[0].Outcome)
	core.AssertEqual(t, "sandbox", substrate[0].Scope)
	core.AssertEqual(t, "sandbox.Spawn", substrate[0].Meta["op"])
	core.AssertEqual(t, "renderer", substrate[0].Meta["caller_tier"])
	core.AssertEqual(t, "internal", substrate[0].Meta["allowed_tiers"])
	core.AssertEqual(t, "tier_go_only", substrate[0].Meta["reason"])
}
