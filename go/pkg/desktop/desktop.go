// SPDX-Licence-Identifier: EUPL-1.2

// Package desktop is the CoreGUI-backed runtime for the lthn GUI mode.
// Wails remains the private bootstrap substrate here only until CoreGUI
// exposes the app-construction and tray-window surface.
// Constructs the Application with:
//
//   - Assets.Handler = the same gin engine pkg/server exposes for
//     `lthn serve`. The WebView reaches /v1/chat/completions etc.
//     same-origin — no CORS, no port hunting.
//   - Assets.Middleware = the Gin middleware pattern from
//     the native webview runtime: delegate /wails/* back to
//     Wails internals, hand everything else to Gin.
//   - A NoRoute fallback on the Gin engine that proxies frontend requests
//     to Angular during Wails development and otherwise serves the embedded
//     Angular dist so the SPA loads at "/".
//   - Mac.ActivationPolicy = Accessory (menu-bar app, no Dock icon).
//   - ApplicationShouldTerminateAfterLastWindowClosed = false
//     (the NSStatusItem remains the process lifetime anchor).
//   - SystemTray attached to a compact Angular panel; left-click opens
//     the always-on-top panel while the status-item menu remains available.
//
// The main window loads Angular's hash-located OS shell at "/#/".
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
	"runtime"

	core "dappco.re/go"
	"dappco.re/go/config"
	gui "dappco.re/go/render/display/webkit"
	guilifecycle "dappco.re/go/render/display/webkit/pkg/lifecycle"
	guisystray "dappco.re/go/render/display/webkit/pkg/systray"
	guiwindow "dappco.re/go/render/display/webkit/pkg/window"
	lthn "dappco.re/lthn/desktop"
	"dappco.re/lthn/desktop/pkg/account"
	"dappco.re/lthn/desktop/pkg/agents"
	"dappco.re/lthn/desktop/pkg/apikey"
	"dappco.re/lthn/desktop/pkg/appconfig"
	"dappco.re/lthn/desktop/pkg/audit"
	"dappco.re/lthn/desktop/pkg/benchmark"
	"dappco.re/lthn/desktop/pkg/build"
	"dappco.re/lthn/desktop/pkg/calibrate"
	"dappco.re/lthn/desktop/pkg/clbpl"
	"dappco.re/lthn/desktop/pkg/connection"
	"dappco.re/lthn/desktop/pkg/container"
	"dappco.re/lthn/desktop/pkg/contentshield"
	"dappco.re/lthn/desktop/pkg/deploys"
	"dappco.re/lthn/desktop/pkg/desktopstate"
	"dappco.re/lthn/desktop/pkg/downloader"
	"dappco.re/lthn/desktop/pkg/firstlaunch"
	"dappco.re/lthn/desktop/pkg/fleet"
	"dappco.re/lthn/desktop/pkg/git"
	"dappco.re/lthn/desktop/pkg/incidents"
	"dappco.re/lthn/desktop/pkg/integrations"
	"dappco.re/lthn/desktop/pkg/keys"
	"dappco.re/lthn/desktop/pkg/lemma"
	"dappco.re/lthn/desktop/pkg/lint"
	"dappco.re/lthn/desktop/pkg/marketing/analytics"
	"dappco.re/lthn/desktop/pkg/marketing/audience"
	"dappco.re/lthn/desktop/pkg/marketing/campaigns"
	"dappco.re/lthn/desktop/pkg/marketing/content"
	"dappco.re/lthn/desktop/pkg/marketing/social"
	"dappco.re/lthn/desktop/pkg/marketplace"
	"dappco.re/lthn/desktop/pkg/modelruntime"
	"dappco.re/lthn/desktop/pkg/office/documents"
	officefile "dappco.re/lthn/desktop/pkg/office/files"
	"dappco.re/lthn/desktop/pkg/office/mail"
	"dappco.re/lthn/desktop/pkg/ollama"
	"dappco.re/lthn/desktop/pkg/openaibench"
	"dappco.re/lthn/desktop/pkg/opencode"
	"dappco.re/lthn/desktop/pkg/paths"
	"dappco.re/lthn/desktop/pkg/permissions"
	lthnphp "dappco.re/lthn/desktop/pkg/php"
	"dappco.re/lthn/desktop/pkg/plugin"
	"dappco.re/lthn/desktop/pkg/r1"
	r1analytics "dappco.re/lthn/desktop/pkg/r1/analytics"
	"dappco.re/lthn/desktop/pkg/repos"
	"dappco.re/lthn/desktop/pkg/runbooks"
	"dappco.re/lthn/desktop/pkg/runner"
	"dappco.re/lthn/desktop/pkg/sales/contacts"
	"dappco.re/lthn/desktop/pkg/sales/deals"
	"dappco.re/lthn/desktop/pkg/sales/forecast"
	"dappco.re/lthn/desktop/pkg/sales/pipeline"
	"dappco.re/lthn/desktop/pkg/sandbox"
	"dappco.re/lthn/desktop/pkg/seeds"
	"dappco.re/lthn/desktop/pkg/server"
	"dappco.re/lthn/desktop/pkg/serverkey"
	lthnservices "dappco.re/lthn/desktop/pkg/services"
	"dappco.re/lthn/desktop/pkg/sessions"
	"dappco.re/lthn/desktop/pkg/tasks"
	"dappco.re/lthn/desktop/pkg/telemetry"
	"dappco.re/lthn/desktop/pkg/terminal"
	"dappco.re/lthn/desktop/pkg/tools"
	"dappco.re/lthn/desktop/pkg/training"
	"dappco.re/lthn/desktop/pkg/validator"
	"dappco.re/lthn/desktop/pkg/vi"
	"github.com/gin-gonic/gin"
)

const trayOpenEvent = "lthn:tray:open"

// Tray menu ActionIDs — these are the labels the core/gui systray
// emits in ActionTrayMenuItemClicked when a tray menu entry is
// clicked. The Run() handler below routes each one to openWindow +
// emitCoreEvent. Plugin entries reuse the trayPluginPrefix and pack
// the plugin code after the colon (e.g. "lthn:tray:plugin:my-code").
const (
	trayActionOpenApp      = "lthn:tray:open-app"
	trayActionOpenChat     = "lthn:tray:open-chat"
	trayActionOpenModels   = "lthn:tray:open-models"
	trayActionOpenSettings = "lthn:tray:open-settings"
	trayActionOpenApps     = "lthn:tray:open-apps"
	trayActionOpenAbout    = "lthn:tray:open-about"
	trayActionQuit         = "lthn:tray:quit"
	trayPluginPrefix       = "lthn:tray:plugin:"
)

// Options configures the desktop service.
type Options struct {
	// Name is the macOS menu-bar accessibility label. Default: "lthn".
	Name string
	// Description is the app description shown by the OS.
	Description string
	// Frontend is the embedded Angular build (cmd/lthn/embed.go). The
	// service proxies frontend requests to Angular during Wails development
	// and otherwise serves it via a NoRoute fallback on the gin engine.
	// The core.FS root should contain index.html at its top level.
	Frontend core.FS
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
	// Fleet owns the user's compute-fleet view (machines, routing
	// rules) + the live agent_activity queue. Backed by the master
	// DuckDB at ~/Lethean/data/lthn.duckdb. Required.
	Fleet *fleet.Service
	// Keys owns the encrypted-at-rest secrets store under
	// ~/Lethean/data/keys/. Frontend writes provider API keys via
	// the Wails binding; plaintext never crosses the WebView (Get
	// is Go-side only). Required.
	Keys *keys.Service
	// Connection owns the Wails WebSocket transport and its remotely
	// reachable client set. Required so the GUI, a browser, or a mobile
	// client can all use the same binding surface.
	Connection *connection.Service
	// TrayIcon is the light-mode systray icon bytes. Empty leaves the
	// platform default tray glyph unchanged.
	TrayIcon []byte
	// AppIcon is the application icon shown in the default About box
	// (application.Options.Icon). Empty = Wails-default 'W'. macOS
	// Dock / Launchpad uses build/darwin/icons.icns from the .app
	// bundle separately; both should derive from the same source PNG
	// to stay visually consistent.
	AppIcon []byte
	// ShowAppOnLaunch opens the full Angular OS shell after
	// ApplicationStarted. False starts as a tray-only process; clicking
	// the status item still opens the compact tray panel.
	ShowAppOnLaunch bool
}

// Service holds the core/gui configuration facade, the transport-aware
// Wails runtime, and the SystemTray anchor.
type Service struct {
	opts    Options
	gui     *gui.Service
	runtime *guiRuntime
	// selfRefreshStop signals the local-machine refresh ticker to
	// exit. Closed by PostShutdown; the goroutine selects on it
	// alongside the ticker channel.
	selfRefreshStop chan struct{}
	// trayStatusStop signals the tray-tooltip refresh ticker to exit.
	// Closed by PostShutdown; the goroutine selects on it alongside
	// the ticker channel.
	trayStatusStop chan struct{}
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

// Register is the zero-option core.WithName-compatible factory. Use
// RegisterService(opts) when the caller needs to bind Frontend, icons,
// or other binary-state options at registration time (those can't be
// supplied via post-registration config since the Wails app captures
// them at construction).
//
// Usage example:
//
//	core.New(core.WithName("desktop", desktop.Register))
func Register(c *core.Core) core.Result {
	return core.Ok(NewService(Options{Core: c}))
}

// RegisterService is the options-binding factory. Resolves Server /
// Runner / Fleet / Keys from the Core service registry at registration
// time so the caller only needs to supply binary-state options
// (Frontend, TrayIcon, AppIcon, ShowAppOnLaunch).
//
// Usage example:
//
//	core.New(
//	    core.WithName("server", server.RegisterService(serverOpts)),
//	    core.WithName("desktop", desktop.RegisterService(desktop.Options{
//	        Frontend:        frontendDist,
//	        TrayIcon:        trayIcon,
//	        AppIcon:         appIcon,
//	        ShowAppOnLaunch: dev,
//	    })),
//	)
func RegisterService(opts Options) func(*core.Core) core.Result {
	return func(c *core.Core) core.Result {
		opts.Core = c
		if opts.Server == nil {
			if s, _ := core.ServiceFor[*server.Service](c, "server"); s != nil {
				opts.Server = s
			}
		}
		if opts.Runner == nil {
			if r, _ := core.ServiceFor[*runner.Service](c, "runner"); r != nil {
				opts.Runner = r
			}
		}
		if opts.Fleet == nil {
			if f, _ := core.ServiceFor[*fleet.Service](c, "fleet"); f != nil {
				opts.Fleet = f
			}
		}
		if opts.Keys == nil {
			if k, _ := core.ServiceFor[*keys.Service](c, "keys"); k != nil {
				opts.Keys = k
			}
		}
		if opts.Connection == nil {
			if manager, _ := core.ServiceFor[*connection.Service](c, "connection"); manager != nil {
				opts.Connection = manager
			}
		}
		return core.Ok(NewService(opts))
	}
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
	if s.opts.Connection == nil {
		return core.Fail(core.E("desktop.Run", "Connection is required", nil))
	}
	servicesSvc, servicesOK := core.ServiceFor[*lthnservices.Service](
		s.opts.Core,
		"services",
	)
	if !servicesOK || servicesSvc == nil {
		return core.Fail(core.E(
			"desktop.Run",
			"managed services are unavailable",
			nil,
		))
	}
	modelRuntimeSvc, modelRuntimeOK := core.ServiceFor[*modelruntime.Service](
		s.opts.Core,
		"modelruntime",
	)
	if !modelRuntimeOK || modelRuntimeSvc == nil {
		return core.Fail(core.E(
			"desktop.Run",
			"model runtime is unavailable",
			nil,
		))
	}
	configSvc, _ := core.ServiceFor[*config.Service](s.opts.Core, "config")
	singleInstanceEnabled := desktopSingleInstanceEnabled(configSvc)

	// Resolve the per-install SingleInstance encryption key. Generated
	// once on first launch, persisted at ~/Lethean/data/keys/
	// single-instance.aead, reloaded on every subsequent boot.
	// Cerberus #1442: replaces the build-time constant that was shared
	// across every installed binary on every machine.
	var singleInstanceKey [32]byte
	if singleInstanceEnabled && s.opts.Keys != nil {
		kr := s.opts.Keys.SingleInstanceKey()
		if !kr.OK {
			return core.Fail(core.E("desktop.Run", "load single-instance key", kr.Value.(error)))
		}
		singleInstanceKey = kr.Value.([32]byte)
	}

	// Mount the dappco.re/go/api + dappco.re/go/mcp HTTP surfaces onto
	// our gin engine BEFORE the SPA fallback — pkg/server's NoRoute
	// fallback catches anything not handled by registered routes, so
	// these prefixed handlers need to be in place first.
	if r := mountSubsystems(s.opts.Core, s.opts.Server.Engine(), s.opts.Runner); !r.OK {
		return r
	}

	// Mount the SPA fallback on the gin engine — every request that
	// doesn't match a registered API route falls through to Angular
	// during Wails development or the embedded dist otherwise.
	if r := s.attachSPA(); !r.OK {
		return r
	}

	engine := s.opts.Server.Handler()

	// Binding adapters exposed to the WebView via Wails TS bindings.
	// Each adapter wraps one lthn domain package, translating
	// core.Result → (T, error) for clean TS typing.
	//
	// CoreGUI owns native dock/taskbar and notification services. Wails
	// remains its private substrate; lthn routes those behaviours through
	// Core actions. Dock-icon elevation for the `app` window is declared
	// via Window.ShowDockIcon in the registry; gui.OpenWindow auto-fires
	// dock.show_icon when it opens that window.
	// Window IPC binding now ships from core/gui — gui.NewWindowBindingService
	// exposes Open / Hide / List / SetSize verbs that route through
	// gui.OpenWindow / gui.HideWindow / GuiConfig.WindowRegistry.
	windowSvc := gui.NewWindowBindingService(s.opts.Core)

	// Fetch the renderer-facing services. Raw CoreGO config and i18n services
	// remain backend composition concerns: Angular uses its compile-time
	// localisation catalogue and the curated appconfig bridge.
	appconfigSvc := appconfig.NewService(appconfig.Options{Core: s.opts.Core})
	pluginSvc, _ := core.ServiceFor[*plugin.Service](s.opts.Core, "plugin")
	sandboxSvc, _ := core.ServiceFor[*sandbox.Service](s.opts.Core, "sandbox")
	opencodeSvc, _ := core.ServiceFor[*opencode.Service](s.opts.Core, "opencode")
	// Use the Core-registered repos service so RegisterSource calls
	// (e.g. opencode-imports wiring in cmd/lthn/app.go) take effect
	// — constructing a fresh instance here would create a sibling
	// that doesn't see the registered sources.
	reposSvc, _ := core.ServiceFor[*repos.Service](s.opts.Core, "repos")
	// Vi — Lethean Desktop mascot's data spine. The Core-registered
	// instance owns the probe loop; the same instance is bound to
	// Wails here so Sites() / Catalogue() resolve to live data.
	viSvc, _ := core.ServiceFor[*vi.Service](s.opts.Core, "vi")
	// incidents — Operations view incident log. Core-registered instance
	// so events fired during Wails method calls propagate on the shared bus.
	incidentsSvc, _ := core.ServiceFor[*incidents.Service](s.opts.Core, "incidents")
	// runbooks — Operations view runbook library. Core-registered instance.
	runbooksSvc, _ := core.ServiceFor[*runbooks.Service](s.opts.Core, "runbooks")
	// sales/contacts — CRM contact catalogue. Core-registered so events fired
	// during Wails method calls propagate on the shared bus.
	contactsSvc, _ := core.ServiceFor[*contacts.Service](s.opts.Core, "sales-contacts")
	// sales/deals — deal record + activity log. Core-registered; source-of-truth
	// for pipeline stage consumed by sales/pipeline.
	dealsSvc, _ := core.ServiceFor[*deals.Service](s.opts.Core, "sales-deals")
	// sales/pipeline — derived Kanban rollup. Core-registered; reads from the
	// same deals dir that dealsSvc writes to.
	pipelineSvc, _ := core.ServiceFor[*pipeline.Service](s.opts.Core, "sales-pipeline")
	// sales/forecast — quarterly probability-weighted rollup. Core-registered;
	// reads from deals dir, fires no events.
	forecastSvc, _ := core.ServiceFor[*forecast.Service](s.opts.Core, "sales-forecast")
	// marketing/campaigns — campaign thread catalogue. Core-registered so events
	// fired during Wails method calls propagate on the shared bus.
	campaignsSvc, _ := core.ServiceFor[*campaigns.Service](s.opts.Core, "marketing-campaigns")
	// marketing/content — editorial content calendar. Core-registered.
	contentSvc, _ := core.ServiceFor[*content.Service](s.opts.Core, "marketing-content")
	// marketing/social — social post queue. Core-registered.
	socialSvc, _ := core.ServiceFor[*social.Service](s.opts.Core, "marketing-social")
	// marketing/audience — subscriber segment catalogue. Core-registered.
	audienceSvc, _ := core.ServiceFor[*audience.Service](s.opts.Core, "marketing-audience")
	// marketing/analytics — web analytics rollup (read-only). Core-registered.
	analyticsSvc, _ := core.ServiceFor[*analytics.Service](s.opts.Core, "marketing-analytics")
	// office/documents — Office document catalogue. Core-registered; read-only v1.
	documentsSvc, _ := core.ServiceFor[*documents.Service](s.opts.Core, "office-documents")
	// office/mail — Office mailbox catalogue. Core-registered; read-only v1.
	mailSvc, _ := core.ServiceFor[*mail.Service](s.opts.Core, "office-mail")
	// office/files — sole provider-neutral Files binding. Its registered
	// io.Medium mounts enforce the content and metadata boundary.
	filesSvc, _ := core.ServiceFor[*officefile.Service](s.opts.Core, "office-files")
	permissionSvc, _ := core.ServiceFor[*permissions.Service](
		s.opts.Core,
		"permissions",
	)
	// telemetry — bind the Core-registered sampler so Core actions and all
	// renderer windows share one set of host counter deltas.
	telemetrySvc, _ := core.ServiceFor[*telemetry.Service](s.opts.Core, "telemetry")
	if telemetrySvc == nil {
		telemetrySvc = telemetry.NewService(telemetry.Options{})
	}
	// desktopstate — Core-owned, Medium-backed inner-shell and Terminal
	// workspace documents. Bind the registered instance rather than a sibling.
	desktopStateSvc, _ := core.ServiceFor[*desktopstate.Service](s.opts.Core, "desktopstate")
	// coding/deploys — Coding role deploy history catalogue. Core-registered;
	// reads and writes Trix-style markdown from ~/Lethean/deploys/. v1 scope:
	// List / Get / Create. Wails: Deploys.List / Get / Create.
	deploysSvc, _ := core.ServiceFor[*deploys.Service](s.opts.Core, "coding-deploys")
	// serverkey — Stage B first-run auth-gate. Core-registered + Bootstrap()ed
	// from cmd/lthn/app.go::newAppCore so the in-memory key + verifier are
	// live by the time the WebView mounts <lthn-auth-gate> and calls
	// ServerKey.AccountStatus() / ServerKey.IssueBootstrapToken().
	serverkeySvc, _ := core.ServiceFor[*serverkey.Service](s.opts.Core, "serverkey")
	// account — Stage B' account-creation handler. Core-registered so
	// pkg/server's RoutesProvider auto-discovery mounts /v1/account/create
	// on the public engine alongside the bootstrap-auth middleware that
	// gates it. Wails-bound here only to reserve the binding namespace —
	// the REST endpoint is the canonical Stage C consumer per RFC §2.5.
	accountSvc, _ := core.ServiceFor[*account.Service](s.opts.Core, "account")
	// agents — CoreAgent (lthn-agent) client. Core-registered in
	// cmd/lthn/app.go via core.WithName("agents", agents.Register); looked
	// up + Wails-bound here so the binding generator emits @desktop/agents
	// and the Agents view's Dispatch panel can call Agents.Dispatch().
	agentsSvc, _ := core.ServiceFor[*agents.Service](s.opts.Core, "agents")
	// Bridge opencode-serve's /global/event SSE stream → Wails event
	// bus. The opencode side runs the SSE goroutine + parses; each
	// event JSON is forwarded here, where emitCoreEvent ferries it
	// onto the same bus the Fleet window already subscribes to.
	// CLI/serve modes leave this unset so the SSE connections never
	// open (no consumer = wasted work). See RFC.opencode.md §5.3.
	if opencodeSvc != nil {
		opencodeSvc.SetEventEmitter(func(eventJSON string) {
			emitCoreEvent(s.opts.Core, "lthn:opencode:event", eventJSON)
		})
	}

	// Benchmark — runner-agnostic results substrate. Constructed +
	// registered here so the Wails binding has a fully-wired *Service
	// with s.core set before any RegisterBencher / History call.
	//
	// Bencher adapters self-register against this Service. Ollama
	// auto-registers against the default localhost endpoint — if the
	// daemon isn't running, CanBench soft-fails and Bench reports a
	// clean HTTP error; the bencher stays inert but visible in the
	// picker so users see the "ollama isn't running" affordance. The
	// openaibench adapter (#1769) ships as a library — users add
	// endpoints via Settings → Integrations (separate ticket); each
	// configured endpoint gets a distinct registered Bencher.
	//
	// Future: pkg/runner Bencher (lthn-mlx, #1768) lands once the
	// runner subsystem is benchmark-ready; pure-subprocess llama-bench
	// (separate ticket) for power users who don't run llama-server.
	benchmarkSvc := benchmark.NewService(benchmark.Options{})
	if r := benchmarkSvc.Register(s.opts.Core); !r.OK {
		core.Warn("desktop.benchmark.register", "error", r.Error())
	}
	if r := benchmarkSvc.RegisterBencher(ollama.NewBencher(ollama.Options{})); !r.OK {
		core.Warn("desktop.benchmark.ollama_register", "error", r.Error())
	}
	// lthn-mlx local bencher (the #1768 runner bencher) — shells
	// `lthn-mlx bench --json` via pkg/calibrate so the admin Benchmark
	// table shows real local prefill/decode/memory rows instead of
	// fixtures. Power omitted (bench carries no watts; joules is a
	// deferred driver-profile concern).
	if r := benchmarkSvc.RegisterBencher(calibrate.NewBencher(s.opts.Core)); !r.OK {
		core.Warn("desktop.benchmark.calibrate_register", "error", r.Error())
	}
	// Queue substrate wiring (Mantis #1770) — "benchmark.gpu" kind
	// handler registered against pkg/queue so EnqueueBench from the
	// Wails surface dispatches via the substrate's single-worker
	// throttle. Concurrent Run clicks queue cleanly instead of
	// contending for GPU. Completion event ferries the BenchCompleted
	// Core action through to the WebView so benchmark-window can
	// refresh History without polling.
	if r := benchmark.RegisterQueueHandler(s.opts.Core, benchmarkSvc); !r.OK {
		core.Warn("desktop.benchmark.queue_register", "error", r.Error())
	}
	benchmark.SubscribeCompleted(s.opts.Core, func(_ *core.Core, ev benchmark.BenchCompleted) {
		emitCoreEvent(s.opts.Core, "benchmark:completed", ev)
	})
	// OpenAI-compat endpoints (Mantis #1775) — restore user-configured
	// endpoints from pkg/keys-backed store + bind the Wails Settings
	// surface. RegisterPersistedEndpoints reads tier-1 records on
	// every boot so the bencher picker reflects the operator's last
	// configuration without a Settings round-trip. The WailsService
	// (added to wailsServices below) lets the Settings UI add/remove
	// endpoints at runtime.
	if r := openaibench.RegisterPersistedEndpoints(s.opts.Core, benchmarkSvc); !r.OK {
		core.Warn("desktop.openaibench.boot_register", "error", r.Error())
	}
	openaibenchSvc := openaibench.NewWailsService(s.opts.Core, benchmarkSvc)

	// Downloader → Wails event bus. The downloader spawns its fetch
	// via c.Go and reports progress + terminal state through this
	// emitter; the WebView listens for "downloader:progress" +
	// "downloader:done" off the same bus the Fleet window already
	// uses.
	downloaderSvc := downloader.NewWailsService(s.opts.Core)
	downloaderSvc.SetEmitter(func(name string, data any) {
		emitCoreEvent(s.opts.Core, name, data)
	})

	// Marketplace → Wails event bus. Every Install/Launch/Stop/Uninstall
	// fires a BundleChanged on Core's ACTION bus; this subscriber ferries
	// it to the WebView so <lthn-marketplace-window> can update status
	// pills without polling ListInstalled on a timer.
	// Frontend listens on "marketplace:bundle:changed".
	marketplace.Subscribe(s.opts.Core, func(_ *core.Core, ev marketplace.BundleChanged) {
		emitCoreEvent(s.opts.Core, "marketplace:bundle:changed", ev)
	})

	// ContentShield — non-LLM text scoring tier (sycophancy, grammar
	// imprint, differential, authority). Pure deterministic, runs
	// in-process, scores chat input / AI response / training-data
	// chunks / opencode session output / plugin output. Register the
	// Core action surface (bridge / CLI / MCP callers) AND the
	// WailsService for the WebView's typed bindings.
	if r := contentshield.Register(s.opts.Core); !r.OK {
		core.Warn("desktop.contentshield.register", "error", r.Error())
	}

	wailsBindings := []gui.Binding{
		// In-this-repo packages — each ships its own *WailsService /
		// *Service with Wails3 lifecycle + (T, error) methods. Bindings
		// land at frontend/bindings/dappco.re/lthn/desktop/pkg/<pkg>/.
		gui.Bind(s.opts.Runner),
		gui.Bind(s.opts.Server),
		gui.Bind(sessions.NewWailsService(s.opts.Core)),
		gui.Bind(downloaderSvc),
		gui.Bind(firstlaunch.NewWailsService()),
		gui.Bind(integrations.NewWailsService()),
		gui.Bind(apikey.NewWailsService(s.opts.Core)),
		gui.Bind(git.NewService(s.opts.Core)),
		gui.Bind(terminal.NewService(s.opts.Core)),
		gui.Bind(build.NewService(s.opts.Core)),
		gui.Bind(container.NewService(s.opts.Core)),
		gui.Bind(lint.NewService(s.opts.Core)),
		gui.Bind(marketplace.NewService(s.opts.Core)),
		gui.Bind(lthnphp.NewService(s.opts.Core)),
		gui.Bind(pluginSvc),
		gui.Bind(sandboxSvc),
		gui.Bind(contentshield.NewWailsService()),
		// ModelRuntime is the sole renderer-facing LEM surface. Trusted
		// Go-side tray and fleet reconciliation below may still use the
		// bounded lemma.Admin protocol client, but the WebView receives no
		// native model paths, endpoint details, or credentials.
		gui.Bind(modelruntime.NewWailsService(modelRuntimeSvc)),
		// calibrate → lthn-mlx profiling CLI (discover / bench / auto-tune).
		// Sibling to lemma: lemma is the HTTP admin client to a *running*
		// serve; calibrate shells the one-shot profiling subcommands for
		// the "Calibrate this machine" flow.
		gui.Bind(calibrate.NewService(s.opts.Core)),
		gui.Bind(clbpl.NewWailsService(clbpl.Options{})),
		gui.Bind(r1.NewWailsService()),
		gui.Bind(r1analytics.NewWailsService()),
		gui.Bind(seeds.NewWailsService()),
		gui.Bind(training.NewWailsService(s.opts.Core, training.NewService(s.opts.Core, training.Options{}))),
		gui.Bind(opencode.NewWailsService(opencodeSvc)),
		gui.Bind(reposSvc),
		// tasks → Shape (a.i) IPC-entry wrapper (RFC v3.1 §4.4 /
		// Cerberus #73 F-1 / Mantis #1755). The wrapper stamps
		// TierRenderer at every Wails IPC entry so the substrate
		// *Service.Require gate fires correctly; the bare *Service
		// stays available via Substrate() for in-Go consumers.
		gui.Bind(tasks.NewWailsService(tasks.NewService(s.opts.Core))),
		// vi → Shape (a.i) IPC-entry wrapper (RFC v3.1 §4.4 /
		// Cerberus #72 F-3 / Mantis #1750). The wrapper stamps
		// TierRenderer at every Wails IPC entry so the substrate's
		// Require gate on the four read methods (Catalogue / Sites /
		// Repos / Activity) fires correctly. OnStart / OnStop live on
		// the substrate ONLY — the wrapper deliberately omits them so
		// wails3 cannot bind those container-lifecycle hooks to the
		// renderer. The Core-registered *vi.Service instance (driving
		// the probe loop via OnStart) is still the one wrapped here;
		// Substrate() exposes it to the composition root unchanged.
		gui.Bind(vi.NewWailsService(viSvc)),
		gui.Bind(incidentsSvc),
		gui.Bind(runbooksSvc),
		gui.Bind(contactsSvc),
		gui.Bind(dealsSvc),
		gui.Bind(pipelineSvc),
		gui.Bind(forecastSvc),
		gui.Bind(campaignsSvc),
		gui.Bind(contentSvc),
		gui.Bind(socialSvc),
		gui.Bind(audienceSvc),
		gui.Bind(analyticsSvc),
		gui.Bind(documentsSvc),
		gui.Bind(mailSvc),
		gui.Bind(filesSvc),
		gui.Bind(desktopstate.NewWailsService(desktopStateSvc)),
		gui.Bind(permissions.NewWailsService(permissionSvc)),
		gui.Bind(deploysSvc),
		gui.Bind(serverkeySvc),
		gui.Bind(accountSvc),
		gui.Bind(agentsSvc),
		gui.Bind(s.opts.Fleet),
		gui.Bind(s.opts.Keys),
		gui.Bind(tools.NewWailsService(s.opts.Core)),
		gui.Bind(validator.NewWailsService()),
		gui.Bind(telemetrySvc),
		gui.Bind(benchmarkSvc),
		gui.Bind(openaibenchSvc),
		gui.Bind(lthnservices.NewWailsService(servicesSvc)),
		// appconfig is the curated, typed settings bridge. It validates
		// user-facing desktop controls before delegating persistence to
		// the registered config service without exposing raw provider APIs.
		gui.Bind(appconfigSvc),
		// Window registry — see note above.
		gui.Bind(windowSvc),
	}

	// Auto-register the local machine in the Fleet substrate. The UI
	// renders the "You" pill off is_self=true, but nobody calls
	// UpsertMachine in production today — the row is silently absent
	// from Fleet → Machines. One-shot insert here populates the
	// hostname / arch / loopback endpoint / inference capability so
	// the user sees their own machine alongside any paired remotes.
	// Idempotent: ON CONFLICT (id) DO UPDATE re-syncs hostname /
	// updated_at on every boot without duplicating rows.
	if s.opts.Fleet != nil {
		if r := s.opts.Fleet.UpsertMachine(selfMachineRow()); !r.OK {
			core.Warn("desktop.fleet.self_upsert", "error", r.Error())
		}
		// Kick a refresh in a goroutine immediately so the row's
		// Status reflects Lemma reachability within ~3s (the admin
		// timeout) rather than waiting 10s for the first ticker
		// fire — the initial selfMachineRow() always returns
		// status="online" which is wrong if Lemma is actually down.
		// Backgrounded so a Lemma timeout doesn't block boot.
		go refreshSelfMachineOnce(s.opts.Fleet)
		// Keep the self row's Model + Status live — every 10s, pull
		// Lemma.Status and re-upsert. When the user hot-swaps a model
		// via the model-browser, the Fleet row reflects the change
		// without waiting for the next boot. Stopped via PostShutdown
		// closing selfRefreshStop so the goroutine doesn't outlive
		// the process.
		s.selfRefreshStop = make(chan struct{})
		go runSelfMachineRefresh(s.opts.Fleet, s.selfRefreshStop)
		// Bring up the local crew sidecars the self machine is capable of
		// (inference -> lthn-ai, sandbox -> lthn-agent hub). One-shot at
		// boot — deliberately NOT on the 10s refresh, which would
		// kill+respawn the crew every tick. The supervisor adopts an
		// already-running sidecar, else spawns + health-gates + respawns
		// it in its own goroutines; torn down by Fleet.ServiceShutdown.
		// Backgrounded so a slow sidecar can't stall boot.
		//
		// The hub requires MCP_JWT_SECRET (JWT signing key) and
		// MCP_AUTH_TOKEN (per-request bearer for the MCP plane).  Both
		// are machine-level secrets resolved from pkg/keys tier-0 so
		// they persist across restarts without requiring the user to
		// unlock an account. We read them here, before backgrounding,
		// so the goroutine captures stable values rather than racing
		// against a concurrent SetKEKProvider call.
		sandboxEnv := hubSandboxEnv(s.opts.Core)
		mcpToken := hubMCPToken(sandboxEnv)
		go func() {
			if r := s.opts.Fleet.SuperviseLocalCrew(core.Background(), sandboxEnv); !r.OK {
				core.Warn("desktop.fleet.crew_supervise", "error", r.Error())
			}
			// The hub's MCP plane is coming up on :9202 — start the
			// Agents channel listener so the view gets CoreAgent push
			// events (agent.blocked, …) live. Reconnects until the hub
			// answers; the bearer token is required for the fail-closed
			// MCP transport (MCP_AUTH_TOKEN).
			if a, _ := core.ServiceFor[*agents.Service](s.opts.Core, "agents"); a != nil {
				a.StartChannels(s.opts.Core, mcpToken)
			}
		}()
	}

	// Compute window state path under ~/Lethean/conf/. Without this
	// the path defaults to $DIR_CONFIG which lthn-desktop doesn't set,
	// dropping state under the binary's cwd.
	windowStatePath := ""
	if confR := paths.ConfDir(); confR.OK {
		windowStatePath = core.PathJoin(confR.Value.(string), "window_state.json")
	}

	// Build the tray menu — static items, plugin entries (if any),
	// then the trailing About + Quit. Click routing lives in the
	// RegisterAction handler below; this slice is purely declarative.
	trayMenuItems := []gui.TrayItem{
		{Label: "Open Lethean Desktop", ActionID: trayActionOpenApp},
		{Type: "separator"},
		{Label: "Open Chat…", ActionID: trayActionOpenChat},
		{Label: "Models…", ActionID: trayActionOpenModels},
		{Label: "Tools…", ActionID: trayActionOpenApps},
		{Label: "Settings…", ActionID: trayActionOpenSettings},
	}
	if pluginSvc != nil {
		entriesR := pluginSvc.Menus()
		if entriesR.OK {
			entries, _ := entriesR.Value.([]plugin.MenuEntry)
			pluginItems := buildPluginTrayItems(entries)
			if len(pluginItems) > 0 {
				trayMenuItems = append(trayMenuItems, gui.TrayItem{Type: "separator"})
				trayMenuItems = append(trayMenuItems, pluginItems...)
			}
		}
	}
	trayMenuItems = append(trayMenuItems,
		gui.TrayItem{Type: "separator"},
		gui.TrayItem{Label: "About lthn", ActionID: trayActionOpenAbout},
		gui.TrayItem{Type: "separator"},
		gui.TrayItem{Label: "Quit lthn", ActionID: trayActionQuit},
	)
	trayTooltip := desktopConfigString(
		configSvc,
		"desktop.tray.tooltip",
		"Lethean Desktop",
	)
	trayLabel := desktopConfigString(configSvc, "desktop.tray.label", "")
	var singleInstanceOptions *gui.SingleInstanceOptions
	if singleInstanceEnabled {
		singleInstanceOptions = &gui.SingleInstanceOptions{
			UniqueID:      "ai.lthn.desktop",
			EncryptionKey: singleInstanceKey,
			AdditionalData: map[string]string{
				"app":     "lthn-desktop",
				"version": lthn.Version,
			},
			OnSecondInstanceLaunch: func(d gui.SecondInstanceData) {
				handleSecondInstanceLaunch(s.opts.Core, d)
			},
		}
	}
	// Build the GuiConfig — core/gui's Service owns wails app construction
	// + sub-service registration. lthn/desktop holds no wails imports.
	guiCfg := gui.GuiConfig{
		// Tray mode auto-wires Mac.ActivationPolicy=Accessory +
		// Windows.DisableQuitOnLastWindowClosed=true. The NSStatusItem
		// remains the process lifetime anchor.
		Mode:            gui.ModeTray,
		Name:            s.opts.Name,
		Description:     s.opts.Description,
		Icon:            s.opts.AppIcon,
		Bindings:        wailsBindings,
		WindowRegistry:  windowRegistry(configSvc),
		WindowStatePath: windowStatePath,
		Tray: &gui.TrayConfig{
			Icon:         s.opts.TrayIcon,
			IconTemplate: true, // darwin template icon; ignored elsewhere
			Tooltip:      trayTooltip,
			Label:        trayLabel,
			Menu:         trayMenuItems,
			// The panel is pre-created hidden in windowRegistry. Wails
			// positions and toggles it under the status item while the
			// right-click menu remains independently available.
			PopoverWindow:  trayPanelWindowName,
			PopoverOffsetY: 4,
			Routes: []gui.TrayRoute{
				{ActionID: trayActionQuit, Quit: true},
			},
		},
		Keybindings: []gui.Keybinding{
			{Accelerator: "Cmd+J", Description: "lthn:popover", EventName: "lthn:key:popover"},
			{Accelerator: "Ctrl+J", Description: "lthn:popover", EventName: "lthn:key:popover"},
			{Accelerator: "Cmd+N", Description: "lthn:new-session", EventName: "lthn:key:new-session"},
			{Accelerator: "Ctrl+N", Description: "lthn:new-session", EventName: "lthn:key:new-session"},
			{Accelerator: "Cmd+K", Description: "lthn:command", EventName: "lthn:key:command"},
			{Accelerator: "Ctrl+K", Description: "lthn:command", EventName: "lthn:key:command"},
			{Accelerator: "Cmd+,", Description: "lthn:settings", EventName: "lthn:key:settings"},
			{Accelerator: "Ctrl+,", Description: "lthn:settings", EventName: "lthn:key:settings"},
			{Accelerator: "Cmd+Shift+M", Description: "lthn:models", EventName: "lthn:key:models"},
			{Accelerator: "Ctrl+Shift+M", Description: "lthn:models", EventName: "lthn:key:models"},
			{Accelerator: "Cmd+/", Description: "lthn:help", EventName: "lthn:key:help"},
			{Accelerator: "Ctrl+/", Description: "lthn:help", EventName: "lthn:key:help"},
			{Accelerator: "Escape", Description: "lthn:dismiss", EventName: "lthn:key:dismiss"},
		},
		ContextMenus: []gui.ContextMenu{
			{
				Name:            "lthn-message",
				EventTemplate:   "lthn:context:{menu}:{action}",
				MenuPrefixStrip: "lthn-",
				Items: []gui.ContextMenuItem{
					{Label: "Copy", ActionID: "copy"},
					{Label: "Regenerate", ActionID: "regenerate"},
					{Label: "Edit", ActionID: "edit"},
					{Type: "separator"},
					{Label: "Delete", ActionID: "delete"},
				},
			},
			{
				Name:            "lthn-input",
				EventTemplate:   "lthn:context:{menu}:{action}",
				MenuPrefixStrip: "lthn-",
				Items: []gui.ContextMenuItem{
					{Label: "Cut", ActionID: "cut"},
					{Label: "Copy", ActionID: "copy"},
					{Label: "Paste", ActionID: "paste"},
					{Type: "separator"},
					{Label: "Select All", ActionID: "selectall"},
				},
			},
			{
				Name:            "lthn-model",
				EventTemplate:   "lthn:context:{menu}:{action}",
				MenuPrefixStrip: "lthn-",
				Items: []gui.ContextMenuItem{
					{Label: "Reveal in Finder", ActionID: "reveal"},
					{Label: "Model Info", ActionID: "info"},
					{Type: "separator"},
					{Label: "Remove...", ActionID: "remove"},
				},
			},
			{
				Name:            "lthn-route",
				EventTemplate:   "lthn:context:{menu}:{action}",
				MenuPrefixStrip: "lthn-",
				Items: []gui.ContextMenuItem{
					{Label: "Edit", ActionID: "edit"},
					{Label: "Test Connection", ActionID: "test"},
					{Type: "separator"},
					{Label: "Disable", ActionID: "disable"},
					{Label: "Remove...", ActionID: "remove"},
				},
			},
		},
		Assets: gui.AssetOptions{
			Handler:    engine,
			Middleware: gui.WailsHTTPMiddleware(engine),
		},
		// Mac defaults wired by gui.ModeTray (ActivationPolicy=Accessory,
		// terminate-after-last-window stays false).
		AppMenu: []gui.MenuItem{
			{Role: &gui.RoleAppMenu},
			{Role: &gui.RoleEditMenu},
			{Role: &gui.RoleWindowMenu},
		},
		Windows: gui.WindowsOptions{
			// DisableQuitOnLastWindowClosed wired by gui.ModeTray.
			// EnabledFeatures stays explicit — Wails3 needs the
			// WebView2 draggable-regions feature for --wails-draggable
			// CSS to work on Windows.
			EnabledFeatures: []string{"msWebView2EnableDraggableRegions"},
		},
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
		// real injection risk.
		//
		// Cerberus #1442: the key is per-install, generated once on
		// first launch and persisted at ~/Lethean/data/keys/
		// single-instance.aead by pkg/keys.
		SingleInstance: singleInstanceOptions,
		ShouldQuit:     func() bool { return true },
		OnShutdown: func() {
			emitCoreEvent(s.opts.Core, "lthn:app:shutdown", nil)
		},
		PostShutdown: func() {
			if s.selfRefreshStop != nil {
				close(s.selfRefreshStop)
				s.selfRefreshStop = nil
			}
			if s.trayStatusStop != nil {
				close(s.trayStatusStop)
				s.trayStatusStop = nil
			}
			if s.opts.Server != nil {
				if r := s.opts.Server.Stop(core.Background()); !r.OK {
					core.Warn("desktop server shutdown failed", "err", r.Error())
				}
			}
		},
		OnPanic: func(d gui.PanicDetails) {
			errStr := ""
			if d.Error != nil {
				errStr = d.Error.Error()
			}
			emitCoreEvent(s.opts.Core, "lthn:app:panic", map[string]any{
				"error":      errStr,
				"stack":      d.StackTrace,
				"full_stack": d.FullStackTrace,
			})
		},
	}

	// Keep core/gui's typed configuration service in the registry so its
	// OpenWindow and WindowBindingService helpers can resolve the window
	// catalogue. The local runtime constructs the same Wails/CoreGUI stack
	// with the connection manager's custom transport attached.
	guiSvcResult := gui.NewService(guiCfg)(s.opts.Core)
	if !guiSvcResult.OK {
		return guiSvcResult
	}
	s.gui = guiSvcResult.Value.(*gui.Service)
	if r := s.opts.Core.RegisterService("gui", s.gui); !r.OK {
		return r
	}
	runtimeResult := newGUIRuntime(s.opts.Core, guiCfg, s.opts.Connection.Transport())
	if !runtimeResult.OK {
		return runtimeResult
	}
	s.runtime = runtimeResult.Value.(*guiRuntime)
	registerRuntimeSystemEvents(s.opts.Core, s.runtime.App())

	// Window state path is now wired via GuiConfig.WindowStatePath
	// above. Pre-creation of the registered windows is owned by
	// gui.Service.OnStartup — no post-hoc StateManager.SetPath call
	// needed.

	// Attach the constructed app to services that need app refs
	// post-construction (the Wails App reference isn't available
	// pre-application.New()). Today only WindowService still
	// depends on this — env / clipboard / screen / browser / dialog
	// previously wrapped here are now consumed by the frontend
	// directly from @wailsio/runtime. gui.WindowBindingService is bound
	// to s.opts.Core at construction (above) — no post-app wiring needed.

	// Application menu is now declarative via GuiConfig.AppMenu above —
	// gui.Service auto-gates to darwin and fires menu.set_app_menu.

	// Systray icon + tooltip + menu + panel-window attachment are now
	// declared via gui.GuiConfig.Tray (built above) and applied by
	// gui.Service.OnStartup. The lthn-specific click router supplies
	// meaningful navigation targets and validates plugin codes.
	s.opts.Core.RegisterAction(func(c *core.Core, msg core.Message) core.Result {
		click, ok := msg.(guisystray.ActionTrayMenuItemClicked)
		if !ok {
			return core.Ok(nil)
		}
		if click.ActionID == trayActionOpenAbout {
			if s.gui != nil && s.gui.App() != nil && s.gui.App().Menu != nil {
				s.gui.App().Menu.ShowAbout()
			}
			return core.Ok(nil)
		}
		target, routed := trayTargetForAction(click.ActionID)
		if !routed {
			return core.Ok(nil)
		}
		result := openTrayTarget(c, target)
		if result.OK && core.HasPrefix(target, "plugin:") {
			emitTrayPluginClicked(core.TrimPrefix(target, "plugin:"))
		}
		return result
	})

	// Context menus + key bindings now declared via GuiConfig.ContextMenus
	// and GuiConfig.Keybindings above — gui.Service registers each and
	// installs the relay handlers that emit lthn:context:* + lthn:key:*
	// events with the right context data.

	// System event re-broadcasts. Wails' ApplicationStarted /
	// OpenedWithFile is resolved to an opaque lthn:host:intent while
	// LaunchedWithUrl remains on the allowlisted lthn:app:* path.
	// See sysevents.go for the table.
	registerSystemEvents(s.opts.Core)
	registerAppconfigEvents(s.opts.Core)
	registerFilesEvents(s.opts.Core)
	registerServicesEvents(s.opts.Core)
	registerModelRuntimeEvents(s.opts.Core)

	// Per-window lthn:window:* event re-broadcasts (ready / focus /
	// blur / hide / show / resize). File drops use lthn:host:intent.
	// Window registry pre-creation + compact-panel tray attachment are
	// owned by gui.Service via GuiConfig.WindowRegistry + GuiConfig.Tray.

	// The tray panel is the normal launch surface. Development and other
	// explicit callers may request the full OS shell immediately with
	// ShowAppOnLaunch. This must fire AFTER ApplicationStarted —
	// pre-Run() window operations on macOS SEGV inside AppKit because
	// the NSApp run loop isn't up yet.
	s.opts.Core.RegisterAction(func(c *core.Core, msg core.Message) core.Result {
		if _, ok := msg.(guilifecycle.ActionApplicationStarted); ok && s.opts.ShowAppOnLaunch {
			gui.OpenWindow(c, mainWindowName)
		}
		return core.Ok(nil)
	})

	// Live tray-tooltip status. The tray icon is the at-a-glance
	// surface; the tooltip reflects the currently-loaded Lemma model
	// + ready state without the user opening any window. An immediate
	// backgrounded poll fires the first read within the admin timeout
	// (~3s) so the tooltip reflects truth shortly after launch rather
	// than waiting a full 30s tick. The ticker keeps it live as the
	// user hot-swaps models. Stopped via PostShutdown closing
	// trayStatusStop so the goroutine doesn't outlive the process.
	go refreshTrayTooltipOnce(s.opts.Core)
	s.trayStatusStop = make(chan struct{})
	go runTrayStatusRefresh(s.opts.Core, s.trayStatusStop)

	return s.runtime.Run()
}

// runTrayStatusRefresh keeps the native tray tooltip in sync with the
// currently-loaded Lemma model on a 30s ticker. Coarser than a chat
// surface needs — the tooltip is at-a-glance, not real-time — but
// frequent enough to catch a model hot-swap or an engine stop/start.
// Exits when stop closes (PostShutdown).
//
//	s.trayStatusStop = make(chan struct{})
//	go runTrayStatusRefresh(s.opts.Core, s.trayStatusStop)
func runTrayStatusRefresh(c *core.Core, stop <-chan struct{}) {
	tick := core.NewTicker(30 * core.Second)
	defer tick.Stop()
	for {
		select {
		case <-stop:
			return
		case <-tick.C:
			refreshTrayTooltipOnce(c)
		}
	}
}

// refreshTrayTooltipOnce performs one Lemma.Status read + pushes the
// result to the native tray tooltip via the systray.set_tooltip action.
// Independent of the ticker so the boot-time first-fire (and tests) can
// drive a single iteration.
//
// Lemma up with a model loaded → tooltip = configured tray prefix +
// basename(model_path) + " — ready". Lemma unreachable, errored, or no
// model loaded → configured prefix + " — no model loaded". Errors are
// debug-level only — no panic, no user-visible noise (a menu-bar utility
// shouldn't surface engine transients).
func refreshTrayTooltipOnce(c *core.Core) {
	if c == nil {
		return
	}
	configSvc, _ := core.ServiceFor[*config.Service](c, "config")
	prefix := desktopConfigString(
		configSvc,
		"desktop.tray.tooltip",
		"Lethean Desktop",
	)
	tooltip := trayStatusTooltip(prefix, "")
	admin, err := lemma.NewAdmin(lemma.AdminConfig{})
	if err != nil {
		core.Debug("desktop.tray.status", "err", err.Error())
	} else {
		ctx, cancel := core.WithTimeout(core.Background(), 3*core.Second)
		defer cancel()
		status, statusErr := admin.Status(ctx)
		if statusErr != nil {
			core.Debug("desktop.tray.status", "err", statusErr.Error())
		} else if base := pathBase(status.ModelPath); base != "" {
			tooltip = trayStatusTooltip(prefix, base)
		}
	}
	c.Action("systray.set_tooltip").Run(core.Background(), core.NewOptions(
		core.Option{Key: "task", Value: guisystray.TaskSetTrayTooltip{Tooltip: tooltip}},
	))
}

func trayStatusTooltip(prefix string, model string) string {
	prefix = core.Trim(prefix)
	model = core.Trim(model)
	if model == "" {
		if prefix == "" {
			return "no model loaded"
		}
		return prefix + " — no model loaded"
	}
	if prefix == "" {
		return model + " — ready"
	}
	return prefix + " — " + model + " — ready"
}

// restoreSecondInstanceWindow brings a window forward in response to a
// second-instance launch. It restores the one registered Angular OS
// shell, then falls through to a window.open create-and-show attempt if
// the shell is not yet reachable (for example, during a pre-create
// race). Always emits an EventDesktopSecondInstanceFallback audit row
// when the fallback engages.
//
// Cerberus #70 F-4 LOW (STRIDE-R Repudiation / defence-in-depth):
// the audit row proves the fallback branch ran even if window.open
// itself fails downstream.
func restoreSecondInstanceWindow(c *core.Core) {
	if c == nil {
		return
	}
	if gui.OpenWindow(c, mainWindowName) {
		return
	}
	emitSecondInstanceFallback()
	if spec, ok := gui.WindowSpec(c, mainWindowName); ok {
		// gui.Service has already pre-created it hidden; this just shows
		// + focuses by re-running window.open with Hidden=false.
		opened := *spec
		opened.Hidden = false
		c.Action("window.open").Run(core.Background(), core.NewOptions(
			core.Option{Key: "task", Value: guiwindow.TaskOpenWindow{Window: &opened}},
		))
	}
}

// handleSecondInstanceLaunch re-broadcasts the authenticated hand-off and
// routes the first lthn:// argument through the same deep-link path as an
// operating-system URL event. Launches without a deep link retain the existing
// restore-and-focus behaviour.
func handleSecondInstanceLaunch(c *core.Core, data gui.SecondInstanceData) {
	if c == nil {
		return
	}
	emitCoreEvent(c, "lthn:app:second-instance", map[string]any{
		"args":       data.Args,
		"workdir":    data.WorkingDir,
		"additional": data.AdditionalData,
	})

	for _, argument := range data.Args {
		candidate := core.Trim(argument)
		if core.HasPrefix(core.Lower(candidate), "lthn://") {
			if handled := handleDeepLink(c, candidate); handled.OK {
				return
			} else {
				core.Warn(
					"desktop second-instance deep link ignored",
					"err",
					handled.Error(),
				)
				break
			}
		}
		if core.Lower(core.PathExt(candidate)) == ".lthn" {
			restoreSecondInstanceWindow(c)
			c.ACTION(guilifecycle.ActionOpenedWithFile{Path: candidate})
			return
		}
	}
	restoreSecondInstanceWindow(c)
}

// attachSPA mounts the frontend as the coreapi.Engine's no-route fallback.
// Anything that doesn't match an explicit lthn / subsystem route is proxied
// to Angular during Wails development or served from the embedded dist
// otherwise. The handler inherits the canonical middleware chain (auth,
// sunset, cache, tracing) just like any other route.
func (s *Service) attachSPA() core.Result {
	sr := core.Sub(s.opts.Frontend, s.opts.FrontendRoot)
	if !sr.OK {
		return core.Fail(core.E("desktop.attachSPA", "frontend root not found", sr.Value.(error)))
	}
	sub := sr.Value.(core.FS)
	fileServer := frontendAssetHandler(sub)
	s.opts.Server.Engine().SetNoRoute(func(c *gin.Context) {
		fileServer.ServeHTTP(c.Writer, c.Request)
	})
	return core.Ok(nil)
}

// trayTargetForAction translates native menu ActionIDs into the semantic
// target consumed by Angular's lthn:tray:open listener. Plugin codes are
// validated at this native boundary before they become event data.
func trayTargetForAction(actionID string) (string, bool) {
	switch actionID {
	case trayActionOpenApp:
		return "desktop", true
	case trayActionOpenChat:
		return "chat", true
	case trayActionOpenModels:
		return "models", true
	case trayActionOpenSettings:
		return "settings", true
	case trayActionOpenApps:
		return "tools", true
	}
	if !core.HasPrefix(actionID, trayPluginPrefix) {
		return "", false
	}
	code := core.TrimPrefix(actionID, trayPluginPrefix)
	if code == "" || !paths.IsValidPluginCode(code) {
		return "", false
	}
	return "plugin:" + code, true
}

// openTrayTarget brings the full desktop forward, emits the semantic Angular
// navigation target, and dismisses the compact popover. Errors use the Core
// error shape because this helper runs on the Core action bus.
func openTrayTarget(c *core.Core, target string) core.Result {
	if c == nil {
		return core.Fail(core.E("desktop.openTrayTarget", "core is nil", nil))
	}
	if !validTrayTarget(target) {
		return core.Fail(core.E("desktop.openTrayTarget", "invalid tray target", nil))
	}
	if !gui.OpenWindow(c, mainWindowName) {
		return core.Fail(core.E("desktop.openTrayTarget", "desktop window is unavailable", nil))
	}
	emitCoreEvent(c, trayOpenEvent, target)
	gui.HideWindow(c, trayPanelWindowName)
	return core.Ok(nil)
}

func validTrayTarget(target string) bool {
	switch target {
	case "desktop", "chat", "models", "settings", "telemetry", "tools":
		return true
	}
	if !core.HasPrefix(target, "plugin:") {
		return false
	}
	code := core.TrimPrefix(target, "plugin:")
	return code != "" && paths.IsValidPluginCode(code)
}

// trayPluginMaxLabelBytes caps a plugin-manifest label before it lands
// on the native tray surface. Closes Cerberus #70 F-3 MED — STRIDE-T
// Tampering. A hostile manifest could otherwise ship a 1MB label that
// drags tray rendering or hides downstream menu items off-screen. 64
// bytes is generous for human-readable labels (matches the plugin
// code's MaxPluginCodeBytes ceiling) while keeping the worst-case row
// width bounded.
const trayPluginMaxLabelBytes = 64

// trayPluginStoppedSuffix is the textual tag appended to a plugin
// label when the supervisor reports the plugin not running. The
// label-cap above is applied to the BASE label (pre-suffix) so the
// tag is always visible — a 64-byte attacker label cannot crowd out
// the operator's "stopped" affordance.
const trayPluginStoppedSuffix = " · stopped"

// buildPluginTrayItems translates pkg/plugin.MenuEntry rows into the
// guisystray.TrayMenuItem shape, filtering invalid plugin codes,
// stripping control characters, and capping the label byte length.
// Pure function — testable without an active NSApp / Wails loop.
//
// Filter ordering (Cerberus #70 F-3 / paths.IsValidPluginCode is the
// authoritative validator):
//
//  1. paths.IsValidPluginCode(e.Code) — drops entries whose code
//     contains path separators, leading dot/dash, NUL, or any byte
//     outside the bounded vocabulary. A hostile manifest cannot
//     surface "../etc" / "/bin/sh" / "code\x00sneak" to the tray.
//  2. sanitizePluginLabel(e.Label) — strips ASCII control bytes
//     (< 0x20 and 0x7F) so a label cannot inject newline/CR/NUL
//     into the rendered menu surface; then byte-caps to
//     trayPluginMaxLabelBytes.
//  3. The "· stopped" suffix is appended AFTER the cap so the
//     operator-facing affordance is never trimmed away by a
//     pathological label.
//
// Usage example:
//
//	items := buildPluginTrayItems([]plugin.MenuEntry{
//	    {Code: "opencode", Label: "OpenCode", Running: true},
//	    {Code: "../etc",   Label: "evil",     Running: true}, // dropped
//	})
//	// items has one entry — the "../etc" row was rejected.
func buildPluginTrayItems(entries []plugin.MenuEntry) []guisystray.TrayMenuItem {
	if len(entries) == 0 {
		return nil
	}
	out := make([]guisystray.TrayMenuItem, 0, len(entries))
	for _, e := range entries {
		if !paths.IsValidPluginCode(e.Code) {
			core.Warn("desktop tray skipping plugin entry — invalid code",
				"code_len", len(e.Code))
			continue
		}
		label := sanitizePluginLabel(e.Label)
		if !e.Running {
			label = label + trayPluginStoppedSuffix
		}
		out = append(out, guisystray.TrayMenuItem{
			Label:    label,
			ActionID: trayPluginPrefix + e.Code,
		})
	}
	return out
}

// sanitizePluginLabel strips ASCII control bytes from a label and
// caps the byte length at trayPluginMaxLabelBytes. Control bytes
// (< 0x20 except for SPACE 0x20, plus DEL 0x7F) are removed because
// they can scramble the native menu renderer (newlines splitting a
// row, NUL truncating downstream items on some platforms). TAB is
// also stripped — tray surfaces are single-line.
//
// Returns the sanitised + capped label. Caller-controlled label
// bytes never reach the native tray API; this is the boundary.
//
// Usage example:
//
//	sanitizePluginLabel("OpenCode")                // → "OpenCode"
//	sanitizePluginLabel("evil\x00\nrow")           // → "evilrow"
//	sanitizePluginLabel(strings.Repeat("x", 200))  // → 64-byte "xxxx…x"
func sanitizePluginLabel(label string) string {
	if label == "" {
		return ""
	}
	// First pass — strip control chars in O(n).
	buf := make([]byte, 0, len(label))
	for i := 0; i < len(label); i++ {
		b := label[i]
		if b < 0x20 || b == 0x7F {
			continue
		}
		buf = append(buf, b)
	}
	// Byte-cap (post-strip). Byte-level cap is intentional: the
	// label is shipped to the native tray API which counts bytes
	// not runes, and a multi-byte UTF-8 char truncated mid-sequence
	// is the operator's chrome problem (not a security one). Take
	// the prefix and let the renderer cope.
	if len(buf) > trayPluginMaxLabelBytes {
		buf = buf[:trayPluginMaxLabelBytes]
	}
	return string(buf)
}

// trayScope is the Event.Scope literal stamped on tray-rooted audit
// rows. Mirrors the per-package scope convention across pkg/vi /
// pkg/sessions / pkg/sandbox — keeps the chip-filter in
// <lthn-audit-viewer> grouping tray surfaces under a single facet.
const trayScope = "tray"

// desktopScope is the Event.Scope literal stamped on desktop-rooted
// audit rows — second-instance fallback, future composition rows that
// don't belong under the tray facet. Mirrors trayScope above; keeps
// the chip-filter in <lthn-audit-viewer> grouping pkg/desktop emits
// under a single facet distinct from tray clicks.
const desktopScope = "desktop"

// emitSecondInstanceFallback fires the Cerberus #70 F-4 audit row
// when the OnSecondInstanceLaunch handler falls through to the
// window.open create-and-show path because the unified Angular app
// shell was not registered or reachable. Single event — the row
// records "the handler reached the degraded UX branch". The
// SecondInstanceData payload
// (Args / WorkingDir / AdditionalData) is NEVER recorded — those bytes
// are caller-controlled and would defeat the bounded-keyspace promise
// of the audit substrate (sibling discipline to the plugin-label drop
// on EventTrayPluginClicked + the URL bytes drop on
// EventViPRFetchRejected). The fallback path runs the window.open
// regardless of whether the audit emit succeeded — and the audit emit
// runs regardless of whether the window.open succeeds downstream —
// because forensic observability is the load-bearing value-add per
// the Cerberus #70 F-4 recommendation.
//
// Usage example (internal):
//
//	emitSecondInstanceFallback()
func emitSecondInstanceFallback() {
	_ = audit.Default().Record(audit.Event{
		Event:   audit.EventDesktopSecondInstanceFallback,
		TS:      core.Now().UTC().Unix(),
		Scope:   desktopScope,
		Outcome: audit.OutcomeOK,
		Meta: map[string]any{
			"primary_targets": mainWindowName,
			"fallback_target": mainWindowName,
			"fallback_via":    "window.open",
		},
	})
}

// emitTrayPluginClicked fires the Cerberus #70 F-3 audit row at the
// tray click router AFTER paths.IsValidPluginCode has accepted the
// resolved code. Single event — the row records "the operator opened
// plugin X via the tray"; the plugin manifest label is NEVER recorded
// (label bytes are attacker-controlled; the plugin code is bounded
// vocab + 1:1 against the installed-plugins manifest catalogue so a
// walker can resolve the open-event back to a concrete plugin).
//
// Usage example (internal):
//
//	emitTrayPluginClicked(code)
func emitTrayPluginClicked(code string) {
	_ = audit.Default().Record(audit.Event{
		Event:   audit.EventTrayPluginClicked,
		TS:      core.Now().UTC().Unix(),
		Scope:   trayScope,
		Outcome: audit.OutcomeOK,
		Meta: map[string]any{
			"plugin_code":  code,
			"resolved_via": "tray_menu",
		},
	})
}

// selfMachineRow builds the fleet.Machine entry that represents this
// instance. Called once per boot from Run() so the user sees their own
// machine listed in Fleet → Machines alongside any paired remotes.
//
// ID is derived from hostname so reruns update the same row rather
// than creating duplicates; if the hostname changes (rename), the
// previous row stays as a stale entry until the user removes it.
//
// Capabilities defaults to inference — lthn-mlx is the engine intent
// even when not currently running; the row's status reflects the
// engine reachability, capabilities reflect what the machine CAN do.
//
// Host/Port are the loopback inference endpoint convention (127.0.0.1
// :9100 — the lthn-ai host) matching pkg/lemma defaults. Future
// remote-tunnelled installs replace these when pairing.
// runSelfMachineRefresh keeps the local-machine fleet row in sync
// with the currently-loaded model. Tick interval is 10s — same
// order of magnitude as the tray poll (2s) but coarser since Fleet
// is the "across-machines view" rather than the at-a-glance status.
// Exits when stop closes.
//
// Lemma down → Model field clears, Status flips to "offline" so
// the Fleet → Machines row visibly reflects "engine not running";
// Lemma up → Model = basename(model_path), Status = "online".
func runSelfMachineRefresh(svc *fleet.Service, stop <-chan struct{}) {
	tick := core.NewTicker(10 * core.Second)
	defer tick.Stop()
	for {
		select {
		case <-stop:
			return
		case <-tick.C:
			refreshSelfMachineOnce(svc)
		}
	}
}

// refreshSelfMachineOnce performs one Lemma.Status read + upsert.
// Independent of the ticker so callers (tests / first-fire post-
// boot) can drive a single iteration. Errors are warnings — the
// row stays as it was, no panic, no silent drift.
func refreshSelfMachineOnce(svc *fleet.Service) {
	if svc == nil {
		return
	}
	row := mergeSelfMachineRow(svc)
	admin, err := lemma.NewAdmin(lemma.AdminConfig{})
	if err != nil {
		row.Status = "offline"
		if r := svc.UpsertMachine(row); !r.OK {
			core.Warn("desktop.fleet.self_refresh", "error", r.Error())
		}
		return
	}
	ctx, cancel := core.WithTimeout(core.Background(), 3*core.Second)
	defer cancel()
	status, statusErr := admin.Status(ctx)
	if statusErr != nil {
		row.Status = "offline"
	} else {
		row.Status = "online"
		if base := pathBase(status.ModelPath); base != "" {
			row.Model = base
		}
	}
	if r := svc.UpsertMachine(row); !r.OK {
		core.Warn("desktop.fleet.self_refresh", "error", r.Error())
	}
	// Only auto-create the Local Lemma agent when reachable. The first
	// time the user starts lthn-mlx, the agent appears in Fleet →
	// Agents already-configured + ready to use. Subsequent ticks keep
	// it in sync (model name / status); never delete — if Lemma drops
	// later the row stays so the user can re-enable rather than
	// re-configure from scratch.
	//
	// Read existing agents first so user-edited fields (Persona,
	// ModelSettings, Tags) survive the re-upsert. Without this, every
	// 10s tick would wipe whatever persona/temperature the user set
	// via Configure Agent.
	if statusErr == nil {
		var existing *fleet.Agent
		if listRes := svc.Agents(); listRes.OK {
			if agents, ok := listRes.Value.([]fleet.Agent); ok {
				for i := range agents {
					if agents[i].ID == "local-lemma" {
						existing = &agents[i]
						break
					}
				}
			}
		}
		if r := svc.UpsertAgent(localLemmaAgentRow(status.ModelPath, existing)); !r.OK {
			core.Warn("desktop.fleet.local_agent_refresh", "error", r.Error())
		}
	}
}

// pathBase strips dir + returns the trailing path component.
// Used to render "lemer-lite-4bit" instead of the full absolute
// path in the Fleet row's Model field.
func pathBase(p string) string {
	if p == "" {
		return ""
	}
	trimmed := p
	for len(trimmed) > 1 && trimmed[len(trimmed)-1] == '/' {
		trimmed = trimmed[:len(trimmed)-1]
	}
	for i := len(trimmed) - 1; i >= 0; i-- {
		if trimmed[i] == '/' {
			return trimmed[i+1:]
		}
	}
	return trimmed
}

// localLemmaAgentRow builds the fleet.Agent entry for the local
// lthn-mlx engine. ID is fixed ("local-lemma") so refresh ticks
// update the same row. Provider matches the catalogue entry id in
// configure-agent-modal.ts so the modal renders the right field set
// when the user clicks edit. Empty APIKeyRef because the loopback
// admin endpoint is auth-by-process-identity, not by token.
//
// modelPath comes from Lemma.Status — basename'd for display.
// Empty when no model is loaded yet (engine up but pre-load).
//
// existing is the current row (if any) — carries forward the
// user-controlled fields (Persona, ModelSettings, Tags, Name) so
// the 10s refresh ticker doesn't wipe edits made via the Configure
// Agent modal. Substrate-controlled fields (Model, Status,
// Provider, Kind, BaseURL) always overwrite — they reflect engine
// truth, not user choice.
func localLemmaAgentRow(modelPath string, existing *fleet.Agent) fleet.Agent {
	model := pathBase(modelPath)
	out := fleet.Agent{
		ID:       "local-lemma",
		Name:     "Local Lemma",
		Provider: "lemma-local",
		Kind:     "local",
		BaseURL:  "http://127.0.0.1:9100/v1",
		Model:    model,
		Status:   "online",
	}
	if existing != nil {
		if existing.Name != "" {
			out.Name = existing.Name
		}
		out.Persona = existing.Persona
		out.ModelSettings = existing.ModelSettings
		out.Tags = existing.Tags
		out.Features = existing.Features
	}
	return out
}

// mergeSelfMachineRow reads the existing self-machine row (if any)
// and merges substrate-controlled fields (the fresh selfMachineRow
// defaults) with user-controlled fields (Tags, Capabilities, Name —
// preserve). Without this every 10s refresh tick wipes whatever
// custom capabilities or tags the operator added via the pair-
// machine modal.
//
// Mirrors the localLemmaAgentRow merge pattern for the same reason:
// auto-managed rows shouldn't fight user edits.
func mergeSelfMachineRow(svc *fleet.Service) fleet.Machine {
	out := selfMachineRow()
	if svc == nil {
		return out
	}
	listRes := svc.Machines()
	if !listRes.OK {
		return out
	}
	machines, ok := listRes.Value.([]fleet.Machine)
	if !ok {
		return out
	}
	for i := range machines {
		if !machines[i].IsSelf {
			continue
		}
		// First IsSelf row wins. The refresh upsert keys on ID so
		// the merged shape is what lands.
		existing := machines[i]
		if existing.Name != "" {
			out.Name = existing.Name
		}
		out.Tags = existing.Tags
		if len(existing.Capabilities) > 0 {
			out.Capabilities = existing.Capabilities
		}
		break
	}
	return out
}

func selfMachineRow() fleet.Machine {
	host := "127.0.0.1"
	name := host
	if r := core.Hostname(); r.OK {
		if h, ok := r.Value.(string); ok && core.Trim(h) != "" {
			name = h
		}
	}
	return fleet.Machine{
		ID:           "self:" + name,
		Name:         name,
		Arch:         runtime.GOOS + "/" + runtime.GOARCH,
		Host:         host,
		Port:         9100,
		Status:       "online",
		IsSelf:       true,
		Capabilities: []string{fleet.CapabilityInference, fleet.CapabilitySandbox},
	}
}

// hubSandboxEnv resolves the two secrets the lthn-agent hub requires at
// spawn time and returns them as KEY=VALUE env pairs for the crew member.
// Both secrets are stored in pkg/keys tier-0 (machine-level, pre-unlock)
// so they survive process restarts without user interaction.
//
//   - MCP_JWT_SECRET: HMAC signing key for the hub's JWT mint+verify path.
//   - MCP_AUTH_TOKEN: per-request bearer credential checked by the hub's
//     fail-closed MCP HTTP+SSE transport.
//
// When the keys service is unavailable or tier-0 is not yet wired, the
// function falls back to fresh random hex values (logged as warnings) so
// the hub still starts — the secrets will differ from run to run until
// the keys substrate is live, but that is preferable to a hard boot
// failure on first install.
//
//	env := hubSandboxEnv(c)
//	// ["MCP_JWT_SECRET=...", "MCP_AUTH_TOKEN=..."]
func hubSandboxEnv(c *core.Core) []string {
	const (
		jwtSecretRef = "hub-mcp-jwt-secret"
		authTokenRef = "hub-mcp-auth-token"
		envJWTSecret = "MCP_JWT_SECRET"
		envAuthToken = "MCP_AUTH_TOKEN"
	)
	generate := func() ([]byte, error) {
		rr := core.RandomBytes(32)
		if !rr.OK {
			return nil, rr.Value.(error)
		}
		return []byte(core.HexEncode(rr.Value.([]byte))), nil
	}

	resolve := func(ref string) string {
		if c != nil {
			if ks, _ := core.ServiceFor[*keys.Service](c, "keys"); ks != nil {
				if r := ks.GetOrCreateTier0(ref, generate); r.OK {
					return core.Trim(string(r.Value.([]byte)))
				}
				core.Warn("desktop.hub: tier-0 key resolve failed for " + ref + ", using ephemeral value")
			}
		}
		// Fallback: ephemeral random value (no persistence).
		rr := core.RandomBytes(32)
		if !rr.OK {
			return ""
		}
		return core.HexEncode(rr.Value.([]byte))
	}

	jwtSecret := resolve(jwtSecretRef)
	authToken := resolve(authTokenRef)
	env := []string{}
	if jwtSecret != "" {
		env = append(env, envJWTSecret+"="+jwtSecret)
	}
	if authToken != "" {
		env = append(env, envAuthToken+"="+authToken)
	}
	return env
}

// hubMCPToken extracts the MCP_AUTH_TOKEN value from a sandboxEnv slice
// returned by hubSandboxEnv so the channel listener can send it as bearer.
//
//	env := hubSandboxEnv(c)
//	token := hubMCPToken(env)
func hubMCPToken(env []string) string {
	const prefix = "MCP_AUTH_TOKEN="
	for _, kv := range env {
		if core.HasPrefix(kv, prefix) {
			return kv[len(prefix):]
		}
	}
	return ""
}
