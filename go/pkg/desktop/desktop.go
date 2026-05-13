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

	core "dappco.re/go"
	"dappco.re/go/config"
	coreI18n "dappco.re/go/i18n"
	"dappco.re/lthn/desktop/pkg/apikey"
	"dappco.re/lthn/desktop/pkg/bridge"
	"dappco.re/lthn/desktop/pkg/build"
	"dappco.re/lthn/desktop/pkg/container"
	"dappco.re/lthn/desktop/pkg/firstlaunch"
	"dappco.re/lthn/desktop/pkg/git"
	"dappco.re/lthn/desktop/pkg/integrations"
	"dappco.re/lthn/desktop/pkg/lint"
	"dappco.re/lthn/desktop/pkg/marketplace"
	"dappco.re/lthn/desktop/pkg/models"
	lthnphp "dappco.re/lthn/desktop/pkg/php"
	"dappco.re/lthn/desktop/pkg/plugin"
	"dappco.re/lthn/desktop/pkg/repos"
	"dappco.re/lthn/desktop/pkg/runner"
	"dappco.re/lthn/desktop/pkg/sandbox"
	"dappco.re/lthn/desktop/pkg/server"
	lthnservices "dappco.re/lthn/desktop/pkg/services"
	"dappco.re/lthn/desktop/pkg/sessions"
	"dappco.re/lthn/desktop/pkg/telemetry"
	"dappco.re/lthn/desktop/pkg/tools"
	"dappco.re/lthn/desktop/pkg/validator"
	"github.com/gin-gonic/gin"
	"github.com/leaanthony/u"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"github.com/wailsapp/wails/v3/pkg/icons"
	"github.com/wailsapp/wails/v3/pkg/services/dock"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"
)

const trayOpenEvent = "lthn:tray:open"

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
	// AppIcon is the application icon shown in the default About box
	// (application.Options.Icon). Empty = Wails-default 'W'. macOS
	// Dock / Launchpad uses build/darwin/icons.icns from the .app
	// bundle separately; both should derive from the same source PNG
	// to stay visually consistent.
	AppIcon []byte
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

// Register constructs the desktop service for Core registration.
//
// Usage example:
//
//	core.New(core.WithService(desktop.Register))
func Register(c *core.Core) core.Result {
	return core.Ok(NewService(Options{Core: c}))
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

	// Mount the dappco.re/go/api + dappco.re/go/mcp HTTP surfaces onto
	// our gin engine BEFORE the SPA fallback — pkg/server's NoRoute
	// fallback catches anything not handled by registered routes, so
	// these prefixed handlers need to be in place first.
	if r := mountSubsystems(s.opts.Core, s.opts.Server.Engine(), s.opts.Runner); !r.OK {
		return r
	}

	// Mount the SPA fallback on the gin engine — every request that
	// doesn't match a registered API route falls through to the
	// embedded dist.
	if r := s.attachSPA(); !r.OK {
		return r
	}

	engine := s.opts.Server.Handler()

	// Binding adapters exposed to the WebView via Wails TS bindings.
	// Each adapter wraps one lthn domain package, translating
	// core.Result → (T, error) for clean TS typing.
	//
	// dock.New() is the Wails-native dock/taskbar service — it
	// ships in `(T, error)` shape already, no adapter needed. The
	// generated TS bindings give the frontend SetBadge / RemoveBadge
	// / HideAppIcon / ShowAppIcon. Useful for unread-count badges
	// over the tray (macOS draws them on the menubar item) and for
	// surfacing the app in the Dock when chat is active (so users
	// can ⌘-Tab to it), then hiding back to tray-only when chat
	// closes.
	notifier := notifications.New()
	// Dock service captured so windows.go can elevate/demote the macOS
	// activation policy when the unified app shell opens / closes. See
	// policy.go for the routing.
	dockSvc := dock.New()
	attachDock(dockSvc)
	// Window service stays as a thin wrapper today — it dispatches
	// against the in-process openWindow registry rather than wrapping
	// any single Go package. Once windows.go grows into a real
	// package this becomes a direct registration like the others.
	windowSvc := NewWindowService()

	// Fetch the Core-registered upstream services and register them
	// directly with Wails so bindings land at dappco.re/go/<pkg>/.
	// No adapter layer — Wails generates straight from the package.
	i18nSvc, _ := core.ServiceFor[*coreI18n.CoreService](s.opts.Core, "i18n")
	configSvc, _ := core.ServiceFor[*config.Service](s.opts.Core, "config")
	bridgeSvc, _ := core.ServiceFor[*bridge.Service](s.opts.Core, "bridge")
	pluginSvc, _ := core.ServiceFor[*plugin.Service](s.opts.Core, "plugin")
	sandboxSvc, _ := core.ServiceFor[*sandbox.Service](s.opts.Core, "sandbox")

	wailsServices := []application.Service{
		// In-this-repo packages — each ships its own *WailsService /
		// *Service with Wails3 lifecycle + (T, error) methods. Bindings
		// land at frontend/bindings/dappco.re/lthn/desktop/pkg/<pkg>/.
		application.NewService(s.opts.Runner),
		application.NewService(s.opts.Server),
		application.NewService(sessions.NewWailsService(s.opts.Core)),
		application.NewService(models.NewWailsService()),
		application.NewService(firstlaunch.NewWailsService()),
		application.NewService(integrations.NewWailsService()),
		application.NewService(apikey.NewWailsService(s.opts.Core)),
		application.NewService(git.NewService(s.opts.Core)),
		application.NewService(build.NewService(s.opts.Core)),
		application.NewService(container.NewService(s.opts.Core)),
		application.NewService(lint.NewService(s.opts.Core)),
		application.NewService(marketplace.NewService(s.opts.Core)),
		application.NewService(lthnphp.NewService(s.opts.Core)),
		application.NewService(pluginSvc),
		application.NewService(sandboxSvc),
		application.NewService(repos.NewService(s.opts.Core)),
		application.NewService(tools.NewWailsService(s.opts.Core)),
		application.NewService(validator.NewWailsService()),
		application.NewService(telemetry.NewService(telemetry.Options{})),
		application.NewService(lthnservices.NewWailsService()),
		// Upstream dappco.re/go services — register the Core-built
		// instances directly. Bindings land at frontend/bindings/
		// dappco.re/go/<pkg>/.
		application.NewService(i18nSvc),
		application.NewService(configSvc),
		application.NewService(bridgeSvc),
		// Window registry — see note above.
		application.NewService(windowSvc),
		// Wails3 native services — dock + notifications ship from
		// upstream wails/v3/pkg/services. Frontend env / dialog /
		// browser / screen / clipboard come straight from
		// @wailsio/runtime — no Go wrapper.
		application.NewService(dockSvc),
		application.NewService(notifier),
	}

	s.app = application.New(application.Options{
		Name:        s.opts.Name,
		Description: s.opts.Description,
		Icon:        s.opts.AppIcon,
		Services:    wailsServices,
		// SingleInstance — a second launch hands off URL/file/args
		// to the first instance via OnSecondInstanceLaunch and then
		// exits. UniqueID is the macOS Bundle Identifier so the OS
		// flock honours app-store distribution + dev/prod isolation.
		//
		// EncryptionKey enables AES-256-GCM on the inter-instance
		// channel. Without it the wails docs warn:
		// "your app should treat any data passed to it from second
		//  instance callback as untrusted". Since lthn re-broadcasts
		// the second-instance payload straight onto the lthn:* event
		// bus where the frontend acts on it, untrusted args are a
		// real injection risk. The 32-byte literal here is build-time
		// constant (same value in every install of the same binary,
		// so legit second launches authenticate against the running
		// first instance; a third-party attacker would need the
		// binary's bytes to forge a payload).
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: "io.lethean.desktop",
			EncryptionKey: [32]byte{
				0x6c, 0x74, 0x68, 0x6e, 0x2e, 0x73, 0x69, 0x6e,
				0x67, 0x6c, 0x65, 0x2d, 0x69, 0x6e, 0x73, 0x74,
				0x61, 0x6e, 0x63, 0x65, 0x2e, 0x76, 0x31, 0x2e,
				0x6c, 0x65, 0x74, 0x68, 0x65, 0x61, 0x6e, 0x21,
			},
			AdditionalData: map[string]string{
				"app":     "lthn-desktop",
				"version": "0.2.0-rc1",
			},
			OnSecondInstanceLaunch: func(d application.SecondInstanceData) {
				if s.app == nil {
					return
				}
				// Re-broadcast the second-launch context so any
				// frontend subscribers (router / wizard / chat)
				// can act on it. Same shape as ApplicationStarted
				// / OpenedWithFile / LaunchedWithUrl emit. Safe to
				// trust because EncryptionKey above authenticates
				// the channel — anything reaching this callback
				// came from a binary holding the same key.
				s.app.Event.Emit("lthn:app:second-instance", map[string]any{
					"args":       d.Args,
					"workdir":    d.WorkingDir,
					"additional": d.AdditionalData,
				})
				// Bring the unified app shell (preferred) or the
				// tray popover back to the foreground. Restore()
				// before Focus() handles the case where the window
				// was minimised — Wails docs canon for the second-
				// instance UX.
				if w, ok := s.app.Window.GetByName("app"); ok {
					w.Restore()
					w.Focus()
				} else if w, ok := s.app.Window.GetByName("tray"); ok {
					w.Restore()
					w.Focus()
				}
			},
		},
		// ShouldQuit fires when the OS / user requests quit. Today
		// we always allow — pkg/sessions and pkg/store flush on
		// OnShutdown and survive a clean exit. Return false here to
		// veto (e.g. unsaved-state guard once chat composer state
		// is wired into the loop).
		ShouldQuit: func() bool { return true },
		// OnShutdown — pre-quit cleanup. Sessions persist to the
		// store between every interaction already; this is the
		// belt-and-braces flush + a hook for any service that
		// needs to drain in-flight work (runner / telemetry).
		OnShutdown: func() {
			s.app.Event.Emit("lthn:app:shutdown", nil)
		},
		// PostShutdown runs after the Wails event loop has fully
		// stopped. Last chance to close anything that held a ref
		// into the event loop (HTTP server, store, runner).
		PostShutdown: func() {
			if s.opts.Server != nil {
				if r := s.opts.Server.Stop(core.Background()); !r.OK {
					core.Warn("desktop server shutdown failed", "err", r.Error())
				}
			}
		},
		// PanicHandler captures uncaught panics from Go-side service
		// methods (binding adapters etc.) and re-broadcasts them
		// onto lthn:app:panic so the frontend can show a crash
		// pane + offer a "send report" button. Without this the
		// panic kills the process silently.
		PanicHandler: func(details *application.PanicDetails) {
			if s.app == nil || details == nil {
				return
			}
			errStr := ""
			if details.Error != nil {
				errStr = details.Error.Error()
			}
			s.app.Event.Emit("lthn:app:panic", map[string]any{
				"error":      errStr,
				"stack":      details.StackTrace,
				"full_stack": details.FullStackTrace,
			})
		},
		Mac: application.MacOptions{
			// Tray IS the process — closing every window must NOT quit.
			ApplicationShouldTerminateAfterLastWindowClosed: false,
			// Accessory: menu-bar only, no Dock icon, no Cmd+Tab entry.
			ActivationPolicy: application.ActivationPolicyAccessory,
		},
		Windows: application.WindowsOptions{
			// Windows-side equivalent of the Mac flag above — without
			// this, closing the last window quits the process and the
			// systray goes with it. v3/examples/systray-custom canon.
			DisableQuitOnLastWindowClosed: true,
			// Enable WebView2's draggable-regions feature — Wails3
			// needs this for --wails-draggable CSS to work on
			// Windows (macOS handles it natively without the flag).
			EnabledFeatures: []string{"msWebView2EnableDraggableRegions"},
		},
		Assets: application.AssetOptions{
			Handler:    engine,
			Middleware: ginMiddleware(engine),
		},
	})

	// Attach the constructed app to services that need app refs
	// post-construction (the Wails App reference isn't available
	// pre-application.New()). Today only WindowService still
	// depends on this — env / clipboard / screen / browser / dialog
	// previously wrapped here are now consumed by the frontend
	// directly from @wailsio/runtime.
	windowSvc.app = s.app

	// Re-broadcast OS theme changes to the WebView as "lthn:theme".
	// Lit elements subscribe via @wailsio/runtime's Events.On.
	s.app.Event.OnApplicationEvent(events.Common.ThemeChanged, func(_ *application.ApplicationEvent) {
		mode := "light"
		if s.app.Env.IsDarkMode() {
			mode = "dark"
		}
		s.app.Event.Emit("lthn:theme", mode)
	})

	// Bridge notification-action callbacks back to the WebView as
	// "lthn:notification:response". The Lit element that sent the
	// notification subscribes via Events.On and dispatches per the
	// response.ActionIdentifier (OPEN / REPLY / ARCHIVE / etc.).
	notifier.OnNotificationResponse(func(result notifications.NotificationResult) {
		s.app.Event.Emit("lthn:notification:response", result.Response)
	})

	// Application menu — macOS-only. Accessory apps still get a
	// menubar when their windows are focused, and standard roles
	// give us Cmd+Q / Cmd+W / Cmd+M / Cmd+H / Edit menu shortcuts
	// for free. Without AddRole(AppMenu) we'd lose those.
	if runtime.GOOS == "darwin" {
		appMenu := s.app.Menu.New()
		appMenu.AddRole(application.AppMenu)
		appMenu.AddRole(application.EditMenu)
		appMenu.AddRole(application.WindowMenu)
		s.app.Menu.Set(appMenu)
	}

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

	// Systray menu — quick-access verbs that match the popover
	// surfaces. The "open" entries emit window-open events the
	// frontend listens for, so navigation goes through the same
	// Lit router whether triggered by tray or by an in-popover link.
	menu := s.app.Menu.New()
	// Open Lethean Desktop is the headline menu item — the same action
	// the popover's screen-icon button triggers. Used to be a disabled
	// label; promoted to the primary verb so the systray right-click
	// menu has parity with the in-popover surface.
	menu.Add("Open Lethean Desktop").OnClick(func(_ *application.Context) {
		openWindow(s.app, "app")
		s.app.Event.Emit(trayOpenEvent, "app")
	})
	menu.AddSeparator()
	menu.Add("Open Chat…").OnClick(func(_ *application.Context) {
		openWindow(s.app, "chat")
		s.app.Event.Emit(trayOpenEvent, "chat")
	})
	menu.Add("Models…").OnClick(func(_ *application.Context) {
		openWindow(s.app, "models")
		s.app.Event.Emit(trayOpenEvent, "models")
	})
	menu.Add("Settings…").OnClick(func(_ *application.Context) {
		openWindow(s.app, "settings")
		s.app.Event.Emit(trayOpenEvent, "settings")
	})

	// Plugins — dynamic per-plugin menu entries surfaced from
	// the running plugin host. Each installed plugin with a
	// manifest.menu block gets one entry; clicking opens a
	// generic "plugin" window with ?code=<code> so the Lit
	// router can mount the plugin's UI (iframe via
	// ui.entrypoint when declared).
	if pluginSvc != nil {
		entriesR := pluginSvc.Menus()
		if entriesR.OK {
			entries, _ := entriesR.Value.([]plugin.MenuEntry)
			if len(entries) > 0 {
				menu.AddSeparator()
				for _, e := range entries {
					code := e.Code
					label := e.Label
					if !e.Running {
						label = label + " · stopped"
					}
					menu.Add(label).OnClick(func(_ *application.Context) {
						openPluginWindow(s.app, code)
						s.app.Event.Emit(trayOpenEvent, "plugin:"+code)
					})
				}
			}
		}
	}

	menu.AddSeparator()
	menu.Add("About lthn").OnClick(func(_ *application.Context) {
		openWindow(s.app, "about")
		s.app.Event.Emit(trayOpenEvent, "about")
	})
	menu.AddSeparator()
	menu.Add("Quit lthn").OnClick(func(_ *application.Context) {
		s.app.Quit()
	})
	systray.SetMenu(menu)

	// Context menus — right-click surfaces for the chat UI. Lit
	// elements declare `style="--custom-contextmenu: lthn-message"`
	// (etc.) plus `--custom-contextmenu-data: <message-id>` so the
	// click handler knows WHICH message was right-clicked. Each
	// action emits an "lthn:context:<menu>:<action>" event with the
	// data; the originating Lit element dispatches accordingly.
	registerContextMenus(s.app)

	// Global keyboard shortcuts. Each emits "lthn:key:<verb>" with
	// the active window's name. Cmd+J toggle popover / Cmd+N new
	// session / Cmd+, settings / etc. See keybindings.go.
	registerKeyBindings(s.app)

	// System event re-broadcasts. Wails' ApplicationStarted /
	// OpenedWithFile / LaunchedWithUrl get republished as lthn:app:*
	// so the frontend has one event-bus contract for everything.
	// See sysevents.go for the table.
	registerSystemEvents(s.app)

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
		// Transparent background so the rounded card corners render
		// against the desktop, matching the rest of the windows.
		BackgroundColour: application.NewRGBA(0, 0, 0, 0),
		// We ship our own context menus — the WebView's native menu
		// would only confuse things on the tray popover.
		DefaultContextMenuDisabled: true,
		Mac: application.MacWindow{
			// Floating keeps the popover above normal windows when
			// AlwaysOnTop isn't enough (Lion+ raised the bar for
			// "above-everything" — Floating is the menubar-utility
			// level used by Bartender / Itsycal / 1Password Mini).
			WindowLevel: application.MacWindowLevelFloating,
			// CanJoinAllSpaces — the popover appears on every Space
			// (no disappearing when the user swipes desktops).
			// FullScreenAuxiliary — the popover can overlay
			// fullscreen apps (otherwise it would vanish whenever
			// the user enters Cmd+Ctrl+F on any app).
			// IgnoresCycle — exclude from Cmd+` window cycling.
			CollectionBehavior: application.MacWindowCollectionBehaviorCanJoinAllSpaces |
				application.MacWindowCollectionBehaviorFullScreenAuxiliary |
				application.MacWindowCollectionBehaviorIgnoresCycle,
			// Top 40px = our renderChrome titlebar strip — declare
			// it as the OS-native drag region so macOS picks up the
			// drag without depending on the --wails-draggable CSS
			// path alone.
			InvisibleTitleBarHeight: 40,
			WebviewPreferences: application.MacWebviewPreferences{
				AllowsBackForwardNavigationGestures: u.False,
			},
		},
		Linux: application.LinuxWindow{
			Icon: s.opts.AppIcon,
		},
		Windows: application.WindowsWindow{
			HiddenOnTaskbar: true,
		},
	})

	// Close-hides rather than destroys — tray-rooted lifecycle.
	window.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		window.Hide()
		e.Cancel()
	})

	// Per-window lthn:window:* event re-broadcasts (ready / focus /
	// blur / hide / show / resize / files-dropped). See sysevents.go.
	registerWindowEvents(s.app, window)

	// Pre-create welcome / chat / models / settings / about windows
	// hidden, so first tray-menu open is instant. See windows.go.
	// Options threaded through for Linux icon + future per-window
	// opts that depend on s.opts (telemetry endpoint, brand, etc.).
	preCreateWindows(s.app, s.opts)

	systray.AttachWindow(window).WindowOffset(5)

	// First-launch detection — if ~/Lethean/conf/lthn.yaml and the
	// state DB don't exist, open the welcome wizard on top of the
	// systray rather than dumping a fresh user straight into the
	// popover. The wizard's final step writes config + opens the
	// settings window; firstlaunch.Detect flips fresh→false naturally.
	if state := firstlaunch.Detect(nil); state.OK {
		if fl, ok := state.Value.(firstlaunch.State); ok && fl.Fresh {
			openWindow(s.app, "welcome")
		}
	}

	if err := s.app.Run(); err != nil {
		return core.Fail(err)
	}
	return core.Ok(nil)
}

// attachSPA mounts the embedded frontend as the coreapi.Engine's
// no-route fallback. Anything that doesn't match an explicit lthn /
// subsystem route gets served from the embedded dist — index.html,
// assets/*, etc. The handler inherits the canonical middleware chain
// (auth, sunset, cache, tracing) just like any other route.
func (s *Service) attachSPA() core.Result {
	sub, err := fs.Sub(s.opts.Frontend, s.opts.FrontendRoot)
	if err != nil {
		return core.Fail(core.E("desktop.attachSPA", "frontend root not found", err))
	}
	fileServer := http.FileServer(http.FS(sub))
	s.opts.Server.Engine().SetNoRoute(func(c *gin.Context) {
		fileServer.ServeHTTP(c.Writer, c.Request)
	})
	return core.Ok(nil)
}

// ginMiddleware delegates /wails/* requests back to Wails, hands
// everything else to the engine. Matches the example at
// wails/v3/examples/gin-routing/main.go.
//
// One carve-out: /wails/custom.js. Wails' runtime fetches this URL
// at boot to allow user-supplied JS overrides; we don't ship any,
// so the default Wails handler returns 404 and spams the console.
// Intercept here and return an empty 200 instead — the runtime
// happily continues with no overrides applied.
func ginMiddleware(engine http.Handler) application.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/wails/custom.js" {
				w.Header().Set("Content-Type", "application/javascript")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("/* no user overrides */\n"))
				return
			}
			if core.HasPrefix(r.URL.Path, "/wails") {
				next.ServeHTTP(w, r)
				return
			}
			engine.ServeHTTP(w, r)
		})
	}
}
