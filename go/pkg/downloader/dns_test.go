// SPDX-Licence-Identifier: EUPL-1.2

// Cerberus Mantis #1429 — DNS rebinding / SSRF defence at the network-
// resolve tier. The host allowlist gates on hostname; without this
// check a malicious resolver returning 127.0.0.1 / 169.254.169.254 /
// RFC 1918 private space for an allowlisted hostname could pivot the
// fetcher onto localhost services, cloud-metadata endpoints, or the
// private network.
//
// Internal package (downloader, not downloader_test) so the resolver
// seam lookupHostFn is reachable without exporting it. Pattern matches
// trust_internal_test.go.

package downloader

import (
	"net/http/httptest"

	core "dappco.re/go"
)

// stubResolver swaps lookupHostFn for the duration of the test and
// restores the original on cleanup. Canned IPs let us assert the gate
// shape without depending on real DNS in the test path.
func stubResolver(t *core.T, ips map[string][]string) {
	t.Helper()
	prev := lookupHostFn
	lookupHostFn = func(ctx core.Context, host string) ([]string, error) {
		if got, ok := ips[host]; ok {
			return got, nil
		}
		return nil, core.NewError("stubResolver: no canned answer for " + host)
	}
	t.Cleanup(func() { lookupHostFn = prev })
}

// TestTrust_LoopbackEntries_AreSubset_Good — loopbackAllowlistEntries
// is the subset of allowedHostSuffixes whose IP-gate is skipped.
// Drift between the two would either re-introduce the DNS-rebinding
// gap (entry missing here) or break httptest (entry missing in the
// suffix allowlist).
func TestTrust_LoopbackEntries_AreSubset_Good(t *core.T) {
	suffixes := map[string]bool{}
	for _, s := range allowedHostSuffixes {
		suffixes[s] = true
	}
	for entry := range loopbackAllowlistEntries {
		core.AssertTrue(t, suffixes[entry],
			core.Concat("loopbackAllowlistEntries entry not in allowedHostSuffixes: ", entry))
	}
}

// TestTrust_VerifyResolvedIPNotPrivate_Good_PublicIP — a public IP is
// the happy path. Returns nil.
func TestTrust_VerifyResolvedIPNotPrivate_Good_PublicIP(t *core.T) {
	stubResolver(t, map[string][]string{
		"huggingface.co": {"3.5.79.10"}, // arbitrary public space
	})
	core.AssertEqual(t, nil, verifyResolvedIPNotPrivate("huggingface.co"))
}

// TestTrust_VerifyResolvedIPNotPrivate_Good_Loopback — loopback-trust
// entries skip the lookup entirely (no resolver call). Test by NOT
// stubbing the resolver — any call would fail the test.
func TestTrust_VerifyResolvedIPNotPrivate_Good_Loopback(t *core.T) {
	prev := lookupHostFn
	lookupHostFn = func(ctx core.Context, host string) ([]string, error) {
		t.Fatal(core.Concat(
			"verifyResolvedIPNotPrivate must not resolve loopback entries; called for ", host))
		return nil, nil
	}
	t.Cleanup(func() { lookupHostFn = prev })
	core.AssertEqual(t, nil, verifyResolvedIPNotPrivate("localhost"))
	core.AssertEqual(t, nil, verifyResolvedIPNotPrivate("127.0.0.1"))
	core.AssertEqual(t, nil, verifyResolvedIPNotPrivate("::1"))
	core.AssertEqual(t, nil, verifyResolvedIPNotPrivate("[::1]"))
}

// TestTrust_VerifyResolvedIPNotPrivate_Bad_Loopback_v4 — allowlisted
// hostname resolves to 127.0.0.1 (DNS rebinding to a localhost
// service). MUST reject with ErrPrivateIPResolved.
func TestTrust_VerifyResolvedIPNotPrivate_Bad_Loopback_v4(t *core.T) {
	stubResolver(t, map[string][]string{
		"huggingface.co": {"127.0.0.1"},
	})
	err := verifyResolvedIPNotPrivate("huggingface.co")
	core.AssertTrue(t, err != nil)
	core.AssertTrue(t, core.Is(err, ErrPrivateIPResolved),
		"expected ErrPrivateIPResolved")
}

// TestTrust_VerifyResolvedIPNotPrivate_Bad_Loopback_v6 — IPv6 ::1.
func TestTrust_VerifyResolvedIPNotPrivate_Bad_Loopback_v6(t *core.T) {
	stubResolver(t, map[string][]string{
		"huggingface.co": {"::1"},
	})
	err := verifyResolvedIPNotPrivate("huggingface.co")
	core.AssertTrue(t, core.Is(err, ErrPrivateIPResolved))
}

// TestTrust_VerifyResolvedIPNotPrivate_Bad_CloudIMDS — 169.254.169.254
// is the AWS/GCP/Azure/DigitalOcean cloud metadata service endpoint.
// IsLinkLocalUnicast covers the whole 169.254/16 range.
func TestTrust_VerifyResolvedIPNotPrivate_Bad_CloudIMDS(t *core.T) {
	stubResolver(t, map[string][]string{
		"huggingface.co": {"169.254.169.254"},
	})
	err := verifyResolvedIPNotPrivate("huggingface.co")
	core.AssertTrue(t, core.Is(err, ErrPrivateIPResolved))
}

// TestTrust_VerifyResolvedIPNotPrivate_Bad_RFC1918 — RFC 1918 private
// space (10/8, 172.16/12, 192.168/16). IsPrivate covers all three.
func TestTrust_VerifyResolvedIPNotPrivate_Bad_RFC1918(t *core.T) {
	for _, ip := range []string{"10.0.0.1", "172.16.0.1", "192.168.1.1"} {
		stubResolver(t, map[string][]string{
			"huggingface.co": {ip},
		})
		err := verifyResolvedIPNotPrivate("huggingface.co")
		core.AssertTrue(t, core.Is(err, ErrPrivateIPResolved),
			core.Concat("expected ErrPrivateIPResolved for ", ip))
	}
}

// TestTrust_VerifyResolvedIPNotPrivate_Bad_IPv6Private — IPv6 ULA
// (fc00::/7) + link-local (fe80::/10). IsPrivate covers ULA;
// IsLinkLocalUnicast covers fe80::.
func TestTrust_VerifyResolvedIPNotPrivate_Bad_IPv6Private(t *core.T) {
	for _, ip := range []string{"fc00::1", "fd00::1", "fe80::1"} {
		stubResolver(t, map[string][]string{
			"huggingface.co": {ip},
		})
		err := verifyResolvedIPNotPrivate("huggingface.co")
		core.AssertTrue(t, core.Is(err, ErrPrivateIPResolved),
			core.Concat("expected ErrPrivateIPResolved for ", ip))
	}
}

// TestTrust_VerifyResolvedIPNotPrivate_Bad_Unspecified — 0.0.0.0 / ::
// are the unspecified addresses; resolving an allowlisted name to
// these is categorically wrong + should fail closed.
func TestTrust_VerifyResolvedIPNotPrivate_Bad_Unspecified(t *core.T) {
	for _, ip := range []string{"0.0.0.0", "::"} {
		stubResolver(t, map[string][]string{
			"huggingface.co": {ip},
		})
		err := verifyResolvedIPNotPrivate("huggingface.co")
		core.AssertTrue(t, core.Is(err, ErrPrivateIPResolved),
			core.Concat("expected ErrPrivateIPResolved for ", ip))
	}
}

// TestTrust_VerifyResolvedIPNotPrivate_Bad_MixedSet — when ANY of the
// resolved IPs is private the gate fires. A resolver that returns
// [public, private] would otherwise let the dialer pick the private
// one on retry (per happy-eyeballs / round-robin) — fail closed on
// first private hit.
func TestTrust_VerifyResolvedIPNotPrivate_Bad_MixedSet(t *core.T) {
	stubResolver(t, map[string][]string{
		"huggingface.co": {"3.5.79.10", "127.0.0.1"},
	})
	err := verifyResolvedIPNotPrivate("huggingface.co")
	core.AssertTrue(t, core.Is(err, ErrPrivateIPResolved))
}

// TestTrust_VerifyResolvedIPNotPrivate_Bad_EmptyResolution — a resolver
// that returns no addresses cannot prove the gate; fail closed.
func TestTrust_VerifyResolvedIPNotPrivate_Bad_EmptyResolution(t *core.T) {
	stubResolver(t, map[string][]string{
		"huggingface.co": {},
	})
	err := verifyResolvedIPNotPrivate("huggingface.co")
	core.AssertTrue(t, err != nil)
}

// TestTrust_VerifyResolvedIPNotPrivate_Bad_ResolverError — DNS lookup
// itself errored. Fail closed (no fetch on resolver outage).
func TestTrust_VerifyResolvedIPNotPrivate_Bad_ResolverError(t *core.T) {
	// no stub for the requested host → stubResolver returns an error.
	stubResolver(t, map[string][]string{})
	err := verifyResolvedIPNotPrivate("huggingface.co")
	core.AssertTrue(t, err != nil)
}

// TestTrust_VerifyResolvedIPNotPrivate_Ugly_EmptyHost — bare-minimum
// guard against caller passing "" through.
func TestTrust_VerifyResolvedIPNotPrivate_Ugly_EmptyHost(t *core.T) {
	err := verifyResolvedIPNotPrivate("")
	core.AssertTrue(t, err != nil)
}

// TestTrust_FetchVerified_Bad_PrivateIPResolved — end-to-end: the
// FetchVerified entry-point rejects an allowlisted hostname that
// resolves to a private IP before any bytes flow. Pairs the gate
// with its caller so a future regression that bypasses the call site
// is caught at the integration boundary, not just the unit boundary.
func TestTrust_FetchVerified_Bad_PrivateIPResolved(t *core.T) {
	internalHomeFixture(t)
	// Stand up a real httptest server (binds 127.0.0.1) but rewrite
	// its URL to a fake allowlisted hostname. The resolver stub then
	// pins that hostname to 169.254.169.254. We expect ErrPrivateIPResolved
	// BEFORE any GET issues.
	srv := httptest.NewServer(core.HandlerFunc(func(w core.ResponseWriter, _ *core.Request) {
		t.Fatal("FetchVerified must not issue HTTP when DNS gate rejects")
	}))
	defer srv.Close()
	// Build an https-shaped URL whose hostname is allowlisted but whose
	// IP we'll pin to a cloud-IMDS address. Path doesn't matter; the
	// gate fires before connect.
	url := "https://huggingface.co/snider/model/resolve/main/m.gguf"
	stubResolver(t, map[string][]string{
		"huggingface.co": {"169.254.169.254"},
	})
	r := FetchVerified(url, "m.gguf", "", nil)
	core.AssertFalse(t, r.OK)
	core.AssertTrue(t, core.Is(r.Value.(error), ErrPrivateIPResolved),
		"expected ErrPrivateIPResolved")
}
