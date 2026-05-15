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
// overhead (identical to Fetch).
//
// Usage example:
//
//	r := downloader.FetchWithProgress(url, name, func(w, t int64) {
//	    emit("dl:progress", map[string]any{"written": w, "total": t})
//	})
//	if r.OK { dest := r.Value.(string); _ = dest }
func FetchWithProgress(url, name string, onProgress Progress) core.Result {
	if url == "" {
		return core.Fail(core.E(fetchOp, "url is required", nil))
	}
	if name == "" {
		return core.Fail(core.E(fetchOp, "name is required", nil))
	}
	dirR := paths.ModelsDir()
	if !dirR.OK {
		return dirR
	}
	dest := core.PathJoin(dirR.Value.(string), name)

	getR := core.HTTPGet(url)
	if !getR.OK {
		return getR
	}
	resp := getR.Value.(*core.Response)
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return core.Fail(core.E(fetchOp,
			core.Concat("HTTP ", core.Sprintf("%d", resp.StatusCode), " from ", url),
			nil))
	}

	createR := core.Create(dest)
	if !createR.OK {
		return createR
	}
	file := createR.Value.(*core.OSFile)
	defer file.Close()

	var src core.Reader = resp.Body
	if onProgress != nil {
		src = &countingReader{
			inner:    resp.Body,
			total:    resp.ContentLength,
			onChange: onProgress,
		}
	}
	if r := core.Copy(file, src); !r.OK {
		return core.Fail(core.E(fetchOp, "stream copy failed", r.Value.(error)))
	}
	return core.Ok(dest)
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
