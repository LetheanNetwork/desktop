// SPDX-Licence-Identifier: EUPL-1.2
// Vi · sites — <lthn-vi-sites-window>
//
// First real-data wedge of the Vi Control Panel. Lists the
// catalogued probe targets and the latest probe outcome per
// domain — green dot for OK, red dot for failure, response time
// in ms, ISO timestamp of the last check.
//
// The probe loop lives in go/pkg/vi (queue-driven, ~60s cadence
// per site). This window is a polling surface — re-fetches on
// connect + every 30s while mounted. Real-time push via Wails
// Events.Emit is the §"Future: real-time push" follow-up tracked
// separately.
//
// Open via windowservice.Open("vi-sites"). No params today —
// catalogue + state come from the Vi service.

import { LitElement, html, nothing } from "lit";
import { renderChrome } from "../chrome";

interface SiteStatus {
  domain: string;
  stack: string;
  status: "green" | "amber" | "red";
  response: number;
  lastChecked: string;
  error?: string;
}

interface SitesOutput {
  sites: SiteStatus[];
  scanned: number;
}

type Filter = "all" | "ok" | "failing";

const REFRESH_MS = 30_000;

class LthnViSitesWindow extends LitElement {
  static readonly properties = {
    w:        { type: Number },
    h:        { type: Number },
    embedded: { type: Boolean, reflect: true },
    chrome:   { state: true },
    sites:    { state: true },
    scanned:  { state: true },
    filter:   { state: true },
    loading:  { state: true },
    err:      { state: true },
  };
  declare w: number;
  declare h: number;
  declare embedded: boolean;
  declare chrome: { title: string; subtitle: string };
  declare sites: SiteStatus[];
  declare scanned: number;
  declare filter: Filter;
  declare loading: boolean;
  declare err: string;

  private _timer: ReturnType<typeof setInterval> | null = null;

  constructor() {
    super();
    this.w = 920; this.h = 620; this.embedded = false;
    this.chrome = { title: "Sites", subtitle: "Vi · uptime watcher" };
    this.sites = [];
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
      const svc = await import("@desktop/vi/wailsservice");
      // Vi.Sites() returns core.Result whose Value is SitesOutput.
      // The Wails binding wraps it as core$0.Result, so we read
      // through to .Value.
      const r = await svc.Sites() as { Value: SitesOutput };
      const out = r?.Value ?? { sites: [], scanned: 0 };
      this.sites = out.sites || [];
      this.scanned = out.scanned || 0;
    } catch (e: unknown) {
      this.err = e instanceof Error ? e.message : String(e);
    } finally {
      this.loading = false;
    }
  }

  _counts() {
    return {
      total:   this.sites.length,
      ok:      this.sites.filter(s => s.status === "green").length,
      failing: this.sites.filter(s => s.status !== "green").length,
    };
  }

  _visible(): SiteStatus[] {
    switch (this.filter) {
      case "ok":      return this.sites.filter(s => s.status === "green");
      case "failing": return this.sites.filter(s => s.status !== "green");
      default:        return this.sites;
    }
  }

  _formatChecked(iso: string): string {
    if (!iso) return "—";
    const t = Date.parse(iso);
    if (Number.isNaN(t)) return iso;
    const delta = Math.max(0, Date.now() - t);
    const s = Math.round(delta / 1000);
    if (s < 60) return `${s}s ago`;
    const m = Math.round(s / 60);
    if (m < 60) return `${m}m ago`;
    const h = Math.round(m / 60);
    return `${h}h ago`;
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
        <button @click=${() => this.filter = "ok"} style=${chipStyle(this.filter === "ok")}>ok · ${counts.ok}</button>
        <button @click=${() => this.filter = "failing"} style=${chipStyle(this.filter === "failing")}>failing · ${counts.failing}</button>
      </div>
      <div style="flex:1"></div>
      <lthn-btn tone="primary" size="sm" ?dim=${this.loading}
        @click=${() => void this._refresh()}>
        <i class="fa-solid ${this.loading ? "fa-spinner" : "fa-rotate"}"
           style="font-size:10px;"></i>
        ${this.loading ? "Probing…" : "Refresh"}
      </lthn-btn>
    `;

    const dotColor = (status: SiteStatus["status"]) =>
      status === "green" ? "#34d399"
        : status === "amber" ? "#fbbf24"
          : "#f87171";

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
              ? "Vi isn't watching any sites yet — set vi.sites in config."
              : counts.total === 0
                ? `Vi is watching ${this.scanned} site${this.scanned === 1 ? "" : "s"} — no probe results yet, give it a moment.`
                : `no sites match filter "${this.filter}"`}
          </div>
        ` : html`
          <div style="display:grid;
                      grid-template-columns:repeat(auto-fill, minmax(280px, 1fr));
                      gap:10px;">
            ${visible.map(s => html`
              <div title=${s.domain}
                style="background:rgba(0,0,0,0.18); border-radius:8px;
                       padding:12px; display:flex; flex-direction:column;
                       gap:8px; text-align:left;
                       border:1px solid ${s.status === "green"
                         ? "rgba(52, 211, 153, 0.25)"
                         : s.status === "amber"
                           ? "rgba(251, 191, 36, 0.30)"
                           : "rgba(248, 113, 113, 0.35)"};
                       color:var(--fg-0);">
                <div style="display:flex; justify-content:space-between;
                            align-items:baseline; gap:8px;">
                  <span style="display:inline-flex; align-items:center; gap:6px;
                               font-weight:600; font-size:13px; color:var(--fg-0);
                               overflow:hidden; text-overflow:ellipsis; white-space:nowrap;">
                    <span style="width:8px; height:8px; border-radius:50%;
                                 background:${dotColor(s.status)}; flex:none;"></span>
                    ${s.domain}
                  </span>
                  <span style="font-family:var(--font-mono); font-size:10px;
                               color:var(--fg-3); flex:none;">
                    ${s.response}ms
                  </span>
                </div>
                ${s.stack ? html`
                  <div style="font-size:11px; color:var(--fg-2); line-height:1.4;">
                    ${s.stack}
                  </div>
                ` : nothing}
                <div style="font-family:var(--font-mono); font-size:10px;
                            color:var(--fg-3);">
                  ${this._formatChecked(s.lastChecked)}
                </div>
                ${s.error ? html`
                  <div style="font-size:10px; color:var(--err-400);
                              font-style:italic; line-height:1.4;
                              word-break:break-word;">
                    ${s.error}
                  </div>
                ` : nothing}
              </div>
            `)}
          </div>
        `}
      </div>
    `;

    return renderChrome({
      title: this.chrome.title, subtitle: this.chrome.subtitle,
      w: this.w, h: this.h, toolbar, body,
      footer: html`${this.scanned} watched · ${counts.total} reported ·
        ${counts.ok} ok · ${counts.failing} failing`,
      embedded: this.embedded,
    });
  }
}
customElements.define("lthn-vi-sites-window", LthnViSitesWindow);
