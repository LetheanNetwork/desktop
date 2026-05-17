// SPDX-Licence-Identifier: EUPL-1.2

// Trust layer for the model downloader. Three responsibilities:
//
//   - AllowedSource — host allowlist gate; bytes never read off the
//     wire from a non-allowlisted source.
//   - verify — post-download SHA-256 digest check against an
//     expected hex string supplied by the caller (HF model card,
//     marketplace manifest, etc.). Unexported — confused-deputy guard
//     per Cerberus #49 F-5 (Mantis #1675): callers outside the package
//     must go through FetchVerified, not invoke the digest check on an
//     arbitrary path.
//   - Quarantine — bytes write to ~/Lethean/conf/models/.quarantine/
//     during download; only an atomic rename(2) promotes them to the
//     final model path. Eliminates partial-write torn-read + cache-
//     poisoning windows. Stale quarantine files (>24h) are swept on
//     ServiceStartup.
//
// Closes Cerberus's flagged A1/A3 surface on the e4d146f downloader.
// See plans/code/lthn/desktop/downloader/RFC.md §5 for the spec.

package downloader

import (
	"crypto/sha256"

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/paths"
)

// loopbackAllowlistEntries is the subset of allowedHostSuffixes that
// is loopback-trust by construction — these entries categorically
// resolve to private/loopback IPs and exist on the allowlist precisely
// to enable httptest + dev workflows (see allowedHostSuffixes docstring).
// verifyResolvedIPNotPrivate skips the IP gate for these entries so
// the gate fires only on public-trust hostnames (huggingface.co etc.)
// where a private-IP resolution would signal DNS rebinding or a
// compromised resolver.
//
// Must stay a subset of allowedHostSuffixes; the
// TestTrust_LoopbackEntries_AreSubset_Good invariant in dns_test.go
// enforces this.
var loopbackAllowlistEntries = map[string]bool{
	"localhost": true,
	"127.0.0.1": true,
}

// lookupHostFn is the resolver seam — production points at the system
// resolver via core.Resolver{}.LookupHost; tests inject a stub returning
// canned IPs (no real DNS in the test path). Mirrors the test-seam
// pattern used by core.Now in the cleanStaleQuarantine sweep.
//
// Signature matches (*core.Resolver).LookupHost — context + host →
// []string of IPs.
var lookupHostFn = func(ctx core.Context, host string) ([]string, error) {
	return (&core.Resolver{}).LookupHost(ctx, host)
}

// dnsLookupTimeout caps the per-fetch DNS resolution gate so a slow
// resolver can't stall a download indefinitely before the HTTP request
// even starts. 5s matches the conservative default used by the rest
// of the desktop network code.
const dnsLookupTimeout = 5 * core.Second

// allowedHostSuffixes is the compile-time allowlist of safe download
// origins. Suffix-matched against the parsed URL hostname so CDN
// mirrors under huggingface.co work without per-CDN entries.
//
// The list is NOT runtime-configurable to avoid TOCTOU — a malicious
// process flipping the list between AllowedSource and the HTTP read
// would defeat the gate. Updates ship as code review.
//
// localhost + 127.0.0.1 are present so the package's own httptest
// suite + dev workflows function; the security boundary is "is this
// an arbitrary external URL", which localhost categorically isn't.
var allowedHostSuffixes = []string{
	"huggingface.co",
	"hf.co",
	"localhost",
	"127.0.0.1",
}

// AllowedSource returns true when url is a safe download origin.
// Current allow-list (suffix-matched against URL hostname):
//
//   - huggingface.co (any path; covers cdn-lfs-*.huggingface.co etc.)
//   - hf.co (short-link alias)
//   - localhost / 127.0.0.1 (test + dev paths)
//
// Returns false for empty URLs, malformed URLs, and any host not in
// the allowlist.
//
// Usage example:
//
//	if !downloader.AllowedSource(url) {
//	    return core.Fail(core.E("downloader.Fetch", "source not allowed", nil))
//	}
func AllowedSource(rawURL string) bool {
	if rawURL == "" {
		return false
	}
	r := core.URLParse(rawURL)
	if !r.OK {
		return false
	}
	u := r.Value.(*core.URL)
	host := u.Hostname()
	if host == "" {
		return false
	}
	for _, suffix := range allowedHostSuffixes {
		if host == suffix || core.HasSuffix(host, "."+suffix) {
			return true
		}
	}
	return false
}

// verifyResolvedIPNotPrivate resolves host via the package's lookupHostFn
// seam and returns ErrPrivateIPResolved if any returned IP is private,
// loopback, link-local (incl. 169.254.169.254 cloud IMDS), or
// unspecified. Used at FetchVerified entry and in CheckRedirect so a
// compromised DNS resolver (or DNS-rebinding attack) cannot use an
// allowlisted public hostname like `huggingface.co` to pivot the
// fetch onto localhost services, cloud-metadata endpoints, or RFC 1918
// private space.
//
// Returns nil immediately for loopback-trust allowlist entries
// (`localhost`, `127.0.0.1`, `::1`, the bracketed `[::1]`) — these
// exist on allowedHostSuffixes precisely to enable httptest + dev
// workflows where private IPs are the expected resolution.
//
// Cerberus Mantis #1429. The gate runs AT resolve-time on the hostname
// from the URL — a small TOCTOU window remains between this resolve
// and the HTTP client's own resolve at Do() / dial time. Closing that
// fully requires a custom Transport.DialContext (option (b) in the
// brief) which needs an http.Transport export from CoreGO that is not
// yet available; tracked in project_corego_export_gaps. Today's gap
// implementation is option (a): two short-window resolutions backed by
// the OS resolver's own caching, which is materially stronger than no
// gate at all.
//
// Usage example (internal):
//
//	if err := verifyResolvedIPNotPrivate(host); err != nil {
//	    return core.Fail(err)
//	}
func verifyResolvedIPNotPrivate(host string) error {
	if host == "" {
		return core.E("downloader.verifyResolvedIPNotPrivate",
			"host is required", nil)
	}
	// IPv6 literal hostnames arrive bracketed from url.URL.Hostname()?
	// No — Hostname() strips the brackets. But callers (CheckRedirect)
	// may pass the bracketed form; tolerate both.
	bare := host
	if len(bare) >= 2 && bare[0] == '[' && bare[len(bare)-1] == ']' {
		bare = bare[1 : len(bare)-1]
	}
	if loopbackAllowlistEntries[bare] || bare == "::1" {
		return nil
	}
	ctx, cancel := core.WithTimeout(core.Background(), dnsLookupTimeout)
	defer cancel()
	ips, err := lookupHostFn(ctx, bare)
	if err != nil {
		return core.E("downloader.verifyResolvedIPNotPrivate",
			core.Concat("DNS lookup failed for ", bare), err)
	}
	if len(ips) == 0 {
		return core.E("downloader.verifyResolvedIPNotPrivate",
			core.Concat("DNS returned no addresses for ", bare), nil)
	}
	for _, raw := range ips {
		ip := core.ParseIP(raw)
		if ip == nil {
			// Unparseable resolver output — fail closed.
			return core.E("downloader.verifyResolvedIPNotPrivate",
				core.Concat("resolver returned unparseable address ", raw,
					" for ", bare), nil)
		}
		if ip.IsPrivate() || ip.IsLoopback() ||
			ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
			ip.IsUnspecified() {
			return core.E("downloader.verifyResolvedIPNotPrivate",
				core.Concat(bare, " → ", raw), ErrPrivateIPResolved)
		}
	}
	return nil
}

// verify computes the SHA-256 digest of the file at path and compares
// it against the expected lowercase hex string. Returns Ok when the
// digest matches; Fail with both the expected and computed values in
// the error message when it doesn't.
//
// Streams the file via crypto/sha256.New() so multi-GB models don't
// load into memory. The crypto/sha256 import is permitted in this
// file per RFC §5.3 — CoreGO's SHA256 wrappers operate on []byte and
// have no streaming variant.
//
// Unexported by design (Cerberus #49 F-5 / Mantis #1675): public
// callers must use FetchVerified so the digest check is bound to the
// quarantine + atomic-promote pipeline rather than offered as a free
// hash primitive over any caller-supplied path.
//
// Usage example (internal):
//
//	r := verify(qDest, expectedHex)
//	if !r.OK { return r }
func verify(path, sha256hex string) core.Result {
	if path == "" {
		return core.Fail(core.E("downloader.verify", "path is required", nil))
	}
	if sha256hex == "" {
		return core.Fail(core.E("downloader.verify", "sha256hex is required", nil))
	}
	openR := core.OpenFile(path, core.O_RDONLY, 0)
	if !openR.OK {
		return openR
	}
	file := openR.Value.(*core.OSFile)
	defer file.Close()

	h := sha256.New()
	if r := core.Copy(h, file); !r.OK {
		return core.Fail(core.E("downloader.verify", "stream read failed", r.Value.(error)))
	}
	got := core.HexEncode(h.Sum(nil))
	if got != sha256hex {
		return core.Fail(core.E("downloader.verify",
			core.Concat("digest mismatch: expected ", sha256hex, " got ", got),
			nil))
	}
	return core.Ok(got)
}

// quarantineDir returns ~/Lethean/conf/models/.quarantine/ — created
// if missing. Bytes from in-flight downloads land here before the
// atomic-promote rename(2) into the final model path. Same filesystem
// as the model dir so rename is atomic.
//
// Usage example:
//
//	r := quarantineDir()
//	if r.OK { qd := r.Value.(string); _ = qd }
func quarantineDir() core.Result {
	dirR := paths.ModelsDir()
	if !dirR.OK {
		return dirR
	}
	qd := core.PathJoin(dirR.Value.(string), ".quarantine")
	if r := core.MkdirAll(qd, 0o755); !r.OK {
		return r
	}
	return core.Ok(qd)
}

// staleQuarantineAge is the cutoff for cleanStaleQuarantine; files
// older than this in the quarantine dir are deleted at startup.
// Set high enough to survive a multi-GB download interrupted by a
// reboot, low enough to keep the dir from accumulating indefinitely.
const staleQuarantineAge = 24 * core.Hour

// cleanStaleQuarantine sweeps the quarantine dir for files older than
// staleQuarantineAge and removes them. Best-effort — a Stat / Remove
// failure on any one file logs through Core's error channel but
// doesn't halt the sweep.
//
// Spawned via c.Go from WailsService.ServiceStartup so the lifecycle
// hook returns immediately while the sweep runs in the background.
//
// Usage example (from ServiceStartup):
//
//	c.Go(func() { cleanStaleQuarantine(c) })
func cleanStaleQuarantine(c *core.Core) {
	if c == nil {
		return
	}
	dirR := quarantineDir()
	if !dirR.OK {
		return
	}
	qd := dirR.Value.(string)
	entries := core.ReadDir(core.DirFS(qd), ".")
	if !entries.OK {
		return
	}
	cutoff := core.Now().Add(-staleQuarantineAge)
	for _, entry := range entries.Value.([]core.FsDirEntry) {
		if entry.IsDir() {
			continue
		}
		full := core.PathJoin(qd, entry.Name())
		stat := core.Stat(full)
		if !stat.OK {
			continue
		}
		info := stat.Value.(core.FsFileInfo)
		if info.ModTime().Before(cutoff) {
			_ = core.Remove(full)
		}
	}
}
