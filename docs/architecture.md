---
title: Architecture
description: Internals of the lthn binary — CLI router, subsystem composition (tray / runner / telemetry / future blockchain + LNS + wallet), Wails GUI as one consumer of dispatch, OpenAI-compatible HTTP server via core/api, and the Angular desktop frontend.
---

<!-- SPDX-License-Identifier: EUPL-1.2 -->

# Architecture

This document explains how the lthn binary works internally. It covers the CLI dispatch, the subsystem-composition pattern, the Wails-as-consumer architecture, the frontend pipeline, and the planned wiring for blockchain / LNS / wallet modules.

---

## 1. The CLI router

The binary's entry is `cmd/lthn/main.go`. It is a flat dispatch:

```
core.Args()[1:] ──► switch args[0]
                    ├─ "" / no args      → cmdDefault
                    ├─ "version"         → cmdVersion
                    ├─ "help"            → cmdHelp
                    ├─ "gui"             → cmdGUI
                    ├─ "tray"            → cmdTray
                    ├─ "serve"           → cmdServe
                    ├─ "ai"              → cmdAI ──► chat / generate / models / serve
                    └─ unknown           → exit 2 with help-pointer
```

Each `cmdXxx` is a thin handler that:

1. Parses subcommand-specific flags via `core.ParseFlag`.
2. Constructs the relevant `pkg/*` service with `NewService(opts)`.
3. Either invokes the service directly (`lthn ai generate`) or hands the lifecycle to a Wails / HTTP loop (`lthn gui`, `lthn serve`).

The CLI dispatch is the load-bearing entry. The Wails GUI lives behind one handler (`cmdGUI`). If the GUI build is broken, `lthn serve` and `lthn ai` still function — the binary is useful without any display server.

### Why CLI-first

Three structural benefits:

1. **Deliverable decoupling.** Server deployments need `lthn serve` only — no display, no Wails dependency at runtime.
2. **Progressive wiring.** Each subsystem can be stubbed independently; the dispatch shape is stable even as `cmdXxx` handlers move from stub to full implementation.
3. **Namespace coherence.** The CLI grammar `lthn <verb> <noun>` mirrors the URI scheme `lthn://<verb>/<noun>` and the LNS resolution `<noun>.lthn`. One symbol, six routing surfaces — see `plans/project/lthn/RFC.md` §7.

---

## 2. Subsystem composition

Each `pkg/*` directory is a self-contained subsystem following the Mantis #1336 canonical Service.go shape:

```go
type Service struct { /* fields */ }

type Options struct { /* construction params */ }

// NewService is the typed constructor.
func NewService(opts Options) *Service { return &Service{opts: opts} }

// (Service).Register is the method form — wire an existing service into Core.
func (s *Service) Register(c *core.Core) core.Result { /* … */ }

// Register is the free-function one-shot — used by callers that just want
// "register this subsystem with default options".
func Register(c *core.Core) core.Result {
    return NewService(Options{}).Register(c)
}
```

The free `Register(c *core.Core) core.Result` enables one-line wiring at the binary entry:

```go
c := core.New()
if r := tray.Register(c); !r.OK { return r }
if r := runner.Register(c); !r.OK { return r }
if r := telemetry.Register(c); !r.OK { return r }
```

Adding a new subsystem (e.g. `pkg/wallet/`) follows the same shape. The dispatch in `cmd/lthn/main.go` grows by one case; the binary stays one entry.

### Today's subsystems

- **`pkg/tray/`** — registers the NSStatusItem on macOS, anchors the popover panel, and exposes a `tray.spawn` action that opens a transient expansion window by name. Consumed by `lthn gui`.
- **`pkg/runner/`** — adapts the `go-mlx` inference engine. Exposes lifecycle signals (`status`, `tokensPerSec`, `power`, `memoryMB`) consumed by the frontend via Wails bindings and by the HTTP server for stats endpoints. Consumed by `lthn ai` and `lthn serve`.
- **`pkg/telemetry/`** — polls the platform power source (`powermetrics` on macOS, or `IOReport` framework / XPC helper — decision pending). Drives the watts + memory readouts in the tray panel and the live-telemetry window.

### Planned subsystems

- **`pkg/server/`** — `dappco.re/go/api` Engine wrapping the OpenAI-compatible v1 route group (`/v1/chat/completions`, `/v1/models`, `/v1/embeddings`, `/v1/health`). Streaming via SSE.
- **`pkg/blockchain/`** — Lethean network daemon controls, transaction submission, balance queries.
- **`pkg/lns/`** — `.lthn` decentralised TLD resolution, name registration, ENS bridge.
- **`pkg/wallet/`** — key management consuming the Sovereign rootFS (Borg/Enchantrix/Poindexter triadic) for storage.
- **`pkg/mining/`** — XMRig / TTMiner / pool / proxy control, lifted from the Mining proof-of-pattern repo.

Each ships under the same pattern: a `Service` + `NewService` + dual `Register` shape.

---

## 3. The Wails consumer

`lthn gui` is the GUI consumer of the dispatch. The Wails binary is constructed inside `cmdGUI`:

1. `core.New()` constructs the Core container.
2. `tray.Register(c)` wires the NSStatusItem + popover anchor.
3. `runner.Register(c)` + `telemetry.Register(c)` wire the AI lifecycle and platform telemetry.
4. The Wails `application.App` is constructed with `ApplicationShouldTerminateAfterLastWindowClosed: false` — the NSStatusItem is the lifetime anchor.
5. The Angular frontend is embedded via `application.AssetOptions{Handler: ...}` and served by the Wails internal handler.
6. The popover panel is the default window; expansion windows (chat, settings, benchmark, telemetry) spawn from the tray dropdown or programmatic dispatch.

When the user closes a window, the app keeps running. When the user selects "Quit" from the tray menu, the app exits cleanly via `core.Exit`.

The decoupling is structural: `cmdServe` and `cmdAI` never construct an `application.App`. They use `core.New()` plus the same subsystem `Register` calls, then run their own event loops (HTTP server or REPL).

### WebSocket connection manager

`pkg/connection` is the transport boundary between a Wails backend and any
frontend that consumes its generated bindings:

```
Angular/generated binding
        │
        ▼
ConnectionManagerService ── ws:// or wss:// ──► pkg/connection
                                                    │
                                                    ▼
                                         Wails MessageProcessor
                                                    │
                                                    ▼
                                         registered Go services
```

The service implements Wails' custom transport, asset-server transport, and
event-listener contracts. The default listener is
`127.0.0.1:9099`, with binding IPC at
`ws://localhost:9099/wails/ws`. `pkg/desktop` consumes the registered
connection manager and passes its transport into `application.Options`; it
does not own the listener or client lifecycle.

Generated bindings do not change. `@wailsio/runtime` delegates every call to
the Angular `ConnectionManagerService`, which correlates responses and
forwards server-pushed events into Wails' normal event dispatcher. A browser
or mobile client may therefore run separately from the native WebView and
connect to the authoritative desktop backend.

The Core service exists in CLI, serve, and GUI compositions, but remains
unbound until a Wails application supplies its `MessageProcessor`. This
preserves the rule that `lthn serve` and `lthn ai` do not start a display or
an extra listener.

Remote access is proxy-first: keep the backend on loopback, terminate TLS and
user authentication at the proxy, publish `wss://.../wails/ws`, and configure
an exact HTTPS Origin allow-list. The manager never places its access token
in served JavaScript or status output.

---

## 4. The HTTP server

`lthn serve` starts a `dappco.re/go/api` Engine. The Engine wraps Gin internally and exposes:

- **Route groups** — subsystems register their route groups via `engine.Register(group)`. The OpenAI-compatible `/v1` group is the default.
- **Middleware** — request IDs, response metadata, optional bearer auth, optional CORS — composed via `api.With*` options at construction.
- **Graceful shutdown** — `engine.Serve(ctx)` blocks until `ctx` is cancelled, then drains in-flight requests for up to 10 seconds before stopping.

The `/v1` route group implements:

| Endpoint | Purpose |
|---|---|
| `GET /v1/health` | Liveness probe — returns `{status, service, version, time}` |
| `GET /v1/models` | Enumerate local models in `~/Lethean/conf/models/` |
| `POST /v1/chat/completions` | OpenAI-compatible chat (streaming + non-streaming) |
| `POST /v1/completions` | OpenAI-compatible legacy completion |
| `POST /v1/embeddings` | Text embeddings (when an embedding model is loaded) |

Inference handlers call into `pkg/runner.Service.Generate(...)` — the same path the GUI chat window uses. One runtime, many transports.

---

## 5. The frontend

The production frontend lives at `frontend/` and is a standalone Angular
application. It is client-side rendered and uses hash routing because Wails
serves a static asset bundle rather than a History API server.

- `frontend/src/main.ts` bootstraps Angular directly. Development builds expose
  Wails' built-in MCP service on loopback through the `mcp` build tag; the former
  bridge shim remains in the tree as an unbootstrapped compatibility fallback.
- `frontend/src/app/app.config.ts` wires hash routing, NgRx, WebMCP, and app initialisation.
- `frontend/src/app/app.routes.ts` owns `#/` and `#/w/:app`.
- `frontend/src/locale/` carries the Angular localisation catalogues.
- `frontend/angular.json` writes the browser output directly to `go/cmd/lthn/dist/`, with `index.html` at the root.

Production Wails builds embed that directory. During `wails3 task dev`, Wails
proxies Angular's development server on port 9245, so frontend changes use HMR
while Go changes continue through the normal rebuild-and-relaunch loop.

---

## 6. User-data substrate

lthn writes user-visible data under `~/Lethean/` (no hidden dot-dirs — visibility is a design principle, not a convention).

```
~/Lethean/
├── cli/         binaries
├── data/        runtime data (logs, generated artefacts)
├── conf/        configuration
│   ├── models/  AI models (consumed by runner)
│   └── keys/    signing keys
└── wallets/     wallet files (when blockchain side-loads)
```

User-sensitive data (conversations, wallet keys) routes through the Sovereign rootFS at `~/Lethean/drive/{lthnHash(workspace)}/{lthnHash(path)}` — a triadic composition of Snider/Borg (Secure / Binary), Snider/Enchantrix (Secure / Environment), Snider/Poindexter (Secure / Pointer). The OS sees opaque-named files of encrypted bytes; the user sees disk usage; the workspace key unlocks the contents. The lthn binary consumes this via `dappco.re/go/io`'s `Medium` abstraction.

---

## 7. The CoreGO substrate

Every Go file in the repo uses `dappco.re/go` (alias `core`) wrappers for output, error construction, result handling, argv, flag parsing, and exit. The v0.9.0 compliance audit at `core/go/tests/cli/v090-upgrade/audit.sh` enforces this. Direct stdlib imports of `fmt`, `errors`, `strings`, `os`, `log`, `encoding/json`, `bytes`, `path` are banned.

Function signatures return `core.Result` rather than `error` or `(T, error)`. The `Result` type recovers panics inside the function body, so callers branch on `r.OK` and pull values from `r.Value`. This is the v0.9.0 idiom — see `core/go/docs/` for the full Core API surface.

---

## 8. Future architecture

- **`lthn://` URI handler** — Wails app registers a custom URI scheme; URIs route through the same CLI dispatch via the Go-side router. Clicking `lthn://ai/chat?model=gemma-4-e2b` opens the chat window at that state.
- **Side-loaded modules** — additional `pkg/*` subsystems (blockchain, LNS, mining, wallet) ship as additional `Register` calls and Angular app surfaces. The binary stays one entry; the dispatch grows.
- **Heterogeneous compute** — when `go-rocm` (AMD HIP) and CUDA backends land, the runner abstracts over them; the user picks via settings, the model runs on the best card for each layer. Eventually federated across LetherNet peers.
- **External API fallback** — `go-ratelimit` (already shipped, Sonnet-CLEAN) routes overflow requests to OpenAI / Anthropic when local capacity saturates. The user owns their compute first; cloud is the explicit fallback.

See `plans/project/lthn/desktop/RFC.first-release.md` §2.4 for the platform support trajectory and §9 for the omlx feature parity table.
