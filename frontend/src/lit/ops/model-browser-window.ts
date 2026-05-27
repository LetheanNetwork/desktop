// SPDX-Licence-Identifier: EUPL-1.2
// E1.3 · model browser — <lthn-model-browser-window>
// Light-DOM Lit element. Composes renderChrome() from ../chrome.js.

import { LitElement, html, nothing } from "lit";
import { renderChrome } from "../chrome";
import { T } from "@lthn/i18n/coreservice";
import type { LocalModel } from "../types";

// Wails event payload shapes for the downloader bus.
interface DlProgressPayload { id: string; name: string; written: number; total: number; }
interface DlDonePayload    { id: string; name: string; ok: boolean; dest?: string; error?: string; }

class LthnModelBrowserWindow extends LitElement {
  static readonly properties = {
    selected:        { type: String, reflect: true },
    w:               { type: Number },
    h:               { type: Number },
    embedded:        { type: Boolean, reflect: true },
    chrome:          { state: true },
    local:           { state: true },
    loadErr:         { state: true },
    modelsDir:       { state: true },
    diskFree:        { state: true },
    downloads:       { state: true },
    t:               { state: true },
    // Lemma admin state — feeds the "active loaded" indicator on rail
    // items + the Activate / profile-picker controls in the detail aside.
    activeModelPath: { state: true },
    machineHash:     { state: true },
    profiles:        { state: true },
    selectedProfile: { state: true },
    adapters:        { state: true },
    selectedAdapter: { state: true },
    activeAdapter:   { state: true },
    sftRunning:      { state: true },
    deleteBusy:      { state: true },
    deleteErr:       { state: true },
    reloadBusy:      { state: true },
    reloadErr:       { state: true },
    lemmaUnavailable:{ state: true },
  };
  declare selected: string;
  declare w: number;
  declare h: number;
  declare embedded: boolean;
  declare chrome: { title: string; subtitle: string };
  declare local: LocalModel[];
  declare loadErr: string;
  declare modelsDir: string;
  declare diskFree: number;
  // jobId → { name, written, total } for in-flight downloads.
  declare downloads: Map<string, { name: string; written: number; total: number }>;
  // Cleanup fns from Events.On so we unsub on disconnect.
  private _dlUnsubscribe: (() => void)[] = [];
  // Lemma admin facets.
  declare activeModelPath: string;
  declare machineHash: string;
  declare profiles: Array<{ name: string; path?: string; model?: string; backend?: string }>;
  declare selectedProfile: string;
  /** Adapters from Lemma.SFTAdapters — completed LoRA dirs under
   *  ~/Lethean/data/adapters. User picks one to overlay on the
   *  selected base model when activating; empty string means no
   *  overlay (base only). */
  declare adapters: Array<{ name: string; path?: string }>;
  declare selectedAdapter: string;
  /** Currently-active adapter from Lemma.Status().config.adapter_path
   *  — used to pre-select the dropdown so the dropdown reflects what
   *  the engine actually has loaded, not just what the user last
   *  picked. */
  declare activeAdapter: string;
  /** True when Lemma.SFTStatus("") reports state="running" — gates
   *  the Use button because a mid-training model swap will fail the
   *  SFT job. Refreshed alongside Status / Profiles on _refresh. */
  declare sftRunning: boolean;
  /** Busy flag while Models.Delete is in flight — prevents
   *  double-click. Cleared on success/fail. */
  declare deleteBusy: boolean;
  declare deleteErr: string;
  declare reloadBusy: boolean;
  declare reloadErr: string;
  declare lemmaUnavailable: boolean;
  declare t: {
    btnFilters: string; btnImportGguf: string;
    railLabel: string; railEmpty: string; railEmptyHint: string;
    railFailed: string; railFooter: string;
    labelSelected: string; btnOpenChat: string;
    rowFamily: string; rowParameters: string; rowQuantisation: string;
    rowContext: string; rowVocabulary: string; rowArchitecture: string;
    rowLastLoaded: string; rowAvgTps: string;
    labelFiles: string; btnDownload: string;
    footer: string;
  };
  constructor() {
    super();
    this.selected = ""; this.w = 1040; this.h = 700; this.embedded = false;
    this.chrome = { title: "Models", subtitle: "local · — · huggingface" };
    this.local = [];
    this.loadErr = "";
    this.modelsDir = "~/.lthn/models/";
    this.diskFree = 0;
    this.downloads = new Map();
    this.activeModelPath = "";
    this.machineHash = "";
    this.profiles = [];
    this.selectedProfile = "";
    this.adapters = [];
    this.selectedAdapter = "";
    this.activeAdapter = "";
    this.sftRunning = false;
    this.deleteBusy = false;
    this.deleteErr = "";
    this.reloadBusy = false;
    this.reloadErr = "";
    this.lemmaUnavailable = false;
    this.t = {
      btnFilters: "Filters", btnImportGguf: "Import GGUF…",
      railLabel: "Local",
      railEmpty: "No models found in %s.",
      railEmptyHint: "Import a GGUF or pull from Hugging Face to get started.",
      railFailed: "Failed to list models:",
      railFooter: "Right-click for pin · open in chat · delete.",
      labelSelected: "Selected", btnOpenChat: "Open in chat",
      rowFamily: "Family", rowParameters: "Parameters", rowQuantisation: "Quantisation",
      rowContext: "Context", rowVocabulary: "Vocabulary", rowArchitecture: "Architecture",
      rowLastLoaded: "Last loaded", rowAvgTps: "Average tok/s",
      labelFiles: "Files", btnDownload: "Download",
      footer: "%d local · %s free · %s · airplane-mode OK (browsing requires network)",
    };
  }
  createRenderRoot() { return this; }
  async connectedCallback() {
    super.connectedCallback();
    const [
      title, subtitleTpl,
      bf, big, rl, re, reh, rf, rfoot,
      ls, boc,
      rFam, rPar, rQua, rCtx, rVoc, rArc, rLL, rTps,
      lFiles, bDl, foot,
    ] = await Promise.all([
      T("window.models.title"),
      T("window.models.subtitle"),
      T("window.models.btn_filters"),
      T("window.models.btn_import_gguf"),
      T("window.models.rail_label"),
      T("window.models.rail_empty"),
      T("window.models.rail_empty_hint"),
      T("window.models.rail_failed"),
      T("window.models.rail_footer"),
      T("window.models.label_selected"),
      T("window.models.btn_open_chat"),
      T("window.models.row_family"),
      T("window.models.row_parameters"),
      T("window.models.row_quantisation"),
      T("window.models.row_context"),
      T("window.models.row_vocabulary"),
      T("window.models.row_architecture"),
      T("window.models.row_last_loaded"),
      T("window.models.row_avg_tps"),
      T("window.models.label_files"),
      T("window.models.btn_download"),
      T("window.models.footer"),
    ]);
    this.t = {
      btnFilters: bf, btnImportGguf: big,
      railLabel: rl, railEmpty: re, railEmptyHint: reh,
      railFailed: rf, railFooter: rfoot,
      labelSelected: ls, btnOpenChat: boc,
      rowFamily: rFam, rowParameters: rPar, rowQuantisation: rQua,
      rowContext: rCtx, rowVocabulary: rVoc, rowArchitecture: rArc,
      rowLastLoaded: rLL, rowAvgTps: rTps,
      labelFiles: lFiles, btnDownload: bDl,
      footer: foot,
    };
    // Pull real model entries from pkg/models.List(); maps each
    // Entry → LocalModel via deriveLocalModel below. Status defaults
    // to "available" since there's no per-model loaded indicator
    // surfaced from the runner yet. Bindings return core.Result post
    // the Mantis #1341 cascade — unwrap before reading .map / numeric
    // comparison or every call silently corrupts state.
    try {
      const [ms, { unwrap }] = await Promise.all([
        import("@desktop/models/wailsservice"),
        import("../result"),
      ]);
      type Entry = { name: string; size: number; path: string; is_dir: boolean };
      const [entries, free] = await Promise.all([
        unwrap<Entry[]>(ms.List(), []),
        unwrap<number>(ms.DiskFree(), 0),
      ]);
      this.local = entries.map(deriveLocalModel);
      if (this.local.length > 0 && !this.selected) {
        this.selected = this.local[0].id;
      }
      if (free > 0) this.diskFree = free;
    } catch (err: unknown) {
      this.loadErr = err instanceof Error ? err.message : String(err);
      this.local = [];
    }
    // Real models-dir path for the footer + empty-state slot. Same
    // source the welcome wizard + Settings → Models bind against;
    // collapses /Users/<name>/... to ~/ so the footer reads coherently.
    try {
      const [fl, { unwrap }] = await Promise.all([
        import("@desktop/firstlaunch/wailsservice"),
        import("../result"),
      ]);
      const paths = await unwrap<{ models_dir?: string }>(fl.Paths(), {});
      if (paths.models_dir) this.modelsDir = collapseHome(paths.models_dir);
    } catch { /* keep fallback */ }
    // Rebuild the subtitle with the live count — the locale string
    // is "local · 4 · huggingface" today; swap the "4" for the
    // real count so the chrome reflects what's actually on disk.
    this.chrome = {
      title,
      subtitle: subtitleTpl.replace(/·\s*\d+\s*·/, `· ${this.local.length} ·`)
                           .replace(/·\s*—\s*·/, `· ${this.local.length} ·`),
    };

    // Lemma admin facets — runs lazily so a missing lthn-mlx doesn't
    // block the rest of the surface. Bearer auth lives in Go-side
    // pkg/lemma.WailsService; JS never sees the admin token. When the
    // engine is down we mark lemmaUnavailable + leave activeModelPath
    // empty so the rail renders without "loaded" badges and the
    // detail-aside hides Activate.
    void this._refreshLemmaAdmin();

    // Subscribe to downloader bus events from the Wails event bridge.
    // Dynamic import so the component stays mountable in test + canvas
    // environments where the binding isn't present.
    try {
      const { Events } = await import("@wailsio/runtime");
      const offProgress = Events.On("downloader:progress", (e: { data: DlProgressPayload }) => {
        const p = e?.data;
        if (!p?.id) return;
        const next = new Map(this.downloads);
        next.set(p.id, { name: p.name, written: p.written, total: p.total });
        this.downloads = next;
        // Reflect downloading status in the local rail while in-flight.
        this.local = this.local.map(m =>
          m.name === p.name ? { ...m, status: "downloading" as const } : m,
        );
      });
      const offDone = Events.On("downloader:done", (e: { data: DlDonePayload }) => {
        const d = e?.data;
        if (!d?.id) return;
        const next = new Map(this.downloads);
        next.delete(d.id);
        this.downloads = next;
        // Re-list local models so the newly fetched file appears in the rail.
        try {
          import("@desktop/models/wailsservice").then(async ms => {
            const { unwrap } = await import("../result");
            type Entry = { name: string; size: number; path: string; is_dir: boolean };
            const entries = await unwrap<Entry[]>(ms.List(), []);
            this.local = entries.map(deriveLocalModel);
            this.chrome = {
              ...this.chrome,
              subtitle: this.chrome.subtitle
                .replace(/·\s*\d+\s*·/, `· ${this.local.length} ·`)
                .replace(/·\s*—\s*·/, `· ${this.local.length} ·`),
            };
          });
        } catch { /* non-Wails env — keep existing local list */ }
        // Re-emit on the unified models-changed channel so windows
        // listening to it (lemma-window) also refresh their pickers.
        // Without this, downloads triggered via pkg/downloader land
        // on disk but the Lemma admin lane's availableModels picker
        // stays stale until the user reopens it. Same channel
        // lemma-window's Lemma.Download path fires on.
        Events.Emit("lthn:lemma:models-changed", null);
      });
      // Cross-window broadcast: lemma-window emits this when an
      // admin-API download finishes. Re-pull the Lemma admin lane
      // (model list / active model / adapters) so the Reload picker
      // + adapter dropdown reflect the new file without the user
      // having to close + reopen the surface.
      const offLemmaModels = Events.On("lthn:lemma:models-changed", () => {
        void this._refreshLemmaAdmin();
      });
      // Sibling channel: distillation-window emits when SFT
      // completes + the new adapter dir lands on disk. Same
      // _refreshLemmaAdmin pulls SFTAdapters so the Adapter
      // dropdown picks up the new entry without manual refresh.
      const offLemmaAdapters = Events.On("lthn:lemma:adapters-changed", () => {
        void this._refreshLemmaAdmin();
      });
      this._dlUnsubscribe = [offProgress, offDone, offLemmaModels, offLemmaAdapters];
    } catch { /* non-Wails env — no event bus */ }
  }

  disconnectedCallback() {
    super.disconnectedCallback();
    for (const unsub of this._dlUnsubscribe) { try { unsub(); } catch { /* ignore */ } }
    this._dlUnsubscribe = [];
  }

  // Download trigger on Hugging Face fixture rows is intentionally
  // absent: until pkg/downloader can verify the file it receives matches
  // the digest of the file Hugging Face listed, we don't expose a path
  // that calls dl.Download / dl.DownloadVerified from this surface. See
  // Mantis #1682 Shape B (defer) and the F-2 close on #1676. Local-rail
  // entries still receive downloader bus events for any out-of-band
  // download started by other surfaces (e.g. CLI), so the progress
  // overlay in _localItem remains live.

  render() {
    const local: LocalModel[] = this.local;
    // The actually-selected local model for the detail rail's
    // "Selected" header. Falls back to the design literal so the
    // detail rail still reads coherently in canvas preview (no
    // local entries scanned, or pre-selection settles).
    const selected = local.find(m => m.id === this.selected);
    const selName   = selected?.name   || "gemma-4-e2b";
    const selFamily = selected?.family || "Google";
    const selSize   = selected?.size   || "2.1 GB";
    const selStatus = selected?.status === "loaded" ? "loaded"
                    : selected ? selected.status : "loaded";
    const results = [
      { name:"Qwen2.5-Coder-7B-Instruct",     author:"Qwen",       size:"4.8 GB", q:"q4_k_m", family:"Coder",   tools:true,  vision:false, dlCount:"1.2M", hfRepo:"Qwen/Qwen2.5-Coder-7B-Instruct-GGUF",        file:"qwen2.5-coder-7b-instruct-q4_k_m.gguf" },
      { name:"Mistral-Nemo-12B-Instruct",     author:"MistralAI",  size:"8.4 GB", q:"q4_k_m", family:"Mistral", tools:true,  vision:false, dlCount:"420k",  hfRepo:"MistralAI/Mistral-Nemo-Instruct-2407-GGUF",   file:"mistral-nemo-instruct-2407-q4_k_m.gguf" },
      { name:"Llama-3.2-11B-Vision-Instruct", author:"Meta",       size:"9.1 GB", q:"q4_k_m", family:"Llama",   tools:false, vision:true,  dlCount:"880k",  hfRepo:"bartowski/Llama-3.2-11B-Vision-Instruct-GGUF", file:"Llama-3.2-11B-Vision-Instruct-Q4_K_M.gguf" },
      { name:"Gemma-3-27B-IT",                author:"Google",     size:"16 GB",  q:"q4_k_m", family:"Gemma",   tools:false, vision:false, dlCount:"340k",  hfRepo:"bartowski/gemma-3-27b-it-GGUF",               file:"gemma-3-27b-it-Q4_K_M.gguf" },
      { name:"Phi-4-14B-Instruct",            author:"Microsoft",  size:"9.6 GB", q:"q4_k_m", family:"Phi",     tools:true,  vision:false, dlCount:"260k",  hfRepo:"microsoft/Phi-4-mini-instruct-GGUF",          file:"Phi-4-mini-instruct-q4.gguf" },
    ];

    const toolbar = html`
      <lthn-btn tone="ghost" size="sm"><i class="fa-solid fa-filter" style="font-size:10px;"></i> ${this.t.btnFilters}</lthn-btn>
      <lthn-btn tone="primary" size="sm"><i class="fa-solid fa-arrow-down" style="font-size:10px;"></i> ${this.t.btnImportGguf}</lthn-btn>
    `;

    const body = html`
      <div style="flex:1; display:grid; grid-template-columns: 240px 1fr 300px; min-height:0;">
        <!-- local rail -->
        <aside style="background:rgba(0,0,0,0.18); border-right:1px solid rgba(255,255,255,0.05);
                      display:flex; flex-direction:column; min-height:0;">
          <lthn-label style="display:block; padding:12px 14px 6px;">${this.t.railLabel} · ${local.length}</lthn-label>
          <div style="padding:0 6px; display:flex; flex-direction:column; gap:1px;">
            ${local.length === 0 ? html`
              <div style="padding:18px 14px; font-size:11.5px; color:var(--fg-3); line-height:1.55;
                          display:flex; flex-direction:column; gap:12px;">
                <div>
                  ${this.loadErr
                    ? html`<span style="color:var(--error-400);">${this.t.railFailed}</span><br>${this.loadErr}`
                    : (() => {
                        const parts = this.t.railEmpty.split("%s");
                        return html`${parts[0]}<code style="color:var(--fg-2);">${this.modelsDir}</code>${parts[1] || ""}<br>${this.t.railEmptyHint}`;
                      })()}
                </div>
                ${this.loadErr ? nothing : html`
                  <lthn-btn tone="primary" size="sm" @click=${() => this._openLemma()}>
                    <i class="fa-solid fa-cube" style="font-size:10px;"></i> Open Lemma
                  </lthn-btn>
                `}
              </div>
            ` : local.map(m => this._localItem(m))}
          </div>
          <div style="flex:1"></div>
          <div style="padding:10px 12px; border-top:1px solid rgba(255,255,255,0.05);
                      font-size:10.5px; color:var(--fg-3); line-height:1.5;">
            ${this.t.railFooter}
          </div>
        </aside>

        <!-- search results -->
        <main style="display:flex; flex-direction:column; min-height:0;">
          <div style="padding:14px 18px 10px; display:flex; flex-direction:column; gap:10px;">
            <div style="display:flex; align-items:center; gap:9px; height:32px; padding:0 12px;
                        background:rgba(255,255,255,0.04); border:1px solid rgba(255,255,255,0.07); border-radius:8px;">
              <i class="fa-solid fa-magnifying-glass" style="font-size:11px; color:var(--fg-3);"></i>
              <span style="font-size:12.5px; color:var(--fg-1);">coder · gguf · q4_k_m</span>
              <div style="flex:1"></div>
              <span style="font-family:var(--font-mono); font-size:10px; color:var(--fg-3);">${results.length} results · huggingface.co</span>
            </div>
            <div style="font-size:11px; color:var(--fg-3); line-height:1.55; padding:2px 2px 0;">
              Browsing is a preview — direct downloads from Hugging Face are
              coming once we have a way to verify the file you receive matches
              the one listed. Pin a model you'd like first and we'll let you
              know when it's ready to fetch.
            </div>
            <div style="display:flex; gap:6px; flex-wrap:wrap;">
              ${["Gemma","Llama","Phi","Qwen","Mistral","≤ 5 GB","≤ 10 GB","Has vision","Has tools"].map((f, i) => html`
                <span style="font-size:10.5px; padding:3px 9px; border-radius:999px;
                             background:${i < 2 ? "rgba(64,193,197,0.10)" : "rgba(255,255,255,0.04)"};
                             border:1px solid ${i < 2 ? "rgba(64,193,197,0.20)" : "rgba(255,255,255,0.06)"};
                             color:${i < 2 ? "var(--brand-300)" : "var(--fg-2)"};
                             letter-spacing:-0.005em;">${f}</span>
              `)}
            </div>
          </div>
          <div style="flex:1; overflow:auto; padding:4px 18px 18px; display:flex; flex-direction:column; gap:8px;">
            ${results.map((r, i) => html`
              <div style="padding:12px 14px; border-radius:8px;
                          background:${i === 0 ? "rgba(255,255,255,0.05)" : "rgba(255,255,255,0.025)"};
                          border:1px solid ${i === 0 ? "rgba(64,193,197,0.22)" : "rgba(255,255,255,0.05)"};
                          display:flex; flex-direction:column; gap:0;">
                <div style="display:flex; align-items:center; gap:14px;">
                  <div style="flex:1; min-width:0;">
                    <div style="font-family:var(--font-mono); font-size:12px; color:var(--fg-0); letter-spacing:-0.005em;">${r.name}</div>
                    <div style="font-size:10.5px; color:var(--fg-3); margin-top:3px; display:flex; gap:12px;">
                      <span>by ${r.author}</span><span>· ${r.dlCount} downloads</span>
                    </div>
                    <div style="display:flex; gap:4px; margin-top:6px;">
                      ${[r.family, r.q, r.tools && "tools", r.vision && "vision"].filter(Boolean).map(b => html`
                        <span style="font-family:var(--font-mono); font-size:9.5px; padding:1px 6px; border-radius:999px;
                                     background:rgba(255,255,255,0.05); border:1px solid rgba(255,255,255,0.07);
                                     color:var(--fg-2); letter-spacing:0.02em;">${b}</span>
                      `)}
                    </div>
                  </div>
                  <div style="text-align:right; font-family:var(--font-mono); font-size:11.5px; color:var(--fg-1);">${r.size}</div>
                  <lthn-btn
                    tone="ghost"
                    size="sm"
                    disabled
                    title="Direct download is coming once we can verify the file matches the one listed on Hugging Face."
                    style="--wails-draggable:no-drag;">
                    <i class="fa-solid fa-clock" style="font-size:10px;"></i>
                    Coming soon
                  </lthn-btn>
                </div>
              </div>`)}
          </div>
        </main>

        <!-- detail -->
        <aside style="background:rgba(0,0,0,0.18); border-left:1px solid rgba(255,255,255,0.05);
                      padding:18px; overflow:auto; display:flex; flex-direction:column; gap:14px;">
          <div>
            <lthn-label>${this.t.labelSelected}</lthn-label>
            <div style="font-family:var(--font-mono); font-size:13px; color:var(--fg-0); margin-top:6px; letter-spacing:-0.005em; word-break:break-all;">${selName}</div>
            <div style="font-size:11px; color:var(--fg-3); margin-top:3px;">by ${selFamily} · ${selStatus} · ${selSize} on disk</div>
          </div>
          <div style="display:flex; gap:6px;">
            <lthn-btn tone="primary" size="md" style="flex:1; justify-content:center;">
              <i class="fa-regular fa-comment"></i> ${this.t.btnOpenChat}
            </lthn-btn>
            <lthn-btn tone="ghost" size="md"><i class="fa-solid fa-thumbtack" style="font-size:10px;"></i></lthn-btn>
          </div>
          ${this._renderActivate(selected)}
          <div style="display:flex; flex-direction:column; gap:8px; font-size:11.5px;">
            <lthn-rail-row k=${this.t.rowFamily}       v=${selected ? selFamily : "Gemma 4"}></lthn-rail-row>
            <lthn-rail-row k=${this.t.rowParameters}   v=${selected ? modelParams(selected.name) : "2 B"}></lthn-rail-row>
            <lthn-rail-row k=${this.t.rowQuantisation} v=${selected ? modelQuant(selected.name) : "q4_k_m"}></lthn-rail-row>
            <lthn-rail-row k=${this.t.rowContext}      v="8,192"></lthn-rail-row>
            <lthn-rail-row k=${this.t.rowVocabulary}   v="262,144"></lthn-rail-row>
            <lthn-rail-row k=${this.t.rowArchitecture} v="MoE · 4-expert"></lthn-rail-row>
            <lthn-rail-row k=${this.t.rowLastLoaded}   v="2 minutes ago"></lthn-rail-row>
            <lthn-rail-row k=${this.t.rowAvgTps}       v="47.2 · last 100 runs"></lthn-rail-row>
          </div>
          <div style="padding:10px; border-radius:6px;
                      background:rgba(255,255,255,0.03); border:1px solid rgba(255,255,255,0.06);">
            <lthn-label>${this.t.labelFiles}</lthn-label>
            <div style="margin-top:6px; display:flex; flex-direction:column; gap:4px;
                        font-family:var(--font-mono); font-size:10.5px; color:var(--fg-2);">
              ${selected?.isDir === false ? html`
                <div style="display:flex; justify-content:space-between;"><span>${selected.name}</span><span style="color:var(--fg-3);">${selected.size}</span></div>
              ` : html`
                <div style="display:flex; justify-content:space-between;"><span>gemma-4-e2b-q4_k_m.gguf</span><span style="color:var(--fg-3);">1.9 GB</span></div>
                <div style="display:flex; justify-content:space-between;"><span>tokenizer.json</span><span style="color:var(--fg-3);">4.2 MB</span></div>
                <div style="display:flex; justify-content:space-between;"><span>config.json</span><span style="color:var(--fg-3);">1.1 KB</span></div>
              `}
            </div>
          </div>
          <div style="font-size:11px; color:var(--fg-3); line-height:1.55;">
            Small dense model tuned for assistant-style turns. Lethean-recommended
            starter — fastest tok/s per watt on Apple Silicon at this size.
          </div>
        </aside>
      </div>
    `;

    return renderChrome({
      title: this.chrome.title,
      subtitle: this.chrome.subtitle,
      w: this.w, h: this.h,
      toolbar, body,
      footer: html`${this.t.footer
        .replace("%d", String(local.length || 4))
        .replace("%s", this.diskFree > 0 ? fmtBytes(this.diskFree) : "312 GB")
        .replace("%s", this.modelsDir)}`,
      embedded: this.embedded,
    });
  }

  // Render the Activate panel — profile picker + "Use this model"
  // button + inline error. Hidden when the engine is unreachable
  // (lthn-mlx not running) OR the selected entry is a directory
  // skeleton with no path. The match against activeModelPath flips
  // the button copy to "Loaded" + disables so the operator doesn't
  // pay the reload cost for a no-op swap.
  private _renderActivate(selected: LocalModel | undefined) {
    if (this.lemmaUnavailable) {
      return html`
        <div style="padding:8px 10px; border-radius:6px;
                    background:rgba(255,255,255,0.02); border:1px solid rgba(255,255,255,0.05);
                    font-size:10.5px; color:var(--fg-3); line-height:1.5;">
          Lemma engine not reachable — start <code style="font-family:var(--font-mono);">lthn serve</code>
          to enable model swapping from this pane.
        </div>`;
    }
    if (!selected || !selected.path || selected.isDir) {
      return nothing;
    }
    const isLoaded = selected.path === this.activeModelPath;
    const btnLabel = this.reloadBusy   ? "Reloading…"
                   : this.sftRunning   ? "Training in progress"
                   : isLoaded          ? "Loaded"
                   :                     "Use this model";
    return html`
      <div style="display:flex; flex-direction:column; gap:8px; padding:10px;
                  border-radius:6px; background:rgba(255,255,255,0.03);
                  border:1px solid rgba(255,255,255,0.06);">
        ${this.profiles.length > 0 ? html`
          <div>
            <lthn-label>Profile</lthn-label>
            <select
              .value=${this.selectedProfile}
              @change=${(e: Event) => { this.selectedProfile = (e.target as HTMLSelectElement).value; }}
              style="width:100%; margin-top:4px; padding:5px 7px; font-size:11.5px;
                     background:rgba(0,0,0,0.25); color:var(--fg-1);
                     border:1px solid rgba(255,255,255,0.08); border-radius:4px;
                     --wails-draggable:no-drag;">
              <option value="">(use serve default)</option>
              ${this.profiles.map(p => html`<option value=${p.path ?? p.name}>${p.name}${p.backend ? ` — ${p.backend}` : ""}</option>`)}
            </select>
          </div>
        ` : nothing}
        ${this.adapters.length > 0 ? html`
          <div>
            <lthn-label>Adapter</lthn-label>
            <select
              .value=${this.selectedAdapter}
              @change=${(e: Event) => { this.selectedAdapter = (e.target as HTMLSelectElement).value; }}
              style="width:100%; margin-top:4px; padding:5px 7px; font-size:11.5px;
                     background:rgba(0,0,0,0.25); color:var(--fg-1);
                     border:1px solid rgba(255,255,255,0.08); border-radius:4px;
                     --wails-draggable:no-drag;">
              <option value="">(base model only)</option>
              ${this.adapters.map(a => {
                const isActive = a.path === this.activeAdapter;
                return html`<option value=${a.path ?? a.name}>${a.name}${isActive ? "  (loaded)" : ""}</option>`;
              })}
            </select>
          </div>
        ` : nothing}
        <lthn-btn
          tone=${isLoaded || this.sftRunning ? "ghost" : "primary"}
          size="md"
          ?disabled=${this.reloadBusy || isLoaded || this.sftRunning}
          title=${this.sftRunning ? "Cannot swap model while fine-tune is in flight — Stop it first from the Lemma engine panel" : ""}
          @click=${() => { void this._doReload(selected.path!); }}
          style="justify-content:center; --wails-draggable:no-drag;">
          <i class="fa-solid ${this.sftRunning ? "fa-hourglass-half" : isLoaded ? "fa-check" : "fa-arrow-right-arrow-left"}" style="font-size:10px;"></i>
          ${btnLabel}
        </lthn-btn>
        ${this.reloadErr ? html`
          <div style="font-size:10.5px; color:var(--error-400); line-height:1.45;">${this.reloadErr}</div>
        ` : nothing}
        ${selected.name
          ? html`
            <lthn-btn
              tone="ghost"
              size="sm"
              ?disabled=${this.deleteBusy || isLoaded || this.sftRunning}
              title=${isLoaded
                ? "Cannot delete the currently-loaded model — swap to a different one first"
                : this.sftRunning
                  ? "Cannot delete a model while fine-tune is in flight"
                  : `Delete ${selected.name} from disk${selected.size ? ` — frees ${selected.size}` : ""}`}
              @click=${() => { void this._doDelete(selected.name); }}
              style="justify-content:center; --wails-draggable:no-drag; color:var(--error-400);">
              <i class="fa-regular fa-trash-can" style="font-size:10px;"></i>
              ${this.deleteBusy ? "Deleting…" : "Delete from disk"}
            </lthn-btn>
            ${this.deleteErr ? html`
              <div style="font-size:10.5px; color:var(--error-400); line-height:1.45;">${this.deleteErr}</div>
            ` : nothing}
          `
          : nothing}
      </div>
    `;
  }

  // Pull Status / Machine / Profiles in parallel. Status.model_path
  // drives the "active loaded" indicator on rail rows; Machine.hash
  // becomes the confirm_machine gate value for Reload; Profiles
  // populates the picker in the detail aside. Any failure marks
  // lemmaUnavailable rather than throwing — the rest of the surface
  // stays usable when lthn-mlx isn't running.
  private async _refreshLemmaAdmin(): Promise<void> {
    try {
      const Lemma = await import("@desktop/lemma/wailsservice");
      const [statusRes, machineRes, profilesRes, adaptersRes, sftRes] = await Promise.allSettled([
        Lemma.Status(),
        Lemma.Machine(),
        Lemma.Profiles(),
        Lemma.SFTAdapters(),
        // SFTStatus("") gates the Use button — single-flight upstream
        // means a mid-training model swap will fail the job, so the
        // UI disables the action when state === "running" rather than
        // letting the user submit a doomed reload.
        Lemma.SFTStatus(""),
      ]);
      if (statusRes.status === "fulfilled") {
        this.activeModelPath = statusRes.value?.model_path ?? "";
        this.activeAdapter = statusRes.value?.config?.adapter_path ?? "";
        // Pre-select the active adapter so the dropdown defaults to
        // "what's loaded right now" rather than empty — the user can
        // still pick a different one or revert to "(none)".
        if (this.activeAdapter && !this.selectedAdapter) {
          this.selectedAdapter = this.activeAdapter;
        }
        this.lemmaUnavailable = false;
      } else {
        this.lemmaUnavailable = true;
        this.activeModelPath = "";
      }
      if (machineRes.status === "fulfilled") {
        this.machineHash = machineRes.value?.hash ?? "";
      }
      if (profilesRes.status === "fulfilled") {
        this.profiles = profilesRes.value?.profiles ?? [];
      }
      if (adaptersRes.status === "fulfilled") {
        this.adapters = adaptersRes.value?.adapters ?? [];
      }
      this.sftRunning = sftRes.status === "fulfilled" && sftRes.value?.state === "running";
    } catch {
      this.lemmaUnavailable = true;
    }
  }

  // Hot-swap the loaded model. Caller passes the absolute path of the
  // local-rail entry. ConfirmMachine is the gate the engine checks
  // (rejects the call if the running instance hash doesn't match —
  // operator foot-gun prevention). After success, refresh Status so
  // the active-row indicator flips to the new selection.
  private async _doReload(modelPath: string): Promise<void> {
    if (this.reloadBusy) return;
    this.reloadErr = "";
    if (!this.machineHash) {
      this.reloadErr = "machine hash not loaded — engine may be offline";
      return;
    }
    if (!modelPath) {
      this.reloadErr = "no model path";
      return;
    }
    this.reloadBusy = true;
    try {
      const Lemma = await import("@desktop/lemma/wailsservice");
      const { ReloadRequest } = await import("@desktop/lemma/models");
      await Lemma.Reload(new ReloadRequest({
        confirm_machine: this.machineHash,
        model_path:      modelPath,
        profile_path:    this.selectedProfile || undefined,
        adapter_path:    this.selectedAdapter || undefined,
        context_length:  0,
      }));
      await this._refreshLemmaAdmin();
      // Broadcast so peer windows refresh their adminStatus mirror
      // — lemma-window's Model path / Adapter / Profile rows reflect
      // the new state without waiting for the next manual refresh.
      // Tray + telemetry already poll every 2s so they pick up
      // independently; benchmark/fleet don't track model_path so
      // they're no-ops on this channel.
      try {
        const { Events } = await import("@wailsio/runtime");
        Events.Emit("lthn:lemma:model-reloaded", null);
      } catch { /* wails runtime absent in test contexts */ }
    } catch (err) {
      this.reloadErr = err instanceof Error ? err.message : String(err);
    } finally {
      this.reloadBusy = false;
    }
  }

  /** Confirm + Models.Delete + refresh. Native confirm() suffices —
   *  the action is destructive but reversible (user can re-download).
   *  Path-traversal guards live Go-side in models.Delete, so the
   *  name string we pass is safe even if a row's name was somehow
   *  injection-shaped. After delete, re-pull local rail + broadcast
   *  models-changed so peer windows refresh too. */
  private async _doDelete(name: string): Promise<void> {
    if (this.deleteBusy) return;
    this.deleteErr = "";
    if (!confirm(`Delete "${name}" from disk?\n\nThe file is removed locally. You can re-download from the source repo if you change your mind.`)) {
      return;
    }
    this.deleteBusy = true;
    try {
      const Models = await import("@desktop/models/wailsservice");
      const { demand } = await import("../result");
      await demand<unknown>(Models.Delete(name));
      // Re-pull local rail so the deleted row disappears immediately.
      const { unwrap } = await import("../result");
      type Entry = { name: string; size: number; path: string; is_dir: boolean };
      const entries = await unwrap<Entry[]>(Models.List(), []);
      this.local = entries.map(deriveLocalModel);
      // Clear the selection if it pointed at the deleted row.
      if (this.selected && !this.local.some(m => m.id === this.selected)) {
        this.selected = "";
      }
      // Broadcast so peer windows' availableModels pickers refresh.
      try {
        const { Events } = await import("@wailsio/runtime");
        Events.Emit("lthn:lemma:models-changed", null);
      } catch { /* wails runtime absent in test contexts */ }
    } catch (err) {
      this.deleteErr = err instanceof Error ? err.message : String(err);
    } finally {
      this.deleteBusy = false;
    }
  }

  /** Switch the app shell to the Lemma pane + expand the admin
   *  panel so the user lands on the Download form directly. Two
   *  events chained: setpane (same channel tray popover + logs
   *  Open Chat use) and open-admin (lemma-window deep-link). */
  private async _openLemma(): Promise<void> {
    try {
      const { Events } = await import("@wailsio/runtime");
      Events.Emit("lthn:app:setpane", "lemma");
      Events.Emit("lthn:lemma:open-admin", null);
    } catch (err) {
      console.error("model-browser: open lemma failed", err);
    }
  }

  _localItem(m: LocalModel) {
    const active = m.id === this.selected;
    // Real "loaded" state derives from Lemma.Status().model_path —
    // the LocalModel.status field still defaults to "available" out
    // of pkg/models.List() (no runner cross-check at scan time).
    // Match on the engine's path to flip the indicator dot live.
    const isLoaded = this.activeModelPath !== "" && m.path === this.activeModelPath;
    const tone = isLoaded ? "var(--success-400)"
               : m.status === "downloading" ? "var(--warning-400)"
               : "var(--fg-3)";
    return html`
      <div
        @click=${() => { this.selected = m.id; }}
        style="padding:9px 10px; border-radius:6px;
                  background:${active ? "rgba(255,255,255,0.07)" : "transparent"};
                  border-left:${active ? "2px solid var(--brand-400)" : "2px solid transparent"};
                  display:flex; flex-direction:column; gap:3px; cursor:pointer;
                  --wails-draggable: no-drag;">
        <div style="display:flex; align-items:center; gap:8px;">
          <span style="width:6px; height:6px; border-radius:50%; background:${tone};
                       box-shadow:${isLoaded ? `0 0 4px ${tone}` : "none"};"></span>
          <span style="font-family:var(--font-mono); font-size:11.5px; color:var(--fg-0); letter-spacing:-0.005em;">${m.name}</span>
        </div>
        <div style="display:flex; justify-content:space-between; font-size:10px; color:var(--fg-3);">
          <span>${m.family}</span>
          <span style="font-family:var(--font-mono);">${m.size}</span>
        </div>
        ${m.status === "downloading" ? (() => {
          const dl = [...this.downloads.values()].find(d => d.name === m.name);
          const pct = dl && dl.total > 0 ? Math.round(100 * dl.written / dl.total) : 0;
          return html`
          <div style="height:2px; background:rgba(255,255,255,0.06); border-radius:1px; margin-top:4px; overflow:hidden;">
            <div style="width:${pct}%; height:100%; background:var(--warning-400); transition:width 250ms linear;"></div>
          </div>`;
        })() : nothing}
      </div>
    `;
  }
}
customElements.define("lthn-model-browser-window", LthnModelBrowserWindow);

// ─── helpers ────────────────────────────────────────────────────────

/** Common LLM family parsed from the file/dir name. Best-effort —
 *  matches the prefix conventions Hugging Face / lthn use. Falls
 *  back to "Local" so the rail entry always renders something. */
function modelFamily(name: string): string {
  const n = name.toLowerCase();
  if (n.startsWith("gemma"))   return "Gemma";
  if (n.startsWith("llama"))   return "Llama";
  if (n.startsWith("phi"))     return "Phi";
  if (n.startsWith("qwen"))    return "Qwen";
  if (n.startsWith("mistral")) return "Mistral";
  if (n.startsWith("nemo"))    return "Mistral";
  if (n.startsWith("deepseek"))return "DeepSeek";
  if (n.startsWith("yi"))      return "Yi";
  if (n.startsWith("falcon"))  return "Falcon";
  if (n.startsWith("mixtral")) return "Mistral";
  if (n.startsWith("granite")) return "Granite";
  return "Local";
}

/** Pull a parameter-count tag from the filename — best-effort regex.
 *  Matches patterns like "7B", "2b", "0.5B", "70b-instruct". Returns
 *  "—" when no match so the detail-rail slot reads honestly. */
function modelParams(name: string): string {
  const m = name.match(/(\d+(?:\.\d+)?)\s*[Bb](?:[-._]|$)/);
  return m ? `${m[1]} B` : "—";
}

/** Pull a quantisation tag from the filename — q4_k_m, Q4_K_M, q8_0,
 *  fp16, bf16, etc. Returns "—" when nothing matches. */
function modelQuant(name: string): string {
  const n = name.toLowerCase();
  const m = n.match(/(q\d+_[a-z0-9_]+|q\d+_\d+|fp16|fp32|bf16|int8|int4|nf4)/);
  return m ? m[1] : "—";
}

/** Slug for the LocalModel.id field — lowercase, kebab-case. Falls
 *  back to "model" when the source name is empty. */
function modelSlug(name: string): string {
  const s = name.toLowerCase().replaceAll(/[^a-z0-9]+/g, "-").replaceAll(/^-+|-+$/g, "");
  return s || "model";
}

/** Collapse the leading $HOME into "~/" so the footer + empty-state
 *  hint match the terminal short-form a user would type. No browser
 *  API for $HOME — matches common macOS / Linux user dirs. */
function collapseHome(absPath: string): string {
  if (!absPath) return absPath;
  const m = absPath.match(/^\/(Users|home)\/[^/]+\//);
  return m ? "~/" + absPath.slice(m[0].length) : absPath;
}

/** Human-readable size from byte count. Matches the units the rail
 *  already uses; precision tracks magnitude so a 2.1 GB model
 *  doesn't render as 2.1234 GB. */
function fmtBytes(bytes: number): string {
  if (bytes === 0) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  let i = 0;
  let v = bytes;
  while (v >= 1024 && i < units.length - 1) { v /= 1024; i += 1; }
  return v >= 100 || i === 0 ? `${v.toFixed(0)} ${units[i]}` : `${v.toFixed(1)} ${units[i]}`;
}

/** Maps a pkg/models Entry → LocalModel. Status default is
 *  "available" — no runtime "loaded" signal yet (would need a
 *  runner cross-check). */
function deriveLocalModel(e: { name: string; size: number; path: string; is_dir: boolean }): LocalModel {
  return {
    id:     modelSlug(e.name),
    name:   e.name,
    family: modelFamily(e.name),
    size:   fmtBytes(e.size || 0),
    status: "available",
    path:   e.path,
    isDir:  e.is_dir,
  };
}
