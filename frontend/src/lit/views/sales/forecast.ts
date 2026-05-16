// SPDX-Licence-Identifier: EUPL-1.2
// Sales · Forecast — <lthn-view-forecast>
//
// Forecast surface: 4-card top summary (target / committed / best /
// probability-weighted) over a per-quarter bar chart with committed +
// low-case + best-case + target glyphs.
//
// Monochrome treatment per HANDOVER-VIEWS.md — observational only, no
// goal-shame red-line "you're behind", no celebration toasts. Honest
// numbers + the four lines describing them.
//
// Two-shell: standalone (chrome) when spawned directly, embedded (no
// chrome) when mounted by <lthn-app-shell>. Mirrors the welcome /
// settings window pattern documented in design_two_shell_pattern.md.
//
// Data: fixture arrays only for v1. Real source is a future
// `pkg/sales/forecast` Go service that maps a probability-weighted
// pipeline rollup into the same { quarter, target, committed, best,
// low, status } shape codified here. See HANDOVER-VIEWS.md.

import { LitElement, html } from "lit";
import { renderChrome } from "../../chrome";

/** Per-quarter forecast row. Numbers are in £000 so a pure-number
 *  bar chart can scale all four lines against a single max. */
interface ForecastRow {
  /** Display label ("Q2 · 2026"). */
  q:         string;
  /** Internal target (£000). */
  target:    number;
  /** Already-committed revenue (£000). */
  committed: number;
  /** Best-case projection (£000). */
  best:      number;
  /** Low-case projection (£000). */
  low:       number;
  /** "open" = active quarter we're forecasting against, "plan" = future
   *  quarter still in planning. Drives a subtle dim treatment in v2. */
  status:    "open" | "plan";
}

/** One headline KPI card. */
interface ForecastKpi {
  /** Mono small-caps label ("Y1 target ARR"). */
  l: string;
  /** Pre-formatted value ("£420 K"). */
  v: string;
}

/** Fixture forecast rows. When pkg/sales/forecast lands, swap this
 *  for a fetch + Reactive Controller mirroring chat-window patterns. */
const FIXTURE_ROWS: ForecastRow[] = [
  { q: "Q2 · 2026", target: 200, committed: 142, best: 188, low: 108, status: "open" },
  { q: "Q3 · 2026", target: 300, committed: 64,  best: 280, low: 120, status: "open" },
  { q: "Q4 · 2026", target: 420, committed: 18,  best: 360, low: 140, status: "open" },
  { q: "Q1 · 2027", target: 580, committed: 0,   best: 460, low: 180, status: "plan" },
];

/** Fixture KPIs. Strings stay opaque — the rollup service is the
 *  source-of-truth, not arithmetic on the client. */
const FIXTURE_KPIS: ForecastKpi[] = [
  { l: "Y1 target ARR",         v: "£420 K" },
  { l: "Committed",             v: "£142 K" },
  { l: "Best case",             v: "£280 K" },
  { l: "Probability-weighted",  v: "£196 K" },
];

/** Bar chart max (£000) — picked so the biggest target (580) still
 *  leaves a small right margin. Pure constant so a future v2 chart
 *  can read it from props without re-deriving on every render. */
const CHART_MAX_K = 600;

class LthnViewForecast extends LitElement {
  static readonly properties = {
    w:        { type: Number },
    h:        { type: Number },
    embedded: { type: Boolean, reflect: true },
    /** Fixture data — replace with a live backend call when
     *  pkg/sales/forecast bindings land (Mantis follow-up). */
    rows:     { state: true },
    kpis:     { state: true },
  };

  declare w: number;
  declare h: number;
  declare embedded: boolean;
  declare rows: ForecastRow[];
  declare kpis: ForecastKpi[];

  constructor() {
    super();
    this.w = 1180;
    this.h = 720;
    this.embedded = false;
    this.rows = FIXTURE_ROWS;
    this.kpis = FIXTURE_KPIS;
  }

  createRenderRoot() { return this; }

  async connectedCallback() {
    super.connectedCallback();
    await this._loadFromBackend();
  }

  /** Pull the quarterly forecast from pkg/sales/forecast (shipped in
   *  c7a7c9a). Backend computes probability-weighted rollup from deals.
   *  Degrades to FIXTURE_ROWS + FIXTURE_KPIS on binding-missing. */
  async _loadFromBackend(): Promise<void> {
    try {
      const svc = await import("@desktop/sales/forecast/service").catch(() => null);
      if (!svc || typeof (svc as { Quarterly?: unknown }).Quarterly !== "function") {
        return;
      }
      const r = await (svc as {
        Quarterly: (input: { quarters: number }) => Promise<{
          Value?: { rows?: ForecastRow[]; kpis?: ForecastKpi[] }
        }>
      }).Quarterly({ quarters: 4 });
      const rows = r?.Value?.rows;
      const kpis = r?.Value?.kpis;
      if (rows && rows.length > 0) this.rows = rows;
      if (kpis && kpis.length > 0) this.kpis = kpis;
    } catch {
      // Backend unavailable — keep fixtures.
    }
  }

  render() {
    const rows = this.rows;
    const max  = CHART_MAX_K;

    const body = html`
      <div class="lthn-view-forecast" style="flex:1; padding:18px 22px; display:flex; flex-direction:column; gap:14px; overflow:auto;">
        <!-- top KPI cards -->
        <div class="lthn-view-forecast-kpis"
             style="display:grid; grid-template-columns:repeat(4, 1fr); gap:12px;">
          ${this.kpis.map(k => html`
            <div class="lthn-view-forecast-kpi" data-label=${k.l}
                 style="padding:14px 16px; border-radius:10px;
                        background:rgba(255,255,255,0.025);
                        border:1px solid rgba(255,255,255,0.06);">
              <div style="font-family:var(--font-mono); font-size:9.5px; color:var(--fg-3);
                          letter-spacing:0.10em; text-transform:uppercase;">${k.l}</div>
              <div style="font-family:var(--font-mono); font-size:24px; color:var(--fg-0);
                          font-weight:300; letter-spacing:-0.02em; margin-top:6px;">${k.v}</div>
            </div>
          `)}
        </div>

        <!-- per-quarter chart -->
        <div class="lthn-view-forecast-chart"
             style="padding:18px 22px; border-radius:12px;
                    background:rgba(255,255,255,0.025);
                    border:1px solid rgba(255,255,255,0.06);">
          <div style="font-family:var(--font-mono); font-size:10px; color:var(--fg-3);
                      letter-spacing:0.10em; text-transform:uppercase; margin-bottom:14px;">
            By quarter · £000
          </div>

          ${rows.map(q => html`
            <div class="lthn-view-forecast-row" data-quarter=${q.q} data-status=${q.status}
                 style="display:grid;
                        grid-template-columns: 110px 1fr 80px 80px 80px;
                        gap:16px; padding:14px 0;
                        border-top:1px solid rgba(255,255,255,0.04);
                        align-items:center;">
              <span style="font-family:var(--font-mono); font-size:12px; color:var(--fg-1);">${q.q}</span>

              <!-- bar: three stacked lines (low / committed) + best dot + target tick.
                   Pure inline SVG-style absolutes — keeps the cell self-contained. -->
              <div style="position:relative; height:32px;">
                <!-- track -->
                <div style="position:absolute; top:14px; left:0; right:0; height:4px;
                            background:rgba(255,255,255,0.06); border-radius:2px;"></div>
                <!-- low-case -->
                <div class="lthn-view-forecast-bar-low"
                     style="position:absolute; top:14px; left:0; width:${(q.low / max) * 100}%;
                            height:4px; background:rgba(245,158,11,0.40); border-radius:2px;"></div>
                <!-- committed -->
                <div class="lthn-view-forecast-bar-committed"
                     style="position:absolute; top:14px; left:0; width:${(q.committed / max) * 100}%;
                            height:4px; background:var(--brand-400); border-radius:2px;"></div>
                <!-- target tick -->
                <div class="lthn-view-forecast-bar-target"
                     style="position:absolute; top:8px; left:${(q.target / max) * 100}%;
                            width:2px; height:16px; background:var(--fg-0);"></div>
                <!-- best dot -->
                <div class="lthn-view-forecast-bar-best"
                     style="position:absolute; top:11px; left:${(q.best / max) * 100}%;
                            width:8px; height:8px; border-radius:50%; background:var(--success-400);
                            transform:translate(-50%, 0);"></div>
              </div>

              <span style="font-family:var(--font-mono); font-size:11px; color:var(--fg-1); text-align:right;">£${q.committed}</span>
              <span style="font-family:var(--font-mono); font-size:11px; color:var(--success-400); text-align:right;">£${q.best}</span>
              <span style="font-family:var(--font-mono); font-size:11px; color:var(--fg-0); text-align:right;">£${q.target}</span>
            </div>
          `)}

          <!-- legend — explains the four glyphs honestly, no spin. -->
          <div class="lthn-view-forecast-legend"
               style="display:flex; gap:18px; padding:14px 0 0;
                      border-top:1px solid rgba(255,255,255,0.04); margin-top:6px;">
            <span style="display:flex; align-items:center; gap:6px; font-family:var(--font-mono); font-size:10px; color:var(--fg-3);">
              <span style="width:14px; height:4px; background:var(--brand-400);"></span> committed
            </span>
            <span style="display:flex; align-items:center; gap:6px; font-family:var(--font-mono); font-size:10px; color:var(--fg-3);">
              <span style="width:14px; height:4px; background:rgba(245,158,11,0.40);"></span> low case
            </span>
            <span style="display:flex; align-items:center; gap:6px; font-family:var(--font-mono); font-size:10px; color:var(--fg-3);">
              <span style="width:8px; height:8px; border-radius:50%; background:var(--success-400);"></span> best case
            </span>
            <span style="display:flex; align-items:center; gap:6px; font-family:var(--font-mono); font-size:10px; color:var(--fg-3);">
              <span style="width:2px; height:10px; background:var(--fg-0);"></span> target
            </span>
          </div>
        </div>
      </div>
    `;

    return renderChrome({
      title:    "Forecast",
      subtitle: `${rows.length} quarters · £000`,
      w: this.w, h: this.h,
      embedded: this.embedded,
      body,
      footer: html`probabilities applied per stage · best case = committed + 60% engaged + 30% proposal · refresh weekly`,
    });
  }
}

customElements.define("lthn-view-forecast", LthnViewForecast);
