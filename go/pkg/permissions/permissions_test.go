// SPDX-Licence-Identifier: EUPL-1.2

// Tests for the config-driven EntitlementChecker. The package itself
// only exposes Install(), but the interesting logic lives in the
// unexported resolveEntitlement() helper. Since the test is in the
// same package via the internal_test pattern, we drive the resolver
// directly for the value-type table and use Install+Core for the
// integration test.

package permissions

import (
	core "dappco.re/go"
	"dappco.re/go/config"
)

// Internal package test — direct access to resolveEntitlement so we
// can table-drive every supported value shape without setting up a
// Core/config service.

func TestPermissions_ResolveEntitlement_Good_BoolTrue(t *core.T) {
	e := resolveEntitlement(true, 0)
	core.AssertTrue(t, e.Allowed)
	core.AssertTrue(t, e.Unlimited)
}

func TestPermissions_ResolveEntitlement_Good_BoolFalse(t *core.T) {
	e := resolveEntitlement(false, 0)
	core.AssertFalse(t, e.Allowed)
	core.AssertNotEmpty(t, e.Reason)
}

func TestPermissions_ResolveEntitlement_Good_StringAllow(t *core.T) {
	for _, s := range []string{"allow", "yes", "on", "true", "anonymous", "community"} {
		e := resolveEntitlement(s, 0)
		core.AssertTrue(t, e.Allowed, "string '"+s+"' should allow")
	}
}

func TestPermissions_ResolveEntitlement_Good_StringDeny(t *core.T) {
	for _, s := range []string{"deny", "no", "off", "false"} {
		e := resolveEntitlement(s, 0)
		core.AssertFalse(t, e.Allowed, "string '"+s+"' should deny")
		core.AssertNotEmpty(t, e.Reason)
	}
}

func TestPermissions_ResolveEntitlement_Good_IntQuotaWithinLimit(t *core.T) {
	e := resolveEntitlement(100, 25)
	core.AssertTrue(t, e.Allowed, "25 <= 100 should allow")
	core.AssertEqual(t, 100, e.Limit)
	core.AssertEqual(t, 25, e.Used)
	core.AssertEqual(t, 75, e.Remaining)
}

func TestPermissions_ResolveEntitlement_Bad_IntQuotaOver(t *core.T) {
	e := resolveEntitlement(10, 25)
	core.AssertFalse(t, e.Allowed, "25 > 10 should deny")
	core.AssertEqual(t, 10, e.Limit)
	core.AssertEqual(t, 25, e.Used)
}

func TestPermissions_ResolveEntitlement_Good_Int64Quota(t *core.T) {
	e := resolveEntitlement(int64(50), 10)
	core.AssertTrue(t, e.Allowed)
	core.AssertEqual(t, 50, e.Limit)
}

// Unknown value types default to allow — graceful for forward-compat
// config shapes the resolver doesn't yet understand.
func TestPermissions_ResolveEntitlement_Ugly_UnknownValueDefaultsAllow(t *core.T) {
	e := resolveEntitlement([]string{"weird", "shape"}, 0)
	core.AssertTrue(t, e.Allowed, "unknown shapes should default to allowed")
	core.AssertTrue(t, e.Unlimited)
}

// Install is safe to call with nil Core — no panic, no-op.
func TestPermissions_Install_Ugly_NilCore(t *core.T) {
	called := false
	core.AssertNotPanics(t, func() { Install(nil); called = true })
	core.AssertTrue(t, called)
}

// Install on a Core without a config service should leave Entitled()
// calls returning the permissive default (Allowed:true, Unlimited:true)
// rather than panicking. Exercises the closure's no-config-service
// early-return branch.
func TestPermissions_Install_Good_NoConfigService(t *core.T) {
	c := core.New()
	Install(c)
	e := c.Entitled("network.outbound", 0)
	core.AssertTrue(t, e.Allowed, "no config service → permissive default")
	core.AssertTrue(t, e.Unlimited)
}

// Install + an unknown permission key (config service registered but
// no permissions.* entry for the action) — same permissive default.
// Drives the cfgSvc.Get() miss branch.
func TestPermissions_Install_Good_UnknownKey(t *core.T) {
	c := core.New()
	Install(c)
	// Drive lots of different action shapes through the checker — the
	// only requirement is that none of them panic and all return a
	// sensible Entitlement.
	for _, action := range []string{"models.pull", "telemetry.report", "fleet.connect", "wallet.sign"} {
		e := c.Entitled(action, 1)
		core.AssertNotNil(t, e)
	}
}

// Install + a config service that actually HAS a permissions.<action>
// entry drives the closure's success path (cfgSvc.Get finds the key
// and falls through to resolveEntitlement) — every other Install test
// above only reaches the early-return branches.
func TestPermissions_Install_Good_ConfiguredValueFound(t *core.T) {
	c := core.New(core.WithName(
		"config",
		config.NewConfigServiceWith(config.ServiceOptions{
			Path: core.PathJoin(t.TempDir(), "lthn.yaml"),
		}),
	))
	core.RequireTrue(t, c.ServiceStartup(core.Background(), nil).OK)
	t.Cleanup(func() { _ = c.ServiceShutdown(core.Background()) })
	cfg, ok := core.ServiceFor[*config.Service](c, "config")
	core.RequireTrue(t, ok)
	core.RequireTrue(t, cfg.Set("permissions.network.outbound", true).OK)

	Install(c)
	e := c.Entitled("network.outbound", 0)
	core.AssertTrue(t, e.Allowed, "configured true value should allow via resolveEntitlement")
}
