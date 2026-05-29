// SPDX-Licence-Identifier: EUPL-1.2
// Agents view · Flows — <lthn-view-agent-flows>
//
// The Do-Engine ability registry: the codified abilities a model can reliably
// trigger. Each flow is shown by its UNIFORM CONTRACT — name · group ·
// description · input schema — the shape the runtime guarantees regardless of
// whether the ability is a computational tool or an agentic process underneath.
// That uniform contract is what lets flows compose (N+1): every ability speaks
// the same call-shape, so a flow can be built from flows.
//
// Backend: Tools.List() from @desktop/tools/wailsservice (wraps the MCP tool
// catalogue) — dynamic import so tests run without the Wails runtime. Loaded on
// mount: the catalogue is local + cheap, no API hit. Read-only — it surfaces
// what CAN be done; abilities are triggered from chat / the Dispatch panel.
//
// Agentic compositions (multi-step flows) are the N+1 layer — they mount here
// alongside the base tool abilities once the flow executor lands.
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
    loaded:   { state: true },
    busy:     { state: true },
    err:      { state: true },
    filter:   { state: true },
    open:     { state: true },
  };

  declare w: number;
  declare h: number;
  declare embedded: boolean;
  declare tools: ToolView[];
  declare loaded: boolean;
  declare busy: boolean;
  declare err: string;
  declare filter: string;
  declare open: Record<string, boolean>;

  constructor() {
    super();
    this.w = 1180; this.h = 720; this.embedded = false;
    this.tools = [];
    this.loaded = false;
    this.busy = false;
    this.err = "";
    this.filter = "";
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

  _filtered(): ToolView[] {
    const q = this.filter.trim().toLowerCase();
    if (!q) return this.tools;
    return this.tools.filter(t =>
      (t.name ?? "").toLowerCase().includes(q) ||
      (t.group ?? "").toLowerCase().includes(q) ||
      (t.description ?? "").toLowerCase().includes(q));
  }

  render() {
    const fieldStyle = `
      padding:7px 9px; font-size:12px; background:rgba(0,0,0,0.25); color:var(--fg-0);
      border:1px solid rgba(255,255,255,0.08); border-radius:5px;
      font-family:inherit; --wails-draggable:no-drag;
    `;
    const badge = `
      font-family:var(--font-mono); font-size:9.5px; padding:1px 7px; border-radius:999px;
      background:rgba(255,255,255,0.05); border:1px solid rgba(255,255,255,0.1); color:var(--fg-3);
    `;
    const shown = this._filtered();

    const toolbar = html`
      <div style="display:flex; align-items:center; gap:8px; flex:1;">
        <input style="${fieldStyle} flex:1; max-width:340px;"
          placeholder="filter abilities (name · group · description)"
          .value=${this.filter}
          @input=${(e: Event) => { this.filter = (e.target as HTMLInputElement).value; }}>
      </div>
    `;

    const body = html`
      <div style="flex:1; padding:14px 22px 22px; overflow:auto;">
        ${this.err ? html`
          <div style="padding:14px; color:var(--err-400); font-size:12px; line-height:1.55;
                      background:rgba(255,76,76,0.06); border:1px solid rgba(255,76,76,0.18);
                      border-radius:10px; font-family:var(--font-mono);">
            ${this.err}
          </div>
        ` : shown.length === 0 ? html`
          <div style="padding:40px; text-align:center; color:var(--fg-3); font-size:12px;">
            ${this.loaded
              ? (this.filter ? `No abilities match "${this.filter}".`
                             : "No tools registered — the MCP catalogue is empty.")
              : "Loading the ability registry…"}
          </div>
        ` : html`
          <div style="font-size:11px; color:var(--fg-3); margin-bottom:10px;">
            ${shown.length} codified ${shown.length === 1 ? "ability" : "abilities"} the model can trigger · uniform contract, runtime executes
          </div>
          <div style="background:rgba(255,255,255,0.025); border:1px solid rgba(255,255,255,0.06); border-radius:10px; overflow:hidden;">
            ${shown.map((t, i) => html`
              <div style="border-bottom:${i < shown.length - 1 ? "1px solid rgba(255,255,255,0.04)" : "none"};">
                <div @click=${() => { if (t.input_schema) this._toggle(t.name); }}
                     style="padding:13px 16px; display:grid; grid-template-columns:1fr auto; gap:12px; align-items:center;
                            cursor:${t.input_schema ? "pointer" : "default"}; --wails-draggable:no-drag;">
                  <div style="min-width:0;">
                    <div style="display:flex; align-items:center; gap:8px; flex-wrap:wrap;">
                      <span style="font-family:var(--font-mono); font-size:13px; color:var(--fg-0);">${t.name}</span>
                      ${t.group ? html`<span style="${badge}">${t.group}</span>` : nothing}
                    </div>
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
        `}
      </div>
    `;

    return renderChrome({
      title: "Flows",
      subtitle: this.loaded ? `${this.tools.length} abilities in the registry` : "the Do-Engine ability registry",
      w: this.w, h: this.h,
      toolbar, body,
      footer: html`codified abilities · uniform input/output contract · runtime executes · data via Tools.List()`,
      embedded: this.embedded,
    });
  }
}

customElements.define("lthn-view-agent-flows", LthnViewAgentFlows);
