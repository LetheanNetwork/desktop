// SPDX-Licence-Identifier: EUPL-1.2
// E4.2 · distillation — <lthn-distillation-window>
// Light-DOM Lit element. Composes renderChrome() from ../chrome.js.

import { LitElement, html, nothing } from "lit";
import { renderChrome } from "../chrome";
import { T } from "@lthn/i18n/coreservice";

/** One model entry from Models.List() — shape mirrors pkg/models.Entry. */
interface ModelEntry { name: string; path: string; size: number; is_dir: boolean }

/** One adapter from Lemma.SFTAdapters(). Mirrors lemma.SFTAdapter. */
interface AdapterEntry { name: string; path: string; size_bytes: number; modified_unix: number }

/** Live SFT job snapshot — mirrors lemma.SFTJob (kebab-case from JSON). */
interface SFTJobView {
  job_id:       string;
  state:        string;
  model_path:   string;
  dataset_path: string;
  adapter_dir:  string;
  step:         number;
  epoch:        number;
  last_loss:    number;
  samples:      number;
  error?:       string;
  loss?:        Array<{ step: number; epoch: number; loss: number; ts_unix: number }>;
}

class LthnDistillationWindow extends LitElement {
  static readonly properties = {
    w: { type: Number },
    h: { type: Number },
    embedded: { type: Boolean, reflect: true },
    chrome: { state: true },
    t: { state: true },
    // Live SFT state — driven by Lemma.SFTStart/Status/Stop/Adapters.
    job:           { state: true },
    baseModels:    { state: true },
    adapters:      { state: true },
    modelPath:     { state: true },
    datasetPath:   { state: true },
    adapterName:   { state: true },
    epochs:        { state: true },
    batchSize:     { state: true },
    learningRate:  { state: true },
    loraRank:      { state: true },
    loraAlpha:     { state: true },
    busy:          { state: true },
    err:           { state: true },
  };
  declare w: number;
  declare h: number;
  declare embedded: boolean;
  declare chrome: { title: string; subtitle: string };
  declare job: SFTJobView | null;
  declare baseModels: ModelEntry[];
  declare adapters: AdapterEntry[];
  declare modelPath: string;
  declare datasetPath: string;
  declare adapterName: string;
  declare epochs: number;
  declare batchSize: number;
  declare learningRate: number;
  declare loraRank: number;
  declare loraAlpha: number;
  declare busy: boolean;
  declare err: string;
  private _poll: ReturnType<typeof setInterval> | null = null;
  declare t: {
    stepBase: string; stepDataset: string; stepConfig: string;
    stepRun: string; stepPublish: string; btnStop: string;
    labelRecipe: string;
    rowMethod: string; rowRank: string; rowAlpha: string; rowDropout: string;
    rowLR: string; rowBatch: string; rowEpochs: string; rowTargets: string;
    labelBaseDataset: string;
    rowBase: string; rowDataset: string; rowSamples: string; rowSplit: string; rowFormat: string;
    mEpoch: string; mLoss: string; mTps: string; mWatts: string;
    labelLoss: string; epochMarker: string;
    labelSample: string; whoBase: string; whoOurs: string;
    labelAdapter: string;
    btnTestChat: string; btnMerge: string; btnPushHf: string;
    labelSystem: string;
    rowBackend: string; rowGpuMem: string; rowDiskIo: string; rowEta: string;
    calloutLocal: string; footer: string;
  };
  constructor() {
    super();
    this.w = 1100; this.h = 740; this.embedded = false;
    this.chrome = { title: "Fine-tune", subtitle: "LoRA · SFT · distill · merge" };
    this.job = null;
    this.baseModels = [];
    this.adapters = [];
    this.modelPath = "";
    this.datasetPath = "";
    this.adapterName = "";
    this.epochs = 3;
    this.batchSize = 8;
    this.learningRate = 1e-4;
    this.loraRank = 16;
    this.loraAlpha = 32;
    this.busy = false;
    this.err = "";
    this.t = {
      stepBase: "Base model", stepDataset: "Dataset", stepConfig: "Config",
      stepRun: "Run", stepPublish: "Publish", btnStop: "Stop",
      labelRecipe: "Recipe",
      rowMethod: "Method", rowRank: "Rank", rowAlpha: "Alpha", rowDropout: "Dropout",
      rowLR: "LR", rowBatch: "Batch", rowEpochs: "Epochs", rowTargets: "Targets",
      labelBaseDataset: "Base + dataset",
      rowBase: "Base", rowDataset: "Dataset", rowSamples: "Samples",
      rowSplit: "Split", rowFormat: "Format",
      mEpoch: "Epoch", mLoss: "Loss", mTps: "tok/s", mWatts: "Watts",
      labelLoss: "Loss · steps 0 → 184", epochMarker: "epoch 2 begins",
      labelSample: "Sample · eval prompt #142",
      whoBase: "base · pre-tune", whoOurs: "ours · step 184",
      labelAdapter: "Output adapter",
      btnTestChat: "Test in chat", btnMerge: "Merge into base", btnPushHf: "Push to HuggingFace",
      labelSystem: "System",
      rowBackend: "Backend", rowGpuMem: "GPU mem", rowDiskIo: "Disk i/o", rowEta: "ETA",
      calloutLocal: "Training runs locally. The dataset stays on this Mac. The adapter is yours.",
      footer: "step 4 of 5 · running · epoch 2/3 · loss 0.84 · ETA 14 min · 9.8 W",
    };
  }
  createRenderRoot() { return this; }
  async connectedCallback() {
    super.connectedCallback();
    const keys = [
      "title","subtitle",
      "step_base","step_dataset","step_config","step_run","step_publish","btn_stop",
      "label_recipe",
      "row_method","row_rank","row_alpha","row_dropout","row_lr","row_batch","row_epochs","row_targets",
      "label_base_dataset",
      "row_base","row_dataset","row_samples","row_split","row_format",
      "metric_epoch","metric_loss","metric_tps","metric_watts",
      "label_loss","epoch_marker",
      "label_sample","who_base","who_ours",
      "label_adapter","btn_test_chat","btn_merge","btn_push_hf",
      "label_system",
      "row_backend","row_gpumem","row_diskio","row_eta",
      "callout_local","footer",
    ];
    const vals = await Promise.all(keys.map(k => T(`window.distillation.${k}`)));
    const m: Record<string, string> = {};
    keys.forEach((k, i) => { m[k] = vals[i]; });
    this.chrome = { title: m.title, subtitle: m.subtitle };
    this.t = {
      stepBase: m.step_base, stepDataset: m.step_dataset, stepConfig: m.step_config,
      stepRun: m.step_run, stepPublish: m.step_publish, btnStop: m.btn_stop,
      labelRecipe: m.label_recipe,
      rowMethod: m.row_method, rowRank: m.row_rank, rowAlpha: m.row_alpha, rowDropout: m.row_dropout,
      rowLR: m.row_lr, rowBatch: m.row_batch, rowEpochs: m.row_epochs, rowTargets: m.row_targets,
      labelBaseDataset: m.label_base_dataset,
      rowBase: m.row_base, rowDataset: m.row_dataset, rowSamples: m.row_samples,
      rowSplit: m.row_split, rowFormat: m.row_format,
      mEpoch: m.metric_epoch, mLoss: m.metric_loss, mTps: m.metric_tps, mWatts: m.metric_watts,
      labelLoss: m.label_loss, epochMarker: m.epoch_marker,
      labelSample: m.label_sample, whoBase: m.who_base, whoOurs: m.who_ours,
      labelAdapter: m.label_adapter,
      btnTestChat: m.btn_test_chat, btnMerge: m.btn_merge, btnPushHf: m.btn_push_hf,
      labelSystem: m.label_system,
      rowBackend: m.row_backend, rowGpuMem: m.row_gpumem, rowDiskIo: m.row_diskio, rowEta: m.row_eta,
      calloutLocal: m.callout_local, footer: m.footer,
    };
    // Live data — fail quietly when Lemma engine isn't reachable. The
    // form fields stay editable so the user can fill them in and try
    // again once `lthn serve` is up.
    void this._refreshModels();
    void this._refreshAdapters();
    void this._refreshJob();
  }

  disconnectedCallback() {
    super.disconnectedCallback();
    if (this._poll !== null) { clearInterval(this._poll); this._poll = null; }
  }

  private async _refreshModels(): Promise<void> {
    try {
      const Models = await import("@desktop/models/wailsservice");
      const { unwrap } = await import("../result");
      this.baseModels = await unwrap<ModelEntry[]>(Models.List(), []);
      if (!this.modelPath && this.baseModels.length > 0) {
        this.modelPath = this.baseModels[0].path;
      }
    } catch { /* engine offline — keep empty list, form stays editable */ }
  }

  private async _refreshAdapters(): Promise<void> {
    try {
      const Lemma = await import("@desktop/lemma/wailsservice");
      const list = await Lemma.SFTAdapters();
      const arr = (list as unknown as { adapters?: AdapterEntry[] })?.adapters;
      this.adapters = Array.isArray(arr) ? arr : [];
    } catch { /* engine offline */ }
  }

  /** Pulls the active job state. When a job is running, starts a 2s
   *  poll so loss curve + metrics tick live. Stops the poll on
   *  terminal state (done / failed / stopped) — no need to keep
   *  asking once nothing's changing. On the running→done transition
   *  broadcasts "lthn:lemma:adapters-changed" so peer windows pick
   *  up the new adapter dir without manual refresh. */
  private async _refreshJob(): Promise<void> {
    const prevState = this.job?.state ?? "";
    try {
      const Lemma = await import("@desktop/lemma/wailsservice");
      const job = await Lemma.SFTStatus("");
      this.job = job as unknown as SFTJobView;
      this._managePollLifecycle();
      // Edge-trigger broadcast: only on the transition INTO "done"
      // (not on every poll while sitting in "done"). The condition
      // wasRunning && nowDone catches the moment the adapter dir
      // becomes complete. "failed" + "stopped" don't broadcast —
      // there's no new adapter to surface in those cases.
      const wasLive = prevState === "running" || prevState === "pending";
      if (wasLive && this.job?.state === "done") {
        // Re-pull our own Recent Adapters rail so the new entry
        // surfaces locally without waiting for the next poll cycle.
        void this._refreshAdapters();
        try {
          const { Events } = await import("@wailsio/runtime");
          Events.Emit("lthn:lemma:adapters-changed", null);
        } catch { /* wails runtime absent in test contexts */ }
      }
    } catch (e) {
      // 404 when no job — clear local state, stop poll.
      const msg = e instanceof Error ? e.message : String(e);
      if (msg.includes("404") || msg.includes("no SFT")) {
        this.job = null;
        if (this._poll !== null) { clearInterval(this._poll); this._poll = null; }
      }
    }
  }

  private _managePollLifecycle(): void {
    const isLive = this.job && (this.job.state === "pending" || this.job.state === "running");
    if (isLive && this._poll === null) {
      this._poll = setInterval(() => { void this._refreshJob(); }, 2000);
    } else if (!isLive && this._poll !== null) {
      clearInterval(this._poll);
      this._poll = null;
    }
  }

  /** OS-native file picker for the dataset JSONL path. Filters to
   *  .jsonl so the user lands in the right shape; cancel returns
   *  undefined and we leave the field alone. Mirrors the SaveFile
   *  pattern chat-window's _slashExport uses. */
  private async _pickDataset(): Promise<void> {
    try {
      const dlg = await import("@wailsio/runtime").then(m => m.Dialogs);
      const picked = await dlg.OpenFile({
        Title: "Pick training dataset",
        ButtonText: "Use this file",
        Filters: [{ DisplayName: "JSONL", Pattern: "*.jsonl" }],
      });
      if (picked && typeof picked === "string") {
        this.datasetPath = picked;
      }
    } catch (e) {
      this.err = e instanceof Error ? e.message : String(e);
    }
  }

  private async _doStart(): Promise<void> {
    if (this.busy) return;
    this.err = "";
    if (!this.modelPath.trim() || !this.datasetPath.trim()) {
      this.err = "model path + dataset path are both required";
      return;
    }
    this.busy = true;
    try {
      const Lemma = await import("@desktop/lemma/wailsservice");
      const { SFTStartRequest } = await import("@desktop/lemma/models");
      const req = new SFTStartRequest({
        model_path:    this.modelPath,
        dataset_path:  this.datasetPath,
        adapter_name:  this.adapterName,
        batch_size:    this.batchSize,
        epochs:        this.epochs,
        learning_rate: this.learningRate,
        lora_rank:     this.loraRank,
        lora_alpha:    this.loraAlpha,
      });
      const job = await Lemma.SFTStart(req);
      this.job = job as unknown as SFTJobView;
      this._managePollLifecycle();
    } catch (e) {
      this.err = e instanceof Error ? e.message : String(e);
    } finally {
      this.busy = false;
    }
  }

  private async _doStop(): Promise<void> {
    if (!this.job?.job_id || this.busy) return;
    this.err = "";
    this.busy = true;
    try {
      const Lemma = await import("@desktop/lemma/wailsservice");
      const job = await Lemma.SFTStop(this.job.job_id);
      this.job = job as unknown as SFTJobView;
      this._managePollLifecycle();
      void this._refreshAdapters();
    } catch (e) {
      this.err = e instanceof Error ? e.message : String(e);
    } finally {
      this.busy = false;
    }
  }

  /** Test the named adapter overlaid on the base model — calls
   *  Lemma.Reload with adapter_path set, then opens the Chat surface
   *  so the operator can immediately probe the adapter's behaviour
   *  against the base it forked from. The A/B happens by toggling
   *  the adapter on/off via successive reloads. */
  private async _doTestAdapter(adapterPath: string): Promise<void> {
    if (this.busy || !this.modelPath) {
      this.err = "pick a base model first to overlay the adapter on";
      return;
    }
    this.err = "";
    this.busy = true;
    try {
      const Lemma = await import("@desktop/lemma/wailsservice");
      const { ReloadRequest } = await import("@desktop/lemma/models");
      const machine = await Lemma.Machine();
      const machineHash = (machine as unknown as { hash?: string }).hash || "";
      if (!machineHash) {
        this.err = "machine hash unavailable — engine offline?";
        return;
      }
      await Lemma.Reload(new ReloadRequest({
        confirm_machine: machineHash,
        model_path:      this.modelPath,
        adapter_path:    adapterPath,
      }));
      // Open the Chat pane so the operator can drive the test
      // immediately — same Lethean-6 app-shell pane-set pattern the
      // tray "Open Chat" button uses.
      try {
        const WindowService = await import("@desktop/desktop/windowservice");
        await WindowService.Open("app");
        const { Events } = await import("@wailsio/runtime");
        Events.Emit("lthn:app:setpane", "chat");
      } catch { /* non-Wails env — skip the navigation, the reload still happened */ }
    } catch (e) {
      this.err = e instanceof Error ? e.message : String(e);
    } finally {
      this.busy = false;
    }
  }

  /** Small label + numeric input pair for the Recipe rail — matches
   *  the lthn-rail-row visual density. `float` flag accepts decimals
   *  (LR), otherwise integer-only. */
  private _numInput(label: string, value: number, set: (v: number) => void, float: boolean = false) {
    return html`
      <div style="display:flex; align-items:center; justify-content:space-between; gap:8px;">
        <span style="font-size:11.5px; color:var(--fg-3);">${label}</span>
        <input type="number" .value=${String(value)}
               step=${float ? "0.0001" : "1"}
               ?disabled=${this.job?.state === "running" || this.job?.state === "pending"}
               @change=${(e: Event) => {
                 const raw = (e.target as HTMLInputElement).value;
                 const n = float ? parseFloat(raw) : parseInt(raw, 10);
                 if (!Number.isNaN(n)) set(n);
               }}
               style="width:90px; padding:3px 6px; font-size:11px; font-family:var(--font-mono); background:rgba(0,0,0,0.25); color:var(--fg-1); border:1px solid rgba(255,255,255,0.08); border-radius:4px; --wails-draggable:no-drag;">
      </div>
    `;
  }

  render() {
    // Live loss series from job.loss when present; fall back to the
    // deterministic placeholder curve in idle state so the chrome
    // still reads coherently in design preview / first-launch.
    const liveLoss = this.job?.loss && this.job.loss.length > 0
      ? this.job.loss.map(s => s.loss)
      : [];
    const loss = liveLoss.length > 0
      ? liveLoss
      : Array.from({ length: 40 }, (_, i) =>
          2.4 * Math.exp(-i * 0.06) + 0.4 + ((Math.sin(i * 1.7) * 0.08)));
    const cw = 740, ch = 220, pad = { l: 40, r: 14, t: 12, b: 24 };
    const lossMax = loss.length > 0 ? Math.max(...loss, 0.1) : 2.4;
    const isRunning = this.job?.state === "running" || this.job?.state === "pending";
    const stateLabel = this.job ? this.job.state : "idle";

    // Step progress derives from job state — Run is the active step
    // when training; Publish lights up only after a checkpoint exists.
    const activeStepIdx = !this.job ? 2
                        : isRunning  ? 3
                        : this.job.state === "done" ? 4
                        : 3;
    const steps = [
      { id: "1", label: this.t.stepBase },
      { id: "2", label: this.t.stepDataset },
      { id: "3", label: this.t.stepConfig },
      { id: "4", label: this.t.stepRun },
      { id: "5", label: this.t.stepPublish },
    ];

    const toolbar = html`
      ${steps.map((s, i) => html`
        <div style="display:flex; align-items:center; gap:6px;">
          <div style="width:18px; height:18px; border-radius:50%; border:1.5px solid ${i < activeStepIdx ? "var(--brand-500)" : i === activeStepIdx ? "var(--brand-400)" : "rgba(255,255,255,0.12)"}; background:${i < activeStepIdx ? "var(--brand-500)" : "transparent"}; display:flex; align-items:center; justify-content:center; font-size:10px; font-weight:600; color:${i < activeStepIdx ? "#fff" : i === activeStepIdx ? "var(--brand-300)" : "var(--fg-3)"};">
            ${i < activeStepIdx ? html`<i class="fa-solid fa-check" style="font-size:8px;"></i>` : s.id}
          </div>
          <span style="font-size:12px; color:${i <= activeStepIdx ? "var(--fg-0)" : "var(--fg-3)"}; font-weight:${i === activeStepIdx ? 500 : 400};">${s.label}</span>
          ${i < 4 ? html`<span style="width:24px; height:1px; background:rgba(255,255,255,0.08); margin:0 8px;"></span>` : nothing}
        </div>
      `)}
      <div style="flex:1"></div>
      <span style="font-size:11px; color:var(--fg-3); font-family:var(--font-mono);">${stateLabel}</span>
      ${isRunning
        ? html`<lthn-btn tone="quiet" size="sm" ?disabled=${this.busy} @click=${() => { void this._doStop(); }}>
            <i class="fa-solid fa-stop" style="font-size:9px;"></i> ${this.busy ? "Stopping…" : this.t.btnStop}
          </lthn-btn>`
        : html`<lthn-btn tone="primary" size="sm" ?disabled=${this.busy || !this.modelPath || !this.datasetPath} @click=${() => { void this._doStart(); }}>
            <i class="fa-solid fa-play" style="font-size:9px;"></i> ${this.busy ? "Starting…" : "Run"}
          </lthn-btn>`}
    `;

    const lossPath = loss.length > 1
      ? "M " + loss.map((v, i) =>
          `${pad.l + (i / (loss.length - 1)) * (cw - pad.l - pad.r)} ${pad.t + (1 - v / lossMax) * (ch - pad.t - pad.b)}`
        ).join(" L ")
      : "";

    const body = html`
      <div style="flex:1; display:grid; grid-template-columns:300px 1fr 320px; min-height:0;">
        <aside style="background:rgba(0,0,0,0.18); border-right:1px solid rgba(255,255,255,0.05); padding:18px; overflow:auto; display:flex; flex-direction:column; gap:16px;">
          <div>
            <lthn-label>${this.t.labelRecipe}</lthn-label>
            <div style="margin-top:8px; display:flex; flex-direction:column; gap:8px; font-size:11.5px;">
              <lthn-rail-row k=${this.t.rowMethod} v="LoRA · AdamW"></lthn-rail-row>
              ${this._numInput(this.t.rowRank,    this.loraRank,     v => { this.loraRank = v; })}
              ${this._numInput(this.t.rowAlpha,   this.loraAlpha,    v => { this.loraAlpha = v; })}
              ${this._numInput(this.t.rowLR,      this.learningRate, v => { this.learningRate = v; }, true)}
              ${this._numInput(this.t.rowBatch,   this.batchSize,    v => { this.batchSize = v; })}
              ${this._numInput(this.t.rowEpochs,  this.epochs,       v => { this.epochs = v; })}
              <lthn-rail-row k=${this.t.rowTargets} v="q_proj · v_proj · o_proj"></lthn-rail-row>
            </div>
          </div>
          <div style="height:1px; background:rgba(255,255,255,0.05);"></div>
          <div>
            <lthn-label>${this.t.labelBaseDataset}</lthn-label>
            <div style="margin-top:8px; display:flex; flex-direction:column; gap:8px; font-size:11.5px;">
              <div style="display:flex; flex-direction:column; gap:4px;">
                <span style="font-size:10.5px; color:var(--fg-3);">${this.t.rowBase}</span>
                <select .value=${this.modelPath}
                        ?disabled=${isRunning}
                        @change=${(e: Event) => { this.modelPath = (e.target as HTMLSelectElement).value; }}
                        style="padding:5px 7px; font-size:11px; font-family:var(--font-mono); background:rgba(0,0,0,0.25); color:var(--fg-1); border:1px solid rgba(255,255,255,0.08); border-radius:4px; --wails-draggable:no-drag;">
                  ${this.baseModels.length === 0
                    ? html`<option value="">(no models — start lthn serve)</option>`
                    : this.baseModels.map(m => html`<option value=${m.path}>${m.name}</option>`)}
                </select>
              </div>
              <div style="display:flex; flex-direction:column; gap:4px;">
                <span style="font-size:10.5px; color:var(--fg-3);">${this.t.rowDataset} (JSONL path)</span>
                <div style="display:flex; gap:6px;">
                  <input type="text" .value=${this.datasetPath}
                         ?disabled=${isRunning}
                         @input=${(e: Event) => { this.datasetPath = (e.target as HTMLInputElement).value; }}
                         placeholder="/Lethean/data/datasets/your.jsonl"
                         style="flex:1; padding:5px 7px; font-size:11px; font-family:var(--font-mono); background:rgba(0,0,0,0.25); color:var(--fg-1); border:1px solid rgba(255,255,255,0.08); border-radius:4px; --wails-draggable:no-drag;">
                  <lthn-btn tone="ghost" size="sm"
                    ?disabled=${isRunning}
                    @click=${() => { void this._pickDataset(); }}>
                    <i class="fa-regular fa-folder-open" style="font-size:10px;"></i>
                    Browse
                  </lthn-btn>
                </div>
              </div>
              <div style="display:flex; flex-direction:column; gap:4px;">
                <span style="font-size:10.5px; color:var(--fg-3);">Adapter name (optional)</span>
                <input type="text" .value=${this.adapterName}
                       ?disabled=${isRunning}
                       @input=${(e: Event) => { this.adapterName = (e.target as HTMLInputElement).value; }}
                       placeholder="my-adapter-name"
                       style="padding:5px 7px; font-size:11px; font-family:var(--font-mono); background:rgba(0,0,0,0.25); color:var(--fg-1); border:1px solid rgba(255,255,255,0.08); border-radius:4px; --wails-draggable:no-drag;">
              </div>
              <lthn-rail-row k=${this.t.rowFormat} v="ChatML / JSONL"></lthn-rail-row>
            </div>
          </div>
          ${this.err ? html`
            <div style="padding:8px 10px; font-size:11px; color:var(--error-400, #f82da7); background:rgba(248,45,167,0.06); border:1px solid rgba(248,45,167,0.22); border-radius:4px;">
              ${this.err}
            </div>` : nothing}
        </aside>

        <main style="padding:20px 26px; overflow:auto; display:flex; flex-direction:column; gap:18px;">
          <div style="display:grid; grid-template-columns:repeat(4, 1fr); gap:8px;">
            ${[
              { k: this.t.mEpoch, v: this.job ? `${this.job.epoch} / ${this.epochs}` : "—",
                sub: this.job ? `step ${this.job.step}` : "(idle)" },
              { k: this.t.mLoss,  v: this.job && this.job.last_loss > 0 ? this.job.last_loss.toFixed(3) : "—",
                sub: this.job ? `${this.job.samples} samples` : "(idle)" },
              { k: this.t.mTps,   v: "—",
                sub: "training throughput (TODO)" },
              { k: this.t.mWatts, v: "—",
                sub: "GPU + ANE (TODO)" },
            ].map(m => html`
              <div style="padding:12px 14px; border-radius:8px; background:rgba(255,255,255,0.025); border:1px solid rgba(255,255,255,0.06);">
                <div style="font-size:10.5px; color:var(--fg-3); letter-spacing:0.04em; text-transform:uppercase;">${m.k}</div>
                <div style="font-family:var(--font-mono); font-size:22px; color:var(--fg-0); margin-top:4px; letter-spacing:-0.01em;">${m.v}</div>
                <div style="font-size:10.5px; color:var(--fg-3); margin-top:3px;">${m.sub}</div>
              </div>
            `)}
          </div>
          <div style="background:rgba(0,0,0,0.20); border:1px solid rgba(255,255,255,0.05); border-radius:8px; padding:12px;">
            <lthn-label>${this.t.labelLoss}</lthn-label>
            <svg viewBox="0 0 ${cw} ${ch}" width="100%" height=${ch} preserveAspectRatio="none" style="margin-top:4px;">
              ${[0, 0.6, 1.2, 1.8, 2.4].map(y => {
                const yy = pad.t + (1 - y / 2.4) * (ch - pad.t - pad.b);
                return html`
                  <line x1=${pad.l} x2=${cw - pad.r} y1=${yy} y2=${yy} stroke="rgba(255,255,255,0.05)"></line>
                  <text x=${pad.l - 6} y=${yy + 3} fill="rgba(255,255,255,0.40)" font-size="9.5" text-anchor="end" font-family="ui-monospace, monospace">${y.toFixed(1)}</text>
                `;
              })}
              <path d=${lossPath} stroke="var(--brand-400)" stroke-width="1.6" fill="none"></path>
              <line x1=${pad.l + 0.68 * (cw - pad.l - pad.r)} x2=${pad.l + 0.68 * (cw - pad.l - pad.r)}
                y1=${pad.t} y2=${ch - pad.b} stroke="var(--warning-400)" stroke-dasharray="3 3"></line>
              <text x=${pad.l + 0.68 * (cw - pad.l - pad.r) + 4} y=${pad.t + 12} fill="var(--warning-400)" font-size="9.5" font-family="ui-monospace, monospace">${this.t.epochMarker}</text>
            </svg>
          </div>
          <div>
            <lthn-label>${this.t.labelSample}</lthn-label>
            <div style="margin-top:8px; display:grid; grid-template-columns:1fr 1fr; gap:8px;">
              ${[
                { who: this.t.whoBase, text: "Sure! Here are some general tips that may help you set up a Lethean instance, though I'm not certain about the specifics…", tone: "var(--fg-3)" },
                { who: this.t.whoOurs, text: "Add `LTHN_HOME=~/.lthn` to your shell, then `lthn runner start --model gemma-4-e2b`. The tray icon should appear within a few seconds.", tone: "var(--fg-1)" },
              ].map(s => html`
                <div style="padding:10px 12px; border-radius:6px; background:rgba(255,255,255,0.025); border:1px solid rgba(255,255,255,0.05);">
                  <div style="font-family:var(--font-mono); font-size:9.5px; color:var(--fg-3); letter-spacing:0.06em; text-transform:uppercase; margin-bottom:6px;">${s.who}</div>
                  <div style="font-size:11.5px; color:${s.tone}; line-height:1.55;">${s.text}</div>
                </div>
              `)}
            </div>
          </div>
        </main>

        <aside style="background:rgba(0,0,0,0.18); border-left:1px solid rgba(255,255,255,0.05); padding:18px; overflow:auto; display:flex; flex-direction:column; gap:14px;">
          <div>
            <lthn-label>${this.t.labelAdapter}</lthn-label>
            ${this.adapters.length === 0
              ? html`<div style="font-size:11px; color:var(--fg-3); font-style:italic; margin-top:6px;">No adapters yet — run a Fine-tune to create one.</div>`
              : html`
                <div style="margin-top:6px; display:flex; flex-direction:column; gap:6px; max-height:240px; overflow:auto;">
                  ${[...this.adapters].sort((a, b) => b.modified_unix - a.modified_unix).slice(0, 6).map(a => html`
                    <div style="padding:6px 8px; background:rgba(255,255,255,0.025); border:1px solid rgba(255,255,255,0.05); border-radius:5px; display:flex; flex-direction:column; gap:4px;">
                      <div style="font-family:var(--font-mono); font-size:11px; color:var(--fg-0); word-break:break-all;">${a.name}</div>
                      <div style="display:flex; align-items:center; justify-content:space-between; gap:6px;">
                        <span style="font-size:10px; color:var(--fg-3);">${(a.size_bytes / (1024 * 1024)).toFixed(1)} MB</span>
                        <lthn-btn tone="ghost" size="sm" ?disabled=${this.busy || !this.modelPath}
                                  @click=${() => { void this._doTestAdapter(a.path); }}
                                  style="--wails-draggable:no-drag;">
                          <i class="fa-regular fa-comment" style="font-size:9px;"></i> Test
                        </lthn-btn>
                      </div>
                    </div>
                  `)}
                </div>`}
          </div>
          <div style="display:flex; flex-direction:column; gap:6px;">
            <lthn-btn tone="ghost" size="md" disabled title="Merge adapter → base — comes once go-mlx exposes the merge verb over admin"><i class="fa-solid fa-code-merge" style="font-size:11px;"></i> ${this.t.btnMerge}</lthn-btn>
            <lthn-btn tone="ghost" size="md" disabled title="Push to HuggingFace — comes once token auth lands"><i class="fa-solid fa-cloud-arrow-up" style="font-size:11px;"></i> ${this.t.btnPushHf}</lthn-btn>
          </div>
          <div style="height:1px; background:rgba(255,255,255,0.05);"></div>
          <div>
            <lthn-label>${this.t.labelSystem}</lthn-label>
            <div style="margin-top:8px; display:flex; flex-direction:column; gap:6px; font-size:11px;">
              <lthn-rail-row k=${this.t.rowBackend} v="go-mlx · Metal"></lthn-rail-row>
              <lthn-rail-row k=${this.t.rowGpuMem}  v="13.2 / 36 GB"></lthn-rail-row>
              <lthn-rail-row k=${this.t.rowDiskIo}  v="22 MB/s"></lthn-rail-row>
              <lthn-rail-row k=${this.t.rowEta}     v="14 min"></lthn-rail-row>
            </div>
          </div>
          <div style="font-size:11px; color:var(--fg-3); font-style:italic; line-height:1.55;">
            ${this.t.calloutLocal}
          </div>
        </aside>
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
customElements.define("lthn-distillation-window", LthnDistillationWindow);
