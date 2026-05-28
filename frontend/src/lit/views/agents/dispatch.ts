// SPDX-Licence-Identifier: EUPL-1.2
// Agents view · Dispatch — <lthn-view-agent-dispatch>
//
// Launch a CoreAgent run: pick a repo + a fleet agent (the harness the
// run uses) + a task, and fire Agents.Dispatch() → agentic_dispatch over
// the loopback HTTP MCP. The agent runs detached; the run then shows up
// in the Activity panel. The agent picker is the Fleet registry — Dispatch
// consumes the fleet, it doesn't configure it.
//
// Backend: Agents.Dispatch() from @desktop/agents/service + Fleet.Agents()
// from @desktop/fleet/service — both imported dynamically so tests run
// without the Wails runtime. Needs lthn-agent serve up (the crew brings it
// up); a clear error shows when it isn't.
//
// Supports the `embedded` attribute — no chrome when set.

import { LitElement, html, nothing } from "lit";
import { renderChrome } from "../../chrome";

/** Minimal shape of a Fleet agent for the picker (mirrors fleet Agent). */
interface FleetAgent {
  id:       string;
  name:     string;
  provider: string;
}

/** Result of a dispatch (mirrors agents.DispatchResult). */
interface DispatchResult {
  agent:         string;
  repo:          string;
  workspace_dir: string;
  pid:           number;
}

class LthnViewAgentDispatch extends LitElement {
  static readonly properties = {
    w:        { type: Number },
    h:        { type: Number },
    embedded: { type: Boolean, reflect: true },
    agents:   { state: true },
    repo:     { state: true },
    agent:    { state: true },
    task:     { state: true },
    branch:   { state: true },
    dryRun:   { state: true },
    busy:     { state: true },
    result:   { state: true },
    err:      { state: true },
  };

  declare w: number;
  declare h: number;
  declare embedded: boolean;
  declare agents: FleetAgent[];
  declare repo: string;
  declare agent: string;
  declare task: string;
  declare branch: string;
  declare dryRun: boolean;
  declare busy: boolean;
  declare result: DispatchResult | null;
  declare err: string;

  constructor() {
    super();
    this.w = 1180; this.h = 720; this.embedded = false;
    this.agents = [];
    this.repo = "";
    this.agent = "";
    this.task = "";
    this.branch = "";
    this.dryRun = false;
    this.busy = false;
    this.result = null;
    this.err = "";
  }

  createRenderRoot() { return this; }

  async connectedCallback() {
    super.connectedCallback();
    await this._loadAgents();
  }

  /** Populate the agent picker from the Fleet registry. */
  async _loadAgents() {
    try {
      const svc = await import("@desktop/fleet/service");
      const r = await (svc as { Agents: () => Promise<{ Value: FleetAgent[] }> }).Agents();
      this.agents = r?.Value ?? [];
      if (!this.agent && this.agents.length > 0) this.agent = this.agents[0].name;
    } catch {
      // Fleet unavailable — the picker degrades to a free-text agent.
      this.agents = [];
    }
  }

  /** Fire the dispatch. Repo + task are required (the backend re-checks). */
  async _dispatch() {
    if (this.busy) return;
    if (!this.repo.trim() || !this.task.trim()) {
      this.err = "Repo and task are required.";
      return;
    }
    this.busy = true;
    this.err = "";
    this.result = null;
    try {
      const svc = await import("@desktop/agents/service");
      const req = {
        repo:    this.repo.trim(),
        task:    this.task.trim(),
        agent:   this.agent.trim(),
        branch:  this.branch.trim(),
        dry_run: this.dryRun,
      };
      const r = await (svc as { Dispatch: (req: unknown) => Promise<{ Value: DispatchResult }> }).Dispatch(req);
      this.result = r?.Value ?? null;
    } catch (e: unknown) {
      this.err = e instanceof Error ? e.message : String(e);
    } finally {
      this.busy = false;
    }
  }

  render() {
    const fieldStyle = `
      width:100%; margin-top:5px; padding:7px 9px; font-size:12px;
      background:rgba(0,0,0,0.25); color:var(--fg-0);
      border:1px solid rgba(255,255,255,0.08); border-radius:5px;
      font-family:inherit; --wails-draggable:no-drag;
    `;

    const body = html`
      <div style="flex:1; padding:18px 22px 22px; overflow:auto; max-width:640px;">
        <div style="display:flex; flex-direction:column; gap:16px;">
          <div>
            <lthn-label>Repo</lthn-label>
            <input style=${fieldStyle} placeholder="e.g. go-io  (or  org/repo)"
              .value=${this.repo}
              @input=${(e: Event) => { this.repo = (e.target as HTMLInputElement).value; }}>
          </div>

          <div>
            <lthn-label>Agent</lthn-label>
            ${this.agents.length > 0 ? html`
              <select style=${fieldStyle}
                .value=${this.agent}
                @change=${(e: Event) => { this.agent = (e.target as HTMLSelectElement).value; }}>
                ${this.agents.map(a => html`<option value=${a.name}>${a.name} — ${a.provider}</option>`)}
              </select>
            ` : html`
              <input style=${fieldStyle} placeholder="e.g. codex, claude, codex:gpt-5.4-mini"
                .value=${this.agent}
                @input=${(e: Event) => { this.agent = (e.target as HTMLInputElement).value; }}>
              <div style="margin-top:5px; font-size:10.5px; color:var(--fg-3);">
                No fleet agents configured — add one in Fleet, or type a harness name.
              </div>
            `}
          </div>

          <div>
            <lthn-label>Task</lthn-label>
            <textarea style="${fieldStyle} min-height:96px; resize:vertical; line-height:1.5;"
              placeholder="What should the agent do?"
              .value=${this.task}
              @input=${(e: Event) => { this.task = (e.target as HTMLTextAreaElement).value; }}></textarea>
          </div>

          <div style="display:flex; gap:16px; align-items:flex-end;">
            <div style="flex:1;">
              <lthn-label>Branch (optional)</lthn-label>
              <input style=${fieldStyle} placeholder="defaults to a task branch"
                .value=${this.branch}
                @input=${(e: Event) => { this.branch = (e.target as HTMLInputElement).value; }}>
            </div>
            <label style="display:flex; align-items:center; gap:7px; font-size:11.5px; color:var(--fg-2);
                          padding-bottom:8px; --wails-draggable:no-drag; cursor:pointer;">
              <input type="checkbox" .checked=${this.dryRun}
                @change=${(e: Event) => { this.dryRun = (e.target as HTMLInputElement).checked; }}>
              Dry run (prep only, don't spawn)
            </label>
          </div>

          <div style="display:flex; align-items:center; gap:12px; margin-top:2px;">
            <lthn-btn tone="primary" ?dim=${this.busy} @click=${() => void this._dispatch()}>
              <i class="fa-solid ${this.busy ? "fa-spinner" : "fa-paper-plane"}" style="font-size:11px;"></i>
              ${this.busy ? "Dispatching…" : this.dryRun ? "Preview dispatch" : "Dispatch"}
            </lthn-btn>
            <span style="font-size:10.5px; color:var(--fg-3);">runs on the fleet via CoreAgent</span>
          </div>

          ${this.err ? html`
            <div style="padding:10px 14px; color:var(--err-400);
                        background:rgba(255,76,76,0.06); border:1px solid rgba(255,76,76,0.18);
                        border-radius:6px; font-size:12px; line-height:1.55;">
              ${this.err}
            </div>
          ` : nothing}

          ${this.result ? html`
            <div style="padding:12px 14px; color:var(--success-400);
                        background:rgba(64,193,197,0.05); border:1px solid rgba(64,193,197,0.2);
                        border-radius:6px; font-size:12px; line-height:1.6;">
              <div style="color:var(--fg-0); margin-bottom:4px;">
                <i class="fa-solid fa-check" style="font-size:10px; margin-right:6px;"></i>
                Dispatched ${this.result.agent || this.agent} on ${this.result.repo || this.repo}
              </div>
              <div style="font-family:var(--font-mono); font-size:10.5px; color:var(--fg-3);">
                ${this.result.workspace_dir || "—"}${this.result.pid ? ` · pid ${this.result.pid}` : ""}
              </div>
              <div style="font-size:10.5px; color:var(--fg-3); margin-top:5px;">
                Watch it in Activity.
              </div>
            </div>
          ` : nothing}
        </div>
      </div>
    `;

    return renderChrome({
      title: "Dispatch",
      subtitle: "launch a CoreAgent run across the fleet",
      w: this.w, h: this.h,
      toolbar: nothing,
      body,
      footer: html`repo + agent + task → agentic_dispatch · agent runs detached · data via Agents.Dispatch()`,
      embedded: this.embedded,
    });
  }
}

customElements.define("lthn-view-agent-dispatch", LthnViewAgentDispatch);
