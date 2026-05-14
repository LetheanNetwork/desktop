// SPDX-Licence-Identifier: EUPL-1.2
// E3.1 · integrations — <lthn-integrations-window>
// Light-DOM Lit element. Composes renderChrome() from ../chrome.js.

import { LitElement, html, nothing } from "lit";
import { renderChrome } from "../chrome";
import { T } from "@lthn/i18n/coreservice";

interface ClientView {
  id: string;
  name: string;
  description: string;
  config_path: string;
  config_path_raw: string;
  exists: boolean;
  state: string;
}

class LthnIntegrationsWindow extends LitElement {
  static readonly properties = {
    w: { type: Number },
    h: { type: Number },
    embedded: { type: Boolean, reflect: true },
    chrome: { state: true },
    clients: { state: true },
    selectedId: { state: true },
    endpoint: { state: true },
    defaultModel: { state: true },
    apiKey: { state: true },
    t: { state: true },
  };
  declare w: number;
  declare h: number;
  declare embedded: boolean;
  declare chrome: { title: string; subtitle: string };
  declare clients: ClientView[];
  declare selectedId: string;
  declare endpoint: string;
  declare defaultModel: string;
  declare apiKey: string;
  declare t: {
    railLabel: string; railEmpty: string;
    rowConfigPath: string; rowOnDisk: string; rowEndpoint: string; rowDefaultModel: string;
    snippetLabel: string; snippetHelp: string;
    yes: string; no: string;
    noClient: string;
  };
  constructor() {
    super();
    this.w = 880; this.h = 660; this.embedded = false;
    this.chrome = { title: "Integrations", subtitle: "clients · MCP · webhooks" };
    this.clients = [];
    this.selectedId = "";
    this.endpoint = "http://localhost:8000/v1";
    this.defaultModel = "—";
    this.apiKey = "sk-lthn-•••• (managed by lthn)";
    this.t = {
      railLabel: "Clients",
      railEmpty: "No clients enumerated yet. The integrations service is the source of truth.",
      rowConfigPath: "Config path", rowOnDisk: "On disk",
      rowEndpoint: "Endpoint", rowDefaultModel: "Default model",
      snippetLabel: "Config snippet · drop this into %s",
      snippetHelp:  "Only the apiBase, apiKey and model keys are lthn-managed. Anything else you set in this file is left alone.",
      yes: "yes", no: "no",
      noClient: "No client selected.",
    };
  }
  createRenderRoot() { return this; }
  async connectedCallback() {
    super.connectedCallback();
    const [title, subtitle, rl, re, rcp, rod, rep, rdm, sl, sh, yes, no, nc] = await Promise.all([
      T("window.integrations.title"),
      T("window.integrations.subtitle"),
      T("window.integrations.rail_label"),
      T("window.integrations.rail_empty"),
      T("window.integrations.row_config_path"),
      T("window.integrations.row_on_disk"),
      T("window.integrations.row_endpoint"),
      T("window.integrations.row_default_model"),
      T("window.integrations.snippet_label"),
      T("window.integrations.snippet_help"),
      T("window.integrations.yes"),
      T("window.integrations.no"),
      T("window.integrations.no_client"),
    ]);
    this.chrome = { title, subtitle };
    this.t = {
      railLabel: rl, railEmpty: re,
      rowConfigPath: rcp, rowOnDisk: rod, rowEndpoint: rep, rowDefaultModel: rdm,
      snippetLabel: sl, snippetHelp: sh,
      yes, no, noClient: nc,
    };
    try {
      const [integrations, runner, server, ak] = await Promise.all([
        import("@desktop/integrations/wailsservice"),
        import("@desktop/runner/service"),
        import("@desktop/server/service"),
        import("@desktop/apikey/wailsservice"),
      ]);
      const [list, models, addr, masked] = await Promise.all([
        integrations.List(),
        runner.WModels().catch((): string[] => []),
        server.WAddr().catch((): string => ""),
        ak.Masked().catch((): string => ""),
      ]);
      this.clients = (list || []) as ClientView[];
      if (this.clients.length > 0) this.selectedId = this.clients[0].id;
      if (models && models.length > 0) this.defaultModel = models[0];
      if (addr) this.endpoint = endpointFromAddr(addr);
      if (masked) this.apiKey = masked;
    } catch (err) {
      console.error("integrations: lookup failed", err);
    }
  }

  _statusVariant(state: string) {
    if (state === "configured") return "ok";
    if (state === "available") return "warn";
    return "idle";
  }

  render() {
    const clients = this.clients;
    const selected = clients.find(c => c.id === this.selectedId) || clients[0] || null;

    const body = html`
      <div style="flex:1; display:grid; grid-template-columns:260px 1fr; min-height:0;">
        <aside style="background:rgba(0,0,0,0.18); border-right:1px solid rgba(255,255,255,0.05); padding:12px 8px; display:flex; flex-direction:column; gap:1px;">
          <lthn-label style="padding:4px 10px 8px;">${this.t.railLabel}</lthn-label>
          ${clients.length === 0 ? html`
            <div style="padding:14px 12px; font-size:11.5px; color:var(--fg-3); line-height:1.55;">
              ${this.t.railEmpty}
            </div>
          ` : clients.map((c) => {
            const active = c.id === this.selectedId;
            const variant = this._statusVariant(c.state);
            return html`
              <div
                @click=${() => { this.selectedId = c.id; }}
                style="padding:10px 12px; border-radius:6px; background:${active ? "rgba(255,255,255,0.06)" : "transparent"}; border-left:${active ? "2px solid var(--brand-400)" : "2px solid transparent"}; display:flex; flex-direction:column; gap:4px; cursor:pointer; --wails-draggable: no-drag;">
                <div style="display:flex; align-items:center; gap:8px;">
                  <lthn-status-dot variant=${variant}></lthn-status-dot>
                  <span style="font-size:12.5px; color:var(--fg-0); font-weight:500;">${c.name}</span>
                </div>
                <div style="font-size:10.5px; color:var(--fg-3); margin-left:15px;">${c.state}</div>
              </div>
            `;
          })}
        </aside>
        <main style="padding:24px 32px; overflow:auto; display:flex; flex-direction:column; gap:22px;">
          ${selected ? html`
            <div>
              <div style="display:flex; align-items:center; gap:12px;">
                <div style="font-size:20px; font-weight:600; color:var(--fg-0); letter-spacing:-0.015em;">${selected.name}</div>
                <lthn-state-pill variant=${selected.state === "configured" ? "connected" : "disconnected"}>${selected.state}</lthn-state-pill>
              </div>
              <div style="font-size:12.5px; color:var(--fg-2); margin-top:6px; max-width:460px; line-height:1.55;">${selected.description}</div>
            </div>
            <div style="display:grid; grid-template-columns:1fr 1fr; gap:10px;">
              ${[
                { k: this.t.rowConfigPath,   v: selected.config_path_raw },
                { k: this.t.rowOnDisk,       v: selected.exists ? this.t.yes : this.t.no },
                { k: this.t.rowEndpoint,     v: this.endpoint },
                { k: this.t.rowDefaultModel, v: this.defaultModel },
              ].map(row => html`
                <div style="padding:10px 14px; border-radius:6px; background:rgba(255,255,255,0.025); border:1px solid rgba(255,255,255,0.05);">
                  <div style="font-size:10.5px; color:var(--fg-3); letter-spacing:0.04em; text-transform:uppercase;">${row.k}</div>
                  <div style="font-family:var(--font-mono); font-size:12.5px; color:var(--fg-0); margin-top:4px;">${row.v}</div>
                </div>
              `)}
            </div>
            <div>
              <lthn-label>${this.t.snippetLabel.replace("%s", selected.config_path_raw)}</lthn-label>
              <div style="margin-top:8px; background:rgba(0,0,0,0.30); border:1px solid rgba(255,255,255,0.06); border-radius:8px; padding:12px 14px; font-family:var(--font-mono); font-size:11.5px; line-height:1.6; color:var(--fg-1); white-space:pre;">${`{
  "apiBase":  "${this.endpoint}",
  "apiKey":   "${this.apiKey}",
  "model":    "${this.defaultModel}",
  "stream":   true
}`}</div>
              <div style="margin-top:10px; font-size:11px; color:var(--fg-3); line-height:1.55;">
                ${this.t.snippetHelp}
              </div>
            </div>
          ` : html`
            <div style="font-size:13px; color:var(--fg-3); padding:24px 0;">${this.t.noClient}</div>
          `}
        </main>
      </div>
    `;
    const configured = clients.filter(c => c.state === "configured").length;
    return renderChrome({
      title: this.chrome.title, subtitle: this.chrome.subtitle,
      w: this.w, h: this.h, body,
      footer: html`${configured} configured · ${clients.length} known · endpoint ${this.endpoint} · only outbound action lthn ever takes`,
      embedded: this.embedded,
    });
  }
}
customElements.define("lthn-integrations-window", LthnIntegrationsWindow);

/** Compose the OpenAI-compatible endpoint URL from the server's raw
 *  bind address. ":8000" → "http://localhost:8000/v1"; an explicit
 *  host already in the addr ("127.0.0.1:8000") passes through. */
function endpointFromAddr(addr: string): string {
  if (!addr) return "http://localhost:8000/v1";
  const a = addr.startsWith(":") ? `localhost${addr}` : addr;
  return `http://${a}/v1`;
}
