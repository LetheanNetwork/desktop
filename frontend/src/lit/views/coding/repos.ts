// SPDX-Licence-Identifier: EUPL-1.2
// Coding view · Repos — <lthn-view-repos>
//
// Lists watched repos with lang, branch, last commit, build state and
// open-PR count. Fixtures only in v1; real workspace data via
// core/ide/pkg/workspace is tracked as a Mantis follow-up.
//
// Supports the `embedded` attribute: when set, renderChrome omits the
// titlebar / footer frame and the element fills the shell body slot.
// Mirror of the embedded contract in welcome-window.ts.

import { LitElement, html, nothing } from "lit";
import { renderChrome } from "../../chrome";

/** Shape of one watched-repo fixture row. */
interface RepoRow {
  name:   string;
  lang:   string;
  branch: string;
  commit: string;
  build:  "passing" | "running" | "failing";
  prs:    number;
}

/** Map lang string to the dot colour used in the design reference. */
function langColour(lang: string): string {
  const map: Record<string, string> = {
    Go:   "#5fd7da",
    Rust: "#f59e0b",
    TS:   "#3b82f6",
    MD:   "#888",
  };
  return map[lang] ?? "#888";
}

/** Render the build-state pill via the design-system <lthn-state-pill>. */
function buildPill(build: RepoRow["build"]) {
  switch (build) {
    case "passing":
      return html`<lthn-state-pill variant="connected">passing</lthn-state-pill>`;
    case "running":
      return html`<lthn-state-pill variant="queued">running</lthn-state-pill>`;
    case "failing":
      return html`<lthn-state-pill variant="disconnected">failing</lthn-state-pill>`;
  }
}

class LthnViewRepos extends LitElement {
  static readonly properties = {
    w:        { type: Number },
    h:        { type: Number },
    embedded: { type: Boolean, reflect: true },
    /** Fixture data — replace with a live backend call when
     *  core/ide/pkg/workspace bindings land (Mantis follow-up). */
    repos:    { state: true },
  };

  declare w: number;
  declare h: number;
  declare embedded: boolean;
  declare repos: RepoRow[];

  constructor() {
    super();
    this.w = 1180; this.h = 720; this.embedded = false;
    // Design-reference fixtures — identical names + data to the Claude
    // Design reference impl (lit-views-coding.js) so the rendered output
    // matches the approved mockup. Mantis follow-up: replace with a live
    // call to the workspace service once bindings exist.
    this.repos = [
      { name: "lethean/desktop",  lang: "Go",   branch: "main",  commit: "a3f12c", build: "passing", prs: 3 },
      { name: "lethean/runtime",  lang: "Go",   branch: "main",  commit: "7b2901", build: "passing", prs: 1 },
      { name: "lethean/core",     lang: "Rust", branch: "main",  commit: "e4f8a1", build: "passing", prs: 0 },
      { name: "lethean/web",      lang: "TS",   branch: "v0.2",  commit: "1a9f33", build: "running", prs: 2 },
      { name: "lethean/docs",     lang: "MD",   branch: "main",  commit: "c2d014", build: "passing", prs: 5 },
      { name: "host-uk/platform", lang: "TS",   branch: "main",  commit: "f9e220", build: "failing", prs: 1 },
    ];
  }

  createRenderRoot() { return this; }

  /** Count repos by build state for the footer summary. */
  _summary(): { passing: number; failing: number } {
    return this.repos.reduce(
      (acc, r) => {
        if (r.build === "passing") acc.passing++;
        if (r.build === "failing") acc.failing++;
        return acc;
      },
      { passing: 0, failing: 0 },
    );
  }

  render() {
    const sum = this._summary();
    const prsTotal = this.repos.reduce((t, r) => t + r.prs, 0);

    const body = html`
      <div style="flex:1; display:flex; flex-direction:column; min-height:0;">
        <div style="padding:18px 22px 12px; display:flex; align-items:center; gap:12px;">
          <h2 style="margin:0; font-size:22px; color:var(--fg-0); letter-spacing:-0.02em; font-weight:600;">Repos</h2>
          <span style="font-family:var(--font-mono); font-size:11px; color:var(--fg-3);">
            ${this.repos.length} watched · ${prsTotal} PRs in flight
          </span>
          <div style="flex:1"></div>
          <lthn-btn tone="ghost" size="sm">
            <i class="fa-solid fa-magnifying-glass" style="font-size:10px;"></i> Filter
          </lthn-btn>
          <lthn-btn tone="primary" size="sm">
            <i class="fa-solid fa-plus" style="font-size:10px;"></i> Clone
          </lthn-btn>
        </div>
        <div style="flex:1; overflow:auto; padding:0 22px 18px;">
          <div style="background:rgba(255,255,255,0.025); border:1px solid rgba(255,255,255,0.06); border-radius:10px; overflow:hidden;">
            <!-- header row -->
            <div style="display:grid; grid-template-columns: 1.6fr 70px 110px 110px 110px 60px; gap:14px;
                        padding:10px 16px; background:rgba(0,0,0,0.20);
                        border-bottom:1px solid rgba(255,255,255,0.05);
                        font-family:var(--font-mono); font-size:9.5px; color:var(--fg-3);
                        letter-spacing:0.10em; text-transform:uppercase;">
              <span>Repo</span>
              <span>Lang</span>
              <span>Branch</span>
              <span>Last commit</span>
              <span>Build</span>
              <span style="text-align:right;">PRs</span>
            </div>
            <!-- data rows -->
            ${this.repos.map((r, i) => html`
              <div class="lthn-view-repos-row"
                   data-repo=${r.name}
                   style="display:grid; grid-template-columns: 1.6fr 70px 110px 110px 110px 60px; gap:14px;
                          padding:14px 16px;
                          border-bottom:${i < this.repos.length - 1 ? "1px solid rgba(255,255,255,0.04)" : "none"};
                          align-items:center;">
                <div style="display:flex; align-items:center; gap:10px;">
                  <span style="width:6px; height:6px; border-radius:50%; background:${langColour(r.lang)};"></span>
                  <span style="font-family:var(--font-mono); font-size:13px; color:var(--fg-0);">${r.name}</span>
                </div>
                <span style="font-family:var(--font-mono); font-size:11px; color:var(--fg-2);">${r.lang}</span>
                <span style="font-family:var(--font-mono); font-size:11px; color:var(--fg-1);">${r.branch}</span>
                <span style="font-family:var(--font-mono); font-size:10.5px; color:var(--fg-3);">${r.commit}</span>
                <span>${buildPill(r.build)}</span>
                <span style="font-family:var(--font-mono); font-size:11px;
                             color:${r.prs > 0 ? "var(--brand-300)" : "var(--fg-3)"};
                             text-align:right;">${r.prs}</span>
              </div>
            `)}
          </div>
        </div>
      </div>
    `;

    return renderChrome({
      title: "Repos",
      subtitle: `${this.repos.length} watched`,
      w: this.w, h: this.h,
      body,
      footer: html`${sum.passing} passing · ${sum.failing} failing · fixtures (workspace bindings pending) · workspace: ~/Code/lethean`,
      embedded: this.embedded,
    });
  }
}

customElements.define("lthn-view-repos", LthnViewRepos);
