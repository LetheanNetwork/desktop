// SPDX-Licence-Identifier: EUPL-1.2
// Coding view · Issues — <lthn-view-issues>
//
// Filterable issue tracker. Backend: pkg/tasks (already adopted in lthn
// from core/ide/pkg/tasks, Mantis #359). No Wails binding for tasks
// exists yet — this element ships with fixtures and the binding wire-up
// is tracked as a Mantis follow-up (see footer note).
//
// Sidebar filters: All open / Assigned to me / Mentions / Closed.
// Label filter chips: bug / enhancement / ui / docs / metal / telemetry.
//
// Supports the `embedded` attribute — no chrome when set.

import { LitElement, html, nothing } from "lit";
import { renderChrome } from "../../chrome";
import { clampRows } from "../_bounds";

/** An issue row as rendered. Derived from tasks.Issue on the Go side;
 *  fixtures mirror the Claude Design reference layout. */
interface IssueRow {
  n:        number;
  title:    string;
  repo:     string;
  labels:   string[];
  age:      string;
  assignee: string | null;
}

type SidebarFilter = "all" | "mine" | "mentions" | "closed";

/** Wails-shaped Issue from pkg/tasks — partial type matching the
 *  fields the binding emits. Optional because regen drift shouldn't
 *  blow up the mapper. */
interface BackendIssue {
  ID?:       string;
  Project?:  string;
  Summary?:  string;
  State?:    string;
  Severity?: string;
  Priority?: string;
}

/** Map a backend Issue into the view's IssueRow shape.
 *  - n: deterministic short-hash → small int. Replace when issues
 *    grow native numeric IDs (Mantis schema gap).
 *  - repo: Issue.Project. Until per-issue repo metadata lands the
 *    project IS the repo for routing purposes.
 *  - labels: derived from Severity + Priority for visual cues
 *    until tag/label fields ship on the schema.
 *  - age: empty for now (Issue lacks created/updated timestamps in
 *    the v1 surface). Refine when schema extends.
 *  - assignee: null — Assignee field exists in api.go but not in
 *    the Wails List output v1; will populate once the binding
 *    surfaces it. */
function mapBackendIssue(it: BackendIssue): IssueRow {
  const idHash = (it.ID ?? "0").slice(0, 4);
  const n = parseInt(idHash, 16) % 9999;
  const labels: string[] = [];
  const sev = (it.Severity ?? "").toLowerCase();
  const prio = (it.Priority ?? "").toLowerCase();
  if (sev === "block" || sev === "crash" || sev === "major") labels.push("bug");
  if (prio === "urgent" || prio === "immediate") labels.push("urgent");
  return {
    n,
    title:    it.Summary ?? "(untitled)",
    repo:     it.Project ?? "—",
    labels,
    age:      "",
    assignee: null,
  };
}

/** Label → CSS colour. Kept local so the element needs no shared util. */
function labelColour(label: string): string {
  const map: Record<string, string> = {
    bug:         "var(--err-400)",
    ui:          "var(--brand-300)",
    ux:          "var(--brand-300)",
    enhancement: "var(--success-400)",
    docs:        "var(--fg-2)",
    metal:       "#5fd7da",
    telemetry:   "var(--warning-400)",
  };
  return map[label] ?? "var(--fg-3)";
}

class LthnViewIssues extends LitElement {
  static readonly properties = {
    w:           { type: Number },
    h:           { type: Number },
    embedded:    { type: Boolean, reflect: true },
    sideFilter:  { state: true },
    labelFilter: { state: true },
    /** Fixture rows — replace with tasks.List() binding when it ships. */
    issues:      { state: true },
  };

  declare w: number;
  declare h: number;
  declare embedded: boolean;
  declare sideFilter: SidebarFilter;
  declare labelFilter: string | null;
  declare issues: IssueRow[];
  private _loading = false;
  private _timer: ReturnType<typeof setInterval> | null = null;

  constructor() {
    super();
    this.w = 1180; this.h = 720; this.embedded = false;
    this.sideFilter = "all";
    this.labelFilter = null;
    // Design-reference fixtures — match the Claude Design mockup exactly.
    this.issues = [
      { n: 284, title: "Tray icon active state not updating on model swap",      repo: "desktop", labels: ["bug", "ui"],              age: "2h",   assignee: "you"  },
      { n: 283, title: "Metal kernel fails on M1 Air with 8 GB RAM",              repo: "runtime", labels: ["bug", "metal"],            age: "5h",   assignee: "Tobi" },
      { n: 281, title: "Add `--dry-run` flag to `lthn deploy`",                  repo: "desktop", labels: ["enhancement"],             age: "yest", assignee: null   },
      { n: 278, title: "Document the EUPL-1.2 commercial-use clause",             repo: "docs",    labels: ["docs"],                   age: "2d",   assignee: "Mei"  },
      { n: 275, title: "Settings → API key regenerate is destructive without warning", repo: "desktop", labels: ["bug", "ux"],         age: "3d",   assignee: "you"  },
      { n: 271, title: "Benchmark · histogram of P50/P95/P99 token latency",      repo: "desktop", labels: ["enhancement", "telemetry"], age: "5d", assignee: null   },
    ];
  }

  createRenderRoot() { return this; }

  async connectedCallback() {
    super.connectedCallback();
    await this._loadFromBackend();
    this._timer = setInterval(() => { void this._loadFromBackend(); }, 60_000);
  }

  disconnectedCallback() {
    if (this._timer) { clearInterval(this._timer); this._timer = null; }
    super.disconnectedCallback();
  }

  async _loadFromBackend(): Promise<void> {
    if (this._loading) return;
    this._loading = true;
    try {
      const svc = await import("@desktop/tasks/service").catch(() => null);
      if (!svc || typeof (svc as { List?: unknown }).List !== "function") {
        return;
      }
      const r = await (svc as {
        List: (input: { state: string; limit: number }) => Promise<{ Value?: { issues?: BackendIssue[] } }>
      }).List({ state: "open", limit: 50 });
      // Mantis #1491 — defence-in-depth length cap on backend response.
      const rows = clampRows<BackendIssue>(r?.Value?.issues);
      if (rows.length > 0) {
        this.issues = rows.map(mapBackendIssue);
      }
    } catch {
      // Backend unavailable — fixture stays.
    } finally {
      this._loading = false;
    }
  }

  /** Issues visible after applying the active sidebar + label filter. */
  _visible(): IssueRow[] {
    let rows = this.issues;
    if (this.sideFilter === "mine")     rows = rows.filter(r => r.assignee === "you");
    if (this.sideFilter === "mentions") rows = rows.filter(r => r.assignee !== null);
    if (this.sideFilter === "closed")   rows = [];          // fixtures have no closed rows
    if (this.labelFilter !== null) {
      rows = rows.filter(r => r.labels.includes(this.labelFilter!));
    }
    return rows;
  }

  _sidebarItem(label: string, n: number, filter: SidebarFilter) {
    const active = this.sideFilter === filter;
    return html`
      <div @click=${() => { this.sideFilter = filter; this.labelFilter = null; }}
           style="display:flex; align-items:center; gap:10px; padding:8px 12px; border-radius:6px;
                  background:${active ? "rgba(64,193,197,0.10)" : "transparent"};
                  border:1px solid ${active ? "rgba(64,193,197,0.22)" : "transparent"};
                  color:${active ? "var(--brand-300)" : "var(--fg-2)"}; font-size:12.5px; cursor:pointer;
                  --wails-draggable: no-drag;">
        <span style="flex:1;">${label}</span>
        <span style="font-family:var(--font-mono); font-size:10px; color:var(--fg-3);">${n}</span>
      </div>
    `;
  }

  render() {
    const visible = this._visible();
    const labels = ["bug", "enhancement", "ui", "docs", "metal", "telemetry"];

    const body = html`
      <div style="flex:1; display:grid; grid-template-columns: 200px 1fr; min-height:0;">
        <!-- sidebar -->
        <aside style="background:rgba(0,0,0,0.18); border-right:1px solid rgba(255,255,255,0.05);
                      padding:14px 8px; display:flex; flex-direction:column; gap:2px;">
          ${this._sidebarItem("All open",      this.issues.length, "all")}
          ${this._sidebarItem("Assigned to me", this.issues.filter(i => i.assignee === "you").length, "mine")}
          ${this._sidebarItem("Mentions",       this.issues.filter(i => i.assignee !== null).length, "mentions")}
          ${this._sidebarItem("Closed",         0, "closed")}
          <div style="height:1px; background:rgba(255,255,255,0.05); margin:10px 8px;"></div>
          <div style="padding:0 12px 6px; font-family:var(--font-mono); font-size:9.5px; color:var(--fg-3);
                      letter-spacing:0.10em; text-transform:uppercase;">Labels</div>
          ${labels.map(t => {
            const active = this.labelFilter === t;
            return html`
              <div @click=${() => { this.labelFilter = active ? null : t; this.sideFilter = "all"; }}
                   style="display:flex; align-items:center; gap:10px; padding:6px 12px; font-size:12px;
                          color:${active ? "var(--fg-0)" : "var(--fg-2)"}; cursor:pointer;
                          --wails-draggable: no-drag;">
                <span style="width:7px; height:7px; border-radius:2px; background:${labelColour(t)};"></span>
                ${t}
              </div>
            `;
          })}
        </aside>
        <!-- issue list -->
        <main style="overflow:auto; padding:14px 22px 22px;">
          ${visible.length === 0 ? html`
            <div style="padding:40px; text-align:center; color:var(--fg-3); font-size:12px;">
              No issues match the current filter.
            </div>
          ` : html`
            <div style="background:rgba(255,255,255,0.025); border:1px solid rgba(255,255,255,0.06); border-radius:10px; overflow:hidden;">
              ${visible.map((it, i) => html`
                <div class="lthn-view-issues-row"
                     data-issue=${it.n}
                     style="display:grid; grid-template-columns: 60px 1fr 100px 80px; gap:14px;
                            padding:14px 16px;
                            border-bottom:${i < visible.length - 1 ? "1px solid rgba(255,255,255,0.04)" : "none"};
                            align-items:center;">
                  <span style="font-family:var(--font-mono); font-size:11.5px; color:var(--fg-3);">#${it.n}</span>
                  <div style="min-width:0;">
                    <div style="font-size:13px; color:var(--fg-0); line-height:1.4;">${it.title}</div>
                    <div style="display:flex; align-items:center; gap:6px; margin-top:6px;">
                      <span style="font-family:var(--font-mono); font-size:10px; color:var(--fg-3);">${it.repo}</span>
                      ${it.labels.map(l => html`
                        <span style="font-family:var(--font-mono); font-size:9.5px; padding:1px 6px;
                                     border-radius:999px; background:${labelColour(l)}1a; color:${labelColour(l)};">
                          ${l}
                        </span>
                      `)}
                    </div>
                  </div>
                  <span style="font-family:var(--font-mono); font-size:11px;
                               color:${it.assignee ? "var(--fg-1)" : "var(--fg-3)"};">
                    ${it.assignee ?? "unassigned"}
                  </span>
                  <span style="font-family:var(--font-mono); font-size:10.5px; color:var(--fg-3); text-align:right;">
                    ${it.age}
                  </span>
                </div>
              `)}
            </div>
          `}
        </main>
      </div>
    `;

    return renderChrome({
      title: "Issues",
      subtitle: `${visible.length} of ${this.issues.length} open`,
      w: this.w, h: this.h,
      body,
      footer: html`label-filter ${this.labelFilter ? `"${this.labelFilter}"` : "off"} · fixtures (tasks bindings pending Mantis #359) · sorted by last-activity`,
      embedded: this.embedded,
    });
  }
}

customElements.define("lthn-view-issues", LthnViewIssues);
