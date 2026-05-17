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

// TestDownloader_NameTraversal_Rejects_Bad — Cerberus #49 F-1.
// Renderer-supplied names containing traversal tokens, leading dots,
// path separators, or NUL bytes must be rejected BEFORE any
// core.PathJoin so a malicious model name can't pivot writes outside
// ~/Lethean/conf/models/. Mirrors the IsValidID contract used across
// the 20+ wails surfaces hardened by Cerberus #1486.
func TestDownloader_NameTraversal_Rejects_Bad(t *core.T) {
	homeFixture(t)
	srv := httptest.NewServer(core.HandlerFunc(func(w core.ResponseWriter, _ *core.Request) {
		_, _ = w.Write([]byte("payload"))
	}))
	defer srv.Close()

	cases := []struct {
		label, name string
	}{
		{"parent traversal", "../escape.gguf"},
		{"deep traversal", "../../wallets/server.key"},
		{"absolute path", "/etc/passwd"},
		{"leading dot", ".hidden.gguf"},
		{"backslash", "evil\\path.gguf"},
		{"NUL byte", "model\x00.gguf"},
		{"bare ..", ".."},
	}
	for _, tc := range cases {
		r := downloader.Fetch(srv.URL, tc.name)
		core.AssertFalse(t, r.OK,
			"name "+tc.label+" ("+tc.name+") must be rejected")
		core.AssertTrue(t,
			core.Contains(r.Error(), "paths.invalid_id") ||
				core.Contains(r.Error(), "paths.escape"),
			"error must carry paths.invalid_id or paths.escape; got: "+r.Error())
	}
}

// TestDownloader_WailsEmptyDigest_Rejects_Bad — Cerberus #49 F-2.
// Wails.Download(url, name) without a SHA-256 digest must fire
// "downloader:done" with ok=false + error code
// `downloader.digest_required` and MUST NOT touch the network.
// Mirrors the opencode Upgrade pattern (Mantis #1621).
func TestDownloader_WailsEmptyDigest_Rejects_Bad(t *core.T) {
	homeFixture(t)
	// Counter on the test server so we can assert NO request fired —
	// the digest gate must reject pre-network.
	var hits core.AtomicInt32
	srv := httptest.NewServer(core.HandlerFunc(func(w core.ResponseWriter, _ *core.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte("should-never-arrive"))
	}))
	defer srv.Close()

	// Capture the "downloader:done" event via SetEmitter — same
	// pattern as the trust quarantine sweep test.
	c := core.New()
	core.AssertTrue(t, c.ServiceStartup(core.Background(), nil).OK)
	defer func() { _ = c.ServiceShutdown(core.Background()) }()
	svc := downloader.NewWailsService(c)

	var doneMu core.Mutex
	var done map[string]any
	svc.SetEmitter(func(name string, data any) {
		if name != "downloader:done" {
			return
		}
		doneMu.Lock()
		defer doneMu.Unlock()
		done, _ = data.(map[string]any)
	})

	// Empty-digest Download — must fire done(ok=false, digest_required).
	id := svc.Download(srv.URL, modelGGUF)
	core.AssertTrue(t, id != "", "Download must still return a job id")

	// Spawned via c.Go — give it a tick.
	core.Sleep(50 * core.Millisecond)

	doneMu.Lock()
	got := done
	doneMu.Unlock()
	core.AssertTrue(t, got != nil, "downloader:done must fire")
	core.AssertEqual(t, false, got["ok"], "ok must be false on empty digest")
	errStr, _ := got["error"].(string)
	core.AssertTrue(t,
		core.Contains(errStr, "downloader.digest_required"),
		"error must carry downloader.digest_required; got: "+errStr)

	// Network MUST not have been touched.
	core.AssertEqual(t, int32(0), hits.Load(),
		"no HTTP request must fire when the digest gate rejects")

	// DownloadVerified with explicit empty digest — same gate.
	done = nil
	id2 := svc.DownloadVerified(srv.URL, modelGGUF, "")
	core.AssertTrue(t, id2 != "")
	core.Sleep(50 * core.Millisecond)
	doneMu.Lock()
	got2 := done
	doneMu.Unlock()
	core.AssertTrue(t, got2 != nil, "downloader:done must fire for DownloadVerified('') too")
	core.AssertEqual(t, false, got2["ok"])
	errStr2, _ := got2["error"].(string)
	core.AssertTrue(t,
		core.Contains(errStr2, "downloader.digest_required"),
		"DownloadVerified('') must also carry downloader.digest_required; got: "+errStr2)
	core.AssertEqual(t, int32(0), hits.Load(),
		"DownloadVerified('') must also stay pre-network")
}

// TestDownloader_ConcurrentSameName_NoRaceBypass_Ugly — Cerberus #49
// F-3. Two parallel FetchVerified calls targeting the same name must
// not interleave Create / Copy / Verify / Rename in a way that lets
// bytes B promote under digest A. The per-name mutex serialises them
// so each call sees a consistent quarantine view; one call wins and
// the other writes against the now-existing final file.
//
// Pattern: spin two goroutines that fetch the SAME name from servers
// returning DIFFERENT bodies; assert the final file content matches
// exactly one of the two payloads (whichever won the lock). Run
// repeatedly under -race to flush any remaining write-write hazard.
func TestDownloader_ConcurrentSameName_NoRaceBypass_Ugly(t *core.T) {
	home := homeFixture(t)
	payloadA := []byte("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")
	payloadB := []byte("BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB")
	srvA := httptest.NewServer(core.HandlerFunc(func(w core.ResponseWriter, _ *core.Request) {
		_, _ = w.Write(payloadA)
	}))
	defer srvA.Close()
	srvB := httptest.NewServer(core.HandlerFunc(func(w core.ResponseWriter, _ *core.Request) {
		_, _ = w.Write(payloadB)
	}))
	defer srvB.Close()

	name := "race.gguf"
	done := make(chan core.Result, 2)
	go func() { done <- downloader.Fetch(srvA.URL, name) }()
	go func() { done <- downloader.Fetch(srvB.URL, name) }()
	r1 := <-done
	r2 := <-done

	// Both calls return Ok (each completes the full pipeline; the
	// second overwrites the first's atomic-promoted file).
	core.AssertTrue(t, r1.OK)
	core.AssertTrue(t, r2.OK)

	// Final on-disk content is EXACTLY one of the two payloads —
	// never a torn mix. The mutex ensures Copy → Rename runs atomically
	// per-name; without it a partial-write of B could overlap A's
	// Rename and surface mixed bytes.
	dest := core.PathJoin(home, "Lethean", "conf", "models", name)
	got := core.ReadFile(dest)
	core.AssertTrue(t, got.OK)
	final := string(got.Value.([]byte))
	core.AssertTrue(t,
		final == string(payloadA) || final == string(payloadB),
		"final content must be exactly payloadA or payloadB, got: "+final)
}
