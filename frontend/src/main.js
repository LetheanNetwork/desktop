/**
 * lthn — Lethean Desktop frontend entry
 *
 * Mounts the Lit primitives from Lethean-5 (the design canon Lit port).
 * Each window is a prop-driven custom element; see lit/ for the catalogue.
 *
 * The tray popover (P0) renders via renderChrome() at 400×560.
 * Expansion windows (chat, settings, etc.) open as transient surfaces;
 * the Go-side core/gui wrapper spawns them via Wails window APIs.
 */

import { html, render } from "lit";
import { renderChrome } from "./lit/index.js";
import "./lit/index.js";

const app = document.getElementById("app");
const params = new URLSearchParams(location.search);
const surface = params.get("surface") || "canvas";

switch (surface) {
  case "tray": {
    /* P0 tray popover — 400×560, composed inline from Lit primitives
     * (lit-chrome.js). No bespoke custom element — only renderChrome +
     * status-dot + btn + sparkline + label + state-pill. */
    render(renderChrome({
      title: "lthn",
      subtitle: "local · ready",
      w: 400, h: 560,
      body: html`
        <div style="display:flex; flex-direction:column; gap:14px; padding:14px; flex:1; min-height:0; overflow-y:auto;">
          <section style="display:flex; align-items:center; gap:10px; padding:10px 12px;
                          background:rgba(255,255,255,0.025); border:1px solid rgba(255,255,255,0.06); border-radius:8px;">
            <lthn-status-dot variant="ok" pulse></lthn-status-dot>
            <div style="display:flex; flex-direction:column; gap:2px; min-width:0; flex:1;">
              <div style="font-size:12.5px; font-weight:500; color:var(--fg-0);">Gemma 4 E2B</div>
              <div style="font-family:var(--font-mono); font-size:10px; color:var(--fg-3);">2,041 / 4,096 tokens · 1 runner</div>
            </div>
            <lthn-state-pill variant="running">live</lthn-state-pill>
          </section>

          <section style="display:flex; flex-direction:column; gap:6px;">
            <lthn-label>Open</lthn-label>
            <lthn-btn tone="ghost" size="md" @click=${() => location.assign("?surface=chat")}>Chat</lthn-btn>
            <lthn-btn tone="ghost" size="md" @click=${() => location.assign("?surface=models")}>Models</lthn-btn>
            <lthn-btn tone="ghost" size="md" @click=${() => location.assign("?surface=settings")}>Settings</lthn-btn>
          </section>

          <section style="display:flex; flex-direction:column; gap:8px; padding:10px 12px;
                          background:rgba(255,255,255,0.018); border:1px solid rgba(255,255,255,0.05); border-radius:8px;">
            <div style="display:flex; align-items:center; gap:8px;">
              <lthn-label>Activity</lthn-label>
              <span style="flex:1"></span>
              <span style="font-family:var(--font-mono); font-size:10.5px; color:var(--brand-300);">42 tok/s</span>
            </div>
            <lthn-sparkline width="372" height="32" fill></lthn-sparkline>
          </section>
        </div>
      `,
      footer: html`
        <span style="opacity:0.7;">lthn v0.1.0</span>
        <span style="flex:1"></span>
        <lthn-status-dot variant="active"></lthn-status-dot>
        <span style="opacity:0.7;">connected</span>
      `,
    }), app);
    break;
  }
  case "chat": {
    const state = params.get("state") || "multi-turn";
    app.innerHTML = `<lthn-chat-window state="${state}"></lthn-chat-window>`;
    break;
  }
  case "welcome": {
    const step = params.get("step") || "1";
    app.innerHTML = `<lthn-welcome-window step="${step}"></lthn-welcome-window>`;
    break;
  }
  case "settings": {
    const open = params.get("open") || "general";
    app.innerHTML = `<lthn-settings-window open="${open}"></lthn-settings-window>`;
    break;
  }
  case "models": {
    app.innerHTML = `<lthn-model-browser-window></lthn-model-browser-window>`;
    break;
  }
  case "benchmark": {
    app.innerHTML = `<lthn-benchmark-window></lthn-benchmark-window>`;
    break;
  }
  case "logs": {
    const tab = params.get("tab") || "live";
    app.innerHTML = `<lthn-logs-window tab="${tab}"></lthn-logs-window>`;
    break;
  }
  case "telemetry": {
    app.innerHTML = `<lthn-telemetry-window></lthn-telemetry-window>`;
    break;
  }
  case "integrations": {
    app.innerHTML = `<lthn-integrations-window></lthn-integrations-window>`;
    break;
  }
  case "tools": {
    app.innerHTML = `<lthn-tools-window></lthn-tools-window>`;
    break;
  }
  case "network": {
    app.innerHTML = `<lthn-network-window></lthn-network-window>`;
    break;
  }
  case "distillation": {
    app.innerHTML = `<lthn-distillation-window></lthn-distillation-window>`;
    break;
  }
  case "fleet": {
    app.innerHTML = `<lthn-fleet-window></lthn-fleet-window>`;
    break;
  }
  case "canvas":
  default: {
    app.innerHTML = `
      <div style="display:flex;flex-direction:column;gap:24px;padding:24px;">
        <h2 style="font-family:var(--font-sans,system-ui);">lthn — design canvas</h2>
        <p style="opacity:0.7;font-size:14px;">Mount any window via <code>?surface=chat&amp;state=multi-turn</code> etc.</p>
        <ul style="opacity:0.7;font-size:13px;font-family:var(--font-mono,monospace);">
          <li><a href="?surface=chat">chat</a> · <a href="?surface=welcome">welcome</a> · <a href="?surface=settings">settings</a> · <a href="?surface=models">models</a></li>
          <li><a href="?surface=benchmark">benchmark</a> · <a href="?surface=logs">logs</a> · <a href="?surface=telemetry">telemetry</a></li>
          <li><a href="?surface=integrations">integrations</a> · <a href="?surface=tools">tools</a></li>
          <li><a href="?surface=network">network</a> · <a href="?surface=distillation">distillation</a> · <a href="?surface=fleet">fleet</a></li>
        </ul>
        <lthn-chat-window state="multi-turn"></lthn-chat-window>
      </div>`;
    break;
  }
}
