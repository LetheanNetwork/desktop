// SPDX-Licence-Identifier: EUPL-1.2

package marketplace_test

import (
	core "dappco.re/go"
	subject "dappco.re/lthn/desktop/pkg/marketplace"
)

// bundlePHPMyAdminYAML is the canonical PHPMyAdmin bundle manifest.
// Mirrors bundles/phpmyadmin/manifest.yml — keep in sync if the
// file changes. Multi-image composition shape per RFC.marketplace
// §8.2; demonstrates db + app sibling sandboxes resolving each
// other via the bundle's internal Docker network.
const bundlePHPMyAdminYAML = `
schema: lthn-vm/v1
name: phpmyadmin
display: PHPMyAdmin
description: MySQL / MariaDB admin UI. Browse databases, edit rows, run queries. The MariaDB sandbox runs alongside so you have a working database out of the box.
category: dev-tools
icon: fa-database
homepage: https://www.phpmyadmin.net/
license: GPL-2.0

images:
  - id: db
    image: mariadb:11
    env:
      MARIADB_ROOT_PASSWORD: ${env.DB_PASSWORD}
    volumes:
      - container: /var/lib/mysql
        persist: phpmyadmin-db
  - id: app
    image: phpmyadmin/phpmyadmin:5
    env:
      PMA_HOST: db
      PMA_USER: root
      PMA_PASSWORD: ${env.DB_PASSWORD}
    expose:
      port: 80
      route: /phpmyadmin

plugin:
  routes:
    - title: PHPMyAdmin
      icon: fa-database
      group: extend
      target: ${expose.app.route}
      open_after_install: true
  commands:
    - id: phpmyadmin.open
      title: Open PHPMyAdmin
      runs: route:${expose.app.route}
  settings:
    - key: db.password
      type: secret
      prompt: Database root password
      default: ${random.password(32)}

env:
  - key: DB_PASSWORD
    prompt: Database root password (shared between MariaDB and PHPMyAdmin)
    type: secret
    default: ${random.password(32)}

permissions:
  - scope: service.notify
    mode: invoke
    reason: Notify when database is ready for queries.
`

// TestBundle_PHPMyAdmin_Good — multi-image composition manifest parses
// + validates clean. First day-2 bundle per RFC.marketplace §8.2.
func TestBundle_PHPMyAdmin_Good(t *core.T) {
	r := subject.ParseManifestBytes([]byte(bundlePHPMyAdminYAML))
	core.RequireTrue(t, r.OK)
	m := r.Value.(subject.BundleManifest)
	core.AssertEqual(t, "phpmyadmin", m.Name)
	core.AssertEqual(t, "PHPMyAdmin", m.Display)
	core.AssertEqual(t, "lthn-vm/v1", m.Schema)
	core.AssertEqual(t, "dev-tools", m.Category)

	// Validate passes.
	core.AssertTrue(t, subject.ValidateManifest(m).OK)
}

// TestBundle_PHPMyAdmin_Bad — sibling-resolution wiring: PMA_HOST=db
// is preserved as a literal string (NOT ${...} substituted at parse).
// The container DNS resolves "db" via the bundle's internal network
// at runtime; parser leaves the value intact.
func TestBundle_PHPMyAdmin_Bad(t *core.T) {
	r := subject.ParseManifestBytes([]byte(bundlePHPMyAdminYAML))
	core.RequireTrue(t, r.OK)
	m := r.Value.(subject.BundleManifest)

	// Find the app image — order is preserved in the parser.
	var app subject.ImageEntry
	for _, img := range m.Images {
		if img.ID == "app" {
			app = img
			break
		}
	}
	core.AssertEqual(t, "phpmyadmin/phpmyadmin:5", app.Image)
	core.AssertEqual(t, "db", app.Env["PMA_HOST"])
	core.AssertEqual(t, "${env.DB_PASSWORD}", app.Env["PMA_PASSWORD"])
}

// TestBundle_PHPMyAdmin_Ugly — two images with the env-token referenced
// from both. Both env values share the same ${env.DB_PASSWORD} token;
// the env-block resolution at install time ensures both containers see
// the same password value (sibling auth wouldn't work otherwise).
func TestBundle_PHPMyAdmin_Ugly(t *core.T) {
	r := subject.ParseManifestBytes([]byte(bundlePHPMyAdminYAML))
	core.RequireTrue(t, r.OK)
	m := r.Value.(subject.BundleManifest)
	core.AssertLen(t, m.Images, 2)

	// Both images reference ${env.DB_PASSWORD} — index by id so we
	// don't depend on declaration order.
	byID := map[string]subject.ImageEntry{}
	for _, img := range m.Images {
		byID[img.ID] = img
	}
	core.AssertEqual(t, "${env.DB_PASSWORD}", byID["db"].Env["MARIADB_ROOT_PASSWORD"])
	core.AssertEqual(t, "${env.DB_PASSWORD}", byID["app"].Env["PMA_PASSWORD"])

	// Only the app exposes a route — db stays internal-only.
	core.AssertNotNil(t, byID["app"].Expose)
	core.AssertEqual(t, "/phpmyadmin", byID["app"].Expose.Route)
	// db.Expose is nil — internal services don't surface to lthn.sh.
	if byID["db"].Expose != nil {
		t.Fatalf("db image should not expose a route (got %+v)", byID["db"].Expose)
	}

	// Env block has one entry; it's the shared password the two
	// images reference.
	core.AssertLen(t, m.Env, 1)
	core.AssertEqual(t, "DB_PASSWORD", m.Env[0].Key)
	core.AssertEqual(t, "secret", m.Env[0].Type)
}
