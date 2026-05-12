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
	"dappco.re/lthn/desktop/pkg/runner"
	"dappco.re/lthn/desktop/pkg/server"
	"dappco.re/lthn/desktop/pkg/tray"
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
	default:
		core.Print(core.Stderr(), "lthn: unknown subcommand %q\nrun `lthn help` for available commands\n", args[0])
		core.Exit(2)
	}
}

// cmdDefault is invoked when `lthn` is run without a subcommand.
// Today: prints the banner pointing at help. When the GUI is wired,
// this will launch the tray + popover.
//
// Usage example:
//
//	core.Exit(cmdDefault(core.Args()[1:]))
func cmdDefault(args []string) int {
	core.Println("lthn — Lethean unified binary")
	core.Println("")
	core.Println("This is the default mode. The GUI is not yet wired in the scaffold.")
	core.Println("Available subcommands:")
	core.Println("  lthn version       — print version")
	core.Println("  lthn help          — full subcommand list")
	core.Println("  lthn serve         — HTTP API server (stub)")
	core.Println("  lthn ai            — AI subsystem")
	core.Println("")
	core.Println("See `lthn help` or plans/project/lthn/desktop/RFC.first-release.md")
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
	t := tray.NewService(tray.Options{
		Name:        "lthn",
		Description: "Lethean Desktop",
	})
	if r := t.Run(); !r.OK {
		core.Print(core.Stderr(), "lthn gui: %s\n", r.Error())
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
	r := runner.NewServiceFromCore(c)
	if rr := r.Register(c); !rr.OK {
		core.Print(core.Stderr(), "lthn serve: %s\n", rr.Error())
		return 1
	}
	s := server.NewService(server.Options{
		Addr:   core.Concat(":", port),
		Runner: r,
	})
	if rr := s.Register(c); !rr.OK {
		core.Print(core.Stderr(), "lthn serve: %s\n", rr.Error())
		return 1
	}
	if rr := s.Start(core.Background()); !rr.OK {
		core.Print(core.Stderr(), "lthn serve: %s\n", rr.Error())
		return 1
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
