// SPDX-Licence-Identifier: EUPL-1.2
// Vi · activity — <lthn-vi-activity-window>
//
// Second real-data wedge of the Vi Control Panel. Lists open
// pull requests across the configured Forgejo + GitHub repos —
// one card per open PR with title, repo, author, opened-at /
// updated-at relative timestamps, and a clickable upstream link.
//
// The fetcher lives in go/pkg/vi (queue-driven, ~5min cadence per
// repo). This window is a polling surface — re-fetches on connect
// + every 60s while mounted. Real-time push via Wails Events.Emit
// is the §"Future: real-time push" follow-up tracked separately.
//
// Open via windowservice.Open("vi-activity"). No params today —
// catalogue + state come from the Vi service.

import { LitElement, html, nothing } from "lit";
import { renderChrome } from "../chrome";

interface ActivityEntry {
  kind: string;
  provider: string;
  repo: string;
  number: number;
  title: string;
  author: string;
  state: string;
  url: string;
  openedAt: string;
  updatedAt: string;
  checkedAt: string;
}

interface ActivityOutput {
  items: ActivityEntry[];
  scanned: number;
}

type Filter = "all" | "forge" | "github";

const REFRESH_MS = 60_000;

class LthnViActivityWindow extends LitElement {
  static readonly properties = {
    w:        { type: Number },
    h:        { type: Number },
    embedded: { type: Boolean, reflect: true },
    chrome:   { state: true },
    items:    { state: true },
    scanned:  { state: true },
    filter:   { state: true },
    loading:  { state: true },
    err:      { state: true },
  };
  declare w: number;
  declare h: number;
  declare embedded: boolean;
  declare chrome: { title: string; subtitle: string };
  declare items: ActivityEntry[];
  declare scanned: number;
  declare filter: Filter;
  declare loading: boolean;
  declare err: string;

  private _timer: ReturnType<typeof setInterval> | null = null;

  constructor() {
    super();
    this.w = 920; this.h = 620; this.embedded = false;
    this.chrome = { title: "Activity", subtitle: "Vi · open pull requests" };
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
      const svc = await import("@desktop/vi/service");
      // Vi.Activity() returns core.Result whose Value is
      // ActivityOutput. The Wails binding wraps it as
      // core$0.Result, so we read through to .Value.
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

  _counts() {
    return {
      total:  this.items.length,
      forge:  this.items.filter(i => i.provider === "forge").length,
      github: this.items.filter(i => i.provider === "github").length,
    };
  }

  _visible(): ActivityEntry[] {
    switch (this.filter) {
      case "forge":  return this.items.filter(i => i.provider === "forge");
      case "github": return this.items.filter(i => i.provider === "github");
      default:       return this.items;
    }
  }

  _formatRelative(iso: string): string {
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

  _providerLabel(p: string): string {
    if (p === "forge") return "forge";
    if (p === "github") return "github";
    return p;
  }

  _providerColor(p: string): string {
    // Forge — Lethean indigo accent; GitHub — neutral grey. Both
    // sit on the dark-only palette per the desktop's accessibility
    // floor (Snider's brightness-sensitivity baseline).
    if (p === "forge") return "rgba(129, 140, 248, 0.9)";
    if (p === "github") return "rgba(156, 163, 175, 0.9)";
    return "var(--fg-3)";
  }

  render() {
    const counts = this._counts();
    const visible = this._visible();

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
        <button @click=${() => this.filter = "all"} style=${chipStyle(this.filter === "all")}>all · ${counts.total}</button>
        <button @click=${() => this.filter = "forge"} style=${chipStyle(this.filter === "forge")}>forge · ${counts.forge}</button>
        <button @click=${() => this.filter = "github"} style=${chipStyle(this.filter === "github")}>github · ${counts.github}</button>
      </div>
      <div style="flex:1"></div>
      <lthn-btn tone="primary" size="sm" ?dim=${this.loading}
        @click=${() => void this._refresh()}>
        <i class="fa-solid ${this.loading ? "fa-spinner" : "fa-rotate"}"
           style="font-size:10px;"></i>
        ${this.loading ? "Polling…" : "Refresh"}
      </lthn-btn>
    `;

    const body = html`
      <div style="flex:1; min-height:0; overflow:auto; padding:16px 18px;">
        ${this.err ? html`
          <div style="padding:10px 14px; color:var(--err-400);
                      background:rgba(255,76,76,0.06);
                      border:1px solid rgba(255,76,76,0.18);
                      border-radius:6px; font-size:12px;
                      line-height:1.55; margin-bottom:14px;">
            ${this.err}
          </div>
        ` : nothing}
        ${visible.length === 0 && !this.loading ? html`
          <div style="padding:40px; text-align:center; color:var(--fg-3);
                      font-size:12px;">
            ${counts.total === 0 && this.scanned === 0
              ? "Vi isn't watching any repos yet — set vi.repos in config."
              : counts.total === 0
                ? `Vi is watching ${this.scanned} repo${this.scanned === 1 ? "" : "s"} — no open PRs right now, or the fetcher hasn't run yet.`
                : `no PRs match filter "${this.filter}"`}
          </div>
        ` : html`
          <div style="display:flex; flex-direction:column; gap:8px;">
            ${visible.map(i => html`
              <a href=${i.url} target="_blank" rel="noopener noreferrer"
                title=${i.title}
                style="text-decoration:none; color:inherit;
                       --wails-draggable: no-drag;">
                <div style="background:rgba(0,0,0,0.18); border-radius:8px;
                            padding:12px 14px; display:flex; flex-direction:column;
                            gap:6px; text-align:left;
                            border:1px solid rgba(255,255,255,0.06);
                            color:var(--fg-0);
                            transition:border-color 120ms ease;">
                  <div style="display:flex; justify-content:space-between;
                              align-items:baseline; gap:10px;">
                    <span style="font-weight:600; font-size:13px; color:var(--fg-0);
                                 overflow:hidden; text-overflow:ellipsis; white-space:nowrap;
                                 flex:1; min-width:0;">
                      ${i.title || "(untitled)"}
                    </span>
                    <span style="font-family:var(--font-mono); font-size:10px;
                                 color:${this._providerColor(i.provider)}; flex:none;">
                      ${this._providerLabel(i.provider)} · #${i.number}
                    </span>
                  </div>
                  <div style="font-family:var(--font-mono); font-size:10.5px;
                              color:var(--fg-2);
                              overflow:hidden; text-overflow:ellipsis; white-space:nowrap;">
                    ${i.repo}${i.author ? ` · by ${i.author}` : ""}
                  </div>
                  <div style="font-family:var(--font-mono); font-size:10px;
                              color:var(--fg-3); display:flex; gap:14px;">
                    <span>opened ${this._formatRelative(i.openedAt)}</span>
                    <span>updated ${this._formatRelative(i.updatedAt)}</span>
                  </div>
                </div>
              </a>
            `)}
          </div>
        `}
      </div>
    `;

    return renderChrome({
      title: this.chrome.title, subtitle: this.chrome.subtitle,
      w: this.w, h: this.h, toolbar, body,
      footer: html`${this.scanned} watched · ${counts.total} open ·
        ${counts.forge} forge · ${counts.github} github`,
      embedded: this.embedded,
    });
  }
}
customElements.define("lthn-vi-activity-window", LthnViActivityWindow);
