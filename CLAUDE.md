# CLAUDE.md — lthn/desktop

Repo-local context for agents working in `~/Code/lthn/desktop/`.

## What this repo is

The **Lethean Desktop product repo** — first consumer of the Lethean GUI substrate. Compiles to the `lthn` binary. **Not** Snider's daily-driver IDE (that's `~/Code/core/ide`, a separate product).

See:

- Long-term spec: [`plans/project/lthn/desktop/RFC.md`](../../host-uk/core/plans/project/lthn/desktop/RFC.md)
- First release scope: [`plans/project/lthn/desktop/RFC.first-release.md`](../../host-uk/core/plans/project/lthn/desktop/RFC.first-release.md)
- Visual design ancestry: [`plans/ops/hostuk/website/_design/lethean-5/`](../../host-uk/core/plans/ops/hostuk/website/_design/lethean-5/)
- `lthn` namespace canon: [`plans/project/lthn/RFC.md`](../../host-uk/core/plans/project/lthn/RFC.md) §7

## Architectural rules (load-bearing — read first)

1. **The binary is a CLI router first; Wails is one mode.** `cmd/lthn/main.go` dispatches on subcommand (`lthn`, `lthn version`, `lthn serve`, `lthn ai chat`, `lthn gui`, etc.). The Wails GUI is one consumer of that dispatch, NOT the binary's identity. **Decoupled by design** — if GUI is broken, `lthn serve` + `lthn ai` still ship. The CLI grammar follows the namespace canon (`lthn <verb> <noun>`).
2. **The tray IS the process (in GUI mode).** When Wails launches, `ApplicationShouldTerminateAfterLastWindowClosed = false`. Windows are transient surfaces; closing all of them does NOT quit the app. The NSStatusItem is the lifetime anchor.
3. **Single screen tray panel — no internal navigation.** The 400×560 popover has no side menus, tabs, drawers. Anything that doesn't fit ships as a separate transient window.
4. **Glue only.** No new library code lives in this repo. Library capability lives in `core/`, `go-mlx`, `core/gui`, etc. This repo composes them.
5. **Angular CSR frontend.** The standalone app uses hash routing inside Wails and builds directly to `go/cmd/lthn/dist/`. Do not add Angular SSR or hydration.
6. **British English everywhere.** colour, organisation, centre, behaviour.
7. **EUPL-1.2 / CIC asset-locked.** No "Pro" gates, no upgrade prompts, no feature paywalls.

## Repo shape (canonical Lethean Go layout)

```
lthn/desktop/
├── go.work                  — workspace pins ./go + ./external/* for dev-branch sources
├── .gitmodules              — submodule URLs (dev branch on each)
├── go/                      — the Go module (dappco.re/lthn/desktop)
│   ├── go.mod
│   ├── go.sum
│   ├── cmd/lthn/            — CLI router (main entry). Subcommands: version / help / gui / tray / serve / ai. Future: gateway / build / wallet.
│   ├── pkg/tray/            — NSStatusItem + popover anchor + window-spawn router (consumed by `lthn gui`)
│   ├── pkg/runner/          — go-mlx adapter (start/stop/generate + signals; consumed by `lthn ai` and `lthn serve`)
│   └── pkg/telemetry/       — powermetrics/IOReport sampler
├── external/                — git submodules pinned to dev branches
│   └── go/                  — dappco.re/go (Core primitives)
├── frontend-ng/
│   ├── angular.json         — direct output to go/cmd/lthn/dist/
│   └── src/
│       ├── main.ts          — bridge-first Angular bootstrap
│       ├── wails-bridge.ts  — Wails event + WebMCP shim
│       ├── locale/          — localisation catalogues
│       └── app/             — standalone shell, hash routes, NgRx, app views
└── docs/
    ├── index.md / architecture.md / development.md
    └── design/
        ├── HANDOVER.md      — Lethean-5 Lit handover
        └── lethean-4-react-reference/  — animated React/JSX visual source (reference only)
```

Workspace mode is the bar — `go.work` is the dev-resolution mechanism. Submodules pinned to `dev` branches give live upstream sources; the build resolves through `external/` first.

### CLI dispatch shape

```
lthn                       # default — launches GUI (when GUI is wired; today: scaffold banner)
lthn version               # version info
lthn help [subcommand]     # built-in help

lthn gui                   # explicit Wails GUI launch
lthn tray                  # tray-only mode (NSStatusItem, no popover pre-open)

lthn serve [--port PORT]   # HTTP API only — OpenAI-compatible, no GUI
lthn ai chat               # interactive REPL with the loaded model
lthn ai generate "prompt"  # one-shot generation
lthn ai models ls          # list local models in ~/Lethean/conf/models/
lthn ai models pull NAME   # download from HuggingFace
lthn ai serve              # alias for `lthn serve`

# Future subcommands per namespace canon:
lthn gateway vpn ...       # gateway controls
lthn build ...             # branded `core build`
lthn wallet ...            # blockchain wallet (when side-loaded)
```

`lthn://` URI handlers route through the same dispatch — see `plans/project/lthn/RFC.md` §7.

## Frontend dev

```bash
cd frontend-ng && npm install
npm start -- --host 127.0.0.1 --port 9245 --hmr --poll 1000
# → http://127.0.0.1:9245/#/
# → http://127.0.0.1:9245/#/w/:app
```

`wails3 task dev` starts this server in the background and proxies it into the
WebView. Angular source changes use HMR; Go changes rebuild and relaunch Wails.

## Go services (canonical Service.go pattern — Mantis #1336)

Each `pkg/*/service.go` follows:

```go
type Service struct { /* fields */ }
func New() *Service { return &Service{} }
func (s *Service) Register() error { /* wire actions / commands / lifecycle */ return nil }
```

Wiring against:
- `dappco.re/go/core` (primitives)
- `dappco.re/go/gui` (window / tray / app)
- `dappco.re/go/mlx` (Apple Metal inference)
- `dappco.re/go/store` (KV persistence)
- `dappco.re/go/inference/state` (portable KV state primitive)

## Tests + coverage

Foundation in place on both sides. Coverage target ≥70% per package; lifting from there toward 80%+ is an open agent workstream. Full details in `AGENTS.md` § Testing.

**One-shot entrypoints:**

```bash
wails3 task test                 # Go + frontend
wails3 task test:cover           # both with coverage reports
wails3 task test:cover:go        # → go/coverage.{out,html}
wails3 task test:cover:frontend  # → frontend-ng/coverage/lcov.info
```

**Go side** — 12 packages tested, 10 over 70%. Tests use the `core/go` framework: external `_test` package, alias-import `dappco.re/go` for `core.T`/`AssertEqual`/`AssertTrue`/etc, no separate `"testing"` import, AX `Good/Bad/Ugly` naming, HOME-isolated fixtures via `t.TempDir()` + `t.Cleanup`.

**Frontend side** — Angular's unit-test builder runs Vitest in jsdom. Specs are
colocated under `frontend-ng/src/`; `npm run test:ci` is the non-watch test and
coverage gate.

**Coverage outliers (ceilinged, not bugs):**
- `pkg/services` 49.4% — kardianos writes to `~/Library/LaunchAgents/` etc; integration-suite work, not unit.
- `pkg/desktop` 9.3% — `Service.Run()` boots Wails; headless integration is its own workstream.

Both ceilings are documented in their test files.

## Wails canonical surface (committed 2026-05-12)

`pkg/desktop` wires the full alpha.91 surface so the same codebase targets iOS/Android later (UI just needs media-query responsiveness):

- **Per-window** — `Mac.InvisibleTitleBarHeight` (native drag region), `Mac.WindowLevel`, `Mac.CollectionBehavior` (tray over fullscreen / all Spaces / out of Cmd+`), `Mac.Backdrop`, `Mac.WebviewPreferences.AllowsBackForwardNavigationGestures: u.False` (swipe-back was breaking SPA routes), `Linux.Icon`, `ContentProtectionEnabled`, `DefaultContextMenuDisabled`.
- **App-level** — `SingleInstance` with 32-byte `EncryptionKey` for AES-256-GCM auth on the inter-instance channel (without it second-instance args are untrusted per Wails docs), `AdditionalData: app/version`, `ShouldQuit`, `OnShutdown`, `PostShutdown`, `PanicHandler`, `Windows.EnabledFeatures: msWebView2EnableDraggableRegions`.
- **Dock policy** — Routed through Wails3's canonical `services/dock.HideAppIcon / ShowAppIcon` (not a custom cgo shim). App boots in Accessory (tray-only); opening the unified `app` shell elevates to Regular (Dock icon + Cmd+Tab); close demotes. Code lives in `pkg/desktop/policy.go`.
- **Window events** — `WindowDidMove`, `WindowMaximise/UnMaximise`, `WindowMinimise/UnMinimise` re-broadcast onto the `lthn:window:*` bus for the frontend.
- **Build** — Taskfile ARCH propagation fixed across `package` → `create:*` → `build` chains for all three platforms. Custom `darwin:create:dmg` mirror-implementing Wails' `dmg.Creator` (the alpha.91 dispatch is disabled but the Go package ships). `Linux.Icon` wired through `preCreateWindows`.

## UI shell

The Angular root and desktop component fill the native WebView. The OS window
model in `pkg/desktop` remains authoritative; frontend navigation stays behind
the hash so native `wails://` URLs do not require server-side fallbacks.

## OpenAPI + @lthn/api TypeScript SDK pipeline (2026-05-13)

The HTTP gateway surface lives at `pkg/api/`. Each RouteGroup
implements `coreapi.RouteGroup` + `coreapi.DescribableGroup` so the
spec generator can build a complete OpenAPI 3.1 document straight off
the live `api.Engine`. RouteGroups today: `RunnerGroup` (GET /v1/runner/models, POST /v1/runner/generate, POST /v1/runner/chat).

**Two consumer paths, one surface:**
- **Wails3 runtime** — same-process service access from Angular uses `@wailsio/runtime`; the bridge shim is `frontend-ng/src/wails-bridge.ts`. Generated bindings are staged under `frontend-ng/bindings/`.
- **`@lthn/sdk-*` family on npm** — external clients (Claude Code, Codex, OpenCode, Raycast extensions, future plugins) and Lethean fleet peers. Each flavour lives in its own GitHub repo (`LetheanNetwork/sdk-<flavour>`) and publishes to npm separately. The flavour list is in `build/sdk/publish.sh`'s MANIFEST.

**Published flavours** (`LetheanNetwork/sdk-<id>` → `@lthn/sdk-<id>` on the matching package registry):
- TypeScript: `typescript-fetch`, `typescript-axios`, `typescript`, `typescript-angular`, `typescript-rxjs`, `typescript-node`, `typescript-redux-query`, `typescript-inversify`, `typescript-aurelia`, `typescript-jquery`
- JavaScript: `javascript`, `javascript-flowtyped`, `javascript-closure-angular`, `javascript-apollo` (generator name carries `-deprecated` suffix in 7.x; we still publish since the repo exists)
- Native: `c` (libcurl), `cpp-restsdk` (Casablanca), `cpp-qt` (Qt; formerly `cpp-qt5-client`), `csharp` (multi-target .NET), `objc`, `swift5`, `kotlin`, `rust`, `dart`, `dart-dio`, `clojure`

Generator-name aliases handled in the MANIFEST: openapi-generator 7.x dropped `cpp-qt5-client` → `cpp-qt-client`, dropped `swift4` (use `swift5`), dropped `csharp-netcore` (rolled into `csharp` via `targetFramework=` additional-property), renamed `javascript-apollo` → `javascript-apollo-deprecated`. The MANIFEST keeps a stable repo id while routing to the live generator name; `--additional-properties npmName=... npmVersion=...` is applied only to TS/JS generators, natives use their own package-naming conventions.

**Workflow** (regen on spec change):

```bash
# Generate spec only — written to build/sdk/openapi.yaml
lthn api spec --format yaml --out build/sdk/openapi.yaml

# Generate + push every flavour to its LetheanNetwork remote
./build/sdk/publish.sh

# Or just a subset:
./build/sdk/publish.sh typescript-fetch typescript-axios

# Taskfile entrypoint for the local-only generation lane
wails3 task api:spec
```

The publish driver is idempotent — force-pushes per flavour so the SDK content stays in lockstep with the spec. Per-flavour repos contain ONLY generated code; lthn/desktop tracks the spec + the driver, not the SDK bodies (gitignored under `build/sdk/*/`).

`pkg/desktop/mountSubsystems` registers the lthn RouteGroups on the api.Engine BEFORE wrapping its `Handler()` (handler snapshots the gin tree on first call). The Wails WebView reaches `/api/v1/*` same-origin; standalone clients hit the same paths over TCP via `lthn serve`.

`pkg/api` coverage: 75.7% (9 tests). SDK-gen itself is a shell-out to openapi-generator-cli — covered by argument validation tests; full chain is run by `publish.sh` / CI. Requires Java JDK + openapi-generator-cli on PATH at SDK-gen time, not at runtime.

## Tray popover redesign (committed 2026-05-13)

`main.ts` tray surface refactored into three logical sections under `renderChrome`:
- **Hero card** — model status as a 14px headline, mini-stats grid (HEAP / UPTIME) when a model is loaded, inviting "Pick a model → Browse" dashed card when none, thin inline sparkline strip.
- **Tabbed info card** — System / Runner / Activity tabs, each renders 4 key/value rows. Placeholder surfaces today; built to grow charts + richer stats per tab.
- **Open section** — 2-col grid for Chat / Models / Telemetry (Lethean Desktop moved to systray right-click menu + titlebar screen icon; Settings moved to titlebar cog icon).

## Resolved decisions

- **Bundle ID:** `ai.lthn.desktop` (Snider 2026-05-12)
- **First version:** `v0.1.0` (Snider 2026-05-12)
- **User-data root:** `~/Lethean/` — visible in Finder, no hidden dot-dirs. **Never** `~/.lthn/`. Sub-layout: `~/Lethean/{cli, data, conf, wallets}`. Models live at `~/Lethean/conf/models/`. Per the "no hidden user bloat" principle (Snider 2026-05-12 — memory `design_no_hidden_user_bloat.md`). Uniform with the blockchain app's existing convention.
- **App shell size:** 1440×900 (min 1000×680). Bumped from 1200×800 because the four-column chat layout (nav + conversations + chat body + right rail) was cramped at the prior default.

## Open decisions (need Snider's call)

Tracked at [`plans/project/lthn/desktop/RFC.first-release.md`](../../host-uk/core/plans/project/lthn/desktop/RFC.first-release.md) §7:

1. Telemetry source — `powermetrics` (sudo) vs `IOReport` (no sudo) vs XPC helper
2. First-launch flow when no model present

## Historical first-release task split

From `RFC.first-release.md §9.4`, 14 units A-N. Frontend paths below point at
their current Angular homes:

- A — tray + NSStatusItem registration → `pkg/tray/`
- B — popover anchor + window manager → `pkg/tray/`
- C — header section (brand + Start/Stop) → `frontend-ng/src/app/desktop/`
- D — stats strip (4 rows) → `frontend-ng/src/app/desktop/`
- E — prompt + generation + sparkline → `frontend-ng/src/app/desktop/apps/`
- F — footer status states → `frontend-ng/src/app/desktop/`
- G — runner service (Go ↔ go-mlx) → `pkg/runner/`
- H — telemetry service → `pkg/telemetry/`
- I — Wails bindings + signal wiring → glue across `go/pkg/` + `frontend-ng/`
- J — `lthn` main + service composition → `go/cmd/lthn/`
- K — Taskfile + build/darwin packaging → `Taskfile.yml` + `build/`
- L — codesigning config + first-launch UX → `build/`
- M — theme tokens → `frontend-ng/src/foundations/`
- N — Lethean visual assets → `frontend-ng/public/`

C/D/E/F can ship against fixture data from G/H. K/L runs alongside.
