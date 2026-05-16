// SPDX-Licence-Identifier: EUPL-1.2
// Marketing · Campaigns — <lthn-view-campaigns>
//
// Live + scheduled + draft campaign threads with reach / convert / spend.
// Monochrome treatment per HANDOVER-VIEWS.md — no growth-hack greenery,
// no celebration toasts, no leaderboards. Calm + observational + dense.
//
// Two-shell: standalone (with chrome) when spawned directly, embedded
// (no chrome) when mounted by <lthn-app-shell>. Mirrors the welcome /
// settings window pattern.
//
// Data: fixture array only for v1. Real source is a future
// `pkg/marketing/campaigns` Go service that maps a CRM + UTM rollup
// into the same { name, state, reach, convert, spend, channel } shape
// the design intentionally codifies. See HANDOVER-VIEWS.md.

import { LitElement, html } from "lit";
import { renderChrome } from "../../chrome";

interface Campaign {
  name: string;
  state: "live" | "scheduled" | "draft" | "complete";
  reach: string;
  convert: string;
  spend: string;
  channel: string;
}

/** Fixture campaigns. Shape mirrors the future pkg/marketing/campaigns
 *  Go service — when that ships, swap this for a fetch + Reactive
 *  Controller (mirroring chat-window patterns). Strings stay opaque
 *  (e.g. "42 K", "£800") because the source-of-truth is the rollup
 *  service, not arithmetic on the client. */
const FIXTURE_CAMPAIGNS: Campaign[] = [
  { name: "v0.2 launch · 'sovereign compute'", state: "live",      reach: "42 K",    convert: "3.2%", spend: "£0",    channel: "earned" },
  { name: "Investor outreach · Q2 cohort",     state: "live",      reach: "38",      convert: "21%",  spend: "£0",    channel: "direct" },
  { name: "Host UK · email re-engage",         state: "scheduled", reach: "4.2 K",   convert: "—",    spend: "£0",    channel: "email" },
  { name: "Reddit AMA · r/LocalLLaMA",         state: "scheduled", reach: "~180 K",  convert: "—",    spend: "£0",    channel: "earned" },
  { name: "Lethean dev-rel sponsorship",       state: "draft",     reach: "—",       convert: "—",    spend: "£800",  channel: "paid" },
  { name: "Q1 retrospective · post-mortem",    state: "complete",  reach: "18 K",    convert: "1.8%", spend: "£0",    channel: "earned" },
];

const STATE_COLOUR: Record<Campaign["state"], string> = {
  live:      "var(--success-400)",
  scheduled: "var(--brand-300)",
  draft:     "var(--fg-3)",
  complete:  "var(--fg-2)",
};

class LthnViewCampaigns extends LitElement {
  static readonly properties = {
    w:        { type: Number },
    h:        { type: Number },
    embedded: { type: Boolean, reflect: true },
  };
  declare w: number;
  declare h: number;
  declare embedded: boolean;

  constructor() {
    super();
    this.w = 1180;
    this.h = 720;
    this.embedded = false;
  }

  createRenderRoot() { return this; }

  render() {
    const items = FIXTURE_CAMPAIGNS;
    const liveCount      = items.filter(i => i.state === "live").length;
    const scheduledCount = items.filter(i => i.state === "scheduled").length;

    const summary = [
      { l: "Live",         v: String(liveCount),       c: "var(--success-400)" },
      { l: "Scheduled",    v: String(scheduledCount),  c: "var(--brand-300)" },
      { l: "Reach · 30d",  v: "82 K",                  c: "var(--fg-0)" },
      { l: "Spend · 30d",  v: "£800",                  c: "var(--fg-0)" },
    ];

    const body = html`
      <div class="lthn-view-campaigns" style="flex:1; padding:18px 22px; overflow:auto; display:flex; flex-direction:column; gap:18px;">
        <div style="display:grid; grid-template-columns: repeat(4, 1fr); gap:12px;">
          ${summary.map(k => html`
            <div style="padding:14px 16px; border-radius:10px; background:rgba(255,255,255,0.025); border:1px solid rgba(255,255,255,0.06);">
              <div style="font-family:var(--font-mono); font-size:9.5px; color:var(--fg-3); letter-spacing:0.10em; text-transform:uppercase;">${k.l}</div>
              <div style="font-family:var(--font-mono); font-size:26px; color:${k.c}; font-weight:300; letter-spacing:-0.02em; margin-top:6px;">${k.v}</div>
            </div>
          `)}
        </div>
        <div class="lthn-view-campaigns-list" style="background:rgba(255,255,255,0.025); border:1px solid rgba(255,255,255,0.06); border-radius:10px; overflow:hidden;">
          ${items.map((it, i) => html`
            <div class="lthn-view-campaigns-row" data-state=${it.state} style="display:grid; grid-template-columns: 1fr 100px 80px 100px 80px 80px; gap:14px; padding:14px 16px; border-bottom:${i < items.length - 1 ? "1px solid rgba(255,255,255,0.04)" : "none"}; align-items:center;">
              <div>
                <div style="font-size:13px; color:var(--fg-0);">${it.name}</div>
                <div style="font-family:var(--font-mono); font-size:10px; color:var(--fg-3); margin-top:4px;">${it.channel}</div>
              </div>
              <span style="font-family:var(--font-mono); font-size:10.5px; padding:3px 9px; border-radius:999px; background:${STATE_COLOUR[it.state]}1a; color:${STATE_COLOUR[it.state]}; letter-spacing:0.04em; text-align:center;">${it.state}</span>
              <span style="font-family:var(--font-mono); font-size:11px; color:var(--fg-1); text-align:right;">${it.reach}</span>
              <span style="font-family:var(--font-mono); font-size:11px; color:${it.convert === "—" ? "var(--fg-3)" : "var(--brand-300)"}; text-align:right;">${it.convert}</span>
              <span style="font-family:var(--font-mono); font-size:11px; color:var(--fg-2); text-align:right;">${it.spend}</span>
              <lthn-btn tone="ghost" size="sm">Open</lthn-btn>
            </div>
          `)}
        </div>
      </div>
    `;

    return renderChrome({
      title:    "Campaigns",
      subtitle: `${items.length} active threads`,
      w: this.w, h: this.h,
      embedded: this.embedded,
      body,
      footer: html`${items.length} campaigns · earned-media heavy · UTM-tracked · numbers refresh hourly`,
    });
  }
}

customElements.define("lthn-view-campaigns", LthnViewCampaigns);
