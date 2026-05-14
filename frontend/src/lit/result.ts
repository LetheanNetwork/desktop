// SPDX-Licence-Identifier: EUPL-1.2
//
// Helpers for consuming Wails bindings that return Go's `core.Result`
// shape ({ OK: boolean; Value: any }). Every Service method that can
// fail now returns Result (per v0.9.0 canon — single-return collapses
// the old (T, error) tuple). TS callers need to unwrap.
//
// Two flavours:
//
//   unwrap<T>(promise, fallback)
//     → success returns Value cast to T; fail / reject returns fallback.
//     Use for read-side flows that should degrade gracefully (lists,
//     status snapshots, optional metadata).
//
//   demand<T>(promise)
//     → success returns Value cast to T; fail / reject throws.
//     Use for write-side flows where the caller needs to surface
//     errors (Create, Append, Save) and you want try/catch.
//
// Pairs with the frontend bindings at
// frontend/bindings/dappco.re/lthn/desktop/pkg/*/{service,wailsservice}.ts.

import type { Result } from "../../bindings/dappco.re/go/models";

/** Awaits a Promise<Result>; returns the typed Value on OK, or
 *  fallback otherwise. Never throws. */
export async function unwrap<T>(p: Promise<Result>, fallback: T): Promise<T> {
  try {
    const r = await p;
    return r.OK ? (r.Value as T) : fallback;
  } catch {
    return fallback;
  }
}

/** Awaits a Promise<Result>; returns the typed Value on OK, throws
 *  on Result.OK=false or promise rejection. Use when callers wrap
 *  in try/catch and need the error surfaced. */
export async function demand<T>(p: Promise<Result>): Promise<T> {
  const r = await p;
  if (!r.OK) {
    const msg = typeof r.Value === "string" ? r.Value
      : (r.Value && typeof r.Value === "object" && "error" in r.Value
          ? String((r.Value as { error: unknown }).error)
          : "service call failed");
    throw new Error(msg);
  }
  return r.Value as T;
}
