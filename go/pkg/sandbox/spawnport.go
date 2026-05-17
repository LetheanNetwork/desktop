// SPDX-Licence-Identifier: EUPL-1.2

// SpawnPort — cross-package seam to the sandbox substrate.
//
// The sandbox surface ships TWO tiers in lockstep:
//
//   - Wails-table tier — the uppercase *Service methods (Spawn /
//     SpawnLong / Kill / ListHandles / GetHandle / InstallID) reachable
//     from the renderer. These are SHIM methods: they emit
//     audit.EventSandboxSpawnRejected with `reason="tier_go_only"` and
//     return ErrTierGoOnly. The shape exists because Wails binding
//     generation walks exported methods on the registered service —
//     keeping the names lets the generator produce typed bindings the
//     frontend already imports, with the runtime check intercepting
//     before any substrate work occurs.
//
//   - Substrate tier — the lowercase package-private methods (spawn /
//     spawnLong / kill / listHandles / getHandle / installID) that
//     contain the load-bearing audit + runtime logic. Reachable only
//     from Go callers that hold a SpawnPort obtained via NewSpawnPort.
//
// SpawnPort is the cross-pkg seam pkg/bridge + pkg/marketplace consume
// via the existing lazy `core.ServiceFor[*sandbox.Service](...)`
// resolver pattern, transformed to return SpawnPort via NewSpawnPort.
// Method signatures mirror the historical *Service shape (all return
// core.Result) so consumer call-sites and their `if !r.OK { return r }
// / r.Value.(X)` patterns survive unchanged.
//
// Cerberus #55 ADD-2 / Mantis #1664 Phase B (RFC v1.1 §7A.4) — the
// structural boundary is the lowercase rename + ErrTierGoOnly shim;
// the interface itself is the cross-pkg seam, not the type-safety
// enforcer. Detect stays uppercase on *Service (runtime introspection,
// different threat class — D-2a adjudication).
//
// Shape b'' adjudication — interface lives in pkg/sandbox (NOT
// internal/) so non-`pkg/sandbox/...` callers (pkg/bridge,
// pkg/marketplace) can both import the type AND implement it via the
// closure-adapter returned by NewSpawnPort. See
// `feedback_internal_pkg_blocks_sibling_substrate_pattern.md` for the
// H#216 lesson on why `internal/` is the wrong location for this seam.
//
// Usage example (lazy resolver shape):
//
//	func (s *Service) sandboxPort() sandbox.SpawnPort {
//	    sb, _ := core.ServiceFor[*sandbox.Service](s.core, "sandbox")
//	    if sb == nil { return nil }
//	    return sandbox.NewSpawnPort(sb)
//	}

package sandbox

import (
	core "dappco.re/go"

	"dappco.re/lthn/desktop/pkg/audit"
)

// SpawnPort is the substrate-tier surface for in-Go consumers. Every
// method delegates to the lowercase package-private substrate on
// *Service. Renderer-tier callers reach the same names on *Service
// directly and get the typed ErrTierGoOnly reject instead.
//
// Method shapes mirror the pre-Phase-B *Service surface so existing
// `if !r.OK { return r } / r.Value.(X)` consumer patterns survive
// the transform unchanged.
type SpawnPort interface {
	// Spawn runs a one-shot container. See *Service.spawn (substrate)
	// for the dispatch semantics. Returns SpawnOutput on success.
	Spawn(input SpawnInput) core.Result

	// SpawnLong starts a detached long-running container. See
	// *Service.spawnLong (substrate) for the lifecycle semantics.
	// Returns ContainerHandle on success.
	SpawnLong(input SpawnLongInput) core.Result

	// Kill stops the container for sandboxID and removes its handle.
	// Returns core.Ok(nil) on success.
	Kill(sandboxID string) core.Result

	// ListHandles returns all currently-registered ContainerHandles.
	// Returns []ContainerHandle on success.
	ListHandles() core.Result

	// GetHandle returns the ContainerHandle for sandboxID. Returns
	// ContainerHandle on success.
	GetHandle(sandboxID string) core.Result

	// InstallID returns the persisted per-install identifier, generating
	// on first call. Returns string on success.
	InstallID() core.Result
}

// NewSpawnPort wraps a *Service into a SpawnPort, exposing the
// lowercase substrate methods to Go callers under the historical
// uppercase shape. The returned adapter is cheap (single pointer);
// callers may cache per-call or per-service as they prefer (D-3b
// adjudication — lazy port at callsite).
//
// nil svc returns nil — callers MUST nil-check before invoking the
// port, matching the existing lazy-resolver shape.
//
// Usage example:
//
//	sb, _ := core.ServiceFor[*sandbox.Service](c, "sandbox")
//	port := sandbox.NewSpawnPort(sb)
//	if port == nil { return core.Fail(...) }
//	r := port.Spawn(input)
func NewSpawnPort(svc *Service) SpawnPort {
	if svc == nil {
		return nil
	}
	return spawnPortImpl{svc: svc}
}

// spawnPortImpl is the closure-adapter returned by NewSpawnPort. It
// holds a *Service pointer and forwards each SpawnPort method to the
// lowercase substrate counterpart. Value receiver — adapter has no
// mutable state of its own.
type spawnPortImpl struct {
	svc *Service
}

// Spawn delegates to (*Service).spawn (substrate).
func (p spawnPortImpl) Spawn(input SpawnInput) core.Result {
	return p.svc.spawn(input)
}

// SpawnLong delegates to (*Service).spawnLong (substrate).
func (p spawnPortImpl) SpawnLong(input SpawnLongInput) core.Result {
	return p.svc.spawnLong(input)
}

// Kill delegates to (*Service).kill (substrate).
func (p spawnPortImpl) Kill(sandboxID string) core.Result {
	return p.svc.kill(sandboxID)
}

// ListHandles delegates to (*Service).listHandles (substrate).
func (p spawnPortImpl) ListHandles() core.Result {
	return p.svc.listHandles()
}

// GetHandle delegates to (*Service).getHandle (substrate).
func (p spawnPortImpl) GetHandle(sandboxID string) core.Result {
	return p.svc.getHandle(sandboxID)
}

// InstallID delegates to (*Service).installID (substrate).
func (p spawnPortImpl) InstallID() core.Result {
	return p.svc.installID()
}

// emitTierReject records the Wails-shim TierGoOnly rejection on the
// audit substrate. Dual-emit shape per RFC.tier-auth-substrate §4.7
// (Cerberus #73 F-4 / Mantis #1758):
//
//  1. EventSandboxSpawnRejected (legacy literal) — preserved for
//     backward grep + the existing forensic walkers that key on the
//     sandbox-cluster event name. Meta keeps `reason="tier_go_only"`
//     + `surface=<shim>` per the historical shape.
//  2. EventTierReject (substrate-tier cluster) — joins the 1st
//     substrate-tier audit adopter; audit consumers grep one literal
//     across all tier-reject sites for the forensic trail. Meta-shape
//     mirrors the pkg/auth.Require deny-emit so a downstream consumer
//     sees a uniform row across the substrate. The `op` field uses the
//     surface label so the Wails-shim site is distinguishable from
//     auth.Require call sites.
//
// Centralised so every uppercase shim has identical Meta shape.
//
// Usage example (internal):
//
//	emitTierReject("sandbox.Spawn")
func emitTierReject(surface string) {
	ts := core.Now().UTC().Unix()
	_ = audit.Default().Record(audit.Event{
		Event:   audit.EventSandboxSpawnRejected,
		TS:      ts,
		Scope:   "sandbox",
		Outcome: audit.OutcomeDenied,
		Meta: map[string]any{
			"reason":  "tier_go_only",
			"surface": surface,
		},
	})
	_ = audit.Default().Record(audit.Event{
		Event:   audit.EventTierReject,
		TS:      ts,
		Scope:   "sandbox",
		Outcome: audit.OutcomeDenied,
		Meta: map[string]any{
			"op":             surface,
			"caller_tier":    "renderer",
			"caller_subject": "",
			"caller_source":  "wails",
			"allowed_tiers":  "internal",
			"reason":         "tier_go_only",
		},
	})
}
