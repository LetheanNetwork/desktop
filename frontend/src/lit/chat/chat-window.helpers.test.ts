// SPDX-Licence-Identifier: EUPL-1.2
//
// Direct unit tests for the pure helpers extracted from chat-window
// (Athena 2026-05-16). These import the helpers module directly — no
// LthnChatWindow element boot — so the pure-logic surface is exercised
// in isolation. (chat-window.test.ts also covers some of these via the
// re-export from the element module; this file pins the helpers
// independently, including paths that file doesn't reach: relativeTime,
// the tag parser/filter, the composer counter, and withSystemPrompt /
// userMessageHistory.)

import { describe, it, expect, beforeEach } from "vitest";
import {
  relativeTime,
  matchesTagFilter,
  parseTagInput,
  tagsToInput,
  withSystemPrompt,
  userMessageHistory,
  composerCounterText,
  composerCounterTone,
  COMPOSER_COUNTER_WARN_AT,
  COMPOSER_MAX_HEIGHT,
  buildTranscript,
  deriveAutoTitle,
  AUTO_TITLE_MAX,
  flattenForDisplay,
  matchesSearch,
  splitPinnedConversations,
  loadPinnedConversations,
  savePinnedConversations,
  PINNED_CONVERSATIONS_KEY,
  loadActiveConversation,
  saveActiveConversation,
  ACTIVE_CONVERSATION_KEY,
} from "./chat-window.helpers";
import type { ChatTurn, Conversation } from "../types";

describe("relativeTime — humanised past timestamps", () => {
  const NOW = 1_700_000_000; // fixed reference, unix seconds

  it("returns empty string for a zero / negative timestamp", () => {
    expect(relativeTime(0, NOW)).toBe("");
    expect(relativeTime(-5, NOW)).toBe("");
  });

  it("returns 'now' for under a minute", () => {
    expect(relativeTime(NOW - 30, NOW)).toBe("now");
    expect(relativeTime(NOW, NOW)).toBe("now");
  });

  it("renders minutes for under an hour", () => {
    expect(relativeTime(NOW - 5 * 60, NOW)).toBe("5m ago");
  });

  it("renders hours for under a day", () => {
    expect(relativeTime(NOW - 3 * 3600, NOW)).toBe("3h ago");
  });

  it("renders 'yesterday' between 24 and 48 hours", () => {
    expect(relativeTime(NOW - 30 * 3600, NOW)).toBe("yesterday");
  });

  it("renders days under a week", () => {
    expect(relativeTime(NOW - 3 * 86400, NOW)).toBe("3d ago");
  });

  it("renders weeks under ~4 weeks", () => {
    expect(relativeTime(NOW - 2 * 604800, NOW)).toBe("2w ago");
  });

  it("renders an absolute UTC date past ~4 weeks", () => {
    const then = Date.UTC(2023, 0, 15) / 1000; // 2023-01-15
    const now = then + 60 * 86400; // 60 days later
    expect(relativeTime(then, now)).toBe("2023-01-15");
  });
});

describe("matchesTagFilter", () => {
  it("empty filter matches everything (no filter applied)", () => {
    expect(matchesTagFilter({ tags: ["x"] }, "")).toBe(true);
    expect(matchesTagFilter({ tags: [] }, "   ")).toBe(true);
    expect(matchesTagFilter(null, null)).toBe(true);
  });

  it("matches a tag case-insensitively", () => {
    expect(matchesTagFilter({ tags: ["Work", "Personal"] }, "work")).toBe(true);
    expect(matchesTagFilter({ tags: ["work"] }, "WORK")).toBe(true);
  });

  it("does not match when the tag is absent", () => {
    expect(matchesTagFilter({ tags: ["work"] }, "play")).toBe(false);
  });

  it("handles a conversation with no tags field", () => {
    expect(matchesTagFilter({}, "work")).toBe(false);
    expect(matchesTagFilter(null, "work")).toBe(false);
  });
});

describe("parseTagInput / tagsToInput — round-trip normalisation", () => {
  it("trims, lowercases, drops blanks, de-duplicates (first wins)", () => {
    expect(parseTagInput("Work,  personal , WORK , ,  ")).toEqual(["work", "personal"]);
  });

  it("returns [] for null / empty input", () => {
    expect(parseTagInput(null)).toEqual([]);
    expect(parseTagInput("")).toEqual([]);
  });

  it("tagsToInput joins with comma-space", () => {
    expect(tagsToInput(["work", "personal"])).toBe("work, personal");
  });

  it("tagsToInput returns empty string for null / empty array", () => {
    expect(tagsToInput(null)).toBe("");
    expect(tagsToInput([])).toBe("");
  });

  it("parse(render(parse(x))) is stable", () => {
    const once = parseTagInput("Work, Work, personal");
    expect(parseTagInput(tagsToInput(once))).toEqual(once);
  });
});

describe("withSystemPrompt", () => {
  type Msg = { role: string; content: string };
  const hist: Msg[] = [{ role: "user", content: "hi" }];

  it("prepends a system message when the prompt is non-empty", () => {
    expect(withSystemPrompt(hist, "be terse")).toEqual([
      { role: "system", content: "be terse" },
      { role: "user", content: "hi" },
    ]);
  });

  it("returns the history unchanged for empty / whitespace prompts", () => {
    expect(withSystemPrompt(hist, "")).toBe(hist);
    expect(withSystemPrompt(hist, "   ")).toBe(hist);
    expect(withSystemPrompt(hist, null)).toBe(hist);
  });

  it("trims the prompt before embedding it", () => {
    expect(withSystemPrompt(hist, "  steer  ")[0]).toEqual({ role: "system", content: "steer" });
  });
});

describe("userMessageHistory", () => {
  const turn = (role: ChatTurn["role"], text: string): ChatTurn => ({ role, text });

  it("returns only the user-side messages in source order", () => {
    const out = userMessageHistory([
      turn("you", "one"),
      turn("model", "reply"),
      turn("you", "two"),
    ]);
    expect(out).toEqual(["one", "two"]);
  });

  it("filters whitespace-only user turns", () => {
    expect(userMessageHistory([turn("you", "   "), turn("you", "kept")])).toEqual(["kept"]);
  });

  it("returns [] for null turns", () => {
    expect(userMessageHistory(null)).toEqual([]);
  });
});

describe("composer counter", () => {
  it("renders empty string for blank input", () => {
    expect(composerCounterText("")).toBe("");
    expect(composerCounterText(null)).toBe("");
  });

  it("renders singular '1 char' and plural 'N chars'", () => {
    expect(composerCounterText("a")).toBe("1 char");
    expect(composerCounterText("abc")).toBe("3 chars");
  });

  it("tone is quiet below the warn threshold, warn at/above it", () => {
    expect(composerCounterTone("a".repeat(COMPOSER_COUNTER_WARN_AT - 1))).toBe("quiet");
    expect(composerCounterTone("a".repeat(COMPOSER_COUNTER_WARN_AT))).toBe("warn");
  });

  it("exposes the documented constants", () => {
    expect(COMPOSER_COUNTER_WARN_AT).toBe(1500);
    expect(COMPOSER_MAX_HEIGHT).toBe(240);
  });
});

describe("buildTranscript / deriveAutoTitle", () => {
  const turn = (role: ChatTurn["role"], text: string): ChatTurn => ({ role, text });

  it("buildTranscript labels speakers and joins with blank lines", () => {
    expect(buildTranscript([turn("you", "ping"), turn("model", "pong")], "Gemma"))
      .toBe("You: ping\n\nGemma: pong");
  });

  it("buildTranscript falls back to Assistant on an empty model name", () => {
    expect(buildTranscript([turn("model", "hi")], "")).toBe("Assistant: hi");
  });

  it("buildTranscript returns empty for null / empty turns", () => {
    expect(buildTranscript(null, "X")).toBe("");
    expect(buildTranscript([], "X")).toBe("");
  });

  it("deriveAutoTitle normalises whitespace and trims", () => {
    expect(deriveAutoTitle("  Hello\t\nworld  ")).toBe("Hello world");
  });

  it("deriveAutoTitle truncates past AUTO_TITLE_MAX with an ellipsis", () => {
    const out = deriveAutoTitle("a".repeat(AUTO_TITLE_MAX + 5));
    expect(out.length).toBe(AUTO_TITLE_MAX);
    expect(out.endsWith("…")).toBe(true);
  });

  it("deriveAutoTitle returns empty for whitespace-only input", () => {
    expect(deriveAutoTitle("   \n\t ")).toBe("");
  });
});

describe("flattenForDisplay / matchesSearch / splitPinnedConversations", () => {
  const c = (id: string, bucket: Conversation["bucket"], title = id): Conversation =>
    ({ id, bucket, title, snippet: "", model: "" });

  it("flattenForDisplay orders by bucket (today, yesterday, week)", () => {
    const out = flattenForDisplay([c("a", "week"), c("b", "today"), c("c", "yesterday")]);
    expect(out.map(x => x.id)).toEqual(["b", "c", "a"]);
  });

  it("matchesSearch is a case-insensitive title substring match", () => {
    expect(matchesSearch(c("x", "today", "Launch Plan"), "launch")).toBe(true);
    expect(matchesSearch(c("x", "today", "Launch Plan"), "zebra")).toBe(false);
    expect(matchesSearch(c("x", "today", "anything"), "")).toBe(true);
  });

  it("splitPinnedConversations partitions by pin membership preserving order", () => {
    const out = splitPinnedConversations(
      [c("a", "today"), c("b", "today"), c("c", "today")],
      new Set(["b"]),
    );
    expect(out.pinned.map(x => x.id)).toEqual(["b"]);
    expect(out.rest.map(x => x.id)).toEqual(["a", "c"]);
  });
});

describe("pinned-conversation persistence (localStorage)", () => {
  beforeEach(() => localStorage.removeItem(PINNED_CONVERSATIONS_KEY));

  it("loadPinnedConversations returns an empty Set when the key is missing", () => {
    const s = loadPinnedConversations();
    expect(s).toBeInstanceOf(Set);
    expect(s.size).toBe(0);
  });

  it("save then load round-trips the id set", () => {
    savePinnedConversations(new Set(["b", "a"]));
    const loaded = loadPinnedConversations();
    expect(loaded.has("a")).toBe(true);
    expect(loaded.has("b")).toBe(true);
  });

  it("uses the documented key", () => {
    expect(PINNED_CONVERSATIONS_KEY).toBe("lthn.chat.pinned");
  });
});

describe("active-conversation persistence (localStorage)", () => {
  beforeEach(() => localStorage.removeItem(ACTIVE_CONVERSATION_KEY));

  it("save writes, load reads", () => {
    saveActiveConversation("conv-1");
    expect(loadActiveConversation()).toBe("conv-1");
  });

  it("save(null) clears the key", () => {
    saveActiveConversation("conv-1");
    saveActiveConversation(null);
    expect(loadActiveConversation()).toBeNull();
  });

  it("uses the documented key", () => {
    expect(ACTIVE_CONVERSATION_KEY).toBe("lthn.chat.active-conversation");
  });
});
