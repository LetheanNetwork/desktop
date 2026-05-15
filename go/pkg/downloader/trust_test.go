// SPDX-Licence-Identifier: EUPL-1.2

// Trust-layer tests for the downloader. Covers AllowedSource,
// Verify, the quarantine + atomic-promote flow, and the stale
// quarantine sweep. Aligned with the test shapes in
// plans/code/lthn/desktop/downloader/RFC.md §7.2.

package downloader_test

import (
	"net/http/httptest"
	"os"

	core "dappco.re/go"

	"dappco.re/lthn/desktop/pkg/downloader"
)

func TestTrust_AllowedSource_Good(t *core.T) {
	core.AssertTrue(t, downloader.AllowedSource("https://huggingface.co/snider/model/resolve/main/model.gguf"))
}

func TestTrust_AllowedSource_Good_CDN(t *core.T) {
	core.AssertTrue(t, downloader.AllowedSource("https://cdn-lfs-us-1.huggingface.co/foo/bar/baz.gguf"))
	core.AssertTrue(t, downloader.AllowedSource("https://hf.co/short/link"))
}

func TestTrust_AllowedSource_Bad_ArbitraryHost(t *core.T) {
	core.AssertFalse(t, downloader.AllowedSource("https://example.com/model.gguf"))
	// Hostname suffix-trick (a malicious host that ends in our suffix) — must
	// fail because the suffix match requires either exact match or "."+suffix.
	core.AssertFalse(t, downloader.AllowedSource("https://evilhuggingface.co/model.gguf"))
}

func TestTrust_AllowedSource_Bad_EmptyURL(t *core.T) {
	core.AssertFalse(t, downloader.AllowedSource(""))
	core.AssertFalse(t, downloader.AllowedSource("not-a-url"))
	core.AssertFalse(t, downloader.AllowedSource("https://"))
}

func TestTrust_Verify_Good(t *core.T) {
	home := homeFixture(t)
	dest := core.PathJoin(home, "Lethean", "conf", "models", "tiny.bin")
	core.AssertTrue(t, core.MkdirAll(core.PathDir(dest), 0o755).OK)
	payload := []byte("MOCK-PAYLOAD")
	core.AssertTrue(t, core.WriteFile(dest, payload, 0o644).OK)

	expected := core.SHA256HexString("MOCK-PAYLOAD")
	r := downloader.Verify(dest, expected)
	core.AssertTrue(t, r.OK)
}

func TestTrust_Verify_Bad_Mismatch(t *core.T) {
	home := homeFixture(t)
	dest := core.PathJoin(home, "Lethean", "conf", "models", "tiny.bin")
	core.AssertTrue(t, core.MkdirAll(core.PathDir(dest), 0o755).OK)
	core.AssertTrue(t, core.WriteFile(dest, []byte("ACTUAL"), 0o644).OK)

	r := downloader.Verify(dest, "0000000000000000000000000000000000000000000000000000000000000000")
	core.AssertFalse(t, r.OK)
	// Error message includes both the expected and computed digests
	// for log surfacing.
	msg := r.Error()
	core.AssertTrue(t, core.Contains(msg, "expected"))
	core.AssertTrue(t, core.Contains(msg, "got"))
}

func TestTrust_Verify_Bad_MissingFile(t *core.T) {
	homeFixture(t)
	r := downloader.Verify("/nonexistent/path/nope.bin",
		"0000000000000000000000000000000000000000000000000000000000000000")
	core.AssertFalse(t, r.OK)
}

func TestTrust_Fetch_Bad_DisallowedSource(t *core.T) {
	homeFixture(t)
	// example.com is not in the allow-list; Fetch must reject before
	// any HTTP read is attempted.
	r := downloader.Fetch("https://example.com/model.gguf", modelGGUF)
	core.AssertFalse(t, r.OK)
	core.AssertTrue(t, core.Contains(r.Error(), "source not allowed"))
}

func TestTrust_DownloadVerified_Good(t *core.T) {
	home := homeFixture(t)
	payload := []byte("KNOWN-GOOD-BYTES")
	srv := httptest.NewServer(core.HandlerFunc(func(w core.ResponseWriter, _ *core.Request) {
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	expected := core.SHA256HexString(string(payload))
	r := downloader.FetchVerified(srv.URL, modelGGUF, expected, nil)
	core.AssertTrue(t, r.OK)

	dest := r.Value.(string)
	core.AssertEqual(t, core.PathJoin(home, "Lethean", "conf", "models", modelGGUF), dest)

	read := core.ReadFile(dest)
	core.AssertTrue(t, read.OK)
	core.AssertEqual(t, string(payload), string(read.Value.([]byte)))
}

func TestTrust_DownloadVerified_Bad_DigestMismatch(t *core.T) {
	home := homeFixture(t)
	payload := []byte("BYTES-THAT-WONT-MATCH")
	srv := httptest.NewServer(core.HandlerFunc(func(w core.ResponseWriter, _ *core.Request) {
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	bogus := "0000000000000000000000000000000000000000000000000000000000000000"
	r := downloader.FetchVerified(srv.URL, modelGGUF, bogus, nil)
	core.AssertFalse(t, r.OK)

	// Quarantine file deleted; final path absent.
	finalDest := core.PathJoin(home, "Lethean", "conf", "models", modelGGUF)
	core.AssertFalse(t, core.Stat(finalDest).OK)
	qDest := core.PathJoin(home, "Lethean", "conf", "models", ".quarantine", modelGGUF)
	core.AssertFalse(t, core.Stat(qDest).OK)
}

func TestTrust_QuarantineClean_Ugly(t *core.T) {
	home := homeFixture(t)
	qd := core.PathJoin(home, "Lethean", "conf", "models", ".quarantine")
	core.AssertTrue(t, core.MkdirAll(qd, 0o755).OK)

	// Stale file: write + back-date mtime to 48h ago.
	stale := core.PathJoin(qd, "stale.bin")
	core.AssertTrue(t, core.WriteFile(stale, []byte("old"), 0o644).OK)
	staleTime := core.Now().Add(-48 * core.Hour)
	// os.Chtimes — tests are allowed stdlib I/O (matches the net/http/
	// httptest precedent already in this package). No CoreGO wrapper
	// exists yet for setting file mtime; logged in athena's memory as
	// a CoreGO export gap for later upstream.
	core.AssertTrue(t, os.Chtimes(stale, staleTime, staleTime) == nil)

	// Fresh file: just written, mtime = now.
	fresh := core.PathJoin(qd, "fresh.bin")
	core.AssertTrue(t, core.WriteFile(fresh, []byte("new"), 0o644).OK)

	// Trigger the sweep via the WailsService startup path.
	c := core.New()
	core.AssertTrue(t, c.ServiceStartup(core.Background(), nil).OK)
	defer func() { _ = c.ServiceShutdown(core.Background()) }()
	svc := downloader.NewWailsService(c)
	core.AssertTrue(t, svc.ServiceStartup(core.Background(), nil).OK)
	// Sweep runs via c.Go — give it a tick to settle.
	core.Sleep(50 * core.Millisecond)

	// Stale gone; fresh untouched.
	core.AssertFalse(t, core.Stat(stale).OK, "stale file should be removed")
	core.AssertTrue(t, core.Stat(fresh).OK, "fresh file should survive")
}
