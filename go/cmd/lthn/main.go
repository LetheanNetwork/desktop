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
	"dappco.re/lthn/desktop/pkg/apikey"
	"dappco.re/lthn/desktop/pkg/desktop"
	"dappco.re/lthn/desktop/pkg/firstlaunch"
	"dappco.re/lthn/desktop/pkg/fleet"
	"dappco.re/lthn/desktop/pkg/gateway"
	"dappco.re/lthn/desktop/pkg/keys"
	"dappco.re/lthn/desktop/pkg/mdns"
	"dappco.re/lthn/desktop/pkg/opencode"
	"dappco.re/lthn/desktop/pkg/plugin"
	"dappco.re/lthn/desktop/pkg/runner"
	"dappco.re/lthn/desktop/pkg/server"
	"golang.org/x/term"
)

// version is the lthn binary's release tag. Updated per Mantis ticket.
const version = "0.1.0"

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
	core.Print(core.Stdout(), "lthn v%s\n", version)
	return 0
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
	s := server.NewService(server.Options{
		Runner:   r,
		LocalKey: key,
		Brand:    server.Brand{Version: firstlaunch.Version},
		Core:     c,
	})
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
	// Build the ExtraGroups slice before NewService — engine.Handler()
	// snapshots routes at construction time, so late .Engine().Register
	// calls don't reach the live http.Server.
	var extras []coreapi.RouteGroup
	if opencodeSvc, _ := core.ServiceFor[*opencode.Service](c, "opencode"); opencodeSvc != nil {
		extras = append(extras, opencodeSvc.ProxyGroup(), opencode.NewControlGroup(opencodeSvc))
		// Wire opencode → runner so the runner's dynamic-route set
		// refreshes whenever an opencode sandbox starts or stops.
		// /v1/chat/completions then transparently reaches opencode-
		// routed providers without restarting lthn serve.
		opencodeSvc.SetOnSandboxChange(func() {
			_ = r.SetDynamicRoutes(opencodeSvc.Routes())
		})
		// Reconcile FIRST — sweep the host runtime for surviving
		// lthn-opencode-* containers and re-register them in the
		// orm + reverse-proxy. The orm is in-memory (Memium), so
		// records from the previous serve invocation are gone;
		// the containers themselves persist on the docker daemon.
		// Without this sweep, the auto-resume path below would see
		// "nothing running" and spawn duplicates.
		if rr := opencodeSvc.Reconcile(); !rr.OK {
			core.Print(core.Stderr(),
				"lthn serve: opencode reconcile failed: %s\n", rr.Error())
		}
		// Auto-resume — RFC.opencode.md §7 "Restart". If the user
		// previously called Enable, the persisted flag is still
		// true; ensure a sandbox is running. Idempotent via Enable's
		// already-running short-circuit, which now sees the just-
		// reconciled records and skips re-spawn.
		if opencodeSvc.IsEnabled() {
			if rr := opencodeSvc.Enable(""); !rr.OK {
				core.Print(core.Stderr(),
					"lthn serve: opencode auto-resume failed: %s\n", rr.Error())
				// Continue startup — opencode is optional.
			}
		}
		// Pick up any sandboxes already running (whether resumed
		// just now or surviving from a previous serve invocation).
		_ = r.SetDynamicRoutes(opencodeSvc.Routes())
	}
	if pluginSvc, _ := core.ServiceFor[*plugin.Service](c, "plugin"); pluginSvc != nil {
		extras = append(extras, pluginSvc.ProxyGroup())
	}
	if gatewaySvc, _ := core.ServiceFor[*gateway.Service](c, "gateway"); gatewaySvc != nil {
		extras = append(extras, gateway.NewRoutes(gatewaySvc))
	}
	// lthn-process (+ any future service implementing
	// server.RoutesProvider) is auto-discovered via Options.Core
	// passed to server.NewService below — no manual accumulation.
	s := server.NewService(server.Options{
		Addr:        core.Concat(":", port),
		Runner:      r,
		LocalKey:    key,
		Brand:       server.Brand{Version: firstlaunch.Version},
		ExtraGroups: extras,
		Core:        c,
	})
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
					"version=" + firstlaunch.Version,
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
