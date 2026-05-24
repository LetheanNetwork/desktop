// SPDX-Licence-Identifier: EUPL-1.2

// <lthn-marketing-hero> — quiet typographic hero for the lthn.ai
// homepage spotlight (route /), per
// plans/project/lthn/ai/RFC.homepage-spotlight.md §3.
//
// Two-line headline (thesis + loop-in-four-words) plus a one-sentence
// subline. No CTA button, no chart, no hero image — the four loop
// panels below carry the action affordances.
//
// Defaults match the RFC copy. Callers can override via attributes for
// staging variants or A/B copy. UK English, no exclamation marks, no
// banned vocab (leverage / utilise / seamless / robust / game-changing
// / best-in-class) — enforced by tests.
//
// Source of truth for the hero shape. The lthn.ai Blade partial inside
// the Website\Lthn module lifts the rendered DOM; this Lit element is
// the canonical design reference plus the test surface.

import { LitElement, html, nothing } from "lit";

// Default copy lifted verbatim from RFC.homepage-spotlight.md §3.
// Kept here so the element renders correctly out of the box without
// attribute attachment — supports zero-config dev preview and ensures
// the constraints tests have a stable target.
const DEFAULT_LINE1   = "Build with the full stack.";
const DEFAULT_LINE2   = "Lemma + state + score + serve.";
const DEFAULT_SUBLINE =
  "Open-source AI infrastructure. Train your model, run it locally, " +
  "score every output, serve it where you need.";

class LthnMarketingHero extends LitElement {
  static readonly properties = {
    line1:    { type: String },
    line2:    { type: String },
    subline:  { type: String },
    accent:   { type: Boolean, reflect: true },
    embedded: { type: Boolean, reflect: true },
  };
  declare line1:    string;
  declare line2:    string;
  declare subline:  string;
  declare accent:   boolean;
  declare embedded: boolean;

  constructor() {
    super();
    this.line1    = "";
    this.line2    = "";
    this.subline  = "";
    this.accent   = true;   // RFC §3 — accent goes on line2 by default
    this.embedded = false;
  }

  createRenderRoot() { return this; }

  private _l1(): string { return this.line1   || DEFAULT_LINE1;   }
  private _l2(): string { return this.line2   || DEFAULT_LINE2;   }
  private _sb(): string { return this.subline || DEFAULT_SUBLINE; }

  render() {
    const body = html`
      <div class="hero-stack">
        <h1 class="hero-line-1">${this._l1()}</h1>
        <h2 class="hero-line-2 ${this.accent ? "accent" : ""}">${this._l2()}</h2>
        <p class="hero-subline">${this._sb()}</p>
      </div>
    `;
    if (this.embedded) return body;
    return html`
      <section class="marketing-hero" role="banner">
        ${body}
      </section>
    `;
  }

  // Silence unused-import warning for environments that lint imports
  // strictly. `nothing` is part of the Lit surface this component may
  // grow into (conditional chrome slots etc.); kept for forward shape.
  _keepNothing() { return nothing; }
}

if (!customElements.get("lthn-marketing-hero")) {
  customElements.define("lthn-marketing-hero", LthnMarketingHero);
}

export { LthnMarketingHero };
export { DEFAULT_LINE1, DEFAULT_LINE2, DEFAULT_SUBLINE };
