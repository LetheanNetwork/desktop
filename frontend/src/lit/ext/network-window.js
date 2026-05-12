// SPDX-Licence-Identifier: EUPL-1.2
// E4.1 · network — <lthn-network-window>
// Light-DOM Lit element. Composes renderChrome() from ../chrome.js.

import { LitElement, html, nothing } from "lit";
import { renderChrome } from "../chrome.js";

class LthnNetworkWindow extends LitElement {
  static properties = { w: { type: Number }, h: { type: Number } };
  constructor() { super(); this.w = 1080; this.h = 720; }
  createRenderRoot() { return this; }

  render() {
    const peers = [
      { id: "self",  name: "this Mac · M3 Pro",    role: "you",       layers: "embeddings · 0-3", lat: "—",     x: 0.50, y: 0.50 },
      { id: "p1",    name: "tobias-m4 · M4 Max",   role: "peer",      layers: "attention · 4-15", lat: "8 ms",  x: 0.78, y: 0.30 },
      { id: "p2",    name: "vault-7950x · RTX",    role: "peer",      layers: "FFN · 16-31",      lat: "14 ms", x: 0.82, y: 0.62 },
      { id: "p3",    name: "homeserver · 7900",    role: "peer",      layers: "KV-cache",         lat: "11 ms", x: 0.22, y: 0.34 },
      { id: "p4",    name: "ana-air · M2",         role: "peer-idle", layers: "—",                lat: "42 ms", x: 0.20, y: 0.68 },
    ];

    const toolbar = html`
      <lthn-btn tone="primary" size="sm" active>This session</lthn-btn>
      <lthn-btn tone="ghost" size="sm">Available peers</lthn-btn>
      <lthn-btn tone="ghost" size="sm">Ledger</lthn-btn>
      <div style="flex:1"></div>
      <lthn-state-pill variant="preview">Preview · v0.7</lthn-state-pill>
    `;

    const body = html`
      <div style="flex:1; display:grid; grid-template-columns:1fr 340px; min-height:0;">
        <main style="background:radial-gradient(circle at 50% 50%, rgba(64,193,197,0.05) 0%, rgba(11,16,22,0) 60%), var(--surf-0); position:relative; min-height:0; overflow:hidden;">
          <svg viewBox="0 0 1000 600" width="100%" height="100%" preserveAspectRatio="xMidYMid meet">
            ${[80, 160, 240, 320].map(r => html`<circle cx="500" cy="300" r=${r} fill="none" stroke="rgba(64,193,197,0.06)" stroke-dasharray="2 4"></circle>`)}
            ${peers.slice(1).map(p => html`
              <line x1="500" y1="300" x2=${p.x * 1000} y2=${p.y * 600}
                stroke=${p.role === "peer-idle" ? "rgba(255,255,255,0.10)" : "rgba(64,193,197,0.30)"}
                stroke-width=${p.role === "peer-idle" ? 1 : 1.6}
                stroke-dasharray=${p.role === "peer-idle" ? "3 3" : "0"}></line>
            `)}
            ${peers.slice(1, 4).map((p, i) => {
              const x1 = 500, y1 = 300, x2 = p.x * 1000, y2 = p.y * 600;
              const mx = (x1 + x2) / 2 + (i - 1) * 12;
              const my = (y1 + y2) / 2 + (i - 1) * 8;
              return html`
                <circle cx=${mx} cy=${my} r="3" fill="var(--brand-400)">
                  <animate attributeName="cx" values="${x1};${x2};${x1}" dur="3.2s" repeatCount="indefinite" begin="${i * 0.4}s"></animate>
                  <animate attributeName="cy" values="${y1};${y2};${y1}" dur="3.2s" repeatCount="indefinite" begin="${i * 0.4}s"></animate>
                  <animate attributeName="opacity" values="0;1;1;0" dur="3.2s" repeatCount="indefinite" begin="${i * 0.4}s"></animate>
                </circle>
              `;
            })}
            ${peers.map(p => {
              const cx = p.x * 1000, cy = p.y * 600;
              const isSelf = p.role === "you";
              const idle = p.role === "peer-idle";
              return html`
                <g transform="translate(${cx}, ${cy})">
                  ${isSelf ? html`<circle r="34" fill="rgba(64,193,197,0.10)"></circle>` : nothing}
                  <circle r=${isSelf ? 22 : 16}
                    fill=${idle ? "rgba(255,255,255,0.05)" : isSelf ? "var(--brand-500)" : "rgba(64,193,197,0.10)"}
                    stroke=${idle ? "rgba(255,255,255,0.15)" : "var(--brand-400)"}
                    stroke-width=${isSelf ? 0 : 1.5}></circle>
                  ${isSelf ? html`<text y="6" fill="#fff" font-size="14" text-anchor="middle" font-family="ui-monospace, monospace">λ</text>` : nothing}
                  <text y=${isSelf ? 50 : 36} fill="rgba(255,255,255,0.85)" font-size="11" text-anchor="middle" font-family="ui-monospace, monospace">${p.name}</text>
                  <text y=${isSelf ? 64 : 50} fill="rgba(255,255,255,0.40)" font-size="9.5" text-anchor="middle" font-family="ui-monospace, monospace">${p.layers}</text>
                  ${!isSelf && !idle ? html`<text y="-22" fill="var(--brand-300)" font-size="9" text-anchor="middle" font-family="ui-monospace, monospace">${p.lat}</text>` : nothing}
                </g>
              `;
            })}
          </svg>
          <div style="position:absolute; top:14px; left:16px; font-family:var(--font-mono); font-size:10.5px; color:var(--fg-3); letter-spacing:0.06em;">
            session · sora-1 · 142 tokens served · 14 ms median round-trip
          </div>
        </main>

        <aside style="background:rgba(0,0,0,0.18); border-left:1px solid rgba(255,255,255,0.05); padding:20px; overflow:auto; display:flex; flex-direction:column; gap:16px;">
          <div>
            <lthn-label>Active session</lthn-label>
            <div style="font-family:var(--font-mono); font-size:13px; color:var(--fg-0); margin-top:6px;">sora-1 · 70B</div>
            <div style="font-size:11px; color:var(--fg-3); margin-top:3px;">Split across 4 machines · 1 of which is yours</div>
          </div>
          <div style="display:flex; flex-direction:column; gap:6px; font-size:11.5px;">
            <lthn-rail-row k="You serve"      v="Embeddings · L0–3"></lthn-rail-row>
            <lthn-rail-row k="Peers serve"    v="Attention · FFN · KV"></lthn-rail-row>
            <lthn-rail-row k="Median latency" v="14 ms"></lthn-rail-row>
            <lthn-rail-row k="Privacy"        v="prompts split client-side"></lthn-rail-row>
            <lthn-rail-row k="Ledger"         v="0.0142 LTHN earned"></lthn-rail-row>
          </div>
          <div style="padding:12px; border-radius:6px; background:rgba(64,193,197,0.06); border:1px solid rgba(64,193,197,0.18); font-size:11.5px; color:var(--fg-1); line-height:1.55;">
            <div style="display:flex; align-items:center; gap:6px; margin-bottom:6px;">
              <i class="fa-solid fa-shield-halved" style="color:var(--brand-300); font-size:11px;"></i>
              <span style="color:var(--brand-300); font-family:var(--font-mono); font-size:10px; letter-spacing:0.06em; text-transform:uppercase;">Privacy</span>
            </div>
            Peers see model layers, not your prompts. Prompts are split + masked client-side before any layer leaves this Mac.
          </div>
          <div style="font-size:11px; color:var(--fg-3); line-height:1.55; font-style:italic;">
            "Sovereign first. Federated when you opt in. Never the other way round."
          </div>
          <lthn-btn tone="quiet" size="md">Leave session</lthn-btn>
        </aside>
      </div>
    `;

    return renderChrome({
      title: "Network", subtitle: "LetherNet · v0.7 preview",
      w: this.w, h: this.h, toolbar, body,
      footer: html`Disaggregated · 4 peers · session privacy-preserved · no PII shared · always opt-in`,
    });
  }
}
customElements.define("lthn-network-window", LthnNetworkWindow);
