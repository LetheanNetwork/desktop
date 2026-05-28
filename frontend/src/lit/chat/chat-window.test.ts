// SPDX-Licence-Identifier: EUPL-1.2
//
// Canonical per-window test — chat-window. Uses the shared mountWindow
// fixture from src/test/window-fixture.ts. Pattern for the other 11
// <lthn-*-window> tests:
//
//   1. Smoke: mounts without throwing + the renderChrome titlebar
//      carries the right title.
//   2. Embedded sweep: when `embedded` attribute is set, the chrome
//      collapses to the flat .lthn-window--embedded container — verifies
//      the two-shell pattern (banked Snider memory).
//   3. Content presence: a couple of distinctive strings the window's
//      body should render (model name in chat, "Models" header in
//      model browser, etc.) so a regression that breaks the body
//      fails loudly.
//   4. Reactive prop: change one declared property, await
//      updateComplete, assert the rendered DOM reflects the change.

import { describe, it, expect } from "vitest";
import { mountWindow, expectChromeTitle, isEmbedded, findCard } from "../../test/window-fixture";

// Side-effect import — defines lthn-chat-window on the registry.
import "./chat-window";

describe("lthn-chat-window — smoke", () => {
  it("mounts + renders the chrome titlebar with 'Lethean Chat'", async () => {
    const { host } = await mountWindow("lthn-chat-window");
    expectChromeTitle(host, "lthn · chat");
    expect(host.querySelector("header")).not.toBeNull();
  });

  it("renders the conversation list rail by default", async () => {
    const { host } = await mountWindow("lthn-chat-window");
    // The conversation rail's search input is a stable marker for
    // the rail section being present. The placeholder text isn't
    // in textContent — assert the input element + its placeholder
    // attribute instead.
    const search = host.querySelector("input.lthn-conversation-search");
    expect(search, "rail should mount a conversation search input").not.toBeNull();
    expect((search as HTMLInputElement).placeholder).toBe("Search conversations");
  });

  it("renders the right-rail metadata when rightRail is expanded", async () => {
    const { host } = await mountWindow("lthn-chat-window", {
      props: { rightRail: "expanded" },
    });
    expect(host.textContent).toContain("Turn metadata");
  });
});

describe("lthn-chat-window — two-shell pattern", () => {
  it("default (no embedded attr) renders the full card with chrome", async () => {
    const { host } = await mountWindow("lthn-chat-window");
    expect(isEmbedded(host)).toBe(false);
    expect(findCard(host)).not.toBeNull();
    expect(host.querySelector("header")).not.toBeNull();
  });

  it("with embedded attribute, collapses to the flat embedded shell", async () => {
    const { host } = await mountWindow("lthn-chat-window", {
      attrs: { embedded: "" },
    });
    expect(isEmbedded(host)).toBe(true);
    // No titlebar in embedded mode — the parent <lthn-app-shell>
    // paints its own.
    expect(host.querySelector("header")).toBeNull();
  });
});

describe("lthn-chat-window — reactive state", () => {
  it("rail prop change re-renders to reflect the new rail mode", async () => {
    const { el, host } = await mountWindow<HTMLElement & { rail: string; updateComplete: Promise<boolean> }>(
      "lthn-chat-window",
      { props: { rail: "filled" } },
    );
    // Sanity-check initial state before mutation.
    expect(host.querySelector("input.lthn-conversation-search")).not.toBeNull();

    el.rail = "collapsed";
    await el.updateComplete;
    // After collapse the conversation rail hides its search input
    // (the rail-mode switch removes the surface entirely).
    // We assert the element re-rendered by checking the new
    // attribute reflects (rail is reflect:true in static properties).
    expect(el.getAttribute("rail")).toBe("collapsed");
  });
});

// — Pure derivation tests for the auto-rename title helper. Lives in
// the same file as the element tests since the helper is exported
// from the same module; saves the test runner an extra file boot.

import { deriveAutoTitle, AUTO_TITLE_MAX, matchesSearch, buildTranscript, flattenForDisplay, RAIL_BUCKET_ORDER, ACTIVE_CONVERSATION_KEY, loadActiveConversation, saveActiveConversation, PINNED_CONVERSATIONS_KEY, loadPinnedConversations, savePinnedConversations, splitPinnedConversations, composerCounterText, composerCounterTone, COMPOSER_COUNTER_WARN_AT, COMPOSER_MAX_HEIGHT, userMessageHistory, withSystemPrompt, highlightMatch, findMatchPositions, findSplitTurn, parseTagInput, tagsToInput, matchesTagFilter, relativeTime } from "./chat-window";
import type { Conversation, ChatTurn } from "../types";

describe("matchesSearch — conversation rail filter", () => {
  const make = (title: string): Conversation =>
    ({ id: "x", bucket: "today", title, snippet: "", model: "" });

  it("matches a case-insensitive substring of the title", () => {
    expect(matchesSearch(make("Drafting the launch"), "launch")).toBe(true);
    expect(matchesSearch(make("Drafting the launch"), "LAUNCH")).toBe(true);
    expect(matchesSearch(make("Drafting the launch"), "DraFT")).toBe(true);
  });

  it("does NOT match when substring is absent", () => {
    expect(matchesSearch(make("Drafting the launch"), "zebra")).toBe(false);
  });

  it("empty / whitespace-only query matches every conversation", () => {
    expect(matchesSearch(make("anything"), "")).toBe(true);
    expect(matchesSearch(make("anything"), "   ")).toBe(true);
  });

  it("trims the query before matching", () => {
    expect(matchesSearch(make("important"), "  important  ")).toBe(true);
  });

  it("handles missing title field gracefully (no throw)", () => {
    expect(matchesSearch({ id: "x", bucket: "today", title: "", snippet: "", model: "" }, "q")).toBe(false);
  });
});

describe("deriveAutoTitle — pure derivation", () => {
  it("returns the input unchanged when short + clean", () => {
    expect(deriveAutoTitle("Hello world")).toBe("Hello world");
  });

  it("normalises internal whitespace (tabs, newlines, multi-space → single space)", () => {
    expect(deriveAutoTitle("Hello\t\nworld\n\nagain")).toBe("Hello world again");
  });

  it("trims leading + trailing whitespace", () => {
    expect(deriveAutoTitle("   padded   ")).toBe("padded");
  });

  it("returns empty string for whitespace-only input", () => {
    expect(deriveAutoTitle("   \n\t  ")).toBe("");
    expect(deriveAutoTitle("")).toBe("");
  });

  it("truncates strings longer than AUTO_TITLE_MAX with an ellipsis", () => {
    const input = "a".repeat(AUTO_TITLE_MAX + 10);
    const out = deriveAutoTitle(input);
    expect(out.length).toBe(AUTO_TITLE_MAX);
    expect(out.endsWith("…")).toBe(true);
  });

  it("keeps strings exactly AUTO_TITLE_MAX chars long intact (no ellipsis)", () => {
    const input = "a".repeat(AUTO_TITLE_MAX);
    expect(deriveAutoTitle(input)).toBe(input);
  });

  it("does not leave a trailing space before the ellipsis when the cut lands on a space", () => {
    // Construct an input where the slice boundary lands on a space —
    // trimEnd should clip it.
    const input = "word ".repeat(20).trim();
    const out = deriveAutoTitle(input);
    expect(out.endsWith(" …")).toBe(false);
    expect(out.endsWith("…")).toBe(true);
  });
});

describe("buildTranscript — clipboard transcript renderer", () => {
  const turn = (role: ChatTurn["role"], text: string): ChatTurn => ({ role, text });

  it("returns empty string when turns is null", () => {
    expect(buildTranscript(null, "Gemma")).toBe("");
  });

  it("returns empty string when turns is an empty array", () => {
    expect(buildTranscript([], "Gemma")).toBe("");
  });

  it("labels user turns 'You' and model turns by the model name", () => {
    const out = buildTranscript([
      turn("you", "ping"),
      turn("model", "pong"),
    ], "Gemma");
    expect(out).toBe("You: ping\n\nGemma: pong");
  });

  it("falls back to 'Assistant' when the model name is empty", () => {
    const out = buildTranscript([turn("model", "hello")], "");
    expect(out).toBe("Assistant: hello");
  });

  it("preserves multi-line message bodies inline (no escaping)", () => {
    const out = buildTranscript([turn("model", "line one\nline two")], "X");
    expect(out).toBe("X: line one\nline two");
  });

  it("separates consecutive turns with a single blank line", () => {
    const out = buildTranscript([
      turn("you", "a"),
      turn("model", "b"),
      turn("you", "c"),
    ], "X");
    expect(out.split("\n\n")).toEqual(["You: a", "X: b", "You: c"]);
  });
});

describe("flattenForDisplay — bucket-ordered flat list", () => {
  const c = (id: string, bucket: Conversation["bucket"]): Conversation =>
    ({ id, bucket, title: id, snippet: "", model: "" });

  it("returns input ordered by RAIL_BUCKET_ORDER (today, yesterday, week)", () => {
    const input = [c("a", "week"), c("b", "today"), c("c", "yesterday"), c("d", "today")];
    const out = flattenForDisplay(input).map(x => x.id);
    expect(out).toEqual(["b", "d", "c", "a"]);
  });

  it("preserves source order within each bucket", () => {
    const input = [c("z", "today"), c("a", "today"), c("m", "today")];
    expect(flattenForDisplay(input).map(x => x.id)).toEqual(["z", "a", "m"]);
  });

  it("skips empty buckets without padding", () => {
    const input = [c("a", "week")];
    expect(flattenForDisplay(input).map(x => x.id)).toEqual(["a"]);
  });

  it("returns empty array for empty input", () => {
    expect(flattenForDisplay([])).toEqual([]);
  });

  it("RAIL_BUCKET_ORDER is the documented order", () => {
    expect(RAIL_BUCKET_ORDER).toEqual(["today", "yesterday", "week"]);
  });
});

describe("active conversation persistence", () => {
  beforeEach(() => {
    localStorage.removeItem(ACTIVE_CONVERSATION_KEY);
  });

  it("saveActiveConversation writes the id to localStorage", () => {
    saveActiveConversation("abc-123");
    expect(localStorage.getItem(ACTIVE_CONVERSATION_KEY)).toBe("abc-123");
  });

  it("saveActiveConversation(null) clears the key", () => {
    localStorage.setItem(ACTIVE_CONVERSATION_KEY, "abc-123");
    saveActiveConversation(null);
    expect(localStorage.getItem(ACTIVE_CONVERSATION_KEY)).toBeNull();
  });

  it("loadActiveConversation reads the stored id", () => {
    localStorage.setItem(ACTIVE_CONVERSATION_KEY, "xyz-456");
    expect(loadActiveConversation()).toBe("xyz-456");
  });

  it("loadActiveConversation returns null when key is missing", () => {
    expect(loadActiveConversation()).toBeNull();
  });

  it("uses the documented localStorage key", () => {
    expect(ACTIVE_CONVERSATION_KEY).toBe("lthn.chat.active-conversation");
  });
});

describe("conversation pin persistence", () => {
  beforeEach(() => {
    localStorage.removeItem(PINNED_CONVERSATIONS_KEY);
  });

  it("loadPinnedConversations returns empty Set when key missing", () => {
    const s = loadPinnedConversations();
    expect(s).toBeInstanceOf(Set);
    expect(s.size).toBe(0);
  });

  it("loadPinnedConversations hydrates from JSON array", () => {
    localStorage.setItem(PINNED_CONVERSATIONS_KEY, JSON.stringify(["a", "b"]));
    const s = loadPinnedConversations();
    expect(s.has("a")).toBe(true);
    expect(s.has("b")).toBe(true);
    expect(s.has("c")).toBe(false);
  });

  it("loadPinnedConversations falls through on malformed JSON", () => {
    localStorage.setItem(PINNED_CONVERSATIONS_KEY, "not-valid-json{");
    expect(loadPinnedConversations().size).toBe(0);
  });

  it("savePinnedConversations writes a sorted JSON array", () => {
    savePinnedConversations(new Set(["c", "a", "b"]));
    expect(localStorage.getItem(PINNED_CONVERSATIONS_KEY)).toBe(JSON.stringify(["a", "b", "c"]));
  });

  it("key constant matches the documented path", () => {
    expect(PINNED_CONVERSATIONS_KEY).toBe("lthn.chat.pinned");
  });
});

describe("splitPinnedConversations — partition by pin state", () => {
  const c = (id: string, bucket: Conversation["bucket"]): Conversation =>
    ({ id, bucket, title: id, snippet: "", model: "" });

  it("returns both buckets empty for empty input", () => {
    expect(splitPinnedConversations([], new Set())).toEqual({ pinned: [], rest: [] });
  });

  it("partitions by membership in the pin set", () => {
    const list = [c("a", "today"), c("b", "today"), c("c", "yesterday")];
    const out = splitPinnedConversations(list, new Set(["b"]));
    expect(out.pinned.map(x => x.id)).toEqual(["b"]);
    expect(out.rest.map(x => x.id)).toEqual(["a", "c"]);
  });

  it("preserves source order within each bucket", () => {
    const list = [c("a", "today"), c("b", "today"), c("c", "today"), c("d", "today")];
    const out = splitPinnedConversations(list, new Set(["c", "a"]));
    expect(out.pinned.map(x => x.id)).toEqual(["a", "c"]);
    expect(out.rest.map(x => x.id)).toEqual(["b", "d"]);
  });

  it("pinned id not in list yields empty pinned bucket", () => {
    const list = [c("a", "today")];
    const out = splitPinnedConversations(list, new Set(["ghost"]));
    expect(out.pinned).toEqual([]);
    expect(out.rest.map(x => x.id)).toEqual(["a"]);
  });
});

describe("rail search — content match union", () => {
  type ChatWindowEl = HTMLElement & {
    conversations: Conversation[];
    searchQuery: string;
    contentMatchedIds: Set<string>;
    _scheduleContentSearch: () => void;
    _runContentSearch: (q: string) => Promise<void>;
    updateComplete: Promise<boolean>;
  };

  const c = (id: string, title: string): Conversation =>
    ({ id, title, snippet: "", bucket: "today", model: "" });

  it("contentMatchedIds defaults to an empty Set", async () => {
    const { el } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    expect(el.contentMatchedIds).toBeInstanceOf(Set);
    expect(el.contentMatchedIds.size).toBe(0);
  });

  it("rail renders conversations matched by content even when title doesn't match", async () => {
    const { el, host } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el.conversations = [
      c("alpha", "meeting notes"),
      c("bravo", "weekly review"),
    ];
    el.searchQuery = "regex";
    // Content-match path: bravo's title doesn't contain "regex" but its
    // messages do — backend marks it as a content hit.
    el.contentMatchedIds = new Set(["bravo"]);
    await el.updateComplete;

    expect(host.querySelector('[data-conversation-id="bravo"]'), "content-only match renders").not.toBeNull();
    expect(host.querySelector('[data-conversation-id="alpha"]'), "non-match stays hidden").toBeNull();
  });

  it("title match still wins independently of content matches", async () => {
    const { el, host } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el.conversations = [c("alpha", "regex pitfalls"), c("bravo", "off-topic")];
    el.searchQuery = "regex";
    el.contentMatchedIds = new Set(); // No content matches yet (debounce pending)
    await el.updateComplete;

    expect(host.querySelector('[data-conversation-id="alpha"]'), "title match renders without waiting for content search").not.toBeNull();
    expect(host.querySelector('[data-conversation-id="bravo"]')).toBeNull();
  });

  it("title + content matches union without duplication", async () => {
    const { el, host } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el.conversations = [
      c("alpha", "regex deep dive"),  // title match
      c("bravo", "the chat"),         // content match
      c("charlie", "weekend plans"),  // no match
    ];
    el.searchQuery = "regex";
    el.contentMatchedIds = new Set(["alpha", "bravo"]); // alpha matches both, bravo content-only
    await el.updateComplete;

    expect(host.querySelector('[data-conversation-id="alpha"]')).not.toBeNull();
    expect(host.querySelector('[data-conversation-id="bravo"]')).not.toBeNull();
    expect(host.querySelector('[data-conversation-id="charlie"]')).toBeNull();
    // alpha appears once even though it's in both sets.
    const alphaRows = host.querySelectorAll('[data-conversation-id="alpha"]');
    expect(alphaRows.length).toBe(1);
  });

  it("clearing the query drops the content match set immediately", async () => {
    const { el } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el.searchQuery = "regex";
    el.contentMatchedIds = new Set(["a", "b", "c"]);
    el.searchQuery = "";
    el._scheduleContentSearch();
    expect(el.contentMatchedIds.size, "empty query → matches dropped instantly").toBe(0);
  });

  it("_runContentSearch('') clears matches and skips the service call", async () => {
    const { el } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el.contentMatchedIds = new Set(["x", "y"]);
    await el._runContentSearch("");
    expect(el.contentMatchedIds.size).toBe(0);
  });
});

// Mantis #1571 / Cerberus #12 F4 — surface SearchResult.truncated to
// the user. The backend SearchResult contract carries truncated:true
// when MaxSearchScan (5000) was hit before every session was scanned;
// the rail must badge that state instead of presenting the partial
// list as exhaustive.
//
// Notes on test shape: _runContentSearch dynamically imports the
// @desktop/sessions/wailsservice ES module at call time. ES module
// exports are getter-backed (non-writable), so we can't swap `Search`
// on the namespace at runtime — and a top-level vi.mock would also
// stub svc.List / svc.Create that _reloadRail uses, breaking the
// mount. The unit we care about is "searchTruncated flag → DOM" so
// we drive that boundary directly: set el.searchTruncated + assert
// the banner renders, plus a unit test on _runContentSearch's empty
// path resetting the flag (already covered above) and on the schedule
// helper clearing it.
describe("rail search — truncated banner (Mantis #1571)", () => {
  type ChatWindowEl = HTMLElement & {
    conversations: Conversation[];
    searchQuery: string;
    searchTruncated: boolean;
    contentMatchedIds: Set<string>;
    _runContentSearch: (q: string) => Promise<void>;
    _scheduleContentSearch: () => void;
    updateComplete: Promise<boolean>;
  };

  const c = (id: string, title: string): Conversation =>
    ({ id, title, snippet: "", bucket: "today", model: "" });

  // TestChatWindow_SearchTruncatedBadge_ShownWhenFlagged_Good
  it("renders the narrow-query banner when searchTruncated is true and a query is active", async () => {
    const { el, host } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    // Mirror the state shape _runContentSearch produces on a truncated
    // backend response: a populated content-match set + truncated:true.
    el.conversations = [c("alpha", "regex deep dive")];
    el.searchQuery = "regex";
    el.contentMatchedIds = new Set(Array.from({ length: 50 }, (_, i) => "s" + i));
    el.searchTruncated = true;
    await el.updateComplete;

    const banner = host.querySelector(".lthn-conversation-search-truncated");
    expect(banner, "truncated banner present in DOM").not.toBeNull();
    expect(banner?.textContent || "", "carries narrow-query guidance copy").toContain("narrow your query");
    // Accessibility: status role + polite live region so screen readers
    // announce the hint without interrupting whatever the user is doing.
    expect(banner?.getAttribute("role")).toBe("status");
    expect(banner?.getAttribute("aria-live")).toBe("polite");
  });

  // TestChatWindow_SearchTruncatedBadge_HiddenWhenNotFlagged_Good
  it("does not render the banner when searchTruncated is false (exhaustive scan)", async () => {
    const { el, host } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el.conversations = [c("alpha", "regex deep dive")];
    el.searchQuery = "regex";
    el.contentMatchedIds = new Set(["alpha"]);
    el.searchTruncated = false;
    await el.updateComplete;

    const banner = host.querySelector(".lthn-conversation-search-truncated");
    expect(banner, "no banner when scan was exhaustive").toBeNull();
  });

  it("does not render the banner when the query is empty even if the flag is stale", async () => {
    // Safety net: the schedule + run helpers both clear searchTruncated
    // on empty-query, but the render guard ALSO requires hasQuery so a
    // stale flag never paints a banner under a blank input.
    const { el, host } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el.searchQuery = "";
    el.searchTruncated = true;
    await el.updateComplete;

    const banner = host.querySelector(".lthn-conversation-search-truncated");
    expect(banner).toBeNull();
  });

  it("_runContentSearch('') resets searchTruncated to false", async () => {
    const { el } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el.searchTruncated = true;
    await el._runContentSearch("");
    expect(el.searchTruncated).toBe(false);
  });

  it("_scheduleContentSearch with an empty query clears a stale truncated flag", async () => {
    const { el } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el.searchTruncated = true;
    el.searchQuery = "";
    el._scheduleContentSearch();
    expect(el.searchTruncated, "empty query → truncated banner dropped instantly").toBe(false);
  });
});

describe("/export-all slash command", () => {
  type ChatWindowEl = HTMLElement & {
    _slashMenuController: { open: boolean; close: () => boolean; toggle: () => void };
    _slashExportAll: () => Promise<void>;
    _exportAllTo: (dir: string) => Promise<number>;
    updateComplete: Promise<boolean>;
  };

  it("/export-all slash entry is present in the menu", async () => {
    const { el, host } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el._slashMenuController.toggle();
    await el.updateComplete;
    const item = host.querySelector(".lthn-chat-slash-export-all");
    expect(item, "/export-all item rendered").not.toBeNull();
    expect(item?.textContent || "").toContain("/export-all");
  });

  it("_exportAllTo('') is a silent no-op returning 0", async () => {
    const { el } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    let count = -1;
    let threw: unknown = null;
    try { count = await el._exportAllTo(""); }
    catch (e) { threw = e; }
    expect(threw).toBeNull();
    expect(count).toBe(0);
  });

  it("cheat sheet lists /export-all", async () => {
    const { el, host } = await mountWindow<HTMLElement & { _helpOverlayController: { open: boolean; close: () => boolean; show: () => void; toggle: () => void }; updateComplete: Promise<boolean> }>("lthn-chat-window");
    el._helpOverlayController.show();
    await el.updateComplete;
    const text = host.textContent || "";
    expect(text).toContain("/export-all");
    expect(text).toContain("Back up every conversation");
  });
});

describe("/help slash command", () => {
  type ChatWindowEl = HTMLElement & {
    _slashMenuController: { open: boolean; close: () => boolean; toggle: () => void };
    _helpOverlayController: { open: boolean; close: () => boolean; show: () => void; toggle: () => void };
    _contextMenuController: { for: { id: string; x: number; y: number } | null; close: () => boolean };
    _slashHelp: () => void;
    updateComplete: Promise<boolean>;
  };

  it("/help slash item opens the help overlay + closes the slash menu", async () => {
    const { el, host } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el._slashMenuController.toggle();
    await el.updateComplete;

    const help = host.querySelector<HTMLElement>(".lthn-chat-slash-help");
    expect(help, "help slash entry rendered").not.toBeNull();
    help!.click();
    await el.updateComplete;

    expect(el._slashMenuController.open, "slash menu closes on /help click").toBe(false);
    expect(el._helpOverlayController.open, "help overlay opens").toBe(true);
    expect(host.querySelector(".lthn-chat-help"), "help dialog rendered").not.toBeNull();
    host.remove();
  });

  it("help overlay lists keyboard shortcuts + slash commands", async () => {
    const { el, host } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el._helpOverlayController.show();
    await el.updateComplete;

    const text = host.textContent || "";
    // Section headings (canonical strings the user should see).
    expect(text).toContain("Conversations");
    expect(text).toContain("Composer");
    expect(text).toContain("Slash commands");
    // A representative shortcut + slash command from each section.
    expect(text).toContain("⌘N");
    expect(text).toContain("F2");
    expect(text).toContain("⌘↵");
    expect(text).toContain("/export");
    host.remove();
  });

  it("close button dismisses the overlay", async () => {
    const { el, host } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el._helpOverlayController.show();
    await el.updateComplete;

    const close = host.querySelector<HTMLButtonElement>(".lthn-chat-help-close");
    expect(close, "close button present").not.toBeNull();
    close!.click();
    await el.updateComplete;

    expect(el._helpOverlayController.open, "close button → overlay dismissed").toBe(false);
    host.remove();
  });

  it("clicking the backdrop dismisses the overlay", async () => {
    const { el, host } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el._helpOverlayController.show();
    await el.updateComplete;

    const backdrop = host.querySelector<HTMLElement>(".lthn-chat-help-backdrop");
    expect(backdrop, "backdrop present").not.toBeNull();
    backdrop!.click();
    await el.updateComplete;

    expect(el._helpOverlayController.open, "backdrop click → overlay dismissed").toBe(false);
    host.remove();
  });

  it("clicking inside the dialog does NOT dismiss it (stop-propagation)", async () => {
    const { el, host } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el._helpOverlayController.show();
    await el.updateComplete;

    const dialog = host.querySelector<HTMLElement>(".lthn-chat-help");
    dialog!.click();
    await el.updateComplete;

    expect(el._helpOverlayController.open, "inner click stays open").toBe(true);
    host.remove();
  });

  it("Escape priority: context menu first, help overlay second, slash third", async () => {
    const { el, host } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el._contextMenuController.openAt("x", 0, 0);
    el._helpOverlayController.show();
    el._slashMenuController.toggle();
    await el.updateComplete;

    // First Escape closes context menu, leaves help + slash open.
    window.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
    await el.updateComplete;
    expect(el._contextMenuController.for).toBeNull();
    expect(el._helpOverlayController.open).toBe(true);
    expect(el._slashMenuController.open).toBe(true);

    // Second Escape closes help overlay, leaves slash open.
    window.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
    await el.updateComplete;
    expect(el._helpOverlayController.open).toBe(false);
    expect(el._slashMenuController.open).toBe(true);

    // Third Escape closes slash menu.
    window.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
    await el.updateComplete;
    expect(el._slashMenuController.open).toBe(false);
    host.remove();
  });
});

describe("transcript scroll-to-bottom", () => {
  type ChatWindowEl = HTMLElement & {
    state: string;
    activeConversationId: string | null;
    liveTurns: ChatTurn[] | null;
    atBottom: boolean;
    updateComplete: Promise<boolean>;
    _recomputeAtBottom: () => void;
    _scrollToBottom: (behavior?: ScrollBehavior) => void;
  };

  /** happy-dom doesn't compute layout — stub the three scroll metrics
   *  directly on the element so atBottom math is deterministic. */
  function stubScroll(el: HTMLElement, sh: number, ch: number, st: number) {
    Object.defineProperty(el, "scrollHeight", { value: sh, configurable: true });
    Object.defineProperty(el, "clientHeight", { value: ch, configurable: true });
    el.scrollTop = st;
  }

  it("scrolled-up state sets atBottom=false + renders the Latest button", async () => {
    const { el, host } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el.state = "multi-turn";
    el.activeConversationId = "scroll-test";
    el.liveTurns = [
      { role: "you", text: "hello" },
      { role: "assistant", text: "hi back" },
      { role: "you", text: "and again" },
    ];
    await el.updateComplete;

    const transcript = host.querySelector<HTMLElement>(".lthn-chat-transcript");
    expect(transcript, "transcript scroller rendered").not.toBeNull();
    // scrollTop=0, scrollHeight=2000, clientHeight=400 → distance=1600
    stubScroll(transcript!, 2000, 400, 0);
    transcript!.dispatchEvent(new Event("scroll", { bubbles: true }));
    await el.updateComplete;

    expect(el.atBottom, "scrolled to top → atBottom false").toBe(false);
    expect(host.querySelector(".lthn-chat-scroll-bottom"), "Latest button visible").not.toBeNull();
    host.remove();
  });

  it("scrolled-to-bottom state keeps atBottom=true + hides the Latest button", async () => {
    const { el, host } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el.state = "multi-turn";
    el.activeConversationId = "scroll-test";
    el.liveTurns = [
      { role: "you", text: "hi" },
      { role: "assistant", text: "yo" },
    ];
    await el.updateComplete;

    const transcript = host.querySelector<HTMLElement>(".lthn-chat-transcript")!;
    // distance = 2000 - 1610 - 400 = -10 (clamped to <=16 threshold → at bottom)
    stubScroll(transcript, 2000, 400, 1610);
    transcript.dispatchEvent(new Event("scroll", { bubbles: true }));
    await el.updateComplete;

    expect(el.atBottom, "near-bottom (within 16px) is at-bottom").toBe(true);
    expect(host.querySelector(".lthn-chat-scroll-bottom"), "Latest button hidden").toBeNull();
    host.remove();
  });

  it("clicking the Latest button restores atBottom=true and snaps scrollTop", async () => {
    const { el, host } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el.state = "multi-turn";
    el.activeConversationId = "scroll-test";
    el.liveTurns = [
      { role: "you", text: "1" },
      { role: "assistant", text: "2" },
      { role: "you", text: "3" },
    ];
    await el.updateComplete;

    const transcript = host.querySelector<HTMLElement>(".lthn-chat-transcript")!;
    stubScroll(transcript, 2000, 400, 0);
    transcript.dispatchEvent(new Event("scroll", { bubbles: true }));
    await el.updateComplete;

    const btn = host.querySelector<HTMLButtonElement>(".lthn-chat-scroll-bottom");
    expect(btn, "button rendered when scrolled up").not.toBeNull();
    btn!.click();
    await el.updateComplete;

    expect(el.atBottom).toBe(true);
    expect(transcript.scrollTop, "scrollTop snapped to scrollHeight").toBe(2000);
    expect(host.querySelector(".lthn-chat-scroll-bottom"), "button gone after jump").toBeNull();
    host.remove();
  });

  it("new turn arriving while atBottom triggers an auto-scroll", async () => {
    const { el, host } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el.state = "multi-turn";
    el.activeConversationId = "scroll-test";
    el.liveTurns = [{ role: "you", text: "first" }];
    await el.updateComplete;

    const transcript = host.querySelector<HTMLElement>(".lthn-chat-transcript")!;
    // Start at bottom.
    stubScroll(transcript, 500, 400, 100);
    transcript.dispatchEvent(new Event("scroll", { bubbles: true }));
    await el.updateComplete;
    expect(el.atBottom).toBe(true);

    // Grow scroll height (new content appended), then add a new turn.
    stubScroll(transcript, 1000, 400, 100);
    el.liveTurns = [
      { role: "you", text: "first" },
      { role: "assistant", text: "second" },
    ];
    await el.updateComplete;
    // queueMicrotask + scrollTo run after updated() — yield once more.
    await Promise.resolve();
    await el.updateComplete;

    expect(transcript.scrollTop, "scrollTop snapped past scrollHeight").toBe(1000);
    expect(el.atBottom).toBe(true);
    host.remove();
  });

  it("new turn arriving while scrolled up does NOT auto-scroll", async () => {
    const { el, host } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el.state = "multi-turn";
    el.activeConversationId = "scroll-test";
    el.liveTurns = [{ role: "you", text: "older" }];
    await el.updateComplete;

    const transcript = host.querySelector<HTMLElement>(".lthn-chat-transcript")!;
    // Scroll up first.
    stubScroll(transcript, 2000, 400, 0);
    transcript.dispatchEvent(new Event("scroll", { bubbles: true }));
    await el.updateComplete;
    expect(el.atBottom).toBe(false);

    // New turn arrives — should NOT yank the viewport.
    el.liveTurns = [
      { role: "you", text: "older" },
      { role: "assistant", text: "fresh" },
    ];
    await el.updateComplete;
    await Promise.resolve();
    await el.updateComplete;

    expect(transcript.scrollTop, "scrollTop preserved").toBe(0);
    expect(el.atBottom, "still scrolled up — button stays").toBe(false);
    host.remove();
  });
});

describe("conversation context menu", () => {
  type ChatWindowEl = HTMLElement & {
    conversations: Conversation[];
    activeConversationId: string | null;
    renamingId: string | null;
    pinnedConversations: Set<string>;
    _contextMenuController: { for: { id: string; x: number; y: number } | null; close: () => boolean };
    _slashMenuController: { open: boolean; close: () => boolean; toggle: () => void };
    updateComplete: Promise<boolean>;
  };

  const seed: Conversation = {
    id: "ctx-target",
    title: "Conversation A",
    snippet: "",
    bucket: "today",
    model: "test-model",
  };

  it("right-click on a rail row opens the context menu + activates the row", async () => {
    const { el, host } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el.conversations = [seed];
    el.activeConversationId = null;
    await el.updateComplete;

    const row = host.querySelector<HTMLElement>('[data-conversation-id="ctx-target"]')!;
    expect(row, "rail row rendered").not.toBeNull();
    const ev = new MouseEvent("contextmenu", { bubbles: true, cancelable: true, clientX: 120, clientY: 80, button: 2 });
    row.dispatchEvent(ev);
    await el.updateComplete;

    expect(el._contextMenuController.for).not.toBeNull();
    expect(el._contextMenuController.for!.id).toBe("ctx-target");
    expect(el._contextMenuController.for!.x).toBe(120);
    expect(el._contextMenuController.for!.y).toBe(80);
    expect(el.activeConversationId, "right-click also focuses the row").toBe("ctx-target");

    const menu = host.querySelector(".lthn-conversation-context-menu");
    expect(menu, "menu rendered").not.toBeNull();
    expect(host.querySelector(".lthn-conversation-context-rename"), "Rename item").not.toBeNull();
    expect(host.querySelector(".lthn-conversation-context-pin"), "Pin item").not.toBeNull();
    expect(host.querySelector(".lthn-conversation-context-export"), "Export item").not.toBeNull();
    expect(host.querySelector(".lthn-conversation-context-delete"), "Delete item").not.toBeNull();
    host.remove();
  });

  it("Rename item enters rename mode + closes the menu", async () => {
    const { el, host } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el.conversations = [seed];
    el._contextMenuController.openAt("ctx-target", 0, 0);
    el.activeConversationId = "ctx-target";
    el.renamingId = null;
    await el.updateComplete;

    const renameBtn = host.querySelector<HTMLButtonElement>(".lthn-conversation-context-rename")!;
    renameBtn.click();
    await el.updateComplete;

    expect(el.renamingId).toBe("ctx-target");
    expect(el._contextMenuController.for, "menu closes after action").toBeNull();
    host.remove();
  });

  it("Pin item toggles the pin set + closes the menu", async () => {
    const { el, host } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el.conversations = [seed];
    el.pinnedConversations = new Set();
    el._contextMenuController.openAt("ctx-target", 0, 0);
    el.activeConversationId = "ctx-target";
    await el.updateComplete;

    host.querySelector<HTMLButtonElement>(".lthn-conversation-context-pin")!.click();
    await el.updateComplete;

    expect(el.pinnedConversations.has("ctx-target"), "pin added").toBe(true);
    expect(el._contextMenuController.for, "menu closes").toBeNull();

    // Re-opening the menu shows "Unpin" (the label flips based on pin state).
    el._contextMenuController.openAt("ctx-target", 0, 0);
    await el.updateComplete;
    const pinItem = host.querySelector(".lthn-conversation-context-pin");
    expect(pinItem?.textContent).toContain("Unpin");
    host.remove();
  });

  it("Escape closes the context menu (priority over slash menu)", async () => {
    const { el, host } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el._contextMenuController.openAt("ctx-target", 0, 0);
    el._slashMenuController.toggle();
    await el.updateComplete;

    window.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
    await el.updateComplete;

    expect(el._contextMenuController.for, "Escape closes context menu first").toBeNull();
    expect(el._slashMenuController.open, "slash menu stays open this round — context menu took the key").toBe(true);

    // Second Escape should now reach the slash menu.
    window.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
    await el.updateComplete;
    expect(el._slashMenuController.open, "second Escape closes slash menu").toBe(false);
    host.remove();
  });

  it("Duplicate item is present in the context menu", async () => {
    const { el, host } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el.conversations = [seed];
    el._contextMenuController.openAt("ctx-target", 0, 0);
    el.activeConversationId = "ctx-target";
    await el.updateComplete;

    const dup = host.querySelector(".lthn-conversation-context-duplicate");
    expect(dup, "Duplicate item rendered").not.toBeNull();
    expect(dup?.textContent).toContain("Duplicate");
    host.remove();
  });

  it("clicking outside the menu closes it", async () => {
    const { el, host } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el._contextMenuController.openAt("ctx-target", 0, 0);
    await el.updateComplete;

    // Click on document.body (a node OUTSIDE the menu's composedPath).
    document.body.dispatchEvent(new MouseEvent("click", { bubbles: true, composed: true }));
    await el.updateComplete;

    expect(el._contextMenuController.for, "outside-click dismisses menu").toBeNull();
    host.remove();
  });

  it("_duplicateConversation('') is a silent no-op (no throw)", async () => {
    const { el } = await mountWindow<ChatWindowEl & {
      _duplicateConversation: (id: string) => Promise<void>;
    }>("lthn-chat-window");
    let threw: unknown = null;
    try { await el._duplicateConversation(""); }
    catch (e) { threw = e; }
    expect(threw, "empty id must not throw — early return").toBeNull();
  });
});

describe("? opens help cheat sheet", () => {
  type ChatWindowEl = HTMLElement & {
    _helpOverlayController: { open: boolean; close: () => boolean; show: () => void; toggle: () => void };
    updateComplete: Promise<boolean>;
  };

  it("bare ? opens the help overlay", async () => {
    const { el } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    expect(el._helpOverlayController.open).toBe(false);
    window.dispatchEvent(new KeyboardEvent("keydown", { key: "?", bubbles: true }));
    await el.updateComplete;
    expect(el._helpOverlayController.open).toBe(true);
  });

  it("second ? closes the help overlay (toggle)", async () => {
    const { el } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el._helpOverlayController.show();
    await el.updateComplete;
    window.dispatchEvent(new KeyboardEvent("keydown", { key: "?", bubbles: true }));
    await el.updateComplete;
    expect(el._helpOverlayController.open).toBe(false);
  });

  it("? does NOT open help when focus is in an input", async () => {
    const { el, host } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el._helpOverlayController.close();
    await el.updateComplete;

    const input = host.querySelector<HTMLInputElement>("input.lthn-conversation-search")!;
    expect(input, "rail search input present").not.toBeNull();
    input.focus();
    input.dispatchEvent(new KeyboardEvent("keydown", { key: "?", bubbles: true }));
    await el.updateComplete;
    expect(el._helpOverlayController.open, "? in input must not trigger help").toBe(false);
  });

  it("cheat sheet lists ? under Composer", async () => {
    const { el, host } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el._helpOverlayController.show();
    await el.updateComplete;

    const text = host.textContent || "";
    expect(text).toContain("Show this cheat sheet");
  });
});

describe("⌘F find in transcript", () => {
  type ChatWindowEl = HTMLElement & {
    _findController: { open: boolean; query: string; cursor: number; setQuery: (q: string) => void; step: (d: number) => void; matchCount: () => number; scrollActiveIntoView: () => void; tryHandleEscape: () => boolean; toggle: () => void };
    liveTurns: ChatTurn[] | null;
    activeConversationId: string | null;
    state: string;
    _slashMenuController: { open: boolean; close: () => boolean; toggle: () => void };
    _helpOverlayController: { open: boolean; close: () => boolean; show: () => void; toggle: () => void };
    _contextMenuController: { for: { id: string; x: number; y: number } | null; close: () => boolean };
    updateComplete: Promise<boolean>;
  };

  it("⌘F opens the find bar", async () => {
    const { el, host } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    expect(el._findController.open).toBe(false);

    window.dispatchEvent(new KeyboardEvent("keydown", { key: "f", metaKey: true, bubbles: true }));
    await el.updateComplete;

    expect(el._findController.open).toBe(true);
    expect(host.querySelector(".lthn-chat-find"), "find bar rendered").not.toBeNull();
    expect(host.querySelector(".lthn-chat-find-input"), "find input rendered").not.toBeNull();
    host.remove();
  });

  it("Ctrl+F triggers the same toggle", async () => {
    const { el, host } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    window.dispatchEvent(new KeyboardEvent("keydown", { key: "f", ctrlKey: true, bubbles: true }));
    await el.updateComplete;
    expect(el._findController.open).toBe(true);
    host.remove();
  });

  it("⌘F again closes the find bar + clears the query", async () => {
    const { el, host } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el._findController.toggle();
    el._findController.setQuery("regex");
    await el.updateComplete;

    window.dispatchEvent(new KeyboardEvent("keydown", { key: "f", metaKey: true, bubbles: true }));
    await el.updateComplete;

    expect(el._findController.open).toBe(false);
    expect(el._findController.query).toBe("");
    host.remove();
  });

  it("Escape closes the find bar + clears the query", async () => {
    const { el, host } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el._findController.toggle();
    el._findController.setQuery("anchor");
    await el.updateComplete;

    window.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
    await el.updateComplete;

    expect(el._findController.open).toBe(false);
    expect(el._findController.query).toBe("");
    host.remove();
  });

  it("close button dismisses the bar", async () => {
    const { el, host } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el._findController.toggle();
    el._findController.setQuery("x");
    await el.updateComplete;

    const close = host.querySelector<HTMLButtonElement>(".lthn-chat-find-close");
    expect(close).not.toBeNull();
    close!.click();
    await el.updateComplete;

    expect(el._findController.open).toBe(false);
    expect(el._findController.query).toBe("");
    host.remove();
  });

  it("highlights matched substrings within turn text when find is active", async () => {
    const { el, host } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el.state = "multi-turn";
    el.activeConversationId = "find-test";
    el.liveTurns = [
      { role: "you", text: "alpha bravo charlie" },
      { role: "assistant", text: "bravo zulu delta" },
    ];
    el._findController.toggle();
    el._findController.setQuery("bravo");
    await el.updateComplete;

    const matches = host.querySelectorAll(".lthn-chat-find-match");
    expect(matches.length, "every bravo occurrence in transcript gets a highlight span").toBeGreaterThanOrEqual(2);
    expect(Array.from(matches).every(m => m.textContent === "bravo")).toBe(true);
    host.remove();
  });

  it("counter renders 'no matches' when the query has zero hits", async () => {
    const { el, host } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el._findController.toggle();
    el._findController.setQuery("ghost-needle");
    el.liveTurns = [{ role: "you", text: "alpha bravo" }];
    await el.updateComplete;

    const count = host.querySelector(".lthn-chat-find-count");
    expect(count?.textContent?.trim()).toBe("no matches");
    host.remove();
  });

  it("Escape priority: context → help → find → slash", async () => {
    const { el, host } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el._contextMenuController.openAt("x", 0, 0);
    el._helpOverlayController.show();
    el._findController.toggle();
    el._findController.setQuery("x");
    el._slashMenuController.toggle();
    await el.updateComplete;

    window.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
    await el.updateComplete;
    expect(el._contextMenuController.for).toBeNull();
    expect(el._helpOverlayController.open).toBe(true);
    expect(el._findController.open).toBe(true);
    expect(el._slashMenuController.open).toBe(true);

    window.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
    await el.updateComplete;
    expect(el._helpOverlayController.open).toBe(false);
    expect(el._findController.open).toBe(true);

    window.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
    await el.updateComplete;
    expect(el._findController.open).toBe(false);
    expect(el._slashMenuController.open).toBe(true);

    window.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
    await el.updateComplete;
    expect(el._slashMenuController.open).toBe(false);
    host.remove();
  });
});

describe("findMatchPositions — flat positions across turns", () => {
  it("returns [] for empty query / no turns", () => {
    expect(findMatchPositions(null, "x")).toEqual([]);
    expect(findMatchPositions([{ role: "you", text: "hi" }], "")).toEqual([]);
    expect(findMatchPositions([{ role: "you", text: "hi" }], "   ")).toEqual([]);
  });

  it("walks every match across turns in order", () => {
    const out = findMatchPositions([
      { role: "you",       text: "regex regex" },
      { role: "assistant", text: "no" },
      { role: "you",       text: "more regex here" },
    ], "regex");
    expect(out).toEqual([
      { turnIndex: 0, offset: 0 },
      { turnIndex: 0, offset: 6 },
      { turnIndex: 2, offset: 5 },
    ]);
  });

  it("is case-insensitive", () => {
    const out = findMatchPositions([{ role: "you", text: "Hello WORLD hello" }], "hello");
    expect(out).toHaveLength(2);
    expect(out[0]).toEqual({ turnIndex: 0, offset: 0 });
    expect(out[1]).toEqual({ turnIndex: 0, offset: 12 });
  });
});

describe("findSplitTurn — per-turn split with active flag", () => {
  it("returns [] for empty text", () => {
    expect(findSplitTurn("", "x", -1)).toEqual([]);
  });

  it("returns a single non-match segment for blank query", () => {
    expect(findSplitTurn("hello", "", -1)).toEqual([{ text: "hello", match: false, active: false }]);
  });

  it("flags the segment whose offset matches activeOffset", () => {
    const segs = findSplitTurn("aXaXa", "X", 3);
    expect(segs).toEqual([
      { text: "a", match: false, active: false },
      { text: "X", match: true,  active: false },
      { text: "a", match: false, active: false },
      { text: "X", match: true,  active: true  },
      { text: "a", match: false, active: false },
    ]);
  });

  it("active=false on every match when activeOffset is -1", () => {
    const segs = findSplitTurn("aaa", "a", -1);
    expect(segs.every(s => !s.active)).toBe(true);
  });
});

describe("chat-window — find cursor + render", () => {
  type ChatWindowEl = HTMLElement & {
    _findController: { open: boolean; query: string; cursor: number; setQuery: (q: string) => void; step: (d: number) => void; matchCount: () => number; scrollActiveIntoView: () => void; tryHandleEscape: () => boolean; toggle: () => void };
    liveTurns: ChatTurn[] | null;
    activeConversationId: string | null;
    state: string;
    updateComplete: Promise<boolean>;
  };

  it("counter renders 'X of N' when matches exist", async () => {
    const { el, host } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el._findController.toggle();
    el._findController.setQuery("regex");
    el.liveTurns = [
      { role: "you", text: "regex regex regex" },
      { role: "assistant", text: "regex matters" },
    ];
    el._findController.cursor = 0; el.requestUpdate();
    await el.updateComplete;

    const count = host.querySelector(".lthn-chat-find-count");
    expect(count?.textContent?.trim()).toBe("1 of 4");

    el._findController.cursor = 2; el.requestUpdate();
    await el.updateComplete;
    expect(host.querySelector(".lthn-chat-find-count")?.textContent?.trim()).toBe("3 of 4");
    host.remove();
  });

  it("changing the query resets findCursor to 0", async () => {
    const { el } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el._findController.toggle();
    el._findController.setQuery("regex");
    el.liveTurns = [{ role: "you", text: "regex regex" }];
    el._findController.cursor = 1; el.requestUpdate();
    await el.updateComplete;

    el._findController.setQuery("different");
    await el.updateComplete;
    expect(el._findController.cursor, "new query → cursor 0").toBe(0);
  });

  it("next-match button steps the cursor forward", async () => {
    const { el, host } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el._findController.toggle();
    el._findController.setQuery("a");
    el.liveTurns = [{ role: "you", text: "aaa" }];
    el._findController.cursor = 0; el.requestUpdate();
    await el.updateComplete;

    const next = host.querySelector<HTMLButtonElement>(".lthn-chat-find-next");
    next!.click();
    await el.updateComplete;
    expect(el._findController.cursor).toBe(1);
    host.remove();
  });

  it("renders an active class on the cursor-current match span", async () => {
    const { el, host } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el.state = "multi-turn";
    el.activeConversationId = "find-active";
    el.liveTurns = [{ role: "you", text: "alpha alpha alpha" }];
    el._findController.toggle();
    el._findController.setQuery("alpha");
    el._findController.cursor = 1; el.requestUpdate(); // middle match
    await el.updateComplete;

    const active = host.querySelector(".lthn-chat-find-active");
    expect(active, "active match flagged").not.toBeNull();
    expect(active?.textContent).toBe("alpha");
    // Exactly one active span (the others are plain matches).
    const allActive = host.querySelectorAll(".lthn-chat-find-active");
    expect(allActive.length, "exactly one match carries the active class").toBe(1);
    host.remove();
  });
});

describe("⌘B toggles right rail", () => {
  type ChatWindowEl = HTMLElement & {
    rightRail: string;
    updateComplete: Promise<boolean>;
  };

  it("⌘B flips rightRail expanded → collapsed", async () => {
    const { el, host } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el.rightRail = "expanded";
    await el.updateComplete;

    window.dispatchEvent(new KeyboardEvent("keydown", { key: "b", metaKey: true, bubbles: true }));
    await el.updateComplete;

    expect(el.rightRail).toBe("collapsed");
    host.remove();
  });

  it("⌘B flips rightRail collapsed → expanded", async () => {
    const { el, host } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el.rightRail = "collapsed";
    await el.updateComplete;

    window.dispatchEvent(new KeyboardEvent("keydown", { key: "b", metaKey: true, bubbles: true }));
    await el.updateComplete;

    expect(el.rightRail).toBe("expanded");
    host.remove();
  });

  it("Ctrl+B triggers the same toggle (Linux/Windows)", async () => {
    const { el, host } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el.rightRail = "expanded";
    await el.updateComplete;

    window.dispatchEvent(new KeyboardEvent("keydown", { key: "b", ctrlKey: true, bubbles: true }));
    await el.updateComplete;

    expect(el.rightRail).toBe("collapsed");
    host.remove();
  });

  it("plain 'b' without modifier does NOT toggle the rail", async () => {
    const { el, host } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el.rightRail = "expanded";
    await el.updateComplete;

    window.dispatchEvent(new KeyboardEvent("keydown", { key: "b", bubbles: true }));
    await el.updateComplete;

    expect(el.rightRail, "plain b is just a key — not a toggle").toBe("expanded");
    host.remove();
  });

  it("only the last-mounted chat-window handles ⌘B (multi-instance gate)", async () => {
    const a = await mountWindow<ChatWindowEl>("lthn-chat-window");
    const b = await mountWindow<ChatWindowEl>("lthn-chat-window");
    a.el.rightRail = "expanded";
    b.el.rightRail = "expanded";
    await a.el.updateComplete;
    await b.el.updateComplete;

    window.dispatchEvent(new KeyboardEvent("keydown", { key: "b", metaKey: true, bubbles: true }));
    await a.el.updateComplete;
    await b.el.updateComplete;

    expect(a.el.rightRail, "first instance stays idle under the gate").toBe("expanded");
    expect(b.el.rightRail, "last-mounted instance owns the shortcut").toBe("collapsed");

    a.host.remove();
    b.host.remove();
  });

  it("/help cheat sheet lists ⌘B under Conversations", async () => {
    const { el, host } = await mountWindow<HTMLElement & {
      _helpOverlayController: { open: boolean; close: () => boolean; show: () => void; toggle: () => void };
      updateComplete: Promise<boolean>;
    }>("lthn-chat-window");
    el._helpOverlayController.show();
    await el.updateComplete;

    const text = host.textContent || "";
    expect(text).toContain("⌘B");
    expect(text).toContain("Toggle the right rail");
    host.remove();
  });
});

describe("F2 rename shortcut", () => {
  it("F2 with an active conversation enters rename mode on it", async () => {
    const { el, host } = await mountWindow<HTMLElement & {
      activeConversationId: string | null;
      renamingId: string | null;
      updateComplete: Promise<boolean>;
    }>("lthn-chat-window");
    el.activeConversationId = "session-abc";
    el.renamingId = null;
    await el.updateComplete;

    const ev = new KeyboardEvent("keydown", { key: "F2", bubbles: true, cancelable: true });
    window.dispatchEvent(ev);
    await el.updateComplete;

    expect(el.renamingId).toBe("session-abc");
    host.remove();
  });

  it("F2 without an active conversation is a no-op", async () => {
    const { el, host } = await mountWindow<HTMLElement & {
      activeConversationId: string | null;
      renamingId: string | null;
      updateComplete: Promise<boolean>;
    }>("lthn-chat-window");
    el.activeConversationId = null;
    el.renamingId = null;
    await el.updateComplete;

    window.dispatchEvent(new KeyboardEvent("keydown", { key: "F2", bubbles: true }));
    await el.updateComplete;

    expect(el.renamingId).toBeNull();
    host.remove();
  });

  it("F2 dispatched from inside an input does not steal the key", async () => {
    const { el, host } = await mountWindow<HTMLElement & {
      activeConversationId: string | null;
      renamingId: string | null;
      updateComplete: Promise<boolean>;
    }>("lthn-chat-window");
    el.activeConversationId = "session-xyz";
    el.renamingId = null;
    await el.updateComplete;

    const input = host.querySelector<HTMLInputElement>("input.lthn-conversation-search")!;
    expect(input, "rail search input present").not.toBeNull();
    input.focus();
    input.dispatchEvent(new KeyboardEvent("keydown", { key: "F2", bubbles: true }));
    await el.updateComplete;

    expect(el.renamingId, "input-focused F2 must not trigger rename").toBeNull();
    host.remove();
  });

  it("Escape closes the slash menu when it's open", async () => {
    const { el, host } = await mountWindow<HTMLElement & {
      _slashMenuController: { open: boolean; close: () => boolean; toggle: () => void };
      updateComplete: Promise<boolean>;
    }>("lthn-chat-window");
    el._slashMenuController.toggle();
    await el.updateComplete;

    window.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
    await el.updateComplete;

    expect(el._slashMenuController.open, "Escape closes an open slash menu").toBe(false);
    host.remove();
  });

  it("Escape with menu closed does not toggle it", async () => {
    const { el, host } = await mountWindow<HTMLElement & {
      _slashMenuController: { open: boolean; close: () => boolean; toggle: () => void };
      updateComplete: Promise<boolean>;
    }>("lthn-chat-window");
    el._slashMenuController.close();
    await el.updateComplete;

    window.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
    await el.updateComplete;

    expect(el._slashMenuController.open, "Escape must not flip closed-to-open").toBe(false);
    host.remove();
  });

  it("only the last-mounted chat-window handles F2 (multi-instance gate)", async () => {
    // Mount two instances. The second is the canonical handler under
    // the gate; the first should remain idle.
    const a = await mountWindow<HTMLElement & {
      activeConversationId: string | null;
      renamingId: string | null;
      updateComplete: Promise<boolean>;
    }>("lthn-chat-window");
    const b = await mountWindow<HTMLElement & {
      activeConversationId: string | null;
      renamingId: string | null;
      updateComplete: Promise<boolean>;
    }>("lthn-chat-window");
    a.el.activeConversationId = "alpha";
    b.el.activeConversationId = "bravo";
    a.el.renamingId = null;
    b.el.renamingId = null;
    await a.el.updateComplete;
    await b.el.updateComplete;

    window.dispatchEvent(new KeyboardEvent("keydown", { key: "F2", bubbles: true }));
    await a.el.updateComplete;
    await b.el.updateComplete;

    expect(a.el.renamingId, "first instance stays idle under the gate").toBeNull();
    expect(b.el.renamingId, "last-mounted instance owns the shortcut").toBe("bravo");

    a.host.remove();
    b.host.remove();
  });
});

describe("composerCounterText — char count formatter", () => {
  it("returns empty string for empty / blank / null value", () => {
    expect(composerCounterText("")).toBe("");
    expect(composerCounterText(null)).toBe("");
    expect(composerCounterText(undefined)).toBe("");
  });

  it("uses singular form for exactly one character", () => {
    expect(composerCounterText("x")).toBe("1 char");
  });

  it("uses plural form for two or more characters", () => {
    expect(composerCounterText("xx")).toBe("2 chars");
    expect(composerCounterText("hello world")).toBe("11 chars");
  });

  it("counts whitespace + punctuation verbatim — no normalisation", () => {
    expect(composerCounterText("a b c")).toBe("5 chars");
    expect(composerCounterText("   ")).toBe("3 chars");
  });
});

describe("composerCounterTone — quiet / warn picker", () => {
  it("returns 'quiet' below the warn threshold", () => {
    expect(composerCounterTone("")).toBe("quiet");
    expect(composerCounterTone("a")).toBe("quiet");
    expect(composerCounterTone("x".repeat(COMPOSER_COUNTER_WARN_AT - 1))).toBe("quiet");
  });

  it("returns 'warn' at and above the warn threshold", () => {
    expect(composerCounterTone("x".repeat(COMPOSER_COUNTER_WARN_AT))).toBe("warn");
    expect(composerCounterTone("x".repeat(COMPOSER_COUNTER_WARN_AT + 10))).toBe("warn");
  });

  it("treats null / undefined as quiet (no length to warn about)", () => {
    expect(composerCounterTone(null)).toBe("quiet");
    expect(composerCounterTone(undefined)).toBe("quiet");
  });
});

describe("chat-window — composer counter render", () => {
  type ChatWindowEl = HTMLElement & {
    composerValue: string;
    state: string;
    updateComplete: Promise<boolean>;
  };

  it("renders the counter element when composerValue is non-empty", async () => {
    const { el, host } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el.state = "multi-turn";
    el.composerValue = "hello world";
    await el.updateComplete;

    const counter = host.querySelector(".lthn-chat-composer-counter");
    expect(counter, "counter rendered").not.toBeNull();
    expect(counter?.textContent?.trim()).toBe("11 chars");
    expect(counter?.getAttribute("data-counter-tone")).toBe("quiet");
  });

  it("counter does not render when composerValue is empty", async () => {
    const { el, host } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el.composerValue = "";
    await el.updateComplete;

    expect(host.querySelector(".lthn-chat-composer-counter")).toBeNull();
  });

  it("counter switches to warn tone past the threshold", async () => {
    const { el, host } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el.state = "multi-turn";
    el.composerValue = "x".repeat(COMPOSER_COUNTER_WARN_AT + 50);
    await el.updateComplete;

    const counter = host.querySelector(".lthn-chat-composer-counter");
    expect(counter).not.toBeNull();
    expect(counter?.getAttribute("data-counter-tone")).toBe("warn");
  });
});

describe("chat-window — _autoGrowComposer", () => {
  type ChatWindowEl = HTMLElement & {
    _autoGrowComposer: (ta: HTMLTextAreaElement) => void;
  };

  it("sets height to scrollHeight when content fits under the cap", async () => {
    const { el } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    const ta = document.createElement("textarea");
    Object.defineProperty(ta, "scrollHeight", { value: 120, configurable: true });
    el._autoGrowComposer(ta);
    expect(ta.style.height).toBe("120px");
  });

  it("caps height at COMPOSER_MAX_HEIGHT for very long content", async () => {
    const { el } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    const ta = document.createElement("textarea");
    Object.defineProperty(ta, "scrollHeight", { value: 9999, configurable: true });
    el._autoGrowComposer(ta);
    expect(ta.style.height).toBe(`${COMPOSER_MAX_HEIGHT}px`);
  });

  it("is a no-op for null target", async () => {
    const { el } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    let threw: unknown = null;
    try { el._autoGrowComposer(null as unknown as HTMLTextAreaElement); }
    catch (e) { threw = e; }
    expect(threw).toBeNull();
  });
});

describe("highlightMatch — search substring splitter", () => {
  it("returns [] for empty text", () => {
    expect(highlightMatch("", "x")).toEqual([]);
  });

  it("returns a single non-match segment for empty query", () => {
    expect(highlightMatch("hello world", "")).toEqual([{ text: "hello world", match: false }]);
    expect(highlightMatch("hello world", "  ")).toEqual([{ text: "hello world", match: false }]);
  });

  it("splits a single match", () => {
    expect(highlightMatch("hello world", "lo w")).toEqual([
      { text: "hel", match: false },
      { text: "lo w", match: true },
      { text: "orld", match: false },
    ]);
  });

  it("splits multiple matches", () => {
    expect(highlightMatch("ababab", "ab")).toEqual([
      { text: "ab", match: true },
      { text: "ab", match: true },
      { text: "ab", match: true },
    ]);
  });

  it("is case-insensitive but preserves the original casing", () => {
    expect(highlightMatch("Hello", "hello")).toEqual([
      { text: "Hello", match: true },
    ]);
    expect(highlightMatch("HELLO World", "hello w")).toEqual([
      { text: "HELLO W", match: true },
      { text: "orld", match: false },
    ]);
  });

  it("treats special regex characters as literals", () => {
    expect(highlightMatch("a.*b", ".*")).toEqual([
      { text: "a", match: false },
      { text: ".*", match: true },
      { text: "b", match: false },
    ]);
  });

  it("returns just a non-match when query is not present", () => {
    expect(highlightMatch("the quick brown fox", "xyz")).toEqual([
      { text: "the quick brown fox", match: false },
    ]);
  });

  it("handles a match at the start", () => {
    expect(highlightMatch("regex pitfalls", "regex")).toEqual([
      { text: "regex", match: true },
      { text: " pitfalls", match: false },
    ]);
  });

  it("handles a match at the end", () => {
    expect(highlightMatch("pitfalls of regex", "regex")).toEqual([
      { text: "pitfalls of ", match: false },
      { text: "regex", match: true },
    ]);
  });
});

describe("chat-window — _renderHighlighted in rail rows", () => {
  type ChatWindowEl = HTMLElement & {
    conversations: Conversation[];
    activeConversationId: string | null;
    searchQuery: string;
    state: string;
    updateComplete: Promise<boolean>;
  };

  it("renders a .lthn-rail-match span when the title matches the query", async () => {
    const { el, host } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el.conversations = [{ id: "x", bucket: "today", title: "regex pitfalls", snippet: "", model: "" }];
    el.searchQuery = "regex";
    await el.updateComplete;

    const row = host.querySelector('[data-conversation-id="x"]')!;
    const match = row.querySelector(".lthn-rail-match");
    expect(match, "matched span rendered").not.toBeNull();
    expect(match?.textContent).toBe("regex");
  });

  it("does NOT add highlight spans when no query is set", async () => {
    const { el, host } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el.conversations = [{ id: "x", bucket: "today", title: "regex pitfalls", snippet: "", model: "" }];
    el.searchQuery = "";
    await el.updateComplete;

    const row = host.querySelector('[data-conversation-id="x"]')!;
    expect(row.querySelector(".lthn-rail-match")).toBeNull();
  });

  it("renders the system-prompt indicator on rows whose conversation has a prompt set", async () => {
    const { el, host } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el.conversations = [
      { id: "a", bucket: "today", title: "with prompt", snippet: "", model: "", systemPrompt: "Be terse." },
      { id: "b", bucket: "today", title: "without prompt", snippet: "", model: "", systemPrompt: "" },
    ];
    el.searchQuery = "";
    await el.updateComplete;

    const flagA = host.querySelector('[data-conversation-id="a"] .lthn-conversation-prompt-flag');
    const flagB = host.querySelector('[data-conversation-id="b"] .lthn-conversation-prompt-flag');
    expect(flagA, "row with a prompt gets the indicator").not.toBeNull();
    expect(flagB, "row without a prompt has no indicator").toBeNull();
  });

  it("highlights snippet matches too", async () => {
    const { el, host } = await mountWindow<ChatWindowEl & {
      contentMatchedIds: Set<string>;
    }>("lthn-chat-window");
    el.conversations = [{
      id: "x", bucket: "today",
      title: "weekly review",
      snippet: "let's talk about regex anchors next",
      model: "",
    }];
    // Content-only match: title doesn't match "regex" but the
    // backend Sessions.Search would surface this row via id. Seed
    // the set directly so the row renders without firing a real
    // Search call.
    el.contentMatchedIds = new Set(["x"]);
    el.searchQuery = "regex";
    await el.updateComplete;

    const row = host.querySelector('[data-conversation-id="x"]')!;
    expect(row, "content-matched row renders").not.toBeNull();
    const matches = row.querySelectorAll(".lthn-rail-match");
    expect(matches.length, "snippet match also highlighted").toBeGreaterThanOrEqual(1);
    expect(Array.from(matches).some(m => m.textContent === "regex")).toBe(true);
  });
});

describe("withSystemPrompt — system message prefix", () => {
  type Msg = { role: string; content: string };
  const u: Msg = { role: "user", content: "hi" };
  const a: Msg = { role: "assistant", content: "hello" };

  it("passes the history through unchanged when no prompt is set", () => {
    expect(withSystemPrompt([u, a], "")).toEqual([u, a]);
    expect(withSystemPrompt([u, a], null)).toEqual([u, a]);
    expect(withSystemPrompt([u, a], undefined)).toEqual([u, a]);
    expect(withSystemPrompt([u, a], "   ")).toEqual([u, a]);
  });

  it("prepends a system message when a prompt is set", () => {
    const out = withSystemPrompt([u, a], "Be terse.");
    expect(out).toHaveLength(3);
    expect(out[0]).toEqual({ role: "system", content: "Be terse." });
    expect(out[1]).toEqual(u);
    expect(out[2]).toEqual(a);
  });

  it("trims the prompt before prepending", () => {
    const out = withSystemPrompt([u], "  Be terse.  ");
    expect(out[0]).toEqual({ role: "system", content: "Be terse." });
  });

  it("does not mutate the input history", () => {
    const hist = [u];
    const before = [...hist];
    withSystemPrompt(hist, "x");
    expect(hist).toEqual(before);
  });
});

describe("parseTagInput — comma-separated parser", () => {
  it("returns [] for empty / null", () => {
    expect(parseTagInput("")).toEqual([]);
    expect(parseTagInput(null)).toEqual([]);
    expect(parseTagInput(undefined)).toEqual([]);
  });

  it("splits on commas + trims each", () => {
    expect(parseTagInput("work, ops, research")).toEqual(["work", "ops", "research"]);
    expect(parseTagInput("  work  ,  ops  ")).toEqual(["work", "ops"]);
  });

  it("lowercases tags", () => {
    expect(parseTagInput("Work, OPS, ReSearch")).toEqual(["work", "ops", "research"]);
  });

  it("dedupes — first occurrence wins", () => {
    expect(parseTagInput("a,b,A,B,a")).toEqual(["a", "b"]);
  });

  it("drops blank segments", () => {
    expect(parseTagInput(",,a,,,b,,,")).toEqual(["a", "b"]);
    expect(parseTagInput(", ,  ,")).toEqual([]);
  });
});

describe("tagsToInput — render array back to input string", () => {
  it("returns empty string for nullish / empty arrays", () => {
    expect(tagsToInput(null)).toBe("");
    expect(tagsToInput(undefined)).toBe("");
    expect(tagsToInput([])).toBe("");
  });

  it("joins with comma + space", () => {
    expect(tagsToInput(["a", "b", "c"])).toBe("a, b, c");
  });

  it("parseTagInput ∘ tagsToInput is identity for normalised arrays", () => {
    const tags = ["work", "ops", "research"];
    expect(parseTagInput(tagsToInput(tags))).toEqual(tags);
  });
});

describe("relativeTime — human-readable elapsed", () => {
  const now = 1_700_000_000; // arbitrary fixed reference for deterministic tests

  it("returns 'now' for under a minute", () => {
    expect(relativeTime(now, now)).toBe("now");
    expect(relativeTime(now - 30, now)).toBe("now");
    expect(relativeTime(now - 59, now)).toBe("now");
  });

  it("returns 'Xm ago' for minutes", () => {
    expect(relativeTime(now - 60, now)).toBe("1m ago");
    expect(relativeTime(now - 5 * 60, now)).toBe("5m ago");
    expect(relativeTime(now - 59 * 60, now)).toBe("59m ago");
  });

  it("returns 'Xh ago' for hours", () => {
    expect(relativeTime(now - 3600, now)).toBe("1h ago");
    expect(relativeTime(now - 5 * 3600, now)).toBe("5h ago");
    expect(relativeTime(now - 23 * 3600, now)).toBe("23h ago");
  });

  it("returns 'yesterday' for 24h-48h ago", () => {
    expect(relativeTime(now - 86400, now)).toBe("yesterday");
    expect(relativeTime(now - 36 * 3600, now)).toBe("yesterday");
    expect(relativeTime(now - 47 * 3600, now)).toBe("yesterday");
  });

  it("returns 'Xd ago' for 2-6 days ago", () => {
    expect(relativeTime(now - 2 * 86400, now)).toBe("2d ago");
    expect(relativeTime(now - 6 * 86400, now)).toBe("6d ago");
  });

  it("returns 'Xw ago' for 1-3 weeks ago", () => {
    expect(relativeTime(now - 7 * 86400, now)).toBe("1w ago");
    expect(relativeTime(now - 21 * 86400, now)).toBe("3w ago");
  });

  it("returns yyyy-mm-dd for ~4 weeks or older", () => {
    const past = now - 60 * 86400; // ~2 months
    const out = relativeTime(past, now);
    expect(out).toMatch(/^\d{4}-\d{2}-\d{2}$/);
  });

  it("returns '' for missing / zero timestamp", () => {
    expect(relativeTime(0, now)).toBe("");
    expect(relativeTime(-1, now)).toBe("");
  });

  it("clamps future timestamps to 'now' (never negative)", () => {
    expect(relativeTime(now + 60, now)).toBe("now");
  });
});

describe("chat-window — relative time render", () => {
  type ChatWindowEl = HTMLElement & {
    conversations: Conversation[];
    updateComplete: Promise<boolean>;
  };

  it("renders the .lthn-conversation-time span when updatedAt is set", async () => {
    const { el, host } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    const nowSec = Math.floor(Date.now() / 1000);
    el.conversations = [{
      id: "x", bucket: "today", title: "fresh", snippet: "", model: "",
      updatedAt: nowSec - 600, // 10m ago
    }];
    await el.updateComplete;

    const stamp = host.querySelector('[data-conversation-id="x"] .lthn-conversation-time');
    expect(stamp, "time stamp rendered").not.toBeNull();
    expect(stamp?.textContent?.trim()).toBe("10m ago");
  });

  it("hides the time stamp when updatedAt is missing", async () => {
    const { el, host } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el.conversations = [{
      id: "x", bucket: "today", title: "no-stamp", snippet: "", model: "",
    }];
    await el.updateComplete;

    expect(host.querySelector('[data-conversation-id="x"] .lthn-conversation-time')).toBeNull();
  });
});

describe("matchesTagFilter — tag-based filter predicate", () => {
  it("returns true when filter is empty (no filter applied)", () => {
    expect(matchesTagFilter({ tags: [] }, "")).toBe(true);
    expect(matchesTagFilter({ tags: ["a"] }, "  ")).toBe(true);
    expect(matchesTagFilter({ tags: ["a"] }, null)).toBe(true);
    expect(matchesTagFilter({ tags: ["a"] }, undefined)).toBe(true);
  });

  it("returns true when the tag is present (case-insensitive)", () => {
    expect(matchesTagFilter({ tags: ["work", "ops"] }, "work")).toBe(true);
    expect(matchesTagFilter({ tags: ["work", "ops"] }, "WORK")).toBe(true);
    expect(matchesTagFilter({ tags: ["WoRk"] }, "work")).toBe(true);
  });

  it("returns false when the tag is missing", () => {
    expect(matchesTagFilter({ tags: ["work"] }, "ops")).toBe(false);
    expect(matchesTagFilter({ tags: [] }, "work")).toBe(false);
    expect(matchesTagFilter({}, "work")).toBe(false);
  });

  it("trims the filter value before comparing", () => {
    expect(matchesTagFilter({ tags: ["work"] }, "  work  ")).toBe(true);
  });
});

describe("chat-window — _toggleTagFilter + tag filter render", () => {
  type ChatWindowEl = HTMLElement & {
    conversations: Conversation[];
    tagFilter: string;
    _toggleTagFilter: (tag: string) => void;
    updateComplete: Promise<boolean>;
  };

  it("_toggleTagFilter sets the filter to the given tag", async () => {
    const { el } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el._toggleTagFilter("work");
    expect(el.tagFilter).toBe("work");
  });

  it("_toggleTagFilter with the active tag clears it (toggle)", async () => {
    const { el } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el.tagFilter = "work";
    el._toggleTagFilter("work");
    expect(el.tagFilter).toBe("");
  });

  it("_toggleTagFilter with a different tag replaces", async () => {
    const { el } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el.tagFilter = "work";
    el._toggleTagFilter("ops");
    expect(el.tagFilter).toBe("ops");
  });

  it("_toggleTagFilter normalises to lowercase", async () => {
    const { el } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el._toggleTagFilter("  WORK  ");
    expect(el.tagFilter).toBe("work");
  });

  it("filter pill renders when a tag is active", async () => {
    const { el, host } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el.tagFilter = "work";
    await el.updateComplete;
    const pill = host.querySelector(".lthn-conversation-tag-filter");
    expect(pill, "filter pill rendered").not.toBeNull();
    expect(pill?.textContent || "").toContain("work");
  });

  it("filter pill hidden when no tag is active", async () => {
    const { el, host } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el.tagFilter = "";
    await el.updateComplete;
    expect(host.querySelector(".lthn-conversation-tag-filter")).toBeNull();
  });

  it("clear button on the pill clears the filter", async () => {
    const { el, host } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el.tagFilter = "work";
    await el.updateComplete;
    const clear = host.querySelector<HTMLButtonElement>(".lthn-conversation-tag-filter-clear");
    expect(clear).not.toBeNull();
    clear!.click();
    await el.updateComplete;
    expect(el.tagFilter).toBe("");
  });

  it("tag filter intersects with rail render — only matching rows show", async () => {
    const { el, host } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el.conversations = [
      { id: "a", bucket: "today", title: "alpha", snippet: "", model: "", tags: ["work"] },
      { id: "b", bucket: "today", title: "bravo", snippet: "", model: "", tags: ["ops"] },
      { id: "c", bucket: "today", title: "charlie", snippet: "", model: "", tags: ["work", "ops"] },
    ];
    el.tagFilter = "work";
    await el.updateComplete;

    expect(host.querySelector('[data-conversation-id="a"]'), "work tag row visible").not.toBeNull();
    expect(host.querySelector('[data-conversation-id="b"]'), "ops-only row hidden").toBeNull();
    expect(host.querySelector('[data-conversation-id="c"]'), "multi-tag row visible (work included)").not.toBeNull();
  });

  it("clicking a tag chip on a rail row sets the filter", async () => {
    const { el, host } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el.conversations = [
      { id: "a", bucket: "today", title: "alpha", snippet: "", model: "", tags: ["work"] },
    ];
    await el.updateComplete;

    const chip = host.querySelector<HTMLButtonElement>('[data-conversation-id="a"] .lthn-conversation-tag');
    expect(chip, "chip rendered as button").not.toBeNull();
    chip!.click();
    await el.updateComplete;
    expect(el.tagFilter).toBe("work");
  });
});

describe("chat-window — tags right-rail + chip render", () => {
  type ChatWindowEl = HTMLElement & {
    conversations: Conversation[];
    activeConversationId: string | null;
    tagsDraft: string;
    rightRail: string;
    _loadTagsDraft: () => void;
    _saveTags: () => Promise<void>;
    updateComplete: Promise<boolean>;
  };

  it("renders the Tags input in the expanded right rail", async () => {
    const { el, host } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el.rightRail = "expanded";
    await el.updateComplete;
    expect(host.querySelector("input.lthn-chat-tags-input"), "tags input present").not.toBeNull();
  });

  it("_loadTagsDraft seeds draft from active conversation's tags", async () => {
    const { el } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el.conversations = [{ id: "x", bucket: "today", title: "x", snippet: "", model: "", tags: ["work", "ops"] }];
    el.activeConversationId = "x";
    el._loadTagsDraft();
    expect(el.tagsDraft).toBe("work, ops");
  });

  it("_loadTagsDraft blanks the draft when no active conversation", async () => {
    const { el } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el.activeConversationId = null;
    el.tagsDraft = "stale";
    el._loadTagsDraft();
    expect(el.tagsDraft).toBe("");
  });

  it("_saveTags is a no-op when no active conversation", async () => {
    const { el } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el.activeConversationId = null;
    el.tagsDraft = "anything";
    let threw: unknown = null;
    try { await el._saveTags(); }
    catch (e) { threw = e; }
    expect(threw, "no-active state must not throw").toBeNull();
  });

  it("rail row renders one chip per tag", async () => {
    const { el, host } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el.conversations = [{
      id: "tagged", bucket: "today", title: "tagged convo", snippet: "", model: "",
      tags: ["work", "ops", "research"],
    }];
    await el.updateComplete;

    const chips = host.querySelectorAll('[data-conversation-id="tagged"] .lthn-conversation-tag');
    expect(chips.length).toBe(3);
    expect(Array.from(chips).map(c => c.textContent)).toEqual(["work", "ops", "research"]);
  });

  it("rail row renders no chip strip when tags are empty", async () => {
    const { el, host } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el.conversations = [{
      id: "no-tags", bucket: "today", title: "untagged", snippet: "", model: "", tags: [],
    }];
    await el.updateComplete;

    expect(host.querySelector('[data-conversation-id="no-tags"] .lthn-conversation-tags')).toBeNull();
  });
});

describe("chat-window — system prompt right-rail render", () => {
  type ChatWindowEl = HTMLElement & {
    conversations: Conversation[];
    activeConversationId: string | null;
    systemPromptDraft: string;
    state: string;
    rightRail: string;
    _loadSystemPromptDraft: () => void;
    _saveSystemPrompt: () => Promise<void>;
    updateComplete: Promise<boolean>;
  };

  const c = (id: string, prompt: string = ""): Conversation =>
    ({ id, bucket: "today", title: id, snippet: "", model: "", systemPrompt: prompt });

  it("renders the Instructions textarea inside the expanded right rail", async () => {
    const { el, host } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el.rightRail = "expanded";
    await el.updateComplete;

    const ta = host.querySelector("textarea.lthn-chat-systemprompt-input");
    expect(ta, "Instructions textarea present").not.toBeNull();
  });

  it("_loadSystemPromptDraft seeds the draft from the active conversation", async () => {
    const { el } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el.conversations = [c("alpha", "Be brief.")];
    el.activeConversationId = "alpha";
    el._loadSystemPromptDraft();
    expect(el.systemPromptDraft).toBe("Be brief.");
  });

  it("_loadSystemPromptDraft blanks the draft for a session with no prompt", async () => {
    const { el } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el.conversations = [c("alpha", "")];
    el.activeConversationId = "alpha";
    el.systemPromptDraft = "stale";
    el._loadSystemPromptDraft();
    expect(el.systemPromptDraft).toBe("");
  });

  it("_loadSystemPromptDraft blanks the draft when no conversation is active", async () => {
    const { el } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el.activeConversationId = null;
    el.systemPromptDraft = "stale";
    el._loadSystemPromptDraft();
    expect(el.systemPromptDraft).toBe("");
  });

  it("_saveSystemPrompt is a no-op when no conversation is active", async () => {
    const { el } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el.activeConversationId = null;
    el.systemPromptDraft = "anything";
    let threw: unknown = null;
    try { await el._saveSystemPrompt(); }
    catch (e) { threw = e; }
    expect(threw, "no-active state must not throw").toBeNull();
  });
});

describe("userMessageHistory — pure helper", () => {
  it("returns [] for null / undefined input", () => {
    expect(userMessageHistory(null)).toEqual([]);
    expect(userMessageHistory(undefined)).toEqual([]);
    expect(userMessageHistory([])).toEqual([]);
  });

  it("keeps only role==='you' entries, oldest first", () => {
    const turns: ChatTurn[] = [
      { role: "you", text: "first" },
      { role: "assistant", text: "reply A" },
      { role: "you", text: "second" },
      { role: "assistant", text: "reply B" },
      { role: "you", text: "third" },
    ];
    expect(userMessageHistory(turns)).toEqual(["first", "second", "third"]);
  });

  it("filters whitespace-only user turns", () => {
    const turns: ChatTurn[] = [
      { role: "you", text: "real" },
      { role: "you", text: "   " },
      { role: "you", text: "" },
      { role: "you", text: "another" },
    ];
    expect(userMessageHistory(turns)).toEqual(["real", "another"]);
  });

  it("ignores assistant turns entirely", () => {
    const turns: ChatTurn[] = [
      { role: "assistant", text: "hello" },
      { role: "assistant", text: "world" },
    ];
    expect(userMessageHistory(turns)).toEqual([]);
  });
});

describe("chat-window — composer history navigation", () => {
  type ChatWindowEl = HTMLElement & {
    composerValue: string;
    liveTurns: ChatTurn[] | null;
    activeConversationId: string | null;
    state: string;
    _composerKeydown: (e: KeyboardEvent) => void;
    updateComplete: Promise<boolean>;
  };

  function seedHistory(el: ChatWindowEl, msgs: string[]) {
    el.activeConversationId = "history-test";
    el.state = "multi-turn";
    el.liveTurns = msgs.map(text => ({ role: "you" as const, text }));
  }

  it("↑ on empty composer loads the newest user message", async () => {
    const { el } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    seedHistory(el, ["alpha", "bravo", "charlie"]);
    el.composerValue = "";
    await el.updateComplete;

    const ev = new KeyboardEvent("keydown", { key: "ArrowUp", bubbles: true, cancelable: true });
    el._composerKeydown(ev);
    expect(el.composerValue, "↑ loads the newest entry").toBe("charlie");
  });

  it("↑ ↑ walks back through history", async () => {
    const { el } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    seedHistory(el, ["alpha", "bravo", "charlie"]);
    el.composerValue = "";
    await el.updateComplete;

    el._composerKeydown(new KeyboardEvent("keydown", { key: "ArrowUp", bubbles: true, cancelable: true }));
    expect(el.composerValue).toBe("charlie");
    el._composerKeydown(new KeyboardEvent("keydown", { key: "ArrowUp", bubbles: true, cancelable: true }));
    expect(el.composerValue).toBe("bravo");
    el._composerKeydown(new KeyboardEvent("keydown", { key: "ArrowUp", bubbles: true, cancelable: true }));
    expect(el.composerValue).toBe("alpha");
    // Further ↑ stays at the oldest (no wrap).
    el._composerKeydown(new KeyboardEvent("keydown", { key: "ArrowUp", bubbles: true, cancelable: true }));
    expect(el.composerValue, "↑ at the oldest stays put").toBe("alpha");
  });

  it("↓ walks forward and exits history past the newest", async () => {
    const { el } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    seedHistory(el, ["alpha", "bravo"]);
    el.composerValue = "";
    await el.updateComplete;

    el._composerKeydown(new KeyboardEvent("keydown", { key: "ArrowUp", bubbles: true, cancelable: true })); // newest = bravo
    el._composerKeydown(new KeyboardEvent("keydown", { key: "ArrowUp", bubbles: true, cancelable: true })); // alpha
    expect(el.composerValue).toBe("alpha");
    el._composerKeydown(new KeyboardEvent("keydown", { key: "ArrowDown", bubbles: true, cancelable: true })); // bravo
    expect(el.composerValue).toBe("bravo");
    el._composerKeydown(new KeyboardEvent("keydown", { key: "ArrowDown", bubbles: true, cancelable: true })); // exit
    expect(el.composerValue, "↓ past newest exits history mode → empty").toBe("");
  });

  it("↑ on a non-empty composer (not in history) is a no-op", async () => {
    const { el } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    seedHistory(el, ["alpha", "bravo"]);
    el.composerValue = "drafting…";
    await el.updateComplete;

    el._composerKeydown(new KeyboardEvent("keydown", { key: "ArrowUp", bubbles: true, cancelable: true }));
    expect(el.composerValue, "active draft preserved — ↑ goes to default textarea behavior").toBe("drafting…");
  });

  it("↑ with no user-message history is a no-op", async () => {
    const { el } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el.liveTurns = [{ role: "assistant", text: "hi" }];
    el.composerValue = "";
    await el.updateComplete;

    el._composerKeydown(new KeyboardEvent("keydown", { key: "ArrowUp", bubbles: true, cancelable: true }));
    expect(el.composerValue, "no user turns → no history entry to load").toBe("");
  });

  it("↓ with cursor === -1 is a no-op (textarea default)", async () => {
    const { el } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    seedHistory(el, ["alpha"]);
    el.composerValue = "";
    await el.updateComplete;

    el._composerKeydown(new KeyboardEvent("keydown", { key: "ArrowDown", bubbles: true, cancelable: true }));
    expect(el.composerValue).toBe("");
  });
});

// — XSS regression-guards for SessionInfo-sourced rail fields
//
// SessionInfo carries user-controlled content (snippet from message
// previews, systemPrompt from direct paste, tags from direct input).
// If any rail consumer ever swapped Lit text-binding for unsafeHTML /
// innerHTML, a user prompting about HTML (legitimate use: "explain
// this <script>") would land an executable payload inside the Wails
// WebView — which has Go-side bridge access (host RCE).
//
// These specs pin the safe behaviour: malicious markup MUST render as
// literal text, never materialise as DOM nodes. Originated from
// Cerberus F6 audit on Mantis #1539 (H#9 sessions data plane).
describe("rail XSS regression-guards — SessionInfo-sourced fields", () => {
  type ChatWindowEl = HTMLElement & {
    conversations: Array<Conversation & {
      systemPrompt?: string;
      tags?: string[];
      updatedAt?: number;
    }>;
    activeConversationId: string | null;
    updateComplete: Promise<boolean>;
  };

  const XSS_PAYLOAD = '<img src=x onerror="alert(1)">';
  const SCRIPT_PAYLOAD = '<script>alert("xss")</script>';

  it("snippet renders as text, never as DOM (Cerberus F6)", async () => {
    const { el, host } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el.conversations = [{
      id: "x1",
      bucket: "today",
      title: "benign title",
      snippet: XSS_PAYLOAD,
      model: "",
    }];
    await el.updateComplete;

    const row = host.querySelector('[data-conversation-id="x1"]');
    expect(row, "row should render").not.toBeNull();
    // No <img> materialised from the payload (chat-window itself never
    // emits <img>; if one appears, unsafeHTML let it through).
    expect(row!.querySelector("img"), "payload must NOT become an <img> node").toBeNull();
    // Literal text appears in the rendered DOM (Lit escapes the
    // angle brackets so the user sees what they typed, escaped).
    expect(row!.textContent, "literal payload text appears in DOM").toContain(XSS_PAYLOAD);
  });

  it("snippet with <script> payload renders as text, never as a <script> node", async () => {
    const { el, host } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el.conversations = [{
      id: "x2",
      bucket: "today",
      title: "benign",
      snippet: SCRIPT_PAYLOAD,
      model: "",
    }];
    await el.updateComplete;

    const row = host.querySelector('[data-conversation-id="x2"]');
    expect(row, "row should render").not.toBeNull();
    expect(row!.querySelector("script"), "payload must NOT become a <script> node").toBeNull();
    expect(row!.textContent).toContain(SCRIPT_PAYLOAD);
  });

  it("title renders as text, never as DOM", async () => {
    const { el, host } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el.conversations = [{
      id: "x3",
      bucket: "today",
      title: XSS_PAYLOAD,
      snippet: "",
      model: "",
    }];
    await el.updateComplete;

    const row = host.querySelector('[data-conversation-id="x3"]');
    expect(row, "row should render").not.toBeNull();
    expect(row!.querySelector("img"), "title payload must NOT become an <img> node").toBeNull();
    expect(row!.textContent).toContain(XSS_PAYLOAD);
  });

  it("tags render as text, never as DOM", async () => {
    const { el, host } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el.conversations = [{
      id: "x4",
      bucket: "today",
      title: "benign",
      snippet: "",
      model: "",
      tags: [XSS_PAYLOAD, SCRIPT_PAYLOAD],
    }];
    await el.updateComplete;

    const row = host.querySelector('[data-conversation-id="x4"]');
    expect(row, "row should render").not.toBeNull();
    // Tag chips are rendered as <button> elements; none of the
    // payload markup should materialise inside them.
    expect(row!.querySelector("img"), "tag payload must NOT become an <img> node").toBeNull();
    expect(row!.querySelector("script"), "tag payload must NOT become a <script> node").toBeNull();
    // Both literal tag strings appear in the tag chip buttons.
    const tagButtons = Array.from(row!.querySelectorAll("button.lthn-conversation-tag"));
    expect(tagButtons.length, "two tag chips render").toBe(2);
    const tagText = tagButtons.map(b => b.textContent || "").join(" | ");
    expect(tagText).toContain(XSS_PAYLOAD);
    expect(tagText).toContain(SCRIPT_PAYLOAD);
  });

  it("systemPrompt is a truthy-flag only — payload never reaches a render slot", async () => {
    // Today the rail only consults systemPrompt to decide whether to
    // render a small wand-magic icon (line 1669, chat-window.ts). It
    // never renders the prompt content itself. This spec pins that
    // behaviour: even with a malicious payload, no <img>/<script>
    // appears in the row, and the payload text does NOT leak into
    // the rail's DOM as visible text. If a future change adds a
    // preview surface for systemPrompt, the same auto-escape
    // discipline must hold — update this spec to assert literal-text
    // appearance instead of absence.
    const { el, host } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el.conversations = [{
      id: "x5",
      bucket: "today",
      title: "benign",
      snippet: "",
      model: "",
      systemPrompt: XSS_PAYLOAD,
    }];
    await el.updateComplete;

    const row = host.querySelector('[data-conversation-id="x5"]');
    expect(row, "row should render").not.toBeNull();
    expect(row!.querySelector("img"), "systemPrompt payload must NOT become an <img> node").toBeNull();
    expect(row!.querySelector("script"), "systemPrompt payload must NOT become a <script> node").toBeNull();
    // Wand-magic flag icon present (proves the truthy gate fired).
    expect(row!.querySelector("i.lthn-conversation-prompt-flag"), "wand-magic flag icon renders").not.toBeNull();
  });

});

// — ComposerGlyphController integration with chat-window. The controller
// itself is covered by composer-glyph.controller.test.ts; here we just
// assert the window's render helper paints the right class hook +
// honours the null-score case + does NOT gate the send button. We
// poke `el._glyphController.score` directly rather than driving the
// debounce through the @input handler — the controller is mocked in
// its own file; the window's job is wiring.

describe("lthn-chat-window — composer ethics-tier glyph", () => {
  it("renders glyph with --hollow-flattery class when score is tier-2", async () => {
    const { host, el } = await mountWindow<HTMLElement & {
      updateComplete: Promise<boolean>;
      requestUpdate: () => void;
      _glyphController: { score: unknown };
      composerValue: string;
    }>("lthn-chat-window");
    el._glyphController.score = {
      sycophancy: {
        tier: 2,
        label: "hollow_flattery",
        composite: 47,
      },
    };
    el.requestUpdate();
    await el.updateComplete;

    const glyph = host.querySelector(".composer-glyph");
    expect(glyph, "glyph node renders when score is populated").not.toBeNull();
    expect(glyph!.classList.contains("composer-glyph--hollow-flattery"))
      .toBe(true);
    // Tooltip carries the human-readable summary.
    expect(glyph!.getAttribute("title")).toContain("hollow_flattery");
    expect(glyph!.getAttribute("title")).toContain("47");
    expect(glyph!.getAttribute("aria-label")).toContain("hollow_flattery");
  });

  it("hides glyph when controller.score is null", async () => {
    const { host, el } = await mountWindow<HTMLElement & {
      updateComplete: Promise<boolean>;
      _glyphController: { score: unknown };
    }>("lthn-chat-window");
    // Default-mounted score is null (no @input has fired).
    expect(el._glyphController.score).toBeNull();
    await el.updateComplete;
    expect(host.querySelector(".composer-glyph"), "no glyph node when score is null").toBeNull();
  });

  it("glyph does NOT disable the send button — even at tier-3 (submission)", async () => {
    const { host, el } = await mountWindow<HTMLElement & {
      updateComplete: Promise<boolean>;
      requestUpdate: () => void;
      _glyphController: { score: unknown };
    }>("lthn-chat-window");
    el._glyphController.score = {
      sycophancy: {
        tier: 3,
        label: "submission",
        composite: 92,
      },
    };
    el.requestUpdate();
    await el.updateComplete;

    // Send button is the only <lthn-btn> carrying the up-arrow icon in
    // the composer footer. Look it up via its icon class to avoid
    // depending on tone="primary" (which other surfaces use too).
    const sendIcon = host.querySelector("i.fa-arrow-up");
    expect(sendIcon, "send button is rendered").not.toBeNull();
    const sendBtn = sendIcon!.closest("lthn-btn");
    expect(sendBtn, "send button wraps the icon").not.toBeNull();
    // The button must NOT carry `disabled` attribute and must NOT be
    // dimmed because of the glyph — `?dim` only mirrors the composer's
    // `disabled` prop (model-not-ready), never the glyph state.
    expect(sendBtn!.hasAttribute("disabled")).toBe(false);
  });

  it("renders correct kebab-case suffix for each tier", async () => {
    const { host, el } = await mountWindow<HTMLElement & {
      updateComplete: Promise<boolean>;
      requestUpdate: () => void;
      _glyphController: { score: unknown };
    }>("lthn-chat-window");
    const cases: Array<[number, string, string]> = [
      [0, "appropriate_empathy", "composer-glyph--appropriate-empathy"],
      [1, "soft_agreement",      "composer-glyph--soft-agreement"],
      [2, "hollow_flattery",     "composer-glyph--hollow-flattery"],
      [3, "submission",          "composer-glyph--submission"],
    ];
    for (const [tier, label, cls] of cases) {
      el._glyphController.score = {
        sycophancy: { tier, label, composite: 10 + tier * 20 },
      };
      el.requestUpdate();
      await el.updateComplete;
      const glyph = host.querySelector(".composer-glyph");
      expect(glyph, `tier ${tier} renders`).not.toBeNull();
      expect(glyph!.classList.contains(cls), `tier ${tier} class hook`).toBe(true);
    }
  });

  it("tooltip appends suggestion count when suggestions[] is populated", async () => {
    const { host, el } = await mountWindow<HTMLElement & {
      updateComplete: Promise<boolean>;
      requestUpdate: () => void;
      _glyphController: { score: unknown };
    }>("lthn-chat-window");
    el._glyphController.score = {
      sycophancy: { tier: 1, label: "soft_agreement", composite: 22 },
      suggestions: [
        { type: "compliance_marker", span: [0, 4], severity: "low", note: "soft hedge" },
        { type: "formulaic_preamble", span: [5, 12], severity: "info", note: "preamble" },
      ],
    };
    el.requestUpdate();
    await el.updateComplete;
    const glyph = host.querySelector(".composer-glyph");
    expect(glyph!.getAttribute("title")).toContain("2 suggestions");
  });
});

// — ChatTurnScoreController integration with chat-window. The controller
// itself is covered by chat-turn-score.controller.test.ts; here we just
// assert the window's render helper paints the indicator on assistant
// messages when a populated DiffResult is in the cache, hides it for
// pending / null, and crucially does NOT modify the assistant
// message rendering. We poke `el._turnScoreController.cache` directly
// rather than firing the ScorePair pipeline — the controller is mocked
// in its own file; the window's job is wiring.

describe("lthn-chat-window — per-turn ethics indicator", () => {
  /** Seed `liveTurns` with [user, model] so turnIndex=1 has a valid
   *  prompt at index 0 and the indicator render path actually fires.
   *  Mirrors the shape of the post-Sessions.Read() data the host
   *  normally renders from. */
  async function mountWithTurns() {
    type WindowEl = HTMLElement & {
      updateComplete: Promise<boolean>;
      requestUpdate: () => void;
      liveTurns: ChatTurn[] | null;
      activeConversationId: string | null;
      _turnScoreController: {
        cache: Map<number, unknown>;
      };
    };
    const { host, el } = await mountWindow<WindowEl>("lthn-chat-window");
    // Bypass the activeConversationId === null short-circuit so
    // _renderSurface paints liveTurns instead of the fixture.
    el.activeConversationId = "test-conv";
    el.liveTurns = [
      { role: "you", text: "explain the spec" },
      { role: "model", text: "you're absolutely right, the spec says…" },
    ];
    el.requestUpdate();
    await el.updateComplete;
    return { host, el };
  }

  it("renders turn-score indicator on assistant-message footer when score resolves", async () => {
    const { host, el } = await mountWithTurns();
    // Seed the cache as if a ScorePair already resolved for turn 1.
    el._turnScoreController.cache.set(1, {
      prompt: {
        sycophancy: { tier: 0, label: "appropriate_empathy", composite: 4 },
      },
      response: {
        sycophancy: { tier: 2, label: "hollow_flattery", composite: 58 },
      },
      differential: {
        echo: 0.62,
        verb_shift: 0.18,
        tense_shift: 0.04,
        noun_echo: 0.71,
        question_flip: 0,
        domain_shift: 0.12,
      },
      authority: {
        targets: ["the spec"],
        deference: 0.78,
        pattern: "deference",
      },
    });
    el.requestUpdate();
    await el.updateComplete;

    const indicator = host.querySelector(".turn-score");
    expect(indicator, "indicator node renders when score is populated").not.toBeNull();
    expect(indicator!.classList.contains("turn-score--hollow-flattery"),
      "tier-2 class hook applied").toBe(true);
    // Tooltip carries the human-readable summary.
    const title = indicator!.getAttribute("title");
    expect(title).toContain("hollow_flattery");
    expect(title).toContain("echo 62%");
    expect(title).toContain("authority: deference");
    expect(indicator!.getAttribute("aria-label")).toContain("hollow_flattery");
  });

  it("hides indicator when cache slot is 'pending' or null", async () => {
    const { host, el } = await mountWithTurns();
    // Pending slot — render helper returns nothing.
    el._turnScoreController.cache.set(1, "pending");
    el.requestUpdate();
    await el.updateComplete;
    expect(host.querySelector(".turn-score"), "no indicator while pending").toBeNull();

    // Null slot — still nothing.
    el._turnScoreController.cache.set(1, null);
    el.requestUpdate();
    await el.updateComplete;
    expect(host.querySelector(".turn-score"), "no indicator when slot is null").toBeNull();
  });

  it("does NOT modify the assistant message rendering — even at tier-3 (submission)", async () => {
    const { host, el } = await mountWithTurns();
    el._turnScoreController.cache.set(1, {
      prompt: {
        sycophancy: { tier: 0, label: "appropriate_empathy", composite: 2 },
      },
      response: {
        sycophancy: { tier: 3, label: "submission", composite: 94 },
      },
      differential: null,
      authority: null,
    });
    el.requestUpdate();
    await el.updateComplete;

    // The indicator is rendered…
    const indicator = host.querySelector(".turn-score--submission");
    expect(indicator, "indicator present at tier-3").not.toBeNull();

    // …but the assistant message body, model name, and action buttons
    // are ALL still in the DOM and visible. The message lands first;
    // the dot is informational only.
    expect(host.textContent, "model response text still rendered")
      .toContain("you're absolutely right");
    // The user prompt is also visible (proves the transcript wasn't
    // collapsed / replaced by a warning panel).
    expect(host.textContent, "user prompt still rendered")
      .toContain("explain the spec");
    // Copy + regenerate buttons remain — the message is interactive.
    const copyIcon = host.querySelector("i.fa-copy");
    expect(copyIcon, "copy affordance still present at tier-3").not.toBeNull();
  });
});
