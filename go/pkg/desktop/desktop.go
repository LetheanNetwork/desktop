// SPDX-Licence-Identifier: EUPL-1.2

// Package desktop is the Wails v3 wrapper for the lthn GUI mode.
// Constructs the Application with:
//
//   - Assets.Handler = the same gin engine pkg/server exposes for
//     `lthn serve`. The WebView reaches /v1/chat/completions etc.
//     same-origin — no CORS, no port hunting.
//   - Assets.Middleware = the Gin middleware pattern from
//     wails/v3/examples/gin-routing: delegate /wails/* back to
//     Wails internals, hand everything else to Gin.
//   - A NoRoute fallback on the Gin engine that serves the embedded
//     Vite frontend dist so the SPA loads at "/".
//   - Mac.ActivationPolicy = Accessory (menu-bar app, no Dock icon).
//   - ApplicationShouldTerminateAfterLastWindowClosed = false
//     (the tray IS the process — closing the popover hides it).
//   - SystemTray + popover Window attached via WindowOffset(5).
//
// The window URL is "/?surface=tray" — the index.html in the
// embedded dist reads ?surface= and mounts the matching Lit element.
//
// Usage example:
//
//	d := desktop.NewService(desktop.Options{
//	    Frontend: frontendFS,
//	    Server:   server.NewService(server.Options{Runner: r}),
//	})
//	if rr := d.Run(); !rr.OK { return rr }
package desktop

import (
	"io/fs"
	"net/http"
	"runtime"
	"strings"

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/runner"
	"dappco.re/lthn/desktop/pkg/server"
	"github.com/gin-gonic/gin"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"github.com/wailsapp/wails/v3/pkg/icons"
)

// Options configures the desktop service.
type Options struct {
	// Name is the macOS menu-bar accessibility label. Default: "lthn".
	Name string
	// Description is the app description shown by the OS.
	Description string
	// Frontend is the embedded Vite build (cmd/lthn/embed.go). The
	// service serves it via a NoRoute fallback on the gin engine.
	// The fs.FS root should contain index.html at its top level.
	Frontend fs.FS
	// FrontendRoot is the subdirectory within Frontend that holds
	// index.html + assets/. Defaults to "dist" — matches the
	// embed.FS shape `//go:embed all:dist`.
	FrontendRoot string
	// Server is the pkg/server.Service whose gin engine the WebView
	// talks to. Required.
	Server *server.Service
	// Core is the constructed *core.Core that the binding adapters
	// dispatch actions against (config / store / stream / process /
	// i18n / io are registered on it). Required.
	Core *core.Core
	// Runner is the talk-surface used by the RunnerService binding.
	// Required.
	Runner *runner.Service
	// TrayIcon is the light-mode systray icon bytes. Empty = use the
	// Wails default macOS template glyph.
	TrayIcon []byte
}

// Service holds the Wails application and the SystemTray anchor.
type Service struct {
	opts Options
	app  *application.App
}

// NewService constructs the desktop service. Does NOT start Wails
// yet — call Run() to block on the event loop.
//
// Usage example:
//
//	d := desktop.NewService(desktop.Options{Server: s, Frontend: fs})
//	d.Run()
func NewService(opts Options) *Service {
	if opts.Name == "" {
		opts.Name = "lthn"
	}
	if opts.Description == "" {
		opts.Description = "Lethean Desktop"
	}
	if opts.FrontendRoot == "" {
		opts.FrontendRoot = "dist"
	}
	return &Service{opts: opts}
}

// Run launches the Wails event loop. Blocks until the user picks
// "Quit lthn" from the systray menu (or the OS service mgr stops
// the process).
//
// Usage example:
//
//	if r := desktop.NewService(desktop.Options{...}).Run(); !r.OK {
//	    return r
//	}
func (s *Service) Run() core.Result {
	if s.opts.Server == nil {
		return core.Fail(core.E("desktop.Run", "Server is required", nil))
	}

	// Mount the SPA fallback on the gin engine — every request that
	// doesn't match a registered API route falls through to the
	// embedded dist.
	if err := s.attachSPA(); err != nil {
		return core.Fail(err)
	}

	engine := s.opts.Server.Handler()

	// Binding adapters exposed to the WebView via Wails TS bindings.
	// Each adapter wraps one lthn domain package, translating
	// core.Result → (T, error) for clean TS typing.
	wailsServices := []application.Service{
		application.NewService(NewRunnerService(s.opts.Runner)),
		application.NewService(NewSessionsService(s.opts.Core)),
		application.NewService(NewConfigService(s.opts.Core)),
		application.NewService(NewModelsService()),
		application.NewService(NewFirstLaunchService()),
		application.NewService(NewValidatorService()),
		application.NewService(NewTelemetryService()),
		application.NewService(NewLifecycleService()),
	}

	s.app = application.New(application.Options{
		Name:        s.opts.Name,
		Description: s.opts.Description,
		Services:    wailsServices,
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: false,
			ActivationPolicy:                                application.ActivationPolicyAccessory,
		},
		Assets: application.AssetOptions{
			Handler:    engine,
			Middleware: ginMiddleware(engine),
		},
	})

	systray := s.app.SystemTray.New()
	if runtime.GOOS == "darwin" {
		if s.opts.TrayIcon != nil {
			systray.SetTemplateIcon(s.opts.TrayIcon)
		} else {
			systray.SetTemplateIcon(icons.SystrayMacTemplate)
		}
	} else if s.opts.TrayIcon != nil {
		systray.SetIcon(s.opts.TrayIcon)
	}

	menu := s.app.Menu.New()
	menu.Add("Lethean Desktop").SetEnabled(false)
	menu.AddSeparator()
	menu.Add("Quit lthn").OnClick(func(_ *application.Context) {
		s.app.Quit()
	})
	systray.SetMenu(menu)

	window := s.app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:            "tray",
		Title:           "Lethean Desktop",
		Width:           400,
		Height:          560,
		Frameless:       true,
		AlwaysOnTop:     true,
		Hidden:          true,
		DisableResize:   true,
		HideOnEscape:    true,
		HideOnFocusLost: true,
		URL:             "/?surface=tray",
		Windows: application.WindowsWindow{
			HiddenOnTaskbar: true,
		},
	})

	// Close-hides rather than destroys — tray-rooted lifecycle.
	window.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		window.Hide()
		e.Cancel()
	})

	systray.AttachWindow(window).WindowOffset(5)

	if err := s.app.Run(); err != nil {
		return core.Fail(err)
	}
	return core.Ok(nil)
}

// attachSPA mounts the embedded frontend on the gin engine's
// NoRoute handler. Anything that doesn't match an API route gets
// served from the embedded dist — index.html, assets/*, etc.
func (s *Service) attachSPA() error {
	sub, err := fs.Sub(s.opts.Frontend, s.opts.FrontendRoot)
	if err != nil {
		return core.E("desktop.attachSPA", "frontend root not found", err)
	}
	fileServer := http.FileServer(http.FS(sub))
	s.opts.Server.Engine().NoRoute(func(c *gin.Context) {
		fileServer.ServeHTTP(c.Writer, c.Request)
	})
	return nil
}

// ginMiddleware delegates /wails/* requests back to Wails, hands
// everything else to the engine. Matches the example at
// wails/v3/examples/gin-routing/main.go.
func ginMiddleware(engine http.Handler) application.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, "/wails") {
				next.ServeHTTP(w, r)
				return
			}
			engine.ServeHTTP(w, r)
		})
	}
}
