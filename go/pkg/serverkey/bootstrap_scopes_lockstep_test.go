// SPDX-Licence-Identifier: EUPL-1.2

// Lockstep parity tests between serverkey.bootstrapAllowedScopes and
// server.BootstrapPathScopes — Mantis #1467 path/scope lockstep
// structural prevention. Stage X.B Phase 3 shipped a BootstrapPathScopes
// entry WITHOUT a matching bootstrapAllowedScopes entry, surfacing as a
// framed "Couldn't mint a setup token" mockup body at the gate — the
// mint refused because the scope wasn't in the allow-list, but the
// path WAS in the gated set. These tests fail CI on any future drift.
//
// Both maps live in different packages by design (allow-list is the
// mint-side policy, path-scopes is the verify-side policy), so the
// only enforcement is build-time parity assertion.

package serverkey_test

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/server"
	subject "dappco.re/lthn/desktop/pkg/serverkey"
)

// TestBootstrapScopes_LockstepBetweenServerkeyAndServer_Good asserts
// the KEY sets of bootstrapAllowedScopes (mint-side) and the VALUE
// set of BootstrapPathScopes (verify-side, path→scope) are identical.
// A drift in either direction means a path is reachable without a
// mintable token, OR a scope can be minted but never gates anything.
func TestBootstrapScopes_LockstepBetweenServerkeyAndServer_Good(t *core.T) {
	allowed := subject.ExportedBootstrapAllowedScopes()

	pathScopes := make(map[string]bool, len(server.BootstrapPathScopes))
	for _, scope := range server.BootstrapPathScopes {
		pathScopes[scope] = true
	}

	// allow-list ⊂ path-scopes — every mintable scope MUST gate at
	// least one path. Otherwise the scope is a dead end (mintable but
	// useless), masking config drift behind a successful mint.
	for scope := range allowed {
		core.AssertTrue(t, pathScopes[scope],
			"scope "+scope+" is in serverkey.bootstrapAllowedScopes but no entry in server.BootstrapPathScopes maps to it — drift")
	}

	// path-scopes ⊂ allow-list — every gated path's required scope
	// MUST be mintable. Otherwise the path is reachable only via a
	// token type the mint refuses to produce, surfacing as the
	// "Couldn't mint a setup token" gate failure that motivated #1467.
	for scope := range pathScopes {
		core.AssertTrue(t, allowed[scope],
			"scope "+scope+" is referenced by server.BootstrapPathScopes but missing from serverkey.bootstrapAllowedScopes — drift")
	}

	// Cardinality belt — equal-size sets confirm bijection. Catches
	// the edge case where a scope is duplicated across multiple
	// BootstrapPathScopes paths (which is fine) versus genuinely
	// distinct scope sets (which is not).
	core.AssertEqual(t, len(allowed), len(pathScopes))
}

// TestBootstrapScopes_KnownScopesInBoth_Good pins the Stage B/E.B/X.B
// scope inventory explicitly so a silent removal from EITHER map
// trips CI. New scopes added later extend this list — the rule is
// "every documented scope appears in both", which by induction
// extends the lockstep guarantee to every future addition.
func TestBootstrapScopes_KnownScopesInBoth_Good(t *core.T) {
	knownScopes := []string{
		"account.create",    // Stage B  — initial account write
		"account.unlock",    // Stage E.B — passphrase-unlock path
		"account.provision", // Stage X.B — server-side keygen path
	}

	allowed := subject.ExportedBootstrapAllowedScopes()
	pathScopes := make(map[string]bool, len(server.BootstrapPathScopes))
	for _, scope := range server.BootstrapPathScopes {
		pathScopes[scope] = true
	}

	for _, scope := range knownScopes {
		core.AssertTrue(t, allowed[scope],
			"documented scope "+scope+" MUST be in serverkey.bootstrapAllowedScopes")
		core.AssertTrue(t, pathScopes[scope],
			"documented scope "+scope+" MUST be referenced by at least one server.BootstrapPathScopes entry")
	}
}
