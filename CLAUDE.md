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

1. **The tray IS the process.** `ApplicationShouldTerminateAfterLastWindowClosed = false`. Windows are transient surfaces; closing all of them does NOT quit the app. The NSStatusItem is the lifetime anchor.
2. **Single screen tray panel — no internal navigation.** The 400×560 popover has no side menus, tabs, drawers. Anything that doesn't fit ships as a separate transient window.
3. **Glue only.** No new library code lives in this repo. Library capability lives in `core/`, `go-mlx`, `core/gui`, etc. This repo composes them.
4. **Lit + light DOM for windows.** Leaf components are light DOM (matches the Lethean Lit-rule canon). Tokens.css cascades in.
5. **British English everywhere.** colour, organisation, centre, behaviour.
6. **EUPL-1.2 / CIC asset-locked.** No "Pro" gates, no upgrade prompts, no feature paywalls.

## Repo shape

```
lthn/desktop/
├── cmd/lthn/             — main entry
├── pkg/tray/             — NSStatusItem + popover anchor + window-spawn router
├── pkg/runner/           — go-mlx adapter (start/stop/generate + signals)
├── pkg/telemetry/        — powermetrics/IOReport sampler
├── frontend/
│   ├── index.html        — app entry (single-window mount via ?surface=)
│   ├── canvas.html       — design canvas (every window side-by-side)
│   └── src/
│       ├── tokens.css    — Lethean-4 OKLCH tokens, Vi-anchored
│       ├── main.js       — surface router for index.html
│       └── lit/          — Lit primitives + windows from Lethean-5
└── docs/design/
    ├── HANDOVER.md       — Lethean-5 Lit handover (architecture + SwiftUI/Tauri translation notes)
    └── lethean-4-react-reference/  — original React/JSX visual source (animated; reference only, not built)
```

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

## Resolved decisions

- **Bundle ID:** `ai.lthn.desktop` (Snider 2026-05-12)

## Open decisions (need Snider's call)

Tracked at [`plans/project/lthn/desktop/RFC.first-release.md`](../../host-uk/core/plans/project/lthn/desktop/RFC.first-release.md) §7:

1. Telemetry source — `powermetrics` (sudo) vs `IOReport` (no sudo) vs XPC helper
2. First-release version number — `0.1.0` vs `0.0.1`
3. Model directory convention — `~/.lthn/models/` (proposed; not yet confirmed)
4. First-launch flow when no model present

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
