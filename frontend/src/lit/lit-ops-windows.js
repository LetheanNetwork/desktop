/* lit-ops-windows.js — E1 operational windows
 *
 *   <lthn-welcome-window step="1|2|3">              760×580
 *   <lthn-settings-window open="general|models|…">  760×600
 *   <lthn-model-browser-window selected="…">       1040×700
 *
 * Faithful Lit port of windows-ops.jsx. Light DOM (so tokens.css applies),
 * composes the renderChrome() helper + the shared atoms from lit-chrome.js.
 */

import { LitElement, html, nothing } from "lit";
import { renderChrome } from "./lit-chrome.js";

/* ════════════════════════════════════════════════════════════════════
 *  E1.1 · <lthn-welcome-window step="1|2|3">
 * ════════════════════════════════════════════════════════════════════ */
class LthnWelcomeWindow extends LitElement {
  static properties = {
    step: { type: Number, reflect: true },
    w:    { type: Number },
    h:    { type: Number },
  };
  constructor() { super(); this.step = 1; this.w = 760; this.h = 580; }
  createRenderRoot() { return this; }

  render() {
    const steps = [
      { n: 1, label: "Model directory", hint: "Where models live" },
      { n: 2, label: "First model",     hint: "Pick a starter" },
      { n: 3, label: "Connect",         hint: "Wire up clients" },
    ];

    const body = html`
      <div style="flex:1; display:grid; grid-template-columns: 240px 1fr; min-height:0;">
        <!-- steps rail -->
        <aside style="background:rgba(0,0,0,0.18); border-right:1px solid rgba(255,255,255,0.05);
                      padding:26px 22px; display:flex; flex-direction:column; gap:18px;">
          <div style="display:flex; align-items:center; gap:10px;">
            <lthn-glyph size="24" color="var(--fg-0)" active></lthn-glyph>
            <div>
              <div style="font-size:13px; font-weight:600; color:var(--fg-0);">lthn</div>
              <div style="font-family:var(--font-mono); font-size:10px; color:var(--fg-3); letter-spacing:0.04em;">
                sovereign · single-watt
              </div>
            </div>
          </div>
          <div style="height:1px; background:rgba(255,255,255,0.06); margin:4px 0;"></div>
          ${steps.map(s => {
            const done = s.n < this.step;
            const here = s.n === this.step;
            return html`
              <div style="display:flex; gap:12px; align-items:flex-start;">
                <div style="width:22px; height:22px; border-radius:50%;
                            background:${done ? "var(--brand-500)" : "transparent"};
                            border:${here ? "1.5px solid var(--brand-400)" :
                                    done ? "1.5px solid var(--brand-500)" :
                                    "1.5px solid rgba(255,255,255,0.12)"};
                            display:flex; align-items:center; justify-content:center;
                            font-size:11px; font-weight:600;
                            color:${done ? "#fff" : here ? "var(--brand-300)" : "var(--fg-3)"};
                            flex-shrink:0;">
                  ${done
                    ? html`<i class="fa-solid fa-check" style="font-size:9px;"></i>`
                    : s.n}
                </div>
                <div>
                  <div style="font-size:12px; font-weight:500;
                              color:${here ? "var(--fg-0)" : "var(--fg-2)"};">${s.label}</div>
                  <div style="font-size:10.5px; color:var(--fg-3); margin-top:2px;">${s.hint}</div>
                </div>
              </div>
            `;
          })}
          <div style="flex:1"></div>
          <div style="font-size:10.5px; color:var(--fg-3); line-height:1.5;">
            You can change all of this later in Settings. Nothing leaves this Mac.
          </div>
        </aside>

        <!-- step body -->
        <main style="padding:32px 40px; display:flex; flex-direction:column; min-height:0;">
          ${this.step === 1 ? this._step1() : this.step === 2 ? this._step2() : this._step3()}
          <div style="flex:1"></div>
          <div style="display:flex; align-items:center; gap:10px; padding-top:18px;">
            ${this.step > 1 ? html`<lthn-btn tone="ghost" size="lg">Back</lthn-btn>` : nothing}
            <lthn-btn tone="quiet" size="lg">Skip for now</lthn-btn>
            <div style="flex:1"></div>
            <lthn-btn tone="primary" size="lg">
              ${this.step === 3
                ? html`<i class="fa-solid fa-check"></i> Finish`
                : this.step === 1
                ? html`<i class="fa-solid fa-arrow-right"></i> Use this folder`
                : html`<i class="fa-solid fa-arrow-right"></i> Download & continue`}
            </lthn-btn>
          </div>
        </main>
      </div>
    `;

    return renderChrome({
      title: "Welcome to lthn",
      subtitle: `step ${this.step} of 3`,
      w: this.w, h: this.h,
      body,
      footer: html`British English · dark default · accessibility light in Settings · v0.2.0-rc1`,
    });
  }

  _step1() {
    return html`
      <div style="display:flex; flex-direction:column; gap:18px;">
        <div>
          <div style="font-size:24px; font-weight:600; color:var(--fg-0); letter-spacing:-0.018em;">
            Where shall we keep your models?
          </div>
          <div style="font-size:13px; color:var(--fg-2); margin-top:8px; line-height:1.55; max-width:440px;">
            A folder on this Mac. Models can be big — pick somewhere with room.
            We default to your home directory; change it if you have a faster volume.
          </div>
        </div>
        <div style="margin-top:4px; padding:20px 22px; border:1.5px dashed rgba(64,193,197,0.30);
                    border-radius:10px; background:rgba(64,193,197,0.04);
                    display:flex; align-items:center; gap:18px;">
          <div style="width:44px; height:44px; border-radius:10px;
                      background:rgba(64,193,197,0.10); border:1px solid rgba(64,193,197,0.20);
                      display:flex; align-items:center; justify-content:center;">
            <i class="fa-solid fa-folder-open" style="font-size:18px; color:var(--brand-300);"></i>
          </div>
          <div style="flex:1;">
            <div style="font-family:var(--font-mono); font-size:13px; color:var(--fg-0); letter-spacing:-0.005em;">
              ~/.lthn/models/
            </div>
            <div style="font-size:11px; color:var(--fg-3); margin-top:2px;">
              312 GB free on this volume · default location
            </div>
          </div>
          <lthn-btn tone="ghost" size="md">Choose folder…</lthn-btn>
        </div>
      </div>
    `;
  }

  _step2() {
    const models = [
      { name:"Gemma 4 E2B (-assistant)", author:"Google",    size:"2.1 GB", ram:"4 GB", desc:"Best balance for first run · Lethean-recommended", rec:true },
      { name:"Llama 3.2 3B Instruct",    author:"Meta",      size:"3.4 GB", ram:"6 GB", desc:"Solid general-purpose · longer context window" },
      { name:"Phi 3.5 Mini Instruct",    author:"Microsoft", size:"2.6 GB", ram:"5 GB", desc:"Punches above its weight on reasoning" },
    ];
    return html`
      <div style="display:flex; flex-direction:column; gap:16px; min-height:0;">
        <div>
          <div style="font-size:24px; font-weight:600; color:var(--fg-0); letter-spacing:-0.018em;">
            Pick a model to start with.
          </div>
          <div style="font-size:13px; color:var(--fg-2); margin-top:8px; line-height:1.55; max-width:460px;">
            Three small models that run comfortably on Apple Silicon.
            You can add more from the model browser anytime.
          </div>
        </div>
        <div style="display:flex; flex-direction:column; gap:8px;">
          ${models.map(m => html`
            <div style="display:flex; align-items:center; gap:14px; padding:14px 16px; border-radius:10px;
                        background:${m.rec ? "rgba(64,193,197,0.06)" : "rgba(255,255,255,0.03)"};
                        border:1px solid ${m.rec ? "rgba(64,193,197,0.22)" : "rgba(255,255,255,0.06)"};">
              <div style="width:18px; height:18px; border-radius:50%;
                          border:1.5px solid ${m.rec ? "var(--brand-400)" : "rgba(255,255,255,0.18)"};
                          display:flex; align-items:center; justify-content:center; flex-shrink:0;">
                ${m.rec ? html`<div style="width:8px; height:8px; border-radius:50%; background:var(--brand-400);"></div>` : nothing}
              </div>
              <div style="flex:1;">
                <div style="display:flex; align-items:baseline; gap:8px;">
                  <span style="font-size:13.5px; font-weight:500; color:var(--fg-0); letter-spacing:-0.005em;">${m.name}</span>
                  <span style="font-size:11px; color:var(--fg-3);">· ${m.author}</span>
                  ${m.rec ? html`<lthn-state-pill variant="latest">Recommended</lthn-state-pill>` : nothing}
                </div>
                <div style="font-size:11.5px; color:var(--fg-2); margin-top:3px;">${m.desc}</div>
              </div>
              <div style="text-align:right; font-family:var(--font-mono); font-size:11px; color:var(--fg-3); letter-spacing:0.02em;">
                <div>${m.size}</div>
                <div style="margin-top:2px;">${m.ram} RAM</div>
              </div>
            </div>
          `)}
        </div>
      </div>
    `;
  }

  _step3() {
    const clients = [
      { name:"Claude Code", desc:"Anthropic's CLI · drop-in OpenAI-compatible endpoint", path:"~/.config/claude/config.json", checked:true },
      { name:"OpenCode",    desc:"Open-source coding agent",                              path:"~/.config/opencode/config.toml" },
      { name:"Codex",       desc:"OpenAI Codex CLI",                                      path:"~/.codex/config.yaml" },
    ];
    return html`
      <div style="display:flex; flex-direction:column; gap:16px;">
        <div>
          <div style="font-size:24px; font-weight:600; color:var(--fg-0); letter-spacing:-0.018em;">
            Want to wire it into your tools?
          </div>
          <div style="font-size:13px; color:var(--fg-2); margin-top:8px; line-height:1.55; max-width:460px;">
            lthn speaks the OpenAI-compatible API on
            <span style="font-family:var(--font-mono); color:var(--fg-1);">http://localhost:8000/v1</span>.
            We can drop the endpoint into these configs for you. The only outbound action lthn ever takes without you asking.
          </div>
        </div>
        <div style="display:flex; flex-direction:column; gap:6px;">
          ${clients.map(c => html`
            <div style="display:flex; align-items:center; gap:14px; padding:12px 14px; border-radius:8px;
                        background:rgba(255,255,255,0.03); border:1px solid rgba(255,255,255,0.06);">
              <input type="checkbox" ?checked=${c.checked} style="accent-color:var(--brand-400);" />
              <div style="flex:1;">
                <div style="font-size:12.5px; font-weight:500; color:var(--fg-0);">${c.name}</div>
                <div style="font-size:11px; color:var(--fg-3); margin-top:1px;
                            font-family:var(--font-mono); letter-spacing:0.01em;">${c.path}</div>
              </div>
              <div style="font-size:11px; color:var(--fg-3);">${c.desc}</div>
            </div>
          `)}
        </div>
      </div>
    `;
  }
}
customElements.define("lthn-welcome-window", LthnWelcomeWindow);

/* ════════════════════════════════════════════════════════════════════
 *  E1.2 · <lthn-settings-window open="models">
 * ════════════════════════════════════════════════════════════════════ */
class LthnSettingsWindow extends LitElement {
  static properties = {
    open: { type: String, reflect: true },
    w:    { type: Number },
    h:    { type: Number },
  };
  constructor() { super(); this.open = "models"; this.w = 760; this.h = 600; }
  createRenderRoot() { return this; }

  render() {
    const sections = [
      { id:"general",      icon:"fa-sliders",      label:"General" },
      { id:"models",       icon:"fa-cube",         label:"Models" },
      { id:"runner",       icon:"fa-gauge-high",   label:"Runner" },
      { id:"api",          icon:"fa-plug",         label:"API" },
      { id:"telemetry",    icon:"fa-wave-square",  label:"Telemetry" },
      { id:"integrations", icon:"fa-link",         label:"Integrations" },
      { id:"about",        icon:"fa-circle-info",  label:"About" },
    ];

    const body = html`
      <div style="flex:1; display:grid; grid-template-columns: 200px 1fr; min-height:0;">
        <!-- section rail -->
        <aside style="background:rgba(0,0,0,0.18); border-right:1px solid rgba(255,255,255,0.05);
                      padding:12px 8px; display:flex; flex-direction:column; gap:1px;">
          ${sections.map(s => html`
            <div style="padding:8px 12px; border-radius:6px;
                        background:${s.id === this.open ? "rgba(255,255,255,0.07)" : "transparent"};
                        display:flex; align-items:center; gap:10px;
                        font-size:12.5px;
                        color:${s.id === this.open ? "var(--fg-0)" : "var(--fg-2)"};
                        cursor:pointer;">
              <i class="fa-solid ${s.icon}" style="font-size:11px; width:14px; text-align:center;
                  color:${s.id === this.open ? "var(--brand-300)" : "var(--fg-3)"};"></i>
              ${s.label}
            </div>
          `)}
        </aside>

        <!-- body -->
        <main style="padding:28px 32px; overflow:auto; display:flex; flex-direction:column; gap:22px;">
          ${this._sectionModels()}
          ${this._sectionRunner()}
          ${this._sectionApi()}
        </main>
      </div>
    `;

    return renderChrome({
      title: "Settings",
      subtitle: "lthn · v0.2.0-rc1",
      w: this.w, h: this.h, body,
      footer: html`Changes apply immediately · ⌘W to close · the runner keeps running`,
    });
  }

  _section({ title, desc, open, content }) {
    return html`
      <div style="display:flex; flex-direction:column; gap:14px;">
        <div style="display:flex; align-items:center; gap:8px;">
          <i class="fa-solid ${open ? "fa-angle-down" : "fa-angle-right"}"
             style="font-size:11px; color:var(--fg-3);"></i>
          <div style="font-size:14.5px; font-weight:600; color:var(--fg-0); letter-spacing:-0.015em;">${title}</div>
        </div>
        ${desc ? html`<div style="font-size:11.5px; color:var(--fg-3); line-height:1.55; margin-left:20px;">${desc}</div>` : nothing}
        ${open ? html`
          <div style="margin-left:20px; display:flex; flex-direction:column; gap:14px;
                      padding:8px 0; border-top:1px solid rgba(255,255,255,0.05);">
            ${content}
          </div>
        ` : nothing}
      </div>
    `;
  }

  _row(label, hint, control) {
    return html`
      <div style="display:grid; grid-template-columns: 200px 1fr; gap:18px; align-items:flex-start; padding-top:8px;">
        <div style="display:flex; flex-direction:column; gap:3px;">
          <div style="font-size:12.5px; color:var(--fg-1); font-weight:500;">${label}</div>
          ${hint ? html`<div style="font-size:10.5px; color:var(--fg-3); line-height:1.5;">${hint}</div>` : nothing}
        </div>
        <div>${control}</div>
      </div>
    `;
  }

  _select(value) {
    return html`
      <div style="display:inline-flex; align-items:center; gap:8px; padding:6px 10px; border-radius:6px;
                  background:rgba(255,255,255,0.04); border:1px solid rgba(255,255,255,0.07);
                  font-size:11.5px; color:var(--fg-1);">
        ${value}
        <i class="fa-solid fa-angle-down" style="font-size:9px; color:var(--fg-3);"></i>
      </div>
    `;
  }

  _segment(value, options) {
    return html`
      <div style="display:inline-flex; border-radius:6px;
                  background:rgba(0,0,0,0.18); border:1px solid rgba(255,255,255,0.06); padding:2px;">
        ${options.map(o => html`
          <div style="padding:4px 10px; font-family:var(--font-mono); font-size:10.5px;
                      color:${o === value ? "var(--fg-0)" : "var(--fg-3)"};
                      background:${o === value ? "rgba(255,255,255,0.08)" : "transparent"};
                      border-radius:4px; letter-spacing:0.02em;">${o}</div>
        `)}
      </div>
    `;
  }

  _sectionModels() {
    return this._section({
      title: "Models", open: true,
      desc: "Where lthn looks for models and which one loads at startup.",
      content: html`
        ${this._row("Model directory", null, html`
          <div style="display:flex; align-items:center; gap:8px; padding:6px 10px; border-radius:6px;
                      background:rgba(255,255,255,0.04); border:1px solid rgba(255,255,255,0.07);
                      font-family:var(--font-mono); font-size:11.5px; color:var(--fg-1);">
            <i class="fa-regular fa-folder" style="font-size:11px; color:var(--fg-3);"></i>
            ~/.lthn/models/
            <lthn-btn tone="quiet" size="sm" style="margin-left:4px;">Change…</lthn-btn>
          </div>
        `)}
        ${this._row("Default model", "Auto-loads when the runner starts. Empty = no auto-load.",
          this._select("Gemma 4 E2B"))}
        ${this._row("Quantisation preference", "Pick the smallest quant your hardware comfortably runs.",
          this._segment("q4_k_m", ["q4_0", "q4_k_m", "q5_k_m", "q8_0"]))}
        ${this._row("Default sampling", "Per-model overrides live in the model browser.", html`
          <div style="display:flex; gap:18px; font-size:11.5px; color:var(--fg-2);">
            <span>Temp <span style="color:var(--fg-0); font-family:var(--font-mono);">0.7</span></span>
            <span>Top-p <span style="color:var(--fg-0); font-family:var(--font-mono);">0.95</span></span>
            <span>Max tok <span style="color:var(--fg-0); font-family:var(--font-mono);">1024</span></span>
          </div>
        `)}
      `,
    });
  }

  _sectionRunner() {
    return this._section({
      title: "Runner", open: false,
      desc: "How the inference process behaves. Don't change these unless you're sure.",
      content: nothing,
    });
  }

  _sectionApi() {
    return this._section({
      title: "API", open: true,
      desc: "HTTP server for OpenAI-compatible clients. Off by default; nothing leaves this Mac unless you turn it on.",
      content: html`
        ${this._row("HTTP server", null, html`<lthn-toggle on></lthn-toggle>`)}
        ${this._row("Endpoint", null, html`
          <span style="font-family:var(--font-mono); font-size:11.5px; color:var(--fg-1);">http://localhost:8000/v1</span>
        `)}
        ${this._row("API key", "Required for any client connecting to the local server.", html`
          <div style="display:flex; align-items:center; gap:6px;">
            <span style="font-family:var(--font-mono); font-size:11px; color:var(--fg-1);">sk-lthn-••••••••••••••••2qB7</span>
            <lthn-btn tone="quiet" size="sm"><i class="fa-regular fa-copy" style="font-size:10px;"></i></lthn-btn>
            <lthn-btn tone="quiet" size="sm"><i class="fa-solid fa-rotate-right" style="font-size:10px;"></i></lthn-btn>
          </div>
        `)}
      `,
    });
  }
}
customElements.define("lthn-settings-window", LthnSettingsWindow);

/* ════════════════════════════════════════════════════════════════════
 *  E1.3 · <lthn-model-browser-window selected="…">
 * ════════════════════════════════════════════════════════════════════ */
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

window.LthnWelcomeWindow = LthnWelcomeWindow;
window.LthnSettingsWindow = LthnSettingsWindow;
window.LthnModelBrowserWindow = LthnModelBrowserWindow;
