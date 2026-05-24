// SPDX-Licence-Identifier: EUPL-1.2

// <lthn-view-ml-lab> — ML Lab Workbench shell, per
// plans/project/lthn/desktop/RFC.ml-lab.md §2 architecture diagram.
//
// Layout:
//   ┌──────────────────────────────────────────────────────────────┐
//   │ ML Lab      [Influx][DuckDB][Models][LoRA]    ASK …          │  titlebar
//   ├──────────┬───────────────────────────────────────────────────┤
//   │ Models   │                                                   │
//   │ • lemma… │                                                   │
//   │ Runs     │           <slot> — active tab body                │
//   │ • run-…  │                                                   │
//   │ Saved    │                                                   │
//   │ • …      │                                                   │
//   ├──────────┴───────────────────────────────────────────────────┤
//   │ selected: lemma-12b-v4-p6.model · run-2026-05-22-…           │  status
//   └──────────────────────────────────────────────────────────────┘
//
// Tab routing is internal state — the four tab elements
// (<lthn-view-ml-lab-{influx,duckdb,models,lora}>) mount lazily as
// the user clicks each pill. Shared selection lives on the shell
// and is forwarded to each tab via attribute reflection
// (selected-model, selected-run, time-range-start, time-range-stop).
// @lit/context binding lands later if attribute fan-out grows ugly.

import { LitElement, html, nothing } from "lit";
import { renderChrome } from "../../chrome";
import { apiFetch } from "../../api-fetch";

// Side-effect imports register the child custom elements; mounting
// is lazy via dynamic-import in _ensureTab so a heavyweight tab body
// (influx chart, duckdb result table) doesn't load until visited.
// The Models + LoRA tabs already exist (pre-shell work); Influx +
// DuckDB ship alongside this shell.
import "./models";
import "./lora";
import "./influx";
import "./duckdb";

type TabID = "influx" | "duckdb" | "models" | "lora";

const TAB_ORDER: TabID[] = ["influx", "duckdb", "models", "lora"];

const TAB_LABEL: Record<TabID, string> = {
  influx:  "Influx",
  duckdb:  "DuckDB",
  models:  "Models",
  lora:    "LoRA",
};

const TAB_ELEMENT: Record<TabID, string> = {
  influx:  "lthn-view-ml-lab-influx",
  duckdb:  "lthn-view-ml-lab-duckdb",
  models:  "lthn-view-ml-lab-models",
  lora:    "lthn-view-ml-lab-lora",
};

interface ModelLite  { filename: string; arch: string; fingerprint: string }
interface RunLite    { id: string; status: string; base_model: string }
interface SavedLite  { id: string; tab: string; name: string }

class LthnViewMlLab extends LitElement {
  static readonly properties = {
    w:             { type: Number },
    h:             { type: Number },
    embedded:      { type: Boolean, reflect: true },
    tab:           { type: String, reflect: true },
    selectedModel: { state: true, attribute: "selected-model", reflect: true },
    selectedRun:   { state: true, attribute: "selected-run", reflect: true },
    timeRangeStart:{ state: true, attribute: "time-range-start", reflect: true },
    timeRangeStop: { state: true, attribute: "time-range-stop", reflect: true },
    models:        { state: true },
    runs:          { state: true },
    savedQueries:  { state: true },
    askPrompt:     { state: true },
    askPending:    { state: true },
  };
  declare w: number;
  declare h: number;
  declare embedded: boolean;
  declare tab: TabID;
  declare selectedModel: string;
  declare selectedRun: string;
  declare timeRangeStart: string;
  declare timeRangeStop: string;
  declare models: ModelLite[];
  declare runs: RunLite[];
  declare savedQueries: SavedLite[];
  declare askPrompt: string;
  declare askPending: boolean;

  constructor() {
    super();
    this.w = 1400;
    this.h = 880;
    this.embedded = false;
    this.tab = "models";
    this.selectedModel = "";
    this.selectedRun = "";
    // Default time-window: last 6h. Sidebar topbar binds these as
    // ${start}/${stop} into Flux template vars when the Influx tab
    // executes.
    const now = new Date();
    this.timeRangeStop  = now.toISOString();
    this.timeRangeStart = new Date(now.getTime() - 6 * 60 * 60 * 1000).toISOString();
    this.models = [];
    this.runs = [];
    this.savedQueries = [];
    this.askPrompt = "";
    this.askPending = false;
  }
  createRenderRoot() { return this; }

  async connectedCallback(): Promise<void> {
    super.connectedCallback();
    await Promise.all([
      this._loadModels(),
      this._loadRuns(),
      this._loadSavedQueries(),
    ]);
  }

  // _loadModels / _loadRuns / _loadSavedQueries fetch the navigator
  // shapes from the lab backend. Each falls back to an empty list
  // when the backend is degraded; sidebar headers stay visible so
  // the user can still navigate by typing into the editor.
  private async _loadModels(): Promise<void> {
    try {
      const res = await apiFetch("/v1/ml-lab/models", { method: "GET" });
      if (!res.ok) return;
      const json = await res.json() as { models?: ModelLite[] };
      this.models = json.models ?? [];
    } catch { /* offline */ }
  }
  private async _loadRuns(): Promise<void> {
    try {
      const res = await apiFetch("/v1/ml-lab/runs", { method: "GET" });
      if (!res.ok) return;
      const json = await res.json() as { runs?: RunLite[] };
      this.runs = json.runs ?? [];
    } catch { /* offline */ }
  }
  private async _loadSavedQueries(): Promise<void> {
    try {
      const res = await apiFetch("/v1/ml-lab/saved-queries", { method: "GET" });
      if (!res.ok) return;
      const json = await res.json() as { queries?: SavedLite[] };
      this.savedQueries = json.queries ?? [];
    } catch { /* offline */ }
  }

  // _selectTab swaps the rendered tab body element.
  private _selectTab(tab: TabID): void {
    if (this.tab === tab) return;
    this.tab = tab;
  }

  // _selectModel + _selectRun update shared selection. Children
  // re-receive the attribute on next render; tabs that care will
  // refetch (their attributeChangedCallback handles it).
  private _selectModel(filename: string): void {
    this.selectedModel = filename;
  }
  private _selectRun(id: string): void {
    this.selectedRun = id;
  }

  // _onAskInput tracks the topbar ASK box. Submission via Enter
  // POSTs to /v1/ml-lab/ask; the RewrittenQuery's target_tab + body
  // populate the matching tab editor on success. Mock backend
  // returns deterministic mappings so the UX is shaped before a
  // real rewriter lands.
  private _onAskInput(e: Event): void {
    const t = e.target as HTMLInputElement;
    this.askPrompt = t.value;
  }
  private async _onAskSubmit(e: KeyboardEvent): Promise<void> {
    if (e.key !== "Enter" || this.askPending) return;
    const prompt = this.askPrompt.trim();
    if (!prompt) return;
    this.askPending = true;
    try {
      const res = await apiFetch("/v1/ml-lab/ask", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ prompt, active_tab: this.tab }),
      });
      if (!res.ok) return;
      const rew = await res.json() as { target_tab?: string; editor_body?: string; explanation?: string };
      if (rew.target_tab && TAB_ORDER.includes(rew.target_tab as TabID)) {
        this._selectTab(rew.target_tab as TabID);
      }
      // The editor_body + explanation are pushed to the target tab
      // via a CustomEvent on the next microtask — child mount must
      // complete first or the event lands before listeners attach.
      queueMicrotask(() => {
        this.dispatchEvent(new CustomEvent("lthn-ml-lab-ask-rewrite", {
          detail: { tab: rew.target_tab, body: rew.editor_body, explanation: rew.explanation },
          bubbles: true,
          composed: true,
        }));
      });
      this.askPrompt = "";
    } catch { /* offline — ASK silently no-ops */ }
    this.askPending = false;
  }

  private _renderTabPill(tab: TabID) {
    const active = this.tab === tab;
    return html`
      <button
        type="button"
        class="ml-lab-tab ${active ? "active" : ""}"
        style="--wails-draggable:no-drag;background:${active ? "var(--colour-surface-2)" : "transparent"};border:1px solid ${active ? "var(--colour-border-strong)" : "transparent"};color:${active ? "var(--colour-text-primary)" : "var(--colour-text-secondary)"};padding:6px 14px;border-radius:6px;font-size:13px;font-weight:500;cursor:pointer;"
        @click=${() => this._selectTab(tab)}
      >${TAB_LABEL[tab]}</button>
    `;
  }

  private _renderToolbar() {
    return html`
      <div style="display:flex;align-items:center;gap:8px;flex:1;padding:0 8px;">
        <div style="display:flex;gap:6px;">
          ${TAB_ORDER.map(t => this._renderTabPill(t))}
        </div>
        <div style="flex:1"></div>
        <input
          type="text"
          placeholder="Ask anything — Flux / SQL / LQL …"
          .value=${this.askPrompt}
          @input=${this._onAskInput}
          @keydown=${this._onAskSubmit}
          ?disabled=${this.askPending}
          style="--wails-draggable:no-drag;background:var(--colour-surface-2);border:1px solid var(--colour-border);color:var(--colour-text-primary);padding:6px 12px;border-radius:6px;font-size:13px;width:320px;outline:none;"
        />
      </div>
    `;
  }

  private _renderSidebar() {
    return html`
      <aside style="width:240px;border-right:1px solid var(--colour-border);overflow:auto;background:var(--colour-surface-1);">
        <div style="padding:12px 14px;border-bottom:1px solid var(--colour-border-subtle);">
          <div style="font-size:11px;text-transform:uppercase;letter-spacing:0.05em;color:var(--colour-text-tertiary);margin-bottom:6px;">Models (${this.models.length})</div>
          ${this.models.length === 0
            ? html`<div style="font-size:12px;color:var(--colour-text-tertiary);">no models loaded</div>`
            : this.models.slice(0, 8).map(m => html`
              <button
                type="button"
                @click=${() => this._selectModel(m.filename)}
                style="display:block;width:100%;text-align:left;background:${this.selectedModel === m.filename ? "var(--colour-surface-2)" : "transparent"};border:none;color:${this.selectedModel === m.filename ? "var(--colour-text-primary)" : "var(--colour-text-secondary)"};padding:4px 6px;border-radius:4px;font-size:12px;cursor:pointer;font-family:var(--font-mono);overflow:hidden;text-overflow:ellipsis;white-space:nowrap;"
                title=${m.filename}
              >${m.filename}</button>
            `)}
        </div>
        <div style="padding:12px 14px;border-bottom:1px solid var(--colour-border-subtle);">
          <div style="font-size:11px;text-transform:uppercase;letter-spacing:0.05em;color:var(--colour-text-tertiary);margin-bottom:6px;">Runs (${this.runs.length})</div>
          ${this.runs.length === 0
            ? html`<div style="font-size:12px;color:var(--colour-text-tertiary);">no runs known</div>`
            : this.runs.slice(0, 10).map(r => html`
              <button
                type="button"
                @click=${() => this._selectRun(r.id)}
                style="display:block;width:100%;text-align:left;background:${this.selectedRun === r.id ? "var(--colour-surface-2)" : "transparent"};border:none;color:${this.selectedRun === r.id ? "var(--colour-text-primary)" : "var(--colour-text-secondary)"};padding:4px 6px;border-radius:4px;font-size:12px;cursor:pointer;font-family:var(--font-mono);overflow:hidden;text-overflow:ellipsis;white-space:nowrap;"
                title=${`${r.id} · ${r.status}`}
              >${r.id}</button>
            `)}
        </div>
        <div style="padding:12px 14px;">
          <div style="font-size:11px;text-transform:uppercase;letter-spacing:0.05em;color:var(--colour-text-tertiary);margin-bottom:6px;">Saved (${this.savedQueries.length})</div>
          ${this.savedQueries.length === 0
            ? html`<div style="font-size:12px;color:var(--colour-text-tertiary);">no saved queries</div>`
            : this.savedQueries.map(q => html`
              <div style="font-size:12px;color:var(--colour-text-secondary);padding:4px 6px;font-family:var(--font-mono);overflow:hidden;text-overflow:ellipsis;white-space:nowrap;">
                <span style="color:var(--colour-text-tertiary);font-size:10px;text-transform:uppercase;">[${q.tab}]</span> ${q.name}
              </div>
            `)}
        </div>
      </aside>
    `;
  }

  private _renderActiveTab() {
    const tag = TAB_ELEMENT[this.tab];
    // Re-mount on tab switch — lit's keyed render keeps each tab's
    // local state separate. Attributes (selected-model, selected-run,
    // time-range-*) reach the child via attribute reflection.
    return html`
      <main style="flex:1;display:flex;flex-direction:column;min-width:0;min-height:0;overflow:hidden;">
        <div
          style="flex:1;display:flex;flex-direction:column;min-height:0;"
        >
          ${this._renderTabBody(tag)}
        </div>
      </main>
    `;
  }

  private _renderTabBody(tag: string) {
    // Use unsafeStatic? No — just html with a tag literal isn't valid.
    // Build the element via document.createElement so the tag name is
    // dynamic. Lit renders the returned Node directly when wrapped.
    const el = document.createElement(tag);
    el.setAttribute("embedded", "");
    if (this.selectedModel) el.setAttribute("selected-model", this.selectedModel);
    if (this.selectedRun)   el.setAttribute("selected-run", this.selectedRun);
    if (this.timeRangeStart) el.setAttribute("time-range-start", this.timeRangeStart);
    if (this.timeRangeStop)  el.setAttribute("time-range-stop", this.timeRangeStop);
    el.style.flex = "1";
    el.style.display = "flex";
    el.style.flexDirection = "column";
    el.style.minHeight = "0";
    return el;
  }

  private _renderStatusBar() {
    const model = this.selectedModel || "(no model)";
    const run   = this.selectedRun || "(no run)";
    return html`
      <div style="font-size:11px;color:var(--colour-text-tertiary);font-family:var(--font-mono);overflow:hidden;text-overflow:ellipsis;white-space:nowrap;">
        ${this.tab} · ${model} · ${run}
      </div>
    `;
  }

  render() {
    return renderChrome({
      title:    "ML Lab",
      subtitle: "Workbench",
      w: this.w,
      h: this.h,
      toolbar:  this._renderToolbar(),
      body: html`
        <div style="flex:1;display:flex;min-height:0;overflow:hidden;">
          ${this._renderSidebar()}
          ${this._renderActiveTab()}
        </div>
      `,
      footer: this._renderStatusBar(),
    });
  }
}

if (!customElements.get("lthn-view-ml-lab")) {
  customElements.define("lthn-view-ml-lab", LthnViewMlLab);
}

export { LthnViewMlLab };
