// SPDX-Licence-Identifier: EUPL-1.2
// E2.3 · telemetry — <lthn-telemetry-window>
// Light-DOM Lit element. Composes renderChrome() from ../chrome.js.

import { LitElement, html, nothing } from "lit";
import { renderChrome } from "../chrome";
import { T } from "@lthn/i18n/coreservice";

/** Parse the "1s" / "2s" / "5s" / "off" picker value into a setInterval
 *  delay in ms. "off" → 0 so the caller can skip scheduling. Unknown
 *  values fall back to 2s — same default the tray + Settings ship. */
function parseInterval(v: string): number {
  if (v === "off") return 0;
  const m = /^(\d+)s$/.exec(v);
  if (!m) return 2000;
  return parseInt(m[1], 10) * 1000;
}

class LthnTelemetryWindow extends LitElement {
  static properties = {
    w: { type: Number },
    h: { type: Number },
    fullscreen: { type: Boolean },
    embedded: { type: Boolean, reflect: true },
    chrome: { state: true },
    sample: { state: true },
    heapHistory: { state: true },
    goHistory: { state: true },
    model: { state: true },
    err: { state: true },
    t: { state: true },
  };
  declare w: number;
  declare h: number;
  declare fullscreen: boolean;
  declare embedded: boolean;
  declare chrome: { title: string; subtitleNormal: string; subtitleFullscreen: string };
  declare sample: { heap_alloc_mb: number; uptime_seconds: number; num_goroutines: number; num_cgo_calls: number };
  declare heapHistory: number[];
  declare goHistory: number[];
  declare model: string;
  declare err: string;
  declare t: {
    bigHeapL: string; bigHeapS: string;
    bigUptimeL: string; bigUptimeS: string;
    rowModel: string; rowGoroutines: string; rowCgo: string;
    statusOk: string; statusErr: string;
    tagline: string; footerNormal: string;
  };

  private _pollTimer: number | null = null;

  constructor() {
    super();
    this.w = 880; this.h = 560; this.fullscreen = false; this.embedded = false;
    this.chrome = {
      title: "Live telemetry",
      subtitleNormal: "demo surface",
      subtitleFullscreen: "fullscreen · ⎋ to exit",
    };
    this.sample = { heap_alloc_mb: 0, uptime_seconds: 0, num_goroutines: 0, num_cgo_calls: 0 };
    this.heapHistory = [];
    this.goHistory = [];
    this.model = "—";
    this.err = "";
    this.t = {
      bigHeapL: "heap · MB", bigHeapS: "Go runtime · live",
      bigUptimeL: "uptime", bigUptimeS: "since process start",
      rowModel: "model", rowGoroutines: "goroutines", rowCgo: "cgo calls",
      statusOk: "airplane-mode OK", statusErr: "telemetry error",
      tagline: "lthn · sovereign · single-watt",
      footerNormal: "model · %s · goroutines %d · airplane-mode OK · ⌥⌘F for fullscreen",
    };
  }
  createRenderRoot() { return this; }
  async connectedCallback() {
    super.connectedCallback();
    const [
      title, subN, subF,
      bhL, bhS, buL, buS, rMo, rGo, rCg, sOk, sErr, tag, fNorm,
    ] = await Promise.all([
      T("window.telemetry.title"),
      T("window.telemetry.subtitle_normal"),
      T("window.telemetry.subtitle_fullscreen"),
      T("window.telemetry.big_heap_label"),
      T("window.telemetry.big_heap_sub"),
      T("window.telemetry.big_uptime_label"),
      T("window.telemetry.big_uptime_sub"),
      T("window.telemetry.row_model"),
      T("window.telemetry.row_goroutines"),
      T("window.telemetry.row_cgo"),
      T("window.telemetry.status_ok"),
      T("window.telemetry.status_err"),
      T("window.telemetry.tagline"),
      T("window.telemetry.footer_normal"),
    ]);
    this.chrome = { title, subtitleNormal: subN, subtitleFullscreen: subF };
    this.t = {
      bigHeapL: bhL, bigHeapS: bhS,
      bigUptimeL: buL, bigUptimeS: buS,
      rowModel: rMo, rowGoroutines: rGo, rowCgo: rCg,
      statusOk: sOk, statusErr: sErr,
      tagline: tag, footerNormal: fNorm,
    };
    void this._poll();
    // Cadence + rolling-window size live in localStorage, written by
    // Settings → Telemetry. "off" disables polling outright — the
    // sample stays fixed on whatever the first read returned.
    const interval = parseInterval(localStorage.getItem("lthn.telemetry.interval") || "2s");
    if (interval > 0) {
      this._pollTimer = window.setInterval(() => void this._poll(), interval);
    }
  }
  disconnectedCallback() {
    super.disconnectedCallback();
    if (this._pollTimer !== null) {
      clearInterval(this._pollTimer);
      this._pollTimer = null;
    }
  }

  /** One telemetry tick + model refresh. Pushes onto the rolling
   *  history arrays the big-number sparklines read from. 24 samples
   *  × 2s ≈ a 48-second rolling window — same cadence as the tray. */
  async _poll() {
    try {
      const [tel, runner] = await Promise.all([
        import("@desktop/telemetry/service"),
        import("@desktop/runner/service"),
      ]);
      const reading = await tel.CurrentSample();
      const models = await runner.WModels().catch((): string[] => []);
      this.sample = {
        heap_alloc_mb: reading.heap_alloc_mb || 0,
        uptime_seconds: reading.uptime_seconds || 0,
        num_goroutines: reading.num_goroutines || 0,
        num_cgo_calls: reading.num_cgo_calls || 0,
      };
      const heapNext = [...this.heapHistory, this.sample.heap_alloc_mb];
      const goNext = [...this.goHistory, this.sample.num_goroutines];
      const cap = Math.max(4, parseInt(localStorage.getItem("lthn.telemetry.samples") || "24", 10) || 24);
      this.heapHistory = heapNext.length > cap ? heapNext.slice(-cap) : heapNext;
      this.goHistory = goNext.length > cap ? goNext.slice(-cap) : goNext;
      this.model = models?.[0] || "no model loaded";
      this.err = "";
    } catch (e: unknown) {
      this.err = e instanceof Error ? e.message : String(e);
    }
  }

  /** Format an uptime in seconds the way the tray does, but with
   *  enough headroom to read long sessions cleanly in the big-number
   *  display. < 60s → "Ns"; < 1h → "Nm Ms"; otherwise "Nh Mm". */
  _fmtUptime(s: number): string {
    if (s < 60) return `${Math.trunc(s)}s`;
    if (s < 3600) return `${Math.trunc(s / 60)}m ${Math.trunc(s % 60)}s`;
    return `${Math.trunc(s / 3600)}h ${Math.trunc((s % 3600) / 60)}m`;
  }

  render() {
    const heapSpark = this.heapHistory.length > 1 ? this.heapHistory.join(",") : "";
    const goSpark   = this.goHistory.length > 1 ? this.goHistory.join(",") : "";
    const heapMax = Math.max(1, ...this.heapHistory) * 1.2;
    const goMax   = Math.max(1, ...this.goHistory) * 1.2;

    const big = (label: string, value: string, sub: string, glow: string, data: string, max: number) => html`
      <div style="display:flex; flex-direction:column; align-items:center; gap:10px;">
        <div style="font-family:var(--font-mono); font-size:10.5px; color:var(--fg-3); letter-spacing:0.20em; text-transform:uppercase;">${label}</div>
        <div style="font-family:var(--font-mono); font-size:92px; font-weight:300; color:var(--fg-0); letter-spacing:-0.04em; line-height:1; text-shadow:0 0 30px ${glow}55, 0 0 60px ${glow}22;">${value}</div>
        <lthn-sparkline data=${data} color=${glow} max=${max} .width=${320} .height=${36} fill></lthn-sparkline>
        <div style="font-size:11.5px; color:var(--fg-3);">${sub}</div>
      </div>
    `;

    const body = html`
      <div style="flex:1; background:radial-gradient(circle at 50% 35%, rgba(64,193,197,0.10) 0%, rgba(11,16,22,0) 60%), var(--surf-0); display:flex; flex-direction:column; align-items:center; justify-content:center; padding:40px 60px; gap:36px; position:relative; overflow:hidden;">
        <div style="display:grid; grid-template-columns:1fr 1fr; gap:64px; width:100%;">
          ${big(this.t.bigHeapL, this.sample.heap_alloc_mb.toFixed(1), this.t.bigHeapS, "var(--brand-400)", heapSpark, heapMax)}
          ${big(this.t.bigUptimeL, this._fmtUptime(this.sample.uptime_seconds), this.t.bigUptimeS, "#a78bfa", goSpark, goMax)}
        </div>
        <div style="display:flex; gap:28px; align-items:center; font-family:var(--font-mono); font-size:12px; color:var(--fg-2); padding-top:8px; border-top:1px solid rgba(255,255,255,0.05); width:100%; justify-content:center;">
          <div><span style="color:var(--fg-3);">${this.t.rowModel} </span><span style="color:var(--fg-0);">${this.model}</span></div>
          <div style="width:1px; height:14px; background:rgba(255,255,255,0.06);"></div>
          <div><span style="color:var(--fg-3);">${this.t.rowGoroutines} </span><span style="color:var(--fg-0);">${this.sample.num_goroutines}</span></div>
          <div style="width:1px; height:14px; background:rgba(255,255,255,0.06);"></div>
          <div><span style="color:var(--fg-3);">${this.t.rowCgo} </span><span style="color:var(--fg-0);">${this.sample.num_cgo_calls}</span></div>
          <div style="width:1px; height:14px; background:rgba(255,255,255,0.06);"></div>
          <div style="display:flex; align-items:center; gap:6px;">
            <lthn-status-dot variant=${this.err ? "err" : "ok"}></lthn-status-dot>
            <span style="color:${this.err ? "var(--error-400)" : "var(--success-400)"};">${this.err ? this.t.statusErr : this.t.statusOk}</span>
          </div>
        </div>
        <div style="position:absolute; bottom:18px; right:24px; display:flex; align-items:center; gap:8px; font-family:var(--font-mono); font-size:10px; color:var(--fg-3); letter-spacing:0.06em;">
          <lthn-glyph size="12" color="var(--fg-3)"></lthn-glyph>
          ${this.t.tagline}
        </div>
      </div>
    `;
    return renderChrome({
      title: this.chrome.title,
      subtitle: this.fullscreen ? this.chrome.subtitleFullscreen : this.chrome.subtitleNormal,
      w: this.w, h: this.h, body,
      footer: html`${this.t.footerNormal.replace("%s", this.model).replace("%d", String(this.sample.num_goroutines))}`,
      embedded: this.embedded,
    });
  }
}
customElements.define("lthn-telemetry-window", LthnTelemetryWindow);
