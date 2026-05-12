# Lethean Desktop — Lit Components Handover

This is the **Lit-based** version of the desktop window catalogue, prepared for
translation to a real shell (SwiftUI, Tauri, Electron + native menubar, etc.).

Every window is a single `customElement` whose template structure mirrors what
the equivalent SwiftUI `View` body or Tauri component would look like —
declarative, prop-driven, no JSX intermediary stripping motion details.

---

## File layout

```
lit/
├── HANDOVER.md              ← you are here
├── lit-desktop.html         ← canvas / review surface (drop each <lthn-*> into one section)
├── lit-chrome.js            ← shared primitives + renderChrome() window frame
├── lit-chat-window.js       ← E0 · chat
├── lit-ops-windows.js       ← E1 · welcome / settings / model browser
├── lit-obs-windows.js       ← E2 · benchmark / logs / telemetry
└── lit-ext-windows.js       ← E3 + E4 · integrations / tools / network / fine-tune / fleet
```

`tokens.css` (one level up at the project root) carries the brand variables —
`--brand-400`, `--surf-0`, `--fg-0…3`, `--font-mono`, `--success-400`,
`--warning-400`, `--err-400`, etc.

---

## Architecture

### Light DOM, not shadow DOM

Every element calls `createRenderRoot() { return this; }` so that the global
`tokens.css` and Font-Awesome stylesheets apply without per-component
re-importing. Trade-off: style encapsulation is by convention, not enforcement.
If you port to shadow DOM you'll need to either adopt the stylesheets or inline
the tokens.

### `renderChrome({ title, subtitle, w, h, toolbar, body, footer })`

The shared window frame — titlebar (traffic-lights + glyph + title), optional
44 px toolbar row, body area, optional 28 px footer status row. Every window
calls this from inside its own `render()`, passing slot content as
`html`-tagged templates.

When you translate to SwiftUI, this maps to a single reusable `WindowChrome`
view that takes `title`, `subtitle`, and three closures (`toolbar`, `body`,
`footer`).

### Primitives (in `lit-chrome.js`)

| Element | Props | Purpose |
|---|---|---|
| `<lthn-glyph>` | `size`, `color`, `active`, `accent` | Spartan helmet brand mark |
| `<lthn-traffic-lights>` | — | macOS-style window controls |
| `<lthn-label>` | (slot) | Small-caps section header |
| `<lthn-btn>` | `tone`, `size`, `active`, `dim` | Toolbar / action button |
| `<lthn-rail-row>` | `k`, `v` | Key/value row used in right rails |
| `<lthn-toggle>` | `on` | Settings toggle |
| `<lthn-status-dot>` | `variant`, `pulse` | ok / warn / err / idle / active |
| `<lthn-state-pill>` | `variant` (slot) | Connected / running / queued / preview / latest |
| `<lthn-sparkline>` | `data`, `max`, `color`, `width`, `height`, `fill` | Inline trend line |

`<lthn-btn>` tones: `primary` (brand gradient), `ghost` (default toolbar), `quiet` (transparent), `danger`.

---

## Window catalogue

All windows take optional `w` and `h` props (number, in px). Defaults are tuned
for the canvas; bump them up for the real shell.

### E0 · Chat (`lit-chat-window.js`)

```html
<lthn-chat-window state="multi-turn"></lthn-chat-window>
```

`state` ∈ `multi-turn` · `generating` · `switched-model` · `empty` · `no-model`.
`rail="empty"` collapses the right-hand telemetry rail for first-run / no-model.

### E1.1 · Welcome (`lit-ops-windows.js`)

```html
<lthn-welcome-window step="1"></lthn-welcome-window>
```

`step` ∈ `1` (model dir) · `2` (starter model) · `3` (wire CLIs).

### E1.2 · Settings

```html
<lthn-settings-window open="models"></lthn-settings-window>
```

`open` controls which section is expanded.

### E1.3 · Model browser

```html
<lthn-model-browser-window selected="gemma-4-e2b"></lthn-model-browser-window>
```

### E2.1 · Benchmark

```html
<lthn-benchmark-window></lthn-benchmark-window>
```

### E2.2 · Activity / logs

```html
<lthn-logs-window tab="live"></lthn-logs-window>
```

`tab` ∈ `live` · `history` · `power`.

### E2.3 · Live telemetry (demo surface)

```html
<lthn-telemetry-window></lthn-telemetry-window>
```

### E3.1 · Integrations

```html
<lthn-integrations-window></lthn-integrations-window>
```

### E3.2 · Tools / MCP

```html
<lthn-tools-window></lthn-tools-window>
```

### E4 · Concept previews

```html
<lthn-network-window></lthn-network-window>       <!-- LetherNet · v0.7 preview -->
<lthn-distillation-window></lthn-distillation-window> <!-- LoRA wizard -->
<lthn-fleet-window></lthn-fleet-window>            <!-- multi-machine routing -->
```

---

## Translation notes (for Claude Code)

### To SwiftUI

- Each `<lthn-*>` custom element → one `View` struct.
- `renderChrome({title, toolbar, body, footer})` → a `WindowChrome` view with
  three `@ViewBuilder` closures.
- Reactive `state` / `tab` / `step` properties → `@State` on the parent.
- The SVG sparklines / charts can be rendered with `Canvas { ctx, size in … }`
  or the `Charts` framework — the data arrays in each file are the source of
  truth for what to render.
- `tokens.css` variables map 1:1 to a `Color` extension or an `Asset Catalog`.
  Brand teal = `--brand-400` ≈ `#40c1c5`.
- Inline `<i class="fa-…">` icons → SF Symbols. Mapping is roughly:
  `fa-play` → `play.fill`, `fa-pause` → `pause.fill`, `fa-bolt` → `bolt.fill`,
  `fa-shield-halved` → `shield.lefthalf.filled`, etc.
- Animated SVG packets in `lit-network-window` → `withAnimation` on offset, or
  `TimelineView` with a phase oscillator.

### To Tauri (React/Vue/Svelte + Rust backend)

- Each `<lthn-*>` maps directly to a single component — the template literals
  already are JSX-shaped.
- Keep the `data` arrays in each file as `props` or fetched from the Rust side.
- The Lit version is fully self-contained for the design — you can crib markup
  + styles line-for-line.

### Animation inventory

Things that animate today (preserve in port):

1. **Status-dot pulse** — `<lthn-status-dot pulse>` uses the `lthn-pulse`
   keyframes defined in `lit-chrome.js`.
2. **Generation cursor** — in `<lthn-chat-window state="generating">`, the last
   token has `animation: lthn-cursor 1s steps(2, end) infinite`.
3. **Network packets** — SVG `<animate>` tags in `<lthn-network-window>` run
   peer→self packet flights at 3.2 s loops with staggered begins.
4. **Sparklines** — currently static. For live data, swap the `data` attribute
   on a timer; Lit will re-render the path.

---

## Known quirks

1. **ResizeObserver loop warning.** Logged at a browser layer beneath
   user-script `console.error` overrides. Benign — Lit triggers it during
   nested re-renders of the canvas. Does not affect functionality. The
   `lit-desktop.html` preview installs a deferred ResizeObserver shim that
   reduces frequency.

2. **`<lthn-sparkline>` requires numeric `width`/`height` props as JS values.**
   When using inside a Lit template, prefer `.width=${320}` (property bind) over
   `width="320"` (attribute, gets stringified). The element handles both, but
   the property binding is faster.

3. **Light DOM means styles cascade in.** If you wrap windows in a parent that
   sets `font-family` or `color`, it will leak. The current canvas sets these
   on `html, body` only and lets the windows do the rest.

---

## What's _not_ in here

- Real model loading / runtime — these are pure design surfaces.
- The marketing site (host.uk.com / lethean public pages) — separate folder.
- The tray icon SVG family — see `P0/` at project root.
- The native SwiftUI / iOS / Android shells — separate exploration in `native/`.

---

That's the lot. Ping me when the first SwiftUI port is wired up — I'd like to
see the `WindowChrome` view side-by-side with `renderChrome()`.

— Claude
