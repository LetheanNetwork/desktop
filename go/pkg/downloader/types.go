// SPDX-Licence-Identifier: EUPL-1.2

// types.go — typed sentinels for the downloader trust layer. Callers
// use core.Is (or errors.Is) to classify rejections without string-
// matching Error(). Mirrors the imagetrust.ErrIMDSAddress pattern at
// the network-resolve tier rather than the registry-domain-string tier.

package downloader

import (
	core "dappco.re/go"
)

// ErrPrivateIPResolved is returned when a hostname on the suffix
// allowlist (e.g. huggingface.co) resolves to an IP that falls inside
// the private / loopback / link-local / unspecified ranges. Defends
// against DNS rebinding + a compromised resolver returning 127.0.0.1
// / 169.254.169.254 (cloud IMDS) / RFC 1918 private space for an
// allowlisted public hostname.
//
// Localhost / 127.0.0.1 / ::1 entries on allowedHostSuffixes are
// loopback-trust by construction (test + dev workflows) and are NOT
// gated by this check — see verifyResolvedIPNotPrivate.
//
// Usage:
//
//	if errors.Is(err, downloader.ErrPrivateIPResolved) { ... }
var ErrPrivateIPResolved = core.NewError(
	"downloader: allowlisted hostname resolved to a private / loopback / link-local IP")
