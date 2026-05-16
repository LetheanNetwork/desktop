// SPDX-Licence-Identifier: EUPL-1.2

// Tests for IsValidID + WithinDir — Cerberus #1486 confused-deputy
// guards. Bad-path coverage enumerates every reject-case in the
// IsValidID doc-comment so a future loosening of the rule trips the
// suite immediately.

package paths_test

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/paths"
)

// --- IsValidID Good ---

func TestIDValidation_IsValidID_Good_SimpleSlug(t *core.T) {
	core.AssertNil(t, paths.IsValidID("ada-penley"), "kebab-case slug should pass")
}

func TestIDValidation_IsValidID_Good_DealLikeID(t *core.T) {
	core.AssertNil(t, paths.IsValidID("202605-DEAL-001"), "deal-style ID should pass")
}

func TestIDValidation_IsValidID_Good_Numeric(t *core.T) {
	core.AssertNil(t, paths.IsValidID("123"), "numeric IDs should pass")
}

func TestIDValidation_IsValidID_Good_MaxLength(t *core.T) {
	id := make([]byte, paths.MaxIDBytes)
	for i := range id {
		id[i] = 'a'
	}
	core.AssertNil(t, paths.IsValidID(string(id)), "exactly 255 bytes should pass")
}

// --- IsValidID Bad — every reject-case enumerated ---

func TestIDValidation_IsValidID_Bad_Empty(t *core.T) {
	core.AssertNotNil(t, paths.IsValidID(""), "empty must reject")
}

func TestIDValidation_IsValidID_Bad_ContainsSlash(t *core.T) {
	core.AssertNotNil(t, paths.IsValidID("foo/bar"), "slash must reject")
}

func TestIDValidation_IsValidID_Bad_ContainsBackslash(t *core.T) {
	core.AssertNotNil(t, paths.IsValidID("foo\\bar"), "backslash must reject")
}

func TestIDValidation_IsValidID_Bad_ContainsDotDot(t *core.T) {
	core.AssertNotNil(t, paths.IsValidID("foo..bar"), "embedded .. must reject")
}

func TestIDValidation_IsValidID_Bad_ParentTraversal(t *core.T) {
	core.AssertNotNil(t, paths.IsValidID("../../wallets/lethean-default"),
		"classic traversal must reject")
}

func TestIDValidation_IsValidID_Bad_ContainsNUL(t *core.T) {
	core.AssertNotNil(t, paths.IsValidID("foo\x00bar"), "NUL byte must reject")
}

func TestIDValidation_IsValidID_Bad_LeadingDot(t *core.T) {
	core.AssertNotNil(t, paths.IsValidID(".hidden"), "leading dot must reject")
}

func TestIDValidation_IsValidID_Bad_DotOnly(t *core.T) {
	core.AssertNotNil(t, paths.IsValidID("."), "single dot must reject")
}

func TestIDValidation_IsValidID_Bad_DotDotOnly(t *core.T) {
	core.AssertNotNil(t, paths.IsValidID(".."), "double dot must reject")
}

func TestIDValidation_IsValidID_Bad_TooLong(t *core.T) {
	id := make([]byte, paths.MaxIDBytes+1)
	for i := range id {
		id[i] = 'a'
	}
	core.AssertNotNil(t, paths.IsValidID(string(id)), "> 255 bytes must reject")
}

// --- WithinDir Good ---

func TestIDValidation_WithinDir_Good_DirectChild(t *core.T) {
	core.AssertTrue(t, paths.WithinDir("/Users/x/Lethean/sales", "/Users/x/Lethean/sales/contacts/ada.md"))
}

func TestIDValidation_WithinDir_Good_BaseEqualsCandidate(t *core.T) {
	core.AssertTrue(t, paths.WithinDir("/Users/x/Lethean", "/Users/x/Lethean"))
}

// --- WithinDir Bad ---

func TestIDValidation_WithinDir_Bad_OutsideBase(t *core.T) {
	core.AssertFalse(t, paths.WithinDir("/Users/x/Lethean/sales", "/Users/x/Lethean/wallets/keystore"))
}

func TestIDValidation_WithinDir_Bad_TraversalEscape(t *core.T) {
	// CleanPath collapses .. so "/a/sales/../wallets" becomes "/a/wallets"
	// which is OUTSIDE /a/sales.
	core.AssertFalse(t, paths.WithinDir("/Users/x/Lethean/sales", "/Users/x/Lethean/sales/../wallets"))
}

func TestIDValidation_WithinDir_Bad_PrefixCollision(t *core.T) {
	// /a/sale must NOT be treated as a child of /a/sales.
	core.AssertFalse(t, paths.WithinDir("/Users/x/Lethean/sales", "/Users/x/Lethean/sales-leaked"))
}

func TestIDValidation_WithinDir_Bad_RelativeCandidate(t *core.T) {
	core.AssertFalse(t, paths.WithinDir("/Users/x/Lethean", "Lethean/contacts"))
}

func TestIDValidation_WithinDir_Bad_EmptyBase(t *core.T) {
	core.AssertFalse(t, paths.WithinDir("", "/Users/x/Lethean"))
}

func TestIDValidation_WithinDir_Bad_EmptyCandidate(t *core.T) {
	core.AssertFalse(t, paths.WithinDir("/Users/x", ""))
}
