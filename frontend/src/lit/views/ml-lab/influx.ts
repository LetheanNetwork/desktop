// SPDX-Licence-Identifier: EUPL-1.2

// <lthn-view-ml-lab-influx> — Influx tab of the ML Lab workbench,
// per plans/project/lthn/desktop/RFC.ml-lab.md §4. Editor on top
// (multi-line Flux / InfluxQL), result panel below (line chart
// when shape suggests a time-series, table otherwise).
//
// Backend: POST /v1/ml-lab/influx with InfluxQuery body. Mock
// provider returns 60 points of decaying loss + ascending
// throughput so the chart panel renders meaningful shapes without
// a real InfluxDB wired. Default editor body is the canonical
// "training loss by run" query the topbar time-range binds into.

import { LitElement, html, type TemplateResult } from "lit";
import { renderChrome } from "../../chrome";
import { apiFetch } from "../../api-fetch";

interface ColumnSpec {
  name: string;
  kind: string;
}
interface InfluxResult {
  columns:    ColumnSpec[];
  rows:       Array<Array<string | number | boolean | null>>;
  chart_hint: string;
}

const DEFAULT_FLUX = `from(bucket: "train")
  |> range(start: \${start}, stop: \${stop})
  |> filter(fn: (r) => r._measurement == "loss")
  |> aggregateWindow(every: 1m, fn: mean)`;

// FIXTURE_RESULT lights the chart before any backend call lands —
// matches the mock provider's deterministic 8-point envelope so the
// frontend test can assert the chart renders without a live backend.
const FIXTURE_RESULT: InfluxResult = {
  columns: [
    { name: "ts",             kind: "time" },
    { name: "loss",           kind: "float" },
    { name: "throughput_tps", kind: "float" },
  ],
  rows: [
    ["2026-05-24T09:00:00Z", 2.41, 38.2],
    ["2026-05-24T09:05:00Z", 1.92, 42.6],
    ["2026-05-24T09:10:00Z", 1.52, 47.3],
    ["2026-05-24T09:15:00Z", 1.18, 51.8],
    ["2026-05-24T09:20:00Z", 0.91, 55.4],
    ["2026-05-24T09:25:00Z", 0.71, 58.9],
    ["2026-05-24T09:30:00Z", 0.57, 60.7],
    ["2026-05-24T09:35:00Z", 0.46, 62.1],
  ],
  chart_hint: "line",
};

class LthnViewMlLabInflux extends LitElement {
  static readonly properties = {
    embedded:       { type: Boolean, reflect: true },
    selectedModel:  { type: String, attribute: "selected-model", reflect: true },
    selectedRun:    { type: String, attribute: "selected-run", reflect: true },
    timeRangeStart: { type: String, attribute: "time-range-start", reflect: true },
    timeRangeStop:  { type: String, attribute: "time-range-stop", reflect: true },
    body:           { state: true },
    result:         { state: true },
    loading:        { state: true },
    error:          { state: true },
  };
  declare embedded: boolean;
  declare selectedModel: string;
  declare selectedRun: string;
  declare timeRangeStart: string;
  declare timeRangeStop: string;
  declare body: string;
  declare result: InfluxResult;
  declare loading: boolean;
  declare error: string;

  constructor() {
    super();
    this.embedded = false;
    this.selectedModel = "";
    this.selectedRun = "";
    this.timeRangeStart = "";
    this.timeRangeStop = "";
    this.body = DEFAULT_FLUX;
    this.result = FIXTURE_RESULT;
    this.loading = false;
    this.error = "";
  }
  createRenderRoot() { return this; }

  async connectedCallback(): Promise<void> {
    super.connectedCallback();
    // Defer first auto-run so attribute reflection from the shell
    // (selected-model / time-range-*) lands before the request.
    await Promise.resolve();
    await this._run();
  }

  private async _run(): Promise<void> {
    this.loading = true;
    this.error = "";
    try {
      const res = await apiFetch("/v1/ml-lab/influx", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          body: this.body,
          time_range: {
            start: this.timeRangeStart,
            stop:  this.timeRangeStop,
          },
        }),
      });
      if (!res.ok) {
        this.error = `HTTP ${res.status}`;
        return;
      }
      const result = await res.json() as InfluxResult;
      // Keep the fixture if the backend returned an empty shape so
      // the chart stays populated during early-boot.
      if (result.rows && result.rows.length > 0) {
        this.result = result;
      }
    } catch (e) {
      this.error = (e as Error).message ?? "offline";
    } finally {
      this.loading = false;
    }
  }

  private _onBodyInput(e: Event): void {
    this.body = (e.target as HTMLTextAreaElement).value;
  }
  private _onRun(): void {
    void this._run();
  }
  private _onReset(): void {
    this.body = DEFAULT_FLUX;
  }

  // _renderChart draws a tiny inline SVG line-chart of column 1
  // (typically loss) and column 2 (typically throughput) against
  // the time index. No d3, no chart library — keeps the bundle
  // light and the chart shape readable at small sizes. Scales to
  // the container width via 100% SVG.
  private _renderChart(): TemplateResult {
    const rows = this.result.rows;
    if (rows.length < 2) {
      return html`<div style="padding:24px;color:var(--colour-text-tertiary);font-size:12px;">not enough points to chart (need ≥ 2)</div>`;
    }
    const cols = this.result.columns;
    // Find first numeric column for the primary series, second for
    // secondary. Fall back to the simplest two-series shape.
    const seriesIdx: number[] = [];
    for (let i = 0; i < cols.length && seriesIdx.length < 2; i++) {
      const k = cols[i].kind;
      if (k === "float" || k === "int") seriesIdx.push(i);
    }
    if (seriesIdx.length === 0) {
      return html`<div style="padding:24px;color:var(--colour-text-tertiary);font-size:12px;">no numeric column to plot</div>`;
    }
    const series = seriesIdx.map(i => rows.map(r => Number(r[i] ?? 0)));
    const w = 800;
    const h = 240;
    const pad = 24;
    const innerW = w - 2 * pad;
    const innerH = h - 2 * pad;
    const colourFor = ["#ff6b9b", "#7dd6ff"];
    const seriesPaths = series.map((vals, sIdx) => {
      const lo = Math.min(...vals);
      const hi = Math.max(...vals);
      const span = hi - lo || 1;
      const step = innerW / (vals.length - 1);
      let d = "";
      for (let i = 0; i < vals.length; i++) {
        const x = pad + i * step;
        const y = pad + innerH - ((vals[i] - lo) / span) * innerH;
        d += `${i === 0 ? "M" : "L"} ${x.toFixed(2)} ${y.toFixed(2)} `;
      }
      return html`
        <path d=${d} stroke=${colourFor[sIdx]} stroke-width="1.5" fill="none" />
        <text x="${pad + 4}" y="${pad + 12 + sIdx * 14}" fill=${colourFor[sIdx]} font-size="10" font-family="var(--font-mono)">
          ${cols[seriesIdx[sIdx]].name}
        </text>
      `;
    });
    return html`
      <div style="padding:12px 18px;">
        <svg viewBox="0 0 ${w} ${h}" preserveAspectRatio="none" style="width:100%;height:240px;background:var(--colour-surface-1);border:1px solid var(--colour-border-subtle);border-radius:6px;">
          ${seriesPaths}
        </svg>
      </div>
    `;
  }

  private _renderTable(): TemplateResult {
    return html`
      <div style="overflow:auto;flex:1;font-family:var(--font-mono);font-size:11px;">
        <table style="width:100%;border-collapse:collapse;">
          <thead style="background:var(--colour-surface-2);position:sticky;top:0;">
            <tr>
              ${this.result.columns.map(c => html`
                <th style="text-align:left;padding:6px 10px;color:var(--colour-text-secondary);font-weight:500;border-bottom:1px solid var(--colour-border);">
                  ${c.name}
                  <span style="color:var(--colour-text-tertiary);font-weight:400;margin-left:6px;">${c.kind}</span>
                </th>
              `)}
            </tr>
          </thead>
          <tbody>
            ${this.result.rows.slice(0, 200).map(r => html`
              <tr>
                ${r.map(v => html`
                  <td style="padding:4px 10px;color:var(--colour-text-primary);border-bottom:1px solid var(--colour-border-subtle);">
                    ${v === null ? "·" : String(v)}
                  </td>
                `)}
              </tr>
            `)}
          </tbody>
        </table>
      </div>
    `;
  }

  render() {
    return renderChrome({
      title: "Influx",
      embedded: this.embedded,
      body: html`
        <div style="display:flex;flex-direction:column;height:100%;">
          <div style="border-bottom:1px solid var(--colour-border);padding:10px 14px;">
            <textarea
              .value=${this.body}
              @input=${this._onBodyInput}
              spellcheck="false"
              style="width:100%;height:100px;background:var(--colour-surface-1);border:1px solid var(--colour-border);color:var(--colour-text-primary);padding:8px 12px;border-radius:6px;font-family:var(--font-mono);font-size:12px;resize:vertical;outline:none;"
            ></textarea>
            <div style="display:flex;gap:8px;margin-top:8px;align-items:center;">
              <button type="button" @click=${this._onRun} ?disabled=${this.loading}
                style="background:var(--colour-accent);color:var(--colour-text-inverse);border:none;padding:6px 14px;border-radius:6px;font-size:12px;cursor:pointer;font-weight:500;">
                ${this.loading ? "Running…" : "Run"}
              </button>
              <button type="button" @click=${this._onReset}
                style="background:transparent;color:var(--colour-text-secondary);border:1px solid var(--colour-border);padding:6px 14px;border-radius:6px;font-size:12px;cursor:pointer;">
                Reset
              </button>
              ${this.error
                ? html`<span style="color:var(--colour-danger);font-size:11px;">${this.error}</span>`
                : html`<span style="color:var(--colour-text-tertiary);font-size:11px;">${this.result.rows.length} rows</span>`}
            </div>
          </div>
          ${this.result.chart_hint === "line" ? this._renderChart() : null}
          ${this._renderTable()}
        </div>
      `,
    });
  }
}

if (!customElements.get("lthn-view-ml-lab-influx")) {
  customElements.define("lthn-view-ml-lab-influx", LthnViewMlLabInflux);
}

export { LthnViewMlLabInflux };
