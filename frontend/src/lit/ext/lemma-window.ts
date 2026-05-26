// SPDX-Licence-Identifier: EUPL-1.2
// E5.1 · lemma — <lthn-lemma-window>
//
// Minimal status panel for the local lthn-mlx engine. Mounted at
// ?surface=lemma, shipped in both lthn-desktop and lthn-mlx (the
// menubar). Talks only to the OpenAI-compatible HTTP endpoints
// (/v1/health, /v1/models) that lthn-mlx serve exposes — NO Wails
// service bindings, NO event-bus subscriptions, NO downloader.
// Means lthn-mlx can embed the shared frontend dist and surface this
// window even though it doesn't host lthn-desktop's full service tree.
//
// Light-DOM Lit element. Composes renderChrome() from ../chrome.js.

import { LitElement, html, nothing } from "lit";
import { renderChrome } from "../chrome";

interface HealthPayload {
  status?: string;
  runtime?: string;
  models?: string[];
  time?: number;
}

interface ModelEntry {
  id?: string;
  object?: string;
}

interface ModelsPayload {
  object?: string;
  data?: ModelEntry[];
}

class LthnLemmaWindow extends LitElement {
  static readonly properties = {
    w:        { type: Number },
    h:        { type: Number },
    embedded: { type: Boolean, reflect: true },
    endpoint: { type: String },
    chrome:   { state: true },
    health:   { state: true },
    models:   { state: true },
    err:      { state: true },
  };
  declare w: number;
  declare h: number;
  declare embedded: boolean;
  declare endpoint: string;
  declare chrome: { title: string; subtitle: string };
  declare health: HealthPayload | null;
  declare models: ModelEntry[];
  declare err: string;
  private pollHandle: ReturnType<typeof setInterval> | null = null;

  constructor() {
    super();
    this.w = 720; this.h = 480; this.embedded = false;
    this.endpoint = "http://localhost:11434";
    this.chrome = { title: "Lemma", subtitle: "Local AI engine · OpenAI / Anthropic / Ollama compatible" };
    this.health = null;
    this.models = [];
    this.err = "";
  }

  createRenderRoot() { return this; }

  connectedCallback() {
    super.connectedCallback();
    void this.poll();
    this.pollHandle = setInterval(() => { void this.poll(); }, 2000);
  }

  disconnectedCallback() {
    super.disconnectedCallback();
    if (this.pollHandle !== null) {
      clearInterval(this.pollHandle);
      this.pollHandle = null;
    }
  }

  private async poll(): Promise<void> {
    try {
      const [hRes, mRes] = await Promise.allSettled([
        fetch(this.endpoint + "/v1/health", { cache: "no-store" }),
        fetch(this.endpoint + "/v1/models", { cache: "no-store" }),
      ]);
      let nextErr = "";
      let nextHealth: HealthPayload | null = null;
      let nextModels: ModelEntry[] = [];
      if (hRes.status === "fulfilled" && hRes.value.ok) {
        nextHealth = (await hRes.value.json()) as HealthPayload;
      } else if (hRes.status === "rejected") {
        nextErr = String(hRes.reason);
      }
      if (mRes.status === "fulfilled" && mRes.value.ok) {
        const body = (await mRes.value.json()) as ModelsPayload;
        nextModels = body.data ?? [];
      }
      this.health = nextHealth;
      this.models = nextModels;
      this.err = nextErr;
    } catch (e) {
      this.err = String(e);
      this.health = null;
      this.models = [];
    }
  }

  private renderStatus() {
    if (this.health && this.health.status === "ok") {
      return html`<lthn-state-pill variant="ok">serving</lthn-state-pill>`;
    }
    if (this.err) {
      return html`<lthn-state-pill variant="warn">unreachable</lthn-state-pill>`;
    }
    return html`<lthn-state-pill variant="muted">idle</lthn-state-pill>`;
  }

  private renderBody() {
    const runtime = this.health?.runtime ?? "—";
    return html`
      <div style="display:flex; flex-direction:column; gap:12px; padding:16px;">
        <lthn-rail-row k="Status" v="${nothing}">${this.renderStatus()}</lthn-rail-row>
        <lthn-rail-row k="Runtime" v="${runtime}"></lthn-rail-row>
        <lthn-rail-row k="Endpoint" v="${this.endpoint}"></lthn-rail-row>
        <lthn-label>Loaded models</lthn-label>
        ${this.models.length === 0
          ? html`<div style="opacity:0.6; font-size:13px; padding:8px 0;">No models loaded. Start the engine from the menu bar.</div>`
          : html`<ul style="list-style:none; padding:0; margin:0; display:flex; flex-direction:column; gap:4px;">
              ${this.models.map((m) => html`<li style="font-family:ui-monospace, monospace; font-size:12px; opacity:0.85;">${m.id ?? "(unnamed)"}</li>`)}
            </ul>`}
        ${this.err
          ? html`<div style="opacity:0.6; font-size:11px; color:var(--error, #f82da7); padding-top:8px;">${this.err}</div>`
          : nothing}
      </div>
    `;
  }

  render() {
    return renderChrome({
      title: this.chrome.title,
      subtitle: this.chrome.subtitle,
      w: this.w,
      h: this.h,
      embedded: this.embedded,
      body: this.renderBody(),
    });
  }
}

customElements.define("lthn-lemma-window", LthnLemmaWindow);
