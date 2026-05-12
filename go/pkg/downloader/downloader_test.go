// SPDX-Licence-Identifier: EUPL-1.2

// Tests for the model downloader. httptest.Server gives us a real HTTP
// roundtrip; HOME override sandboxes ~/Lethean/conf/models/ so files
// land in a tempdir that the t.Cleanup will scrub.

package downloader_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"

	core "dappco.re/go"

	"dappco.re/lthn/desktop/pkg/downloader"
)

// homeFixture sandboxes $HOME into a t.TempDir so the downloader's
// paths.ModelsDir() resolves under a disposable tree.
func homeFixture(t *core.T) string {
	t.Helper()
	tmp := t.TempDir()
	prev, hadPrev := os.LookupEnv("HOME")
	core.AssertNoError(t, os.Setenv("HOME", tmp))
	t.Cleanup(func() {
		if hadPrev {
			_ = os.Setenv("HOME", prev)
		} else {
			_ = os.Unsetenv("HOME")
		}
	})
	return tmp
}

func TestDownloader_Fetch_Good(t *core.T) {
	home := homeFixture(t)
	payload := []byte("MOCK-GGUF-BYTES")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	r := downloader.Fetch(srv.URL, "model.gguf")
	core.AssertTrue(t, r.OK)
	dest := r.Value.(string)
	core.AssertEqual(t, filepath.Join(home, "Lethean", "conf", "models", "model.gguf"), dest)

	got, err := os.ReadFile(dest)
	core.AssertNoError(t, err)
	core.AssertEqual(t, string(payload), string(got))
}

func TestDownloader_Fetch_Good_Overwrite(t *core.T) {
	home := homeFixture(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("v2"))
	}))
	defer srv.Close()

	dest := filepath.Join(home, "Lethean", "conf", "models", "m.bin")
	core.AssertNoError(t, os.MkdirAll(filepath.Dir(dest), 0o755))
	core.AssertNoError(t, os.WriteFile(dest, []byte("v1-existing"), 0o644))

	r := downloader.Fetch(srv.URL, "m.bin")
	core.AssertTrue(t, r.OK)

	got, _ := os.ReadFile(dest)
	core.AssertEqual(t, "v2", string(got), "Fetch should overwrite existing file")
}

func TestDownloader_Fetch_Bad_EmptyURL(t *core.T) {
	homeFixture(t)
	r := downloader.Fetch("", "model.gguf")
	core.AssertFalse(t, r.OK)
}

func TestDownloader_Fetch_Bad_EmptyName(t *core.T) {
	homeFixture(t)
	r := downloader.Fetch("https://example.com/model.gguf", "")
	core.AssertFalse(t, r.OK)
}

func TestDownloader_Fetch_Bad_HTTPError(t *core.T) {
	homeFixture(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, "not found")
	}))
	defer srv.Close()

	r := downloader.Fetch(srv.URL, "missing.gguf")
	core.AssertFalse(t, r.OK, "404 should propagate as Fail")
}

func TestDownloader_Fetch_Bad_HomeUnusable(t *core.T) {
	tmp := t.TempDir()
	blocker := filepath.Join(tmp, "blocker")
	core.AssertNoError(t, os.WriteFile(blocker, []byte("x"), 0o644))
	prev, hadPrev := os.LookupEnv("HOME")
	core.AssertNoError(t, os.Setenv("HOME", blocker))
	t.Cleanup(func() {
		if hadPrev {
			_ = os.Setenv("HOME", prev)
		} else {
			_ = os.Unsetenv("HOME")
		}
	})

	r := downloader.Fetch("https://example.com/m.gguf", "m.gguf")
	core.AssertFalse(t, r.OK, "ModelsDir() failure must propagate")
}
