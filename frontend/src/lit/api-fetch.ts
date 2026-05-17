// SPDX-Licence-Identifier: EUPL-1.2

// Cerberus Mantis #1430 — same-origin fetch wrapper that injects the
// local bearer token so WithBearerAuth on the lthn HTTP API can stay
// enabled. Before this wrapper existed, the WebView fetched the API
// with no Authorization header, every request 401'd, and the server
// auth was commented out as a workaround (TEMP DISABLED 2026-05-13
// note in pkg/server/service.go). That left the entire HTTP surface
// open to any local process. This module closes that gap.
//
// Design:
//   - Token is loaded once via apikey.Reveal() on first call and
//     cached in module state. apikey.Reveal returns the same value
//     the Go side feeds into WithBearerAuth (single source of truth).
//   - Token rotation (apikey.WRotate from Settings → API) is rare;
//     the cached token goes stale until the WebView reloads. If
//     that becomes an issue, expose a clearApiToken() helper from
//     this module + call it from the Rotate button's success path.
//   - Empty token = no Authorization header (degraded mode — the
//     server will 401; the failure surfaces to the caller as a
//     normal HTTP error rather than a silent auth bypass).
//
// Usage:
//
//   import { apiFetch } from "@desktop/lit/api-fetch";
//   const res = await apiFetch("/v1/api/process/list", { method: "GET" });
//   const json = await res.json();
//
// Behaviour is identical to fetch() except that:
//   - the Authorization: Bearer <token> header is injected when known;
//   - the wrapper is fire-and-forget on the token lookup — if Reveal
//     fails the request still goes out, and the server responds 401
//     so the caller can surface the auth problem.

let cachedToken: string | null = null;
let inflightTokenFetch: Promise<string> | null = null;

// Stage E.C — session-token chokepoint. When the unlock flow succeeds,
// auth-gate calls setSessionToken(t) with the LTHN-SESS-1.<…> token
// returned by /v1/account/unlock. While set, apiFetch prefers this over
// the static LocalKey bearer so user-data endpoints route through the
// session tier (per RFC.stage-e §4).
//
// Closure-only discipline per Cerberus #1465 + RFC.stage-e §3.2 — the
// token lives in this module-scope binding ONLY. NO localStorage, NO
// sessionStorage, NO cookie, NO IndexedDB. App restart = re-unlock.
// `clearSessionToken()` zeros the binding on lock / 401 / expiry so
// the next apiFetch falls back to LocalKey for local-tier reads.
let cachedSessionToken: string | null = null;

/** Replace the in-memory session-token. Called by auth-gate after a
 *  successful unlock. Passing an empty string is equivalent to
 *  clearSessionToken — the falsy branch falls back to LocalKey. */
export function setSessionToken(token: string): void {
  cachedSessionToken = typeof token === "string" && token.length > 0 ? token : null;
}

/** Drop the session-token. Called by the lock handler, by apiFetch on
 *  401 against a session-tier route (the token expired or was rotated
 *  server-side), and by the gate's _onWreathIn retry path. */
export function clearSessionToken(): void {
  cachedSessionToken = null;
}

// AUTH_GRANT_TYPE — §5.1 protocol literal posted to the iframe when
// grantTokenToFrame succeeds. Re-homed from plugin-view-opencode-shim
// per RFC.plugin-view-token-boundary v2 §5 step (b): the literal
// describes the wire shape the broker produces, so it belongs with
// the broker (api-fetch is the only module that POSTs this payload).
export const AUTH_GRANT_TYPE = "lthn:auth:grant";

// CAPABILITY_GRANT_AUDIT_PATH — server-side audit-grant receiver per
// Mantis #1523. grantTokenToFrame POSTs here BEFORE delivering token
// bytes via postMessage so the audit row commits first; on non-200
// the broker short-circuits with reason "audit-failed" and the
// postMessage path is never reached. Path kept in lockstep with
// pkg/server.PluginViewCapabilityGrantPath.
export const CAPABILITY_GRANT_AUDIT_PATH = "/v1/plugin-view/capability-grant";

// GrantTargetFrame — narrow shape grantTokenToFrame accepts as its
// destination. Mirrors the inbound-verified MessageEvent fields the
// caller (shim) already validated. api-fetch does NOT re-verify these
// — verification is the shim's job (it has the descriptor + iframe
// ref; api-fetch does not). api-fetch trusts what the shim passes and
// uses targetOrigin as the explicit postMessage targetOrigin.
//
// Browser-enforced mitigation for forged source: a malicious internal
// caller passing source=attackerWindow with targetOrigin=legitimateOrigin
// does NOT escape the boundary. The browser's postMessage targetOrigin
// enforcement silently drops the message because the actual receiving
// window's origin does not match the named targetOrigin. The audit row
// commits BEFORE postMessage, naming targetOrigin as legitimateOrigin
// and pluginCode as the caller's claim — so a forged-source attempt
// leaves a server-side forensic record under the legitimate plugin's
// identity while the token never crosses to the attacker. See
// RFC.plugin-view-token-boundary v2 §6.2.
export interface GrantTargetFrame {
  source: Window;
  targetOrigin: string;
  pluginCode: string;
}

// GrantOutcome — discriminator the caller branches on to dispatch
// UI events. Token bytes are NEVER in this return value — only the
// outcome literal + reason on deny (RFC §3.0 contract item (c)).
export type GrantOutcome =
  | { ok: true; scopes: string[] }
  | { ok: false; reason: "audit-failed" | "no-session" };

/** grantTokenToFrame — the ONLY path that releases cachedSessionToken
 *  outside api-fetch.ts. Audits server-side per scope FIRST; on any
 *  non-200 short-circuits with reason "audit-failed" and does NOT
 *  postMessage. On success, posts ONE grant message per call carrying
 *  the token + granted scopes to target.source with target.targetOrigin
 *  as the explicit targetOrigin (never "*").
 *
 *  Returns a GrantOutcome the caller uses to dispatch its UI event;
 *  the token bytes do NOT escape via the return value. The function
 *  itself is the boundary — no other code path reads cachedSessionToken.
 *
 *  Closure discipline (Cerberus #1465 mechanism, not convention — RFC
 *  §3.0 broker invariant contract):
 *    (a) audit server-side BEFORE the postMessage.
 *    (b) take (target, scopes) shape so the audit row names
 *        origin + pluginCode + capability.
 *    (c) return outcome-discriminator with NO state bytes.
 *    (d) read state from a closure-scoped const referenced only by
 *        the postMessage call, discarded on return.
 *    (e) live in the same module that owns the state's closure —
 *        never accept state as an argument from caller.
 *
 *  Caller responsibilities (NOT enforced here — design intent):
 *    - Verify ev.source === iframe.contentWindow before calling.
 *    - Verify ev.origin === descriptor.loopbackOrigin before calling.
 *    - Validate descriptor.capabilities includes every requested scope.
 *    - Pass pluginCode consistent with the descriptor the user granted.
 *
 *  Usage:
 *
 *    const outcome = await grantTokenToFrame(
 *      { source: ev.source as Window, targetOrigin: expectedOrigin, pluginCode: code },
 *      ["session-token"],
 *    );
 *    if (outcome.ok) dispatchGranted(outcome.scopes);
 *    else            dispatchDenied(outcome.reason);
 */
export async function grantTokenToFrame(
  target: GrantTargetFrame,
  scopes: readonly string[],
): Promise<GrantOutcome> {
  // Per-scope audit POST, abort on first failure. The shim's previous
  // _grant loop relocated into the module that owns the token (RFC
  // §3.0 item (a)+(e)). Server-side audit-row semantic gap: rows
  // committed before a later-scope failure persist; readers MUST
  // interpret "row exists" as "grant-attempted" not "grant-occurred"
  // until the follow-up at Mantis #1576 lands.
  for (const scope of scopes) {
    let ok = false;
    try {
      const res = await apiFetch(CAPABILITY_GRANT_AUDIT_PATH, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          plugin_id:  target.pluginCode,
          capability: scope,
          origin:     target.targetOrigin,
        }),
      });
      ok = res.ok;
    } catch {
      ok = false;
    }
    if (!ok) return { ok: false, reason: "audit-failed" };
  }

  // Token read into closure-scoped const HERE — after audit, before
  // postMessage. No branch returns it; no branch persists it; no
  // branch logs it. JS engine reclaims on function return (RFC §3.0
  // item (d)).
  const token = cachedSessionToken;
  if (!token) return { ok: false, reason: "no-session" };

  const payload = { type: AUTH_GRANT_TYPE, token, scopes: [...scopes] };
  try {
    target.source.postMessage(payload, target.targetOrigin);
  } catch {
    // Non-browser context — drop silently. The audit row already
    // committed; caller still receives ok=true since the audit chain
    // succeeded. Future test harnesses that stub source can rely on
    // this shape.
  }
  return { ok: true, scopes: [...scopes] };
}

/** Internal — resolve the local bearer token via the Wails apikey
 *  service. Cached after first successful resolution. Returns the
 *  empty string when the binding is unavailable or returns empty —
 *  callers see the unauthenticated request fail at the server. */
async function loadToken(): Promise<string> {
  if (cachedToken !== null) return cachedToken;
  if (inflightTokenFetch) return inflightTokenFetch;
  inflightTokenFetch = (async () => {
    try {
      // Late import so the module graph stays SSR-friendly + the test
      // mock at @wailsio/runtime intercepts the binding's Call.ByID.
      const svc = await import("@desktop/apikey/wailsservice");
      const v = await svc.Reveal();
      // Reveal returns coreapi.Result; unwrap defensively.
      const tok = (v as { Value?: unknown })?.Value;
      cachedToken = typeof tok === "string" ? tok : "";
    } catch {
      cachedToken = "";
    } finally {
      inflightTokenFetch = null;
    }
    return cachedToken ?? "";
  })();
  return inflightTokenFetch;
}

/** Drop the cached token so the next apiFetch re-loads from
 *  apikey.Reveal(). Call from the Settings → API "Rotate" button
 *  success path so the WebView picks up the fresh key without a
 *  full reload. */
export function clearApiToken(): void {
  cachedToken = null;
}

/** CustomEvent name dispatched on `window` whenever an apiFetch
 *  response comes back with status 401. The auth-gate (and the
 *  app-shell) listen for this and transition into the framed-error
 *  state so the user sees a recoverable surface instead of the raw
 *  unauthorised envelope. Mirrors the constant exported from
 *  auth-gate.ts; kept here verbatim so api-fetch has no inbound
 *  dependency on the lit-element layer. */
export const AUTH_401_EVENT = "lthn:auth:401";

/** Same-origin fetch wrapper that injects Authorization: Bearer.
 *  Drop-in replacement for fetch — callers don't change their
 *  request/response handling. Pass the same (input, init) you'd
 *  pass to fetch().
 *
 *  Stage C of plans/code/lthn/desktop/auth-gate/RFC.md §5 — on a 401
 *  response we additionally dispatch a window-level CustomEvent so
 *  the auth-gate can mount. The original response is still returned
 *  to the caller so their existing error path keeps firing; the
 *  event is purely additive. */
export async function apiFetch(
  input: RequestInfo | URL,
  init?: RequestInit,
): Promise<Response> {
  // Session-token (post-unlock) takes precedence over the static
  // LocalKey. RFC.stage-e §4 — session-tier routes require it;
  // local-tier routes accept either. Falling back to LocalKey when no
  // session is present keeps /health + setup-flow probes working
  // before the user unlocks. Authorization-header on init wins outright
  // so callers can override per-request when needed.
  const session = cachedSessionToken;
  const token = session ?? await loadToken();
  const headers = new Headers(init?.headers ?? {});
  if (token && !headers.has("Authorization")) {
    headers.set("Authorization", `Bearer ${token}`);
  }
  const response = await fetch(input, { ...init, headers });
  if (response.status === 401) {
    // Session-tier 401 means the token expired / was rotated server-side.
    // Drop it so the next apiFetch falls back to LocalKey (which 401s on
    // session-tier routes, surfacing the gate). Without this clear, the
    // expired token sticks and every subsequent request burns the same
    // expired-401 cycle.
    if (session) cachedSessionToken = null;
    // Surface the request-id from the server response so the gate's
    // error frame can show the same trace id the server-side logs
    // index by. Header name follows the api gateway's existing
    // X-Request-Id convention.
    const requestId = response.headers.get("X-Request-Id")
      || response.headers.get("X-Request-ID")
      || "";
    try {
      window.dispatchEvent(new CustomEvent(AUTH_401_EVENT, { detail: { requestId } }));
    } catch {
      // Non-browser context (SSR / worker) — dispatch is best-effort.
    }
  }
  return response;
}
