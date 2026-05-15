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
}

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
	}
	return core.Ok(m)
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
