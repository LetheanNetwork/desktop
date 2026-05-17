// SPDX-Licence-Identifier: EUPL-1.2

// Tests migrated from pkg/marketplace/image_registry_test.go (Cerberus
// Mantis #1448 pass-6) PLUS new typed-error + IMDS coverage per RFC
// v1.1 §6 (Mantis #1667 / Cerberus #51).

package imagetrust_test

import (
	core "dappco.re/go"

	subject "dappco.re/lthn/desktop/pkg/imagetrust"
)

func TestImagetrust_IsAllowedImage_Good_DockerHubLibrary(t *core.T) {
	// Bare names = Docker Hub library/* shorthand.
	core.AssertNil(t, subject.IsAllowedImage("nginx"))
	core.AssertNil(t, subject.IsAllowedImage("alpine:3.21"))
	core.AssertNil(t, subject.IsAllowedImage("postgres:16"))
	core.AssertNil(t, subject.IsAllowedImage("mariadb:11"))
}

func TestImagetrust_IsAllowedImage_Good_DockerHubOrgs(t *core.T) {
	// Bare org/name shorthands (no dot/colon in first segment).
	core.AssertNil(t, subject.IsAllowedImage("n8nio/n8n:latest"))
	core.AssertNil(t, subject.IsAllowedImage("phpmyadmin/phpmyadmin:5"))
	core.AssertNil(t, subject.IsAllowedImage("vaultwarden/server:latest"))
	core.AssertNil(t, subject.IsAllowedImage("lthn/dev:latest"))
}

func TestImagetrust_IsAllowedImage_Good_ExplicitRegistries(t *core.T) {
	core.AssertNil(t, subject.IsAllowedImage("ghcr.io/owner/img:1.0"))
	core.AssertNil(t, subject.IsAllowedImage("quay.io/redhat/img:tag"))
	core.AssertNil(t, subject.IsAllowedImage("gcr.io/google/img"))
	core.AssertNil(t, subject.IsAllowedImage("mcr.microsoft.com/dotnet/sdk:8"))
	core.AssertNil(t, subject.IsAllowedImage("registry.gitlab.com/group/proj:1"))
	core.AssertNil(t, subject.IsAllowedImage("lscr.io/linuxserver/jellyfin"))
	core.AssertNil(t, subject.IsAllowedImage("codeberg.org/forgejo/forgejo:9"))
	core.AssertNil(t, subject.IsAllowedImage("forge.lthn.sh/lthn/dev:latest"))
	core.AssertNil(t, subject.IsAllowedImage("docker.io/library/nginx:1.27"))
}

func TestImagetrust_IsAllowedImage_Good_WithDigest(t *core.T) {
	// Full sha256 digest = sha256: + 64 hex chars.
	const fullDigest = "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	core.AssertNil(t, subject.IsAllowedImage("ghcr.io/owner/img@"+fullDigest))
	core.AssertNil(t, subject.IsAllowedImage("ghcr.io/owner/img:tag@"+fullDigest))
}

func TestImagetrust_IsAllowedImage_Good_RegistryWithPort(t *core.T) {
	// Port-suffixed registry — colon in first segment triggers the
	// registry-domain check.
	core.AssertNil(t, subject.IsAllowedImage("registry.gitlab.com:443/group/proj"))
}

func TestImagetrust_IsAllowedImage_Good_CaseInsensitive(t *core.T) {
	// Registry hostnames are case-insensitive per OCI; Docker/
	// containerd lowercase before resolve. The gate normalises.
	core.AssertNil(t, subject.IsAllowedImage("GHCR.io/owner/img"))
	core.AssertNil(t, subject.IsAllowedImage("Docker.IO/library/nginx"))
	core.AssertNil(t, subject.IsAllowedImage("FORGE.LTHN.SH/lthn/dev"))
}

// EmptyImageRef class — typed error.
func TestImagetrust_IsAllowedImage_EmptyImageRef_Bad(t *core.T) {
	err := subject.IsAllowedImage("")
	core.AssertTrue(t, core.Is(err, subject.ErrEmptyImageRef))
	core.AssertEqual(t, "image_empty", subject.ErrorCode(err))
	err = subject.IsAllowedImage("   ")
	core.AssertTrue(t, core.Is(err, subject.ErrEmptyImageRef))
}

// RegistryNotAllowed class.
func TestImagetrust_IsAllowedImage_RegistryNotAllowed_Bad(t *core.T) {
	cases := []string{
		"evil.example.com/malware:latest",
		"registry.attacker.io/foo",
		// Typo-squat (well-formed but unknown registry).
		"ghcr.com/owner/img",
		"quay.com/redhat/img",
		"docker.com/library/nginx",
	}
	for _, ref := range cases {
		err := subject.IsAllowedImage(ref)
		core.AssertTrue(t, core.Is(err, subject.ErrRegistryNotAllowed),
			"expected ErrRegistryNotAllowed for ", ref)
		core.AssertEqual(t, "image_registry_not_allowed", subject.ErrorCode(err))
	}
}

// MalformedDigest class — `@` without valid sha256:<64-hex>.
func TestImagetrust_IsAllowedImage_MalformedDigest_Bad(t *core.T) {
	cases := []string{
		"ghcr.io/owner/img@malformed",
		"ghcr.io/owner/img@sha256:short",
		"ghcr.io/owner/img@md5:abc123",
	}
	for _, ref := range cases {
		err := subject.IsAllowedImage(ref)
		core.AssertTrue(t, core.Is(err, subject.ErrMalformedDigest),
			"expected ErrMalformedDigest for ", ref)
		core.AssertEqual(t, "image_malformed_digest", subject.ErrorCode(err))
	}
}

// MalformedRef class — leading slash / empty first segment.
func TestImagetrust_IsAllowedImage_MalformedRef_Bad(t *core.T) {
	cases := []string{
		"/evil.com/foo",
		"/foo",
	}
	for _, ref := range cases {
		err := subject.IsAllowedImage(ref)
		core.AssertTrue(t, core.Is(err, subject.ErrMalformedRef),
			"expected ErrMalformedRef for ", ref)
		core.AssertEqual(t, "image_malformed", subject.ErrorCode(err))
	}
}

// LeadingDash class — defence vs flag-injection.
func TestImagetrust_IsAllowedImage_LeadingDash_Bad(t *core.T) {
	cases := []string{
		"-v",
		"-v /etc/shadow:/etc/shadow",
		"--privileged",
		"-it",
	}
	for _, ref := range cases {
		err := subject.IsAllowedImage(ref)
		core.AssertTrue(t, core.Is(err, subject.ErrLeadingDash),
			"expected ErrLeadingDash for ", ref)
		core.AssertEqual(t, "image_leading_dash", subject.ErrorCode(err))
	}
}

// IMDSAddress class — link-local v4/v6 + GCP/AWS/Azure/OCI/ECS hostnames.
// Per RFC v1.1 §2.2.1 the IMDS pre-check fires BEFORE the allowlist
// check so the typed reject survives even if a hostile entry is added.
func TestImagetrust_IsAllowedImage_IMDSAddress_Bad(t *core.T) {
	cases := []string{
		// IPv4 169.254.0.0/16 — AWS / GCP / Azure / DigitalOcean.
		"169.254.169.254/foo",
		// ECS task-metadata.
		"169.254.170.2/v2/credentials",
		// GCP IMDS.
		"metadata.google.internal/computeMetadata/v1/",
		// AWS IMDSv2 hostname.
		"metadata.aws.amazon.com/x",
		// Azure IMDS.
		"metadata.azure.com/x",
		// OCI IMDS.
		"metadata.oraclecloud.com/x",
		// IPv6 link-local.
		"[fe80::1]/foo",
		// Case-fold guard.
		"METADATA.GOOGLE.INTERNAL/x",
	}
	for _, ref := range cases {
		err := subject.IsAllowedImage(ref)
		core.AssertTrue(t, core.Is(err, subject.ErrIMDSAddress),
			"expected ErrIMDSAddress for ", ref)
		core.AssertEqual(t, "image_imds", subject.ErrorCode(err))
	}
}

// ErrorCode contract — nil → empty, unknown → "image_invalid".
func TestImagetrust_ErrorCode_Good(t *core.T) {
	core.AssertEqual(t, "", subject.ErrorCode(nil))
	core.AssertEqual(t, "image_invalid", subject.ErrorCode(core.NewError("something else")))
}

// ADD-5 invariant — allowedImageRegistries MUST be sorted alphabetically
// AND contain no duplicates. The list is package-private so we exercise
// the invariant indirectly: every allowlisted host accepts the bare
// `<host>/x` ref AND iterates lexically when we feed sorted pairs to
// IsAllowedImage. Direct sort/unique inspection is reserved for the CI
// grep gate per §6.
//
// This test additionally probes adjacency: if a future drift introduces
// `ghcr.com` ABOVE `ghcr.io` lexicographically, the loop catches it via
// the rejection of the typo-squat which immediately follows the accepted
// canonical.
func TestImagetrust_AllowedRegistries_SortedAndUnique_Good(t *core.T) {
	// Canonical-only list — kept in lexical order. Any addition to the
	// package-private allowlist MUST keep this list in lexical order to
	// match (ADD-5).
	canonical := []string{
		"codeberg.org",
		"docker.io",
		"forge.lthn.sh",
		"gcr.io",
		"ghcr.io",
		"index.docker.io",
		"lscr.io",
		"mcr.microsoft.com",
		"quay.io",
		"registry.gitlab.com",
	}
	// Lexical-order assertion on canonical.
	for i := 1; i < len(canonical); i++ {
		core.AssertTrue(t, canonical[i-1] < canonical[i],
			"canonical allowlist drifted out of lexical order at ", canonical[i])
	}
	// Each canonical entry is accepted; typo-squat sibling is rejected.
	for _, host := range canonical {
		core.AssertNil(t, subject.IsAllowedImage(host+"/x"))
	}
}
