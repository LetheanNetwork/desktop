// SPDX-Licence-Identifier: EUPL-1.2

// Typed sentinel errors exported from pkg/sandbox.
//
// ErrTierGoOnly is the categorical rejection the Wails-table shim
// methods return when an in-process renderer-tier caller invokes the
// substrate-port surface (Spawn / SpawnLong / Kill / ListHandles /
// GetHandle / InstallID) directly. The Phase B substrate refactor
// (Mantis #1664) re-shapes Spawn-from-renderer as a typed reject; the
// lowercase substrate stays callable only via the SpawnPort interface
// returned by NewSpawnPort — the in-pkg seam consumers reach via the
// lazy resolver pattern in pkg/bridge + pkg/marketplace.
//
// Cerberus #55 ADD-2 / Mantis #1697 — the constant is the load-bearing
// type-equality target for downstream callers that need to distinguish
// a Wails-table reject from a substrate validation failure:
//
//	if core.Is(err, sandbox.ErrTierGoOnly) { /* renderer attempted spawn */ }

package sandbox

import core "dappco.re/go"

// ErrTierGoOnly is returned by every uppercase Wails-table shim on
// *Service (Spawn / SpawnLong / Kill / ListHandles / GetHandle /
// InstallID) when invoked at the renderer tier. The substrate-tier
// counterparts (lowercase) remain callable from Go via the SpawnPort
// adapter returned by NewSpawnPort.
//
// Usage example:
//
//	r := svc.Spawn(input)
//	if !r.OK {
//	    if cause, ok := r.Value.(error); ok && core.Is(cause, sandbox.ErrTierGoOnly) {
//	        // Renderer-tier reject — surfaced as audit.EventSandboxSpawnRejected.
//	    }
//	}
var ErrTierGoOnly = core.E("sandbox.tier", "spawn not permitted from renderer tier", nil)
