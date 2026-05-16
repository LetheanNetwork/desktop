// SPDX-Licence-Identifier: EUPL-1.2
//
// Planning view · Calendar — <lthn-view-calendar>
//
// Five-day week grid with hour rows. Events render as coloured tiles
// rooted at (day, hour) with a duration span. Tones: focus / brand /
// ship / mute. Today's column is brand-tinted in the day header.
//
// Reused by Office view as well — see HANDOVER-VIEWS.md. Backed by
// pkg/tasks calendar block events when they ship; today fixtures-only.

import { LitElement, html, nothing } from "lit";
import { renderChrome } from "../../chrome";

type Tone = "focus" | "brand" | "ship" | "mute";

/** A calendar event placed on the day × hour grid. */
interface CalEvent {
  /** Day index (0..4 — Mon..Fri). */
  d: number;
  /** Hour index (1-based, against the `hours` array). */
  h: number;
  /** Span in hour rows. */
  dur: number;
  /** Event title. */
  title: string;
  /** Visual tone — drives the colour tuple. */
  tone: Tone;
}

interface CalendarData {
  /** Display week label, e.g. "Week of 12 May". */
  weekLabel: string;
  /** Day labels left → right. */
  days:   string[];
  /** Day-of-month shown under each day label. Same length as `days`. */
  dates:  number[];
  /** Index into `days` of the highlighted "today" column. */
  today:  number;
  /** Hour labels top → bottom. */
  hours:  string[];
  /** Events placed on the grid. */
  events: CalEvent[];
}

const CAL_FIXTURE: CalendarData = {
  weekLabel: "Week of 12 May",
  days:  ["Mon", "Tue", "Wed", "Thu", "Fri"],
  dates: [12, 13, 14, 15, 16],
  today: 4,
  hours: ["09", "10", "11", "12", "13", "14", "15", "16", "17"],
  events: [
    { d: 0, h: 1, dur: 1, title: "Standup",                 tone: "mute" },
    { d: 0, h: 2, dur: 2, title: "Deep work · model browser", tone: "focus" },
    { d: 0, h: 5, dur: 1, title: "Calliope VC call",         tone: "brand" },
    { d: 0, h: 7, dur: 1, title: "v0.2 release",             tone: "ship" },
    { d: 1, h: 1, dur: 1, title: "Standup",                 tone: "mute" },
    { d: 1, h: 2, dur: 3, title: "Deep work · LoRA wizard",  tone: "focus" },
    { d: 1, h: 6, dur: 1, title: "Pair · Linux packaging",   tone: "mute" },
    { d: 2, h: 1, dur: 1, title: "Standup",                 tone: "mute" },
    { d: 2, h: 3, dur: 2, title: "Sprint 24 planning",       tone: "brand" },
    { d: 3, h: 1, dur: 1, title: "Standup",                 tone: "mute" },
    { d: 3, h: 2, dur: 3, title: "Deep work · release notes", tone: "focus" },
    { d: 4, h: 1, dur: 1, title: "Standup",                 tone: "mute" },
    { d: 4, h: 4, dur: 2, title: "Retro · Sprint 24",        tone: "brand" },
    { d: 4, h: 7, dur: 1, title: "Friday demo",              tone: "ship" },
  ],
};

/** Resolve the colour triple for a tone — exported so the test suite
 *  can pin tile-background invariants. */
export function eventColours(tone: Tone): [string, string, string] {
  switch (tone) {
    case "focus": return ["rgba(64,193,197,0.08)", "rgba(64,193,197,0.22)", "var(--brand-300)"];
    case "brand": return ["rgba(64,193,197,0.18)", "rgba(64,193,197,0.40)", "var(--brand-300)"];
    case "ship":  return ["rgba(245,158,11,0.10)", "rgba(245,158,11,0.30)", "var(--warning-400)"];
    case "mute":  return ["rgba(255,255,255,0.03)","rgba(255,255,255,0.07)","var(--fg-2)"];
  }
}

/** Count focus blocks. Pure helper used by header summary + tests. */
export function countFocusBlocks(events: readonly CalEvent[]): number {
  return events.filter(e => e.tone === "focus").length;
}

class LthnViewCalendar extends LitElement {
  static readonly properties = {
    embedded: { type: Boolean, reflect: true },
    data:     { state: true },
  };
  declare embedded: boolean;
  declare data: CalendarData;

  constructor() {
    super();
    this.embedded = false;
    this.data = CAL_FIXTURE;
  }
  createRenderRoot() { return this; }

  async connectedCallback() {
    super.connectedCallback();
    void this._loadFromBackend();
  }

  /** Pending pkg/tasks Wails binding. Today: keep fixture. */
  async _loadFromBackend(): Promise<void> { /* no-op */ }

  render() {
    const { days, dates, today, hours, events, weekLabel } = this.data;
    const cols = `48px repeat(${days.length}, 1fr)`;
    const focusCount = countFocusBlocks(events);

    const body = html`
      <div style="flex:1; display:flex; flex-direction:column; min-height:0; overflow:hidden;">
        <div style="padding:18px 22px 12px; display:flex; align-items:center; gap:12px;">
          <h2 style="margin:0; font-size:22px; color:var(--fg-0); letter-spacing:-0.02em; font-weight:600;">${weekLabel}</h2>
          <span style="font-family:var(--font-mono); font-size:11px; color:var(--fg-3);">${events.length} events · ${focusCount} focus blocks</span>
          <div style="flex:1"></div>
          <lthn-btn tone="ghost" size="sm"><i class="fa-solid fa-chevron-left" style="font-size:9px;"></i></lthn-btn>
          <lthn-btn tone="ghost" size="sm">Today</lthn-btn>
          <lthn-btn tone="ghost" size="sm"><i class="fa-solid fa-chevron-right" style="font-size:9px;"></i></lthn-btn>
        </div>

        <div style="flex:1; overflow:auto; padding:0 22px 22px; min-height:0;">
          <div class="lthn-calendar-grid" style="display:grid; grid-template-columns: ${cols}; gap:8px;">
            <div></div>
            ${days.map((d, i) => html`
              <div class="lthn-calendar-day-header" data-today=${i === today ? "true" : "false"}
                   style="padding:8px 12px; border-bottom:1px solid rgba(255,255,255,0.06); text-align:center;">
                <div style="font-family:var(--font-mono); font-size:10px; color:var(--fg-3); letter-spacing:0.10em; text-transform:uppercase;">${d}</div>
                <div style="font-size:18px; color:${i === today ? "var(--brand-300)" : "var(--fg-0)"}; font-weight:${i === today ? 600 : 400}; margin-top:2px;">${dates[i]}</div>
              </div>
            `)}
            ${hours.map((h, hi) => html`
              <div style="font-family:var(--font-mono); font-size:9.5px; color:var(--fg-3); text-align:right; padding-top:8px; padding-right:4px;">${h}:00</div>
              ${days.map((_, di) => {
                const evt = events.find(e => e.d === di && e.h === hi + 1);
                return html`
                  <div class="lthn-calendar-cell" data-day=${di} data-hour=${hi + 1}
                       style="min-height:48px; border-top:1px dashed rgba(255,255,255,0.04);
                              padding:3px; position:relative;">
                    ${evt ? this._renderEvent(evt) : nothing}
                  </div>
                `;
              })}
            `)}
          </div>
        </div>
      </div>
    `;

    return renderChrome({
      title: "Calendar",
      subtitle: weekLabel,
      body,
      footer: html`focus blocks · ⌘B to add · ⌘shift+R to find free hour · syncs locally only`,
      embedded: this.embedded,
    });
  }

  private _renderEvent(evt: CalEvent) {
    const [bg, bd, fg] = eventColours(evt.tone);
    return html`
      <div class="lthn-calendar-event" data-tone=${evt.tone}
           style="position:absolute; left:3px; right:3px; top:3px; height:${evt.dur * 48 - 6}px;
                  background:${bg}; border:1px solid ${bd}; border-left:2px solid ${fg};
                  border-radius:6px; padding:6px 8px; overflow:hidden;">
        <div style="font-size:11.5px; color:var(--fg-0); font-weight:500; line-height:1.3; white-space:nowrap; overflow:hidden; text-overflow:ellipsis;">${evt.title}</div>
        <div style="font-family:var(--font-mono); font-size:9.5px; color:${fg}; margin-top:3px;">${evt.dur}h</div>
      </div>
    `;
  }
}
customElements.define("lthn-view-calendar", LthnViewCalendar);
