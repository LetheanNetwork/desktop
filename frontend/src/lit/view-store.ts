// SPDX-Licence-Identifier: EUPL-1.2
//
// viewStore — per-plugin localStorage helper for tier-0 lit views
// (first-party) per plans/code/lthn/desktop/views/RFC.plugin-views.md
// §8 + §8.1 (Cerberus MED-4).
//
// Each lit-kind plugin gets a namespaced store under the prefix
// `lthn.plugin.<code>.*`. The helper enforces two byte caps:
//
//   - per-plugin: 1 MiB across all keys for one plugin code
//   - total: 10 MiB across every `lthn.plugin.*` key
//
// Browser localStorage quota is typically ~5-10 MiB per origin SHARED
// with the shell; exhausting it would brick core shell features.
//
// Exceeding either cap rejects writes with a DOMException-shaped
// `QuotaExceededError` so plugin code can branch on the standard
// `e.name === "QuotaExceededError"` check.
//
// Iframe-kind plugins use their own loopback-origin localStorage
// (browser same-origin isolation handles the cleanup automatically
// on uninstall via §2.1 step 6); only tier-0 lit plugins need this
// helper because they live inside the shell origin.
//
// Usage example (inside a first-party lit-kind plugin view):
//
//   import { viewStore } from "@desktop/lit/view-store";
//   const store = viewStore("lthn");
//   store.set("activeProject", "ofm");
//   const v = store.get("activeProject"); // "ofm"

// Namespace prefix every keyed entry lives under. Cleared on uninstall
// by walking localStorage keys with this prefix per §2.1 step 6.
const STORE_PREFIX = "lthn.plugin.";

// Per-plugin byte cap (1 MiB). A first-party plugin needing more
// should use the medium/io stack, not localStorage.
export const PER_PLUGIN_BYTE_CAP = 1024 * 1024;

// Total cap across every `lthn.plugin.*` key (10 MiB). Defends the
// shared shell origin's localStorage quota.
export const TOTAL_PLUGIN_BYTE_CAP = 10 * 1024 * 1024;

// QuotaExceededError name. Mirrors the platform DOMException name so
// plugin code can branch on the standard literal.
const QUOTA_EXCEEDED_NAME = "QuotaExceededError";

// pluginKey returns the full localStorage key for one plugin's entry.
function pluginKey(pluginCode: string, key: string): string {
  return `${STORE_PREFIX}${pluginCode}.${key}`;
}

// safeLocalStorage returns the browser localStorage or null when
// running in a non-browser context (SSR / tests without jsdom). Every
// public method below falls back gracefully so callers don't need a
// per-call try/catch.
function safeLocalStorage(): Storage | null {
  try {
    return typeof localStorage !== "undefined" ? localStorage : null;
  } catch {
    return null;
  }
}

// pluginBytesUsed returns the total bytes consumed by ONE plugin's
// keys. Calculated as key.length + value.length (UTF-16 char count
// approximation — localStorage quotas are measured in chars on most
// browsers).
function pluginBytesUsed(pluginCode: string): number {
  const ls = safeLocalStorage();
  if (!ls) return 0;
  const prefix = `${STORE_PREFIX}${pluginCode}.`;
  let total = 0;
  for (let i = 0; i < ls.length; i++) {
    const k = ls.key(i);
    if (!k || !k.startsWith(prefix)) continue;
    const v = ls.getItem(k) ?? "";
    total += k.length + v.length;
  }
  return total;
}

// totalPluginBytesUsed returns the bytes used across EVERY plugin's
// keys under STORE_PREFIX. Used for the §8.1 total cap.
function totalPluginBytesUsed(): number {
  const ls = safeLocalStorage();
  if (!ls) return 0;
  let total = 0;
  for (let i = 0; i < ls.length; i++) {
    const k = ls.key(i);
    if (!k || !k.startsWith(STORE_PREFIX)) continue;
    const v = ls.getItem(k) ?? "";
    total += k.length + v.length;
  }
  return total;
}

// throwQuota throws a DOMException-shaped QuotaExceededError. The
// name field is what callers branch on per the web platform
// convention.
function throwQuota(reason: string): never {
  // DOMException is available in browser + jsdom; the fallback
  // throws a plain Error with the same .name field so test
  // environments without DOMException don't crash with a different
  // error shape.
  if (typeof DOMException !== "undefined") {
    throw new DOMException(reason, QUOTA_EXCEEDED_NAME);
  }
  const e = new Error(reason);
  e.name = QUOTA_EXCEEDED_NAME;
  throw e;
}

// PluginViewStore is the namespaced localStorage handle returned by
// viewStore(code). Methods mirror the Storage interface for
// familiarity — get/set/remove/clear/keys.
export interface PluginViewStore {
  get(key: string): string | null;
  set(key: string, value: string): void;
  remove(key: string): void;
  clear(): void;
  keys(): string[];
}

// viewStore returns a namespaced localStorage handle for one plugin
// code. Multiple calls with the same code return equivalent handles
// (they share the underlying storage).
//
// Per RFC.plugin-views §8 — the helper is for tier-0 first-party
// lit plugins. Iframe plugins use their own origin's localStorage
// directly; the browser same-origin policy already isolates them.
//
// Usage example:
//
//   const s = viewStore("lthn");
//   s.set("foo", JSON.stringify({n: 1}));
//   const raw = s.get("foo"); // "{\"n\":1}"
export function viewStore(pluginCode: string): PluginViewStore {
  const code = (pluginCode ?? "").trim();
  if (!code) {
    throw new Error("viewStore: pluginCode is required");
  }

  return {
    get(key: string): string | null {
      const ls = safeLocalStorage();
      if (!ls) return null;
      return ls.getItem(pluginKey(code, key));
    },

    set(key: string, value: string): void {
      const ls = safeLocalStorage();
      if (!ls) return;
      const fullKey = pluginKey(code, key);
      const newPair = fullKey.length + value.length;
      const existing = ls.getItem(fullKey);
      const oldPair = existing === null ? 0 : fullKey.length + existing.length;
      const deltaForPlugin = newPair - oldPair;
      // Per-plugin cap. Calculate post-write bytes vs PER_PLUGIN_BYTE_CAP.
      const pluginAfter = pluginBytesUsed(code) + deltaForPlugin;
      if (pluginAfter > PER_PLUGIN_BYTE_CAP) {
        throwQuota(
          `viewStore: per-plugin cap exceeded (${pluginAfter} > ${PER_PLUGIN_BYTE_CAP} bytes for plugin "${code}")`,
        );
      }
      // Total cap. Same delta applies — we're either replacing or
      // adding bytes under STORE_PREFIX.
      const totalAfter = totalPluginBytesUsed() + deltaForPlugin;
      if (totalAfter > TOTAL_PLUGIN_BYTE_CAP) {
        throwQuota(
          `viewStore: total plugin-store cap exceeded (${totalAfter} > ${TOTAL_PLUGIN_BYTE_CAP} bytes across all plugins)`,
        );
      }
      try {
        ls.setItem(fullKey, value);
      } catch (e) {
        // Re-throw native QuotaExceededError unchanged — callers
        // already handle that shape. Other errors propagate verbatim.
        throw e;
      }
    },

    remove(key: string): void {
      const ls = safeLocalStorage();
      if (!ls) return;
      ls.removeItem(pluginKey(code, key));
    },

    clear(): void {
      const ls = safeLocalStorage();
      if (!ls) return;
      const prefix = `${STORE_PREFIX}${code}.`;
      const toDelete: string[] = [];
      for (let i = 0; i < ls.length; i++) {
        const k = ls.key(i);
        if (k && k.startsWith(prefix)) toDelete.push(k);
      }
      for (const k of toDelete) ls.removeItem(k);
    },

    keys(): string[] {
      const ls = safeLocalStorage();
      if (!ls) return [];
      const prefix = `${STORE_PREFIX}${code}.`;
      const out: string[] = [];
      for (let i = 0; i < ls.length; i++) {
        const k = ls.key(i);
        if (k && k.startsWith(prefix)) {
          out.push(k.slice(prefix.length));
        }
      }
      return out;
    },
  };
}

// clearAllPluginStores wipes every `lthn.plugin.*` key — used by
// test cleanup + future "reset shell state" flows. NOT called by
// uninstall; that path uses viewStore(code).clear() per plugin.
export function clearAllPluginStores(): void {
  const ls = safeLocalStorage();
  if (!ls) return;
  const toDelete: string[] = [];
  for (let i = 0; i < ls.length; i++) {
    const k = ls.key(i);
    if (k && k.startsWith(STORE_PREFIX)) toDelete.push(k);
  }
  for (const k of toDelete) ls.removeItem(k);
}
