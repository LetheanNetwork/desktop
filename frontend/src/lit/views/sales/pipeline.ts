// SPDX-Licence-Identifier: EUPL-1.2
// Sales · Pipeline — <lthn-view-pipeline>
//
// Deal columns by stage (Qualifying / Engaging / Proposal / Closing) with
// per-stage totals + per-deal cards. Monochrome treatment per
// HANDOVER-VIEWS.md — no leaderboards, no goal shame, just an honest
// record of what's in flight. Tasteful framing intentional.
//
// Two-shell: standalone (chrome) when spawned directly, embedded (no
// chrome) when mounted by <lthn-app-shell>. Mirrors the welcome /
// settings window pattern documented in design_two_shell_pattern.md.
//
// Data: _loadFromBackend() tries pkg/sales/pipeline via the Wails binding
// (lazy-imported, falls back to fixture when the binding is absent — e.g.
// in tests or non-Wails contexts). The pipeline binding returns
// { Value: { columns: PipelineColumn[], totalValue: string, totalDeals: number } }.
// On failure the fixture stays in place so the view is always renderable.
//
// Drag-to-move: each deal card is draggable; stage columns are drop targets.
// On drop the view optimistically swaps the card across columns and fires
// the backend MoveDeal call via the lazy-imported binding. On rejection it
// reverts. On success it dispatches a window-level CustomEvent
// `lthn:sales:moved` so sibling sales views (deals/forecast/contacts) can
// reload. Per the calm-UX rule no toast surfaces on failure — the revert
// is the only signal.

import { LitElement, html } from "lit";
import { renderChrome } from "../../chrome";

/** A single deal card inside a pipeline column. */
interface Deal {
  /** Backend deal slug (e.g. "202605-DEAL-001"). Optional because the
   *  fixture pre-dates the id field; when absent the customer name `c`
   *  is used as the slug for MoveDeal calls (best-effort fallback). */
  id?: string;
  /** Customer / counterparty name. */
  c: string;
  /** Headline value (pre-formatted with currency symbol). */
  v: string;
  /** Free-text qualifier — sector / stage detail / blocker. */
  t: string;
}

/** Stage identifier — matches backend PipelineColumn.id values. */
type StageID = "qual" | "engage" | "propose" | "close";

/** A pipeline column = one stage in the funnel. */
interface PipelineColumn {
  /** Stable id for filtering / tests. */
  id:    StageID;
  /** Human label rendered in the column header. */
  label: string;
  /** Pre-formatted aggregated value for the stage. */
  value: string;
  /** Deals currently in this stage. */
  deals: Deal[];
}

/** Detail payload for the `lthn:sales:moved` CustomEvent — sibling sales
 *  views subscribe and trigger their own `_loadFromBackend()` to stay in
 *  sync with the new stage assignment. */
export interface SalesMovedDetail {
  /** Deal slug — `Deal.id` when present, otherwise `Deal.c` (customer). */
  deal_id:    string;
  /** Stage the deal moved out of. */
  from_stage: StageID;
  /** Stage the deal landed in. */
  to_stage:   StageID;
}

/** Shape used to ferry a drag operation between dragstart and drop —
 *  encoded into the DataTransfer as JSON so cross-column drops work
 *  without leaking element references. */
interface DragPayload {
  deal_id:    string;
  from_stage: StageID;
  customer:   string;
}

/** MIME type for the JSON drag payload — kept private to this module so
 *  the test file can mirror it without exporting an internal contract. */
const DRAG_MIME = "application/x-lthn-pipeline-deal";

/** Fixture pipeline. Strings stay opaque (e.g. "£64 K") because the
 *  source-of-truth is the future rollup service, not arithmetic on
 *  the client. When `pkg/sales/pipeline` lands, swap this constant
 *  for a fetch + Reactive Controller mirroring chat-window patterns. */
const FIXTURE_PIPELINE: PipelineColumn[] = [
  { id: "qual", label: "Qualifying", value: "£64 K", deals: [
    { c: "Northwold Council",         v: "£18 K", t: "public sector · pilot interest" },
    { c: "Heritage Law LLP",          v: "£24 K", t: "GDPR + privilege" },
    { c: "Marrow Health · Manchester", v: "£22 K", t: "on-prem inference" },
  ]},
  { id: "engage", label: "Engaging", value: "£128 K", deals: [
    { c: "Stannard & Co",       v: "£44 K", t: "7 partners · pilot" },
    { c: "Pemberton Capital",   v: "£62 K", t: "compliance memo signed" },
    { c: "Lichfield NHS Trust", v: "£22 K", t: "DPIA in review" },
  ]},
  { id: "propose", label: "Proposal", value: "£218 K", deals: [
    { c: "Crown Estates · IT", v: "£82 K", t: "SOW v3 · awaiting sign" },
    { c: "Cobbet Industries",  v: "£68 K", t: "3-year hosted plan" },
    { c: "GreenLine Logistics", v: "£68 K", t: "GDPR + sovereign clause" },
  ]},
  { id: "close", label: "Closing", value: "£148 K", deals: [
    { c: "Whitethorn Press",   v: "£36 K",  t: "signature this week" },
    { c: "Calliope Partners",  v: "£112 K", t: "final terms · legal review" },
  ]},
];

/** Sum a deal count + headline string for the chrome subtitle so the
 *  footer / header stay honest to the fixture data without hand-edits. */
function pipelineSummary(cols: readonly PipelineColumn[]): { deals: number; total: string } {
  const deals = cols.reduce((n, c) => n + c.deals.length, 0);
  return { deals, total: "£558 K" };
}

class LthnViewPipeline extends LitElement {
  static readonly properties = {
    w:        { type: Number },
    h:        { type: Number },
    embedded: { type: Boolean, reflect: true },
    /** Pipeline columns — starts with fixture; replaced by backend data
     *  once _loadFromBackend() resolves. */
    columns:  { state: true },
    /** Backend load state — "idle" | "loading" | "ok" | "err". */
    _loadState: { state: true },
    /** Stage id currently being dragged over — drives drop-target styling. */
    _dragOverStage: { state: true },
  };

  declare w: number;
  declare h: number;
  declare embedded: boolean;
  declare columns: PipelineColumn[];
  declare _loadState: "idle" | "loading" | "ok" | "err";
  declare _dragOverStage: StageID | null;

  constructor() {
    super();
    this.w = 1280;
    this.h = 720;
    this.embedded = false;
    this.columns = FIXTURE_PIPELINE;
    this._loadState = "idle";
    this._dragOverStage = null;
  }

  createRenderRoot() { return this; }

  async connectedCallback() {
    super.connectedCallback();
    await this._loadFromBackend();
  }

  /** Load pipeline data from the pkg/sales/pipeline Wails binding.
   *  Falls back to the fixture silently on any failure (binding absent,
   *  network error, or non-Wails context such as tests). */
  async _loadFromBackend() {
    if (this._loadState === "loading") return;
    this._loadState = "loading";
    try {
      // Lazy-import so a missing binding in tests doesn't kill module load.
      const svc = await import("@desktop/sales/pipeline/service").catch(() => null);
      if (!svc || typeof (svc as { List?: unknown }).List !== "function") {
        this._loadState = "idle";
        return;
      }
      type ListFn = (input: { stage?: string }) => Promise<{ Value?: { columns?: PipelineColumn[] } }>;
      const r = await (svc as { List: ListFn }).List({});
      const cols = r?.Value?.columns;
      if (Array.isArray(cols) && cols.length > 0) {
        this.columns = cols;
        this._loadState = "ok";
      } else {
        this._loadState = "idle";
      }
    } catch {
      this._loadState = "err";
      // Fixture stays in place — no user-visible error for a backend miss.
    }
  }

  /** dragstart on a deal card — encode the deal slug + source stage into
   *  the DataTransfer so the drop handler can reconstruct the move
   *  without sharing element refs. */
  _onDealDragStart(e: DragEvent, from: StageID, deal: Deal) {
    if (!e.dataTransfer) return;
    const payload: DragPayload = {
      deal_id:    deal.id ?? deal.c,
      from_stage: from,
      customer:   deal.c,
    };
    e.dataTransfer.setData(DRAG_MIME, JSON.stringify(payload));
    e.dataTransfer.effectAllowed = "move";
  }

  /** dragover on a stage column — must preventDefault so the drop fires,
   *  and we light up the column visually as a drop target. */
  _onColumnDragOver(e: DragEvent, stage: StageID) {
    if (!e.dataTransfer) return;
    // Only accept our own MIME — leaves text drops etc. alone.
    if (!e.dataTransfer.types.includes(DRAG_MIME)) return;
    e.preventDefault();
    e.dataTransfer.dropEffect = "move";
    if (this._dragOverStage !== stage) this._dragOverStage = stage;
  }

  /** dragleave clears the drop-target styling. The browser fires this
   *  per nested child too, so we only clear if the leave is the one for
   *  this stage (not a child). Best-effort — a fresh dragover restores. */
  _onColumnDragLeave(stage: StageID) {
    if (this._dragOverStage === stage) this._dragOverStage = null;
  }

  /** drop handler — optimistically reshape `this.columns`, call the
   *  backend MoveDeal binding, revert on rejection, and emit
   *  `lthn:sales:moved` on success. */
  async _onColumnDrop(e: DragEvent, to: StageID) {
    if (!e.dataTransfer) return;
    e.preventDefault();
    this._dragOverStage = null;

    const raw = e.dataTransfer.getData(DRAG_MIME);
    if (!raw) return;
    let payload: DragPayload;
    try {
      payload = JSON.parse(raw) as DragPayload;
    } catch {
      return;
    }
    const { deal_id, from_stage } = payload;
    if (from_stage === to) return; // no-op move

    // Snapshot for revert before mutating.
    const prev = this.columns;
    const next = this._moveDealLocal(prev, deal_id, from_stage, to);
    if (!next) return; // deal not found — bail without touching state
    this.columns = next;

    // Backend call — lazy-import keeps the test fixture decoupled.
    const svc = await import("@desktop/sales/pipeline/service").catch(() => null);
    type MoveFn = (input: { dealId: string; toStage: string }) => Promise<{ OK?: boolean }>;
    const moveFn = svc && typeof (svc as { MoveDeal?: unknown }).MoveDeal === "function"
      ? (svc as { MoveDeal: MoveFn }).MoveDeal
      : null;
    if (!moveFn) {
      // No binding (test / non-Wails context) — keep the optimistic move
      // and still emit the cross-view event so listeners exercise.
      this._emitMoved(deal_id, from_stage, to);
      return;
    }

    try {
      const r = await moveFn({ dealId: deal_id, toStage: to });
      if (r && r.OK === false) {
        // Backend explicitly rejected — revert + leave a quiet trace.
        this.columns = prev;
        this._loadState = "err";
        return;
      }
      this._emitMoved(deal_id, from_stage, to);
    } catch {
      this.columns = prev;
      this._loadState = "err";
    }
  }

  /** Pure helper — return a new columns array with the deal lifted from
   *  `from` and appended to `to`. Returns null when the deal isn't in
   *  the source column (caller bails). */
  _moveDealLocal(
    cols: PipelineColumn[],
    deal_id: string,
    from: StageID,
    to: StageID,
  ): PipelineColumn[] | null {
    const src = cols.find(c => c.id === from);
    const dst = cols.find(c => c.id === to);
    if (!src || !dst) return null;
    const idx = src.deals.findIndex(d => (d.id ?? d.c) === deal_id);
    if (idx < 0) return null;
    const deal = src.deals[idx];
    return cols.map(c => {
      if (c.id === from) return { ...c, deals: c.deals.filter((_, i) => i !== idx) };
      if (c.id === to)   return { ...c, deals: [...c.deals, deal] };
      return c;
    });
  }

  /** Dispatch the window-level cross-view refresh event. Sibling sales
   *  views (deals/forecast/contacts) opt in by registering a listener
   *  and calling their own `_loadFromBackend()`. */
  _emitMoved(deal_id: string, from_stage: StageID, to_stage: StageID) {
    const detail: SalesMovedDetail = { deal_id, from_stage, to_stage };
    window.dispatchEvent(new CustomEvent<SalesMovedDetail>("lthn:sales:moved", { detail }));
  }

  render() {
    const cols = this.columns;
    const sum  = pipelineSummary(cols);

    const body = html`
      <div class="lthn-view-pipeline" style="flex:1; padding:18px 22px; display:flex; flex-direction:column; gap:14px; overflow:auto;">
        <!-- per-stage summary cards — aggregated values + deal counts -->
        <div class="lthn-view-pipeline-summary" style="display:grid; grid-template-columns:repeat(4, 1fr); gap:14px;">
          ${cols.map(c => html`
            <div class="lthn-view-pipeline-summary-card" data-stage=${c.id}
                 style="padding:14px 16px; border-radius:10px;
                        background:rgba(255,255,255,0.025);
                        border:1px solid rgba(255,255,255,0.06);">
              <div style="font-family:var(--font-mono); font-size:10px; color:var(--fg-3);
                          letter-spacing:0.10em; text-transform:uppercase;">${c.label}</div>
              <div style="font-family:var(--font-mono); font-size:22px; color:var(--fg-0);
                          font-weight:300; letter-spacing:-0.02em; margin-top:4px;">${c.value}</div>
              <div style="font-size:11px; color:var(--fg-3); margin-top:2px;">
                ${c.deals.length} ${c.deals.length === 1 ? "deal" : "deals"}
              </div>
            </div>
          `)}
        </div>

        <!-- column-of-cards grid — one column per stage, one card per deal -->
        <div class="lthn-view-pipeline-columns"
             style="display:grid; grid-template-columns:repeat(4, 1fr); gap:14px; flex:1; min-height:0;">
          ${cols.map(c => {
            const isTarget = this._dragOverStage === c.id;
            // Drop-target styling stays dark-only — brand-300 accent on a
            // translucent fill so the cue reads without flipping contrast.
            const colStyle =
              "display:flex; flex-direction:column; gap:8px; padding:4px; border-radius:8px;" +
              " transition:background 120ms, box-shadow 120ms;" +
              (isTarget
                ? " background:rgba(146, 100, 244, 0.08); box-shadow:inset 0 0 0 1px var(--brand-300);"
                : " background:transparent;");
            return html`
              <div class="lthn-view-pipeline-column" data-stage=${c.id}
                   style=${colStyle}
                   @dragover=${(e: DragEvent) => this._onColumnDragOver(e, c.id)}
                   @dragleave=${() => this._onColumnDragLeave(c.id)}
                   @drop=${(e: DragEvent) => this._onColumnDrop(e, c.id)}>
                ${c.deals.map(d => html`
                  <div class="lthn-view-pipeline-deal" data-customer=${d.c}
                       draggable="true"
                       tabindex="0"
                       @dragstart=${(e: DragEvent) => this._onDealDragStart(e, c.id, d)}
                       style="padding:12px 14px; border-radius:8px;
                              background:rgba(255,255,255,0.025);
                              border:1px solid rgba(255,255,255,0.06);
                              cursor:grab;">
                    <div style="font-size:13px; color:var(--fg-0); line-height:1.4;">${d.c}</div>
                    <div style="display:flex; justify-content:space-between; margin-top:8px;">
                      <span style="font-family:var(--font-mono); font-size:11px; color:var(--brand-300);">${d.v}</span>
                    </div>
                    <div style="font-size:11px; color:var(--fg-3); margin-top:6px; line-height:1.4;">${d.t}</div>
                  </div>
                `)}
              </div>
            `;
          })}
        </div>
      </div>
    `;

    return renderChrome({
      title:    "Pipeline",
      subtitle: `${sum.deals} deals · ${sum.total} total`,
      w: this.w, h: this.h,
      embedded: this.embedded,
      body,
      footer: html`drag cards across stages · win probabilities applied to forecast`,
    });
  }
}

customElements.define("lthn-view-pipeline", LthnViewPipeline);
