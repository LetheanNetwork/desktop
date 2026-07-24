# lthn — Lethean Desktop

> Native desktop and mobile app — sovereign-compute local LLM client.
> One symbol, six routing surfaces. See [`forge.lthn.sh/lthn/desktop`](https://forge.lthn.sh/lthn/desktop).

**Binary:** `lthn`
**Status:** Angular shell wired to Wails on desktop, iOS and Android.
**Licence:** EUPL-1.2

## What it is

A native macOS tray icon + 400×560 popover panel that runs local LLM inference on Apple Silicon. The tray is the desktop process; expansion windows (chat, settings, benchmark, telemetry) are transient surfaces anchored to it. The same Angular application now runs in Wails' iOS and Android hosts, using the device presentation and a shared lifecycle, battery, network, safe-area and native-feature event contract.

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
│   ├── src/wails-bridge.ts — retained Wails event/WebMCP compatibility shim
│   ├── src/locale/       — Angular localisation catalogues
│   ├── angular.json      — builds directly to go/cmd/lthn/dist/
│   └── package.json
├── build/{darwin,linux,windows,ios,android}/
│                         — platform build configs, native hosts and packaging
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
# Wails' built-in MCP service listens at http://127.0.0.1:9099/mcp.
# The MCP build tag is development-only and is absent from release builds.

# One-shot release build (auto-detect OS):
task build               # produces bin/lthn{.app,.exe,}
task package             # produces a distributable bundle

# Targeted per-OS:
task darwin:build        # macOS .app
task linux:build         # Linux ELF
task windows:build       # Windows .exe

# Mobile packages:
wails3 task ios:package          # bin/lthn.app, arm64 iOS Simulator
wails3 task android:package:fat  # bin/lthn.apk, arm64-v8a + x86_64

# Provisioned iPhone/iPad package:
wails3 task ios:package IOS_PLATFORM=device \
  CODESIGN_IDENTITY="Apple Development: …" \
  PROVISIONING_PROFILE=/path/to/profile.mobileprovision
wails3 task ios:package:ipa IOS_PLATFORM=device \
  CODESIGN_IDENTITY="Apple Development: …" \
  PROVISIONING_PROFILE=/path/to/profile.mobileprovision
```

### Prerequisites

| Tool | Min version | Purpose |
|---|---|---|
| Go | 1.26.0 | backend |
| Node | 22 | Angular frontend |
| `wails3` | v3.0.0-alpha2.117 | pinned mobile overlay generator + task runner |
| `task` (go-task) | 3.x | build runner |
| Xcode | current stable | iOS SDK, linker, assets and signing |
| JDK | 21 | Android Gradle build |
| Android SDK | API 35 | Android compile and packaging tools |
| Android NDK | 26.3.11579264+ | Go c-shared mobile library |

```bash
# One-time tool install:
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-alpha2.117
go install github.com/go-task/task/v3/cmd/task@latest

# Linux only — webkit + GTK:
sudo apt-get install libgtk-3-dev libwebkit2gtk-4.1-dev
```

The mobile tasks use the module-pinned CLI for generated overlays. Set
`WAILS3_TOOL` only when pointing at an already-built CLI of that exact version.
Android release packages use the debug signing key unless the
`ANDROID_KEYSTORE_*` variables documented in
[`build/android/Taskfile.yml`](build/android/Taskfile.yml) are supplied.

## CI / artifact builds

GitHub Actions builds darwin-arm64, linux-amd64, windows-amd64, an arm64
iOS Simulator app and a universal Android APK on every push to `main` / `dev`,
then uploads them as workflow artifacts (7-day retention).

Pushing a `v*` tag creates a GitHub Release with the artifacts attached:

```bash
git tag v0.1.0 && git push github v0.1.0
# → .github/workflows/build.yml runs the matrix, then `release` job
#   also attaches lthn-ios-simulator-arm64.zip + lthn-android-universal.apk
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
