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
