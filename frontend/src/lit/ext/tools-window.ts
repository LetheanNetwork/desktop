// SPDX-Licence-Identifier: EUPL-1.2
// E3.2 · tools — <lthn-tools-window>
// Light-DOM Lit element. Composes renderChrome() from ../chrome.js.

import { LitElement, html, nothing } from "lit";
import { renderChrome } from "../chrome";
import { T } from "@lthn/i18n/coreservice";

interface ToolView {
  name: string;
  description: string;
  group: string;
  input_schema: string;
}

class LthnToolsWindow extends LitElement {
  static readonly properties = {
    w: { type: Number },
    h: { type: Number },
    embedded: { type: Boolean, reflect: true },
    chrome: { state: true },
    toolList: { state: true },
    selectedTool: { state: true },
    activeModel: { state: true },
    t: { state: true },
  };
  declare w: number;
  declare h: number;
  declare embedded: boolean;
  declare chrome: { title: string; subtitle: string };
  declare toolList: ToolView[];
  declare selectedTool: string;
  declare activeModel: string;
  declare t: {
    btnAddServer: string; btnReload: string; toolbarModel: string;
    tagRegistered: string;
    rowServer: string; rowToolName: string; rowSource: string; rowSourceValue: string;
    labelSchema: string; schemaEmpty: string;
    labelRecent: string; labelTryit: string;
    btnInvoke: string; tryitHelp: string;
    emptyRegistry: string;
  };
  constructor() {
    super();
    this.w = 1040; this.h = 700; this.embedded = false;
    this.chrome = { title: "Tools · MCP", subtitle: "2 servers · 12 tools · 648 calls today" };
    this.toolList = [];
    this.selectedTool = "";
    this.activeModel = "gemma-4-e2b";
    this.t = {
      btnAddServer: "Add server", btnReload: "Reload",
      toolbarModel: "tool-use availability depends on model · current model: %s · ✓ supports tools",
      tagRegistered: "registered",
      rowServer: "Server", rowToolName: "Tool name",
      rowSource: "Source", rowSourceValue: "mcp registry",
      labelSchema: "Schema",
      schemaEmpty: "No input schema declared. The tool accepts an empty object.",
      labelRecent: "Recent calls",
      labelTryit:  "Try it · craft a test call",
      btnInvoke:   "Invoke",
      tryitHelp:   "Test calls bypass the model — useful for sanity-checking a server before plumbing it into a tool-using chat.",
      emptyRegistry: "No MCP tools registered yet.",
    };
  }
  createRenderRoot() { return this; }
  async connectedCallback() {
    super.connectedCallback();
    const [
      title, subtitleTpl,
      bas, br, tm,
      tr, rs, rtn, rsrc, rsv,
      ls, se, lr, lt, bi, th, er,
    ] = await Promise.all([
      T("window.tools.title"),
      T("window.tools.subtitle"),
      T("window.tools.btn_add_server"),
      T("window.tools.btn_reload"),
      T("window.tools.toolbar_model"),
      T("window.tools.tag_registered"),
      T("window.tools.row_server"),
      T("window.tools.row_tool_name"),
      T("window.tools.row_source"),
      T("window.tools.row_source_value"),
      T("window.tools.label_schema"),
      T("window.tools.schema_empty"),
      T("window.tools.label_recent"),
      T("window.tools.label_tryit"),
      T("window.tools.btn_invoke"),
      T("window.tools.tryit_help"),
      T("window.tools.empty_registry"),
    ]);
    this.t = {
      btnAddServer: bas, btnReload: br, toolbarModel: tm,
      tagRegistered: tr,
      rowServer: rs, rowToolName: rtn, rowSource: rsrc, rowSourceValue: rsv,
      labelSchema: ls, schemaEmpty: se,
      labelRecent: lr, labelTryit: lt,
      btnInvoke: bi, tryitHelp: th,
      emptyRegistry: er,
    };
    try {
      const svc = await import("@desktop/tools/wailsservice");
      const list = await svc.List();
      this.toolList = (list || []) as ToolView[];
      if (this.toolList.length > 0) this.selectedTool = this.toolList[0].name;
    } catch (err) {
      console.error("tools: list failed", err);
      this.toolList = [];
    }
    // Toolbar's "current model: X" slot — same source-of-truth as
    // chat-window. Keeps the gemma-4-e2b fallback so the design
    // string still reads coherently if WModels() returns empty.
    try {
      const runner = await import("@desktop/runner/service");
      const { unwrap } = await import("../result");
      const models = await unwrap<string[]>(runner.WModels(), []);
      if (models?.[0]) this.activeModel = models[0];
    } catch { /* keep fallback */ }
    // Build the subtitle from real counts — N servers (distinct
    // groups) · M tools. Falls back to the locale string when the
    // list is empty.
    const groupCount = new Set(this.toolList.map(t => t.group)).size;
    const subtitle = this.toolList.length > 0
      ? `${groupCount} ${groupCount === 1 ? "server" : "servers"} · ${this.toolList.length} ${this.toolList.length === 1 ? "tool" : "tools"}`
      : subtitleTpl;
    this.chrome = { title, subtitle };
  }

  render() {
    // Group the live tool list by Group → server view. on:true today
    // for every group since the MCP service has no per-group disable
    // toggle yet; the toggle UI stays so the future enable/disable
    // path has a UI seat already.
    const byGroup = new Map<string, ToolView[]>();
    for (const t of this.toolList) {
      const g = t.group || "ungrouped";
      if (!byGroup.has(g)) byGroup.set(g, []);
      byGroup.get(g)!.push(t);
    }
    const servers = Array.from(byGroup.entries()).map(([g, ts]) => ({
      id: g, name: g, tools: ts.length, on: true,
    }));
    const tools = this.toolList.map(t => ({
      server: t.group || "ungrouped",
      name: t.name,
      desc: t.description,
      schema: t.input_schema,
      sel: t.name === this.selectedTool,
    }));
    const sel = tools.find(t => t.sel) || tools[0];
    // Selected-tool schema — live JSON from the MCP registry. Empty
    // string when the upstream tool didn't declare one, in which case
    // the panel renders a "no schema" placeholder instead.
    const selSchema = sel ? sel.schema : "";

    const toolbar = html`
      <lthn-btn tone="ghost" size="sm"><i class="fa-solid fa-plus" style="font-size:10px;"></i> ${this.t.btnAddServer}</lthn-btn>
      <lthn-btn tone="ghost" size="sm"><i class="fa-solid fa-arrows-rotate" style="font-size:10px;"></i> ${this.t.btnReload}</lthn-btn>
      <div style="flex:1"></div>
      <span style="font-family:var(--font-mono); font-size:10.5px; color:var(--fg-3);">
        ${this.t.toolbarModel.replace("%s", this.activeModel)}
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
                    <div
                      @click=${() => { this.selectedTool = t.name; }}
                      style="padding:5px 14px 5px 22px; font-family:var(--font-mono); font-size:11px; border-radius:4px; background:${t.sel ? "rgba(255,255,255,0.06)" : "transparent"}; color:${t.sel ? "var(--fg-0)" : "var(--fg-2)"}; cursor:pointer; --wails-draggable: no-drag;">${t.name}</div>
                  `)}
                </div>
              ` : nothing}
            </div>
          `)}
        </aside>

        <!-- schema + calls -->
        <main style="padding:22px 26px; overflow:auto; display:flex; flex-direction:column; gap:18px;">
          ${sel ? html`
            <div>
              <div style="display:flex; align-items:baseline; gap:10px;">
                <span style="font-family:var(--font-mono); font-size:18px; color:var(--fg-0); letter-spacing:-0.005em;">${sel.server}.${sel.name}</span>
                <span style="font-size:11px; color:var(--fg-3);">· ${this.t.tagRegistered}</span>
              </div>
              <div style="font-size:12.5px; color:var(--fg-2); margin-top:5px; line-height:1.55;">${sel.desc}</div>
            </div>
            <div style="display:grid; grid-template-columns:repeat(3, 1fr); gap:8px;">
              ${[
                { k: this.t.rowServer,    v: sel.server },
                { k: this.t.rowToolName,  v: sel.name },
                { k: this.t.rowSource,    v: this.t.rowSourceValue },
              ].map(m => html`
                <div style="padding:10px 14px; border-radius:6px; background:rgba(255,255,255,0.025); border:1px solid rgba(255,255,255,0.05);">
                  <div style="font-size:10.5px; color:var(--fg-3); letter-spacing:0.04em; text-transform:uppercase;">${m.k}</div>
                  <div style="font-family:var(--font-mono); font-size:14px; color:var(--fg-0); margin-top:4px;">${m.v}</div>
                </div>
              `)}
            </div>
          ` : html`
            <div style="font-size:13px; color:var(--fg-3); padding:24px 0;">
              ${this.t.emptyRegistry}
            </div>
          `}
          <div>
            <lthn-label>${this.t.labelSchema}</lthn-label>
            ${selSchema ? html`
              <div style="margin-top:8px; background:rgba(0,0,0,0.30); border:1px solid rgba(255,255,255,0.06); border-radius:8px; padding:12px 14px; font-family:var(--font-mono); font-size:11.5px; line-height:1.6; color:var(--fg-1); white-space:pre; overflow:auto; max-height:280px;">${selSchema}</div>
            ` : html`
              <div style="margin-top:8px; padding:12px 14px; border-radius:8px;
                          background:rgba(255,255,255,0.025); border:1px solid rgba(255,255,255,0.05);
                          font-size:12px; color:var(--fg-3); font-style:italic;">
                ${this.t.schemaEmpty}
              </div>
            `}
          </div>
          <div>
            <lthn-label>${this.t.labelRecent}</lthn-label>
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
          <lthn-label>${this.t.labelTryit}</lthn-label>
          <div style="background:rgba(0,0,0,0.30); border:1px solid rgba(255,255,255,0.06); border-radius:6px; padding:10px; font-family:var(--font-mono); font-size:11.5px; line-height:1.6; color:var(--fg-1); white-space:pre; min-height:110px;">${String.raw`{
  "path":    "./scratch/hello.txt",
  "content": "hello, world\n"
}`}</div>
          <lthn-btn tone="primary" size="md"><i class="fa-solid fa-play" style="font-size:10px;"></i> ${this.t.btnInvoke}</lthn-btn>
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
            ${this.t.tryitHelp}
          </div>
        </aside>
      </div>
    `;

    // Footer counts come from the live registry — "configured" + "enabled"
    // are equal today since the MCP service has no per-group disable
    // toggle. "648 calls today · 99.4 % ok" stays as design canon —
    // no call-log service yet. Fall back to the design literals (5/3)
    // when the list is empty so canvas preview reads coherently.
    const sc = servers.length || 5;
    const se = servers.length || 3;
    return renderChrome({
      title: this.chrome.title, subtitle: this.chrome.subtitle,
      w: this.w, h: this.h, toolbar, body,
      footer: html`~/.lthn/mcp.json · ${sc} servers configured · ${se} enabled · 648 calls today · 99.4 % ok`,
      embedded: this.embedded,
    });
  }
}
customElements.define("lthn-tools-window", LthnToolsWindow);
