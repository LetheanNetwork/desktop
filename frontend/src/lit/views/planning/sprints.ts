// SPDX-Licence-Identifier: EUPL-1.2
//
// Planning view · Sprints — <lthn-view-sprints>
//
// Lightweight kanban board on a fortnight horizon. Four columns
// (To do / In flight / Review / Done), card chips with estimate
// dots and assignee → reviewer chain. Filter / Add controls in the
// toolbar; sparkline on the right showing burndown shape.
//
// Backed by core/ide pkg/tasks. Today wires fixtures only — once
// pkg/tasks ships Wails bindings, _loadFromBackend() will fetch
// State-grouped Issues for the active sprint.

import { LitElement, html, nothing } from "lit";
import { renderChrome } from "../../chrome";

/** A kanban card. Mirrors a tasks.Issue with sprint-board overlay. */
interface Card {
  /** Mantis-style id, e.g. "L-184". */
  id: string;
  /** One-line title. */
  title: string;
  /** Story-point estimate, drawn as 1-of-5 dots. */
  est: number;
  /** Lane tag — drives the colour pip. */
  tag: "docs" | "feat" | "ship" | "ui" | "ops" | "platform" | "design";
  /** Owner handle. Optional for backlog-cold cards. */
  assignee?: string;
  /** Reviewer handle, if the card is in the Review column. */
  reviewer?: string;
}

/** A kanban column. */
interface Column {
  /** Column id — used as a stable key for filter / persistence. */
  id: "todo" | "doing" | "review" | "done";
  /** Header label. */
  label: string;
  /** Total card count (may differ from cards.length when filtered). */
  count: number;
  /** Cards in display order. */
  cards: Card[];
}

/** Snapshot of sprint board state. */
interface SprintsData {
  /** Sprint number (display), e.g. 24. */
  sprint:    number;
  /** Window label, e.g. "12 May → 23 May". */
  window:    string;
  /** Days remaining in the sprint. */
  remaining: number;
  /** Completed-of-planned counts. */
  done:      number;
  planned:   number;
  /** Burndown sparkline samples (newest last). */
  series:    number[];
  /** Active filter query — empty = show all. */
  query:     string;
  /** Columns rendered left-to-right. */
  columns:   Column[];
}

const SPRINT_FIXTURE: SprintsData = {
  sprint: 24, window: "12 May → 23 May", remaining: 8,
  done: 14, planned: 18,
  series: [14, 16, 18, 17, 15, 18, 14],
  query: "",
  columns: [
    { id: "todo", label: "To do", count: 4, cards: [
      { id: "L-184", title: "Document MCP tool registry format", est: 3, tag: "docs" },
      { id: "L-185", title: "Wire HuggingFace search to model browser", est: 5, tag: "feat" },
      { id: "L-187", title: "Telemetry · per-model success rate", est: 2, tag: "feat" },
      { id: "L-189", title: "Audit log retention policy", est: 1, tag: "ops" },
    ]},
    { id: "doing", label: "In flight", count: 3, cards: [
      { id: "L-181", title: "v0.2 release notes + announcement", est: 3, tag: "ship", assignee: "you" },
      { id: "L-182", title: "LoRA training UI · final pass", est: 5, tag: "feat", assignee: "Tobi" },
      { id: "L-180", title: "Linux tray binary · packaging", est: 8, tag: "platform", assignee: "Mei" },
    ]},
    { id: "review", label: "Review", count: 2, cards: [
      { id: "L-178", title: "Settings · sectioned scroll refactor", est: 3, tag: "ui", assignee: "you", reviewer: "Tobi" },
      { id: "L-176", title: "Tools window · try-it sandbox", est: 5, tag: "feat", assignee: "Tobi", reviewer: "Mei" },
    ]},
    { id: "done", label: "Done", count: 5, cards: [
      { id: "L-173", title: "Tray icon SVG family", est: 2, tag: "design" },
      { id: "L-171", title: "Benchmark history persistence", est: 3, tag: "feat" },
      { id: "L-170", title: "MCP tools window v1", est: 8, tag: "feat" },
      { id: "L-168", title: "Status pill component", est: 1, tag: "ui" },
      { id: "L-165", title: "Live log filter rail", est: 2, tag: "feat" },
    ]},
  ],
};

/** Filter cards across columns by free-text query against id + title.
 *  Returns a NEW columns array — original fixture is never mutated.
 *  Empty / whitespace query returns the input unchanged so callers
 *  can use the result blindly. Pure — exported so the test suite can
 *  pin the filter contract. */
export function filterCards(columns: readonly Column[], query: string): Column[] {
  const q = (query || "").trim().toLowerCase();
  if (!q) return columns.map(c => ({ ...c, cards: c.cards.slice() }));
  return columns.map(c => {
    const cards = c.cards.filter(card =>
      card.title.toLowerCase().includes(q) || card.id.toLowerCase().includes(q),
    );
    return { ...c, cards, count: cards.length };
  });
}

class LthnViewSprints extends LitElement {
  static readonly properties = {
    embedded: { type: Boolean, reflect: true },
    data:     { state: true },
  };
  declare embedded: boolean;
  declare data: SprintsData;

  constructor() {
    super();
    this.embedded = false;
    this.data = SPRINT_FIXTURE;
  }
  createRenderRoot() { return this; }

  async connectedCallback() {
    super.connectedCallback();
    void this._loadFromBackend();
  }

  /** Fetch sprint board snapshot from pkg/tasks. Today: no-op (fixture
   *  remains). Tomorrow: tasks.List(filter) per column.State, sliced
   *  to the active sprint window. */
  async _loadFromBackend(): Promise<void> {
    // Pending Wails binding — see go/pkg/tasks/wails.go (TBD).
  }

  /** Update the active filter query and re-render. Public so the
   *  test suite can drive the input wire without firing an event. */
  setQuery(q: string): void {
    this.data = { ...this.data, query: q };
  }

  private _tagColor(t: Card["tag"]): string {
    switch (t) {
      case "docs":     return "var(--fg-3)";
      case "feat":     return "var(--brand-300)";
      case "ship":     return "var(--warning-400)";
      case "ui":       return "#a78bfa";
      case "ops":      return "var(--success-400)";
      case "platform": return "#5fd7da";
      case "design":   return "var(--brand-300)";
    }
  }

  render() {
    const filtered = filterCards(this.data.columns, this.data.query);
    const sparkData = this.data.series.join(",");

    const body = html`
      <div style="flex:1; display:flex; flex-direction:column; min-height:0; overflow:hidden;">
        <div style="padding:18px 22px 14px; display:flex; align-items:baseline; justify-content:space-between; border-bottom:1px solid rgba(255,255,255,0.05);">
          <div>
            <h2 style="margin:0; font-size:22px; color:var(--fg-0); letter-spacing:-0.02em; font-weight:600;">Sprint ${this.data.sprint}</h2>
            <div style="font-family:var(--font-mono); font-size:11px; color:var(--fg-3); margin-top:4px;">${this.data.window} · ${this.data.remaining} days remaining · ${this.data.done} / ${this.data.planned} done</div>
          </div>
          <div style="display:flex; align-items:center; gap:8px;">
            <lthn-sparkline data=${sparkData} max="20" color="var(--brand-400)" .width=${100} .height=${28}></lthn-sparkline>
            <input class="lthn-sprints-filter" type="text"
                   placeholder="Filter…"
                   .value=${this.data.query}
                   @input=${(ev: Event) => this.setQuery((ev.target as HTMLInputElement).value)}
                   style="background:rgba(255,255,255,0.04); border:1px solid rgba(255,255,255,0.07);
                          color:var(--fg-1); padding:4px 8px; border-radius:6px;
                          font:inherit; font-size:11px; width:120px;
                          --wails-draggable: no-drag;" />
            <lthn-btn tone="primary" size="sm"><i class="fa-solid fa-plus" style="font-size:10px;"></i> Card</lthn-btn>
          </div>
        </div>

        <div class="lthn-sprints-board" style="flex:1; display:grid; grid-template-columns: repeat(4, 1fr); gap:14px; padding:18px 22px; overflow:auto; min-height:0;">
          ${filtered.map(c => html`
            <div class="lthn-sprints-column" data-col=${c.id} style="display:flex; flex-direction:column; gap:8px; min-width:0;">
              <div style="display:flex; align-items:center; gap:8px; padding:0 4px 4px;">
                <span style="font-family:var(--font-mono); font-size:10.5px; color:var(--fg-3); letter-spacing:0.10em; text-transform:uppercase;">${c.label}</span>
                <span style="font-family:var(--font-mono); font-size:10.5px; color:var(--fg-2); background:rgba(255,255,255,0.06); padding:1px 7px; border-radius:999px;">${c.count}</span>
              </div>
              ${c.cards.map(card => html`
                <div class="lthn-sprints-card" data-card-id=${card.id}
                     style="padding:12px 14px; border-radius:8px;
                            background:rgba(255,255,255,0.025);
                            border:1px solid rgba(255,255,255,0.06);
                            display:flex; flex-direction:column; gap:8px; cursor:pointer;">
                  <div style="display:flex; align-items:center; justify-content:space-between;">
                    <span style="font-family:var(--font-mono); font-size:9.5px; color:var(--fg-3); letter-spacing:0.04em;">${card.id}</span>
                    <span style="font-family:var(--font-mono); font-size:9.5px; padding:1px 6px; border-radius:999px;
                                 background:${this._tagColor(card.tag)}1a; color:${this._tagColor(card.tag)};">${card.tag}</span>
                  </div>
                  <div style="font-size:12.5px; color:var(--fg-0); line-height:1.4;">${card.title}</div>
                  <div style="display:flex; align-items:center; justify-content:space-between; margin-top:2px;">
                    <div style="display:flex; gap:4px;">
                      ${Array.from({length: 5}, (_, i) => html`<span style="width:5px; height:5px; border-radius:1px; background:${i < card.est ? "var(--brand-400)" : "rgba(255,255,255,0.10)"};"></span>`)}
                    </div>
                    ${card.assignee ? html`<span style="font-family:var(--font-mono); font-size:9.5px; color:var(--fg-3);">${card.assignee}${card.reviewer ? ` → ${card.reviewer}` : ""}</span>` : nothing}
                  </div>
                </div>
              `)}
            </div>
          `)}
        </div>
      </div>
    `;

    return renderChrome({
      title: `Sprint ${this.data.sprint}`,
      subtitle: this.data.window,
      body,
      footer: html`${this.data.done} done · 3 in flight · 2 in review · 4 to do · burndown on track`,
      embedded: this.embedded,
    });
  }
}
customElements.define("lthn-view-sprints", LthnViewSprints);
