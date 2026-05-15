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
// starts servicing requests. No setup needed — Download spawns its
// own goroutine on demand.
func (s *WailsService) ServiceStartup(_ core.Context, _ any) core.Result {
	return core.Ok(nil)
}

// ServiceShutdown is the Wails3 lifecycle hook fired when the app
// shuts down. In-flight downloads are drained by Core's
// ServiceShutdown waitGroup (each Download spawns via c.Go);
// nothing to do here.
func (s *WailsService) ServiceShutdown() core.Result { return core.Ok(nil) }

// Download begins an async model fetch and returns immediately with
// the job id the frontend uses to correlate subsequent
// "downloader:progress" / "downloader:done" events.
//
// Event payloads:
//
//	"downloader:progress" → {id, name, written, total}
//	"downloader:done"     → {id, name, dest, ok, error?}
//
// Usage example (from TS):
//
//	const id = await Download(url, "gemma-4-e2b.gguf");
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

// run is the goroutine body — invokes FetchWithProgress with a
// throttled Wails-event-emitting callback, broadcasts the terminal
// "done" event when the fetch returns.
func (s *WailsService) run(id, url, name string) {
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
	r := FetchWithProgress(url, name, onProgress)

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
