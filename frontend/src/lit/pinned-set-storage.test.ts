// SPDX-Licence-Identifier: EUPL-1.2

import { describe, it, expect, beforeEach } from "vitest";
import { createPinnedSet } from "./pinned-set-storage";

const KEY = "test.pinned-set-storage";

describe("createPinnedSet — shared Set<string> ↔ localStorage helper", () => {
  beforeEach(() => {
    localStorage.removeItem(KEY);
  });

  it("load returns an empty Set when no key is stored", () => {
    const store = createPinnedSet(KEY);
    const out = store.load();
    expect(out).toBeInstanceOf(Set);
    expect(out.size).toBe(0);
  });

  it("save → load round-trips the set contents", () => {
    const store = createPinnedSet(KEY);
    store.save(new Set(["alpha", "bravo", "charlie"]));
    const out = store.load();
    expect(out.size).toBe(3);
    expect(out.has("alpha")).toBe(true);
    expect(out.has("bravo")).toBe(true);
    expect(out.has("charlie")).toBe(true);
  });

  it("save serialises to a sorted JSON array (stable storage shape)", () => {
    const store = createPinnedSet(KEY);
    store.save(new Set(["c", "a", "b"]));
    const raw = localStorage.getItem(KEY);
    expect(raw).toBe('["a","b","c"]');
  });

  it("load returns empty for malformed JSON", () => {
    localStorage.setItem(KEY, "not-valid-json{");
    expect(createPinnedSet(KEY).load().size).toBe(0);
  });

  it("load returns empty when the value is not an array", () => {
    localStorage.setItem(KEY, '{"not":"array"}');
    expect(createPinnedSet(KEY).load().size).toBe(0);
  });

  it("load filters out non-string entries safely", () => {
    localStorage.setItem(KEY, '["a", 42, null, "b"]');
    const out = createPinnedSet(KEY).load();
    expect(out.size).toBe(2);
    expect(out.has("a")).toBe(true);
    expect(out.has("b")).toBe(true);
  });

  it("toggle returns a NEW Set (Lit reference identity)", () => {
    const store = createPinnedSet(KEY);
    const initial = new Set(["x"]);
    const next = store.toggle(initial, "y");
    expect(next).not.toBe(initial);
    expect(initial.has("y"), "input not mutated").toBe(false);
    expect(next.has("y")).toBe(true);
  });

  it("toggle adds when absent, removes when present", () => {
    const store = createPinnedSet(KEY);
    let s = new Set<string>();
    s = store.toggle(s, "alpha");
    expect(s.has("alpha")).toBe(true);
    s = store.toggle(s, "alpha");
    expect(s.has("alpha")).toBe(false);
    s = store.toggle(s, "alpha");
    expect(s.has("alpha")).toBe(true);
  });

  it("two stores with different keys don't collide", () => {
    const a = createPinnedSet("test.pinned.A");
    const b = createPinnedSet("test.pinned.B");
    a.save(new Set(["only-A"]));
    b.save(new Set(["only-B"]));
    expect(a.load().has("only-A")).toBe(true);
    expect(a.load().has("only-B")).toBe(false);
    expect(b.load().has("only-A")).toBe(false);
    expect(b.load().has("only-B")).toBe(true);
    localStorage.removeItem("test.pinned.A");
    localStorage.removeItem("test.pinned.B");
  });
});
