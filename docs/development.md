---
title: Development Guide
description: How to build, test, and extend the lthn binary — prerequisites, Go + frontend toolchains, audit gate, subsystem-adding workflow, dispatch-handler patterns.
---

<!-- SPDX-License-Identifier: EUPL-1.2 -->

# Development Guide

This guide covers everything needed to build, test, extend, and contribute to lthn.

**Module path:** `dappco.re/lthn/desktop`
**Licence:** EUPL-1.2
**Language:** Go 1.26 + Angular (frontend)

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
9. [WebSocket connection manager](#9-websocket-connection-manager)
10. [Coding standards](#10-coding-standards)

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
    return core.Ok(s)
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
cd frontend-ng
npm install
npm start -- --host 127.0.0.1 --port 9245 --hmr --poll 1000
```

The browser-only shell is available at:

- `http://127.0.0.1:9245/#/` — the desktop shell.
- `http://127.0.0.1:9245/#/w/:app` — a standalone native-window host.

Production build:

```bash
npx ng build    # → ../go/cmd/lthn/dist/index.html
```

Angular writes the browser bundle directly to the directory embedded by
`go/cmd/lthn/embed.go`. The application remains client-side rendered: do not
add Angular SSR, a server entry point, or hydration.

---

## 9. WebSocket connection manager

The GUI backend exposes Wails bindings and events through
`pkg/connection`. Its local development endpoint is:

```text
ws://localhost:9099/wails/ws
```

Backend configuration is resolved when the connection service is
constructed:

| Environment variable | Purpose | Default |
|---|---|---|
| `LTHN_WAILS_WS_LISTEN` | Backend TCP listen address | `127.0.0.1:9099` |
| `LTHN_WAILS_WS_PATH` | HTTP WebSocket upgrade path | `/wails/ws` |
| `LTHN_WAILS_WS_URL` | URL or root-relative path published to clients | `ws://localhost:9099/wails/ws` |
| `LTHN_WAILS_WS_ORIGINS` | Comma-separated exact browser Origins | Native Wails and loopback origins |
| `LTHN_WAILS_WS_TOKEN` | Optional upgrade token | Empty on loopback |
| `LTHN_WAILS_WS_TRUST_PROXY` | Acknowledge that an authenticating proxy is the sole route to a non-loopback listener | `false` |

The Angular client accepts an injected URL, an intentional bootstrap
override, backend-served configuration, or a URL previously selected through
`ConnectionManagerService.configure()`. It converts an HTTPS same-origin
path to WSS. It rejects remote plaintext WS, URL credentials, and fragments.
Only the non-secret URL is persisted; tokens are never written to browser
storage.

### Secure reverse proxy

For a host proxy, leave the Go listener on loopback and publish a WSS route.
For example, the essential nginx upgrade shape is:

```nginx
location /wails/ws {
    proxy_pass http://127.0.0.1:9099/wails/ws;
    proxy_http_version 1.1;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection "upgrade";
    proxy_set_header Origin $http_origin;
}
```

Terminate TLS and authenticate the user before this location, then configure:

```bash
LTHN_WAILS_WS_URL=/wails/ws
LTHN_WAILS_WS_ORIGINS=https://desktop.example
```

The browser will resolve the relative path as
`wss://desktop.example/wails/ws`. A proxy in another container or network
namespace may require a non-loopback listen address. In that case, firewall
the backend from direct access and set either
`LTHN_WAILS_WS_TOKEN` or the explicit
`LTHN_WAILS_WS_TRUST_PROXY=true` acknowledgement.

Browser WebSocket clients supply a short-lived token as `access_token` when
the backend token is enabled. Non-browser clients may instead send
`Authorization: Bearer <token>`. Never place a long-lived token in
`LTHN_WAILS_WS_URL`, page history, served JavaScript, proxy access logs, or
mobile persistent storage. Any token used outside loopback must travel over
WSS.

Run the focused transport checks with:

```bash
cd go && go test -race ./pkg/connection
cd ../frontend-ng
npx ng test --configuration=ci --include=src/app/connection-manager.service.spec.ts
```

---

## 10. Coding standards

- **UK English** throughout: `colour`, `behaviour`, `centre`, `organisation`, `licence`. Never American spellings.
- **CoreGO wrappers** for all stdlib equivalents (see audit gate above).
- **`core.Result` return shape** — never `error`, never `(T, error)`.
- **Mantis #1336 Service shape** for every `pkg/*` subsystem — `NewService` + method `Register` + free `Register`.
- **`// Usage example:` doc marker** for every file declaring a service or top-level entry point.
- **TDD** — every public symbol ships with its `Test*_{Good,Bad,Ugly}` triplet + `Example*` in the same commit. Do not accumulate test backlog.
- **No version pins in docs** — `go.mod` and `package.json` are the source of truth for dependency versions. Describe what a dep is, not what version it's at.
- **No hidden user bloat** — user-visible data goes under `~/Lethean/`, never `~/.lthn/` or other dot-dirs.
- **No supply-chain bloat** — frontend dependencies stay deliberate and recorded in `frontend-ng/package.json`; backend code stays within Go stdlib + `golang.org/x/*` + `dappco.re/*`.

When in doubt, read the v0.9.0 audit at `core/go/tests/cli/v090-upgrade/audit.sh` — the dimensions encode the standards.
