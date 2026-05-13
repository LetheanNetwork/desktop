# CLAUDE.md — lthn/desktop

Repo-local context for agents working in `~/Code/lthn/desktop/`.

## What this repo is

The **Lethean Desktop product repo** — first consumer of the Lethean GUI substrate. Compiles to the `lthn` binary. **Not** Snider's daily-driver IDE (that's `~/Code/core/ide`, a separate product).

See:

- Long-term spec: [`plans/project/lthn/desktop/RFC.md`](../../host-uk/core/plans/project/lthn/desktop/RFC.md)
- First release scope: [`plans/project/lthn/desktop/RFC.first-release.md`](../../host-uk/core/plans/project/lthn/desktop/RFC.first-release.md)
- Design canon (Lit port): [`plans/ops/hostuk/website/_design/lethean-5/`](../../host-uk/core/plans/ops/hostuk/website/_design/lethean-5/)
- `lthn` namespace canon: [`plans/project/lthn/RFC.md`](../../host-uk/core/plans/project/lthn/RFC.md) §7

## Architectural rules (load-bearing — read first)

1. **The binary is a CLI router first; Wails is one mode.** `cmd/lthn/main.go` dispatches on subcommand (`lthn`, `lthn version`, `lthn serve`, `lthn ai chat`, `lthn gui`, etc.). The Wails GUI is one consumer of that dispatch, NOT the binary's identity. **Decoupled by design** — if GUI is broken, `lthn serve` + `lthn ai` still ship. The CLI grammar follows the namespace canon (`lthn <verb> <noun>`).
2. **The tray IS the process (in GUI mode).** When Wails launches, `ApplicationShouldTerminateAfterLastWindowClosed = false`. Windows are transient surfaces; closing all of them does NOT quit the app. The NSStatusItem is the lifetime anchor.
3. **Single screen tray panel — no internal navigation.** The 400×560 popover has no side menus, tabs, drawers. Anything that doesn't fit ships as a separate transient window.
4. **Glue only.** No new library code lives in this repo. Library capability lives in `core/`, `go-mlx`, `core/gui`, etc. This repo composes them.
5. **Lit + light DOM for windows.** Leaf components are light DOM (matches the Lethean Lit-rule canon). Tokens.css cascades in.
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
├── frontend/
│   ├── index.html           — app entry (single-window mount via ?surface=)
│   ├── canvas.html          — design canvas (every window side-by-side)
│   └── src/
│       ├── tokens.css       — Lethean-4 OKLCH tokens, Vi-anchored
│       ├── main.js          — surface router for index.html
│       └── lit/             — Lit primitives + windows from Lethean-5
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
cd frontend && npm install && npm run dev
# → http://127.0.0.1:5173/canvas.html   ← every window side-by-side (the design canvas)
# → http://127.0.0.1:5173/?surface=chat ← single-window mount (app entry)
```

### Two viewing surfaces

- **`canvas.html`** — the design canvas from the Lethean-5 handover. Every window rendered side-by-side with section captions. Drop-in from `frontend/src/lit/lit-desktop.html`-origin (now at `frontend/canvas.html`). Open in a browser; pan around; review what's shipped.
- **`index.html`** — the app entry. Mounts one window at a time via `?surface=` URL param. This is the surface Wails serves at production runtime.

Mount any single window for review:
- `?surface=tray` — popover (TODO: port from Lethean-4)
- `?surface=chat&state=multi-turn|generating|switched-model|empty|no-model`
- `?surface=welcome&step=1|2|3`
- `?surface=settings&open=models`
- `?surface=models`
- `?surface=benchmark`
- `?surface=logs&tab=live|history|power`
- `?surface=telemetry`
- `?surface=integrations`
- `?surface=tools`
- `?surface=canvas` (default — index of all)

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
wails3 task test:cover:frontend  # → frontend/coverage/
```

**Go side** — 12 packages tested, 10 over 70%. Tests use the `core/go` framework: external `_test` package, alias-import `dappco.re/go` for `core.T`/`AssertEqual`/`AssertTrue`/etc, no separate `"testing"` import, AX `Good/Bad/Ugly` naming, HOME-isolated fixtures via `t.TempDir()` + `t.Cleanup`.

**Frontend side** — vitest@^3 + happy-dom + @vitest/coverage-v8. 14 spec files, 70 tests, ~600ms wall time. Per-window tests use the shared `frontend/src/test/window-fixture.ts` (`mountWindow`, `expectChromeTitle`, `isEmbedded`, `findCard`). Canonical 4-section pattern: smoke / embedded-sweep / content-presence / reactive-prop.

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

## UI polish — retina + window 100%/100%

- `frontend/index.html` — `-webkit-font-smoothing: antialiased` + `-moz-osx-font-smoothing: grayscale` + `text-rendering: optimizeLegibility` + `font-feature-settings: kern/liga/calt`. Without these the macOS WebView falls back to subpixel-antialiased which makes thin light-on-dark text feel cramped. `color-scheme: dark` meta tells WebView's native form controls + scrollbars + dialogs to render with dark defaults. Viewport meta gains `viewport-fit=cover` for mobile-ready future.
- `frontend/src/lit/chrome.ts` — non-embedded card uses `width:100%; height:100%` instead of fixed `${w}px/${h}px`. The OS window from `pkg/desktop/windows.go` is the authoritative size; the card fills it. `ChromeOptions` gains optional `actions` slot for titlebar right-side icons (cog + screen on the tray today).
- `frontend/src/tokens.css` — global rules give every `<lthn-*-window>` custom element `display:flex; width:100%; height:100%` so the 100% chrome has somewhere to grow.

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

## Parallel sub-tasks for multi-agent direction

From `RFC.first-release.md §9.4`, 14 units A-N. Each has a single owner, single output artefact:

- A — tray + NSStatusItem registration → `pkg/tray/`
- B — popover anchor + window manager → `pkg/tray/`
- C — header section (brand + Start/Stop) → `frontend/src/lit/` (panel)
- D — stats strip (4 rows) → `frontend/src/lit/` (panel)
- E — prompt + generation + sparkline → `frontend/src/lit/` (panel)
- F — footer status states → `frontend/src/lit/` (panel)
- G — runner service (Go ↔ go-mlx) → `pkg/runner/`
- H — telemetry service → `pkg/telemetry/`
- I — Wails bindings + signal wiring → glue across pkg/ + frontend/
- J — `cmd/lthn/` main + service composition → `cmd/lthn/`
- K — Taskfile + build/darwin packaging → `Taskfile.yml` + `build/`
- L — codesigning config + first-launch UX → `build/`
- M — Lethean-4 token import + theme wiring → `frontend/src/` (done — `tokens.css` in place)
- N — Lethean violet tray-icon SVG → `frontend/src/assets/` (pending design pass output)

C/D/E/F can ship against fixture data from G/H. K/L runs alongside.
