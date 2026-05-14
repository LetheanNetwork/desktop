// SPDX-Licence-Identifier: EUPL-1.2
// IDE · plugin host — <lthn-plugin-window>
//
// Generic plugin window. Reads ?code=<plugin-code> from the
// URL, queries the plugin host for the plugin's status +
// manifest-declared menu entry + UI entrypoint, then either:
//
//   1. Mounts an iframe at /v1/api/plugin/<namespace>/<entrypoint>
//      when the plugin declared a ui.entrypoint
//   2. Shows a status card (running state, port, PID, last
//      error) when no UI is declared
//   3. Shows an "install required" card when the plugin code
//      isn't installed yet
//
// Open via openPluginWindow(<code>) in pkg/desktop's tray. Each
// code keys its own Wails window so multiple plugins can be
// open at the same time.

import { LitElement, html, nothing } from "lit";
import { renderChrome } from "../chrome";

interface MenuEntry {
  code: string;
  label: string;
  icon?: string;
  surface: string;
  namespace: string;
  entry_url?: string;
  running: boolean;
}

interface Status {
  code: string;
  name: string;
  version: string;
  namespace: string;
  state: string;
  port?: number;
  pid?: number;
  started_at?: string;
  stopped_at?: string;
  last_error?: string;
}

class LthnPluginWindow extends LitElement {
  static readonly properties = {
    w:        { type: Number },
    h:        { type: Number },
    embedded: { type: Boolean, reflect: true },
    code:     { type: String },
    chrome:   { state: true },
    entry:    { state: true },
    status:   { state: true },
    loading:  { state: true },
    err:      { state: true },
  };
  declare w: number;
  declare h: number;
  declare embedded: boolean;
  declare code: string;
  declare chrome: { title: string; subtitle: string };
  declare entry: MenuEntry | null;
  declare status: Status | null;
  declare loading: boolean;
  declare err: string;

  constructor() {
    super();
    this.w = 1180; this.h = 760; this.embedded = false;
    this.code = "";
    this.chrome = { title: "Plugin", subtitle: "loading…" };
    this.entry = null;
    this.status = null;
    this.loading = false;
    this.err = "";
  }
  createRenderRoot() { return this; }

  async connectedCallback() {
    super.connectedCallback();
    if (this.code) await this._refresh();
  }

  async _refresh() {
    if (!this.code || this.loading) return;
    this.loading = true;
    this.err = "";
    try {
      const svc = await import("@desktop/plugin/service");
      const [menus, status] = await Promise.all([
        svc.Menus() as Promise<MenuEntry[]>,
        svc.Status(this.code) as Promise<Status>,
      ]);
      this.entry = (menus || []).find(m => m.code === this.code) || null;
      this.status = status;
      this.chrome = {
        title:    status.name || this.code,
        subtitle: status.namespace + " · " + status.state,
      };
    } catch (e: unknown) {
      this.err = e instanceof Error ? e.message : String(e);
    } finally {
      this.loading = false;
    }
  }

  async _start() {
    try {
      const svc = await import("@desktop/plugin/service");
      await svc.Start(this.code);
      await this._refresh();
    } catch (e: unknown) {
      this.err = e instanceof Error ? e.message : String(e);
    }
  }

  async _stop() {
    try {
      const svc = await import("@desktop/plugin/service");
      await svc.Stop(this.code);
      await this._refresh();
    } catch (e: unknown) {
      this.err = e instanceof Error ? e.message : String(e);
    }
  }

  private isRunning() {
    return this.status?.state === "running";
  }

  private hasUI() {
    return Boolean(this.entry?.entry_url);
  }

  private statusLine() {
    if (!this.status) return "loading…";
    const parts = [this.status.state];
    if (this.status.port) parts.push(":" + this.status.port);
    if (this.status.pid) parts.push("pid " + this.status.pid);
    return parts.join(" · ");
  }

  private renderRunButton(running: boolean) {
    if (running) {
      return html`
        <lthn-btn tone="ghost" size="sm" @click=${() => void this._stop()}>
          <i class="fa-solid fa-stop" style="font-size:10px;"></i> Stop
        </lthn-btn>
      `;
    }
    return html`
      <lthn-btn tone="primary" size="sm" @click=${() => void this._start()}>
        <i class="fa-solid fa-play" style="font-size:10px;"></i> Start
      </lthn-btn>
    `;
  }

  private renderToolbar(running: boolean) {
    return html`
      <span style="font-family:var(--font-mono); font-size:11px; color:var(--fg-3);
                   padding:0 6px;">
        ${this.statusLine()}
      </span>
      <div style="flex:1"></div>
      ${this.renderRunButton(running)}
      <lthn-btn tone="ghost" size="sm" @click=${() => void this._refresh()}>
        <i class="fa-solid fa-rotate" style="font-size:10px;"></i>
      </lthn-btn>
    `;
  }

  private renderBanner() {
    if (!this.err) return nothing;
    return html`
      <div style="padding:10px 14px; color:var(--err-400);
                  background:rgba(255,76,76,0.06);
                  border:1px solid rgba(255,76,76,0.18);
                  border-radius:6px; font-size:12px;
                  line-height:1.55; margin:14px 18px 0;">
        ${this.err}
      </div>
    `;
  }

  private renderEmptyState() {
    return html`
      <div style="padding:40px; text-align:center; color:var(--fg-3);
                  font-size:12px;">
        ${this.loading ? "Loading…" : "no plugin selected"}
      </div>
    `;
  }

  private renderPluginFrame(entryUrl: string) {
    return html`
      <iframe src=${entryUrl}
        style="flex:1; width:100%; border:none;
               background:rgba(0,0,0,0.18);"
        sandbox="allow-scripts allow-forms allow-same-origin allow-popups"
        referrerpolicy="same-origin"></iframe>
    `;
  }

  private renderCell(label: string, value: unknown) {
    return html`
      <div style="display:flex; flex-direction:column; gap:2px;
                  padding:8px 10px; background:rgba(0,0,0,0.18);
                  border:1px solid rgba(255,255,255,0.05);
                  border-radius:4px;">
        <span style="font-size:10px; text-transform:uppercase;
                     letter-spacing:0.06em; color:var(--fg-3);">${label}</span>
        <span style="font-size:12px; color:var(--fg-0);
                     font-family:var(--font-mono);">${value || "—"}</span>
      </div>
    `;
  }

  private formattedStartedAt(status: Status) {
    if (!status.started_at || status.started_at === "0001-01-01T00:00:00Z") {
      return "—";
    }
    return status.started_at.replace("T", " ").slice(0, 19);
  }

  private renderLastError(status: Status) {
    if (!status.last_error) return nothing;
    return html`
      <h4 style="font-size:11px; text-transform:uppercase;
                 letter-spacing:0.06em; color:var(--fg-3);
                 margin:18px 0 8px;">Last error</h4>
      <pre style="background:rgba(255,76,76,0.06);
                  border:1px solid rgba(255,76,76,0.18);
                  border-radius:6px; padding:10px 14px;
                  font-family:var(--font-mono); font-size:11px;
                  color:var(--err-400); white-space:pre-wrap;
                  line-height:1.55; margin:0;">${status.last_error}</pre>
    `;
  }

  private renderStoppedNotice(status: Status) {
    return html`
      <div style="padding:18px; text-align:center; color:var(--fg-3);
                  font-style:italic; font-size:12px; margin-top:18px;
                  background:rgba(0,0,0,0.18); border-radius:6px;
                  border:1px solid rgba(255,255,255,0.05);">
        plugin is ${status.state} — click Start to launch it
      </div>
    `;
  }

  private renderNoUiNotice(status: Status) {
    return html`
      <div style="padding:18px; text-align:center; color:var(--fg-3);
                  font-style:italic; font-size:12px; margin-top:18px;
                  background:rgba(0,0,0,0.18); border-radius:6px;
                  border:1px solid rgba(255,255,255,0.05);">
        this plugin has no UI surface · use HTTP at
        <code>/v1/api/plugin/${status.namespace}/*</code>
      </div>
    `;
  }

  private renderRuntimeNotice(status: Status, running: boolean, hasUI: boolean) {
    if (!running) return this.renderStoppedNotice(status);
    if (!hasUI) return this.renderNoUiNotice(status);
    return nothing;
  }

  private renderStatusCard(status: Status, running: boolean, hasUI: boolean) {
    return html`
      <div style="flex:1; padding:18px; overflow-y:auto;">
        <h3 style="font-size:16px; color:var(--fg-0); margin:0 0 4px;">
          ${status.name || this.code}</h3>
        <code style="font-family:var(--font-mono); font-size:11px;
                     color:var(--fg-3); display:block; margin-bottom:14px;">
          ${status.namespace} · v${status.version}</code>

        <div style="display:grid; grid-template-columns:repeat(auto-fill, minmax(160px, 1fr));
                    gap:8px; margin-bottom:14px;">
          ${this.renderCell("state", status.state)}
          ${this.renderCell("port", status.port)}
          ${this.renderCell("pid", status.pid)}
          ${this.renderCell("started", this.formattedStartedAt(status))}
        </div>

        ${this.renderLastError(status)}
        ${this.renderRuntimeNotice(status, running, hasUI)}
      </div>
    `;
  }

  private renderInner(running: boolean, hasUI: boolean) {
    if (!this.status) return this.renderEmptyState();
    if (running && hasUI && this.entry?.entry_url) {
      return this.renderPluginFrame(this.entry.entry_url);
    }
    return this.renderStatusCard(this.status, running, hasUI);
  }

  private renderFooter() {
    if (!this.status) return this.code;
    return "/v1/api/plugin/" + this.status.namespace;
  }

  render() {
    const running = this.isRunning();
    const hasUI = this.hasUI();
    return renderChrome({
      title: this.chrome.title, subtitle: this.chrome.subtitle,
      w: this.w, h: this.h, toolbar: this.renderToolbar(running),
      body: html`${this.renderBanner()}${this.renderInner(running, hasUI)}`,
      footer: html`${this.renderFooter()}`,
      embedded: this.embedded,
    });
  }
}
customElements.define("lthn-plugin-window", LthnPluginWindow);
