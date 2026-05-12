/**
 * lthn — Lethean Desktop frontend entry
 *
 * Mounts the Lit primitives from Lethean-5 (the design canon Lit port).
 * Each window is a prop-driven custom element; see lit/ for the catalogue.
 *
 * The tray popover (P0) is the default surface. Expansion windows
 * (<lthn-chat-window>, <lthn-settings-window>, etc.) open as transient
 * surfaces anchored to the tray-process; the Go-side core/gui wrapper
 * spawns them via Wails window APIs.
 */

import "./lit/chrome.js";
import "./lit/chat-window.js";
import "./lit/ops-windows.js";
import "./lit/obs-windows.js";
import "./lit/ext-windows.js";

// Default mount: in dev mode, render the design canvas (every window
// side-by-side) so design changes can be reviewed without booting Wails.
// In production, the URL routing (set by core/gui via WebviewWindow)
// determines which window mounts.

const app = document.getElementById("app");
const params = new URLSearchParams(location.search);
const surface = params.get("surface") || "canvas";

switch (surface) {
  case "tray": {
    // P0 — the popover panel. 400×560.
    // TODO: import desktop-panel once ported from Lethean-4 React → Lit.
    app.innerHTML = `<div style="padding:16px;font-family:var(--font-mono,monospace);">tray popover — port from lethean-4/desktop-panel.jsx</div>`;
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
  case "canvas":
  default: {
    // Dev canvas — every window side-by-side for design review.
    app.innerHTML = `
      <div style="display:flex;flex-direction:column;gap:24px;padding:24px;">
        <h2 style="font-family:var(--font-sans,system-ui);">lthn — design canvas</h2>
        <p style="opacity:0.7;font-size:14px;">Mount any window via <code>?surface=chat&amp;state=multi-turn</code> etc.</p>
        <ul style="opacity:0.7;font-size:13px;font-family:var(--font-mono,monospace);">
          <li><a href="?surface=chat">chat</a> · <a href="?surface=welcome">welcome</a> · <a href="?surface=settings">settings</a> · <a href="?surface=models">models</a></li>
          <li><a href="?surface=benchmark">benchmark</a> · <a href="?surface=logs">logs</a> · <a href="?surface=telemetry">telemetry</a></li>
          <li><a href="?surface=integrations">integrations</a> · <a href="?surface=tools">tools</a></li>
        </ul>
        <lthn-chat-window state="multi-turn"></lthn-chat-window>
      </div>`;
    break;
  }
}
