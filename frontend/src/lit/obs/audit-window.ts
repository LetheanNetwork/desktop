// SPDX-Licence-Identifier: EUPL-1.2
//
// <lthn-audit-viewer> — forensic-readout surface for the Stage F
// audit-log substrate. Sibling of <lthn-logs-window> per
// plans/code/lthn/desktop/auth-gate/RFC.stage-e-audit-viewer.md v2
// §1.1 — lives under frontend/src/lit/obs/ so the operator's reflex
// "did anything happen?" lands in the same window-strip that already
// surfaces logs + telemetry.
//
// Consumes GET /v1/audit/events (session-tier, per pkg/server/service.go:299
// RouteTiers entry) via the apiFetch helper — no Wails binding (the
// audit Service exposes REST only per pkg/audit/service.go:411 + the
// E.C.B backend Unit B at SHA 832f5de).
//
// SECURITY DISCIPLINE — Cerberus #29 ADD-MED-1 (RFC §4.5):
//
//   Meta values are rendered via Lit TEXT-INTERPOLATION ONLY. This
//   module MUST NOT import `lit/directives/unsafe-html.js`, MUST NOT
//   assign to `.innerHTML`, MUST NOT use `eval` / `Function`. The
//   audit emitters control Meta key+value strings today, but a future
//   emit-site taking external input into Meta (e.g. seal_failed with
//   a reason string from an HTTP body) would otherwise create an XSS
//   sink at the viewer. The test `TestAuditWindow_MetaValueHTMLEscaped_Bad`
//   asserts a `<script>`/`<img>` payload renders as literal text and
//   greps this file to enforce the "no unsafe-html import" rule.
//
// PRIVACY POSTURE — RFC §4.1 + §4.2:
//
//   account_id and Meta.path_hash arrive HMAC-hashed from the server
//   (RFC.stage-f §6.4). UI shows truncated hex + copy-to-clipboard
//   affordance; full value lives in the title tooltip. No code path
//   attempts to reverse-resolve hashes.
//
// AUTO-REFRESH GATING — RFC §5.4 + Cerberus #29 LOW:
//
//   Toggle defaults OFF. When ON, fetches every 30s — but only when
//   `document.visibilityState === "visible"`. A `visibilitychange →
//   visible` event MUST NOT issue a fetch within 5 seconds of the
//   previous fetch (burst-throttle defends against rapid tab-flicker).
//
// Usage:
//
//   <lthn-audit-viewer></lthn-audit-viewer>            // standalone
//   <lthn-audit-viewer embedded></lthn-audit-viewer>   // inside app-shell

import { LitElement, html, nothing, type TemplateResult } from "lit";
import { renderChrome } from "../chrome";
import { T } from "@ui/i18n";
import { apiFetch } from "../api-fetch";
import {
  AUDIT_EVENT_NAMES,
  AUDIT_OUTCOMES,
  type AuditOutcome,
} from "./audit-constants";

/** AuditEvent matches the JSON shape the backend emits per
 *  pkg/audit/types.go Event + RFC §2.2 (envelope per
 *  [[feedback_coreapi_data_envelope_not_value]]).
 *
 *  Meta is `Record<string, unknown>` — render-side discipline (§4.5)
 *  is the only defence against attacker-influenced values.  */
export interface AuditEvent {
  event: string;
  account_id?: string;
  ts: number;
  exp?: number;
  scope?: string;
  outcome?: AuditOutcome;
  request_id?: string;
  meta?: Record<string, unknown>;
}

interface AuditPage {
  events: AuditEvent[];
  nextCursor: string;
}

/** Burst-throttle floor for visibility-driven refetch (Cerberus #29 LOW).
 *  A 5-second minimum inter-fetch window stops rapid tab-flicker from
 *  hammering the endpoint. Applies to both the 30s timer AND the
 *  visibilitychange handler. */
const BURST_THROTTLE_MS = 5_000;

/** Default 30-second auto-refresh interval — only fires when the toggle
 *  is ON and the window is visible. */
const AUTO_REFRESH_MS = 30_000;

/** Default page size for the initial fetch — matches the server's
 *  default cap (pkg/audit/routes.go) so the operator sees a full page
 *  without an extra round-trip. */
const PAGE_SIZE_DEFAULT = 200;

/** Hex-prefix length for truncated account_id / path_hash display. RFC
 *  §4.1 — first-8-hex + copy affordance. */
const HASH_TRUNCATE = 8;

/** Default fixture row — keeps the developer-mounting-the-file-without-
 *  wiring-backend case showing a non-empty surface per
 *  [[design_lit_view_backend_wire_pattern]] §"fixture-still-renders". */
const FIXTURE_EVENT: AuditEvent = {
  event: "auth.session.issued",
  account_id: "abc12345def67890abcdef0123456789",
  ts: 1779000000,
  exp: 1779003600,
  scope: "session",
  outcome: "ok",
  request_id: "fixture-req-0001",
  meta: { path_hash: "f3a8b1c2d4e5f6071829304a5b6c7d8e", version: 1 },
};

class LthnAuditWindow extends LitElement {
  static readonly properties = {
    w:          { type: Number },
    h:          { type: Number },
    embedded:   { type: Boolean, reflect: true },
    events:     { state: true },
    cursor:     { state: true },
    eventFilter:   { state: true },
    outcomeFilter: { state: true },
    autoRefresh:   { state: true },
    expanded:      { state: true },
    loading:       { state: true },
    errorMsg:      { state: true },
    boundaryBadge: { state: true },
    chrome:        { state: true },
    t:             { state: true },
  };
  declare w: number;
  declare h: number;
  declare embedded: boolean;
  declare events: AuditEvent[];
  declare cursor: string;
  declare eventFilter: string;      // "" = all
  declare outcomeFilter: string;    // "" = all
  declare autoRefresh: boolean;
  declare expanded: Set<number>;    // row indices currently expanded
  declare loading: boolean;
  declare errorMsg: string;
  declare boundaryBadge: boolean;
  declare chrome: { title: string; subtitle: string };
  declare t: {
    filterEventAll: string;
    filterOutcomeAll: string;
    btnRefresh: string;
    btnLoadMore: string;
    btnAutoRefresh: string;
    btnExportJSONL: string;
    btnExportCSV: string;
    colTs: string;
    colEvent: string;
    colOutcome: string;
    colAccount: string;
    colRequest: string;
    accountHelp: string;
    empty: string;
    error: string;
    sameSecondWarn: string;
    autoRefreshCaveat: string;
  };

  /** Last-fetch wall-clock ms, used by the burst-throttle (§5.4 LOW). */
  private _lastFetchAt = 0;
  /** setInterval id for the auto-refresh poll. */
  private _autoTimer: number | null = null;
  /** visibilitychange listener reference for clean teardown. */
  private _visListener: (() => void) | null = null;

  constructor() {
    super();
    this.w = 1100;
    this.h = 720;
    this.embedded = false;
    this.events = [FIXTURE_EVENT];
    this.cursor = "";
    this.eventFilter = "";
    this.outcomeFilter = "";
    this.autoRefresh = false;
    this.expanded = new Set<number>();
    this.loading = false;
    this.errorMsg = "";
    this.boundaryBadge = false;
    this.chrome = { title: "Audit log", subtitle: "forensic record" };
    this.t = {
      filterEventAll:   "All events",
      filterOutcomeAll: "All outcomes",
      btnRefresh:       "Refresh",
      btnLoadMore:      "Load more",
      btnAutoRefresh:   "Auto-refresh",
      btnExportJSONL:   "Export JSON-Lines",
      btnExportCSV:     "Export CSV",
      colTs:            "Time",
      colEvent:         "Event",
      colOutcome:       "Outcome",
      colAccount:       "Account",
      colRequest:       "Request",
      accountHelp:      "Paste an account ID; the server matches it privately so the typed value never leaves your machine in clear.",
      empty:            "No events match the current filter.",
      error:            "Failed to load audit events — retry, or check connection.",
      sameSecondWarn:   "Events sharing this exact second may be skipped at the page boundary.",
      autoRefreshCaveat: "Auto-refresh pauses when the window is hidden.",
    };
  }

  createRenderRoot() { return this; }

  async connectedCallback() {
    super.connectedCallback();
    // i18n best-effort. Constructor defaults already populate this.t /
    // chrome so a binding rejection (test fixture) does not blank the
    // surface — same pattern as logs-window.
    try {
      const [title, subtitle, fEv, fOut, bR, bL, bA, bEJ, bEC, cTs, cEv, cO, cA, cR, aH, em, er, ssw, arc] = await Promise.all([
        T("window.audit.title"),
        T("window.audit.subtitle"),
        T("window.audit.filter.event_all"),
        T("window.audit.filter.outcome_all"),
        T("window.audit.btn_refresh"),
        T("window.audit.btn_load_more"),
        T("window.audit.btn_auto_refresh"),
        T("window.audit.btn_export_jsonl"),
        T("window.audit.btn_export_csv"),
        T("window.audit.col.ts"),
        T("window.audit.col.event"),
        T("window.audit.col.outcome"),
        T("window.audit.col.account"),
        T("window.audit.col.request"),
        T("window.audit.account_id_help"),
        T("window.audit.empty_state"),
        T("window.audit.error_load"),
        T("window.audit.same_second_warning"),
        T("window.audit.auto_refresh_caveat"),
      ]);
      this.chrome = { title, subtitle };
      this.t = {
        filterEventAll: fEv, filterOutcomeAll: fOut,
        btnRefresh: bR, btnLoadMore: bL, btnAutoRefresh: bA,
        btnExportJSONL: bEJ, btnExportCSV: bEC,
        colTs: cTs, colEvent: cEv, colOutcome: cO, colAccount: cA, colRequest: cR,
        accountHelp: aH, empty: em, error: er,
        sameSecondWarn: ssw, autoRefreshCaveat: arc,
      };
    } catch {
      // i18n binding unavailable in tests / non-Wails contexts — keep
      // the English masters from the constructor.
    }
    // Visibility listener for the burst-throttle + auto-refresh gating
    // (§5.4). Always installed so a future toggle-ON picks up an
    // already-visible window correctly.
    this._visListener = () => this._onVisibilityChange();
    document.addEventListener("visibilitychange", this._visListener);
    // First load — try to fetch but tolerate a missing backend so the
    // fixture row stays visible in dev / test (the four-spec wire
    // pattern from [[design_lit_view_backend_wire_pattern]]).
    void this._loadFromBackend({ replace: true });
  }

  disconnectedCallback() {
    super.disconnectedCallback();
    if (this._autoTimer !== null) {
      clearInterval(this._autoTimer);
      this._autoTimer = null;
    }
    if (this._visListener) {
      document.removeEventListener("visibilitychange", this._visListener);
      this._visListener = null;
    }
  }

  /** Visibility-change handler. When the window goes visible AND auto-
   *  refresh is ON, schedule an immediate fetch — but only if the burst
   *  throttle hasn't fired in the last BURST_THROTTLE_MS. */
  private _onVisibilityChange() {
    if (!this.autoRefresh) return;
    if (document.visibilityState !== "visible") return;
    const now = Date.now();
    if (now - this._lastFetchAt < BURST_THROTTLE_MS) return;
    void this._loadFromBackend({ replace: true });
  }

  /** Toggle the auto-refresh poll. ON → schedule the timer, OFF → clear
   *  it. The timer body itself also checks visibility + burst-throttle
   *  so a `visibilitychange` while the timer is mid-flight doesn't
   *  double-fire. */
  private _toggleAutoRefresh() {
    this.autoRefresh = !this.autoRefresh;
    if (this.autoRefresh) {
      this._autoTimer = window.setInterval(() => {
        if (document.visibilityState !== "visible") return;
        if (Date.now() - this._lastFetchAt < BURST_THROTTLE_MS) return;
        void this._loadFromBackend({ replace: true });
      }, AUTO_REFRESH_MS);
    } else if (this._autoTimer !== null) {
      clearInterval(this._autoTimer);
      this._autoTimer = null;
    }
  }

  /** Build the query-string the helper consumes. Empty filters are
   *  omitted so the server applies its default-30-day range cap (§5.7
   *  ADD-MED-3) on the first call. */
  private _buildQuery(cursor: string): string {
    const params = new URLSearchParams();
    if (this.eventFilter)   params.set("event",   this.eventFilter);
    if (this.outcomeFilter) params.set("outcome", this.outcomeFilter);
    params.set("limit", String(PAGE_SIZE_DEFAULT));
    if (cursor) params.set("cursor", cursor);
    return params.toString();
  }

  /** Fetch a single page from /v1/audit/events. Backend rejection or
   *  network failure keeps the prior `events` array intact so the
   *  fixture / last successful page stays on screen (per
   *  [[design_lit_view_backend_wire_pattern]] rejects-keeps-fixture
   *  contract). */
  private async _loadFromBackend(opts: { replace: boolean }): Promise<void> {
    this.loading = true;
    this.errorMsg = "";
    this._lastFetchAt = Date.now();
    try {
      const qs = this._buildQuery(opts.replace ? "" : this.cursor);
      const res = await apiFetch(`/v1/audit/events?${qs}`);
      if (!res.ok) throw new Error(`status=${res.status}`);
      const json = await res.json() as { success?: boolean; data?: unknown; Value?: unknown; value?: unknown };
      // [[feedback_coreapi_data_envelope_not_value]] — parse from data
      // first, fall through to Value / value for legacy/cased shapes
      // some early mocks emit. Real coreapi.Response[T] is always `data`.
      const payload = (json?.data ?? json?.Value ?? json?.value) as
        | { events?: AuditEvent[]; next_cursor?: string }
        | undefined;
      if (!payload || !Array.isArray(payload.events)) {
        // Malformed payload — treat as empty page on first load (replace
        // the fixture) so the operator sees the empty-state copy rather
        // than a stale design row that doesn't match reality.
        if (opts.replace) this.events = [];
        this.cursor = "";
        this.boundaryBadge = false;
        return;
      }
      const newRows = payload.events;
      const nextCursor = typeof payload.next_cursor === "string" ? payload.next_cursor : "";
      if (opts.replace) {
        this.events = newRows;
        this.expanded = new Set<number>();
      } else {
        this.events = [...this.events, ...newRows];
      }
      this.cursor = nextCursor;
      // Cerberus #29 LOW — surface the same-second-pagination warning
      // at every page boundary (i.e. whenever the server returned a
      // non-empty next_cursor).
      this.boundaryBadge = nextCursor !== "";
    } catch (err) {
      this.errorMsg = this.t.error;
      // Keep this.events / this.cursor unchanged — last good state
      // remains on screen per the wire-pattern's
      // "rejects-keeps-fixture" contract.
      // eslint-disable-next-line no-console
      console.error("audit-window: load failed", err);
    } finally {
      this.loading = false;
    }
  }

  /** Copy a string to the system clipboard via the user-gesture-bound
   *  navigator.clipboard.writeText. Best-effort — older webview
   *  contexts without clipboard write permission silently no-op. */
  private async _copyToClipboard(text: string): Promise<void> {
    try {
      await navigator.clipboard?.writeText?.(text);
    } catch {
      // ignore — clipboard permission denied or unavailable
    }
  }

  /** JSON-Lines export — one event per line, NDJSON shape that mirrors
   *  the on-disk format at ~/Lethean/audit/<YYYY-MM-DD>.ndjson. Blob
   *  download via an off-screen <a> element. */
  private _exportJSONL() {
    const lines = this.events.map(ev => JSON.stringify(ev)).join("\n");
    this._download(lines, "audit-events.jsonl", "application/x-ndjson");
  }

  /** CSV export — flattens the top-level fields and stringifies Meta as
   *  JSON so spreadsheet tools can ingest without losing the per-event
   *  context. Quoting follows RFC 4180 (double-quote escape via
   *  doubling). */
  private _exportCSV() {
    const cols = ["ts", "event", "outcome", "account_id", "scope", "request_id", "meta"];
    const esc = (v: unknown) => {
      const s = v === undefined || v === null ? "" : String(v);
      if (/[",\n\r]/.test(s)) return `"${s.replace(/"/g, '""')}"`;
      return s;
    };
    const rows = [cols.join(",")];
    for (const ev of this.events) {
      const meta = ev.meta ? JSON.stringify(ev.meta) : "";
      rows.push([
        esc(ev.ts), esc(ev.event), esc(ev.outcome ?? ""),
        esc(ev.account_id ?? ""), esc(ev.scope ?? ""),
        esc(ev.request_id ?? ""), esc(meta),
      ].join(","));
    }
    this._download(rows.join("\n"), "audit-events.csv", "text/csv");
  }

  /** Off-screen anchor click → triggers a download. Object-URL is
   *  revoked on the next tick so the browser doesn't leak the blob. */
  private _download(body: string, filename: string, mime: string) {
    try {
      const blob = new Blob([body], { type: mime });
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = filename;
      a.style.display = "none";
      document.body.appendChild(a);
      a.click();
      // Revoke + remove on a microtask so the click fires first.
      queueMicrotask(() => {
        URL.revokeObjectURL(url);
        a.remove();
      });
    } catch (err) {
      // Non-browser context (test happy-dom) — log + skip.
      // eslint-disable-next-line no-console
      console.error("audit-window: export failed", err);
    }
  }

  /** Toggle a row's expansion state. Index is the position in the
   *  CURRENT filtered list — recomputed each render so it's stable
   *  within a render pass. */
  private _toggleExpanded(idx: number) {
    const next = new Set(this.expanded);
    if (next.has(idx)) next.delete(idx); else next.add(idx);
    this.expanded = next;
  }

  /** Apply chip filters in-memory. Backend-side filters are applied via
   *  the query string on the next refresh — until then we filter what
   *  we already have so toggling chips feels instant. */
  private _filteredEvents(): AuditEvent[] {
    if (!this.eventFilter && !this.outcomeFilter) return this.events;
    return this.events.filter(ev => {
      if (this.eventFilter && ev.event !== this.eventFilter) return false;
      if (this.outcomeFilter && ev.outcome !== this.outcomeFilter) return false;
      return true;
    });
  }

  /** Format a unix-second TS as a compact local-time string. Falls
   *  back to raw seconds if Intl is unavailable. */
  private _fmtTs(ts: number): string {
    if (!ts) return "";
    try {
      return new Date(ts * 1000).toISOString().replace("T", " ").replace(/\.\d+Z$/, "Z");
    } catch {
      return String(ts);
    }
  }

  /** Truncate a hex hash + return both the visible prefix and the
   *  full hash for the tooltip. RFC §4.1 — first-8-hex pattern. */
  private _truncHash(hex: string): { short: string; full: string } {
    if (!hex) return { short: "", full: "" };
    const short = hex.length > HASH_TRUNCATE ? hex.slice(0, HASH_TRUNCATE) + "…" : hex;
    return { short, full: hex };
  }

  /** Outcome → status-dot variant. RFC §4.4 palette. */
  private _outcomeVariant(outcome?: string): "ok" | "warn" | "err" | "idle" {
    switch (outcome) {
      case "ok":     return "ok";
      case "failed": return "warn";
      case "denied": return "warn";
      case "error":  return "err";
      default:       return "idle";
    }
  }

  /** Render Meta as a definition-list. Each value goes through Lit
   *  TEXT-INTERPOLATION ONLY — RFC §4.5 / Cerberus #29 ADD-MED-1.
   *  NEVER unsafeHTML, NEVER innerHTML=, NEVER eval. The test
   *  TestAuditWindow_MetaValueHTMLEscaped_Bad asserts a `<img>` in a
   *  Meta value renders as literal text. */
  private _renderMeta(meta: Record<string, unknown> | undefined): TemplateResult {
    if (!meta) return html`<span style="color:var(--fg-3); font-size:11px;">no meta</span>`;
    const entries = Object.entries(meta);
    if (entries.length === 0) return html`<span style="color:var(--fg-3); font-size:11px;">no meta</span>`;
    return html`
      <dl style="display:grid; grid-template-columns:max-content 1fr; gap:4px 12px; font-family:var(--font-mono); font-size:11px; margin:0;">
        ${entries.map(([k, v]) => html`
          <dt style="color:var(--fg-3);">${String(k)}</dt>
          <dd style="margin:0; color:var(--fg-1); word-break:break-all;">
            ${this._renderMetaValue(v)}
          </dd>
        `)}
      </dl>
    `;
  }

  /** Per-value rendering — recurses into nested objects via
   *  JSON.stringify into a <pre>. All branches text-interpolate; none
   *  reach for unsafe-html. */
  private _renderMetaValue(v: unknown): TemplateResult {
    if (v === null || v === undefined) {
      return html`<span style="color:var(--fg-3);">${String(v)}</span>`;
    }
    if (typeof v === "string") {
      // Long hash-shaped strings get the truncated + copy treatment.
      if (/^[0-9a-f]{16,}$/i.test(v)) {
        const { short, full } = this._truncHash(v);
        return html`
          <span title=${full} style="font-family:var(--font-mono);">${short}</span>
          <button
            @click=${() => this._copyToClipboard(full)}
            style="margin-left:6px; padding:1px 6px; font-size:10px; border-radius:4px; background:rgba(255,255,255,0.05); border:1px solid rgba(255,255,255,0.10); color:var(--fg-2); cursor:pointer;"
            title="Copy full value">copy</button>
        `;
      }
      return html`<span>${v}</span>`;
    }
    if (typeof v === "number" || typeof v === "boolean") {
      return html`<span style="color:var(--brand-300);">${String(v)}</span>`;
    }
    // Object / array — JSON.stringify into <pre>. Text-interpolation
    // keeps the XSS guard intact even for nested attacker-influenced
    // strings.
    return html`<pre style="margin:0; white-space:pre-wrap; word-break:break-all;">${JSON.stringify(v, null, 2)}</pre>`;
  }

  render() {
    const rows = this._filteredEvents();
    const toolbar = html`
      <select
        @change=${(e: Event) => { this.eventFilter = (e.target as HTMLSelectElement).value; }}
        .value=${this.eventFilter}
        style="background:rgba(0,0,0,0.20); border:1px solid rgba(255,255,255,0.08); color:var(--fg-1); border-radius:6px; padding:4px 8px; font-size:11px;">
        <option value="">${this.t.filterEventAll}</option>
        ${AUDIT_EVENT_NAMES.map(name => html`<option value=${name}>${name}</option>`)}
      </select>
      <select
        @change=${(e: Event) => { this.outcomeFilter = (e.target as HTMLSelectElement).value; }}
        .value=${this.outcomeFilter}
        style="background:rgba(0,0,0,0.20); border:1px solid rgba(255,255,255,0.08); color:var(--fg-1); border-radius:6px; padding:4px 8px; font-size:11px;">
        <option value="">${this.t.filterOutcomeAll}</option>
        ${AUDIT_OUTCOMES.map(o => html`<option value=${o}>${o}</option>`)}
      </select>
      <div style="flex:1"></div>
      <lthn-btn tone="ghost" size="sm"
        @click=${() => this._loadFromBackend({ replace: true })}
        ?dim=${this.loading}>
        <i class="fa-solid fa-rotate" style="font-size:10px;"></i> ${this.t.btnRefresh}
      </lthn-btn>
      <lthn-btn tone=${this.autoRefresh ? "primary" : "ghost"} size="sm"
        @click=${() => this._toggleAutoRefresh()}
        title=${this.t.autoRefreshCaveat}>
        <i class="fa-solid fa-clock" style="font-size:10px;"></i> ${this.t.btnAutoRefresh}
      </lthn-btn>
      <lthn-btn tone="ghost" size="sm" @click=${() => this._exportJSONL()}>
        <i class="fa-solid fa-file-export" style="font-size:10px;"></i> ${this.t.btnExportJSONL}
      </lthn-btn>
      <lthn-btn tone="ghost" size="sm" @click=${() => this._exportCSV()}>
        <i class="fa-solid fa-file-csv" style="font-size:10px;"></i> ${this.t.btnExportCSV}
      </lthn-btn>
    `;

    const body = html`
      <div style="flex:1; display:flex; flex-direction:column; min-height:0; overflow:auto; font-family:var(--font-mono); font-size:11.5px;">
        ${this.errorMsg ? html`
          <div style="padding:10px 16px; background:rgba(255,76,76,0.08); color:var(--err-300, #ffb4b4); border-bottom:1px solid rgba(255,76,76,0.20);">
            <i class="fa-solid fa-triangle-exclamation" style="margin-right:6px;"></i> ${this.errorMsg}
          </div>
        ` : nothing}
        <div style="display:grid; grid-template-columns:170px 1.5fr 80px 110px 1fr; padding:8px 14px; border-bottom:1px solid rgba(255,255,255,0.06); color:var(--fg-3); font-size:10px; letter-spacing:0.04em; text-transform:uppercase;">
          <span>${this.t.colTs}</span>
          <span>${this.t.colEvent}</span>
          <span>${this.t.colOutcome}</span>
          <span>${this.t.colAccount}</span>
          <span>${this.t.colRequest}</span>
        </div>
        ${rows.length === 0 ? html`
          <div style="padding:24px 16px; text-align:center; color:var(--fg-3); font-family:var(--font-sans); font-size:12px;">
            ${this.t.empty}
          </div>
        ` : rows.map((ev, i) => this._renderRow(ev, i))}
        ${this.boundaryBadge ? html`
          <div class="audit-same-second-warning" style="padding:8px 16px; background:rgba(245,158,11,0.06); border-top:1px solid rgba(245,158,11,0.15); color:var(--warning-400); font-family:var(--font-sans); font-size:11px;">
            <i class="fa-solid fa-circle-info" style="margin-right:6px;"></i> ${this.t.sameSecondWarn}
          </div>
        ` : nothing}
        ${this.cursor ? html`
          <div style="padding:10px 16px; display:flex; justify-content:center;">
            <lthn-btn tone="ghost" size="sm" @click=${() => this._loadFromBackend({ replace: false })}>
              <i class="fa-solid fa-circle-plus" style="font-size:10px;"></i> ${this.t.btnLoadMore}
            </lthn-btn>
          </div>
        ` : nothing}
      </div>
    `;

    const footer = html`
      ${rows.length} events · session-tier · ${this.autoRefresh ? "auto-refresh on (30s, visibility-gated)" : "manual refresh"}
    `;

    return renderChrome({
      title: this.chrome.title,
      subtitle: this.chrome.subtitle,
      w: this.w, h: this.h,
      toolbar, body, footer,
      embedded: this.embedded,
    });
  }

  private _renderRow(ev: AuditEvent, idx: number): TemplateResult {
    const isExpanded = this.expanded.has(idx);
    const acct = this._truncHash(ev.account_id ?? "");
    const variant = this._outcomeVariant(ev.outcome);
    return html`
      <div class="audit-row" data-row-index=${idx}>
        <div
          @click=${() => this._toggleExpanded(idx)}
          style="display:grid; grid-template-columns:170px 1.5fr 80px 110px 1fr; padding:6px 14px; border-bottom:1px solid rgba(255,255,255,0.04); cursor:pointer; align-items:center;">
          <span style="color:var(--fg-2);">${this._fmtTs(ev.ts)}</span>
          <span style="color:var(--fg-0);">${ev.event}</span>
          <span style="display:flex; align-items:center; gap:6px;">
            <lthn-status-dot variant=${variant}></lthn-status-dot>
            <span style="color:var(--fg-2); font-size:10.5px;">${ev.outcome ?? ""}</span>
          </span>
          <span style="display:flex; align-items:center; gap:4px;">
            <span title=${acct.full} style="color:var(--fg-1);">${acct.short}</span>
            ${acct.full ? html`
              <button
                class="audit-copy-account"
                @click=${(e: Event) => { e.stopPropagation(); void this._copyToClipboard(acct.full); }}
                style="padding:1px 6px; font-size:9px; border-radius:4px; background:rgba(255,255,255,0.05); border:1px solid rgba(255,255,255,0.10); color:var(--fg-2); cursor:pointer;"
                title="Copy full account ID">copy</button>
            ` : nothing}
          </span>
          <span style="color:var(--fg-3); font-size:10.5px; overflow:hidden; text-overflow:ellipsis; white-space:nowrap;">${ev.request_id ?? ""}</span>
        </div>
        ${isExpanded ? html`
          <div class="audit-row-detail" style="padding:10px 22px 14px; background:rgba(0,0,0,0.12); border-bottom:1px solid rgba(255,255,255,0.05);">
            <div style="margin-bottom:6px; font-size:10px; color:var(--fg-3); letter-spacing:0.04em; text-transform:uppercase;">Meta</div>
            ${this._renderMeta(ev.meta)}
            <div style="margin-top:8px; font-size:10.5px; color:var(--fg-3); font-family:var(--font-sans);">
              ${this.t.accountHelp}
            </div>
          </div>
        ` : nothing}
      </div>
    `;
  }
}

customElements.define("lthn-audit-viewer", LthnAuditWindow);

export type { LthnAuditWindow };
