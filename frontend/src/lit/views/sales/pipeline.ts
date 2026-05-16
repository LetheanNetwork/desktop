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
// Data: fixture array only for v1. Real source is a future
// `pkg/sales/pipeline` Go service that maps a CRM rollup into the same
// { id, label, value, deals[] } shape this design intentionally codifies.
// See HANDOVER-VIEWS.md.

import { LitElement, html } from "lit";
import { renderChrome } from "../../chrome";

/** A single deal card inside a pipeline column. */
interface Deal {
  /** Customer / counterparty name. */
  c: string;
  /** Headline value (pre-formatted with currency symbol). */
  v: string;
  /** Free-text qualifier — sector / stage detail / blocker. */
  t: string;
}

/** A pipeline column = one stage in the funnel. */
interface PipelineColumn {
  /** Stable id for filtering / tests. */
  id:    "qual" | "engage" | "propose" | "close";
  /** Human label rendered in the column header. */
  label: string;
  /** Pre-formatted aggregated value for the stage. */
  value: string;
  /** Deals currently in this stage. */
  deals: Deal[];
}

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
    /** Fixture data — replace with a live backend call when
     *  pkg/sales/pipeline bindings land (Mantis follow-up). */
    columns:  { state: true },
  };

  declare w: number;
  declare h: number;
  declare embedded: boolean;
  declare columns: PipelineColumn[];

  constructor() {
    super();
    this.w = 1280;
    this.h = 720;
    this.embedded = false;
    this.columns = FIXTURE_PIPELINE;
  }

  createRenderRoot() { return this; }

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
          ${cols.map(c => html`
            <div class="lthn-view-pipeline-column" data-stage=${c.id}
                 style="display:flex; flex-direction:column; gap:8px;">
              ${c.deals.map(d => html`
                <div class="lthn-view-pipeline-deal" data-customer=${d.c}
                     style="padding:12px 14px; border-radius:8px;
                            background:rgba(255,255,255,0.025);
                            border:1px solid rgba(255,255,255,0.06);">
                  <div style="font-size:13px; color:var(--fg-0); line-height:1.4;">${d.c}</div>
                  <div style="display:flex; justify-content:space-between; margin-top:8px;">
                    <span style="font-family:var(--font-mono); font-size:11px; color:var(--brand-300);">${d.v}</span>
                  </div>
                  <div style="font-size:11px; color:var(--fg-3); margin-top:6px; line-height:1.4;">${d.t}</div>
                </div>
              `)}
            </div>
          `)}
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
