// SPDX-Licence-Identifier: EUPL-1.2

// Wails-call-site conflict dispatch — sales/deals/pipeline writes go
// through Wails $Call.ByID returning core.Result, NOT through apiFetch.
// The RFC's apiFetch CONFLICT_409_EVENT extension is unreachable for
// these consumers; the event still serves as the shared signal, but it
// fires from the consumer's update-handler closure when the core.Result
// indicates a stale-version conflict.
//
// Cascade W1 (Mantis #1540) forward-path: the apiFetch HTTP-tier
// extension lands when the first HTTP-tier writer exists. Until then
// every Wails-side write that wraps the paths.VersionStale envelope as
// a "<service>.<verb>.conflict" core code can route through this
// helper to surface the same toast.
//
// Pattern at call-site:
//
//   const result = await svc.UpdateContact(id, input);
//   if (!result.OK) {
//     if (handleStaleVersionConflict(result, "sales.contacts.update")) {
//       // Toast surfaces; view can choose to re-load via the
//       // CONFLICT_RELOAD_EVENT listener.
//       return;
//     }
//     // Other error — show normal error path.
//     throw new Error(typeof result.Value === "string" ? result.Value : "unknown error");
//   }

/** Window event fired by handleStaleVersionConflict when a Wails-side
 *  write returns a paths.VersionStale-wrapped conflict. The shared
 *  <lthn-conflict-toast> element listens for this event and renders
 *  the user-visible reload prompt. */
export const CONFLICT_409_EVENT = "lthn:conflict:stale";

/** Window event fired by <lthn-conflict-toast> when the user clicks
 *  Reload. Sales views subscribe (scoped by service) and call their
 *  own `_loadFromBackend()` when their service matches. */
export const CONFLICT_RELOAD_EVENT = "lthn:conflict:reload-requested";

/** Internal — the structured payload shape emitted by Go-side writers
 *  that wrap paths.VersionStale. The primitive ships the envelope
 *  under one of two shapes depending on Result.Value serialisation:
 *  the envelope direct OR nested under an `error` key. Both are
 *  handled. */
interface VersionStaleEnvelope {
  code?: string;
  current_version?: number;
  current_hash?: string;
  message?: string;
}

/** Detail payload of CONFLICT_409_EVENT. Listeners (notably
 *  <lthn-conflict-toast>) read this to render the user surface. */
export interface ConflictDetail {
  /** The "current_version" the disk now holds (from VersionStale). */
  currentVersion?: number;
  /** The "current_hash" the disk now holds (from VersionStale). */
  currentHash?: string;
  /** The error code the Go writer returned, e.g.
   *  "sales.contacts.update.conflict". */
  errorCode?: string;
  /** The service identifier the call-site supplied
   *  (e.g. "sales.contacts.update"). Listeners scope-filter on this so
   *  one toast can serve multiple views without cross-talk. */
  service?: string;
}

/** Wails core.Result shape. Mirrors the Go core.Result envelope: OK
 *  flag plus an opaque Value (success payload OR error envelope). */
interface CoreResult {
  OK: boolean;
  Value: unknown;
}

/** Inspect a core.Result for a stale-version conflict shape and
 *  dispatch CONFLICT_409_EVENT if matched. Returns true if a conflict
 *  was detected and dispatched, false otherwise (caller continues
 *  their normal error path for non-conflict failures).
 *
 *  The match heuristic is "result.Value (or result.Value.error) has a
 *  string `code` field ending in `.conflict`". This is the exact
 *  shape pkg/sales/{contacts,deals,pipeline} produce when wrapping
 *  paths.CodeVersionStale.
 *
 *  Usage example:
 *
 *    const r = await svc.UpdateContact(input);
 *    if (handleStaleVersionConflict(r, "sales.contacts.update")) return;
 *    // ... normal error handling ...
 */
export function handleStaleVersionConflict(
  result: CoreResult,
  serviceCode: string,
): boolean {
  if (!result || result.OK) return false;

  const env = extractEnvelope(result.Value);
  if (!env) return false;

  const detail: ConflictDetail = {
    currentVersion: env.current_version,
    currentHash:    env.current_hash,
    errorCode:      env.code,
    service:        serviceCode,
  };

  try {
    window.dispatchEvent(new CustomEvent<ConflictDetail>(CONFLICT_409_EVENT, { detail }));
  } catch {
    // Non-browser context (worker, server-render) — no-op. The
    // function still reports true so the caller short-circuits its
    // own error path; the matching scope means the conflict was
    // identified even if no listener is wired.
  }

  return true;
}

/** Walk the result.Value shape looking for the VersionStale envelope.
 *  Handles both the direct shape (`{code, current_version, ...}`) and
 *  the nested shape (`{error: {code, current_version, ...}}`). Returns
 *  null when no `code` field ending in `.conflict` is found. */
function extractEnvelope(raw: unknown): VersionStaleEnvelope | null {
  if (!raw || typeof raw !== "object") return null;
  const v = raw as Record<string, unknown>;

  if (typeof v.code === "string" && v.code.endsWith(".conflict")) {
    return v as VersionStaleEnvelope;
  }
  if (v.error && typeof v.error === "object") {
    const err = v.error as Record<string, unknown>;
    if (typeof err.code === "string" && err.code.endsWith(".conflict")) {
      return err as VersionStaleEnvelope;
    }
  }
  return null;
}
