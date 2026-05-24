// SPDX-Licence-Identifier: EUPL-1.2

// <lthn-marketing-panel> — shared chrome shell for the lthn.ai homepage
// loop panels (train / run / score / serve), per
// plans/project/lthn/ai/RFC.homepage-spotlight.md §2 / §9.
//
// Provides: outer wrapper with 1px border, accent rail (4px left
// column), header row with title + subtitle, slot for the loop-panel
// body. Each of the four loop-panel components consumes this shell —
// `<lthn-marketing-train>` etc. render their content inside the default
// slot of an instance of this element.
//
// Sibling of the typographic hero (hero.ts). Same canon: source of
// truth for the panel shape, Blade conversion lifts the DOM into the
// Website\Lthn module. UK English, no exclamation marks, no banned
// vocab — though copy lives in the consumer slot, not in this shell.

import { LitElement, html, nothing } from "lit";

class LthnMarketingPanel extends LitElement {
  static readonly properties = {
    title:    { type: String },
    subtitle: { type: String },
    accent:   { type: Boolean, reflect: true },
    embedded: { type: Boolean, reflect: true },
  };
  declare title:    string;
  declare subtitle: string;
  declare accent:   boolean;
  declare embedded: boolean;

  constructor() {
    super();
    this.title    = "";
    this.subtitle = "";
    this.accent   = true;
    this.embedded = false;
  }

  createRenderRoot() { return this; }

  // Header row composes the accent rail (4px column) + title + subtitle.
  // Missing title still renders the shell — empty header row collapses
  // to just the rail, body slot remains usable. Lets the four loop-panel
  // bodies prototype individually without title attribution.
  private _renderHeader() {
    if (!this.title && !this.subtitle) {
      // Body-only shell — still render the rail when accent is on so the
      // chrome character is visible.
      return this.accent ? html`<div class="accent-rail" aria-hidden="true"></div>` : nothing;
    }
    return html`
      <header class="panel-header">
        ${this.accent ? html`<div class="accent-rail" aria-hidden="true"></div>` : nothing}
        <div class="panel-titles">
          ${this.title    ? html`<h2 class="panel-title">${this.title}</h2>` : nothing}
          ${this.subtitle ? html`<p  class="panel-subtitle">${this.subtitle}</p>` : nothing}
        </div>
      </header>
    `;
  }

  render() {
    const body = html`
      ${this._renderHeader()}
      <div class="panel-body">
        <slot></slot>
      </div>
    `;
    if (this.embedded) return body;
    return html`<section class="marketing-panel">${body}</section>`;
  }
}

if (!customElements.get("lthn-marketing-panel")) {
  customElements.define("lthn-marketing-panel", LthnMarketingPanel);
}

export { LthnMarketingPanel };
