// SPDX-Licence-Identifier: EUPL-1.2

// Package downloader fetches model files into ~/Lethean/conf/models/
// via the dappco.re/go HTTP wrappers. HuggingFace and any
// direct-link source work as long as the URL is reachable.
//
// Two surfaces:
//
//   - Fetch(url, name)                       — blocking, no measurement
//   - FetchWithProgress(url, name, callback) — blocking, fires the
//     callback after every chunk read so a Wails-bound consumer can
//     emit cumulative-bytes events into the WebView event bus.
//
// Sovereign-rootFS arc (banked, not in this layer): when the
// integration lands, route writes through Snider/Borg/pkg/datanode
// + Enchantrix so model files live as content-addressed lthnHash
// blobs under ~/Lethean/drive/{lthnHash(folder)}/{lthnHash(file)}
// per the triadic Borg+Enchantrix+Poindexter shape. Today: plain
// HTTP→file.
//
// Usage example:
//
//	r := downloader.Fetch(
//	    "https://huggingface.co/.../resolve/main/model.gguf",
//	    "gemma-4-e2b.gguf",
//	)
//	if !r.OK { return r }
//	dest := r.Value.(string)
package downloader

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/paths"
)

const fetchOp = "downloader.Fetch"

// maxDownloadBytes is the hard cap on a single fetched file. Sized to
// fit the largest GGUF distributed on HuggingFace today (≈140GB for
// 4-bit Qwen 235B, ≈70GB for gpt-oss-120B) plus headroom. Bumping
// requires a code review per the same TOCTOU rationale that keeps the
// allow-list compile-time.
//
// Cerberus Mantis #1425 — without a cap, a malicious or compromised
// allowlisted mirror can stream unbounded bytes regardless of any
// Content-Length value, exhausting disk + crashing the host.
//
// var (not const) so the package-internal _test.go can shrink it for
// overflow assertions that would otherwise need to write 256 GiB to a
// tempdir. Production callers never mutate it.
var maxDownloadBytes int64 = 256 << 30 // 256 GiB

// maxRedirects mirrors the Go default (10) but is enforced explicitly
// in CheckRedirect so the policy is auditable in this file.
const maxRedirects = 10

// httpClient is the package-private fetch client. CheckRedirect
// re-validates every hop through AllowedSource — Cerberus Mantis
// #1424 closed the gap where core.HTTPGet (= http.Get) followed any
// 302 from an allowlisted host to an arbitrary URL without re-checking
// the trust gate.
//
// Built once, reused for every fetch. Transport is pinnedTransport()
// — the TLS pinning FRAMEWORK is wired even when the pin SET is
// empty (today's default per Mantis #1428). When marketplace manifest
// (#1438) ships SPKI fingerprints, adding a real pin is one entry in
// tlsPinnedHosts (see tlspin.go) — no code-path change. MinVersion
// is clamped to TLS 1.2 in the transport regardless of pin state.
var httpClient = &core.HTTPClient{
	Transport: pinnedTransport(),
	CheckRedirect: func(req *core.Request, via []*core.Request) error {
		if len(via) >= maxRedirects {
			return core.NewError("downloader: stopped after " +
				core.Sprintf("%d", maxRedirects) + " redirects")
		}
		next := req.URL.String()
		if !AllowedSource(next) {
			return core.NewError("downloader: redirect to disallowed source: " + next)
		}
		// Cerberus Mantis #1429 — re-gate at every hop. A 302 from an
		// allowlisted CDN to e.g. `huggingface.co` whose DNS now points
		// at 169.254.169.254 must reject here, not after the dialer
		// connects to the metadata service.
		if err := verifyResolvedIPNotPrivate(req.URL.Hostname()); err != nil {
			return err
		}
		return nil
	},
}

// nameLocks serialises FetchVerified calls per quarantine basename
// (Cerberus #49 F-3). The map is guarded by nameLocksMu; the values
// are per-name *core.Mutex used to gate Create → Copy → Verify →
// Rename so two parallel calls with the same name can't interleave
// and let bytes B promote under digest A. Map entries are never
// removed — a fixed maximum of distinct model names per process keeps
// memory bounded in practice; if that assumption ever breaks, a refcount-
// and-evict pattern can replace this without changing call sites.
var (
	nameLocksMu core.Mutex
	nameLocks   = map[string]*core.Mutex{}
)

// lockName returns the unlock function for the per-name mutex
// guarding name's quarantine path. Callers MUST defer the returned
// function. Lazily creates the mutex on first use under nameLocksMu.
//
// Usage example:
//
//	unlock := lockName(name)
//	defer unlock()
//	// ... Create / Copy / Verify / Rename ...
func lockName(name string) func() {
	nameLocksMu.Lock()
	mu, ok := nameLocks[name]
	if !ok {
		mu = &core.Mutex{}
		nameLocks[name] = mu
	}
	nameLocksMu.Unlock()
	mu.Lock()
	return mu.Unlock
}

// Progress is the callback signature for FetchWithProgress reports.
// Receives (bytesWritten, totalBytes) — totalBytes mirrors
// http.Response.ContentLength: positive when the server sent a
// Content-Length header, -1 for chunked transfer (some HF mirrors).
// Fires on every chunk read; consumers that need throttling (Wails
// event-bus emit, UI repaints) wrap their own.
//
// Usage example:
//
//	downloader.FetchWithProgress(url, name, func(written, total int64) {
//	    pct := 0.0
//	    if total > 0 { pct = 100 * float64(written) / float64(total) }
//	    core.Print(core.Stdout(), "\r%.1f%% (%d/%d)", pct, written, total)
//	})
type Progress func(written, total int64)

// Fetch downloads url and writes it to ~/Lethean/conf/models/<name>.
// Overwrites any existing file at the destination. Returns the
// absolute destination path on success. Convenience wrapper around
// FetchWithProgress with no callback.
//
// Usage example:
//
//	r := downloader.Fetch("https://example.com/model.gguf", "model.gguf")
//	if r.OK { dest := r.Value.(string); _ = dest }
func Fetch(url, name string) core.Result {
	return FetchWithProgress(url, name, nil)
}

// FetchWithProgress is Fetch with a Progress callback fired after
// every chunk read off the wire. nil callback = no measurement
// overhead (identical to Fetch). Equivalent to FetchVerified with an
// empty digest.
//
// Usage example:
//
//	r := downloader.FetchWithProgress(url, name, func(w, t int64) {
//	    emit("dl:progress", map[string]any{"written": w, "total": t})
//	})
//	if r.OK { dest := r.Value.(string); _ = dest }
func FetchWithProgress(url, name string, onProgress Progress) core.Result {
	return FetchVerified(url, name, "", onProgress)
}

// FetchVerified is the trust-aware fetch primitive. Source is gated
// through AllowedSource; bytes write to ~/Lethean/conf/models/.quarantine/<name>
// during download; if sha256hex is non-empty the digest is checked
// post-download via Verify; on success the file is atomically promoted
// to ~/Lethean/conf/models/<name> via rename(2). On failure the
// quarantine file is removed so partial / unverified bytes never reach
// the model path.
//
// sha256hex of "" skips the verify step but still routes through
// quarantine + atomic-promote — eliminating the partial-write torn-
// read window even for non-checksummed downloads.
//
// Usage example:
//
//	r := downloader.FetchVerified(url, name, "abc...def", onProgress)
//	if r.OK { dest := r.Value.(string); _ = dest }
func FetchVerified(url, name, sha256hex string, onProgress Progress) core.Result {
	if url == "" {
		err := core.E(fetchOp, "url is required", nil)
		emitFetchFailed(url, core.Fail(err))
		return core.Fail(err)
	}
	if name == "" {
		err := core.E(fetchOp, "name is required", nil)
		emitFetchFailed(url, core.Fail(err))
		return core.Fail(err)
	}
	// Cerberus #49 F-1 — name flows through core.PathJoin into both
	// the quarantine path and the final model path. core.PathJoin
	// COLLAPSES `..` segments but does not REJECT them, so a renderer-
	// supplied name like "../../wallets/server" would silently land
	// outside ~/Lethean/conf/models/. paths.IsValidID is the shared
	// shape gate (no `/`, no `..`, no `\\`, no NUL, no leading `.`,
	// no >255 bytes) used by the 20+ wails surfaces hardened under
	// Cerberus #1486. Mirror the discipline here so the model-path
	// surface can't pivot writes/reads outside the models dir.
	if err := paths.IsValidID(name); err != nil {
		emitFetchFailed(url, core.Fail(err))
		return core.Fail(err)
	}
	// Cerberus shape-observation (audit-cluster adoption) — emit the
	// Requested row AFTER the input-shape gates pass but BEFORE the
	// host-allowlist + DNS-resolve-private-IP policy gates. The pre-gate
	// position lets a forensic walker correlate caller intent (which
	// host) with the gate outcome (which policy fired) in the same
	// trace; the post-shape-gate position keeps the substrate from
	// recording rows for trivially-malformed callers that never had a
	// parseable URL.
	emitFetchRequested(url, sha256hex)

	if !AllowedSource(url) {
		emitFetchRejected(url, reasonHostNotAllowed)
		return core.Fail(core.E(fetchOp,
			core.Concat("source not allowed: ", url), nil))
	}
	// Cerberus Mantis #1429 — DNS rebinding / SSRF defence at the
	// network-resolve tier. The hostname is on the allowlist; verify
	// it doesn't resolve to a private / loopback / link-local / cloud-
	// IMDS address. Reject BEFORE the HTTP request issues so no bytes
	// flow to internal services even on a one-shot rebind. Same gate
	// fires on each redirect hop via CheckRedirect above.
	parsed := core.URLParse(url)
	if !parsed.OK {
		err := core.E(fetchOp,
			core.Concat("URL parse failed: ", url), parsed.Value.(error))
		emitFetchFailed(url, core.Fail(err))
		return core.Fail(err)
	}
	if err := verifyResolvedIPNotPrivate(parsed.Value.(*core.URL).Hostname()); err != nil {
		emitFetchRejected(url, reasonPrivateIPResolved)
		return core.Fail(err)
	}
	dirR := paths.ModelsDir()
	if !dirR.OK {
		emitFetchFailed(url, dirR)
		return dirR
	}
	// Belt-and-braces partner of IsValidID — if a future regression
	// loosens the shape gate or core.PathJoin's cleaning behaviour
	// shifts under us, JoinAndCheck refuses to return a path that
	// escapes the models dir. Same discipline as the cascade closure
	// across sales/incidents/runbooks/marketing.
	finalDest, escErr := paths.JoinAndCheck(dirR.Value.(string), name)
	if escErr != nil {
		emitFetchFailed(url, core.Fail(escErr))
		return core.Fail(escErr)
	}

	qdR := quarantineDir()
	if !qdR.OK {
		emitFetchFailed(url, qdR)
		return qdR
	}
	qDest, escErr := paths.JoinAndCheck(qdR.Value.(string), name)
	if escErr != nil {
		emitFetchFailed(url, core.Fail(escErr))
		return core.Fail(escErr)
	}

	// Cerberus #49 F-3 — serialise FetchVerified calls that target
	// the same quarantine basename so two parallel downloads can't
	// interleave Create / Copy / Verify / Rename and let bytes B
	// promote under digest A. Per-name mutex keeps unrelated names
	// running in parallel while same-name calls queue.
	unlock := lockName(name)
	defer unlock()

	reqR := core.NewHTTPRequest("GET", url, nil)
	if !reqR.OK {
		emitFetchFailed(url, reqR)
		return reqR
	}
	req := reqR.Value.(*core.Request)
	resp, err := httpClient.Do(req)
	if err != nil {
		// On CheckRedirect rejection the http client may return a
		// non-nil resp whose body still needs closing. Be defensive.
		if resp != nil {
			_ = resp.Body.Close()
		}
		failErr := core.E(fetchOp, "GET failed", err)
		emitFetchFailed(url, core.Fail(failErr))
		return core.Fail(failErr)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		failErr := core.E(fetchOp,
			core.Concat("HTTP ", core.Sprintf("%d", resp.StatusCode), " from ", url),
			nil)
		emitFetchFailed(url, core.Fail(failErr))
		return core.Fail(failErr)
	}
	// Cerberus #1425 — pre-check Content-Length when the server sent
	// one. Saves the disk + bandwidth of opening the body for a stream
	// we already know will be rejected. -1 = chunked transfer; the
	// LimitReader wrap below catches the unbounded case.
	if resp.ContentLength > maxDownloadBytes {
		failErr := core.E(fetchOp,
			core.Concat("Content-Length ",
				core.Sprintf("%d", resp.ContentLength),
				" exceeds cap ",
				core.Sprintf("%d", maxDownloadBytes)),
			nil)
		emitFetchFailed(url, core.Fail(failErr))
		return core.Fail(failErr)
	}

	// Cerberus #49 F-4 (Mantis #1674) — quarantine open MUST refuse
	// pre-existing files AND symlinks. core.Create is os.Create =
	// O_RDWR|O_CREATE|O_TRUNC with no O_NOFOLLOW; an attacker with
	// filesystem write access to the quarantine dir could pre-plant
	// `quarantine/<name> -> ~/.ssh/authorized_keys` and have the
	// downloader truncate-and-write through it. createQuarantineFile
	// opens with O_CREATE|O_EXCL|O_WRONLY|O_NOFOLLOW (POSIX) so
	// pre-existing entries refuse with download.quarantine_exists
	// and symlinks refuse with download.quarantine_symlink_refused.
	createR := createQuarantineFile(qDest)
	if !createR.OK {
		emitFetchFailed(url, createR)
		return createR
	}
	file := createR.Value.(*core.OSFile)

	// Wrap body at cap+1 so a server that lies about Content-Length
	// (or omits it entirely) still hits the cap. After copy we compare
	// the written byte count against the cap — equality on the +1
	// signals overflow.
	bounded := core.LimitReader(resp.Body, maxDownloadBytes+1)
	var src core.Reader = bounded
	if onProgress != nil {
		src = &countingReader{
			inner:    bounded,
			total:    resp.ContentLength,
			onChange: onProgress,
		}
	}
	copyR := core.Copy(file, src)
	if !copyR.OK {
		_ = file.Close()
		_ = core.Remove(qDest)
		failErr := core.E(fetchOp, "stream copy failed", copyR.Value.(error))
		emitFetchFailed(url, core.Fail(failErr))
		return core.Fail(failErr)
	}
	written := copyR.Value.(int64)
	if written > maxDownloadBytes {
		_ = file.Close()
		_ = core.Remove(qDest)
		failErr := core.E(fetchOp,
			core.Concat("download exceeded ",
				core.Sprintf("%d", maxDownloadBytes),
				" byte cap"),
			nil)
		emitFetchFailed(url, core.Fail(failErr))
		return core.Fail(failErr)
	}
	// Close before Verify + Rename so the file handle is released and
	// the rename(2) doesn't race with an open writer on Windows.
	if err := file.Close(); err != nil {
		_ = core.Remove(qDest)
		failErr := core.E(fetchOp, "quarantine close failed", err)
		emitFetchFailed(url, core.Fail(failErr))
		return core.Fail(failErr)
	}

	if sha256hex != "" {
		if r := verify(qDest, sha256hex); !r.OK {
			_ = core.Remove(qDest)
			emitFetchFailed(url, r)
			return r
		}
	}

	if r := core.Rename(qDest, finalDest); !r.OK {
		_ = core.Remove(qDest)
		emitFetchFailed(url, r)
		return r
	}
	emitFetchSucceeded(url, written)
	return core.Ok(finalDest)
}

// countingReader wraps a Reader, reporting cumulative bytes read to
// the Progress callback after every Read. total reflects the
// Content-Length the server sent (0 when unknown — chunked transfer).
type countingReader struct {
	inner    core.Reader
	written  int64
	total    int64
	onChange Progress
}

// Read is the io.Reader contract — counts bytes through, fires the
// Progress callback when n > 0.
func (cr *countingReader) Read(p []byte) (int, error) {
	n, err := cr.inner.Read(p)
	if n > 0 {
		cr.written += int64(n)
		cr.onChange(cr.written, cr.total)
	}
	return n, err
}
