// SPDX-Licence-Identifier: EUPL-1.2

// Bundle manifest parser + validator for lthn-vm bundles.
// A manifest.yml describes one installable bundle: OCI images to spawn,
// optional five-pillar plugin registrations, optional env-var prompts,
// optional gateway permission declarations.
//
// Spec: plans/project/lthn/desktop/RFC.marketplace.md §2.

package marketplace

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/imagetrust"
	"dappco.re/lthn/desktop/pkg/paths"
	"gopkg.in/yaml.v3"
)

const (
	manifestSchema  = "lthn-vm/v1"
	parseManifestOp = "marketplace.ParseManifest"
	validateOp      = "marketplace.ValidateManifest"
)

// BundleManifest is the parsed form of a manifest.yml file.
type BundleManifest struct {
	Schema      string       `yaml:"schema"`
	Name        string       `yaml:"name"`
	Display     string       `yaml:"display"`
	Description string       `yaml:"description"`
	Category    string       `yaml:"category"`
	Icon        string       `yaml:"icon"`
	Homepage    string       `yaml:"homepage"`
	License     string       `yaml:"license"`
	Images      []ImageEntry `yaml:"images"`
	Plugin      *PluginBlock `yaml:"plugin,omitempty"`
	Env         []EnvEntry   `yaml:"env,omitempty"`
	Permissions []Permission `yaml:"permissions,omitempty"`
}

// ImageEntry is one container image in a bundle. Each entry becomes
// one sandbox.SpawnLong() call at install time.
type ImageEntry struct {
	ID      string            `yaml:"id"`
	Image   string            `yaml:"image"`
	Env     map[string]string `yaml:"env,omitempty"`
	Volumes []VolumeMount     `yaml:"volumes,omitempty"`
	Expose  *ExposeBlock      `yaml:"expose,omitempty"`
}

// VolumeMount describes a named persistent volume.
// The host-side volume is managed by pkg/sandbox.
type VolumeMount struct {
	Container string `yaml:"container"` // mount path inside the container
	Persist   string `yaml:"persist"`   // named volume identifier
}

// ExposeBlock declares which port/path gets a reverse-proxy mount.
// Only entries with an Expose block are reachable at lthn.sh/<route>.
type ExposeBlock struct {
	Port  int    `yaml:"port"`
	Route string `yaml:"route"` // path under lthn.sh e.g. "/phpmyadmin"
}

// PluginBlock holds optional five-pillar registrations.
type PluginBlock struct {
	Routes   []RouteEntry   `yaml:"routes,omitempty"`
	Commands []CommandEntry `yaml:"commands,omitempty"`
	Settings []SettingEntry `yaml:"settings,omitempty"`

	// Code is the plugin's globally-unique code. Used as the namespace
	// for ${expose.<id>.route} resolution + view-id collision rules +
	// uninstall back-reference. When omitted, defaults to the bundle's
	// top-level Name at validation time so existing manifests stay
	// backward-compatible. Reserved code "lthn" identifies first-party
	// plugins (tier-0 lit-kind acceptance per
	// plans/code/lthn/desktop/views/RFC.plugin-views.md §5.2).
	Code string `yaml:"code,omitempty"`

	// Views is the optional set of full-screen view registrations the
	// plugin adds to the lthn shell's VIEWS registry. Each entry is
	// materialised on the frontend as a <lthn-plugin-view> element
	// inside the shell chrome. See RFC.plugin-views.md §2 + §9 for
	// validation rules.
	Views []PluginView `yaml:"views,omitempty"`
}

// PluginView declares one full-screen view a plugin registers with
// the lthn shell. Validated at install time per
// plans/code/lthn/desktop/views/RFC.plugin-views.md §9.
//
// Usage example (manifest YAML):
//
//	plugin:
//	  code: opencode
//	  views:
//	    - id: opencode
//	      label: Opencode
//	      icon: fa-terminal
//	      kind: iframe
//	      source: ${expose.web.route}
//	      capabilities: [session-token]
type PluginView struct {
	ID           string   `yaml:"id"`
	Label        string   `yaml:"label"`
	Icon         string   `yaml:"icon,omitempty"`
	Kind         string   `yaml:"kind"`
	Source       string   `yaml:"source"`
	Capabilities []string `yaml:"capabilities,omitempty"`
}

// PluginViewKindIframe identifies an iframe-rendered plugin view —
// the plugin's own webapp loaded from its declared loopback origin.
const PluginViewKindIframe = "iframe"

// PluginViewKindLit identifies a lit-element rendered plugin view —
// the custom-element tag registered in the shell's customElements
// registry. First-party only in v1 per RFC.plugin-views §5.2.
const PluginViewKindLit = "lit"

// reservedFirstPartyPluginCode names the plugin code reserved for
// first-party bundles. A third-party manifest claiming this code is
// rejected at validation time per RFC.plugin-views §5.2 (CRIT-2).
const reservedFirstPartyPluginCode = "lthn"

// maxPluginViewsPerManifest caps the number of view registrations a
// single plugin may declare per RFC.plugin-views §9 / Cerberus
// HIGH-3. Plugins needing more surfaces use sub-pane navigation
// inside one view rather than registering many top-level views.
const maxPluginViewsPerManifest = 8

// reservedViewIDs names the built-in view ids the lthn shell ships.
// A plugin manifest declaring any of these is rejected at validation
// time per RFC.plugin-views §2 identity rules. Values must stay in
// lockstep with frontend/src/lit/app-shell.ts VIEWS registry ids.
var reservedViewIDs = map[string]bool{
	"admin":      true,
	"planning":   true,
	"coding":     true,
	"marketing":  true,
	"operations": true,
	"sales":      true,
	"office":     true,
}

// knownPluginViewCapabilities is the closed set of capability names a
// plugin view may declare. Unknown capabilities REJECT the manifest
// per RFC.plugin-views §9 + Cerberus Q4 / MED-2 — this forecloses
// the silent-enable-on-upgrade trap where a manifest declaring
// `cap: vi-events` becomes silently honoured the day the host learns
// the capability.
var knownPluginViewCapabilities = map[string]bool{
	"session-token": true,
	"vi-events":     true,
	"marketplace":   true,
}

// rejectionReasonThirdPartyLitPending is the documented reason code
// emitted when validation rejects a third-party `kind: lit` manifest
// pending the @dappcore/ui plugin-mediation surface per
// RFC.plugin-views §5.2 forward-contract (CRIT-2). The string is
// part of the audit log + marketplace-UI rejection surface contract;
// callers grep for this literal.
const rejectionReasonThirdPartyLitPending = "third-party-lit-pending-dappcore-ui-mediation"

// RouteEntry declares a navigation entry in the lthn sidebar.
type RouteEntry struct {
	Title   string `yaml:"title"`
	Icon    string `yaml:"icon,omitempty"`
	Group   string `yaml:"group,omitempty"`  // nav group e.g. "extend"
	Target  string `yaml:"target,omitempty"` // supports ${expose.<id>.route}
	OpenAfterInstall bool `yaml:"open_after_install,omitempty"`
}

// CommandEntry declares a Core command the bundle registers.
type CommandEntry struct {
	ID    string `yaml:"id"`
	Title string `yaml:"title"`
	Runs  string `yaml:"runs"` // supports route:${expose.<id>.route}
}

// SettingEntry declares a user-visible setting persisted to go-store.
type SettingEntry struct {
	Key     string `yaml:"key"`
	Type    string `yaml:"type"`    // "string" | "secret" | "bool" | "int"
	Prompt  string `yaml:"prompt"`
	Default string `yaml:"default,omitempty"` // supports ${random.password(N)}
}

// EnvEntry declares an install-time env-var prompt shown to the user.
type EnvEntry struct {
	Key     string `yaml:"key"`
	Prompt  string `yaml:"prompt"`
	Type    string `yaml:"type"`              // "string" | "secret"
	Default string `yaml:"default,omitempty"` // supports ${random.password(N)}
}

// Permission declares one gateway scope the bundle may access at runtime.
type Permission struct {
	Scope  string `yaml:"scope"`  // e.g. "project.metadata"
	Mode   string `yaml:"mode"`   // "read" | "write" | "invoke"
	Reason string `yaml:"reason"` // plain-language shown to user at install
}

// ParseManifest reads a manifest.yml from disk, unmarshals, and validates it.
//
// Usage example:
//
//	r := marketplace.ParseManifest("/path/to/manifest.yml")
//	m := r.Value.(marketplace.BundleManifest)
func ParseManifest(path string) core.Result {
	read := core.ReadFile(path)
	if !read.OK {
		return core.Fail(core.E(parseManifestOp, "read failed: "+path, nil))
	}
	raw, _ := read.Value.([]byte)
	return parseManifestBytes(raw)
}

// ParseManifestBytes parses a manifest from raw YAML bytes.
// Useful for manifests loaded from go-embed or network fetch.
//
// Usage example:
//
//	r := marketplace.ParseManifestBytes(yamlBytes)
//	m := r.Value.(marketplace.BundleManifest)
func ParseManifestBytes(raw []byte) core.Result {
	return parseManifestBytes(raw)
}

func parseManifestBytes(raw []byte) core.Result {
	var m BundleManifest
	if err := yaml.Unmarshal(raw, &m); err != nil {
		return core.Fail(core.E(parseManifestOp, "yaml parse failed", err))
	}
	return ValidateManifest(m)
}

// ValidateManifest validates a BundleManifest without I/O.
// Returns the manifest unchanged on success.
//
// Usage example:
//
//	r := marketplace.ValidateManifest(m)
//	if !r.OK { return r }
func ValidateManifest(m BundleManifest) core.Result {
	if core.Trim(m.Schema) != manifestSchema {
		return core.Fail(core.E(validateOp,
			"schema must be \""+manifestSchema+"\"; got: "+m.Schema, nil))
	}
	if core.Trim(m.Name) == "" {
		return core.Fail(core.E(validateOp, "name is required", nil))
	}
	if !isValidBundleName(m.Name) {
		return core.Fail(core.E(validateOp,
			"name must be alphanumeric + dash only: "+m.Name, nil))
	}
	// Mantis #1580 — gate the effective plugin code against
	// paths.IsValidPluginCode BEFORE any path concat or registry
	// insert. The marketplace surface used to skip this check while
	// pkg/plugin (binary plugins) enforced it from Cerberus #1436;
	// the gap let a marketplace-declared plugin.code like
	// "../../etc/passwd" or "-rf" escape directory basenames + URL
	// route segments. Gate uses the shared paths.IsValidPluginCode
	// extracted to pkg/paths/id_validation.go so the rule lives in
	// one place across both consumers.
	if m.Plugin != nil && core.Trim(m.Plugin.Code) != "" {
		if !paths.IsValidPluginCode(core.Trim(m.Plugin.Code)) {
			return core.Fail(core.E(validateOp,
				"invalid plugin.code (must be alphanumeric basename, 1-64 chars, leading alnum, no path separators): "+m.Plugin.Code, nil))
		}
	}
	if len(m.Images) == 0 {
		return core.Fail(core.E(validateOp, "at least one image entry is required", nil))
	}
	for i, img := range m.Images {
		if core.Trim(img.ID) == "" {
			return core.Fail(core.E(validateOp,
				core.Sprintf("images[%d].id is required", i), nil))
		}
		if core.Trim(img.Image) == "" {
			return core.Fail(core.E(validateOp,
				core.Sprintf("images[%d].image is required", i), nil))
		}
		// Cerberus Mantis #1448 — registry-domain allowlist. Without
		// this, a malicious manifest could declare an image from any
		// registry the host can reach (a typo-squatted Docker Hub repo,
		// a hostile registry that serves a different binary, etc).
		// Mantis #1667 — the validator moved to pkg/imagetrust so
		// sandbox + bridge consult the SAME allowlist as marketplace.
		// Typed-error rejection lets the validate chain surface the
		// reason (registry-not-allowed vs IMDS vs malformed-digest etc)
		// per RFC v1.1 §2.4.
		if err := imagetrust.IsAllowedImage(img.Image); err != nil {
			return core.Fail(core.E(validateOp,
				core.Sprintf("images[%d].image %s: %s", i, img.Image, err.Error()), err))
		}
	}
	if r := validatePluginViews(m); !r.OK {
		return r
	}
	return core.Ok(m)
}

// validatePluginViews enforces the manifest views[] block contract
// per plans/code/lthn/desktop/views/RFC.plugin-views.md §9.
//
// Rejections:
//   - reserved first-party code claimed by a third-party manifest
//     (CRIT-2 — first-party-tier protection)
//   - views[] count over maxPluginViewsPerManifest (HIGH-3)
//   - view id matching a reserved built-in id (admin, planning, …)
//   - view id failing isValidPluginViewID (alnum + _-: only, must
//     equal plugin code OR start with `<plugin-code>:`)
//   - iframe view source NOT referencing a declared ${expose.<id>.route}
//   - iframe view source naming an expose id the manifest doesn't
//     declare (cross-cut to the manifest's actual image expose blocks)
//   - lit view source failing custom-element-tag validation
//   - lit view from a third-party plugin (forward-contract per §5.2)
//   - capabilities entry outside knownPluginViewCapabilities (Q4)
//   - unknown / missing kind discriminator
func validatePluginViews(m BundleManifest) core.Result {
	if m.Plugin == nil || len(m.Plugin.Views) == 0 {
		return core.Ok(m)
	}
	code := pluginCodeOf(m)
	if code == "" {
		return core.Fail(core.E(validateOp,
			"plugin.views[] declared but plugin.code (or manifest.name) is empty", nil))
	}
	if !isFirstPartyPluginCode(code) && code == reservedFirstPartyPluginCode {
		return core.Fail(core.E(validateOp,
			"plugin.code \""+reservedFirstPartyPluginCode+
				"\" is reserved for first-party bundles", nil))
	}
	if len(m.Plugin.Views) > maxPluginViewsPerManifest {
		return core.Fail(core.E(validateOp,
			core.Sprintf("plugin.views[] exceeds cap of %d entries (got %d)",
				maxPluginViewsPerManifest, len(m.Plugin.Views)), nil))
	}
	seen := map[string]bool{}
	for i, v := range m.Plugin.Views {
		id := core.Trim(v.ID)
		if id == "" {
			return core.Fail(core.E(validateOp,
				core.Sprintf("plugin.views[%d].id is required", i), nil))
		}
		if reservedViewIDs[id] {
			return core.Fail(core.E(validateOp,
				core.Sprintf("plugin.views[%d].id is reserved built-in: %s", i, id), nil))
		}
		if !isValidPluginViewID(id, code) {
			return core.Fail(core.E(validateOp,
				core.Sprintf("plugin.views[%d].id must equal %q or start with %q (alnum + _-: only): %s",
					i, code, code+":", id), nil))
		}
		if seen[id] {
			return core.Fail(core.E(validateOp,
				core.Sprintf("plugin.views[%d].id duplicate within manifest: %s", i, id), nil))
		}
		seen[id] = true
		if core.Trim(v.Label) == "" {
			return core.Fail(core.E(validateOp,
				core.Sprintf("plugin.views[%d].label is required", i), nil))
		}
		switch v.Kind {
		case PluginViewKindIframe:
			if r := validateIframeViewSource(m, v, i); !r.OK {
				return r
			}
		case PluginViewKindLit:
			if !isFirstPartyPluginCode(code) {
				return core.Fail(core.E(validateOp,
					core.Sprintf("plugin.views[%d].kind=\"lit\" rejected: %s (third-party)",
						i, rejectionReasonThirdPartyLitPending), nil))
			}
			if !isValidCustomElementTag(v.Source) {
				return core.Fail(core.E(validateOp,
					core.Sprintf("plugin.views[%d].source must be a valid custom-element tag "+
						"(lowercase, contains -, no path chars): %s", i, v.Source), nil))
			}
		case "":
			return core.Fail(core.E(validateOp,
				core.Sprintf("plugin.views[%d].kind is required (\"iframe\" or \"lit\")", i), nil))
		default:
			return core.Fail(core.E(validateOp,
				core.Sprintf("plugin.views[%d].kind unknown: %s (allowed: iframe, lit)",
					i, v.Kind), nil))
		}
		for j, cap := range v.Capabilities {
			capID := core.Trim(cap)
			if !knownPluginViewCapabilities[capID] {
				return core.Fail(core.E(validateOp,
					core.Sprintf("plugin.views[%d].capabilities[%d] unknown: %s "+
						"(allowed: session-token, vi-events, marketplace)",
						i, j, cap), nil))
			}
		}
	}
	return core.Ok(m)
}

// pluginCodeOf returns the effective plugin code — the explicit
// plugin.code when set, falling back to the manifest top-level name
// for backward compatibility with manifests that predate the
// plugin.code field.
func pluginCodeOf(m BundleManifest) string {
	if m.Plugin != nil && core.Trim(m.Plugin.Code) != "" {
		return core.Trim(m.Plugin.Code)
	}
	return core.Trim(m.Name)
}

// isFirstPartyPluginCode reports whether the plugin code identifies
// a first-party bundle (tier-0 lit-kind acceptance per
// RFC.plugin-views §5.2). v1 treats reservedFirstPartyPluginCode as
// the only first-party marker; a follow-up sweep wires the binary-
// signing identity gate alongside.
func isFirstPartyPluginCode(code string) bool {
	return core.Trim(code) == reservedFirstPartyPluginCode
}

// isValidPluginViewID enforces the §2 identity rule: id must equal
// the plugin code OR start with `<plugin-code>:`. Characters allowed
// are lowercase alnum + underscore + dash + colon. Uppercase, dots,
// slashes, spaces are rejected.
func isValidPluginViewID(id, pluginCode string) bool {
	if id == "" || pluginCode == "" {
		return false
	}
	if id != pluginCode && !core.HasPrefix(id, pluginCode+":") {
		return false
	}
	for _, c := range id {
		ok := (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') ||
			c == '_' || c == '-' || c == ':'
		if !ok {
			return false
		}
	}
	return true
}

// isValidCustomElementTag enforces the W3C custom-element-name
// shape: ASCII lowercase, must contain at least one `-`, must start
// with [a-z], and contain only [a-z0-9-]. No dots, slashes, colons,
// or other path-shaped characters. Mirrors the platform's own
// CustomElementRegistry.define rejection rules.
func isValidCustomElementTag(tag string) bool {
	tag = core.Trim(tag)
	if tag == "" {
		return false
	}
	if !core.Contains(tag, "-") {
		return false
	}
	if tag[0] < 'a' || tag[0] > 'z' {
		return false
	}
	for _, c := range tag {
		ok := (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-'
		if !ok {
			return false
		}
	}
	return true
}

// validateIframeViewSource enforces the §9 iframe-source rule: the
// source MUST be a `${expose.<id>.route}` placeholder referencing an
// expose block actually declared by one of the manifest's images.
// Literal URLs are rejected — the install-time resolver fills the
// loopback URL from the actual port bound at install.
func validateIframeViewSource(m BundleManifest, v PluginView, i int) core.Result {
	src := core.Trim(v.Source)
	const prefix = "${expose."
	const suffix = ".route}"
	if !core.HasPrefix(src, prefix) || !core.HasSuffix(src, suffix) {
		return core.Fail(core.E(validateOp,
			core.Sprintf("plugin.views[%d].source must reference ${expose.<id>.route} (got: %s)",
				i, v.Source), nil))
	}
	exposeID := src[len(prefix) : len(src)-len(suffix)]
	if exposeID == "" {
		return core.Fail(core.E(validateOp,
			core.Sprintf("plugin.views[%d].source ${expose.<id>.route} has empty id", i), nil))
	}
	for _, img := range m.Images {
		if img.ID == exposeID && img.Expose != nil {
			return core.Ok(m)
		}
	}
	return core.Fail(core.E(validateOp,
		core.Sprintf("plugin.views[%d].source references undeclared expose id: %s "+
			"(no image has id=%q with expose block)", i, exposeID, exposeID), nil))
}

// MarshalManifest serialises a BundleManifest back to YAML bytes.
//
// Usage example:
//
//	r := marketplace.MarshalManifest(m)
//	bytes := r.Value.([]byte)
func MarshalManifest(m BundleManifest) core.Result {
	raw, err := yaml.Marshal(m)
	if err != nil {
		return core.Fail(core.E("marketplace.MarshalManifest", "yaml marshal failed", err))
	}
	return core.Ok(raw)
}

// isValidBundleName returns true when name contains only a-z, 0-9, and dash.
// Upper-case, underscore, dots, and spaces are rejected — bundle names are
// used as directory names and URL path segments so they must be URL-safe.
func isValidBundleName(name string) bool {
	for _, c := range name {
		ok := (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-'
		if !ok {
			return false
		}
	}
	return len(name) > 0
}
