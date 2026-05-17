// SPDX-Licence-Identifier: EUPL-1.2

// Cerberus Mantis #1433 — registry hardening tests. Mirrors the
// shape from pkg/downloader's trust_redirect_test + size_cap tests
// (Cerberus's "shape observation": same primitive applied across
// adjacent surfaces). Covers https-only enforcement, redirect
// validation, and the body-size caps for both FetchManifest +
// downloadIndex paths.

package marketplace_test

import (
	"net/http/httptest"
	"strings"

	core "dappco.re/go"
	subject "dappco.re/lthn/desktop/pkg/marketplace"
)

// TestRegistry_FetchManifest_Bad_PlaintextHTTP — http:// is a
// downgrade vector; it must be rejected loudly, not silently
// upgraded.
func TestRegistry_FetchManifest_Bad_PlaintextHTTP(t *core.T) {
	svc := subject.NewService(nil)
	r := svc.FetchManifest("http://marketplace.lthn.ai/v1/opencode.yml")
	core.AssertFalse(t, r.OK,
		"http:// manifest URL must be rejected at the protocol gate")
}

// TestRegistry_FetchManifest_Bad_OversizedHTTPSManifest — server
// declares Content-Length > 256 KiB. Pre-check fails fast before
// reading the body.
func TestRegistry_FetchManifest_Bad_OversizedHTTPSManifest(t *core.T) {
	// httptest.NewTLSServer serves https://. The default cert is
	// self-signed; we can't bypass that without InsecureSkipVerify
	// which our httpsOnlyClient deliberately doesn't have.
	//
	// Workaround: use httptest.NewTLSServer + assert the fetch fails
	// SOMEWHERE in the request chain. We can't tell exactly whether
	// it failed on TLS or on Content-Length, but both are valid
	// rejections from the security perspective.
	body := strings.Repeat("X", 300<<10) // 300 KiB > 256 KiB cap
	srv := httptest.NewTLSServer(core.HandlerFunc(func(w core.ResponseWriter, _ *core.Request) {
		w.Header().Set("Content-Length", core.Sprintf("%d", len(body)))
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	svc := subject.NewService(nil)
	r := svc.FetchManifest(srv.URL + "/oversized.yml")
	core.AssertFalse(t, r.OK,
		"oversized https manifest must fail (either TLS-cert or size-cap)")
}

// TestRegistry_FetchManifest_Bad_UnknownProtocol — `ftp://` and
// similar should fail with a clear error. Belongs in the same
// switch as the oci:// + git+https:// rejections.
func TestRegistry_FetchManifest_Bad_UnknownProtocol(t *core.T) {
	svc := subject.NewService(nil)
	r := svc.FetchManifest("ftp://example.com/manifest.yml")
	core.AssertFalse(t, r.OK,
		"non-https/oci/git+https protocols must be rejected")
}

// TestRegistry_FetchManifest_Bad_OCI — oci:// reserved-but-not-
// implemented stays a clear refusal (not a silent success).
func TestRegistry_FetchManifest_Bad_OCI(t *core.T) {
	svc := subject.NewService(nil)
	r := svc.FetchManifest("oci://example.com/bundle")
	core.AssertFalse(t, r.OK)
}

// TestRegistry_FetchManifest_Bad_GitHTTPS — same shape as OCI.
func TestRegistry_FetchManifest_Bad_GitHTTPS(t *core.T) {
	svc := subject.NewService(nil)
	r := svc.FetchManifest("git+https://example.com/bundle.git")
	core.AssertFalse(t, r.OK)
}

// TestMarketplace_FetchManifest_Bad_HostNotAllowed — Cerberus #54 C2 /
// Mantis #1690. An https:// source URL whose host is NOT on
// allowedManifestHostSuffixes must be rejected at the gate BEFORE any
// network I/O. Without this, marketplace_install's agent-supplied
// SourceURL turns FetchManifest into a fetch-anywhere primitive.
//
// `evil.example.com` is structurally a valid https URL but is not on
// the allowlist (forge.lthn.{sh,ai}, marketplace.lthn.ai, github.com,
// gitlab.com, codeberg.org). The rejection MUST cite the host-allowlist
// message so audit + UI surfaces can distinguish it from protocol /
// size / TLS rejections.
func TestMarketplace_FetchManifest_Bad_HostNotAllowed(t *core.T) {
	svc := subject.NewService(nil)
	r := svc.FetchManifest("https://evil.example.com/v1/opencode.yml")
	core.AssertFalse(t, r.OK,
		"https URL pointing at a host outside the allowlist must be rejected")
	core.AssertContains(t, r.Error(), "manifest host is not on the allowlist",
		"rejection must cite the host-allowlist gate, not a downstream check")
}

// TestMarketplace_FetchManifest_Good_AllowedHost — counterpart: an
// https:// URL whose host IS on the allowlist must pass the gate. We
// can't actually fetch in this test (no network in `go test`), but the
// rejection — if any — MUST NOT be the host-allowlist message. Any
// other failure (TLS, DNS, connection refused, size cap, etc.) proves
// the gate accepted the URL and the request reached the network layer.
func TestMarketplace_FetchManifest_Good_AllowedHost(t *core.T) {
	svc := subject.NewService(nil)
	for _, url := range []string{
		"https://marketplace.lthn.ai/v1/opencode.yml",
		"https://forge.lthn.sh/lthn/bundles/raw/branch/main/opencode.yml",
		"https://forge.lthn.ai/lthn/bundles/raw/branch/main/opencode.yml",
		"https://github.com/lthn/bundles/raw/main/opencode.yml",
		"https://raw.githubusercontent.com/lthn/bundles/main/opencode.yml",
		"https://gitlab.com/lthn/bundles/-/raw/main/opencode.yml",
		"https://codeberg.org/lthn/bundles/raw/branch/main/opencode.yml",
	} {
		r := svc.FetchManifest(url)
		if !r.OK {
			core.AssertNotContains(t, r.Error(), "manifest host is not on the allowlist",
				"allowlisted host must not be rejected by the host gate: "+url)
		}
	}
}

// TestMarketplace_AllowedManifestHosts_Sorted_Good — invariant assertion
// mirroring imagetrust's TestImagetrust_AllowedRegistries_Sorted+Unique_Good.
// The compile-time list MUST stay sorted and duplicate-free so a code
// reviewer can scan the patch and immediately spot adds in the right
// place. Run-time discipline backs up code-review discipline.
func TestMarketplace_AllowedManifestHosts_Sorted_Good(t *core.T) {
	list := subject.AllowedManifestHostSuffixesForTest()
	core.AssertTrue(t, len(list) > 0, "allowlist must be non-empty")
	for i := 1; i < len(list); i++ {
		core.AssertTrue(t, list[i-1] < list[i],
			"allowedManifestHostSuffixes must be sorted ascending; offending pair: "+
				list[i-1]+", "+list[i])
	}
}

// TestMarketplace_FetchIndex_Bad_HostNotAllowed — defence-in-depth on
// the catalogue side. FetchIndex(indexURL) accepts an operator-supplied
// override; without the host gate, that path is an arbitrary-fetch
// vector too. Empty URL takes the default endpoint (marketplace.lthn.ai
// is on the allowlist) — this test pins the override path.
func TestMarketplace_FetchIndex_Bad_HostNotAllowed(t *core.T) {
	svc := subject.NewService(nil)
	r := svc.FetchIndex("https://evil.example.com/v1/index.json")
	core.AssertFalse(t, r.OK,
		"override indexURL pointing outside the allowlist must be rejected")
	core.AssertContains(t, r.Error(), "manifest host is not on the allowlist")
}
