/* lit-ext-windows.js — E3 integrations + E4 future-concept windows
 *
 *   <lthn-integrations-window w h>
 *   <lthn-tools-window w h>
 *   <lthn-network-window w h>
 *   <lthn-distillation-window w h>
 *   <lthn-fleet-window w h>
 *
 * Assumes lit-chrome.js already loaded (provides renderChrome, lthn-btn,
 * lthn-label, lthn-rail-row, lthn-toggle, lthn-status-dot, lthn-state-pill,
 * lthn-glyph).
 */

import { LitElement, html, nothing } from "lit";
import { renderChrome } from "./lit-chrome.js";

/* ─────────────────────────────────────────────────────────────────
 * E3.1 · <lthn-integrations-window>
 * ───────────────────────────────────────────────────────────────── */
class LthnIntegrationsWindow extends LitElement {
  static properties = { w: { type: Number }, h: { type: Number } };
  constructor() { super(); this.w = 880; this.h = 660; }
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
    });
  }
}
customElements.define("lthn-integrations-window", LthnIntegrationsWindow);

/* ─────────────────────────────────────────────────────────────────
 * E3.2 · <lthn-tools-window>
 * ───────────────────────────────────────────────────────────────── */
class LthnToolsWindow extends LitElement {
  static properties = { w: { type: Number }, h: { type: Number } };
  constructor() { super(); this.w = 1040; this.h = 700; }
  createRenderRoot() { return this; }

  render() {
    const servers = [
      { id: "fs",     name: "filesystem", tools: 4, on: true },
      { id: "git",    name: "git",        tools: 6, on: true },
      { id: "fetch",  name: "fetch",      tools: 2, on: true },
      { id: "sqlite", name: "sqlite",     tools: 5, on: false },
      { id: "shell",  name: "shell",      tools: 1, on: false },
    ];
    const tools = [
      { server: "filesystem", name: "read_file",   desc: "Read a UTF-8 file at the given path",                  uses: 184, ms: 12, ok: 100 },
      { server: "filesystem", name: "write_file",  desc: "Write content to a file, creating it if missing",       uses: 62,  ms: 18, ok: 100, sel: true },
      { server: "filesystem", name: "list_dir",    desc: "List entries in a directory",                           uses: 218, ms: 8,  ok: 100 },
      { server: "filesystem", name: "search_text", desc: "Regex search across files",                             uses: 44,  ms: 84, ok: 97.7 },
      { server: "git",        name: "status",      desc: "Show working-tree status",                              uses: 92,  ms: 22, ok: 100 },
      { server: "git",        name: "diff",        desc: "Show diff for a path or commit range",                  uses: 48,  ms: 38, ok: 100 },
    ];
    const sel = tools[1];

    const toolbar = html`
      <lthn-btn tone="ghost" size="sm"><i class="fa-solid fa-plus" style="font-size:10px;"></i> Add server</lthn-btn>
      <lthn-btn tone="ghost" size="sm"><i class="fa-solid fa-arrows-rotate" style="font-size:10px;"></i> Reload</lthn-btn>
      <div style="flex:1"></div>
      <span style="font-family:var(--font-mono); font-size:10.5px; color:var(--fg-3);">
        tool-use availability depends on model · current model: gemma-4-e2b · ✓ supports tools
      </span>
    `;

    const body = html`
      <div style="flex:1; display:grid; grid-template-columns:240px 1fr 320px; min-height:0;">
        <!-- server list -->
        <aside style="background:rgba(0,0,0,0.18); border-right:1px solid rgba(255,255,255,0.05); padding:12px 10px; overflow:auto; display:flex; flex-direction:column; gap:12px;">
          ${servers.map(s => html`
            <div>
              <div style="display:flex; align-items:center; gap:8px; padding:4px 8px; font-size:11.5px; color:var(--fg-0); font-weight:500;">
                <lthn-status-dot variant=${s.on ? "ok" : "idle"}></lthn-status-dot>
                <span>${s.name}</span>
                <span style="margin-left:auto; font-family:var(--font-mono); font-size:9.5px; color:var(--fg-3);">${s.tools}</span>
                <lthn-toggle ?on=${s.on}></lthn-toggle>
              </div>
              ${s.on ? html`
                <div style="margin-top:4px; display:flex; flex-direction:column;">
                  ${tools.filter(t => t.server === s.name).map(t => html`
                    <div style="padding:5px 14px 5px 22px; font-family:var(--font-mono); font-size:11px; border-radius:4px; background:${t.sel ? "rgba(255,255,255,0.06)" : "transparent"}; color:${t.sel ? "var(--fg-0)" : "var(--fg-2)"}; cursor:pointer;">${t.name}</div>
                  `)}
                </div>
              ` : nothing}
            </div>
          `)}
        </aside>

        <!-- schema + calls -->
        <main style="padding:22px 26px; overflow:auto; display:flex; flex-direction:column; gap:18px;">
          <div>
            <div style="display:flex; align-items:baseline; gap:10px;">
              <span style="font-family:var(--font-mono); font-size:18px; color:var(--fg-0); letter-spacing:-0.005em;">filesystem.write_file</span>
              <span style="font-size:11px; color:var(--fg-3);">· enabled</span>
            </div>
            <div style="font-size:12.5px; color:var(--fg-2); margin-top:5px; line-height:1.55;">${sel.desc}</div>
          </div>
          <div style="display:grid; grid-template-columns:repeat(3, 1fr); gap:8px;">
            ${[
              { k: "Calls today",  v: sel.uses },
              { k: "Avg latency",  v: sel.ms + " ms" },
              { k: "Success rate", v: sel.ok + " %" },
            ].map(m => html`
              <div style="padding:10px 14px; border-radius:6px; background:rgba(255,255,255,0.025); border:1px solid rgba(255,255,255,0.05);">
                <div style="font-size:10.5px; color:var(--fg-3); letter-spacing:0.04em; text-transform:uppercase;">${m.k}</div>
                <div style="font-family:var(--font-mono); font-size:18px; color:var(--fg-0); margin-top:4px;">${m.v}</div>
              </div>
            `)}
          </div>
          <div>
            <lthn-label>Schema</lthn-label>
            <div style="margin-top:8px; background:rgba(0,0,0,0.30); border:1px solid rgba(255,255,255,0.06); border-radius:8px; padding:12px 14px; font-family:var(--font-mono); font-size:11.5px; line-height:1.6; color:var(--fg-1); white-space:pre;">${`{
  "name": "write_file",
  "description": "Write content to a file...",
  "parameters": {
    "path":     { "type": "string",  "required": true },
    "content":  { "type": "string",  "required": true },
    "encoding": { "type": "string",  "default":  "utf-8" },
    "create":   { "type": "boolean", "default":  true }
  }
}`}</div>
          </div>
          <div>
            <lthn-label>Recent calls</lthn-label>
            <div style="margin-top:8px; background:rgba(255,255,255,0.025); border:1px solid rgba(255,255,255,0.05); border-radius:8px; font-family:var(--font-mono); font-size:11px;">
              ${[
                { t: "14:32:21", p: '{ "path": "./notes/draft.md", "content": "..." }', ms: 14, ok: true },
                { t: "13:18:04", p: '{ "path": "./tmp/out.json", "content": "..." }', ms: 11, ok: true },
                { t: "12:08:42", p: '{ "path": "./.cache/lock", "create": false }',    ms: 0,  ok: false },
              ].map((c, i) => html`
                <div style="display:grid; grid-template-columns:70px 1fr 50px 18px; padding:8px 14px; gap:10px; border-bottom:${i < 2 ? "1px solid rgba(255,255,255,0.04)" : "none"}; align-items:center;">
                  <span style="color:var(--fg-3);">${c.t}</span>
                  <span style="color:var(--fg-1); white-space:nowrap; overflow:hidden; text-overflow:ellipsis;">${c.p}</span>
                  <span style="color:${c.ok ? "var(--fg-1)" : "var(--err-400)"}; text-align:right;">${c.ms} ms</span>
                  <i class="fa-solid ${c.ok ? "fa-check" : "fa-xmark"}" style="font-size:10px; color:${c.ok ? "var(--success-400)" : "var(--err-400)"};"></i>
                </div>
              `)}
            </div>
          </div>
        </main>

        <!-- try-it rail -->
        <aside style="background:rgba(0,0,0,0.18); border-left:1px solid rgba(255,255,255,0.05); padding:18px; overflow:auto; display:flex; flex-direction:column; gap:12px;">
          <lthn-label>Try it · craft a test call</lthn-label>
          <div style="background:rgba(0,0,0,0.30); border:1px solid rgba(255,255,255,0.06); border-radius:6px; padding:10px; font-family:var(--font-mono); font-size:11.5px; line-height:1.6; color:var(--fg-1); white-space:pre; min-height:110px;">${`{
  "path":    "./scratch/hello.txt",
  "content": "hello, world\\n"
}`}</div>
          <lthn-btn tone="primary" size="md"><i class="fa-solid fa-play" style="font-size:10px;"></i> Invoke</lthn-btn>
          <div style="padding:10px 12px; border-radius:6px; background:rgba(34,197,94,0.06); border:1px solid rgba(34,197,94,0.18); font-size:11.5px; color:var(--fg-1); line-height:1.55;">
            <div style="display:flex; align-items:center; gap:6px; margin-bottom:6px;">
              <i class="fa-solid fa-check" style="color:var(--success-400); font-size:10px;"></i>
              <span style="color:var(--success-400); font-family:var(--font-mono); font-size:10px; letter-spacing:0.06em;">OK · 14 ms</span>
            </div>
            <div style="font-family:var(--font-mono); color:var(--fg-2); font-size:11px;">
              wrote 13 bytes to ./scratch/hello.txt
            </div>
          </div>
          <div style="font-size:10.5px; color:var(--fg-3); line-height:1.55;">
            Test calls bypass the model — useful for sanity-checking a server before plumbing it into a tool-using chat.
          </div>
        </aside>
      </div>
    `;

    return renderChrome({
      title: "Tools · MCP", subtitle: "2 servers · 12 tools · 648 calls today",
      w: this.w, h: this.h, toolbar, body,
      footer: html`~/Lethean/conf/mcp.json · 5 servers configured · 3 enabled · 648 calls today · 99.4 % ok`,
    });
  }
}
customElements.define("lthn-tools-window", LthnToolsWindow);

/* ─────────────────────────────────────────────────────────────────
 * E4.1 · <lthn-network-window>  (LetherNet preview)
 * ───────────────────────────────────────────────────────────────── */
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

/* ─────────────────────────────────────────────────────────────────
 * E4.2 · <lthn-distillation-window>  (LoRA / SFT / distill)
 * ───────────────────────────────────────────────────────────────── */
class LthnDistillationWindow extends LitElement {
  static properties = { w: { type: Number }, h: { type: Number } };
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
                { who: "ours · step 184", text: "Add `LTHN_HOME=~/Lethean` to your shell, then `lthn runner start --model gemma-4-e2b`. The tray icon should appear within a few seconds.", tone: "var(--fg-1)" },
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
            <div style="font-size:11px; color:var(--fg-3); margin-top:3px;">~/Lethean/conf/adapters/ · 42 MB</div>
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

/* ─────────────────────────────────────────────────────────────────
 * E4.3 · <lthn-fleet-window>  (multi-machine routing preview)
 * ───────────────────────────────────────────────────────────────── */
class LthnFleetWindow extends LitElement {
  static properties = { w: { type: Number }, h: { type: Number } };
  constructor() { super(); this.w = 1080; this.h = 700; }
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
    });
  }
}
customElements.define("lthn-fleet-window", LthnFleetWindow);
