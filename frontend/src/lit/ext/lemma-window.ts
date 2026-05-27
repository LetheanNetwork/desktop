// SPDX-Licence-Identifier: EUPL-1.2
// E5.1 · lemma — <lthn-lemma-window>
//
// Status + admin panel for the local lthn-mlx engine. Mounted at
// ?surface=lemma. The status read uses the unauthenticated /v1/health
// + /v1/models endpoints (works even when the desktop binary has no
// admin token); admin verbs (reload, download, profiles, status detail)
// route through the Wails `Lemma` binding so the Bearer token never
// leaves Go.
//
// Light-DOM Lit element. Composes renderChrome() from ../chrome.js.

import { LitElement, html, nothing } from "lit";
import { renderChrome } from "../chrome";
import * as Lemma from "../../../bindings/dappco.re/lthn/desktop/pkg/lemma/wailsservice.js";

interface HealthPayload {
  status?: string;
  runtime?: string;
  models?: string[];
  time?: number;
}

interface ModelEntry {
  id?: string;
  object?: string;
}

interface ModelsPayload {
  object?: string;
  data?: ModelEntry[];
}

interface DownloadProgress {
  jobId: string;
  status: string;
  progress: number;
  bytes: number;
  error?: string;
}

class LthnLemmaWindow extends LitElement {
  static readonly properties = {
    w:           { type: Number },
    h:           { type: Number },
    embedded:    { type: Boolean, reflect: true },
    endpoint:    { type: String },
    chrome:      { state: true },
    health:      { state: true },
    models:      { state: true },
    err:         { state: true },
    // admin-side state
    adminStatus: { state: true },
    sftJob:      { state: true },
    sftStopBusy: { state: true },
    sftStopErr:  { state: true },
    machineHash: { state: true },
    profiles:    { state: true },
    // Pulled from Models.List() at Admin-panel open. Drives the
    // reload picker — falls back to a free-text <input> when the
    // list is empty or the call errors (lthn-mlx may not yet have
    // surfaced models the user just dropped into the dir).
    availableModels: { state: true },
    reloadModel: { state: true },
    reloadProfile: { state: true },
    reloadBusy:  { state: true },
    reloadErr:   { state: true },
    downloadRepo: { state: true },
    downloadJob: { state: true },
    downloadErr: { state: true },
    showAdmin:   { state: true },
  };
  declare w: number;
  declare h: number;
  declare embedded: boolean;
  declare endpoint: string;
  declare chrome: { title: string; subtitle: string };
  declare health: HealthPayload | null;
  declare models: ModelEntry[];
  declare err: string;
  declare adminStatus: any;
  /** Active SFT job snapshot from Lemma.SFTStatus(""). Null when no
   *  job has ever run; non-null but state !== "running" means the
   *  registry is holding a terminal-state record (done/failed/
   *  stopped) the user hasn't dismissed yet. Only state ===
   *  "running" renders the in-flight indicator. */
  declare sftJob: { state?: string; epoch?: number; last_loss?: number; adapter_dir?: string; job_id?: string } | null;
  /** Busy flag while a Stop request is in flight — keeps the user
   *  from double-clicking the button before the upstream
   *  responds. Mirrors the reloadBusy / downloadJob patterns. */
  declare sftStopBusy: boolean;
  declare sftStopErr: string;
  declare machineHash: string;
  declare profiles: any[];
  declare availableModels: Array<{ name: string; path: string; size: number; is_dir: boolean }>;
  declare reloadModel: string;
  declare reloadProfile: string;
  declare reloadBusy: boolean;
  declare reloadErr: string;
  declare downloadRepo: string;
  declare downloadJob: DownloadProgress | null;
  declare downloadErr: string;
  declare showAdmin: boolean;
  private pollHandle: ReturnType<typeof setInterval> | null = null;
  private downloadPollHandle: ReturnType<typeof setInterval> | null = null;

  constructor() {
    super();
    this.w = 720; this.h = 560; this.embedded = false;
    this.endpoint = "http://localhost:11434";
    this.chrome = { title: "Lemma", subtitle: "Local AI engine · OpenAI / Anthropic / Ollama compatible" };
    this.health = null;
    this.models = [];
    this.err = "";
    this.adminStatus = null;
    this.sftJob = null;
    this.sftStopBusy = false;
    this.sftStopErr = "";
    this.machineHash = "";
    this.profiles = [];
    this.availableModels = [];
    this.reloadModel = "";
    this.reloadProfile = "";
    this.reloadBusy = false;
    this.reloadErr = "";
    this.downloadRepo = "";
    this.downloadJob = null;
    this.downloadErr = "";
    this.showAdmin = false;
  }

  createRenderRoot() { return this; }

  connectedCallback() {
    super.connectedCallback();
    void this.poll();
    this.pollHandle = setInterval(() => { void this.poll(); }, 2000);
  }

  disconnectedCallback() {
    super.disconnectedCallback();
    if (this.pollHandle !== null) {
      clearInterval(this.pollHandle);
      this.pollHandle = null;
    }
    if (this.downloadPollHandle !== null) {
      clearInterval(this.downloadPollHandle);
      this.downloadPollHandle = null;
    }
  }

  private async poll(): Promise<void> {
    try {
      const [hRes, mRes] = await Promise.allSettled([
        fetch(this.endpoint + "/v1/health", { cache: "no-store" }),
        fetch(this.endpoint + "/v1/models", { cache: "no-store" }),
      ]);
      let nextErr = "";
      let nextHealth: HealthPayload | null = null;
      let nextModels: ModelEntry[] = [];
      if (hRes.status === "fulfilled" && hRes.value.ok) {
        nextHealth = (await hRes.value.json()) as HealthPayload;
      } else if (hRes.status === "rejected") {
        nextErr = String(hRes.reason);
      }
      if (mRes.status === "fulfilled" && mRes.value.ok) {
        const body = (await mRes.value.json()) as ModelsPayload;
        nextModels = body.data ?? [];
      }
      this.health = nextHealth;
      this.models = nextModels;
      this.err = nextErr;
    } catch (e) {
      this.err = String(e);
      this.health = null;
      this.models = [];
    }
  }

  private async loadAdmin(): Promise<void> {
    this.reloadErr = "";
    try {
      const Models = await import("../../../bindings/dappco.re/lthn/desktop/pkg/models/wailsservice.js");
      const { unwrap } = await import("../result");
      const [statusRes, machineRes, profilesRes, modelsRes, sftRes] = await Promise.allSettled([
        Lemma.Status(),
        Lemma.Machine(),
        Lemma.Profiles(),
        // Models.List returns a core.Result wrapped Entry[]; unwrap()
        // collapses to a plain array (or the fallback on failure) so
        // the picker can iterate without per-call OK checks.
        unwrap<Array<{ name: string; path: string; size: number; is_dir: boolean }>>(Models.List(), []),
        // SFTStatus("") returns the active job (or last completed) —
        // 404 when no job has ever run. Both are "no SFT running" for
        // the panel; only state === "running" surfaces in render.
        Lemma.SFTStatus(""),
      ]);
      if (statusRes.status === "fulfilled") {
        this.adminStatus = statusRes.value;
        if (!this.reloadModel) this.reloadModel = statusRes.value?.model_path ?? "";
      } else {
        this.reloadErr = "status: " + String(statusRes.reason);
      }
      if (machineRes.status === "fulfilled") {
        this.machineHash = machineRes.value?.hash ?? "";
      }
      if (profilesRes.status === "fulfilled") {
        this.profiles = profilesRes.value?.profiles ?? [];
      }
      if (modelsRes.status === "fulfilled") {
        this.availableModels = modelsRes.value ?? [];
      }
      this.sftJob = sftRes.status === "fulfilled" ? sftRes.value : null;
    } catch (e) {
      this.reloadErr = String(e);
    }
  }

  private async toggleAdmin(): Promise<void> {
    this.showAdmin = !this.showAdmin;
    if (this.showAdmin) await this.loadAdmin();
  }

  /** Cancel the active SFT job. Mirrors distillation-window's
   *  _doStop — single-flight upstream means there's only ever one
   *  job to cancel, but we still pass the job_id so the upstream
   *  refuses a stale UI's "stop whatever is running" pattern.
   *  Checkpoints already written to the adapter dir survive — only
   *  the gradient loop stops. */
  private async doStopSFT(): Promise<void> {
    if (this.sftStopBusy) return;
    const jobID = this.sftJob?.job_id ?? "";
    if (!jobID) {
      this.sftStopErr = "no job id — refresh the admin panel and try again";
      return;
    }
    this.sftStopBusy = true;
    this.sftStopErr = "";
    try {
      await Lemma.SFTStop(jobID);
      // Re-pull so the Training row collapses (state flips to
      // "stopped" upstream + the !== "running" guard hides it).
      await this.loadAdmin();
    } catch (e) {
      this.sftStopErr = e instanceof Error ? e.message : String(e);
    } finally {
      this.sftStopBusy = false;
    }
  }

  private async doReload(e: Event): Promise<void> {
    e.preventDefault();
    if (this.reloadBusy) return;
    if (!this.machineHash) {
      this.reloadErr = "machine hash not loaded — open admin panel first";
      return;
    }
    if (!this.reloadModel && !this.reloadProfile) {
      this.reloadErr = "pick a model path or profile";
      return;
    }
    this.reloadBusy = true;
    this.reloadErr = "";
    try {
      await Lemma.Reload({
        ConfirmMachine: this.machineHash,
        ModelPath: this.reloadModel,
        ProfilePath: this.reloadProfile,
        ContextLength: 0,
      } as any);
      // Refresh status — the hot-swap mutates serve state.
      await this.loadAdmin();
      await this.poll();
    } catch (err) {
      this.reloadErr = String(err);
    } finally {
      this.reloadBusy = false;
    }
  }

  private async doDownload(e: Event): Promise<void> {
    e.preventDefault();
    if (!this.downloadRepo.trim()) {
      this.downloadErr = "repo id required (e.g. lthn/lemer-lite)";
      return;
    }
    this.downloadErr = "";
    try {
      const jobId = await Lemma.Download({ RepoID: this.downloadRepo.trim(), Revision: "" } as any);
      this.downloadJob = { jobId, status: "pending", progress: 0, bytes: 0 };
      if (this.downloadPollHandle !== null) {
        clearInterval(this.downloadPollHandle);
      }
      this.downloadPollHandle = setInterval(() => { void this.pollDownload(jobId); }, 2000);
    } catch (err) {
      this.downloadErr = String(err);
    }
  }

  private async pollDownload(jobId: string): Promise<void> {
    try {
      const js = await Lemma.DownloadJob(jobId);
      this.downloadJob = {
        jobId: js.job_id ?? jobId,
        status: js.status ?? "?",
        progress: js.progress ?? 0,
        bytes: js.bytes ?? 0,
        error: js.error,
      };
      if (js.status === "done" || js.status === "failed") {
        if (this.downloadPollHandle !== null) {
          clearInterval(this.downloadPollHandle);
          this.downloadPollHandle = null;
        }
        // Refresh the admin lane on completion. "done" matters most
        // — Models.List() now includes the freshly-downloaded file
        // so the Reload picker reflects it without the user having
        // to close + reopen the panel. "failed" reloads too so any
        // stale state from the in-flight attempt is reconciled.
        if (this.showAdmin) {
          void this.loadAdmin();
        }
      }
    } catch (err) {
      this.downloadErr = String(err);
      if (this.downloadPollHandle !== null) {
        clearInterval(this.downloadPollHandle);
        this.downloadPollHandle = null;
      }
    }
  }

  private renderStatus() {
    if (this.health && this.health.status === "ok") {
      return html`<lthn-state-pill variant="ok">serving</lthn-state-pill>`;
    }
    if (this.err) {
      return html`<lthn-state-pill variant="warn">unreachable</lthn-state-pill>`;
    }
    return html`<lthn-state-pill variant="muted">idle</lthn-state-pill>`;
  }

  private renderAdmin() {
    if (!this.showAdmin) return nothing;
    return html`
      <div style="display:flex; flex-direction:column; gap:14px; padding:12px; border-top:1px solid var(--border, #2a2a2a); margin-top:8px;">
        <div style="display:flex; align-items:center; gap:8px;">
          <lthn-label>Admin</lthn-label>
          ${this.machineHash
            ? html`<span style="font-family:ui-monospace, monospace; font-size:11px; opacity:0.6;">machine ${this.machineHash.substring(0, 16)}…</span>`
            : html`<span style="font-size:11px; opacity:0.5; color:var(--error, #f82da7);">no admin token — start lthn-mlx first</span>`}
        </div>

        ${this.adminStatus
          ? html`
            <div style="display:flex; flex-direction:column; gap:4px; font-size:12px;">
              <lthn-rail-row k="Model path" v="${this.adminStatus.model_path || "—"}"></lthn-rail-row>
              ${this.adminStatus.profile_path
                ? html`<lthn-rail-row k="Profile" v="${this.adminStatus.profile_path}"></lthn-rail-row>`
                : nothing}
              ${this.adminStatus.config?.adapter_path
                ? html`<lthn-rail-row k="Adapter" v="${this.adminStatus.config.adapter_path}"></lthn-rail-row>`
                : nothing}
              <lthn-rail-row k="Context" v="${this.adminStatus.config?.context_length ?? 0}"></lthn-rail-row>
              <lthn-rail-row k="Cache" v="${this.adminStatus.config?.prompt_cache ? "on" : "off"} (${this.adminStatus.config?.cache_policy || "—"})"></lthn-rail-row>
              ${this.sftJob?.state === "running"
                ? html`
                  <div style="display:flex; align-items:center; gap:8px;">
                    <div style="flex:1;">
                      <lthn-rail-row k="Training"
                        v="epoch ${this.sftJob.epoch ?? 0} · loss ${(this.sftJob.last_loss ?? 0).toFixed(3)}"></lthn-rail-row>
                    </div>
                    <lthn-btn tone="ghost" size="sm"
                      ?disabled=${this.sftStopBusy}
                      @click=${() => this.doStopSFT()}>
                      <i class="fa-solid fa-stop" style="font-size:9px;"></i>
                      ${this.sftStopBusy ? "Stopping…" : "Stop"}
                    </lthn-btn>
                  </div>
                  ${this.sftStopErr ? html`
                    <div style="font-size:10.5px; color:var(--error-400); padding-left:8px;">
                      ${this.sftStopErr}
                    </div>
                  ` : nothing}`
                : nothing}
            </div>`
          : nothing}

        <form @submit=${this.doReload}>
          <lthn-label>Hot-swap model</lthn-label>
          ${this.availableModels.length > 0
            ? html`
              <select .value=${this.reloadModel}
                      @change=${(e: Event) => { this.reloadModel = (e.target as HTMLSelectElement).value; }}
                      style="width:100%; box-sizing:border-box; padding:6px 8px; margin-top:4px; font-family:ui-monospace, monospace; font-size:12px; background:var(--bg-elevated, #1a1a1a); color:inherit; border:1px solid var(--border, #2a2a2a); border-radius:4px;">
                ${this.availableModels.map(m => {
                  const isCurrent = m.path === (this.adminStatus?.model_path ?? "");
                  return html`<option value=${m.path} ?selected=${isCurrent}>${m.name}${isCurrent ? "  (loaded)" : ""}</option>`;
                })}
              </select>`
            : html`
              <input type="text" .value=${this.reloadModel}
                     @input=${(e: Event) => { this.reloadModel = (e.target as HTMLInputElement).value; }}
                     placeholder="/path/to/model-dir or model id"
                     style="width:100%; box-sizing:border-box; padding:6px 8px; margin-top:4px; font-family:ui-monospace, monospace; font-size:12px; background:var(--bg-elevated, #1a1a1a); color:inherit; border:1px solid var(--border, #2a2a2a); border-radius:4px;">`}

          ${this.profiles.length > 0
            ? html`
              <label style="display:block; margin-top:8px; font-size:11px; opacity:0.7;">Profile (optional)</label>
              <select .value=${this.reloadProfile}
                      @change=${(e: Event) => { this.reloadProfile = (e.target as HTMLSelectElement).value; }}
                      style="width:100%; margin-top:4px; padding:6px 8px; background:var(--bg-elevated, #1a1a1a); color:inherit; border:1px solid var(--border, #2a2a2a); border-radius:4px;">
                <option value="">(use serve default)</option>
                ${this.profiles.map((p: any) => html`<option value=${p.path || p.name}>${p.name} — ${p.backend || "?"}</option>`)}
              </select>`
            : nothing}

          <button type="submit" ?disabled=${this.reloadBusy || !this.machineHash}
                  style="margin-top:10px; padding:6px 14px; font-size:13px; cursor:${this.reloadBusy ? "wait" : "pointer"}; background:var(--accent, #4c8bf5); color:white; border:none; border-radius:4px;">
            ${this.reloadBusy ? "Reloading…" : "Reload"}
          </button>
          ${this.reloadErr
            ? html`<div style="margin-top:6px; font-size:11px; color:var(--error, #f82da7);">${this.reloadErr}</div>`
            : nothing}
        </form>

        <form @submit=${this.doDownload}>
          <lthn-label>Download model from HuggingFace</lthn-label>
          <div style="display:flex; gap:6px; margin-top:4px;">
            <input type="text" .value=${this.downloadRepo}
                   @input=${(e: Event) => { this.downloadRepo = (e.target as HTMLInputElement).value; }}
                   placeholder="org/repo (e.g. lthn/lemer-lite)"
                   style="flex:1; padding:6px 8px; font-family:ui-monospace, monospace; font-size:12px; background:var(--bg-elevated, #1a1a1a); color:inherit; border:1px solid var(--border, #2a2a2a); border-radius:4px;">
            <button type="submit"
                    style="padding:6px 14px; font-size:13px; cursor:pointer; background:var(--accent, #4c8bf5); color:white; border:none; border-radius:4px;">
              Download
            </button>
          </div>
          ${this.downloadJob
            ? html`
              <div style="margin-top:8px; font-size:11px;">
                <div>job ${this.downloadJob.jobId} — <strong>${this.downloadJob.status}</strong> ${this.downloadJob.progress}%</div>
                ${this.downloadJob.error
                  ? html`<div style="color:var(--error, #f82da7);">${this.downloadJob.error}</div>`
                  : nothing}
              </div>`
            : nothing}
          ${this.downloadErr
            ? html`<div style="margin-top:6px; font-size:11px; color:var(--error, #f82da7);">${this.downloadErr}</div>`
            : nothing}
        </form>
      </div>
    `;
  }

  private renderBody() {
    const runtime = this.health?.runtime ?? "—";
    return html`
      <div style="display:flex; flex-direction:column; gap:12px; padding:16px;">
        <lthn-rail-row k="Status" v="${nothing}">${this.renderStatus()}</lthn-rail-row>
        <lthn-rail-row k="Runtime" v="${runtime}"></lthn-rail-row>
        <lthn-rail-row k="Endpoint" v="${this.endpoint}"></lthn-rail-row>
        <lthn-label>Loaded models</lthn-label>
        ${this.models.length === 0
          ? html`<div style="opacity:0.6; font-size:13px; padding:8px 0;">No models loaded. Start the engine from the menu bar.</div>`
          : html`<ul style="list-style:none; padding:0; margin:0; display:flex; flex-direction:column; gap:4px;">
              ${this.models.map((m) => html`<li style="font-family:ui-monospace, monospace; font-size:12px; opacity:0.85;">${m.id ?? "(unnamed)"}</li>`)}
            </ul>`}

        <div style="margin-top:4px;">
          <button @click=${this.toggleAdmin}
                  style="padding:6px 12px; font-size:12px; cursor:pointer; background:transparent; color:inherit; border:1px solid var(--border, #2a2a2a); border-radius:4px;">
            ${this.showAdmin ? "Hide admin" : "Admin…"}
          </button>
        </div>

        ${this.renderAdmin()}

        ${this.err
          ? html`<div style="opacity:0.6; font-size:11px; color:var(--error, #f82da7); padding-top:8px;">${this.err}</div>`
          : nothing}
      </div>
    `;
  }

  render() {
    return renderChrome({
      title: this.chrome.title,
      subtitle: this.chrome.subtitle,
      w: this.w,
      h: this.h,
      embedded: this.embedded,
      body: this.renderBody(),
    });
  }
}

customElements.define("lthn-lemma-window", LthnLemmaWindow);
