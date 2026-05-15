// SPDX-Licence-Identifier: EUPL-1.2
// E2.5 · processes — <lthn-process-window>
// Light-DOM Lit element. Composes renderChrome() from ../chrome.js.
//
// Renders <lthn-process-panel> against lthn's REST surface at /api/process.

import { LitElement, html } from "lit";
import { renderChrome } from "../chrome";
import "./process/process-panel";

class LthnProcessWindow extends LitElement {
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
    this.w = 1000;
    this.h = 660;
    this.embedded = false;
    this.chrome = { title: "Processes", subtitle: "registry · daemons · runners" };
  }
  createRenderRoot() { return this; }

  render() {
    return renderChrome({
      title: this.chrome.title,
      subtitle: this.chrome.subtitle,
      w: this.w,
      h: this.h,
      body: html`<process-panel api-url="/v1/api/process" style="flex:1;display:flex;flex-direction:column;min-height:0;width:100%;"></process-panel>`,
      embedded: this.embedded,
    });
  }
}
customElements.define("lthn-process-window", LthnProcessWindow);
