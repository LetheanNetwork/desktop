// SPDX-Licence-Identifier: EUPL-1.2
// E2.1 · benchmark — <lthn-benchmark-window>
// Light-DOM Lit element. Composes renderChrome() from ../chrome.js.
//
// Wires to pkg/benchmark substrate (commit fd4b9c7 + 58d2179). The
// substrate is runner-agnostic — Run rows can come from any registered
// Bencher (our lthn-mlx runner, llama.cpp, ollama, NIM, opencode,
// future go-ai endpoints). The fixture rows below stay as fallback
// for the first launch (empty history) and for tests so the design
// surface keeps working when no adapter has registered yet.
//
// Wire pattern follows [[design_lit_view_backend_wire_pattern]]:
//   * lazy dynamic import on connectedCallback
//   * fixture default + fallback on missing / rejecting / empty
//   * no polling (aggregate view — refreshes on user action)

import { LitElement, html, nothing } from "lit";
import { renderChrome } from "../chrome";
import { T } from "@lthn/i18n/coreservice";

/** Row view-model the template renders. Mirrors the on-disk Run
 *  shape but trims to what the table needs and pre-formats fields
 *  for display (timestamp, mem). */
interface RowVM {
  id?: string;
  ts: string;       // pretty timestamp
  bencher: string;  // which Bencher produced this row (substrate-stamped)
  model: string;
  pp: number;       // pp_tok_sec
  tg: number;       // tg_tok_sec
  w: number;        // peak_watts (0 = not measurable, e.g. remote bencher)
  mem: string;      // peak_mem_mb formatted ("2.4 GB") or "—" if 0
  here?: boolean;   // latest row marker
}

interface BencherInfoVM {
  name: string;
  kind: string;
  description?: string;
}

/** Backend Run shape (subset). Mirrors go/pkg/benchmark.Run JSON tags. */
interface RunDTO {
  id?: string;
  timestamp?: string;        // RFC3339
  bencher?: string;
  model?: string;
  ctx?: number;
  pp_tok_sec?: number;
  tg_tok_sec?: number;
  prompt_len?: number;
  output_len?: number;
  peak_watts?: number;
  peak_mem_mb?: number;
  endpoint?: string;
}

/** Fixture runs — design rows that keep the surface believable on
 *  first launch (empty backend) AND during tests. Bencher field reads
 *  "fixture" so an operator can tell at a glance these are not real
 *  measurements. */
const FIXTURE_RUNS: RowVM[] = [
  { ts: "2026-05-11 14:32", bencher: "fixture", model: "gemma-4-e2b",  pp: 4820, tg: 47.2, w: 8.4,  mem: "2.4 GB", here: true },
  { ts: "2026-05-11 09:14", bencher: "fixture", model: "gemma-4-e2b",  pp: 4780, tg: 46.8, w: 8.5,  mem: "2.4 GB" },
  { ts: "2026-05-10 18:02", bencher: "fixture", model: "llama-3.2-3b", pp: 3140, tg: 32.6, w: 11.8, mem: "3.6 GB" },
  { ts: "2026-05-09 21:55", bencher: "fixture", model: "phi-3.5-mini", pp: 3960, tg: 38.4, w: 9.6,  mem: "2.9 GB" },
  { ts: "2026-05-08 11:18", bencher: "fixture", model: "gemma-4-e2b",  pp: 4640, tg: 45.1, w: 8.6,  mem: "2.4 GB" },
];

/** Fixture curve (tg-vs-ctx) — kept as a separate fallback because
 *  the substrate today persists individual Runs, not curve sweeps.
 *  Future RunCurve work will derive this from History grouping. */
const FIXTURE_CURVE = [
  { ctx: 128,  tg: 51.8 }, { ctx: 512,  tg: 50.4 }, { ctx: 1024, tg: 48.6 },
  { ctx: 2048, tg: 47.2 }, { ctx: 4096, tg: 43.1 }, { ctx: 6144, tg: 38.4 }, { ctx: 8192, tg: 33.6 },
];

/** Format a DTO Run into a display row. Empty / missing fields fall
 *  back to "—" so the table never renders bare undefined. */
function dtoToRow(d: RunDTO, isLatest: boolean): RowVM {
  return {
    id: d.id,
    ts: formatTimestamp(d.timestamp),
    bencher: d.bencher || "—",
    model: d.model || "—",
    pp: d.pp_tok_sec ?? 0,
    tg: d.tg_tok_sec ?? 0,
    w: d.peak_watts ?? 0,
    mem: formatMem(d.peak_mem_mb),
    here: isLatest,
  };
}

/** "2026-05-11T14:32:00Z" → "2026-05-11 14:32". Cheap + locale-safe. */
function formatTimestamp(ts: string | undefined): string {
  if (!ts) return "—";
  // RFC3339 → "YYYY-MM-DD HH:MM"
  const m = /^(\d{4}-\d{2}-\d{2})T(\d{2}:\d{2})/.exec(ts);
  return m ? `${m[1]} ${m[2]}` : ts.slice(0, 16);
}

/** Peak memory MB → "2.4 GB" / "640 MB" / "—" when zero. */
function formatMem(mb: number | undefined): string {
  if (!mb || mb <= 0) return "—";
  return mb >= 1024 ? `${(mb / 1024).toFixed(1)} GB` : `${Math.round(mb)} MB`;
}

/** Default ctx for the Run button. Matches the standard benchmark
 *  bucket; the picker UI will let users pick something else later. */
const DEFAULT_RUN_CTX = 2048;

/** Default max output tokens. Matches most benchmark tools' convention. */
const DEFAULT_RUN_MAX_OUTPUT = 256;

/** Sample prompt for one-shot Run. Deterministic across machines so
 *  comparing benchmarks across hardware is meaningful. ~80 words ≈
 *  ~200 tokens — enough to exercise the prompt path without dominating
 *  the run time. Future RunCurve work ships a canonical corpus that
 *  scales to fill the requested ctx; this is the MVP equivalent. */
const SAMPLE_PROMPT =
  "Explain the difference between a relational database and a document database. " +
  "Cover schema flexibility, ACID guarantees, query patterns, scaling characteristics, " +
  "and the kinds of workloads each is best suited to. Keep the answer under three " +
  "paragraphs and use concrete examples rather than abstract definitions. Aim for " +
  "the level of a working senior engineer choosing between PostgreSQL and MongoDB " +
  "for a new product surface; assume the reader knows SQL but has not used a " +
  "document store before.";

class LthnBenchmarkWindow extends LitElement {
  static readonly properties = {
    w: { type: Number },
    h: { type: Number },
    embedded: { type: Boolean, reflect: true },
    chrome: { state: true },
    t: { state: true },
    runs: { state: true },
    benchers: { state: true },
    models: { state: true },
    selectedBencher: { state: true },
    selectedModel: { state: true },
    running: { state: true },
    runErr: { state: true },
  };
  declare w: number;
  declare h: number;
  declare embedded: boolean;
  declare chrome: { title: string; subtitle: string };
  declare runs: RowVM[];
  declare benchers: BencherInfoVM[];
  declare models: string[];
  declare selectedBencher: string;
  declare selectedModel: string;
  declare running: boolean;
  declare runErr: string;
  declare t: {
    btnPp: string; btnTg: string; btnBoth: string; btnRun: string; btnExport: string;
    labelRecent: string; labelCompare: string;
    colTs: string; colBencher: string; colModel: string; colPp: string; colTg: string; colPeakW: string; colMem: string;
    pillLatest: string;
    labelChart: string; labelLogX: string;
    yAxis: string; footer: string;
  };

  constructor() {
    super();
    this.w = 1000; this.h = 660; this.embedded = false;
    this.chrome = { title: "Benchmark", subtitle: "run · compare · export" };
    this.runs = FIXTURE_RUNS;
    this.benchers = [];
    this.models = [];
    this.selectedBencher = "";
    this.selectedModel = "";
    this.running = false;
    this.runErr = "";
    this.t = {
      btnPp: "PP only", btnTg: "TG only", btnBoth: "Both",
      btnRun: "Run", btnExport: "Export",
      labelRecent: "Recent runs · click to overlay on chart",
      labelCompare: "2 selected · compare mode",
      colTs: "Timestamp", colBencher: "Bencher", colModel: "Model",
      colPp: "PP tok/s", colTg: "TG tok/s",
      colPeakW: "Peak W", colMem: "Mem",
      pillLatest: "Latest",
      labelChart: "tok/s vs context length",
      labelLogX: "· log scale on x",
      yAxis: "tok/s",
      footer: "fixture rows · substrate awaits registered Bencher · ~/Lethean/orm.duckdb",
    };
  }
  createRenderRoot() { return this; }

  async connectedCallback() {
    super.connectedCallback();
    const [
      title, subtitle, bPp, bTg, bBoth, bRun, bExport,
      lRecent, lCompare,
      cTs, cBencher, cModel, cPp, cTg, cPeakW, cMem,
      pLatest, lChart, lLogX, yAx, foot,
    ] = await Promise.all([
      T("window.benchmark.title"), T("window.benchmark.subtitle"),
      T("window.benchmark.btn_pp_only"), T("window.benchmark.btn_tg_only"),
      T("window.benchmark.btn_both"), T("window.benchmark.btn_run"),
      T("window.benchmark.btn_export"),
      T("window.benchmark.label_recent"), T("window.benchmark.label_compare"),
      T("window.benchmark.col_timestamp"), T("window.benchmark.col_bencher"),
      T("window.benchmark.col_model"),
      T("window.benchmark.col_pp"), T("window.benchmark.col_tg"),
      T("window.benchmark.col_peak_w"), T("window.benchmark.col_mem"),
      T("window.benchmark.pill_latest"),
      T("window.benchmark.label_chart"), T("window.benchmark.label_log_x"),
      T("window.benchmark.y_axis"), T("window.benchmark.footer"),
    ]);
    this.chrome = { title, subtitle };
    this.t = {
      btnPp: bPp, btnTg: bTg, btnBoth: bBoth, btnRun: bRun, btnExport: bExport,
      labelRecent: lRecent, labelCompare: lCompare,
      colTs: cTs, colBencher: cBencher, colModel: cModel,
      colPp: cPp, colTg: cTg, colPeakW: cPeakW, colMem: cMem,
      pillLatest: pLatest,
      labelChart: lChart, labelLogX: lLogX,
      yAxis: yAx, footer: foot,
    };
    void this._loadFromBackend();
  }

  /** Pull History + ListBenchers from the substrate. Per the canonical
   *  wire pattern: empty / reject / missing-binding keeps fixture rows
   *  so the design surface stays believable. Non-empty replaces.
   *  Also auto-selects the first bencher + loads its models so the
   *  picker is ready for a Run click. */
  async _loadFromBackend(): Promise<void> {
    try {
      const [benchMod, resultMod] = await Promise.all([
        import("@desktop/benchmark/service"),
        import("../result"),
      ]);
      const { unwrap } = resultMod;
      const [runs, infos] = await Promise.all([
        unwrap<RunDTO[]>(benchMod.History({ Limit: 50 } as Parameters<typeof benchMod.History>[0]), []),
        unwrap<BencherInfoVM[]>(benchMod.ListBenchers(), []),
      ]);
      this.benchers = infos || [];
      if (Array.isArray(runs) && runs.length > 0) {
        this.runs = runs.map((d, i) => dtoToRow(d, i === 0));
      }
      // Auto-pick the first bencher + load its models so the Run path
      // is immediately usable without an extra click. If the previously-
      // selected bencher is still registered, keep it (preserves UI
      // state across History refreshes after a Run completion). The
      // await is load-bearing — kicking _loadModels off fire-and-forget
      // races against tests + against the user clicking Run before
      // models populate.
      if (this.benchers.length > 0) {
        const stillThere = this.benchers.find(b => b.name === this.selectedBencher);
        if (!stillThere) {
          this.selectedBencher = this.benchers[0].name;
          this.selectedModel = "";
        }
        await this._loadModels(this.selectedBencher);
      }
    } catch {
      // missing binding / reject → keep fixture (per design memo)
    }
  }

  /** Fetch the model list for the named bencher. Failure (daemon down,
   *  endpoint unreachable) collapses the model picker to an empty list
   *  — the user gets a visible "no models available" affordance and
   *  the Run button stays disabled. */
  async _loadModels(bencherName: string): Promise<void> {
    if (!bencherName) {
      this.models = [];
      this.selectedModel = "";
      return;
    }
    try {
      const [benchMod, resultMod] = await Promise.all([
        import("@desktop/benchmark/service"),
        import("../result"),
      ]);
      const { unwrap } = resultMod;
      const ids = await unwrap<string[]>(benchMod.ModelsForBencher(bencherName), []);
      this.models = Array.isArray(ids) ? ids : [];
      // Preserve user's pick across reloads when still valid; otherwise
      // auto-select the first available model so Run is one click away.
      if (!this.models.includes(this.selectedModel)) {
        this.selectedModel = this.models[0] || "";
      }
    } catch {
      this.models = [];
      this.selectedModel = "";
    }
  }

  /** Trigger one benchmark run via the substrate dispatch path. The
   *  substrate looks up the named bencher, calls its Bench() method,
   *  persists the resulting Run, and we refresh History to surface it.
   *  Button stays disabled while inflight; errors render below the
   *  toolbar until the next successful run. */
  async _runBench(): Promise<void> {
    if (this.running) return;
    if (!this.selectedBencher || !this.selectedModel) {
      this.runErr = "Pick a bencher and a model first.";
      return;
    }
    this.running = true;
    this.runErr = "";
    try {
      const [benchMod, resultMod] = await Promise.all([
        import("@desktop/benchmark/service"),
        import("../result"),
      ]);
      const { demand } = resultMod;
      const req = {
        Model: this.selectedModel,
        Prompt: SAMPLE_PROMPT,
        Ctx: DEFAULT_RUN_CTX,
        MaxOutput: DEFAULT_RUN_MAX_OUTPUT,
      } as Parameters<typeof benchMod.Bench>[0];
      await demand<RunDTO>(benchMod.Bench(req, this.selectedBencher));
      // Refresh history so the new row shows at the top of the table.
      await this._loadFromBackend();
    } catch (e: unknown) {
      this.runErr = e instanceof Error ? e.message : String(e);
    } finally {
      this.running = false;
    }
  }

  /** Pick handler — bencher dropdown change. */
  _onBencherChange(e: Event) {
    const name = (e.target as HTMLSelectElement).value;
    this.selectedBencher = name;
    this.selectedModel = "";
    void this._loadModels(name);
  }

  /** Pick handler — model dropdown change. */
  _onModelChange(e: Event) {
    this.selectedModel = (e.target as HTMLSelectElement).value;
  }

  render() {
    const runs = this.runs;
    const curve = FIXTURE_CURVE;
    const cw = 880, ch = 220, pad = { l: 48, r: 18, t: 16, b: 28 };
    const xs = (c: number) => pad.l + (Math.log2(c / 128) / Math.log2(8192 / 128)) * (cw - pad.l - pad.r);
    const ys = (t: number) => pad.t + (1 - (t - 20) / (60 - 20)) * (ch - pad.t - pad.b);

    const pickerStyle = "padding:4px 24px 4px 10px; border-radius:6px; background:rgba(255,255,255,0.04); border:1px solid rgba(255,255,255,0.07); font-size:11px; color:var(--fg-1); font-family:var(--font-mono); appearance:none; cursor:pointer; outline:none;";
    const runDisabled = this.running || !this.selectedBencher || !this.selectedModel || this.models.length === 0;
    const toolbar = html`
      <select
        title="Bencher"
        style=${pickerStyle}
        .value=${this.selectedBencher}
        ?disabled=${this.benchers.length === 0}
        @change=${(e: Event) => this._onBencherChange(e)}
      >
        ${this.benchers.length === 0
          ? html`<option value="">no benchers registered</option>`
          : this.benchers.map(b => html`<option value=${b.name} ?selected=${b.name === this.selectedBencher}>${b.name}</option>`)}
      </select>
      <select
        title="Model"
        style=${pickerStyle}
        .value=${this.selectedModel}
        ?disabled=${this.models.length === 0}
        @change=${(e: Event) => this._onModelChange(e)}
      >
        ${this.models.length === 0
          ? html`<option value="">${this.selectedBencher ? "no models available" : "pick a bencher first"}</option>`
          : this.models.map(m => html`<option value=${m} ?selected=${m === this.selectedModel}>${m}</option>`)}
      </select>
      <lthn-btn tone="ghost" size="sm">${this.t.btnPp}</lthn-btn>
      <lthn-btn tone="ghost" size="sm">${this.t.btnTg}</lthn-btn>
      <lthn-btn tone="ghost" size="sm" active>${this.t.btnBoth}</lthn-btn>
      <div style="flex:1"></div>
      <lthn-btn
        tone="primary"
        size="sm"
        ?disabled=${runDisabled}
        @click=${() => void this._runBench()}
      >
        <i class="fa-solid ${this.running ? "fa-spinner fa-spin" : "fa-play"}" style="font-size:9px;"></i>
        ${this.running ? "Running…" : this.t.btnRun}
      </lthn-btn>
      <lthn-btn tone="ghost" size="sm"><i class="fa-regular fa-file-arrow-down" style="font-size:10px;"></i> ${this.t.btnExport}</lthn-btn>
    `;

    const errBanner = this.runErr ? html`
      <div style="padding:6px 22px; font-family:var(--font-mono); font-size:10.5px; color:var(--error-400); background:rgba(248,113,113,0.06); border-bottom:1px solid rgba(248,113,113,0.18);">
        ${this.runErr}
      </div>
    ` : nothing;

    const cols = "20px 1.3fr 0.9fr 1.2fr 0.7fr 0.8fr 0.7fr 0.7fr 60px";

    const body = html`
      <div style="flex:1; display:flex; flex-direction:column; min-height:0;">
        ${errBanner}
        <!-- history table -->
        <div style="padding:14px 22px 8px; display:flex; flex-direction:column; gap:6px;">
          <div style="display:flex; align-items:center; justify-content:space-between;">
            <lthn-label>${this.t.labelRecent}</lthn-label>
            <div style="font-family:var(--font-mono); font-size:10px; color:var(--fg-3);">${this.t.labelCompare}</div>
          </div>
          <div style="background:rgba(255,255,255,0.025); border:1px solid rgba(255,255,255,0.06); border-radius:8px; font-family:var(--font-mono); font-size:11px;">
            <div style="display:grid; grid-template-columns:${cols}; padding:8px 14px; border-bottom:1px solid rgba(255,255,255,0.05); color:var(--fg-3); font-size:10px; letter-spacing:0.04em; text-transform:uppercase;">
              <span></span><span>${this.t.colTs}</span><span>${this.t.colBencher}</span><span>${this.t.colModel}</span><span>${this.t.colPp}</span><span>${this.t.colTg}</span><span>${this.t.colPeakW}</span><span>${this.t.colMem}</span><span></span>
            </div>
            ${runs.map((r, i) => {
              const sel = i < 2;
              return html`
                <div style="display:grid; grid-template-columns:${cols}; padding:8px 14px; background:${r.here ? "rgba(64,193,197,0.07)" : "transparent"}; border-bottom:${i < runs.length - 1 ? "1px solid rgba(255,255,255,0.04)" : "none"}; color:var(--fg-1); align-items:center;">
                  <span style="width:12px; height:12px; border-radius:3px; background:${sel ? (i === 0 ? "var(--brand-400)" : "#a78bfa") : "transparent"}; border:${sel ? "none" : "1.5px solid rgba(255,255,255,0.18)"};"></span>
                  <span style="color:var(--fg-2); font-size:10.5px;">${r.ts}</span>
                  <span style="color:var(--fg-2); font-size:10.5px;">${r.bencher}</span>
                  <span style="color:var(--fg-0);">${r.model}</span>
                  <span>${r.pp.toLocaleString()}</span>
                  <span style="color:${r.here ? "var(--brand-300)" : "var(--fg-0)"};">${r.tg.toFixed(1)}</span>
                  <span>${r.w > 0 ? `${r.w} W` : "—"}</span>
                  <span style="color:var(--fg-2);">${r.mem}</span>
                  <span style="text-align:right;">${r.here ? html`<lthn-state-pill variant="latest">${this.t.pillLatest}</lthn-state-pill>` : nothing}</span>
                </div>
              `;
            })}
          </div>
        </div>

        <!-- chart -->
        <div style="flex:1; padding:8px 22px 18px; display:flex; flex-direction:column; min-height:0;">
          <div style="display:flex; align-items:baseline; gap:14px; margin-bottom:6px;">
            <lthn-label>${this.t.labelChart}</lthn-label>
            <span style="font-size:10.5px; color:var(--fg-3);">${this.t.labelLogX}</span>
          </div>
          <div style="flex:1; background:rgba(0,0,0,0.20); border:1px solid rgba(255,255,255,0.05); border-radius:8px; padding:8px;">
            <svg viewBox="0 0 ${cw} ${ch}" width="100%" height="100%" preserveAspectRatio="none">
              ${[60, 50, 40, 30, 20].map((y) => html`
                <line x1=${pad.l} x2=${cw - pad.r} y1=${ys(y)} y2=${ys(y)} stroke="rgba(255,255,255,0.05)"></line>
                <text x=${pad.l - 8} y=${ys(y) + 3} fill="rgba(255,255,255,0.35)" font-size="10" text-anchor="end" font-family="ui-monospace, monospace">${y}</text>
              `)}
              ${[128, 512, 1024, 2048, 4096, 8192].map((c) => html`
                <line x1=${xs(c)} x2=${xs(c)} y1=${pad.t} y2=${ch - pad.b} stroke="rgba(255,255,255,0.04)"></line>
                <text x=${xs(c)} y=${ch - pad.b + 14} fill="rgba(255,255,255,0.40)" font-size="9.5" text-anchor="middle" font-family="ui-monospace, monospace">${c >= 1024 ? `${c/1024}k` : c}</text>
              `)}
              <path d=${"M " + curve.map((p) => `${xs(p.ctx)} ${ys(p.tg)}`).join(" L ")} stroke="var(--brand-400)" stroke-width="2" fill="none"></path>
              ${curve.map((p) => html`<circle cx=${xs(p.ctx)} cy=${ys(p.tg)} r="3" fill="var(--brand-400)"></circle>`)}
              <path d="M 48 88 L 198 92 L 348 100 L 498 108 L 648 130 L 798 158 L 870 178" stroke="#a78bfa" stroke-opacity="0.55" stroke-width="1.8" fill="none" stroke-dasharray="3 3"></path>
              <g transform="translate(${cw - pad.r - 200}, ${pad.t + 6})">
                <rect width="200" height="42" fill="rgba(0,0,0,0.30)" stroke="rgba(255,255,255,0.06)" rx="4"></rect>
                <circle cx="14" cy="14" r="4" fill="var(--brand-400)"></circle>
                <text x="26" y="18" fill="rgba(255,255,255,0.85)" font-size="10" font-family="ui-monospace, monospace">gemma-4-e2b · today</text>
                <circle cx="14" cy="30" r="4" fill="#a78bfa" fill-opacity="0.6"></circle>
                <text x="26" y="34" fill="rgba(255,255,255,0.65)" font-size="10" font-family="ui-monospace, monospace">llama-3.2-3b · -2 d</text>
              </g>
              <text x="8" y=${pad.t + 4} fill="rgba(255,255,255,0.45)" font-size="9.5" font-family="ui-monospace, monospace" transform="rotate(-90 10 ${pad.t + 4})">${this.t.yAxis}</text>
            </svg>
          </div>
        </div>
      </div>
    `;

    return renderChrome({
      title: this.chrome.title, subtitle: this.chrome.subtitle,
      w: this.w, h: this.h, toolbar, body,
      footer: html`${this.t.footer}`,
      embedded: this.embedded,
    });
  }
}
customElements.define("lthn-benchmark-window", LthnBenchmarkWindow);
