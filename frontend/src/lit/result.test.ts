// SPDX-Licence-Identifier: EUPL-1.2
//
// Unit tests for the core.Result unwrap/demand helpers. Pure async
// logic over the { OK, Value } envelope — no DOM, no bindings.

import { describe, it, expect } from "vitest";
import { unwrap, demand } from "./result";
import type { Result } from "../../bindings/dappco.re/go/models";

const ok = (value: unknown): Result => ({ OK: true, Value: value } as unknown as Result);
const fail = (value: unknown): Result => ({ OK: false, Value: value } as unknown as Result);

describe("unwrap — graceful read-side unwrap", () => {
  it("returns the typed Value when Result.OK is true", async () => {
    const out = await unwrap<string[]>(Promise.resolve(ok(["a", "b"])), []);
    expect(out).toEqual(["a", "b"]);
  });

  it("returns the fallback when Result.OK is false", async () => {
    const out = await unwrap<string[]>(Promise.resolve(fail("boom")), ["fallback"]);
    expect(out).toEqual(["fallback"]);
  });

  it("returns the fallback when the promise rejects", async () => {
    const out = await unwrap<number>(Promise.reject(new Error("network")), 42);
    expect(out).toBe(42);
  });

  it("passes through a falsy-but-valid Value (0, '', false) on OK", async () => {
    expect(await unwrap<number>(Promise.resolve(ok(0)), 99)).toBe(0);
    expect(await unwrap<string>(Promise.resolve(ok("")), "fb")).toBe("");
    expect(await unwrap<boolean>(Promise.resolve(ok(false)), true)).toBe(false);
  });
});

describe("demand — write-side unwrap that throws", () => {
  it("returns the typed Value when Result.OK is true", async () => {
    const out = await demand<{ id: string }>(Promise.resolve(ok({ id: "x" })));
    expect(out).toEqual({ id: "x" });
  });

  it("throws with the string Value as the message on OK=false", async () => {
    await expect(demand(Promise.resolve(fail("disk full")))).rejects.toThrow("disk full");
  });

  it("throws with the nested .error string when Value is an object", async () => {
    await expect(demand(Promise.resolve(fail({ error: "constraint violated" }))))
      .rejects.toThrow("constraint violated");
  });

  it("throws a generic message when Value carries no usable error shape", async () => {
    await expect(demand(Promise.resolve(fail({ unrelated: 1 }))))
      .rejects.toThrow("service call failed");
  });

  it("propagates a promise rejection unchanged", async () => {
    await expect(demand(Promise.reject(new Error("transport down"))))
      .rejects.toThrow("transport down");
  });
});
