// SPDX-Licence-Identifier: EUPL-1.2

// <lthn-view-observe-activity> — Activity view that consumes the
// Stage F.B Phase 2.3 audit Query endpoint at GET /v1/audit/events.
//
// Replaces the prior fixture-only "recent activity" surface with a
// real forensic timeline backed by ~/Lethean/audit/. Same wire
// pattern as the other Lit views (per the canonical "lit-view ←→
// Go backend" memory): lazy backend fetch with fixture fallback,
// 60s refresh poller, vi.mock at test module scope.
//
// Wires through apiFetch so the session-tier bearer header gets
// attached automatically. The endpoint is session-tier per RFC §5.3
// (any unlocked session reads the entire audit log — single-user
// desktop assumption; per-account scoping is Stage F+ work).
//
// Fixture displays a hand-curated row set the user sees on a fresh
// install / when the audit log is empty so the surface never blanks
// out. Real events take over the moment the recorder has anything
// to return.

import { LitElement, html } from "lit";
import { renderChrome } from "../../chrome";
import { apiFetch } from "../../api-fetch";

interface ActivityEvent {
  event:        string;
  ts:           number;            // unix seconds
  account_id?:  string;            // hashed (first 8 hex for display)
  outcome?:     string;            // ok | failed | denied | error
  scope?:       string;
  request_id?:  string;
  meta?:        Record<string, unknown>;
}

interface ActivityQueryResponse {
  events?:      ActivityEvent[];
  next_cursor?: string;
}

// Fixture events for the empty-log first-run state. Same shape as
// the audit recorder's on-disk JSON so the renderer doesn't branch
// on source.
const FIXTURE_EVENTS: ActivityEvent[] = [
  { event: "auth.session.issued", ts: now() - 60,   outcome: "ok",     scope: "session", account_id: "fixture0" },
  { event: "tasks.created",       ts: now() - 320,  outcome: "ok",     scope: "core" },
  { event: "queue.enqueued",      ts: now() - 540,  outcome: "ok",     scope: "queue" },
  { event: "auth.unlock.failed",  ts: now() - 1240, outcome: "failed", scope: "unlock", account_id: "fixture0" },
];

function now(): number { return Math.floor(Date.now() / 1000); }

// Refresh cadence per the canonical lit-view→backend memory: 60s
// for user-driven surfaces (audit is read-mostly forensic browsing —
// not live-infra).
const REFRESH_MS = 60_000;

class LthnViewObserveActivity extends LitElement {
  static readonly properties = {
    w:        { type: Number },
    h:        { type: Number },
    embedded: { type: Boolean, reflect: true },
    events:   { state: true },
    loading:  { state: true },
    error:    { state: true },
    filter:   { state: true },
  };
  declare w: number;
  declare h: number;
  declare embedded: boolean;
  declare events: ActivityEvent[];
  declare loading: boolean;
  declare error: string;
  declare filter: string;
  private _timer: ReturnType<typeof setInterval> | null = null;

  constructor() {
    super();
    this.w = 1180;
    this.h = 720;
    this.embedded = false;
    this.events = FIXTURE_EVENTS;
    this.loading = false;
    this.error = "";
    this.filter = "";
  }
  createRenderRoot() { return this; }

  async connectedCallback() {
    super.connectedCallback();
    await this._loadFromBackend();
    this._timer = setInterval(() => { void this._loadFromBackend(); }, REFRESH_MS);
  }

  disconnectedCallback() {
    if (this._timer) { clearInterval(this._timer); this._timer = null; }
    super.disconnectedCallback();
  }

  // _loadFromBackend hits GET /v1/audit/events. On any failure /
  // unexpected shape / empty response, keeps the current event list
  // (fixture or previously-loaded events) per the canonical wire
  // pattern's reject-keeps / empty-keeps invariants.
  async _loadFromBackend() {
    if (this.loading) return;
    this.loading = true;
    this.error = "";
    try {
      const limit = 200;
      const res = await apiFetch(`/v1/audit/events?limit=${limit}`, {
        method: "GET",
        headers: { "Accept": "application/json" },
      });
      if (!res.ok) {
        // 401 surfaces as "session expired" silently — the auth-gate
        // listens for 401 across apiFetch and handles re-auth flow.
        // Other failures leave the current list intact + surface a
        // mild error string into the chrome footer.
        if (res.status !== 401) {
          this.error = `audit query returned ${res.status}`;
        }
        return;
      }
      const wrapper = await res.json();
      // coreapi.Response envelope — extract data field.
      const data = (wrapper as { data?: ActivityQueryResponse })?.data;
      const events = data?.events;
      if (!Array.isArray(events)) {
        // Unexpected shape — keep fixture / current state.
        return;
      }
      if (events.length === 0) {
        // Empty response — keep current state per design contract
        // (don't blank the surface).
        return;
      }
      this.events = events;
    } catch (e) {
      this.error = (e instanceof Error) ? e.message : "audit fetch failed";
    } finally {
      this.loading = false;
    }
  }

  // Filter helper — applies the case-insensitive substring filter
  // to the event-name + scope + account_id fields. Empty filter =
  // no filtering. Pure function over this.events; renderer calls
  // each render.
  _filtered(): ActivityEvent[] {
    const q = (this.filter || "").trim().toLowerCase();
    if (!q) return this.events;
    return this.events.filter(ev =>
      (ev.event || "").toLowerCase().includes(q) ||
      (ev.scope || "").toLowerCase().includes(q) ||
      (ev.account_id || "").toLowerCase().includes(q)
    );
  }

  _onFilterInput(e: Event) {
    this.filter = (e.target as HTMLInputElement)?.value ?? "";
  }

  // _fmtAccountID renders the first 8 hex chars of the (already-
  // hashed) account_id with a hover-for-full-hash title attribute,
  // per RFC §5.2. Empty / undefined renders the literal "—" so
  // system events (no account_id) render uniformly.
  _fmtAccountID(id?: string): string {
    if (!id) return "—";
    return id.length > 8 ? id.slice(0, 8) : id;
  }

  _fmtTimestamp(ts: number): string {
    if (!ts) return "";
    const d = new Date(ts * 1000);
    // Locale-aware short format — operators reading the panel will
    // typically be doing chronological scans, not date arithmetic;
    // ISO seconds would be noisy. Hour:minute:second + date.
    return d.toLocaleString("en-GB", { hour12: false });
  }

  _renderRow(ev: ActivityEvent) {
    const outcomeClass = `outcome-${ev.outcome || "ok"}`;
    return html`
      <tr class="activity-row">
        <td class="ts">${this._fmtTimestamp(ev.ts)}</td>
        <td class="event">${ev.event}</td>
        <td class="${outcomeClass}">${ev.outcome || "ok"}</td>
        <td class="account_id" title=${ev.account_id || ""}>${this._fmtAccountID(ev.account_id)}</td>
        <td class="scope">${ev.scope || ""}</td>
      </tr>
    `;
  }

  render() {
    const rows = this._filtered();
    const body = html`
      <div class="activity-toolbar">
        <input
          type="text"
          placeholder="filter by event, scope, or account_id…"
          .value=${this.filter}
          @input=${(e: Event) => this._onFilterInput(e)}
        />
        ${this.loading ? html`<span class="loading">loading…</span>` : ""}
        ${this.error ? html`<span class="error">${this.error}</span>` : ""}
      </div>
      <table class="activity-table">
        <thead>
          <tr>
            <th>time</th>
            <th>event</th>
            <th>outcome</th>
            <th>account</th>
            <th>scope</th>
          </tr>
        </thead>
        <tbody>
          ${rows.length === 0
            ? html`<tr><td colspan="5" class="empty-state">No events match these filters. Adjust the time window or clear filters to see more.</td></tr>`
            : rows.map(r => this._renderRow(r))}
        </tbody>
      </table>
    `;
    if (this.embedded) return body;
    return renderChrome({
      title:    "Activity",
      subtitle: "Forensic record of auth + lifecycle events",
      w:        this.w,
      h:        this.h,
      embedded: false,
      body,
    });
  }
}

if (!customElements.get("lthn-view-observe-activity")) {
  customElements.define("lthn-view-observe-activity", LthnViewObserveActivity);
}

export { LthnViewObserveActivity };
export type { ActivityEvent, ActivityQueryResponse };
