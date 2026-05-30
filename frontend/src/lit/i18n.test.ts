// SPDX-Licence-Identifier: EUPL-1.2
//
// Unit tests for the safe i18n T() wrapper. The wrapper guards the raw
// coreservice T() binding so a missing key / non-string envelope / failed
// Result never crashes the .replace("%s", …) interpolation sites. We mock
// the generated binding so we can drive every return shape it might emit.

import { describe, it, expect, vi, beforeEach } from "vitest";

// Mock the generated binding the wrapper imports. The factory exposes a
// mutable `__rawTImpl` we swap per-test so each case controls what the
// raw binding resolves to.
let rawTImpl: (key: string) => unknown = async (k: string) => k;
vi.mock("@lthn/i18n/coreservice", () => ({
  T: (key: string) => rawTImpl(key),
}));

import { T } from "./i18n";

beforeEach(() => {
  rawTImpl = async (k: string) => k;
});

describe("T — safe i18n wrapper", () => {
  it("returns the raw translation when it resolves to a non-empty string", async () => {
    rawTImpl = async () => "Bonjour";
    expect(await T("greeting")).toBe("Bonjour");
  });

  it("falls back to the key when the translation is an empty string", async () => {
    rawTImpl = async () => "";
    expect(await T("greeting")).toBe("greeting");
  });

  it("unwraps a core.Result envelope { OK, Value } carrying the string", async () => {
    rawTImpl = async () => ({ OK: true, Value: "Hola" });
    expect(await T("greeting")).toBe("Hola");
  });

  it("falls back to the key when the envelope Value is empty", async () => {
    rawTImpl = async () => ({ OK: true, Value: "" });
    expect(await T("greeting")).toBe("greeting");
  });

  it("falls back to the key when the raw value is null", async () => {
    rawTImpl = async () => null;
    expect(await T("missing.key")).toBe("missing.key");
  });

  it("falls back to the key when the raw value is undefined", async () => {
    rawTImpl = async () => undefined;
    expect(await T("missing.key")).toBe("missing.key");
  });

  it("falls back to the key when the raw binding throws (i18n unreachable)", async () => {
    rawTImpl = async () => { throw new Error("service down"); };
    expect(await T("any.key")).toBe("any.key");
  });

  it("returns the supplied fallback instead of the key when one is given", async () => {
    rawTImpl = async () => null;
    expect(await T("title.key", "Default Title")).toBe("Default Title");
  });

  it("prefers a real translation over the supplied fallback", async () => {
    rawTImpl = async () => "Real Value";
    expect(await T("title.key", "Default Title")).toBe("Real Value");
  });
});
