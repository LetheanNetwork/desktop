// SPDX-Licence-Identifier: EUPL-1.2
// E2.3 · telemetry — <lthn-telemetry-window>
// Light-DOM Lit element. Composes renderChrome() from ../chrome.js.

import { LitElement, html, nothing } from "lit";
import { renderChrome } from "../chrome.js";

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
