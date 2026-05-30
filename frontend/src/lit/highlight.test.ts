// SPDX-Licence-Identifier: EUPL-1.2
//
// Unit tests for the shared highlightMatch substring splitter. Pure
// string logic — drives the rail title + HF result name highlighting.

import { describe, it, expect } from "vitest";
import { highlightMatch } from "./highlight";

describe("highlightMatch — substring split", () => {
  it("returns [] for empty text", () => {
    expect(highlightMatch("", "x")).toEqual([]);
  });

  it("returns a single non-match segment for an empty query", () => {
    expect(highlightMatch("hello world", "")).toEqual([{ text: "hello world", match: false }]);
  });

  it("treats a whitespace-only query as empty (whole string, no match)", () => {
    expect(highlightMatch("hello", "   ")).toEqual([{ text: "hello", match: false }]);
  });

  it("splits a single mid-string match into before/match/after", () => {
    expect(highlightMatch("alpha bravo charlie", "bravo")).toEqual([
      { text: "alpha ", match: false },
      { text: "bravo", match: true },
      { text: " charlie", match: false },
    ]);
  });

  it("matches case-insensitively but preserves the source casing in output", () => {
    expect(highlightMatch("HELLO world", "hello")).toEqual([
      { text: "HELLO", match: true },
      { text: " world", match: false },
    ]);
  });

  it("emits a match segment at the very start with no leading non-match", () => {
    expect(highlightMatch("bravo charlie", "bravo")).toEqual([
      { text: "bravo", match: true },
      { text: " charlie", match: false },
    ]);
  });

  it("walks every occurrence of the query", () => {
    expect(highlightMatch("ababab", "ab")).toEqual([
      { text: "ab", match: true },
      { text: "ab", match: true },
      { text: "ab", match: true },
    ]);
  });

  it("returns a single non-match segment when the query is absent", () => {
    expect(highlightMatch("nothing here", "zebra")).toEqual([
      { text: "nothing here", match: false },
    ]);
  });

  it("trims the query before matching", () => {
    expect(highlightMatch("find me", "  me  ")).toEqual([
      { text: "find ", match: false },
      { text: "me", match: true },
    ]);
  });
});
