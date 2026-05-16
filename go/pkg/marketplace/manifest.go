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
		// Cerberus Mantis #1448 — registry-domain allowlist. Without
		// this, a malicious manifest could declare an image from any
		// registry the host can reach (a typo-squatted Docker Hub repo,
		// a hostile registry that serves a different binary, etc).
		// IsAllowedImageRegistry is permissive on intent (covers the
		// big-tent OSS registries) but strict on shape (must parse).
		if !IsAllowedImageRegistry(img.Image) {
			return core.Fail(core.E(validateOp,
				core.Sprintf("images[%d].image registry not allowed: %s "+
					"(allowed: docker.io / ghcr.io / quay.io / gcr.io / "+
					"mcr.microsoft.com / registry.gitlab.com / lscr.io / "+
					"forge.lthn.sh / lthn)", i, img.Image), nil))
		}
	}
	return core.Ok(m)
}

// allowedImageRegistries is the compile-time allowlist of OCI
// registry domains a bundle's image may pull from when the image
// reference includes an explicit registry. Cerberus Mantis #1448.
// Same compile-time discipline as pkg/downloader.allowedHostSuffixes
// (#1424) — runtime mutation would defeat the gate, so updates ship
// as code review.
//
// Big-tent OSS coverage: Docker Hub (registry-explicit form),
// GitHub Container Registry, Red Hat Quay, Google Container
// Registry, Microsoft, GitLab, LinuxServer.io (popular in self-host
// bundles), Codeberg (Forgejo-hosted), and our own Forge.
//
// For Docker Hub shorthand (bare `<org>/<name>` without registry
// prefix), see IsAllowedImageRegistry's policy: bare orgs are
// allowed because forcing every manifest to fully-qualify Docker
// Hub refs would break the existing OSS bundle catalogue. The
// supply-chain trust there is on the user reading the manifest
// before install — the gate's value is stopping silent pulls from
// attacker-controlled or typo-squatted REGISTRY DOMAINS, not
// auditing every Docker Hub user-namespace.
var allowedImageRegistries = []string{
	"docker.io",
	"index.docker.io",
	"ghcr.io",
	"quay.io",
	"gcr.io",
	"mcr.microsoft.com",
	"registry.gitlab.com",
	"lscr.io",
	"codeberg.org",
	"forge.lthn.sh",
}

// IsAllowedImageRegistry reports whether `image` references a
// registry on the allowlist. Three image-ref shapes are handled:
//
//	<registry>/<path>[:tag][@digest]   e.g. ghcr.io/owner/img:1.0
//	<org>/<name>[:tag][@digest]        e.g. n8nio/n8n:latest     → Docker Hub-implicit (allowed)
//	<name>[:tag][@digest]              e.g. nginx                → docker.io/library/nginx-implicit (allowed)
//
// We treat the first slash-segment as a REGISTRY domain only when
// it contains a `.` or `:` (the canonical OCI distinguisher between
// "registry domain" and "Docker Hub org"). Registry domains must be
// in allowedImageRegistries. Docker Hub orgs (no dot/colon in first
// segment) are allowed unconditionally — see allowedImageRegistries
// docstring for the supply-chain framing.
func IsAllowedImageRegistry(image string) bool {
	image = core.Trim(image)
	if image == "" {
		return false
	}
	// Strip @digest + :tag for the host check.
	if i := core.LastIndex(image, "@"); i > 0 {
		image = image[:i]
	}
	if i := core.LastIndex(image, ":"); i > 0 {
		// Only strip the :tag if it doesn't span a `/` (else this is
		// a registry-with-port like `registry.gitlab.com:443/...`).
		tail := image[i:]
		if !core.Contains(tail, "/") {
			image = image[:i]
		}
	}
	slash := core.Index(image, "/")
	if slash < 0 {
		// `nginx` / `alpine` / `postgres` — Docker Hub library/*
		// shorthand. Library is Docker's own curated namespace of
		// official images.
		return true
	}
	first := image[:slash]
	// If the first segment contains . or :, it's a registry domain.
	if core.Contains(first, ".") || core.Contains(first, ":") {
		// Strip :port if present so `registry.gitlab.com:443` matches
		// `registry.gitlab.com` in the allowlist.
		host := first
		if i := core.Index(host, ":"); i > 0 {
			host = host[:i]
		}
		for _, allowed := range allowedImageRegistries {
			if host == allowed {
				return true
			}
		}
		return false
	}
	// Otherwise it's a Docker Hub org shorthand — allowed (see
	// allowedImageRegistries docstring for the framing).
	return true
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
