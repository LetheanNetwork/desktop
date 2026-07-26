// SPDX-Licence-Identifier: EUPL-1.2

// Wails3 Service shape for the downloader package — wraps Fetch/
// FetchWithProgress so the model-browser GUI can trigger downloads
// and watch progress without blocking the WebView. Bound by
// application.NewService(downloader.NewWailsService(c)) in
// pkg/desktop/desktop.go; pkg/desktop wires SetEmitter after the
// Wails app exists so progress + done events reach the event bus.
//
// Usage from TS (after Wails generates the binding):
//
//	import { Download } from "@desktop/downloader/wailsservice";
//	const id = await Download(
//	    "https://huggingface.co/.../resolve/main/model.gguf",
//	    "gemma-4-e2b.gguf",
//	);
//	Events.On("downloader:progress", (e) => { ... });
//	Events.On("downloader:done",     (e) => { ... });

package downloader

import (
	core "dappco.re/go"
)

// progressThrottle caps event-bus emits to one per interval so a
// fast network read doesn't drown the WebView in updates. The
// frontend only needs ~4-10 progress repaints per second to feel
// live; finer-grain measurement is wasted work.
const progressThrottle = 250 * core.Millisecond

// CodeDigestRequired is the typed error code the Wails Download /
// DownloadVerified surfaces raise when the renderer omits the
// SHA-256 digest. Mirrors pkg/opencode's `upgrade.digest_required`
// pattern (Mantis #1621) — the renderer-reachable surface is
// fail-closed; HTTP / Go-internal callers may still pass "" to
// FetchVerified for the quarantine-only path. Cerberus #49 F-2.
//
// Stable for HTTP envelope matching + frontend toast routing.
const CodeDigestRequired = "downloader.digest_required"

// Emitter is the late-bound callback that ships events to the Wails
// event bus. pkg/desktop sets it via SetEmitter once the Wails app
// instance exists. nil = no emit (CLI/serve modes), Download still
// runs to completion but doesn't broadcast.
//
// Usage example (from pkg/desktop wiring):
//
//	dlSvc.SetEmitter(func(name string, data any) {
//	    s.app.Event.Emit(name, data)
//	})
type Emitter func(name string, data any)

// WailsService is the Wails-bindable surface for the downloader.
// Download(url, name) kicks off a tracked goroutine via c.Go so
// ServiceShutdown drains in-flight downloads cleanly; progress +
// done events go out via the Emitter installed by pkg/desktop.
type WailsService struct {
	core *core.Core
	emit Emitter
}

// NewWailsService constructs the bindable service. core is required
// so Download can spawn through c.Go (waitGroup-tracked goroutine);
// the emitter is installed later via SetEmitter.
//
// Usage example:
//
//	dlSvc := downloader.NewWailsService(c)
//	app.NewService(dlSvc)
//	dlSvc.SetEmitter(func(name string, data any) {
//	    app.Event.Emit(name, data)
//	})
func NewWailsService(c *core.Core) *WailsService {
	return &WailsService{core: c}
}

// SetEmitter installs the Wails event-bus callback. Safe to call
// before or after Download invocations — late callbacks pick up the
// next progress fire.
//
// Usage example:
//
//	dlSvc.SetEmitter(func(name string, data any) {
//	    s.app.Event.Emit(name, data)
//	})
//
//wails:ignore
func (s *WailsService) SetEmitter(emit Emitter) {
	if s == nil {
		return
	}
	s.emit = emit
}

// ServiceName returns the canonical service identifier for Wails3
// binding generation.
func (s *WailsService) ServiceName() string { return "Downloader" }

// ServiceStartup is the Wails3 lifecycle hook fired when the app
// starts servicing requests. Spawns a background sweep of the
// quarantine dir to remove orphan files from a previous interrupted
// download. The sweep runs via c.Go so the lifecycle hook returns
// immediately.
func (s *WailsService) ServiceStartup(_ core.Context, _ any) core.Result {
	if s != nil && s.core != nil {
		s.core.Go(func() { cleanStaleQuarantine(s.core) })
	}
	return core.Ok(nil)
}

// ServiceShutdown is the Wails3 lifecycle hook fired when the app
// shuts down. In-flight downloads are drained by Core's
// ServiceShutdown waitGroup (each Download spawns via c.Go);
// nothing to do here.
func (s *WailsService) ServiceShutdown() core.Result { return core.Ok(nil) }

// ResolveHFManifest resolves the per-file metadata
// (sha256 digest, byte size, canonical resolve URL) for a Hugging
// Face repo file. Exposed on the Wails surface so the model-browser
// frontend can hand the user a "Download" path that the F-2 fail-
// closed gate accepts: resolve → DownloadVerified(URL, name, digest).
//
// Two return shapes per Mantis #1686:
//
//   - LFS-stored file → HFManifest.Digest populated, Unverifiable=false.
//   - Non-LFS / small file → Digest="", Unverifiable=true. The frontend
//     decides whether to (a) refuse the download (strict) or (b) call
//     Download() in soft-mode (accept the unverifiable trade-off, the
//     audit log already records the choice).
//
// Returns the manifest as core.Result.Value so the Wails-generated TS
// binding surfaces it as a typed object. JSON-marshalled at the
// Wails boundary; HFManifest's exported fields render with their Go
// names (RepoID / FileName / ResolveURL / Digest / Size /
// Unverifiable).
//
// Usage example (TypeScript, post Wails binding regen):
//
//	const m = await ResolveHFManifest(repo, file);
//	if (m.Unverifiable) { warnSoftMode(m); }
//	await DownloadVerified(m.ResolveURL, m.FileName, m.Digest);
func (s *WailsService) ResolveHFManifest(repoID, fileName string) core.Result {
	return ResolveHFManifest(repoID, fileName)
}

// Download begins an async model fetch and returns immediately with
// the job id the frontend uses to correlate subsequent
// "downloader:progress" / "downloader:done" events.
//
// **DEPRECATED on the Wails surface — use DownloadVerified.** Per
// Cerberus #49 F-2 the renderer-reachable fetch must carry a SHA-256
// digest; Download() fires a "downloader:done" event with ok=false +
// error code `downloader.digest_required` and does NOT touch the
// network. The job id still returns so the frontend's correlation map
// stays consistent. Internal Go callers wanting the quarantine-only
// path (no verify) call FetchWithProgress directly.
//
// Event payloads:
//
//	"downloader:progress" → {id, name, written, total}
//	"downloader:done"     → {id, name, dest, ok, error?}
//
// Usage example (from TS — note: emits digest_required immediately):
//
//	const id = await DownloadVerified(url, "gemma-4-e2b.gguf", "<sha256hex>");
//	Events.On("downloader:progress", (e) => {
//	    if (e.data.id === id) updateBar(e.data.written / e.data.total);
//	});
//	Events.On("downloader:done", (e) => {
//	    if (e.data.id === id) e.data.ok ? showReady(e.data.dest) : showError(e.data.error);
//	});
func (s *WailsService) Download(url, name string) string {
	id := newJobID()
	if s == nil {
		return id
	}
	if s.core == nil {
		// Defensive — if no Core was injected, run synchronously so
		// the call doesn't silently drop. This path mostly hits
		// tests; production registration always passes Core.
		s.run(id, url, name)
		return id
	}
	s.core.Go(func() { s.run(id, url, name) })
	return id
}

// DownloadVerified is Download with an expected SHA-256 hex digest
// the downloaded file must match. Mismatch → quarantine file deleted,
// final path absent, "downloader:done" fires with ok=false carrying
// the digest-mismatch error message. sha256hex of "" is treated
// identically to Download (no verify, but quarantine + atomic-promote
// still apply for partial-write protection).
//
// Usage example (TypeScript):
//
//	const id = await DownloadVerified(url, "gemma.gguf", "<sha256hex>");
//	Events.On("downloader:done", (e) => {
//	    if (e.data.id === id && !e.data.ok) {
//	        showError(`Verify failed: ${e.data.error}`);
//	    }
//	});
func (s *WailsService) DownloadVerified(url, name, sha256hex string) string {
	id := newJobID()
	if s == nil {
		return id
	}
	if s.core == nil {
		s.runVerified(id, url, name, sha256hex)
		return id
	}
	s.core.Go(func() { s.runVerified(id, url, name, sha256hex) })
	return id
}

// run is the goroutine body for Download — invokes FetchWithProgress
// with a throttled Wails-event-emitting callback, broadcasts the
// terminal "done" event when the fetch returns. Equivalent to
// runVerified with an empty digest.
func (s *WailsService) run(id, url, name string) {
	s.runVerified(id, url, name, "")
}

// runVerified is the goroutine body for both Download (sha256hex="")
// and DownloadVerified — invokes FetchVerified with a throttled
// progress callback, broadcasts the terminal "done" event.
//
// Cerberus #49 F-2 — fail-closed on empty sha256hex at the Wails
// boundary. A renderer-reachable code path that fetches arbitrary
// bytes WITHOUT a content-addressed identity is an unbounded
// supply-chain surface; the marketplace + model card always carry
// a digest, so an empty value here is a misconfiguration or a
// bypass attempt. Internal Go callers (CLI, tests) may still call
// FetchVerified with "" for the quarantine-only path; this gate
// scopes to the renderer surface only.
func (s *WailsService) runVerified(id, url, name, sha256hex string) {
	if sha256hex == "" {
		s.fire("downloader:done", map[string]any{
			"id":   id,
			"name": name,
			"ok":   false,
			"error": core.E(CodeDigestRequired,
				"sha256 digest is required on the Wails download surface", nil).Error(),
		})
		return
	}
	var lastEmit core.Time
	onProgress := func(written, total int64) {
		now := core.Now()
		if !lastEmit.IsZero() && core.Since(lastEmit) < progressThrottle {
			return
		}
		lastEmit = now
		s.fire("downloader:progress", map[string]any{
			"id":      id,
			"name":    name,
			"written": written,
			"total":   total,
		})
	}
	r := FetchVerified(url, name, sha256hex, onProgress)

	payload := map[string]any{
		"id":   id,
		"name": name,
		"ok":   r.OK,
	}
	if r.OK {
		payload["dest"] = r.Value.(string)
	} else {
		payload["error"] = r.Error()
	}
	s.fire("downloader:done", payload)
}

// fire shields the callback site from a nil emitter — pre-bind
// downloads (CLI / serve / tests) silently drop events while still
// completing the download itself.
func (s *WailsService) fire(name string, data any) {
	if s == nil || s.emit == nil {
		return
	}
	s.emit(name, data)
}

// newJobID generates a "dl-" prefixed ID for grep-friendliness in
// logs + Wails event payloads. crypto-random suffix so concurrent
// Downloads don't collide.
func newJobID() string {
	r := core.RandomString(8)
	if !r.OK {
		return "dl-" + core.Sprintf("%d", core.UnixNow())
	}
	return "dl-" + r.Value.(string)
}
