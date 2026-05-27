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
	"runtime"

	core "dappco.re/go"
	"dappco.re/go/ai/pkg/lab"
	"dappco.re/go/config"
	guilifecycle "dappco.re/go/gui/pkg/lifecycle"
	guimenu "dappco.re/go/gui/pkg/menu"
	guisystray "dappco.re/go/gui/pkg/systray"
	guiwindow "dappco.re/go/gui/pkg/window"
	coreI18n "dappco.re/go/i18n"
	lthn "dappco.re/lthn/desktop"
	"dappco.re/lthn/desktop/pkg/account"
	"dappco.re/lthn/desktop/pkg/apikey"
	"dappco.re/lthn/desktop/pkg/audit"
	"dappco.re/lthn/desktop/pkg/benchmark"
	"dappco.re/lthn/desktop/pkg/bridge"
	"dappco.re/lthn/desktop/pkg/clbpl"
	"dappco.re/lthn/desktop/pkg/contentshield"
	"dappco.re/lthn/desktop/pkg/ollama"
	"dappco.re/lthn/desktop/pkg/openaibench"
	"dappco.re/lthn/desktop/pkg/build"
	"dappco.re/lthn/desktop/pkg/container"
	"dappco.re/lthn/desktop/pkg/downloader"
	"dappco.re/lthn/desktop/pkg/firstlaunch"
	"dappco.re/lthn/desktop/pkg/git"
	"dappco.re/lthn/desktop/pkg/integrations"
	"dappco.re/lthn/desktop/pkg/lemma"
	"dappco.re/lthn/desktop/pkg/lint"
	"dappco.re/lthn/desktop/pkg/marketplace"
	"dappco.re/lthn/desktop/pkg/models"
	lthnphp "dappco.re/lthn/desktop/pkg/php"
	"dappco.re/lthn/desktop/pkg/plugin"
	"dappco.re/lthn/desktop/pkg/repos"
	"dappco.re/lthn/desktop/pkg/fleet"
	"dappco.re/lthn/desktop/pkg/keys"
	"dappco.re/lthn/desktop/pkg/paths"
	"dappco.re/lthn/desktop/pkg/r1"
	r1analytics "dappco.re/lthn/desktop/pkg/r1/analytics"
	"dappco.re/lthn/desktop/pkg/runner"
	"dappco.re/lthn/desktop/pkg/seeds"
	"dappco.re/lthn/desktop/pkg/opencode"
	"dappco.re/lthn/desktop/pkg/sandbox"
	"dappco.re/lthn/desktop/pkg/server"
	"dappco.re/lthn/desktop/pkg/serverkey"
	"dappco.re/lthn/desktop/pkg/tasks"
	"dappco.re/lthn/desktop/pkg/training"
	lthnservices "dappco.re/lthn/desktop/pkg/services"
	"dappco.re/lthn/desktop/pkg/sessions"
	"dappco.re/lthn/desktop/pkg/telemetry"
	"dappco.re/lthn/desktop/pkg/tools"
	"dappco.re/lthn/desktop/pkg/validator"
	"dappco.re/lthn/desktop/pkg/vi"
	"dappco.re/lthn/desktop/pkg/incidents"
	"dappco.re/lthn/desktop/pkg/runbooks"
	"dappco.re/lthn/desktop/pkg/sales/contacts"
	"dappco.re/lthn/desktop/pkg/sales/deals"
	"dappco.re/lthn/desktop/pkg/sales/forecast"
	"dappco.re/lthn/desktop/pkg/sales/pipeline"
	"dappco.re/lthn/desktop/pkg/marketing/campaigns"
	"dappco.re/lthn/desktop/pkg/marketing/content"
	"dappco.re/lthn/desktop/pkg/marketing/social"
	"dappco.re/lthn/desktop/pkg/marketing/audience"
	"dappco.re/lthn/desktop/pkg/marketing/analytics"
	"dappco.re/lthn/desktop/pkg/office/documents"
	"dappco.re/lthn/desktop/pkg/office/mail"
	officefile "dappco.re/lthn/desktop/pkg/office/files"
	"dappco.re/lthn/desktop/pkg/deploys"
	"github.com/gin-gonic/gin"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// TODO(snider): core/gui needs an app-construction surface covering
// application.Options, service binding registration, single-instance
// callbacks, native systray bootstrap, tray-window options, and panic/
// shutdown hooks. Until that exists, desktop.Run is the lthn-side Wails
// boundary and all other packages route GUI behavior through CoreGUI.

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
	// Frontend is the embedded Vite build (cmd/lthn/embed.go). The
	// service serves it via a NoRoute fallback on the gin engine.
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
	// TrayIcon is the light-mode systray icon bytes. Empty leaves the
	// platform default tray glyph unchanged.
	TrayIcon []byte
	// AppIcon is the application icon shown in the default About box
	// (application.Options.Icon). Empty = Wails-default 'W'. macOS
	// Dock / Launchpad uses build/darwin/icons.icns from the .app
	// bundle separately; both should derive from the same source PNG
	// to stay visually consistent.
	AppIcon []byte
	// ShowAppOnLaunch opens the main "app" window automatically after
	// pre-create finishes. Useful in dev mode so iteration doesn't
	// require clicking the tray every restart. Production builds
	// leave it false — the tray IS the process; the user clicks in.
	// Toggled in main.go from the LTHN_DEV env var.
	ShowAppOnLaunch bool
}

// Service holds the Wails application and the SystemTray anchor.
type Service struct {
	opts Options
	app  *application.App
	// selfRefreshStop signals the local-machine refresh ticker to
	// exit. Closed by PostShutdown; the goroutine selects on it
	// alongside the ticker channel.
	selfRefreshStop chan struct{}
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

	// Resolve the per-install SingleInstance encryption key. Generated
	// once on first launch, persisted at ~/Lethean/data/keys/
	// single-instance.aead, reloaded on every subsequent boot.
	// Cerberus #1442: replaces the build-time constant that was shared
	// across every installed binary on every machine.
	var singleInstanceKey [32]byte
	if s.opts.Keys != nil {
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
	// CoreGUI owns native dock/taskbar and notification services. Wails
	// remains its private substrate; lthn routes those behaviours through
	// Core actions after registerCoreGUI wires the framework.
	attachDock(s.opts.Core)
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
	// office/files — Office filesystem browser. Core-registered; read-only v1.
	filesSvc, _ := core.ServiceFor[*officefile.Service](s.opts.Core, "office-files")
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
	// lab — ML Lab Workbench coordinator. Core-registered in app.go
	// via core.WithName("lab", lab.Register); looked up here so the
	// Wails Services array can bind it, and so mountSubsystems can
	// reach it when wiring /v1/ml-lab/* HTTP routes. Per
	// plans/project/lthn/desktop/RFC.ml-lab.md §3.
	labSvc, _ := core.ServiceFor[*lab.Service](s.opts.Core, "lab")
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

	wailsServices := []application.Service{
		// In-this-repo packages — each ships its own *WailsService /
		// *Service with Wails3 lifecycle + (T, error) methods. Bindings
		// land at frontend/bindings/dappco.re/lthn/desktop/pkg/<pkg>/.
		application.NewService(s.opts.Runner),
		application.NewService(s.opts.Server),
		application.NewService(sessions.NewWailsService(s.opts.Core)),
		application.NewService(models.NewWailsService()),
		application.NewService(downloaderSvc),
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
		application.NewService(contentshield.NewWailsService()),
		// lemma → admin facade on lthn-mlx /v1/admin/* (status / reload /
		// download / profiles). Exposes the Lemma surface to the WebView
		// without leaking the Bearer token to JS — the service runs in-Go
		// with read access to ~/Lethean/data/admin.token; JS only sees
		// the typed verb signatures Wails generates from this struct.
		application.NewService(lemma.NewWailsService(lemma.AdminConfig{})),
		application.NewService(clbpl.NewWailsService(clbpl.Options{})),
		application.NewService(r1.NewWailsService()),
		application.NewService(r1analytics.NewWailsService()),
		application.NewService(seeds.NewWailsService()),
		application.NewService(training.NewWailsService(s.opts.Core, training.NewService(s.opts.Core, training.Options{}))),
		application.NewService(labSvc),
		application.NewService(opencode.NewWailsService(opencodeSvc)),
		application.NewService(reposSvc),
		// tasks → Shape (a.i) IPC-entry wrapper (RFC v3.1 §4.4 /
		// Cerberus #73 F-1 / Mantis #1755). The wrapper stamps
		// TierRenderer at every Wails IPC entry so the substrate
		// *Service.Require gate fires correctly; the bare *Service
		// stays available via Substrate() for in-Go consumers.
		application.NewService(tasks.NewWailsService(tasks.NewService(s.opts.Core))),
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
		application.NewService(vi.NewWailsService(viSvc)),
		application.NewService(incidentsSvc),
		application.NewService(runbooksSvc),
		application.NewService(contactsSvc),
		application.NewService(dealsSvc),
		application.NewService(pipelineSvc),
		application.NewService(forecastSvc),
		application.NewService(campaignsSvc),
		application.NewService(contentSvc),
		application.NewService(socialSvc),
		application.NewService(audienceSvc),
		application.NewService(analyticsSvc),
		application.NewService(documentsSvc),
		application.NewService(mailSvc),
		application.NewService(filesSvc),
		application.NewService(deploysSvc),
		application.NewService(serverkeySvc),
		application.NewService(accountSvc),
		application.NewService(s.opts.Fleet),
		application.NewService(s.opts.Keys),
		application.NewService(tools.NewWailsService(s.opts.Core)),
		application.NewService(validator.NewWailsService()),
		application.NewService(telemetry.NewService(telemetry.Options{})),
		application.NewService(benchmarkSvc),
		application.NewService(openaibenchSvc),
		application.NewService(lthnservices.NewWailsService()),
		// Upstream dappco.re/go services — register the Core-built
		// instances directly. Bindings land at frontend/bindings/
		// dappco.re/go/<pkg>/.
		application.NewService(i18nSvc),
		application.NewService(configSvc),
		application.NewService(bridgeSvc),
		// Window registry — see note above.
		application.NewService(windowSvc),
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
		// real injection risk.
		//
		// Cerberus #1442: the key is per-install, generated once on
		// first launch and persisted at ~/Lethean/data/keys/
		// single-instance.aead by pkg/keys. Every machine gets a
		// distinct random 32-byte key; the build-time constant that
		// was identical across every installed binary has been removed.
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID:      "io.lethean.desktop",
			EncryptionKey: singleInstanceKey,
			AdditionalData: map[string]string{
				"app": "lthn-desktop",
				// Pulled from the canonical build-stamped Version in
				// dappco.re/lthn/desktop — the prior hardcoded
				// "0.2.0-rc1" literal would have lied to the
				// second-instance receiver about which build
				// launched, breaking telemetry + the forensic
				// audit trail whenever versions drift.
				"version": lthn.Version,
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
				emitCoreEvent(s.opts.Core,"lthn:app:second-instance", map[string]any{
					"args":       d.Args,
					"workdir":    d.WorkingDir,
					"additional": d.AdditionalData,
				})
				// Bring the unified app shell (preferred) or the
				// tray popover back to the foreground. Restore()
				// before Focus() handles the case where the window
				// was minimised — Wails docs canon for the second-
				// instance UX. If neither window is registered (race
				// during pre-create / both destroyed), fall through
				// to a window.open create-and-show on the tray as
				// last-resort UX + emit the fallback audit row so a
				// forensic walker can grep for the degraded path
				// (Cerberus #70 F-4 LOW).
				restoreSecondInstanceWindow(s.opts.Core, s.opts)
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
			emitCoreEvent(s.opts.Core,"lthn:app:shutdown", nil)
		},
		// PostShutdown runs after the Wails event loop has fully
		// stopped. Last chance to close anything that held a ref
		// into the event loop (HTTP server, store, runner).
		PostShutdown: func() {
			if s.selfRefreshStop != nil {
				close(s.selfRefreshStop)
				s.selfRefreshStop = nil
			}
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
			emitCoreEvent(s.opts.Core,"lthn:app:panic", map[string]any{
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

	if r := s.registerCoreGUI(); !r.OK {
		return r
	}

	// Point the window service's StateManager at ~/Lethean/conf/
	// window_state.json so per-window position/size/maximised auto-
	// persist across restarts (debounced 500ms inside the manager).
	// Without this the path defaults to $DIR_CONFIG which lthn-desktop
	// doesn't set, dropping state under the binary's cwd. Failure
	// non-fatal — windows just won't remember their slot.
	if confR := paths.ConfDir(); confR.OK {
		if winSvc, ok := core.ServiceFor[*guiwindow.Service](s.opts.Core, "window"); ok {
			winSvc.Manager().State().SetPath(
				core.PathJoin(confR.Value.(string), "window_state.json"),
			)
		}
	}

	// Attach the constructed app to services that need app refs
	// post-construction (the Wails App reference isn't available
	// pre-application.New()). Today only WindowService still
	// depends on this — env / clipboard / screen / browser / dialog
	// previously wrapped here are now consumed by the frontend
	// directly from @wailsio/runtime.
	windowSvc.app = s.opts.Core

	// TODO(snider): core/gui needs notification response/action callbacks
	// so native notification body clicks and action buttons can be
	// forwarded as "lthn:notification:response" without lthn importing
	// Wails notification services directly.

	// Application menu — macOS-only. Accessory apps still get a
	// menubar when their windows are focused, and standard roles
	// give us Cmd+Q / Cmd+W / Cmd+M / Cmd+H / Edit menu shortcuts
	// for free. Without AddRole(AppMenu) we'd lose those.
	if runtime.GOOS == "darwin" {
		appRole := guimenu.RoleAppMenu
		editRole := guimenu.RoleEditMenu
		windowRole := guimenu.RoleWindowMenu
		s.opts.Core.Action("menu.set_app_menu").Run(core.Background(), core.NewOptions(
			core.Option{Key: "task", Value: guimenu.TaskSetAppMenu{Items: []guimenu.MenuItem{
				{Role: &appRole},
				{Role: &editRole},
				{Role: &windowRole},
			}}},
		))
	}

	// Systray icon + tooltip + menu — driven entirely through core/gui
	// actions. registerCoreGUI has already created the underlying tray
	// (gui.systray.OnStartup runs Setup with a default icon and "Core"
	// tooltip); these calls replace those defaults with the lthn-branded
	// surface. The previous direct s.app.SystemTray.New() created a
	// SECOND tray, leaving two icons in the macOS menu bar — fixed
	// 2026-05-14.
	if s.opts.TrayIcon != nil {
		iconAction := "systray.set_icon"
		iconTask := any(guisystray.TaskSetTrayIcon{Data: s.opts.TrayIcon})
		if runtime.GOOS == "darwin" {
			iconAction = "systray.set_template_icon"
			iconTask = guisystray.TaskSetTrayTemplateIcon{Data: s.opts.TrayIcon}
		}
		s.opts.Core.Action(iconAction).Run(core.Background(), core.NewOptions(
			core.Option{Key: "task", Value: iconTask},
		))
	}
	// macOS renders SetTooltip as the menu-bar title text next to the
	// icon — there is no separate hover-tooltip surface. Tray is
	// icon-only, so clear it on darwin; keep it as a real tooltip on
	// other platforms.
	trayTooltip := "Lethean Desktop"
	if runtime.GOOS == "darwin" {
		trayTooltip = ""
	}
	s.opts.Core.Action("systray.set_tooltip").Run(core.Background(), core.NewOptions(
		core.Option{Key: "task", Value: guisystray.TaskSetTrayTooltip{Tooltip: trayTooltip}},
	))
	// Clear the menu-bar label — core/gui's Setup defaults it to "Core".
	s.opts.Core.Action("systray.set_label").Run(core.Background(), core.NewOptions(
		core.Option{Key: "task", Value: guisystray.TaskSetTrayLabel{Label: ""}},
	))

	// Tray menu — built as a []TrayMenuItem with ActionIDs. Click
	// routing lives in the RegisterAction handler below; this keeps
	// the menu declaration declarative + survives the action-bus
	// boundary (closures don't).
	trayMenuItems := []guisystray.TrayMenuItem{
		{Label: "Open Lethean Desktop", ActionID: trayActionOpenApp},
		{Type: "separator"},
		{Label: "Open Chat…", ActionID: trayActionOpenChat},
		{Label: "Models…", ActionID: trayActionOpenModels},
		{Label: "Settings…", ActionID: trayActionOpenSettings},
	}
	if pluginSvc != nil {
		entriesR := pluginSvc.Menus()
		if entriesR.OK {
			entries, _ := entriesR.Value.([]plugin.MenuEntry)
			pluginItems := buildPluginTrayItems(entries)
			if len(pluginItems) > 0 {
				trayMenuItems = append(trayMenuItems, guisystray.TrayMenuItem{Type: "separator"})
				trayMenuItems = append(trayMenuItems, pluginItems...)
			}
		}
	}
	trayMenuItems = append(trayMenuItems,
		guisystray.TrayMenuItem{Type: "separator"},
		guisystray.TrayMenuItem{Label: "About lthn", ActionID: trayActionOpenAbout},
		guisystray.TrayMenuItem{Type: "separator"},
		guisystray.TrayMenuItem{Label: "Quit lthn", ActionID: trayActionQuit},
	)
	s.opts.Core.Action("systray.set_menu").Run(core.Background(), core.NewOptions(
		core.Option{Key: "task", Value: guisystray.TaskSetTrayMenu{Items: trayMenuItems}},
	))

	// Click router — core/gui dispatches ActionTrayMenuItemClicked
	// onto the action bus whenever a tray menu entry is clicked.
	// Switch on ActionID; plugin entries route via the
	// trayPluginPrefix suffix.
	s.opts.Core.RegisterAction(func(_ *core.Core, msg core.Message) core.Result {
		click, ok := msg.(guisystray.ActionTrayMenuItemClicked)
		if !ok {
			return core.Result{OK: true}
		}
		switch click.ActionID {
		case trayActionOpenApp:
			openWindow(s.opts.Core, "app")
			emitCoreEvent(s.opts.Core, trayOpenEvent, "app")
		case trayActionOpenChat:
			openWindow(s.opts.Core, "chat")
			emitCoreEvent(s.opts.Core, trayOpenEvent, "chat")
		case trayActionOpenModels:
			openWindow(s.opts.Core, "models")
			emitCoreEvent(s.opts.Core, trayOpenEvent, "models")
		case trayActionOpenSettings:
			openWindow(s.opts.Core, "settings")
			emitCoreEvent(s.opts.Core, trayOpenEvent, "settings")
		case trayActionOpenAbout:
			openWindow(s.opts.Core, "about")
			emitCoreEvent(s.opts.Core, trayOpenEvent, "about")
		case trayActionQuit:
			s.opts.Core.Action("lifecycle.quit").Run(core.Background(), core.NewOptions(
				core.Option{Key: "task", Value: guilifecycle.TaskQuit{}},
			))
		default:
			if core.HasPrefix(click.ActionID, trayPluginPrefix) {
				code := core.TrimPrefix(click.ActionID, trayPluginPrefix)
				// Re-validate at the click boundary — defence-in-depth
				// against a race between Menus() snapshot at menu-build
				// time and the click landing here (Cerberus #70 F-3).
				// The build-time filter (buildPluginTrayItems) is the
				// primary gate; this one stops a hostile ActionID that
				// somehow bypassed it from reaching openPluginWindow.
				if code != "" && paths.IsValidPluginCode(code) {
					openPluginWindow(s.opts.Core, code)
					emitCoreEvent(s.opts.Core, trayOpenEvent, "plugin:"+code)
					emitTrayPluginClicked(code)
				}
			}
		}
		return core.Result{OK: true}
	})

	// Context menus — right-click surfaces for the chat UI. Lit
	// elements declare `style="--custom-contextmenu: lthn-message"`
	// (etc.) plus `--custom-contextmenu-data: <message-id>` so the
	// click handler knows WHICH message was right-clicked. Each
	// action emits an "lthn:context:<menu>:<action>" event with the
	// data; the originating Lit element dispatches accordingly.
	registerContextMenus(s.opts.Core)

	// Global keyboard shortcuts. Each emits "lthn:key:<verb>" with
	// the active window's name. Cmd+J toggle popover / Cmd+N new
	// session / Cmd+, settings / etc. See keybindings.go.
	registerKeyBindings(s.opts.Core)

	// System event re-broadcasts. Wails' ApplicationStarted /
	// OpenedWithFile / LaunchedWithUrl get republished as lthn:app:*
	// so the frontend has one event-bus contract for everything.
	// See sysevents.go for the table.
	registerSystemEvents(s.opts.Core)

	// Tray popover construction via core/gui. core/gui's window service
	// creates the underlying wails window from this spec; we then look
	// the wails handle up by name to register the close-hide hook
	// (cancel-able window hooks are not yet exposed on the core/gui
	// surface — that's the remaining direct wails seam).
	s.opts.Core.Action("window.open").Run(core.Background(), core.NewOptions(
		core.Option{Key: "task", Value: guiwindow.TaskOpenWindow{Window: &guiwindow.Window{
			Name:                       "tray",
			Title:                      "Lethean Desktop",
			Width:                      400,
			Height:                     560,
			Frameless:                  true,
			AlwaysOnTop:                true,
			Hidden:                     true,
			DisableResize:              true,
			HideOnEscape:               true,
			HideOnFocusLost:            true,
			URL:                        "/?surface=tray",
			BackgroundColour:           [4]uint8{0, 0, 0, 0},
			DefaultContextMenuDisabled: true,
			Mac: guiwindow.MacWindow{
				WindowLevel: guiwindow.MacWindowLevelFloating,
				CollectionBehavior: guiwindow.MacCollectionBehaviorCanJoinAllSpaces |
					guiwindow.MacCollectionBehaviorFullScreenAuxiliary |
					guiwindow.MacCollectionBehaviorIgnoresCycle,
				InvisibleTitleBarHeight: 40,
				DisableBackForwardNav:   true,
			},
			Linux: guiwindow.LinuxWindow{
				Icon: s.opts.AppIcon,
			},
			Windows: guiwindow.WindowsWindow{
				HiddenOnTaskbar: true,
			},
		}}},
	))

	// Close-hides rather than destroys — tray-rooted lifecycle. The
	// popover survives the user clicking its close button so the tray
	// icon stays the canonical entry point. Driven through core/gui's
	// window.set_close_behavior action; no direct wails seam needed.
	s.opts.Core.Action("window.set_close_behavior").Run(core.Background(), core.NewOptions(
		core.Option{Key: "task", Value: guiwindow.TaskSetCloseBehavior{
			Name:     "tray",
			Behavior: guiwindow.CloseBehaviorHide,
		}},
	))

	// Per-window lthn:window:* event re-broadcasts (ready / focus /
	// blur / hide / show / resize / files-dropped). See sysevents.go.
	// Pre-create welcome / chat / models / settings / about windows
	// hidden, so first tray-menu open is instant. See windows.go.
	// Options threaded through for Linux icon + future per-window
	// opts that depend on s.opts (telemetry endpoint, brand, etc.).
	preCreateWindows(s.opts.Core, s.opts)

	s.opts.Core.Action("systray.attach_window").Run(core.Background(), core.NewOptions(
		core.Option{Key: "task", Value: guisystray.TaskAttachWindow{Name: "tray", OffsetY: 5}},
	))

	// First-launch detection — if ~/Lethean/conf/lthn.yaml and the
	// state DB don't exist, open the welcome wizard. The wizard's
	// completeOnboarding step writes config + opens the app shell;
	// firstlaunch.Detect flips fresh→false naturally for subsequent
	// launches.
	freshInstall := false
	if state := firstlaunch.Detect(nil); state.OK {
		if fl, ok := state.Value.(firstlaunch.State); ok && fl.Fresh {
			freshInstall = true
			openWindow(s.opts.Core, "welcome")
		}
	}

	// Non-fresh launches drop the user straight into the app shell —
	// "first run people get the welcome window, then drop into the
	// desktop going forwards". Must fire AFTER ApplicationStarted —
	// pre-Run() window operations on macOS SEGV inside AppKit because
	// the NSApp run loop isn't up yet.
	//
	// Fresh installs skip this branch because the welcome wizard's
	// completeOnboarding step opens "app" itself once the user
	// finishes — opening it both at boot and from the wizard would
	// flash a second shell.
	//
	// ShowAppOnLaunch still applies as an override: when true (dev
	// mode via LTHN_DEV=1) the app shell opens even during a fresh-
	// install session so iteration-mode work doesn't require the
	// wizard each restart.
	if !freshInstall || s.opts.ShowAppOnLaunch {
		s.opts.Core.RegisterAction(func(c *core.Core, msg core.Message) core.Result {
			if _, ok := msg.(guilifecycle.ActionApplicationStarted); ok {
				openWindow(c, "app")
			}
			return core.Ok(nil)
		})
	}

	if err := s.app.Run(); err != nil {
		return core.Fail(err)
	}
	return core.Ok(nil)
}

func restoreFocusedWindow(c *core.Core, name string) bool {
	if windowExists(c, name) {
		openWindow(c, name)
		return true
	}
	return false
}

// restoreSecondInstanceWindow brings a window forward in response to a
// second-instance launch. Tries the unified app shell first, the tray
// popover second, and falls through to a window.open create-and-show on
// the unified app shell if neither is registered (race during pre-create
// or both destroyed). Always emits an EventDesktopSecondInstanceFallback
// audit row when the fallback engages so a forensic walker can grep for
// the degraded UX path even when the create-and-show itself fails.
//
// Fallback target is "app" (Lethean Desktop unified shell) rather than
// "tray": tray is a transient menubar-utility popover and is built
// inline at boot via a hand-spec TaskOpenWindow that pkg/desktop owns
// (not in windowRegistry()), whereas "app" lives in the registry with
// a complete WindowSpec and is the operator-facing primary surface. If
// the operator double-launched the binary the user-visible intent is
// "show me the app", not "rebuild the menubar popover that should
// already exist".
//
// Cerberus #70 F-4 LOW (STRIDE-R Repudiation / defence-in-depth):
// the pre-fix shape silently no-op'd when both restore calls returned
// false — the bus event still fired (renderer consumers acted on the
// SecondInstanceData payload) but no window came forward visibly, and
// the substrate had no row recording that the path engaged. Operator
// observability: a visible window is preferable to a silent dead
// click. Forensic observability: the audit row is the value-add per
// the Cerberus recommendation — even if window.open fails downstream
// the substrate carries proof the handler reached the fallback branch.
//
// Usage example (internal):
//
//	restoreSecondInstanceWindow(s.opts.Core, s.opts)
func restoreSecondInstanceWindow(c *core.Core, opts Options) {
	if c == nil {
		return
	}
	if restoreFocusedWindow(c, "app") {
		return
	}
	if restoreFocusedWindow(c, "tray") {
		return
	}
	emitSecondInstanceFallback()
	if spec, ok := windowSpecByName("app"); ok {
		openWindowSpec(c, spec, opts, false)
	}
}

// windowSpecByName returns the registry entry whose Name matches the
// supplied key. The lookup walks windowRegistry() — small fixed slice
// (single-digit entries today) so linear scan is fine; promoting to a
// map would just hide an O(N) scan that already runs at human latency.
//
// Returns the spec + true on hit, the zero value + false on miss.
//
// Usage example (internal):
//
//	spec, ok := windowSpecByName("tray")
//	if !ok { return }
//	openWindowSpec(c, spec, opts, false)
func windowSpecByName(name string) (WindowSpec, bool) {
	if name == "" {
		return WindowSpec{}, false
	}
	for _, spec := range windowRegistry() {
		if spec.Name == name {
			return spec, true
		}
	}
	return WindowSpec{}, false
}

// attachSPA mounts the embedded frontend as the coreapi.Engine's
// no-route fallback. Anything that doesn't match an explicit lthn /
// subsystem route gets served from the embedded dist — index.html,
// assets/*, etc. The handler inherits the canonical middleware chain
// (auth, sunset, cache, tracing) just like any other route.
func (s *Service) attachSPA() core.Result {
	sr := core.Sub(s.opts.Frontend, s.opts.FrontendRoot)
	if !sr.OK {
		return core.Fail(core.E("desktop.attachSPA", "frontend root not found", sr.Value.(error)))
	}
	sub := sr.Value.(core.FS)
	fileServer := core.HTTPFileServer(core.HTTPFS(sub))
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
func ginMiddleware(engine core.Handler) application.Middleware {
	return func(next core.Handler) core.Handler {
		return core.HandlerFunc(func(w core.ResponseWriter, r *core.Request) {
			if r.URL.Path == "/wails/custom.js" {
				w.Header().Set("Content-Type", "application/javascript")
				w.WriteHeader(core.StatusOK)
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
// window.open create-and-show path because neither the unified app
// shell nor the tray popover was registered (both restoreFocusedWindow
// calls returned false). Single event — the row records "the handler
// reached the degraded UX branch". The SecondInstanceData payload
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
			"primary_targets": "app,tray",
			"fallback_target": "app",
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
// Host/Port are the loopback admin endpoint convention (127.0.0.1
// :11434) matching pkg/lemma defaults. Future remote-tunnelled
// installs replace these when pairing.
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
		BaseURL:  "http://127.0.0.1:11434/v1",
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
		Port:         11434,
		Status:       "online",
		IsSelf:       true,
		Capabilities: []string{fleet.CapabilityInference},
	}
}
