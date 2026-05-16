// SPDX-Licence-Identifier: EUPL-1.2
// Office · Documents — <lthn-view-documents>
//
// Local markdown corpus list — recent edits, draft/ready/final/live state,
// per-file size + author + last-edit. The Monday-morning surface: calm,
// dense, functional. Dark-only baseline (accessibility floor, not aesthetic).
//
// Two-shell: standalone (with chrome) when spawned directly, embedded (no
// chrome) when mounted inside <lthn-app-shell>. Mirrors the welcome /
// settings window pattern in ../../ops/welcome-window.ts.
//
// Data: loads from pkg/office/documents via the Wails binding when available;
// falls back to fixture data in test environments and pre-binding builds.
// Binding import is lazy (dynamic) so a missing binding does not kill the
// module load — same pattern as incidents.ts.

import { LitElement, html } from "lit";
import { renderChrome } from "../../chrome";

/** Shape of one document row. Mirrors the DocRow type in
 *  pkg/office/documents/types.go — field names are canonical. */
interface DocRow {
  /** Document title — first H1 in the markdown body, falls back to filename. */
  title:  string;
  /** Lifecycle state — draft (work-in-progress), ready (review), final
   *  (signed off), live (published / shipped). */
  state:  "draft" | "ready" | "final" | "live";
  /** Author display name — "you" for current-user files, name otherwise. */
  author: string;
  /** Human-readable last-edit timestamp ("now", "yest", "3 d ago"). */
  edited: string;
  /** Human-readable file size ("4.2 KB", "248 KB"). */
  size:   string;
}

/** Map document state → swatch colour. Identical palette to the design
 *  reference (plans/ops/lthn/desktop/design-package-2026-05-16/
 *  lit-views-office.js). Kept centralised so a tone tweak is one edit. */
const STATE_COLOUR: Record<DocRow["state"], string> = {
  draft: "var(--fg-3)",
  ready: "var(--brand-300)",
  final: "var(--success-400)",
  live:  "var(--success-400)",
};

/** Fixture documents. Shown when the Documents binding is unavailable
 *  (test environments, pre-binding builds). Strings stay opaque (KB/ago)
 *  because the source-of-truth is the indexer's human-readable formatter. */
const FIXTURE_DOCUMENTS: DocRow[] = [
  { title: "v0.2 release notes",          state: "draft", author: "you",  edited: "now",      size: "4.2 KB" },
  { title: "Sovereign-compute manifesto", state: "ready", author: "you",  edited: "yest",     size: "12 KB" },
  { title: "Q2 board pack · numbers",     state: "final", author: "Mei",  edited: "3 d ago",  size: "248 KB" },
  { title: "Hiring · senior Go engineer", state: "live",  author: "you",  edited: "1 w ago",  size: "6.4 KB" },
  { title: "SOW · Heritage Law LLP v2",   state: "draft", author: "you",  edited: "4 d ago",  size: "38 KB" },
  { title: "Runbook · DNS rotation",      state: "final", author: "Tobi", edited: "2 w ago",  size: "8.2 KB" },
];

class LthnViewDocuments extends LitElement {
  static readonly properties = {
    w:        { type: Number },
    h:        { type: Number },
    embedded: { type: Boolean, reflect: true },
    /** Active state filter — null means "all". Backed by the filter
     *  buttons in the toolbar; consumed by the visible-doc list below. */
    filter:   { state: true },
    /** Live documents from the backend. Falls back to FIXTURE_DOCUMENTS. */
    docs:     { state: true },
    loading:  { state: true },
  };
  declare w: number;
  declare h: number;
  declare embedded: boolean;
  declare filter: DocRow["state"] | null;
  declare docs: DocRow[];
  declare loading: boolean;

  private _timer: ReturnType<typeof setInterval> | null = null;

  constructor() {
    super();
    this.w = 1180;
    this.h = 720;
    this.embedded = false;
    this.filter = null;
    this.docs = FIXTURE_DOCUMENTS;
    this.loading = false;
  }

  createRenderRoot() { return this; }

  async connectedCallback() {
    super.connectedCallback();
    await this._loadFromBackend();
    // Re-poll every 30 s — document mtime changes are infrequent.
    this._timer = setInterval(() => { void this._loadFromBackend(); }, 30_000);
  }

  disconnectedCallback() {
    if (this._timer) { clearInterval(this._timer); this._timer = null; }
    super.disconnectedCallback();
  }

  async _loadFromBackend() {
    if (this.loading) return;
    this.loading = true;
    try {
      // Lazy-import the Wails binding — missing binding in tests does not
      // kill the module load; falls back to fixture data on any failure.
      const svc = await import("@desktop/office/documents/service").catch(() => null);
      if (!svc || typeof (svc as { List?: unknown }).List !== "function") {
        return;
      }
      const r = await (svc as {
        List: (input: { state: string; limit: number }) => Promise<{
          Value?: { docs?: DocRow[] }
        }>
      }).List({ state: this.filter ?? "", limit: 50 });
      const rows = r?.Value?.docs;
      if (rows && rows.length > 0) {
        this.docs = rows;
      }
    } catch {
      // Backend unavailable — keep current data (fixture or last good load).
    } finally {
      this.loading = false;
    }
  }

  /** Visible documents = full list when no filter, otherwise the
   *  subset whose state matches. Split out so tests can drive the
   *  filter property directly and assert the row count. */
  _visible(): DocRow[] {
    if (!this.filter) return this.docs;
    return this.docs.filter(d => d.state === this.filter);
  }

  /** Toggle a state filter — clicking the active filter clears it. */
  _setFilter(state: DocRow["state"]) {
    this.filter = (this.filter === state) ? null : state;
    // Re-fetch with the new filter so the backend can narrow the scan.
    void this._loadFromBackend();
  }

  render() {
    const docs = this._visible();
    const total = this.docs.length;

    const body = html`
      <div class="lthn-view-documents" style="flex:1; display:flex; flex-direction:column; min-height:0;">
        <div style="padding:18px 22px 12px; display:flex; align-items:center; gap:12px;">
          <h2 style="margin:0; font-size:22px; color:var(--fg-0); letter-spacing:-0.02em; font-weight:600;">Documents</h2>
          <span style="font-family:var(--font-mono); font-size:11px; color:var(--fg-3);">
            ${docs.length} of ${total} · ~/.lthn/docs/
          </span>
          <div style="flex:1"></div>
          <lthn-btn
            class="lthn-view-documents-filter"
            data-state="draft"
            tone="ghost"
            size="sm"
            ?active=${this.filter === "draft"}
            @click=${() => this._setFilter("draft")}>
            Drafts
          </lthn-btn>
          <lthn-btn
            class="lthn-view-documents-filter"
            data-state="ready"
            tone="ghost"
            size="sm"
            ?active=${this.filter === "ready"}
            @click=${() => this._setFilter("ready")}>
            Ready
          </lthn-btn>
          <lthn-btn
            class="lthn-view-documents-filter"
            data-state="final"
            tone="ghost"
            size="sm"
            ?active=${this.filter === "final"}
            @click=${() => this._setFilter("final")}>
            Final
          </lthn-btn>
          <lthn-btn tone="primary" size="sm">
            <i class="fa-solid fa-plus" style="font-size:10px;"></i> New
          </lthn-btn>
        </div>
        <div style="flex:1; overflow:auto; padding:0 22px 18px;">
          <div class="lthn-view-documents-list" style="background:rgba(255,255,255,0.025); border:1px solid rgba(255,255,255,0.06); border-radius:10px; overflow:hidden;">
            ${docs.length === 0 ? html`
              <div class="lthn-view-documents-empty" style="padding:32px 16px; text-align:center; font-size:12px; color:var(--fg-3);">
                No documents match this filter.
              </div>
            ` : docs.map((d, i) => html`
              <div class="lthn-view-documents-row"
                   data-state=${d.state}
                   style="display:grid; grid-template-columns: 32px 1fr 80px 100px 80px 60px; gap:14px;
                          padding:14px 16px;
                          border-bottom:${i < docs.length - 1 ? "1px solid rgba(255,255,255,0.04)" : "none"};
                          align-items:center;">
                <i class="fa-regular fa-file-lines" style="font-size:14px; color:var(--fg-3);"></i>
                <span style="font-size:13px; color:var(--fg-0);">${d.title}</span>
                <span style="font-family:var(--font-mono); font-size:10.5px; padding:3px 9px; border-radius:999px; background:${STATE_COLOUR[d.state]}1a; color:${STATE_COLOUR[d.state]}; letter-spacing:0.04em; text-align:center;">${d.state}</span>
                <span style="font-family:var(--font-mono); font-size:11px; color:var(--fg-2);">${d.author}</span>
                <span style="font-family:var(--font-mono); font-size:10.5px; color:var(--fg-3); text-align:right;">${d.edited}</span>
                <span style="font-family:var(--font-mono); font-size:10.5px; color:var(--fg-3); text-align:right;">${d.size}</span>
              </div>
            `)}
          </div>
        </div>
      </div>
    `;

    return renderChrome({
      title: "Documents",
      subtitle: `${docs.length} ${this.filter ? this.filter : "recent"} · ~/.lthn/docs/`,
      w: this.w, h: this.h,
      body,
      footer: html`local markdown · auto-syncs across machines · ⌘N to write · pkg/office/documents`,
      embedded: this.embedded,
    });
  }
}

customElements.define("lthn-view-documents", LthnViewDocuments);
