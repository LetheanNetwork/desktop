// SPDX-Licence-Identifier: EUPL-1.2

package marketplace_test

import (
	core "dappco.re/go"
	subject "dappco.re/lthn/desktop/pkg/marketplace"
)

// bundleOpenCodeYAML is the canonical OpenCode bundle manifest. Mirrors
// bundles/opencode/manifest.yml — keep in sync if the file changes.
const bundleOpenCodeYAML = `
schema: lthn-vm/v1
name: opencode
display: OpenCode
description: Sandboxed AI coding agent with 75+ provider adapters. Runs inside lthn-vm — no host install required.
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
      - container: /workspace
        persist: opencode-workspace
    expose:
      port: 4096
      route: /opencode

plugin:
  code: opencode
  routes:
    - title: OpenCode
      icon: fa-robot
      group: extend
      target: ${expose.app.route}
      open_after_install: true
  commands:
    - id: opencode.open
      title: Open OpenCode
      runs: route:${expose.app.route}
  settings:
    - key: theme
      type: string
      prompt: UI theme
      default: dark
  views:
    - id: opencode
      label: OpenCode
      icon: fa-robot
      kind: iframe
      source: ${expose.app.route}
      capabilities:
        - session-token

env:
  - key: SERVER_PASSWORD
    prompt: Server password (used by OpenCode to authenticate requests)
    type: secret
    default: ${random.password(32)}

permissions:
  - scope: project.metadata
    mode: read
    reason: List available projects in the workspace picker.
  - scope: project.outputs
    mode: write
    reason: Save agent outputs next to source files.
`

// TestBundle_OpenCode_Good verifies the OpenCode bundle manifest parses
// and validates correctly — this is the demo proof per RFC.marketplace.md §8.1.
func TestBundle_OpenCode_Good(t *core.T) {
	r := subject.ParseManifestBytes([]byte(bundleOpenCodeYAML))
	core.RequireTrue(t, r.OK)

	m := r.Value.(subject.BundleManifest)
	core.AssertEqual(t, "opencode", m.Name)
	core.AssertEqual(t, "OpenCode", m.Display)
	core.AssertEqual(t, "lthn-vm/v1", m.Schema)
	core.AssertEqual(t, "ai-agents", m.Category)

	// Single image.
	core.AssertLen(t, m.Images, 1)
	img := m.Images[0]
	core.AssertEqual(t, "app", img.ID)
	core.AssertEqual(t, "lthn/dev:latest", img.Image)

	// Expose block — port + route.
	core.AssertNotNil(t, img.Expose)
	core.AssertEqual(t, 4096, img.Expose.Port)
	core.AssertEqual(t, "/opencode", img.Expose.Route)

	// Plugin block.
	core.AssertNotNil(t, m.Plugin)
	core.AssertEqual(t, "opencode", m.Plugin.Code)
	core.AssertLen(t, m.Plugin.Routes, 1)
	core.AssertEqual(t, "OpenCode", m.Plugin.Routes[0].Title)
	core.AssertLen(t, m.Plugin.Commands, 1)
	core.AssertEqual(t, "opencode.open", m.Plugin.Commands[0].ID)
	core.AssertLen(t, m.Plugin.Settings, 1)
	core.AssertEqual(t, "theme", m.Plugin.Settings[0].Key)

	// Views block — Unit C addition per plans/code/lthn/desktop/views/RFC.plugin-views.md §9.
	// The manifest declares one iframe-kind view backed by the app expose
	// block; the session-token capability triggers the §5.5 install prompt
	// + §5.1 postMessage handshake.
	core.AssertLen(t, m.Plugin.Views, 1)
	v := m.Plugin.Views[0]
	core.AssertEqual(t, "opencode", v.ID)
	core.AssertEqual(t, "OpenCode", v.Label)
	core.AssertEqual(t, "fa-robot", v.Icon)
	core.AssertEqual(t, subject.PluginViewKindIframe, v.Kind)
	core.AssertEqual(t, "${expose.app.route}", v.Source)
	core.AssertLen(t, v.Capabilities, 1)
	core.AssertEqual(t, "session-token", v.Capabilities[0])

	// Env block.
	core.AssertLen(t, m.Env, 1)
	core.AssertEqual(t, "SERVER_PASSWORD", m.Env[0].Key)
	core.AssertEqual(t, "secret", m.Env[0].Type)

	// Permissions.
	core.AssertLen(t, m.Permissions, 2)
	core.AssertEqual(t, "project.metadata", m.Permissions[0].Scope)
	core.AssertEqual(t, "read", m.Permissions[0].Mode)
	core.AssertEqual(t, "project.outputs", m.Permissions[1].Scope)
	core.AssertEqual(t, "write", m.Permissions[1].Mode)
}

// TestBundle_OpenCode_Bad verifies env token ${env.SERVER_PASSWORD} is
// preserved as-is (parser does not resolve substitutions).
func TestBundle_OpenCode_Bad(t *core.T) {
	r := subject.ParseManifestBytes([]byte(bundleOpenCodeYAML))
	core.RequireTrue(t, r.OK)
	m := r.Value.(subject.BundleManifest)
	core.AssertEqual(t, "${env.SERVER_PASSWORD}", m.Images[0].Env["OPENCODE_SERVER_PASSWORD"])
}

// TestBundle_OpenCode_Ugly verifies the manifest round-trips through
// MarshalManifest → ParseManifestBytes cleanly.
func TestBundle_OpenCode_Ugly(t *core.T) {
	r := subject.ParseManifestBytes([]byte(bundleOpenCodeYAML))
	core.RequireTrue(t, r.OK)
	m := r.Value.(subject.BundleManifest)

	marshalR := subject.MarshalManifest(m)
	core.RequireTrue(t, marshalR.OK)
	raw, _ := marshalR.Value.([]byte)

	r2 := subject.ParseManifestBytes(raw)
	core.RequireTrue(t, r2.OK)
	m2 := r2.Value.(subject.BundleManifest)

	core.AssertEqual(t, m.Name, m2.Name)
	core.AssertEqual(t, m.Images[0].Image, m2.Images[0].Image)
	core.AssertEqual(t, m.Images[0].Expose.Port, m2.Images[0].Expose.Port)
	core.AssertLen(t, m2.Permissions, 2)
}
