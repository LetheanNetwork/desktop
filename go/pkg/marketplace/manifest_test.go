// SPDX-Licence-Identifier: EUPL-1.2

package marketplace_test

import (
	core "dappco.re/go"
	subject "dappco.re/lthn/desktop/pkg/marketplace"
)

// minimalManifestYAML is the smallest valid manifest — single image, no plugin block.
const minimalManifestYAML = `
schema: lthn-vm/v1
name: test-bundle
display: Test Bundle
images:
  - id: app
    image: alpine:3.21
`

// fullManifestYAML exercises every optional block.
const fullManifestYAML = `
schema: lthn-vm/v1
name: opencode
display: OpenCode
description: Sandboxed coding agent.
category: ai-agents
icon: fa-robot
homepage: https://opencode.ai
license: MIT
images:
  - id: app
    image: lthn/dev:latest
    env:
      OPENCODE_SERVER_PASSWORD: ${env.SERVER_PASSWORD}
    volumes:
      - container: /data
        persist: opencode-data
    expose:
      port: 4096
      route: /opencode
plugin:
  routes:
    - title: OpenCode
      icon: fa-robot
      group: extend
      target: ${expose.app.route}
  commands:
    - id: opencode.open
      title: Open OpenCode
      runs: route:${expose.app.route}
  settings:
    - key: theme
      type: string
      prompt: UI theme
      default: dark
env:
  - key: SERVER_PASSWORD
    prompt: Server password
    type: secret
    default: ${random.password(32)}
permissions:
  - scope: project.metadata
    mode: read
    reason: List available projects.
`

// TestManifest_ParseManifestBytes_Good verifies a valid manifest round-trips
// through ParseManifestBytes with all fields populated correctly.
func TestManifest_ParseManifestBytes_Good(t *core.T) {
	r := subject.ParseManifestBytes([]byte(fullManifestYAML))
	core.RequireTrue(t, r.OK)
	m := r.Value.(subject.BundleManifest)

	core.AssertEqual(t, "opencode", m.Name)
	core.AssertEqual(t, "OpenCode", m.Display)
	core.AssertEqual(t, "lthn-vm/v1", m.Schema)
	core.AssertLen(t, m.Images, 1)
	core.AssertEqual(t, "app", m.Images[0].ID)
	core.AssertEqual(t, "lthn/dev:latest", m.Images[0].Image)
	core.AssertNotNil(t, m.Images[0].Expose)
	core.AssertEqual(t, 4096, m.Images[0].Expose.Port)
	core.AssertEqual(t, "/opencode", m.Images[0].Expose.Route)
	core.AssertNotNil(t, m.Plugin)
	core.AssertLen(t, m.Plugin.Routes, 1)
	core.AssertLen(t, m.Plugin.Commands, 1)
	core.AssertLen(t, m.Plugin.Settings, 1)
	core.AssertLen(t, m.Env, 1)
	core.AssertLen(t, m.Permissions, 1)
	core.AssertEqual(t, "project.metadata", m.Permissions[0].Scope)
}

// TestManifest_ParseManifestBytes_Bad verifies that each class of invalid
// manifest is rejected with a typed error.
func TestManifest_ParseManifestBytes_Bad(t *core.T) {
	cases := []struct {
		name  string
		yaml  string
		errIn string
	}{
		{
			name: "wrong schema version",
			yaml: `schema: lthn-vm/v2
name: test-bundle
images:
  - id: app
    image: alpine:3.21`,
			errIn: "schema must be",
		},
		{
			name: "missing name",
			yaml: `schema: lthn-vm/v1
images:
  - id: app
    image: alpine:3.21`,
			errIn: "name is required",
		},
		{
			name: "invalid name characters",
			yaml: `schema: lthn-vm/v1
name: Test_Bundle
images:
  - id: app
    image: alpine:3.21`,
			errIn: "alphanumeric",
		},
		{
			name: "no images",
			yaml: `schema: lthn-vm/v1
name: test-bundle`,
			errIn: "at least one image",
		},
		{
			name: "image missing id",
			yaml: `schema: lthn-vm/v1
name: test-bundle
images:
  - image: alpine:3.21`,
			errIn: "images[0].id",
		},
		{
			name: "image missing image ref",
			yaml: `schema: lthn-vm/v1
name: test-bundle
images:
  - id: app`,
			errIn: "images[0].image",
		},
	}

	for _, tc := range cases {
		r := subject.ParseManifestBytes([]byte(tc.yaml))
		core.AssertFalse(t, r.OK)
		core.AssertContains(t, r.Error(), tc.errIn)
	}
}

// TestManifest_ParseManifestBytes_Ugly verifies edge cases: minimal manifest
// parses, ${...} substitution tokens are preserved as-is, and
// MarshalManifest round-trips back to parseable YAML.
func TestManifest_ParseManifestBytes_Ugly(t *core.T) {
	// Minimal manifest — no optional blocks.
	r := subject.ParseManifestBytes([]byte(minimalManifestYAML))
	core.RequireTrue(t, r.OK)
	m := r.Value.(subject.BundleManifest)
	core.AssertNil(t, m.Plugin)
	core.AssertLen(t, m.Env, 0)
	core.AssertLen(t, m.Permissions, 0)

	// ${...} tokens preserved — parser does not resolve substitutions.
	r2 := subject.ParseManifestBytes([]byte(fullManifestYAML))
	core.RequireTrue(t, r2.OK)
	m2 := r2.Value.(subject.BundleManifest)
	core.AssertEqual(t, "${env.SERVER_PASSWORD}", m2.Images[0].Env["OPENCODE_SERVER_PASSWORD"])
	core.AssertEqual(t, "${expose.app.route}", m2.Plugin.Routes[0].Target)

	// Round-trip: marshal → parse produces equivalent struct.
	marshalR := subject.MarshalManifest(m2)
	core.RequireTrue(t, marshalR.OK)
	raw, _ := marshalR.Value.([]byte)
	r3 := subject.ParseManifestBytes(raw)
	core.RequireTrue(t, r3.OK)
	m3 := r3.Value.(subject.BundleManifest)
	core.AssertEqual(t, m2.Name, m3.Name)
	core.AssertEqual(t, m2.Images[0].Image, m3.Images[0].Image)
}

// TestManifest_ValidateManifest_Good verifies ValidateManifest on a
// pre-populated struct (no I/O path).
func TestManifest_ValidateManifest_Good(t *core.T) {
	m := subject.BundleManifest{
		Schema:  "lthn-vm/v1",
		Name:    "my-bundle",
		Display: "My Bundle",
		Images: []subject.ImageEntry{
			{ID: "app", Image: "alpine:3.21"},
		},
	}
	r := subject.ValidateManifest(m)
	core.RequireTrue(t, r.OK)
}

// TestManifest_ValidateManifest_Bad confirms that nil/empty struct is rejected.
func TestManifest_ValidateManifest_Bad(t *core.T) {
	r := subject.ValidateManifest(subject.BundleManifest{})
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "schema must be")
}

// TestManifest_ValidateManifest_Ugly verifies names with dots and spaces are
// rejected; dash is permitted; numeric-only name is permitted.
func TestManifest_ValidateManifest_Ugly(t *core.T) {
	base := subject.BundleManifest{
		Schema: "lthn-vm/v1",
		Images: []subject.ImageEntry{{ID: "app", Image: "alpine:3.21"}},
	}

	cases := []struct {
		name  string
		valid bool
	}{
		{"opencode", true},
		{"my-bundle", true},
		{"bundle123", true},
		{"123", true},
		{"My-Bundle", false},    // upper-case
		{"my_bundle", false},    // underscore
		{"my.bundle", false},    // dot
		{"my bundle", false},    // space
		{"", false},             // empty
	}

	for _, tc := range cases {
		m := base
		m.Name = tc.name
		r := subject.ValidateManifest(m)
		if tc.valid {
			core.AssertTrue(t, r.OK)
		} else {
			core.AssertFalse(t, r.OK)
		}
	}
}
