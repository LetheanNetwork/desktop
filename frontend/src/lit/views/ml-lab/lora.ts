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
    w:           { type: Number },
    h:           { type: Number },
    embedded:    { type: Boolean, reflect: true },
    runs:        { state: true },
    loading:     { state: true },
    error:       { state: true },
    filter:      { state: true },
    selected:    { state: true },
    showWizard:  { state: true },
    wizardBaseModel: { state: true },
    wizardDataset:   { state: true },
    wizardMode:      { state: true },
    wizardIters:     { state: true },
    wizardBusy:      { state: true },
    controlBusy:     { state: true },
    liveTick:        { state: true },
  };
  declare w: number;
  declare h: number;
  declare embedded: boolean;
  declare runs: LoRARun[];
  declare loading: boolean;
  declare error: string;
  declare filter: string;
  declare selected: string;     // run id, "" when none selected
  declare showWizard: boolean;
  declare wizardBaseModel: string;
  declare wizardDataset: string;
  declare wizardMode: LoRAMode;
  declare wizardIters: number;
  declare wizardBusy: boolean;
  declare controlBusy: boolean;
  // liveTick is the latest RunEvent frame from the SSE stream for
  // the selected run. Patched into the selected row's snapshot on
  // render so the user sees iteration / loss / throughput tick
  // without waiting for the 60s registry poll.
  declare liveTick: {
    runID: string; iteration: number; loss: number;
    throughputTPS: number; etaSeconds: number;
  } | null;
  private _timer: ReturnType<typeof setInterval> | null = null;
  private _eventSource: EventSource | null = null;

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
    this.showWizard = false;
    this.wizardBaseModel = "lemma-12b-v4-p6";
    this.wizardDataset = "datasets/lethean-prep-v1.jsonl";
    this.wizardMode = "sft";
    this.wizardIters = 200;
    this.wizardBusy = false;
    this.controlBusy = false;
    this.liveTick = null;
  }

  createRenderRoot() { return this; }

  async connectedCallback() {
    super.connectedCallback();
    await this._loadFromBackend();
    this._timer = setInterval(() => { void this._loadFromBackend(); }, REFRESH_MS);
  }

  disconnectedCallback() {
    if (this._timer) { clearInterval(this._timer); this._timer = null; }
    this._closeEventSource();
    super.disconnectedCallback();
  }

  // _openEventSource opens an SSE stream for the named run so the
  // selected row's loss / throughput / iteration tick in real time
  // (every ~200ms from the mock driver). Auto-merges into the
  // run's snapshot via _onLiveFrame. Idempotent — re-opening for
  // the same id is a no-op.
  private _openEventSource(runID: string): void {
    if (this._eventSource && this.liveTick?.runID === runID) return;
    this._closeEventSource();
    if (!runID) return;
    try {
      const es = new EventSource(`/v1/ml-lab/runs/${encodeURIComponent(runID)}/events`);
      es.onmessage = (ev) => this._onLiveFrame(ev.data);
      es.addEventListener("end", () => this._closeEventSource());
      es.onerror = () => { /* fall through to next reconnect attempt */ };
      this._eventSource = es;
    } catch {
      // EventSource not available (e.g. test env) — silent degrade,
      // the 60s poller still covers the gap.
    }
  }

  private _closeEventSource(): void {
    if (this._eventSource) {
      this._eventSource.close();
      this._eventSource = null;
    }
    this.liveTick = null;
  }

  private _onLiveFrame(data: string): void {
    if (!data || data.startsWith(":")) return;
    try {
      const frame = JSON.parse(data) as {
        run_id?: string; iteration?: number; loss?: number;
        throughput_tps?: number; eta_seconds?: number; kind?: string;
      };
      if (!frame.run_id) return;
      this.liveTick = {
        runID:         frame.run_id,
        iteration:     frame.iteration ?? 0,
        loss:          frame.loss ?? 0,
        throughputTPS: frame.throughput_tps ?? 0,
        etaSeconds:    frame.eta_seconds ?? 0,
      };
      if (frame.kind === "complete" || frame.kind === "error") {
        this._closeEventSource();
        // Trigger a registry refresh so the row's terminal state
        // (completed / failed) lands without waiting for the poller.
        void this._loadFromBackend();
      }
    } catch { /* malformed frame — ignore */ }
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
    // Open SSE stream when an active run is selected; close when
    // deselected or selecting a terminal run.
    const r = this.runs.find(x => x.id === id);
    if (this.selected && r && (r.status === "active" || r.status === "queued")) {
      this._openEventSource(this.selected);
    } else {
      this._closeEventSource();
    }
  }

  // _controlRun POSTs an action verb to the per-run control endpoint.
  // Driver responds with the post-action status; trigger a refresh so
  // the row's status badge updates immediately.
  async _controlRun(action: "pause" | "resume" | "cancel" | "checkpoint"): Promise<void> {
    if (!this.selected || this.controlBusy) return;
    this.controlBusy = true;
    try {
      const res = await apiFetch(`/v1/ml-lab/runs/${encodeURIComponent(this.selected)}/control`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ run_id: this.selected, action }),
      });
      if (!res.ok) {
        this.error = `control ${action} returned ${res.status}`;
        return;
      }
      await this._loadFromBackend();
    } catch (e) {
      this.error = (e instanceof Error) ? e.message : `control ${action} failed`;
    } finally {
      this.controlBusy = false;
    }
  }

  // _onStartRun submits the wizard form to /v1/ml-lab/runs/start.
  // The returned RunHandle.run_id becomes the new selection so the
  // user lands on the live row immediately.
  async _onStartRun(): Promise<void> {
    if (this.wizardBusy) return;
    this.wizardBusy = true;
    try {
      const res = await apiFetch("/v1/ml-lab/runs/start", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          base_model:   this.wizardBaseModel,
          dataset:      this.wizardDataset,
          mode:         this.wizardMode,
          total_iters:  this.wizardIters,
          // Tick cadence — keep mock-driver default (200ms) for the
          // demo feel; production runs override via run-recipe.
          hyperparams:  { tick_ms: 200 },
        }),
      });
      if (!res.ok) {
        this.error = `start run returned ${res.status}`;
        return;
      }
      const handle = await res.json() as { run_id?: string };
      this.showWizard = false;
      await this._loadFromBackend();
      if (handle.run_id) {
        this.selected = handle.run_id;
        this._openEventSource(handle.run_id);
      }
    } catch (e) {
      this.error = (e instanceof Error) ? e.message : "start run failed";
    } finally {
      this.wizardBusy = false;
    }
  }

  _toggleWizard() { this.showWizard = !this.showWizard; }

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
    // Merge the live SSE tick onto the selected row's snapshot so
    // iteration / loss / throughput / eta update without waiting
    // for the 60s registry refresh. Non-selected rows render their
    // last-known registry values.
    const useLive = isSelected && this.liveTick && this.liveTick.runID === r.id;
    const iter = useLive ? this.liveTick!.iteration       : r.iteration;
    const loss = useLive ? this.liveTick!.loss            : r.last_loss;
    const tps  = useLive ? this.liveTick!.throughputTPS   : r.throughput_tps;
    const eta  = useLive ? this.liveTick!.etaSeconds      : r.eta_seconds;
    return html`
      <tr class="run-row ${isSelected ? "selected" : ""}" @click=${() => this._onRowClick(r.id)}>
        <td class="status">${this._statusBadge(r.status)}</td>
        <td class="base-model" title=${r.id}>${r.base_model}</td>
        <td class="mode">${this._modeBadge(r.mode)}</td>
        <td class="dataset">${r.dataset}</td>
        <td class="progress">${iter} / ${r.total_iters} (${this._fmtPct(iter, r.total_iters)})</td>
        <td class="loss">${this._fmtLoss(loss)}</td>
        <td class="throughput">${this._fmtThroughput(tps)}</td>
        <td class="eta">${this._fmtETA(eta)}</td>
        <td class="started">${this._fmtTimestamp(r.started)}</td>
      </tr>
    `;
  }

  _renderControlBar() {
    if (!this.selected) return null;
    const selectedRun = this.runs.find(r => r.id === this.selected);
    if (!selectedRun) return null;
    const isActive    = selectedRun.status === "active";
    const isPaused    = selectedRun.status === "paused";
    const isTerminal  = selectedRun.status === "completed" || selectedRun.status === "cancelled" || selectedRun.status === "failed";
    const btn = (label: string, action: "pause"|"resume"|"cancel"|"checkpoint", disabled = false) => html`
      <button type="button" @click=${(e: Event) => { e.stopPropagation(); void this._controlRun(action); }}
        ?disabled=${disabled || this.controlBusy}
        style="background:var(--colour-surface-2);color:var(--colour-text-primary);border:1px solid var(--colour-border);padding:5px 12px;border-radius:5px;font-size:12px;cursor:pointer;font-family:var(--font-sans);">
        ${label}
      </button>
    `;
    return html`
      <div style="display:flex;align-items:center;gap:8px;padding:8px 14px;border-bottom:1px solid var(--colour-border-subtle);background:var(--colour-surface-1);">
        <span style="font-size:11px;color:var(--colour-text-tertiary);font-family:var(--font-mono);">${this.selected}</span>
        <div style="flex:1"></div>
        ${btn("Pause", "pause", !isActive)}
        ${btn("Resume", "resume", !isPaused)}
        ${btn("Checkpoint", "checkpoint", isTerminal)}
        ${btn("Cancel", "cancel", isTerminal)}
        ${this.controlBusy
          ? html`<span style="font-size:11px;color:var(--colour-text-tertiary);">…</span>`
          : null}
      </div>
    `;
  }

  _renderWizard() {
    if (!this.showWizard) return null;
    return html`
      <div style="position:absolute;inset:0;background:rgba(0,0,0,0.6);display:flex;align-items:center;justify-content:center;z-index:10;"
           @click=${(e: Event) => { if (e.target === e.currentTarget) this._toggleWizard(); }}>
        <div style="background:var(--colour-surface-1);border:1px solid var(--colour-border);border-radius:8px;padding:24px;width:480px;display:flex;flex-direction:column;gap:14px;">
          <h3 style="margin:0;font-size:15px;color:var(--colour-text-primary);">New LoRA Run</h3>
          <label style="display:flex;flex-direction:column;gap:4px;font-size:12px;color:var(--colour-text-secondary);">
            Base model
            <input type="text" .value=${this.wizardBaseModel}
              @input=${(e: Event) => { this.wizardBaseModel = (e.target as HTMLInputElement).value; }}
              style="background:var(--colour-surface-2);border:1px solid var(--colour-border);color:var(--colour-text-primary);padding:6px 10px;border-radius:5px;font-family:var(--font-mono);font-size:12px;outline:none;" />
          </label>
          <label style="display:flex;flex-direction:column;gap:4px;font-size:12px;color:var(--colour-text-secondary);">
            Dataset
            <input type="text" .value=${this.wizardDataset}
              @input=${(e: Event) => { this.wizardDataset = (e.target as HTMLInputElement).value; }}
              style="background:var(--colour-surface-2);border:1px solid var(--colour-border);color:var(--colour-text-primary);padding:6px 10px;border-radius:5px;font-family:var(--font-mono);font-size:12px;outline:none;" />
          </label>
          <div style="display:flex;gap:14px;">
            <label style="display:flex;flex-direction:column;gap:4px;font-size:12px;color:var(--colour-text-secondary);flex:1;">
              Mode
              <select .value=${this.wizardMode}
                @change=${(e: Event) => { this.wizardMode = (e.target as HTMLSelectElement).value as LoRAMode; }}
                style="background:var(--colour-surface-2);border:1px solid var(--colour-border);color:var(--colour-text-primary);padding:6px 10px;border-radius:5px;font-family:var(--font-mono);font-size:12px;outline:none;">
                <option value="sft">sft</option>
                <option value="grpo">grpo</option>
                <option value="distill">distill</option>
              </select>
            </label>
            <label style="display:flex;flex-direction:column;gap:4px;font-size:12px;color:var(--colour-text-secondary);flex:1;">
              Total iterations
              <input type="number" min="1" max="100000" .value=${String(this.wizardIters)}
                @input=${(e: Event) => { this.wizardIters = Number((e.target as HTMLInputElement).value) || 0; }}
                style="background:var(--colour-surface-2);border:1px solid var(--colour-border);color:var(--colour-text-primary);padding:6px 10px;border-radius:5px;font-family:var(--font-mono);font-size:12px;outline:none;" />
            </label>
          </div>
          <div style="display:flex;gap:8px;justify-content:flex-end;margin-top:8px;">
            <button type="button" @click=${this._toggleWizard}
              style="background:transparent;color:var(--colour-text-secondary);border:1px solid var(--colour-border);padding:6px 14px;border-radius:5px;font-size:12px;cursor:pointer;">
              Cancel
            </button>
            <button type="button" @click=${this._onStartRun} ?disabled=${this.wizardBusy}
              style="background:var(--colour-accent);color:var(--colour-text-inverse);border:none;padding:6px 14px;border-radius:5px;font-size:12px;cursor:pointer;font-weight:500;">
              ${this.wizardBusy ? "Starting…" : "Start Run"}
            </button>
          </div>
        </div>
      </div>
    `;
  }

  _activeCount(): number {
    return this.runs.filter(r => r.status === "active").length;
  }

  render() {
    const rows = this._filtered();
    const body = html`
      <div style="position:relative;display:flex;flex-direction:column;flex:1;min-height:0;">
        ${this._renderWizard()}
      <div class="lora-toolbar">
        <input
          type="text"
          placeholder="filter by base model / dataset / mode / status / id"
          .value=${this.filter}
          @input=${this._onFilterInput}
          class="filter-input"
        />
        <button type="button" @click=${this._toggleWizard}
          style="background:var(--colour-accent);color:var(--colour-text-inverse);border:none;padding:5px 14px;border-radius:5px;font-size:12px;cursor:pointer;font-weight:500;margin-left:8px;">
          New Run
        </button>
        <span class="status">
          ${this.error ? html`<span class="error">${this.error}</span>` : null}
          ${this.loading ? html`<span class="loading">refreshing…</span>` : null}
          <span class="active-count">${this._activeCount()} active</span>
          <span class="count">${rows.length} of ${this.runs.length}</span>
        </span>
      </div>
      ${this._renderControlBar()}
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
      </div>
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
