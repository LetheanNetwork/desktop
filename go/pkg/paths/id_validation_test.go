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

// --- IsValidAbsoluteUnderRoot Good ---
//
// Per mail-v2 §3.1 — the helper validates a user-typed absolute path
// is safe to use as a destination under a known root. Test triad
// enumerates every contract clause so a future loosening trips the
// suite immediately.

// withTempHome sets a per-test HOME so tilde-expansion sees a
// predictable path independent of the developer's environment.
func withTempHome(t *core.T) string {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	return tmp
}

func TestIDValidation_IsValidAbsoluteUnderRoot_Good_DirectChild(t *core.T) {
	err := paths.IsValidAbsoluteUnderRoot("/Users/x/Lethean", "/Users/x/Lethean/sales/contacts/ada.md")
	core.AssertNil(t, err, "clean child path should pass")
}

func TestIDValidation_IsValidAbsoluteUnderRoot_Good_CandidateEqualsRoot(t *core.T) {
	// Per contract — equality is accepted; callers usually append a
	// leaf segment afterwards that re-anchors deeper inside.
	err := paths.IsValidAbsoluteUnderRoot("/Users/x/Lethean", "/Users/x/Lethean")
	core.AssertNil(t, err, "candidate == root should pass")
}

func TestIDValidation_IsValidAbsoluteUnderRoot_Good_DeeperThanRoot(t *core.T) {
	err := paths.IsValidAbsoluteUnderRoot(
		"/Users/x/Lethean",
		"/Users/x/Lethean/sales/deals/202605/q2/draft.md",
	)
	core.AssertNil(t, err, "deep child path should pass")
}

func TestIDValidation_IsValidAbsoluteUnderRoot_Good_TildeExpand(t *core.T) {
	home := withTempHome(t)
	// root + candidate both anchored under the test-controlled HOME so
	// tilde expansion resolves predictably.
	err := paths.IsValidAbsoluteUnderRoot(home+"/Lethean", "~/Lethean/sales/contacts/ada.md")
	core.AssertNil(t, err, "tilde-expanded child should pass")
}

func TestIDValidation_IsValidAbsoluteUnderRoot_Good_BareTildeExpandsToHome(t *core.T) {
	home := withTempHome(t)
	err := paths.IsValidAbsoluteUnderRoot(home, "~")
	core.AssertNil(t, err, "bare '~' should expand to HOME (root)")
}

// --- IsValidAbsoluteUnderRoot Bad ---

func TestIDValidation_IsValidAbsoluteUnderRoot_Bad_Empty(t *core.T) {
	err := paths.IsValidAbsoluteUnderRoot("/Users/x/Lethean", "")
	core.AssertNotNil(t, err, "empty candidate must reject")
}

func TestIDValidation_IsValidAbsoluteUnderRoot_Bad_NotAbsolute(t *core.T) {
	err := paths.IsValidAbsoluteUnderRoot("/Users/x/Lethean", "sales/contacts/ada.md")
	core.AssertNotNil(t, err, "relative path must reject")
}

func TestIDValidation_IsValidAbsoluteUnderRoot_Bad_ParentTraversal(t *core.T) {
	// `/a/sales/../wallets` is structurally absolute but Clean would
	// collapse it to `/a/wallets` — which differs byte-wise from the
	// submitted value. We reject on the byte-equality check, NOT after
	// silently rewriting (rewriting would mask the attack signal).
	err := paths.IsValidAbsoluteUnderRoot(
		"/Users/x/Lethean/sales",
		"/Users/x/Lethean/sales/../wallets",
	)
	core.AssertNotNil(t, err, "parent-traversal '..' must reject")
}

func TestIDValidation_IsValidAbsoluteUnderRoot_Bad_DotSegment(t *core.T) {
	err := paths.IsValidAbsoluteUnderRoot(
		"/Users/x/Lethean",
		"/Users/x/Lethean/./sales/ada.md",
	)
	core.AssertNotNil(t, err, "'.' segment must reject (not path-clean)")
}

func TestIDValidation_IsValidAbsoluteUnderRoot_Bad_DoubleSlash(t *core.T) {
	err := paths.IsValidAbsoluteUnderRoot(
		"/Users/x/Lethean",
		"/Users/x/Lethean//sales/ada.md",
	)
	core.AssertNotNil(t, err, "'//' double-slash must reject (Clean collapses → not path-clean)")
}

func TestIDValidation_IsValidAbsoluteUnderRoot_Bad_NULByte(t *core.T) {
	err := paths.IsValidAbsoluteUnderRoot(
		"/Users/x/Lethean",
		"/Users/x/Lethean/sales\x00.md",
	)
	core.AssertNotNil(t, err, "NUL byte must reject")
}

func TestIDValidation_IsValidAbsoluteUnderRoot_Bad_NULByteInRoot(t *core.T) {
	err := paths.IsValidAbsoluteUnderRoot(
		"/Users/x/Lethean\x00",
		"/Users/x/Lethean/sales.md",
	)
	core.AssertNotNil(t, err, "NUL byte in root must reject")
}

func TestIDValidation_IsValidAbsoluteUnderRoot_Bad_RootNotAbsolute(t *core.T) {
	err := paths.IsValidAbsoluteUnderRoot("Lethean", "/Users/x/Lethean/sales.md")
	core.AssertNotNil(t, err, "relative root must reject")
}

func TestIDValidation_IsValidAbsoluteUnderRoot_Bad_RootEmpty(t *core.T) {
	err := paths.IsValidAbsoluteUnderRoot("", "/Users/x/Lethean/sales.md")
	core.AssertNotNil(t, err, "empty root must reject")
}

func TestIDValidation_IsValidAbsoluteUnderRoot_Bad_EscapesRoot(t *core.T) {
	// Path is clean (no `..`) but lives OUTSIDE root after both are
	// cleaned. This is the load-bearing escape case — the user submits
	// a structurally valid absolute path that doesn't live where the
	// service expects.
	err := paths.IsValidAbsoluteUnderRoot(
		"/Users/x/Lethean/sales",
		"/Users/x/Lethean/wallets/keystore",
	)
	core.AssertNotNil(t, err, "clean path outside root must reject")
}

func TestIDValidation_IsValidAbsoluteUnderRoot_Bad_PrefixCollision(t *core.T) {
	// /a/sales must NOT match /a/sales-leaked as a child via byte
	// prefix — equal-or-strict-child discipline matters.
	err := paths.IsValidAbsoluteUnderRoot(
		"/Users/x/Lethean/sales",
		"/Users/x/Lethean/sales-leaked",
	)
	core.AssertNotNil(t, err, "prefix-collision sibling must reject")
}

// --- IsValidAbsoluteUnderRoot Ugly: tilde expansion edge cases ---

func TestIDValidation_IsValidAbsoluteUnderRoot_Ugly_TildeEscapesRoot(t *core.T) {
	// Tilde expands to a path OUTSIDE the configured root — the user
	// asked for "~/Other/path" but the root is "~/Lethean". Even
	// though the post-expand path is absolute + clean, it doesn't
	// live under root.
	home := withTempHome(t)
	err := paths.IsValidAbsoluteUnderRoot(home+"/Lethean", "~/Other/sneaky")
	core.AssertNotNil(t, err, "tilde-expanded path that escapes root must reject")
}

func TestIDValidation_IsValidAbsoluteUnderRoot_Ugly_UserNamedTildeRejects(t *core.T) {
	// We don't support `~alice/...` shape (that'd require resolving
	// arbitrary user directories — surface attack). After our partial
	// tilde-expand, the string still starts with `~alice/...` which
	// isn't absolute → reject.
	_ = withTempHome(t)
	err := paths.IsValidAbsoluteUnderRoot("/Users/x/Lethean", "~alice/foo")
	core.AssertNotNil(t, err, "~alice/... user-named tilde must reject (not supported)")
}

func TestIDValidation_IsValidAbsoluteUnderRoot_Ugly_RootHasTrailingSlash(t *core.T) {
	// Caller-submitted root has a trailing slash that Clean strips.
	// The candidate is bytewise clean. Both should clean down to the
	// same byte sequence + the prefix check should succeed.
	err := paths.IsValidAbsoluteUnderRoot("/Users/x/Lethean/", "/Users/x/Lethean/sales/ada.md")
	core.AssertNil(t, err, "trailing slash in root cleans + accepts child")
}
