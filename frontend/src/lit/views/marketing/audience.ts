// SPDX-Licence-Identifier: EUPL-1.2
// Marketing · Audience — <lthn-view-audience>
//
// Subscriber segments with weekly growth + a sparkline over 12 weeks.
// Numbers stated, not celebrated — no "WE GREW!" toasts, no green
// thumbs-ups. The numbers themselves are the message.
//
// Two-shell: embedded attribute collapses chrome for app-shell mount.
//
// Data: fixture only for v1. Real source is a future
// `pkg/marketing/audience` Go service backed by signup-tagged segments
// (CRM source) + opt-in telemetry (runtime source). The { name, n,
// growth, src } shape is the contract.

import { LitElement, html } from "lit";
import { renderChrome } from "../../chrome";

interface Segment {
  name:   string;
  n:      number;
  growth: string;
  src:    string;
}

const FIXTURE_SEGMENTS: Segment[] = [
  { name: "All subscribers",           n: 8214, growth: "+184 / w", src: "all" },
  { name: "Local-AI developers",        n: 4892, growth: "+62 / w",  src: "signup-tagged" },
  { name: "Regulated SMB · UK",         n: 1284, growth: "+18 / w",  src: "sales-tagged" },
  { name: "Investors · followed",       n: 142,  growth: "+4 / w",   src: "manual" },
  { name: "Active runtime users (30d)", n: 2618, growth: "+312 / w", src: "telemetry · opt-in" },
];

/** Weekly subscriber totals over the last 12 weeks. Real source will be
 *  the same `pkg/marketing/audience` Go service — emitted as a number[]
 *  the <lthn-sparkline> element renders as a calm wave. */
const GROWTH_SPARK = "284,302,318,344,362,401,438,482,512,548";

class LthnViewAudience extends LitElement {
  static readonly properties = {
    w:        { type: Number },
    h:        { type: Number },
    embedded: { type: Boolean, reflect: true },
    segments: { state: true },
    loading:  { state: true },
  };
  declare w: number;
  declare h: number;
  declare embedded: boolean;
  declare segments: Segment[];
  declare loading: boolean;
  private _timer: ReturnType<typeof setInterval> | null = null;

  constructor() {
    super();
    this.w = 1180;
    this.h = 720;
    this.embedded = false;
    this.segments = FIXTURE_SEGMENTS;
    this.loading = false;
  }

  createRenderRoot() { return this; }

  async connectedCallback() {
    super.connectedCallback();
    await this._loadFromBackend();
    this._timer = setInterval(() => { void this._loadFromBackend(); }, 60_000);
  }

  disconnectedCallback() {
    if (this._timer) { clearInterval(this._timer); this._timer = null; }
    super.disconnectedCallback();
  }

  async _loadFromBackend(): Promise<void> {
    if (this.loading) return;
    this.loading = true;
    try {
      const svc = await import("@desktop/marketing/audience/service").catch(() => null);
      if (!svc || typeof (svc as { List?: unknown }).List !== "function") return;
      const r = await (svc as {
        List: (input: Record<string, never>) => Promise<{ Value?: { segments?: Segment[] } }>
      }).List({});
      const segments = r?.Value?.segments;
      if (segments && segments.length > 0) this.segments = segments;
    } catch {
      // Binding unavailable — keep fixture data.
    } finally {
      this.loading = false;
    }
  }

  render() {
    const segs = this.segments;
    const all = segs.find(s => s.name === "All subscribers");
    const total = all?.n ?? 0;
    const totalLabel = total.toLocaleString();

    const body = html`
      <div class="lthn-view-audience" style="flex:1; padding:18px 22px; display:flex; flex-direction:column; gap:18px; overflow:auto;">
        <div class="lthn-view-audience-hero" style="padding:18px 22px; border-radius:12px; background:rgba(255,255,255,0.025); border:1px solid rgba(255,255,255,0.06); display:grid; grid-template-columns: 1fr auto; gap:24px; align-items:center;">
          <div>
            <div style="font-family:var(--font-mono); font-size:10px; color:var(--fg-3); letter-spacing:0.10em; text-transform:uppercase;">Subscribers · 12 weeks</div>
            <div style="font-family:var(--font-mono); font-size:36px; color:var(--fg-0); font-weight:300; letter-spacing:-0.02em; margin-top:6px;">${totalLabel} <span style="font-size:14px; color:var(--success-400);">${all?.growth ?? ""}</span></div>
            <div style="font-size:11.5px; color:var(--fg-3); margin-top:4px;">Median weekly growth: 9.2 % · best week: +482 after launch tweet</div>
          </div>
          <lthn-sparkline data=${GROWTH_SPARK} max="600" color="var(--brand-400)" .width=${300} .height=${72}></lthn-sparkline>
        </div>
        <div class="lthn-view-audience-table" style="background:rgba(255,255,255,0.025); border:1px solid rgba(255,255,255,0.06); border-radius:10px; overflow:hidden;">
          <div style="display:grid; grid-template-columns: 1.6fr 100px 100px 1fr; gap:14px; padding:10px 16px; background:rgba(0,0,0,0.20); border-bottom:1px solid rgba(255,255,255,0.05); font-family:var(--font-mono); font-size:9.5px; color:var(--fg-3); letter-spacing:0.10em; text-transform:uppercase;">
            <span>Segment</span><span style="text-align:right;">Size</span><span style="text-align:right;">Weekly</span><span>Source</span>
          </div>
          ${segs.map((s, i) => html`
            <div class="lthn-view-audience-row" data-segment=${s.name} style="display:grid; grid-template-columns: 1.6fr 100px 100px 1fr; gap:14px; padding:13px 16px; border-bottom:${i < segs.length - 1 ? "1px solid rgba(255,255,255,0.04)" : "none"}; align-items:center;">
              <span style="font-size:13px; color:var(--fg-0);">${s.name}</span>
              <span style="font-family:var(--font-mono); font-size:12px; color:var(--fg-1); text-align:right;">${s.n.toLocaleString()}</span>
              <span style="font-family:var(--font-mono); font-size:11px; color:var(--success-400); text-align:right;">${s.growth}</span>
              <span style="font-family:var(--font-mono); font-size:10.5px; color:var(--fg-3);">${s.src}</span>
            </div>
          `)}
        </div>
      </div>
    `;

    return renderChrome({
      title:    "Audience",
      subtitle: `${segs.length} segments · 8.2 K subscribers`,
      w: this.w, h: this.h,
      embedded: this.embedded,
      body,
      footer: html`signup tagging · GDPR-compliant · opt-in telemetry · no third-party tracking`,
    });
  }
}

customElements.define("lthn-view-audience", LthnViewAudience);
