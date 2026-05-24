// SPDX-Licence-Identifier: EUPL-1.2

// <lthn-view-ml-lab-lora> — LoRA tab of the ML Lab workbench,
// per plans/project/lthn/desktop/RFC.ml-lab.md §7. Renders the
// training-run registry: status, base model, mode, progress, last
// loss, throughput, ETA, started.
//
// Same canonical wire pattern as the sibling models view: lazy
// backend fetch with fixture fallback, 60s refresh poller. Backend
// endpoint TODO(snider) — wire to LabService.ListLoRARuns once
// go-ml/go/lab/ lands. Current fixture surfaces a mix of run states
// so the panel can be styled cold.
//
// Bidirectional control (start / pause / resume / cancel / fuse) +
// live event-stream subscription are part of the LoRA tab's full
// shape (RFC §7 control bar + SubscribeRunEvents) but are not in
// this first surface — they want a Wails event-bus seam the
// canonical apiFetch wire pattern doesn't carry. Stub TODO marker
// in connectedCallback so the seam is honest.

import { LitElement, html } from "lit";
import { renderChrome } from "../../chrome";
import { apiFetch } from "../../api-fetch";
import { clampRows } from "../_bounds";

type LoRARunStatus = "active" | "paused" | "completed" | "cancelled" | "failed" | "queued";
type LoRAMode = "sft" | "grpo" | "distill";

interface LoRARun {
  id:             string;
  base_model:     string;
  dataset:        string;
  mode:           LoRAMode;
  status:         LoRARunStatus;
  iteration:      number;
  total_iters:    number;
  last_loss:      number;
  throughput_tps: number;   // tokens / sec
  eta_seconds:    number;   // 0 when completed / cancelled / failed
  started:        string;   // RFC3339
  train_path:     string;   // .train file being written
}

interface LoRARunsQueryResponse {
  runs?: LoRARun[];
}

const FIXTURE_RUNS: LoRARun[] = [
  {
    id:             "run-2026-05-22-12b-v4-p6-rsft",
    base_model:     "lemma-12b-v4-p6",
    dataset:        "golden_set_v2",
    mode:           "sft",
    status:         "active",
    iteration:      8612,
    total_iters:    13479,
    last_loss:      0.412,
    throughput_tps: 1804,
    eta_seconds:    9120,
    started:        "2026-05-22T08:14:22Z",
    train_path:     "/Users/agent/Lethean/data/train/lemma-12b-v4-p6-rsft-2026-05-22.train",
  },
  {
    id:             "run-2026-05-23-4b-grpo-zen",
    base_model:     "lemma-4b-e4b",
    dataset:        "zen_composure_v1",
    mode:           "grpo",
    status:         "active",
    iteration:      1240,
    total_iters:    2400,
    last_loss:      1.218,
    throughput_tps: 2412,
    eta_seconds:    3210,
    started:        "2026-05-23T22:01:09Z",
    train_path:     "/Users/agent/Lethean/data/train/lemma-4b-grpo-2026-05-23.train",
  },
  {
    id:             "run-2026-05-21-1b-distill",
    base_model:     "lemer-2b-e2b",
    dataset:        "lek-1-distill-corpus",
    mode:           "distill",
    status:         "completed",
    iteration:      2400,
    total_iters:    2400,
    last_loss:      0.187,
    throughput_tps: 0,
    eta_seconds:    0,
    started:        "2026-05-21T03:22:11Z",
    train_path:     "/Users/agent/Lethean/data/train/lemer-2b-distill-2026-05-21.train",
  },
  {
    id:             "run-2026-05-20-26b-moe-paused",
    base_model:     "lemmy-26b-a4b-moe",
    dataset:        "lek-axiom-sandwich-v3",
    mode:           "sft",
    status:         "paused",
    iteration:      4801,
    total_iters:    18000,
    last_loss:      0.658,
    throughput_tps: 0,
    eta_seconds:    0,
    started:        "2026-05-20T11:55:13Z",
    train_path:     "/Users/agent/Lethean/data/train/lemmy-26b-moe-2026-05-20.train",
  },
  {
    id:             "run-2026-05-19-4b-failed-oom",
    base_model:     "lemma-4b-e4b",
    dataset:        "expansion_prompts_v2",
    mode:           "sft",
    status:         "failed",
    iteration:      213,
    total_iters:    8000,
    last_loss:      2.804,
    throughput_tps: 0,
    eta_seconds:    0,
    started:        "2026-05-19T18:00:00Z",
    train_path:     "/Users/agent/Lethean/data/train/lemma-4b-oom-2026-05-19.train",
  },
  {
    id:             "run-2026-05-24-queued",
    base_model:     "lemma-12b-v4-p7",
    dataset:        "golden_set_v2",
    mode:           "sft",
    status:         "queued",
    iteration:      0,
    total_iters:    13479,
    last_loss:      0,
    throughput_tps: 0,
    eta_seconds:    0,
    started:        "",
    train_path:     "",
  },
];

const REFRESH_MS = 60_000;

class LthnViewMlLabLora extends LitElement {
  static readonly properties = {
    w:        { type: Number },
    h:        { type: Number },
    embedded: { type: Boolean, reflect: true },
    runs:     { state: true },
    loading:  { state: true },
    error:    { state: true },
    filter:   { state: true },
    selected: { state: true },
  };
  declare w: number;
  declare h: number;
  declare embedded: boolean;
  declare runs: LoRARun[];
  declare loading: boolean;
  declare error: string;
  declare filter: string;
  declare selected: string;     // run id, "" when none selected
  private _timer: ReturnType<typeof setInterval> | null = null;

  constructor() {
    super();
    this.w = 1180;
    this.h = 720;
    this.embedded = false;
    this.runs = FIXTURE_RUNS;
    this.loading = false;
    this.error = "";
    this.filter = "";
    this.selected = "";
  }

  createRenderRoot() { return this; }

  async connectedCallback() {
    super.connectedCallback();
    await this._loadFromBackend();
    this._timer = setInterval(() => { void this._loadFromBackend(); }, REFRESH_MS);
    // TODO(snider): subscribe to LabService.SubscribeRunEvents for
    // active runs so loss / throughput tick without waiting for the
    // 60s poller. Bidirectional Wails event-bus seam, not apiFetch.
  }

  disconnectedCallback() {
    if (this._timer) { clearInterval(this._timer); this._timer = null; }
    super.disconnectedCallback();
  }

  async _loadFromBackend() {
    if (this.loading) return;
    this.loading = true;
    this.error = "";
    try {
      const res = await apiFetch("/v1/ml-lab/runs", {
        method: "GET",
        headers: { "Accept": "application/json" },
      });
      if (!res.ok) {
        if (res.status !== 401) {
          this.error = `runs query returned ${res.status}`;
        }
        return;
      }
      const wrapper = await res.json();
      const data = (wrapper as { data?: LoRARunsQueryResponse })?.data;
      const runs = clampRows<LoRARun>(data?.runs);
      if (runs.length === 0) {
        return; // keep current state per design contract
      }
      this.runs = runs;
    } catch (e) {
      this.error = (e instanceof Error) ? e.message : "runs fetch failed";
    } finally {
      this.loading = false;
    }
  }

  _filtered(): LoRARun[] {
    const q = (this.filter || "").trim().toLowerCase();
    if (!q) return this.runs;
    return this.runs.filter(r =>
      (r.base_model || "").toLowerCase().includes(q) ||
      (r.dataset    || "").toLowerCase().includes(q) ||
      (r.mode       || "").toLowerCase().includes(q) ||
      (r.status     || "").toLowerCase().includes(q) ||
      (r.id         || "").toLowerCase().includes(q)
    );
  }

  _onFilterInput(e: Event) {
    this.filter = (e.target as HTMLInputElement)?.value ?? "";
  }

  _onRowClick(id: string) {
    this.selected = this.selected === id ? "" : id;
  }

  _fmtPct(it: number, total: number): string {
    if (!total) return "—";
    const p = Math.min(100, Math.max(0, (it / total) * 100));
    return `${p.toFixed(1)}%`;
  }

  _fmtETA(sec: number): string {
    if (!sec || sec <= 0) return "—";
    if (sec < 60) return `${sec}s`;
    if (sec < 3600) return `${Math.round(sec / 60)}m`;
    const h = Math.floor(sec / 3600);
    const m = Math.round((sec % 3600) / 60);
    return m === 0 ? `${h}h` : `${h}h ${m}m`;
  }

  _fmtThroughput(tps: number): string {
    if (!tps || tps <= 0) return "—";
    if (tps < 1000) return `${tps.toFixed(0)} tok/s`;
    return `${(tps / 1000).toFixed(1)}k tok/s`;
  }

  _fmtLoss(loss: number): string {
    if (!Number.isFinite(loss) || loss === 0) return "—";
    return loss.toFixed(3);
  }

  _fmtTimestamp(iso: string): string {
    if (!iso) return "—";
    const d = new Date(iso);
    if (Number.isNaN(d.getTime())) return iso;
    return d.toLocaleString("en-GB", { hour12: false });
  }

  _statusBadge(status: LoRARunStatus) {
    return html`<span class="status-badge status-${status}">${status}</span>`;
  }

  _modeBadge(mode: LoRAMode) {
    return html`<span class="mode-badge mode-${mode}">${mode.toUpperCase()}</span>`;
  }

  _renderRow(r: LoRARun) {
    const isSelected = this.selected === r.id;
    return html`
      <tr class="run-row ${isSelected ? "selected" : ""}" @click=${() => this._onRowClick(r.id)}>
        <td class="status">${this._statusBadge(r.status)}</td>
        <td class="base-model" title=${r.id}>${r.base_model}</td>
        <td class="mode">${this._modeBadge(r.mode)}</td>
        <td class="dataset">${r.dataset}</td>
        <td class="progress">${r.iteration} / ${r.total_iters} (${this._fmtPct(r.iteration, r.total_iters)})</td>
        <td class="loss">${this._fmtLoss(r.last_loss)}</td>
        <td class="throughput">${this._fmtThroughput(r.throughput_tps)}</td>
        <td class="eta">${this._fmtETA(r.eta_seconds)}</td>
        <td class="started">${this._fmtTimestamp(r.started)}</td>
      </tr>
    `;
  }

  _activeCount(): number {
    return this.runs.filter(r => r.status === "active").length;
  }

  render() {
    const rows = this._filtered();
    const body = html`
      <div class="lora-toolbar">
        <input
          type="text"
          placeholder="filter by base model / dataset / mode / status / id"
          .value=${this.filter}
          @input=${this._onFilterInput}
          class="filter-input"
        />
        <span class="status">
          ${this.error ? html`<span class="error">${this.error}</span>` : null}
          ${this.loading ? html`<span class="loading">refreshing…</span>` : null}
          <span class="active-count">${this._activeCount()} active</span>
          <span class="count">${rows.length} of ${this.runs.length}</span>
        </span>
      </div>
      <table class="lora-table">
        <thead>
          <tr>
            <th>status</th>
            <th>base model</th>
            <th>mode</th>
            <th>dataset</th>
            <th>progress</th>
            <th>loss</th>
            <th>throughput</th>
            <th>eta</th>
            <th>started</th>
          </tr>
        </thead>
        <tbody>
          ${rows.length === 0
            ? html`<tr><td colspan="9" class="empty-state">No LoRA runs match these filters. Adjust the filter or kick off a run via <code>lthn ai train &lt;recipe&gt;</code> to populate the registry.</td></tr>`
            : rows.map(r => this._renderRow(r))}
        </tbody>
      </table>
    `;
    if (this.embedded) return body;
    return renderChrome({
      title:    "LoRA",
      subtitle: "Training run registry — active / paused / completed / failed",
      w:        this.w,
      h:        this.h,
      embedded: false,
      body,
    });
  }
}

if (!customElements.get("lthn-view-ml-lab-lora")) {
  customElements.define("lthn-view-ml-lab-lora", LthnViewMlLabLora);
}

export { LthnViewMlLabLora };
export type { LoRARun, LoRARunStatus, LoRAMode, LoRARunsQueryResponse };
