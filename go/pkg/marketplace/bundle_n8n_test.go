// SPDX-Licence-Identifier: EUPL-1.2

package marketplace_test

import (
	core "dappco.re/go"
	subject "dappco.re/lthn/desktop/pkg/marketplace"
)

const bundleN8nYAML = `
schema: lthn-vm/v1
name: n8n
display: n8n
description: Workflow automation — visual flow editor, 400+ integrations, agent-friendly webhook + HTTP nodes. Pairs naturally with the Lethean Agent + Agentic surfaces for autonomous task orchestration.
category: automation
icon: fa-diagram-project
homepage: https://n8n.io
license: Sustainable-Use-License

images:
  - id: app
    image: n8nio/n8n:latest
    env:
      N8N_HOST: ${env.PUBLIC_HOST}
      N8N_PORT: "5678"
      N8N_PROTOCOL: ${env.PROTOCOL}
      WEBHOOK_URL: ${env.PUBLIC_URL}
      GENERIC_TIMEZONE: ${env.TIMEZONE}
    volumes:
      - container: /home/node/.n8n
        persist: n8n-data
    expose:
      port: 5678
      route: /n8n

plugin:
  routes:
    - title: n8n
      icon: fa-diagram-project
      group: extend
      target: ${expose.app.route}
      open_after_install: true
  commands:
    - id: n8n.open
      title: Open n8n
      runs: route:${expose.app.route}
  settings:
    - key: timezone
      type: string
      prompt: Default timezone for scheduled workflows
      default: UTC

env:
  - key: PUBLIC_HOST
    prompt: Hostname n8n advertises in webhook URLs
    type: string
    default: lthn.local
  - key: PROTOCOL
    prompt: URL protocol (http or https)
    type: string
    default: http
  - key: PUBLIC_URL
    prompt: Full public URL n8n uses for webhook callbacks
    type: string
    default: http://lthn.local/n8n/
  - key: TIMEZONE
    prompt: Timezone for scheduled workflows
    type: string
    default: UTC
`

// TestBundle_N8n_Good — workflow automation bundle parses + validates.
// No permissions block — n8n is purely UI-driven, doesn't need
// gateway scope access for v1.
func TestBundle_N8n_Good(t *core.T) {
	r := subject.ParseManifestBytes([]byte(bundleN8nYAML))
	core.RequireTrue(t, r.OK)
	m := r.Value.(subject.BundleManifest)
	core.AssertEqual(t, "n8n", m.Name)
	core.AssertEqual(t, "automation", m.Category)
	core.AssertTrue(t, subject.ValidateManifest(m).OK)
}

// TestBundle_N8n_Bad — empty Permissions block is a valid shape
// (per RFC §7a: "Bundles without explicit permissions: get the
// empty-set default (gateway responds to no scopes — the plugin
// can run, but cannot read or write user data; useful for purely
// UI / visualisation plugins that consume only their own state)").
func TestBundle_N8n_Bad(t *core.T) {
	r := subject.ParseManifestBytes([]byte(bundleN8nYAML))
	core.RequireTrue(t, r.OK)
	m := r.Value.(subject.BundleManifest)
	core.AssertLen(t, m.Permissions, 0)
}

// TestBundle_N8n_Ugly — four env entries (PUBLIC_HOST, PROTOCOL,
// PUBLIC_URL, TIMEZONE) all reach the container as ${env.*} tokens
// at parse time. The substitution resolves at install-time when the
// user fills in (or accepts defaults for) each.
func TestBundle_N8n_Ugly(t *core.T) {
	r := subject.ParseManifestBytes([]byte(bundleN8nYAML))
	core.RequireTrue(t, r.OK)
	m := r.Value.(subject.BundleManifest)
	core.AssertLen(t, m.Env, 4)

	envKeys := map[string]bool{}
	for _, e := range m.Env {
		envKeys[e.Key] = true
	}
	core.AssertTrue(t, envKeys["PUBLIC_HOST"])
	core.AssertTrue(t, envKeys["PROTOCOL"])
	core.AssertTrue(t, envKeys["PUBLIC_URL"])
	core.AssertTrue(t, envKeys["TIMEZONE"])

	// All four ${env.*} tokens preserved literal in the image env block.
	core.AssertEqual(t, "${env.PUBLIC_HOST}", m.Images[0].Env["N8N_HOST"])
	core.AssertEqual(t, "${env.TIMEZONE}", m.Images[0].Env["GENERIC_TIMEZONE"])
}
