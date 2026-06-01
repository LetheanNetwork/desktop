// SPDX-Licence-Identifier: EUPL-1.2
// Coding view · Repos — <lthn-view-repos>
//
// Lists watched repos with lang, branch, last commit, build state and
// open-PR count. Wired to pkg/repos.Service.Status() — the canonical
// scan of $HOME/Code/{core,lthn,host-uk,lab,snider} plus any paths
// contributed by registered SourceProviders.
//
// Fixture data is kept as a fallback so the view renders something
// useful in headless dev / when the Wails binding isn't loaded.
// Live data replaces the fixture once Status() resolves (~50ms flash
// pattern mirrored from operations/status.ts).
//
// Supports the `embedded` attribute: when set, renderChrome omits the
// titlebar / footer frame and the element fills the shell body slot.
// Mirror of the embedded contract in welcome-window.ts.

import { LitElement, html, nothing } from "lit";
import { renderChrome } from "../../chrome";
import { clampRows } from "../_bounds";

/** Shape of one watched-repo fixture row. */
interface RepoRow {
  name:   string;
  path?:  string;   // local filesystem path (live rows only) — for core-lint
  lang:   string;
  branch: string;
  commit: string;
  build:  "passing" | "running" | "failing";
  prs:    number;
}

/** Backend row shape from pkg/repos.Status — JSON-tagged Go struct. */
interface BackendRepoStatus {
  name?:      string;
  path?:      string;
  branch?:    string;
  modified?:  number;
  untracked?: number;
  staged?:    number;
  ahead?:     number;
  behind?:    number;
  dirty?:     boolean;
  error?:     string;
}

/**
 * Map a backend Status row into the view's RepoRow shape.
 *
 * Field-mapping notes:
 *   - name/branch: direct.
 *   - lang: the backend doesn't classify language today — default "Go"
 *     since the canonical scan roots are dev workspaces. Refining this
 *     (extension probe / .git/config heuristic) is a Mantis follow-up.
 *   - commit: Status doesn't carry a SHA — placeholder "—" until the
 *     scm/git surface grows a head-sha field.
 *   - build: derived from { error, dirty } — error → failing,
 *     dirty/ahead/behind → running, clean → passing. Good-enough
 *     mapping until CI integration lands.
 *   - prs: backend has no PR count surface — use `ahead` (commits not
 *     yet pushed) as the closest "outstanding work" proxy. Real PR
 *     count is a Mantis follow-up (needs forge/github introspection).
 */
function mapBackendRow(r: BackendRepoStatus): RepoRow {
  const err = (r.error ?? "").trim() !== "";
  const churn = (r.ahead ?? 0) + (r.behind ?? 0) + (r.modified ?? 0) + (r.untracked ?? 0) + (r.staged ?? 0);
  const build: RepoRow["build"] = err ? "failing" : (r.dirty || churn > 0) ? "running" : "passing";
  return {
    name:   r.name   ?? "",
    path:   r.path   ?? "",
    lang:   "Go",
    branch: r.branch ?? "",
    commit: "—",
    build,
    prs:    r.ahead ?? 0,
  };
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
    /** Live data from pkg/repos.Service.Status(); falls back to the
     *  constructor-seeded fixture when the binding is unavailable. */
    repos:    { state: true },
    scanning: { state: true }, // repo name currently being scanned ("" = none)
    scanMsg:  { state: true }, // last scan result, shown in the header
  };

  declare w: number;
  declare h: number;
  declare embedded: boolean;
  declare repos: RepoRow[];
  declare scanning: string;
  declare scanMsg: string;

  /** Repo state changes slowly — 60s refresh is plenty. */
  private _timer: ReturnType<typeof setInterval> | null = null;

  constructor() {
    super();
    this.w = 1180; this.h = 720; this.embedded = false;
    this.scanning = ""; this.scanMsg = "";
    // Design-reference fixtures — identical names + data to the Claude
    // Design reference impl (lit-views-coding.js) so the rendered output
    // matches the approved mockup. Used as fallback when the Wails
    // binding isn't loaded (headless dev / tests without the bridge).
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

  async connectedCallback() {
    super.connectedCallback();
    // Fire the first load after the constructor's fixture seed so the
    // panel always renders something (~50ms flash pattern shared with
    // operations/status.ts). On success the fixture is replaced; on
    // failure the fixture stays as a useful dev fallback.
    await this._loadFromBackend();
    // 60s cadence — repo state churns far slower than uptime probes,
    // so a minute between scans keeps the panel fresh without thrashing
    // `git status` across the canonical roots.
    this._timer = setInterval(() => { void this._loadFromBackend(); }, 60_000);
  }

  disconnectedCallback() {
    if (this._timer) { clearInterval(this._timer); this._timer = null; }
    super.disconnectedCallback();
  }

  /**
   * Pull live repo data from pkg/repos.Service.Status() and replace
   * `this.repos`. On any failure (binding missing in dev, Status()
   * rejected, malformed response) the existing repos array is kept
   * untouched so the view continues to render the fixture fallback.
   */
  async _loadFromBackend() {
    try {
      // Lazy-import so a missing binding (e.g. happy-dom test runs
      // without the wails3 generated tree) doesn't blow up module
      // load. Same defensive pattern as operations/status.ts.
      const svc = await import("@desktop/repos/service").catch(() => null);
      if (!svc || typeof (svc as { Status?: unknown }).Status !== "function") {
        console.warn("[repos] @desktop/repos/service unavailable — keeping fixture");
        return;
      }
      const r = await (svc as { Status: (input: unknown) => Promise<{ Value?: { repos?: BackendRepoStatus[] } }> }).Status({});
      // Mantis #1491 — defence-in-depth length cap on backend response.
      // clampRows returns [] for non-arrays so the !Array.isArray branch
      // collapses into "no rows → keep fixture".
      const rows = clampRows<BackendRepoStatus>(r?.Value?.repos);
      if (rows.length === 0) {
        console.warn("[repos] Status() returned no repos array — keeping fixture", r);
        return;
      }
      // Drop entries with no name (malformed rows from the backend).
      this.repos = rows
        .filter((row) => (row?.name ?? "").trim() !== "")
        .map(mapBackendRow);
    } catch (e: unknown) {
      console.warn("[repos] Status() failed — keeping fixture:", e);
    }
  }

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

  /** Ask the shell to open Dispatch pre-filled with this repo (Coding →
   *  Dispatch). The app-shell listens for lthn:dispatch-repo on window. */
  _dispatch(repo: string) {
    window.dispatchEvent(new CustomEvent("lthn:dispatch-repo", { detail: { repo } }));
  }

  /** Run core-lint over a repo and file new findings as tasks (Tasks.Detect).
   *  The count shows in the header; the tasks land in Coding/Issues, ready to
   *  dispatch. Needs a live local path (live rows only) + the agent endpoint. */
  async _scan(r: RepoRow) {
    if (!r.path || this.scanning) return;
    this.scanning = r.name;
    this.scanMsg = "";
    try {
      const svc = await import("@desktop/tasks/wailsservice").catch(() => null);
      type DetectFn = (i: unknown) => Promise<{ Value?: { created?: number; skipped?: number; findings?: number } }>;
      const detect = svc && (svc as { Detect?: DetectFn }).Detect;
      const detectUpdates = svc && (svc as { DetectUpdates?: DetectFn }).DetectUpdates;
      if (!detect) { this.scanMsg = "scan unavailable — is the agent endpoint running?"; return; }
      // One Scan = lint findings + outdated dependencies, both filed as tasks.
      let created = 0, known = 0, found = 0;
      for (const fn of [detect, detectUpdates]) {
        if (!fn) continue;
        const v = (await fn({ repo: r.name, path: r.path }))?.Value ?? {};
        created += v.created ?? 0; known += v.skipped ?? 0; found += v.findings ?? 0;
      }
      this.scanMsg = `${r.name}: ${created} filed · ${known} known · ${found} found`;
    } catch (e: unknown) {
      this.scanMsg = `scan failed: ${e instanceof Error ? e.message : String(e)}`;
    } finally {
      this.scanning = "";
    }
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
          ${this.scanMsg ? html`
            <span style="font-family:var(--font-mono); font-size:10.5px; color:var(--brand-300);">${this.scanMsg}</span>
          ` : nothing}
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
                   title="Dispatch an agent on ${r.name}"
                   @click=${() => this._dispatch(r.name)}
                   style="display:grid; grid-template-columns: 1.6fr 70px 110px 110px 110px 60px; gap:14px;
                          padding:14px 16px; cursor:pointer;
                          border-bottom:${i < this.repos.length - 1 ? "1px solid rgba(255,255,255,0.04)" : "none"};
                          align-items:center;">
                <div style="display:flex; align-items:center; gap:10px;">
                  <span style="width:6px; height:6px; border-radius:50%; background:${langColour(r.lang)};"></span>
                  <span style="font-family:var(--font-mono); font-size:13px; color:var(--fg-0);">${r.name}</span>
                  <i class="fa-solid fa-paper-plane" style="font-size:9px; color:var(--fg-3); margin-left:2px;"></i>
                  ${r.path ? html`
                    <span @click=${(e: Event) => { e.preventDefault(); e.stopPropagation(); void this._scan(r); }}
                          title="Scan ${r.name} with core-lint → file issues as tasks"
                          style="cursor:pointer; --wails-draggable:no-drag; margin-left:6px; color:var(--fg-3);">
                      <i class="fa-solid ${this.scanning === r.name ? "fa-spinner" : "fa-magnifying-glass"}" style="font-size:9px;"></i>
                    </span>
                  ` : nothing}
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
      footer: html`${sum.passing} passing · ${sum.failing} failing · pkg/repos · refreshed every 60 s · workspace: ~/Code/{core,lthn,host-uk,lab,snider}`,
      embedded: this.embedded,
    });
  }
}

customElements.define("lthn-view-repos", LthnViewRepos);
