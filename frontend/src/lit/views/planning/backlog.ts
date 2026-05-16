// SPDX-Licence-Identifier: EUPL-1.2
//
// Planning view · Backlog — <lthn-view-backlog>
//
// RICE-scored backlog list with weighted impact / effort / confidence
// dot-rails. Sort + filter + search controls in the toolbar; clicking
// a column header re-orders the list. Top N items shaded brand to hint
// "this fits the next sprint".
//
// Backed by pkg/tasks via the Wails surface. _loadFromBackend()
// lazy-imports @desktop/tasks/service and pulls open-state Issues
// when the binding is present; falls back to the fixture array
// otherwise so dev / headless runs stay useful.
//
// RICE fields (impact, effort, confidence, theme) are not yet on
// the Issue schema — see deferred Mantis follow-up. Until then the
// row mapper synthesises sensible placeholders from existing fields
// (priority → score, project → theme, fixed 3-dot bars).

import { LitElement, html } from "lit";
import { renderChrome } from "../../chrome";

/** A backlog item with RICE inputs + computed weighted score. */
interface BacklogItem {
  /** Stable id, e.g. "L-201". */
  id: string;
  /** Item title. */
  title: string;
  /** Computed RICE-style score (0-100). */
  score: number;
  /** Weighted impact (1-5). */
  impact: number;
  /** Weighted effort (1-5). */
  effort: number;
  /** Weighted confidence (1-5). */
  conf: number;
  /** Free-form theme tag, e.g. "L3 · network". */
  theme: string;
}

/** Sortable column ids. */
type SortKey = "score" | "impact" | "effort" | "conf";

/** Backlog view state. */
interface BacklogData {
  items:  BacklogItem[];
  /** Active free-text search across id + title. Empty = all. */
  query:  string;
  /** Active sort key + direction. */
  sortBy: SortKey;
  sortDir: "asc" | "desc";
}

/** Backend Issue shape from pkg/tasks.List — JSON-tagged Go struct. */
interface BackendIssue {
  ID?:       string;
  Project?:  string;
  Summary?:  string;
  State?:    string;
  Severity?: string;
  Priority?: string;
}

/** Map a backend Issue into the view's BacklogItem shape.
 *
 *  Mapping notes (until extended Issue fields land — see deferred
 *  Mantis follow-up):
 *    - score: derived from Priority (immediate/urgent/high/normal/
 *      low/none → 95/85/75/60/40/20). Replace with real RICE compute
 *      once Estimate / Impact / Confidence columns exist.
 *    - impact/effort/conf: placeholder dots (3/3/3). Avoids empty
 *      rails without misrepresenting unknown values as zero.
 *    - theme: Issue.Project so multi-project backlogs at least group
 *      visually by source. Theme field on the schema is a follow-up.
 */
function mapBackendIssue(it: BackendIssue): BacklogItem {
  const PRIORITY_SCORE: Record<string, number> = {
    immediate: 95, urgent: 85, high: 75, normal: 60, low: 40, none: 20,
  };
  const priority = (it.Priority ?? "normal").toLowerCase();
  return {
    id:     it.ID      ?? "",
    title:  it.Summary ?? "(untitled)",
    score:  PRIORITY_SCORE[priority] ?? 60,
    impact: 3,
    effort: 3,
    conf:   3,
    theme:  it.Project ?? "",
  };
}

const BACKLOG_FIXTURE: BacklogData = {
  query: "", sortBy: "score", sortDir: "desc",
  items: [
    { id: "L-201", title: "Federated chat across LetherNet peers",   score: 92, impact: 5, effort: 5, conf: 4, theme: "L3 · network" },
    { id: "L-189", title: "Audit log retention policy + UI control", score: 88, impact: 4, effort: 1, conf: 5, theme: "Compliance" },
    { id: "L-187", title: "Telemetry · per-model success rate",       score: 84, impact: 4, effort: 2, conf: 4, theme: "Observability" },
    { id: "L-198", title: "Quantisation auto-picker per hardware",    score: 81, impact: 5, effort: 3, conf: 3, theme: "Runtime" },
    { id: "L-203", title: "Vi voice mode · local Whisper transcribe", score: 76, impact: 3, effort: 4, conf: 4, theme: "Chat" },
    { id: "L-185", title: "Wire HuggingFace search to model browser", score: 72, impact: 3, effort: 3, conf: 5, theme: "Models" },
    { id: "L-205", title: "iOS / iPad shells (read-only)",            score: 68, impact: 4, effort: 5, conf: 2, theme: "Platforms" },
    { id: "L-184", title: "Document MCP tool registry format",        score: 60, impact: 2, effort: 1, conf: 5, theme: "Docs" },
  ],
};

/** Filter + sort backlog items. Pure — exported so tests can pin the
 *  contract without mounting the element. */
export function projectBacklog(
  items: readonly BacklogItem[],
  query: string,
  sortBy: SortKey,
  sortDir: "asc" | "desc",
): BacklogItem[] {
  const q = (query || "").trim().toLowerCase();
  let out = q
    ? items.filter(it => it.title.toLowerCase().includes(q) || it.id.toLowerCase().includes(q))
    : items.slice();
  out = out.slice().sort((a, b) => {
    const av = a[sortBy] as number;
    const bv = b[sortBy] as number;
    return sortDir === "asc" ? av - bv : bv - av;
  });
  return out;
}

class LthnViewBacklog extends LitElement {
  static readonly properties = {
    embedded: { type: Boolean, reflect: true },
    data:     { state: true },
  };
  declare embedded: boolean;
  declare data: BacklogData;

  constructor() {
    super();
    this.embedded = false;
    this.data = BACKLOG_FIXTURE;
  }
  createRenderRoot() { return this; }

  async connectedCallback() {
    super.connectedCallback();
    void this._loadFromBackend();
  }

  /**
   * Pull live backlog data from pkg/tasks.Service.List() and replace
   * `this.data.items`. On any failure (binding missing in dev,
   * List() rejected, malformed response) the existing fixture array
   * is kept untouched so the view continues to render usefully.
   *
   * Defensive shape mirrors operations/status.ts + coding/repos.ts —
   * lazy-import + typeof guard + try/catch + console.warn on the
   * fall-through paths.
   */
  async _loadFromBackend(): Promise<void> {
    try {
      // Lazy-import so a missing binding (e.g. happy-dom test runs
      // without the wails3 generated tree) doesn't blow up module
      // load. Same defensive pattern as coding/repos.ts.
      const svc = await import("@desktop/tasks/service").catch(() => null);
      if (!svc || typeof (svc as { List?: unknown }).List !== "function") {
        console.warn("[backlog] @desktop/tasks/service unavailable — keeping fixture");
        return;
      }
      const r = await (svc as { List: (input: unknown) => Promise<{ Value?: { issues?: BackendIssue[] } }> })
        .List({ state: "open" });
      const rows = r?.Value?.issues;
      if (!Array.isArray(rows)) {
        console.warn("[backlog] List() returned no issues array — keeping fixture", r);
        return;
      }
      const mapped = rows
        .filter((row) => (row?.Summary ?? "").trim() !== "")
        .map(mapBackendIssue);
      // Only replace when the backend gave us something — empty array
      // could mean "no open tasks" OR "service half-wired"; keep the
      // fixture instead of flashing an empty pane in dev.
      if (mapped.length > 0) {
        this.data = { ...this.data, items: mapped };
      }
    } catch (e: unknown) {
      console.warn("[backlog] List() failed — keeping fixture:", e);
    }
  }

  /** Update the active filter query. Public for the test suite. */
  setQuery(q: string): void {
    this.data = { ...this.data, query: q };
  }

  /** Toggle sort: clicking the active column flips direction;
   *  clicking a different column resets to descending. */
  setSort(key: SortKey): void {
    if (this.data.sortBy === key) {
      this.data = { ...this.data, sortDir: this.data.sortDir === "desc" ? "asc" : "desc" };
    } else {
      this.data = { ...this.data, sortBy: key, sortDir: "desc" };
    }
  }

  private _dots(n: number, max = 5) {
    return Array.from({length: max}, (_, i) => html`<span style="width:5px; height:5px; border-radius:50%; background:${i < n ? "var(--brand-400)" : "rgba(255,255,255,0.10)"};"></span>`);
  }

  private _header(label: string, key: SortKey) {
    const active = this.data.sortBy === key;
    const arrow = active ? (this.data.sortDir === "desc" ? "▾" : "▴") : "";
    return html`
      <button class="lthn-backlog-sort" data-sort=${key} data-active=${active ? "true" : "false"}
              @click=${() => this.setSort(key)}
              style="background:transparent; border:none; color:${active ? "var(--brand-300)" : "var(--fg-3)"};
                     font:inherit; font-family:var(--font-mono); font-size:9.5px;
                     letter-spacing:0.10em; text-transform:uppercase; cursor:pointer; padding:0;
                     --wails-draggable: no-drag;">
        ${label} ${arrow}
      </button>
    `;
  }

  render() {
    const visible = projectBacklog(this.data.items, this.data.query, this.data.sortBy, this.data.sortDir);

    const body = html`
      <div style="flex:1; display:flex; flex-direction:column; min-height:0; overflow:hidden;">
        <div style="padding:18px 22px 12px; display:flex; align-items:center; gap:12px;">
          <h2 style="margin:0; font-size:22px; color:var(--fg-0); letter-spacing:-0.02em; font-weight:600;">Backlog</h2>
          <span style="font-family:var(--font-mono); font-size:11px; color:var(--fg-3);">${this.data.items.length} items · sorted by ${this.data.sortBy} ${this.data.sortDir === "desc" ? "↓" : "↑"}</span>
          <div style="flex:1"></div>
          <input class="lthn-backlog-search" type="text"
                 placeholder="Search…"
                 .value=${this.data.query}
                 @input=${(ev: Event) => this.setQuery((ev.target as HTMLInputElement).value)}
                 style="background:rgba(255,255,255,0.04); border:1px solid rgba(255,255,255,0.07);
                        color:var(--fg-1); padding:4px 8px; border-radius:6px;
                        font:inherit; font-size:11px; width:160px;
                        --wails-draggable: no-drag;" />
          <lthn-btn tone="primary" size="sm"><i class="fa-solid fa-plus" style="font-size:10px;"></i> New</lthn-btn>
        </div>

        <div style="flex:1; overflow:auto; padding:0 22px 18px; min-height:0;">
          <div style="background:rgba(255,255,255,0.025); border:1px solid rgba(255,255,255,0.06); border-radius:10px; overflow:hidden;">
            <div style="display:grid; grid-template-columns: 56px 1fr 130px 80px 80px 80px; gap:12px; padding:10px 16px;
                        background:rgba(0,0,0,0.20); border-bottom:1px solid rgba(255,255,255,0.05);
                        font-family:var(--font-mono); font-size:9.5px; color:var(--fg-3); letter-spacing:0.10em; text-transform:uppercase;">
              ${this._header("Score", "score")}
              <span>Item</span>
              <span>Theme</span>
              ${this._header("Impact", "impact")}
              ${this._header("Effort", "effort")}
              ${this._header("Conf",   "conf")}
            </div>
            ${visible.length === 0 ? html`
              <div class="lthn-backlog-empty" style="padding:32px 16px; text-align:center; font-size:12px; color:var(--fg-3);">
                No items match "${this.data.query.trim()}".
              </div>
            ` : visible.map((it, i) => html`
              <div class="lthn-backlog-row" data-id=${it.id}
                   style="display:grid; grid-template-columns: 56px 1fr 130px 80px 80px 80px; gap:12px; padding:14px 16px;
                          border-bottom:${i < visible.length - 1 ? "1px solid rgba(255,255,255,0.04)" : "none"};
                          align-items:center;">
                <span style="font-family:var(--font-mono); font-size:14px; color:${it.score >= 80 ? "var(--brand-300)" : "var(--fg-1)"}; font-weight:500;">${it.score}</span>
                <div style="min-width:0;">
                  <div style="font-size:13px; color:var(--fg-0);">${it.title}</div>
                  <div style="font-family:var(--font-mono); font-size:10px; color:var(--fg-3); margin-top:3px;">${it.id}</div>
                </div>
                <span style="font-family:var(--font-mono); font-size:10.5px; color:var(--fg-2);">${it.theme}</span>
                <div style="display:flex; gap:3px;">${this._dots(it.impact)}</div>
                <div style="display:flex; gap:3px;">${this._dots(it.effort)}</div>
                <div style="display:flex; gap:3px;">${this._dots(it.conf)}</div>
              </div>
            `)}
          </div>
        </div>
      </div>
    `;

    return renderChrome({
      title: "Backlog",
      subtitle: `${this.data.items.length} items · RICE-scored`,
      body,
      footer: html`weighted by impact × confidence ÷ effort · drag to reorder · top 8 fit next sprint`,
      embedded: this.embedded,
    });
  }
}
customElements.define("lthn-view-backlog", LthnViewBacklog);
