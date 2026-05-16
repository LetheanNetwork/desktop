// SPDX-Licence-Identifier: EUPL-1.2
//
// Tests for the tier-0 lit viewStore helper per
// plans/code/lthn/desktop/views/RFC.plugin-views.md §8 + §8.1
// (Cerberus MED-4 quota caps).

import { describe, it, expect, beforeEach, beforeAll } from "vitest";
import {
  viewStore,
  clearAllPluginStores,
  PER_PLUGIN_BYTE_CAP,
  TOTAL_PLUGIN_BYTE_CAP,
} from "./view-store";

// happy-dom exposes window.localStorage as a property name but the
// getter throws / returns a non-functional object in the workers we
// run against. Replace with a Map-backed shim that implements the
// Storage interface tightly enough for our caps + key/value paths.
beforeAll(() => {
  const map = new Map<string, string>();
  const shim: Storage = {
    get length() {
      return map.size;
    },
    clear() {
      map.clear();
    },
    getItem(key: string) {
      return map.has(key) ? map.get(key)! : null;
    },
    key(index: number) {
      return Array.from(map.keys())[index] ?? null;
    },
    removeItem(key: string) {
      map.delete(key);
    },
    setItem(key: string, value: string) {
      map.set(key, value);
    },
  };
  Object.defineProperty(globalThis, "localStorage", {
    configurable: true,
    value: shim,
  });
  Object.defineProperty(window, "localStorage", {
    configurable: true,
    value: shim,
  });
});

beforeEach(() => {
  clearAllPluginStores();
});

describe("viewStore — namespaced read/write", () => {
  it("isolates keys per plugin code", () => {
    const a = viewStore("alpha");
    const b = viewStore("beta");

    a.set("k", "alpha-value");
    b.set("k", "beta-value");

    expect(a.get("k")).toBe("alpha-value");
    expect(b.get("k")).toBe("beta-value");
  });

  it("remove + clear scope to the plugin", () => {
    const a = viewStore("alpha");
    const b = viewStore("beta");

    a.set("k1", "1");
    a.set("k2", "2");
    b.set("k1", "B1");

    a.remove("k1");
    expect(a.get("k1")).toBeNull();
    expect(a.get("k2")).toBe("2");
    expect(b.get("k1")).toBe("B1");

    a.clear();
    expect(a.keys().length).toBe(0);
    expect(b.get("k1")).toBe("B1");
  });

  it("requires a non-empty pluginCode", () => {
    expect(() => viewStore("")).toThrowError(/pluginCode is required/);
  });
});

describe("viewStore — quota caps", () => {
  // Per-plugin cap — one plugin trying to push past 1 MiB rejects.
  // Test pattern uses one key that's just-under-cap + a second
  // attempt that pushes total past 1 MiB.
  it("TestViewStore_PerPluginByteCap_Bad rejects writes above PER_PLUGIN_BYTE_CAP", () => {
    const s = viewStore("alpha");
    // Pre-fill the plugin with a near-cap entry. Account for the key
    // string itself in the byte total (key.length + value.length).
    const keyOverhead = "lthn.plugin.alpha.first".length;
    const remaining = PER_PLUGIN_BYTE_CAP - keyOverhead - 10; // 10-byte buffer
    s.set("first", "x".repeat(remaining));

    // Next write should put us past the cap.
    let caught: unknown;
    try {
      s.set("second", "y".repeat(100));
    } catch (e) {
      caught = e;
    }
    expect((caught as { name?: string })?.name).toBe("QuotaExceededError");
  });

  // Total cap — many plugins together cannot push past 10 MiB.
  it("TestViewStore_TotalPluginStoreCap_Bad rejects writes above TOTAL_PLUGIN_BYTE_CAP", () => {
    // Fill several plugins each near-per-plugin-cap so the total
    // sits just under TOTAL_PLUGIN_BYTE_CAP. Then attempt to push
    // any additional byte and assert rejection.
    const perPluginPayload = PER_PLUGIN_BYTE_CAP - 50;
    const pluginsNeeded = Math.floor(TOTAL_PLUGIN_BYTE_CAP / PER_PLUGIN_BYTE_CAP);

    for (let i = 0; i < pluginsNeeded; i++) {
      const s = viewStore(`plugin${i}`);
      s.set("payload", "x".repeat(perPluginPayload));
    }

    // One more plugin trying to write should be rejected by total cap.
    const overflow = viewStore("overflow");
    let caught: unknown;
    try {
      overflow.set("k", "y".repeat(1024));
    } catch (e) {
      caught = e;
    }
    expect((caught as { name?: string })?.name).toBe("QuotaExceededError");
  });

  it("allows replacing existing keys without re-counting old bytes", () => {
    const s = viewStore("alpha");
    s.set("k", "x".repeat(1000));
    // Replacing with same-size payload must not trip the cap.
    s.set("k", "y".repeat(1000));
    expect(s.get("k")).toBe("y".repeat(1000));
  });
});
