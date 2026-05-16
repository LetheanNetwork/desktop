// SPDX-Licence-Identifier: EUPL-1.2
//
// Planning view · Retros — <lthn-view-retros>
//
// Three-column retrospective board: Went well / Didn't work / Try
// next sprint. Each column has its own tone (success / warning /
// brand) and a row of card chips. Add-button per column appends a
// pending entry to the local list (real persistence wires through
// pkg/tasks Note later — Notes hung off the sprint-anchor Issue).

import { LitElement, html } from "lit";
import { renderChrome } from "../../chrome";

/** A retro note. */
interface RetroNote {
  /** Note body. */
  t: string;
  /** Author handle (or "all" / pair string). */
  by: string;
}

type Tone = "ok" | "warn" | "brand";

/** A retro column. */
interface RetroColumn {
  /** Stable column id. */
  id: "good" | "bad" | "next";
  /** Header label. */
  label: string;
  /** Colour tone for the header card. */
  tone: Tone;
  /** Font Awesome icon name. */
  icon: string;
  /** Items in display order. */
  items: RetroNote[];
}

interface RetrosData {
  /** Sprint number this retro belongs to. */
  sprint:  number;
  /** Display window, e.g. "28 Apr → 9 May". */
  window:  string;
  /** Number of participants. */
  people:  number;
  /** Columns in display order. */
  columns: RetroColumn[];
}

const RETROS_FIXTURE: RetrosData = {
  sprint: 23, window: "28 Apr → 9 May", people: 3,
  columns: [
    { id: "good", label: "Went well",      tone: "ok",    icon: "fa-thumbs-up", items: [
      { t: "Tray icon family landed on first review", by: "Tobi" },
      { t: "MCP tools window shipped 2 days early",   by: "you" },
      { t: "First 50 closed-alpha users active",       by: "Mei" },
    ]},
    { id: "bad",  label: "Didn't work",     tone: "warn",  icon: "fa-thumbs-down", items: [
      { t: "Linux packaging slipped a sprint", by: "Mei" },
      { t: "Settings refactor blocked release notes prep", by: "you" },
    ]},
    { id: "next", label: "Try next sprint", tone: "brand", icon: "fa-arrow-right", items: [
      { t: "Lock release scope by Wednesday, freeze by Friday", by: "you" },
      { t: "Pair on Linux packaging next sprint", by: "Mei + Tobi" },
      { t: "Move design review to async on Loom", by: "all" },
    ]},
  ],
};

/** Resolve the three-stop colour tuple [bg, border, fg] for a tone.
 *  Exported so the test suite can pin colour invariants without
 *  inspecting style strings. */
export function toneColours(tone: Tone): [string, string, string] {
  switch (tone) {
    case "ok":    return ["rgba(34,197,94,0.06)", "rgba(34,197,94,0.22)", "var(--success-400)"];
    case "warn":  return ["rgba(245,158,11,0.06)","rgba(245,158,11,0.22)","var(--warning-400)"];
    case "brand": return ["rgba(64,193,197,0.06)","rgba(64,193,197,0.22)","var(--brand-300)"];
  }
}

/** Count items across all columns. Pure helper for the test suite. */
export function totalItems(columns: readonly RetroColumn[]): number {
  return columns.reduce((acc, c) => acc + c.items.length, 0);
}

class LthnViewRetros extends LitElement {
  static readonly properties = {
    embedded: { type: Boolean, reflect: true },
    data:     { state: true },
  };
  declare embedded: boolean;
  declare data: RetrosData;

  constructor() {
    super();
    this.embedded = false;
    this.data = RETROS_FIXTURE;
  }
  createRenderRoot() { return this; }

  async connectedCallback() {
    super.connectedCallback();
    void this._loadFromBackend();
  }

  /** Pending pkg/tasks Wails binding. Today: keep fixture. */
  async _loadFromBackend(): Promise<void> { /* no-op */ }

  /** Append a draft note to a column. Public so the test suite can
   *  drive the add-button wire without typing in a real input. */
  addNote(colId: RetroColumn["id"], body: string, author = "you"): void {
    if (!body.trim()) return;
    const columns = this.data.columns.map(c =>
      c.id === colId ? { ...c, items: [...c.items, { t: body.trim(), by: author }] } : c,
    );
    this.data = { ...this.data, columns };
  }

  render() {
    const body = html`
      <div style="flex:1; display:flex; flex-direction:column; min-height:0; overflow:hidden;">
        <div style="padding:18px 22px 14px; display:flex; align-items:baseline; gap:12px;">
          <h2 style="margin:0; font-size:22px; color:var(--fg-0); letter-spacing:-0.02em; font-weight:600;">Retro · Sprint ${this.data.sprint}</h2>
          <span style="font-family:var(--font-mono); font-size:11px; color:var(--fg-3);">${this.data.window} · ${this.data.people} people · ${totalItems(this.data.columns)} items</span>
        </div>

        <div class="lthn-retros-board" style="flex:1; display:grid; grid-template-columns:repeat(3, 1fr); gap:14px; padding:0 22px 18px; min-height:0; overflow:auto;">
          ${this.data.columns.map(c => {
            const [bg, bd, fg] = toneColours(c.tone);
            return html`
              <div class="lthn-retros-column" data-col=${c.id}
                   style="display:flex; flex-direction:column; gap:10px; min-height:0;">
                <div style="display:flex; align-items:center; gap:10px; padding:14px 16px; border-radius:10px;
                            background:${bg}; border:1px solid ${bd};">
                  <div style="width:32px; height:32px; border-radius:8px;
                              background:${bd}; display:flex; align-items:center; justify-content:center;">
                    <i class="fa-solid ${c.icon}" style="font-size:12px; color:${fg};"></i>
                  </div>
                  <div>
                    <div style="font-size:14px; font-weight:500; color:${fg};">${c.label}</div>
                    <div style="font-family:var(--font-mono); font-size:10px; color:var(--fg-3); margin-top:2px;">${c.items.length} items</div>
                  </div>
                </div>
                <div style="display:flex; flex-direction:column; gap:8px;">
                  ${c.items.map(it => html`
                    <div class="lthn-retros-note"
                         style="padding:12px 14px; border-radius:8px;
                                background:rgba(255,255,255,0.025);
                                border:1px solid rgba(255,255,255,0.06);">
                      <div style="font-size:13px; color:var(--fg-0); line-height:1.5;">${it.t}</div>
                      <div style="font-family:var(--font-mono); font-size:10px; color:var(--fg-3); margin-top:6px;">— ${it.by}</div>
                    </div>
                  `)}
                  <button class="lthn-retros-add" data-col=${c.id}
                          @click=${() => this.addNote(c.id, "New note", "you")}
                          style="padding:10px; border-radius:8px;
                                 background:transparent; border:1px dashed rgba(255,255,255,0.10);
                                 color:var(--fg-3); cursor:pointer; font-family:var(--font-sans); font-size:12px;
                                 --wails-draggable: no-drag;">
                    <i class="fa-solid fa-plus" style="font-size:10px;"></i> Add
                  </button>
                </div>
              </div>
            `;
          })}
        </div>
      </div>
    `;

    return renderChrome({
      title: "Retro",
      subtitle: `Sprint ${this.data.sprint} · 9 May`,
      body,
      footer: html`actions assigned · review at sprint ${this.data.sprint + 1} close · history at retros/2026-05-09.md`,
      embedded: this.embedded,
    });
  }
}
customElements.define("lthn-view-retros", LthnViewRetros);
