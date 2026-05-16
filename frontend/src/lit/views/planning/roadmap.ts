// SPDX-Licence-Identifier: EUPL-1.2
//
// Planning view · Roadmap — <lthn-view-roadmap>
//
// Quarter-band swimlane timeline. Lanes (Tray / Runtime / Fabric /
// Commercial) cross four quarters; each cell shows shipped (green
// tick) or scheduled (brand arrow) items. Lane filter toggles the
// rendered set; "Edit" reserved for the inline-edit pass that
// pkg/tasks gains in a later iteration.
//
// Backed by core/ide pkg/tasks (lane = derived from a tasks.Issue
// label / theme + TargetVersion = quarter band). Today fixtures-only.

import { LitElement, html } from "lit";
import { renderChrome } from "../../chrome";

/** A single roadmap item placed on the lane × quarter grid. */
interface RoadmapItem {
  /** Quarter index (0..N-1). */
  q: number;
  /** Display title. */
  title: string;
  /** Cell width hint — reserved for future spans. */
  w: number;
  /** True if the item has shipped — drawn as a green tick. */
  done?: boolean;
}

/** A lane is a horizontal strand grouping related items. */
interface Lane {
  /** Stable lane id, e.g. "L1" / "GTM". */
  id: string;
  /** Display label, e.g. "Tray · L1". */
  label: string;
  /** Items in display order. */
  items: RoadmapItem[];
}

interface RoadmapData {
  /** Quarter band labels (left → right). */
  quarters: string[];
  /** Index into `quarters` of the "now" cell — highlighted brand. */
  nowIndex: number;
  /** Lanes top → bottom. */
  lanes:    Lane[];
  /** Lanes the user has hidden via the filter (set of lane ids). */
  hidden:   Set<string>;
}

const ROADMAP_FIXTURE: RoadmapData = {
  quarters: ["Q2 · 2026", "Q3 · 2026", "Q4 · 2026", "Q1 · 2027"],
  nowIndex: 0,
  hidden: new Set<string>(),
  lanes: [
    { id: "L1", label: "Tray · L1", items: [
      { q: 0, title: "v0.2 GA",                  w: 1, done: true },
      { q: 1, title: "Windows + Linux beta",      w: 1 },
      { q: 2, title: "LoRA training in-app",      w: 1 },
      { q: 3, title: "Multi-account workspaces",  w: 1 },
    ]},
    { id: "L2", label: "Runtime · L2", items: [
      { q: 0, title: "go-mlx Metal optimisation", w: 1, done: true },
      { q: 1, title: "HIP backend (AMD)",          w: 1 },
      { q: 2, title: "CUDA backend",                w: 2 },
    ]},
    { id: "L3", label: "Fabric · L3", items: [
      { q: 1, title: "LetherNet design",          w: 1 },
      { q: 2, title: "Peer discovery alpha",        w: 1 },
      { q: 3, title: "Disaggregated inference",     w: 1 },
    ]},
    { id: "GTM", label: "Commercial", items: [
      { q: 0, title: "Host UK channel pilots",       w: 1, done: true },
      { q: 2, title: "Hosted (L2) commercial GA",    w: 1 },
      { q: 3, title: "Regulated SMB sales motion",    w: 1 },
    ]},
  ],
};

/** Visible lanes after the user's hide-set is applied. Pure helper —
 *  exported so the test suite can pin the contract. */
export function visibleLanes(lanes: readonly Lane[], hidden: ReadonlySet<string>): Lane[] {
  return lanes.filter(l => !hidden.has(l.id));
}

class LthnViewRoadmap extends LitElement {
  static readonly properties = {
    embedded: { type: Boolean, reflect: true },
    data:     { state: true },
  };
  declare embedded: boolean;
  declare data: RoadmapData;

  constructor() {
    super();
    this.embedded = false;
    this.data = ROADMAP_FIXTURE;
  }
  createRenderRoot() { return this; }

  async connectedCallback() {
    super.connectedCallback();
    void this._loadFromBackend();
  }

  /** Pending pkg/tasks Wails binding. Today: fixture remains. */
  async _loadFromBackend(): Promise<void> { /* no-op */ }

  /** Toggle a lane's visibility. Public for the test suite. */
  toggleLane(id: string): void {
    const next = new Set(this.data.hidden);
    if (next.has(id)) next.delete(id); else next.add(id);
    this.data = { ...this.data, hidden: next };
  }

  render() {
    const lanes = visibleLanes(this.data.lanes, this.data.hidden);
    const colTemplate = `120px repeat(${this.data.quarters.length}, 1fr)`;

    const body = html`
      <div style="flex:1; display:flex; flex-direction:column; min-height:0; overflow:hidden;">
        <div style="padding:18px 22px 12px; display:flex; align-items:center; gap:12px;">
          <h2 style="margin:0; font-size:22px; color:var(--fg-0); letter-spacing:-0.02em; font-weight:600;">Roadmap</h2>
          <span style="font-family:var(--font-mono); font-size:11px; color:var(--fg-3);">${this.data.quarters.length} quarters · ${lanes.length} of ${this.data.lanes.length} lanes</span>
          <div style="flex:1"></div>
          <div class="lthn-roadmap-lane-filter" style="display:flex; align-items:center; gap:6px;">
            ${this.data.lanes.map(l => {
              const on = !this.data.hidden.has(l.id);
              return html`
                <button class="lthn-roadmap-lane-toggle" data-lane=${l.id} data-on=${on ? "true" : "false"}
                        @click=${() => this.toggleLane(l.id)}
                        style="background:${on ? "rgba(64,193,197,0.10)" : "transparent"};
                               border:1px solid ${on ? "rgba(64,193,197,0.22)" : "rgba(255,255,255,0.07)"};
                               color:${on ? "var(--brand-300)" : "var(--fg-3)"};
                               font:inherit; font-family:var(--font-mono); font-size:10px;
                               padding:3px 8px; border-radius:999px; cursor:pointer;
                               --wails-draggable: no-drag;">
                  ${l.id}
                </button>
              `;
            })}
          </div>
        </div>

        <div style="flex:1; overflow:auto; padding:0 22px 22px; min-height:0;">
          <div class="lthn-roadmap-grid" style="display:grid; grid-template-columns: ${colTemplate}; gap:10px;
                      padding:10px 0; border-bottom:1px solid rgba(255,255,255,0.06); margin-bottom:14px;">
            <div></div>
            ${this.data.quarters.map((q, i) => html`
              <div style="font-family:var(--font-mono); font-size:10.5px; color:${i === this.data.nowIndex ? "var(--brand-300)" : "var(--fg-3)"}; letter-spacing:0.10em; text-transform:uppercase; padding:0 12px;">
                ${q}${i === this.data.nowIndex ? " · now" : ""}
              </div>
            `)}
          </div>

          ${lanes.length === 0 ? html`
            <div class="lthn-roadmap-empty" style="padding:48px 16px; text-align:center; font-size:13px; color:var(--fg-3);">
              All lanes hidden — toggle one back on to see items.
            </div>
          ` : lanes.map(lane => html`
            <div class="lthn-roadmap-lane" data-lane=${lane.id}
                 style="display:grid; grid-template-columns: ${colTemplate}; gap:10px;
                        padding:8px 0; min-height:80px; border-bottom:1px solid rgba(255,255,255,0.04); align-items:start;">
              <div style="padding-top:14px; padding-right:16px; font-family:var(--font-mono); font-size:11px; color:var(--fg-2); letter-spacing:0.04em;">${lane.label}</div>
              ${this.data.quarters.map((_, qi) => html`
                <div style="display:flex; flex-direction:column; gap:6px; padding:6px;">
                  ${lane.items.filter(it => it.q === qi).map(it => html`
                    <div class="lthn-roadmap-item" data-done=${it.done ? "true" : "false"}
                         style="padding:8px 12px; border-radius:8px;
                                background:${it.done ? "rgba(34,197,94,0.06)" : "rgba(64,193,197,0.06)"};
                                border:1px solid ${it.done ? "rgba(34,197,94,0.22)" : "rgba(64,193,197,0.22)"};
                                display:flex; align-items:center; gap:8px;">
                      <i class="fa-solid ${it.done ? "fa-check" : "fa-arrow-right"}" style="font-size:9px; color:${it.done ? "var(--success-400)" : "var(--brand-300)"};"></i>
                      <span style="flex:1; font-size:12px; color:var(--fg-0); line-height:1.4;">${it.title}</span>
                    </div>
                  `)}
                </div>
              `)}
            </div>
          `)}
        </div>
      </div>
    `;

    return renderChrome({
      title: "Roadmap",
      subtitle: `${this.data.quarters[0]} → ${this.data.quarters[this.data.quarters.length - 1]}`,
      body,
      footer: html`Q2 2026 · 3 of 4 lane items shipped · 1 in flight · Q3 lane plan locks 1 June`,
      embedded: this.embedded,
    });
  }
}
customElements.define("lthn-view-roadmap", LthnViewRoadmap);
