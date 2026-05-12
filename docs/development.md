---
title: Development Guide
description: How to build, test, and extend the lthn binary — prerequisites, Go + frontend toolchains, audit gate, subsystem-adding workflow, dispatch-handler patterns.
---

<!-- SPDX-License-Identifier: EUPL-1.2 -->

# Development Guide

This guide covers everything needed to build, test, extend, and contribute to lthn.

**Module path:** `dappco.re/lthn/desktop`
**Licence:** EUPL-1.2
**Language:** Go 1.26 + Lit (frontend)

---

## Table of Contents

1. [Prerequisites](#1-prerequisites)
2. [Build](#2-build)
3. [Run](#3-run)
4. [Test](#4-test)
5. [Audit gate](#5-audit-gate)
6. [Adding a subcommand](#6-adding-a-subcommand)
7. [Adding a subsystem](#7-adding-a-subsystem)
8. [Frontend development](#8-frontend-development)
9. [Coding standards](#9-coding-standards)

---

## 1. Prerequisites

- Go 1.26 or newer.
- Node 20+ and npm (for the frontend).
- macOS for the GUI / tray (Apple Silicon for full inference performance).
- `gofmt` and `golangci-lint` on PATH for the audit gate.
- `bash` + `python3` for the v0.9.0 compliance audit script.

lthn/desktop uses workspace mode (the canonical Lethean pattern). The `go.work` at the repo root pulls live dev sources from `external/` submodules. Clone with submodules to set it up:

```bash
git clone --recursive <repo-url> lthn-desktop
cd lthn-desktop
go work sync
```

If you cloned without `--recursive`, fetch the submodules:

```bash
git submodule update --init --recursive
```

---

## 2. Build

```bash
go build -o bin/lthn ./go/cmd/lthn
```

Produces a single binary at `bin/lthn`. The frontend is not embedded yet — when the GUI is wired, the Wails build step will compile the frontend assets into the binary.

For a release-shape macOS build (signed `.app` bundle):

```bash
task darwin:package    # TODO — wire via core/gui's package pipeline
```

---

## 3. Run

```bash
./bin/lthn                 # default mode (banner pointing at help — GUI not yet wired)
./bin/lthn version         # v0.1.0
./bin/lthn help            # full subcommand list
./bin/lthn help ai         # subcommand-specific help
./bin/lthn ai              # subsystem dispatch (verb-shaped)
./bin/lthn serve --port 8000   # HTTP server when wired
./bin/lthn gui             # Wails launch when wired
```

Unknown subcommands return exit 2 with a help pointer. Missing required args return exit 2 with usage guidance. All routine output goes to `core.Stdout()`; errors and usage guidance go to `core.Stderr()`.

---

## 4. Test

```bash
go test -count=1 ./go/...
```

Tests follow the canonical Test triplet pattern. For every public symbol `Foo` in `pkg/x/x.go`:

```go
// pkg/x/x_test.go
func TestX_Foo_Good(t *core.T) { /* happy path */ }
func TestX_Foo_Bad(t *core.T)  { /* error path */ }
func TestX_Foo_Ugly(t *core.T) { /* edge case */ }
```

And an example in `pkg/x/x_example_test.go`:

```go
func ExampleFoo() {
    // Usage example body that prints expected output.
    // Output: …
}
```

Use `core.AssertEqual`, `core.AssertTrue`, `core.AssertNotNil`, `core.AssertNoError`, etc. — never `testify`. The `*T` test fixture is from `dappco.re/go` (alias `core`), not `testing.T`.

The audit's `ax7-triplet-gaps`, `example-gaps`, `missing-test-files`, and `missing-example-files` dimensions catch missing tests/examples per public symbol. The scaffold today carries a backlog in these dimensions — fill the gap as the symbol is added (TDD), don't accumulate.

---

## 5. Audit gate

From the repo root with `go.work` active:

```bash
gofmt -l go/
go work sync
go vet ./go/...
go test -count=1 ./go/...
bash /Users/snider/Code/core/go/tests/cli/v090-upgrade/audit.sh .
```

The audit script reports compliance dimensions across the v0.9.0 Core idiom. Eight **code-wrongness** dimensions must stay at zero on every commit:

- `legacy-imports` — `dappco.re/go/core` → `dappco.re/go`
- `banned-imports` — `fmt`, `errors`, `strings`, `os`, `log`, `path`, `path/filepath`, `os/exec`, `io/ioutil`, `encoding/json`, `bytes`
- `err-shape-funcs` — `func ... error` should be `func ... core.Result`
- `tuple-result-shape` — `func ... (*T, error)` should collapse to single-return `core.Result` with value in `r.Value`
- `result-discards` — `_ = expr(...)` in production likely throws away a Result
- `service-name-empty` — `c.Service("", ...)` empty service-name registration
- `service-usage-example` — every `NewService`-declaring file must carry a `// Usage example:` marker
- `service-canonical-shape` — packages with `NewService` must also declare a free `Register(c *core.Core) core.Result` function

Completeness dimensions (tests, examples, docs, licence) may carry a backlog during scaffold work; do not regress what is already at zero.

---

## 6. Adding a subcommand

Subcommands live as flat handler functions in `cmd/lthn/main.go`. To add `lthn newcmd`:

1. Add a `cmdNewcmd(args []string) int` function near the existing handlers. Use `core.Print`, `core.Println`, `core.Sprintf` for output. Parse flags with `core.ParseFlag`.
2. Add a `case "newcmd":` in `main()` that calls `core.Exit(cmdNewcmd(args[1:]))`.
3. Add a help section for the subcommand in `cmdHelp`'s switch.
4. If the subcommand needs subsystem state, construct it via `core.New()` + `<pkg>.Register(c)`.
5. Write the test triplet and example: `cmd/lthn/main_test.go` extends with `TestMain_CmdNewcmd_{Good,Bad,Ugly}` plus an `Example` in `cmd/lthn/main_example_test.go`.

The handler returns the desired exit code: 0 success, 1 runtime failure, 2 usage error.

---

## 7. Adding a subsystem

Each `pkg/*` directory is a self-contained subsystem. To add `pkg/x/`:

1. Create `pkg/x/x.go` with the canonical shape:

```go
// Package x does <thing>.
//
// Usage example:
//
//	c := core.New()
//	s := x.NewService(x.Options{})
//	if r := s.Register(c); !r.OK { return r }
package x

import core "dappco.re/go"

type Options struct { /* … */ }

type Service struct { opts Options }

func NewService(opts Options) *Service { return &Service{opts: opts} }

func (s *Service) Register(c *core.Core) core.Result {
    // Wire actions, signals, lifecycle hooks here.
    return core.Ok(nil)
}

func Register(c *core.Core) core.Result {
    return NewService(Options{}).Register(c)
}
```

2. Create `pkg/x/x_test.go` with `TestX_NewService_{Good,Bad,Ugly}`, `TestX_Service_Register_{Good,Bad,Ugly}`, `TestX_Register_{Good,Bad,Ugly}`.
3. Create `pkg/x/x_example_test.go` with `ExampleNewService`, `ExampleService_Register`, `ExampleRegister`.
4. If the subsystem needs a CLI entry, add a subcommand in `cmd/lthn/main.go`.
5. Run the audit gate — all eight code-wrongness dimensions should stay at zero.

The free `Register(c *core.Core) core.Result` function is canonical per Mantis #1336; without it the `service-canonical-shape` audit dimension fails.

---

## 8. Frontend development

```bash
cd frontend
npm install
npm run dev        # → http://127.0.0.1:5173/
```

Two viewing surfaces:

- `http://127.0.0.1:5173/` — app entry. Mount a single window via `?surface=chat&state=multi-turn` (etc).
- `http://127.0.0.1:5173/canvas.html` — design canvas. Every window side-by-side for design review.

Surface route names: `chat`, `welcome`, `settings`, `models`, `benchmark`, `logs`, `telemetry`, `integrations`, `tools`, `network`, `distillation`, `fleet`, `canvas` (default).

Production build:

```bash
npm run build    # → frontend/dist/{index.html,canvas.html,assets/}
```

The build emits both pages plus the asset bundle. Wails embeds the production output when the GUI ships.

Components and primitives live at `frontend/src/lit/lit-*.js`. The Lethean-5 handover is the source of truth — changes should flow through the design pass at `docs/design/lethean-4-react-reference/` first, then land in the Lit port. Do not add UI framework dependencies (Angular, React, Vue, Svelte are banned by the supply-chain-surface principle).

---

## 9. Coding standards

- **UK English** throughout: `colour`, `behaviour`, `centre`, `organisation`, `licence`. Never American spellings.
- **CoreGO wrappers** for all stdlib equivalents (see audit gate above).
- **`core.Result` return shape** — never `error`, never `(T, error)`.
- **Mantis #1336 Service shape** for every `pkg/*` subsystem — `NewService` + method `Register` + free `Register`.
- **`// Usage example:` doc marker** for every file declaring a service or top-level entry point.
- **TDD** — every public symbol ships with its `Test*_{Good,Bad,Ugly}` triplet + `Example*` in the same commit. Do not accumulate test backlog.
- **No version pins in docs** — `go.mod` and `package.json` are the source of truth for dependency versions. Describe what a dep is, not what version it's at.
- **No hidden user bloat** — user-visible data goes under `~/Lethean/`, never `~/.lthn/` or other dot-dirs.
- **No supply-chain bloat** — see the supply-chain-surface memory; the curated allow-list is Lit + Vite + Lethean tokens (frontend), Go stdlib + `golang.org/x/*` + `dappco.re/*` (backend).

When in doubt, read the v0.9.0 audit at `core/go/tests/cli/v090-upgrade/audit.sh` — the dimensions encode the standards.
