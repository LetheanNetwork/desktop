// SPDX-Licence-Identifier: EUPL-1.2

package main

import (
	core "dappco.re/go"
	"dappco.re/go/orm"
	lthn "dappco.re/lthn/desktop"
	"dappco.re/lthn/desktop/pkg/gateway"
	"dappco.re/lthn/desktop/pkg/marketplace"
	"dappco.re/lthn/desktop/pkg/plugin"
	"dappco.re/lthn/desktop/pkg/runner"
)

func TestMain_CmdVersionLabel_Good_PreservesTagPrefix(t *core.T) {
	original := lthn.Version
	t.Cleanup(func() { lthn.Version = original })
	lthn.Version = "v1.2.3"

	core.AssertEqual(t, "v1.2.3", cmdVersionLabel())
}

func TestMain_CmdVersionLabel_Bad_AddsMissingTagPrefix(t *core.T) {
	original := lthn.Version
	t.Cleanup(func() { lthn.Version = original })
	lthn.Version = "1.2.3"

	core.AssertEqual(t, "v1.2.3", cmdVersionLabel())
}

func TestMain_CmdVersionLabel_Ugly_PreservesDirtyTag(t *core.T) {
	original := lthn.Version
	t.Cleanup(func() { lthn.Version = original })
	lthn.Version = "v1.2.3-4-gabc123-dirty"

	core.AssertEqual(t, "v1.2.3-4-gabc123-dirty", cmdVersionLabel())
}

// newBuildOptsCore wires the minimum services buildServerOpts inspects:
// gateway (data firewall route group), plugin (binary-plugin install
// tier + ProxyGroup), marketplace (bundle install tier + ViewRegistry).
// opencode is intentionally OMITTED — its Reconcile path shells docker,
// which the hermetic test bench can't depend on; buildServerOpts already
// short-circuits when opencode isn't registered, so the SECURITY-relevant
// wiring (gateway routes, PluginInstalledChecker, PluginOriginChecker)
// can be asserted without it.
func newBuildOptsCore(t *core.T) *core.Core {
	t.Helper()
	c := core.New()
	core.RequireTrue(t, orm.Register(c).OK)
	mem := orm.NewMemium()
	core.RequireTrue(t, orm.Mount(c, "default", mem).OK)
	schema := marketplace.InstalledBundle{}.Schema()
	core.RequireTrue(t, orm.RegisterSchema(c, schema).OK)
	mem.RegisterTable(schema.Name, schema)
	pluginR := plugin.NewService(plugin.Options{})(c)
	core.RequireTrue(t, pluginR.OK)
	core.RequireTrue(t, c.RegisterService("plugin", pluginR.Value.(*plugin.Service)).OK)
	core.RequireTrue(t, c.RegisterService("marketplace", marketplace.NewService(c)).OK)
	core.RequireTrue(t, c.RegisterService("gateway", gateway.NewService(c)).OK)
	core.RequireTrue(t, c.ServiceStartup(core.Background(), nil).OK)
	t.Cleanup(func() { _ = c.ServiceShutdown(core.Background()) })
	return c
}

// TestMain_BuildServerOpts_Good_MountsGatewayAndCheckers pins the
// Mantis #1741 / Cerberus #70 F-1 contract: buildServerOpts MUST emit
// the gateway RouteGroup into ExtraGroups whenever pkg/gateway is
// registered, AND MUST wire the PluginInstalledChecker +
// PluginOriginChecker callbacks. Pre-#1741, cmdGUI omitted this
// entire surface (data firewall absent in GUI mode); the helper
// extraction is what restores parity with cmdServe.
func TestMain_BuildServerOpts_Good_MountsGatewayAndCheckers(t *core.T) {
	c := newBuildOptsCore(t)
	r := runner.NewService(runner.Options{})
	opts := buildServerOpts(c, r, "test-local-key", false)

	core.AssertNotNil(t, opts.ExtraGroups,
		"ExtraGroups must be populated when gateway service is registered")
	core.AssertGreater(t, len(opts.ExtraGroups), 0,
		"ExtraGroups must contain at least the gateway RouteGroup")

	// Walk the ExtraGroups and prove the gateway RouteGroup is mounted.
	// pkg/gateway.NewRoutes returns a RouteGroup whose Name() prefix
	// is "gateway"; defensive on the exact name to avoid pinning a
	// brittle private string.
	foundGateway := false
	for _, g := range opts.ExtraGroups {
		if core.Contains(g.Name(), "gateway") {
			foundGateway = true
			break
		}
	}
	core.AssertTrue(t, foundGateway,
		"gateway RouteGroup must be present in ExtraGroups (data firewall)")

	core.AssertNotNil(t, opts.PluginInstalledChecker,
		"PluginInstalledChecker must be wired when plugin or marketplace exists")
	core.AssertNotNil(t, opts.PluginOriginChecker,
		"PluginOriginChecker must be wired (Cerberus #21 audit-integrity gate)")
}

// TestMain_BuildServerOpts_Bad_HandlesEmptyCoreGracefully pins
// degradation behaviour — buildServerOpts on a Core with no
// gateway/plugin/marketplace MUST NOT panic, MUST return an opts
// surface with the always-present fields (Runner, LocalKey, Brand,
// Core) populated, and MAY leave ExtraGroups / checkers empty.
func TestMain_BuildServerOpts_Bad_HandlesEmptyCoreGracefully(t *core.T) {
	c := core.New()
	core.RequireTrue(t, c.ServiceStartup(core.Background(), nil).OK)
	t.Cleanup(func() { _ = c.ServiceShutdown(core.Background()) })

	r := runner.NewService(runner.Options{})
	opts := buildServerOpts(c, r, "empty-core-key", false)

	core.AssertEqual(t, "empty-core-key", opts.LocalKey)
	core.AssertNotNil(t, opts.Runner)
	core.AssertNotNil(t, opts.Core)
	// PluginOriginChecker always wires (it doesn't depend on a
	// registered service — it reaches marketplace.ViewRegistry, a
	// package-level value); PluginInstalledChecker may be nil.
	core.AssertNotNil(t, opts.PluginOriginChecker,
		"PluginOriginChecker wires unconditionally on marketplace.ViewRegistry")
}

// TestMain_BuildServerOpts_Ugly_GUIOmitsDuplicateProxyGroups pins the
// intentional GUI/serve split: both modes retain their shared checkers,
// while GUI mode omits proxy groups mounted later by desktop subsystems.
func TestMain_BuildServerOpts_Ugly_GUIOmitsDuplicateProxyGroups(t *core.T) {
	c := newBuildOptsCore(t)
	r := runner.NewService(runner.Options{})

	guiOpts := buildServerOpts(c, r, "shared-key", true)
	serveOpts := buildServerOpts(c, r, "shared-key", false)
	serveOpts.Addr = ":8000" // serve verb's only delta from GUI

	core.AssertGreater(t, len(serveOpts.ExtraGroups), len(guiOpts.ExtraGroups))
	core.AssertEqual(t, guiOpts.LocalKey, serveOpts.LocalKey)
	core.AssertEqual(t, guiOpts.PluginInstalledChecker == nil,
		serveOpts.PluginInstalledChecker == nil)
	core.AssertEqual(t, guiOpts.PluginOriginChecker == nil,
		serveOpts.PluginOriginChecker == nil)
	core.AssertEqual(t, "", guiOpts.Addr)
	core.AssertEqual(t, ":8000", serveOpts.Addr)
}

// TestMain_CmdVersion_Good — prints the v-prefixed version and exits
// clean.
func TestMain_CmdVersion_Good(t *core.T) {
	core.AssertEqual(t, 0, cmdVersion(nil))
}

// TestMain_CmdHelp_Good — the general help, the inline ai/serve
// topics, and the fleet delegation all exit clean.
func TestMain_CmdHelp_Good(t *core.T) {
	core.AssertEqual(t, 0, cmdHelp(nil))
	core.AssertEqual(t, 0, cmdHelp([]string{"ai"}))
	core.AssertEqual(t, 0, cmdHelp([]string{"serve"}))
	core.AssertEqual(t, 0, cmdHelp([]string{"fleet"}))
}

// TestMain_CmdHelp_Bad — verbs without inline help point at their
// no-args verb list; unknown subcommands are rejected.
func TestMain_CmdHelp_Bad(t *core.T) {
	core.AssertEqual(t, 2, cmdHelp([]string{"config"}))
	core.AssertEqual(t, 2, cmdHelp([]string{"unknownverb"}))
}

// TestMain_CmdAI_Bad — missing and unknown verbs, plus the chat /
// generate usage errors reached through the dispatcher.
func TestMain_CmdAI_Bad(t *core.T) {
	core.AssertEqual(t, 2, cmdAI(nil))
	core.AssertEqual(t, 2, cmdAI([]string{"nonsense"}))
	core.AssertEqual(t, 2, cmdAI([]string{"chat"}))
	core.AssertEqual(t, 2, cmdAI([]string{"generate"}))
}
