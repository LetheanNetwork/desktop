// SPDX-Licence-Identifier: EUPL-1.2

// Command lthn — Lethean's unified CLI binary.
//
// Single binary, multiple modes. The CLI router dispatches based on
// subcommand; the Wails GUI is one consumer of that dispatch, not the
// binary's identity.
//
//	lthn                       # default mode (launches tray + GUI)
//	lthn version               # version info
//	lthn gui                   # explicit GUI launch (tray + popover + windows)
//	lthn tray                  # tray-only mode (no popover-window pre-open)
//	lthn serve [--port PORT]   # HTTP API only — OpenAI-compatible endpoints, no GUI
//	lthn ai chat               # interactive CLI chat with the loaded model
//	lthn ai generate "prompt"  # one-shot generation
//	lthn ai models ls          # list local models
//	lthn ai models pull NAME   # download from HuggingFace
//	lthn ai serve              # alias for `lthn serve` with AI-only scope
//	lthn help [subcommand]     # built-in help
//
// Future subcommands (not yet wired — placeholders per the namespace canon):
//
//	lthn gateway vpn ...       # gateway / VPN controls
//	lthn build ...             # build pipeline (branded `core build`)
//	lthn wallet ...            # wallet operations (when blockchain side-loads)
//
// Architectural rule: the CLI dispatch is the LOAD-BEARING entry. The
// Wails GUI is decoupled — if GUI build is broken, the binary still
// ships via CLI + serve modes. See plans/project/lthn/desktop/
// RFC.first-release.md §1.3.
package main

import (
	"fmt"
	"os"
)

const version = "0.1.0" // first release per Snider 2026-05-12

// dispatch maps a subcommand string to its handler. The empty-string
// key handles the default `lthn` (no-args) invocation.
//
// Handlers receive the remaining argv slice (after the subcommand
// itself was consumed) and return an exit code.
type handler func(args []string) int

func main() {
	args := os.Args[1:]

	// Default mode (no subcommand): launch GUI.
	// Until GUI is wired, print a banner directing the user to
	// `lthn help` so the binary is useful even pre-GUI.
	if len(args) == 0 {
		os.Exit(cmdDefault(args))
	}

	switch args[0] {
	case "version", "-v", "--version":
		os.Exit(cmdVersion(args[1:]))
	case "help", "-h", "--help":
		os.Exit(cmdHelp(args[1:]))
	case "gui":
		os.Exit(cmdGUI(args[1:]))
	case "tray":
		os.Exit(cmdTray(args[1:]))
	case "serve":
		os.Exit(cmdServe(args[1:]))
	case "ai":
		os.Exit(cmdAI(args[1:]))
	default:
		fmt.Fprintf(os.Stderr, "lthn: unknown subcommand %q\nrun `lthn help` for available commands\n", args[0])
		os.Exit(2)
	}
}

// cmdDefault — no subcommand. Future: launch tray + GUI.
// Today: stub printing the namespace banner.
//
// TODO: wire core.New() + tray.Register() + runner.Register() +
// telemetry.Register() + api.Register() + the GUI bootstrap. The
// pattern follows core/ide/cmd/core-ide/main.go.
func cmdDefault(args []string) int {
	fmt.Println("lthn — Lethean unified binary")
	fmt.Println()
	fmt.Println("This is the default mode. The GUI is not yet wired in the scaffold.")
	fmt.Println("Available subcommands right now:")
	fmt.Println("  lthn version       — print version")
	fmt.Println("  lthn help          — full subcommand list")
	fmt.Println("  lthn serve         — HTTP API server (stub)")
	fmt.Println("  lthn ai            — AI subsystem")
	fmt.Println()
	fmt.Println("See `lthn help` or plans/project/lthn/desktop/RFC.first-release.md")
	return 0
}

// cmdVersion — `lthn version` / `lthn -v` / `lthn --version`.
func cmdVersion(args []string) int {
	fmt.Printf("lthn v%s\n", version)
	return 0
}

// cmdHelp — `lthn help [subcommand]`. Built-in help text.
func cmdHelp(args []string) int {
	if len(args) == 0 {
		fmt.Println("lthn — Lethean unified binary")
		fmt.Println()
		fmt.Println("Usage: lthn <subcommand> [args...]")
		fmt.Println()
		fmt.Println("Subcommands:")
		fmt.Println("  version              Print version information")
		fmt.Println("  help [subcommand]    Show help for a subcommand")
		fmt.Println("  gui                  Launch the Wails GUI (tray + popover + windows)")
		fmt.Println("  tray                 Tray-only mode (NSStatusItem, no popover pre-open)")
		fmt.Println("  serve [--port PORT]  HTTP API server (OpenAI-compatible)")
		fmt.Println("  ai <verb> [args...]  AI subsystem — chat, generate, models")
		fmt.Println()
		fmt.Println("Address handler: lthn:// URIs route through the same dispatch")
		fmt.Println("(see plans/project/lthn/RFC.md §7 — the unified namespace canon)")
		return 0
	}
	switch args[0] {
	case "ai":
		fmt.Println("lthn ai — AI subsystem")
		fmt.Println()
		fmt.Println("Verbs:")
		fmt.Println("  chat                       Interactive REPL with the loaded model")
		fmt.Println("  generate \"prompt\"          One-shot generation")
		fmt.Println("  models ls                  List local models in ~/Lethean/conf/models/")
		fmt.Println("  models pull NAME           Download model from HuggingFace")
		fmt.Println("  serve [--port PORT]        AI HTTP API (alias for `lthn serve`)")
	case "serve":
		fmt.Println("lthn serve — HTTP API server")
		fmt.Println()
		fmt.Println("Starts the OpenAI-compatible HTTP API on the given port.")
		fmt.Println("Endpoints: /v1/chat/completions, /v1/completions, /v1/models")
		fmt.Println("Default port: 8000")
	default:
		fmt.Fprintf(os.Stderr, "lthn help: no help for unknown subcommand %q\n", args[0])
		return 2
	}
	return 0
}

// cmdGUI — `lthn gui`. Launches the Wails app.
// TODO: import core/gui + Lethean-5 Lit frontend, follow core/ide pattern.
func cmdGUI(args []string) int {
	fmt.Fprintln(os.Stderr, "lthn gui: not yet wired in scaffold")
	fmt.Fprintln(os.Stderr, "see plans/project/lthn/desktop/RFC.first-release.md §4 for the target")
	return 1
}

// cmdTray — `lthn tray`. NSStatusItem-only, no popover pre-open.
func cmdTray(args []string) int {
	fmt.Fprintln(os.Stderr, "lthn tray: not yet wired in scaffold")
	return 1
}

// cmdServe — `lthn serve [--port PORT]`. HTTP API only, no GUI.
// TODO: wire core/api for the HTTP server, runner for generation,
// telemetry for stats endpoints.
func cmdServe(args []string) int {
	fmt.Fprintln(os.Stderr, "lthn serve: not yet wired in scaffold")
	fmt.Fprintln(os.Stderr, "target: gin-based HTTP server via dappco.re/go/api,")
	fmt.Fprintln(os.Stderr, "OpenAI-compatible endpoints, in-process inference via go-mlx")
	return 1
}

// cmdAI — `lthn ai <verb> [args...]`. AI subsystem dispatch.
func cmdAI(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "lthn ai: missing verb (chat / generate / models / serve)")
		fmt.Fprintln(os.Stderr, "run `lthn help ai` for usage")
		return 2
	}
	switch args[0] {
	case "chat":
		fmt.Fprintln(os.Stderr, "lthn ai chat: not yet wired in scaffold")
		return 1
	case "generate":
		fmt.Fprintln(os.Stderr, "lthn ai generate: not yet wired in scaffold")
		return 1
	case "models":
		fmt.Fprintln(os.Stderr, "lthn ai models: not yet wired in scaffold")
		return 1
	case "serve":
		// Alias for top-level `lthn serve`.
		return cmdServe(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "lthn ai: unknown verb %q\nrun `lthn help ai` for available verbs\n", args[0])
		return 2
	}
}
