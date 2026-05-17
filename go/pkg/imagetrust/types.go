// SPDX-Licence-Identifier: EUPL-1.2

// Package imagetrust is the canonical registry-domain trust gate for
// OCI image references. Marketplace + sandbox + bridge consult this
// package before any runtime call uses an image string — replacing
// the prior asymmetry where marketplace.IsAllowedImageRegistry was
// the only enforcement point and sandbox.Spawn / SpawnLong /
// bridge.toolSandboxSpawn forwarded attacker-influenceable input to
// the container runtime with no consultation of the allowlist.
//
// Athena RFC: plans/code/lthn/desktop/sandbox/RFC.image-allowlist.md
// (v1.1, Mantis #1667). Cerberus #51 DREAD: CLEAN-FOR-IMPL.
//
// Sole entry point is IsAllowedImage(ref string) error. See its
// usage example for the typed-error rejection classes.

package imagetrust

import (
	core "dappco.re/go"
)

// Typed-error sentinels. Callers MUST use core.Is(err, ErrX) (or
// errors.Is) for classification — never string-match Error() text.
// The short code returned by ErrorCode below is the canonical
// audit/UI discriminator per RFC §2.4.
var (
	// ErrEmptyImageRef is returned when the image reference is empty
	// or whitespace-only.
	ErrEmptyImageRef = core.NewError("image ref is empty")

	// ErrLeadingDash is returned when the image ref starts with `-`.
	// Defence vs flag-injection — without this gate a caller could
	// supply `-v /etc/shadow:/etc/shadow` as the image, which a future
	// docker/podman flag-parser change could re-interpret as a docker
	// run flag rather than as the image position.
	ErrLeadingDash = core.NewError("image ref starts with '-' (flag-injection guard)")

	// ErrMalformedRef is returned for structurally-broken refs that
	// neither match a registry domain shape nor a Docker Hub shorthand
	// — leading slash, empty first segment, etc.
	ErrMalformedRef = core.NewError("image ref is malformed")

	// ErrIMDSAddress is returned when the registry host segment is a
	// link-local IPv4 (169.254.0.0/16), link-local IPv6 (fe80::/10), or
	// a known cloud-metadata hostname (GCP/AWS/Azure/OCI/ECS). Matched
	// BEFORE the allowlist check so the typed reject survives even if
	// a hostile entry is ever added to allowedImageRegistries — defence
	// in depth per Cerberus #1677.
	ErrIMDSAddress = core.NewError("image ref targets an instance-metadata (IMDS) address")

	// ErrMalformedDigest is returned when an `@` digest suffix does not
	// match the canonical `sha256:<64-hex>` shape.
	ErrMalformedDigest = core.NewError("image ref has malformed digest (need sha256:<64-hex>)")

	// ErrRegistryNotAllowed is returned when the registry host is not
	// on the compile-time allowlist (allowedImageRegistries).
	ErrRegistryNotAllowed = core.NewError("image registry is not on the allowlist")
)

// ErrorCode maps a typed error into the short, stable audit/UI code
// used by EventSandboxSpawnFailed.Meta["error_code"]. Returns the
// empty string for nil and a generic "image_invalid" for any other
// error so the audit substrate never carries an unbounded raw
// error_code string.
//
// 1:1 with the typed-error class per Cerberus #51 Q-4.
//
// Usage example:
//
//	if err := imagetrust.IsAllowedImage(input.Image); err != nil {
//	    audit.Default().Record(audit.Event{
//	        Meta: map[string]any{"error_code": imagetrust.ErrorCode(err)},
//	    })
//	}
func ErrorCode(err error) string {
	if err == nil {
		return ""
	}
	switch {
	case core.Is(err, ErrEmptyImageRef):
		return "image_empty"
	case core.Is(err, ErrLeadingDash):
		return "image_leading_dash"
	case core.Is(err, ErrIMDSAddress):
		return "image_imds"
	case core.Is(err, ErrMalformedDigest):
		return "image_malformed_digest"
	case core.Is(err, ErrMalformedRef):
		return "image_malformed"
	case core.Is(err, ErrRegistryNotAllowed):
		return "image_registry_not_allowed"
	}
	return "image_invalid"
}
