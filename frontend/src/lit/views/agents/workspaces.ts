// SPDX-Licence-Identifier: EUPL-1.2
// Agents view · Workspaces — <lthn-view-agent-workspaces>
//
// Prepare an agent workspace and inspect what the agent will get. Prep is a
// rich pipeline (clone/resume → branch → assemble the prompt from the issue
// body, brain-recalled memories, downstream-consumer impact, wiki, and git
// log → versioned snapshot); this panel drives it via Agents.Prep() →
// `lthn-agent prep --json` and surfaces the result — the assembled prompt plus
// counts of memories recalled and consumers found — so you can see (and tune)
// the context before dispatching an agent into it.
//
// Backend: Agents.Prep() from @desktop/agents/service — imported dynamically so
// tests run without the Wails runtime. Prep clones + assembles synchronously
// (can take a while); a clear error shows when lthn-agent isn't reachable.
//
// Supports the `embedded` attribute — no chrome when set.

import { LitElement, html, nothing } from "lit";
import { renderChrome } from "../../chrome";
import { demand } from "../../result";

/** PrepResult mirrors agents.PrepResult. */
interface PrepResult {
  success:        boolean;
  workspace_dir:  string;
  repo_dir:       string;
  branch:         string;
  prompt?:        string;
  prompt_version?: string;
  memories:       number;
  consumers:      number;
  resumed:        boolean;
}

class LthnViewAgentWorkspaces extends LitElement {
  static readonly properties = {
    w:        { type: Number },
    h:        { type: Number },
    embedded: { type: Boolean, reflect: true },
    repo:     { state: true },
    org:      { state: true },
    task:     { state: true },
    issue:    { state: true },
    branch:   { state: true },
    template: { state: true },
    persona:  { state: true },
    busy:     { state: true },
    result:   { state: true },
    err:      { state: true },
  };

  declare w: number;
  declare h: number;
  declare embedded: boolean;
  declare repo: string;
  declare org: string;
  declare task: string;
  declare issue: string;
  declare branch: string;
  declare template: string;
  declare persona: string;
  declare busy: boolean;
  declare result: PrepResult | null;
  declare err: string;

  constructor() {
    super();
    this.w = 1180; this.h = 720; this.embedded = false;
    this.repo = "";
    this.org = "core";
    this.task = "";
    this.issue = "";
    this.branch = "";
    this.template = "";
    this.persona = "";
    this.busy = false;
    this.result = null;
    this.err = "";
  }

  createRenderRoot() { return this; }

  async _prep() {
    if (this.busy) return;
    if (!this.repo.trim()) { this.err = "Repo is required."; return; }
    this.busy = true;
    this.err = "";
    this.result = null;
    try {
      const svc = await import("@desktop/agents/service");
      const req = {
        repo:     this.repo.trim(),
        org:      this.org.trim(),
        task:     this.task.trim(),
        issue:    Number(this.issue.trim()) || 0,
        branch:   this.branch.trim(),
        template: this.template.trim(),
        persona:  this.persona.trim(),
      };
      this.result = await demand<PrepResult>(
        (svc as { Prep: (req: unknown) => Promise<{ OK: boolean; Value: unknown }> }).Prep(req),
      );
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
    const r = this.result;

    const body = html`
      <div style="flex:1; padding:18px 22px 22px; overflow:auto; max-width:760px;">
        <div style="display:flex; flex-direction:column; gap:16px;">
          <div style="display:flex; gap:14px;">
            <div style="flex:1;">
              <lthn-label>Repo</lthn-label>
              <input style=${fieldStyle} placeholder="e.g. go-io" .value=${this.repo}
                @input=${(e: Event) => { this.repo = (e.target as HTMLInputElement).value; }}>
            </div>
            <div style="width:140px;">
              <lthn-label>Org</lthn-label>
              <input style=${fieldStyle} placeholder="core" .value=${this.org}
                @input=${(e: Event) => { this.org = (e.target as HTMLInputElement).value; }}>
            </div>
          </div>

          <div>
            <lthn-label>Task (optional)</lthn-label>
            <textarea style="${fieldStyle} min-height:76px; resize:vertical; line-height:1.5;"
              placeholder="What should the agent do? (refines the assembled prompt)"
              .value=${this.task}
              @input=${(e: Event) => { this.task = (e.target as HTMLTextAreaElement).value; }}></textarea>
          </div>

          <div style="display:flex; gap:14px;">
            <div style="width:120px;">
              <lthn-label>Issue #</lthn-label>
              <input style=${fieldStyle} placeholder="optional" .value=${this.issue}
                @input=${(e: Event) => { this.issue = (e.target as HTMLInputElement).value; }}>
            </div>
            <div style="flex:1;">
              <lthn-label>Branch (optional)</lthn-label>
              <input style=${fieldStyle} placeholder="defaults to a task branch" .value=${this.branch}
                @input=${(e: Event) => { this.branch = (e.target as HTMLInputElement).value; }}>
            </div>
          </div>

          <div style="display:flex; gap:14px;">
            <div style="flex:1;">
              <lthn-label>Template (optional)</lthn-label>
              <input style=${fieldStyle} placeholder="coding" .value=${this.template}
                @input=${(e: Event) => { this.template = (e.target as HTMLInputElement).value; }}>
            </div>
            <div style="flex:1;">
              <lthn-label>Persona (optional)</lthn-label>
              <input style=${fieldStyle} placeholder="e.g. code/reviewer" .value=${this.persona}
                @input=${(e: Event) => { this.persona = (e.target as HTMLInputElement).value; }}>
            </div>
          </div>

          <div style="display:flex; align-items:center; gap:12px; margin-top:2px;">
            <lthn-btn tone="primary" ?dim=${this.busy} @click=${() => void this._prep()}>
              <i class="fa-solid ${this.busy ? "fa-spinner" : "fa-layer-group"}" style="font-size:11px;"></i>
              ${this.busy ? "Preparing…" : "Prepare workspace"}
            </lthn-btn>
            <span style="font-size:10.5px; color:var(--fg-3);">clones + assembles the prompt (issue · brain · consumers · wiki)</span>
          </div>

          ${this.err ? html`
            <div style="padding:10px 14px; color:var(--err-400);
                        background:rgba(255,76,76,0.06); border:1px solid rgba(255,76,76,0.18);
                        border-radius:6px; font-size:12px; line-height:1.55;">
              ${this.err}
            </div>
          ` : nothing}

          ${r ? html`
            <div style="display:flex; gap:8px; flex-wrap:wrap; margin-top:2px;">
              ${this._pill("workspace", r.workspace_dir.split("/").slice(-3).join("/") || r.workspace_dir)}
              ${this._pill("branch", r.branch || "—")}
              ${this._pill("memories", String(r.memories))}
              ${this._pill("consumers", String(r.consumers))}
              ${r.prompt_version ? this._pill("version", r.prompt_version) : nothing}
              ${r.resumed ? this._pill("resumed", "yes") : nothing}
            </div>
            ${r.prompt ? html`
              <div>
                <div style="font-size:11px; color:var(--fg-3); margin-bottom:6px;">
                  assembled prompt — ${r.prompt.length} chars (what the agent gets)
                </div>
                <pre style="margin:0; max-height:340px; overflow:auto; padding:14px 16px;
                            background:rgba(0,0,0,0.28); border:1px solid rgba(255,255,255,0.06);
                            border-radius:10px; font-family:var(--font-mono); font-size:11px;
                            line-height:1.55; color:var(--fg-1); white-space:pre-wrap; word-break:break-word;">${r.prompt}</pre>
              </div>
            ` : nothing}
          ` : nothing}
        </div>
      </div>
    `;

    return renderChrome({
      title: "Workspaces",
      subtitle: r ? `prepared ${r.branch || "workspace"}` : "prepare a workspace",
      w: this.w, h: this.h,
      toolbar: nothing,
      body,
      footer: html`clone/resume + assemble prompt → agentic_prep_workspace · data via Agents.Prep() (lthn-agent CLI)`,
      embedded: this.embedded,
    });
  }

  /** A labelled result pill. */
  _pill(label: string, value: string) {
    return html`
      <span style="display:inline-flex; align-items:center; gap:6px; padding:4px 10px;
                   border-radius:6px; background:rgba(64,193,197,0.06);
                   border:1px solid rgba(64,193,197,0.2);
                   font-family:var(--font-mono); font-size:11px;">
        <span style="color:var(--fg-3);">${label}</span>
        <span style="color:var(--fg-0);">${value}</span>
      </span>
    `;
  }
}

customElements.define("lthn-view-agent-workspaces", LthnViewAgentWorkspaces);
