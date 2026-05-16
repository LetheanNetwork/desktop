// SPDX-Licence-Identifier: EUPL-1.2

package marketplace_test

import (
	core "dappco.re/go"
	subject "dappco.re/lthn/desktop/pkg/marketplace"
)

const bundleVaultwardenYAML = `
schema: lthn-vm/v1
name: vaultwarden
display: Vaultwarden
description: Self-hosted Bitwarden-compatible password manager. Personal-infra shape — single binary, embedded SQLite, encrypted-at-rest vault. Vaultwarden's own master-password gate is the unlock primitive; lthn hosts the container.
category: personal
icon: fa-key
homepage: https://github.com/dani-garcia/vaultwarden
license: AGPL-3.0

images:
  - id: app
    image: vaultwarden/server:latest
    env:
      DOMAIN: ${env.PUBLIC_URL}
      ADMIN_TOKEN: ${env.ADMIN_TOKEN}
      SIGNUPS_ALLOWED: "false"
    volumes:
      - container: /data
        persist: vaultwarden-data
    expose:
      port: 80
      route: /vaultwarden

plugin:
  routes:
    - title: Vaultwarden
      icon: fa-key
      group: extend
      target: ${expose.app.route}
      open_after_install: true
  commands:
    - id: vaultwarden.open
      title: Open Vaultwarden
      runs: route:${expose.app.route}
    - id: vaultwarden.admin
      title: Open Vaultwarden admin panel
      runs: route:${expose.app.route}/admin
  settings:
    - key: signups.allowed
      type: bool
      prompt: Allow new user signups
      default: "false"

env:
  - key: PUBLIC_URL
    prompt: Public URL Vaultwarden advertises (used in invite + reset links)
    type: string
    default: http://lthn.local/vaultwarden/
  - key: ADMIN_TOKEN
    prompt: Admin panel token (long random string; required to reach /admin)
    type: secret
    default: ${random.password(48)}

permissions:
  - scope: service.notify
    mode: invoke
    reason: Notify on vault unlock prompts and security events.
`

// TestBundle_Vaultwarden_Good — personal-infra shape parses + validates.
// Single image, single volume, admin-token-as-secret pattern.
func TestBundle_Vaultwarden_Good(t *core.T) {
	r := subject.ParseManifestBytes([]byte(bundleVaultwardenYAML))
	core.RequireTrue(t, r.OK)
	m := r.Value.(subject.BundleManifest)
	core.AssertEqual(t, "vaultwarden", m.Name)
	core.AssertEqual(t, "personal", m.Category)
	core.AssertTrue(t, subject.ValidateManifest(m).OK)
}

// TestBundle_Vaultwarden_Bad — SIGNUPS_ALLOWED env preserves YAML
// boolean-as-string ("false"). Parser doesn't coerce types; the
// container env layer treats env values as strings always.
func TestBundle_Vaultwarden_Bad(t *core.T) {
	r := subject.ParseManifestBytes([]byte(bundleVaultwardenYAML))
	core.RequireTrue(t, r.OK)
	m := r.Value.(subject.BundleManifest)
	core.AssertEqual(t, "false", m.Images[0].Env["SIGNUPS_ALLOWED"])
}

// TestBundle_Vaultwarden_Ugly — two commands (open + admin) covering
// the same target but different sub-routes. Both round-trip with
// their full Runs targets including the path component after the
// substitution token.
func TestBundle_Vaultwarden_Ugly(t *core.T) {
	r := subject.ParseManifestBytes([]byte(bundleVaultwardenYAML))
	core.RequireTrue(t, r.OK)
	m := r.Value.(subject.BundleManifest)
	core.AssertLen(t, m.Plugin.Commands, 2)
	byID := map[string]string{}
	for _, cmd := range m.Plugin.Commands {
		byID[cmd.ID] = cmd.Runs
	}
	core.AssertEqual(t, "route:${expose.app.route}", byID["vaultwarden.open"])
	core.AssertEqual(t, "route:${expose.app.route}/admin", byID["vaultwarden.admin"])
}
