// SPDX-Licence-Identifier: EUPL-1.2

// Menus — read-only Wails surface enumerating plugin menu
// entries. The tray construction (pkg/desktop/desktop.go) calls
// MenuEntries at boot to add per-plugin menu items; the Lit
// canvas-host index lists them so users can launch a plugin
// surface from the same place they launch built-in surfaces.
//
// A plugin without a manifest.menu block is skipped — plenty
// of plugins are pure HTTP services with no UI surface.

package plugin

import core "dappco.re/go"

// MenuEntry is the wire shape of one plugin menu item.
// Surface is "plugin-<code>" so the main.ts router can resolve
// it back to "render the plugin window for <code>". When the
// plugin also has a ui.entrypoint, EntryURL is populated so the
// UI can render the iframe directly without round-tripping
// through Status.
type MenuEntry struct {
	Code      string `json:"code"`
	Label     string `json:"label"`
	Icon      string `json:"icon,omitempty"` // Font Awesome class
	Surface   string `json:"surface"`        // "plugin-<code>"
	Namespace string `json:"namespace"`
	EntryURL  string `json:"entry_url,omitempty"` // "/v1/api/plugin/<ns>/<ui.entrypoint>"
	Running   bool   `json:"running"`
}

// Menus returns one entry per installed plugin that declared a
// manifest.menu block. Plugins without a menu are skipped.
func (s *Service) Menus() core.Result {
	installed := s.scanInstalled()
	out := make([]MenuEntry, 0, len(installed))
	for _, p := range installed {
		dirR := pluginDir(p.Code)
		if !dirR.OK {
			continue
		}
		dir, _ := dirR.Value.(string)
		manifestPath := core.PathJoin(dir, "plugin.json")
		manifestR := loadManifest(manifestPath)
		if !manifestR.OK {
			continue
		}
		m, _ := manifestR.Value.(Manifest)
		if m.Menu == nil {
			continue
		}
		entry := MenuEntry{
			Code:      m.Code,
			Label:     m.Menu.Label,
			Icon:      m.Menu.Icon,
			Surface:   "plugin-" + m.Code,
			Namespace: m.Namespace,
			Running:   p.Status.State == "running",
		}
		if m.UI != nil && m.UI.Entrypoint != "" {
			entry.EntryURL = "/v1/api/plugin/" + m.Namespace + m.UI.Entrypoint
		}
		if entry.Label == "" {
			entry.Label = m.Name
		}
		out = append(out, entry)
	}
	return core.Ok(out)
}
