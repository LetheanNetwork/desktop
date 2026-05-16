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
// Data: _loadFromBackend() tries pkg/marketing/campaigns via the Wails
// binding (lazy-imported, falls back to fixture when the binding is absent
// — e.g. in tests or pre-Wails contexts). The binding returns
// { Value: { campaigns: Campaign[], liveCount: number, scheduledCount: number } }.
// On failure the fixture stays in place so the view is always renderable.

import { LitElement, html } from "lit";
import { renderChrome } from "../../chrome";
import {
  CONFLICT_RELOAD_EVENT,
  type ConflictDetail,
} from "../../conflict-dispatch";

/** Service identifier the shared <lthn-conflict-toast> filters its
 *  reload signal by. Matches the Go-side wrapped conflict code
 *  ("campaigns.update.conflict") that pkg/marketing/campaigns.writeCampaign
 *  emits via paths.NewConflictEnvelope on optimistic-lock loss
 *  (Cascade W2, RFC §B.3 row 7). Today only the listener side is
 *  load-bearing — campaigns.ts has no write call-site (Create + Update
 *  are invoked from elsewhere). When an in-view Edit UI lands it MUST
 *  call handleStaleVersionConflict(result, CAMPAIGNS_SERVICE) on its
 *  update path. */
const CAMPAIGNS_SERVICE = "campaigns.update";

interface Campaign {
  name: string;
  state: "live" | "scheduled" | "draft" | "complete";
  reach: string;
  convert: string;
  spend: string;
  channel: string;
}

/** Fixture campaigns. Shape mirrors the pkg/marketing/campaigns Go service.
 *  When the backend resolves, this array is replaced by the live list.
 *  Strings stay opaque ("42 K", "£800") — arithmetic is the backend's job. */
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
    w:         { type: Number },
    h:         { type: Number },
    embedded:  { type: Boolean, reflect: true },
    campaigns: { state: true },
    loading:   { state: true },
  };
  declare w: number;
  declare h: number;
  declare embedded: boolean;
  declare campaigns: Campaign[];
  declare loading: boolean;

  private _timer: ReturnType<typeof setInterval> | null = null;

  constructor() {
    super();
    this.w = 1180;
    this.h = 720;
    this.embedded = false;
    this.campaigns = FIXTURE_CAMPAIGNS;
    this.loading = false;
  }

  createRenderRoot() { return this; }

  /** Cascade W2 (RFC §B.3 row 7) — listener for CONFLICT_RELOAD_EVENT
   *  dispatched by the shared <lthn-conflict-toast>. Scope-filter on
   *  CAMPAIGNS_SERVICE so sibling marketing views' reload signals don't
   *  trigger our refetch. */
  private _onConflictReload = (e: Event) => {
    const ce = e as CustomEvent<ConflictDetail>;
    if (ce.detail?.service !== CAMPAIGNS_SERVICE) return;
    void this._loadFromBackend();
  };

  async connectedCallback() {
    super.connectedCallback();
    window.addEventListener(CONFLICT_RELOAD_EVENT, this._onConflictReload);
    await this._loadFromBackend();
    // Re-poll every 60 s — campaigns change infrequently.
    this._timer = setInterval(() => { void this._loadFromBackend(); }, 60_000);
  }

  disconnectedCallback() {
    if (this._timer) { clearInterval(this._timer); this._timer = null; }
    window.removeEventListener(CONFLICT_RELOAD_EVENT, this._onConflictReload);
    super.disconnectedCallback();
  }

  async _loadFromBackend() {
    if (this.loading) return;
    this.loading = true;
    try {
      // Lazy-import the Wails binding so a missing binding in tests
      // (or pre-Wails builds) doesn't kill the module load.
      const svc = await import("@desktop/marketing/campaigns/service").catch(() => null);
      if (!svc || typeof (svc as { List?: unknown }).List !== "function") {
        return;
      }
      const r = await (svc as {
        List: (input: { state: string }) => Promise<{
          Value?: { campaigns?: Campaign[]; liveCount?: number; scheduledCount?: number }
        }>
      }).List({ state: "" });
      const rows = r?.Value?.campaigns;
      if (rows && rows.length > 0) {
        this.campaigns = rows;
      }
    } catch {
      // Backend unavailable — keep fixture data in place.
    } finally {
      this.loading = false;
    }
  }

  render() {
    const items = this.campaigns;
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
