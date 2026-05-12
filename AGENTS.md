<!-- SPDX-License-Identifier: EUPL-1.2 -->

# Agent Notes

This repository is the Lethean Desktop product binary — `lthn`. It is a CLI router that dispatches to subsystems (GUI, HTTP server, AI runtime, future blockchain / LNS / wallet modules). The Wails GUI is one consumer of that dispatch, not the binary's identity.

## Repository layout

Canonical Lethean Go repo shape:

- `go/` — the Go module (`dappco.re/lthn/desktop`). All Go code lives here.
- `external/` — git submodules of canonical Lethean dependencies, pinned to `dev` branches via `.gitmodules`.
- `go.work` at repo root — workspace mode points at `./go` + `./external/*`. Live dev sources resolve through here.
- `frontend/`, `docs/`, `bin/`, `build/`, `LICENCE`, `README.md`, `CLAUDE.md`, `AGENTS.md`, `Taskfile.yml` at repo root.

## Code Map

- `go/cmd/lthn/main.go` — CLI router. Parses `core.Args()`, dispatches on subcommand (`version`, `help`, `gui`, `tray`, `serve`, `ai`). Add new subcommands here as flat handlers that delegate to `go/pkg/*`.
- `go/pkg/tray/tray.go` — NSStatusItem + popover anchor + window-spawn router (consumed by `lthn gui`).
- `go/pkg/runner/service.go` — go-mlx adapter signals contract (consumed by `lthn ai` and `lthn serve`).
- `go/pkg/telemetry/service.go` — `powermetrics` / `IOReport` sampler.
- `external/go/` — submodule of `dappco.re/go` (the Core primitives module) on its `dev` branch.
- `frontend/` — Vite + Lit. Lethean-5 components in `src/lit/`. `index.html` is the app entry; `canvas.html` is the design canvas.
- `docs/design/lethean-4-react-reference/` — animated React/JSX visual source for design review only; not built.

Each package in `pkg/` follows the Mantis #1336 canonical Service.go shape: a `Service` struct, a `NewService(opts Options) *Service` constructor, a `(s *Service) Register(c *core.Core) core.Result` method, AND a free `Register(c *core.Core) core.Result` function for one-shot wiring. Files declaring `NewService` carry a `// Usage example:` doc marker per Mantis #1383.

## Compliance Rules

Follow the v0.9.0 Core compliance shape. Use `dappco.re/go` wrappers for output (`core.Print`, `core.Println`, `core.Sprintf`), argv (`core.Args()`), exit (`core.Exit`), flag parsing (`core.ParseFlag`), errors (`core.E`, `core.NewError`), and results (`core.Result`, `core.Ok`, `core.Fail`). Direct stdlib imports of `fmt`, `errors`, `strings`, `os`, `log`, `encoding/json`, `bytes`, `path`, `path/filepath`, `os/exec`, `io/ioutil` are banned in production AND test files.

Function signatures return `core.Result`, never `error` and never `(T, error)` tuples. The Result type recovers panics inside the function body, so callers branch on `r.OK` and pull the value from `r.Value`.

Do not roll your own primitives that CoreGO already provides. Check `dappco.re/go/*.go` for the canonical wrapper before importing stdlib.

Use TDD when adding code. Each new public symbol ships with `Test<File>_<Symbol>_{Good,Bad,Ugly}` triplets in the matching `<file>_test.go` and at least one `Example<Symbol>` in `<file>_example_test.go`. The pre-existing scaffold has a test-scaffolding backlog flagged by the v0.9.0 audit (`ax7-triplet-gaps`, `example-gaps`, `missing-test-files`, `missing-example-files`) — file the gap as it gets filled, do not extend the backlog with new untested public symbols.

## Before Stopping

Workspace mode is the bar. From the repo root with `go.work` active:

```bash
go work sync
go vet ./go/...
go test -count=1 ./go/...
gofmt -l go/
bash /Users/snider/Code/core/go/tests/cli/v090-upgrade/audit.sh .
```

The audit reports compliance dimensions. Eight code-wrongness dimensions (`banned-imports`, `err-shape-funcs`, `tuple-result-shape`, `result-discards`, `service-canonical-shape`, `service-usage-example`, `service-name-empty`, `legacy-imports`) should stay at zero on every commit. Test/example/docs completeness dimensions may carry a backlog while scaffold work continues; do not regress what is already at zero.

Frontend builds with Vite. From `frontend/`:

```bash
npm install
npm run build       # → frontend/dist/
npm run dev         # → http://127.0.0.1:5173/
```

The Wails GUI consumer of the binary is decoupled — `lthn serve` and `lthn ai` must function with the GUI broken. Test against this property when changing dispatch.
