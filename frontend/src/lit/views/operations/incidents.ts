// SPDX-Licence-Identifier: EUPL-1.2

// <lthn-view-incidents> — Operations view's Incident log panel.
//
// Loads live incident data from pkg/incidents.List() via the Wails
// binding. Falls back to the fixture data when the binding is
// unavailable (test environments, pre-binding builds).
//
// Vi cross-cut: a call-out at the bottom shows Vi's post-mortem
// draft offering — same voice pattern as the Mail triage and PR
// review Vi callouts in other views per HANDOVER-VIEWS.md.
//
// Embedded-aware: when `embedded` is set, renders bare body content
// for the app-shell's body slot. Standalone wraps in renderChrome().

import { LitElement, html, nothing } from "lit";
import { renderChrome } from "../../chrome";
import {
  CONFLICT_RELOAD_EVENT,
  type ConflictDetail,
} from "../../conflict-dispatch";

/** Service identifier the shared <lthn-conflict-toast> filters its
 *  reload signal by. Matches the Go-side wrapped conflict code
 *  ("incidents.update.conflict") that pkg/incidents.writeRecord
 *  emits via paths.NewConflictEnvelope on optimistic-lock loss
 *  (Cascade W3, RFC §B.3 row 8). Today only the listener side is
 *  load-bearing — incidents.ts has no in-view write call-site
 *  (Create / UpdateState / AddPostmortem are invoked from elsewhere).
 *  When an in-view Edit UI lands it MUST call
 *  handleStaleVersionConflict(result, INCIDENTS_SERVICE) on its
 *  update path. */
const INCIDENTS_SERVICE = "incidents.update";

interface IncidentEntry {
  ts:        string;
  sev:       "P1" | "P2" | "P3" | "P4";
  state:     "investigating" | "post-mortem" | "resolved";
  title:     string;
  svc:       string;
  who:       string;
  comments:  number;
  dur?:      string;
}

// Fixture matches the design's reference shape. Future v1.x: replace
// via Wails binding to `pkg/incidents.List()`.
const FIXTURE_INCIDENTS: IncidentEntry[] = [
  { ts: "now",       sev: "P3", state: "investigating", title: "hub.host.uk.com · elevated p99 latency",   svc: "hub",   who: "Mei",  comments: 3 },
  { ts: "11 d ago",  sev: "P2", state: "resolved",      title: "Mail delivery delays · upstream Postfix",   svc: "mail",  who: "you",  comments: 14, dur: "42 min" },
  { ts: "2 w ago",   sev: "P3", state: "resolved",      title: "Forge build queue stalled",                  svc: "forge", who: "Tobi", comments: 5,  dur: "18 min" },
  { ts: "3 w ago",   sev: "P1", state: "post-mortem",   title: "Total outage · DNS cache poisoning",         svc: "all",   who: "all",  comments: 38, dur: "2 h 14" },
  { ts: "5 w ago",   sev: "P2", state: "resolved",      title: "Auth token rotation broke 1 % of sessions", svc: "app",   who: "you",  comments: 9,  dur: "1 h 02" },
];

class LthnViewIncidents extends LitElement {
  static readonly properties = {
    w:        { type: Number },
    h:        { type: Number },
    embedded: { type: Boolean, reflect: true },
    incidents: { state: true },
    loading:   { state: true },
  };
  declare w: number;
  declare h: number;
  declare embedded: boolean;
  declare incidents: IncidentEntry[];
  declare loading: boolean;

  private _timer: ReturnType<typeof setInterval> | null = null;

  constructor() {
    super();
    this.w = 1180;
    this.h = 720;
    this.embedded = false;
    this.incidents = FIXTURE_INCIDENTS;
    this.loading = false;
  }
  createRenderRoot() { return this; }

  /** Cascade W3 (RFC §B.3 row 8) — listener for CONFLICT_RELOAD_EVENT
   *  dispatched by the shared <lthn-conflict-toast>. Scope-filter on
   *  INCIDENTS_SERVICE so sibling views' reload signals don't trigger
   *  our refetch. */
  private _onConflictReload = (e: Event) => {
    const ce = e as CustomEvent<ConflictDetail>;
    if (ce.detail?.service !== INCIDENTS_SERVICE) return;
    void this._loadFromBackend();
  };

  async connectedCallback() {
    super.connectedCallback();
    window.addEventListener(CONFLICT_RELOAD_EVENT, this._onConflictReload);
    await this._loadFromBackend();
    // Re-poll every 60 s — incidents change infrequently; no need to hammer.
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
      // doesn't kill the module load. Falls back to fixture on failure.
      const svc = await import("@desktop/incidents/service").catch(() => null);
      if (!svc || typeof (svc as { List?: unknown }).List !== "function") {
        return;
      }
      const r = await (svc as {
        List: (input: { state: string; limit: number }) => Promise<{
          Value?: { incidents?: IncidentEntry[] }
        }>
      }).List({ state: "", limit: 50 });
      const rows = r?.Value?.incidents;
      if (rows && rows.length > 0) {
        this.incidents = rows;
      }
    } catch {
      // Backend unavailable — keep fixture data in place.
    } finally {
      this.loading = false;
    }
  }

  private _sevColor(s: IncidentEntry["sev"]): string {
    return ({
      P1: "var(--err-400)",
      P2: "var(--warning-400)",
      P3: "var(--brand-300)",
      P4: "var(--fg-3)",
    } as Record<string, string>)[s] ?? "var(--fg-3)";
  }
  private _stateBg(s: IncidentEntry["state"]): string {
    return ({
      "investigating": "rgba(245,158,11,0.10)",
      "post-mortem":   "rgba(64,193,197,0.10)",
      "resolved":      "rgba(34,197,94,0.08)",
    } as Record<string, string>)[s] ?? "rgba(255,255,255,0.04)";
  }
  private _stateColor(s: IncidentEntry["state"]): string {
    return ({
      "investigating": "var(--warning-400)",
      "post-mortem":   "var(--brand-300)",
      "resolved":      "var(--success-400)",
    } as Record<string, string>)[s] ?? "var(--fg-3)";
  }

  _renderBody() {
    const active = this.incidents.filter(i => i.state !== "resolved").length;
    const resolved = this.incidents.filter(i => i.state === "resolved").length;
    return html`
      <div style="flex:1; padding:18px 22px 22px; overflow:auto;">
        <div style="background:rgba(255,255,255,0.025); border:1px solid rgba(255,255,255,0.06); border-radius:10px; overflow:hidden;">
          ${this.incidents.map((it, i) => html`
            <div style="padding:14px 16px; border-bottom:${i < this.incidents.length - 1 ? "1px solid rgba(255,255,255,0.04)" : "none"}; ${it.state === "investigating" ? "background:rgba(245,158,11,0.04);" : ""}">
              <div style="display:grid; grid-template-columns: 60px 1fr 130px 90px 80px; gap:14px; align-items:center;">
                <span style="font-family:var(--font-mono); font-size:14px; font-weight:600; color:${this._sevColor(it.sev)};">${it.sev}</span>
                <div style="min-width:0;">
                  <div style="font-size:13px; color:var(--fg-0); line-height:1.4;">${it.title}</div>
                  <div style="display:flex; gap:8px; margin-top:6px; font-family:var(--font-mono); font-size:10px; color:var(--fg-3);">
                    <span>${it.svc}</span><span>·</span><span>${it.who}</span><span>·</span><span>${it.comments} comments</span>
                    ${it.dur ? html`<span>·</span><span>${it.dur}</span>` : nothing}
                  </div>
                </div>
                <span style="font-family:var(--font-mono); font-size:10px; padding:3px 9px; border-radius:999px; background:${this._stateBg(it.state)}; color:${this._stateColor(it.state)}; letter-spacing:0.04em; text-align:center; white-space:nowrap;">${it.state}</span>
                <span style="font-family:var(--font-mono); font-size:11px; color:var(--fg-3);">${it.ts}</span>
                <lthn-btn tone="ghost" size="sm">Open</lthn-btn>
              </div>
            </div>
          `)}
        </div>
        <div style="margin-top:18px; padding:14px 16px; border-radius:10px; background:rgba(0,0,0,0.20); border:1px solid rgba(255,255,255,0.05); display:flex; align-items:center; gap:16px;">
          <i class="fa-solid fa-feather" style="color:var(--brand-300); font-size:13px;"></i>
          <span style="flex:1; font-size:12.5px; color:var(--fg-1); line-height:1.55;">
            <span style="font-family:var(--font-mono); color:var(--brand-300); font-size:10px; letter-spacing:0.10em; text-transform:uppercase;">Vi</span> · I drafted a post-mortem for last month's P1. Want me to email it round?
          </span>
          <lthn-btn tone="ghost" size="sm">Review draft</lthn-btn>
        </div>
        <div style="margin-top:10px; padding:0 16px; font-family:var(--font-mono); font-size:10px; color:var(--fg-3); letter-spacing:0.04em;">
          ${active} active · ${resolved} resolved · 90-day window
        </div>
      </div>
    `;
  }

  render() {
    const body = this._renderBody();
    if (this.embedded) return body;
    const active = this.incidents.filter(i => i.state !== "resolved").length;
    const resolved = this.incidents.filter(i => i.state === "resolved").length;
    return renderChrome({
      title: "Incidents",
      subtitle: `${active} active · ${resolved} resolved · 90-day`,
      w: this.w,
      h: this.h,
      body,
      footer: html`pkg/incidents (planned) · auto-paged via PagerDuty · runbooks linked from each`,
    });
  }
}
customElements.define("lthn-view-incidents", LthnViewIncidents);
