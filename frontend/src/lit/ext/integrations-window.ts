// SPDX-Licence-Identifier: EUPL-1.2
// E3.1 · integrations — <lthn-integrations-window>
// Light-DOM Lit element. Composes renderChrome() from ../chrome.js.

import { LitElement, html, nothing } from "lit";
import { renderChrome } from "../chrome";

class LthnIntegrationsWindow extends LitElement {
  static properties = { w: { type: Number }, h: { type: Number }, embedded: { type: Boolean, reflect: true } };
  declare w: number;
  declare h: number;
  declare embedded: boolean;
  constructor() { super(); this.w = 880; this.h = 660; this.embedded = false; }
  createRenderRoot() { return this; }

  render() {
    const clients = [
      { id: "claude-code", name: "Claude Code",   state: "connected",    desc: "Anthropic CLI · OpenAI-compatible endpoint mode",
        path: "~/.config/claude/config.json", lastPing: "8 s ago · 142 ms" },
      { id: "opencode",    name: "OpenCode",      state: "connected",    desc: "Open-source coding agent",
        path: "~/.config/opencode/config.toml", lastPing: "1 m ago · 158 ms" },
      { id: "codex",       name: "Codex CLI",     state: "disconnected", desc: "OpenAI CLI",
        path: "~/.codex/config.yaml" },
      { id: "copilot",     name: "GitHub Copilot",state: "disconnected", desc: "VS Code extension proxy mode",
        path: "~/Library/Application Support/Code/copilot/config.json" },
      { id: "pi",          name: "Pi (raycast)",  state: "available",    desc: "Raycast extension talks to lthn directly",
        path: "(no config needed)" },
    ];
    const selected = clients[0];

    const body = html`
      <div style="flex:1; display:grid; grid-template-columns:260px 1fr; min-height:0;">
        <aside style="background:rgba(0,0,0,0.18); border-right:1px solid rgba(255,255,255,0.05); padding:12px 8px; display:flex; flex-direction:column; gap:1px;">
          <lthn-label style="padding:4px 10px 8px;">Clients</lthn-label>
          ${clients.map((c, i) => {
            const active = i === 0;
            const variant = c.state === "connected" ? "ok" : c.state === "disconnected" ? "idle" : "warn";
            return html`
              <div style="padding:10px 12px; border-radius:6px; background:${active ? "rgba(255,255,255,0.06)" : "transparent"}; border-left:${active ? "2px solid var(--brand-400)" : "2px solid transparent"}; display:flex; flex-direction:column; gap:4px; cursor:pointer;">
                <div style="display:flex; align-items:center; gap:8px;">
                  <lthn-status-dot variant=${variant}></lthn-status-dot>
                  <span style="font-size:12.5px; color:var(--fg-0); font-weight:500;">${c.name}</span>
                </div>
                <div style="font-size:10.5px; color:var(--fg-3); margin-left:15px;">${c.state}</div>
              </div>
            `;
          })}
        </aside>
        <main style="padding:24px 32px; overflow:auto; display:flex; flex-direction:column; gap:22px;">
          <div>
            <div style="display:flex; align-items:center; gap:12px;">
              <div style="font-size:20px; font-weight:600; color:var(--fg-0); letter-spacing:-0.015em;">${selected.name}</div>
              <lthn-state-pill variant="connected">Connected</lthn-state-pill>
            </div>
            <div style="font-size:12.5px; color:var(--fg-2); margin-top:6px; max-width:460px; line-height:1.55;">${selected.desc}</div>
          </div>
          <div style="display:flex; gap:8px;">
            <lthn-btn tone="ghost" size="md"><i class="fa-solid fa-arrow-rotate-right" style="font-size:11px;"></i> Test connection</lthn-btn>
            <lthn-btn tone="ghost" size="md"><i class="fa-solid fa-arrow-up-right-from-square" style="font-size:10px;"></i> Open config in Finder</lthn-btn>
            <div style="flex:1"></div>
            <lthn-btn tone="quiet" size="md">Disconnect</lthn-btn>
          </div>
          <div style="display:grid; grid-template-columns:1fr 1fr; gap:10px;">
            ${[
              { k: "Last ping",     v: selected.lastPing },
              { k: "Endpoint",      v: "localhost:8000/v1" },
              { k: "Auth",          v: "sk-lthn-••••2qB7" },
              { k: "Default model", v: "gemma-4-e2b" },
            ].map(row => html`
              <div style="padding:10px 14px; border-radius:6px; background:rgba(255,255,255,0.025); border:1px solid rgba(255,255,255,0.05);">
                <div style="font-size:10.5px; color:var(--fg-3); letter-spacing:0.04em; text-transform:uppercase;">${row.k}</div>
                <div style="font-family:var(--font-mono); font-size:12.5px; color:var(--fg-0); margin-top:4px;">${row.v}</div>
              </div>
            `)}
          </div>
          <div>
            <lthn-label>Config preview · what lthn writes</lthn-label>
            <div style="margin-top:8px; background:rgba(0,0,0,0.30); border:1px solid rgba(255,255,255,0.06); border-radius:8px; padding:12px 14px; font-family:var(--font-mono); font-size:11.5px; line-height:1.6; color:var(--fg-1); white-space:pre;">${`{
  "apiBase":  "http://localhost:8000/v1",
  "apiKey":   "sk-lthn-•••• (managed by lthn)",
  "model":    "gemma-4-e2b",
  "stream":   true,
  "managed":  true
}`}</div>
            <div style="margin-top:10px; font-size:11px; color:var(--fg-3); line-height:1.55;">
              Only the <code>apiBase</code>, <code>apiKey</code> and <code>model</code> keys are managed by lthn. Anything else you set in this file is left alone.
            </div>
          </div>
        </main>
      </div>
    `;
    return renderChrome({
      title: "Integrations", subtitle: "clients · MCP · webhooks",
      w: this.w, h: this.h, body,
      footer: html`2 connected · 1 endpoint · http://localhost:8000/v1 · only outbound action lthn ever takes`,
      embedded: this.embedded,
    });
  }
}
customElements.define("lthn-integrations-window", LthnIntegrationsWindow);
