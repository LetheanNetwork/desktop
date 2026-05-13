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
  static properties = {
    w: { type: Number },
    h: { type: Number },
    embedded: { type: Boolean, reflect: true },
    chrome: { state: true },
    toolList: { state: true },
    selectedTool: { state: true },
    activeModel: { state: true },
  };
  declare w: number;
  declare h: number;
  declare embedded: boolean;
  declare chrome: { title: string; subtitle: string };
  declare toolList: ToolView[];
  declare selectedTool: string;
  declare activeModel: string;
  constructor() {
    super();
    this.w = 1040; this.h = 700; this.embedded = false;
    this.chrome = { title: "Tools · MCP", subtitle: "" };
    this.toolList = [];
    this.selectedTool = "";
    this.activeModel = "";
  }
  createRenderRoot() { return this; }
  async connectedCallback() {
    super.connectedCallback();
    const [title, subtitleTpl] = await Promise.all([
      T("window.tools.title"),
      T("window.tools.subtitle"),
    ]);
    try {
      const svc = await import("@desktop/tools/wailsservice");
      const list = await svc.List();
      this.toolList = (list || []) as ToolView[];
      if (this.toolList.length > 0) this.selectedTool = this.toolList[0].name;
    } catch (err) {
      console.error("tools: list failed", err);
      this.toolList = [];
    }
    // Active model — same pattern as chat-window. Empty string when
    // the runner has no model loaded.
    try {
      const runner = await import("@desktop/runner/service");
      const models = await runner.WModels().catch((): string[] => []);
      this.activeModel = (models && models[0]) || "";
    } catch {
      this.activeModel = "";
    }
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

    // Toolbar shows the live active model so the user knows which
    // model decides whether the tool list below is actually reachable
    // via chat. "Add server" / "Reload" buttons are deliberately
    // absent — neither has a binding behind it yet.
    const toolbar = html`
      <div style="flex:1"></div>
      <span style="font-family:var(--font-mono); font-size:10.5px; color:var(--fg-3);">
        tool-use availability depends on model · current model: ${this.activeModel || "—"}
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
                <span style="font-size:11px; color:var(--fg-3);">· registered</span>
              </div>
              <div style="font-size:12.5px; color:var(--fg-2); margin-top:5px; line-height:1.55;">${sel.desc}</div>
            </div>
            <div style="display:grid; grid-template-columns:repeat(3, 1fr); gap:8px;">
              ${[
                { k: "Server", v: sel.server },
                { k: "Tool name", v: sel.name },
                { k: "Source", v: "mcp registry" },
              ].map(m => html`
                <div style="padding:10px 14px; border-radius:6px; background:rgba(255,255,255,0.025); border:1px solid rgba(255,255,255,0.05);">
                  <div style="font-size:10.5px; color:var(--fg-3); letter-spacing:0.04em; text-transform:uppercase;">${m.k}</div>
                  <div style="font-family:var(--font-mono); font-size:14px; color:var(--fg-0); margin-top:4px;">${m.v}</div>
                </div>
              `)}
            </div>
          ` : html`
            <div style="font-size:13px; color:var(--fg-3); padding:24px 0;">
              No MCP tools registered yet.
            </div>
          `}
          <div>
            <lthn-label>Schema</lthn-label>
            ${selSchema ? html`
              <div style="margin-top:8px; background:rgba(0,0,0,0.30); border:1px solid rgba(255,255,255,0.06); border-radius:8px; padding:12px 14px; font-family:var(--font-mono); font-size:11.5px; line-height:1.6; color:var(--fg-1); white-space:pre; overflow:auto; max-height:280px;">${selSchema}</div>
            ` : html`
              <div style="margin-top:8px; padding:12px 14px; border-radius:8px;
                          background:rgba(255,255,255,0.025); border:1px solid rgba(255,255,255,0.05);
                          font-size:12px; color:var(--fg-3); font-style:italic;">
                No input schema declared. The tool accepts an empty object.
              </div>
            `}
          </div>
          <div>
            <lthn-label>Recent calls</lthn-label>
            <div style="margin-top:8px; padding:12px 14px; border-radius:8px;
                        background:rgba(255,255,255,0.025); border:1px solid rgba(255,255,255,0.05);
                        font-size:12px; color:var(--fg-3); font-style:italic;">
              Call history lands when the MCP service grows a per-tool log.
            </div>
          </div>
        </main>

        <!-- right rail · placeholder for try-it (invoke path is not wired) -->
        <aside style="background:rgba(0,0,0,0.18); border-left:1px solid rgba(255,255,255,0.05); padding:18px; overflow:auto; display:flex; flex-direction:column; gap:12px;">
          <lthn-label>Try it</lthn-label>
          <div style="padding:12px 14px; border-radius:8px;
                      background:rgba(255,255,255,0.025); border:1px solid rgba(255,255,255,0.05);
                      font-size:12px; color:var(--fg-3); line-height:1.55;">
            Tool invocation from this window isn't wired yet. Tools run today through the model — pick a tool-capable model in chat and ask for the action it should take.
          </div>
        </aside>
      </div>
    `;

    // Footer reflects what's actually known: group + tool counts from the
    // live registry. "calls today" / "% ok" wait on the call log.
    const groupCount = new Set(this.toolList.map(t => t.group)).size;
    const footer = this.toolList.length === 0
      ? html`No MCP tools registered`
      : html`${groupCount} ${groupCount === 1 ? "group" : "groups"} · ${this.toolList.length} ${this.toolList.length === 1 ? "tool" : "tools"} · sourced from the MCP registry`;

    return renderChrome({
      title: this.chrome.title, subtitle: this.chrome.subtitle,
      w: this.w, h: this.h, toolbar, body,
      footer,
      embedded: this.embedded,
    });
  }
}
customElements.define("lthn-tools-window", LthnToolsWindow);
