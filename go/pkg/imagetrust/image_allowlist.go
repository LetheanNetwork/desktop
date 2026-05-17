// SPDX-Licence-Identifier: EUPL-1.2

// image_allowlist.go — registry-domain allowlist + IMDS pre-check.
// Policy preserved verbatim from the previous home in
// pkg/marketplace/manifest.go:517-606 per RFC v1.1 §2.3 (no allowlist
// policy change in this RFC; the IMDS pre-check is a new typed-reject
// class, not an allowlist change). Updates ship as code review.

package imagetrust

import (
	core "dappco.re/go"
)

// allowedImageRegistries is the compile-time allowlist of OCI
// registry domains a caller's image may pull from when the reference
// includes an explicit registry. Cerberus Mantis #1448 (originally),
// Mantis #1667 (lifted into pkg/imagetrust).
//
// Same compile-time discipline as pkg/downloader.allowedHostSuffixes
// (#1424) — runtime mutation would defeat the gate, so updates ship
// as code review.
//
// Big-tent OSS coverage: Docker Hub (registry-explicit form),
// GitHub Container Registry, Red Hat Quay, Google Container
// Registry, Microsoft, GitLab, LinuxServer.io (popular in self-host
// bundles), Codeberg (Forgejo-hosted), and our own Forge.
//
// MUST stay sorted alphabetically AND contain no duplicates — invariant
// asserted in TestImagetrust_AllowedRegistries_Sorted+Unique_Good per
// RFC v1.1 §6 (ADD-5).
var allowedImageRegistries = []string{
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

// imdsHostnames lists known cloud-metadata service hostnames. The
// classifier rejects any image whose first segment matches one of
// these BEFORE the allowlist check fires (Cerberus #1677 — ErrIMDSAddress
// must survive even if a hostile entry is ever added to the allowlist).
// New IMDS endpoints surface as code review (same compile-time discipline
// as the allowlist).
var imdsHostnames = []string{
	"metadata.aws.amazon.com",
	"metadata.azure.com",
	"metadata.google.internal",
	"metadata.oraclecloud.com",
}

// IsAllowedImage reports whether ref is on the registry allowlist
// and well-formed. Replaces marketplace.IsAllowedImageRegistry —
// callers in marketplace + sandbox + bridge MUST consult this before
// any runtime call uses the reference.
//
// Validation order (load-bearing — encoded in TestImagetrust_*):
//
//  1. ErrEmptyImageRef       — empty / whitespace-only
//  2. ErrLeadingDash         — defence vs flag-injection (positional-as-flag)
//  3. ErrMalformedRef        — leading-slash / empty-first-segment
//  4. ErrIMDSAddress         — link-local + cloud-metadata hostnames (pre-allowlist)
//  5. ErrMalformedDigest     — '@' without sha256:<64-hex>
//  6. ErrRegistryNotAllowed  — registry not on allowlist
//
// IMDS classification runs BEFORE the allowlist check so the typed
// error survives even if a hostile registry is ever added to the
// allowlist (defence in depth — #1677).
//
// Returns a typed error rather than a bool so the caller can
// surface the rejection reason in audit + UI without a parallel
// error-classification step. Use core.Is(err, imagetrust.ErrX) (or
// errors.Is) for classification; never string-match Error().
//
// Usage example:
//
//	if err := imagetrust.IsAllowedImage(input.Image); err != nil {
//	    return core.Fail(core.E("sandbox.prepareSpawnInput",
//	        "image rejected by allowlist", err))
//	}
func IsAllowedImage(ref string) error {
	ref = core.Trim(ref)
	if ref == "" {
		return ErrEmptyImageRef
	}

	// Leading-dash guard: catch the flag-injection shape BEFORE any
	// parsing so a hostile `-v /etc/shadow:/etc/shadow` style image can
	// never sneak past as either a registry domain or a Hub shorthand.
	if ref[0] == '-' {
		return ErrLeadingDash
	}

	// Reject empty-first-segment refs (e.g. `/evil.com/foo`) up front;
	// they would otherwise fall through the Docker Hub shorthand path
	// in the legacy implementation and accept.
	if ref[0] == '/' {
		return ErrMalformedRef
	}

	// Cerberus pass-6 LOW (#1448) — registry hostnames are case-
	// insensitive per OCI distribution spec. Lowercase so all
	// subsequent index/contains checks are case-folded.
	ref = core.Lower(ref)

	// Cerberus pass-6 LOW (#1448) — `@` without a valid digest is a
	// malformed ref. Require digest to be `sha256:<64-hex>` (the only
	// algorithm distribution guarantees today).
	if i := core.LastIndex(ref, "@"); i > 0 {
		digest := ref[i+1:]
		if !core.HasPrefix(digest, "sha256:") || len(digest) != len("sha256:")+64 {
			return ErrMalformedDigest
		}
		ref = ref[:i]
	}

	// Strip a trailing `:tag` if it doesn't span a `/` (else this is
	// a registry-with-port like `registry.gitlab.com:443/...`).
	if i := core.LastIndex(ref, ":"); i > 0 {
		tail := ref[i:]
		if !core.Contains(tail, "/") {
			ref = ref[:i]
		}
	}

	slash := core.Index(ref, "/")
	if slash < 0 {
		// `nginx` / `alpine` / `postgres` — Docker Hub library/*
		// shorthand. The IMDS host shapes all carry a `/` path segment
		// in the ref (e.g. `169.254.169.254/v2/...`), so a bare token
		// here cannot be an IMDS reach.
		return nil
	}
	first := ref[:slash]
	if first == "" {
		return ErrMalformedRef
	}

	// First segment is a registry domain when it contains a `.` or `:`.
	if core.Contains(first, ".") || core.Contains(first, ":") {
		// Strip :port if present so `registry.gitlab.com:443` matches
		// `registry.gitlab.com` in the allowlist.
		host := first
		if i := core.Index(host, ":"); i > 0 {
			host = host[:i]
		}

		// IMDS pre-check (ADD-1, Cerberus #1677). Runs BEFORE the
		// allowlist check so the typed reject survives even if a
		// hostile entry is ever added to allowedImageRegistries.
		// Bracketed IPv6 literals (`[fe80::1]`) arrive here as the
		// pre-strip first segment — match on the un-stripped variant
		// to catch IPv6.
		if isIMDSHost(host, first) {
			return ErrIMDSAddress
		}

		for _, allowed := range allowedImageRegistries {
			if host == allowed {
				return nil
			}
		}
		return ErrRegistryNotAllowed
	}

	// Otherwise it's a Docker Hub org shorthand — allowed (see
	// allowedImageRegistries docstring for the framing).
	return nil
}

// isIMDSHost reports whether host (port-stripped) or hostWithPort
// (bracketed-IPv6-preserving) matches a known IMDS shape.
//
// Match shapes:
//   - exact known cloud-metadata hostname (case-folded by caller)
//   - IPv4 169.254.0.0/16 (AWS / GCP / Azure / DigitalOcean / ECS)
//   - IPv6 fe80::/10 link-local (delivered as `[fe80:...]` so checked
//     against the bracketed form)
func isIMDSHost(host, hostWithPort string) bool {
	for _, h := range imdsHostnames {
		if host == h {
			return true
		}
	}
	if isIPv4LinkLocal(host) {
		return true
	}
	if isIPv6LinkLocalBracketed(hostWithPort) {
		return true
	}
	return false
}

// isIPv4LinkLocal reports whether host parses as 169.254.x.y. Hand-
// rolled rather than via net.ParseIP because we already have the
// host segment isolated and want to avoid pulling the net dep into
// the validator path.
func isIPv4LinkLocal(host string) bool {
	if !core.HasPrefix(host, "169.254.") {
		return false
	}
	rest := host[len("169.254."):]
	dot := core.Index(rest, ".")
	if dot <= 0 || dot == len(rest)-1 {
		return false
	}
	return isDecOctet(rest[:dot]) && isDecOctet(rest[dot+1:])
}

// isIPv6LinkLocalBracketed reports whether s is a `[fe80::...]`-shaped
// IPv6 link-local literal. The validator only fires on the first
// segment of the ref, and IPv6 literals must be bracketed in the
// docker/OCI URI shape to disambiguate from the port colon.
func isIPv6LinkLocalBracketed(s string) bool {
	if len(s) < len("[fe80::]") {
		return false
	}
	if s[0] != '[' || s[len(s)-1] != ']' {
		return false
	}
	inner := s[1 : len(s)-1]
	// fe80::/10 covers fe80..febf. The first 10 bits are 1111111010
	// — first hextet starts with fe8/fe9/fea/feb (case-folded by the
	// caller's core.Lower).
	if len(inner) < 4 {
		return false
	}
	if inner[0] != 'f' || inner[1] != 'e' {
		return false
	}
	switch inner[2] {
	case '8', '9', 'a', 'b':
		// fall through
	default:
		return false
	}
	// Require either the canonical `::` shortcut or the `:` continuation
	// so we don't accept `fe8d-something-else`.
	return core.Contains(inner, ":")
}

// isDecOctet reports whether s is a 1..3-digit decimal string in 0..255.
func isDecOctet(s string) bool {
	if len(s) == 0 || len(s) > 3 {
		return false
	}
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
		n = n*10 + int(c-'0')
	}
	return n <= 255
}
