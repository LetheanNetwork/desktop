// SPDX-Licence-Identifier: EUPL-1.2

// Package marketplace is the lthn-side plugin marketplace surface.
// Browses available plugins, lists installed ones, dispatches
// install/remove against the (forthcoming) plugin host.
//
// The plugin runtime — start binary on a random port + reverse-
// proxy mount at /v1/api/plugin/<ns>/* — isn't built yet. This
// package ships the catalogue + UI surface so the slot is wired
// for when the runtime lands; Install/Remove return a "plugin
// host not running" Result until then.
//
// Ported from core/ide's marketplacebridge.go + pkg_bridge.go.
// The MCP-tool indirection there existed for AI-tool dual-mount;
// lthn goes direct because the Wails GUI is the only consumer
// today. The DuckDB-backed cache from core/ide is dropped —
// the fixture catalogue is in-memory and small.

package marketplace

import (
	"sync"

	core "dappco.re/go"
)

// Service owns the marketplace surface. Holds *core.Core for
// late resolution of any plugin-host service that lands later.
//
// bundleMu (Mantis #1583 MED) holds a per-bundleID Mutex registry so
// Install / Launch / Stop / Uninstall calls for the SAME bundle id
// serialise — concurrent lifecycle ops for the same bundle used to
// race on the orm record + sandbox handles, producing orphaned
// containers or split state. Different bundle ids remain concurrent.
// Mirror of pkg/office/mail.Service.accountMu (Mantis #1555) and the
// sessions per-session mutex (#1417/#1418). mu guards the bundleMu
// map itself.
type Service struct {
	core     *core.Core
	mu       sync.Mutex
	bundleMu map[string]*sync.Mutex
}

// NewService constructs the marketplace surface against a Core
// container. Wired via application.NewService(marketplace.NewService(c))
// in pkg/desktop/desktop.go.
//
// Usage example:
//
//	svc := marketplace.NewService(c)
func NewService(c *core.Core) *Service {
	return &Service{
		core:     c,
		bundleMu: map[string]*sync.Mutex{},
	}
}

// bundleMutex returns the per-bundleID Mutex, creating it on first
// call. Install / Launch / Stop / Uninstall acquire this at entry so
// the four lifecycle ops for the SAME bundle id serialise (Mantis
// #1583 MED). Different bundle ids remain concurrent.
//
// Usage example:
//
//	m := s.bundleMutex(bundleID)
//	m.Lock(); defer m.Unlock()
func (s *Service) bundleMutex(bundleID string) *sync.Mutex {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.bundleMu == nil {
		s.bundleMu = map[string]*sync.Mutex{}
	}
	if m, ok := s.bundleMu[bundleID]; ok {
		return m
	}
	m := &sync.Mutex{}
	s.bundleMu[bundleID] = m
	return m
}

// Register constructs the marketplace service for Core registration.
//
// Usage example:
//
//	core.New(core.WithService(marketplace.Register))
func Register(c *core.Core) core.Result {
	return core.Ok(NewService(c))
}

// Package is the catalogue record returned by Search. Mirrors
// core/ide's marketplace.IdeModule shape — code is the stable
// identifier, entrypoint is the URL the future plugin host will
// reverse-proxy to.
type Package struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	Version     string `json:"version,omitempty"`
	Repo        string `json:"repo,omitempty"`
	Category    string `json:"category,omitempty"`
	Entrypoint  string `json:"entrypoint,omitempty"`
	Description string `json:"description,omitempty"`
}

// InstalledPackage is what Installed returns — slimmer than
// Package because the catalogue metadata is already on disk.
type InstalledPackage struct {
	Code       string `json:"code"`
	Name       string `json:"name"`
	Version    string `json:"version"`
	EntryPoint string `json:"entry_point,omitempty"`
}

// fixture is the in-memory catalogue lthn ships with. Once a
// remote registry endpoint is wired (config.Marketplace.Endpoint
// in core/ide's pattern) the loader falls back to HTTP; until
// then this is the only source of truth so the UI lights up out
// of the box. Keep entries focused on plugins that make sense
// for lthn-desktop specifically — no IDE-only items like
// formatters or snippet packs.
var fixture = []Package{
	{
		Code: "coreagent", Name: "CoreAgent",
		Version:  "0.1.0",
		Repo:     "lthn/coreagent",
		Category: "agents",
		Description: "First-party agent plugin — chat, brain, sessions, tools. " +
			"Will ship as the canonical example of the plugin runtime " +
			"(binary + --namespace + reverse-proxy mount).",
	},
	{
		Code: "lemma-runner", Name: "Lemma local runner",
		Version:  "0.3.1",
		Repo:     "lthn/lemma-runner",
		Category: "agents",
		Description: "Run Lemma / Gemma 4 / Qwen 3 locally via go-mlx. " +
			"Browse models, generate, fine-tune. Pairs with lthn's " +
			"native inference sidecar.",
	},
	{
		Code: "vi", Name: "Vi — local companion",
		Version:  "0.1.0",
		Repo:     "lthn/vi",
		Category: "agents",
		Description: "Vi is your local companion — watches your sites, " +
			"surfaces alerts, runs local agents. Already shipping in " +
			"lthn-desktop as a built-in; the plugin variant will be " +
			"the sandboxed external-process form.",
	},
}

// GetViewDescriptor returns the resolved PluginViewDescriptor for a
// view id, looked up from the live ViewRegistry. Returns a Fail
// Result with code "marketplace.view.not_found" when the id is not
// currently registered (plugin not installed, plugin uninstalled
// after the frontend cached the id, etc).
//
// Per plans/code/lthn/desktop/views/RFC.plugin-views.md §2 — the
// frontend calls this on mount-time to hydrate the descriptor it
// uses to render <lthn-plugin-view>. The descriptor table mutates
// at install / uninstall; the lookup is cheap (one map read).
//
// Usage example:
//
//	r := svc.GetViewDescriptor("opencode")
//	if !r.OK { /* fall back per §6 chain */ }
//	d := r.Value.(marketplace.PluginViewDescriptor)
func (s *Service) GetViewDescriptor(id string) core.Result {
	id = core.Trim(id)
	if id == "" {
		return core.Fail(core.E("marketplace.view.not_found",
			"view id is required", nil))
	}
	d, ok := ViewRegistry.Lookup(id)
	if !ok {
		return core.Fail(core.E("marketplace.view.not_found",
			"no plugin view registered for id: "+id, nil))
	}
	return core.Ok(d)
}

// search filters the in-memory catalogue by query + category.
// query is matched case-insensitively against code/name/category/
// description; category is exact (case-insensitive) match.
func search(query, category string) []Package {
	queryLower := core.Lower(core.Trim(query))
	categoryLower := core.Lower(core.Trim(category))
	out := make([]Package, 0, len(fixture))
	for _, p := range fixture {
		if categoryLower != "" && core.Lower(p.Category) != categoryLower {
			continue
		}
		if queryLower != "" {
			hay := core.Lower(p.Code + " " + p.Name + " " + p.Category + " " + p.Description)
			if !core.Contains(hay, queryLower) {
				continue
			}
		}
		out = append(out, p)
	}
	return out
}

// findByCode looks one entry up by code. Used by Info-style
// callers that want the full record for a known plugin.
func findByCode(code string) (Package, bool) {
	target := core.Lower(core.Trim(code))
	for _, p := range fixture {
		if core.Lower(p.Code) == target {
			return p, true
		}
	}
	return Package{}, false
}
