# lthn — Lethean Desktop

> Native macOS tray app — sovereign-compute local LLM runner.
> One symbol, six routing surfaces. See [`forge.lthn.sh/lthn/desktop`](https://forge.lthn.sh/lthn/desktop).

**Binary:** `lthn`
**Status:** Angular desktop shell wired to the Wails runtime and Go services.
**Licence:** EUPL-1.2

## What it is

A native macOS tray icon + 400×560 popover panel that runs local LLM inference on Apple Silicon. The tray is the process; expansion windows (chat, settings, benchmark, telemetry) are transient surfaces anchored to the tray-process. Closing all windows does NOT quit the app.

The product story: **sovereign compute, single-watt** — AI on the user's own hardware, no cloud round-trip, airplane-mode capable.

See the canonical spec:

- [`plans/project/lthn/desktop/RFC.first-release.md`](../../host-uk/core/plans/project/lthn/desktop/RFC.first-release.md) — first-release scope (P0 tray + the v0 to v1.0 trajectory)
- [`plans/project/lthn/desktop/DESIGN-BRIEF.md`](../../host-uk/core/plans/project/lthn/desktop/DESIGN-BRIEF.md) — visual design canon

## Repo layout

```
lthn/desktop/
├── go/
│   ├── cmd/lthn/         — main binary and embedded frontend target
│   └── pkg/              — tray, desktop, runner, telemetry, API, and other services
├── frontend-ng/          — Angular CSR app
│   ├── src/app/          — standalone shell, hash routes, NgRx state, and app views
│   ├── src/wails-bridge.ts — Wails event and WebMCP bridge shim
│   ├── src/locale/       — Angular localisation catalogues
│   ├── angular.json      — builds directly to go/cmd/lthn/dist/
│   └── package.json
├── build/{darwin,linux,windows}/  — platform build configs (codesigning, packaging)
├── docs/
└── Taskfile.yml
```

## Quickstart (dev)

```bash
# Frontend-only — Angular shell, no Go runtime:
cd frontend-ng && npm install
npm start -- --host 127.0.0.1 --port 9245 --hmr --poll 1000
# → http://127.0.0.1:9245/#/

# Full hot-reload dev loop — Wails app + Angular HMR + Go rebuild watcher:
wails3 task dev
# .app launches on first build cycle; menubar icon = lthn-glyph
# Angular edits use HMR; Go edits rebuild and relaunch the app.

# One-shot release build (auto-detect OS):
task build               # produces bin/lthn{.app,.exe,}
task package             # produces a distributable bundle

# Targeted per-OS:
task darwin:build        # macOS .app
task linux:build         # Linux ELF
task windows:build       # Windows .exe
```

### Prerequisites

| Tool | Min version | Purpose |
|---|---|---|
| Go | 1.26.0 | backend |
| Node | 22 | Angular frontend |
| `wails3` | v3.0.0-alpha.91 | CLI scaffold + dev orchestrator |
| `task` (go-task) | 3.x | build runner |

```bash
# One-time tool install:
go install github.com/wailsapp/wails/v3/cmd/wails3@latest
go install github.com/go-task/task/v3/cmd/task@latest

# Linux only — webkit + GTK:
sudo apt-get install libgtk-3-dev libwebkit2gtk-4.1-dev
```

## CI / artifact builds

GitHub Actions builds darwin-arm64, linux-amd64, windows-amd64 on every push to `main` / `dev` and uploads the binaries as workflow artifacts (7-day retention).

Pushing a `v*` tag creates a GitHub Release with the artifacts attached:

```bash
git tag v0.1.0 && git push github v0.1.0
# → .github/workflows/build.yml runs the matrix, then `release` job
#   attaches lthn-darwin-arm64.zip + lthn-linux-amd64 + lthn-windows-amd64.exe
```

Workflow definition: [`.github/workflows/build.yml`](.github/workflows/build.yml).

## First-run flow

The systray app detects fresh installs via `firstlaunch.Detect()` — checks `~/Lethean/conf/lthn.yaml`, the state DB, and any configured routes. When all three are absent, the welcome wizard opens on top of the tray:

1. **Model directory** — where models will live (default `~/.lthn/models/`)
2. **First model** — Gemma 4 E2B (Lethean-recommended starter)
3. **Connect** — opt-in OpenAI-compatible endpoint wiring for Claude Code / OpenCode / Codex

The final "Finish" / "Skip for now" buttons call `ConfigService.Set("welcome.completed", "true")` and open the settings window, so the user can change their mind without re-running the wizard.

## Consumed libraries

The lthn binary is glue. Real capability comes from:

- `dappco.re/go/core` — Core primitives (Options, Config, Service, Action)
- `dappco.re/go/gui` — CoreGUI: window / tray / app lifecycle (wraps the upstream GUI substrate)
- `dappco.re/go/mlx` — Apple Metal native inference engine
- `dappco.re/go/store` — SQLite KV persistence
- `dappco.re/go/io` — filesystem sandbox
- `dappco.re/go/inference/state` — portable KV-as-video-file primitive (warm-resume across sessions / machines)

No new library code lives in this repo — that's the architectural rule. If a capability needs new behaviour, it lives in the canonical home for that capability.

## Platform trajectory

| Release | Adds |
|---|---|
| v0 | macOS / Apple Silicon (Apple Metal — go-mlx) |
| v0.2 | AMD HIP (go-rocm has the custom kernels ready) |
| v0.3 | NVIDIA CUDA |
| v0.5 | Heterogeneous multi-card — link every card the user owns |
| v0.7 | Cross-machine federated compute (LetherNet) |
| v1.0 | External API overflow via go-ratelimit |

The USP: **one runner, every card the user owns.** Mac + AMD + NVIDIA in the same logical flow. When local saturates, overflow routes to a chosen external provider; cloud is the explicit fallback, not the default.

## See also

- [`CLAUDE.md`](CLAUDE.md) — repo-local agent context
- [`forge.lthn.sh/core/ide`](https://forge.lthn.sh/core/ide) — the library Lethean Desktop consumes
- [`forge.lthn.sh/core/gui`](https://forge.lthn.sh/core/gui) — CoreGUI (window/tray substrate)
- [`forge.lthn.sh/core/go-mlx`](https://forge.lthn.sh/core/go-mlx) — Apple Metal inference engine
