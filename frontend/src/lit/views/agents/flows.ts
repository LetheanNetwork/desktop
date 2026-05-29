// SPDX-Licence-Identifier: EUPL-1.2
// Agents view · Flows — <lthn-view-agent-flows>
//
// The Do-Engine ability registry: the codified abilities a model can reliably
// trigger, surfaced from the MCP tool catalogue (Tools.List()). Grouped into
// logical units behind an in-surface side-menu — the same rail-and-body pattern
// the settings window uses (ops/settings-window.ts): the left rail lists the
// ability groups, the body shows the selected group's abilities by their
// UNIFORM CONTRACT — name · description · expandable input schema. That uniform
// contract is what lets flows compose (N+1): every ability speaks the same
// call-shape, so a flow can be built from flows.
//
// Backend: Tools.List() from @desktop/tools/wailsservice (wraps the MCP tool
// catalogue) — dynamic import so tests run without the Wails runtime. Loaded on
// mount: the catalogue is local + cheap, no API hit. Read-only — it surfaces
// what CAN be done; abilities are triggered from chat / the Dispatch panel.
//
// Agentic compositions (multi-step flows) are the N+1 layer — they mount here
// as their own group alongside the base tool groups once the executor lands.
//
// Supports the `embedded` attribute — no chrome when set.

import { LitElement, html, nothing } from "lit";
import { renderChrome } from "../../chrome";

interface ToolView {
  name:         string;
  description:  string;
  group:        string;
  input_schema: string;
}

class LthnViewAgentFlows extends LitElement {
  static readonly properties = {
    w:        { type: Number },
    h:        { type: Number },
    embedded: { type: Boolean, reflect: true },
    tools:    { state: true },
    group:    { state: true },
    loaded:   { state: true },
    busy:     { state: true },
    err:      { state: true },
    open:     { state: true },
  };

  declare w: number;
  declare h: number;
  declare embedded: boolean;
  declare tools: ToolView[];
  declare group: string;
  declare loaded: boolean;
  declare busy: boolean;
  declare err: string;
  declare open: Record<string, boolean>;

  constructor() {
    super();
    this.w = 1180; this.h = 720; this.embedded = false;
    this.tools = [];
    this.group = "";
    this.loaded = false;
    this.busy = false;
    this.err = "";
    this.open = {};
  }

  createRenderRoot() { return this; }

  firstUpdated() { void this._load(); }

  async _load() {
    if (this.busy) return;
    this.busy = true;
    this.err = "";
    try {
      const svc = await import("@desktop/tools/wailsservice");
      const list = await (svc as { List: () => Promise<ToolView[]> }).List();
      this.tools = Array.isArray(list) ? list : [];
      // Land on the richest group so the registry opens onto something.
      if (!this.group) {
        const groups = this._groups();
        if (groups.length) this.group = groups[0].id;
      }
      this.loaded = true;
    } catch (e: unknown) {
      this.err = e instanceof Error ? e.message : String(e);
    } finally {
      this.busy = false;
    }
  }

  _toggle(name: string) {
    this.open = { ...this.open, [name]: !this.open[name] };
  }

  /** The logical units — one per distinct tool group, with its count.
   *  Sorted richest-first (alpha tie-break) so the rail is stable and
   *  the default landing group is the most capable. */
  _groups(): { id: string; count: number }[] {
    const m = new Map<string, number>();
    for (const t of this.tools) {
      const g = t.group || "other";
      m.set(g, (m.get(g) ?? 0) + 1);
    }
    return [...m.entries()]
      .map(([id, count]) => ({ id, count }))
      .sort((a, b) => b.count - a.count || a.id.localeCompare(b.id));
  }

  /** Abilities in the active group. */
  _activeTools(): ToolView[] {
    return this.tools.filter(t => (t.group || "other") === this.group);
  }

  /** Rail icon per group — best-effort, falls back to a generic
   *  "ability" bolt for groups added later. */
  _groupIcon(id: string): string {
    const m: Record<string, string> = {
      files:       "fa-folder",
      rag:         "fa-database",
      marketplace: "fa-store",
      ws:          "fa-tower-broadcast",
      language:    "fa-language",
      metrics:     "fa-gauge-high",
      webview:     "fa-window-maximize",
    };
    return m[id] ?? "fa-bolt";
  }

  /** Display label — acronyms stay upper, everything else capitalises. */
  _groupLabel(id: string): string {
    const m: Record<string, string> = { rag: "RAG", ws: "WS", webview: "WebView" };
    return m[id] ?? id.charAt(0).toUpperCase() + id.slice(1);
  }

  render() {
    const groups = this._groups();
    const active = this._activeTools();

    const rail = html`
      <aside style="background:rgba(0,0,0,0.18); border-right:1px solid rgba(255,255,255,0.05);
                    padding:12px 8px; display:flex; flex-direction:column; gap:1px; overflow:auto;">
        ${groups.map(g => html`
          <div @click=${() => { this.group = g.id; }}
            style="padding:8px 12px; border-radius:6px;
                   background:${g.id === this.group ? "rgba(255,255,255,0.07)" : "transparent"};
                   display:flex; align-items:center; gap:10px; font-size:12.5px;
                   color:${g.id === this.group ? "var(--fg-0)" : "var(--fg-2)"};
                   cursor:pointer; --wails-draggable:no-drag;">
            <i class="fa-solid ${this._groupIcon(g.id)}" style="font-size:11px; width:14px; text-align:center;
                color:${g.id === this.group ? "var(--brand-300)" : "var(--fg-3)"};"></i>
            <span style="flex:1; min-width:0; overflow:hidden; text-overflow:ellipsis; white-space:nowrap;">${this._groupLabel(g.id)}</span>
            <span style="font-family:var(--font-mono); font-size:10px; color:var(--fg-3);">${g.count}</span>
          </div>
        `)}
      </aside>
    `;

    const mainPane = html`
      <main style="padding:20px 24px; overflow:auto; display:flex; flex-direction:column; gap:14px; min-width:0;">
        <div style="display:flex; align-items:baseline; gap:8px;">
          <span style="font-size:14.5px; font-weight:600; color:var(--fg-0); letter-spacing:-0.015em;">${this._groupLabel(this.group)}</span>
          <span style="font-size:11px; color:var(--fg-3);">
            ${active.length} ${active.length === 1 ? "ability" : "abilities"} · uniform contract, runtime executes
          </span>
        </div>
        <div style="background:rgba(255,255,255,0.025); border:1px solid rgba(255,255,255,0.06); border-radius:10px; overflow:hidden;">
          ${active.map((t, i) => html`
            <div style="border-bottom:${i < active.length - 1 ? "1px solid rgba(255,255,255,0.04)" : "none"};">
              <div @click=${() => { if (t.input_schema) this._toggle(t.name); }}
                   style="padding:13px 16px; display:grid; grid-template-columns:1fr auto; gap:12px; align-items:center;
                          cursor:${t.input_schema ? "pointer" : "default"}; --wails-draggable:no-drag;">
                <div style="min-width:0;">
                  <span style="font-family:var(--font-mono); font-size:13px; color:var(--fg-0);">${t.name}</span>
                  ${t.description ? html`<div style="font-size:11.5px; color:var(--fg-2); margin-top:5px; line-height:1.5;">${t.description}</div>` : nothing}
                </div>
                ${t.input_schema
                  ? html`<i class="fa-solid ${this.open[t.name] ? "fa-chevron-up" : "fa-chevron-down"}" style="font-size:10px; color:var(--fg-3);"></i>`
                  : html`<span style="font-size:9.5px; color:var(--fg-4);">no schema</span>`}
              </div>
              ${t.input_schema && this.open[t.name] ? html`
                <pre style="margin:0; padding:12px 16px; background:rgba(0,0,0,0.25); font-family:var(--font-mono);
                            font-size:10.5px; color:var(--fg-2); overflow:auto; line-height:1.5; white-space:pre-wrap;">${t.input_schema}</pre>
              ` : nothing}
            </div>
          `)}
        </div>
      </main>
    `;

    const message = (text: string) => html`
      <div style="flex:1; padding:40px; text-align:center; color:var(--fg-3); font-size:12px;">${text}</div>
    `;

    const body = this.err ? html`
        <div style="flex:1; padding:14px 22px;">
          <div style="padding:14px; color:var(--err-400); font-size:12px; line-height:1.55;
                      background:rgba(255,76,76,0.06); border:1px solid rgba(255,76,76,0.18);
                      border-radius:10px; font-family:var(--font-mono);">
            ${this.err}
          </div>
        </div>
      ` : !this.loaded ? message("Loading the ability registry…")
        : this.tools.length === 0 ? message("No tools registered — the MCP catalogue is empty.")
        : html`
          <div style="flex:1; display:grid; grid-template-columns:200px 1fr; min-height:0;">
            ${rail}${mainPane}
          </div>
        `;

    return renderChrome({
      title: "Flows",
      subtitle: this.loaded ? `${this.tools.length} abilities · ${groups.length} groups` : "the Do-Engine ability registry",
      w: this.w, h: this.h,
      body,
      footer: html`codified abilities · uniform input/output contract · runtime executes · data via Tools.List()`,
      embedded: this.embedded,
    });
  }
}

customElements.define("lthn-view-agent-flows", LthnViewAgentFlows);
