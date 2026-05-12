/* lit-obs-windows.js — E2 observability windows for Lethean Desktop
 *
 *   <lthn-benchmark-window w h>
 *   <lthn-logs-window w h tab="live|history|power">
 *   <lthn-telemetry-window w h fullscreen>
 *
 * Light-DOM Lit elements — assumes tokens.css + font-awesome loaded by host,
 * and lit-chrome.js already registered (provides <lthn-btn>, <lthn-label>,
 * <lthn-rail-row>, <lthn-status-dot>, <lthn-state-pill>, <lthn-glyph>,
 * <lthn-sparkline>, renderChrome()).
 */

import { LitElement, html, nothing } from "lit";
import { renderChrome } from "./lit-chrome.js";

/* ─────────────────────────────────────────────────────────────────
 * E2.1 · <lthn-benchmark-window>
 * ───────────────────────────────────────────────────────────────── */
class LthnBenchmarkWindow extends LitElement {
  static properties = { w: { type: Number }, h: { type: Number } };
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
    const xs = (c) => pad.l + (Math.log2(c / 128) / Math.log2(8192 / 128)) * (cw - pad.l - pad.r);
    const ys = (t) => pad.t + (1 - (t - 20) / (60 - 20)) * (ch - pad.t - pad.b);

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

/* ─────────────────────────────────────────────────────────────────
 * E2.2 · <lthn-logs-window>  (tab: live | history | power)
 * ───────────────────────────────────────────────────────────────── */
class LthnLogsWindow extends LitElement {
  static properties = { w: { type: Number }, h: { type: Number }, tab: { type: String } };
  constructor() { super(); this.w = 1000; this.h = 660; this.tab = "live"; }
  createRenderRoot() { return this; }

  render() {
    const tabs = [
      { id: "live",    label: "Live log",            icon: "fa-wave-square" },
      { id: "history", label: "Generation history",  icon: "fa-clock-rotate-left" },
      { id: "power",   label: "Power history",       icon: "fa-bolt" },
    ];
    const toolbar = html`
      ${tabs.map(t => html`
        <lthn-btn tone=${this.tab === t.id ? "primary" : "ghost"} size="sm" ?active=${this.tab === t.id}>
          <i class="fa-solid ${t.icon}" style="font-size:10px;"></i> ${t.label}
        </lthn-btn>
      `)}
      <div style="flex:1"></div>
      ${this.tab === "live" ? html`
        <lthn-btn tone="ghost" size="sm"><i class="fa-solid fa-magnifying-glass" style="font-size:10px;"></i> Filter</lthn-btn>
        <lthn-btn tone="ghost" size="sm"><i class="fa-solid fa-pause" style="font-size:10px;"></i> Pause</lthn-btn>
      ` : nothing}
    `;
    const footers = {
      live:    "streaming · 1,284 lines · 4 components · debug verbose=off",
      history: "27 generations · last 7 days · 1.42M tokens · 142.6 Wh",
      power:   "showing last 24h · sample 1 s · powermetrics backend",
    };
    const body =
      this.tab === "live"    ? this._renderLive()    :
      this.tab === "history" ? this._renderHistory() :
                               this._renderPower();
    return renderChrome({
      title: "Activity", subtitle: "logs · history · power",
      w: this.w, h: this.h, toolbar, body,
      footer: html`${footers[this.tab]}`,
    });
  }

  _renderLive() {
    const lines = [
      { t: "14:32:08.412", c: "runner",  s: "info",  m: "loaded gemma-4-e2b-q4_k_m.gguf (2.1 GB) into Metal heap" },
      { t: "14:32:08.418", c: "runner",  s: "info",  m: "kv-cache allocated · 8192 ctx · 384 MB" },
      { t: "14:32:08.421", c: "api",     s: "info",  m: "HTTP server listening on 127.0.0.1:8000" },
      { t: "14:32:14.802", c: "api",     s: "info",  m: "POST /v1/chat/completions · model=gemma-4-e2b · stream=true" },
      { t: "14:32:14.804", c: "runner",  s: "debug", m: "tokenize · 142 tokens · cache hit @ prefix(64)" },
      { t: "14:32:14.811", c: "runner",  s: "info",  m: "prefill · 78 new tok · 4820 tok/s · 8.2 W" },
      { t: "14:32:14.831", c: "runner",  s: "info",  m: "decode · streaming · target 47.2 tok/s" },
      { t: "14:32:18.106", c: "telem",   s: "debug", m: "powermetrics sample · cpu 4.2 W · gpu 4.1 W · ane 0.1 W" },
      { t: "14:32:21.488", c: "runner",  s: "info",  m: "decode complete · 158 tok in 3.35s · 47.2 tok/s · finish=stop" },
      { t: "14:32:21.491", c: "api",     s: "info",  m: "response sent · 4.687s e2e · 8.4 W peak" },
      { t: "14:32:24.002", c: "tray",    s: "debug", m: "sparkline frame · 60 samples · idle 0.4 W" },
      { t: "14:33:02.114", c: "api",     s: "warn",  m: "rate-limit · 127.0.0.1 · 12 req/s · soft cap 8 — backing off" },
      { t: "14:33:08.802", c: "telem",   s: "debug", m: "powermetrics sample · cpu 0.3 W · gpu 0.0 W · ane 0.0 W" },
      { t: "14:34:11.502", c: "kernel",  s: "info",  m: "metal command-buffer · 142 ops · 18.4 ms" },
    ];
    const sevColor = { info: "var(--fg-2)", debug: "var(--fg-3)", warn: "var(--warning-400)", error: "var(--err-400)" };
    const comps = [
      { k: "runner", n: 428, on: true }, { k: "api", n: 612, on: true },
      { k: "telem", n: 144, on: true }, { k: "tray", n: 62, on: true }, { k: "kernel", n: 38, on: true },
    ];
    const sevs = [
      { k: "error", c: "var(--err-400)",     on: true,  n: 0 },
      { k: "warn",  c: "var(--warning-400)", on: true,  n: 1 },
      { k: "info",  c: "var(--brand-300)",   on: true,  n: 8 },
      { k: "debug", c: "var(--fg-3)",        on: false, n: 5 },
    ];
    return html`
      <div style="flex:1; display:grid; grid-template-columns:180px 1fr; min-height:0;">
        <aside style="background:rgba(0,0,0,0.18); border-right:1px solid rgba(255,255,255,0.05); padding:14px 12px; display:flex; flex-direction:column; gap:12px;">
          <div>
            <lthn-label>Components</lthn-label>
            <div style="margin-top:8px; display:flex; flex-direction:column; gap:4px;">
              ${comps.map(c => html`
                <div style="display:flex; align-items:center; gap:8px; font-size:11px;">
                  <input type="checkbox" ?checked=${c.on} style="accent-color:var(--brand-400);">
                  <span style="font-family:var(--font-mono); color:var(--fg-1); flex:1;">${c.k}</span>
                  <span style="font-family:var(--font-mono); font-size:9.5px; color:var(--fg-3);">${c.n}</span>
                </div>
              `)}
            </div>
          </div>
          <div>
            <lthn-label>Severity</lthn-label>
            <div style="margin-top:8px; display:flex; flex-direction:column; gap:4px;">
              ${sevs.map(s => html`
                <div style="display:flex; align-items:center; gap:8px; font-size:11px;">
                  <input type="checkbox" ?checked=${s.on} style="accent-color:var(--brand-400);">
                  <span style="width:6px; height:6px; border-radius:50%; background:${s.c};"></span>
                  <span style="color:var(--fg-1); flex:1;">${s.k}</span>
                  <span style="font-family:var(--font-mono); font-size:9.5px; color:var(--fg-3);">${s.n}</span>
                </div>
              `)}
            </div>
          </div>
        </aside>
        <main style="overflow:auto; padding:8px 0; font-family:var(--font-mono); font-size:11.5px; line-height:1.6;">
          ${lines.map((l, i) => html`
            <div style="display:grid; grid-template-columns:112px 64px 50px 1fr; padding:1.5px 16px; background:${l.s === "warn" ? "rgba(245,158,11,0.06)" : i % 2 === 0 ? "transparent" : "rgba(255,255,255,0.015)"}; gap:10px;">
              <span style="color:var(--fg-3);">${l.t}</span>
              <span style="color:var(--fg-2);">${l.c}</span>
              <span style="color:${sevColor[l.s]}; letter-spacing:0.04em; font-size:10px; text-transform:uppercase;">${l.s}</span>
              <span style="color:var(--fg-1); white-space:pre-wrap;">${l.m}</span>
            </div>
          `)}
          <div style="padding:8px 16px; display:flex; align-items:center; gap:8px; font-size:10.5px; color:var(--fg-3);">
            <lthn-status-dot variant="ok" pulse></lthn-status-dot>
            live · waiting for next event…
          </div>
        </main>
      </div>
    `;
  }

  _renderHistory() {
    const gens = [
      { ts: "14:32:14",  model: "gemma-4-e2b",  tok: 158, tg: 47.2, w: 8.4,  prompt: "Rewrite this function to use streams instead of arrays…" },
      { ts: "12:08:42",  model: "gemma-4-e2b",  tok: 384, tg: 46.8, w: 8.3,  prompt: "Summarise the changes between v0.1 and v0.2-rc1 of the runner…" },
      { ts: "11:55:18",  model: "llama-3.2-3b", tok: 220, tg: 32.6, w: 11.8, prompt: "What's the difference between LoRA rank 8 and rank 16?" },
      { ts: "09:42:01",  model: "gemma-4-e2b",  tok: 642, tg: 45.9, w: 8.5,  prompt: "Draft a release note for the new model browser…" },
      { ts: "08:18:33",  model: "phi-3.5-mini", tok: 184, tg: 38.4, w: 9.6,  prompt: "Translate the following help-centre article to British English…" },
    ];
    return html`
      <div style="flex:1; padding:12px 22px 18px; overflow:auto;">
        <div style="background:rgba(255,255,255,0.025); border:1px solid rgba(255,255,255,0.06); border-radius:8px; font-family:var(--font-mono); font-size:11.5px;">
          <div style="display:grid; grid-template-columns:100px 1.3fr 0.6fr 0.6fr 0.6fr 2fr; padding:10px 14px; border-bottom:1px solid rgba(255,255,255,0.06); color:var(--fg-3); font-size:10px; letter-spacing:0.04em; text-transform:uppercase;">
            <span>Time</span><span>Model</span><span>Tokens</span><span>tok/s</span><span>Peak W</span><span>Prompt</span>
          </div>
          ${gens.map((g, i) => html`
            <div style="display:grid; grid-template-columns:100px 1.3fr 0.6fr 0.6fr 0.6fr 2fr; padding:10px 14px; border-bottom:${i < gens.length - 1 ? "1px solid rgba(255,255,255,0.04)" : "none"}; align-items:center; gap:8px;">
              <span style="color:var(--fg-2);">${g.ts}</span>
              <span style="color:var(--fg-0);">${g.model}</span>
              <span style="color:var(--fg-1);">${g.tok}</span>
              <span style="color:var(--brand-300);">${g.tg}</span>
              <span style="color:var(--fg-1);">${g.w}</span>
              <span style="color:var(--fg-2); white-space:nowrap; overflow:hidden; text-overflow:ellipsis;">${g.prompt}</span>
            </div>
          `)}
        </div>
        <div style="margin-top:14px; font-size:11px; color:var(--fg-3); line-height:1.55;">
          Local. Never leaves this Mac. Right-click a row to re-open it in chat or export the transcript.
        </div>
      </div>
    `;
  }

  _renderPower() {
    const samples = Array.from({ length: 60 }, (_, i) => {
      const base = 0.4 + Math.sin(i * 0.3) * 0.3;
      const spike = [12, 13, 14, 22, 23, 38, 39, 40, 50, 51].includes(i) ? 6 + ((i * 17 % 30) / 30) * 3 : 0;
      return Math.max(0.2, base + spike);
    });
    const w = 940, h = 280, pad = 32, max = 12;
    const kpis = [
      { l: "24h average",  v: "1.8 W",   s: "≈ a USB-C trickle" },
      { l: "24h total",    v: "44.2 Wh", s: "≈ 8 phone-charges" },
      { l: "Peak today",   v: "9.4 W",   s: "during decode @ 14:32" },
    ];
    return html`
      <div style="flex:1; padding:12px 22px 18px; display:flex; flex-direction:column; gap:14px; overflow:auto;">
        <div style="display:grid; grid-template-columns:repeat(3, 1fr); gap:10px;">
          ${kpis.map(k => html`
            <div style="padding:14px 16px; border-radius:8px; background:rgba(255,255,255,0.03); border:1px solid rgba(255,255,255,0.06);">
              <lthn-label>${k.l}</lthn-label>
              <div style="font-family:var(--font-mono); font-size:22px; color:var(--fg-0); margin-top:6px; letter-spacing:-0.01em;">${k.v}</div>
              <div style="font-size:11px; color:var(--fg-3); margin-top:4px;">${k.s}</div>
            </div>
          `)}
        </div>
        <div style="background:rgba(0,0,0,0.20); border:1px solid rgba(255,255,255,0.05); border-radius:8px; padding:12px;">
          <lthn-label>Watts · last 24 hours</lthn-label>
          <svg viewBox="0 0 ${w} ${h}" width="100%" height=${h} preserveAspectRatio="none" style="margin-top:6px;">
            ${[0, 3, 6, 9, 12].map(v => {
              const yy = h - pad - (v / max) * (h - pad - 16);
              return html`
                <line x1=${pad} x2=${w} y1=${yy} y2=${yy} stroke="rgba(255,255,255,0.04)"></line>
                <text x=${pad - 6} y=${yy + 3} fill="rgba(255,255,255,0.40)" font-size="10" text-anchor="end" font-family="ui-monospace, monospace">${v} W</text>
              `;
            })}
            <path d=${"M " + samples.map((s, i) => `${pad + (i / (samples.length - 1)) * (w - pad)} ${h - pad - (s / max) * (h - pad - 16)}`).join(" L ") + ` L ${w} ${h - pad} L ${pad} ${h - pad} Z`} fill="rgba(64,193,197,0.10)"></path>
            <path d=${"M " + samples.map((s, i) => `${pad + (i / (samples.length - 1)) * (w - pad)} ${h - pad - (s / max) * (h - pad - 16)}`).join(" L ")} stroke="var(--brand-400)" stroke-width="1.4" fill="none"></path>
            ${["00:00", "06:00", "12:00", "18:00", "now"].map((t, i) => html`
              <text x=${pad + (i / 4) * (w - pad)} y=${h - 8} fill="rgba(255,255,255,0.40)" font-size="10" text-anchor=${i === 4 ? "end" : "middle"} font-family="ui-monospace, monospace">${t}</text>
            `)}
          </svg>
          <div style="margin-top:8px; font-size:11px; color:var(--fg-3); font-style:italic; line-height:1.5;">
            For comparison — a typical fridge averages ~150 W. A Christmas-tree bulb, ~5 W.
          </div>
        </div>
      </div>
    `;
  }
}
customElements.define("lthn-logs-window", LthnLogsWindow);

/* ─────────────────────────────────────────────────────────────────
 * E2.3 · <lthn-telemetry-window>  (fullscreen demo readout)
 * ───────────────────────────────────────────────────────────────── */
class LthnTelemetryWindow extends LitElement {
  static properties = { w: { type: Number }, h: { type: Number }, fullscreen: { type: Boolean } };
  constructor() { super(); this.w = 880; this.h = 560; this.fullscreen = false; }
  createRenderRoot() { return this; }

  render() {
    const tokSpark  = "38,41,44,45,46,47.2,47,46.8,47.1,47.4,47.2,47.0,47.3,47.2,47.4,47.2,47.1,47.3,47.2,47.0";
    const wattSpark = "0.6,0.8,4.2,7.8,8.2,8.4,8.3,8.4,8.5,8.4,8.3,8.4,8.4,8.5,8.4,8.3,8.4,8.4,8.3,8.2";

    const big = (label, value, sub, glow, data, max) => html`
      <div style="display:flex; flex-direction:column; align-items:center; gap:10px;">
        <div style="font-family:var(--font-mono); font-size:10.5px; color:var(--fg-3); letter-spacing:0.20em; text-transform:uppercase;">${label}</div>
        <div style="font-family:var(--font-mono); font-size:92px; font-weight:300; color:var(--fg-0); letter-spacing:-0.04em; line-height:1; text-shadow:0 0 30px ${glow}55, 0 0 60px ${glow}22;">${value}</div>
        <lthn-sparkline data=${data} color=${glow} max=${max} .width=${320} .height=${36} fill></lthn-sparkline>
        <div style="font-size:11.5px; color:var(--fg-3);">${sub}</div>
      </div>
    `;

    const body = html`
      <div style="flex:1; background:radial-gradient(circle at 50% 35%, rgba(64,193,197,0.10) 0%, rgba(11,16,22,0) 60%), var(--surf-0); display:flex; flex-direction:column; align-items:center; justify-content:center; padding:40px 60px; gap:36px; position:relative; overflow:hidden;">
        <div style="display:grid; grid-template-columns:1fr 1fr; gap:64px; width:100%;">
          ${big("tok/s", "47.2", "generation speed", "var(--brand-400)", tokSpark, 60)}
          ${big("watts", "8.4",  "peak this turn",   "#a78bfa",          wattSpark, 12)}
        </div>
        <div style="display:flex; gap:28px; align-items:center; font-family:var(--font-mono); font-size:12px; color:var(--fg-2); padding-top:8px; border-top:1px solid rgba(255,255,255,0.05); width:100%; justify-content:center;">
          <div><span style="color:var(--fg-3);">model </span><span style="color:var(--fg-0);">gemma-4-e2b</span></div>
          <div style="width:1px; height:14px; background:rgba(255,255,255,0.06);"></div>
          <div><span style="color:var(--fg-3);">context </span><span style="color:var(--fg-0);">142 / 8,192</span></div>
          <div style="width:1px; height:14px; background:rgba(255,255,255,0.06);"></div>
          <div><span style="color:var(--fg-3);">quant </span><span style="color:var(--fg-0);">q4_k_m</span></div>
          <div style="width:1px; height:14px; background:rgba(255,255,255,0.06);"></div>
          <div style="display:flex; align-items:center; gap:6px;">
            <lthn-status-dot variant="ok"></lthn-status-dot>
            <span style="color:var(--success-400);">airplane-mode OK</span>
          </div>
        </div>
        <div style="position:absolute; bottom:18px; right:24px; display:flex; align-items:center; gap:8px; font-family:var(--font-mono); font-size:10px; color:var(--fg-3); letter-spacing:0.06em;">
          <lthn-glyph size="12" color="var(--fg-3)"></lthn-glyph>
          lthn · sovereign · single-watt
        </div>
      </div>
    `;
    return renderChrome({
      title: "Live telemetry",
      subtitle: this.fullscreen ? "fullscreen · ⎋ to exit" : "demo surface",
      w: this.w, h: this.h, body,
      footer: html`model · gemma-4-e2b · context 142 / 8192 · airplane-mode OK · ⌥⌘F for fullscreen`,
    });
  }
}
customElements.define("lthn-telemetry-window", LthnTelemetryWindow);
