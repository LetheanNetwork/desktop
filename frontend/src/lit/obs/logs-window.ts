// SPDX-Licence-Identifier: EUPL-1.2
// E2.2 · logs — <lthn-logs-window>
// Light-DOM Lit element. Composes renderChrome() from ../chrome.js.

import { LitElement, html, nothing } from "lit";
import { renderChrome } from "../chrome";

class LthnLogsWindow extends LitElement {
  static properties = { w: { type: Number }, h: { type: Number }, tab: { type: String } };
  declare w: number;
  declare h: number;
  declare tab: string;
  declare embedded: boolean;
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
      footer: html`${footers[this.tab as keyof typeof footers]}`,
      embedded: this.embedded,
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
              <span style="color:${sevColor[l.s as keyof typeof sevColor]}; letter-spacing:0.04em; font-size:10px; text-transform:uppercase;">${l.s}</span>
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
