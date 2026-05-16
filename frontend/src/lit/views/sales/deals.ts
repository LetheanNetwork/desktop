// SPDX-Licence-Identifier: EUPL-1.2
// Sales · Deals — <lthn-view-deals>
//
// Deal detail surface: a single deal's header (stage / value / probability),
// an activity log (calls / emails / meetings), plus a stakeholder + docs
// rail. The reference shows the Heritage Law deal — the canonical
// "engaging stage" worked example.
//
// Monochrome treatment per HANDOVER-VIEWS.md — record-keeping, not
// coaching. No celebration toasts when an email lands. Honest log only.
//
// Two-shell: standalone (chrome) when spawned directly, embedded (no
// chrome) when mounted by <lthn-app-shell>. Mirrors the welcome /
// settings window pattern documented in design_two_shell_pattern.md.
//
// Data: fixture object only for v1. Real source is a future
// `pkg/sales/deals` Go service that maps a deal + activity timeline
// into the same { customer, headline, stage, probability, log[],
// stakeholders[], docs[] } shape codified here. See HANDOVER-VIEWS.md.

import { LitElement, html } from "lit";
import { renderChrome } from "../../chrome";
import {
  CONFLICT_RELOAD_EVENT,
  type ConflictDetail,
} from "../../conflict-dispatch";

/** Service identifier the shared <lthn-conflict-toast> filters its
 *  reload signal by. Matches the Go-side wrapped conflict code's
 *  prefix. Cascade W1 (Mantis #1540). Today only the listener side is
 *  load-bearing — deals.ts has no write call-site yet. When an Edit /
 *  AddActivity UI lands it MUST call handleStaleVersionConflict(result,
 *  DEALS_SERVICE) on its update path. */
const DEALS_SERVICE = "sales.deals.update";

/** Kind of activity-log entry. Drives the icon + alt text. */
type ActivityKind = "call" | "email" | "meet";

/** Document workflow state. The pill colour mirrors the design's
 *  brand-300 (in-progress / shared) accent. */
type DocState = "draft" | "signed" | "shared";

/** One activity-log entry. */
interface Activity {
  /** Pre-formatted timestamp ("today · 14:02", "3 d ago"). Opaque
   *  by design — source-of-truth is the rollup service, not arithmetic
   *  on the client. */
  ts: string;
  /** Kind — call / email / meet. */
  k:  ActivityKind;
  /** Initiator label ("you", "Ada P."). */
  who: string;
  /** Activity body — already prose-shaped, may include quoted reply. */
  t:  string;
}

/** One linked document. */
interface DocLink {
  /** Display title (e.g. "SOW v2 · draft"). */
  t: string;
  /** Workflow state pill. */
  s: DocState;
}

/** Full deal payload shape. */
interface Deal {
  customer:     string;
  headline:     string;
  stage:        string;
  probability:  string;
  closeTarget:  string;
  log:          Activity[];
  stakeholders: string[];
  docs:         DocLink[];
}

/** Fixture deal. When pkg/sales/deals lands, swap this for a fetch
 *  + Reactive Controller keyed on a deal-id property; the shape
 *  stays identical so the template doesn't churn. */
const FIXTURE_DEAL: Deal = {
  customer:    "Heritage Law LLP",
  headline:    "£24 K · 12-month hosted",
  stage:       "Engaging",
  probability: "60%",
  closeTarget: "14 Jun",
  log: [
    { ts: "today · 14:02", k: "call",  who: "you",     t: "30-min call with Ada Penley · walked through privacy posture, no blockers raised, sending SOW v2 by Friday." },
    { ts: "yest · 16:18",  k: "email", who: "Ada P.",  t: "Replied · 'Looking forward to seeing the proposal. We'll need to loop in our DPO.'" },
    { ts: "3 d ago",        k: "meet",  who: "you",     t: "Demo session · two partners + IT director · positive signals on on-prem story." },
    { ts: "1 w ago",        k: "email", who: "you",     t: "Sent intro deck + Q&A doc · 6 questions answered, 2 deferred to Imogen." },
  ],
  stakeholders: [
    "Ada Penley · CTO",
    "Imogen Beck · DPO",
    "Marcus Stannard · Partner",
  ],
  docs: [
    { t: "SOW v2 · draft", s: "draft"  },
    { t: "NDA · signed",   s: "signed" },
    { t: "Q&A doc",        s: "shared" },
  ],
};

/** Map an activity kind to the Font Awesome glyph used in the avatar. */
function activityIcon(k: ActivityKind): string {
  switch (k) {
    case "call":  return "fa-phone";
    case "email": return "fa-envelope";
    case "meet":  return "fa-people-group";
  }
}

class LthnViewDeals extends LitElement {
  static readonly properties = {
    w:        { type: Number },
    h:        { type: Number },
    embedded: { type: Boolean, reflect: true },
    /** Active log filter — "" = all, otherwise restrict to that kind.
     *  Drives the pill row at the top of the log section. */
    logKind:  { type: String, reflect: true },
    /** Fixture data — replace with a live backend call when
     *  pkg/sales/deals bindings land (Mantis follow-up). */
    deal:     { state: true },
  };

  declare w: number;
  declare h: number;
  declare embedded: boolean;
  declare logKind: "" | ActivityKind;
  declare deal: Deal;

  constructor() {
    super();
    this.w = 1180;
    this.h = 720;
    this.embedded = false;
    this.logKind = "";
    this.deal = FIXTURE_DEAL;
  }

  createRenderRoot() { return this; }

  /** Cascade W1 (Mantis #1540) — listener for CONFLICT_RELOAD_EVENT
   *  dispatched by the shared <lthn-conflict-toast>. Scope-filter on
   *  DEALS_SERVICE so sibling sales views' reload signals don't
   *  trigger our refetch. */
  private _onConflictReload = (e: Event) => {
    const ce = e as CustomEvent<ConflictDetail>;
    if (ce.detail?.service !== DEALS_SERVICE) return;
    void this._loadFromBackend();
  };

  async connectedCallback() {
    super.connectedCallback();
    window.addEventListener(CONFLICT_RELOAD_EVENT, this._onConflictReload);
    await this._loadFromBackend();
  }

  disconnectedCallback() {
    super.disconnectedCallback();
    window.removeEventListener(CONFLICT_RELOAD_EVENT, this._onConflictReload);
  }

  /** Pull the first available deal from pkg/sales/deals (shipped in
   *  63062aa). v1 shows whichever deal Service.List returns first;
   *  multi-deal browsing is a follow-up (Mantis #1478). Degrades to
   *  FIXTURE_DEAL on binding-missing or rejection. */
  async _loadFromBackend(): Promise<void> {
    try {
      const svc = await import("@desktop/sales/deals/service").catch(() => null);
      if (!svc || typeof (svc as { List?: unknown }).List !== "function") {
        return;
      }
      const r = await (svc as {
        List: (input: { stage: string; owner: string; limit: number }) => Promise<{
          Value?: { deals?: Deal[] }
        }>
      }).List({ stage: "", owner: "", limit: 1 });
      const deals = r?.Value?.deals;
      if (deals && deals.length > 0) {
        this.deal = deals[0];
      }
    } catch {
      // Backend unavailable — keep fixture deal.
    }
  }

  /** Filter the activity log by the active kind pill. Pure helper so
   *  the unit test can exercise the filter without mounting the DOM. */
  _filteredLog(): Activity[] {
    if (!this.logKind) return this.deal.log;
    return this.deal.log.filter(a => a.k === this.logKind);
  }

  /** Toggle a log-kind filter — clicking the active pill clears it. */
  _toggleKind(k: ActivityKind) {
    this.logKind = this.logKind === k ? "" : k;
  }

  render() {
    const d   = this.deal;
    const log = this._filteredLog();

    const kindBtn = (k: ActivityKind, label: string) => html`
      <lthn-btn class="lthn-view-deals-kind"
                data-kind=${k}
                tone="ghost" size="sm"
                ?active=${this.logKind === k}
                @click=${() => this._toggleKind(k)}>
        <i class="fa-solid ${activityIcon(k)}" style="font-size:10px;"></i>
        ${label}
      </lthn-btn>
    `;

    const body = html`
      <div class="lthn-view-deals" style="flex:1; display:grid; grid-template-columns: 1.5fr 1fr; min-height:0;">
        <!-- main: deal header + activity log -->
        <main class="lthn-view-deals-main"
              style="padding:22px 26px; overflow:auto; display:flex; flex-direction:column; gap:18px;
                     border-right:1px solid rgba(255,255,255,0.05);">
          <div class="lthn-view-deals-header">
            <div style="font-family:var(--font-mono); font-size:10px; color:var(--brand-300);
                        letter-spacing:0.10em; text-transform:uppercase;">Deal · ${d.customer}</div>
            <h2 style="margin:6px 0 8px; font-size:24px; color:var(--fg-0); letter-spacing:-0.02em; font-weight:600;">
              ${d.headline}
            </h2>
            <div style="display:flex; align-items:center; gap:10px;">
              <lthn-state-pill variant="latest">${d.stage}</lthn-state-pill>
              <span style="font-family:var(--font-mono); font-size:11px; color:var(--fg-3);">
                Probability ${d.probability} · close target ${d.closeTarget}
              </span>
            </div>
          </div>

          <div class="lthn-view-deals-log">
            <div style="display:flex; align-items:center; gap:10px; margin-bottom:10px;">
              <div style="font-family:var(--font-mono); font-size:10px; color:var(--fg-3);
                          letter-spacing:0.10em; text-transform:uppercase;">Activity</div>
              <div style="flex:1"></div>
              ${kindBtn("call",  "calls")}
              ${kindBtn("email", "emails")}
              ${kindBtn("meet",  "meetings")}
            </div>
            <div style="display:flex; flex-direction:column; gap:12px;">
              ${log.length === 0 ? html`
                <div style="padding:18px; text-align:center; font-size:12px; color:var(--fg-3);
                            background:rgba(255,255,255,0.02); border:1px solid rgba(255,255,255,0.05);
                            border-radius:8px;">
                  No ${this.logKind} entries.
                </div>
              ` : log.map(l => html`
                <div class="lthn-view-deals-log-entry" data-kind=${l.k}
                     style="display:grid; grid-template-columns: 28px 1fr 110px; gap:12px;
                            align-items:flex-start;">
                  <div style="width:24px; height:24px; border-radius:6px;
                              background:rgba(64,193,197,0.10);
                              border:1px solid rgba(64,193,197,0.22);
                              display:flex; align-items:center; justify-content:center;">
                    <i class="fa-solid ${activityIcon(l.k)}"
                       style="font-size:10px; color:var(--brand-300);"></i>
                  </div>
                  <div>
                    <div style="font-size:12.5px; color:var(--fg-1); line-height:1.55;">${l.t}</div>
                    <div style="font-family:var(--font-mono); font-size:10px; color:var(--fg-3); margin-top:4px;">${l.who}</div>
                  </div>
                  <span style="font-family:var(--font-mono); font-size:10.5px; color:var(--fg-3); text-align:right;">${l.ts}</span>
                </div>
              `)}
            </div>
          </div>
        </main>

        <!-- aside: stakeholders + documents -->
        <aside class="lthn-view-deals-aside"
               style="padding:22px 24px; overflow:auto; display:flex; flex-direction:column; gap:14px;">
          <div style="font-family:var(--font-mono); font-size:10px; color:var(--fg-3);
                      letter-spacing:0.10em; text-transform:uppercase;">Stakeholders</div>
          ${d.stakeholders.map(s => html`
            <div class="lthn-view-deals-stakeholder"
                 style="padding:10px 12px; border-radius:8px;
                        background:rgba(255,255,255,0.025);
                        border:1px solid rgba(255,255,255,0.06);
                        font-size:12.5px; color:var(--fg-1);">${s}</div>
          `)}

          <div style="height:1px; background:rgba(255,255,255,0.05);"></div>

          <div style="font-family:var(--font-mono); font-size:10px; color:var(--fg-3);
                      letter-spacing:0.10em; text-transform:uppercase;">Documents</div>
          ${d.docs.map(doc => html`
            <div class="lthn-view-deals-doc" data-state=${doc.s}
                 style="display:flex; align-items:center; gap:10px; padding:10px 12px; border-radius:8px;
                        background:rgba(255,255,255,0.025);
                        border:1px solid rgba(255,255,255,0.06);">
              <i class="fa-regular fa-file" style="font-size:11px; color:var(--fg-3);"></i>
              <span style="flex:1; font-size:12.5px; color:var(--fg-1);">${doc.t}</span>
              <span style="font-family:var(--font-mono); font-size:10px; color:var(--brand-300);">${doc.s}</span>
            </div>
          `)}
        </aside>
      </div>
    `;

    return renderChrome({
      title:    `Deal · ${d.customer}`,
      subtitle: `${d.stage.toLowerCase()} · ${d.headline.split(" · ")[0]}`,
      w: this.w, h: this.h,
      embedded: this.embedded,
      body,
      footer: html`auto-logged from email + calendar · ⌘L to log a call · sensitive info encrypted at rest`,
    });
  }
}

customElements.define("lthn-view-deals", LthnViewDeals);
