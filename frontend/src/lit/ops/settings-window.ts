// SPDX-Licence-Identifier: EUPL-1.2
// E1.2 · settings — <lthn-settings-window>
// Light-DOM Lit element. Composes renderChrome() from ../chrome.js.

import { LitElement, html, nothing } from "lit";
import { renderChrome } from "../chrome";
import type { LitContent } from "../types";

class LthnSettingsWindow extends LitElement {
  static properties = {
    open: { type: String, reflect: true },
    w:    { type: Number },
    h:    { type: Number },
  };
  declare open: string;
  declare w: number;
  declare h: number;
  declare embedded: boolean;
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
      embedded: this.embedded,
    });
  }

  _section({ title, desc, open, content }: { title: string; desc?: string; open: boolean; content: LitContent }) {
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

  _row(label: string, hint: string | null, control: LitContent) {
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

  _select(value: string) {
    return html`
      <div style="display:inline-flex; align-items:center; gap:8px; padding:6px 10px; border-radius:6px;
                  background:rgba(255,255,255,0.04); border:1px solid rgba(255,255,255,0.07);
                  font-size:11.5px; color:var(--fg-1);">
        ${value}
        <i class="fa-solid fa-angle-down" style="font-size:9px; color:var(--fg-3);"></i>
      </div>
    `;
  }

  _segment(value: string, options: string[]) {
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
