// SPDX-Licence-Identifier: EUPL-1.2
//
// Safe i18n wrapper around the generated coreservice T() binding.
//
// The raw binding can resolve to undefined/null — a missing key, or the
// i18n service being unreachable (before first paint, or a failed Result
// envelope). Components interpolate the result with `.replace("%s", …)`
// / `.split("%s")`, which throws `X.replace is not a function` on a
// non-string. That is the cred-migration-banner crash class, and the
// E2E sweep found it across ~9 surfaces.
//
// This wrapper guarantees a string: the real translation when available,
// else the key itself (visible, debuggable) — so a missing/late
// translation degrades to a key, never a crash. Killed at the i18n
// boundary instead of guarded at every interpolation site.
//
// Drop-in: same `T(key)` call signature. Components import from
// "@ui/i18n" instead of "@lthn/i18n/coreservice".

import { T as rawT } from "@lthn/i18n/coreservice";

export async function T(key: string, fallback?: string): Promise<string> {
  try {
    const v: unknown = await rawT(key);
    if (typeof v === "string" && v.length > 0) return v;
    // Tolerate a core.Result envelope ({ OK, Value }) arriving on the wire.
    if (v && typeof v === "object") {
      const val = (v as { Value?: unknown }).Value;
      if (typeof val === "string" && val.length > 0) return val;
    }
  } catch {
    /* i18n unreachable — fall through to the safe default */
  }
  return fallback ?? key;
}
