// SPDX-Licence-Identifier: EUPL-1.2

package marketplace_test

import (
	core "dappco.re/go"
	subject "dappco.re/lthn/desktop/pkg/marketplace"
)

// bundleSonarQubeYAML is the canonical SonarQube + Postgres bundle.
// Multi-image composition with a heavier service (SonarQube needs
// ~2GB RAM + multiple persistent volumes).
const bundleSonarQubeYAML = `
schema: lthn-vm/v1
name: sonarqube
display: SonarQube
description: Static code analysis + quality reports for 30+ languages. Multi-image bundle — SonarQube depends on a Postgres companion for issue history and durable scan results.
category: dev-tools
icon: fa-shield-halved
homepage: https://www.sonarsource.com/products/sonarqube/
license: LGPL-3.0

images:
  - id: db
    image: postgres:16
    env:
      POSTGRES_USER: sonar
      POSTGRES_PASSWORD: ${env.DB_PASSWORD}
      POSTGRES_DB: sonar
    volumes:
      - container: /var/lib/postgresql/data
        persist: sonarqube-db
  - id: app
    image: sonarqube:10-community
    env:
      SONAR_JDBC_URL: jdbc:postgresql://db:5432/sonar
      SONAR_JDBC_USERNAME: sonar
      SONAR_JDBC_PASSWORD: ${env.DB_PASSWORD}
    volumes:
      - container: /opt/sonarqube/data
        persist: sonarqube-data
      - container: /opt/sonarqube/extensions
        persist: sonarqube-extensions
      - container: /opt/sonarqube/logs
        persist: sonarqube-logs
    expose:
      port: 9000
      route: /sonarqube

plugin:
  routes:
    - title: SonarQube
      icon: fa-shield-halved
      group: extend
      target: ${expose.app.route}
      open_after_install: true
  commands:
    - id: sonarqube.open
      title: Open SonarQube
      runs: route:${expose.app.route}
  settings:
    - key: db.password
      type: secret
      prompt: SonarQube database password
      default: ${random.password(32)}

env:
  - key: DB_PASSWORD
    prompt: Database password (shared between SonarQube and Postgres)
    type: secret
    default: ${random.password(32)}

permissions:
  - scope: service.notify
    mode: invoke
    reason: Notify when SonarQube is ready for scans.
`

// TestBundle_SonarQube_Good — heavier multi-image bundle with multiple
// persistent volumes per image. Parses + validates cleanly.
func TestBundle_SonarQube_Good(t *core.T) {
	r := subject.ParseManifestBytes([]byte(bundleSonarQubeYAML))
	core.RequireTrue(t, r.OK)
	m := r.Value.(subject.BundleManifest)
	core.AssertEqual(t, "sonarqube", m.Name)
	core.AssertTrue(t, subject.ValidateManifest(m).OK)
	core.AssertLen(t, m.Images, 2)
}

// TestBundle_SonarQube_Bad — JDBC URL references the db sibling by id;
// parser preserves the literal "jdbc:postgresql://db:5432/sonar" — the
// "db" host resolves via bundle-internal DNS at runtime.
func TestBundle_SonarQube_Bad(t *core.T) {
	r := subject.ParseManifestBytes([]byte(bundleSonarQubeYAML))
	core.RequireTrue(t, r.OK)
	m := r.Value.(subject.BundleManifest)
	byID := map[string]subject.ImageEntry{}
	for _, img := range m.Images {
		byID[img.ID] = img
	}
	core.AssertEqual(t, "jdbc:postgresql://db:5432/sonar", byID["app"].Env["SONAR_JDBC_URL"])
}

// TestBundle_SonarQube_Ugly — app image carries THREE persistent
// volumes (data, extensions, logs). All three round-trip through
// parse with their container paths + persist names intact.
func TestBundle_SonarQube_Ugly(t *core.T) {
	r := subject.ParseManifestBytes([]byte(bundleSonarQubeYAML))
	core.RequireTrue(t, r.OK)
	m := r.Value.(subject.BundleManifest)
	byID := map[string]subject.ImageEntry{}
	for _, img := range m.Images {
		byID[img.ID] = img
	}
	app := byID["app"]
	core.AssertLen(t, app.Volumes, 3)
	persistNames := map[string]bool{}
	for _, v := range app.Volumes {
		persistNames[v.Persist] = true
	}
	core.AssertTrue(t, persistNames["sonarqube-data"])
	core.AssertTrue(t, persistNames["sonarqube-extensions"])
	core.AssertTrue(t, persistNames["sonarqube-logs"])

	// db doesn't expose — internal-only, sibling-resolved.
	if byID["db"].Expose != nil {
		t.Fatalf("db should not expose a route (got %+v)", byID["db"].Expose)
	}
}
