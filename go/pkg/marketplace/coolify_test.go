// SPDX-Licence-Identifier: EUPL-1.2

// Tests for the Coolify template → lthn-vm manifest converter.
// Fixtures: t.TempDir() with synthesised docker-compose.yml +
// .env.template content so the parse path is exercised end-to-end
// against the real ImportCoolify file-IO logic.

package marketplace_test

import (
	core "dappco.re/go"

	subject "dappco.re/lthn/desktop/pkg/marketplace"
)

// fixtureCompose is a minimal multi-service compose representative
// of a typical Coolify template (Ghost blog + MariaDB pattern).
const fixtureCompose = `
services:
  db:
    image: mariadb:11
    environment:
      MARIADB_ROOT_PASSWORD: $DB_PASSWORD
      MARIADB_DATABASE: ghost
    volumes:
      - ghost-db:/var/lib/mysql
  app:
    image: ghost:5
    environment:
      database__client: mysql
      database__connection__host: db
      database__connection__password: ${DB_PASSWORD}
      url: ${PUBLIC_URL}
    volumes:
      - ghost-content:/var/lib/ghost/content
    ports:
      - "2368:2368"
`

const fixtureEnvTemplate = `
# Database
DB_PASSWORD=changeme
PUBLIC_URL="http://lthn.local/ghost/"

# Optional
ADMIN_EMAIL=admin@example.com
ADMIN_API_KEY=set-me
`

// writeFixture lays out a Coolify template directory under t.TempDir
// + returns the path. Empty envContent skips the .env.template file.
func writeFixture(t *core.T, name string, compose, envContent string) string {
	t.Helper()
	dir := core.PathJoin(t.TempDir(), name)
	core.RequireTrue(t, core.MkdirAll(dir, 0o755).OK)
	composePath := core.PathJoin(dir, "docker-compose.yml")
	core.RequireTrue(t, core.WriteFile(composePath, []byte(compose), 0o644).OK)
	if envContent != "" {
		envPath := core.PathJoin(dir, ".env.template")
		core.RequireTrue(t, core.WriteFile(envPath, []byte(envContent), 0o644).OK)
	}
	return dir
}

// ─── ImportCoolify ─────────────────────────────────────────────────────

func TestCoolify_ImportCoolify_Good(t *core.T) {
	dir := writeFixture(t, "ghost", fixtureCompose, fixtureEnvTemplate)
	r := subject.ImportCoolify(dir)
	core.RequireTrue(t, r.OK)
	m := r.Value.(subject.BundleManifest)

	core.AssertEqual(t, "lthn-vm/v1", m.Schema)
	core.AssertEqual(t, "ghost", m.Name)

	// Two images, sorted alphabetically (app before db).
	core.AssertLen(t, m.Images, 2)
	byID := map[string]subject.ImageEntry{}
	for _, img := range m.Images {
		byID[img.ID] = img
	}
	core.AssertEqual(t, "ghost:5", byID["app"].Image)
	core.AssertEqual(t, "mariadb:11", byID["db"].Image)

	// app exposes; db doesn't (no ports in fixture).
	core.AssertNotNil(t, byID["app"].Expose)
	core.AssertEqual(t, 2368, byID["app"].Expose.Port)
	core.AssertEqual(t, "/app", byID["app"].Expose.Route)
	if byID["db"].Expose != nil {
		t.Fatalf("db should not expose (no ports in fixture)")
	}

	// Named volumes mapped to persist; bind mounts would be skipped.
	core.AssertLen(t, byID["app"].Volumes, 1)
	core.AssertEqual(t, "ghost-content", byID["app"].Volumes[0].Persist)
	core.AssertEqual(t, "/var/lib/ghost/content", byID["app"].Volumes[0].Container)

	// Manifest env populated from .env.template + secret heuristic
	// flips DB_PASSWORD + ADMIN_API_KEY to type=secret.
	core.AssertLen(t, m.Env, 4)
	envByKey := map[string]subject.EnvEntry{}
	for _, e := range m.Env {
		envByKey[e.Key] = e
	}
	core.AssertEqual(t, "secret", envByKey["DB_PASSWORD"].Type)
	core.AssertEqual(t, "secret", envByKey["ADMIN_API_KEY"].Type)
	core.AssertEqual(t, "string", envByKey["PUBLIC_URL"].Type)
	core.AssertEqual(t, "string", envByKey["ADMIN_EMAIL"].Type)
	// Quoted defaults are stripped at parse.
	core.AssertEqual(t, "http://lthn.local/ghost/", envByKey["PUBLIC_URL"].Default)
}

func TestCoolify_ImportCoolify_Bad(t *core.T) {
	// Empty path fails.
	core.AssertFalse(t, subject.ImportCoolify("").OK)

	// Non-existent path fails (no docker-compose.yml).
	core.AssertFalse(t, subject.ImportCoolify("/nonexistent/coolify/template").OK)

	// Compose file with zero services fails.
	dir := writeFixture(t, "empty", "services: {}\n", "")
	r := subject.ImportCoolify(dir)
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "no services")

	// Malformed YAML fails cleanly (no panic).
	badDir := writeFixture(t, "bad", "services:\n  broken: [not-a-mapping", "")
	bad := subject.ImportCoolify(badDir)
	core.AssertFalse(t, bad.OK)
}

func TestCoolify_ImportCoolify_Ugly(t *core.T) {
	// Verify shell-style $VAR + ${VAR} get rewritten to ${env.VAR}
	// at parse time so a subsequent install resolves them via the
	// manifest's env block.
	dir := writeFixture(t, "rewrite", fixtureCompose, fixtureEnvTemplate)
	r := subject.ImportCoolify(dir)
	core.RequireTrue(t, r.OK)
	m := r.Value.(subject.BundleManifest)

	byID := map[string]subject.ImageEntry{}
	for _, img := range m.Images {
		byID[img.ID] = img
	}
	// $DB_PASSWORD → ${env.DB_PASSWORD}
	core.AssertEqual(t, "${env.DB_PASSWORD}", byID["db"].Env["MARIADB_ROOT_PASSWORD"])
	// ${DB_PASSWORD} → ${env.DB_PASSWORD}
	core.AssertEqual(t, "${env.DB_PASSWORD}", byID["app"].Env["database__connection__password"])
	// ${PUBLIC_URL} → ${env.PUBLIC_URL}
	core.AssertEqual(t, "${env.PUBLIC_URL}", byID["app"].Env["url"])
	// Sibling-id reference passes through literal (compose's "db"
	// hostname resolves via internal Docker network at runtime).
	core.AssertEqual(t, "db", byID["app"].Env["database__connection__host"])

	// Round-trip through MarshalManifest + ParseManifestBytes —
	// the imported manifest must survive YAML serialisation
	// without losing fidelity.
	marshalR := subject.MarshalManifest(m)
	core.RequireTrue(t, marshalR.OK)
	raw := marshalR.Value.([]byte)
	r2 := subject.ParseManifestBytes(raw)
	core.RequireTrue(t, r2.OK)
	m2 := r2.Value.(subject.BundleManifest)

	core.AssertEqual(t, m.Name, m2.Name)
	core.AssertLen(t, m2.Images, 2)
	core.AssertLen(t, m2.Env, 4)
}

// ─── No .env.template ──────────────────────────────────────────────────

func TestCoolify_ImportCoolify_NoEnvTemplate_Good(t *core.T) {
	dir := writeFixture(t, "noenv", `
services:
  app:
    image: nginx:latest
    ports:
      - "80:80"
`, "")
	r := subject.ImportCoolify(dir)
	core.RequireTrue(t, r.OK)
	m := r.Value.(subject.BundleManifest)
	core.AssertEqual(t, "noenv", m.Name)
	core.AssertLen(t, m.Images, 1)
	core.AssertLen(t, m.Env, 0) // no .env.template → empty Env block
}

// ─── Heuristics ────────────────────────────────────────────────────────

func TestCoolify_SecretHeuristic_Good(t *core.T) {
	// Verify the looksLikeSecret heuristic via the manifest output —
	// each suffix flips the env type to "secret".
	envContent := `
USER_PASSWORD=x
GITHUB_TOKEN=y
SOME_SECRET=z
JWT_KEY=w
API_KEY=v
STRIPE_API_KEY=u
PASSWORD=top-level
ORDINARY_VAR=ok
`
	dir := writeFixture(t, "heuristic", `
services:
  app:
    image: alpine:3
`, envContent)
	r := subject.ImportCoolify(dir)
	core.RequireTrue(t, r.OK)
	m := r.Value.(subject.BundleManifest)

	byKey := map[string]string{}
	for _, e := range m.Env {
		byKey[e.Key] = e.Type
	}
	core.AssertEqual(t, "secret", byKey["USER_PASSWORD"])
	core.AssertEqual(t, "secret", byKey["GITHUB_TOKEN"])
	core.AssertEqual(t, "secret", byKey["SOME_SECRET"])
	core.AssertEqual(t, "secret", byKey["JWT_KEY"])
	core.AssertEqual(t, "secret", byKey["API_KEY"])
	core.AssertEqual(t, "secret", byKey["STRIPE_API_KEY"])
	core.AssertEqual(t, "secret", byKey["PASSWORD"])
	core.AssertEqual(t, "string", byKey["ORDINARY_VAR"])
}
