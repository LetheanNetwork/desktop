/* lit-chrome.js — shared Lit pieces for Lethean Desktop
 *
 * Approach: light DOM + a `renderChrome()` template helper so the global
 * tokens.css / font-awesome stylesheets apply without shadow-root juggling.
 *
 * Exports (all attached to globalThis for cross-script use):
 *   renderChrome({ title, subtitle, w, h, toolbar, footer, body })
 *
 * Custom elements registered:
 *   <lthn-glyph size color active accent>
 *   <lthn-traffic-lights>
 *   <lthn-label>                            (slot — small-caps section header)
 *   <lthn-btn tone size active dim>         (slot — text label; use <i.fa> for icons)
 *   <lthn-rail-row k="" v="">
 *   <lthn-toggle on>
 *   <lthn-status-dot variant pulse>
 *   <lthn-state-pill variant>               (slot — pill text)
 *   <lthn-sparkline data="..." color width height fill>
 */

import { LitElement, html, nothing, type TemplateResult } from "lit";
import type {
  BtnTone, BtnSize, StatusDotVariant, StatePillVariant, LitContent,
} from "./types";

export interface ChromeOptions {
  title?:    string;
  subtitle?: string;
  w?:        number;
  h?:        number;
  toolbar?:  LitContent;
  body?:     LitContent;
  footer?:   LitContent;
  // actions: optional right-side titlebar content — typically a row of
  // small icon buttons (settings cog, open-in-app, etc.). Sits in the
  // titlebar after the flex spacer so it's flush to the right edge.
  // Caller is responsible for opting out of drag via --wails-draggable:
  // no-drag on any interactive children.
  actions?:  LitContent;
  // embedded: when true (set by <lthn-app-shell> when it mounts a
  // child window), skip the titlebar / footer / rounded-card frame
  // and just render the body filling the parent. Standalone use
  // (systray-spawned "lite" windows) leaves embedded=false and gets
  // the full Lethean-5 cute card chrome.
  embedded?: boolean;
}

declare global {
  interface Window {
    lthnRenderChrome: typeof renderChrome;
    __lthnLitReady:   boolean;
  }
}

/* ────────────────────────────────── <lthn-glyph> ────────────────── */
class LthnGlyph extends LitElement {
  static readonly properties = {
    size:   { type: Number },
    color:  { type: String },
    active: { type: Boolean, reflect: true },
    accent: { type: String },
  };
  declare size: number;
  declare color: string;
  declare active: boolean;
  declare accent: string;
  constructor() { super(); this.size = 16; this.color = "currentColor"; this.active = false; }
  createRenderRoot() { return this; }
  render() {
    const s = this.size;
    const acc = this.accent || "var(--brand-400)";
    return html`
      <span style="position:relative; display:inline-flex; width:${s}px; height:${s}px; color:${this.color}; flex-shrink:0;">
        <svg viewBox="0 0 100 100" width=${s} height=${s} fill="currentColor" aria-hidden="true">
          <path d="M50 8 L78 22 L78 38 Q78 56 64 70 L64 82 L72 82 L72 92 L28 92 L28 82 L36 82 L36 70 Q22 56 22 38 L22 22 Z M48 32 L48 56 L52 56 L52 32 Z M30 36 L46 36 L46 44 L30 44 Z M54 36 L70 36 L70 44 L54 44 Z" />
        </svg>
        ${this.active ? html`<span style="position:absolute; right:-1px; bottom:-1px; width:${Math.max(4, s*0.32)}px; height:${Math.max(4, s*0.32)}px; border-radius:50%; background:${acc}; box-shadow:0 0 4px ${acc};"></span>` : nothing}
      </span>
    `;
  }
}
customElements.define("lthn-glyph", LthnGlyph);

/* ────────────────────────────────── <lthn-traffic-lights> ───────── */
class LthnTrafficLights extends LitElement {
  createRenderRoot() { return this; }
  // Wire the macOS-style buttons to the live Wails Window API. Red =
  // Close (Wails handles the WindowClosing event; our windows are
  // hide-on-close so the side-effect is hide). Yellow = Minimise.
  // Green = Fullscreen toggle (Maximise on Windows/Linux semantics).
  // Dynamic import keeps the runtime out of the non-Wails canvas
  // preview's hot-path.
  private async _act(verb: "close" | "minimise" | "fullscreen") {
    try {
      const { Window } = await import("@wailsio/runtime");
      if (verb === "close") await Window.Close();
      else if (verb === "minimise") await Window.Minimise();
      else if (verb === "fullscreen") {
        const fs = await Window.IsFullscreen();
        if (fs) await Window.UnFullscreen(); else await Window.Fullscreen();
      }
    } catch (err) {
      console.error(`traffic-light ${verb} failed:`, err);
    }
  }
  render() {
    const btn = (color: string, verb: "close" | "minimise" | "fullscreen") => html`
      <span
        @click=${() => this._act(verb)}
        title=${verb}
        style="width:12px; height:12px; border-radius:50%; background:${color}; cursor:pointer; --wails-draggable: no-drag;"
      ></span>
    `;
    return html`
      <div style="display:flex; gap:8px; align-items:center; --wails-draggable: no-drag;">
        ${btn("#ff5f57", "close")}
        ${btn("#febc2e", "minimise")}
        ${btn("#28c840", "fullscreen")}
      </div>
    `;
  }
}
customElements.define("lthn-traffic-lights", LthnTrafficLights);

/* ────────────────────────────────── <lthn-label> ────────────────── */
class LthnLabel extends LitElement {
  createRenderRoot() { return this; }
  render() {
    return html`
      <div style="font-family:var(--font-mono); font-size:10px; color:var(--fg-3); letter-spacing:0.08em; text-transform:uppercase;">
        <slot></slot>
      </div>
    `;
  }
}
customElements.define("lthn-label", LthnLabel);

/* ────────────────────────────────── <lthn-btn> ──────────────────── */
class LthnBtn extends LitElement {
  static readonly properties = {
    tone:   { type: String, reflect: true },
    size:   { type: String, reflect: true },
    active: { type: Boolean, reflect: true },
    dim:    { type: Boolean, reflect: true },
  };
  declare tone:   BtnTone;
  declare size:   BtnSize;
  declare active: boolean;
  declare dim:    boolean;
  constructor() { super(); this.tone = "ghost"; this.size = "md"; this.active = false; this.dim = false; }
  // Shadow DOM (default) — required because the template uses <slot>
  // to project the button label. Light DOM would leave the slot inert
  // (appears empty next to the slotted text). Per feedback memory
  // feedback_lit_shadow_dom_rules.md: slot → Shadow DOM. All styles
  // are inline so the token-css-doesn't-cross-shadow caveat is moot.
  render() {
    const sizes = {
      sm: { pad: "4px 9px",  font: 11,   gap: 6 },
      md: { pad: "6px 12px", font: 12,   gap: 7 },
      lg: { pad: "9px 16px", font: 13,   gap: 8 },
    };
    const s = sizes[this.size] || sizes.md;
    const tones = {
      primary: { bg: "linear-gradient(180deg, var(--brand-400), var(--brand-500))", color: "#fff", border: "1px solid rgba(64,193,197,0.45)", shadow: "0 1px 0 rgba(255,255,255,0.10) inset, 0 1px 2px rgba(0,0,0,0.30)" },
      ghost:   { bg: this.active ? "rgba(64,193,197,0.10)" : "rgba(255,255,255,0.04)", color: this.active ? "var(--brand-300)" : "var(--fg-1)", border: this.active ? "1px solid rgba(64,193,197,0.30)" : "1px solid rgba(255,255,255,0.07)", shadow: "none" },
      quiet:   { bg: "transparent", color: "var(--fg-2)", border: "1px solid transparent", shadow: "none" },
      danger:  { bg: "rgba(255,76,76,0.12)", color: "var(--err-300, #ffb4b4)", border: "1px solid rgba(255,76,76,0.30)", shadow: "none" },
    };
    const t = tones[this.tone] || tones.ghost;
    return html`
      <button style="
        --wails-draggable: no-drag;
        display:inline-flex; align-items:center; gap:${s.gap}px;
        padding:${s.pad}; border-radius:6px;
        font-size:${s.font}px; font-weight:500;
        font-family: var(--font-sans);
        background:${t.bg}; color:${t.color};
        border:${t.border}; box-shadow:${t.shadow};
        cursor: pointer; white-space: nowrap;
        letter-spacing:-0.005em;
        opacity:${this.dim ? 0.55 : 1};
      "><slot></slot></button>
    `;
  }
}
customElements.define("lthn-btn", LthnBtn);

/* ────────────────────────────────── <lthn-rail-row> ─────────────── */
class LthnRailRow extends LitElement {
  static readonly properties = { k: { type: String }, v: { type: String } };
  declare k: string;
  declare v: string;
  createRenderRoot() { return this; }
  render() {
    return html`
      <div style="display:flex; justify-content:space-between; gap:12px; font-size:11.5px;">
        <span style="color:var(--fg-3);">${this.k}</span>
        <span style="color:var(--fg-1); font-family:var(--font-mono); font-size:11px; text-align:right;">${this.v}</span>
      </div>
    `;
  }
}
customElements.define("lthn-rail-row", LthnRailRow);

/* ────────────────────────────────── <lthn-toggle> ───────────────── */
class LthnToggle extends LitElement {
  static readonly properties = { on: { type: Boolean, reflect: true } };
  declare on: boolean;
  createRenderRoot() { return this; }
  render() {
    return html`
      <span style="width:32px; height:18px; border-radius:999px; background:${this.on ? "var(--brand-500)" : "rgba(255,255,255,0.10)"}; position:relative; display:inline-block;">
        <span style="position:absolute; top:2px; left:${this.on ? 16 : 2}px; width:14px; height:14px; border-radius:50%; background:#fff; box-shadow:0 1px 2px rgba(0,0,0,0.3); transition:left .15s;"></span>
      </span>
    `;
  }
}
customElements.define("lthn-toggle", LthnToggle);

/* ────────────────────────────────── <lthn-status-dot> ───────────── */
class LthnStatusDot extends LitElement {
  static readonly properties = { variant: { type: String, reflect: true }, pulse: { type: Boolean } };
  declare variant: StatusDotVariant;
  declare pulse:   boolean;
  constructor() { super(); this.variant = "ok"; this.pulse = false; }
  createRenderRoot() { return this; }
  render() {
    const c = { ok:"var(--success-400)", warn:"var(--warning-400)", err:"var(--err-400)", idle:"var(--fg-3)", active:"var(--brand-400)" }[this.variant] || "var(--fg-3)";
    return html`
      <span style="display:inline-block; width:7px; height:7px; border-radius:50%; background:${c}; box-shadow:${this.variant === "idle" ? "none" : `0 0 4px ${c}`}; ${this.pulse ? "animation: lthn-pulse 1.4s ease-in-out infinite;" : ""}"></span>
    `;
  }
}
customElements.define("lthn-status-dot", LthnStatusDot);

/* ────────────────────────────────── <lthn-state-pill> ───────────── */
class LthnStatePill extends LitElement {
  static readonly properties = { variant: { type: String, reflect: true } };
  declare variant: StatePillVariant;
  constructor() { super(); this.variant = "connected"; }
  createRenderRoot() { return this; }
  render() {
    const map = {
      connected:    { bg:"rgba(34,197,94,0.10)",   bd:"rgba(34,197,94,0.22)",   fg:"var(--success-400)" },
      running:      { bg:"rgba(34,197,94,0.10)",   bd:"rgba(34,197,94,0.22)",   fg:"var(--success-400)" },
      queued:       { bg:"rgba(255,255,255,0.05)", bd:"rgba(255,255,255,0.07)", fg:"var(--fg-2)" },
      disconnected: { bg:"rgba(255,255,255,0.04)", bd:"rgba(255,255,255,0.06)", fg:"var(--fg-3)" },
      preview:      { bg:"rgba(245,158,11,0.10)",  bd:"rgba(245,158,11,0.22)",  fg:"var(--warning-400)" },
      latest:       { bg:"rgba(64,193,197,0.10)",  bd:"rgba(64,193,197,0.22)",  fg:"var(--brand-300)" },
    };
    const t = map[this.variant] || map.queued;
    return html`
      <span style="font-family:var(--font-mono); font-size:9.5px; padding:2px 8px; border-radius:999px; background:${t.bg}; border:1px solid ${t.bd}; color:${t.fg}; letter-spacing:0.06em; text-transform:uppercase; display:inline-block; width:fit-content;">
        <slot></slot>
      </span>
    `;
  }
}
customElements.define("lthn-state-pill", LthnStatePill);

/* ────────────────────────────────── <lthn-sparkline> ────────────── */
class LthnSparkline extends LitElement {
  static readonly properties = {
    data:  { type: String },
    max:   { type: Number },
    color: { type: String },
    width: { type: Number },
    height:{ type: Number },
    fill:  { type: Boolean },
  };
  declare data: string;
  declare max: number;
  declare color: string;
  declare width: number;
  declare height: number;
  declare fill: boolean;
  constructor() { super(); this.data = ""; this.max = 60; this.color = "var(--brand-400)"; this.width = 70; this.height = 22; this.fill = true; }
  createRenderRoot() { return this; }
  render() {
    let samples = (this.data || "").split(",").map(s => parseFloat(s)).filter(n => !isNaN(n));
    if (!samples.length) {
      // procedurally generate a calm wave so a bare <lthn-sparkline> still draws something
      samples = Array.from({length: 24}, (_, i) => 30 + Math.sin(i * 0.7) * 10 + Math.sin(i * 1.3) * 5);
    }
    const w = this.width, h = this.height, m = this.max || 1;
    // With a single sample the x-step formula `i / (n-1)` divides by zero.
    // Draw a flat horizontal line across the canvas instead.
    const xStep = samples.length > 1 ? w / (samples.length - 1) : 0;
    const path = "M " + samples.map((s, i) =>
      `${(samples.length > 1 ? i * xStep : w / 2)} ${h - (s / m) * h}`
    ).join(" L ");
    return html`
      <svg viewBox="0 0 ${w} ${h}" width=${w} height=${h} style="display:block;">
        ${this.fill ? html`<path d="${path} L ${w} ${h} L 0 ${h} Z" fill=${this.color} fill-opacity="0.10" />` : nothing}
        <path d=${path} stroke=${this.color} stroke-width="1.5" fill="none" />
      </svg>
    `;
  }
}
customElements.define("lthn-sparkline", LthnSparkline);

/* ────────────────────────────────── renderChrome() ──────────────── *
 * The shared window frame, returned as a Lit TemplateResult so window
 * elements can compose it inside their own templates.
 *
 *   renderChrome({
 *     title: "lthn · chat",
 *     subtitle: "conversation · local",
 *     w: 1100,
 *     h: 740,
 *     toolbar: html`...`,    // optional 44px toolbar row
 *     body:    html`...`,    // required body content
 *     footer:  html`...`,    // optional 28px status row
 *   })
 */
export function renderChrome({ title, subtitle, w = 900, h = 600, toolbar, body, footer, actions, embedded = false }: ChromeOptions = {}) {
  // Embedded mode — mounted inside <lthn-app-shell>. The shell paints
  // its own titlebar / status bar and we fill its body slot. No card
  // frame, no rounded edges (the shell already has those), no titlebar
  // (the shell already has one). Toolbar still ships because it's a
  // window-local concern (e.g. chat's "model · ctx · 1 runner" row).
  if (embedded) {
    return html`
      <div class="lthn-window lthn-window--embedded" style="
        width:100%; height:100%;
        display:flex; flex-direction:column;
        color: var(--fg-1);
        font-family: var(--font-sans);
        --wails-draggable: drag;
      ">
        ${toolbar ? html`
          <div style="
            display:flex; align-items:center; gap:8px;
            min-height:44px; padding:8px 14px;
            background: rgba(0,0,0,0.12);
            border-bottom: 1px solid rgba(255,255,255,0.04);
            flex-shrink:0;
          ">${toolbar}</div>
        ` : nothing}
        <div style="flex:1; display:flex; flex-direction:column; min-height:0; overflow:hidden;">
          ${body}
        </div>
      </div>
    `;
  }
  // Wails windows ARE the size container — the OS window is sized by
  // pkg/desktop/windows.go's registry. The card should fill that
  // container (100% × 100%) rather than paint a fixed-pixel card at
  // each component's self-declared w/h, which previously caused the
  // inner card to either clip past the OS window edge or leave gaps.
  // The w / h ChromeOptions stay as canvas-preview hints (used by the
  // ?surface=canvas dev route, which can wrap windows in fixed-sized
  // containers). Future mobile breakpoints can layer on top of the
  // 100% layout via media queries inside the body content.
  return html`
    <div class="lthn-window" style="
      width:100%; height:100%;
      display:flex; flex-direction:column;
      background: linear-gradient(180deg, rgba(20,18,26,0.92) 0%, rgba(12,11,16,0.94) 100%);
      backdrop-filter: blur(20px) saturate(180%);
      -webkit-backdrop-filter: blur(20px) saturate(180%);
      border: 1px solid rgba(255,255,255,0.08);
      border-radius: 10px;
      overflow: hidden;
      box-shadow:
        0 24px 60px rgba(0,0,0,0.55),
        0 2px 6px rgba(0,0,0,0.30),
        inset 0 1px 0 rgba(255,255,255,0.04);
      color: var(--fg-1);
      font-family: var(--font-sans);
      /* Default-drag: the whole frameless card acts like a native macOS
         window background. Interactive children (buttons, traffic lights,
         inputs, scrollable lists) opt out via --wails-draggable: no-drag.
         Native-macOS-equivalent behaviour without users needing to find
         the narrow title strip. */
      --wails-draggable: drag;
    ">
      <!-- titlebar — Wails3 frameless drag region via --wails-draggable.
           Interactive children should override with --wails-draggable: no-drag. -->
      <header style="
        display:flex; align-items:center; gap:14px;
        height:40px; padding:0 14px;
        --wails-draggable: drag;
        user-select: none;
        background: rgba(255,255,255,0.015);
        border-bottom: 1px solid rgba(255,255,255,0.04);
        flex-shrink:0;
      ">
        <lthn-traffic-lights></lthn-traffic-lights>
        <div style="display:flex; align-items:baseline; gap:10px; min-width:0;">
          <lthn-glyph size="14" color="var(--fg-1)"></lthn-glyph>
          <div style="font-size:13px; font-weight:600; color:var(--fg-0); letter-spacing:-0.005em;">${title}</div>
          ${subtitle ? html`<div style="font-family:var(--font-mono); font-size:10.5px; color:var(--fg-3); letter-spacing:0.02em;">· ${subtitle}</div>` : nothing}
        </div>
        <div style="flex:1"></div>
        ${actions ? html`<div style="display:flex; align-items:center; gap:4px; --wails-draggable: no-drag;">${actions}</div>` : nothing}
      </header>

      ${toolbar ? html`
        <div style="
          display:flex; align-items:center; gap:8px;
          min-height:44px; padding:8px 14px;
          background: rgba(0,0,0,0.12);
          border-bottom: 1px solid rgba(255,255,255,0.04);
          flex-shrink:0;
        ">${toolbar}</div>
      ` : nothing}

      <!-- body -->
      <div style="flex:1; display:flex; flex-direction:column; min-height:0; overflow:hidden;">
        ${body}
      </div>

      ${footer ? html`
        <footer style="
          height:28px; padding:0 14px;
          display:flex; align-items:center; gap:8px;
          background: rgba(0,0,0,0.20);
          border-top: 1px solid rgba(255,255,255,0.04);
          font-family: var(--font-mono); font-size:10.5px; color:var(--fg-3); letter-spacing:0.01em;
          flex-shrink:0;
        ">${footer}</footer>
      ` : nothing}
    </div>
  `;
}

/* Expose helper globally for non-module callers */
window.lthnRenderChrome = renderChrome;
window.__lthnLitReady = true;

/* Global keyframes — appended once */
if (!document.getElementById("lthn-kf")) {
  const s = document.createElement("style");
  s.id = "lthn-kf";
  s.textContent = `
    @keyframes lthn-pulse { 0%,100%{opacity:1;transform:scale(1);} 50%{opacity:0.55;transform:scale(1.18);} }
    @keyframes lthn-cursor { 0%,100%{opacity:1;} 50%{opacity:0;} }
  `;
  document.head.appendChild(s);
}
