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

/** Same-origin fetch wrapper that injects Authorization: Bearer.
 *  Drop-in replacement for fetch — callers don't change their
 *  request/response handling. Pass the same (input, init) you'd
 *  pass to fetch(). */
export async function apiFetch(
  input: RequestInfo | URL,
  init?: RequestInit,
): Promise<Response> {
  const token = await loadToken();
  const headers = new Headers(init?.headers ?? {});
  if (token && !headers.has("Authorization")) {
    headers.set("Authorization", `Bearer ${token}`);
  }
  return fetch(input, { ...init, headers });
}
