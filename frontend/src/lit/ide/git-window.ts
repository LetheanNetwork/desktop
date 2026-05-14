// SPDX-Licence-Identifier: EUPL-1.2
// IDE · git — <lthn-git-window>
//
// Source-control surface against any local repo. Surfaces:
//   - branch + ahead/behind
//   - status entries (staged / unstaged / untracked)
//   - per-file stage / unstage / open diff in editor
//   - commit composer
//   - recent log
//
// Binds directly to @desktop/git/service (the Wails surface
// exposed by pkg/git). Open via windowservice.Open("git") with
// the workspace root in ?path=… (defaults to the user's home
// dir for the demo state).

import { LitElement, html, nothing } from "lit";
import { renderChrome } from "../chrome";
import { T } from "@lthn/i18n/coreservice";

interface StatusEntry {
  path: string;
  index_status: string;
  worktree_status: string;
  staged: boolean;
  unstaged: boolean;
  untracked: boolean;
}

interface BranchInfo {
  branch: string;
  ahead: number;
  behind: number;
}

interface LogEntry {
  hash: string;
  short_sha: string;
  author: string;
  date: string;
  subject: string;
}

class LthnGitWindow extends LitElement {
  static readonly properties = {
    w:        { type: Number },
    h:        { type: Number },
    embedded: { type: Boolean, reflect: true },
    path:     { type: String },
    chrome:   { state: true },
    branch:   { state: true },
    entries:  { state: true },
    log:      { state: true },
    diff:     { state: true },
    selected: { state: true },
    message:  { state: true },
    busy:     { state: true },
    err:      { state: true },
  };
  declare w: number;
  declare h: number;
  declare embedded: boolean;
  declare path: string;
  declare chrome: { title: string; subtitle: string };
  declare branch: BranchInfo;
  declare entries: StatusEntry[];
  declare log: LogEntry[];
  declare diff: string;
  declare selected: string;
  declare message: string;
  declare busy: boolean;
  declare err: string;

  constructor() {
    super();
    this.w = 1120; this.h = 740; this.embedded = false;
    this.path = "";
    this.chrome = { title: "Git", subtitle: "branch · status · diff · commit" };
    this.branch = { branch: "—", ahead: 0, behind: 0 };
    this.entries = [];
    this.log = [];
    this.diff = "";
    this.selected = "";
    this.message = "";
    this.busy = false;
    this.err = "";
  }
  createRenderRoot() { return this; }

  async connectedCallback() {
    super.connectedCallback();
    const [title, subtitle] = await Promise.all([
      T("window.git.title"),
      T("window.git.subtitle"),
    ]);
    this.chrome = { title, subtitle };
    await this._reload();
  }

  async _reload() {
    if (!this.path) return;
    this.busy = true;
    this.err = "";
    try {
      const svc = await import("@desktop/git/service");
      const [b, st, lg] = await Promise.all([
        svc.Branch(this.path).catch(() => ({ branch: "—", ahead: 0, behind: 0 })),
        svc.Status(this.path).catch((): StatusEntry[] => []),
        svc.Log(this.path, 30).catch((): LogEntry[] => []),
      ]);
      this.branch = b as BranchInfo;
      this.entries = (st || []) as StatusEntry[];
      this.log = (lg || []) as LogEntry[];
      if (this.selected && !this.entries.some(e => e.path === this.selected)) {
        this.selected = "";
        this.diff = "";
      }
    } catch (e: unknown) {
      this.err = e instanceof Error ? e.message : String(e);
    } finally {
      this.busy = false;
    }
  }

  async _viewDiff(entry: StatusEntry) {
    this.selected = entry.path;
    try {
      const svc = await import("@desktop/git/service");
      this.diff = await svc.Diff(this.path, entry.path, entry.staged);
    } catch (e: unknown) {
      this.err = e instanceof Error ? e.message : String(e);
      this.diff = "";
    }
  }

  async _stage(entry: StatusEntry) {
    try {
      const svc = await import("@desktop/git/service");
      await svc.Add(this.path, [entry.path], false);
      await this._reload();
    } catch (e: unknown) {
      this.err = e instanceof Error ? e.message : String(e);
    }
  }

  async _unstage(entry: StatusEntry) {
    try {
      const svc = await import("@desktop/git/service");
      await svc.Unstage(this.path, [entry.path]);
      await this._reload();
    } catch (e: unknown) {
      this.err = e instanceof Error ? e.message : String(e);
    }
  }

  async _stageAll() {
    try {
      const svc = await import("@desktop/git/service");
      await svc.Add(this.path, [], true);
      await this._reload();
    } catch (e: unknown) {
      this.err = e instanceof Error ? e.message : String(e);
    }
  }

  async _commit() {
    const msg = this.message.trim();
    if (!msg) return;
    try {
      const svc = await import("@desktop/git/service");
      await svc.Commit(this.path, msg);
      this.message = "";
      await this._reload();
    } catch (e: unknown) {
      this.err = e instanceof Error ? e.message : String(e);
    }
  }

  _statusGlyph(e: StatusEntry): string {
    if (e.untracked) return "?";
    if (e.staged && !e.unstaged) return "✓";
    if (e.staged && e.unstaged) return "±";
    if (e.unstaged) return "·";
    return " ";
  }

  _statusTone(e: StatusEntry): string {
    if (e.untracked) return "var(--fg-3)";
    if (e.staged && !e.unstaged) return "var(--success-400)";
    if (e.staged && e.unstaged) return "var(--warning-400)";
    if (e.unstaged) return "var(--brand-300)";
    return "var(--fg-3)";
  }

  render() {
    const staged   = this.entries.filter(e => e.staged && !e.unstaged);
    const mixed    = this.entries.filter(e => e.staged && e.unstaged);
    const unstaged = this.entries.filter(e => !e.staged && e.unstaged);
    const untracked = this.entries.filter(e => e.untracked);

    const toolbar = html`
      <span style="font-family:var(--font-mono); font-size:11px; color:var(--fg-1);
                   padding:4px 9px; background:rgba(255,255,255,0.04);
                   border:1px solid rgba(255,255,255,0.07); border-radius:6px;">
        ${this.branch.branch}
        ${this.branch.ahead > 0 ? html`<span style="color:var(--brand-300); margin-left:6px;">↑${this.branch.ahead}</span>` : nothing}
        ${this.branch.behind > 0 ? html`<span style="color:var(--warning-400); margin-left:6px;">↓${this.branch.behind}</span>` : nothing}
      </span>
      <div style="flex:1"></div>
      <lthn-btn tone="ghost" size="sm" @click=${() => void this._reload()}>
        <i class="fa-solid fa-arrows-rotate" style="font-size:10px;"></i> Refresh
      </lthn-btn>
      <lthn-btn tone="ghost" size="sm" @click=${() => void this._stageAll()}>
        <i class="fa-solid fa-plus" style="font-size:10px;"></i> Stage all
      </lthn-btn>
    `;

    const groupHTML = (label: string, items: StatusEntry[], onAction: (e: StatusEntry) => void, actionGlyph: string) => {
      if (items.length === 0) return nothing;
      return html`
        <lthn-label style="display:block; padding:10px 12px 4px;">${label} · ${items.length}</lthn-label>
        <div style="display:flex; flex-direction:column;">
          ${items.map(e => html`
            <div @click=${() => void this._viewDiff(e)}
              style="display:grid; grid-template-columns:18px 1fr 24px;
                     gap:8px; padding:6px 12px; align-items:center; cursor:pointer;
                     background:${e.path === this.selected ? "rgba(255,255,255,0.06)" : "transparent"};
                     --wails-draggable: no-drag;">
              <span style="font-family:var(--font-mono); font-size:11px; color:${this._statusTone(e)}; text-align:center;">${this._statusGlyph(e)}</span>
              <span style="font-family:var(--font-mono); font-size:11px; color:var(--fg-1);
                           white-space:nowrap; overflow:hidden; text-overflow:ellipsis;">${e.path}</span>
              <lthn-btn tone="quiet" size="sm" @click=${(ev: Event) => { ev.stopPropagation(); onAction(e); }}>
                <i class="fa-solid ${actionGlyph}" style="font-size:9px;"></i>
              </lthn-btn>
            </div>
          `)}
        </div>
      `;
    };

    const railContent = this.entries.length === 0 ? html`
      <div style="padding:18px 14px; font-size:11.5px; color:var(--fg-3); line-height:1.55;">
        ${this.err ? html`<span style="color:var(--error-400);">${this.err}</span>` : "Working tree clean."}
      </div>
    ` : html`
      ${groupHTML("Staged",    staged,    e => void this._unstage(e), "fa-minus")}
      ${groupHTML("Mixed",     mixed,     e => void this._stage(e),   "fa-plus")}
      ${groupHTML("Unstaged",  unstaged,  e => void this._stage(e),   "fa-plus")}
      ${groupHTML("Untracked", untracked, e => void this._stage(e),   "fa-plus")}
    `;

    const body = html`
      <div style="flex:1; display:grid; grid-template-columns:300px 1fr 300px; min-height:0;">
        <aside style="background:rgba(0,0,0,0.18); border-right:1px solid rgba(255,255,255,0.05);
                      display:flex; flex-direction:column; min-height:0; overflow:auto;">
          ${railContent}
        </aside>

        <main style="display:flex; flex-direction:column; min-height:0;">
          <div style="padding:10px 14px; border-bottom:1px solid rgba(255,255,255,0.05);
                      font-family:var(--font-mono); font-size:11px; color:var(--fg-3);">
            ${this.selected || "select a file to view diff"}
          </div>
          <pre style="flex:1; margin:0; padding:12px 14px; overflow:auto;
                      font-family:var(--font-mono); font-size:11px; line-height:1.55;
                      color:var(--fg-1); white-space:pre;">${this.diff || (this.selected ? "(no changes)" : "")}</pre>
          <div style="padding:12px 14px; border-top:1px solid rgba(255,255,255,0.05);
                      display:flex; flex-direction:column; gap:8px;">
            <textarea
              .value=${this.message}
              @input=${(e: Event) => { this.message = (e.target as HTMLTextAreaElement).value; }}
              placeholder="Commit message · ⌘↵ to commit"
              @keydown=${(e: KeyboardEvent) => { if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) { e.preventDefault(); void this._commit(); } }}
              style="background:rgba(255,255,255,0.04); border:1px solid rgba(255,255,255,0.07);
                     border-radius:6px; padding:8px 10px; font-family:var(--font-sans);
                     font-size:12px; color:var(--fg-0); resize:vertical; min-height:48px;
                     --wails-draggable: no-drag;"></textarea>
            <div style="display:flex; align-items:center; gap:6px;">
              <div style="flex:1"></div>
              <lthn-btn tone="primary" size="sm" ?dim=${!this.message.trim() || staged.length + mixed.length === 0}
                @click=${() => void this._commit()}>
                <i class="fa-solid fa-check" style="font-size:10px;"></i> Commit
              </lthn-btn>
            </div>
          </div>
        </main>

        <aside style="background:rgba(0,0,0,0.18); border-left:1px solid rgba(255,255,255,0.05);
                      padding:12px 14px; overflow:auto; display:flex; flex-direction:column; gap:6px;">
          <lthn-label>Recent log · ${this.log.length}</lthn-label>
          ${this.log.length === 0 ? html`
            <div style="font-size:11px; color:var(--fg-3); font-style:italic;">no commits yet</div>
          ` : this.log.map(c => html`
            <div style="display:flex; flex-direction:column; gap:2px; padding:6px 4px;
                        border-bottom:1px solid rgba(255,255,255,0.04);">
              <div style="display:flex; align-items:baseline; gap:6px;">
                <span style="font-family:var(--font-mono); font-size:10px; color:var(--brand-300);">${c.short_sha}</span>
                <span style="font-size:10px; color:var(--fg-3);">${c.date}</span>
              </div>
              <div style="font-size:11.5px; color:var(--fg-1); line-height:1.45;">${c.subject}</div>
              <div style="font-size:10px; color:var(--fg-3);">${c.author}</div>
            </div>
          `)}
        </aside>
      </div>
    `;

    return renderChrome({
      title: this.chrome.title, subtitle: this.chrome.subtitle,
      w: this.w, h: this.h, toolbar, body,
      footer: html`${this.path || "no workspace · pass ?path=…"} · ${this.entries.length} changes · ${this.branch.branch}`,
      embedded: this.embedded,
    });
  }
}
customElements.define("lthn-git-window", LthnGitWindow);
