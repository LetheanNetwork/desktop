// SPDX-Licence-Identifier: EUPL-1.2
// E4.2 · distillation — <lthn-distillation-window>
// Light-DOM Lit element. Composes renderChrome() from ../chrome.js.

import { LitElement, html, nothing } from "lit";
import { renderChrome } from "../chrome";

class LthnDistillationWindow extends LitElement {
  static properties = { w: { type: Number }, h: { type: Number } };
  declare w: number;
  declare h: number;
  constructor() { super(); this.w = 1100; this.h = 740; }
  createRenderRoot() { return this; }

  render() {
    // Deterministic-ish loss curve so the screenshot is stable
    const loss = Array.from({ length: 40 }, (_, i) =>
      2.4 * Math.exp(-i * 0.06) + 0.4 + ((Math.sin(i * 1.7) * 0.08))
    );
    const cw = 740, ch = 220, pad = { l: 40, r: 14, t: 12, b: 24 };

    const steps = [
      { id: "1", label: "Base model" },
      { id: "2", label: "Dataset" },
      { id: "3", label: "Config" },
      { id: "4", label: "Run" },
      { id: "5", label: "Publish" },
    ];

    const toolbar = html`
      ${steps.map((s, i) => html`
        <div style="display:flex; align-items:center; gap:6px;">
          <div style="width:18px; height:18px; border-radius:50%; border:1.5px solid ${i < 3 ? "var(--brand-500)" : i === 3 ? "var(--brand-400)" : "rgba(255,255,255,0.12)"}; background:${i < 3 ? "var(--brand-500)" : "transparent"}; display:flex; align-items:center; justify-content:center; font-size:10px; font-weight:600; color:${i < 3 ? "#fff" : i === 3 ? "var(--brand-300)" : "var(--fg-3)"};">
            ${i < 3 ? html`<i class="fa-solid fa-check" style="font-size:8px;"></i>` : s.id}
          </div>
          <span style="font-size:12px; color:${i <= 3 ? "var(--fg-0)" : "var(--fg-3)"}; font-weight:${i === 3 ? 500 : 400};">${s.label}</span>
          ${i < 4 ? html`<span style="width:24px; height:1px; background:rgba(255,255,255,0.08); margin:0 8px;"></span>` : nothing}
        </div>
      `)}
      <div style="flex:1"></div>
      <lthn-btn tone="quiet" size="sm"><i class="fa-solid fa-stop" style="font-size:9px;"></i> Stop</lthn-btn>
    `;

    const lossPath = "M " + loss.map((v, i) =>
      `${pad.l + (i / (loss.length - 1)) * (cw - pad.l - pad.r)} ${pad.t + (1 - v / 2.4) * (ch - pad.t - pad.b)}`
    ).join(" L ");

    const body = html`
      <div style="flex:1; display:grid; grid-template-columns:300px 1fr 320px; min-height:0;">
        <aside style="background:rgba(0,0,0,0.18); border-right:1px solid rgba(255,255,255,0.05); padding:18px; overflow:auto; display:flex; flex-direction:column; gap:16px;">
          <div>
            <lthn-label>Recipe</lthn-label>
            <div style="margin-top:8px; display:flex; flex-direction:column; gap:8px; font-size:11.5px;">
              <lthn-rail-row k="Method"  v="LoRA · AdamW"></lthn-rail-row>
              <lthn-rail-row k="Rank"    v="16"></lthn-rail-row>
              <lthn-rail-row k="Alpha"   v="32"></lthn-rail-row>
              <lthn-rail-row k="Dropout" v="0.05"></lthn-rail-row>
              <lthn-rail-row k="LR"      v="1e-4 · cosine"></lthn-rail-row>
              <lthn-rail-row k="Batch"   v="8 · grad-accum 4"></lthn-rail-row>
              <lthn-rail-row k="Epochs"  v="3"></lthn-rail-row>
              <lthn-rail-row k="Targets" v="q_proj · v_proj · o_proj"></lthn-rail-row>
            </div>
          </div>
          <div style="height:1px; background:rgba(255,255,255,0.05);"></div>
          <div>
            <lthn-label>Base + dataset</lthn-label>
            <div style="margin-top:8px; display:flex; flex-direction:column; gap:8px; font-size:11.5px;">
              <lthn-rail-row k="Base"    v="gemma-4-e2b"></lthn-rail-row>
              <lthn-rail-row k="Dataset" v="lthn-helpcenter-v3"></lthn-rail-row>
              <lthn-rail-row k="Samples" v="4,820"></lthn-rail-row>
              <lthn-rail-row k="Split"   v="train 4.5k · eval 320"></lthn-rail-row>
              <lthn-rail-row k="Format"  v="ChatML"></lthn-rail-row>
            </div>
          </div>
        </aside>

        <main style="padding:20px 26px; overflow:auto; display:flex; flex-direction:column; gap:18px;">
          <div style="display:grid; grid-template-columns:repeat(4, 1fr); gap:8px;">
            ${[
              { k: "Epoch", v: "2 / 3",  sub: "step 184 / 270" },
              { k: "Loss",  v: "0.84",   sub: "↓ from 2.31" },
              { k: "tok/s", v: "1,142",  sub: "training throughput" },
              { k: "Watts", v: "9.8 W",  sub: "GPU + ANE" },
            ].map(m => html`
              <div style="padding:12px 14px; border-radius:8px; background:rgba(255,255,255,0.025); border:1px solid rgba(255,255,255,0.06);">
                <div style="font-size:10.5px; color:var(--fg-3); letter-spacing:0.04em; text-transform:uppercase;">${m.k}</div>
                <div style="font-family:var(--font-mono); font-size:22px; color:var(--fg-0); margin-top:4px; letter-spacing:-0.01em;">${m.v}</div>
                <div style="font-size:10.5px; color:var(--fg-3); margin-top:3px;">${m.sub}</div>
              </div>
            `)}
          </div>
          <div style="background:rgba(0,0,0,0.20); border:1px solid rgba(255,255,255,0.05); border-radius:8px; padding:12px;">
            <lthn-label>Loss · steps 0 → 184</lthn-label>
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
              <text x=${pad.l + 0.68 * (cw - pad.l - pad.r) + 4} y=${pad.t + 12} fill="var(--warning-400)" font-size="9.5" font-family="ui-monospace, monospace">epoch 2 begins</text>
            </svg>
          </div>
          <div>
            <lthn-label>Sample · eval prompt #142</lthn-label>
            <div style="margin-top:8px; display:grid; grid-template-columns:1fr 1fr; gap:8px;">
              ${[
                { who: "base · pre-tune", text: "Sure! Here are some general tips that may help you set up a Lethean instance, though I'm not certain about the specifics…", tone: "var(--fg-3)" },
                { who: "ours · step 184", text: "Add `LTHN_HOME=~/.lthn` to your shell, then `lthn runner start --model gemma-4-e2b`. The tray icon should appear within a few seconds.", tone: "var(--fg-1)" },
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
            <lthn-label>Output adapter</lthn-label>
            <div style="font-family:var(--font-mono); font-size:12px; color:var(--fg-0); margin-top:6px;">gemma-4-e2b-helpcenter-lora</div>
            <div style="font-size:11px; color:var(--fg-3); margin-top:3px;">~/.lthn/adapters/ · 42 MB</div>
          </div>
          <div style="display:flex; flex-direction:column; gap:6px;">
            <lthn-btn tone="primary" size="md"><i class="fa-regular fa-comment"></i> Test in chat</lthn-btn>
            <lthn-btn tone="ghost" size="md"><i class="fa-solid fa-code-merge" style="font-size:11px;"></i> Merge into base</lthn-btn>
            <lthn-btn tone="ghost" size="md"><i class="fa-solid fa-cloud-arrow-up" style="font-size:11px;"></i> Push to HuggingFace</lthn-btn>
          </div>
          <div style="height:1px; background:rgba(255,255,255,0.05);"></div>
          <div>
            <lthn-label>System</lthn-label>
            <div style="margin-top:8px; display:flex; flex-direction:column; gap:6px; font-size:11px;">
              <lthn-rail-row k="Backend"  v="go-mlx · Metal"></lthn-rail-row>
              <lthn-rail-row k="GPU mem"  v="13.2 / 36 GB"></lthn-rail-row>
              <lthn-rail-row k="Disk i/o" v="22 MB/s"></lthn-rail-row>
              <lthn-rail-row k="ETA"      v="14 min"></lthn-rail-row>
            </div>
          </div>
          <div style="font-size:11px; color:var(--fg-3); font-style:italic; line-height:1.55;">
            Training runs locally. The dataset stays on this Mac. The adapter is yours.
          </div>
        </aside>
      </div>
    `;

    return renderChrome({
      title: "Fine-tune", subtitle: "LoRA · SFT · distill · merge",
      w: this.w, h: this.h, toolbar, body,
      footer: html`step 4 of 5 · running · epoch 2/3 · loss 0.84 · ETA 14 min · 9.8 W`,
    });
  }
}
customElements.define("lthn-distillation-window", LthnDistillationWindow);
