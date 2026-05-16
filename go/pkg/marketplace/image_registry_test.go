// SPDX-Licence-Identifier: EUPL-1.2

// Cerberus Mantis #1448 — IsAllowedImageRegistry gate tests. The
// allowlist accepts Docker Hub (explicit + implicit), the major
// OSS-community registries, and Lethean's own Forge. Rejects
// arbitrary attacker-controlled registries.

package marketplace_test

import (
	core "dappco.re/go"

	subject "dappco.re/lthn/desktop/pkg/marketplace"
)

func TestManifest_IsAllowedImageRegistry_Good_DockerHubLibrary(t *core.T) {
	// Bare names = Docker Hub library/* shorthand.
	core.AssertTrue(t, subject.IsAllowedImageRegistry("nginx"))
	core.AssertTrue(t, subject.IsAllowedImageRegistry("alpine:3.21"))
	core.AssertTrue(t, subject.IsAllowedImageRegistry("postgres:16"))
	core.AssertTrue(t, subject.IsAllowedImageRegistry("mariadb:11"))
}

func TestManifest_IsAllowedImageRegistry_Good_DockerHubOrgs(t *core.T) {
	// Bare org/name shorthands (no dot/colon in first segment).
	core.AssertTrue(t, subject.IsAllowedImageRegistry("n8nio/n8n:latest"))
	core.AssertTrue(t, subject.IsAllowedImageRegistry("phpmyadmin/phpmyadmin:5"))
	core.AssertTrue(t, subject.IsAllowedImageRegistry("vaultwarden/server:latest"))
	core.AssertTrue(t, subject.IsAllowedImageRegistry("lthn/dev:latest"))
}

func TestManifest_IsAllowedImageRegistry_Good_ExplicitRegistries(t *core.T) {
	core.AssertTrue(t, subject.IsAllowedImageRegistry("ghcr.io/owner/img:1.0"))
	core.AssertTrue(t, subject.IsAllowedImageRegistry("quay.io/redhat/img:tag"))
	core.AssertTrue(t, subject.IsAllowedImageRegistry("gcr.io/google/img"))
	core.AssertTrue(t, subject.IsAllowedImageRegistry("mcr.microsoft.com/dotnet/sdk:8"))
	core.AssertTrue(t, subject.IsAllowedImageRegistry("registry.gitlab.com/group/proj:1"))
	core.AssertTrue(t, subject.IsAllowedImageRegistry("lscr.io/linuxserver/jellyfin"))
	core.AssertTrue(t, subject.IsAllowedImageRegistry("codeberg.org/forgejo/forgejo:9"))
	core.AssertTrue(t, subject.IsAllowedImageRegistry("forge.lthn.sh/lthn/dev:latest"))
	core.AssertTrue(t, subject.IsAllowedImageRegistry("docker.io/library/nginx:1.27"))
}

func TestManifest_IsAllowedImageRegistry_Good_WithDigest(t *core.T) {
	core.AssertTrue(t, subject.IsAllowedImageRegistry("ghcr.io/owner/img@sha256:abc123"))
	core.AssertTrue(t, subject.IsAllowedImageRegistry("ghcr.io/owner/img:tag@sha256:abc"))
}

func TestManifest_IsAllowedImageRegistry_Good_RegistryWithPort(t *core.T) {
	// Port-suffixed registry — colon in first segment triggers the
	// registry-domain check.
	core.AssertTrue(t, subject.IsAllowedImageRegistry("registry.gitlab.com:443/group/proj"))
}

func TestManifest_IsAllowedImageRegistry_Bad_UnknownRegistry(t *core.T) {
	core.AssertFalse(t, subject.IsAllowedImageRegistry("evil.example.com/malware:latest"))
	core.AssertFalse(t, subject.IsAllowedImageRegistry("registry.attacker.io/foo"))
	core.AssertFalse(t, subject.IsAllowedImageRegistry("169.254.169.254/imds-leak"))
}

func TestManifest_IsAllowedImageRegistry_Bad_TypoSquatRegistry(t *core.T) {
	// Typo-squat the registry domain — must reject because not in
	// the allowlist.
	core.AssertFalse(t, subject.IsAllowedImageRegistry("ghcr.com/owner/img"))
	core.AssertFalse(t, subject.IsAllowedImageRegistry("quay.com/redhat/img"))
	core.AssertFalse(t, subject.IsAllowedImageRegistry("docker.com/library/nginx"))
}

func TestManifest_IsAllowedImageRegistry_Bad_Empty(t *core.T) {
	core.AssertFalse(t, subject.IsAllowedImageRegistry(""))
	core.AssertFalse(t, subject.IsAllowedImageRegistry("   "))
}
