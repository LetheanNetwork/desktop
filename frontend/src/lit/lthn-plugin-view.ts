// SPDX-Licence-Identifier: EUPL-1.2
//
// <lthn-plugin-view> — the lit wrapper that mounts a marketplace
// plugin's view into the shell body slot, per
// plans/code/lthn/desktop/views/RFC.plugin-views.md §3 + §6.
//
// Two render branches discriminated by descriptor.kind:
//
//   - "iframe" — render <iframe sandbox="..." src=...> with the
//     defaultPluginIframeSandbox floor. The 1500ms mount-timeout
//     starts in connectedCallback; if the iframe load event hasn't
//     fired by then, dispatches lthn:plugin-view:mount-timeout so
//     the shell falls back per §6 chain.
//
//   - "lit" — render the registered custom-element tag from
//     descriptor.source. v1 ships first-party only; the Go manifest
//     validator gates third-party lit-kind per §5.2 forward-contract.
//
// Hydration: the element calls marketplace.GetViewDescriptor(viewId)
// on connectedCallback per the design_lit_view_backend_wire_pattern
// shape — fixture default + lazy import + safe degradation when the
// binding is unavailable. The descriptor is also accepted directly
// via a .descriptor property so the parent shell can pre-resolve
// without hitting the Go round-trip.

import { LitElement, html, nothing } from "lit";

// PluginViewDescriptor mirrors the Go-side marketplace.
// PluginViewDescriptor shape. Declared here (not imported from a
// generated TS interface) because the Go binding returns the
// descriptor under core.Result.Value as `any` — the frontend
// owns the typed cast.
export interface PluginViewDescriptor {
  id: string;
  label: string;
  icon: string;
  group: "plugin";
  kind: "iframe" | "lit";
  source: string;
  pluginCode: string;
  capabilities?: string[];
  loopbackPort?: number;
  loopbackOrigin?: string;
}

// defaultPluginIframeSandbox is the iframe sandbox attribute floor
// per RFC §3.2 (Cerberus MED-5 / Q1). MUST be exactly these three —
// no allow-popups, allow-top-navigation, allow-modals, allow-downloads.
//
//   - allow-scripts        — plugin webapp runs JS
//   - allow-same-origin    — plugin needs its own loopback-origin
//                            localStorage (§8)
//   - allow-forms          — opencode + any plugin with form inputs
//
// A manifest extension attempting to add any other attribute is
// dropped by the manifest validator (Cerberus MED-5) + audit-logged.
export const defaultPluginIframeSandbox = "allow-scripts allow-forms allow-same-origin";

// Mount-timeout per RFC §6.3 (Cerberus CRIT-3). Iframe must fire
// the `load` event within this window or the shell falls back per
// §6 chain. Hard cap at 3000ms (not user-configurable upward).
export const PLUGIN_VIEW_MOUNT_TIMEOUT_MS = 1500;
export const PLUGIN_VIEW_MOUNT_HARD_CAP_MS = 3000;

// CustomEvent names dispatched by this element. The shell listens
// for the timeout event to trigger the §6 fallback chain.
export const PLUGIN_VIEW_MOUNT_TIMEOUT_EVENT = "lthn:plugin-view:mount-timeout";
export const PLUGIN_VIEW_REVOKED_EVENT = "lthn:plugin-view:revoked";

// PluginViewElement is the public class for typing / test asserts.
export class PluginViewElement extends LitElement {
  static readonly properties = {
    viewId: { type: String, attribute: "view-id" },
    descriptor: { state: true },
    _loaded: { state: true },
    _timedOut: { state: true },
  };

  declare viewId: string;
  declare descriptor: PluginViewDescriptor | null;
  declare _loaded: boolean;
  declare _timedOut: boolean;

  private _timeoutHandle: ReturnType<typeof setTimeout> | null = null;

  constructor() {
    super();
    this.viewId = "";
    this.descriptor = null;
    this._loaded = false;
    this._timedOut = false;
  }

  createRenderRoot() {
    return this;
  }

  async connectedCallback() {
    super.connectedCallback();
    await this._loadFromBackend();
    this._startMountTimeout();
  }

  disconnectedCallback() {
    this._clearMountTimeout();
    super.disconnectedCallback();
  }

  // _loadFromBackend hydrates descriptor from
  // marketplace.GetViewDescriptor(viewId) per design_lit_view_backend_wire_pattern.
  // Skipped when the caller pre-set descriptor via property; the
  // pre-set value wins so the shell can avoid a Go round-trip when
  // it already has the descriptor in hand.
  async _loadFromBackend(): Promise<void> {
    if (this.descriptor) return;
    if (!this.viewId) return;
    try {
      const svc = await import("@desktop/marketplace/service").catch(() => null);
      if (!svc || typeof (svc as { GetViewDescriptor?: unknown }).GetViewDescriptor !== "function") {
        return;
      }
      const r = await (svc as {
        GetViewDescriptor: (id: string) => Promise<{ OK?: boolean; Value?: PluginViewDescriptor }>
      }).GetViewDescriptor(this.viewId);
      if (r?.OK === false) return;
      const d = r?.Value;
      if (d && typeof d === "object" && typeof d.id === "string") {
        this.descriptor = d;
      }
    } catch {
      // Binding unavailable → descriptor stays null. Render falls
      // back to a friendly "view-not-found" panel; the parent shell
      // walks the §6 fallback chain via the timeout event.
    }
  }

  // _startMountTimeout arms the 1500ms timer. For lit-kind views
  // mount is synchronous (custom-element-tag lookup), so the timer
  // is short-circuited when descriptor.kind === "lit" + tag is
  // already registered. For iframe-kind views the timer fires unless
  // _onIframeLoad clears it first.
  private _startMountTimeout(): void {
    if (!this.descriptor) {
      // No descriptor: fire timeout immediately so the shell falls
      // back per §6. Use the soft cap so behaviour is consistent.
      this._scheduleTimeout(PLUGIN_VIEW_MOUNT_TIMEOUT_MS);
      return;
    }
    if (this.descriptor.kind === "lit") {
      const tag = (this.descriptor.source ?? "").trim();
      if (tag && customElements.get(tag)) {
        this._loaded = true;
        return;
      }
      // Tag not registered: fall back via timeout.
      this._scheduleTimeout(PLUGIN_VIEW_MOUNT_TIMEOUT_MS);
      return;
    }
    // Iframe-kind: arm the timer; _onIframeLoad clears it.
    this._scheduleTimeout(PLUGIN_VIEW_MOUNT_TIMEOUT_MS);
  }

  private _scheduleTimeout(ms: number): void {
    const capped = Math.min(ms, PLUGIN_VIEW_MOUNT_HARD_CAP_MS);
    this._timeoutHandle = setTimeout(() => {
      this._timeoutHandle = null;
      if (this._loaded) return;
      this._timedOut = true;
      this._emitTimeoutEvent();
    }, capped);
  }

  private _clearMountTimeout(): void {
    if (this._timeoutHandle) {
      clearTimeout(this._timeoutHandle);
      this._timeoutHandle = null;
    }
  }

  private _emitTimeoutEvent(): void {
    const ev = new CustomEvent(PLUGIN_VIEW_MOUNT_TIMEOUT_EVENT, {
      detail: { viewId: this.viewId, pluginCode: this.descriptor?.pluginCode ?? "" },
      bubbles: true,
      composed: true,
    });
    this.dispatchEvent(ev);
    try {
      window.dispatchEvent(ev);
    } catch {
      // Non-browser context — bubble already fired.
    }
  }

  private _onIframeLoad = () => {
    this._loaded = true;
    this._clearMountTimeout();
  };

  render() {
    if (!this.descriptor) {
      return html`<div style="padding:40px; color:var(--fg-3); font-family:var(--font-sans); font-size:13px;">
        Loading view…
      </div>`;
    }
    if (this._timedOut) {
      return html`<div style="padding:40px; color:var(--fg-3); font-family:var(--font-sans); font-size:13px;">
        Plugin view "${this.descriptor.label}" failed to mount within ${PLUGIN_VIEW_MOUNT_TIMEOUT_MS}ms.
      </div>`;
    }
    if (this.descriptor.kind === "iframe") {
      return this._renderIframe(this.descriptor);
    }
    if (this.descriptor.kind === "lit") {
      return this._renderLit(this.descriptor);
    }
    return html`<div style="padding:40px; color:var(--fg-3);">Unknown view kind</div>`;
  }

  // _renderIframe emits the sandboxed iframe per §3.2. Source is the
  // loopbackOrigin (http://127.0.0.1:<port>) when set, or the manifest
  // source field as a fallback (the Go install path resolves
  // ${expose.<id>.route} into the loopback origin at install time).
  private _renderIframe(d: PluginViewDescriptor) {
    const src = d.loopbackOrigin || d.source || "";
    return html`
      <iframe
        sandbox=${defaultPluginIframeSandbox}
        src=${src}
        title=${d.label}
        @load=${this._onIframeLoad}
        style="flex:1; width:100%; height:100%; border:0; min-height:0;"
      ></iframe>
    `;
  }

  // _renderLit instantiates the registered custom-element tag.
  // Manifest validator guarantees the tag is custom-element-shape;
  // we still fall through cleanly when the tag is unregistered
  // (the §6 fallback chain handles via mount-timeout).
  private _renderLit(d: PluginViewDescriptor) {
    const tag = (d.source ?? "").trim();
    if (!tag || !customElements.get(tag)) {
      return html`<div style="padding:40px; color:var(--fg-3);">
        Lit element "${tag}" not registered.
      </div>`;
    }
    // Use Lit's static html with a literal — but we don't have a
    // literal at this point. document.createElement is the
    // sandbox-safe path; the parent slot renders it directly.
    const el = document.createElement(tag);
    el.setAttribute("embedded", "");
    el.setAttribute("plugin-code", d.pluginCode);
    return html`${el}`;
  }
}

if (!customElements.get("lthn-plugin-view")) {
  customElements.define("lthn-plugin-view", PluginViewElement);
}

// Augment the global HTMLElement registry so TS callers get typed
// access via querySelector / document.createElement.
declare global {
  interface HTMLElementTagNameMap {
    "lthn-plugin-view": PluginViewElement;
  }
}
