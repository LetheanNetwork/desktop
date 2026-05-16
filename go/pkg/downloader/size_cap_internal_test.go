// SPDX-Licence-Identifier: EUPL-1.2

// Cerberus #1425 — internal-package test for the maxDownloadBytes
// cap. Internal (package `downloader`, not `downloader_test`) so the
// test can shrink the cap to a few hundred bytes; the external test
// pack would otherwise need a 256 GiB tempfile to exercise overflow.

package downloader

import (
	"net/http/httptest"

	core "dappco.re/go"

	"dappco.re/lthn/desktop/pkg/paths"
)

// homeForInternalTest mirrors the external-pack homeFixture helper.
// Duplicated here because internal _test.go files can't see helpers
// defined in the external pack.
func homeForInternalTest(t *core.T) string {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	return tmp
}

// withCap shrinks maxDownloadBytes for the lifetime of a single test
// and restores the production value via t.Cleanup. Restores even on
// test failure so cross-test contamination is impossible.
func withCap(t *core.T, n int64) {
	t.Helper()
	old := maxDownloadBytes
	maxDownloadBytes = n
	t.Cleanup(func() { maxDownloadBytes = old })
}

// TestDownloader_SizeCap_Bad_BodyExceedsCap — server omits Content-
// Length and streams more bytes than the cap. The LimitReader stops
// at cap+1 and the post-copy size check fails the fetch.
func TestDownloader_SizeCap_Bad_BodyExceedsCap(t *core.T) {
	homeForInternalTest(t)
	withCap(t, 64) // 64 bytes
	payload := make([]byte, 128)
	for i := range payload {
		payload[i] = 'A'
	}
	srv := httptest.NewServer(core.HandlerFunc(func(w core.ResponseWriter, _ *core.Request) {
		// Don't set Content-Length so the pre-check is skipped and the
		// LimitReader path is the gate.
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	r := Fetch(srv.URL, "oversized.gguf")
	core.AssertFalse(t, r.OK, "body that exceeds cap must Fail")

	// And the quarantine file must NOT have been promoted to the
	// model dir — verify the model path is absent.
	dir := paths.ModelsDir().Value.(string)
	stat := core.Stat(core.PathJoin(dir, "oversized.gguf"))
	core.AssertFalse(t, stat.OK, "rejected file must not appear in model dir")
}

// TestDownloader_SizeCap_Bad_ContentLengthExceedsCap — server sends an
// honest Content-Length that's larger than the cap. The pre-check
// fails fast, so the body is never opened (no quarantine file).
func TestDownloader_SizeCap_Bad_ContentLengthExceedsCap(t *core.T) {
	homeForInternalTest(t)
	withCap(t, 64)
	payload := make([]byte, 256)
	srv := httptest.NewServer(core.HandlerFunc(func(w core.ResponseWriter, _ *core.Request) {
		w.Header().Set("Content-Length", core.Sprintf("%d", len(payload)))
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	r := Fetch(srv.URL, "advertised-big.gguf")
	core.AssertFalse(t, r.OK,
		"Content-Length above cap must Fail before any body read")
}

// TestDownloader_SizeCap_Good_BodyExactlyAtCap — server streams
// exactly cap bytes. The +1 byte of LimitReader headroom means we
// successfully read cap bytes and the post-copy check passes (written
// == cap, not > cap).
func TestDownloader_SizeCap_Good_BodyExactlyAtCap(t *core.T) {
	home := homeForInternalTest(t)
	withCap(t, 64)
	payload := make([]byte, 64)
	for i := range payload {
		payload[i] = 'B'
	}
	srv := httptest.NewServer(core.HandlerFunc(func(w core.ResponseWriter, _ *core.Request) {
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	r := Fetch(srv.URL, "atcap.gguf")
	core.AssertTrue(t, r.OK, "body exactly at cap must succeed")
	got := core.ReadFile(core.PathJoin(home, "Lethean", "conf", "models", "atcap.gguf"))
	core.AssertTrue(t, got.OK)
	core.AssertEqual(t, 64, len(got.Value.([]byte)))
}
