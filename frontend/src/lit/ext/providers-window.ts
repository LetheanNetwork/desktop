// SPDX-Licence-Identifier: EUPL-1.2
// E3.x · providers — <lthn-providers-window>
//
// Curated marketplace of service providers we either integrate with
// or recommend. Sits in the EXTEND nav group alongside Integrations
// and Tools · MCP. Different from Fleet → Agents:
//
//   Fleet → Agents    : LLM endpoints + personas (Team Members)
//   Providers (here)  : service-layer providers (VPS / CDN / DNS /
//                       SSL / comms / analytics / marketplace apps)
//
// Affiliate-eligible providers carry a tiny "supports lthn" tag so
// the user knows clicking helps us. Every external click resolves
// through ref.lthn.ai for attribution; we never reference upstream
// URLs directly. See _providers-catalogue.ts for the canonical list.

import { LitElement, html, nothing } from "lit";
import { renderChrome } from "../chrome";
import {
  PROVIDERS, byCategory, refURL,
  type ProviderCategory, type ServiceProvider,
  CATEGORY_LABELS, CATEGORY_BLURBS,
} from "./_providers-catalogue";

class LthnProvidersWindow extends LitElement {
  static readonly properties = {
    w:        { type: Number },
    h:        { type: Number },
    embedded: { type: Boolean, reflect: true },
    chrome:   { state: true },
  };
  declare w:        number;
  declare h:        number;
  declare embedded: boolean;
  declare chrome:   { title: string; subtitle: string };

  constructor() {
    super();
    this.w = 1080; this.h = 700; this.embedded = false;
    this.chrome = { title: "Providers", subtitle: "integrated services + Lethean apps" };
  }
  createRenderRoot() { return this; }

  /** Copy text to clipboard via the WebView's clipboard API. Used by
   *  the per-provider "Search: <name>" affordance so users in a context
   *  where ref.lthn.ai isn't reachable can grab the search term in one
   *  click. Silent on failure; the clipboard API throws in some private
   *  WebView contexts and we don't want to surface that as a UI error. */
  private async _copyText(e: Event, text: string) {
    e.preventDefault();
    try { await navigator.clipboard?.writeText(text); }
    catch { /* clipboard unavailable — silent */ }
  }

  private renderProviderCard(p: ServiceProvider) {
    const isExternal = p.status === "external";
    const isComingSoon = p.status === "coming-soon";
    const isLive = p.status === "live";

    const statusPill = isLive
      ? html`<lthn-state-pill variant="ok">integrated</lthn-state-pill>`
      : isComingSoon
        ? html`<lthn-state-pill variant="preview">coming soon</lthn-state-pill>`
        : nothing;

    const partyPill = p.firstParty
      ? html`<lthn-state-pill variant="latest">Lethean</lthn-state-pill>`
      : nothing;

    return html`
      <div style="display:flex; flex-direction:column; gap:10px;
                  padding:14px 16px;
                  background:rgba(255,255,255,0.03);
                  border:1px solid rgba(255,255,255,0.06);
                  border-radius:10px;">
        <div style="display:flex; align-items:center; gap:10px;">
          <div style="width:32px; height:32px;
                      display:flex; align-items:center; justify-content:center;
                      background:rgba(64,193,197,0.08);
                      border-radius:8px;">
            <i class="fa-solid ${p.icon}" style="font-size:14px; color:var(--brand-300);"></i>
          </div>
          <div style="font-size:13px; font-weight:600; color:var(--fg-0); flex:1;">${p.name}</div>
          ${partyPill}
          ${statusPill}
        </div>
        <div style="font-size:11.5px; color:var(--fg-2); line-height:1.55;">
          ${p.blurb}
        </div>
        <div style="display:flex; align-items:center; gap:10px; padding-top:4px; flex-wrap:wrap;">
          ${isExternal
            ? html`
              <a href=${refURL(p.id)} target="_blank" rel="noopener"
                 style="font-size:11.5px; color:var(--brand-300);
                        text-decoration:none;
                        font-family:var(--font-mono);
                        display:inline-flex; align-items:center; gap:6px;
                        --wails-draggable: no-drag;">
                ref.lthn.ai/${p.id}
                <i class="fa-solid fa-arrow-up-right-from-square" style="font-size:9px;"></i>
              </a>
              <span style="display:inline-flex; align-items:center; gap:6px;
                           font-size:10.5px; color:var(--fg-3);">
                Search:
                <code style="font-family:var(--font-mono); font-size:10.5px;
                             padding:1px 6px; border-radius:4px;
                             background:rgba(255,255,255,0.04);
                             border:1px solid rgba(255,255,255,0.07);
                             color:var(--fg-1);">${p.name}</code>
                <button @click=${(e: Event) => this._copyText(e, p.name)}
                        title="Copy search term to clipboard"
                        style="background:transparent; border:none; cursor:pointer;
                               color:var(--fg-3); padding:2px 4px; line-height:1;
                               --wails-draggable: no-drag;">
                  <i class="fa-regular fa-copy" style="font-size:11px;"></i>
                </button>
              </span>
            `
            : nothing}
          ${isLive && p.firstParty
            ? html`
              <a href=${refURL(p.id)} target="_blank" rel="noopener"
                 style="font-size:11.5px; color:var(--brand-300);
                        text-decoration:none;
                        display:inline-flex; align-items:center; gap:6px;
                        --wails-draggable: no-drag;">
                Open
                <i class="fa-solid fa-arrow-up-right-from-square" style="font-size:9px;"></i>
              </a>
            `
            : nothing}
          ${isComingSoon
            ? html`<span style="font-size:10.5px; color:var(--fg-3);">surface lands in a follow-up</span>`
            : nothing}
          <div style="flex:1"></div>
          ${p.affiliate
            ? html`
              <span style="font-size:9.5px; color:var(--fg-3);
                           padding:2px 6px;
                           background:rgba(255,255,255,0.03);
                           border:1px solid rgba(255,255,255,0.06);
                           border-radius:4px;"
                    title="Clicking sends you via ref.lthn.ai — the click is attributed to lthn so we earn on signup. Helps fund the OSS.">
                supports lthn
              </span>
            `
            : nothing}
        </div>
      </div>
    `;
  }

  private renderCategorySection(cat: ProviderCategory, providers: ServiceProvider[]) {
    if (!providers || providers.length === 0) return nothing;
    return html`
      <section style="display:flex; flex-direction:column; gap:10px;">
        <div style="display:flex; align-items:baseline; gap:10px;">
          <div style="font-size:13px; font-weight:600; color:var(--fg-0);
                      letter-spacing:-0.005em;">
            ${CATEGORY_LABELS[cat]}
          </div>
          <div style="font-size:11px; color:var(--fg-3);">
            ${CATEGORY_BLURBS[cat]}
          </div>
        </div>
        <div style="display:grid; grid-template-columns:1fr 1fr; gap:10px;">
          ${providers.map(p => this.renderProviderCard(p))}
        </div>
      </section>
    `;
  }

  render() {
    const grouped = byCategory();
    // Order of sections — VPS first (the Provision Remote target),
    // then infra, then first-party services + marketplace.
    const order: ProviderCategory[] = [
      "vps", "cdn", "dns", "ssl", "comms", "analytics", "marketplace",
    ];

    const body = html`
      <div style="flex:1; overflow:auto; padding:20px 24px;
                  display:flex; flex-direction:column; gap:22px;">
        ${order.map(cat => this.renderCategorySection(cat, grouped[cat]))}
      </div>
    `;

    const total = PROVIDERS.length;
    const live = PROVIDERS.filter(p => p.status === "live").length;
    const firstParty = PROVIDERS.filter(p => p.firstParty).length;

    return renderChrome({
      title: this.chrome.title,
      subtitle: this.chrome.subtitle,
      w: this.w, h: this.h, body,
      footer: html`${live} integrated · ${firstParty} Lethean · ${total} total`,
      embedded: this.embedded,
    });
  }
}
customElements.define("lthn-providers-window", LthnProvidersWindow);
