// SPDX-Licence-Identifier: EUPL-1.2
// E2.5 · processes — <lthn-processes-window>
// Light-DOM Lit element. Composes renderChrome() from ../chrome.js.
// Wraps <core-process-panel> — upstream component, do not modify.

import { LitElement, html } from "lit";
import { renderChrome } from "../chrome";
import { T } from "@lthn/i18n/coreservice";

// Side-effect import to register the upstream panel + its children
import "./process/process-panel.js";

class LthnProcessesWindow extends LitElement {
  static readonly properties = {
    w:        { type: Number },
    h:        { type: Number },
    embedded: { type: Boolean, reflect: true },
    chrome:   { state: true },
  };
  declare w: number;
  declare h: number;
  declare embedded: boolean;
  declare chrome: { title: string; subtitle: string };

  constructor() {
    super();
    this.w = 1100; this.h = 720; this.embedded = false;
    this.chrome = { title: "Processes", subtitle: "daemons · processes · pipelines" };
  }

  createRenderRoot() { return this; }

  async connectedCallback() {
    super.connectedCallback();
    const [title, subtitle] = await Promise.all([
      T("window.processes.title"),
      T("window.processes.subtitle"),
    ]);
    this.chrome = { title, subtitle };
  }

  private get _apiUrl(): string {
    return `${location.protocol}//${location.host}/api/process`;
  }

  private get _wsUrl(): string {
    const proto = location.protocol === "https:" ? "wss:" : "ws:";
    return `${proto}//${location.host}/api/process/ws`;
  }

  render() {
    const body = html`
      <core-process-panel
        api-url=${this._apiUrl}
        ws-url=${this._wsUrl}
        style="height:100%;"
      ></core-process-panel>
    `;
    return renderChrome({
      title: this.chrome.title,
      subtitle: this.chrome.subtitle,
      w: this.w, h: this.h,
      body,
      embedded: this.embedded,
    });
  }
}
customElements.define("lthn-processes-window", LthnProcessesWindow);
