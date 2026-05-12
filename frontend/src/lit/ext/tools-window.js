// SPDX-Licence-Identifier: EUPL-1.2
// E3.2 · tools — <lthn-tools-window>
// Light-DOM Lit element. Composes renderChrome() from ../chrome.js.

import { LitElement, html, nothing } from "lit";
import { renderChrome } from "../chrome.js";

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
      footer: html`~/.lthn/mcp.json · 5 servers configured · 3 enabled · 648 calls today · 99.4 % ok`,
    });
  }
}
customElements.define("lthn-tools-window", LthnToolsWindow);
