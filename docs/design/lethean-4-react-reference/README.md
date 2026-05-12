# Handoff: Lethean Desktop — Tray app + expansion windows

## Overview

Design system for **lthn** — the Lethean macOS tray application that runs a local LLM inference runtime ("sovereign-compute, single-watt") on Apple Silicon. The product is a **menu-bar app** (always-on tray icon + 400×560 popover) plus a family of **expansion windows** that open from the tray when the user needs more than the popover can give.

The HTML in this bundle covers the complete P0 surface (tray icon + popover + 5 states + sparkline + output area) plus the **E0 → E4 expansion windows** (chat, settings, welcome, model browser, benchmark, activity logs, live telemetry, integrations, MCP tools, and future-concept sketches for network/fine-tune/fleet).

## About the Design Files

These files are **design references created as a React-in-HTML prototype** — not production code to copy directly. They use inline Babel + React 18 inside a `<design-canvas>` so every window can be reviewed side-by-side in one document.

Your task is to **recreate these designs in the target codebase's environment.** The likely targets:

- **macOS native:** SwiftUI for the menu-bar `NSStatusItem` + popover and for the expansion `NSWindow`s. The `MacTitleBarHiddenInsetUnified` style is called out in the brief.
- **Cross-platform desktop:** Tauri or Electron — recreate the chrome with the same dimensions and the same translucent-backdrop aesthetic.

Pick the target the team has agreed on; the visual + interaction spec is identical either way.

## Fidelity

**High-fidelity.** Final colours, typography, spacing, dimensions, interactions, copywriting, and state behaviour are all production-intent. The tokens are in `tokens.css`. The componentry is in the `windows-*.jsx` and `desktop-*.jsx` files.

## The two source documents

1. **`DESIGN-BRIEF.windows.md`** — read this first. The full design brief that drove this prototype: lifecycle, chrome rules, every window's content shape, min-dimensions, and the no-quit-on-last-close rule.
2. **`desktop.html`** — opens the design canvas with every artboard. Pan/zoom; each artboard has a label corresponding to the spec section (W1 → W20).

## The expansion pattern (read before building anything)

- **Tray icon + popover = the process.** Always present. The runner state lives here.
- **Expansion windows = transient surfaces.** Spawned from the tray popover, the right-click menu, or a keyboard shortcut. Closing one does **not** quit the app.
- `ApplicationShouldTerminateAfterLastWindowClosed = false` on macOS.
- Window position and size remember per-window between sessions.
- The runner is **one process**; the popover and any open chat windows are views over the same in-flight generation.

## Windows shipped in this design

### P0 — Tray surface (the always-on UI)

| ID | Surface | Dimensions | Notes |
|---|---|---|---|
| D1 | Tray icon family | 16/32 px template image | Light/dark × static/active. Spartan-helmet glyph silhouette, monochrome. Active variant has a teal dot in the lower-right. |
| D2–D5 | macOS menu bar | full strip | Idle + active variants in both light + dark menu-bar themes. |
| D6 | Popover (anchored) | 400 × 560 | The single-screen tray panel. No internal navigation. |
| D7–D11 | 5 state variants | 400 × 560 | first-run · loading · ready · generating · error. |
| D12 | Sparkline spec | inline | 60 × 20 mono inline tok/s history. |
| D13 | Output surface spec | inline | Mono · selection · copy · code-block rendering. |

### E0 — Chat window (the first expansion · highest leverage)

| ID | State |
|---|---|
| W1 | Multi-turn · right rail expanded |
| W2 | Empty / first conversation |
| W3 | Mid-generation (streaming) |
| W4 | Switched-model warning (KV cache cleared) |
| W5 | Model not loaded |

**Layout:** 240 px conversation rail · flex conversation surface · optional 280 px right rail (turn-level metadata: tok/s, watts, KV-hits, citations). **Min:** 900×600. **Default:** 1100×740.

### E1 — Operational windows

| ID | Window | Default size |
|---|---|---|
| W6–W8 | Welcome · 3 steps (model dir → starter model → connect clients) | 720×560 min |
| W9 | Settings · sectioned scroll, **never tabbed** | 720×560 min |
| W10 | Model browser · local rail + HF search + detail | 1000×680 min |

### E2 — Observability windows

| ID | Window |
|---|---|
| W11 | Benchmark · history table + tok/s vs context chart, compare mode |
| W12 | Activity · live log (filter rail by component + severity) |
| W13 | Activity · generation history |
| W14 | Activity · power history (24h watt chart) |
| W15 | Live telemetry · the demo surface · big mono readouts + sparklines |

### E3 — Integration windows

| ID | Window |
|---|---|
| W16 | Integrations · per-client status + config preview (Claude Code, OpenCode, Codex, Copilot, Pi) |
| W17 | Tools · MCP · servers + schema + try-it rail |

### E4 — Future-expansion concepts

| ID | Window | Ships with |
|---|---|---|
| W18 | Network · LetherNet peer graph with animated packets | v0.7+ federated compute |
| W19 | Fine-tune · LoRA SFT with live loss · base-vs-tuned eval | go-mlx training stack |
| W20 | Fleet · multi-machine list + live queue | multi-machine support |

## Shared chrome — `LthnWindow` (windows-chrome.jsx)

Every expansion window uses one shared frame:

- **Traffic lights** in the upper-left (macOS native; Linux/Windows fallback in brief)
- **Inline title bar** — `MacTitleBarHiddenInsetUnified` — no separate strip, title sits with content padding, draggable
- **Translucent backdrop** — `MacBackdropTranslucent` (vibrant). Solid `--surface-100` fallback off-Mac.
- **Optional toolbar row** — 44 px, used only when content needs it (model picker, run controls)
- **Optional footer status row** — 28 px, single-line, mono, dim. Used for "5 runs on file · last run 47.2 tok/s · 8.4 W" style status text.

Component signature:

```jsx
<LthnWindow
  title="Settings"
  subtitle="lthn · v0.2.0-rc1"
  width={760}
  height={600}
  toolbar={...}      // optional
  footer={...}       // optional
>
  {body}
</LthnWindow>
```

## Design tokens (tokens.css)

### Brand + accents

```css
--brand-300: #5fd7da   /* teal-300, copy + iconography */
--brand-400: #40c1c5   /* teal-400, active rings, sparklines */
--brand-500: #29a6aa   /* teal-500, primary buttons */
--violet-400: #a78bfa  /* paired accent — used for "watts" readouts + comparison overlays */
--success-400: #22c55e
--warning-400: #f59e0b
--err-400:    #ef4444
```

### Surfaces (dark, the default)

```css
--surf-0: #0c0b10   /* page background */
--surface-0: #0c0b10
--surface-100: #14121a  /* solid fallback when translucency off */
```

### Foreground scale

```css
--fg-0: #e7e5ee   /* primary text */
--fg-1: #b8b6c4   /* body */
--fg-2: #8d8b97   /* muted */
--fg-3: #5a5862   /* hint, metadata */
```

### Type

- **Sans:** `--font-sans` (Inter or SF Pro on macOS)
- **Mono:** `--font-mono` (`ui-monospace, "JetBrains Mono", "SF Mono", monospace`) — used for **all numbers, timestamps, model names, paths, command output, and metadata labels**. Mono usage is heavy and intentional; it's the visual language of the substrate.
- **Display:** SF Pro Display on macOS, falls back to system-ui

### Spacing + radii

- 8 px grid throughout
- Card radii: 6 px (compact rails) · 8 px (cards, code blocks) · 10 px (drop-zones, primary affordances)
- Window radius: 10 px (macOS native)

## Interactions & behaviour

Per-window behaviour is documented in `DESIGN-BRIEF.windows.md` §E0–E4. Highlights:

- **Streaming:** tokens land character-by-character in the chat surface with a brand-violet caret. Stop button replaces send during generation.
- **Sparklines:** 60 samples wide. Update at the runner's poll rate (default 1 Hz). Single brand-teal stroke + 10% fill.
- **Switched-model warning:** when the user changes model mid-conversation, show an amber banner "KV cache cleared — next turn replays context" with a "Restore previous" action.
- **Welcome window:** Esc to skip the current step. Each step is keyboard-navigable.
- **Settings:** all changes apply **immediately** — no save button. The runner keeps running through every change.
- **Benchmark compare mode:** multi-select rows in the history table; selected rows overlay on the chart in their indicator colours (teal · violet · cyan · amber).
- **Network window:** packet animations on edges have `dur="3.2s" begin="<i * 0.4>s"` for the staggered SVG `<animate>` — keep the rhythm slow and contemplative, not anxious.

## State management

The runner state is the single source of truth. Both the tray popover and any open chat window are views over the same state. State variables surfaced in the UI:

```
runner.status          : "idle" | "loading" | "ready" | "generating" | "error"
runner.model           : string | null
runner.tps             : number    // live tok/s
runner.watts           : number    // current draw
runner.kvHit           : percent
runner.contextUsed     : number
runner.contextWindow   : number
runner.lastError       : { message, action } | null
```

Per-conversation state lives in the chat-window scope:

```
conversation.id
conversation.model     // can differ from runner.model briefly during switch
conversation.turns     : Turn[]
conversation.generating: boolean
```

## Files in this bundle

- `desktop.html` — the entrypoint. Opens the design canvas with every artboard.
- `tokens.css` — design tokens (the only stylesheet).
- `design-canvas.jsx` — pan/zoom canvas component (review surface only; don't ship).
- `tweaks-panel.jsx` — designer tweaks panel (don't ship).
- `desktop-panel.jsx` — `LthnGlyph`, `MenubarStrip`, `TrayPanel`, sparkline, status dot. **Ship the tray icon + popover from this file.**
- `desktop-spec.jsx` — brief intro card, icon spec, sparkline + output spec cards, open-questions card. **Reference only — these are documentation surfaces, not product.**
- `windows-chrome.jsx` — `LthnWindow`, `WinTrafficLights`, `WinBtn`, `WinLabel`. **Ship as the shared window frame.**
- `windows-chat.jsx` — `ChatWindow` + every chat sub-component. **Ship E0.**
- `windows-ops.jsx` — `WelcomeWindow`, `SettingsWindow`, `ModelBrowserWindow`. **Ship E1.**
- `windows-obs.jsx` — `BenchmarkWindow`, `LogsWindow`, `TelemetryWindow`, `IntegrationsWindow`, `ToolsWindow`, plus `NetworkWindow`, `DistillationWindow`, `FleetWindow` for E4 concepts.
- `DESIGN-BRIEF.windows.md` — the source-of-truth brief (read first).

## What you don't get from this bundle

- **Actual icons.** The Spartan-helmet glyph is rendered from a placeholder asset path. The brand team has the final SVG — ask them.
- **Real models.** Every screenshot of a model name (`gemma-4-e2b`, `llama-3.2-3b`, etc.) is sample data. Wire to the real model directory.
- **Real telemetry.** All charts use static fixture data. Wire to `powermetrics` / `IOReport` per the brief §E2.
- **Linux / Windows window chrome.** The chrome is designed against macOS; the brief calls out the cross-platform fallback rule but the visual spec isn't drawn — design that pass when you get to it.

## Copy & tone

British English. Confident, restrained, technical. Vi's voice is minimal here — this is the substrate surface, not the marketing surface. Status messages tell the truth ("KV cache cleared", "runner refused — too big for this Mac"). No apologies. No emoji unless explicitly in the brand system.
