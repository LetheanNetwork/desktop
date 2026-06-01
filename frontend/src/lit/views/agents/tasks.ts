// SPDX-Licence-Identifier: EUPL-1.2
// Agents view · Backlog — <lthn-view-agent-tasks>
//
// pkg/tasks IS the job queue; this surfaces it in the agentic view. Shows the
// open + in-progress backlog, counted by source (lint / package-update /
// manual), each task dispatchable: a row click fires lthn:dispatch-repo seeded
// with the task's repo + summary, so Backlog → Dispatch closes the loop.
//
// Reads Tasks.List (@desktop/tasks/wailsservice). Supports the `embedded`
// attribute — no chrome when set.

import { LitElement, html, nothing } from "lit";
import { renderChrome } from "../../chrome";

/** A queue task (mirrors the tasks.Issue JSON — capitalised Go field names). */
interface Task {
  ID:        string;
  Project:   string;
  Summary:   string;
  State:     string;
  Severity:  string;
  Reporter:  string;
}

const OPEN_STATES = ["open", "in_progress"];

class LthnViewAgentTasks extends LitElement {
  static readonly properties = {
    w:        { type: Number },
    h:        { type: Number },
    embedded: { type: Boolean, reflect: true },
    tasks:    { state: true },
    loading:  { state: true },
    err:      { state: true },
  };

  declare w: number;
  declare h: number;
  declare embedded: boolean;
  declare tasks: Task[];
  declare loading: boolean;
  declare err: string;

  constructor() {
    super();
    this.w = 1180; this.h = 720; this.embedded = false;
    this.tasks = []; this.loading = false; this.err = "";
  }

  createRenderRoot() { return this; }

  async connectedCallback() {
    super.connectedCallback();
    await this._load();
  }

  /** Load the open backlog from the task queue (Tasks.List). */
  async _load() {
    this.loading = true;
    this.err = "";
    try {
      const svc = await import("@desktop/tasks/wailsservice").catch(() => null);
      const list = svc && (svc as { List?: (i: unknown) => Promise<{ Value?: { issues?: Task[] } }> }).List;
      if (!list) { this.err = "task queue unavailable"; this.tasks = []; return; }
      const issues = (await list({}))?.Value?.issues;
      this.tasks = Array.isArray(issues) ? issues.filter(t => OPEN_STATES.includes(t.State)) : [];
    } catch (e: unknown) {
      this.err = e instanceof Error ? e.message : String(e);
      this.tasks = [];
    } finally {
      this.loading = false;
    }
  }

  /** Clear all detector-filed tasks (lint + package-update). They regenerate
   *  on the next Scan, so this is a cheap reset of the machine-filed backlog;
   *  human-authored tasks are untouched. */
  async _clear() {
    if (this.loading) return;
    this.loading = true;
    try {
      const svc = await import("@desktop/tasks/wailsservice").catch(() => null);
      const clear = svc && (svc as { ClearDetected?: (i: unknown) => Promise<unknown> }).ClearDetected;
      if (clear) await clear({});
    } catch { /* surfaced by the reload below */ }
    finally { this.loading = false; }
    await this._load();
  }

  /** Counts by reporter source for the header summary. */
  _bySource(): { lint: number; updates: number; other: number } {
    const c = { lint: 0, updates: 0, other: 0 };
    for (const t of this.tasks) {
      if (t.Reporter === "core-lint") c.lint++;
      else if (t.Reporter === "package-update") c.updates++;
      else c.other++;
    }
    return c;
  }

  /** Dispatch an agent on this task — open Dispatch seeded with its repo + summary. */
  _dispatch(t: Task) {
    window.dispatchEvent(new CustomEvent("lthn:dispatch-repo", {
      detail: { repo: t.Project, task: t.Summary },
    }));
  }

  render() {
    const src = this._bySource();
    const body = html`
      <div style="flex:1; display:flex; flex-direction:column; min-height:0;">
        <div style="padding:18px 22px 12px; display:flex; align-items:center; gap:12px;">
          <h2 style="margin:0; font-size:22px; color:var(--fg-0); letter-spacing:-0.02em; font-weight:600;">Backlog</h2>
          <span style="font-family:var(--font-mono); font-size:11px; color:var(--fg-3);">
            ${this.tasks.length} open · 🔍 ${src.lint} lint · 📦 ${src.updates} deps · ✍️ ${src.other} other
          </span>
          <div style="flex:1"></div>
          <lthn-btn tone="ghost" size="sm" @click=${() => void this._load()}>
            <i class="fa-solid ${this.loading ? "fa-spinner" : "fa-rotate"}" style="font-size:10px;"></i> Refresh
          </lthn-btn>
          <lthn-btn tone="ghost" size="sm" @click=${() => void this._clear()}
                    title="Delete all lint + dependency tasks — they regenerate on the next Scan">
            <i class="fa-solid fa-broom" style="font-size:10px;"></i> Clear
          </lthn-btn>
        </div>
        ${this.err ? html`<div style="padding:6px 22px 10px; color:var(--err-400); font-size:11.5px;">${this.err}</div>` : nothing}
        <div style="flex:1; overflow:auto; padding:0 22px 18px;">
          ${this.tasks.length === 0 ? html`
            <div style="padding:40px; text-align:center; color:var(--fg-3); font-size:12px;">
              ${this.loading ? "Loading the backlog…" : "Backlog clear — no open tasks. Scan a repo in Repos to file some."}
            </div>
          ` : html`
            <div style="background:rgba(255,255,255,0.025); border:1px solid rgba(255,255,255,0.06); border-radius:10px; overflow:hidden;">
              ${this.tasks.map((t, i) => html`
                <div title="Dispatch an agent on this task"
                     @click=${() => this._dispatch(t)}
                     style="display:grid; grid-template-columns: 1fr 170px 110px 90px; gap:14px;
                            padding:13px 16px; cursor:pointer; align-items:center;
                            border-bottom:${i < this.tasks.length - 1 ? "1px solid rgba(255,255,255,0.04)" : "none"};">
                  <span style="font-size:12.5px; color:var(--fg-0); overflow:hidden; text-overflow:ellipsis; white-space:nowrap;">
                    ${t.Summary} <i class="fa-solid fa-paper-plane" style="font-size:8px; opacity:0.5;"></i>
                  </span>
                  <span style="font-family:var(--font-mono); font-size:10.5px; color:var(--fg-3);
                               overflow:hidden; text-overflow:ellipsis; white-space:nowrap;">${t.Project}</span>
                  <span style="font-family:var(--font-mono); font-size:10px; color:var(--fg-3);">${t.Reporter || "—"}</span>
                  <span style="font-family:var(--font-mono); font-size:10px; color:var(--fg-2); text-align:right;">${t.State}</span>
                </div>
              `)}
            </div>
          `}
        </div>
      </div>
    `;

    return renderChrome({
      title: "Backlog",
      subtitle: "the task queue — scan repos to fill it, click a task to dispatch",
      w: this.w, h: this.h,
      toolbar: nothing,
      body,
      footer: html`pkg/tasks job queue · open + in-progress · click a row → Dispatch`,
      embedded: this.embedded,
    });
  }
}

customElements.define("lthn-view-agent-tasks", LthnViewAgentTasks);
