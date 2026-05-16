// SPDX-Licence-Identifier: EUPL-1.2
//
// Planning view · Today — <lthn-view-today>
//
// Daily briefing surface: three must-do focus cards, today's agenda
// (with focus-block highlighting), Vi's daily brief, what shipped
// today, and a velocity sparkline. Mounted by <lthn-app-shell> when
// the user selects Planning → Today.
//
// Backed by core/ide pkg/tasks (see go/pkg/tasks). Today's wire path
// is fixtures-only — _loadFromBackend() is the seam where Wails
// bindings will land once the backend service ships. Falls back to
// fixtures so design previews + tests stay green in the meantime.

import { LitElement, html, nothing } from "lit";
import { renderChrome } from "../../chrome";

/** A focus card — one of the three "must do today" headline items. */
interface FocusItem {
  /** Concise task title shown on the card. */
  t: string;
  /** Shape hint — drives the small icon glyph. */
  s: "ship" | "code" | "meet";
  /** Visual urgency — "high" lifts the card to brand-tinted accent. */
  urgency: "high" | "med" | "low";
  /** Human-readable due hint, e.g. "today · 16:00". */
  due: string;
}

/** A row in the agenda timeline. */
interface AgendaItem {
  /** 24h time label, e.g. "09:00". */
  time: string;
  /** Title for the slot. */
  title: string;
  /** Duration label, e.g. "15 min" / "2 h". */
  dur: string;
  /** True for deep-work / focus blocks. Quiet brand-tinted background. */
  block?: boolean;
  /** True for brand-anchor moments (releases, key meetings). Glow dot. */
  brand?: boolean;
}

/** A single shipped-today entry. */
interface ShippedItem {
  /** What landed. */
  what: string;
  /** Author handle. */
  by: string;
  /** Time of day, e.g. "14:32". */
  when: string;
}

/** Snapshot of today's data shown by the view. */
interface TodayData {
  focus:    FocusItem[];
  agenda:   AgendaItem[];
  shipped:  ShippedItem[];
  /** Velocity counters — completed-of-planned this week. */
  velocity: { done: number; planned: number; series: number[] };
}

/** Fallback fixture used when the backend isn't wired or fails. Mirrors
 *  the design reference faithfully — keep field names stable when
 *  swapping in real data. */
const TODAY_FIXTURE: TodayData = {
  focus: [
    { t: "Lethean v0.2 release prep", s: "ship", urgency: "high", due: "today · 16:00" },
    { t: "Review LoRA training PR #482", s: "code", urgency: "med",  due: "today · EOD" },
    { t: "Investor call · Calliope VC", s: "meet", urgency: "high", due: "today · 14:30" },
  ],
  agenda: [
    { time: "09:00", title: "Standup · core team",       dur: "15 min" },
    { time: "10:00", title: "Deep work · release notes", dur: "2 h", block: true },
    { time: "14:30", title: "Calliope VC · pitch",        dur: "45 min", brand: true },
    { time: "16:00", title: "Lethean v0.2 release",       dur: "1 h", brand: true },
    { time: "17:30", title: "End-of-day review",          dur: "15 min" },
  ],
  shipped: [
    { what: "MCP tools window · v0.3 → main",          by: "you",  when: "14:32" },
    { what: "Tray icon SVG family · production",        by: "Tobi", when: "11:08" },
    { what: "Benchmark window · history persistence",   by: "you",  when: "09:44" },
  ],
  velocity: { done: 14, planned: 18, series: [9, 11, 14, 12, 16, 14, 18] },
};

class LthnViewToday extends LitElement {
  static readonly properties = {
    embedded: { type: Boolean, reflect: true },
    data:     { state: true },
  };
  declare embedded: boolean;
  declare data: TodayData;

  constructor() {
    super();
    this.embedded = false;
    this.data = TODAY_FIXTURE;
  }
  createRenderRoot() { return this; }

  async connectedCallback() {
    super.connectedCallback();
    void this._loadFromBackend();
  }

  /** Async data loader. Today: a no-op that keeps the fixture in
   *  place. Tomorrow: dynamic-import a Wails binding for pkg/tasks
   *  and rebuild TodayData from real Issue rows + activity events.
   *  Public-named (single underscore) so tests can drive it without
   *  reflection tricks. */
  async _loadFromBackend(): Promise<void> {
    // Backend not wired yet (see go/pkg/tasks/wails.go — pending).
    // Keep TODAY_FIXTURE so design preview + tests stay green.
  }

  private _focusIcon(s: FocusItem["s"]): string {
    switch (s) {
      case "ship": return "fa-rocket";
      case "code": return "fa-code-pull-request";
      case "meet": return "fa-handshake";
    }
  }

  render() {
    const { focus, agenda, shipped, velocity } = this.data;
    const pct = velocity.planned > 0
      ? Math.round((velocity.done / velocity.planned) * 1000) / 10
      : 0;
    const sparkData = velocity.series.join(",");

    const body = html`
      <div style="flex:1; display:grid; grid-template-columns: 1.3fr 1fr; min-height:0; overflow:hidden;">
        <main style="padding:24px 28px; display:flex; flex-direction:column; gap:20px; overflow:auto; border-right:1px solid rgba(255,255,255,0.05);">
          <div>
            <div style="display:flex; align-items:baseline; gap:10px; margin-bottom:14px;">
              <h2 style="margin:0; font-size:24px; color:var(--fg-0); letter-spacing:-0.02em; font-weight:600;">Today's focus</h2>
              <span style="font-family:var(--font-mono); font-size:11px; color:var(--fg-3); letter-spacing:0.04em;">Friday · 16 May</span>
            </div>
            <div class="lthn-today-focus" style="display:flex; flex-direction:column; gap:8px;">
              ${focus.map(f => html`
                <div class="lthn-today-focus-card" data-urgency=${f.urgency}
                     style="display:flex; align-items:center; gap:14px; padding:14px 16px; border-radius:10px;
                            background:${f.urgency === "high" ? "rgba(64,193,197,0.06)" : "rgba(255,255,255,0.03)"};
                            border:1px solid ${f.urgency === "high" ? "rgba(64,193,197,0.22)" : "rgba(255,255,255,0.06)"};">
                  <div style="width:28px; height:28px; border-radius:8px;
                              background:rgba(64,193,197,0.10); border:1px solid rgba(64,193,197,0.20);
                              display:flex; align-items:center; justify-content:center;">
                    <i class="fa-solid ${this._focusIcon(f.s)}" style="font-size:11px; color:var(--brand-300);"></i>
                  </div>
                  <div style="flex:1;">
                    <div style="font-size:14px; color:var(--fg-0); font-weight:500;">${f.t}</div>
                    <div style="font-family:var(--font-mono); font-size:10.5px; color:var(--fg-3); margin-top:3px;">${f.due}</div>
                  </div>
                  ${f.urgency === "high" ? html`<lthn-state-pill variant="latest">High</lthn-state-pill>` : nothing}
                </div>
              `)}
            </div>
          </div>

          <div>
            <h3 style="margin:8px 0 12px; font-family:var(--font-mono); font-size:11px; color:var(--fg-3); letter-spacing:0.10em; text-transform:uppercase;">Agenda · today</h3>
            <div class="lthn-today-agenda" style="position:relative;">
              <div style="position:absolute; left:48px; top:8px; bottom:8px; width:1px; background:rgba(255,255,255,0.06);"></div>
              ${agenda.map(a => html`
                <div class="lthn-today-agenda-row" style="display:grid; grid-template-columns:48px 1fr; gap:14px; padding:8px 0; position:relative;">
                  <div style="font-family:var(--font-mono); font-size:11px; color:var(--fg-3); text-align:right;">${a.time}</div>
                  <div style="display:flex; align-items:center; gap:12px;
                              padding:10px 14px; border-radius:8px; margin-left:8px;
                              background:${a.block ? "rgba(64,193,197,0.06)" : a.brand ? "rgba(64,193,197,0.10)" : "rgba(255,255,255,0.025)"};
                              border:1px solid ${a.block ? "rgba(64,193,197,0.18)" : a.brand ? "rgba(64,193,197,0.30)" : "rgba(255,255,255,0.05)"};">
                    <div style="width:6px; height:6px; border-radius:50%;
                                background:${a.brand ? "var(--brand-400)" : a.block ? "var(--brand-300)" : "var(--fg-3)"};
                                box-shadow:${a.brand ? "0 0 6px var(--brand-400)" : "none"};
                                position:absolute; left:42px; margin-top:11px;"></div>
                    <span style="flex:1; font-size:13px; color:var(--fg-0);">${a.title}</span>
                    <span style="font-family:var(--font-mono); font-size:10.5px; color:var(--fg-3);">${a.dur}</span>
                  </div>
                </div>
              `)}
            </div>
          </div>
        </main>

        <aside style="padding:24px 26px; display:flex; flex-direction:column; gap:18px; overflow:auto;">
          <div class="lthn-today-vi-brief"
               style="padding:18px 20px; border-radius:12px;
                      background:linear-gradient(155deg, rgba(64,193,197,0.10), rgba(64,193,197,0.02));
                      border:1px solid rgba(64,193,197,0.22);">
            <div style="display:flex; align-items:center; gap:8px; margin-bottom:10px;">
              <i class="fa-solid fa-feather" style="font-size:11px; color:var(--brand-300);"></i>
              <span style="font-family:var(--font-mono); font-size:10px; color:var(--brand-300); letter-spacing:0.12em; text-transform:uppercase;">Vi · daily brief</span>
            </div>
            <p style="margin:0; font-size:14px; line-height:1.6; color:var(--fg-1);">
              Two of three release tasks already done. The investor call is your one anchor at 14:30; everything
              else is rearrangeable. <span style="color:var(--brand-300); font-style:italic;">I'd block 10:00–12:00 for the release notes.</span>
            </p>
          </div>

          <div>
            <h3 style="margin:4px 0 12px; font-family:var(--font-mono); font-size:11px; color:var(--fg-3); letter-spacing:0.10em; text-transform:uppercase;">Shipped today</h3>
            <div class="lthn-today-shipped" style="display:flex; flex-direction:column; gap:8px;">
              ${shipped.map(s => html`
                <div class="lthn-today-shipped-row"
                     style="padding:10px 12px; border-radius:8px;
                            background:rgba(255,255,255,0.025);
                            border:1px solid rgba(255,255,255,0.05);
                            display:flex; align-items:flex-start; gap:10px;">
                  <i class="fa-solid fa-check" style="color:var(--success-400); font-size:10px; margin-top:4px;"></i>
                  <div style="flex:1;">
                    <div style="font-size:12.5px; color:var(--fg-0); line-height:1.4;">${s.what}</div>
                    <div style="font-family:var(--font-mono); font-size:10.5px; color:var(--fg-3); margin-top:3px;">${s.by} · ${s.when}</div>
                  </div>
                </div>
              `)}
            </div>
          </div>

          <div class="lthn-today-velocity"
               style="padding:14px 16px; border-radius:10px;
                      background:rgba(0,0,0,0.20); border:1px solid rgba(255,255,255,0.05);">
            <div style="font-family:var(--font-mono); font-size:9.5px; color:var(--fg-3); letter-spacing:0.10em; text-transform:uppercase; margin-bottom:8px;">This week · velocity</div>
            <div style="display:flex; align-items:baseline; gap:8px;">
              <span style="font-family:var(--font-mono); font-size:28px; color:var(--fg-0); font-weight:300; letter-spacing:-0.02em;">${velocity.done}</span>
              <span style="font-size:12px; color:var(--fg-3);">/ ${velocity.planned} planned</span>
              <div style="flex:1"></div>
              <lthn-sparkline data=${sparkData} max="20" color="var(--brand-400)"></lthn-sparkline>
            </div>
            <div style="height:6px; margin-top:10px; border-radius:3px; background:rgba(255,255,255,0.06); overflow:hidden;">
              <div class="lthn-today-velocity-bar"
                   style="width:${pct}%; height:100%; background:var(--brand-400);"></div>
            </div>
            <div style="font-size:11px; color:var(--fg-3); margin-top:8px; line-height:1.5;">
              ${pct}% completion · on track for Friday's commitment.
            </div>
          </div>
        </aside>
      </div>
    `;

    return renderChrome({
      title: "Today",
      subtitle: "Friday · 16 May · 09:14",
      body,
      footer: html`${velocity.done} of ${velocity.planned} sprint tasks complete · 3 in flight · next milestone: v0.2 release · 16:00`,
      embedded: this.embedded,
    });
  }
}
customElements.define("lthn-view-today", LthnViewToday);
