// SPDX-Licence-Identifier: EUPL-1.2

package downloader

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"net"
	"net/http"
	"time"

	core "dappco.re/go"
)

// reasonTLSPinMismatch is the typed audit code emitted when a TLS
// handshake completes against a pinned host but the server's leaf-cert
// SPKI (subject public key info) sha256 does not match any pin in
// tlsPinnedHosts. Distinct from generic TLS-verify failures because
// the chain may have validated against the system trust store — pin
// mismatch is the additional signal that a fraudulently-issued cert
// or a state-CA / mis-issuance is in play.
const reasonTLSPinMismatch = "tls.pin_mismatch"

// tlsPinnedHosts is the compile-time map from hostname to the set of
// permitted SPKI sha256 fingerprints (base64-encoded, no padding —
// match the RFC 7469 "Public Key Pins" header shape). When the map
// entry for a hostname is non-empty, the downloader's TLS handshake
// is gated: the leaf-cert SPKI must match one of the listed pins or
// the connection is refused with reasonTLSPinMismatch.
//
// Mantis #1428 / Cerberus informational — TLS pinning framework. The
// pin SET is empty today because:
//
//   - The canonical SPKI source for huggingface.co + cdn-lfs.huggingface.co
//     is still TBD pending the marketplace manifest landing (#1438),
//     which should ship pins as first-class data alongside SHA-256
//     digests.
//   - Manually-extracted pins go stale fast (Let's Encrypt ~60 days,
//     other CAs longer but still rotate); hardcoding an unmaintained
//     pin is worse than no pin (it locks users out without warning).
//
// The FRAMEWORK is wired even with an empty set — verifyPeerCertPin
// fires on every handshake and is a no-op when the hostname is absent
// from tlsPinnedHosts. Adding a real pin is one entry per host once
// the source is confirmed:
//
//	tlsPinnedHosts["huggingface.co"] = []string{
//	    "abcdef0123456789...",   // SPKI sha256, base64-no-pad
//	}
//
// Extract real pins via:
//
//	openssl s_client -connect huggingface.co:443 -servername huggingface.co < /dev/null 2>/dev/null \
//	    | openssl x509 -pubkey -noout \
//	    | openssl pkey -pubin -outform DER \
//	    | openssl dgst -sha256 -binary \
//	    | base64
var tlsPinnedHosts = map[string][]string{
	// "huggingface.co":        {"<SPKI sha256, base64-no-pad>"},
	// "cdn-lfs.huggingface.co": {"<SPKI sha256, base64-no-pad>"},
}

// SPKIPin computes the base64-no-pad sha256 of the leaf cert's
// SubjectPublicKeyInfo — the value pinned in tlsPinnedHosts. Exposed
// (lowercase but called by tests) so the pinning math is testable
// without a live TLS handshake.
//
// Usage example:
//
//	pin := spkiPin(leafCert)
//	core.AssertEqual(t, expected, pin)
func spkiPin(leaf *x509.Certificate) string {
	sum := sha256.Sum256(leaf.RawSubjectPublicKeyInfo)
	return base64.RawStdEncoding.EncodeToString(sum[:])
}

// pinnedTransport returns an *http.Transport whose TLSClientConfig
// installs the pin-verify hook for every connection. Used by
// httpClient (see downloader.go) so all GET / redirect-hop / quarantine
// fetches inherit the pinning policy.
//
// The hook ALWAYS runs; when tlsPinnedHosts is empty (today's default)
// it accepts every chain. Wiring the framework up-front means adopting
// a real pin is a one-line map entry rather than a code-path rewrite.
//
// MinVersion TLS 1.2 (Go default is TLS 1.0 client-side on older Go).
// huggingface.co + every modern mirror serve TLS 1.2+; clamping
// closes the downgrade-attack surface for free.
//
// Usage example:
//
//	c := &core.HTTPClient{Transport: pinnedTransport()}
func pinnedTransport() *http.Transport {
	dialer := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}
	return &http.Transport{
		DialContext: dialer.DialContext,
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
			// VerifyConnection runs after the normal chain validation;
			// pull the negotiated server name and apply pin policy.
			VerifyConnection: func(cs tls.ConnectionState) error {
				if len(cs.PeerCertificates) == 0 {
					return nil
				}
				// Pin lookup keyed by ServerName (SNI) — matches the
				// hostname the user asked for, not whatever the cert
				// claims (subjectCN can lag SNI on multi-tenant CDNs).
				host := cs.ServerName
				pins, ok := tlsPinnedHosts[host]
				if !ok || len(pins) == 0 {
					return nil
				}
				got := spkiPin(cs.PeerCertificates[0])
				for _, want := range pins {
					if got == want {
						return nil
					}
				}
				return core.E(fetchOp,
					reasonTLSPinMismatch+": "+host+" got "+got, nil)
			},
		},
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          16,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
}
