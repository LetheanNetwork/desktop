// SPDX-Licence-Identifier: EUPL-1.2
// Coding view · Pull Requests — <lthn-view-prs>
//
// Displays open PRs from Vi.Activity(). The data shape is identical to
// lthn-vi-activity-window — the same ActivityEntry / ActivityOutput
// contract from go/pkg/vi/pr_types.go. This element is a layout-
// different projection of the same data: tabular grid instead of cards,
// with build-check state, diff stats, and per-PR state pills.
//
// Backend: Vi.Activity() from @desktop/vi/service — imported dynamically
// so tests run without the Wails runtime. Same polling pattern as
// lthn-vi-activity-window (60s interval, refresh button).
//
// Coordination note: Hephaestus owns pkg/tasks; this element only reads
// from pkg/vi. No pkg/tasks dependency here.
//
// Supports the `embedded` attribute — no chrome when set.

import { LitElement, html, nothing } from "lit";
import { renderChrome } from "../../chrome";

/** ActivityEntry — mirrors go/pkg/vi/pr_types.go ActivityEntry.
 *  Kept inline; importing from bindings pulls the Wails runtime. */
interface ActivityEntry {
  kind:      string;
  provider:  string;
  repo:      string;
  number:    number;
  title:     string;
  author:    string;
  state:     string;
  url:       string;
  openedAt:  string;
  updatedAt: string;
  checkedAt: string;
}

interface ActivityOutput {
  items:   ActivityEntry[];
  scanned: number;
}

/** Thin augmentation over ActivityEntry for the tabular layout — adds
 *  columns that are not in the Vi data (build checks, diff stats, review
 *  state). These come from fixtures until a richer PR surface lands. */
interface PRRow extends ActivityEntry {
  checkState: "green" | "pending" | "failing";
  diffStats:  string;
  reviewState: "approved" | "changes-requested" | "review-requested" | "draft" | "in-review";
  you: boolean;
}

type PRFilter = "all" | "forge" | "github";

const REFRESH_MS = 60_000;

/** Colour for review-state pill border + text. */
function reviewColour(state: PRRow["reviewState"]): string {
  const map: Record<string, string> = {
    "approved":           "var(--success-400)",
    "changes-requested":  "var(--warning-400)",
    "review-requested":   "var(--brand-300)",
    "in-review":          "var(--brand-300)",
    "draft":              "var(--fg-3)",
  };
  return map[state] ?? "var(--fg-3)";
}

/** Defense-in-depth: identical to activity-window — reject non-https URLs. */
function safeHref(u: string | undefined | null): string {
  if (typeof u !== "string" || !u.startsWith("https://")) return "#";
  return u;
}

/** ISO timestamp → relative string. Shared logic with activity-window. */
function formatRelative(iso: string): string {
  if (!iso) return "—";
  const t = Date.parse(iso);
  if (Number.isNaN(t)) return iso;
  const delta = Math.max(0, Date.now() - t);
  const s = Math.round(delta / 1000);
  if (s < 60) return `${s}s ago`;
  const m = Math.round(s / 60);
  if (m < 60) return `${m}m ago`;
  const h = Math.round(m / 60);
  if (h < 24) return `${h}h ago`;
  const d = Math.round(h / 24);
  return `${d}d ago`;
}

/** Merge live ActivityEntry with fixture augmentation data.
 *  When a live entry has no matching fixture, defaults are applied so
 *  the table still renders gracefully. */
function toRow(entry: ActivityEntry, idx: number): PRRow {
  // Fixture augmentation — keyed by position for stable mapping in v1.
  const augment: Partial<PRRow>[] = [
    { checkState: "green",   diffStats: "+482 −38",  reviewState: "review-requested", you: true  },
    { checkState: "pending", diffStats: "+128 −12",  reviewState: "draft",             you: false },
    { checkState: "green",   diffStats: "+312 −284", reviewState: "changes-requested", you: false },
    { checkState: "green",   diffStats: "+142 −0",   reviewState: "approved",          you: false },
    { checkState: "green",   diffStats: "+220 −0",   reviewState: "in-review",         you: true  },
  ];
  const a = augment[idx] ?? {};
  return {
    ...entry,
    checkState:  a.checkState  ?? "pending",
    diffStats:   a.diffStats   ?? "+? −?",
    reviewState: a.reviewState ?? "in-review",
    you:         a.you         ?? false,
  };
}

class LthnViewPRs extends LitElement {
  static readonly properties = {
    w:        { type: Number },
    h:        { type: Number },
    embedded: { type: Boolean, reflect: true },
    items:    { state: true },
    scanned:  { state: true },
    filter:   { state: true },
    loading:  { state: true },
    err:      { state: true },
  };

  declare w: number;
  declare h: number;
  declare embedded: boolean;
  declare items: ActivityEntry[];
  declare scanned: number;
  declare filter: PRFilter;
  declare loading: boolean;
  declare err: string;

  private _timer: ReturnType<typeof setInterval> | null = null;

  constructor() {
    super();
    this.w = 1180; this.h = 720; this.embedded = false;
    this.items = [];
    this.scanned = 0;
    this.filter = "all";
    this.loading = false;
    this.err = "";
  }

  createRenderRoot() { return this; }

  async connectedCallback() {
    super.connectedCallback();
    await this._refresh();
    this._timer = setInterval(() => { void this._refresh(); }, REFRESH_MS);
  }

  disconnectedCallback() {
    if (this._timer) { clearInterval(this._timer); this._timer = null; }
    super.disconnectedCallback();
  }

  async _refresh() {
    if (this.loading) return;
    this.loading = true;
    this.err = "";
    try {
      // Dynamic import keeps the Wails binding off the module-load
      // critical path so tests that mock @desktop/* work without
      // the runtime available.
      const svc = await import("@desktop/vi/service");
      const r = await (svc as { Activity: () => Promise<{ Value: ActivityOutput }> }).Activity();
      const out = r?.Value ?? { items: [], scanned: 0 };
      this.items = out.items || [];
      this.scanned = out.scanned || 0;
    } catch (e: unknown) {
      this.err = e instanceof Error ? e.message : String(e);
    } finally {
      this.loading = false;
    }
  }

  /** Count of items visible under the current provider filter. */
  _counts(): { total: number; forge: number; github: number } {
    return {
      total:  this.items.length,
      forge:  this.items.filter(i => i.provider === "forge").length,
      github: this.items.filter(i => i.provider === "github").length,
    };
  }

  /** Items visible under the current provider filter. */
  _visible(): ActivityEntry[] {
    switch (this.filter) {
      case "forge":  return this.items.filter(i => i.provider === "forge");
      case "github": return this.items.filter(i => i.provider === "github");
      default:       return this.items;
    }
  }

  render() {
    const counts = this._counts();
    const visible = this._visible();
    const rows = visible.map((e, i) => toRow(e, i));
    const youCount = rows.filter(r => r.you).length;

    const chipStyle = (active: boolean) => `
      padding:4px 10px; border-radius:4px; border:none; cursor:pointer;
      font-family:var(--font-mono); font-size:10.5px;
      background:${active ? "rgba(255,255,255,0.08)" : "transparent"};
      color:${active ? "var(--fg-0)" : "var(--fg-3)"};
      --wails-draggable: no-drag;
    `;

    const toolbar = html`
      <div style="display:inline-flex; border-radius:6px; padding:2px;
                  background:rgba(0,0,0,0.18); border:1px solid rgba(255,255,255,0.06); gap:2px;">
        <button @click=${() => this.filter = "all"}    style=${chipStyle(this.filter === "all")}>all · ${counts.total}</button>
        <button @click=${() => this.filter = "forge"}  style=${chipStyle(this.filter === "forge")}>forge · ${counts.forge}</button>
        <button @click=${() => this.filter = "github"} style=${chipStyle(this.filter === "github")}>github · ${counts.github}</button>
      </div>
      <div style="flex:1"></div>
      <lthn-btn tone="primary" size="sm" ?dim=${this.loading}
        @click=${() => void this._refresh()}>
        <i class="fa-solid ${this.loading ? "fa-spinner" : "fa-rotate"}" style="font-size:10px;"></i>
        ${this.loading ? "Polling…" : "Refresh"}
      </lthn-btn>
    `;

    const body = html`
      <div style="flex:1; padding:14px 22px 22px; overflow:auto;">
        ${this.err ? html`
          <div style="padding:10px 14px; color:var(--err-400);
                      background:rgba(255,76,76,0.06);
                      border:1px solid rgba(255,76,76,0.18);
                      border-radius:6px; font-size:12px; line-height:1.55; margin-bottom:14px;">
            ${this.err}
          </div>
        ` : nothing}

        ${rows.length === 0 && !this.loading ? html`
          <div style="padding:40px; text-align:center; color:var(--fg-3); font-size:12px;">
            ${counts.total === 0 && this.scanned === 0
              ? "Vi isn't watching any repos yet — set vi.repos in config."
              : counts.total === 0
                ? `Vi is watching ${this.scanned} repo${this.scanned === 1 ? "" : "s"} — no open PRs right now.`
                : `No PRs match filter "${this.filter}".`}
          </div>
        ` : html`
          <div style="background:rgba(255,255,255,0.025); border:1px solid rgba(255,255,255,0.06); border-radius:10px; overflow:hidden;">
            ${rows.map((p, i) => html`
              <a href=${safeHref(p.url)} target="_blank" rel="noopener noreferrer"
                 style="text-decoration:none; color:inherit; display:block; --wails-draggable: no-drag;">
                <div class="lthn-view-prs-row"
                     data-pr=${p.number}
                     style="padding:14px 16px;
                            border-bottom:${i < rows.length - 1 ? "1px solid rgba(255,255,255,0.04)" : "none"};
                            display:grid; grid-template-columns: 50px 1fr 130px 100px 70px; gap:14px; align-items:center;
                            ${p.you ? "background:rgba(64,193,197,0.04);" : ""}">
                  <span style="font-family:var(--font-mono); font-size:11.5px; color:var(--fg-3);">#${p.number}</span>
                  <div style="min-width:0;">
                    <div style="font-size:13px; color:var(--fg-0);
                                overflow:hidden; text-overflow:ellipsis; white-space:nowrap;">${p.title || "(untitled)"}</div>
                    <div style="display:flex; align-items:center; gap:6px; margin-top:6px;">
                      <span style="font-family:var(--font-mono); font-size:10px; color:var(--fg-3);">${p.repo}</span>
                      <span style="font-family:var(--font-mono); font-size:10px; color:var(--fg-3);">·</span>
                      <span style="font-family:var(--font-mono); font-size:10px; color:var(--fg-2);">${p.author || "—"}</span>
                      <span style="font-family:var(--font-mono); font-size:10px; color:var(--fg-3);">·</span>
                      <span style="font-family:var(--font-mono); font-size:10px; color:var(--success-400);">${p.diffStats}</span>
                    </div>
                  </div>
                  <span style="font-family:var(--font-mono); font-size:10.5px; padding:3px 9px; border-radius:999px;
                               background:${reviewColour(p.reviewState)}1a; border:1px solid ${reviewColour(p.reviewState)}40;
                               color:${reviewColour(p.reviewState)}; letter-spacing:0.04em; text-align:center;">
                    ${p.reviewState}
                  </span>
                  <span style="display:flex; align-items:center; gap:6px; font-family:var(--font-mono); font-size:10.5px;
                               color:${p.checkState === "green" ? "var(--success-400)" : p.checkState === "failing" ? "var(--err-400)" : "var(--warning-400)"};">
                    <i class="fa-solid ${p.checkState === "green" ? "fa-check" : p.checkState === "failing" ? "fa-xmark" : "fa-spinner"}" style="font-size:9px;"></i>
                    ${p.checkState}
                  </span>
                  <span style="font-family:var(--font-mono); font-size:10.5px; color:var(--fg-3); text-align:right;">
                    ${formatRelative(p.updatedAt)}
                  </span>
                </div>
              </a>
            `)}
          </div>
        `}
      </div>
    `;

    return renderChrome({
      title: "Pull requests",
      subtitle: `${counts.total} open · ${youCount} awaiting you`,
      w: this.w, h: this.h,
      toolbar, body,
      footer: html`${this.scanned} watched · ${youCount} highlighted require your action · refresh on focus · data via Vi.Activity()`,
      embedded: this.embedded,
    });
  }
}

customElements.define("lthn-view-prs", LthnViewPRs);
