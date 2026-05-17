// SPDX-Licence-Identifier: EUPL-1.2
//
// <lthn-plugin-view-opencode-shim> — a thin wrapper around
// <lthn-plugin-view> that performs the host-side postMessage
// handshake described in plans/code/lthn/desktop/views/RFC.plugin-views.md
// §5.1 on behalf of a third-party iframe plugin (opencode is the
// proving case).
//
// The upstream opencode bundle is unaware of lthn; until it grows a
// `lthn:auth:request` originator the shim's handshake handler simply
// sits idle. When opencode (or any other iframe webapp) eventually
// dispatches the request message at its parent, the shim:
//
//   1. Filters incoming `message` events to the wrapped iframe's
//      contentWindow only (rejects any other source — HIGH-2 inbound
//      verification).
//   2. Verifies `event.origin` matches the descriptor's loopback
//      origin (rejects cross-origin impersonation).
//   3. Validates requested scopes are declared in descriptor.capabilities
//      and forwarded to a known broker (v1 only honours session-token).
//   4. Delegates the actual token release to api-fetch.grantTokenToFrame
//      — the broker owns the session-token closure, audits server-side,
//      and postMessages on success. The shim never holds token bytes
//      (RFC.plugin-view-token-boundary v2 §3.0 broker contract).
//   5. Emits a window CustomEvent (`lthn:plugin-view:auth-granted` /
//      `lthn:plugin-view:auth-denied`) for the UI observability
//      consumer; the token bytes are NEVER included in the event detail.
//
// The shim does NOT log the token bytes. It never persists them. It
// never reads them. Token release crosses the closure boundary of one
// module (api-fetch) — the shim only sees a GrantOutcome discriminator.
//
// Wiring shape: app-shell instantiates this element in place of
// <lthn-plugin-view> for iframe-kind plugins that declare a session-
// token capability. The shim forwards .descriptor + .viewId to the
// inner element verbatim — consumers don't need to know whether the
// shim or the bare element is mounted.
//
// Usage example (inside the host shell):
//
//   <lthn-plugin-view-opencode-shim
//     .descriptor=${desc}
//   ></lthn-plugin-view-opencode-shim>

import { LitElement, html } from "lit";
import "./lthn-plugin-view";
import type { PluginViewDescriptor } from "./lthn-plugin-view";
import { grantTokenToFrame } from "./api-fetch";

// AUTH_REQUEST_TYPE matches the §5.1 protocol literal the plugin
// webapp posts at its parent. Strings live as exported consts so the
// frontend + the plugin-side contract are referenced from one place.
// AUTH_GRANT_TYPE re-homed to api-fetch (RFC v2 §5 step b) — the
// broker is the only emitter; importers reach for it from there.
export const AUTH_REQUEST_TYPE = "lthn:auth:request";
export const AUTH_DENY_TYPE = "lthn:auth:deny";

// CustomEvent names dispatched on `window` after the handshake. The
// audit consumer subscribes; the token bytes are NEVER in the detail.
export const AUTH_GRANTED_EVENT = "lthn:plugin-view:auth-granted";
export const AUTH_DENIED_EVENT = "lthn:plugin-view:auth-denied";

// CAPABILITY_SESSION_TOKEN is the §5 capability literal the shim
// honours today. Future capabilities (vi-events, marketplace) plug
// in via the same handler shape but route through different brokers.
export const CAPABILITY_SESSION_TOKEN = "session-token";

// DENY_REASON_AUDIT_FAILED is the local deny-reason the shim emits
// when grantTokenToFrame reports the audit chain failed. The grant
// message is never delivered to the iframe; the deny message tells
// the iframe the request was rejected for an audit-side reason.
export const DENY_REASON_AUDIT_FAILED = "audit-failed";

// AuthRequestMessage is the shape the iframe posts at its parent.
interface AuthRequestMessage {
  type: typeof AUTH_REQUEST_TYPE;
  scopes: string[];
}

// AuthDenyMessage is the shape posted back when the request is rejected.
interface AuthDenyMessage {
  type: typeof AUTH_DENY_TYPE;
  reason: string;
}

// DenyReason enumerates the strings emitted on rejection. Callers can
// branch on these literals when surfacing the reason to the user.
export const DENY_REASON_NO_DESCRIPTOR = "no-descriptor";
export const DENY_REASON_NO_ORIGIN = "no-origin";
export const DENY_REASON_ORIGIN_MISMATCH = "origin-mismatch";
export const DENY_REASON_SOURCE_MISMATCH = "source-mismatch";
export const DENY_REASON_NO_SESSION = "no-session";
export const DENY_REASON_CAPABILITY_NOT_DECLARED = "capability-not-declared";
export const DENY_REASON_UNKNOWN_SCOPE = "unknown-scope";

export class OpenCodeShimElement extends LitElement {
  static readonly properties = {
    viewId: { type: String, attribute: "view-id" },
    descriptor: { state: true },
  };

  declare viewId: string;
  declare descriptor: PluginViewDescriptor | null;

  constructor() {
    super();
    this.viewId = "";
    this.descriptor = null;
  }

  createRenderRoot() {
    return this;
  }

  connectedCallback() {
    super.connectedCallback();
    window.addEventListener("message", this._onMessage);
  }

  disconnectedCallback() {
    window.removeEventListener("message", this._onMessage);
    super.disconnectedCallback();
  }

  // _onMessage handles every postMessage delivered to `window`. The
  // gate chain (descriptor present → source-window matches inner
  // iframe → origin matches descriptor → message shape valid) drops
  // anything that isn't a well-formed handshake request from THIS
  // shim's wrapped iframe. On a valid request the actual token
  // release is delegated to grantTokenToFrame — the shim never holds
  // token bytes (RFC.plugin-view-token-boundary v2 §3.0).
  private _onMessage = (ev: MessageEvent) => {
    if (!this.descriptor) {
      return;
    }
    const data = ev.data as Partial<AuthRequestMessage> | null;
    if (!data || data.type !== AUTH_REQUEST_TYPE) {
      return;
    }

    const iframe = this.querySelector<HTMLIFrameElement>("iframe");
    if (!iframe) {
      // The inner <lthn-plugin-view> hasn't materialised the iframe
      // yet (lit-kind branch or mount-timeout state). Drop silently;
      // the iframe re-posts on its own connectedCallback when it
      // mounts (per §5.1 step 5).
      return;
    }
    if (ev.source !== iframe.contentWindow) {
      this._deny(ev, DENY_REASON_SOURCE_MISMATCH);
      return;
    }

    const expectedOrigin = (this.descriptor.loopbackOrigin ?? "").trim();
    if (!expectedOrigin) {
      this._deny(ev, DENY_REASON_NO_ORIGIN);
      return;
    }
    if (ev.origin !== expectedOrigin) {
      this._deny(ev, DENY_REASON_ORIGIN_MISMATCH);
      return;
    }

    const requestedScopes = Array.isArray(data.scopes) ? data.scopes : [];
    if (requestedScopes.length === 0) {
      this._deny(ev, DENY_REASON_CAPABILITY_NOT_DECLARED);
      return;
    }
    const declared = this.descriptor.capabilities ?? [];
    const granted: string[] = [];
    for (const scope of requestedScopes) {
      if (!declared.includes(scope)) {
        this._deny(ev, DENY_REASON_CAPABILITY_NOT_DECLARED);
        return;
      }
      if (scope !== CAPABILITY_SESSION_TOKEN) {
        // v1 only brokers the session-token capability; vi-events +
        // marketplace are forward-contract slots gated on future
        // brokers. Reject so the plugin sees the contract violation
        // instead of a silent honour-when-unimplemented surface.
        this._deny(ev, DENY_REASON_UNKNOWN_SCOPE);
        return;
      }
      granted.push(scope);
    }
    if (granted.length === 0) {
      this._deny(ev, DENY_REASON_CAPABILITY_NOT_DECLARED);
      return;
    }
    void this._broker(ev, granted, expectedOrigin);
  };

  // _broker delegates the token release to api-fetch.grantTokenToFrame.
  // The broker owns the session-token closure — the shim never sees
  // token bytes; only the GrantOutcome discriminator. On success the
  // shim dispatches the AUTH_GRANTED audit CustomEvent (token-free
  // detail). On failure it routes the reason to _deny so the iframe
  // sees a deny message via the same path as descriptor-side rejections.
  private async _broker(
    ev: MessageEvent,
    grantedScopes: string[],
    targetOrigin: string,
  ): Promise<void> {
    const pluginCode = this.descriptor?.pluginCode ?? "";
    const source = ev.source as Window | null;
    if (!source || typeof source.postMessage !== "function") {
      // No usable target — drop silently; the iframe re-posts on
      // its next connectedCallback when the inner element mounts.
      return;
    }
    const outcome = await grantTokenToFrame(
      { source, targetOrigin, pluginCode },
      grantedScopes,
    );
    if (outcome.ok) {
      try {
        window.dispatchEvent(new CustomEvent(AUTH_GRANTED_EVENT, {
          detail: {
            viewId:     this.descriptor?.id ?? "",
            pluginCode: pluginCode,
            scopes:     outcome.scopes,
          },
          bubbles: true,
          composed: true,
        }));
      } catch {
        // Browser-CustomEvent dispatch is best-effort UI observability;
        // the load-bearing forensic surface is the server-side audit row
        // grantTokenToFrame already committed.
      }
      return;
    }
    // outcome.ok === false — route the reason through _deny so the
    // iframe receives a structured deny message + the audit CustomEvent
    // path stays consistent with descriptor-side rejections.
    if (outcome.reason === "audit-failed") {
      this._deny(ev, DENY_REASON_AUDIT_FAILED);
    } else {
      this._deny(ev, DENY_REASON_NO_SESSION);
    }
  }

  // _deny emits the §5.1 deny message with the given reason + an
  // audit event. The reason literal is part of the contract; callers
  // grep for them in the audit log.
  private _deny(ev: MessageEvent, reason: string): void {
    const expectedOrigin = (this.descriptor?.loopbackOrigin ?? "").trim();
    const payload: AuthDenyMessage = {
      type: AUTH_DENY_TYPE,
      reason,
    };
    const source = ev.source as Window | null;
    if (source && typeof source.postMessage === "function" && expectedOrigin) {
      try {
        source.postMessage(payload, expectedOrigin);
      } catch {
        // Non-browser context — drop silently.
      }
    }
    try {
      window.dispatchEvent(new CustomEvent(AUTH_DENIED_EVENT, {
        detail: {
          viewId: this.descriptor?.id ?? "",
          pluginCode: this.descriptor?.pluginCode ?? "",
          reason,
        },
        bubbles: true,
        composed: true,
      }));
    } catch {
      // Audit dispatch is best-effort.
    }
  }

  // render passes descriptor + viewId straight through to the inner
  // <lthn-plugin-view>. The shim adds the handshake-handler layer
  // without changing what the user sees.
  render() {
    if (this.descriptor) {
      return html`
        <lthn-plugin-view
          .descriptor=${this.descriptor}
          view-id=${this.descriptor.id}
        ></lthn-plugin-view>
      `;
    }
    return html`
      <lthn-plugin-view view-id=${this.viewId}></lthn-plugin-view>
    `;
  }
}

if (!customElements.get("lthn-plugin-view-opencode-shim")) {
  customElements.define("lthn-plugin-view-opencode-shim", OpenCodeShimElement);
}

declare global {
  interface HTMLElementTagNameMap {
    "lthn-plugin-view-opencode-shim": OpenCodeShimElement;
  }
}
