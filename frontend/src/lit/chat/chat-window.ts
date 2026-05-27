// SPDX-Licence-Identifier: EUPL-1.2
// E0 · chat — <lthn-chat-window>
// Light-DOM Lit element. Composes renderChrome() from ../chrome.js.

import { LitElement, html, nothing } from "lit";
import { ref as litRef } from "lit/directives/ref.js";
import { renderChrome } from "../chrome";
import { T } from "@lthn/i18n/coreservice";
import type {
  ChatState, ChatStateData, ChatTurn, ChatBanner, ChatComposer,
  Conversation, RailData, RailMode, RightRailMode,
} from "../types";

/* ── data fixtures (shared with the React version) ────────────────── */
const CONVERSATIONS: Conversation[] = [
  { id:"c1", bucket:"today",     title:"Refactor the embed loop",  snippet:"Looks like the issue is the closure capturing…", model:"Gemma 4 E2B" },
  { id:"c2", bucket:"today",     title:"Brief read · drone bill",  snippet:"Two paragraphs summarising the key clauses.",   model:"Llama 3.2 3B" },
  { id:"c3", bucket:"yesterday", title:"JSON to TOML",             snippet:"Here's the converted config in TOML…",         model:"Gemma 4 E2B" },
  { id:"c4", bucket:"yesterday", title:"Vi voice samples",         snippet:"Three drafts in plain-spoken register.",       model:"Gemma 4 E2B" },
  { id:"c5", bucket:"week",      title:"Tokeniser benchmarks",     snippet:"PP throughput on M3 Pro vs M4 Air…",           model:"Gemma 4 E2B" },
  { id:"c6", bucket:"week",      title:"Onboarding microcopy",     snippet:"Calm-presence voice across the welcome flow.", model:"Llama 3.2 3B" },
];

const TURNS_MULTI: ChatTurn[] = [
  { role:"you",   text:"Walk me through how this Go embed loop closes over its loop variable. The captured value is wrong on every iteration." },
  { role:"model", text:"The loop variable is shared across iterations — every closure captures the same address, so by the time the goroutines fire, they all see the final value.\n\nGo 1.22 changed this so each iteration gets its own copy. If you're on 1.21 or earlier, you need to shadow the variable explicitly.",
    code:{ lang:"go", text:"for _, item := range items {\n    item := item // shadow for the closure\n    go func() {\n        process(item)\n    }()\n}" } },
  { role:"you",   text:"Right, so just `item := item` before the goroutine. Why does the runtime not see this as a redundant assignment?" },
  { role:"model", text:"Because it isn't redundant — it creates a new variable in the inner scope. The compiler treats the inner `item` as a distinct binding; the closure captures THAT one. The optimiser can't elide it because the goroutine outlives the iteration.",
    citations:["go.dev/ref/spec#Variable_scope", "go.dev/blog/loopvar-preview"] },
];

const TURNS_GEN: ChatTurn[] = [
  ...TURNS_MULTI.slice(0, 2),
  { role:"you",   text:"Now write the test that would have caught this." },
  { role:"model", text:"A table-driven test that fires each goroutine and asserts the captured value matches the iteration — running it under `-race` proves the closure capture rather than just timing luck.\n\n" },
];

// highlightMatch lives in ../highlight (shared with model-browser);
// pure helpers + constants + types live in ./chat-window.helpers
// (Athena 2026-05-16 architecture review). Both re-exported so
// existing imports keep resolving from this canonical module path.
import { highlightMatch } from "../highlight";
import { ComposerHistoryController } from "./composer-history.controller";
import { ComposerGlyphController, TIER_NAMES } from "./composer-glyph.controller";
import { ChatTurnScoreController, TIER_NAMES as TURN_TIER_NAMES } from "./chat-turn-score.controller";
import { FindController } from "./find.controller";
import { SlashMenuController } from "./slash-menu.controller";
import { ContextMenuController } from "./context-menu.controller";
import { HelpOverlayController } from "./help-overlay.controller";
// Pure helpers + persistence + constants live in chat-window.helpers
// (Athena split 2026-05-16). Import only what the class methods +
// render templates actually call internally; the rest is re-exported
// below for external consumers.
import {
  buildTranscript, composerCounterText, composerCounterTone,
  COMPOSER_COUNTER_WARN_AT, COMPOSER_MAX_HEIGHT,
  deriveAutoTitle, findMatchPositions, findSplitTurn,
  flattenForDisplay,
  loadActiveConversation, loadPinnedConversations,
  matchesSearch, matchesTagFilter, parseTagInput, relativeTime,
  saveActiveConversation, savePinnedConversations, splitPinnedConversations,
  tagsToInput, withSystemPrompt,
} from "./chat-window.helpers";
export { highlightMatch, type HighlightSegment } from "../highlight";
export {
  AUTO_TITLE_MAX, ACTIVE_CONVERSATION_KEY,
  buildTranscript, composerCounterText, composerCounterTone,
  COMPOSER_COUNTER_WARN_AT, COMPOSER_MAX_HEIGHT,
  deriveAutoTitle, findMatchPositions, findSplitTurn,
  flattenForDisplay,
  loadActiveConversation, loadPinnedConversations,
  matchesSearch, matchesTagFilter, parseTagInput,
  PINNED_CONVERSATIONS_KEY, RAIL_BUCKET_ORDER, relativeTime,
  saveActiveConversation, savePinnedConversations,
  splitPinnedConversations, tagsToInput, userMessageHistory,
  withSystemPrompt,
  type ComposerCounterTone, type FindPosition, type FindSegment,
} from "./chat-window.helpers";

/* — shared style fragment for context-menu items so each <button>
 *  doesn't duplicate the layout. Optional colour override is for the
 *  destructive Delete item. */
function ctxItemStyle(colour: string = "var(--fg-0)"): string {
  return [
    "width:100%; display:flex; align-items:center; gap:10px;",
    "padding:7px 10px;",
    "background:transparent; border:none; border-radius:5px;",
    "font:inherit; font-size:12px; text-align:left;",
    `color:${colour};`,
    "cursor:pointer; --wails-draggable: no-drag;",
  ].join(" ");
}

/* ── per-state derived props ──────────────────────────────────────── */
//
// Rail data — live tok/s + watts + KV-hit + token-window aren't sourced
// yet (runner.WChat is a one-shot RPC, no streaming metering surface).
// All values are "—" until per-turn telemetry lands; the previous
// state-keyed fixture numbers ("47.2", "12.4 W", "94%", "1,284 / 4,096")
// were demo-canvas leftovers that lied to users about live stats.
function chatStateData(state: ChatState): ChatStateData {
  const railData: Record<ChatState, RailData> = {
    empty:            { toksLive:"—", watts:"—", kvHit:"—", tokens:"—", ctx:"—" },
    generating:       { toksLive:"—", watts:"—", kvHit:"—", tokens:"—", ctx:"—" },
    "multi-turn":     { toksLive:"—", watts:"—", kvHit:"—", tokens:"—", ctx:"—" },
    "switched-model": { toksLive:"—", watts:"—", kvHit:"—", tokens:"—", ctx:"—" },
    "no-model":       { toksLive:"—", watts:"—", kvHit:"—", tokens:"—", ctx:"—" },
  };

  const turns: Record<ChatState, ChatTurn[] | null> = {
    empty: null,
    generating: TURNS_GEN,
    "multi-turn": TURNS_MULTI,
    "switched-model": TURNS_MULTI,
    "no-model": null,
  };

  const banner: ChatBanner | null =
    state === "switched-model" ? { tone:"warn", text:"Switched to Llama 3.2 3B mid-conversation — KV cache cleared. The next turn will replay context.", action:"Restore Gemma" } :
    state === "no-model"       ? { tone:"warn", text:"No model loaded. Pick one from the tray to start composing.", action:"Open tray" } :
    null;

  const composer: Record<ChatState, ChatComposer> = {
    empty:            { value:"" },
    generating:       { value:"Now write the test that would have caught this.", sending:true, hint:"Esc · stop" },
    "multi-turn":     { value:"", hint:"⌘↵ · send" },
    "switched-model": { value:"", hint:"⌘↵ · send" },
    "no-model":       { value:"", disabled:true },
  };

  const toolbarModel: Record<ChatState, string> = {
    empty:"Gemma 4 E2B", generating:"Gemma 4 E2B",
    "multi-turn":"Gemma 4 E2B", "switched-model":"Llama 3.2 3B",
    "no-model":"No model",
  };

  return {
    railData: railData[state],
    turns: turns[state],
    banner,
    composer: composer[state],
    toolbarModel: toolbarModel[state],
  };
}

class LthnChatWindow extends LitElement {
  static readonly properties = {
    state:     { type: String, reflect: true },
    rail:      { type: String, reflect: true },
    rightRail: { type: String, attribute: "right-rail", reflect: true },
    railSection: { state: true },
    w:         { type: Number },
    h:         { type: Number },
    // Without this static-properties entry Lit won't observe the
    // `embedded=""` attribute that <lthn-app-shell>._instantiate sets
    // on the child element, so renderChrome's embedded branch never
    // fires and the window double-renders inside the shell.
    embedded:  { type: Boolean, reflect: true },
    chrome:    { state: true },
    conversations: { state: true },
    activeConversationId: { state: true },
    renamingId: { state: true },
    searchQuery: { state: true },
    pinnedConversations: { state: true },
    navIndex: { state: true },
    atBottom: { state: true },
    contentMatchedIds: { state: true },
    searchTruncated: { state: true },
    systemPromptDraft: { state: true },
    tagsDraft: { state: true },
    tagFilter: { state: true },
    railErr: { state: true },
    liveTurns: { state: true },
    composerValue: { state: true },
    sending: { state: true },
    sendErr: { state: true },
    activeModel: { state: true },
    version: { state: true },
    runnerCount: { state: true },
    t: { state: true },
  };
  declare state:     ChatState;
  declare rail:      RailMode;
  declare rightRail: RightRailMode;
  /** Which panel the rail shows when expanded. Three sub-sections
   *  the collapsed-state icons each map to: settings (tags +
   *  instructions + sampling), stats (TPS / watts / KV / tokens),
   *  sources (RAG / pinned context cards). Default "settings"
   *  matches the first icon (sliders / bolt) the user sees. */
  declare railSection: "settings" | "stats" | "sources";
  declare w:         number;
  declare h:         number;
  declare embedded: boolean;
  declare chrome:    { title: string; subtitle: string };
  declare conversations: Conversation[];
  declare activeConversationId: string | null;
  /** Id of the conversation currently in inline-rename mode, or null
   *  when nothing is being renamed. The active rail row renders an
   *  <input> instead of a static title when this matches its id. */
  declare renamingId: string | null;
  /** Current value of the conversation-search input. Empty = show
   *  every conversation. Non-empty = case-insensitive substring
   *  filter over title (see matchesSearch below). State-only — the
   *  search isn't persisted across mounts; it's a transient view
   *  filter, not a saved preference. */
  declare searchQuery: string;
  /** Names of conversation ids the user has pinned. Pinned chats
   *  render in their own section above the today/yesterday/week
   *  buckets. Persisted to localStorage under PINNED_CONVERSATIONS_KEY. */
  declare pinnedConversations: Set<string>;
  /** Tri-state: false during the initial connectedCallback / restore
   *  phase, true once we've decided what activeConversationId should
   *  be. Gates the auto-save in updated() — the very first render
   *  fires updated() while active is still null (default), which
   *  would clobber a persisted id with null. Skipping save until
   *  restoration completes prevents that. */
  private _restoreComplete = false;
  /** Highlight index for keyboard navigation in the conversation
   *  rail. -1 = nothing highlighted (default). When the search input
   *  has focus + the user presses ↑/↓, this advances through the
   *  display-ordered filtered list. Enter commits the highlighted
   *  conversation to activeConversationId + clears the highlight. */
  declare navIndex: number;
  /** True when the transcript scroller is at (or within ~16px of) the
   *  bottom. Drives two behaviours: (a) auto-scroll on new turns
   *  arrives only when we were at bottom (so a user reading older
   *  messages doesn't get yanked away), (b) the floating
   *  "↓ Latest" button renders only when we're NOT at bottom. */
  declare atBottom: boolean;
  /** Reference to the live transcript scroll container, captured via
   *  Lit's ref() directive. Null until the first render lands. */
  private _scrollEl: HTMLElement | null = null;
  /** Turn count snapshot used by updated() to detect new arrivals. */
  private _prevTurnCount = 0;
  /** Ids that matched the current searchQuery by message content
   *  (returned by Sessions.Search). UNIONed with the in-memory
   *  title filter when rendering the rail so users can find past
   *  conversations by what was discussed inside them, not just by
   *  title. State-only, reset on every query change. */
  declare contentMatchedIds: Set<string>;
  /** True when the most recent sessions.Search response carried
   *  truncated:true — the backend hit MaxSearchScan (5000) before
   *  examining every session, so the surfaced hits may be partial.
   *  Drives a small inline banner under the rail-search input that
   *  tells the user to narrow their query for full results. Reset on
   *  every new query + on empty-query clears. Mantis #1571 LOW —
   *  closes the A1 truth-to-user gap on the truncated flag. */
  declare searchTruncated: boolean;
  /** Working draft of the active conversation's system prompt. Bound
   *  to the right-rail "Instructions" textarea. Distinct from the
   *  persisted value in Conversation.systemPrompt so the user can
   *  type without each keystroke firing a backend write — the blur
   *  handler / save action commits via SetSystemPrompt. */
  declare systemPromptDraft: string;
  /** Conversation id the systemPromptDraft was initialised against —
   *  guards against stale writes if the user switches conversations
   *  mid-edit. */
  private _systemPromptSource: string = "";
  /** Working draft of the active conversation's tag list, rendered
   *  as the comma-separated string the user is editing. Persisted
   *  via Sessions.SetTags on blur (re-parsed through parseTagInput
   *  to dedupe + lowercase server-side). */
  declare tagsDraft: string;
  /** Conversation id the tagsDraft was initialised against — guards
   *  against stale writes during a mid-edit switch. */
  private _tagsSource: string = "";
  /** Active tag filter — when non-empty, the rail only shows
   *  conversations whose tags include this value. Clicking a tag
   *  chip in a rail row sets it; clicking again on the active filter
   *  pill clears it. Transient state — not persisted across mounts. */
  declare tagFilter: string;
  /** Debounce handle for Sessions.Search — cleared on every keystroke
   *  so only the last query in a burst reaches the backend. */
  private _contentSearchTimer: ReturnType<typeof setTimeout> | null = null;
  /** The query string the most recently FIRED Sessions.Search request
   *  carried. When the response lands, we ignore it unless this still
   *  matches the live searchQuery — race-safe against fast typers. */
  private _contentSearchQuery: string = "";
  /** Composer history navigation — Reactive Controller. Owns the
   *  cursor state + ↑/↓ branches + reset hook. See
   *  composer-history.controller.ts (Athena 2026-05-16, first lane of
   *  the chat-window controllers plan). */
  private _historyController = new ComposerHistoryController(this);
  /** Composer ethics-tier glyph — Reactive Controller. Owns the
   *  debounced contentshield.Score call against the live composer
   *  draft and the resulting ScoreResult that drives the small glyph
   *  rendered next to the send button. See composer-glyph.controller.ts
   *  (Hephaestus 2026-05-20, lane 6 of the chat-window controllers
   *  plan). Render is informational only — never gates the send
   *  action. */
  private _glyphController = new ComposerGlyphController(this);
  /** Per-completed-turn (prompt, response) ethics scoring — Reactive
   *  Controller. Owns the cache of contentshield.ScorePair results
   *  keyed by turn index and drives the small indicator dot painted
   *  next to each assistant-message footer. The dot is informational
   *  only — it NEVER blocks, hides, dims, or modifies the assistant
   *  message rendering. See chat-turn-score.controller.ts (Hephaestus
   *  2026-05-20, sibling to lane 6 composer-glyph). */
  _turnScoreController = new ChatTurnScoreController(this);
  /** Find-in-transcript overlay — Reactive Controller. Owns the open/
   *  query/cursor state, ⌘F + Escape hand-offs, match-count derivation
   *  and the active-match scroll side-effect. See find.controller.ts
   *  (Athena 2026-05-16, lane 2 of the chat-window controllers plan). */
  private _findController = new FindController(this);
  /** Composer slash-menu mechanics — Reactive Controller. Owns the
   *  open flag, toggle, outside-click window listener + Escape hand-off.
   *  Slash-ACTION methods (`_slashNew`, `_slashClear`, etc.) stay on
   *  this host — they mutate conversation state, not menu mechanics
   *  (§6 of the controllers plan). See slash-menu.controller.ts
   *  (Athena 2026-05-16, lane 3 of the chat-window controllers plan). */
  private _slashMenuController = new SlashMenuController(this);
  /** Right-click context-menu mechanics — Reactive Controller. Owns
   *  the open/coords state, openAt(), window outside-click listener,
   *  Escape hand-off and the viewport-clamp position math. The menu
   *  ACTIONS (rename / pin / duplicate / export / delete) stay on
   *  this host — they mutate conversation state, not menu mechanics
   *  (§6 of the controllers plan). See context-menu.controller.ts
   *  (Athena 2026-05-16, lane 4 of the chat-window controllers plan). */
  private _contextMenuController = new ContextMenuController(this);
  /** Keyboard-shortcut cheat-sheet overlay mechanics — Reactive
   *  Controller. Owns the open flag + show/close/toggle + Escape
   *  hand-off. The cheat-sheet CONTENT (the rendered sections +
   *  entries in `_renderHelpOverlay()`) stays on this host — it's a
   *  render concern, not mechanics (§6 of the controllers plan). See
   *  help-overlay.controller.ts (Athena 2026-05-16, lane 5 of the
   *  chat-window controllers plan — final lane, every overlay now
   *  routes through a controller). */
  private _helpOverlayController = new HelpOverlayController(this);
  declare railErr: string;
  declare liveTurns: ChatTurn[] | null;
  declare composerValue: string;
  declare sending: boolean;
  declare sendErr: string;
  declare activeModel: string;
  declare version: string;
  declare runnerCount: number;
  declare t: {
    railSearch: string;
    searchTruncatedHint: string;
    bToday: string; bYesterday: string; bWeek: string;
    railEmpty: string; railNew: string;
    emptyTitle: string; emptyBody: string;
    composerDisabled: string; composerReady: string;
    composerAttach: string; composerSlash: string;
    btnSend: string; btnStop: string;
    railMeta: string;
    statTps: string; statWatts: string; statKv: string; statTokens: string;
    labelSampling: string;
    sampTemp: string; sampTopP: string; sampMaxTok: string; sampContext: string;
    labelSources: string; sourcesEmpty: string;
  };
  constructor() {
    super();
    // Default to "empty" so first-time users see the welcome surface
    // (starter chips + glyph) instead of the demo-canvas Go-loop fixture
    // turns that ship with the "multi-turn" state for design preview.
    // The activeConversationId restore at line ~747 swaps in liveTurns
    // for returning users with prior conversations.
    this.state = "empty";
    this.rail = "filled";
    // Default collapsed: chat surface is the centrepiece; the rail
    // tools (settings / stats / sources) reveal on demand via the
    // three icon buttons in the collapsed strip.
    this.rightRail = "collapsed";
    this.railSection = "settings";
    this.w = 1100;
    this.h = 740; this.embedded = false;
    this.chrome = { title: "lthn · chat", subtitle: "conversation · local" };
    this.conversations = [];
    this.activeConversationId = null;
    this.renamingId = null;
    this.searchQuery = "";
    this.pinnedConversations = loadPinnedConversations();
    this.navIndex = -1;
    this.atBottom = true;
    this.contentMatchedIds = new Set();
    this.searchTruncated = false;
    this.systemPromptDraft = "";
    this.tagsDraft = "";
    this.tagFilter = "";
    this.railErr = "";
    this.liveTurns = null;
    this.composerValue = "";
    this.sending = false;
    this.sendErr = "";
    this.activeModel = "";
    this.version = "0.2.0-rc1";
    this.runnerCount = 1;
    this.t = {
      railSearch: "Search conversations",
      searchTruncatedHint: "Results may be partial — narrow your query for full results.",
      bToday: "Today", bYesterday: "Yesterday", bWeek: "This week",
      railEmpty: "No conversations yet. Start one from the composer.",
      railNew: "New conversation",
      emptyTitle: "What shall we look at?",
      emptyBody:  "Conversations stay on this Mac. Nothing leaves unless you flip on the API server in Settings and a client connects.",
      composerDisabled: "Load a model from the tray to start composing.",
      composerReady:    "Ask anything — runs locally on this Mac. ⌘↵ to send.",
      composerAttach:   "Attach",
      composerSlash:    "Slash commands",
      btnSend:          "Send",
      btnStop:          "Stop",
      railMeta:         "Turn metadata",
      statTps:          "Tok/s · live",
      statWatts:        "Watts · this turn",
      statKv:           "KV cache hit",
      statTokens:       "Tokens used",
      labelSampling:    "Sampling",
      sampTemp:         "Temperature",
      sampTopP:         "Top-p",
      sampMaxTok:       "Max tokens",
      sampContext:      "Context",
      labelSources:     "Sources",
      sourcesEmpty:     "None this turn. Citations appear here when the model grounds an answer.",
    };
  }
  createRenderRoot() { return this; }
  /** Outside-click handler for the slash menu — installed on
   *  connectedCallback, removed on disconnect. Bound once so the
   *  removeEventListener call hits the right reference. */
  /** Global keyboard handler — ⌘K / Ctrl+K focuses the conversation
   *  search input; ⌘N / Ctrl+N creates a fresh conversation.
   *  Mounted on window in connectedCallback so the shortcut works
   *  regardless of where the user is in the chat surface.
   *
   *  Multi-instance safety: only the LATEST chat-window in document
   *  order acts on each event. The shell-mounted instance is always
   *  in the document; tests append a second one for isolation. The
   *  "latest" check ensures the test instance handles its own keys
   *  (it's appended later) without doubling up with the shell. In
   *  production there's only one chat-window so it's always last. */
  private _onKeyDownForCmdK = (ev: KeyboardEvent) => {
    // Multi-instance gate first: only the last-mounted chat-window
    // claims shortcuts, regardless of modifier shape. Two mounted
    // chats would otherwise double-fire ⌘N / ⌘K / F2.
    const all = document.querySelectorAll("lthn-chat-window");
    if (all[all.length - 1] !== this) return;
    // F2 — Finder/Explorer rename convention. No modifier. Skipped
    // when no active conversation or when focus is already inside
    // an editable element (avoid stealing keys mid-typing).
    if (ev.key === "F2") {
      if (!this.activeConversationId) return;
      const tgt = ev.target as Element | null;
      if (tgt instanceof HTMLInputElement || tgt instanceof HTMLTextAreaElement) return;
      ev.preventDefault();
      this._startRename(this.activeConversationId);
      return;
    }
    // ? — universal "show shortcuts" hotkey (Slack / GitHub / many
    // others use this). Bare key, no modifier; skip when focus is in
    // an editable element so the user can actually type "?". Toggle
    // shape: a second ? closes the overlay.
    if (ev.key === "?") {
      const tgt = ev.target as Element | null;
      if (tgt instanceof HTMLInputElement || tgt instanceof HTMLTextAreaElement) return;
      ev.preventDefault();
      this._helpOverlayController.toggle();
      return;
    }
    // Escape — close transient overlays in priority order: context
    // menu (most recently summoned), help overlay, find bar, then
    // slash menu. Higher layers (rename input, dialogs) still get
    // Escape when nothing's open; we only intercept when there's
    // something to dismiss.
    if (ev.key === "Escape") {
      if (this._contextMenuController.tryHandleEscape()) {
        ev.preventDefault();
        return;
      }
      if (this._helpOverlayController.tryHandleEscape()) {
        ev.preventDefault();
        return;
      }
      if (this._findController.tryHandleEscape()) {
        ev.preventDefault();
        return;
      }
      if (this._slashMenuController.tryHandleEscape()) {
        ev.preventDefault();
        return;
      }
    }
    // accel = the platform's "command" modifier. macOS keys carry
    // metaKey; Windows/Linux carry ctrlKey.
    const accel = ev.metaKey || ev.ctrlKey;
    if (!accel) return;
    const key = ev.key.toLowerCase();
    if (key === "k") {
      const input = this.querySelector<HTMLInputElement>("input.lthn-conversation-search");
      if (!input) return;
      ev.preventDefault();
      input.focus();
      input.select();
      return;
    }
    if (key === "n") {
      ev.preventDefault();
      void this._newConversation();
      return;
    }
    if (key === "b") {
      ev.preventDefault();
      this._toggleRightRail();
      return;
    }
    if (this._findController.tryHandleAccel(key)) {
      ev.preventDefault();
      return;
    }
  };

  // _onOutsideClickForSlash moved to SlashMenuController.
  // The controller installs its window-level outside-click listener
  // in hostConnected + strips it in hostDisconnected; no host-side
  // wiring needed (lane 3, Athena 2026-05-16).

  // _onOutsideClickForContext moved to ContextMenuController.
  // The controller installs its window-level outside-click listener
  // in hostConnected + strips it in hostDisconnected; no host-side
  // wiring needed (lane 4, Athena 2026-05-16).

  /** Right-click handler bound per rail row. Captures the conversation
   *  id + viewport coordinates, opens the menu via the controller,
   *  and sets activeConversationId so the rest of the UI tracks the
   *  conversation the user is operating on. The surrounding ceremony
   *  (preventDefault, stopPropagation, slash-menu close, active-id
   *  assignment) is session-state — stays on the host. The menu's
   *  open/coords state lives on ContextMenuController. */
  private _onContextMenuRailRow(ev: MouseEvent, c: Conversation) {
    ev.preventDefault();
    ev.stopPropagation();
    this.activeConversationId = c.id;
    this._contextMenuController.openAt(c.id, ev.clientX, ev.clientY);
    // Close the slash menu if it was open — they're mutually
    // exclusive overlays and the context menu is the newer summon.
    this._slashMenuController.close();
  }

  /** Run a context-menu action then dismiss the menu. Delegator —
   *  the close-after-fire pattern lives on ContextMenuController.
   *  The action callbacks (rename / pin / duplicate / export / delete)
   *  stay on this host because they mutate conversation state, not
   *  menu mechanics. */
  private _runContextMenuAction(fn: () => unknown) {
    this._contextMenuController.runAction(fn);
  }

  /** Set or clear the rail's tag filter. Calling with a tag that's
   *  already active clears (toggle). Calling with a fresh tag
   *  replaces. Empty input clears. Public so chip click handlers +
   *  tests share the same path. */
  _toggleTagFilter(tag: string) {
    const t = (tag || "").trim().toLowerCase();
    if (!t) {
      this.tagFilter = "";
      return;
    }
    this.tagFilter = this.tagFilter === t ? "" : t;
  }

  /** Reload the tags draft from the active conversation's manifest.
   *  Mirror of _loadSystemPromptDraft — keeps the in-progress edit
   *  buffer in sync with the canonical store, skipping when the
   *  source already matches so a typing burst isn't blown away. */
  _loadTagsDraft() {
    const id = this.activeConversationId || "";
    if (!id) {
      this.tagsDraft = "";
      this._tagsSource = "";
      return;
    }
    const conv = this.conversations.find(c => c.id === id);
    const stored = tagsToInput(conv?.tags);
    if (this._tagsSource === id && this.tagsDraft === stored) return;
    this.tagsDraft = stored;
    this._tagsSource = id;
  }

  /** Persist the tag draft via Sessions.SetTags after parsing through
   *  parseTagInput. No-op when the parsed list matches the stored
   *  one, so blur events on unchanged tags don't fire backend writes. */
  async _saveTags() {
    const id = this.activeConversationId;
    if (!id || this._tagsSource !== id) return;
    const conv = this.conversations.find(c => c.id === id);
    const stored = conv?.tags || [];
    const parsed = parseTagInput(this.tagsDraft);
    if (parsed.length === stored.length && parsed.every((t, i) => t === stored[i])) return;
    try {
      const svc = await import("@desktop/sessions/wailsservice");
      const { demand } = await import("../result");
      await demand<unknown>(svc.SetTags(id, parsed));
      await this._reloadRail();
    } catch (err: unknown) {
      console.warn("sessions.SetTags failed:", err);
    }
  }

  /** Reload the system-prompt draft from the active conversation's
   *  manifest. Skips the reload when the draft is already in sync
   *  with the same source id, so a typing burst doesn't get blown
   *  away by a benign re-render. */
  _loadSystemPromptDraft() {
    const id = this.activeConversationId || "";
    if (!id) {
      this.systemPromptDraft = "";
      this._systemPromptSource = "";
      return;
    }
    const conv = this.conversations.find(c => c.id === id);
    const stored = conv?.systemPrompt || "";
    if (this._systemPromptSource === id && this.systemPromptDraft === stored) {
      return;
    }
    this.systemPromptDraft = stored;
    this._systemPromptSource = id;
  }

  /** Persist the system-prompt draft via Sessions.SetSystemPrompt.
   *  Called on textarea blur. Skips when the draft equals the stored
   *  value so unchanged blurs don't fire a no-op backend write. */
  async _saveSystemPrompt() {
    const id = this.activeConversationId;
    if (!id || this._systemPromptSource !== id) return;
    const conv = this.conversations.find(c => c.id === id);
    const stored = conv?.systemPrompt || "";
    const draft = this.systemPromptDraft;
    if (stored === draft) return;
    try {
      const svc = await import("@desktop/sessions/wailsservice");
      const { demand } = await import("../result");
      await demand<unknown>(svc.SetSystemPrompt(id, draft));
      await this._reloadRail();
    } catch (err: unknown) {
      console.warn("sessions.SetSystemPrompt failed:", err);
    }
  }

  /** Threshold in pixels — being within this distance from the bottom
   *  is "at bottom" for auto-scroll purposes. 16px tolerates the
   *  rounding errors browsers introduce on sub-pixel scroll positions
   *  AND the gap created by the last turn's bottom padding. */
  static readonly SCROLL_BOTTOM_THRESHOLD = 16;

  /** Resize the composer textarea to fit its content, capped at
   *  COMPOSER_MAX_HEIGHT so very long pastes don't push the page
   *  off-screen. Wired via @input — height is recomputed each
   *  keystroke. The reset-to-empty pattern (height = "auto" → read
   *  scrollHeight → assign) is the canonical "auto-grow textarea"
   *  recipe; browsers without scrollHeight (none in practice) become
   *  inert here. Public so the suite can drive without dispatching
   *  fake @input events. */
  _autoGrowComposer(ta: HTMLTextAreaElement) {
    if (!ta) return;
    // Clearing height first lets scrollHeight reflect the actual
    // content height rather than the previous expanded box.
    ta.style.height = "auto";
    const next = Math.min(COMPOSER_MAX_HEIGHT, ta.scrollHeight);
    ta.style.height = next + "px";
  }

  /** Recompute atBottom from the live scroll container metrics.
   *  Called from the @scroll handler and from _scrollToBottom after
   *  it nudges scrollTop. Public surface so tests can stub the
   *  container's metrics then trigger the recompute deterministically. */
  _recomputeAtBottom() {
    const el = this._scrollEl;
    if (!el) return;
    const distance = el.scrollHeight - el.scrollTop - el.clientHeight;
    this.atBottom = distance <= LthnChatWindow.SCROLL_BOTTOM_THRESHOLD;
  }

  /** Scroll the transcript container to the bottom. behavior="smooth"
   *  for user-triggered (button click), "auto" for auto-follow on
   *  new turns (smooth feels laggy mid-stream). */
  _scrollToBottom(behavior: ScrollBehavior = "smooth") {
    const el = this._scrollEl;
    if (!el) return;
    // scrollTo with behavior:"smooth" requires animation frames; tests
    // sit in happy-dom which doesn't tick those. The direct scrollTop
    // assignment is the deterministic path.
    el.scrollTop = el.scrollHeight;
    if (behavior === "smooth" && typeof el.scrollTo === "function") {
      try { el.scrollTo({ top: el.scrollHeight, behavior: "smooth" }); } catch { /* no-op */ }
    }
    this.atBottom = true;
  }
  async connectedCallback() {
    super.connectedCallback();
    // SlashMenuController + ContextMenuController self-install their
    // window outside-click listeners in hostConnected — no host-side
    // wiring needed.
    window.addEventListener("keydown", this._onKeyDownForCmdK);
    const [
      title, subtitle, rs, sTrunc, bt, by, bw, re, rn,
      et, eb, cd, cr, ca, cs, bSend, bStop,
      rMeta, sTps, sWatts, sKv, sTokens,
      lSamp, sT, sP, sMax, sCtx,
      lSrc, sEmpty,
    ] = await Promise.all([
      T("window.chat.title"),
      T("window.chat.subtitle"),
      T("window.chat.rail_search"),
      T("window.chat.rail_search_truncated_hint"),
      T("window.chat.rail_bucket_today"),
      T("window.chat.rail_bucket_yesterday"),
      T("window.chat.rail_bucket_week"),
      T("window.chat.rail_empty"),
      T("window.chat.rail_new"),
      T("window.chat.empty_title"),
      T("window.chat.empty_body"),
      T("window.chat.composer_disabled"),
      T("window.chat.composer_ready"),
      T("window.chat.composer_attach"),
      T("window.chat.composer_slash"),
      T("window.chat.btn_send"),
      T("window.chat.btn_stop"),
      T("window.chat.rail_meta"),
      T("window.chat.stat_tps_live"),
      T("window.chat.stat_watts"),
      T("window.chat.stat_kv_hit"),
      T("window.chat.stat_tokens"),
      T("window.chat.label_sampling"),
      T("window.chat.samp_temperature"),
      T("window.chat.samp_top_p"),
      T("window.chat.samp_max_tokens"),
      T("window.chat.samp_context"),
      T("window.chat.label_sources"),
      T("window.chat.sources_empty"),
    ]);
    this.chrome = { title, subtitle };
    this.t = {
      railSearch: rs, searchTruncatedHint: sTrunc,
      bToday: bt, bYesterday: by, bWeek: bw, railEmpty: re, railNew: rn,
      emptyTitle: et, emptyBody: eb,
      composerDisabled: cd, composerReady: cr,
      composerAttach: ca, composerSlash: cs,
      btnSend: bSend, btnStop: bStop,
      railMeta: rMeta,
      statTps: sTps, statWatts: sWatts, statKv: sKv, statTokens: sTokens,
      labelSampling: lSamp,
      sampTemp: sT, sampTopP: sP, sampMaxTok: sMax, sampContext: sCtx,
      labelSources: lSrc, sourcesEmpty: sEmpty,
    };
    // Restore last-active conversation BEFORE _reloadRail. The rail
    // loader's fallback assigns conversations[0] when active is null,
    // which would clobber the saved value via the updated() hook's
    // saveActiveConversation. Setting active up-front lets that
    // guard skip + leaves the saved id in localStorage.
    const saved = loadActiveConversation();
    if (saved) this.activeConversationId = saved;

    await Promise.all([this._reloadRail(), this._reloadModel(), this._reloadBuild()]);

    // Validate: if the restored id is no longer in the rail (the
    // conversation was deleted externally between sessions), fall
    // through to the first-conversation fallback.
    if (this.activeConversationId &&
        !this.conversations.some(c => c.id === this.activeConversationId)) {
      this.activeConversationId = this.conversations[0]?.id || null;
    }
    // Restoration complete — future activeConversationId assignments
    // (user clicks, slash commands, etc.) MUST persist to localStorage.
    this._restoreComplete = true;
    // Sync localStorage to the final restored value so it's a known
    // good state when the next mount reads. (The initial Lit render
    // fired updated() during the suppressed phase, so we owe one save.)
    saveActiveConversation(this.activeConversationId);
  }

  /** Pull the binary version from firstlaunch.Build() so the footer
   *  status line reflects the running binary instead of the design's
   *  v0.2.0-rc1 literal. Errors stay silent — the fallback string
   *  already covers the canvas-preview case. */
  async _reloadBuild() {
    try {
      const svc = await import("@desktop/firstlaunch/wailsservice");
      const { unwrap } = await import("../result");
      const b = await unwrap<{ version?: string } | null>(svc.Build(), null);
      if (b?.version) this.version = b.version;
    } catch { /* keep fallback */ }
  }

  /** Lit lifecycle — strip the outside-click listener + keydown
   *  handler when the element unmounts so we don't leak across
   *  mount/unmount cycles. */
  disconnectedCallback() {
    // SlashMenuController + ContextMenuController self-strip their
    // window outside-click listeners in hostDisconnected — no
    // host-side wiring needed.
    window.removeEventListener("keydown", this._onKeyDownForCmdK);
    // Cancel the 300ms search-debounce timer so a pending fire after
    // unmount doesn't reach into a torn-down element via stale `this`.
    // Each keystroke during typing schedules this; closing the window
    // mid-typing leaves it pending and the timer holds the element.
    if (this._contentSearchTimer) {
      clearTimeout(this._contentSearchTimer);
      this._contentSearchTimer = null;
    }
    super.disconnectedCallback();
  }

  /** Pull the runner's loaded model list and pick the first one for
   *  the toolbar + footer. Empty list → blank string so the render
   *  falls back to the fixture label ("No model" / etc.). Also reads
   *  the configured route count for the toolbar's "N runner" slot. */
  async _reloadModel() {
    try {
      const svc = await import("@desktop/runner/service");
      const { unwrap } = await import("../result");
      const [models, routes] = await Promise.all([
        unwrap<string[]>(svc.WModels(), []),
        unwrap<unknown[]>(svc.WRoutes(), []),
      ]);
      this.activeModel = models?.[0] || "";
      if ((routes?.length ?? 0) > 0) this.runnerCount = routes.length;
    } catch {
      this.activeModel = "";
    }
  }

  /** Lit lifecycle — when activeConversationId changes, reload
   *  the live turns from sessions.Read so the transcript matches
   *  the selected conversation. */
  updated(changed: Map<string, unknown>) {
    if (changed.has("activeConversationId")) {
      // Persist on every assignment so the next mount can restore.
      // Includes null → clears the key, which is what we want when a
      // delete cascade rolls forward to "nothing".
      //
      // Gate on _restoreComplete: the FIRST render (during initial
      // connectedCallback) fires updated() while active is still the
      // null default, before our async restore code reads localStorage.
      // Letting that null write through would clobber the persisted id.
      if (this._restoreComplete) {
        saveActiveConversation(this.activeConversationId);
      }
      void this._loadTurns();
      // A new conversation always opens at the latest message — reset
      // the bottom snapshot so the floating button doesn't flash on
      // the first render.
      this._prevTurnCount = 0;
      this.atBottom = true;
      // Clear any stale ethics-tier glyph — the previous draft's
      // score has no meaning against the new conversation.
      this._glyphController.reset();
      // Drop every cached per-turn (prompt, response) score — the new
      // conversation's turns are unrelated.
      this._turnScoreController.reset();
      // Reload the system-prompt + tags drafts from whichever
      // conversation is now active. Sync from this.conversations
      // rather than the backend so the switch is instant; the
      // catalogue carries both fields as of the last _reloadRail.
      this._loadSystemPromptDraft();
      this._loadTagsDraft();
    }
    if (changed.has("conversations")) {
      // After a List() refresh, the active conversation's prompt or
      // tags may have arrived for the first time (initial mount) or
      // been updated externally. Refresh both drafts.
      this._loadSystemPromptDraft();
      this._loadTagsDraft();
    }
    if (changed.has("liveTurns")) {
      const count = (this.liveTurns || []).length;
      // Auto-scroll when new turns arrive AND the user was already
      // at the bottom. If they've scrolled up to read history, leave
      // the viewport where it is — the floating button surfaces the
      // new content without yanking focus.
      if (count > this._prevTurnCount && this.atBottom) {
        // Defer one microtask so the new turn has rendered before we
        // measure scrollHeight + scrollTo.
        queueMicrotask(() => this._scrollToBottom("auto"));
      }
      this._prevTurnCount = count;
    }
    if (changed.has("searchQuery")) {
      this._scheduleContentSearch();
    }
    // findQuery / findCursor change-handling moved into FindController
    // (Athena 2026-05-16, lane 2). The controller's setQuery() resets
    // cursor + schedules scrollActiveIntoView() atomically with the
    // mutation; the findCursor setter on this host does the same for
    // cursor-only changes.
  }

  /** Debounce the live Sessions.Search call so each keystroke doesn't
   *  fire a disk-walking request. Empty query clears the content-match
   *  set immediately (no need to round-trip). */
  _scheduleContentSearch() {
    if (this._contentSearchTimer) {
      clearTimeout(this._contentSearchTimer);
      this._contentSearchTimer = null;
    }
    const q = (this.searchQuery || "").trim();
    if (!q) {
      // Blank query — drop the content matches; the title filter
      // (instant) is the only path.
      if (this.contentMatchedIds.size > 0) this.contentMatchedIds = new Set();
      // Truncated banner is tied to the live query — clear it too so
      // the stale hint doesn't outlive the search that produced it.
      if (this.searchTruncated) this.searchTruncated = false;
      return;
    }
    this._contentSearchTimer = setTimeout(() => { void this._runContentSearch(q); }, 300);
  }

  /** Fire Sessions.Search for the given query and adopt the returned
   *  ids — race-safe against fast typers: discards the response if
   *  the user has moved on to a different query in the meantime.
   *  Public so the test harness can drive it without waiting on the
   *  300ms debounce window. */
  async _runContentSearch(q: string) {
    if (!q) {
      this.contentMatchedIds = new Set();
      this.searchTruncated = false;
      return;
    }
    this._contentSearchQuery = q;
    try {
      const svc = await import("@desktop/sessions/wailsservice");
      const { unwrap } = await import("../result");
      // Mantis #1566 / #1538: sessions.Search now returns SearchResult
      // ({ hits, truncated }) instead of a flat SessionInfo slice.
      // Truncated:true means MaxSearchScan (5000) was hit before every
      // session was examined — the surfaced hits are still real, just
      // possibly partial. Mantis #1571 (Cerberus #12 F4): we surface
      // sr.truncated to the user via a small inline banner under the
      // rail-search input so the partial list isn't presented as
      // exhaustive — closes the A1 truth-to-user gap.
      type SearchHit = { id: string };
      type SearchResult = { hits: SearchHit[] | null; truncated: boolean };
      const sr = await unwrap<SearchResult>(svc.Search(q), { hits: [], truncated: false });
      // The user may have typed more since we fired — only commit if
      // the live query still matches our snapshot.
      if (this.searchQuery.trim() !== q) return;
      const ids = new Set<string>();
      for (const h of (sr?.hits || [])) {
        if (h && typeof h.id === "string") ids.add(h.id);
      }
      this.contentMatchedIds = ids;
      this.searchTruncated = !!sr?.truncated;
    } catch (err: unknown) {
      console.warn("sessions.Search failed:", err);
      // Don't blank the existing content matches on transient failure
      // — the title filter still works; keep the previous set so the
      // UI doesn't lose hits mid-typing. The truncated banner stays
      // in whatever state the last successful response left it for
      // the same reason — the user's view shouldn't flicker on a
      // single transient miss.
    }
  }

  /** Load messages for the active session and map inference.Message
   *  → ChatTurn for rendering. Empty array when no active session
   *  or the session has no messages yet. */
  async _loadTurns() {
    if (!this.activeConversationId) {
      this.liveTurns = null;
      return;
    }
    try {
      const svc = await import("@desktop/sessions/wailsservice");
      const { unwrap } = await import("../result");
      type Msg = Parameters<typeof messageToTurn>[0];
      const msgs = await unwrap<Msg[]>(svc.Read(this.activeConversationId), []);
      this.liveTurns = (msgs || []).map(messageToTurn);
    } catch (err: unknown) {
      this.sendErr = err instanceof Error ? err.message : String(err);
      this.liveTurns = [];
    }
  }

  /** Send the composer's current value to the active session.
   *  Round-trip: append user → runner.WChat(history) → append
   *  assistant → reload turns. Errors surface inline via sendErr. */
  async _send() {
    const text = this.composerValue.trim();
    if (!text || this.sending) return;
    if (!this.activeConversationId) {
      // Auto-create on first send if no session is selected.
      await this._newConversation();
      if (!this.activeConversationId) return;
    }
    const id = this.activeConversationId;
    this.sending = true;
    this.sendErr = "";
    this.composerValue = "";
    // Reset history navigation — next ↑ should start at the newest
    // (which now includes the message we're about to send).
    this._historyController.reset();
    // Clear the glyph — the composer is now empty + the just-sent
    // message lives in the transcript, so the previous draft's score
    // is no longer meaningful.
    this._glyphController.reset();
    // Composer is empty now — collapse the auto-grown height back to
    // its base so the next message starts from a single-row affordance.
    const taReset = this.querySelector<HTMLTextAreaElement>("textarea.lthn-chat-composer");
    if (taReset) taReset.style.height = "";
    try {
      const [sessions, runner] = await Promise.all([
        import("@desktop/sessions/wailsservice"),
        import("@desktop/runner/service"),
      ]);
      const { demand, unwrap } = await import("../result");
      type Msg = Parameters<typeof messageToTurn>[0];
      await demand<unknown>(sessions.Append(id, "user", text));

      // Auto-rename: the first user message in a still-default-named
      // conversation becomes its title. Skipped when the user has
      // already renamed (title doesn't match this.t.railNew). Failure
      // is best-effort — a Rename hiccup must not break the send.
      const current = this.conversations.find(c => c.id === id);
      if (current && current.title === this.t.railNew) {
        const derived = deriveAutoTitle(text);
        if (derived) {
          try { await demand<unknown>(sessions.Rename(id, derived)); }
          catch { /* keep going — assistant reply matters more */ }
        }
      }

      const rawHistory = await unwrap<Msg[]>(sessions.Read(id), []);
      const history = withSystemPrompt(rawHistory || [], current?.systemPrompt || "");
      // demand (not unwrap) so a runner failure (lthn-mlx down,
      // model not loaded, 500 from the gateway) bubbles to the
      // catch block instead of silently turning into an empty
      // assistant reply. Previously unwrap returned "" on error
      // and we Appended "assistant: \"\"" to the conversation —
      // user saw a blank reply with no error signal AND the empty
      // turn persisted in chathistory polluting future LoRA training
      // data.
      const reply = await demand<string>(runner.WChat(history));
      // Guard the empty-string case too — a successful Result with
      // empty Value is a degenerate response we don't want to
      // persist. Surface as an explicit error so the user retries
      // rather than seeing a silent dead-air assistant turn.
      if (!reply) {
        throw new Error("Empty reply from runner");
      }
      await demand<unknown>(sessions.Append(id, "assistant", reply));
      await this._loadTurns();
      await this._reloadRail();
    } catch (err: unknown) {
      this.sendErr = err instanceof Error ? err.message : String(err);
    } finally {
      this.sending = false;
    }
  }

  /** Keyboard handler for the conversation-search input. Drives the
   *  arrow-key navigation through the filtered rail:
   *    - Escape clears query + blurs (the original behaviour)
   *    - ArrowDown / ArrowUp move navIndex through the display-ordered
   *      filtered list, wrapping at the edges
   *    - Enter commits the highlighted conversation as active +
   *      blurs the input
   *  Other keys fall through to the input's default behaviour. */
  _railSearchKeydown(ev: KeyboardEvent) {
    const input = ev.target as HTMLInputElement;
    if (ev.key === "Escape") {
      ev.preventDefault();
      this.searchQuery = "";
      this.navIndex = -1;
      input.blur();
      return;
    }
    // Build the navigation list — same filter the rail renders.
    // Title match (in-memory) UNION content match (async Sessions.Search),
    // intersected with the active tag filter.
    const filtered = (this.searchQuery.trim().length > 0
      ? this.conversations.filter(c =>
          matchesSearch(c, this.searchQuery) || this.contentMatchedIds.has(c.id))
      : this.conversations)
      .filter(c => matchesTagFilter(c, this.tagFilter));
    const flat = flattenForDisplay(filtered);
    if (flat.length === 0) return;

    if (ev.key === "ArrowDown") {
      ev.preventDefault();
      this.navIndex = this.navIndex < 0
        ? 0
        : (this.navIndex + 1) % flat.length;
      return;
    }
    if (ev.key === "ArrowUp") {
      ev.preventDefault();
      this.navIndex = this.navIndex <= 0
        ? flat.length - 1
        : this.navIndex - 1;
      return;
    }
    if (ev.key === "Enter") {
      const target = this.navIndex >= 0 ? flat[this.navIndex] : flat[0];
      if (target) {
        ev.preventDefault();
        this.activeConversationId = target.id;
        this.navIndex = -1;
        input.blur();
      }
    }
  }

  /** Toggle the composer's slash-commands dropdown. The menu itself
   *  is rendered conditionally on this._slashMenuController.open in
   *  _renderComposer; an outside-click listener (installed in
   *  connectedCallback) closes it when the user clicks elsewhere. */
  _toggleSlashMenu() { this._slashMenuController.toggle(); }

  /** Slash command — `/new`. Closes the menu, then dispatches to the
   *  same code path the rail's "New conversation" button uses. */
  async _slashNew() {
    this._slashMenuController.close();
    await this._newConversation();
  }

  /** Slash command — `/clear`. Wipes the active conversation's
   *  messages while keeping the session itself (title + id stay).
   *  No-op when there's no active session or it's already empty
   *  (idempotent at the Go layer). Reloads the transcript +
   *  refreshes the rail so the rail's message-count drops. */
  async _slashClear() {
    this._slashMenuController.close();
    if (!this.activeConversationId) return;
    try {
      const svc = await import("@desktop/sessions/wailsservice");
      const { demand } = await import("../result");
      await demand<unknown>(svc.ClearMessages(this.activeConversationId));
      await this._loadTurns();
      await this._reloadRail();
    } catch (err: unknown) {
      console.warn("slash /clear failed:", err);
    }
  }

  /** Slash command — `/export`. Opens a SaveFile dialog so the user
   *  picks a destination, then dispatches to _exportTo for the actual
   *  write. Split so the e2e harness can drive _exportTo directly
   *  without the native dialog. */
  async _slashExport() {
    this._slashMenuController.close();
    if (!this.activeConversationId) return;
    try {
      const dlg = await import("@wailsio/runtime").then(m => m.Dialogs);
      const picked = await dlg.SaveFile({
        Title: "Export conversation",
        ButtonText: "Export",
        Filename: "conversation.md",
        Filters: [{ DisplayName: "Markdown", Pattern: "*.md" }],
      });
      if (!picked || typeof picked !== "string") return;
      await this._exportTo(this.activeConversationId, picked);
    } catch (err: unknown) {
      console.warn("export dialog failed:", err);
    }
  }

  /** Write the named session's transcript as markdown to the given
   *  path via sessions.Export. Public so tests can drive without
   *  the native dialog. Throws on failure so the caller can decide
   *  whether to surface inline. */
  async _exportTo(id: string, path: string) {
    if (!id || !path) return;
    const svc = await import("@desktop/sessions/wailsservice");
    const { demand } = await import("../result");
    await demand<unknown>(svc.Export(id, path));
  }

  /** Slash command — `/copy`. Builds a plain-text transcript of the
   *  active conversation and writes it to the system clipboard. No-op
   *  when there's nothing to copy (no turns yet). */
  async _slashCopy() {
    this._slashMenuController.close();
    const transcript = buildTranscript(this.liveTurns, this.activeModel);
    if (!transcript) return;
    try {
      await navigator.clipboard.writeText(transcript);
    } catch {
      // Clipboard write may fail under sandbox; not load-bearing for
      // the suite — the menu still closes either way.
    }
  }

  /** Slash command — `/help`. Opens the shortcut-sheet overlay. The
   *  slash menu closes immediately so the help overlay doesn't have
   *  to coexist with the command list. */
  _slashHelp() {
    this._slashMenuController.close();
    this._helpOverlayController.show();
  }

  /** Slash command — `/export-all`. Opens the OS folder picker, then
   *  dispatches to _exportAllTo for the actual write. Split so the
   *  test harness can drive the ExportAll wire without firing the
   *  native dialog. */
  async _slashExportAll() {
    this._slashMenuController.close();
    try {
      const dlg = await import("@wailsio/runtime").then(m => m.Dialogs);
      const picked = await dlg.OpenFile({
        Title: "Export every conversation",
        ButtonText: "Export here",
        CanChooseFiles: false,
        CanChooseDirectories: true,
      });
      if (!picked || typeof picked !== "string") return;
      await this._exportAllTo(picked);
    } catch (err: unknown) {
      console.warn("export-all dialog failed:", err);
    }
  }

  /** Write every session in the catalogue as markdown into dir via
   *  Sessions.ExportAll. Returns the count written so callers can
   *  surface a confirmation. Public so tests can drive without the
   *  native dialog. Throws on backend failure so the caller sees
   *  the inline error rather than a silent no-op. */
  async _exportAllTo(dir: string): Promise<number> {
    if (!dir) return 0;
    const svc = await import("@desktop/sessions/wailsservice");
    const { demand } = await import("../result");
    const count = await demand<number>(svc.ExportAll(dir));
    return count;
  }

  /** Regenerate the last assistant reply. Pops the trailing message
   *  via sessions.RemoveLast, re-runs the runner against the trimmed
   *  history, and appends a fresh assistant turn. No-op when there's
   *  no active session, the session is empty, or a send is in flight.
   *  Errors surface inline via sendErr — same channel as _send.
   *  Public so tests can drive without UI events. */
  async _regenerate() {
    if (!this.activeConversationId || this.sending) return;
    const id = this.activeConversationId;
    this.sending = true;
    this.sendErr = "";
    try {
      const [sessions, runner] = await Promise.all([
        import("@desktop/sessions/wailsservice"),
        import("@desktop/runner/service"),
      ]);
      const { demand, unwrap } = await import("../result");
      type Msg = Parameters<typeof messageToTurn>[0];
      // Drop the last message (expected to be the assistant reply
      // we want to redo). If the session only has the user turn —
      // because the previous send errored before the assistant
      // landed — RemoveLast will return that user message instead
      // and the next runner call will produce a fresh assistant.
      // Either branch lands the right history shape.
      await demand<unknown>(sessions.RemoveLast(id));
      const rawHistory = await unwrap<Msg[]>(sessions.Read(id), []);
      const current = this.conversations.find(c => c.id === id);
      const history = withSystemPrompt(rawHistory || [], current?.systemPrompt || "");
      // demand + empty-guard — same pattern as _send. unwrap("") here
      // turned every runner failure during a Redo into a blank
      // assistant turn persisted to chathistory, with no error signal
      // to the user. Redo is the "the reply was bad, give me another"
      // affordance — silently substituting a worse blank reply is the
      // worst possible UX for that path.
      const reply = await demand<string>(runner.WChat(history));
      if (!reply) {
        throw new Error("Empty reply from runner");
      }
      await demand<unknown>(sessions.Append(id, "assistant", reply));
      await this._loadTurns();
      await this._reloadRail();
    } catch (err: unknown) {
      this.sendErr = err instanceof Error ? err.message : String(err);
    } finally {
      this.sending = false;
    }
  }

  /** Composer keydown — ⌘↵ / Ctrl+↵ sends; plain Enter inserts a
   *  newline (textarea default). ↑/↓ walks the user-message history
   *  when the composer is empty or already browsing — see
   *  userMessageHistory for the source list. */
  _composerKeydown(e: KeyboardEvent) {
    if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
      e.preventDefault();
      void this._send();
      return;
    }
    if (e.key === "ArrowUp")   { this._historyController.up(e);   return; }
    if (e.key === "ArrowDown") { this._historyController.down(e); return; }
  }

  /** Cycle the right rail expand → collapse → expand. "hidden" is
   *  reserved for the embedded shell to suppress the rail entirely
   *  (set externally); the in-window controls only toggle between
   *  expanded + collapsed so the user always has the rail one click
   *  away. Header sliders + chart-line icons both call this — a
   *  duplicate affordance the design intentionally provides. */
  /** Open the rail directly to a named section. If the rail is
   *  already open AND on the same section the user clicked, treat
   *  that as "I want it collapsed" — single icon serves both
   *  expand-to-this and collapse-when-here. */
  _openRailSection(section: "settings" | "stats" | "sources") {
    if (this.rightRail === "expanded" && this.railSection === section) {
      this.rightRail = "collapsed";
      return;
    }
    this.railSection = section;
    this.rightRail = "expanded";
  }

  _toggleRightRail() {
    this.rightRail = this.rightRail === "expanded" ? "collapsed" : "expanded";
  }

  /** Copy arbitrary text to the system clipboard. Used by the message
   *  bubble's copy icon (transcript text) and the code-block's Copy
   *  button (code body). navigator.clipboard.writeText resolves async
   *  but we don't await — the click is a fire-and-forget UX action;
   *  failures just no-op rather than block the UI. */
  _copyText(text: string) {
    if (!text) return;
    void navigator.clipboard.writeText(text).catch(() => { /* clipboard denied */ });
  }

  /** Delete the active conversation via Dialogs.Question confirmation.
   *  Split from _adoptDelete so tests can drive the deletion path
   *  without firing the native dialog (same pattern as the model
   *  browser's _importGguf / _adoptGguf split). */
  async _deleteActiveConversation() {
    if (!this.activeConversationId) return;
    try {
      const dlg = await import("@wailsio/runtime").then(m => m.Dialogs);
      const choice = await dlg.Question({
        Title: "Delete conversation?",
        Message: "This removes the conversation and all of its messages. The action can't be undone.",
        Buttons: [
          { Label: "Delete", IsDefault: false },
          { Label: "Cancel", IsCancel: true },
        ],
      });
      if (choice !== "Delete") return;
      await this._adoptDelete(this.activeConversationId);
    } catch (err: unknown) {
      console.warn("delete conversation dialog failed:", err);
    }
  }

  /** Delete a conversation by id + refresh the rail. Public so tests
   *  can drive deletion directly (no Dialogs.Question dependency).
   *  After deletion, advance the active selection to the first
   *  surviving conversation, or null when none remain — the chat
   *  surface handles the no-active case by showing the empty state. */
  async _adoptDelete(id: string) {
    if (!id) return;
    try {
      const svc = await import("@desktop/sessions/wailsservice");
      const { demand } = await import("../result");
      await demand<unknown>(svc.Delete(id));
      const wasActive = this.activeConversationId === id;
      if (wasActive) {
        // Defer the active reassignment until after _reloadRail
        // has mutated this.conversations so we can pick the first
        // surviving conversation deterministically.
        this.activeConversationId = null;
      }
      await this._reloadRail();
      if (wasActive && this.conversations.length > 0) {
        this.activeConversationId = this.conversations[0].id;
      }
    } catch (err: unknown) {
      this.railErr = err instanceof Error ? err.message : String(err);
      throw err;
    }
  }

  /** Fork the given conversation — Sessions.Duplicate clones the
   *  manifest + messages into a new id (title suffixed with "(copy)").
   *  After the call lands, the rail is refreshed and the new copy is
   *  promoted to active so the user lands on the fork immediately.
   *  Public so the context menu + future shortcut wire dispatch the
   *  same code path. */
  async _duplicateConversation(id: string) {
    if (!id) return;
    try {
      const svc = await import("@desktop/sessions/wailsservice");
      const { demand } = await import("../result");
      const newId = await demand<string>(svc.Duplicate(id));
      await this._reloadRail();
      if (newId) this.activeConversationId = newId;
    } catch (err: unknown) {
      this.railErr = err instanceof Error ? err.message : String(err);
      throw err;
    }
  }

  /** Toggle pin state on a conversation. Persists to localStorage so
   *  pinned set survives restart. Mirrors the model browser's
   *  _togglePin shape — reassign to a new Set so Lit's `===` change
   *  detection re-renders the rail. */
  _togglePinConversation(id: string) {
    if (!id) return;
    const next = new Set(this.pinnedConversations);
    if (next.has(id)) next.delete(id);
    else next.add(id);
    this.pinnedConversations = next;
    savePinnedConversations(next);
  }

  /** Enter rename mode for the given conversation. The render path
   *  swaps the title <div> for an autofocused <input>. Caller (the
   *  pen icon's @click) passes the conversation id — typically the
   *  active one, since the icon only appears on the active row. */
  _startRename(id: string) {
    if (!id) return;
    this.renamingId = id;
  }

  /** Leave rename mode without persisting. Bound to Esc on the input
   *  + to the icon-click toggle. */
  _cancelRename() {
    this.renamingId = null;
  }

  /** Commit a rename. Returns silently when the title is empty or
   *  unchanged from the current value; otherwise persists via
   *  sessions.Rename + refreshes the rail so the new title shows.
   *  Public so the e2e test can drive without going through the
   *  input's blur/keydown handlers. */
  async _adoptRename(id: string, title: string) {
    if (!id) return;
    const clean = (title || "").trim();
    if (!clean) {
      this.renamingId = null;
      return;
    }
    const existing = this.conversations.find(c => c.id === id);
    if (existing && existing.title === clean) {
      this.renamingId = null;
      return;
    }
    try {
      const svc = await import("@desktop/sessions/wailsservice");
      const { demand } = await import("../result");
      await demand<unknown>(svc.Rename(id, clean));
      this.renamingId = null;
      await this._reloadRail();
    } catch (err: unknown) {
      this.railErr = err instanceof Error ? err.message : String(err);
      this.renamingId = null;
      throw err;
    }
  }

  /** Create a fresh session via sessions.Create + select it.
   *  Title is left as "New conversation" until the user names it
   *  or the first message lands (future polish — rename based on
   *  first user turn). */
  async _newConversation() {
    try {
      const svc = await import("@desktop/sessions/wailsservice");
      const { demand } = await import("../result");
      const id = await demand<string>(svc.Create(this.t.railNew));
      await this._reloadRail();
      this.activeConversationId = id;
    } catch (err: unknown) {
      this.railErr = err instanceof Error ? err.message : String(err);
    }
  }

  /** Pull the session catalogue from sessions.WailsService.List()
   *  and map each SessionInfo → Conversation. Bucket falls out of
   *  the updated_at timestamp; snippet is intentionally blank for
   *  now (would require sessions.Read per id — N+1, defer until
   *  the service surfaces a last-message preview natively). */
  async _reloadRail() {
    try {
      const svc = await import("@desktop/sessions/wailsservice");
      const { unwrap } = await import("../result");
      type SessionInfo = Parameters<typeof deriveConversation>[0];
      const list = await unwrap<SessionInfo[]>(svc.List(), []);
      this.conversations = (list || []).map(deriveConversation).filter(Boolean) as Conversation[];
      if (this.conversations.length > 0 && !this.activeConversationId) {
        this.activeConversationId = this.conversations[0].id;
      }
      this.railErr = "";
    } catch (err: unknown) {
      this.railErr = err instanceof Error ? err.message : String(err);
      this.conversations = [];
    }
  }

  render() {
    const fixture = chatStateData(this.state);
    const { railData, banner } = fixture;
    // Toolbar + footer prefer the live model name when the runner has
    // one loaded; fall back to the per-state fixture label so canvas
    // previews + the "no-model" state still read coherently.
    const toolbarModel = this.activeModel || fixture.toolbarModel;
    // When a real session is selected, use its live turns instead of
    // the demo fixtures. Live empty (session with no messages yet)
    // also wins so we don't fall back to demo turns mid-session.
    const turns = this.activeConversationId !== null ? this.liveTurns : fixture.turns;
    const composer = {
      ...fixture.composer,
      value: this.composerValue,
      sending: this.sending,
    };

    /* — footer per state — */
    //
    // Generating-state footer used to read "47.2 t/s · 12.4 W" — same
    // fake fixture metrics the right-rail Stats section carried.
    // Until per-turn telemetry lands, the footer just says "Generating"
    // (the spinner-shaped status dot is the live signal) rather than
    // inventing numbers.
    const footer =
      this.state === "no-model" ? html`
        <lthn-status-dot variant="idle"></lthn-status-dot>Idle · no model loaded`
      : this.state === "generating" ? html`
        <lthn-status-dot variant="ok"></lthn-status-dot>Generating · Airplane-mode OK`
      : html`
        <lthn-status-dot variant="ok"></lthn-status-dot>Model ready · Airplane-mode OK · ${this.runnerCount} runner · v${this.version}`;

    /* — toolbar — */
    //
    // The model badge looks like a dropdown (down-caret on the right)
    // but the SPA doesn't host an inline model picker yet — clicking
    // hops to the Models pane (model-browser-window) where Activate +
    // Reload + Adapter live. Same lthn:app:setpane channel the tray
    // popover + logs Open Chat use, so the shell deep-links cleanly.
    const toolbar = html`
      <button @click=${() => void this._openModelsPane()}
        title="Open Models — pick + activate a different model"
        style="display:flex; align-items:center; gap:6px; padding:4px 9px; border-radius:7px;
               background:rgba(255,255,255,0.04); border:1px solid rgba(255,255,255,0.07);
               font-size:11.5px; color:var(--fg-1); cursor:pointer; font-family:inherit;
               --wails-draggable: no-drag;">
        <lthn-status-dot variant=${this.state === "no-model" ? "idle" : "ok"}></lthn-status-dot>
        ${toolbarModel}
        <i class="fa-solid fa-angle-down" style="font-size:9px; color:var(--fg-3); margin-left:2px;"></i>
      </button>
      <div style="font-family:var(--font-mono); font-size:10.5px; color:var(--fg-3); letter-spacing:0.02em; padding:0 4px;">
        ${railData.ctx} · ${this.runnerCount} runner
      </div>
      <lthn-btn tone="ghost" size="sm"
                ?active=${this.rightRail === "expanded" && this.railSection === "settings"}
                title="Settings — tags · instructions · sampling"
                @click=${() => this._openRailSection("settings")}>
        <i class="fa-solid fa-sliders" style="font-size:10px;"></i>
      </lthn-btn>
      <lthn-btn tone="ghost" size="sm"
                ?active=${this.rightRail === "expanded" && this.railSection === "stats"}
                title="Live stats — tok/s · watts · KV · tokens"
                @click=${() => this._openRailSection("stats")}>
        <i class="fa-solid fa-chart-line" style="font-size:10px;"></i>
      </lthn-btn>
    `;

    /* — body — */
    const body = html`
      <div style="flex:1; min-height:0; display:flex;">
        ${this._renderRail()}
        <div style="flex:1; min-width:0; display:flex; flex-direction:column;">
          ${this._renderSurface(turns, banner)}
          ${this._renderComposer(composer)}
        </div>
        ${this.rightRail !== "hidden" ? this._renderRightRail(railData) : nothing}
      </div>
      ${this._renderContextMenu()}
      ${this._renderHelpOverlay()}
    `;

    return renderChrome({
      title: this.chrome.title,
      subtitle: this.chrome.subtitle,
      w: this.w, h: this.h,
      toolbar, body, footer,
      embedded: this.embedded,
    });
  }

  /* — left rail (conversation list) — */
  _renderRail() {
    const empty = this.conversations.length === 0;
    const buckets = [
      { label: this.t.bToday,     key: "today" },
      { label: this.t.bYesterday, key: "yesterday" },
      { label: this.t.bWeek,      key: "week" },
    ];
    const activeId = this.activeConversationId;
    const hasQuery = this.searchQuery.trim().length > 0;
    const hasTagFilter = this.tagFilter.trim().length > 0;
    // Title match (in-memory, instant) UNION content match (async,
    // populated by _runContentSearch after a debounce window).
    // INTERSECTED with the active tag filter — both must pass for a
    // conversation to render.
    const filtered = (hasQuery
      ? this.conversations.filter(c =>
          matchesSearch(c, this.searchQuery) || this.contentMatchedIds.has(c.id))
      : this.conversations)
      .filter(c => matchesTagFilter(c, this.tagFilter));
    const noMatches = (hasQuery || hasTagFilter) && filtered.length === 0;
    return html`
      <aside style="width:240px; flex-shrink:0; border-right:1px solid rgba(255,255,255,0.05);
                    background:rgba(0,0,0,0.18); display:flex; flex-direction:column; min-height:0;">
        <div style="padding:12px 12px 8px;">
          <div style="display:flex; align-items:center; gap:7px; height:28px; padding:0 10px;
                      background:rgba(255,255,255,0.04); border:1px solid rgba(255,255,255,0.06);
                      border-radius:6px;">
            <i class="fa-solid fa-magnifying-glass" style="font-size:10px; color:var(--fg-3);"></i>
            <input
              class="lthn-conversation-search"
              type="text"
              .value=${this.searchQuery}
              placeholder=${this.t.railSearch}
              @input=${(ev: Event) => {
                this.searchQuery = (ev.target as HTMLInputElement).value;
                this.navIndex = -1;
              }}
              @keydown=${(ev: KeyboardEvent) => this._railSearchKeydown(ev)}
              style="
                flex:1;
                min-width:0;
                background:transparent;
                border:none;
                outline:none;
                font-size:11.5px;
                color:var(--fg-0);
                font-family:inherit;
                --wails-draggable: no-drag;
              "
            />
            ${hasQuery ? html`
              <button
                class="lthn-conversation-search-clear"
                @click=${() => { this.searchQuery = ""; }}
                title="Clear search"
                style="
                  flex-shrink:0;
                  width:18px; height:18px;
                  display:inline-flex; align-items:center; justify-content:center;
                  background:transparent;
                  border:none;
                  border-radius:3px;
                  color:var(--fg-3);
                  cursor:pointer;
                  padding:0;
                  --wails-draggable: no-drag;
                "
                onmouseover="this.style.color='var(--fg-0)';"
                onmouseout="this.style.color='var(--fg-3)';">
                <i class="fa-solid fa-xmark" style="font-size:9px;"></i>
              </button>
            ` : html`
              <span style="font-family:var(--font-mono); font-size:9.5px; color:var(--fg-3);
                           padding:1px 4px; border:1px solid rgba(255,255,255,0.08); border-radius:3px;">⌘K</span>
            `}
          </div>
        </div>
        ${this.searchTruncated && hasQuery ? html`
          <div class="lthn-conversation-search-truncated"
               role="status"
               aria-live="polite"
               style="margin:0 10px 6px; display:flex; align-items:center; gap:6px;
                      padding:4px 8px;
                      background:rgba(232,165,73,0.08);
                      border:1px solid rgba(232,165,73,0.20);
                      border-radius:6px;
                      font-size:10.5px; color:var(--fg-2); line-height:1.4;">
            <i class="fa-solid fa-circle-info" style="font-size:10px; color:var(--warn-200,#e8a549); flex-shrink:0;"></i>
            <span>${this.t.searchTruncatedHint}</span>
          </div>
        ` : nothing}
        ${hasTagFilter ? html`
          <div class="lthn-conversation-tag-filter"
               style="margin:0 10px 6px; display:flex; align-items:center; gap:6px;
                      padding:4px 8px;
                      background:rgba(64,193,197,0.08);
                      border:1px solid rgba(64,193,197,0.20);
                      border-radius:6px;">
            <span style="font-size:10px; color:var(--fg-3);">Tag</span>
            <span style="font-size:11px; color:var(--brand-200); font-weight:500;">${this.tagFilter}</span>
            <div style="flex:1"></div>
            <button class="lthn-conversation-tag-filter-clear"
                    @click=${() => this._toggleTagFilter(this.tagFilter)}
                    title="Clear tag filter"
                    style="
                      width:16px; height:16px; padding:0;
                      display:inline-flex; align-items:center; justify-content:center;
                      background:transparent; border:none; border-radius:3px;
                      color:var(--fg-3); cursor:pointer;
                      --wails-draggable: no-drag;
                    ">
              <i class="fa-solid fa-xmark" style="font-size:8px;"></i>
            </button>
          </div>
        ` : nothing}
        <div style="flex:1; min-height:0; overflow:hidden; padding:0 6px 8px;">
          ${empty ? html`
            <div style="padding:60px 16px 0; text-align:center; font-size:11.5px;
                        color:var(--fg-3); line-height:1.55;">
              <div style="width:36px; height:36px; margin:0 auto 12px; border-radius:8px;
                          background:rgba(255,255,255,0.04); display:flex; align-items:center; justify-content:center;">
                <i class="fa-regular fa-comment" style="font-size:14px; color:var(--fg-3);"></i>
              </div>
              ${this.t.railEmpty}
            </div>
          ` : noMatches ? html`
            <div class="lthn-conversation-search-empty"
                 style="padding:40px 16px 0; text-align:center; font-size:11.5px;
                        color:var(--fg-3); line-height:1.55;">
              <i class="fa-solid fa-magnifying-glass" style="font-size:16px; color:var(--fg-3); display:block; margin-bottom:10px;"></i>
              No conversations match "${this.searchQuery.trim()}".
            </div>
          ` : html`
            <div style="display:flex; flex-direction:column; gap:1px;">
              ${(() => {
                // Display-ordered flat list — used to map a row's
                // global index back to navIndex for the highlight.
                const flat = flattenForDisplay(filtered);
                const idAtIndex = (this.navIndex >= 0 && this.navIndex < flat.length)
                  ? flat[this.navIndex].id
                  : "";
                const { pinned: pinnedRows, rest } = splitPinnedConversations(filtered, this.pinnedConversations);
                return html`
                  ${pinnedRows.length > 0 ? html`
                    <lthn-label class="lthn-conversation-pinned-label"
                                style="display:block; padding:8px 8px 4px;">
                      Pinned · ${pinnedRows.length}
                    </lthn-label>
                    ${pinnedRows.map(c => this._renderRailItem(c, c.id === activeId, c.id === idAtIndex))}
                  ` : nothing}
                  ${buckets.map(b => {
                    const items = rest.filter(c => c.bucket === b.key);
                    if (items.length === 0) return nothing;
                    return html`
                      <lthn-label style="display:block; padding:8px 8px 4px;">${b.label}</lthn-label>
                      ${items.map(c => this._renderRailItem(c, c.id === activeId, c.id === idAtIndex))}
                    `;
                  })}
                `;
              })()}
            </div>
          `}
        </div>
        <div style="padding:8px 10px; border-top:1px solid rgba(255,255,255,0.05); display:flex; gap:6px;">
          <lthn-btn tone="ghost" size="md" style="flex:1; justify-content:center;"
            @click=${() => this._newConversation()}>
            <i class="fa-solid fa-plus" style="font-size:10px;"></i> ${this.t.railNew}
          </lthn-btn>
        </div>
      </aside>
    `;
  }

  /** Render a text fragment with the active searchQuery highlighted —
   *  matched segments get the brand colour + slight weight bump so
   *  the row visibly carries WHY it matched. Empty query → identity
   *  render (single non-match segment). */
  _renderHighlighted(text: string) {
    const segments = highlightMatch(text || "", this.searchQuery || "");
    if (segments.length === 0) return nothing;
    return segments.map(seg => seg.match
      ? html`<span class="lthn-rail-match"
                   style="color:var(--brand-200); font-weight:600;
                          background:rgba(64,193,197,0.10);
                          border-radius:2px; padding:0 1px;">${seg.text}</span>`
      : html`<span>${seg.text}</span>`);
  }

  _renderRailItem(c: Conversation, active: boolean, navHighlight: boolean = false) {
    const renaming = this.renamingId === c.id;
    // Background priority: active > nav-highlight > none. Border
    // shows on either active or nav-highlight (active is solid
    // brand, nav-highlight is dashed brand) so keyboard nav reads
    // as "preview" rather than "selected" until Enter commits.
    const bg = active ? "rgba(255,255,255,0.07)"
             : navHighlight ? "rgba(64,193,197,0.06)"
             : "transparent";
    const borderLeft = active ? "2px solid var(--brand-400)"
                     : navHighlight ? "2px dashed var(--brand-400)"
                     : "2px solid transparent";
    return html`
      <div
        class=${navHighlight ? "lthn-conversation-row lthn-conversation-nav" : "lthn-conversation-row"}
        data-conversation-id=${c.id}
        @click=${() => { if (!renaming) this.activeConversationId = c.id; }}
        @contextmenu=${(ev: MouseEvent) => this._onContextMenuRailRow(ev, c)}
        style="padding:8px 10px; border-radius:6px;
               background:${bg};
               border-left:${borderLeft};
               display:flex; flex-direction:row; align-items:center; gap:8px;
               cursor:${renaming ? "default" : "pointer"};
               --wails-draggable: no-drag;">
        <div style="flex:1; min-width:0; display:flex; flex-direction:column; gap:2px;">
          ${renaming
            ? html`<input
                class="lthn-conversation-rename"
                data-conversation-id=${c.id}
                .value=${c.title}
                @click=${(ev: Event) => ev.stopPropagation()}
                @keydown=${(ev: KeyboardEvent) => {
                  if (ev.key === "Enter") {
                    ev.preventDefault();
                    void this._adoptRename(c.id, (ev.target as HTMLInputElement).value);
                  } else if (ev.key === "Escape") {
                    ev.preventDefault();
                    this._cancelRename();
                  }
                }}
                @blur=${(ev: Event) => {
                  void this._adoptRename(c.id, (ev.target as HTMLInputElement).value);
                }}
                ${litRef((el?: Element) => { if (el instanceof HTMLInputElement) el.focus(); })}
                style="
                  width:100%;
                  font-size:12px; font-weight:500;
                  color:var(--fg-0);
                  background:rgba(0,0,0,0.32);
                  border:1px solid rgba(64,193,197,0.32);
                  border-radius:4px;
                  padding:3px 6px;
                  letter-spacing:-0.005em;
                  font-family:inherit;
                  outline:none;
                  --wails-draggable: no-drag;
                ">`
            : html`<div style="font-size:12px; font-weight:500; color:var(--fg-0); white-space:nowrap;
                      overflow:hidden; text-overflow:ellipsis; letter-spacing:-0.005em;
                      display:flex; align-items:center; gap:6px;">
                <span style="overflow:hidden; text-overflow:ellipsis; flex:1; min-width:0;">${this._renderHighlighted(c.title)}</span>
                ${c.systemPrompt ? html`<i class="fa-solid fa-wand-magic-sparkles lthn-conversation-prompt-flag"
                                          title="Custom instructions set for this conversation"
                                          style="font-size:9px; color:var(--brand-300); flex-shrink:0;
                                                 opacity:0.85;"></i>` : nothing}
                ${c.updatedAt ? html`<span class="lthn-conversation-time"
                                            style="font-size:9.5px; color:var(--fg-3);
                                                   flex-shrink:0; font-family:var(--font-mono);
                                                   letter-spacing:0.01em;">${relativeTime(c.updatedAt, Math.floor(Date.now() / 1000))}</span>` : nothing}
              </div>`}
          ${c.snippet && !renaming ? html`<div style="font-size:10.5px; color:var(--fg-3); white-space:nowrap;
                      overflow:hidden; text-overflow:ellipsis;">${this._renderHighlighted(c.snippet)}</div>` : nothing}
          ${c.model && !renaming ? html`<div style="font-family:var(--font-mono); font-size:9px; color:var(--fg-3);
                      letter-spacing:0.02em; margin-top:1px;">${c.model}</div>` : nothing}
          ${c.tags && c.tags.length > 0 && !renaming ? html`
            <div class="lthn-conversation-tags"
                 style="display:flex; flex-wrap:wrap; gap:3px; margin-top:3px;">
              ${c.tags.map(tag => {
                const isActive = this.tagFilter === tag;
                return html`
                  <button class="lthn-conversation-tag"
                          data-tag=${tag}
                          ?data-active=${isActive}
                          @click=${(ev: Event) => {
                            ev.stopPropagation();
                            this._toggleTagFilter(tag);
                          }}
                          title=${isActive ? "Clear filter" : `Filter to "${tag}"`}
                          style="font:inherit; font-size:9px;
                                 padding:1px 5px; border-radius:999px;
                                 background:${isActive ? "rgba(64,193,197,0.20)" : "rgba(64,193,197,0.08)"};
                                 border:1px solid ${isActive ? "rgba(64,193,197,0.45)" : "rgba(64,193,197,0.18)"};
                                 color:var(--brand-200); letter-spacing:0.01em;
                                 cursor:pointer;
                                 --wails-draggable: no-drag;">${tag}</button>
                `;
              })}
            </div>
          ` : nothing}
        </div>
        ${active && !renaming ? html`
          <button
            class="lthn-conversation-pin"
            data-conversation-id=${c.id}
            ?data-pinned=${this.pinnedConversations.has(c.id)}
            @click=${(ev: Event) => {
              ev.stopPropagation();
              this._togglePinConversation(c.id);
            }}
            title=${this.pinnedConversations.has(c.id) ? "Unpin" : "Pin"}
            style="
              flex-shrink:0;
              width:22px; height:22px;
              display:inline-flex; align-items:center; justify-content:center;
              background:transparent;
              border:1px solid transparent;
              border-radius:5px;
              color:${this.pinnedConversations.has(c.id) ? "var(--brand-300)" : "var(--fg-3)"};
              cursor:pointer;
              --wails-draggable: no-drag;
            "
            onmouseover="this.style.background='rgba(64,193,197,0.08)';"
            onmouseout="this.style.background='transparent';">
            <i class="fa-${this.pinnedConversations.has(c.id) ? "solid" : "regular"} fa-star" style="font-size:10px;"></i>
          </button>
          <button
            class="lthn-conversation-rename-toggle"
            data-conversation-id=${c.id}
            @click=${(ev: Event) => {
              ev.stopPropagation();
              this._startRename(c.id);
            }}
            title="Rename conversation"
            style="
              flex-shrink:0;
              width:22px; height:22px;
              display:inline-flex; align-items:center; justify-content:center;
              background:transparent;
              border:1px solid transparent;
              border-radius:5px;
              color:var(--fg-3);
              cursor:pointer;
              --wails-draggable: no-drag;
            "
            onmouseover="this.style.background='rgba(64,193,197,0.08)'; this.style.color='var(--brand-300)';"
            onmouseout="this.style.background='transparent'; this.style.color='var(--fg-3)';">
            <i class="fa-regular fa-pen-to-square" style="font-size:10px;"></i>
          </button>
          <button
            class="lthn-conversation-delete"
            data-conversation-id=${c.id}
            @click=${(ev: Event) => {
              ev.stopPropagation();
              void this._deleteActiveConversation();
            }}
            title="Delete conversation"
            style="
              flex-shrink:0;
              width:22px; height:22px;
              display:inline-flex; align-items:center; justify-content:center;
              background:transparent;
              border:1px solid transparent;
              border-radius:5px;
              color:var(--fg-3);
              cursor:pointer;
              --wails-draggable: no-drag;
            "
            onmouseover="this.style.background='rgba(239,68,68,0.08)'; this.style.color='var(--error-400)';"
            onmouseout="this.style.background='transparent'; this.style.color='var(--fg-3)';">
            <i class="fa-regular fa-trash-can" style="font-size:10px;"></i>
          </button>
        ` : nothing}
      </div>
    `;
  }

  /* — right-click context menu (Rename / Pin / Export / Delete) — */
  _renderContextMenu() {
    const cm = this._contextMenuController.for;
    if (!cm) return nothing;
    const pinned = this.pinnedConversations.has(cm.id);
    // Viewport-clamp math lives on ContextMenuController (lane 4,
    // Athena 2026-05-16). `clampedPosition()` returns null when the
    // menu is closed; the early-return above guards that path.
    const { x, y } = this._contextMenuController.clampedPosition()!;
    return html`
      <div
        class="lthn-conversation-context-menu"
        role="menu"
        data-conversation-id=${cm.id}
        style="
          position:fixed; left:${x}px; top:${y}px;
          z-index:9000; width:180px;
          background:rgba(18,20,24,0.96);
          border:1px solid rgba(255,255,255,0.08);
          border-radius:8px;
          padding:4px;
          box-shadow:0 12px 32px rgba(0,0,0,0.5);
          --wails-draggable: no-drag;
        "
        @click=${(ev: Event) => ev.stopPropagation()}>
        <button
          class="lthn-conversation-context-rename"
          role="menuitem"
          @click=${() => this._runContextMenuAction(() => this._startRename(cm.id))}
          style="${ctxItemStyle()}"
          onmouseover="this.style.background='rgba(64,193,197,0.10)';"
          onmouseout="this.style.background='transparent';">
          <i class="fa-regular fa-pen-to-square" style="font-size:11px; width:14px;"></i>
          <span>Rename</span>
          <span style="margin-left:auto; font-size:10px; color:var(--fg-3);">F2</span>
        </button>
        <button
          class="lthn-conversation-context-pin"
          role="menuitem"
          @click=${() => this._runContextMenuAction(() => this._togglePinConversation(cm.id))}
          style="${ctxItemStyle()}"
          onmouseover="this.style.background='rgba(64,193,197,0.10)';"
          onmouseout="this.style.background='transparent';">
          <i class="fa-${pinned ? "solid" : "regular"} fa-star"
             style="font-size:11px; width:14px; color:${pinned ? "var(--brand-300)" : "inherit"};"></i>
          <span>${pinned ? "Unpin" : "Pin"}</span>
        </button>
        <button
          class="lthn-conversation-context-duplicate"
          role="menuitem"
          @click=${() => this._runContextMenuAction(() => this._duplicateConversation(cm.id))}
          style="${ctxItemStyle()}"
          onmouseover="this.style.background='rgba(64,193,197,0.10)';"
          onmouseout="this.style.background='transparent';">
          <i class="fa-regular fa-clone" style="font-size:11px; width:14px;"></i>
          <span>Duplicate</span>
        </button>
        <button
          class="lthn-conversation-context-export"
          role="menuitem"
          @click=${() => this._runContextMenuAction(() => this._slashExport())}
          style="${ctxItemStyle()}"
          onmouseover="this.style.background='rgba(64,193,197,0.10)';"
          onmouseout="this.style.background='transparent';">
          <i class="fa-regular fa-file-arrow-down" style="font-size:11px; width:14px;"></i>
          <span>Export to .md</span>
        </button>
        <div style="height:1px; background:rgba(255,255,255,0.06); margin:4px 6px;"></div>
        <button
          class="lthn-conversation-context-delete"
          role="menuitem"
          @click=${() => this._runContextMenuAction(() => this._deleteActiveConversation())}
          style="${ctxItemStyle("var(--error-400)")}"
          onmouseover="this.style.background='rgba(239,68,68,0.10)';"
          onmouseout="this.style.background='transparent';">
          <i class="fa-regular fa-trash-can" style="font-size:11px; width:14px;"></i>
          <span>Delete</span>
        </button>
      </div>
    `;
  }

  /* — /help overlay (keyboard shortcuts + slash commands cheat sheet) — */
  _renderHelpOverlay() {
    if (!this._helpOverlayController.open) return nothing;
    const sections: Array<{ heading: string; entries: Array<[string, string]> }> = [
      {
        heading: "Conversations",
        entries: [
          ["⌘N",          "New conversation"],
          ["⌘K",          "Focus the rail search"],
          ["⌘B",          "Toggle the right rail"],
          ["F2",          "Rename the active conversation"],
          ["↑ / ↓",       "Walk the conversation list"],
          ["Enter",       "Open the highlighted conversation"],
          ["Right-click", "Context menu — rename / pin / export / delete"],
        ],
      },
      {
        heading: "Composer",
        entries: [
          ["⌘↵",     "Send the message"],
          ["⌘F",     "Find in the transcript"],
          ["?",      "Show this cheat sheet"],
          ["Escape", "Dismiss overlays in priority order"],
        ],
      },
      {
        heading: "Slash commands",
        entries: [
          ["/new",        "New conversation"],
          ["/copy",       "Copy transcript to the clipboard"],
          ["/clear",      "Wipe messages (session stays)"],
          ["/export",     "Export the transcript as Markdown"],
          ["/export-all", "Back up every conversation to a folder"],
          ["/help",       "Show this cheat sheet"],
        ],
      },
    ];
    return html`
      <div
        class="lthn-chat-help-backdrop"
        @click=${() => this._helpOverlayController.close()}
        style="
          position:absolute; inset:0;
          background:rgba(0,0,0,0.48);
          backdrop-filter:blur(2px);
          display:flex; align-items:center; justify-content:center;
          z-index:9100;
          --wails-draggable: no-drag;
        ">
        <div
          class="lthn-chat-help"
          role="dialog"
          aria-label="Keyboard shortcuts"
          @click=${(ev: Event) => ev.stopPropagation()}
          style="
            min-width:380px; max-width:520px; max-height:80%;
            overflow-y:auto;
            background:rgba(18,20,24,0.98);
            border:1px solid rgba(255,255,255,0.10);
            border-radius:10px;
            padding:18px 20px;
            box-shadow:0 24px 48px rgba(0,0,0,0.5);
          ">
          <div style="display:flex; align-items:center; gap:10px; padding-bottom:12px;
                      border-bottom:1px solid rgba(255,255,255,0.06);">
            <div style="font-size:14px; font-weight:600; color:var(--fg-0); letter-spacing:-0.01em;">
              Shortcuts
            </div>
            <div style="flex:1"></div>
            <button
              class="lthn-chat-help-close"
              @click=${() => this._helpOverlayController.close()}
              title="Close"
              style="
                width:24px; height:24px;
                display:inline-flex; align-items:center; justify-content:center;
                background:transparent; border:none; border-radius:5px;
                color:var(--fg-3); cursor:pointer;
                --wails-draggable: no-drag;
              "
              onmouseover="this.style.background='rgba(255,255,255,0.06)'; this.style.color='var(--fg-1)';"
              onmouseout="this.style.background='transparent'; this.style.color='var(--fg-3)';">
              <i class="fa-solid fa-xmark" style="font-size:12px;"></i>
            </button>
          </div>
          ${sections.map(s => html`
            <div style="padding-top:14px;">
              <div class="lthn-chat-help-heading"
                   style="font-size:10.5px; letter-spacing:0.08em; text-transform:uppercase;
                          color:var(--fg-3); margin-bottom:6px;">
                ${s.heading}
              </div>
              <div style="display:flex; flex-direction:column; gap:4px;">
                ${s.entries.map(([key, desc]) => html`
                  <div class="lthn-chat-help-row"
                       style="display:flex; align-items:center; gap:14px; padding:4px 0;">
                    <span style="font-family:var(--font-mono); font-size:11px;
                                 color:var(--brand-300);
                                 min-width:96px;">${key}</span>
                    <span style="font-size:12px; color:var(--fg-1);">${desc}</span>
                  </div>
                `)}
              </div>
            </div>
          `)}
        </div>
      </div>
    `;
  }

  /* — surface (conversation transcript or empty hero) — */
  /** Find bar — floats over the transcript when findOpen is true. */
  _renderFindBar() {
    if (!this._findController.open) return nothing;
    const count = this._findController.matchCount();
    const hasQuery = this._findController.query.trim().length > 0;
    const counterText = !hasQuery ? ""
      : count === 0 ? "no matches"
      : `${this._findController.cursor + 1} of ${count}`;
    const arrowDisabled = count === 0;
    return html`
      <div class="lthn-chat-find"
           style="position:absolute; top:10px; right:20px; z-index:60;
                  display:flex; align-items:center; gap:8px;
                  padding:6px 10px;
                  background:rgba(18,20,24,0.96);
                  border:1px solid rgba(64,193,197,0.22);
                  border-radius:8px;
                  box-shadow:0 8px 20px rgba(0,0,0,0.42);
                  --wails-draggable: no-drag;">
        <i class="fa-solid fa-magnifying-glass" style="font-size:10px; color:var(--fg-3);"></i>
        <input
          class="lthn-chat-find-input"
          type="text"
          placeholder="Find in transcript"
          .value=${this._findController.query}
          @input=${(ev: Event) => { this._findController.setQuery((ev.target as HTMLInputElement).value); }}
          @keydown=${(ev: KeyboardEvent) => {
            // ↓ / Enter → next match; ↑ → previous. preventDefault so
            // the keystroke doesn't move the caret + accidentally
            // submit a form parent if one ever wraps the bar.
            if (ev.key === "ArrowDown" || ev.key === "Enter") {
              ev.preventDefault();
              this._findController.step(1);
              return;
            }
            if (ev.key === "ArrowUp") {
              ev.preventDefault();
              this._findController.step(-1);
              return;
            }
          }}
          ${litRef((el?: Element) => { if (el instanceof HTMLInputElement) el.focus(); })}
          style="
            background:transparent; border:none; outline:none;
            font-size:12px; color:var(--fg-0);
            font-family:inherit;
            width:200px;
            --wails-draggable: no-drag;
          " />
        <span class="lthn-chat-find-count"
              style="font-family:var(--font-mono); font-size:10px; color:var(--fg-3);
                     letter-spacing:0.02em; min-width:60px; text-align:right;">
          ${counterText}
        </span>
        <button
          class="lthn-chat-find-prev"
          title="Previous match"
          ?disabled=${arrowDisabled}
          @click=${() => this._findController.step(-1)}
          style="
            width:22px; height:22px;
            display:inline-flex; align-items:center; justify-content:center;
            background:transparent; border:none; border-radius:4px;
            color:${arrowDisabled ? "var(--fg-3)" : "var(--fg-1)"};
            opacity:${arrowDisabled ? 0.4 : 1};
            cursor:${arrowDisabled ? "default" : "pointer"};
            --wails-draggable: no-drag;
          ">
          <i class="fa-solid fa-chevron-up" style="font-size:9px;"></i>
        </button>
        <button
          class="lthn-chat-find-next"
          title="Next match"
          ?disabled=${arrowDisabled}
          @click=${() => this._findController.step(1)}
          style="
            width:22px; height:22px;
            display:inline-flex; align-items:center; justify-content:center;
            background:transparent; border:none; border-radius:4px;
            color:${arrowDisabled ? "var(--fg-3)" : "var(--fg-1)"};
            opacity:${arrowDisabled ? 0.4 : 1};
            cursor:${arrowDisabled ? "default" : "pointer"};
            --wails-draggable: no-drag;
          ">
          <i class="fa-solid fa-chevron-down" style="font-size:9px;"></i>
        </button>
        <button
          class="lthn-chat-find-close"
          title="Close"
          @click=${() => this._findController.tryHandleEscape()}
          style="
            width:22px; height:22px;
            display:inline-flex; align-items:center; justify-content:center;
            background:transparent; border:none; border-radius:4px;
            color:var(--fg-3); cursor:pointer;
            --wails-draggable: no-drag;
          ">
          <i class="fa-solid fa-xmark" style="font-size:10px;"></i>
        </button>
      </div>
    `;
  }

  _renderSurface(turns: ChatTurn[] | null, banner: ChatBanner | null) {
    // Welcome surface (starters + glyph) only when there are genuinely
    // no live turns to show. An active conversation always wins — even
    // when the state attribute defaults to "empty" — so picking an
    // existing chat from the rail renders that chat's turns rather
    // than the onboarding splash.
    const empty = this.state === "empty" && this.activeConversationId === null;
    const streamingIdx = this.state === "generating" ? (turns?.length || 0) - 1 : -1;
    return html`
      <main style="flex:1; min-height:0; display:flex; flex-direction:column; position:relative;">
        ${this._renderFindBar()}
        ${banner ? html`
          <div style="flex-shrink:0; margin:12px 22px 0; padding:10px 12px; border-radius:8px;
                      background:${banner.tone === "warn" ? "rgba(217,154,72,0.08)" : "rgba(64,193,197,0.06)"};
                      border:1px solid ${banner.tone === "warn" ? "rgba(217,154,72,0.20)" : "rgba(64,193,197,0.18)"};
                      display:flex; align-items:center; gap:10px;
                      font-size:11.5px; color:${banner.tone === "warn" ? "var(--warning-300)" : "var(--brand-200)"};">
            <i class="fa-solid ${banner.tone === "warn" ? "fa-circle-exclamation" : "fa-circle-info"}" style="font-size:11px;"></i>
            <div style="flex:1; line-height:1.5;">${banner.text}</div>
            ${banner.action ? html`
              <span style="font-size:11px; color:var(--fg-3); font-style:italic;"
                title="Banner actions are design copy — click handlers not yet wired to the underlying flows.">
                ${banner.action}
              </span>
            ` : nothing}
          </div>
        ` : nothing}
        <div
          class="lthn-chat-transcript"
          ${litRef((el?: Element) => {
            this._scrollEl = el instanceof HTMLElement ? el : null;
          })}
          @scroll=${() => this._recomputeAtBottom()}
          style="flex:1; min-height:0; overflow-y:auto; padding:20px 22px;
                 display:flex; flex-direction:column; gap:22px;">
          ${empty ? this._renderEmpty() :
            (turns || []).map((t, i, arr) => this._renderTurn(t, i === streamingIdx, i === arr.length - 1, i))}
        </div>
        ${!empty && !this.atBottom ? html`
          <button
            class="lthn-chat-scroll-bottom"
            title="Jump to latest"
            @click=${() => this._scrollToBottom("smooth")}
            style="
              position:absolute; right:18px; bottom:14px;
              display:flex; align-items:center; gap:6px;
              padding:6px 10px;
              background:rgba(64,193,197,0.14);
              border:1px solid rgba(64,193,197,0.32);
              border-radius:999px;
              color:var(--brand-200);
              font-size:11px; letter-spacing:-0.005em;
              cursor:pointer;
              box-shadow:0 8px 16px rgba(0,0,0,0.32);
              --wails-draggable: no-drag;
            ">
            <i class="fa-solid fa-arrow-down" style="font-size:9px;"></i>
            <span>Latest</span>
          </button>
        ` : nothing}
      </main>
    `;
  }

  _renderEmpty() {
    const starters = [
      { icon:"fa-code",    text:"Explain this regex" },
      { icon:"fa-pen-nib", text:"Rewrite a paragraph" },
      { icon:"fa-table",   text:"Reshape this JSON" },
      { icon:"fa-question",text:"What can this model do?" },
    ];
    return html`
      <div style="flex:1; display:flex; flex-direction:column; align-items:center;
                  justify-content:center; gap:22px; padding:40px;">
        <div style="width:56px; height:56px; border-radius:14px;
                    background:linear-gradient(155deg, rgba(64,193,197,0.18), rgba(64,193,197,0.02));
                    border:1px solid rgba(64,193,197,0.18);
                    display:flex; align-items:center; justify-content:center;">
          <lthn-glyph size="28" color="var(--brand-200)" active></lthn-glyph>
        </div>
        <div style="text-align:center; display:flex; flex-direction:column; gap:8px;">
          <div style="font-size:22px; font-weight:600; color:var(--fg-0); letter-spacing:-0.015em;">
            ${this.t.emptyTitle}
          </div>
          <div style="font-size:13px; color:var(--fg-2); max-width:420px; line-height:1.55;">
            ${this.t.emptyBody}
          </div>
        </div>
        <div style="display:grid; grid-template-columns:repeat(2, 220px); gap:8px; margin-top:6px;">
          ${starters.map(s => html`
            <div @click=${() => this._seedComposer(s.text)}
                 style="padding:10px 12px; border-radius:8px;
                        background:rgba(255,255,255,0.03);
                        border:1px solid rgba(255,255,255,0.06);
                        display:flex; align-items:center; gap:10px;
                        font-size:12px; color:var(--fg-1); cursor:pointer;
                        --wails-draggable: no-drag;">
              <i class="fa-solid ${s.icon}" style="font-size:11px; color:var(--fg-3);"></i>${s.text}
            </div>
          `)}
        </div>
      </div>
    `;
  }

  /** Hop the app-shell to the Models pane (model-browser-window),
   *  where Activate + Reload + Adapter all live. Wired to the
   *  model-name badge in the toolbar so the chevron isn't a lie. */
  async _openModelsPane(): Promise<void> {
    try {
      const { Events } = await import("@wailsio/runtime");
      Events.Emit("lthn:app:setpane", "models");
    } catch (err) {
      console.error("chat: open models pane failed", err);
    }
  }

  /** Seed the composer with a starter suggestion + focus the textarea.
   *  Wires the empty-state chips so clicking one drops the suggestion
   *  text into the input — the user can then edit or hit ⌘↵ to send. */
  _seedComposer(text: string) {
    this.composerValue = text;
    queueMicrotask(() => {
      const ta = this.querySelector<HTMLTextAreaElement>("textarea.lthn-chat-composer");
      if (ta) {
        ta.focus();
        // Move caret to end so the user can keep typing.
        ta.setSelectionRange(text.length, text.length);
      }
    });
  }

  _renderTurn(t: ChatTurn, streaming: boolean, isLast: boolean = false, turnIndex: number = -1) {
    const isYou = t.role === "you";
    // Regenerate only makes sense on the final assistant turn —
    // re-running an earlier message would orphan everything after.
    // The button stays disabled while a send/regenerate is in flight.
    const canRegenerate = isLast && !isYou && !this.sending;
    return html`
      <div style="display:flex; flex-direction:column; gap:8px; padding-top:4px;">
        <div style="display:flex; align-items:center; gap:8px;">
          <div style="width:22px; height:22px; border-radius:6px;
                      background:${isYou
                        ? "linear-gradient(145deg, rgba(255,255,255,0.10), rgba(255,255,255,0.04))"
                        : "linear-gradient(145deg, rgba(64,193,197,0.30), rgba(64,193,197,0.08))"};
                      border:1px solid rgba(255,255,255,0.06);
                      display:flex; align-items:center; justify-content:center;">
            ${isYou
              ? html`<i class="fa-solid fa-user" style="font-size:10px; color:var(--fg-1);"></i>`
              : html`<lthn-glyph size="12" color="var(--brand-200)"></lthn-glyph>`}
          </div>
          <div style="font-size:12px; font-weight:600; color:var(--fg-0); letter-spacing:-0.005em;">
            ${isYou ? "You" : (this.activeModel || "Gemma 4 E2B")}
          </div>
          ${!isYou ? this._renderTurnScoreFooter(turnIndex) : nothing}
          <div style="flex:1"></div>
          ${!streaming ? html`
            <div style="display:flex; gap:4px; opacity:0.5;">
              <lthn-btn tone="quiet" size="sm" @click=${() => this._copyText(t.text)}>
                <i class="fa-regular fa-copy" style="font-size:10px;"></i>
              </lthn-btn>
              ${canRegenerate ? html`
                <lthn-btn class="lthn-chat-regenerate" tone="quiet" size="sm"
                          @click=${() => void this._regenerate()}
                          title="Regenerate reply">
                  <i class="fa-solid fa-rotate-right" style="font-size:10px;"></i>
                </lthn-btn>
              ` : nothing}
            </div>
          ` : nothing}
        </div>
        <div style="margin-left:30px; font-size:13.5px; line-height:1.6;
                    color:var(--fg-1); white-space:pre-wrap; letter-spacing:-0.003em;">
          ${this._findController.open && this._findController.query.trim()
            ? (() => {
                // Find the active-match offset inside THIS turn, if any.
                // The flat positions array maps cursor index → {turn,
                // offset}; we look up the cursor's entry then check
                // whether it belongs to this turn.
                const positions = findMatchPositions(this.liveTurns, this._findController.query);
                const active = positions[this._findController.cursor];
                const activeOffsetHere = (active && active.turnIndex === turnIndex)
                  ? active.offset : -1;
                return findSplitTurn(t.text || "", this._findController.query, activeOffsetHere).map(seg => seg.match
                  ? html`<span class=${seg.active ? "lthn-chat-find-match lthn-chat-find-active" : "lthn-chat-find-match"}
                                style="background:${seg.active ? "rgba(64,193,197,0.42)" : "rgba(217,154,72,0.32)"};
                                       color:var(--fg-0);
                                       border-radius:2px; padding:0 1px;">${seg.text}</span>`
                  : html`<span>${seg.text}</span>`);
              })()
            : t.text}${streaming ? html`<span style="display:inline-block; width:7px; height:14px;
            background:var(--brand-300); vertical-align:-3px; margin-left:2px;
            animation:lthn-cursor 1s steps(2) infinite;"></span>` : nothing}
        </div>
        ${t.code ? html`
          <div style="margin-left:30px; border-radius:8px;
                      background:rgba(0,0,0,0.32);
                      border:1px solid rgba(255,255,255,0.05); overflow:hidden;">
            <div style="display:flex; align-items:center; gap:8px; padding:6px 12px;
                        border-bottom:1px solid rgba(255,255,255,0.04);
                        background:rgba(0,0,0,0.18);">
              <span style="font-family:var(--font-mono); font-size:10px; color:var(--fg-3);
                           letter-spacing:0.04em; text-transform:uppercase;">${t.code.lang}</span>
              <div style="flex:1"></div>
              <lthn-btn tone="quiet" size="sm" @click=${() => this._copyText(t.code!.text)}>
                <i class="fa-regular fa-copy" style="font-size:10px;"></i> Copy
              </lthn-btn>
            </div>
            <pre style="margin:0; padding:12px 14px; font-family:var(--font-mono);
                        font-size:11.5px; line-height:1.6; color:var(--fg-1);
                        white-space:pre; overflow:auto;">${t.code.text}</pre>
          </div>
        ` : nothing}
        ${t.citations ? html`
          <div style="margin-left:30px; display:flex; flex-wrap:wrap; gap:6px; margin-top:2px;">
            ${t.citations.map(c => html`
              <span style="font-family:var(--font-mono); font-size:10px; color:var(--brand-300);
                           padding:2px 7px; border-radius:999px;
                           background:rgba(64,193,197,0.08);
                           border:1px solid rgba(64,193,197,0.18);">
                <i class="fa-solid fa-link" style="font-size:8px; margin-right:4px;"></i>${c}
              </span>
            `)}
          </div>
        ` : nothing}
      </div>
    `;
  }

  /* — composer — */
  _renderComposer({ value, disabled, sending, error, hint }: ChatComposer & { error?: string }) {
    const liveErr = error || this.sendErr;
    return html`
      <div style="padding:14px 22px 16px; border-top:1px solid rgba(255,255,255,0.05);
                  background:rgba(0,0,0,0.12); display:flex; flex-direction:column; gap:8px;">
        ${liveErr ? html`
          <div style="display:flex; align-items:center; gap:8px; padding:7px 11px;
                      background:rgba(255,76,76,0.08); border:1px solid rgba(255,76,76,0.18);
                      border-radius:6px; font-size:11.5px; color:var(--err-300, #ffb4b4);">
            <i class="fa-solid fa-triangle-exclamation" style="font-size:11px;"></i>${liveErr}
          </div>
        ` : nothing}
        <div style="position:relative;
                    background:${disabled ? "rgba(255,255,255,0.02)" : "rgba(255,255,255,0.04)"};
                    border:1px solid rgba(255,255,255,0.07); border-radius:10px;
                    min-height:78px; padding:12px 14px 38px;
                    opacity:${disabled ? 0.55 : 1};">
          <textarea
            class="lthn-chat-composer"
            .value=${value}
            ?disabled=${disabled || sending}
            @input=${(e: Event) => {
              const ta = e.target as HTMLTextAreaElement;
              this.composerValue = ta.value;
              this._autoGrowComposer(ta);
              // Any manual edit exits history-browsing — the user has
              // moved on from the loaded past message.
              this._historyController.reset();
              // Restart the contentshield debounce; the glyph next to
              // the send button updates GLYPH_DEBOUNCE_MS after the
              // last keystroke.
              this._glyphController.notify();
            }}
            @keydown=${(e: KeyboardEvent) => this._composerKeydown(e)}
            placeholder=${disabled ? this.t.composerDisabled : this.t.composerReady}
            style="width:100%; min-height:52px; max-height:${COMPOSER_MAX_HEIGHT}px; resize:none;
                   overflow-y:auto;
                   background:transparent; border:none; outline:none;
                   font-family:var(--font-sans); font-size:13px;
                   line-height:1.5; color:var(--fg-0);
                   --wails-draggable: no-drag;
                   ${value ? "color:var(--fg-0);" : ""}"
          ></textarea>
          <div style="position:absolute; left:12px; right:12px; bottom:8px;
                      display:flex; align-items:center; gap:8px;">
            <lthn-btn tone="quiet" size="sm" ?dim=${disabled}>
              <i class="fa-regular fa-paperclip" style="font-size:11px;"></i> ${this.t.composerAttach}
            </lthn-btn>
            <div class="lthn-slash-menu-anchor" style="position:relative;">
              <lthn-btn class="lthn-chat-slash-toggle" tone="quiet" size="sm" ?dim=${disabled}
                        @click=${(ev: Event) => { ev.stopPropagation(); this._toggleSlashMenu(); }}>
                <i class="fa-solid fa-slash-forward" style="font-size:10px;"></i> ${this.t.composerSlash}
              </lthn-btn>
              ${this._slashMenuController.open ? html`
                <div class="lthn-chat-slash-menu"
                     @click=${(ev: Event) => ev.stopPropagation()}
                     style="
                       position:absolute;
                       left:0; bottom:calc(100% + 6px);
                       min-width:200px;
                       background:rgba(20,24,30,0.96);
                       backdrop-filter: blur(12px);
                       border:1px solid rgba(255,255,255,0.08);
                       border-radius:7px;
                       box-shadow: 0 8px 24px rgba(0,0,0,0.32);
                       padding:4px;
                       display:flex; flex-direction:column; gap:1px;
                       z-index:10;
                       --wails-draggable: no-drag;
                     ">
                  <button class="lthn-chat-slash-cmd lthn-chat-slash-new"
                          @click=${() => void this._slashNew()}
                          style="
                            display:flex; align-items:center; gap:10px;
                            padding:7px 10px;
                            background:transparent;
                            border:none;
                            border-radius:4px;
                            color:var(--fg-1);
                            font-size:12px;
                            font-family:inherit;
                            text-align:left;
                            cursor:pointer;
                            --wails-draggable: no-drag;
                          "
                          onmouseover="this.style.background='rgba(255,255,255,0.05)';"
                          onmouseout="this.style.background='transparent';">
                    <span style="font-family:var(--font-mono); color:var(--brand-300); font-size:11.5px;">/new</span>
                    <span style="color:var(--fg-2); font-size:11px;">New conversation</span>
                  </button>
                  <button class="lthn-chat-slash-cmd lthn-chat-slash-copy"
                          @click=${() => void this._slashCopy()}
                          style="
                            display:flex; align-items:center; gap:10px;
                            padding:7px 10px;
                            background:transparent;
                            border:none;
                            border-radius:4px;
                            color:var(--fg-1);
                            font-size:12px;
                            font-family:inherit;
                            text-align:left;
                            cursor:pointer;
                            --wails-draggable: no-drag;
                          "
                          onmouseover="this.style.background='rgba(255,255,255,0.05)';"
                          onmouseout="this.style.background='transparent';">
                    <span style="font-family:var(--font-mono); color:var(--brand-300); font-size:11.5px;">/copy</span>
                    <span style="color:var(--fg-2); font-size:11px;">Copy transcript</span>
                  </button>
                  <button class="lthn-chat-slash-cmd lthn-chat-slash-clear"
                          @click=${() => void this._slashClear()}
                          style="
                            display:flex; align-items:center; gap:10px;
                            padding:7px 10px;
                            background:transparent;
                            border:none;
                            border-radius:4px;
                            color:var(--fg-1);
                            font-size:12px;
                            font-family:inherit;
                            text-align:left;
                            cursor:pointer;
                            --wails-draggable: no-drag;
                          "
                          onmouseover="this.style.background='rgba(255,255,255,0.05)';"
                          onmouseout="this.style.background='transparent';">
                    <span style="font-family:var(--font-mono); color:var(--brand-300); font-size:11.5px;">/clear</span>
                    <span style="color:var(--fg-2); font-size:11px;">Wipe messages</span>
                  </button>
                  <button class="lthn-chat-slash-cmd lthn-chat-slash-export"
                          @click=${() => void this._slashExport()}
                          style="
                            display:flex; align-items:center; gap:10px;
                            padding:7px 10px;
                            background:transparent;
                            border:none;
                            border-radius:4px;
                            color:var(--fg-1);
                            font-size:12px;
                            font-family:inherit;
                            text-align:left;
                            cursor:pointer;
                            --wails-draggable: no-drag;
                          "
                          onmouseover="this.style.background='rgba(255,255,255,0.05)';"
                          onmouseout="this.style.background='transparent';">
                    <span style="font-family:var(--font-mono); color:var(--brand-300); font-size:11.5px;">/export</span>
                    <span style="color:var(--fg-2); font-size:11px;">Export as markdown</span>
                  </button>
                  <button class="lthn-chat-slash-cmd lthn-chat-slash-help"
                          @click=${() => this._slashHelp()}
                          style="
                            display:flex; align-items:center; gap:10px;
                            padding:7px 10px;
                            background:transparent;
                            border:none;
                            border-radius:4px;
                            color:var(--fg-1);
                            font-size:12px;
                            font-family:inherit;
                            text-align:left;
                            cursor:pointer;
                            --wails-draggable: no-drag;
                          "
                          onmouseover="this.style.background='rgba(255,255,255,0.05)';"
                          onmouseout="this.style.background='transparent';">
                    <span style="font-family:var(--font-mono); color:var(--brand-300); font-size:11.5px;">/help</span>
                    <span style="color:var(--fg-2); font-size:11px;">Shortcuts cheat sheet</span>
                  </button>
                  <button class="lthn-chat-slash-cmd lthn-chat-slash-export-all"
                          @click=${() => void this._slashExportAll()}
                          style="
                            display:flex; align-items:center; gap:10px;
                            padding:7px 10px;
                            background:transparent;
                            border:none;
                            border-radius:4px;
                            color:var(--fg-1);
                            font-size:12px;
                            font-family:inherit;
                            text-align:left;
                            cursor:pointer;
                            --wails-draggable: no-drag;
                          "
                          onmouseover="this.style.background='rgba(255,255,255,0.05)';"
                          onmouseout="this.style.background='transparent';">
                    <span style="font-family:var(--font-mono); color:var(--brand-300); font-size:11.5px;">/export-all</span>
                    <span style="color:var(--fg-2); font-size:11px;">Back up every conversation</span>
                  </button>
                </div>
              ` : nothing}
            </div>
            <div style="flex:1"></div>
            ${(() => {
              // Counter sits between the hint and the Send button. When
              // the composer is non-empty, the count replaces the hint;
              // when empty, the hint shows through. The tone helper
              // promotes the counter colour past COMPOSER_COUNTER_WARN_AT
              // so a long message is visually obvious before send.
              const counterText = composerCounterText(this.composerValue);
              const counterTone = composerCounterTone(this.composerValue);
              if (counterText) {
                const colour = counterTone === "warn" ? "var(--brand-300)" : "var(--fg-3)";
                return html`<span class="lthn-chat-composer-counter"
                              data-counter-tone=${counterTone}
                              style="font-family:var(--font-mono); font-size:10px;
                                     color:${colour}; letter-spacing:0.02em;">${counterText}</span>`;
              }
              return hint ? html`<span style="font-family:var(--font-mono); font-size:10px;
                            color:var(--fg-3); letter-spacing:0.02em;">${hint}</span>` : nothing;
            })()}
            ${this._renderComposerGlyph()}
            ${sending ? html`
              <lthn-btn tone="danger" size="sm" disabled
                title="Cancel not yet wired — runner.WChat is a one-shot RPC; the request completes then the composer unlocks.">
                <i class="fa-solid fa-spinner fa-spin" style="font-size:10px;"></i> ${this.t.btnStop}
              </lthn-btn>
            ` : html`
              <lthn-btn tone="primary" size="sm" ?dim=${disabled}
                @click=${() => void this._send()}>
                <i class="fa-solid fa-arrow-up" style="font-size:11px;"></i> ${this.t.btnSend}
              </lthn-btn>
            `}
          </div>
        </div>
      </div>
    `;
  }

  /** Renders the small ethics-tier glyph adjacent to the send button.
   *  Pulls the latest score off the ComposerGlyphController; when no
   *  score is populated (composer empty, scoring failed, Wails not
   *  bound), returns `nothing` so no DOM is emitted. UX contract: the
   *  glyph is informational only — never gates the send action. CSS
   *  class hook is `composer-glyph composer-glyph--<tier-name>` where
   *  the suffix is the kebab-case label (`appropriate-empathy`,
   *  `soft-agreement`, `hollow-flattery`, `submission`); colours land
   *  in a separate design lane against those class names. */
  _renderComposerGlyph() {
    const score = this._glyphController.score;
    if (!score || !score.sycophancy) return nothing;
    const tier = score.sycophancy.tier;
    const toneClass = TIER_NAMES[tier] ?? "appropriate-empathy";
    const label = score.sycophancy.label;
    const composite = Math.round(score.sycophancy.composite);
    const suggestionsCount = score.suggestions?.length ?? 0;
    const suggestionSuffix = suggestionsCount > 0 ? ` · ${suggestionsCount} suggestion${suggestionsCount === 1 ? "" : "s"}` : "";
    const tooltip = `${label} · composite ${composite}/100${suggestionSuffix}`;
    return html`<span
      class="composer-glyph composer-glyph--${toneClass}"
      title=${tooltip}
      aria-label="content scoring: ${label}"
    >•</span>`;
  }

  /** Renders the per-turn ethics indicator next to the model-name
   *  footer on an assistant message. Looks up the (prompt, response)
   *  pair for turn `turnIndex` (prompt = the previous user turn) and
   *  pulls the cached DiffResult off the ChatTurnScoreController —
   *  which fires a contentshield.ScorePair() call in the background
   *  on a cache miss. UX contract: informational only, NEVER blocks
   *  / hides / dims the assistant message. CSS class hook is
   *  `turn-score turn-score--<tier-name>` where the suffix is the
   *  kebab-case sycophancy tier of the RESPONSE side (mirrors the
   *  composer-glyph vocabulary so designers reuse one palette).
   *  Tooltip + aria-label append the differential echo (when
   *  available) + the authority pattern (when identified). */
  _renderTurnScoreFooter(turnIndex: number) {
    const turns = this.liveTurns;
    if (!turns || turnIndex <= 0 || turnIndex >= turns.length) return nothing;
    const responseTurn = turns[turnIndex];
    const promptTurn = turns[turnIndex - 1];
    if (!responseTurn || responseTurn.role !== "model") return nothing;
    if (!promptTurn || promptTurn.role !== "you") return nothing;
    const prompt = promptTurn.text || "";
    const response = responseTurn.text || "";
    if (prompt === "" || response === "") return nothing;
    const slot = this._turnScoreController.score(turnIndex, prompt, response);
    if (slot === null || slot === "pending") return nothing;
    const respSyc = slot.response?.sycophancy;
    if (!respSyc) return nothing;
    const tone = TURN_TIER_NAMES[respSyc.tier] ?? "appropriate-empathy";
    const label = respSyc.label;
    const echoSeg = slot.differential
      ? ` · echo ${Math.round(slot.differential.echo * 100)}%`
      : "";
    const authSeg = slot.authority
      ? ` · authority: ${slot.authority.pattern}`
      : "";
    const tooltip = `response: ${label}${echoSeg}${authSeg}`;
    const echoAria = slot.differential
      ? `, mirror ${Math.round(slot.differential.echo * 100)}%`
      : "";
    const authAria = slot.authority
      ? `, authority ${slot.authority.pattern}`
      : "";
    const aria = `response scoring: ${label}${echoAria}${authAria}`;
    return html`<span
      class="turn-score turn-score--${tone}"
      title=${tooltip}
      aria-label=${aria}
    >•</span>`;
  }

  /* — right rail (turn metadata) — */
  _renderRightRail(data: RailData) {
    if (this.rightRail === "collapsed") {
      return html`
        <aside style="width:36px; flex-shrink:0; border-left:1px solid rgba(255,255,255,0.05);
                      background:rgba(0,0,0,0.18); display:flex; flex-direction:column;
                      align-items:center; padding:10px 0; gap:10px;">
          <lthn-btn tone="quiet" size="sm" title="Settings — tags · instructions · sampling"
                    @click=${() => this._openRailSection("settings")}>
            <i class="fa-solid fa-sliders" style="font-size:11px;"></i>
          </lthn-btn>
          <lthn-btn tone="quiet" size="sm" title="Live stats — tok/s · watts · KV · tokens"
                    @click=${() => this._openRailSection("stats")}>
            <i class="fa-solid fa-chart-line" style="font-size:11px;"></i>
          </lthn-btn>
          <lthn-btn tone="quiet" size="sm" title="Sources — pinned context for this conversation"
                    @click=${() => this._openRailSection("sources")}>
            <i class="fa-solid fa-database" style="font-size:11px;"></i>
          </lthn-btn>
        </aside>
      `;
    }
    const sectionTab = (id: "settings" | "stats" | "sources", icon: string, label: string) => html`
      <lthn-btn tone="quiet" size="sm"
                ?active=${this.railSection === id}
                title=${label}
                @click=${() => this._openRailSection(id)}>
        <i class="fa-solid ${icon}" style="font-size:10px;"></i>
      </lthn-btn>
    `;
    const sectionTitle =
      this.railSection === "settings" ? this.t.railMeta :
      this.railSection === "stats"    ? "Live stats" :
                                        "Sources";
    return html`
      <aside style="width:280px; flex-shrink:0; border-left:1px solid rgba(255,255,255,0.05);
                    background:rgba(0,0,0,0.18); display:flex; flex-direction:column;
                    min-height:0; overflow:hidden;">
        <div style="padding:14px 18px 8px; display:flex; align-items:center; gap:6px;">
          <lthn-label>${sectionTitle}</lthn-label>
          <div style="flex:1"></div>
          ${sectionTab("settings", "fa-sliders",     "Settings")}
          ${sectionTab("stats",    "fa-chart-line",  "Live stats")}
          ${sectionTab("sources",  "fa-database",    "Sources")}
          <lthn-btn tone="quiet" size="sm" title="Collapse"
                    @click=${() => this._toggleRightRail()}>
            <i class="fa-solid fa-angle-right" style="font-size:10px;"></i>
          </lthn-btn>
        </div>
        <div style="padding:0 18px 16px; display:flex; flex-direction:column; gap:14px; overflow:auto;">
          ${this.railSection === "settings" ? html`
            <div class="lthn-chat-tags"
                 style="display:flex; flex-direction:column; gap:6px;">
              <lthn-label>Tags</lthn-label>
              <input
                class="lthn-chat-tags-input"
                type="text"
                .value=${this.tagsDraft}
                ?disabled=${!this.activeConversationId}
                placeholder="comma, separated, labels"
                @input=${(e: Event) => {
                  this.tagsDraft = (e.target as HTMLInputElement).value;
                }}
                @blur=${() => void this._saveTags()}
                style="
                  width:100%;
                  background:rgba(0,0,0,0.28);
                  border:1px solid rgba(255,255,255,0.08);
                  border-radius:6px;
                  padding:7px 9px;
                  font-family:var(--font-sans); font-size:11.5px;
                  color:var(--fg-1);
                  outline:none;
                  --wails-draggable: no-drag;
                " />
            </div>

            <div class="lthn-chat-systemprompt"
                 style="display:flex; flex-direction:column; gap:6px;">
              <lthn-label>Instructions</lthn-label>
              <textarea
                class="lthn-chat-systemprompt-input"
                .value=${this.systemPromptDraft}
                ?disabled=${!this.activeConversationId}
                placeholder="Steering for this conversation only — e.g. \\"You are a Go reviewer; cite playground links.\\""
                @input=${(e: Event) => {
                  this.systemPromptDraft = (e.target as HTMLTextAreaElement).value;
                }}
                @blur=${() => void this._saveSystemPrompt()}
                style="
                  width:100%; min-height:60px; max-height:160px;
                  background:rgba(0,0,0,0.28);
                  border:1px solid rgba(255,255,255,0.08);
                  border-radius:6px;
                  padding:7px 9px;
                  font-family:var(--font-sans); font-size:11.5px;
                  line-height:1.45; color:var(--fg-1);
                  outline:none; resize:vertical;
                  --wails-draggable: no-drag;
                "></textarea>
            </div>

            <lthn-label style="margin-top:4px;">${this.t.labelSampling}</lthn-label>
            <div style="display:flex; flex-direction:column; gap:6px; font-size:11.5px;">
              <lthn-rail-row k=${this.t.sampTemp}    v="—"></lthn-rail-row>
              <lthn-rail-row k=${this.t.sampTopP}    v="—"></lthn-rail-row>
              <lthn-rail-row k=${this.t.sampMaxTok}  v="—"></lthn-rail-row>
              <lthn-rail-row k=${this.t.sampContext} v=${data.ctx || "—"}></lthn-rail-row>
            </div>
            <div style="font-size:10.5px; color:var(--fg-3); font-style:italic; line-height:1.5;">
              Model defaults apply — Lemma serves whatever lthn-mlx is configured to. Per-conversation overrides + live readback land with the sampling-config surface.
            </div>
          ` : nothing}

          ${this.railSection === "stats" ? html`
            ${this._renderRailStat(this.t.statTps, data.toksLive, data.sparkline)}
            ${this._renderRailStat(this.t.statWatts, data.watts)}
            ${this._renderRailStat(this.t.statKv, data.kvHit)}
            ${this._renderRailStat(this.t.statTokens, data.tokens)}
          ` : nothing}

          ${this.railSection === "sources" ? html`
          ${data.sources ? data.sources.map(s => html`
            <div style="padding:8px 10px; border-radius:6px;
                        background:rgba(255,255,255,0.03);
                        border:1px solid rgba(255,255,255,0.06);
                        font-size:11px; color:var(--fg-2);
                        display:flex; flex-direction:column; gap:3px;">
              <div style="color:var(--fg-1); font-weight:500;">${s.title}</div>
              <div style="color:var(--fg-3); font-size:10px;">${s.kind}</div>
            </div>
          `) : html`
            <div style="font-size:11px; color:var(--fg-3); font-style:italic;">
              ${this.t.sourcesEmpty}
            </div>
          `}
          ` : nothing}
        </div>
      </aside>
    `;
  }

  _renderRailStat(label: string, value: string, sparkline = false) {
    return html`
      <div style="padding:10px 12px; border-radius:8px;
                  background:rgba(255,255,255,0.03);
                  border:1px solid rgba(255,255,255,0.06);
                  display:flex; flex-direction:column; gap:4px;">
        <div style="font-family:var(--font-mono); font-size:9.5px; color:var(--fg-3);
                    letter-spacing:0.06em; text-transform:uppercase;">${label}</div>
        <div style="display:flex; align-items:baseline; justify-content:space-between; gap:6px;">
          <div style="font-family:var(--font-mono); font-size:19px; color:var(--fg-0);
                      letter-spacing:-0.005em; font-weight:500;">${value}</div>
          ${sparkline ? html`<lthn-sparkline></lthn-sparkline>` : nothing}
        </div>
      </div>
    `;
  }
}
customElements.define("lthn-chat-window", LthnChatWindow);

// ─── helpers ────────────────────────────────────────────────────────

/** inference.Message → ChatTurn shape the existing transcript
 *  template expects. role: "user" → "you"; everything else maps
 *  to "model" so the assistant + future system / tool messages
 *  render with the assistant pill until we surface richer roles. */
function messageToTurn(msg: { role: string; content: string }): ChatTurn {
  return {
    role: msg.role === "user" ? "you" : "model",
    text: msg.content || "",
  };
}

// AUTO_TITLE_MAX + buildTranscript + PINNED_CONVERSATIONS_KEY +
// load/save/splitPinnedConversations + ACTIVE_CONVERSATION_KEY +
// load/saveActiveConversation + RAIL_BUCKET_ORDER + flattenForDisplay +
// matchesSearch + deriveAutoTitle moved to ./chat-window.helpers
// (Athena 2026-05-16). Re-exported at the top of this file.

interface SessionInfoShape {
  id: string;
  title: string;
  created_at: number;
  updated_at: number;
  messages: number;
  /** Truncated preview of the latest message — populated by the Go
   *  side's Append / RemoveLast hooks. Optional because Search/List
   *  predating the snippet field would omit it; the empty fallback
   *  keeps the rail rendering coherent. */
  snippet?: string;
  /** Per-conversation runner steering. Empty / absent = no system
   *  prompt; the send path uses it verbatim when non-empty. */
  system_prompt?: string;
  /** Free-text labels — lowercased + de-duped by the Go side. Empty
   *  / absent = no tags; the rail row's chip render is conditional
   *  on a non-empty list. */
  tags?: string[];
}

/** SessionInfo → Conversation. Returns null when the timestamp is
 *  outside the rail's "today / yesterday / week" buckets so older
 *  sessions stay out of the immediate view (a "view all" surface
 *  reveals them later). */
function deriveConversation(info: SessionInfoShape): Conversation | null {
  const updatedSec = info.updated_at || info.created_at || 0;
  const ageSec = Math.max(0, Math.floor(Date.now() / 1000) - updatedSec);
  let bucket: Conversation["bucket"];
  if (ageSec < 86400)        bucket = "today";       // < 24 h
  else if (ageSec < 172800)  bucket = "yesterday";   // 24–48 h
  else if (ageSec < 604800)  bucket = "week";        // < 7 d
  else                       return null;            // older — out of rail
  // Prefer the manifest's live snippet (last-message preview) over
  // the fallback message-count placeholder. The Go-side Append +
  // RemoveLast hooks keep `snippet` fresh; older manifests written
  // before this field landed silently fall through to the count.
  const snippet = info.snippet || (info.messages > 0 ? `${info.messages} messages` : "");
  return {
    id: info.id,
    bucket,
    title: info.title || "(untitled)",
    snippet,
    model: "",
    systemPrompt: info.system_prompt || "",
    tags: Array.isArray(info.tags) ? info.tags : [],
    updatedAt: updatedSec || undefined,
  };
}
