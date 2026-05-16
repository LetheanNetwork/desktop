// SPDX-Licence-Identifier: EUPL-1.2
// Sales · Contacts — <lthn-view-contacts>
//
// CRM-style contact list with last-touch + warmth + next action.
// Monochrome treatment per HANDOVER-VIEWS.md — no leaderboard, no
// gamification, just an honest list of who you've been talking to.
//
// Two-shell: standalone (chrome) when spawned directly, embedded (no
// chrome) when mounted by <lthn-app-shell>. Mirrors the welcome /
// settings window pattern documented in design_two_shell_pattern.md.
//
// Data: fixture array only for v1. Real source is a future
// `pkg/sales/contacts` Go service that maps a CRM contact graph into
// the same { name, role, last, warmth, next } shape codified here.
// See HANDOVER-VIEWS.md.

import { LitElement, html } from "lit";
import { renderChrome } from "../../chrome";

/** Warmth bucket — how warm is the conversation right now. Used both
 *  for the dot colour and for the active-warmth pill in the footer
 *  summary. Tasteful + observational, not a score the user is being
 *  graded against. */
type Warmth = "hot" | "warm" | "cool";

/** A single contact row. */
interface Contact {
  /** Person name. */
  name:   string;
  /** Role string — already formatted with the org name. */
  role:   string;
  /** Last touchpoint — short, opaque, the source-of-truth is the
   *  rollup service, not arithmetic on the client. */
  last:   string;
  /** Conversation warmth bucket. */
  warmth: Warmth;
  /** Next planned step — free text owned by the operator. */
  next:   string;
}

/** Fixture contacts. Shape mirrors the future pkg/sales/contacts Go
 *  service — when that ships, swap this for a fetch + Reactive
 *  Controller (mirroring chat-window patterns). Strings stay opaque
 *  because the source-of-truth is the CRM, not the client. */
const FIXTURE_CONTACTS: Contact[] = [
  { name: "Ada Penley",       role: "CTO · Heritage Law",          last: "replied 2 d", warmth: "hot",  next: "call · Fri" },
  { name: "Marcus Stannard",  role: "Partner · Stannard & Co",      last: "emailed 5 d", warmth: "warm", next: "pilot signoff" },
  { name: "Dr. Imogen Beck",  role: "CIO · Lichfield NHS Trust",     last: "meeting 8 d", warmth: "warm", next: "DPIA review" },
  { name: "Tom Pemberton",    role: "COO · Pemberton Capital",       last: "replied 3 w", warmth: "cool", next: "re-engage Q3" },
  { name: "Sarah Whitethorn", role: "Founder · Whitethorn Press",    last: "replied 1 d", warmth: "hot",  next: "contract" },
  { name: "David Crown",      role: "IT Director · Crown Estates",   last: "replied 4 d", warmth: "hot",  next: "SOW review" },
];

/** Map a warmth bucket to the dot colour. Kept as a pure helper so
 *  tests can assert the colour mapping without rendering. */
function warmthColour(w: Warmth): string {
  switch (w) {
    case "hot":  return "var(--err-400)";
    case "warm": return "var(--warning-400)";
    case "cool": return "var(--fg-3)";
  }
}

class LthnViewContacts extends LitElement {
  static readonly properties = {
    w:        { type: Number },
    h:        { type: Number },
    embedded: { type: Boolean, reflect: true },
    /** Free-text filter on name / role. Empty string = no filter.
     *  Drives both the rendered rows and the "showing N of M" copy. */
    query:    { type: String },
    /** Warmth filter — "" = all, otherwise restrict to that bucket. */
    warmth:   { type: String, reflect: true },
    /** Fixture data — replace with a live backend call when
     *  pkg/sales/contacts bindings land (Mantis follow-up). */
    contacts: { state: true },
  };

  declare w: number;
  declare h: number;
  declare embedded: boolean;
  declare query: string;
  declare warmth: "" | Warmth;
  declare contacts: Contact[];

  constructor() {
    super();
    this.w = 1180;
    this.h = 720;
    this.embedded = false;
    this.query = "";
    this.warmth = "";
    this.contacts = FIXTURE_CONTACTS;
  }

  createRenderRoot() { return this; }

  async connectedCallback() {
    super.connectedCallback();
    await this._loadFromBackend();
  }

  /** Pull the live contacts list from pkg/sales/contacts (shipped
   *  in c883227). Maps the backend Contact shape 1:1 onto the view
   *  shape — fields are identical (name/role/last/warmth/next).
   *  Degrades to FIXTURE_CONTACTS on binding-missing or backend
   *  rejection so the view stays useful in headless dev. */
  async _loadFromBackend(): Promise<void> {
    try {
      const svc = await import("@desktop/sales/contacts/service").catch(() => null);
      if (!svc || typeof (svc as { List?: unknown }).List !== "function") {
        return;
      }
      const r = await (svc as {
        List: (input: { warmth: string; query: string; limit: number }) => Promise<{
          Value?: { contacts?: Contact[] }
        }>
      }).List({ warmth: "", query: "", limit: 100 });
      const rows = r?.Value?.contacts;
      if (rows && rows.length > 0) {
        this.contacts = rows;
      }
    } catch {
      // Backend unavailable — keep fixture data.
    }
  }

  /** Apply the query + warmth filters. Pure helper so a unit test can
   *  drive it without mounting the element. */
  _filtered(): Contact[] {
    const q = this.query.trim().toLowerCase();
    return this.contacts.filter(c => {
      if (this.warmth && c.warmth !== this.warmth) return false;
      if (!q) return true;
      return c.name.toLowerCase().includes(q) || c.role.toLowerCase().includes(q);
    });
  }

  /** Update the warmth filter. Clicking the currently-active pill
   *  clears the filter — same toggle UX as the chat-window tab rail. */
  _toggleWarmth(w: Warmth) {
    this.warmth = this.warmth === w ? "" : w;
  }

  render() {
    const list = this._filtered();
    const all  = this.contacts;
    const hot  = all.filter(c => c.warmth === "hot").length;

    const warmthBtn = (w: Warmth, label: string) => html`
      <lthn-btn class="lthn-view-contacts-warmth"
                data-warmth=${w}
                tone="ghost" size="sm"
                ?active=${this.warmth === w}
                @click=${() => this._toggleWarmth(w)}>
        <span style="width:7px; height:7px; border-radius:50%; background:${warmthColour(w)};"></span>
        ${label}
      </lthn-btn>
    `;

    const body = html`
      <div class="lthn-view-contacts" style="flex:1; padding:18px 22px; display:flex; flex-direction:column; gap:14px; overflow:auto;">
        <div style="display:flex; align-items:center; gap:10px;">
          <h2 style="margin:0; font-size:22px; color:var(--fg-0); letter-spacing:-0.02em; font-weight:600;">Contacts</h2>
          <span style="font-family:var(--font-mono); font-size:11px; color:var(--fg-3);">
            ${list.length} of ${all.length}
          </span>
          <div style="flex:1"></div>
          ${warmthBtn("hot",  "hot")}
          ${warmthBtn("warm", "warm")}
          ${warmthBtn("cool", "cool")}
          <lthn-btn tone="primary" size="sm">
            <i class="fa-solid fa-plus" style="font-size:10px;"></i> Add
          </lthn-btn>
        </div>

        <div class="lthn-view-contacts-list"
             style="background:rgba(255,255,255,0.025); border:1px solid rgba(255,255,255,0.06); border-radius:10px; overflow:hidden;">
          ${list.length === 0 ? html`
            <div style="padding:24px; text-align:center; font-size:12px; color:var(--fg-3);">
              No contacts match the current filter.
            </div>
          ` : list.map((c, i) => html`
            <div class="lthn-view-contacts-row"
                 data-name=${c.name}
                 data-warmth=${c.warmth}
                 style="display:grid; grid-template-columns: 1.2fr 1.4fr 120px 100px 130px 60px;
                        gap:14px; padding:14px 16px;
                        border-bottom:${i < list.length - 1 ? "1px solid rgba(255,255,255,0.04)" : "none"};
                        align-items:center;">
              <div>
                <div style="font-size:13px; color:var(--fg-0);">${c.name}</div>
              </div>
              <span style="font-size:12px; color:var(--fg-2);">${c.role}</span>
              <span style="font-family:var(--font-mono); font-size:10.5px; color:var(--fg-3);">${c.last}</span>
              <span style="display:flex; align-items:center; gap:6px;">
                <span style="width:7px; height:7px; border-radius:50%; background:${warmthColour(c.warmth)};"></span>
                <span style="font-family:var(--font-mono); font-size:10.5px; color:var(--fg-2);">${c.warmth}</span>
              </span>
              <span style="font-family:var(--font-mono); font-size:10.5px; color:var(--brand-300);">${c.next}</span>
              <lthn-btn tone="ghost" size="sm">Open</lthn-btn>
            </div>
          `)}
        </div>
      </div>
    `;

    return renderChrome({
      title:    "Contacts",
      subtitle: `${all.length} active · ${hot} hot`,
      w: this.w, h: this.h,
      embedded: this.embedded,
      body,
      footer: html`warmth recomputed weekly from last touch · ⌘N to add · CRM stays local`,
    });
  }
}

customElements.define("lthn-view-contacts", LthnViewContacts);
