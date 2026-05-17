// SPDX-Licence-Identifier: EUPL-1.2

// atrest_adapter.go — shared substrate adapter that satisfies
// recordfile.AtomicWriter against paths.AtomicWriteWithVersion +
// core.ReadFile + core.Remove. Extracted from the wave 1/wave 2
// consumer-side copies (sales/deals, incidents, runbooks) per
// Cerberus #44 PRBW F-2 to eliminate 3x duplication before wave 3
// (marketing 4 sub-pkgs) lands.
//
// The adapter is `scope`-parameterised so each consumer's error codes
// stay grep-stable (`<scope>.atrest.atomic_write_failed`,
// `<scope>.atrest.read_failed`, `<scope>.atrest.remove_failed`) — the
// substrate's typed-error contract is preserved verbatim.
//
// Usage example:
//
//	w := recordfile.NewAtRestWriter(recordfile.AtRestDeps[DealRecord]{
//	    ...
//	    Atomic: paths.AtRestAdapter("deals"),
//	})

package paths

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/recordfile"
)

// AtRestAdapter returns a recordfile.AtomicWriter that routes writes
// through paths.AtomicWriteWithVersion under the IfMatch optimistic-
// lock gate, and read / remove through core.ReadFile / core.Remove.
//
// `scope` becomes the prefix on every typed error code the adapter
// emits — e.g. AtRestAdapter("deals") emits "deals.atrest.read_failed"
// for a core.ReadFile failure. Empty `scope` falls back to "atrest".
//
// Usage example:
//
//	dep := recordfile.AtRestDeps[DealRecord]{
//	    Surface: recordfile.SurfaceSalesDeals,
//	    Atomic:  paths.AtRestAdapter("deals"),
//	    ...
//	}
func AtRestAdapter(scope string) recordfile.AtomicWriter {
	if scope == "" {
		scope = "atrest"
	}
	return atrestAdapter{scope: scope}
}

// atrestAdapter is the concrete implementation behind AtRestAdapter.
// Kept package-private so callers compose via the constructor and the
// scope-prefix invariant is preserved.
type atrestAdapter struct {
	scope string
}

// Write routes through AtomicWriteWithVersion. The substrate's
// IfMatch == hex(sha256(prior ciphertext)) matches the primitive's
// IfMatchHash semantics exactly. Empty IfMatch (first-write) means
// the primitive skips the hash check — same as the legacy-upgrade
// path. Mode is honoured at the substrate's 0o600 default which
// matches the surfaces' existing #1487 PR-1 ruling.
func (a atrestAdapter) Write(req recordfile.AtomicWriteRequest) error {
	r := AtomicWriteWithVersion(req.Path, WriteInput{
		Body:        req.Payload,
		IfMatchHash: req.IfMatch,
		IfNotExist:  req.IfNotExist,
	})
	if r.OK {
		return nil
	}
	return core.NewCode(a.scope+".atrest.atomic_write_failed",
		core.Sprintf("AtomicWriteWithVersion(%q): %s", req.Path, r.Error()))
}

// ReadFile is a thin wrapper around core.ReadFile that adapts the
// (Result) → (bytes, error) shape the substrate expects. Missing
// files surface as core-coded errors so the substrate's read-failed
// path stays grep-stable.
func (a atrestAdapter) ReadFile(path string) ([]byte, error) {
	r := core.ReadFile(path)
	if !r.OK {
		return nil, core.NewCode(a.scope+".atrest.read_failed",
			core.Sprintf("ReadFile(%q): %s", path, r.Error()))
	}
	b, _ := r.Value.([]byte)
	return b, nil
}

// Remove is a thin wrapper around core.Remove. Failures are surfaced
// but tolerated by the substrate's lazy-migration legacy-remove path
// (RFC.stage-e-encrypt-at-rest v2 §3.1: warn-on-failure, do NOT abort
// the encrypted write).
func (a atrestAdapter) Remove(path string) error {
	r := core.Remove(path)
	if !r.OK {
		return core.NewCode(a.scope+".atrest.remove_failed",
			core.Sprintf("Remove(%q): %s", path, r.Error()))
	}
	return nil
}
