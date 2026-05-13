// SPDX-Licence-Identifier: EUPL-1.2

// Manifest parsing + validation for plugin.json files. The
// manifest schema is documented in docs/plugin-host-scope.md.

package plugin

import (
	core "dappco.re/go"
)

// Manifest describes one installable plugin. Stored on disk at
// ~/Lethean/conf/plugins/<code>/plugin.json after Install; also
// embedded in marketplace catalogue entries.
type Manifest struct {
	Code           string   `json:"code"`
	Name           string   `json:"name"`
	Version        string   `json:"version"`
	Namespace      string   `json:"namespace"`
	Binary         string   `json:"binary"` // relative to plugin dir
	Repo           string   `json:"repo,omitempty"`
	BinaryURL      string   `json:"binary_url,omitempty"`
	Checksum       string   `json:"checksum,omitempty"` // sha256:<hex>
	Permissions    []string `json:"permissions,omitempty"`
	StartupTimeout int      `json:"startup_timeout,omitempty"` // seconds; default 5
	Menu           *Menu    `json:"menu,omitempty"`
	Health         *Health  `json:"health,omitempty"`
	UI             *UI      `json:"ui,omitempty"`
}

// Menu describes how a plugin appears in lthn's tray + sidebar.
// Optional — plugins without a UI surface skip this entirely.
type Menu struct {
	Label   string `json:"label"`
	Icon    string `json:"icon,omitempty"`    // Font Awesome class
	Surface string `json:"surface,omitempty"` // becomes ?surface=plugin-<code>
}

// Health describes the readiness probe the host runs on Start
// before mounting the reverse-proxy. Defaults: path
// /<namespace>/health, interval 30s, timeout 5s.
type Health struct {
	Path     string `json:"path,omitempty"`
	Interval int    `json:"interval,omitempty"` // seconds
	Timeout  int    `json:"timeout,omitempty"`  // seconds
}

// UI describes the plugin's embedded UI for the marketplace
// "Open" button + the optional dedicated window. Optional.
type UI struct {
	Entrypoint string `json:"entrypoint,omitempty"` // path under /v1/api/plugin/<code>/
	Embed      string `json:"embed,omitempty"`      // "iframe" | "native"
}

// validate checks the manifest's required fields + normalises
// optional ones with defaults. Returns the normalised copy.
func (m Manifest) validate() (Manifest, core.Result) {
	if core.Trim(m.Code) == "" {
		return m, core.Fail(core.E("plugin.manifest.validate", "code is required", nil))
	}
	if core.Trim(m.Name) == "" {
		return m, core.Fail(core.E("plugin.manifest.validate", "name is required", nil))
	}
	if core.Trim(m.Binary) == "" {
		return m, core.Fail(core.E("plugin.manifest.validate", "binary path is required", nil))
	}
	// Namespace defaults to code when omitted.
	if core.Trim(m.Namespace) == "" {
		m.Namespace = m.Code
	}
	if m.StartupTimeout <= 0 {
		m.StartupTimeout = 5
	}
	if m.Health == nil {
		m.Health = &Health{}
	}
	if core.Trim(m.Health.Path) == "" {
		m.Health.Path = "/" + m.Namespace + "/health"
	}
	if m.Health.Interval <= 0 {
		m.Health.Interval = 30
	}
	if m.Health.Timeout <= 0 {
		m.Health.Timeout = 5
	}
	return m, core.Ok(nil)
}

// loadManifest reads + validates a plugin.json from disk.
func loadManifest(path string) (Manifest, core.Result) {
	read := core.ReadFile(path)
	if !read.OK {
		return Manifest{}, core.Fail(core.E("plugin.loadManifest", "read failed: "+path, nil))
	}
	bytes, _ := read.Value.([]byte)
	var m Manifest
	if r := core.JSONUnmarshal(bytes, &m); !r.OK {
		return Manifest{}, core.Fail(core.E("plugin.loadManifest", "parse failed: "+path, nil))
	}
	return m.validate()
}

// saveManifest writes a manifest to disk under the plugin dir.
func saveManifest(dir string, m Manifest) core.Result {
	raw := core.JSONMarshal(m)
	if !raw.OK {
		return core.Fail(core.E("plugin.saveManifest", "marshal failed", nil))
	}
	bytes, _ := raw.Value.([]byte)
	return core.WriteFile(core.PathJoin(dir, "plugin.json"), bytes, 0o644)
}
