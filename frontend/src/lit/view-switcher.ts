// SPDX-Licence-Identifier: EUPL-1.2
//
// <lthn-view-switcher> — titlebar view-selector dropdown that slots
// immediately left of the Search ⌘K button per
// plans/code/lthn/desktop/views/RFC.plugin-views.md §4.1.
//
// Renders a compact pill: [icon] [view label] [▼]. Click toggles a
// dropdown listing every entry from .views. The dropdown emits a
// "lthn:view-switcher:select" event with the picked view id; the
// host shell handles persistence + actual navigation.
//
// Wholly distinct from the sidebar view-switcher (`_renderViewSwitcher`
// inside app-shell.ts) — that one is the expanded rail-mount switcher;
// this is the small titlebar version for one-click access without
// opening the rail.
//
// The chrome wire pattern keeps the element decoupled from app-shell:
// app-shell sets .views + .activeView + .onSelect, the element renders
// and emits. No imports of app-shell from here, no inbound dependency
// on the shell internals.

import { LitElement, html, nothing } from "lit";

// ViewSwitcherEntry mirrors the subset of ViewDef the switcher needs.
// Decoupled from app-shell's ViewDef so a future plugin-view-aware
// caller can pass the union (built-in + plugin) without an interface
// change here.
export interface ViewSwitcherEntry {
  id: string;
  label: string;
  icon: string;
}

export const VIEW_SWITCHER_SELECT_EVENT = "lthn:view-switcher:select";

export class ViewSwitcherElement extends LitElement {
  static readonly properties = {
    views: { state: true },
    activeView: { type: String, attribute: "active-view" },
    _open: { state: true },
  };

  declare views: ViewSwitcherEntry[];
  declare activeView: string;
  declare _open: boolean;

  constructor() {
    super();
    this.views = [];
    this.activeView = "";
    this._open = false;
  }

  createRenderRoot() {
    return this;
  }

  connectedCallback() {
    super.connectedCallback();
    document.addEventListener("click", this._onDocumentClick);
  }

  disconnectedCallback() {
    document.removeEventListener("click", this._onDocumentClick);
    super.disconnectedCallback();
  }

  private _onDocumentClick = (ev: MouseEvent) => {
    if (!this._open) return;
    const target = ev.target as HTMLElement | null;
    if (!target) return;
    if (this.contains(target)) return;
    this._open = false;
  };

  private _toggle = (ev: Event) => {
    ev.stopPropagation();
    this._open = !this._open;
  };

  private _pick = (id: string) => {
    this._open = false;
    this.dispatchEvent(
      new CustomEvent(VIEW_SWITCHER_SELECT_EVENT, {
        detail: { viewId: id },
        bubbles: true,
        composed: true,
      }),
    );
  };

  render() {
    const active = this.views.find(v => v.id === this.activeView) ?? this.views[0];
    if (!active) {
      // No views supplied yet — render an invisible placeholder so
      // the layout doesn't shift when views arrive.
      return html`<span aria-hidden="true" style="display:inline-block; width:0;"></span>`;
    }
    return html`
      <span style="position:relative; display:inline-flex; --wails-draggable: no-drag;">
        <button
          class="lthn-view-switcher-titlebar"
          @click=${this._toggle}
          title=${`View · ${active.label}`}
          style="display:inline-flex; align-items:center; gap:6px;
                 padding:4px 10px; border-radius:6px;
                 background:rgba(255,255,255,0.04);
                 border:1px solid rgba(255,255,255,0.07);
                 color:var(--fg-2); font-family:var(--font-sans);
                 font-size:11.5px; font-weight:500; cursor:pointer;">
          <i class="fa-solid ${active.icon}" style="font-size:10px; color:var(--fg-2);"></i>
          <span>${active.label}</span>
          <i class="fa-solid ${this._open ? "fa-angle-up" : "fa-angle-down"}"
             style="font-size:9px; color:var(--fg-3);"></i>
        </button>
        ${this._open ? this._renderMenu() : nothing}
      </span>
    `;
  }

  private _renderMenu() {
    return html`
      <div style="position:absolute; top:100%; right:0; margin-top:4px;
                  min-width:180px;
                  background:var(--bg-2, #181a1f);
                  border:1px solid rgba(255,255,255,0.08);
                  border-radius:6px; padding:4px 0; z-index:10000;
                  box-shadow:0 8px 24px rgba(0,0,0,0.40);">
        ${this.views.map(v => html`
          <button
            @click=${() => this._pick(v.id)}
            style="display:flex; align-items:center; gap:8px;
                   width:100%; padding:6px 12px;
                   background:${v.id === this.activeView ? "rgba(64,193,197,0.10)" : "transparent"};
                   border:0; color:var(--fg-1);
                   font-family:var(--font-sans); font-size:12px;
                   text-align:left; cursor:pointer;">
            <i class="fa-solid ${v.icon}" style="font-size:11px; color:var(--brand-300); width:14px;"></i>
            <span>${v.label}</span>
          </button>
        `)}
      </div>
    `;
  }
}

if (!customElements.get("lthn-view-switcher")) {
  customElements.define("lthn-view-switcher", ViewSwitcherElement);
}

declare global {
  interface HTMLElementTagNameMap {
    "lthn-view-switcher": ViewSwitcherElement;
  }
}
