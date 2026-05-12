# lthn — Lethean Desktop

> Native macOS tray app — sovereign-compute local LLM runner.
> One symbol, six routing surfaces. See [`forge.lthn.sh/lthn/desktop`](https://forge.lthn.sh/lthn/desktop).

**Binary:** `lthn`
**Status:** scaffold — Lit primitives wired from Lethean-5; Go services stubbed.
**Licence:** EUPL-1.2

## What it is

A native macOS tray icon + 400×560 popover panel that runs local LLM inference on Apple Silicon. The tray is the process; expansion windows (chat, settings, benchmark, telemetry) are transient surfaces anchored to the tray-process. Closing all windows does NOT quit the app.

The product story: **sovereign compute, single-watt** — AI on the user's own hardware, no cloud round-trip, airplane-mode capable.

See the canonical spec:

- [`plans/project/lthn/desktop/RFC.first-release.md`](../../host-uk/core/plans/project/lthn/desktop/RFC.first-release.md) — first-release scope (P0 tray + the v0 to v1.0 trajectory)
- [`plans/project/lthn/desktop/DESIGN-BRIEF.md`](../../host-uk/core/plans/project/lthn/desktop/DESIGN-BRIEF.md) — design canon (Lethean-4 visual + Lethean-5 Lit port)

## Repo layout

```
lthn/desktop/
├── cmd/lthn/             — main binary entrypoint (tray-rooted, no quit-on-last-close)
├── pkg/tray/             — NSStatusItem + popover anchor + window-spawn router
├── pkg/runner/           — go-mlx inference adapter (start / stop / generate, signals)
├── pkg/telemetry/        — powermetrics / IOReport sampler (watts + memory readings)
├── frontend/             — Vite + Lit
│   ├── src/
│   │   ├── tokens.css    — Lethean-4 design tokens (OKLCH, Vi-anchored)
│   │   ├── main.js       — entry; mounts windows by ?surface=... URL param
│   │   └── lit/          — Lit primitives + windows from Lethean-5
│   │       ├── chrome.js       — renderChrome() + 9 primitives
│   │       ├── chat-window.js  — E0 chat
│   │       ├── ops-windows.js  — E1 welcome / settings / model browser
│   │       ├── obs-windows.js  — E2 benchmark / logs / telemetry
│   │       └── ext-windows.js  — E3 + E4 integrations / tools / network / fine-tune / fleet
│   ├── index.html
│   ├── package.json
│   └── vite.config.js
├── build/{darwin,linux,windows}/  — platform build configs (codesigning, packaging)
├── docs/
└── Taskfile.yml
```

## Quickstart (dev)

```bash
# Frontend design canvas (Lit windows side-by-side):
cd frontend && npm install && npm run dev
# → http://127.0.0.1:5173/  (mount any window via ?surface=chat etc.)

# Production build (when Go services are wired):
task build
```

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
