// SPDX-Licence-Identifier: EUPL-1.2

package marketplace_test

import (
	core "dappco.re/go"
	subject "dappco.re/lthn/desktop/pkg/marketplace"
)

// bundleOdysseusYAML is the canonical Odysseus bundle manifest. Mirrors
// bundles/odysseus/manifest.yml — keep in sync if the file changes.
const bundleOdysseusYAML = `
schema: lthn-vm/v1
name: odysseus
display: Odysseus
description: Self-hosted AI workspace — chat, deep research, agents, documents — running contained in lthn-vm. Your data stays local; the model stays on the host.
category: ai-agents
icon: fa-compass
homepage: https://github.com/pewdiepie-archdaemon/odysseus
license: MIT

images:
  - id: app
    image: lthn/odysseus:latest
    env:
      APP_BIND: 0.0.0.0
      APP_PORT: "7000"
      AUTH_ENABLED: "true"
      SECURE_COOKIES: "true"
      LOCALHOST_BYPASS: "false"
      ODYSSEUS_ADMIN_USER: admin
      ODYSSEUS_ADMIN_PASSWORD: ${env.ADMIN_PASSWORD}
      LLM_HOST: ${env.LLM_HOST}
      CHROMADB_HOST: localhost
      SEARXNG_INSTANCE: http://localhost:8080
      DATABASE_URL: sqlite:///./data/app.db
    volumes:
      - container: /app/data
        persist: odysseus-data
    expose:
      port: 7000
      route: /odysseus

plugin:
  code: odysseus
  routes:
    - title: Odysseus
      icon: fa-compass
      group: extend
      target: ${expose.app.route}
      open_after_install: true
  commands:
    - id: odysseus.open
      title: Open Odysseus
      runs: route:${expose.app.route}
  settings:
    - key: theme
      type: string
      prompt: UI theme
      default: dark
  views:
    - id: odysseus
      label: Odysseus
      icon: fa-compass
      kind: iframe
      source: ${expose.app.route}
      capabilities:
        - session-token

env:
  - key: ADMIN_PASSWORD
    prompt: Odysseus admin password (first-boot admin login)
    type: secret
    default: ${random.password(32)}
  - key: LLM_HOST
    prompt: Local model endpoint Odysseus should use (the host LEM Engine, or Ollama)
    type: string
    default: http://host.lthn.vm:11434

permissions:
  - scope: project.metadata
    mode: read
    reason: List workspace projects in Odysseus's file tools.
`

// TestBundle_Odysseus_Good verifies the Odysseus bundle manifest parses and
// validates — one sealed image (app + chroma + searxng baked) exposing its
// webui at /odysseus, rendered as an iframe view in the desktop panel.
func TestBundle_Odysseus_Good(t *core.T) {
	r := subject.ParseManifestBytes([]byte(bundleOdysseusYAML))
	core.RequireTrue(t, r.OK)

	m := r.Value.(subject.BundleManifest)
	core.AssertEqual(t, "odysseus", m.Name)
	core.AssertEqual(t, "Odysseus", m.Display)
	core.AssertEqual(t, "lthn-vm/v1", m.Schema)
	core.AssertEqual(t, "ai-agents", m.Category)

	// One sealed image — chroma + searxng are baked in, not sibling images.
	core.AssertLen(t, m.Images, 1)
	img := m.Images[0]
	core.AssertEqual(t, "app", img.ID)
	core.AssertEqual(t, "lthn/odysseus:latest", img.Image)

	// Expose — the app's webui port + route.
	core.AssertNotNil(t, img.Expose)
	core.AssertEqual(t, 7000, img.Expose.Port)
	core.AssertEqual(t, "/odysseus", img.Expose.Route)

	// Plugin block.
	core.AssertNotNil(t, m.Plugin)
	core.AssertEqual(t, "odysseus", m.Plugin.Code)
	core.AssertLen(t, m.Plugin.Routes, 1)
	core.AssertEqual(t, "Odysseus", m.Plugin.Routes[0].Title)
	core.AssertLen(t, m.Plugin.Commands, 1)
	core.AssertEqual(t, "odysseus.open", m.Plugin.Commands[0].ID)
	core.AssertLen(t, m.Plugin.Settings, 1)
	core.AssertEqual(t, "theme", m.Plugin.Settings[0].Key)

	// The iframe view — the webui rendered as a desktop panel.
	core.AssertLen(t, m.Plugin.Views, 1)
	v := m.Plugin.Views[0]
	core.AssertEqual(t, "odysseus", v.ID)
	core.AssertEqual(t, "Odysseus", v.Label)
	core.AssertEqual(t, "fa-compass", v.Icon)
	core.AssertEqual(t, subject.PluginViewKindIframe, v.Kind)
	core.AssertEqual(t, "${expose.app.route}", v.Source)
	core.AssertLen(t, v.Capabilities, 1)
	core.AssertEqual(t, "session-token", v.Capabilities[0])

	// Env — the admin secret + the host LLM endpoint.
	core.AssertLen(t, m.Env, 2)
	core.AssertEqual(t, "ADMIN_PASSWORD", m.Env[0].Key)
	core.AssertEqual(t, "secret", m.Env[0].Type)
	core.AssertEqual(t, "LLM_HOST", m.Env[1].Key)

	// Permissions.
	core.AssertLen(t, m.Permissions, 1)
	core.AssertEqual(t, "project.metadata", m.Permissions[0].Scope)
	core.AssertEqual(t, "read", m.Permissions[0].Mode)
}

// TestBundle_Odysseus_Bad verifies the ${env.ADMIN_PASSWORD} token is preserved
// as-is — the parser does not resolve substitutions (install-time does).
func TestBundle_Odysseus_Bad(t *core.T) {
	r := subject.ParseManifestBytes([]byte(bundleOdysseusYAML))
	core.RequireTrue(t, r.OK)
	m := r.Value.(subject.BundleManifest)
	core.AssertEqual(t, "${env.ADMIN_PASSWORD}", m.Images[0].Env["ODYSSEUS_ADMIN_PASSWORD"])
}

// TestBundle_Odysseus_Ugly verifies the manifest round-trips through
// MarshalManifest → ParseManifestBytes cleanly.
func TestBundle_Odysseus_Ugly(t *core.T) {
	r := subject.ParseManifestBytes([]byte(bundleOdysseusYAML))
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
	core.AssertLen(t, m2.Env, 2)
}
