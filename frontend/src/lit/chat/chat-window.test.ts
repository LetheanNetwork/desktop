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

describe("/export-all slash command", () => {
  type ChatWindowEl = HTMLElement & {
    slashMenuOpen: boolean;
    _slashExportAll: () => Promise<void>;
    _exportAllTo: (dir: string) => Promise<number>;
    updateComplete: Promise<boolean>;
  };

  it("/export-all slash entry is present in the menu", async () => {
    const { el, host } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el.slashMenuOpen = true;
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
    const { el, host } = await mountWindow<HTMLElement & { helpOpen: boolean; updateComplete: Promise<boolean> }>("lthn-chat-window");
    el.helpOpen = true;
    await el.updateComplete;
    const text = host.textContent || "";
    expect(text).toContain("/export-all");
    expect(text).toContain("Back up every conversation");
  });
});

describe("/help slash command", () => {
  type ChatWindowEl = HTMLElement & {
    slashMenuOpen: boolean;
    helpOpen: boolean;
    contextMenuFor: { id: string; x: number; y: number } | null;
    _slashHelp: () => void;
    updateComplete: Promise<boolean>;
  };

  it("/help slash item opens the help overlay + closes the slash menu", async () => {
    const { el, host } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el.slashMenuOpen = true;
    await el.updateComplete;

    const help = host.querySelector<HTMLElement>(".lthn-chat-slash-help");
    expect(help, "help slash entry rendered").not.toBeNull();
    help!.click();
    await el.updateComplete;

    expect(el.slashMenuOpen, "slash menu closes on /help click").toBe(false);
    expect(el.helpOpen, "help overlay opens").toBe(true);
    expect(host.querySelector(".lthn-chat-help"), "help dialog rendered").not.toBeNull();
    host.remove();
  });

  it("help overlay lists keyboard shortcuts + slash commands", async () => {
    const { el, host } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el.helpOpen = true;
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
    el.helpOpen = true;
    await el.updateComplete;

    const close = host.querySelector<HTMLButtonElement>(".lthn-chat-help-close");
    expect(close, "close button present").not.toBeNull();
    close!.click();
    await el.updateComplete;

    expect(el.helpOpen, "close button → overlay dismissed").toBe(false);
    host.remove();
  });

  it("clicking the backdrop dismisses the overlay", async () => {
    const { el, host } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el.helpOpen = true;
    await el.updateComplete;

    const backdrop = host.querySelector<HTMLElement>(".lthn-chat-help-backdrop");
    expect(backdrop, "backdrop present").not.toBeNull();
    backdrop!.click();
    await el.updateComplete;

    expect(el.helpOpen, "backdrop click → overlay dismissed").toBe(false);
    host.remove();
  });

  it("clicking inside the dialog does NOT dismiss it (stop-propagation)", async () => {
    const { el, host } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el.helpOpen = true;
    await el.updateComplete;

    const dialog = host.querySelector<HTMLElement>(".lthn-chat-help");
    dialog!.click();
    await el.updateComplete;

    expect(el.helpOpen, "inner click stays open").toBe(true);
    host.remove();
  });

  it("Escape priority: context menu first, help overlay second, slash third", async () => {
    const { el, host } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el.contextMenuFor = { id: "x", x: 0, y: 0 };
    el.helpOpen = true;
    el.slashMenuOpen = true;
    await el.updateComplete;

    // First Escape closes context menu, leaves help + slash open.
    window.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
    await el.updateComplete;
    expect(el.contextMenuFor).toBeNull();
    expect(el.helpOpen).toBe(true);
    expect(el.slashMenuOpen).toBe(true);

    // Second Escape closes help overlay, leaves slash open.
    window.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
    await el.updateComplete;
    expect(el.helpOpen).toBe(false);
    expect(el.slashMenuOpen).toBe(true);

    // Third Escape closes slash menu.
    window.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
    await el.updateComplete;
    expect(el.slashMenuOpen).toBe(false);
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
    contextMenuFor: { id: string; x: number; y: number } | null;
    slashMenuOpen: boolean;
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

    expect(el.contextMenuFor).not.toBeNull();
    expect(el.contextMenuFor!.id).toBe("ctx-target");
    expect(el.contextMenuFor!.x).toBe(120);
    expect(el.contextMenuFor!.y).toBe(80);
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
    el.contextMenuFor = { id: "ctx-target", x: 0, y: 0 };
    el.activeConversationId = "ctx-target";
    el.renamingId = null;
    await el.updateComplete;

    const renameBtn = host.querySelector<HTMLButtonElement>(".lthn-conversation-context-rename")!;
    renameBtn.click();
    await el.updateComplete;

    expect(el.renamingId).toBe("ctx-target");
    expect(el.contextMenuFor, "menu closes after action").toBeNull();
    host.remove();
  });

  it("Pin item toggles the pin set + closes the menu", async () => {
    const { el, host } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el.conversations = [seed];
    el.pinnedConversations = new Set();
    el.contextMenuFor = { id: "ctx-target", x: 0, y: 0 };
    el.activeConversationId = "ctx-target";
    await el.updateComplete;

    host.querySelector<HTMLButtonElement>(".lthn-conversation-context-pin")!.click();
    await el.updateComplete;

    expect(el.pinnedConversations.has("ctx-target"), "pin added").toBe(true);
    expect(el.contextMenuFor, "menu closes").toBeNull();

    // Re-opening the menu shows "Unpin" (the label flips based on pin state).
    el.contextMenuFor = { id: "ctx-target", x: 0, y: 0 };
    await el.updateComplete;
    const pinItem = host.querySelector(".lthn-conversation-context-pin");
    expect(pinItem?.textContent).toContain("Unpin");
    host.remove();
  });

  it("Escape closes the context menu (priority over slash menu)", async () => {
    const { el, host } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el.contextMenuFor = { id: "ctx-target", x: 0, y: 0 };
    el.slashMenuOpen = true;
    await el.updateComplete;

    window.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
    await el.updateComplete;

    expect(el.contextMenuFor, "Escape closes context menu first").toBeNull();
    expect(el.slashMenuOpen, "slash menu stays open this round — context menu took the key").toBe(true);

    // Second Escape should now reach the slash menu.
    window.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
    await el.updateComplete;
    expect(el.slashMenuOpen, "second Escape closes slash menu").toBe(false);
    host.remove();
  });

  it("Duplicate item is present in the context menu", async () => {
    const { el, host } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el.conversations = [seed];
    el.contextMenuFor = { id: "ctx-target", x: 0, y: 0 };
    el.activeConversationId = "ctx-target";
    await el.updateComplete;

    const dup = host.querySelector(".lthn-conversation-context-duplicate");
    expect(dup, "Duplicate item rendered").not.toBeNull();
    expect(dup?.textContent).toContain("Duplicate");
    host.remove();
  });

  it("clicking outside the menu closes it", async () => {
    const { el, host } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el.contextMenuFor = { id: "ctx-target", x: 0, y: 0 };
    await el.updateComplete;

    // Click on document.body (a node OUTSIDE the menu's composedPath).
    document.body.dispatchEvent(new MouseEvent("click", { bubbles: true, composed: true }));
    await el.updateComplete;

    expect(el.contextMenuFor, "outside-click dismisses menu").toBeNull();
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
    helpOpen: boolean;
    updateComplete: Promise<boolean>;
  };

  it("bare ? opens the help overlay", async () => {
    const { el } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    expect(el.helpOpen).toBe(false);
    window.dispatchEvent(new KeyboardEvent("keydown", { key: "?", bubbles: true }));
    await el.updateComplete;
    expect(el.helpOpen).toBe(true);
  });

  it("second ? closes the help overlay (toggle)", async () => {
    const { el } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el.helpOpen = true;
    await el.updateComplete;
    window.dispatchEvent(new KeyboardEvent("keydown", { key: "?", bubbles: true }));
    await el.updateComplete;
    expect(el.helpOpen).toBe(false);
  });

  it("? does NOT open help when focus is in an input", async () => {
    const { el, host } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el.helpOpen = false;
    await el.updateComplete;

    const input = host.querySelector<HTMLInputElement>("input.lthn-conversation-search")!;
    expect(input, "rail search input present").not.toBeNull();
    input.focus();
    input.dispatchEvent(new KeyboardEvent("keydown", { key: "?", bubbles: true }));
    await el.updateComplete;
    expect(el.helpOpen, "? in input must not trigger help").toBe(false);
  });

  it("cheat sheet lists ? under Composer", async () => {
    const { el, host } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el.helpOpen = true;
    await el.updateComplete;

    const text = host.textContent || "";
    expect(text).toContain("Show this cheat sheet");
  });
});

describe("⌘F find in transcript", () => {
  type ChatWindowEl = HTMLElement & {
    findOpen: boolean;
    findQuery: string;
    liveTurns: ChatTurn[] | null;
    activeConversationId: string | null;
    state: string;
    slashMenuOpen: boolean;
    helpOpen: boolean;
    contextMenuFor: { id: string; x: number; y: number } | null;
    _findMatchCount: () => number;
    updateComplete: Promise<boolean>;
  };

  it("⌘F opens the find bar", async () => {
    const { el, host } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    expect(el.findOpen).toBe(false);

    window.dispatchEvent(new KeyboardEvent("keydown", { key: "f", metaKey: true, bubbles: true }));
    await el.updateComplete;

    expect(el.findOpen).toBe(true);
    expect(host.querySelector(".lthn-chat-find"), "find bar rendered").not.toBeNull();
    expect(host.querySelector(".lthn-chat-find-input"), "find input rendered").not.toBeNull();
    host.remove();
  });

  it("Ctrl+F triggers the same toggle", async () => {
    const { el, host } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    window.dispatchEvent(new KeyboardEvent("keydown", { key: "f", ctrlKey: true, bubbles: true }));
    await el.updateComplete;
    expect(el.findOpen).toBe(true);
    host.remove();
  });

  it("⌘F again closes the find bar + clears the query", async () => {
    const { el, host } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el.findOpen = true;
    el.findQuery = "regex";
    await el.updateComplete;

    window.dispatchEvent(new KeyboardEvent("keydown", { key: "f", metaKey: true, bubbles: true }));
    await el.updateComplete;

    expect(el.findOpen).toBe(false);
    expect(el.findQuery).toBe("");
    host.remove();
  });

  it("Escape closes the find bar + clears the query", async () => {
    const { el, host } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el.findOpen = true;
    el.findQuery = "anchor";
    await el.updateComplete;

    window.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
    await el.updateComplete;

    expect(el.findOpen).toBe(false);
    expect(el.findQuery).toBe("");
    host.remove();
  });

  it("_findMatchCount tallies every occurrence across all turns", async () => {
    const { el } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el.findOpen = true;
    el.findQuery = "regex";
    el.liveTurns = [
      { role: "you", text: "regex regex regex" },
      { role: "assistant", text: "regex matters when patterns repeat" },
      { role: "you", text: "no match here" },
    ];
    // 3 in first + 1 in second + 0 in third = 4
    expect(el._findMatchCount()).toBe(4);
  });

  it("_findMatchCount returns 0 when find is closed", async () => {
    const { el } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el.findOpen = false;
    el.findQuery = "regex";
    el.liveTurns = [{ role: "you", text: "regex regex" }];
    expect(el._findMatchCount()).toBe(0);
  });

  it("_findMatchCount returns 0 for blank query", async () => {
    const { el } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el.findOpen = true;
    el.findQuery = "   ";
    el.liveTurns = [{ role: "you", text: "anything" }];
    expect(el._findMatchCount()).toBe(0);
  });

  it("close button dismisses the bar", async () => {
    const { el, host } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el.findOpen = true;
    el.findQuery = "x";
    await el.updateComplete;

    const close = host.querySelector<HTMLButtonElement>(".lthn-chat-find-close");
    expect(close).not.toBeNull();
    close!.click();
    await el.updateComplete;

    expect(el.findOpen).toBe(false);
    expect(el.findQuery).toBe("");
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
    el.findOpen = true;
    el.findQuery = "bravo";
    await el.updateComplete;

    const matches = host.querySelectorAll(".lthn-chat-find-match");
    expect(matches.length, "every bravo occurrence in transcript gets a highlight span").toBeGreaterThanOrEqual(2);
    expect(Array.from(matches).every(m => m.textContent === "bravo")).toBe(true);
    host.remove();
  });

  it("counter renders 'no matches' when the query has zero hits", async () => {
    const { el, host } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el.findOpen = true;
    el.findQuery = "ghost-needle";
    el.liveTurns = [{ role: "you", text: "alpha bravo" }];
    await el.updateComplete;

    const count = host.querySelector(".lthn-chat-find-count");
    expect(count?.textContent?.trim()).toBe("no matches");
    host.remove();
  });

  it("Escape priority: context → help → find → slash", async () => {
    const { el, host } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el.contextMenuFor = { id: "x", x: 0, y: 0 };
    el.helpOpen = true;
    el.findOpen = true;
    el.findQuery = "x";
    el.slashMenuOpen = true;
    await el.updateComplete;

    window.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
    await el.updateComplete;
    expect(el.contextMenuFor).toBeNull();
    expect(el.helpOpen).toBe(true);
    expect(el.findOpen).toBe(true);
    expect(el.slashMenuOpen).toBe(true);

    window.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
    await el.updateComplete;
    expect(el.helpOpen).toBe(false);
    expect(el.findOpen).toBe(true);

    window.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
    await el.updateComplete;
    expect(el.findOpen).toBe(false);
    expect(el.slashMenuOpen).toBe(true);

    window.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
    await el.updateComplete;
    expect(el.slashMenuOpen).toBe(false);
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
    findOpen: boolean;
    findQuery: string;
    findCursor: number;
    liveTurns: ChatTurn[] | null;
    activeConversationId: string | null;
    state: string;
    _findMatchCount: () => number;
    _findStep: (delta: number) => void;
    updateComplete: Promise<boolean>;
  };

  it("counter renders 'X of N' when matches exist", async () => {
    const { el, host } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el.findOpen = true;
    el.findQuery = "regex";
    el.liveTurns = [
      { role: "you", text: "regex regex regex" },
      { role: "assistant", text: "regex matters" },
    ];
    el.findCursor = 0;
    await el.updateComplete;

    const count = host.querySelector(".lthn-chat-find-count");
    expect(count?.textContent?.trim()).toBe("1 of 4");

    el.findCursor = 2;
    await el.updateComplete;
    expect(host.querySelector(".lthn-chat-find-count")?.textContent?.trim()).toBe("3 of 4");
    host.remove();
  });

  it("_findStep cycles forward with wrap", async () => {
    const { el } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el.findOpen = true;
    el.findQuery = "a";
    el.liveTurns = [{ role: "you", text: "aaa" }]; // 3 matches
    el.findCursor = 0;

    el._findStep(1);
    expect(el.findCursor).toBe(1);
    el._findStep(1);
    expect(el.findCursor).toBe(2);
    el._findStep(1);
    expect(el.findCursor, "wrap forward to 0").toBe(0);
  });

  it("_findStep cycles backward with wrap", async () => {
    const { el } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el.findOpen = true;
    el.findQuery = "a";
    el.liveTurns = [{ role: "you", text: "aaa" }];
    el.findCursor = 0;

    el._findStep(-1);
    expect(el.findCursor, "wrap backward to the last").toBe(2);
    el._findStep(-1);
    expect(el.findCursor).toBe(1);
  });

  it("_findStep is a no-op when there are no matches", async () => {
    const { el } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el.findOpen = true;
    el.findQuery = "ghost";
    el.liveTurns = [{ role: "you", text: "no match here" }];
    el.findCursor = 0;
    el._findStep(1);
    expect(el.findCursor).toBe(0);
  });

  it("changing the query resets findCursor to 0", async () => {
    const { el } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el.findOpen = true;
    el.findQuery = "regex";
    el.liveTurns = [{ role: "you", text: "regex regex" }];
    el.findCursor = 1;
    await el.updateComplete;

    el.findQuery = "different";
    await el.updateComplete;
    expect(el.findCursor, "new query → cursor 0").toBe(0);
  });

  it("next-match button steps the cursor forward", async () => {
    const { el, host } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el.findOpen = true;
    el.findQuery = "a";
    el.liveTurns = [{ role: "you", text: "aaa" }];
    el.findCursor = 0;
    await el.updateComplete;

    const next = host.querySelector<HTMLButtonElement>(".lthn-chat-find-next");
    next!.click();
    await el.updateComplete;
    expect(el.findCursor).toBe(1);
    host.remove();
  });

  it("_scrollFindActiveIntoView calls scrollIntoView on the active match", async () => {
    const { el, host } = await mountWindow<ChatWindowEl & {
      _scrollFindActiveIntoView: () => void;
    }>("lthn-chat-window");
    el.state = "multi-turn";
    el.activeConversationId = "find-scroll";
    el.liveTurns = [{ role: "you", text: "alpha alpha alpha" }];
    el.findOpen = true;
    el.findQuery = "alpha";
    el.findCursor = 1;
    await el.updateComplete;

    const active = host.querySelector(".lthn-chat-find-active") as HTMLElement;
    expect(active, "active match present").not.toBeNull();
    let calls = 0;
    active.scrollIntoView = () => { calls += 1; };
    el._scrollFindActiveIntoView();
    expect(calls, "scrollIntoView fired exactly once").toBe(1);
    host.remove();
  });

  it("_scrollFindActiveIntoView is a no-op when find is closed", async () => {
    const { el } = await mountWindow<ChatWindowEl & {
      _scrollFindActiveIntoView: () => void;
    }>("lthn-chat-window");
    el.findOpen = false;
    let threw: unknown = null;
    try { el._scrollFindActiveIntoView(); }
    catch (e) { threw = e; }
    expect(threw, "closed find must not throw").toBeNull();
  });

  it("_scrollFindActiveIntoView is a no-op when there is no active span", async () => {
    const { el } = await mountWindow<ChatWindowEl & {
      _scrollFindActiveIntoView: () => void;
    }>("lthn-chat-window");
    el.findOpen = true;
    el.findQuery = "regex";
    el.liveTurns = []; // No matches → no active span
    await el.updateComplete;
    let threw: unknown = null;
    try { el._scrollFindActiveIntoView(); }
    catch (e) { threw = e; }
    expect(threw).toBeNull();
  });

  it("renders an active class on the cursor-current match span", async () => {
    const { el, host } = await mountWindow<ChatWindowEl>("lthn-chat-window");
    el.state = "multi-turn";
    el.activeConversationId = "find-active";
    el.liveTurns = [{ role: "you", text: "alpha alpha alpha" }];
    el.findOpen = true;
    el.findQuery = "alpha";
    el.findCursor = 1; // middle match
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
      helpOpen: boolean;
      updateComplete: Promise<boolean>;
    }>("lthn-chat-window");
    el.helpOpen = true;
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
      slashMenuOpen: boolean;
      updateComplete: Promise<boolean>;
    }>("lthn-chat-window");
    el.slashMenuOpen = true;
    await el.updateComplete;

    window.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
    await el.updateComplete;

    expect(el.slashMenuOpen, "Escape closes an open slash menu").toBe(false);
    host.remove();
  });

  it("Escape with menu closed does not toggle it", async () => {
    const { el, host } = await mountWindow<HTMLElement & {
      slashMenuOpen: boolean;
      updateComplete: Promise<boolean>;
    }>("lthn-chat-window");
    el.slashMenuOpen = false;
    await el.updateComplete;

    window.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
    await el.updateComplete;

    expect(el.slashMenuOpen, "Escape must not flip closed-to-open").toBe(false);
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
