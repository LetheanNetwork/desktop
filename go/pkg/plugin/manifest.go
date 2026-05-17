// SPDX-Licence-Identifier: EUPL-1.2

// Manifest parsing + validation for plugin.json files. The
// manifest schema is documented in docs/plugin-host-scope.md.

package plugin

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/paths"
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

// isValidPluginCode delegates to the shared paths.IsValidPluginCode
// per Mantis #1580 — extracted to a common helper so pkg/marketplace
// (lthn-vm bundle plugin block) and pkg/plugin (binary plugins) gate
// on the same rule set. The cousin-validator drift class is the gap
// the extraction closes: a marketplace-declared plugin.code used to
// bypass the path-traversal hardening pkg/plugin got from Cerberus
// Mantis #1436 because the marketplace surface had no equivalent
// validator. Now both surfaces import from one source.
//
// Wrapper kept (rather than inlining call sites) so call-site
// references stay unchanged; future loosening / tightening of the
// rule lives in one place.
func isValidPluginCode(code string) bool {
	return paths.IsValidPluginCode(code)
}

// isValidBinaryPath reports whether a Binary field is a safe
// relative path that resolves UNDER the plugin install dir.
// Cerberus Mantis #1436 — without this gate, `Binary: "../../etc/
// passwd"` would have `PathJoin(dir, m.Binary)` escape the plugin
// dir, letting WriteFile clobber an arbitrary host file.
//
// Rules:
//   - Non-empty.
//   - No leading `/` (must be relative).
//   - No `..` segment anywhere — refuses traversal.
//   - No NUL or control characters.
//   - CleanPath of the join must still be prefixed by the plugin dir
//     (the join-then-check is done at the caller via a helper).
//
// For v1 the manifest install path always sets `Binary = "bin/<code>"`
// (install.go:52), so legitimate manifests look like `bin/foo`. The
// validator allows that shape and rejects everything else.
func isValidBinaryPath(p string) bool {
	if p == "" {
		return false
	}
	if core.HasPrefix(p, "/") {
		return false
	}
	for _, ch := range p {
		if ch == 0 || ch < 32 {
			return false
		}
	}
	// Split on / and reject any segment that's ".." or contains
	// backslash (Windows path-traversal trick). Allow . as a no-op
	// segment for forward-compat; CleanPath collapses them.
	parts := core.Split(p, "/")
	for _, seg := range parts {
		if seg == ".." {
			return false
		}
		if core.Contains(seg, "\\") {
			return false
		}
	}
	return true
}

// validate checks the manifest's required fields + normalises
// optional ones with defaults. Returns the normalised copy.
func (m Manifest) validate() core.Result {
	if core.Trim(m.Code) == "" {
		return core.Fail(core.E(manifestValidateOp, "code is required", nil))
	}
	// Cerberus #1436 — path-traversal gate on Code BEFORE it flows
	// into pluginDir(PathJoin(root, code)) or the URL route mount.
	if !isValidPluginCode(m.Code) {
		return core.Fail(core.E(manifestValidateOp,
			"invalid code (must be alphanumeric basename, 1-64 chars, leading alnum): "+m.Code, nil))
	}
	if core.Trim(m.Name) == "" {
		return core.Fail(core.E(manifestValidateOp, "name is required", nil))
	}
	if core.Trim(m.Binary) == "" {
		return core.Fail(core.E(manifestValidateOp, "binary path is required", nil))
	}
	// Cerberus #1436 — path-traversal gate on Binary BEFORE it flows
	// into PathJoin(dir, m.Binary).
	if !isValidBinaryPath(m.Binary) {
		return core.Fail(core.E(manifestValidateOp,
			"invalid binary path (must be relative, no `..` segments, no leading `/`): "+m.Binary, nil))
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
	return core.Ok(m)
}

// loadManifest reads + validates a plugin.json from disk.
func loadManifest(path string) core.Result {
	read := core.ReadFile(path)
	if !read.OK {
		return core.Fail(core.E("plugin.loadManifest", "read failed: "+path, nil))
	}
	bytes, _ := read.Value.([]byte)
	var m Manifest
	if r := core.JSONUnmarshal(bytes, &m); !r.OK {
		return core.Fail(core.E("plugin.loadManifest", "parse failed: "+path, nil))
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
