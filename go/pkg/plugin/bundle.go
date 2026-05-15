// SPDX-Licence-Identifier: EUPL-1.2

// Bundle registration — bridges a marketplace-bundle plugin: block
// (per RFC.marketplace.md §2.1 + §5.1) into pkg/plugin's Core action
// registry. install.Install calls RegisterBundle after the
// sandboxes are up; install.Uninstall calls UnregisterBundle.
//
// This file defines plugin-local types (RouteEntry, CommandEntry,
// SettingEntry, BundleInput) that mirror the marketplace manifest's
// PluginBlock shape so pkg/plugin doesn't import pkg/marketplace
// (the reverse import already exists — marketplace's wails.go uses
// plugin.Service for the legacy binary-plugin host). Marketplace's
// install.go copies values into these types at call site.
//
// Five-pillar mapping (per project_core_ide_five_pillar_plugin_contract):
//
//   - Routes   → c.Action("lthn.route.<bundle>.<title-slug>")
//   - Commands → c.Action("lthn.command.<bundle>.<command-id>")
//   - Settings → go-store under group "plugin.settings.<bundle>"
//   - Status   → not implemented for v1 (manifest doesn't carry status entries)
//   - Events   → not implemented for v1 (manifest doesn't carry event entries)
//
// Token substitution at registration time:
//
//   - ${expose.<image-id>.route} → resolves against BundleInput.Exposes
//   - ${random.password(N)}      → resolves via core.RandomString(N), one-shot at install

package plugin

import (
	core "dappco.re/go"
)

const (
	registerBundleOp   = "plugin.RegisterBundle"
	unregisterBundleOp = "plugin.UnregisterBundle"
)

// BundleRouteEntry mirrors marketplace.RouteEntry. Defined locally so
// pkg/plugin doesn't depend on pkg/marketplace.
type BundleRouteEntry struct {
	Title            string
	Icon             string
	Group            string
	Target           string
	OpenAfterInstall bool
}

// BundleCommandEntry mirrors marketplace.CommandEntry.
type BundleCommandEntry struct {
	ID    string
	Title string
	Runs  string
}

// BundleSettingEntry mirrors marketplace.SettingEntry.
type BundleSettingEntry struct {
	Key     string
	Type    string
	Prompt  string
	Default string
}

// BundleInput drives RegisterBundle. Routes / Commands / Settings
// are the per-pillar entries; Exposes maps image-id → route so
// ${expose.<id>.route} tokens substitute. Empty Exposes leaves
// the tokens literal (intentional — surfaces bugs in the rendered
// URL rather than masking them).
//
// Usage example:
//
//	plugin.RegisterBundle(c, "opencode", plugin.BundleInput{
//	    Routes:   routes,
//	    Commands: commands,
//	    Settings: settings,
//	    Exposes:  map[string]string{"app": "/opencode"},
//	})
type BundleInput struct {
	Routes   []BundleRouteEntry
	Commands []BundleCommandEntry
	Settings []BundleSettingEntry
	Exposes  map[string]string
}

// bundleRegistry tracks which action names a bundle owns so
// UnregisterBundle can disable them all. Keyed by (core-pointer,
// bundle-id) so multiple Cores in the same process (typical for
// tests) keep their owned lists independent.
//
// Package-level state is fine here — RegisterBundle / UnregisterBundle
// are install-side ops that run rarely, and the registry is
// effectively immutable between those ops. Tests should call
// UnregisterBundle on shutdown to keep the registry clean across
// runs.
var (
	bundleRegistryMu core.Mutex
	bundleRegistry   = map[string]map[string][]string{} // core-id → bundleID → []action-name
)

// coreID returns a stable per-Core identifier for the registry map.
// Pointer hex is sufficient — Cores live for the lifetime of the
// binary or the test; cross-Core collisions don't happen.
func coreID(c *core.Core) string {
	return core.Sprintf("%p", c)
}

// ownedFor returns the bundle → []action-name map for one Core,
// initialising on first access.
func ownedFor(c *core.Core) map[string][]string {
	id := coreID(c)
	bundleRegistryMu.Lock()
	defer bundleRegistryMu.Unlock()
	m, ok := bundleRegistry[id]
	if !ok {
		m = map[string][]string{}
		bundleRegistry[id] = m
	}
	return m
}

// RegisterBundle wires every entry in input into the Core action
// registry under bundleID-scoped names. Idempotent — re-registering
// the same bundle re-installs the entries (matches Core's c.Action
// last-write-wins semantics) without leaking the previous owned
// list.
//
// Returns Ok with a slice of the registered action names so the
// caller can log or surface them. Empty input returns Ok with an
// empty slice — not an error.
//
// Usage example:
//
//	r := plugin.RegisterBundle(c, "opencode", input)
//	if r.OK { names := r.Value.([]string); _ = names }
func RegisterBundle(c *core.Core, bundleID string, input BundleInput) core.Result {
	if c == nil {
		return core.Fail(core.E(registerBundleOp, "core is nil", nil))
	}
	if bundleID == "" {
		return core.Fail(core.E(registerBundleOp, "bundle id is required", nil))
	}
	owned := ownedFor(c)
	// Re-register: clean the old set first so the owned slice doesn't
	// accumulate stale entries.
	if _, ok := owned[bundleID]; ok {
		unregisterOwned(c, bundleID)
	}

	registered := make([]string, 0, len(input.Routes)+len(input.Commands))

	for _, r := range input.Routes {
		name := actionRouteName(bundleID, r.Title)
		entry := substituteRoute(r, input.Exposes)
		c.Action(name, routeHandler(entry))
		registered = append(registered, name)
	}

	for _, cmd := range input.Commands {
		name := actionCommandName(bundleID, cmd.ID)
		entry := substituteCommand(cmd, input.Exposes)
		c.Action(name, commandHandler(entry))
		registered = append(registered, name)
	}

	for _, set := range input.Settings {
		// Best-effort — a settings failure shouldn't abort route /
		// command registration that already landed.
		_ = persistSetting(c, bundleID, set)
	}

	bundleRegistryMu.Lock()
	owned[bundleID] = registered
	bundleRegistryMu.Unlock()
	return core.Ok(registered)
}

// UnregisterBundle disables every action this bundle registered and
// removes its settings group from go-store. Idempotent — calling on
// an unknown bundle is a no-op Ok.
//
// Usage example:
//
//	r := plugin.UnregisterBundle(c, bundleID)
//	if !r.OK { return r }
func UnregisterBundle(c *core.Core, bundleID string) core.Result {
	if c == nil {
		return core.Fail(core.E(unregisterBundleOp, "core is nil", nil))
	}
	if bundleID == "" {
		return core.Fail(core.E(unregisterBundleOp, "bundle id is required", nil))
	}
	unregisterOwned(c, bundleID)
	// Settings cleanup — drop the per-bundle settings group from
	// go-store. Best-effort; a store error doesn't fail the
	// unregister (the action disables already landed).
	_ = c.Action("store.delete_group").Run(core.Background(), core.NewOptions(
		core.Option{Key: "group", Value: settingsGroup(bundleID)},
	))
	return core.Ok(nil)
}

// unregisterOwned disables every action recorded for bundleID + clears
// the registry entry. Internal — shared by Register's re-register
// path and Unregister.
func unregisterOwned(c *core.Core, bundleID string) {
	owned := ownedFor(c)
	bundleRegistryMu.Lock()
	names := owned[bundleID]
	delete(owned, bundleID)
	bundleRegistryMu.Unlock()
	for _, name := range names {
		c.Action(name).Disable()
	}
}

// actionRouteName composes the canonical route-action name.
//
// Usage example:
//
//	name := actionRouteName("opencode", "OpenCode")
//	// → "lthn.route.opencode.opencode"
func actionRouteName(bundleID, title string) string {
	return "lthn.route." + bundleID + "." + slug(title)
}

// actionCommandName composes the canonical command-action name.
//
// Usage example:
//
//	name := actionCommandName("opencode", "opencode.open")
//	// → "lthn.command.opencode.opencode.open"
func actionCommandName(bundleID, commandID string) string {
	return "lthn.command." + bundleID + "." + commandID
}

// settingsGroup returns the go-store group name for one bundle's
// settings. Per-bundle group so UnregisterBundle can drop the whole
// set in one call.
func settingsGroup(bundleID string) string {
	return "plugin.settings." + bundleID
}

// routeHandler wraps a substituted RouteEntry into an ActionHandler
// the Core IPC bus dispatches. The handler returns the entry as a
// map so the frontend / MCP / CLI can render it.
func routeHandler(r BundleRouteEntry) core.ActionHandler {
	return func(_ core.Context, _ core.Options) core.Result {
		return core.Ok(map[string]any{
			"title":              r.Title,
			"icon":               r.Icon,
			"group":              r.Group,
			"target":             r.Target,
			"open_after_install": r.OpenAfterInstall,
		})
	}
}

// commandHandler wraps a substituted CommandEntry into an ActionHandler.
// The handler returns the runs: target so a generic dispatcher can
// route by prefix — "route:<path>" navigates, future "shell:<cmd>"
// would shell out, etc.
func commandHandler(cmd BundleCommandEntry) core.ActionHandler {
	return func(_ core.Context, _ core.Options) core.Result {
		return core.Ok(map[string]any{
			"id":    cmd.ID,
			"title": cmd.Title,
			"runs":  cmd.Runs,
		})
	}
}

// persistSetting writes one SettingEntry's default value into the
// bundle's go-store group. Resolves ${random.password(N)} at write
// time so the default isn't re-randomised on every read.
func persistSetting(c *core.Core, bundleID string, s BundleSettingEntry) core.Result {
	value := substituteRandom(s.Default)
	return c.Action("store.set").Run(core.Background(), core.NewOptions(
		core.Option{Key: "group", Value: settingsGroup(bundleID)},
		core.Option{Key: "key", Value: s.Key},
		core.Option{Key: "value", Value: value},
	))
}

// substituteRoute resolves ${expose.<id>.route} tokens in a
// RouteEntry's Target. Returns a copy; the input is untouched.
func substituteRoute(r BundleRouteEntry, exposes map[string]string) BundleRouteEntry {
	r.Target = substituteExpose(r.Target, exposes)
	return r
}

// substituteCommand resolves the same tokens in a CommandEntry's Runs.
func substituteCommand(cmd BundleCommandEntry, exposes map[string]string) BundleCommandEntry {
	cmd.Runs = substituteExpose(cmd.Runs, exposes)
	return cmd
}

// substituteExpose replaces every ${expose.<id>.route} occurrence in s
// with the matching value from exposes. Unknown ids are left literal —
// the rendered URL surfaces the bug rather than masking it.
func substituteExpose(s string, exposes map[string]string) string {
	for id, route := range exposes {
		s = core.Replace(s, "${expose."+id+".route}", route)
	}
	return s
}

// substituteRandom resolves ${random.password(N)} in s by generating
// a fresh random string of length N. Multiple occurrences each get
// independent values. Malformed tokens (non-numeric N, missing
// parens) are left literal.
func substituteRandom(s string) string {
	const prefix = "${random.password("
	const suffix = ")}"
	for {
		i := core.Index(s, prefix)
		if i < 0 {
			return s
		}
		j := core.Index(s[i+len(prefix):], suffix)
		if j < 0 {
			return s
		}
		nStr := s[i+len(prefix) : i+len(prefix)+j]
		nR := core.Atoi(nStr)
		if !nR.OK {
			// Malformed — advance past the prefix so the loop terminates.
			s = s[:i] + s[i+len(prefix):]
			continue
		}
		n := nR.Value.(int)
		if n <= 0 {
			s = s[:i] + s[i+len(prefix):]
			continue
		}
		r := core.RandomString(n)
		repl := ""
		if r.OK {
			repl = r.Value.(string)
		}
		s = s[:i] + repl + s[i+len(prefix)+j+len(suffix):]
	}
}

// slug lowercases s + maps non-alphanumeric to "-" so a route title
// like "Open in chat" becomes "open-in-chat" — safe in a Core action
// name.
func slug(s string) string {
	lower := core.Lower(s)
	out := make([]byte, 0, len(lower))
	prevDash := true
	for i := 0; i < len(lower); i++ {
		b := lower[i]
		switch {
		case b >= 'a' && b <= 'z':
			out = append(out, b)
			prevDash = false
		case b >= '0' && b <= '9':
			out = append(out, b)
			prevDash = false
		case !prevDash:
			out = append(out, '-')
			prevDash = true
		}
	}
	if len(out) > 0 && out[len(out)-1] == '-' {
		out = out[:len(out)-1]
	}
	if len(out) == 0 {
		return "untitled"
	}
	return string(out)
}
