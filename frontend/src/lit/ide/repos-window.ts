// SPDX-Licence-Identifier: EUPL-1.2
// IDE · repos — <lthn-repos-window>
//
// Multi-repo dashboard. Aggregates git status across the user's
// canonical Code/* trees and surfaces a grid of cards keyed by
// branch + ahead/behind/dirty counts.
//
// Filter chips: all / dirty / ahead / behind. Click a card to
// open the git surface scoped to that repo (windowservice.Open
// would dispatch but we just route via ?surface=git&path= here).
//
// Open via windowservice.Open("repos"). No ?path= needed — the
// scan walks $HOME/Code/{core,lthn,host-uk,lab,snider}.

import { LitElement, html, nothing } from "lit";
import { renderChrome } from "../chrome";
import { T } from "@lthn/i18n/coreservice";

interface RepoStatus {
  name: string;
  path: string;
  branch: string;
  modified: number;
  untracked: number;
  staged: number;
  ahead: number;
  behind: number;
  dirty: boolean;
  error?: string;
}

type Filter = "all" | "dirty" | "ahead" | "behind";

class LthnReposWindow extends LitElement {
  static readonly properties = {
    w:        { type: Number },
    h:        { type: Number },
    embedded: { type: Boolean, reflect: true },
    chrome:   { state: true },
    repos:    { state: true },
    scanned:  { state: true },
    filter:   { state: true },
    loading:  { state: true },
    err:      { state: true },
  };
  declare w: number;
  declare h: number;
  declare embedded: boolean;
  declare chrome: { title: string; subtitle: string };
  declare repos: RepoStatus[];
  declare scanned: number;
  declare filter: Filter;
  declare loading: boolean;
  declare err: string;

  constructor() {
    super();
    this.w = 1180; this.h = 760; this.embedded = false;
    this.chrome = { title: "Repos", subtitle: "branch · ahead · behind · dirty" };
    this.repos = [];
    this.scanned = 0;
    this.filter = "all";
    this.loading = false;
    this.err = "";
  }
  createRenderRoot() { return this; }

  async connectedCallback() {
    super.connectedCallback();
    const [title, subtitle] = await Promise.all([
      T("window.repos.title"),
      T("window.repos.subtitle"),
    ]);
    this.chrome = { title, subtitle };
    await this._refresh();
  }

  async _refresh() {
    if (this.loading) return;
    this.loading = true;
    this.err = "";
    try {
      const svc = await import("@desktop/repos/service");
      const r = await svc.Status({}) as { repos: RepoStatus[]; scanned: number };
      this.repos = r.repos || [];
      this.scanned = r.scanned || 0;
    } catch (e: unknown) {
      this.err = e instanceof Error ? e.message : String(e);
    } finally {
      this.loading = false;
    }
  }

  _counts() {
    return {
      total:  this.repos.length,
      dirty:  this.repos.filter(r => r.dirty).length,
      ahead:  this.repos.filter(r => r.ahead > 0).length,
      behind: this.repos.filter(r => r.behind > 0).length,
    };
  }

  _visible(): RepoStatus[] {
    switch (this.filter) {
      case "dirty":  return this.repos.filter(r => r.dirty);
      case "ahead":  return this.repos.filter(r => r.ahead > 0);
      case "behind": return this.repos.filter(r => r.behind > 0);
      default:       return this.repos;
    }
  }

  _openInGit(r: RepoStatus) {
    const params = new URLSearchParams();
    params.set("surface", "git");
    params.set("path", r.path);
    window.location.search = "?" + params.toString();
  }

  render() {
    const counts = this._counts();
    const visible = this._visible();

    const chipStyle = (active: boolean) => `
      padding:4px 10px; border-radius:4px; border:none; cursor:pointer;
      font-family:var(--font-mono); font-size:10.5px;
      background:${active ? "rgba(255,255,255,0.08)" : "transparent"};
      color:${active ? "var(--fg-0)" : "var(--fg-3)"};
      --wails-draggable: no-drag;
    `;

    const toolbar = html`
      <div style="display:inline-flex; border-radius:6px; padding:2px;
                  background:rgba(0,0,0,0.18); border:1px solid rgba(255,255,255,0.06); gap:2px;">
        <button @click=${() => this.filter = "all"} style=${chipStyle(this.filter === "all")}>all · ${counts.total}</button>
        <button @click=${() => this.filter = "dirty"} style=${chipStyle(this.filter === "dirty")}>dirty · ${counts.dirty}</button>
        <button @click=${() => this.filter = "ahead"} style=${chipStyle(this.filter === "ahead")}>↑ ${counts.ahead}</button>
        <button @click=${() => this.filter = "behind"} style=${chipStyle(this.filter === "behind")}>↓ ${counts.behind}</button>
      </div>
      <div style="flex:1"></div>
      <lthn-btn tone="primary" size="sm" ?dim=${this.loading}
        @click=${() => void this._refresh()}>
        <i class="fa-solid ${this.loading ? "fa-spinner" : "fa-rotate"}"
           style="font-size:10px;"></i>
        ${this.loading ? "Scanning…" : "Re-scan"}
      </lthn-btn>
    `;

    const badge = (label: string, color: string) => html`
      <span style="font-size:10px; padding:2px 7px; border-radius:4px;
                   font-family:var(--font-mono); color:${color};
                   background:rgba(0,0,0,0.30);
                   border:1px solid rgba(255,255,255,0.05);">${label}</span>
    `;

    const body = html`
      <div style="flex:1; min-height:0; overflow:auto; padding:16px 18px;">
        ${this.err ? html`
          <div style="padding:10px 14px; color:var(--err-400);
                      background:rgba(255,76,76,0.06);
                      border:1px solid rgba(255,76,76,0.18);
                      border-radius:6px; font-size:12px;
                      line-height:1.55; margin-bottom:14px;">
            ${this.err}
          </div>
        ` : nothing}
        ${visible.length === 0 && !this.loading ? html`
          <div style="padding:40px; text-align:center; color:var(--fg-3);
                      font-size:12px;">
            ${counts.total === 0
              ? "no repos found in $HOME/Code/{core,lthn,host-uk,lab,snider}"
              : `no repos match filter "${this.filter}"`}
          </div>
        ` : html`
          <div style="display:grid;
                      grid-template-columns:repeat(auto-fill, minmax(280px, 1fr));
                      gap:10px;">
            ${visible.map(r => html`
              <button @click=${() => this._openInGit(r)}
                title=${r.path}
                style="background:rgba(0,0,0,0.18); border-radius:8px;
                       padding:12px; display:flex; flex-direction:column;
                       gap:8px; cursor:pointer; text-align:left;
                       border:1px solid ${r.dirty
                         ? "rgba(251, 191, 36, 0.30)"
                         : r.ahead > 0
                           ? "rgba(167, 139, 250, 0.30)"
                           : r.behind > 0
                             ? "rgba(56, 189, 248, 0.30)"
                             : "rgba(255,255,255,0.05)"};
                       color:var(--fg-0); font:inherit;
                       --wails-draggable: no-drag;">
                <div style="display:flex; justify-content:space-between;
                            align-items:baseline;">
                  <span style="font-weight:600; font-size:13px;
                               color:var(--fg-0);">${r.name}</span>
                  <span style="font-family:var(--font-mono); font-size:10px;
                               color:var(--fg-3);">${r.branch}</span>
                </div>
                <div style="display:flex; flex-wrap:wrap; gap:4px;">
                  ${r.staged > 0 ? badge(`+${r.staged}`, "#34d399") : nothing}
                  ${r.modified > 0 ? badge(`~${r.modified}`, "#fbbf24") : nothing}
                  ${r.untracked > 0 ? badge(`?${r.untracked}`, "var(--fg-3)") : nothing}
                  ${r.ahead > 0 ? badge(`↑${r.ahead}`, "#a78bfa") : nothing}
                  ${r.behind > 0 ? badge(`↓${r.behind}`, "#38bdf8") : nothing}
                  ${!r.dirty && r.ahead === 0 && r.behind === 0
                    ? badge("clean", "var(--fg-3)") : nothing}
                </div>
                ${r.error ? html`
                  <div style="font-size:10px; color:var(--err-400);
                              font-style:italic; line-height:1.4;">
                    ${r.error}
                  </div>
                ` : nothing}
              </button>
            `)}
          </div>
        `}
      </div>
    `;

    return renderChrome({
      title: this.chrome.title, subtitle: this.chrome.subtitle,
      w: this.w, h: this.h, toolbar, body,
      footer: html`${this.scanned} scanned · ${counts.total} repos ·
        ${counts.dirty} dirty · ${counts.ahead} ahead · ${counts.behind} behind`,
      embedded: this.embedded,
    });
  }
}
customElements.define("lthn-repos-window", LthnReposWindow);
