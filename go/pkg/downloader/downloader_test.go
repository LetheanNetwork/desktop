// SPDX-Licence-Identifier: EUPL-1.2

// Tests for the model downloader. httptest.Server gives us a real HTTP
// roundtrip; HOME override sandboxes ~/Lethean/conf/models/ so files
// land in a tempdir that the t.Cleanup will scrub.

package downloader_test

import (
	"net/http/httptest"

	core "dappco.re/go"

	"dappco.re/lthn/desktop/pkg/downloader"
)

const modelGGUF = "model.gguf"

// homeFixture sandboxes $HOME into a t.TempDir so the downloader's
// paths.ModelsDir() resolves under a disposable tree.
func homeFixture(t *core.T) string {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	return tmp
}

func TestDownloader_Fetch_Good(t *core.T) {
	home := homeFixture(t)
	payload := []byte("MOCK-GGUF-BYTES")
	srv := httptest.NewServer(core.HandlerFunc(func(w core.ResponseWriter, _ *core.Request) {
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	r := downloader.Fetch(srv.URL, modelGGUF)
	core.AssertTrue(t, r.OK)
	dest := r.Value.(string)
	core.AssertEqual(t, core.PathJoin(home, "Lethean", "conf", "models", modelGGUF), dest)

	read := core.ReadFile(dest)
	core.AssertTrue(t, read.OK)
	got := read.Value.([]byte)
	core.AssertEqual(t, string(payload), string(got))
}

func TestDownloader_Fetch_Good_Overwrite(t *core.T) {
	home := homeFixture(t)
	srv := httptest.NewServer(core.HandlerFunc(func(w core.ResponseWriter, _ *core.Request) {
		_, _ = w.Write([]byte("v2"))
	}))
	defer srv.Close()

	dest := core.PathJoin(home, "Lethean", "conf", "models", "m.bin")
	core.AssertTrue(t, core.MkdirAll(core.PathDir(dest), 0o755).OK)
	core.AssertTrue(t, core.WriteFile(dest, []byte("v1-existing"), 0o644).OK)

	r := downloader.Fetch(srv.URL, "m.bin")
	core.AssertTrue(t, r.OK)

	got := core.ReadFile(dest).Value.([]byte)
	core.AssertEqual(t, "v2", string(got), "Fetch should overwrite existing file")
}

func TestDownloader_Fetch_Bad_EmptyURL(t *core.T) {
	homeFixture(t)
	r := downloader.Fetch("", modelGGUF)
	core.AssertFalse(t, r.OK)
}

func TestDownloader_Fetch_Bad_EmptyName(t *core.T) {
	homeFixture(t)
	r := downloader.Fetch("https://example.com/model.gguf", "")
	core.AssertFalse(t, r.OK)
}

func TestDownloader_Fetch_Bad_HTTPError(t *core.T) {
	homeFixture(t)
	srv := httptest.NewServer(core.HandlerFunc(func(w core.ResponseWriter, _ *core.Request) {
		w.WriteHeader(core.StatusNotFound)
		_ = core.WriteString(w, "not found")
	}))
	defer srv.Close()

	r := downloader.Fetch(srv.URL, "missing.gguf")
	core.AssertFalse(t, r.OK, "404 should propagate as Fail")
}

func TestDownloader_Fetch_Bad_HomeUnusable(t *core.T) {
	tmp := t.TempDir()
	blocker := core.PathJoin(tmp, "blocker")
	core.AssertTrue(t, core.WriteFile(blocker, []byte("x"), 0o644).OK)
	t.Setenv("HOME", blocker)

	r := downloader.Fetch("https://example.com/m.gguf", "m.gguf")
	core.AssertFalse(t, r.OK, "ModelsDir() failure must propagate")
}

// TestDownloader_FetchWithProgress_Good — callback receives strictly
// increasing written counts that sum to the served payload length, and
// the server's Content-Length surfaces as the total argument.
func TestDownloader_FetchWithProgress_Good(t *core.T) {
	homeFixture(t)
	payload := []byte("ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")
	srv := httptest.NewServer(core.HandlerFunc(func(w core.ResponseWriter, _ *core.Request) {
		w.Header().Set("Content-Length", core.Sprintf("%d", len(payload)))
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	var calls int
	var lastWritten, lastTotal int64
	var monotonic = true
	r := downloader.FetchWithProgress(srv.URL, "progress.bin",
		func(written, total int64) {
			calls++
			if written < lastWritten {
				monotonic = false
			}
			lastWritten = written
			lastTotal = total
		})
	core.AssertTrue(t, r.OK)
	core.AssertGreaterOrEqual(t, calls, 1)
	core.AssertTrue(t, monotonic, "progress should be monotonically non-decreasing")
	core.AssertEqual(t, int64(len(payload)), lastWritten)
	core.AssertEqual(t, int64(len(payload)), lastTotal)
}

// TestDownloader_FetchWithProgress_Bad_HTTPError — failed responses do
// NOT fire the progress callback (we never read body bytes), the
// terminal Result still propagates the Fail.
func TestDownloader_FetchWithProgress_Bad_HTTPError(t *core.T) {
	homeFixture(t)
	srv := httptest.NewServer(core.HandlerFunc(func(w core.ResponseWriter, _ *core.Request) {
		w.WriteHeader(core.StatusInternalServerError)
		_ = core.WriteString(w, "boom")
	}))
	defer srv.Close()

	var fired bool
	r := downloader.FetchWithProgress(srv.URL, "fail.bin",
		func(int64, int64) { fired = true })
	core.AssertFalse(t, r.OK, "5xx should propagate as Fail")
	core.AssertFalse(t, fired, "no progress callback for failed transfer")
}

// TestDownloader_FetchWithProgress_Ugly — multi-chunk delivery via
// explicit Flush calls. The callback fires per chunk with
// monotonically increasing written counts; the file lands intact on
// disk. The exact total value depends on whether httptest auto-emits
// a Content-Length header — we don't assert on it.
func TestDownloader_FetchWithProgress_Ugly(t *core.T) {
	home := homeFixture(t)
	chunks := [][]byte{
		[]byte("part-1-"), []byte("part-2-"), []byte("part-3"),
	}
	srv := httptest.NewServer(core.HandlerFunc(func(w core.ResponseWriter, _ *core.Request) {
		flusher, _ := w.(core.Flusher)
		for _, c := range chunks {
			_, _ = w.Write(c)
			if flusher != nil {
				flusher.Flush()
			}
		}
	}))
	defer srv.Close()

	var writes []int64
	r := downloader.FetchWithProgress(srv.URL, "chunked.bin",
		func(written, _ int64) { writes = append(writes, written) })
	core.AssertTrue(t, r.OK)
	core.AssertGreaterOrEqual(t, len(writes), 1)
	for i := 1; i < len(writes); i++ {
		core.AssertGreaterOrEqual(t, writes[i], writes[i-1])
	}

	dest := core.PathJoin(home, "Lethean", "conf", "models", "chunked.bin")
	got := core.ReadFile(dest).Value.([]byte)
	core.AssertEqual(t, "part-1-part-2-part-3", string(got))
}
