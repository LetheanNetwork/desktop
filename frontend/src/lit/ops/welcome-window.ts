// SPDX-Licence-Identifier: EUPL-1.2
// E1.1 · welcome / first-run — <lthn-welcome-window>
// Light-DOM Lit element. Composes renderChrome() from ../chrome.js.

import { LitElement, html, nothing } from "lit";
import { renderChrome } from "../chrome";

class LthnWelcomeWindow extends LitElement {
  static properties = {
    step: { type: Number, reflect: true },
    w:    { type: Number },
    h:    { type: Number },
  };
  declare step: number;
  declare w: number;
  declare h: number;
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
