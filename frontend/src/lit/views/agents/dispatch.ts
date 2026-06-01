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

/** A persona roster card (mirrors agents.PersonaCard / lib.PersonaCard). */
interface PersonaCard {
  path:  string;   // the --persona value, e.g. "code/senior-developer"
  name:  string;
  emoji: string;
  vibe:  string;
}

/** A plan/task template card (mirrors agents.TaskCard / lib.TaskCard). */
interface TaskCard {
  slug:        string;   // the --plan-template value, e.g. "package-update"
  name:        string;
  description: string;
  category:    string;
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
    personas: { state: true },
    tasks:    { state: true },
    repo:     { state: true },
    issue:    { state: true },
    pr:       { state: true },
    agent:    { state: true },
    persona:  { state: true },
    planTemplate: { state: true },
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
  declare personas: PersonaCard[];
  declare tasks: TaskCard[];
  declare repo: string;
  declare issue: number;
  declare pr: number;
  declare agent: string;
  declare persona: string;
  declare planTemplate: string;
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
    this.personas = [];
    this.tasks = [];
    this.repo = "";
    this.issue = 0;
    this.pr = 0;
    this.agent = "";
    this.persona = "";
    this.planTemplate = "";
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
    // A row clicked while this view is already mounted seeds via this event;
    // a cold mount (navigating in) is seeded by the shell's _instantiate.
    window.addEventListener("lthn:dispatch-repo", this._onSeed);
    await this._loadAgents();
    await this._loadPersonas();
    await this._loadTasks();
  }

  disconnectedCallback() {
    window.removeEventListener("lthn:dispatch-repo", this._onSeed);
    super.disconnectedCallback();
  }

  /** Apply a dispatch seed to an already-mounted Dispatch view (repo/issue/PR
   *  + task from a Coding row). issue/pr reset to the event's values so a
   *  plain repo dispatch clears any stale issue/PR chip. */
  _onSeed = (ev: Event) => {
    const d = (ev as CustomEvent<{ repo?: string; issue?: number; pr?: number; task?: string }>).detail;
    if (!d || (!d.repo && !d.issue && !d.pr)) return;
    if (d.repo) this.repo = d.repo;
    this.issue = d.issue ?? 0;
    this.pr = d.pr ?? 0;
    if (d.task) this.task = d.task;
  };

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

  /** Populate the persona picker from the CoreAgent roster (lib personas). */
  async _loadPersonas() {
    this.personas = await this._loadAgentList<PersonaCard>("Personas");
  }

  /** Call an Agents list binding, returning [] on any error or non-array
   *  result. Retries once after a beat: the lthn-agent CLI shell it spawns
   *  can be killed by boot-time crew churn if the view is opened early, and
   *  on error r.Value is the error object — never store that as the list. */
  async _loadAgentList<T>(method: "Personas" | "Tasks"): Promise<T[]> {
    for (let attempt = 0; attempt < 2; attempt++) {
      try {
        const svc = await import("@desktop/agents/service");
        const fn = (svc as Record<string, () => Promise<{ Value: unknown }>>)[method];
        const r = await fn();
        if (Array.isArray(r?.Value)) return r.Value as T[];
      } catch { /* fall through to retry / empty */ }
      if (attempt === 0) await new Promise(res => setTimeout(res, 1500));
    }
    return [];
  }

  /** Group personas by path category (the segment before "/"), sorted. */
  _personaGroups(): [string, PersonaCard[]][] {
    const groups = new Map<string, PersonaCard[]>();
    for (const p of this.personas) {
      const cat = p.path.includes("/") ? p.path.split("/")[0] : "other";
      const list = groups.get(cat);
      if (list) list.push(p); else groups.set(cat, [p]);
    }
    return [...groups.entries()].sort((a, b) => a[0].localeCompare(b[0]));
  }

  /** The vibe line of the selected persona, shown as a hint under the picker. */
  _selectedVibe(): string {
    return this.personas.find(p => p.path === this.persona)?.vibe ?? "";
  }

  /** Populate the premade-task picker from the CoreAgent plan templates. */
  async _loadTasks() {
    this.tasks = await this._loadAgentList<TaskCard>("Tasks");
  }

  /** Group task templates by category, sorted. */
  _taskGroups(): [string, TaskCard[]][] {
    const groups = new Map<string, TaskCard[]>();
    for (const t of this.tasks) {
      const cat = t.category || "other";
      const list = groups.get(cat);
      if (list) list.push(t); else groups.set(cat, [t]);
    }
    return [...groups.entries()].sort((a, b) => a[0].localeCompare(b[0]));
  }

  /** Apply a premade task: set the plan template and prefill the editable task. */
  _pickTask(slug: string) {
    this.planTemplate = slug;
    const card = this.tasks.find(t => t.slug === slug);
    if (card && card.description) this.task = card.description;
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
        agent:        this.agent.trim(),
        persona:      this.persona.trim(),
        plan_template: this.planTemplate.trim(),
        issue:        this.issue || 0,
        pr:           this.pr || 0,
        branch:       this.branch.trim(),
        dry_run:      this.dryRun,
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
            ${(this.issue > 0 || this.pr > 0) ? html`
              <div style="display:flex; align-items:center; gap:8px; margin-top:7px;">
                <span style="font-family:var(--font-mono); font-size:11px; padding:2px 9px; border-radius:999px;
                             background:rgba(64,193,197,0.1); border:1px solid rgba(64,193,197,0.25); color:var(--brand-300);">
                  <i class="fa-solid ${this.issue > 0 ? "fa-circle-exclamation" : "fa-code-pull-request"}" style="font-size:9px;"></i>
                  ${this.issue > 0 ? `issue #${this.issue}` : `PR #${this.pr}`}
                </span>
                <span @click=${() => { this.issue = 0; this.pr = 0; }} title="Clear"
                      style="cursor:pointer; font-size:10.5px; color:var(--fg-3); --wails-draggable:no-drag;">✕ clear</span>
              </div>
            ` : nothing}
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
            <lthn-label>Persona (optional)</lthn-label>
            ${this.personas.length > 0 ? html`
              <select style=${fieldStyle}
                .value=${this.persona}
                @change=${(e: Event) => { this.persona = (e.target as HTMLSelectElement).value; }}>
                <option value="">— no persona —</option>
                ${this._personaGroups().map(([cat, cards]) => html`
                  <optgroup label=${cat}>
                    ${cards.map(p => html`<option value=${p.path}>${p.emoji} ${p.name}</option>`)}
                  </optgroup>
                `)}
              </select>
              ${this._selectedVibe() ? html`
                <div style="margin-top:5px; font-size:10.5px; color:var(--fg-3); font-style:italic;">
                  ${this._selectedVibe()}
                </div>
              ` : nothing}
            ` : nothing}
          </div>

          <div>
            <lthn-label>Premade task (optional)</lthn-label>
            ${this.tasks.length > 0 ? html`
              <select style=${fieldStyle}
                .value=${this.planTemplate}
                @change=${(e: Event) => { this._pickTask((e.target as HTMLSelectElement).value); }}>
                <option value="">— custom (write your own below) —</option>
                ${this._taskGroups().map(([cat, cards]) => html`
                  <optgroup label=${cat}>
                    ${cards.map(t => html`<option value=${t.slug}>${t.name}</option>`)}
                  </optgroup>
                `)}
              </select>
              <div style="margin-top:5px; font-size:10.5px; color:var(--fg-3);">
                Attaches a structured plan and fills the task below — edit it freely.
              </div>
            ` : nothing}
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
