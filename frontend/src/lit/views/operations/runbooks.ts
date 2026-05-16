// SPDX-Licence-Identifier: EUPL-1.2

// <lthn-view-runbooks> — Operations view's Runbook library panel.
//
// v1 fixture data. Future: backs into a `pkg/runbooks` Go service
// that scans `~/Lethean/runbooks/` markdown files, parses the
// last-rehearsed header, and surfaces stale ones for review. The
// header schema lives at `plans/code/lthn/desktop/RFC.runbooks.md`
// (to be drafted; tracking ticket post-Phase 2).
//
// Right rail: library health summary (fresh / aging / stale counts)
// + a "schedule rehearsals" call-out for the stale ones. Same
// design as the Incidents Vi-callout — calm, observational, no shame.
//
// Embedded-aware: when `embedded` is set, renders bare body content
// for the app-shell's body slot. Standalone wraps in renderChrome().

import { LitElement, html } from "lit";
import { renderChrome } from "../../chrome";

interface RunbookEntry {
  id:     string;
  title:  string;
  area:   string;
  last:   string;
  health: "fresh" | "aging" | "stale";
}

// Fixture matches the design's reference shape.
const FIXTURE_BOOKS: RunbookEntry[] = [
  { id: "R-01", title: "Rotate runtime API keys",                area: "auth",    last: "2 d",   health: "fresh" },
  { id: "R-02", title: "Recover from corrupt model directory",   area: "runtime", last: "3 w",   health: "fresh" },
  { id: "R-03", title: "Rollback bad deploy · production",       area: "deploy",  last: "5 d",   health: "fresh" },
  { id: "R-04", title: "Reset DNS · cache poisoning incident",   area: "network", last: "2 w",   health: "fresh" },
  { id: "R-05", title: "Drain a stuck Postfix queue",            area: "mail",    last: "4 mo",  health: "stale" },
  { id: "R-06", title: "Trigger emergency model unload",         area: "runtime", last: "1 mo",  health: "aging" },
  { id: "R-07", title: "Restore tray-app from corrupt config",   area: "client",  last: "6 mo",  health: "stale" },
];

class LthnViewRunbooks extends LitElement {
  static readonly properties = {
    w:        { type: Number },
    h:        { type: Number },
    embedded: { type: Boolean, reflect: true },
    books:    { state: true },
    filter:   { state: true },
  };
  declare w: number;
  declare h: number;
  declare embedded: boolean;
  declare books: RunbookEntry[];
  declare filter: string;

  constructor() {
    super();
    this.w = 1180;
    this.h = 720;
    this.embedded = false;
    this.books = FIXTURE_BOOKS;
    this.filter = "";
  }
  createRenderRoot() { return this; }

  private _healthColor(h: RunbookEntry["health"]): string {
    return ({
      fresh: "var(--success-400)",
      aging: "var(--warning-400)",
      stale: "var(--err-400)",
    } as Record<string, string>)[h] ?? "var(--fg-3)";
  }

  private _visible(): RunbookEntry[] {
    const q = this.filter.trim().toLowerCase();
    if (!q) return this.books;
    return this.books.filter(b =>
      b.title.toLowerCase().includes(q) ||
      b.area.toLowerCase().includes(q) ||
      b.id.toLowerCase().includes(q),
    );
  }

  private _counts() {
    return {
      fresh: this.books.filter(b => b.health === "fresh").length,
      aging: this.books.filter(b => b.health === "aging").length,
      stale: this.books.filter(b => b.health === "stale").length,
    };
  }

  private _stale(): RunbookEntry[] {
    return this.books.filter(b => b.health === "stale");
  }

  _onFilterInput = (e: Event) => {
    const target = e.target as HTMLInputElement | null;
    this.filter = target?.value ?? "";
  };

  _renderBody() {
    const visible = this._visible();
    const counts = this._counts();
    const stale = this._stale();
    return html`
      <div style="flex:1; display:grid; grid-template-columns: 1fr 320px; min-height:0;">
        <main style="padding:18px 22px 22px; overflow:auto;">
          <div style="display:flex; align-items:center; gap:9px; height:32px; padding:0 12px;
                      background:rgba(255,255,255,0.04); border:1px solid rgba(255,255,255,0.07);
                      border-radius:8px; margin-bottom:14px;">
            <i class="fa-solid fa-magnifying-glass" style="font-size:11px; color:var(--fg-3);"></i>
            <input
              type="text"
              .value=${this.filter}
              @input=${this._onFilterInput}
              placeholder="runtime · postfix"
              style="flex:1; background:transparent; border:none; outline:none;
                     font-size:12.5px; color:var(--fg-1); font-family:var(--font-sans);" />
            <span style="font-family:var(--font-mono); font-size:10px; color:var(--fg-3);">${this.books.length} runbooks · ${visible.length} shown</span>
          </div>
          <div style="background:rgba(255,255,255,0.025); border:1px solid rgba(255,255,255,0.06); border-radius:10px; overflow:hidden;">
            ${visible.map((b, i) => html`
              <div style="padding:13px 16px; border-bottom:${i < visible.length - 1 ? "1px solid rgba(255,255,255,0.04)" : "none"}; display:grid; grid-template-columns: 60px 1fr 100px 90px 60px; gap:14px; align-items:center;">
                <span style="font-family:var(--font-mono); font-size:11px; color:var(--fg-3);">${b.id}</span>
                <span style="font-size:13px; color:var(--fg-0);">${b.title}</span>
                <span style="font-family:var(--font-mono); font-size:10.5px; color:var(--fg-2);">${b.area}</span>
                <div style="display:flex; align-items:center; gap:6px;">
                  <span style="width:6px; height:6px; border-radius:50%; background:${this._healthColor(b.health)};"></span>
                  <span style="font-family:var(--font-mono); font-size:10.5px; color:var(--fg-3);">${b.last}</span>
                </div>
                <lthn-btn tone="ghost" size="sm">Run</lthn-btn>
              </div>
            `)}
          </div>
        </main>
        <aside style="background:rgba(0,0,0,0.18); border-left:1px solid rgba(255,255,255,0.05); padding:18px 22px; display:flex; flex-direction:column; gap:16px; overflow:auto;">
          <div style="font-family:var(--font-mono); font-size:10px; color:var(--fg-3); letter-spacing:0.10em; text-transform:uppercase;">Library health</div>
          <div style="display:grid; grid-template-columns: 1fr 1fr 1fr; gap:8px;">
            ${[
              { label: "fresh", v: counts.fresh, c: "var(--success-400)" },
              { label: "aging", v: counts.aging, c: "var(--warning-400)" },
              { label: "stale", v: counts.stale, c: "var(--err-400)" },
            ].map(s => html`
              <div style="padding:10px 12px; border-radius:8px; background:rgba(255,255,255,0.025); border:1px solid rgba(255,255,255,0.06);">
                <div style="font-family:var(--font-mono); font-size:22px; color:${s.c}; font-weight:300;">${s.v}</div>
                <div style="font-family:var(--font-mono); font-size:9.5px; color:var(--fg-3); letter-spacing:0.08em; text-transform:uppercase; margin-top:2px;">${s.label}</div>
              </div>
            `)}
          </div>
          ${stale.length > 0 ? html`
            <div style="padding:14px 16px; border-radius:10px; background:rgba(245,158,11,0.06); border:1px solid rgba(245,158,11,0.22);">
              <div style="font-family:var(--font-mono); font-size:10px; color:var(--warning-400); letter-spacing:0.10em; text-transform:uppercase; margin-bottom:8px;">Stale · review needed</div>
              <div style="font-size:12.5px; color:var(--fg-1); line-height:1.55;">${stale.length} runbook${stale.length === 1 ? "" : "s"} haven't been rehearsed in 4+ months. ${stale.map(s => s.id).join(", ")}.</div>
              <lthn-btn tone="ghost" size="sm" style="margin-top:10px;">Schedule rehearsals</lthn-btn>
            </div>
          ` : html`
            <div style="padding:14px 16px; border-radius:10px; background:rgba(34,197,94,0.06); border:1px solid rgba(34,197,94,0.22);">
              <div style="font-family:var(--font-mono); font-size:10px; color:var(--success-400); letter-spacing:0.10em; text-transform:uppercase; margin-bottom:8px;">Library fresh</div>
              <div style="font-size:12.5px; color:var(--fg-1); line-height:1.55;">All runbooks rehearsed within the last quarter.</div>
            </div>
          `}
          <div style="font-family:var(--font-mono); font-size:10.5px; color:var(--fg-3); line-height:1.7;">
            Runbooks live in <code style="color:var(--fg-1);">~/Lethean/runbooks/</code>.
            Each is a markdown file with a header tracking last-rehearsal date.
          </div>
        </aside>
      </div>
    `;
  }

  render() {
    const body = this._renderBody();
    if (this.embedded) return body;
    const stale = this._stale().length;
    return renderChrome({
      title: "Runbooks",
      subtitle: `${this.books.length} books${stale > 0 ? ` · ${stale} stale` : ""}`,
      w: this.w,
      h: this.h,
      body,
      footer: html`books in ~/Lethean/runbooks/ · auto-test daily · CI runs the simple ones quarterly`,
    });
  }
}
customElements.define("lthn-view-runbooks", LthnViewRunbooks);
