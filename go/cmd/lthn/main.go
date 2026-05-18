// SPDX-Licence-Identifier: EUPL-1.2

// Command lthn — Lethean's unified CLI binary.
//
// Single binary, multiple modes. The CLI router dispatches based on
// subcommand; the Wails GUI is one consumer of that dispatch, not the
// binary's identity.
//
// Usage example:
//
//	lthn                       # default mode (launches tray + GUI when wired)
//	lthn version               # version info
//	lthn gui                   # explicit GUI launch
//	lthn tray                  # tray-only mode (NSStatusItem)
//	lthn serve --port 8000     # HTTP API only, no GUI
//	lthn ai chat               # interactive CLI chat
//	lthn ai generate "prompt"  # one-shot generation
//	lthn ai models ls          # list local models
//	lthn ai models pull NAME   # download from HuggingFace
//	lthn help [subcommand]     # built-in help
//
// Architectural rule: the CLI dispatch is the load-bearing entry. The
// Wails GUI is decoupled — if GUI build is broken, the binary still
// ships via CLI + serve modes. See plans/project/lthn/desktop/
// RFC.first-release.md §1.3.
package main

import (
	core "dappco.re/go"
	coreapi "dappco.re/go/api"
	lthn "dappco.re/lthn/desktop"
	"dappco.re/lthn/desktop/pkg/apikey"
	"dappco.re/lthn/desktop/pkg/desktop"
	"dappco.re/lthn/desktop/pkg/fleet"
	"dappco.re/lthn/desktop/pkg/gateway"
	"dappco.re/lthn/desktop/pkg/keys"
	"dappco.re/lthn/desktop/pkg/marketplace"
	"dappco.re/lthn/desktop/pkg/mdns"
	"dappco.re/lthn/desktop/pkg/opencode"
	"dappco.re/lthn/desktop/pkg/plugin"
	"dappco.re/lthn/desktop/pkg/runner"
	"dappco.re/lthn/desktop/pkg/server"
	"dappco.re/lthn/desktop/pkg/serverkey"
	"golang.org/x/term"
)

func main() {
	args := core.Args()[1:]

	if len(args) == 0 {
		core.Exit(cmdDefault(args))
	}

	switch args[0] {
	case "version", "-v", "--version":
		core.Exit(cmdVersion(args[1:]))
	case "help", "-h", "--help":
		core.Exit(cmdHelp(args[1:]))
	case "gui":
		core.Exit(cmdGUI(args[1:]))
	case "tray":
		core.Exit(cmdTray(args[1:]))
	case "serve":
		core.Exit(cmdServe(args[1:]))
	case "ai":
		core.Exit(cmdAI(args[1:]))
	case "config":
		core.Exit(cmdConfig(args[1:]))
	case "state":
		core.Exit(cmdState(args[1:]))
	case "events":
		core.Exit(cmdEvents(args[1:]))
	case "process":
		core.Exit(cmdProcess(args[1:]))
	case "sessions":
		core.Exit(cmdSessions(args[1:]))
	case "models":
		core.Exit(cmdModels(args[1:]))
	case "validate":
		core.Exit(cmdValidate(args[1:]))
	case "firstlaunch":
		core.Exit(cmdFirstLaunch(args[1:]))
	case "permissions":
		core.Exit(cmdPermissions(args[1:]))
	case "telemetry":
		core.Exit(cmdTelemetry(args[1:]))
	case "service":
		core.Exit(cmdService(args[1:]))
	case "api":
		core.Exit(cmdAPI(args[1:]))
	case "fleet":
		core.Exit(cmdFleet(args[1:]))
	case "opencode":
		core.Exit(cmdOpenCode(args[1:]))
	case "marketplace":
		core.Exit(cmdMarketplace(args[1:]))
	default:
		core.Print(core.Stderr(), "lthn: unknown subcommand %q\nrun `lthn help` for available commands\n", args[0])
		core.Exit(2)
	}
}

// cmdDefault is invoked when `lthn` is run without a subcommand.
// Routes based on stdin TTY:
//
//   - TTY attached (user typed `lthn` in a terminal) → CLI banner
//     pointing at subcommands.
//   - No TTY (double-clicked .app, wails3 dev launch, launchd /
//     systemd / NSSM service) → launch the systray + GUI.
//
// Canonical Unix dual-mode idiom — terminal users get help, GUI
// launchers get GUI. CLI subcommands (serve / ai / config / etc.)
// bypass this check and dispatch directly.
//
// Usage example:
//
//	core.Exit(cmdDefault(core.Args()[1:]))
func cmdDefault(args []string) int {
	stdin, ok := core.Stdin().(*core.OSFile)
	if !ok || !term.IsTerminal(int(stdin.Fd())) {
		return cmdGUI(args)
	}
	// Boot the Core so the welcome banner reads from
	// pkg/i18n/locales/*.json instead of falling back to literal
	// message IDs.
	c := newAppCore()
	if c != nil {
		defer c.ServiceShutdown(core.Background())
	}
	tr := func(key string) string {
		if c == nil {
			return key
		}
		r := c.I18n().Translate(key)
		if !r.OK {
			return key
		}
		value, ok := r.Value.(string)
		if !ok {
			return key
		}
		return value
	}
	core.Println(tr("cli.welcome.title"))
	core.Println("")
	core.Println(tr("cli.welcome.subtitle"))
	core.Println("")
	core.Println("  lthn gui           — " + tr("cli.subcommands.gui"))
	core.Println("  lthn tray          — " + tr("cli.subcommands.tray"))
	core.Println("  lthn serve         — " + tr("cli.subcommands.serve"))
	core.Println("  lthn ai            — " + tr("cli.subcommands.ai"))
	core.Println("  lthn config        — " + tr("cli.subcommands.config"))
	core.Println("  lthn state         — " + tr("cli.subcommands.state"))
	core.Println("  lthn version       — " + tr("cli.subcommands.version"))
	core.Println("  lthn help          — " + tr("cli.subcommands.help"))
	return 0
}

// cmdVersion handles `lthn version` / `lthn -v` / `lthn --version`.
//
// Usage example:
//
//	rc := cmdVersion(nil) // prints "lthn vX.Y.Z" and returns 0
func cmdVersion(args []string) int {
	core.Print(core.Stdout(), "lthn %s\n", cmdVersionLabel())
	return 0
}

func cmdVersionLabel() string {
	if core.HasPrefix(lthn.Version, "v") {
		return lthn.Version
	}
	return "v" + lthn.Version
}

// cmdHelp handles `lthn help [subcommand]`. Built-in help text.
//
// Usage example:
//
//	rc := cmdHelp([]string{"ai"}) // prints `lthn ai` help
func cmdHelp(args []string) int {
	if len(args) == 0 {
		core.Println("lthn — Lethean unified binary")
		core.Println("")
		core.Println("Usage: lthn <subcommand> [args...]")
		core.Println("")
		core.Println("Subcommands:")
		core.Println("  version              Print version information")
		core.Println("  help [subcommand]    Show help for a subcommand")
		core.Println("  gui                  Launch the Wails GUI (tray + popover + windows)")
		core.Println("  tray                 Tray-only mode (NSStatusItem, no popover pre-open)")
		core.Println("  serve [--port PORT]  HTTP API server (OpenAI-compatible)")
		core.Println("  ai <verb> [args...]  AI subsystem — chat, generate, models")
		core.Println("  config <verb>        Config file (get / set / list / commit / path)")
		core.Println("  state <verb>         KV store (get / set / delete / list / groups)")
		core.Println("  events <verb>        Event bus (stats / publish / config / running)")
		core.Println("  process <verb>       Subsystem supervisor (list / get)")
		core.Println("  sessions <verb>      Chat history (create / list / read / append)")
		core.Println("  models <verb>        Local model snapshots (list / pull)")
		core.Println("  validate URL         Probe a remote OpenAI-compat endpoint")
		core.Println("  firstlaunch          Detect fresh-install state (JSON)")
		core.Println("  permissions <verb>   Entitlement checker (check / set / list)")
		core.Println("  telemetry <verb>     Local process metrics (sample)")
		core.Println("  service <verb>       OS daemon lifecycle (install / start / stop / status / list)")
		core.Println("  api <verb>           OpenAPI gateway (spec / sdk LANGUAGE)")
		core.Println("")
		core.Println("Address handler: lthn:// URIs route through the same dispatch")
		core.Println("(see plans/project/lthn/RFC.md §7 — the unified namespace canon)")
		return 0
	}
	switch args[0] {
	case "ai":
		core.Println("lthn ai — AI subsystem")
		core.Println("")
		core.Println("Verbs:")
		core.Println("  chat                       Interactive REPL with the loaded model")
		core.Println("  generate \"prompt\"          One-shot generation")
		core.Println("  models ls                  List local models in ~/Lethean/conf/models/")
		core.Println("  models pull NAME           Download model from HuggingFace")
		core.Println("  serve [--port PORT]        AI HTTP API (alias for `lthn serve`)")
	case "serve":
		core.Println("lthn serve — HTTP API server")
		core.Println("")
		core.Println("Starts the OpenAI-compatible HTTP API on the given port.")
		core.Println("Endpoints: /v1/chat/completions, /v1/completions, /v1/models")
		core.Println("Default port: 8000")
	default:
		core.Print(core.Stderr(), "lthn help: no help for unknown subcommand %q\n", args[0])
		return 2
	}
	return 0
}

// cmdGUI handles `lthn gui`. Launches the Wails app.
// TODO: import core/gui + Lethean-5 Lit frontend, follow core/ide pattern.
//
// Usage example:
//
//	rc := cmdGUI(nil) // launches the GUI when wired; today returns 1
func cmdGUI(args []string) int {
	c := newAppCore()
	if c == nil {
		return 1
	}
	r, _ := core.ServiceFor[*runner.Service](c, "runner")
	if r == nil {
		core.Print(core.Stderr(), "lthn gui: runner service unavailable\n")
		return 1
	}
	keyR := apikey.GenerateOrLoad(c)
	if !keyR.OK {
		core.Print(core.Stderr(), "lthn gui: %s\n", keyR.Error())
		return 1
	}
	key, _ := keyR.Value.(string)
	// Mantis #1741 / Cerberus #70 F-1 — build the same opts shape as
	// cmdServe (gateway routes + opencode wiring + capability checkers)
	// so the GUI mode mounts the data firewall and lifecycle wires
	// uniformly. GUI mode does NOT call s.Start() (the desktop service
	// uses s.Handler() directly), so Addr is left empty here; the
	// difference between GUI and serve is the listen verb, not the
	// surface composition.
	//
	// forGUI=true skips the opencode/plugin ProxyGroup extras —
	// pkg/desktop/subsystems.go.mountSubsystems registers those
	// directly on the engine when the desktop service runs. Without
	// this gate, both registrations land on the same Gin engine and
	// the wildcard at /v1/api/plugin/:code/*proxyPath conflicts with
	// itself, panicking the binary at boot.
	opts := buildServerOpts(c, r, key, true)
	s := server.NewService(opts)
	if rr := c.RegisterService("server", s); !rr.OK {
		core.Print(core.Stderr(), "lthn gui: %s\n", rr.Error())
		return 1
	}
	fleetSvc, _ := core.ServiceFor[*fleet.Service](c, "fleet")
	if fleetSvc == nil {
		core.Print(core.Stderr(), "lthn gui: fleet service unavailable\n")
		return 1
	}
	keysSvc, _ := core.ServiceFor[*keys.Service](c, "keys")
	if keysSvc == nil {
		core.Print(core.Stderr(), "lthn gui: keys service unavailable\n")
		return 1
	}
	d := desktop.NewService(desktop.Options{
		Name:            "lthn",
		Description:     "Lethean Desktop",
		Frontend:        frontendDist,
		Server:          s,
		Core:            c,
		Runner:          r,
		Fleet:           fleetSvc,
		Keys:            keysSvc,
		TrayIcon:        trayIcon,
		AppIcon:         appIcon,
		ShowAppOnLaunch: core.Getenv("LTHN_DEV") == "1",
	})
	if rr := c.RegisterService("desktop", d); !rr.OK {
		core.Print(core.Stderr(), "lthn gui: %s\n", rr.Error())
		return 1
	}
	if rr := d.Run(); !rr.OK {
		core.Print(core.Stderr(), "lthn gui: %s\n", rr.Error())
		return 1
	}
	return 0
}

// cmdTray handles `lthn tray`. NSStatusItem-only, no popover pre-open.
// Today identical to cmdGUI — both launch the Wails app in
// accessory-policy mode with the tray as lifetime anchor. Will
// diverge when cmdGUI gains "open the chat window on launch".
//
// Usage example:
//
//	rc := cmdTray(nil) // tray-only mode
func cmdTray(args []string) int {
	return cmdGUI(args)
}

// cmdServe handles `lthn serve [--port PORT] [--token TOKEN]
// [--cors ORIGINS]`. HTTP API only, no GUI.
//
// Usage example:
//
//	rc := cmdServe([]string{"--port=8000"}) // starts server when wired
func cmdServe(args []string) int {
	const serveErrorFormat = "lthn serve: %s\n"

	port := "8000"
	for i := 0; i < len(args); i++ {
		k, v, valid := core.ParseFlag(args[i])
		if !valid {
			continue
		}
		if k == "port" {
			if v == "" && i+1 < len(args) {
				i++
				v = args[i]
			}
			port = v
		}
	}

	c := newAppCore()
	if c == nil {
		return 1
	}
	r, _ := core.ServiceFor[*runner.Service](c, "runner")
	if r == nil {
		core.Print(core.Stderr(), serveErrorFormat, "runner service unavailable")
		return 1
	}
	keyR := apikey.GenerateOrLoad(c)
	if !keyR.OK {
		core.Print(core.Stderr(), serveErrorFormat, keyR.Error())
		return 1
	}
	key, _ := keyR.Value.(string)
	// Mantis #1741 / Cerberus #70 F-1 — share buildServerOpts with
	// cmdGUI so the gateway data firewall, opencode lifecycle wires
	// and capability checkers mount identically. The serve verb's
	// only delta from GUI is the public listen, expressed below as
	// Addr + s.Start() + mdns broadcast.
	//
	// forGUI=false so extras carries the opencode/plugin ProxyGroup
	// registrations — cmdServe has no desktop service + no
	// mountSubsystems, so the only registration site is the extras
	// path consumed by server.NewService.
	opts := buildServerOpts(c, r, key, false)
	opts.Addr = core.Concat(":", port)
	s := server.NewService(opts)
	if rr := c.RegisterService("server", s); !rr.OK {
		core.Print(core.Stderr(), serveErrorFormat, rr.Error())
		return 1
	}
	if rr := s.Start(core.Background()); !rr.OK {
		core.Print(core.Stderr(), serveErrorFormat, rr.Error())
		return 1
	}
	// mdns — broadcast the HTTP server as _http._tcp.local under
	// "lthn" (resolves to lthn.local). Best-effort: a broadcast
	// failure shouldn't bring the serve down; LAN discovery is a
	// feature, not a hard dependency.
	if mdnsSvc, _ := core.ServiceFor[*mdns.Service](c, "mdns"); mdnsSvc != nil {
		portInt := core.Atoi(port)
		if portInt.OK {
			mdnsSvc.Configure(mdns.Options{
				Port: portInt.Value.(int),
				TXT: []string{
					"version=" + lthn.Version,
					"marketplace=v1",
					"gateway=v1",
				},
			})
			if rr := mdnsSvc.OnStart(); !rr.OK {
				core.Print(core.Stderr(),
					"lthn serve: mdns broadcast failed (non-fatal): %s\n", rr.Error())
			}
		}
	}
	return 0
}

// buildServerOpts assembles the server.Options surface shared by every
// verb that constructs a pkg/server.Service from the Core (cmdGUI,
// cmdServe, and any future verb that mounts the live HTTP engine).
//
// Builds in this order — order matters because engine.Handler()
// snapshots routes at NewService construction time, so every
// RouteGroup must be in ExtraGroups BEFORE the service is constructed
// by the caller:
//
//  1. opencode — ProxyGroup + ControlGroup; wires SetOnSandboxChange
//     so the runner's dynamic-route table refreshes when sandboxes
//     start/stop; Reconcile sweeps surviving containers from a
//     previous invocation; Enable auto-resume if the persisted flag
//     is true; ApplyDynamicRoutes picks up the post-reconcile set.
//  2. plugin — ProxyGroup (binary-plugin tier).
//  3. gateway — NewRoutes (data firewall); SetBundleCodeResolver wires
//     the per-bundle token verifier (Cerberus Mantis #1443).
//  4. PluginInstalledChecker — composite (plugin || marketplace).
//     Gates capability-grant audit 404-on-unknown-plugin (#1562/#1581).
//  5. PluginOriginChecker — marketplace.ViewRegistry.LoopbackOriginFor
//     cross-check (#1595 / Cerberus #21).
//  6. ServerKey + Core for bootstrap+bearer middleware (#1474 successor).
//
// Verb-specific bits — Addr (serve only), s.Start() (serve only), mdns
// (serve only), s.Handler() consumption (GUI only) — stay in the
// caller. The HELPER is the data-firewall + lifecycle surface; the
// VERB is the listen + lifecycle.
//
// Pre-Mantis #1741, cmdGUI omitted the entire ExtraGroups + opencode
// wiring block, leaving the GUI mode without the gateway data firewall
// and without opencode lifecycle plumbing (Cerberus #70 F-1, A1 +
// STRIDE-T + STRIDE-E).
//
// Usage example:
//
//	c := newAppCore()
//	r, _ := core.ServiceFor[*runner.Service](c, "runner")
//	key, _ := apikey.GenerateOrLoad(c).Value.(string)
//	opts := buildServerOpts(c, r, key)
//	opts.Addr = ":8000" // serve only — GUI leaves empty + skips Start()
//	s := server.NewService(opts)
// buildServerOpts assembles the server.Options shared between cmdGUI
// and cmdServe. The forGUI flag toggles registration of route groups
// that pkg/desktop/subsystems.go.mountSubsystems ALSO registers when
// the GUI's desktop service runs — registering them twice on the same
// Gin engine panics at boot (the wildcard at /v1/api/plugin/:code/
// *proxyPath conflicts with itself, and the same shape applies to
// the opencode reverse-proxy). For cmdServe there is no
// mountSubsystems pass, so extras MUST carry the proxy groups; for
// cmdGUI extras skips them and lets subsystems own that registration.
//
// Per-service ownership today:
//   - opencode.ProxyGroup      — subsystems.go in GUI; extras in serve
//   - plugin.ProxyGroup        — subsystems.go in GUI; extras in serve
//   - opencode.NewControlGroup — extras ALWAYS (subsystems doesn't add)
//   - gateway.NewRoutes        — extras ALWAYS (subsystems doesn't add)
func buildServerOpts(c *core.Core, r *runner.Service, key string, forGUI bool) server.Options {
	var extras []coreapi.RouteGroup
	if opencodeSvc, _ := core.ServiceFor[*opencode.Service](c, "opencode"); opencodeSvc != nil {
		if !forGUI {
			extras = append(extras, opencodeSvc.ProxyGroup())
		}
		extras = append(extras, opencode.NewControlGroup(opencodeSvc))
		// Wire opencode → runner so the runner's dynamic-route set
		// refreshes whenever an opencode sandbox starts or stops.
		// /v1/chat/completions then transparently reaches opencode-
		// routed providers without restarting the live server.
		opencodeSvc.SetOnSandboxChange(func() {
			_ = runner.ApplyDynamicRoutes(r, opencodeSvc.Routes())
		})
		// Reconcile FIRST — sweep the host runtime for surviving
		// lthn-opencode-* containers and re-register them in the
		// orm + reverse-proxy. The orm is in-memory (Memium), so
		// records from the previous invocation are gone; the
		// containers themselves persist on the docker daemon.
		// Without this sweep, the auto-resume path below would see
		// "nothing running" and spawn duplicates.
		if rr := opencodeSvc.Reconcile(); !rr.OK {
			core.Print(core.Stderr(),
				"lthn: opencode reconcile failed: %s\n", rr.Error())
		}
		// Auto-resume — RFC.opencode.md §7 "Restart". If the user
		// previously called Enable, the persisted flag is still
		// true; ensure a sandbox is running. Idempotent via Enable's
		// already-running short-circuit, which now sees the just-
		// reconciled records and skips re-spawn.
		if opencodeSvc.IsEnabled() {
			if rr := opencodeSvc.Enable(""); !rr.OK {
				core.Print(core.Stderr(),
					"lthn: opencode auto-resume failed: %s\n", rr.Error())
				// Continue startup — opencode is optional.
			}
		}
		// Pick up any sandboxes already running (whether resumed
		// just now or surviving from a previous invocation).
		_ = runner.ApplyDynamicRoutes(r, opencodeSvc.Routes())
	}
	pluginSvc, _ := core.ServiceFor[*plugin.Service](c, "plugin")
	if pluginSvc != nil && !forGUI {
		// GUI path: subsystems.go.mountSubsystems registers
		// pluginSvc.ProxyGroup directly on the engine after
		// server.NewService. Double-registering panics Gin.
		extras = append(extras, pluginSvc.ProxyGroup())
	}
	if gatewaySvc, _ := core.ServiceFor[*gateway.Service](c, "gateway"); gatewaySvc != nil {
		extras = append(extras, gateway.NewRoutes(gatewaySvc))
		// Cerberus Mantis #1443 — wire the per-bundle token resolver so
		// the gateway can verify X-Bundle-Token and derive the
		// authoritative bundle code rather than trusting Bundle-ID.
		if pluginSvc != nil {
			gateway.SetBundleCodeResolver(gatewaySvc, pluginSvc)
		}
	}
	// Stage B' wiring (Mantis #1474 successor) — resolve the
	// Bootstrap()ed *serverkey.Service so server.NewService can install
	// the combined bootstrap-plus-bearer middleware (instead of the
	// bearer-only one) and gate /v1/account/create with the short-lived
	// PGP-signed token.
	serverkeySvc, _ := core.ServiceFor[*serverkey.Service](c, "serverkey")
	opts := server.Options{
		Runner:      r,
		LocalKey:    key,
		Brand:       server.Brand{Version: lthn.Version},
		ExtraGroups: extras,
		Core:        c,
		ServerKey:   serverkeySvc,
	}
	// Mantis #1562 follow-up to #1523 — wire the plugin-installed
	// checker so the capability-grant audit endpoint can gate plugin_id
	// against the live install set. Nil checker = audit row emits but
	// the 404-on-unknown-plugin gate stays open.
	//
	// Mantis #1581 — composite the two install surfaces: pkg/plugin
	// owns the binary-plugin tier (ProxyGroup().Has) and pkg/marketplace
	// owns the bundle tier (IsInstalled). The capability-grant 404 gate
	// must cover BOTH or a marketplace-installed plugin_id slips past
	// while a binary plugin gets rejected (asymmetric since H#42 added
	// the marketplace install lane).
	marketplaceSvc, _ := core.ServiceFor[*marketplace.Service](c, "marketplace")
	if pluginSvc != nil || marketplaceSvc != nil {
		opts.PluginInstalledChecker = func(code string) bool {
			if pluginSvc != nil && pluginSvc.ProxyGroup().Has(code) {
				return true
			}
			if marketplaceSvc != nil && marketplaceSvc.IsInstalled(code) {
				return true
			}
			return false
		}
	}
	// Mantis #1595 / Cerberus #21 — cross-check capability-grant origin
	// against marketplace.ViewRegistry's authoritative LoopbackOriginFor
	// record. PluginInstalledChecker proves the plugin is installed;
	// this proves the request's `origin` matches the registry's view
	// of that plugin's loopback origin. Closes audit-log-integrity gap
	// where a falsified (pluginCode, origin) tuple would otherwise
	// commit. Browser postMessage targetOrigin enforcement is the
	// safety floor against token leakage.
	opts.PluginOriginChecker = func(code string) (string, bool) {
		return marketplace.ViewRegistry.LoopbackOriginFor(code)
	}
	return opts
}

// cmdAI handles `lthn ai <verb> [args...]`. AI subsystem dispatch.
//
// Usage example:
//
//	rc := cmdAI([]string{"chat"}) // routes to the chat verb (stub today)
func cmdAI(args []string) int {
	if len(args) == 0 {
		core.Print(core.Stderr(), "lthn ai: missing verb (chat / generate / models / serve)\n")
		core.Print(core.Stderr(), "run `lthn help ai` for usage\n")
		return 2
	}
	switch args[0] {
	case "chat":
		return aiChat(args[1:])
	case "generate":
		return aiGenerate(args[1:])
	case "models":
		return aiModels(args[1:])
	case "serve":
		return cmdServe(args[1:])
	default:
		core.Print(core.Stderr(), "lthn ai: unknown verb %q\nrun `lthn help ai` for available verbs\n", args[0])
		return 2
	}
}
