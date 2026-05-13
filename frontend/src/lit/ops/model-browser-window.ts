// SPDX-Licence-Identifier: EUPL-1.2
// E1.3 · model browser — <lthn-model-browser-window>
// Light-DOM Lit element. Composes renderChrome() from ../chrome.js.

import { LitElement, html, nothing } from "lit";
import { renderChrome } from "../chrome";
import { T } from "@lthn/i18n/coreservice";
import type { LocalModel } from "../types";

class LthnModelBrowserWindow extends LitElement {
  static properties = {
    selected: { type: String, reflect: true },
    w:        { type: Number },
    h:        { type: Number },
    embedded: { type: Boolean, reflect: true },
    chrome:   { state: true },
    local:    { state: true },
    loadErr:  { state: true },
  };
  declare selected: string;
  declare w: number;
  declare h: number;
  declare embedded: boolean;
  declare chrome: { title: string; subtitle: string };
  declare local: LocalModel[];
  declare loadErr: string;
  constructor() {
    super();
    this.selected = ""; this.w = 1040; this.h = 700; this.embedded = false;
    this.chrome = { title: "Models", subtitle: "local · — · huggingface" };
    this.local = [];
    this.loadErr = "";
  }
  createRenderRoot() { return this; }
  async connectedCallback() {
    super.connectedCallback();
    const [title, subtitleTpl] = await Promise.all([
      T("window.models.title"),
      T("window.models.subtitle"),
    ]);
    // Pull real model entries from pkg/models.List(); maps each
    // Entry → LocalModel via deriveLocalModel below. Status defaults
    // to "available" since there's no per-model loaded indicator
    // surfaced from the runner yet.
    try {
      const ms = await import("@desktop/models/wailsservice");
      const entries = await ms.List();
      this.local = (entries || []).map(deriveLocalModel);
      if (this.local.length > 0 && !this.selected) {
        this.selected = this.local[0].id;
      }
    } catch (err: unknown) {
      this.loadErr = err instanceof Error ? err.message : String(err);
      this.local = [];
    }
    // Rebuild the subtitle with the live count — the locale string
    // is "local · 4 · huggingface" today; swap the "4" for the
    // real count so the chrome reflects what's actually on disk.
    this.chrome = {
      title,
      subtitle: subtitleTpl.replace(/·\s*\d+\s*·/, `· ${this.local.length} ·`)
                           .replace(/·\s*—\s*·/, `· ${this.local.length} ·`),
    };
  }

  render() {
    const local: LocalModel[] = this.local;
    // The selected model in the detail rail. Falls back to the first
    // local entry when nothing is explicitly picked.
    const selected: LocalModel | null =
      this.local.find(m => m.id === this.selected) || this.local[0] || null;

    // Toolbar intentionally bare today — Filters + Import GGUF land
    // with the discovery service. Leaving placeholder buttons here
    // would invite clicks on dead surface.
    const toolbar = nothing;

    const body = html`
      <div style="flex:1; display:grid; grid-template-columns: 240px 1fr 300px; min-height:0;">
        <!-- local rail -->
        <aside style="background:rgba(0,0,0,0.18); border-right:1px solid rgba(255,255,255,0.05);
                      display:flex; flex-direction:column; min-height:0;">
          <lthn-label style="display:block; padding:12px 14px 6px;">Local · ${local.length}</lthn-label>
          <div style="padding:0 6px; display:flex; flex-direction:column; gap:1px;">
            ${local.length === 0 ? html`
              <div style="padding:18px 14px; font-size:11.5px; color:var(--fg-3); line-height:1.55;">
                ${this.loadErr
                  ? html`<span style="color:var(--error-400);">Failed to list models:</span><br>${this.loadErr}`
                  : html`No models found in <code style="color:var(--fg-2);">~/Lethean/conf/models/</code>.<br>Import a GGUF or pull from Hugging Face to get started.`}
              </div>
            ` : local.map(m => this._localItem(m))}
          </div>
          <div style="flex:1"></div>
          <div style="padding:10px 12px; border-top:1px solid rgba(255,255,255,0.05);
                      font-size:10.5px; color:var(--fg-3); line-height:1.5;">
            Right-click for pin · open in chat · delete.
          </div>
        </aside>

        <!-- search results -->
        <main style="display:flex; flex-direction:column; min-height:0;">
          <div style="padding:14px 18px 10px; display:flex; flex-direction:column; gap:10px;">
            <div style="display:flex; align-items:center; gap:9px; height:32px; padding:0 12px;
                        background:rgba(255,255,255,0.04); border:1px solid rgba(255,255,255,0.07); border-radius:8px;">
              <i class="fa-solid fa-magnifying-glass" style="font-size:11px; color:var(--fg-3);"></i>
              <span style="font-size:12.5px; color:var(--fg-3); font-style:italic;">Hugging Face search · not wired yet</span>
            </div>
          </div>
          <div style="flex:1; overflow:auto; padding:4px 18px 18px;
                      display:flex; flex-direction:column; gap:14px; align-items:center; justify-content:center;">
            <div style="max-width:380px; text-align:center; padding:24px;
                        background:rgba(255,255,255,0.025); border:1px solid rgba(255,255,255,0.06);
                        border-radius:10px;">
              <div style="font-size:13.5px; color:var(--fg-1); font-weight:500; letter-spacing:-0.005em;">
                Discovery is offline today.
              </div>
              <div style="font-size:11.5px; color:var(--fg-3); margin-top:8px; line-height:1.55;">
                Drop a <code style="color:var(--fg-2);">.gguf</code> or model folder into
                <br><code style="color:var(--fg-2); font-size:11px;">~/Lethean/conf/models/</code>
                <br>and lthn picks it up on the next launch. Live Hugging Face
                search + in-app download lands when the discovery service ships.
              </div>
            </div>
          </div>
        </main>

        <!-- detail -->
        <aside style="background:rgba(0,0,0,0.18); border-left:1px solid rgba(255,255,255,0.05);
                      padding:18px; overflow:auto; display:flex; flex-direction:column; gap:14px;">
          ${selected ? html`
            <div>
              <lthn-label>Selected</lthn-label>
              <div style="font-family:var(--font-mono); font-size:13px; color:var(--fg-0);
                          margin-top:6px; letter-spacing:-0.005em; word-break:break-all;">
                ${selected.name}
              </div>
              <div style="font-size:11px; color:var(--fg-3); margin-top:3px;">
                ${selected.family || "unknown family"} · ${selected.size} on disk
              </div>
            </div>
            <div style="display:flex; flex-direction:column; gap:8px; font-size:11.5px;">
              <lthn-rail-row k="Family"        v=${selected.family || "—"}></lthn-rail-row>
              <lthn-rail-row k="Size"          v=${selected.size}></lthn-rail-row>
              <lthn-rail-row k="Kind"          v=${selected.isDir ? "folder" : "file"}></lthn-rail-row>
              <lthn-rail-row k="Status"        v=${selected.status}></lthn-rail-row>
            </div>
            ${selected.path ? html`
              <div style="padding:10px; border-radius:6px;
                          background:rgba(255,255,255,0.03); border:1px solid rgba(255,255,255,0.06);">
                <lthn-label>Path</lthn-label>
                <div style="margin-top:6px; font-family:var(--font-mono); font-size:10.5px;
                            color:var(--fg-2); word-break:break-all; line-height:1.5;">
                  ${selected.path}
                </div>
              </div>
            ` : nothing}
            <div style="font-size:11px; color:var(--fg-3); line-height:1.55;">
              Header parsing (parameters, quantisation, context, architecture)
              lands when pkg/modelmeta wires the GGUF / safetensors readers.
            </div>
          ` : html`
            <div style="font-size:12px; color:var(--fg-3); padding:24px 0; line-height:1.55;">
              Pick a local model to inspect its on-disk details.
            </div>
          `}
        </aside>
      </div>
    `;

    return renderChrome({
      title: this.chrome.title,
      subtitle: this.chrome.subtitle,
      w: this.w, h: this.h,
      toolbar, body,
      footer: html`${local.length} local · ~/Lethean/conf/models/ · airplane-mode OK (browsing requires network)`,
      embedded: this.embedded,
    });
  }

  _localItem(m: LocalModel) {
    const active = m.id === this.selected;
    const tone = m.status === "loaded" ? "var(--success-400)"
               : m.status === "downloading" ? "var(--warning-400)"
               : "var(--fg-3)";
    return html`
      <div style="padding:9px 10px; border-radius:6px;
                  background:${active ? "rgba(255,255,255,0.07)" : "transparent"};
                  border-left:${active ? "2px solid var(--brand-400)" : "2px solid transparent"};
                  display:flex; flex-direction:column; gap:3px; cursor:pointer;">
        <div style="display:flex; align-items:center; gap:8px;">
          <span style="width:6px; height:6px; border-radius:50%; background:${tone};
                       box-shadow:${m.status === "loaded" ? `0 0 4px ${tone}` : "none"};"></span>
          <span style="font-family:var(--font-mono); font-size:11.5px; color:var(--fg-0); letter-spacing:-0.005em;">${m.name}</span>
        </div>
        <div style="display:flex; justify-content:space-between; font-size:10px; color:var(--fg-3);">
          <span>${m.family}</span>
          <span style="font-family:var(--font-mono);">${m.size}</span>
        </div>
        ${m.status === "downloading" ? html`
          <div style="height:2px; background:rgba(255,255,255,0.06); border-radius:1px; margin-top:4px; overflow:hidden;">
            <div style="width:62%; height:100%; background:var(--warning-400);"></div>
          </div>
        ` : nothing}
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

/** Slug for the LocalModel.id field — lowercase, kebab-case. Falls
 *  back to "model" when the source name is empty. */
function modelSlug(name: string): string {
  const s = name.toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-+|-+$/g, "");
  return s || "model";
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
