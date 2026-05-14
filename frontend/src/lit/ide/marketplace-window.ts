// SPDX-Licence-Identifier: EUPL-1.2
// IDE · marketplace — <lthn-marketplace-window>
//
// Browses the plugin catalogue + lists installed plugins +
// dispatches install/remove against the plugin host. The host
// (binary + --namespace + reverse-proxy mount) isn't built yet —
// the Go service returns a "plugin host not running" error from
// Install/Remove, which this window surfaces as an info banner.
//
// Tabs: Browse (catalogue + category filter + search) / Installed.
// Open via windowservice.Open("marketplace").

import { LitElement, html, nothing } from "lit";
import { renderChrome } from "../chrome";
import { T } from "@lthn/i18n/coreservice";

interface Package {
  code: string;
  name: string;
  version?: string;
  repo?: string;
  category?: string;
  entrypoint?: string;
  description?: string;
}

interface InstalledPackage {
  code: string;
  name: string;
  version: string;
  entry_point?: string;
}

type Tab = "browse" | "installed";

class LthnMarketplaceWindow extends LitElement {
  static readonly properties = {
    w:         { type: Number },
    h:         { type: Number },
    embedded:  { type: Boolean, reflect: true },
    chrome:    { state: true },
    tab:       { state: true },
    catalogue: { state: true },
    installed: { state: true },
    query:     { state: true },
    category:  { state: true },
    loading:   { state: true },
    err:       { state: true },
    info:      { state: true },
  };
  declare w: number;
  declare h: number;
  declare embedded: boolean;
  declare chrome: { title: string; subtitle: string };
  declare tab: Tab;
  declare catalogue: Package[];
  declare installed: InstalledPackage[];
  declare query: string;
  declare category: string;
  declare loading: boolean;
  declare err: string;
  declare info: string;

  constructor() {
    super();
    this.w = 1180; this.h = 760; this.embedded = false;
    this.chrome = { title: "Marketplace", subtitle: "browse · install · plugin" };
    this.tab = "browse";
    this.catalogue = [];
    this.installed = [];
    this.query = "";
    this.category = "";
    this.loading = false;
    this.err = "";
    this.info = "";
  }
  createRenderRoot() { return this; }

  async connectedCallback() {
    super.connectedCallback();
    const [title, subtitle] = await Promise.all([
      T("window.marketplace.title"),
      T("window.marketplace.subtitle"),
    ]);
    this.chrome = { title, subtitle };
    await this._refresh();
  }

  async _refresh() {
    if (this.loading) return;
    this.loading = true;
    this.err = "";
    try {
      const svc = await import("@desktop/marketplace/service");
      const [s, i] = await Promise.all([
        svc.Search(this.query, this.category) as Promise<{ packages: Package[] }>,
        svc.Installed() as Promise<{ packages: InstalledPackage[] }>,
      ]);
      this.catalogue = s.packages || [];
      this.installed = i.packages || [];
    } catch (e: unknown) {
      this.err = e instanceof Error ? e.message : String(e);
    } finally {
      this.loading = false;
    }
  }

  async _install(code: string) {
    this.info = "";
    this.err = "";
    try {
      const svc = await import("@desktop/marketplace/service");
      await svc.Install(code);
      this.info = `Installed ${code}`;
      await this._refresh();
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : String(e);
      if (msg.includes("plugin host not running")) {
        this.info = msg;
      } else {
        this.err = msg;
      }
    }
  }

  async _remove(code: string) {
    this.info = "";
    this.err = "";
    try {
      const svc = await import("@desktop/marketplace/service");
      await svc.Remove(code);
      this.info = `Removed ${code}`;
      await this._refresh();
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : String(e);
      if (msg.includes("plugin host not running")) {
        this.info = msg;
      } else {
        this.err = msg;
      }
    }
  }

  _categories(): string[] {
    const cats = new Set<string>();
    for (const p of this.catalogue) if (p.category) cats.add(p.category);
    return Array.from(cats).sort((a, b) => a.localeCompare(b));
  }

  _isInstalled(code: string): boolean {
    return this.installed.some(i => i.code === code);
  }

  render() {
    const cats = this._categories();
    const tabStyle = (active: boolean) => `
      padding:4px 12px; border-radius:4px; border:none; cursor:pointer;
      font-family:var(--font-mono); font-size:11px;
      background:${active ? "rgba(255,255,255,0.08)" : "transparent"};
      color:${active ? "var(--fg-0)" : "var(--fg-3)"};
      --wails-draggable: no-drag;
    `;
    const chipStyle = (active: boolean) => `
      padding:3px 9px; border-radius:4px; border:none; cursor:pointer;
      font-family:var(--font-mono); font-size:10.5px;
      background:${active ? "rgba(255,255,255,0.08)" : "transparent"};
      color:${active ? "var(--fg-0)" : "var(--fg-3)"};
      --wails-draggable: no-drag;
    `;

    const toolbar = html`
      <div style="display:inline-flex; border-radius:6px; padding:2px;
                  background:rgba(0,0,0,0.18);
                  border:1px solid rgba(255,255,255,0.06); gap:2px;">
        <button @click=${() => this.tab = "browse"} style=${tabStyle(this.tab === "browse")}>browse · ${this.catalogue.length}</button>
        <button @click=${() => this.tab = "installed"} style=${tabStyle(this.tab === "installed")}>installed · ${this.installed.length}</button>
      </div>
      <div style="flex:1"></div>
      <lthn-btn tone="primary" size="sm" ?dim=${this.loading}
        @click=${() => void this._refresh()}>
        <i class="fa-solid ${this.loading ? "fa-spinner" : "fa-rotate"}"
           style="font-size:10px;"></i>
        ${this.loading ? "Loading…" : "Refresh"}
      </lthn-btn>
    `;

    const banner = (this.info || this.err) ? html`
      <div style="padding:10px 14px;
                  color:${this.err ? "var(--err-400)" : "var(--brand-300)"};
                  background:${this.err
                    ? "rgba(255,76,76,0.06)"
                    : "rgba(122, 95, 196, 0.06)"};
                  border:1px solid ${this.err
                    ? "rgba(255,76,76,0.18)"
                    : "rgba(122, 95, 196, 0.18)"};
                  border-radius:6px; font-size:12px;
                  line-height:1.55; margin-bottom:14px;">
        ${this.err || this.info}
      </div>
    ` : nothing;

    const browse = html`
      <div style="display:flex; gap:8px; margin-bottom:14px;
                  align-items:center;">
        <input type="text" .value=${this.query}
          @input=${(e: Event) => { this.query = (e.target as HTMLInputElement).value; void this._refresh(); }}
          placeholder="search plugins…"
          style="flex:1; padding:6px 10px; font-family:var(--font-mono);
                 font-size:11px; background:rgba(0,0,0,0.30);
                 border:1px solid rgba(255,255,255,0.06);
                 border-radius:6px; color:var(--fg-0);
                 --wails-draggable: no-drag;">
        <div style="display:inline-flex; border-radius:6px; padding:2px;
                    background:rgba(0,0,0,0.18);
                    border:1px solid rgba(255,255,255,0.06); gap:2px;">
          <button @click=${() => { this.category = ""; void this._refresh(); }}
            style=${chipStyle(this.category === "")}>all</button>
          ${cats.map(c => html`
            <button @click=${() => { this.category = c; void this._refresh(); }}
              style=${chipStyle(this.category === c)}>${c}</button>
          `)}
        </div>
      </div>

      <div style="display:grid;
                  grid-template-columns:repeat(auto-fill, minmax(320px, 1fr));
                  gap:10px;">
        ${this.catalogue.map(p => html`
          <div style="background:rgba(0,0,0,0.18); border-radius:8px;
                      padding:12px; display:flex; flex-direction:column;
                      gap:8px; border:1px solid rgba(255,255,255,0.05);">
            <div style="display:flex; justify-content:space-between;
                        align-items:baseline; gap:8px;">
              <div style="display:flex; flex-direction:column; gap:2px;
                          min-width:0; flex:1;">
                <span style="font-weight:600; font-size:13px;
                             color:var(--fg-0);">${p.name}</span>
                <code style="font-family:var(--font-mono); font-size:10px;
                             color:var(--fg-3);">${p.repo || p.code}</code>
              </div>
              ${p.version ? html`
                <span style="font-size:10px; padding:2px 7px; border-radius:4px;
                             background:rgba(0,0,0,0.30);
                             color:var(--fg-2);
                             font-family:var(--font-mono);">v${p.version}</span>
              ` : nothing}
            </div>
            ${p.description ? html`
              <p style="font-size:11px; color:var(--fg-2); margin:0;
                        line-height:1.5;">${p.description}</p>
            ` : nothing}
            <div style="display:flex; gap:6px; align-items:center;
                        margin-top:auto;">
              ${p.category ? html`
                <span style="font-size:10px; padding:2px 8px;
                             border-radius:999px;
                             background:rgba(122, 95, 196, 0.14);
                             color:var(--brand-300);
                             font-family:var(--font-mono);">${p.category}</span>
              ` : nothing}
              <div style="flex:1"></div>
              ${this._isInstalled(p.code) ? html`
                <lthn-btn tone="ghost" size="sm"
                  @click=${() => void this._remove(p.code)}>
                  <i class="fa-solid fa-trash" style="font-size:10px;"></i>
                  Remove
                </lthn-btn>
              ` : html`
                <lthn-btn tone="primary" size="sm"
                  @click=${() => void this._install(p.code)}>
                  <i class="fa-solid fa-arrow-down" style="font-size:10px;"></i>
                  Install
                </lthn-btn>
              `}
            </div>
          </div>
        `)}
        ${this.catalogue.length === 0 && !this.loading ? html`
          <div style="padding:40px; text-align:center; color:var(--fg-3);
                      font-size:12px; grid-column:1 / -1;">
            no plugins match the current filter
          </div>
        ` : nothing}
      </div>
    `;

    const installedPane = html`
      ${this.installed.length === 0 ? html`
        <div style="padding:40px; text-align:center; color:var(--fg-3);
                    font-size:12px;">
          no plugins installed yet ·
          ${"browse the catalogue and click Install to add one"}
        </div>
      ` : html`
        <div style="display:flex; flex-direction:column; gap:6px;">
          ${this.installed.map(i => html`
            <div style="background:rgba(0,0,0,0.18); border-radius:8px;
                        padding:12px 14px; display:flex; gap:14px;
                        align-items:center;
                        border:1px solid rgba(255,255,255,0.05);">
              <div style="flex:1; min-width:0;">
                <div style="font-weight:600; font-size:13px;
                            color:var(--fg-0);">${i.name}</div>
                <code style="font-family:var(--font-mono); font-size:10px;
                             color:var(--fg-3);">${i.code} · v${i.version}</code>
              </div>
              ${i.entry_point ? html`
                <code style="font-family:var(--font-mono); font-size:10px;
                             color:var(--fg-2);">${i.entry_point}</code>
              ` : nothing}
              <lthn-btn tone="ghost" size="sm"
                @click=${() => void this._remove(i.code)}>
                <i class="fa-solid fa-trash" style="font-size:10px;"></i>
                Remove
              </lthn-btn>
            </div>
          `)}
        </div>
      `}
    `;

    const body = html`
      <div style="flex:1; min-height:0; overflow:auto; padding:16px 18px;">
        ${banner}
        ${this.tab === "browse" ? browse : installedPane}
      </div>
    `;

    return renderChrome({
      title: this.chrome.title, subtitle: this.chrome.subtitle,
      w: this.w, h: this.h, toolbar, body,
      footer: html`${this.catalogue.length} catalogued ·
        ${this.installed.length} installed`,
      embedded: this.embedded,
    });
  }
}
customElements.define("lthn-marketplace-window", LthnMarketplaceWindow);
