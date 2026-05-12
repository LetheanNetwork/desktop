// SPDX-Licence-Identifier: EUPL-1.2

// Package permissions installs a core.EntitlementChecker on the Core
// that gates actions according to user choices persisted at
// `permissions.*` in ~/Lethean/conf/lthn.yaml. Composed from
// dappco.re/go's EntitlementChecker primitive — no new dep tree.
//
// The default checker (permissive) ships in dappco.re/go and answers
// "yes" to every action. This package narrows that based on config:
//
//	permissions:
//	  network.outbound: true       # outbound HTTP allowed
//	  network.inbound: false       # serving inbound (lthn serve) gated off
//	  telemetry: anonymous         # off / anonymous / community
//
// Action names are dotted-lowercase (per the audit's
// action-name-format dim): "network.outbound", "telemetry.report",
// "models.pull", etc.
//
// When no permissions key is set in config, the checker defers to
// the permissive default — same as not installing this package at
// all. This lets developers run the binary without a setup wizard.
//
// Usage example:
//
//	c := newAppCore()
//	permissions.Install(c)
//	r := c.Entitled("network.outbound")
//	if r.Allowed { /* go */ }
package permissions

import (
	core "dappco.re/go"
	"dappco.re/go/config"
)

// Install attaches the config-driven EntitlementChecker to c. Safe
// to call on a Core where no config service is registered — the
// permissive default kicks in.
//
// Usage example:
//
//	c := newAppCore()
//	permissions.Install(c)
func Install(c *core.Core) {
	if c == nil {
		return
	}
	c.SetEntitlementChecker(func(action string, quantity int, _ core.Context) core.Entitlement {
		cfgSvc, ok := core.ServiceFor[*config.Service](c, "config")
		if !ok || cfgSvc == nil {
			return core.Entitlement{Allowed: true, Unlimited: true}
		}
		var value any
		key := core.Concat("permissions.", action)
		if r := cfgSvc.Get(key, &value); !r.OK {
			return core.Entitlement{Allowed: true, Unlimited: true}
		}
		return resolveEntitlement(value, quantity)
	})
}

// resolveEntitlement converts a config value into an Entitlement.
// Supports the common shapes:
//
//	true             → Allowed: true (boolean gate)
//	false            → Allowed: false, Reason: "denied"
//	"allow"          → same as true
//	"deny"           → same as false
//	"anonymous"|"community"|<other string> → Allowed: true,
//	                                           Labels carry mode
//	int N            → quota check (Allowed: quantity <= N)
func resolveEntitlement(value any, quantity int) core.Entitlement {
	switch v := value.(type) {
	case bool:
		if v {
			return core.Entitlement{Allowed: true, Unlimited: true}
		}
		return core.Entitlement{Allowed: false, Reason: "permission denied by config"}
	case string:
		lower := core.Lower(v)
		if lower == "deny" || lower == "false" || lower == "no" || lower == "off" {
			return core.Entitlement{Allowed: false, Reason: "permission denied by config"}
		}
		// Any other string treated as an allow-with-mode-label.
		return core.Entitlement{Allowed: true, Unlimited: true}
	case int:
		return core.Entitlement{
			Allowed:   quantity <= v,
			Limit:     v,
			Used:      quantity,
			Remaining: v - quantity,
		}
	case int64:
		limit := int(v)
		return core.Entitlement{
			Allowed:   quantity <= limit,
			Limit:     limit,
			Used:      quantity,
			Remaining: limit - quantity,
		}
	}
	return core.Entitlement{Allowed: true, Unlimited: true}
}
