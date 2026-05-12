// SPDX-Licence-Identifier: EUPL-1.2
// E2.1 · benchmark — <lthn-benchmark-window>
// Light-DOM Lit element. Composes renderChrome() from ../chrome.js.

import { LitElement, html, nothing } from "lit";
import { renderChrome } from "../chrome";

class LthnBenchmarkWindow extends LitElement {
  static properties = { w: { type: Number }, h: { type: Number } };
  declare w: number;
  declare h: number;
  constructor() { super(); this.w = 1000; this.h = 660; }
  createRenderRoot() { return this; }

  render() {
    const runs = [
      { ts: "2026-05-11 14:32",  model: "gemma-4-e2b",  pp: 4820, tg: 47.2, w: 8.4,  mem: "2.4 GB", here: true },
      { ts: "2026-05-11 09:14",  model: "gemma-4-e2b",  pp: 4780, tg: 46.8, w: 8.5,  mem: "2.4 GB" },
      { ts: "2026-05-10 18:02",  model: "llama-3.2-3b", pp: 3140, tg: 32.6, w: 11.8, mem: "3.6 GB" },
      { ts: "2026-05-09 21:55",  model: "phi-3.5-mini", pp: 3960, tg: 38.4, w: 9.6,  mem: "2.9 GB" },
      { ts: "2026-05-08 11:18",  model: "gemma-4-e2b",  pp: 4640, tg: 45.1, w: 8.6,  mem: "2.4 GB" },
    ];
    const curve = [
      { ctx: 128,  tg: 51.8 }, { ctx: 512,  tg: 50.4 }, { ctx: 1024, tg: 48.6 },
      { ctx: 2048, tg: 47.2 }, { ctx: 4096, tg: 43.1 }, { ctx: 6144, tg: 38.4 }, { ctx: 8192, tg: 33.6 },
    ];
    const cw = 880, ch = 220, pad = { l: 48, r: 18, t: 16, b: 28 };
    const xs = (c: number) => pad.l + (Math.log2(c / 128) / Math.log2(8192 / 128)) * (cw - pad.l - pad.r);
    const ys = (t: number) => pad.t + (1 - (t - 20) / (60 - 20)) * (ch - pad.t - pad.b);

    const toolbar = html`
      <div style="display:flex; align-items:center; gap:6px; padding:4px 10px; border-radius:6px; background:rgba(255,255,255,0.04); border:1px solid rgba(255,255,255,0.07); font-size:11px; color:var(--fg-1); font-family:var(--font-mono);">
        gemma-4-e2b · q4_k_m
        <i class="fa-solid fa-chevron-down" style="font-size:8px; color:var(--fg-3); margin-left:4px;"></i>
      </div>
      <lthn-btn tone="ghost" size="sm">PP only</lthn-btn>
      <lthn-btn tone="ghost" size="sm">TG only</lthn-btn>
      <lthn-btn tone="ghost" size="sm" active>Both</lthn-btn>
      <div style="flex:1"></div>
      <lthn-btn tone="primary" size="sm"><i class="fa-solid fa-play" style="font-size:9px;"></i> Run</lthn-btn>
      <lthn-btn tone="ghost" size="sm"><i class="fa-regular fa-file-arrow-down" style="font-size:10px;"></i> Export</lthn-btn>
    `;

    const body = html`
      <div style="flex:1; display:flex; flex-direction:column; min-height:0;">
        <!-- history table -->
        <div style="padding:14px 22px 8px; display:flex; flex-direction:column; gap:6px;">
          <div style="display:flex; align-items:center; justify-content:space-between;">
            <lthn-label>Recent runs · click to overlay on chart</lthn-label>
            <div style="font-family:var(--font-mono); font-size:10px; color:var(--fg-3);">2 selected · compare mode</div>
          </div>
          <div style="background:rgba(255,255,255,0.025); border:1px solid rgba(255,255,255,0.06); border-radius:8px; font-family:var(--font-mono); font-size:11px;">
            <div style="display:grid; grid-template-columns:20px 1.4fr 1.4fr 0.8fr 0.9fr 0.8fr 0.8fr 60px; padding:8px 14px; border-bottom:1px solid rgba(255,255,255,0.05); color:var(--fg-3); font-size:10px; letter-spacing:0.04em; text-transform:uppercase;">
              <span></span><span>Timestamp</span><span>Model</span><span>PP tok/s</span><span>TG tok/s</span><span>Peak W</span><span>Mem</span><span></span>
            </div>
            ${runs.map((r, i) => {
              const sel = i < 2;
              return html`
                <div style="display:grid; grid-template-columns:20px 1.4fr 1.4fr 0.8fr 0.9fr 0.8fr 0.8fr 60px; padding:8px 14px; background:${r.here ? "rgba(64,193,197,0.07)" : "transparent"}; border-bottom:${i < runs.length - 1 ? "1px solid rgba(255,255,255,0.04)" : "none"}; color:var(--fg-1); align-items:center;">
                  <span style="width:12px; height:12px; border-radius:3px; background:${sel ? (i === 0 ? "var(--brand-400)" : "#a78bfa") : "transparent"}; border:${sel ? "none" : "1.5px solid rgba(255,255,255,0.18)"};"></span>
                  <span style="color:var(--fg-2); font-size:10.5px;">${r.ts}</span>
                  <span style="color:var(--fg-0);">${r.model}</span>
                  <span>${r.pp.toLocaleString()}</span>
                  <span style="color:${r.here ? "var(--brand-300)" : "var(--fg-0)"};">${r.tg.toFixed(1)}</span>
                  <span>${r.w} W</span>
                  <span style="color:var(--fg-2);">${r.mem}</span>
                  <span style="text-align:right;">${r.here ? html`<lthn-state-pill variant="latest">Latest</lthn-state-pill>` : nothing}</span>
                </div>
              `;
            })}
          </div>
        </div>

        <!-- chart -->
        <div style="flex:1; padding:8px 22px 18px; display:flex; flex-direction:column; min-height:0;">
          <div style="display:flex; align-items:baseline; gap:14px; margin-bottom:6px;">
            <lthn-label>tok/s vs context length</lthn-label>
            <span style="font-size:10.5px; color:var(--fg-3);">· log scale on x</span>
          </div>
          <div style="flex:1; background:rgba(0,0,0,0.20); border:1px solid rgba(255,255,255,0.05); border-radius:8px; padding:8px;">
            <svg viewBox="0 0 ${cw} ${ch}" width="100%" height="100%" preserveAspectRatio="none">
              ${[60, 50, 40, 30, 20].map((y) => html`
                <line x1=${pad.l} x2=${cw - pad.r} y1=${ys(y)} y2=${ys(y)} stroke="rgba(255,255,255,0.05)"></line>
                <text x=${pad.l - 8} y=${ys(y) + 3} fill="rgba(255,255,255,0.35)" font-size="10" text-anchor="end" font-family="ui-monospace, monospace">${y}</text>
              `)}
              ${[128, 512, 1024, 2048, 4096, 8192].map((c) => html`
                <line x1=${xs(c)} x2=${xs(c)} y1=${pad.t} y2=${ch - pad.b} stroke="rgba(255,255,255,0.04)"></line>
                <text x=${xs(c)} y=${ch - pad.b + 14} fill="rgba(255,255,255,0.40)" font-size="9.5" text-anchor="middle" font-family="ui-monospace, monospace">${c >= 1024 ? `${c/1024}k` : c}</text>
              `)}
              <path d=${"M " + curve.map((p) => `${xs(p.ctx)} ${ys(p.tg)}`).join(" L ")} stroke="var(--brand-400)" stroke-width="2" fill="none"></path>
              ${curve.map((p) => html`<circle cx=${xs(p.ctx)} cy=${ys(p.tg)} r="3" fill="var(--brand-400)"></circle>`)}
              <path d="M 48 88 L 198 92 L 348 100 L 498 108 L 648 130 L 798 158 L 870 178" stroke="#a78bfa" stroke-opacity="0.55" stroke-width="1.8" fill="none" stroke-dasharray="3 3"></path>
              <g transform="translate(${cw - pad.r - 200}, ${pad.t + 6})">
                <rect width="200" height="42" fill="rgba(0,0,0,0.30)" stroke="rgba(255,255,255,0.06)" rx="4"></rect>
                <circle cx="14" cy="14" r="4" fill="var(--brand-400)"></circle>
                <text x="26" y="18" fill="rgba(255,255,255,0.85)" font-size="10" font-family="ui-monospace, monospace">gemma-4-e2b · today</text>
                <circle cx="14" cy="30" r="4" fill="#a78bfa" fill-opacity="0.6"></circle>
                <text x="26" y="34" fill="rgba(255,255,255,0.65)" font-size="10" font-family="ui-monospace, monospace">llama-3.2-3b · -2 d</text>
              </g>
              <text x="8" y=${pad.t + 4} fill="rgba(255,255,255,0.45)" font-size="9.5" font-family="ui-monospace, monospace" transform="rotate(-90 10 ${pad.t + 4})">tok/s</text>
            </svg>
          </div>
        </div>
      </div>
    `;

    return renderChrome({
      title: "Benchmark", subtitle: "run · compare · export",
      w: this.w, h: this.h, toolbar, body,
      footer: html`5 runs on file · ~/.lthn/bench/results.jsonl · last run 47.2 tok/s · 8.4 W`,
    });
  }
}
customElements.define("lthn-benchmark-window", LthnBenchmarkWindow);
