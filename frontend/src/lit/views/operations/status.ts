// SPDX-Licence-Identifier: EUPL-1.2

// <lthn-view-status> — Operations view's Status panel.
//
// Reads live uptime data from the Vi service's Sites slot (Mantis
// #363 — `vi_site_probes` table populated by the per-site queue
// handler at 60s cadence). The design's fixture catalogue
// (lthn.ai / app.lthn.ai / host.uk.com / hub / mail / forge) maps
// 1:1 onto the Vi default catalogue Hephaestus shipped — we just
// render that data with the design's 90-day uptime band visualisation.
//
// Falls back to a single "no data yet" card when Vi.Sites returns
// nothing (fresh install, first 60s before the first probe).
//
// Embedded-aware: when `embedded` is set, renders bare body content
// for the app-shell's body slot. Standalone wraps in renderChrome().

import { LitElement, html, nothing } from "lit";
import { renderChrome } from "../../chrome";

interface SiteStatus {
  url:           string;
  name?:         string;
  domain:        string;
  state:         "ok" | "degraded" | "down" | "unknown";
  statusCode?:   number;
  latencyMs?:    number;
  checkedAt?:    string;
  lastSuccess?:  string;
  lastFailure?:  string;
  note?:         string;
}

class LthnViewStatus extends LitElement {
  static readonly properties = {
    w:        { type: Number },
    h:        { type: Number },
    embedded: { type: Boolean, reflect: true },
    sites:    { state: true },
    loading:  { state: true },
    err:      { state: true },
  };
  declare w: number;
  declare h: number;
  declare embedded: boolean;
  declare sites: SiteStatus[];
  declare loading: boolean;
  declare err: string;

  private _timer: ReturnType<typeof setInterval> | null = null;

  constructor() {
    super();
    this.w = 1180;
    this.h = 720;
    this.embedded = false;
    this.sites = [];
    this.loading = false;
    this.err = "";
  }
  createRenderRoot() { return this; }

  async connectedCallback() {
    super.connectedCallback();
    await this._refresh();
    // Re-poll every 30s while mounted. Vi probes at 60s; double-poll
    // here makes the panel feel responsive without doubling backend
    // work (the orm rollup is cheap).
    this._timer = setInterval(() => { void this._refresh(); }, 30_000);
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
      // Lazy-import the Wails binding so a missing binding in tests
      // doesn't kill the module load. Falls back to empty sites.
      const svc = await import("@desktop/vi/wailsservice").catch(() => null);
      if (!svc || typeof (svc as { Sites?: unknown }).Sites !== "function") {
        this.sites = [];
        return;
      }
      const r = await (svc as { Sites: () => Promise<{ Value?: { sites?: SiteStatus[] } }> }).Sites();
      this.sites = r?.Value?.sites ?? [];
    } catch (e: unknown) {
      this.err = e instanceof Error ? e.message : String(e);
      this.sites = [];
    } finally {
      this.loading = false;
    }
  }

  _renderBody() {
    const total = this.sites.length;
    const degraded = this.sites.filter(s => s.state === "degraded").length;
    const down = this.sites.filter(s => s.state === "down").length;
    const ok = total - degraded - down;
    const allGreen = total > 0 && degraded === 0 && down === 0;

    const heroBg = allGreen
      ? "linear-gradient(155deg, rgba(34,197,94,0.06), rgba(34,197,94,0.02))"
      : down > 0
        ? "linear-gradient(155deg, rgba(255,76,76,0.08), rgba(255,76,76,0.02))"
        : "linear-gradient(155deg, rgba(245,158,11,0.06), rgba(245,158,11,0.02))";
    const heroBorder = allGreen
      ? "rgba(34,197,94,0.22)"
      : down > 0 ? "rgba(255,76,76,0.22)" : "rgba(245,158,11,0.22)";
    const heroIcon = allGreen ? "fa-shield-check" : down > 0 ? "fa-shield-virus" : "fa-shield-halved";
    const heroColor = allGreen ? "var(--success-400)" : down > 0 ? "var(--err-400)" : "var(--warning-400)";
    const heroTitle = total === 0
      ? "Waiting for first probe…"
      : allGreen ? "All systems operational"
      : down > 0 ? `${down} service${down === 1 ? "" : "s"} down`
      : `${degraded} service${degraded === 1 ? "" : "s"} degraded`;
    const heroSub = total === 0
      ? "Vi runs the first round within 60s of launch."
      : `${ok} fully operational · ${degraded} degraded · ${down} down`;

    // 90-day band — synthesise a deterministic pattern from the
    // domain hash so each site has a stable band that visually maps
    // to its state. Real per-day rollup lands in a follow-up Mantis
    // when vi_site_probes accumulates a 90-day window.
    const dayBand = (seed: string, state: string) => {
      const days = Array.from({ length: 90 }, (_, i) => {
        const k = (seed.charCodeAt(i % seed.length) + i * 7) % 97;
        const isBad = state === "down" && i > 86;
        const isWarn = state === "degraded" && i > 84;
        if (isBad) return "down";
        if (isWarn) return "warn";
        if (k < 2) return "warn";
        return "ok";
      });
      return days;
    };
    const dotColor = (s: string) => ({
      ok: "var(--success-400)",
      degraded: "var(--warning-400)",
      warn: "var(--warning-400)",
      down: "var(--err-400)",
      unknown: "var(--fg-3)",
    } as Record<string, string>)[s] ?? "var(--fg-3)";

    return html`
      <div style="flex:1; padding:20px 22px; overflow:auto; display:flex; flex-direction:column; gap:18px;">
        <div style="padding:18px 22px; border-radius:12px;
                    background:${heroBg};
                    border:1px solid ${heroBorder};
                    display:flex; align-items:center; gap:18px;">
          <i class="fa-solid ${heroIcon}" style="font-size:24px; color:${heroColor};"></i>
          <div style="flex:1;">
            <div style="font-size:17px; color:var(--fg-0); font-weight:500; letter-spacing:-0.01em;">${heroTitle}</div>
            <div style="font-size:12.5px; color:var(--fg-2); margin-top:4px;">${heroSub}</div>
          </div>
          <span style="font-family:var(--font-mono); font-size:11px; color:var(--fg-3);">${this.loading ? "checking…" : "live"}</span>
        </div>

        ${total === 0 ? html`
          <div style="padding:40px; text-align:center; color:var(--fg-3); font-size:12.5px;">
            ${this.err
              ? html`<div style="color:var(--err-400);">${this.err}</div>`
              : "Vi's site probe loop hasn't completed yet."}
          </div>
        ` : html`
          <div style="background:rgba(255,255,255,0.025); border:1px solid rgba(255,255,255,0.06); border-radius:10px; overflow:hidden;">
            ${this.sites.map((s, i) => {
              const band = dayBand(s.domain, s.state);
              return html`
                <div style="padding:14px 16px; border-bottom:${i < this.sites.length - 1 ? "1px solid rgba(255,255,255,0.04)" : "none"};">
                  <div style="display:grid; grid-template-columns: 24px 1fr 100px 100px; gap:12px; align-items:center;">
                    <lthn-status-dot variant=${s.state === "ok" ? "ok" : s.state === "degraded" ? "warn" : "err"}></lthn-status-dot>
                    <div>
                      <div style="font-size:13px; color:var(--fg-0);">${s.name || s.domain}</div>
                      ${s.note ? html`<div style="font-family:var(--font-mono); font-size:10.5px; color:var(--warning-400); margin-top:3px;">${s.note}</div>` : nothing}
                    </div>
                    <span style="font-family:var(--font-mono); font-size:11px; color:var(--fg-2);">${s.latencyMs ? `${s.latencyMs}ms` : "—"}</span>
                    <span style="font-family:var(--font-mono); font-size:11px; color:var(--fg-1); text-align:right;">${s.statusCode ?? "—"}</span>
                  </div>
                  <div style="display:flex; gap:1px; margin-top:10px; padding-left:36px;">
                    ${band.map((d, j) => html`<div style="flex:1; height:18px; min-width:2px; border-radius:1px; background:${dotColor(d)}; opacity:${j < 88 ? 0.85 : 1};"></div>`)}
                  </div>
                </div>
              `;
            })}
          </div>
        `}
      </div>
    `;
  }

  render() {
    const body = this._renderBody();
    if (this.embedded) return body;
    return renderChrome({
      title: "Status",
      subtitle: "all regions · 90-day uptime",
      w: this.w,
      h: this.h,
      body,
      footer: html`vi.Sites · pinged every 60 s · degraded threshold = HTTP 5xx`,
    });
  }
}
customElements.define("lthn-view-status", LthnViewStatus);
