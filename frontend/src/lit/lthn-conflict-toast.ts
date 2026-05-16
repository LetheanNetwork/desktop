// SPDX-Licence-Identifier: EUPL-1.2
//
// <lthn-conflict-toast> — single-mount shared toast that surfaces
// stale-version write conflicts to the user without per-view duplication.
//
// Cascade W1 (Mantis #1540) — listens for CONFLICT_409_EVENT dispatched
// by handleStaleVersionConflict at wails-call-sites. Renders a calm,
// dismissable toast offering a [Reload] action. Reload fires
// CONFLICT_RELOAD_EVENT with the same service identifier so the view
// that owns the data can re-pull from the backend.
//
// Mounted once in <lthn-app-shell> (top-level overlay region). Views do
// NOT mount their own toast — that would multi-fire on the same event.
//
// Calm-voice copy per HANDOVER-VIEWS.md: observational, not alarming.
// "Edited elsewhere — reload to see the current version."
//
// Dark-only treatment per feedback_dark_only_design_system.

import { LitElement, html, nothing } from "lit";
import {
  CONFLICT_409_EVENT,
  CONFLICT_RELOAD_EVENT,
  type ConflictDetail,
} from "./conflict-dispatch";

/** Map a service identifier ("sales.contacts.update") to the
 *  user-visible entity label rendered in the toast ("Contacts"). Pure
 *  helper so tests can pin the mapping. Unknown services fall back to
 *  a generic "Record". */
export function entityLabelFor(service: string | undefined): string {
  if (!service) return "Record";
  // service format: "<area>.<entity>.<verb>" e.g. "sales.contacts.update".
  const parts = service.split(".");
  if (parts.length < 2) return "Record";
  const entity = parts[1];
  switch (entity) {
    case "contacts": return "Contacts";
    case "deals":    return "Deal";
    case "pipeline": return "Pipeline";
    default:
      // Title-case the raw segment as a sane default.
      return entity.charAt(0).toUpperCase() + entity.slice(1);
  }
}

class LthnConflictToast extends LitElement {
  static readonly properties = {
    /** When non-null, the toast renders for this detail. Cleared on
     *  Reload or Dismiss. */
    _active: { state: true },
  };

  declare _active: ConflictDetail | null;

  /** Bound listener for CONFLICT_409_EVENT so removeEventListener can
   *  unbind on disconnect — keeps test isolation clean. */
  private _onConflict = (e: Event) => {
    const ce = e as CustomEvent<ConflictDetail>;
    this._active = ce.detail ?? null;
  };

  constructor() {
    super();
    this._active = null;
  }

  createRenderRoot() { return this; }

  connectedCallback() {
    super.connectedCallback();
    window.addEventListener(CONFLICT_409_EVENT, this._onConflict);
  }

  disconnectedCallback() {
    super.disconnectedCallback();
    window.removeEventListener(CONFLICT_409_EVENT, this._onConflict);
  }

  /** User clicked Reload — fire the request event (sales views
   *  subscribe + filter by service) and dismiss the toast. The toast
   *  does NOT trigger the reload itself; the data-owning view is
   *  the source-of-truth. */
  _onReload() {
    const detail = this._active;
    this._active = null;
    if (!detail) return;
    try {
      window.dispatchEvent(new CustomEvent(CONFLICT_RELOAD_EVENT, { detail }));
    } catch { /* non-browser context */ }
  }

  /** User clicked Dismiss — clear without firing the reload signal.
   *  User edits in the view stay intact; the next write that conflicts
   *  will surface a fresh toast. */
  _onDismiss() {
    this._active = null;
  }

  render() {
    const d = this._active;
    if (!d) return nothing;

    const label = entityLabelFor(d.service);

    return html`
      <div class="lthn-conflict-toast"
           role="alert"
           aria-live="polite"
           style="position:fixed; bottom:48px; left:50%; transform:translateX(-50%);
                  z-index:10000;
                  display:flex; align-items:center; gap:14px;
                  padding:12px 16px; border-radius:10px;
                  background:rgba(15, 14, 20, 0.95);
                  border:1px solid rgba(255, 255, 255, 0.10);
                  box-shadow:0 14px 40px rgba(0, 0, 0, 0.55);
                  font-family:var(--font-sans);
                  font-size:12.5px; color:var(--fg-1); line-height:1.55;">
        <i class="fa-solid fa-circle-info"
           style="color:var(--brand-300); font-size:13px;"></i>
        <span style="flex:1;">
          ${label} was edited elsewhere. Reload to see the current version.
        </span>
        <button class="lthn-conflict-toast-reload"
                data-action="reload"
                @click=${() => this._onReload()}
                style="padding:5px 10px; border-radius:6px;
                       background:rgba(146, 100, 244, 0.16);
                       border:1px solid rgba(146, 100, 244, 0.32);
                       color:var(--brand-300); font-family:var(--font-mono);
                       font-size:10.5px; cursor:pointer;">Reload</button>
        <button class="lthn-conflict-toast-dismiss"
                data-action="dismiss"
                aria-label="Dismiss"
                @click=${() => this._onDismiss()}
                style="padding:5px 8px; border-radius:6px;
                       background:transparent;
                       border:1px solid rgba(255, 255, 255, 0.08);
                       color:var(--fg-3); font-family:var(--font-mono);
                       font-size:10.5px; cursor:pointer;">×</button>
      </div>
    `;
  }
}

customElements.define("lthn-conflict-toast", LthnConflictToast);
