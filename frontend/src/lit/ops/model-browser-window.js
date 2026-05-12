// SPDX-Licence-Identifier: EUPL-1.2
// E1.3 · model browser — <lthn-model-browser-window>
// Light-DOM Lit element. Composes renderChrome() from ../chrome.js.

import { LitElement, html, nothing } from "lit";
import { renderChrome } from "../chrome.js";

class LthnModelBrowserWindow extends LitElement {
  static properties = {
    selected: { type: String, reflect: true },
    w:        { type: Number },
    h:        { type: Number },
  };
  constructor() { super(); this.selected = "gemma-4-e2b"; this.w = 1040; this.h = 700; }
  createRenderRoot() { return this; }

  render() {
    const local = [
      { id:"gemma-4-e2b",  name:"gemma-4-e2b",  family:"Gemma", size:"2.1 GB", status:"loaded" },
      { id:"llama-3.2-3b", name:"llama-3.2-3b", family:"Llama", size:"3.4 GB", status:"available" },
      { id:"phi-3.5-mini", name:"phi-3.5-mini", family:"Phi",   size:"2.6 GB", status:"available" },
      { id:"qwen-2.5-7b",  name:"qwen-2.5-7b",  family:"Qwen",  size:"4.8 GB", status:"downloading" },
    ];
    const results = [
      { name:"Qwen2.5-Coder-7B-Instruct",     author:"Qwen",       size:"4.8 GB", q:"q4_k_m", family:"Coder",   tools:true,  vision:false, downloads:"1.2M" },
      { name:"Mistral-Nemo-12B-Instruct",     author:"MistralAI",  size:"8.4 GB", q:"q4_k_m", family:"Mistral", tools:true,  vision:false, downloads:"420k" },
      { name:"Llama-3.2-11B-Vision-Instruct", author:"Meta",       size:"9.1 GB", q:"q4_k_m", family:"Llama",   tools:false, vision:true,  downloads:"880k" },
      { name:"Gemma-3-27B-IT",                author:"Google",     size:"16 GB",  q:"q4_k_m", family:"Gemma",   tools:false, vision:false, downloads:"340k" },
      { name:"Phi-4-14B-Instruct",            author:"Microsoft",  size:"9.6 GB", q:"q4_k_m", family:"Phi",     tools:true,  vision:false, downloads:"260k" },
    ];

    const toolbar = html`
      <lthn-btn tone="ghost" size="sm"><i class="fa-solid fa-filter" style="font-size:10px;"></i> Filters</lthn-btn>
      <lthn-btn tone="primary" size="sm"><i class="fa-solid fa-arrow-down" style="font-size:10px;"></i> Import GGUF…</lthn-btn>
    `;

    const body = html`
      <div style="flex:1; display:grid; grid-template-columns: 240px 1fr 300px; min-height:0;">
        <!-- local rail -->
        <aside style="background:rgba(0,0,0,0.18); border-right:1px solid rgba(255,255,255,0.05);
                      display:flex; flex-direction:column; min-height:0;">
          <lthn-label style="display:block; padding:12px 14px 6px;">Local · 4</lthn-label>
          <div style="padding:0 6px; display:flex; flex-direction:column; gap:1px;">
            ${local.map(m => this._localItem(m))}
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
              <span style="font-size:12.5px; color:var(--fg-1);">coder · gguf · q4_k_m</span>
              <div style="flex:1"></div>
              <span style="font-family:var(--font-mono); font-size:10px; color:var(--fg-3);">5 results · huggingface.co</span>
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
                          display:flex; align-items:center; gap:14px;">
                <div style="flex:1; min-width:0;">
                  <div style="font-family:var(--font-mono); font-size:12px; color:var(--fg-0); letter-spacing:-0.005em;">${r.name}</div>
                  <div style="font-size:10.5px; color:var(--fg-3); margin-top:3px; display:flex; gap:12px;">
                    <span>by ${r.author}</span><span>· ${r.downloads} downloads</span>
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
                <lthn-btn tone=${i === 0 ? "primary" : "ghost"} size="sm">
                  <i class="fa-solid fa-arrow-down" style="font-size:10px;"></i> Download
                </lthn-btn>
              </div>
            `)}
          </div>
        </main>

        <!-- detail -->
        <aside style="background:rgba(0,0,0,0.18); border-left:1px solid rgba(255,255,255,0.05);
                      padding:18px; overflow:auto; display:flex; flex-direction:column; gap:14px;">
          <div>
            <lthn-label>Selected</lthn-label>
            <div style="font-family:var(--font-mono); font-size:13px; color:var(--fg-0); margin-top:6px; letter-spacing:-0.005em;">gemma-4-e2b</div>
            <div style="font-size:11px; color:var(--fg-3); margin-top:3px;">by Google · loaded · 2.1 GB on disk</div>
          </div>
          <div style="display:flex; gap:6px;">
            <lthn-btn tone="primary" size="md" style="flex:1; justify-content:center;">
              <i class="fa-regular fa-comment"></i> Open in chat
            </lthn-btn>
            <lthn-btn tone="ghost" size="md"><i class="fa-solid fa-thumbtack" style="font-size:10px;"></i></lthn-btn>
          </div>
          <div style="display:flex; flex-direction:column; gap:8px; font-size:11.5px;">
            <lthn-rail-row k="Family"        v="Gemma 4"></lthn-rail-row>
            <lthn-rail-row k="Parameters"    v="2 B"></lthn-rail-row>
            <lthn-rail-row k="Quantisation"  v="q4_k_m"></lthn-rail-row>
            <lthn-rail-row k="Context"       v="8,192"></lthn-rail-row>
            <lthn-rail-row k="Vocabulary"    v="262,144"></lthn-rail-row>
            <lthn-rail-row k="Architecture"  v="MoE · 4-expert"></lthn-rail-row>
            <lthn-rail-row k="Last loaded"   v="2 minutes ago"></lthn-rail-row>
            <lthn-rail-row k="Average tok/s" v="47.2 · last 100 runs"></lthn-rail-row>
          </div>
          <div style="padding:10px; border-radius:6px;
                      background:rgba(255,255,255,0.03); border:1px solid rgba(255,255,255,0.06);">
            <lthn-label>Files</lthn-label>
            <div style="margin-top:6px; display:flex; flex-direction:column; gap:4px;
                        font-family:var(--font-mono); font-size:10.5px; color:var(--fg-2);">
              <div style="display:flex; justify-content:space-between;"><span>gemma-4-e2b-q4_k_m.gguf</span><span style="color:var(--fg-3);">1.9 GB</span></div>
              <div style="display:flex; justify-content:space-between;"><span>tokenizer.json</span><span style="color:var(--fg-3);">4.2 MB</span></div>
              <div style="display:flex; justify-content:space-between;"><span>config.json</span><span style="color:var(--fg-3);">1.1 KB</span></div>
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
      title: "Models",
      subtitle: "local · 4 · huggingface",
      w: this.w, h: this.h,
      toolbar, body,
      footer: html`4 local · 312 GB free · ~/.lthn/models/ · airplane-mode OK (browsing requires network)`,
    });
  }

  _localItem(m) {
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
