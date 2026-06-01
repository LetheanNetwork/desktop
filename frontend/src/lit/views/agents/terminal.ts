// SPDX-Licence-Identifier: EUPL-1.2
// Agents view · Terminal — <lthn-view-terminal>
//
// Tab manager for the in-app terminal. Hosts N <lthn-terminal-session> children
// (one PTY-backed xterm each, see terminal-session.ts) and shows the active one.
// Chrome + tab strip live here; each session owns its xterm, addons, and search.
//
// Tabs come from three places:
//   - the initial tab on mount (seeded with repo/cwd by the shell, else $HOME)
//   - "+" new tab (a fresh $HOME shell)
//   - a `lthn:open-terminal` window event — "open terminal here" from a repo row,
//     same shell hand-off shape as lthn:open-code. The detail may carry an
//     attachId instead of a repo/cwd, which surfaces an EXISTING pool session as
//     a tab (the hook for watching a running agent's terminal).
//
// Sessions persist while this pane is mounted; switching views re-creates the
// pane (the shell doesn't keep panes warm across view switches), so tabs reset
// on leaving the Terminal view — a pane-keepalive is a separate shell change.

import { LitElement, html, nothing } from "lit";
import { repeat } from "lit/directives/repeat.js";
import { renderChrome } from "../../chrome";
import "./terminal-session";

interface Tab {
  key: string;
  repo?: string;
  cwd?: string;
  attachId?: string;
  title: string;
  exited?: boolean;
}

class LthnViewTerminal extends LitElement {
  static readonly properties = {
    w:        { type: Number },
    h:        { type: Number },
    embedded: { type: Boolean, reflect: true },
    repo:     { type: String }, // cold seed for the initial tab (set by the shell)
    cwd:      { type: String },
    tabs:      { state: true },
    activeKey: { state: true },
  };

  declare w: number;
  declare h: number;
  declare embedded: boolean;
  declare repo: string;
  declare cwd: string;
  declare tabs: Tab[];
  declare activeKey: string;

  private seq = 0;

  constructor() {
    super();
    this.w = 1180; this.h = 720; this.embedded = false;
    this.repo = ""; this.cwd = "";
    this.tabs = []; this.activeKey = "";
  }

  createRenderRoot() { return this; }

  connectedCallback() {
    super.connectedCallback();
    // Grow as a flex item in the shell's pane row — without this the manager
    // shrink-wraps to its tab strip (~370px). (The session children carry the
    // same on their own host.)
    this.style.flex = "1";
    this.style.minWidth = "0";
    window.addEventListener("lthn:open-terminal", this._onOpenTerminal as EventListener);
    if (this.tabs.length === 0) {
      this._addTab(this.repo || this.cwd ? { repo: this.repo, cwd: this.cwd } : {});
    }
  }

  disconnectedCallback() {
    super.disconnectedCallback();
    window.removeEventListener("lthn:open-terminal", this._onOpenTerminal as EventListener);
  }

  /** "open terminal here" — a repo row (or any caller) asks for a terminal in a
   *  cwd, or to attach to an existing session. Same hand-off as lthn:open-code. */
  _onOpenTerminal = (ev: Event) => {
    const d = (ev as CustomEvent<{ repo?: string; cwd?: string; path?: string; attachId?: string }>).detail || {};
    this._addTab({ repo: d.repo, cwd: d.cwd || d.path, attachId: d.attachId });
  };

  private _addTab(opts: { repo?: string; cwd?: string; attachId?: string }) {
    const key = "t" + (++this.seq);
    const title = opts.repo
      ? opts.repo
      : opts.attachId
        ? "↪ " + opts.attachId.slice(0, 8)
        : opts.cwd
          ? (opts.cwd.split("/").filter(Boolean).pop() || "shell")
          : "shell " + this.seq;
    this.tabs = [...this.tabs, { key, repo: opts.repo, cwd: opts.cwd, attachId: opts.attachId, title }];
    this.activeKey = key;
  }

  private _newTab() { this._addTab({}); }

  private _selectTab(key: string) { this.activeKey = key; }

  private _closeTab(key: string, e?: Event) {
    e?.stopPropagation();
    const idx = this.tabs.findIndex(t => t.key === key);
    const remaining = this.tabs.filter(t => t.key !== key);
    if (this.activeKey === key) {
      const next = remaining[Math.max(0, idx - 1)];
      this.activeKey = next ? next.key : "";
    }
    this.tabs = remaining;
    if (this.tabs.length === 0) this._newTab(); // never leave the pane empty
  }

  /** A session reported its resolved cwd/shell — label the tab (unless it was
   *  repo-seeded, where the repo name is the better label). */
  private _onReady = (ev: Event) => {
    const d = (ev as CustomEvent<{ key: string; title?: string; cwd?: string }>).detail;
    if (!d?.key) return;
    this.tabs = this.tabs.map(t =>
      t.key === d.key ? { ...t, title: t.repo || d.title || t.title, cwd: d.cwd || t.cwd } : t);
  };

  /** The shell exited — mark the tab so the chip shows it's dead (kept so the
   *  final output is readable; the user closes it). */
  private _onExit = (ev: Event) => {
    const d = (ev as CustomEvent<{ key: string }>).detail;
    if (!d?.key) return;
    this.tabs = this.tabs.map(t => t.key === d.key ? { ...t, exited: true } : t);
  };

  private _tabStrip() {
    return html`
      <div style="display:flex; align-items:center; gap:4px; padding:8px 12px; flex-shrink:0;
                  border-bottom:1px solid rgba(255,255,255,0.05); overflow-x:auto;">
        ${repeat(this.tabs, t => t.key, t => {
          const on = t.key === this.activeKey;
          return html`
            <div @click=${() => this._selectTab(t.key)} title=${t.cwd || t.title}
                 style="display:flex; align-items:center; gap:7px; padding:5px 10px; border-radius:7px; cursor:pointer;
                        font-size:11.5px; white-space:nowrap; --wails-draggable:no-drag;
                        background:${on ? "rgba(64,193,197,0.12)" : "rgba(255,255,255,0.03)"};
                        border:1px solid ${on ? "rgba(64,193,197,0.30)" : "rgba(255,255,255,0.06)"};
                        color:${on ? "var(--brand-200)" : "var(--fg-2)"}; opacity:${t.exited ? 0.55 : 1};">
              <i class="fa-solid ${t.attachId ? "fa-link" : "fa-terminal"}" style="font-size:9px; opacity:0.6;"></i>
              <span style="max-width:160px; overflow:hidden; text-overflow:ellipsis;">${t.title}${t.exited ? " ✗" : ""}</span>
              <i class="fa-solid fa-xmark" @click=${(e: Event) => this._closeTab(t.key, e)}
                 style="font-size:9px; opacity:0.5; padding:1px 2px;"></i>
            </div>`;
        })}
        <div @click=${() => this._newTab()} title="New terminal ($HOME)"
             style="padding:5px 9px; border-radius:7px; cursor:pointer; color:var(--fg-3);
                    border:1px dashed rgba(255,255,255,0.12); --wails-draggable:no-drag; flex-shrink:0;">
          <i class="fa-solid fa-plus" style="font-size:10px;"></i>
        </div>
      </div>
    `;
  }

  render() {
    const at = this.tabs.find(t => t.key === this.activeKey);
    const subtitle = at ? (at.cwd || at.title || "shell") : "PTY shell — your machine, in the app";

    const body = html`
      <div style="flex:1; width:100%; min-width:0; display:flex; flex-direction:column; min-height:0; overflow:hidden;">
        ${this._tabStrip()}
        <div style="flex:1; min-height:0; min-width:0; position:relative;">
          ${repeat(this.tabs, t => t.key, t => html`
            <div style="position:absolute; inset:0; display:${t.key === this.activeKey ? "flex" : "none"};">
              <lthn-terminal-session
                .tabKey=${t.key} .repo=${t.repo || ""} .cwd=${t.cwd || ""} .attachId=${t.attachId || ""}
                .active=${t.key === this.activeKey}
                @lthn-term-ready=${this._onReady} @lthn-term-exit=${this._onExit}></lthn-terminal-session>
            </div>`)}
        </div>
      </div>
    `;

    return renderChrome({
      title: "Terminal",
      subtitle,
      w: this.w, h: this.h,
      toolbar: nothing,
      body,
      footer: html`pkg/terminal · ${this.tabs.length} tab${this.tabs.length === 1 ? "" : "s"} · ⌘F search · bytes over the event bus`,
      embedded: this.embedded,
    });
  }
}

customElements.define("lthn-view-terminal", LthnViewTerminal);
