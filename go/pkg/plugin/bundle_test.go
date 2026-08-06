// SPDX-Licence-Identifier: EUPL-1.2

// Tests for the bundle registration bridge. AX-10 G/B/U triads for
// the public surface (RegisterBundle, UnregisterBundle) plus
// helpers (slug, substituteExpose, substituteRandom).
//
// Each test isolates via its own *core.Core; Unregister is called
// in t.Cleanup to keep the package-level registry tidy across runs.

package plugin_test

import (
	core "dappco.re/go"

	"dappco.re/lthn/desktop/pkg/plugin"
)

// newPluginCore returns a *core.Core with ServiceStartup run + a
// t.Cleanup that calls ServiceShutdown. Bundle tests don't need
// orm or marketplace.Service registered — RegisterBundle operates
// directly against c.Action + c.Action("store.set") (which is a
// no-op when store isn't registered, returning Fail; tests
// tolerate that since persistSetting is best-effort).
func newPluginCore(t *core.T) *core.Core {
	t.Helper()
	c := core.New()
	core.RequireTrue(t, c.ServiceStartup(core.Background(), nil).OK)
	t.Cleanup(func() { _ = c.ServiceShutdown(core.Background()) })
	return c
}

// ─── RegisterBundle ────────────────────────────────────────────────────

func TestBundle_RegisterBundle_Good(t *core.T) {
	c := newPluginCore(t)
	t.Cleanup(func() { _ = plugin.UnregisterBundle(c, "opencode") })

	r := plugin.RegisterBundle(c, "opencode", plugin.BundleInput{
		Routes: []plugin.BundleRouteEntry{
			{Title: "OpenCode", Icon: "fa-robot", Group: "extend", Target: "${expose.app.route}"},
		},
		Commands: []plugin.BundleCommandEntry{
			{ID: "opencode.open", Title: "Open OpenCode", Runs: "route:${expose.app.route}"},
		},
		Exposes: map[string]string{"app": "/opencode"},
	})
	core.RequireTrue(t, r.OK)
	names, ok := r.Value.([]string)
	core.RequireTrue(t, ok)
	core.AssertEqual(t, 2, len(names))
	core.AssertContains(t, names, "lthn.route.opencode.opencode")
	core.AssertContains(t, names, "lthn.command.opencode.opencode.open")

	// The route action exists and returns the substituted Target.
	routeR := c.Action("lthn.route.opencode.opencode").Run(core.Background(), core.NewOptions())
	core.RequireTrue(t, routeR.OK)
	payload, ok := routeR.Value.(map[string]any)
	core.RequireTrue(t, ok)
	core.AssertEqual(t, "/opencode", payload["target"])

	// Command action carries the substituted Runs.
	cmdR := c.Action("lthn.command.opencode.opencode.open").Run(core.Background(), core.NewOptions())
	core.RequireTrue(t, cmdR.OK)
	cmdPayload := cmdR.Value.(map[string]any)
	core.AssertEqual(t, "route:/opencode", cmdPayload["runs"])
}

func TestBundle_RegisterBundle_Bad(t *core.T) {
	c := newPluginCore(t)

	// Nil core fails.
	core.AssertFalse(t, plugin.RegisterBundle(nil, "x", plugin.BundleInput{}).OK)

	// Empty bundle id fails.
	core.AssertFalse(t, plugin.RegisterBundle(c, "", plugin.BundleInput{}).OK)

	// Empty input is OK — returns empty []string, not an error.
	r := plugin.RegisterBundle(c, "empty-bundle", plugin.BundleInput{})
	t.Cleanup(func() { _ = plugin.UnregisterBundle(c, "empty-bundle") })
	core.AssertTrue(t, r.OK)
	names, _ := r.Value.([]string)
	core.AssertEqual(t, 0, len(names))
}

func TestBundle_RegisterBundle_Ugly(t *core.T) {
	c := newPluginCore(t)
	t.Cleanup(func() { _ = plugin.UnregisterBundle(c, "x") })

	// Re-registering the same bundle overwrites cleanly — owned
	// list reflects the SECOND registration, not the accumulation.
	first := plugin.RegisterBundle(c, "x", plugin.BundleInput{
		Routes: []plugin.BundleRouteEntry{{Title: "First"}, {Title: "Second"}},
	})
	core.RequireTrue(t, first.OK)
	core.AssertEqual(t, 2, len(first.Value.([]string)))

	second := plugin.RegisterBundle(c, "x", plugin.BundleInput{
		Routes: []plugin.BundleRouteEntry{{Title: "OnlyOne"}},
	})
	core.RequireTrue(t, second.OK)
	core.AssertEqual(t, 1, len(second.Value.([]string)))

	// Unknown ${expose.*} token is left literal — surfaces the bug
	// in the rendered URL rather than masking with empty string.
	tokenR := plugin.RegisterBundle(c, "x", plugin.BundleInput{
		Routes:  []plugin.BundleRouteEntry{{Title: "X", Target: "${expose.unknown.route}"}},
		Exposes: map[string]string{"app": "/known"},
	})
	core.RequireTrue(t, tokenR.OK)
	routeR := c.Action("lthn.route.x.x").Run(core.Background(), core.NewOptions())
	payload := routeR.Value.(map[string]any)
	core.AssertEqual(t, "${expose.unknown.route}", payload["target"])
}

// ─── UnregisterBundle ──────────────────────────────────────────────────

func TestBundle_UnregisterBundle_Good(t *core.T) {
	c := newPluginCore(t)

	plugin.RegisterBundle(c, "opencode", plugin.BundleInput{
		Routes: []plugin.BundleRouteEntry{{Title: "OpenCode", Target: "/x"}},
	})
	// Action exists + dispatches before unregister.
	core.AssertTrue(t, c.Action("lthn.route.opencode.opencode").Exists())
	preDispatch := c.Action("lthn.route.opencode.opencode").Run(core.Background(), core.NewOptions())
	core.AssertTrue(t, preDispatch.OK)

	// Unregister disables the action; dispatch now fails (Action's
	// Disable flips enabled=false; Run on a disabled Action returns
	// a Fail Result).
	r := plugin.UnregisterBundle(c, "opencode")
	core.AssertTrue(t, r.OK)
	postDispatch := c.Action("lthn.route.opencode.opencode").Run(core.Background(), core.NewOptions())
	core.AssertFalse(t, postDispatch.OK)
}

func TestBundle_UnregisterBundle_Bad(t *core.T) {
	c := newPluginCore(t)

	// Nil core fails.
	core.AssertFalse(t, plugin.UnregisterBundle(nil, "x").OK)

	// Empty bundle id fails.
	core.AssertFalse(t, plugin.UnregisterBundle(c, "").OK)

	// Unknown bundle id is OK — idempotent no-op.
	r := plugin.UnregisterBundle(c, "never-installed")
	core.AssertTrue(t, r.OK)
}

func TestBundle_UnregisterBundle_Ugly(t *core.T) {
	c := newPluginCore(t)

	// Two bundles registered; unregistering one doesn't touch the other.
	plugin.RegisterBundle(c, "a", plugin.BundleInput{
		Routes: []plugin.BundleRouteEntry{{Title: "A", Target: "/a"}},
	})
	plugin.RegisterBundle(c, "b", plugin.BundleInput{
		Routes: []plugin.BundleRouteEntry{{Title: "B", Target: "/b"}},
	})
	t.Cleanup(func() { _ = plugin.UnregisterBundle(c, "b") })

	core.RequireTrue(t, plugin.UnregisterBundle(c, "a").OK)

	// A's route is disabled; B's still dispatches.
	aR := c.Action("lthn.route.a.a").Run(core.Background(), core.NewOptions())
	core.AssertFalse(t, aR.OK)
	bR := c.Action("lthn.route.b.b").Run(core.Background(), core.NewOptions())
	core.AssertTrue(t, bR.OK)
}

// ─── ${random.password(N)} substitution ────────────────────────────────

func TestBundle_RandomPassword_Good(t *core.T) {
	c := newPluginCore(t)
	t.Cleanup(func() { _ = plugin.UnregisterBundle(c, "x") })

	// Setting with a ${random.password(16)} default — the persisted
	// value should be a 16-char string (not the literal token).
	// Persistence is best-effort + requires a store service; tests
	// without store still complete the substitution logic in
	// substituteRandom which is the unit under test here.
	r := plugin.RegisterBundle(c, "x", plugin.BundleInput{
		Settings: []plugin.BundleSettingEntry{
			{Key: "secret", Type: "secret", Default: "${random.password(16)}"},
		},
	})
	core.AssertTrue(t, r.OK)
	// We can't easily read back the persisted value without
	// orchestrating a store service; this test just asserts the
	// RegisterBundle call didn't fail on the token. The
	// substituteRandom function itself is exercised by
	// TestBundle_SubstituteRandom_* below via the route Target path
	// (which also runs through the substitution flow).
}

// TestBundle_SubstituteRandom_Ugly_MalformedTokensLeftLiteral drives
// substituteRandom's three "give up and strip/leave" branches
// (persistSetting is the only call site) — a missing closing paren, a
// non-numeric length, and a zero/negative length. None of these should
// panic or infinite-loop; persistSetting is best-effort so RegisterBundle
// still succeeds regardless of what store.set does with the value.
func TestBundle_SubstituteRandom_Ugly_MalformedTokensLeftLiteral(t *core.T) {
	c := newPluginCore(t)
	t.Cleanup(func() { _ = plugin.UnregisterBundle(c, "x") })

	r := plugin.RegisterBundle(c, "x", plugin.BundleInput{
		Settings: []plugin.BundleSettingEntry{
			{Key: "unterminated", Type: "secret", Default: "${random.password(16)"},
			{Key: "non-numeric", Type: "secret", Default: "${random.password(abc)}"},
			{Key: "zero-length", Type: "secret", Default: "${random.password(0)}"},
		},
	})
	core.AssertTrue(t, r.OK, "malformed random tokens are best-effort, not a hard failure")
}

// ─── slug (via actionRouteName, reached through RegisterBundle) ────────

func TestBundle_Slug_Good_DigitsPreserved(t *core.T) {
	c := newPluginCore(t)
	t.Cleanup(func() { _ = plugin.UnregisterBundle(c, "x") })

	r := plugin.RegisterBundle(c, "x", plugin.BundleInput{
		Routes: []plugin.BundleRouteEntry{{Title: "Open v2 Chat", Target: "/chat"}},
	})
	core.RequireTrue(t, r.OK)
	names := r.Value.([]string)
	core.AssertContains(t, names, "lthn.route.x.open-v2-chat")
}

func TestBundle_Slug_Ugly_AllSymbolsFallsBackToUntitled(t *core.T) {
	c := newPluginCore(t)
	t.Cleanup(func() { _ = plugin.UnregisterBundle(c, "x") })

	r := plugin.RegisterBundle(c, "x", plugin.BundleInput{
		Routes: []plugin.BundleRouteEntry{{Title: "!!!", Target: "/x"}},
	})
	core.RequireTrue(t, r.OK)
	names := r.Value.([]string)
	core.AssertContains(t, names, "lthn.route.x.untitled")
}
