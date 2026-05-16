// SPDX-Licence-Identifier: EUPL-1.2

// Marketplace event bus — bundle lifecycle changes broadcast
// BundleChanged messages on Core's IPC bus (c.ACTION +
// c.RegisterAction). The frontend (`<lthn-marketplace-window>`),
// future MCP tools, and any in-process agent listener subscribe via
// the canonical Core shape; this file defines the message contract
// + a typed-subscribe helper so listeners don't redo
// msg.(BundleChanged) at every callsite.
//
// Mirrors queue.JobChanged + tasks.IssueChanged shapes so a
// subscriber using one pattern can drop into the other unchanged.
//
// Spec: RFC.marketplace §5.2 (lifecycle transitions are the natural
// events) + Mantis #1417.

package marketplace

import (
	core "dappco.re/go"
)

// BundleChanged is broadcast on the Core IPC bus on every bundle
// lifecycle transition. Carries the full InstalledBundle snapshot
// AFTER the change + the BEFORE snapshot (zero-value when Phase ==
// PhaseInstalled — there's no prior state to capture at first
// install).
//
// Usage example (typed Subscribe shorthand):
//
//	marketplace.Subscribe(c, func(c *core.Core, ev marketplace.BundleChanged) {
//	    if ev.Phase == marketplace.PhaseLaunched { /* … */ }
//	})
//
// Or via canonical Core IPC for multi-message subscribers:
//
//	c.RegisterAction(func(c *core.Core, msg core.Message) core.Result {
//	    if ev, ok := msg.(marketplace.BundleChanged); ok { … }
//	    return core.Result{OK: true}
//	})
type BundleChanged struct {
	// Phase names which lifecycle moment fired this event.
	Phase string `json:"phase"`

	// Bundle is the current InstalledBundle state (after the change).
	// For PhaseUninstalled this carries the final state captured just
	// before the orm record is removed.
	Bundle InstalledBundle `json:"bundle"`

	// Before is the InstalledBundle state prior to the change.
	// Zero-value when Phase == PhaseInstalled (no prior state).
	Before InstalledBundle `json:"before"`

	// At is when the broadcast fired.
	At core.Time `json:"at"`
}

// Phase constants name the lifecycle transitions a BundleChanged
// event reports. Field name is Phase (not Action / Kind / Status) to
// match queue.JobChanged + tasks.IssueChanged + avoid colliding with
// the InstalledBundle.Status field.
const (
	// PhaseInstalled — bundle persisted, all images spawned via
	// pkg/sandbox.SpawnLong. Before is zero.
	PhaseInstalled = "installed"
	// PhaseLaunched — Launch fired, status flipped to running.
	PhaseLaunched = "launched"
	// PhaseStopped — Stop fired, status flipped to stopped.
	PhaseStopped = "stopped"
	// PhaseUninstalled — sandboxes torn down + orm record removed.
	PhaseUninstalled = "uninstalled"
)

// PluginUninstalled is broadcast on the Core IPC bus after a plugin
// bundle is uninstalled, carrying just the plugin code. Frontend
// listeners drop descriptor table entries + tear down mounted views
// per plans/code/lthn/desktop/views/RFC.plugin-views.md §2.1 step 4.
//
// Fired AFTER capability table drop (step 2) + CSP allowlist drop
// (step 3) so subscribers observing the event see a state where the
// host has already stopped accepting postMessage from the doomed
// iframe (step 1).
//
// Usage example:
//
//	c.RegisterAction(func(c *core.Core, msg core.Message) core.Result {
//	    if ev, ok := msg.(marketplace.PluginUninstalled); ok {
//	        // drop descriptors keyed by ev.Code
//	    }
//	    return core.Result{OK: true}
//	})
type PluginUninstalled struct {
	// Code is the plugin code (PluginBlock.Code, falling back to
	// bundle Name when unset) being uninstalled. Frontend uses this
	// to namespace localStorage cleanup + descriptor teardown.
	Code string `json:"code"`
	// At is when the broadcast fired.
	At core.Time `json:"at"`
}

// firePluginUninstalled broadcasts a PluginUninstalled event on the
// Core IPC bus. Safe to call when c is nil; drops the broadcast
// silently in that case. Called by Uninstall AFTER the capability +
// CSP cleanup steps so subscribers observe a clean post-uninstall
// state.
func firePluginUninstalled(c *core.Core, code string) {
	if c == nil || code == "" {
		return
	}
	c.ACTION(PluginUninstalled{
		Code: code,
		At:   core.Now().UTC(),
	})
}

// Subscribe is the typed shorthand for "I only care about
// BundleChanged messages on this Core". Wraps c.RegisterAction with
// the msg.(BundleChanged) cast inline. For multi-message subscribers,
// use c.RegisterAction directly.
//
// Listeners run synchronously in Core's broadcast goroutine; keep
// them cheap. Heavy work goes back into the throttled queue (per
// design_cooperative_task_queue) — enqueue a job, don't block the
// broadcast.
//
// Usage example:
//
//	marketplace.Subscribe(c, func(c *core.Core, ev marketplace.BundleChanged) {
//	    if ev.Phase == marketplace.PhaseInstalled {
//	        queue.Enqueue(c, "notify", core.NewOptions(
//	            core.Option{Key: "bundle_id", Value: ev.Bundle.BundleID},
//	        ))
//	    }
//	})
func Subscribe(c *core.Core, fn func(*core.Core, BundleChanged)) {
	if c == nil || fn == nil {
		return
	}
	c.RegisterAction(func(c *core.Core, msg core.Message) core.Result {
		if ev, ok := msg.(BundleChanged); ok {
			fn(c, ev)
		}
		return core.Result{OK: true}
	})
}

// fireBundleChanged broadcasts a BundleChanged on the Core IPC bus.
// Internal helper used by Install / Launch / Stop / Uninstall to
// keep the fire sites uniform — every transition carries the same
// {Phase, Bundle, Before, At} shape so subscribers can branch on
// Phase without special-casing each lifecycle method.
//
// Safe to call when c is nil (defensive — Service.core may be nil
// in test paths that construct via NewService(c=nil)). Drops the
// broadcast silently in that case.
func fireBundleChanged(c *core.Core, phase string, bundle, before InstalledBundle) {
	if c == nil {
		return
	}
	c.ACTION(BundleChanged{
		Phase:  phase,
		Bundle: bundle,
		Before: before,
		At:     core.Now().UTC(),
	})
}
