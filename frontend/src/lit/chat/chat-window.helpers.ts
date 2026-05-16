// SPDX-Licence-Identifier: EUPL-1.2

// Pure helpers + constants + types extracted from chat-window.ts per
// Athena's 2026-05-16 architecture review. Nothing here depends on
// Lit, the LthnChatWindow class, the DOM, or async services —
// everything is unit-testable in isolation.
//
// The chat-window.ts module re-exports these so existing consumers
// (test file, future siblings) keep resolving from the canonical
// chat-window module path. New code MAY import directly from this
// file; either form is supported.

import type { ChatTurn, Conversation } from "../types";
import { createPinnedSet } from "../pinned-set-storage";

/* ── time formatting ──────────────────────────────────────────────── */

/** Human-readable "5m ago" style relative time. Takes a unix-seconds
 *  past timestamp + a reference unix-seconds "now" (test-friendly).
 *  Returns "now" for <60s, "Xm ago" up to an hour, "Xh ago" up to 24h,
 *  "yesterday" for 24-48h, "Xd ago" up to a week, "Xw ago" up to ~4w,
 *  and the absolute date (yyyy-mm-dd) past that. Pure. */
export function relativeTime(thenSec: number, nowSec: number): string {
  if (!thenSec || thenSec <= 0) return "";
  const diff = Math.max(0, Math.floor(nowSec - thenSec));
  if (diff < 60) return "now";
  if (diff < 3600) return `${Math.floor(diff / 60)}m ago`;
  if (diff < 86400) return `${Math.floor(diff / 3600)}h ago`;
  if (diff < 172800) return "yesterday";
  if (diff < 604800) return `${Math.floor(diff / 86400)}d ago`;
  if (diff < 2419200) return `${Math.floor(diff / 604800)}w ago`;
  // Past ~4 weeks render an absolute date — UTC components so the
  // helper stays deterministic across the test runner's TZ.
  const d = new Date(thenSec * 1000);
  const y = d.getUTCFullYear();
  const m = String(d.getUTCMonth() + 1).padStart(2, "0");
  const day = String(d.getUTCDate()).padStart(2, "0");
  return `${y}-${m}-${day}`;
}

/* ── tag filtering + parsing ──────────────────────────────────────── */

/** Check whether a conversation passes the active tag filter. Empty
 *  filter is always-true (no filter applied). Pure — drives both the
 *  rail render filter and tests. */
export function matchesTagFilter(
  conv: { tags?: string[] } | null | undefined,
  filter: string | null | undefined,
): boolean {
  const f = (filter || "").trim().toLowerCase();
  if (!f) return true;
  const tags = conv?.tags || [];
  for (const t of tags) {
    if (t.toLowerCase() === f) return true;
  }
  return false;
}

/** Parse a comma-separated tag string into a normalised tag array.
 *  Trims each, lowercases, drops blanks, de-duplicates (first wins).
 *  Mirrors the Go-side normaliseTags so the frontend draft + the
 *  backend persisted value stay shape-consistent — saves a round
 *  trip to see what the server would have stored. */
export function parseTagInput(raw: string | null | undefined): string[] {
  if (!raw) return [];
  const seen = new Set<string>();
  const out: string[] = [];
  for (const piece of raw.split(",")) {
    const clean = piece.trim().toLowerCase();
    if (!clean) continue;
    if (seen.has(clean)) continue;
    seen.add(clean);
    out.push(clean);
  }
  return out;
}

/** Render a tag array back into the canonical comma-separated string
 *  the input field shows. Pure — keeps render + parse symmetrical. */
export function tagsToInput(tags: string[] | null | undefined): string {
  if (!tags || tags.length === 0) return "";
  return tags.join(", ");
}

/* ── transcript find-bar primitives ───────────────────────────────── */

/** Flat match position — drives find-bar navigation. turnIndex tags
 *  the turn that owns this match (so the renderer can colour the
 *  active one differently), offset is the character index inside
 *  that turn's text where the match starts. Pure data so tests can
 *  assert positions without rendering. */
export interface FindPosition {
  turnIndex: number;
  offset: number;
}

/** Segment of a turn's text after find-bar splitting. Like
 *  HighlightSegment but with an extra `active` bit that marks the
 *  single match the find cursor is currently parked on. The renderer
 *  paints `active` with the brand colour and the rest with the
 *  warning-yellow highlight. */
export interface FindSegment {
  text: string;
  match: boolean;
  active: boolean;
}

/** Split one turn's text into find-aware segments. `activeOffset` is
 *  the character index inside this turn's text where the currently-
 *  active match starts, or -1 when no active match lives in this
 *  turn. Pure function so tests can pin the active segment without
 *  rendering. */
export function findSplitTurn(
  text: string,
  query: string,
  activeOffset: number,
): FindSegment[] {
  if (!text) return [];
  const q = (query || "").trim();
  if (!q) return [{ text, match: false, active: false }];
  const tLow = text.toLowerCase();
  const qLow = q.toLowerCase();
  const out: FindSegment[] = [];
  let i = 0;
  while (i < text.length) {
    const at = tLow.indexOf(qLow, i);
    if (at < 0) {
      out.push({ text: text.slice(i), match: false, active: false });
      break;
    }
    if (at > i) out.push({ text: text.slice(i, at), match: false, active: false });
    out.push({
      text: text.slice(at, at + q.length),
      match: true,
      active: at === activeOffset,
    });
    i = at + q.length;
  }
  return out;
}

/** Walk a turn array and produce every case-insensitive match
 *  position for query as a flat array. Empty query / no turns yields
 *  []. Used by the find bar to count matches AND drive ↑/↓
 *  cycling — the cursor indexes into this flat list. */
export function findMatchPositions(
  turns: ChatTurn[] | null | undefined,
  query: string,
): FindPosition[] {
  const q = (query || "").trim().toLowerCase();
  if (!q || !turns) return [];
  const out: FindPosition[] = [];
  for (let i = 0; i < turns.length; i++) {
    const text = (turns[i]?.text || "").toLowerCase();
    let cursor = 0;
    while (true) {
      const at = text.indexOf(q, cursor);
      if (at < 0) break;
      out.push({ turnIndex: i, offset: at });
      cursor = at + q.length;
    }
  }
  return out;
}

/* ── runner-message helpers ───────────────────────────────────────── */

/** Prepend a synthetic {role:"system"} message to a chat history
 *  when a non-empty system prompt is set. Empty / whitespace-only
 *  prompts pass the history through unchanged so we don't waste
 *  runner context on blank steering. Pure function — tests assert
 *  the prefix shape without spinning up the runner. */
export function withSystemPrompt<T extends { role: string; content: string }>(
  history: T[],
  prompt: string | null | undefined,
): T[] {
  const p = (prompt || "").trim();
  if (!p) return history;
  // Cast back to T so callers passing inference.Message keep their
  // exact element type; the synthetic entry mirrors the same shape
  // (role + content) the runner accepts.
  const head = { role: "system", content: p } as unknown as T;
  return [head, ...history];
}

/** Pull the user-side messages from a turn array in oldest-first
 *  order. Drives composer history (↑/↓): pressing ↑ on an empty
 *  composer loads the most recent user message, ↑/↓ walks the
 *  series. Pure function so tests can assert without spinning up the
 *  whole component. Whitespace-only entries are filtered — they
 *  wouldn't have produced a useful re-send. */
export function userMessageHistory(turns: ChatTurn[] | null | undefined): string[] {
  if (!turns) return [];
  const out: string[] = [];
  for (const t of turns) {
    if (t && t.role === "you" && (t.text || "").trim() !== "") {
      out.push(t.text);
    }
  }
  return out;
}

/* ── composer counter ─────────────────────────────────────────────── */

/** Counter threshold beyond which the composer's char count switches
 *  from quiet to brand-coloured. Picked at 1500 so day-to-day prompts
 *  stay quiet but longer ones (multi-paragraph instructions, code
 *  pastes) start to surface visually before the user feels like the
 *  context is overflowing. Pure constant so the helpers + tests share
 *  one source of truth. */
export const COMPOSER_COUNTER_WARN_AT = 1500;

/** Maximum auto-grown height of the composer textarea in pixels. Past
 *  this point the textarea becomes scrollable rather than continuing
 *  to push the page layout. */
export const COMPOSER_MAX_HEIGHT = 240;

/** Format the live char count for the composer footer. Empty / blank
 *  input renders an empty string so the slot collapses; non-empty
 *  carries an explicit "N char(s)" so the unit is unambiguous (no
 *  guesswork about whether the number is tokens, words, or bytes). */
export function composerCounterText(value: string | null | undefined): string {
  const v = value || "";
  if (v.length === 0) return "";
  return v.length === 1 ? "1 char" : `${v.length} chars`;
}

/** Tone returned by composerCounterTone — drives the colour the
 *  rendered counter picks up. "quiet" = the standard --fg-3 grey;
 *  "warn" = the brand accent so the user notices long-form input. */
export type ComposerCounterTone = "quiet" | "warn";

/** Pick the visual tone for the composer counter from raw value
 *  length. Pure so the threshold logic is unit-testable independent
 *  of Lit render. */
export function composerCounterTone(value: string | null | undefined): ComposerCounterTone {
  return (value || "").length >= COMPOSER_COUNTER_WARN_AT ? "warn" : "quiet";
}

/* ── auto-title + transcript builder ──────────────────────────────── */

/** Maximum character length for an auto-derived conversation title.
 *  Strikes a balance between informativeness and rail readability —
 *  the rail truncates with ellipsis past this width, so anything
 *  longer just gets cut off visually. */
export const AUTO_TITLE_MAX = 50;

/** Render a chat transcript as plain text suitable for clipboard
 *  paste. Each turn is `You:` or `<model>:` followed by the message
 *  body, separated by blank lines. Pure function — exported for unit
 *  tests + reuse from future export-to-file affordances.
 *
 *  Empty turns yield an empty string (callers should short-circuit
 *  before writing to clipboard so the user doesn't get a confusing
 *  "copied empty thing" experience). */
export function buildTranscript(turns: ChatTurn[] | null, modelName: string): string {
  if (!turns || turns.length === 0) return "";
  const speaker = (role: ChatTurn["role"]) =>
    role === "you" ? "You" : (modelName || "Assistant");
  return turns
    .map(t => `${speaker(t.role)}: ${t.text || ""}`)
    .join("\n\n");
}

/** Derive a conversation title from a user message. Strips newlines,
 *  collapses whitespace, trims, and truncates with a single-char
 *  ellipsis. Returns "" for whitespace-only input so callers can
 *  skip the rename when the input wouldn't yield a meaningful name. */
export function deriveAutoTitle(text: string): string {
  const normalised = (text || "").replace(/\s+/g, " ").trim();
  if (!normalised) return "";
  if (normalised.length <= AUTO_TITLE_MAX) return normalised;
  return normalised.slice(0, AUTO_TITLE_MAX - 1).trimEnd() + "…";
}

/* ── rail filter + bucket ordering ────────────────────────────────── */

/** Bucket order matching the rail's visual top-to-bottom render —
 *  today first, then yesterday, then week. Used by flattenForDisplay
 *  so keyboard navigation walks rows in the order the user sees. */
export const RAIL_BUCKET_ORDER = ["today", "yesterday", "week"] as const;

/** Flatten a filtered conversation list into a display-ordered flat
 *  array. Preserves source order within each bucket so the result
 *  matches what the rail actually renders top-to-bottom. Empty
 *  buckets are skipped naturally. Pure function — exported for unit
 *  tests + reuse from any future keyboard-nav surface. */
export function flattenForDisplay(filtered: Conversation[]): Conversation[] {
  const out: Conversation[] = [];
  for (const bucket of RAIL_BUCKET_ORDER) {
    for (const c of filtered) {
      if (c.bucket === bucket) out.push(c);
    }
  }
  return out;
}

/** Case-insensitive substring match for the conversation-search
 *  filter. Empty query matches everything (caller short-circuits in
 *  practice, but matching all is the harmless default). Today it
 *  searches the title only — future extension can broaden to snippet
 *  + model name + content if cheap-enough. */
export function matchesSearch(conversation: Conversation, query: string): boolean {
  const q = (query || "").trim().toLowerCase();
  if (!q) return true;
  return (conversation.title || "").toLowerCase().includes(q);
}

/* ── persistence keys + helpers ───────────────────────────────────── */

/** localStorage key for the conversation pin set. Persisted as a JSON
 *  array of conversation ids; deserialised into an in-memory Set. */
export const PINNED_CONVERSATIONS_KEY = "lthn.chat.pinned";

// Athena 2026-05-16: load + save are thin wrappers over the shared
// pinned-set-storage helper. Kept as functions (not just inline) so
// existing imports + tests continue to resolve from this module.
const _pinnedConversationsStore = createPinnedSet(PINNED_CONVERSATIONS_KEY);

export function loadPinnedConversations(): Set<string> {
  return _pinnedConversationsStore.load();
}

export function savePinnedConversations(pinned: Set<string>) {
  _pinnedConversationsStore.save(pinned);
}

/** Split a conversation list into pinned + rest, preserving source
 *  order within each bucket. Pure function — mirrors splitByPin
 *  from the model browser. */
export function splitPinnedConversations(
  conversations: Conversation[],
  pinned: Set<string>,
): { pinned: Conversation[]; rest: Conversation[] } {
  const out = { pinned: [] as Conversation[], rest: [] as Conversation[] };
  for (const c of conversations) {
    if (pinned.has(c.id)) out.pinned.push(c);
    else out.rest.push(c);
  }
  return out;
}

/** localStorage key for the chat-window's last-active conversation
 *  id. Loaded once on connect (when the rail has settled); written
 *  on every assignment so future restarts land back on the same
 *  conversation the user left mid-thought. */
export const ACTIVE_CONVERSATION_KEY = "lthn.chat.active-conversation";

/** Read the persisted active conversation id. Returns null when the
 *  key is missing or unreadable — degrades silently because the
 *  rail's first-conversation fallback covers the missing case. */
export function loadActiveConversation(): string | null {
  try { return localStorage.getItem(ACTIVE_CONVERSATION_KEY); }
  catch { return null; }
}

/** Persist the active conversation id. Null clears the key so the
 *  next mount falls through to first-conversation. localStorage
 *  failures (quota / disabled) are silent. */
export function saveActiveConversation(id: string | null) {
  try {
    if (id) localStorage.setItem(ACTIVE_CONVERSATION_KEY, id);
    else localStorage.removeItem(ACTIVE_CONVERSATION_KEY);
  } catch { /* in-memory state still works */ }
}
