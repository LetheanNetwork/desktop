// SPDX-Licence-Identifier: EUPL-1.2
// Marketing · Analytics — <lthn-view-analytics>
//
// Web analytics — sessions sparkline, top sources, top pages. Calm +
// observational; no funnel-conversion gauges, no "your traffic is up
// 18%!" celebration banners — the percentage is just stated.
//
// Two-shell: embedded attribute collapses chrome for app-shell mount.
//
// Data: fixture only for v1. Real source is a future
// `pkg/marketing/analytics` Go service wrapping the self-hosted
// Plausible install. The { sources[], pages[], sessions, delta }
// shapes are the contract.
//
// Toolbar exposes a window selector (7d / 30d / 90d). Today only "30d"
// renders real data; the other slots are visible so the future hookup
// is obvious to readers of the code.

import { LitElement, html } from "lit";
import { renderChrome } from "../../chrome";

interface Source {
  label: string;
  v:     number;
  n:     string;
}

interface PageRow {
  p: string;
  v: string;
  b: string;
  d: string;
}

const FIXTURE_SOURCES: Source[] = [
  { label: "Direct",           v: 38, n: "18.4 K" },
  { label: "Hacker News",      v: 24, n: "11.6 K" },
  { label: "r/LocalLLaMA",     v: 14, n: "6.8 K" },
  { label: "Twitter / X",      v: 11, n: "5.3 K" },
  { label: "GitHub README",    v: 8,  n: "3.9 K" },
  { label: "Other",            v: 5,  n: "2.4 K" },
];

const FIXTURE_PAGES: PageRow[] = [
  { p: "/",                    v: "18.2 K", b: "32%", d: "2:14" },
  { p: "/sovereign-compute",   v: "8.8 K",  b: "21%", d: "4:42" },
  { p: "/blog/v0.1-retro",     v: "4.2 K",  b: "18%", d: "6:18" },
  { p: "/docs/install",        v: "3.8 K",  b: "15%", d: "3:54" },
  { p: "/pricing",             v: "2.4 K",  b: "42%", d: "1:32" },
];

const SESSIONS_SPARK = "1.2,1.4,1.6,1.5,1.9,2.1,2.4,2.2,2.6,2.8,3.1,3.3";

type Window = "7d" | "30d" | "90d";

class LthnViewAnalytics extends LitElement {
  static readonly properties = {
    w:        { type: Number },
    h:        { type: Number },
    embedded: { type: Boolean, reflect: true },
    window:   { state: true },
  };
  declare w: number;
  declare h: number;
  declare embedded: boolean;
  /** Reporting window — 7d / 30d / 90d. Only 30d has fixture data
   *  today; the other two render an empty-state row so the future
   *  data-window hookup is visible to anyone reading the code. */
  declare window: Window;

  constructor() {
    super();
    this.w = 1180;
    this.h = 720;
    this.embedded = false;
    this.window = "30d";
  }

  createRenderRoot() { return this; }

  _setWindow(w: Window) { this.window = w; }

  render() {
    const sources = this.window === "30d" ? FIXTURE_SOURCES : [];
    const pages   = this.window === "30d" ? FIXTURE_PAGES   : [];
    const sessions = this.window === "30d" ? "48,412" : "—";
    const windows: Window[] = ["7d", "30d", "90d"];

    const toolbar = html`
      <div class="lthn-view-analytics-windows" style="display:flex; gap:6px; align-items:center;">
        <span style="font-family:var(--font-mono); font-size:10px; color:var(--fg-3); letter-spacing:0.10em; text-transform:uppercase;">window</span>
        ${windows.map(w => html`
          <button
            class="lthn-view-analytics-win-btn"
            data-window=${w}
            data-active=${this.window === w}
            @click=${() => this._setWindow(w)}
            style="--wails-draggable: no-drag; cursor:pointer; padding:4px 10px; border-radius:999px; border:1px solid ${this.window === w ? "var(--brand-300)" : "rgba(255,255,255,0.08)"}; background:${this.window === w ? "rgba(64,193,197,0.10)" : "transparent"}; color:${this.window === w ? "var(--brand-300)" : "var(--fg-2)"}; font-family:var(--font-mono); font-size:10.5px; letter-spacing:0.04em;">
            ${w}
          </button>
        `)}
      </div>
    `;

    const body = html`
      <div class="lthn-view-analytics" style="flex:1; padding:18px 22px; display:grid; grid-template-columns: 1fr 1fr; gap:14px; overflow:auto;">
        <div class="lthn-view-analytics-sources" style="padding:18px 22px; border-radius:12px; background:rgba(255,255,255,0.025); border:1px solid rgba(255,255,255,0.06);">
          <div style="display:flex; justify-content:space-between; align-items:baseline; margin-bottom:18px;">
            <div>
              <div style="font-family:var(--font-mono); font-size:10px; color:var(--fg-3); letter-spacing:0.10em; text-transform:uppercase;">Sessions · ${this.window}</div>
              <div style="font-family:var(--font-mono); font-size:32px; color:var(--fg-0); font-weight:300; letter-spacing:-0.02em; margin-top:4px;">${sessions}</div>
              <div style="font-size:11.5px; color:var(--success-400); margin-top:2px;">${this.window === "30d" ? "+18 % vs prior 30 days" : "no data for this window yet"}</div>
            </div>
            <lthn-sparkline data=${SESSIONS_SPARK} max="4" color="var(--brand-400)" .width=${200} .height=${72}></lthn-sparkline>
          </div>
          <div style="font-family:var(--font-mono); font-size:10px; color:var(--fg-3); letter-spacing:0.10em; text-transform:uppercase; margin-bottom:10px;">Top sources</div>
          ${sources.length === 0 ? html`<div class="lthn-view-analytics-empty" style="padding:14px 0; font-family:var(--font-mono); font-size:11px; color:var(--fg-3);">no data for ${this.window} — switch window or wait for backend</div>` : sources.map(s => html`
            <div class="lthn-view-analytics-source-row" data-source=${s.label} style="display:grid; grid-template-columns: 1fr 80px 60px; gap:12px; align-items:center; padding:6px 0;">
              <span style="font-size:12.5px; color:var(--fg-1);">${s.label}</span>
              <div style="height:6px; border-radius:3px; background:rgba(255,255,255,0.06); overflow:hidden;">
                <div style="width:${s.v * 2.4}%; height:100%; background:var(--brand-400);"></div>
              </div>
              <span style="font-family:var(--font-mono); font-size:11px; color:var(--fg-2); text-align:right;">${s.n}</span>
            </div>
          `)}
        </div>
        <div class="lthn-view-analytics-pages" style="padding:18px 22px; border-radius:12px; background:rgba(255,255,255,0.025); border:1px solid rgba(255,255,255,0.06);">
          <div style="font-family:var(--font-mono); font-size:10px; color:var(--fg-3); letter-spacing:0.10em; text-transform:uppercase; margin-bottom:14px;">Top pages</div>
          <div style="display:grid; grid-template-columns: 1fr 70px 50px 60px; gap:10px; padding:0 0 8px 0; border-bottom:1px solid rgba(255,255,255,0.06); font-family:var(--font-mono); font-size:9.5px; color:var(--fg-3); letter-spacing:0.08em; text-transform:uppercase;">
            <span>Page</span><span style="text-align:right;">Views</span><span style="text-align:right;">Bounce</span><span style="text-align:right;">Time</span>
          </div>
          ${pages.length === 0 ? html`<div class="lthn-view-analytics-empty" style="padding:14px 0; font-family:var(--font-mono); font-size:11px; color:var(--fg-3);">no data for ${this.window}</div>` : pages.map(p => html`
            <div class="lthn-view-analytics-page-row" data-page=${p.p} style="display:grid; grid-template-columns: 1fr 70px 50px 60px; gap:10px; padding:10px 0; border-bottom:1px solid rgba(255,255,255,0.04); align-items:center;">
              <span style="font-family:var(--font-mono); font-size:12px; color:var(--fg-1);">${p.p}</span>
              <span style="font-family:var(--font-mono); font-size:11px; color:var(--fg-0); text-align:right;">${p.v}</span>
              <span style="font-family:var(--font-mono); font-size:11px; color:var(--fg-2); text-align:right;">${p.b}</span>
              <span style="font-family:var(--font-mono); font-size:11px; color:var(--fg-2); text-align:right;">${p.d}</span>
            </div>
          `)}
        </div>
      </div>
    `;

    return renderChrome({
      title:    "Analytics",
      subtitle: `web · ${this.window}`,
      w: this.w, h: this.h,
      embedded: this.embedded,
      toolbar,
      body,
      footer: html`cookieless · GDPR-compliant · self-hosted Plausible · refreshes hourly`,
    });
  }
}

customElements.define("lthn-view-analytics", LthnViewAnalytics);
