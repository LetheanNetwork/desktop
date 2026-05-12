// SPDX-Licence-Identifier: EUPL-1.2
// E4.3 · fleet — <lthn-fleet-window>
// Light-DOM Lit element. Composes renderChrome() from ../chrome.js.

import { LitElement, html, nothing } from "lit";
import { renderChrome } from "../chrome";

class LthnFleetWindow extends LitElement {
  static properties = { w: { type: Number }, h: { type: Number }, embedded: { type: Boolean, reflect: true } };
  declare w: number;
  declare h: number;
  declare embedded: boolean;
  constructor() { super(); this.w = 1080; this.h = 700; this.embedded = false; }
  createRenderRoot() { return this; }

  render() {
    const machines = [
      { id: "this-mac",   name: "this Mac",                arch: "M3 Pro · 36 GB",     status: "online · loaded", model: "gemma-4-e2b",   load: 32, tps: "47.2", you: true },
      { id: "studio",     name: "vault · Studio M2",       arch: "M2 Ultra · 192 GB",  status: "online · idle",   model: "gemma-3-27b",   load: 4,  tps: "0" },
      { id: "ws",         name: "shop · 7950X · RTX 4090", arch: "x86 · 24 GB VRAM",   status: "online · loaded", model: "llama-3.3-70b", load: 78, tps: "11.4" },
      { id: "homeserver", name: "homeserver · 7900X",      arch: "x86 · 96 GB",        status: "online · idle",   model: "—",             load: 2,  tps: "0" },
      { id: "ana-air",    name: "ana-air · M2",            arch: "M2 · 16 GB",         status: "offline",         model: "—",             load: 0,  tps: "—", offline: true },
    ];
    const queue = [
      { id: "q1", who: "claude-code", model: "gemma-4-e2b",   route: "this Mac", state: "running", start: "14:32:14", elapsed: "3.2 s" },
      { id: "q2", who: "raycast",     model: "gemma-4-e2b",   route: "this Mac", state: "queued",  start: "—",        elapsed: "—" },
      { id: "q3", who: "opencode",    model: "llama-3.3-70b", route: "shop",     state: "running", start: "14:32:08", elapsed: "9.1 s" },
    ];

    const toolbar = html`
      <lthn-btn tone="primary" size="sm" active>Machines</lthn-btn>
      <lthn-btn tone="ghost" size="sm">Routing rules</lthn-btn>
      <lthn-btn tone="ghost" size="sm">Snapshots</lthn-btn>
      <div style="flex:1"></div>
      <lthn-state-pill variant="preview">Preview · v1.0</lthn-state-pill>
    `;

    const body = html`
      <div style="flex:1; display:flex; flex-direction:column; min-height:0; overflow:auto;">
        <div style="padding:16px 22px 8px;">
          <lthn-label>Machines · drag-reorder to set route priority</lthn-label>
        </div>
        <div style="padding:0 22px; display:flex; flex-direction:column; gap:8px;">
          ${machines.map(m => html`
            <div style="display:grid; grid-template-columns:16px 1.3fr 1.2fr 1.2fr 1fr 0.8fr 60px; gap:14px; padding:12px 16px; border-radius:8px; background:${m.offline ? "rgba(255,255,255,0.015)" : m.you ? "rgba(64,193,197,0.06)" : "rgba(255,255,255,0.03)"}; border:${m.you ? "1px solid rgba(64,193,197,0.22)" : "1px solid rgba(255,255,255,0.05)"}; opacity:${m.offline ? 0.55 : 1}; align-items:center;">
              <i class="fa-solid fa-grip-vertical" style="font-size:11px; color:var(--fg-3);"></i>
              <div>
                <div style="display:flex; align-items:center; gap:8px;">
                  <lthn-status-dot variant=${m.offline ? "idle" : "ok"}></lthn-status-dot>
                  <span style="font-size:13px; color:var(--fg-0); font-weight:500;">${m.name}</span>
                  ${m.you ? html`<lthn-state-pill variant="latest">You</lthn-state-pill>` : nothing}
                </div>
                <div style="font-family:var(--font-mono); font-size:10.5px; color:var(--fg-3); margin-top:3px;">${m.arch}</div>
              </div>
              <div style="font-size:11.5px; color:${m.offline ? "var(--fg-3)" : "var(--fg-2)"};">${m.status}</div>
              <div style="font-family:var(--font-mono); font-size:11px; color:var(--fg-1);">${m.model}</div>
              <div>
                <div style="display:flex; justify-content:space-between; font-size:10px; color:var(--fg-3); margin-bottom:3px;">
                  <span>load</span>
                  <span style="font-family:var(--font-mono); color:var(--fg-1);">${m.load}%</span>
                </div>
                <div style="height:4px; background:rgba(255,255,255,0.06); border-radius:2px; overflow:hidden;">
                  <div style="width:${m.load}%; height:100%; background:${m.load > 70 ? "var(--warning-400)" : "var(--brand-400)"};"></div>
                </div>
              </div>
              <div style="font-family:var(--font-mono); font-size:11px; color:var(--fg-1); text-align:right;">${m.tps} tok/s</div>
              <lthn-btn tone="quiet" size="sm"><i class="fa-solid fa-ellipsis"></i></lthn-btn>
            </div>
          `)}
        </div>

        <div style="padding:20px 22px 8px;">
          <lthn-label>Live queue</lthn-label>
        </div>
        <div style="margin:0 22px 22px; background:rgba(255,255,255,0.025); border:1px solid rgba(255,255,255,0.06); border-radius:8px; font-family:var(--font-mono); font-size:11.5px;">
          <div style="display:grid; grid-template-columns:100px 1fr 1fr 1fr 0.8fr 0.8fr; padding:10px 14px; border-bottom:1px solid rgba(255,255,255,0.05); color:var(--fg-3); font-size:10px; letter-spacing:0.04em; text-transform:uppercase;">
            <span>State</span><span>Caller</span><span>Model</span><span>Routed to</span><span>Started</span><span>Elapsed</span>
          </div>
          ${queue.map(q => html`
            <div style="display:grid; grid-template-columns:100px 1fr 1fr 1fr 0.8fr 0.8fr; padding:10px 14px; border-bottom:1px solid rgba(255,255,255,0.04); align-items:center;">
              <lthn-state-pill variant=${q.state}>${q.state}</lthn-state-pill>
              <span style="color:var(--fg-1);">${q.who}</span>
              <span style="color:var(--fg-0);">${q.model}</span>
              <span style="color:var(--brand-300);">${q.route}</span>
              <span style="color:var(--fg-2);">${q.start}</span>
              <span style="color:var(--fg-1);">${q.elapsed}</span>
            </div>
          `)}
        </div>
      </div>
    `;

    return renderChrome({
      title: "Fleet", subtitle: "multi-machine · v1.0 preview",
      w: this.w, h: this.h, toolbar, body,
      footer: html`4 of 5 online · routing latency-aware · ⌘R reroute · ⌘S snapshot`,
      embedded: this.embedded,
    });
  }
}
customElements.define("lthn-fleet-window", LthnFleetWindow);
