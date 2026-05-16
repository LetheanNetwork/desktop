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

/** Peek the current session-token without consuming or persisting it
 *  anywhere new. Intended for closure-only consumers that need a
 *  read-at-call-time view of the cache (e.g. plugin-view shims that
 *  broker the §5.1 postMessage handshake on behalf of an iframe webapp;
 *  they call peekSessionToken inside a `tokenProvider: () => string`
 *  closure passed to the shim, so the token only crosses a boundary
 *  when the shim actually needs to grant it). Returns null when no
 *  token is set OR when the user has locked.
 *
 *  Cerberus #1465 discipline: token still lives ONLY in this module's
 *  cachedSessionToken binding. Peek returns the current value; callers
 *  MUST NOT persist what they receive (localStorage / cookie / etc).
 *  The audit trail (plugin.view.capability_granted) is the observability
 *  surface, not the storage one. */
export function peekSessionToken(): string | null {
  return cachedSessionToken;
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
