// SPDX-Licence-Identifier: EUPL-1.2

package marketplace_test

import (
	core "dappco.re/go"
	subject "dappco.re/lthn/desktop/pkg/marketplace"
)

// bundleForgejoYAML is the canonical Forgejo bundle manifest. Mirrors
// bundles/forgejo/manifest.yml — keep in sync if the file changes.
// Long-running stateful service shape per RFC.marketplace §8.2.
const bundleForgejoYAML = `
schema: lthn-vm/v1
name: forgejo
display: Forgejo
description: Self-hosted git forge with web UI, issues, pull requests, and built-in CI runner support. Long-running stateful service — repos and config live in a persistent volume so restarts are clean.
category: dev-tools
icon: fa-code-branch
homepage: https://forgejo.org
license: GPL-3.0

images:
  - id: app
    image: codeberg.org/forgejo/forgejo:9
    env:
      USER_UID: "1000"
      USER_GID: "1000"
      FORGEJO__server__ROOT_URL: ${env.ROOT_URL}
      FORGEJO__server__SSH_DOMAIN: ${env.SSH_DOMAIN}
    volumes:
      - container: /var/lib/gitea
        persist: forgejo-data
      - container: /etc/timezone
        persist: forgejo-tz
    expose:
      port: 3000
      route: /forgejo

plugin:
  routes:
    - title: Forgejo
      icon: fa-code-branch
      group: extend
      target: ${expose.app.route}
      open_after_install: true
  commands:
    - id: forgejo.open
      title: Open Forgejo
      runs: route:${expose.app.route}
  settings:
    - key: admin.email
      type: string
      prompt: Initial admin email (used on first-run setup)
      default: admin@lthn.local

env:
  - key: ROOT_URL
    prompt: Public root URL Forgejo advertises in clones and emails
    type: string
    default: http://lthn.local/forgejo/
  - key: SSH_DOMAIN
    prompt: SSH host shown in repo clone URLs
    type: string
    default: lthn.local

permissions:
  - scope: service.notify
    mode: invoke
    reason: Notify when Forgejo first-run setup is ready.
`

// TestBundle_Forgejo_Good — long-running stateful service manifest
// parses + validates clean. Single image, two volumes (data + timezone),
// two env entries.
func TestBundle_Forgejo_Good(t *core.T) {
	r := subject.ParseManifestBytes([]byte(bundleForgejoYAML))
	core.RequireTrue(t, r.OK)
	m := r.Value.(subject.BundleManifest)
	core.AssertEqual(t, "forgejo", m.Name)
	core.AssertEqual(t, "Forgejo", m.Display)
	core.AssertEqual(t, "dev-tools", m.Category)
	core.AssertTrue(t, subject.ValidateManifest(m).OK)

	// Single image with the expected ref.
	core.AssertLen(t, m.Images, 1)
	core.AssertEqual(t, "codeberg.org/forgejo/forgejo:9", m.Images[0].Image)
}

// TestBundle_Forgejo_Bad — env tokens preserved as-is at parse;
// ${env.ROOT_URL} stays literal until install-time resolution.
func TestBundle_Forgejo_Bad(t *core.T) {
	r := subject.ParseManifestBytes([]byte(bundleForgejoYAML))
	core.RequireTrue(t, r.OK)
	m := r.Value.(subject.BundleManifest)
	core.AssertEqual(t, "${env.ROOT_URL}", m.Images[0].Env["FORGEJO__server__ROOT_URL"])
	core.AssertEqual(t, "${env.SSH_DOMAIN}", m.Images[0].Env["FORGEJO__server__SSH_DOMAIN"])
}

// TestBundle_Forgejo_Ugly — two persistent volumes coexist on one
// image; both surface in the parsed Volumes slice with their
// container paths + persist names intact.
func TestBundle_Forgejo_Ugly(t *core.T) {
	r := subject.ParseManifestBytes([]byte(bundleForgejoYAML))
	core.RequireTrue(t, r.OK)
	m := r.Value.(subject.BundleManifest)
	core.AssertLen(t, m.Images[0].Volumes, 2)

	byPersist := map[string]string{}
	for _, v := range m.Images[0].Volumes {
		byPersist[v.Persist] = v.Container
	}
	core.AssertEqual(t, "/var/lib/gitea", byPersist["forgejo-data"])
	core.AssertEqual(t, "/etc/timezone", byPersist["forgejo-tz"])

	// Two env entries (ROOT_URL + SSH_DOMAIN) — both string-typed
	// with sensible defaults referencing lthn.local from the mDNS lane.
	core.AssertLen(t, m.Env, 2)
}
