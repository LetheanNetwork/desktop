// Tests for the response-shape bound-check helpers (Mantis #1491 MED).
//
// Coverage shape: Good (happy-path passes through), Bad (rejects /
// clamps malformed input without throwing), Ugly (pathological inputs
// — 1M rows, 5MB string fields, deep recursion, non-array, prototype
// pollution attempts).
import { describe, it, expect } from "vitest";
import {
  clampRows,
  boundedRows,
  clampString,
  clampFields,
  isRecord,
  DEFAULT_MAX_ROWS,
  DEFAULT_MAX_FIELD_BYTES,
  DEFAULT_MAX_FIELDS_PER_ROW,
} from "./_bounds";

describe("isRecord", () => {
  it("TestBounds_IsRecord_PlainObject_Good", () => {
    expect(isRecord({ a: 1 })).toBe(true);
    expect(isRecord({})).toBe(true);
  });

  it("TestBounds_IsRecord_NonObjects_Bad", () => {
    expect(isRecord(null)).toBe(false);
    expect(isRecord(undefined)).toBe(false);
    expect(isRecord(42)).toBe(false);
    expect(isRecord("string")).toBe(false);
    expect(isRecord(true)).toBe(false);
    expect(isRecord([])).toBe(false);
    expect(isRecord([1, 2])).toBe(false);
  });

  it("TestBounds_IsRecord_NonPlainObjects_Ugly", () => {
    expect(isRecord(new Date())).toBe(false);
    expect(isRecord(new Map())).toBe(false);
    expect(isRecord(new Set())).toBe(false);
    expect(isRecord(/regex/)).toBe(false);
    // Prototype-rebound objects shouldn't slip through as records.
    const weird = Object.create({ inherited: true });
    expect(isRecord(weird)).toBe(false);
  });
});

describe("clampRows", () => {
  it("TestBounds_ClampRows_UnderCap_Good", () => {
    const rows = [1, 2, 3];
    expect(clampRows<number>(rows)).toEqual([1, 2, 3]);
  });

  it("TestBounds_ClampRows_AtCap_Good", () => {
    const rows = Array.from({ length: DEFAULT_MAX_ROWS }, (_, i) => i);
    expect(clampRows<number>(rows)).toHaveLength(DEFAULT_MAX_ROWS);
  });

  it("TestBounds_ClampRows_OverCap_Bad", () => {
    const rows = Array.from({ length: DEFAULT_MAX_ROWS + 100 }, (_, i) => i);
    const out = clampRows<number>(rows);
    expect(out).toHaveLength(DEFAULT_MAX_ROWS);
    expect(out[0]).toBe(0);
    expect(out[DEFAULT_MAX_ROWS - 1]).toBe(DEFAULT_MAX_ROWS - 1);
  });

  it("TestBounds_ClampRows_NonArray_Bad", () => {
    expect(clampRows(null)).toEqual([]);
    expect(clampRows(undefined)).toEqual([]);
    expect(clampRows({})).toEqual([]);
    expect(clampRows("not an array")).toEqual([]);
    expect(clampRows(42)).toEqual([]);
  });

  it("TestBounds_ClampRows_OneMillionRows_Ugly", () => {
    const rows = Array.from({ length: 1_000_000 }, (_, i) => ({ id: i }));
    const out = clampRows<{ id: number }>(rows);
    expect(out).toHaveLength(DEFAULT_MAX_ROWS);
  });

  it("TestBounds_ClampRows_CustomMax_Good", () => {
    const rows = [1, 2, 3, 4, 5];
    expect(clampRows<number>(rows, 3)).toEqual([1, 2, 3]);
  });
});

describe("boundedRows", () => {
  const accept = (r: unknown) => (isRecord(r) && typeof r.id === "string" ? { id: r.id } : null);

  it("TestBounds_BoundedRows_AllValid_Good", () => {
    const rows = [{ id: "a" }, { id: "b" }];
    expect(boundedRows(rows, accept)).toEqual([{ id: "a" }, { id: "b" }]);
  });

  it("TestBounds_BoundedRows_MixedValidity_Bad", () => {
    const rows = [{ id: "a" }, { id: 42 }, null, { id: "b" }, "string"];
    expect(boundedRows(rows, accept)).toEqual([{ id: "a" }, { id: "b" }]);
  });

  it("TestBounds_BoundedRows_NonArray_Bad", () => {
    expect(boundedRows(null, accept)).toEqual([]);
    expect(boundedRows("not an array", accept)).toEqual([]);
    expect(boundedRows({ length: 5 }, accept)).toEqual([]);
  });

  it("TestBounds_BoundedRows_TenKRowsCapped_Ugly", () => {
    const rows = Array.from({ length: 10_000 }, (_, i) => ({ id: `row-${i}` }));
    const out = boundedRows(rows, accept);
    expect(out).toHaveLength(DEFAULT_MAX_ROWS);
    expect(out[0]).toEqual({ id: "row-0" });
  });

  it("TestBounds_BoundedRows_AllRejected_Ugly", () => {
    const rows = [1, "two", null, undefined, [], {}];
    expect(boundedRows(rows, accept)).toEqual([]);
  });
});

describe("clampString", () => {
  it("TestBounds_ClampString_ShortAscii_Good", () => {
    expect(clampString("hello")).toBe("hello");
    expect(clampString("")).toBe("");
  });

  it("TestBounds_ClampString_Unicode_Good", () => {
    expect(clampString("héllo 世界 🚀")).toBe("héllo 世界 🚀");
  });

  it("TestBounds_ClampString_NonString_Bad", () => {
    expect(clampString(null)).toBe("");
    expect(clampString(undefined)).toBe("");
    expect(clampString(42)).toBe("");
    expect(clampString({})).toBe("");
    expect(clampString([])).toBe("");
  });

  it("TestBounds_ClampString_OverCapAscii_Bad", () => {
    const big = "a".repeat(DEFAULT_MAX_FIELD_BYTES + 1000);
    const out = clampString(big);
    expect(out.length).toBe(DEFAULT_MAX_FIELD_BYTES);
  });

  it("TestBounds_ClampString_OneMegabyte_Ugly", () => {
    const big = "x".repeat(1_000_000);
    const out = clampString(big);
    expect(out.length).toBe(DEFAULT_MAX_FIELD_BYTES);
  });

  it("TestBounds_ClampString_MultibyteBoundary_Ugly", () => {
    // 4-byte emoji repeated; cap intentionally lands mid-codepoint to
    // exercise the trailing-U+FFFD trim.
    const big = "🚀".repeat(50_000);
    const out = clampString(big, 100);
    // Output is valid (no trailing replacement char), and within cap.
    expect(out).not.toMatch(/�/);
    const encoded = new TextEncoder().encode(out);
    expect(encoded.length).toBeLessThanOrEqual(100);
  });

  it("TestBounds_ClampString_CustomMax_Good", () => {
    expect(clampString("hello world", 5)).toBe("hello");
  });
});

describe("clampFields", () => {
  it("TestBounds_ClampFields_UnderCap_Good", () => {
    const r = { a: 1, b: 2, c: 3 };
    expect(clampFields(r)).toEqual({ a: 1, b: 2, c: 3 });
  });

  it("TestBounds_ClampFields_NonRecord_Bad", () => {
    expect(clampFields(null)).toEqual({});
    expect(clampFields([])).toEqual({});
    expect(clampFields("string")).toEqual({});
  });

  it("TestBounds_ClampFields_OverCap_Ugly", () => {
    const r: Record<string, number> = {};
    for (let i = 0; i < 10_000; i++) r[`key${i}`] = i;
    const out = clampFields(r);
    expect(Object.keys(out)).toHaveLength(DEFAULT_MAX_FIELDS_PER_ROW);
    // Insertion order preserved — first 50 keys kept.
    expect(out["key0"]).toBe(0);
    expect(out[`key${DEFAULT_MAX_FIELDS_PER_ROW - 1}`]).toBe(DEFAULT_MAX_FIELDS_PER_ROW - 1);
    expect(out[`key${DEFAULT_MAX_FIELDS_PER_ROW}`]).toBeUndefined();
  });

  it("TestBounds_ClampFields_CustomMax_Good", () => {
    const r = { a: 1, b: 2, c: 3, d: 4 };
    const out = clampFields(r, 2);
    expect(Object.keys(out)).toHaveLength(2);
  });
});
