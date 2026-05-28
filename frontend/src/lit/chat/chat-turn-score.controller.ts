// SPDX-Licence-Identifier: EUPL-1.2

// Hephaestus 2026-05-20: chat-turn-score is the per-completed-turn
// sibling to composer-glyph.controller.ts. Where the composer glyph
// scores the live USER draft via contentshield.Score(text), this
// controller scores the COMPLETED (prompt, response) pair via
// contentshield.ScorePair(prompt, response) and exposes the resulting
// DiffResult so the host can paint a small indicator dot next to the
// model-name footer on each assistant message.
//
// Shape: a Map keyed by turn index. The host calls
// `controller.score(turnIndex, prompt, response)` from the render
// path. On a cache miss the controller marks the slot "pending",
// dispatches a ScorePair call via dynamic import (no `await` in the
// synchronous reader), and populates the cache + fires
// `host.requestUpdate()` when the promise resolves. The reader
// returns synchronously every time:
//
//   - "pending" → dispatch in flight; render `nothing` (no spinner —
//                  the dot lands silently when the score is ready).
//   - null      → empty input on either side, OR ScorePair rejected,
//                  OR the Wails binding isn't reachable (vitest,
//                  design-preview). Graceful degrade — message
//                  renders unchanged, no indicator.
//   - DiffResult → populated; host paints the indicator.
//
// Stale-sequence guard mirrors composer-glyph: a monotonic `_seq`
// captured at dispatch and re-checked after resolution. `reset()`
// (conversation switch) or `hostDisconnected()` (teardown) bumps the
// counter; any in-flight ScorePair promise discards its result
// without touching the cache or calling requestUpdate.
//
// UX rule (matches the composer glyph): the indicator is
// informational only. This controller NEVER blocks, hides, dims,
// or modifies the assistant message rendering — the message lands
// first and stays visible regardless of any score result. Surfaces
// that want to nudge the user style the dot + tooltip; they MUST
// NOT change the message's visibility or interactivity based on
// the tier.
//
// The dynamic import (`@desktop/contentshield/wailsservice`) matches
// the composer-glyph idiom — lazy, tree-shake friendly, mockable via
// `vi.doMock` in tests. When the binding isn't reachable the import
// promise rejects and the catch branch records `null` in the cache
// so the same turn isn't re-attempted on every render pass.

import type { ReactiveController, ReactiveControllerHost } from "lit";
import type { Result } from "../../../bindings/dappco.re/go/models";

/** Per-tier match counts + spans returned by the sycophancy detector.
 *  Mirrors `contentshield.PhraseInfo` in go/pkg/contentshield/types.go.
 *  Identical shape to the inline interface in composer-glyph.controller.ts
 *  — duplicated here so this controller's wire surface stays
 *  self-contained against a Wails binding that emits `Result.Value: any`. */
export interface PhraseInfo {
  spans?: ReadonlyArray<readonly [number, number]>;
  count_by_tier?: Record<string, number>;
}

/** Sycophancy classification carried per-side inside a DiffResult.
 *  Mirrors `contentshield.SycophancyInfo`. */
export interface SycophancyInfo {
  tier: number;
  label: string;
  composite: number;
  phrases?: PhraseInfo;
}

/** Grammar imprint carried per-side. Opaque to the indicator render
 *  path — declared so the type matches the wire faithfully even
 *  though the indicator only consumes Sycophancy + Differential +
 *  Authority today. */
export interface ImprintInfo {
  [key: string]: unknown;
}

/** Per-side score result. The prompt + response slots of
 *  DiffResult both carry this shape. Mirrors `contentshield.ScoreResult`. */
export interface ScoreResult {
  sycophancy?: SycophancyInfo;
  imprint?: ImprintInfo | null;
}

/** Cross-text differential — 6-dimension grammar comparison between
 *  the prompt and the response. Null on the wire when either side
 *  failed to tokenise. snake_case keys mirror the Go JSON tags. */
export interface DifferentialInfo {
  echo: number;
  verb_shift: number;
  tense_shift: number;
  noun_echo: number;
  question_flip: number;
  domain_shift: number;
}

/** Pattern category surfaced by the authority signal. Mirrors the
 *  Go-side `AuthorityPattern` enum on the wire (lower-case string). */
export type AuthorityPattern = "sovereign" | "citation" | "deference" | "submission";

/** Authority signal — who/what the prompt asked the model to defer
 *  to, and which pattern the response landed in. Null on the wire
 *  when no targets were identified in the prompt. snake_case keys
 *  mirror the Go JSON tags. */
export interface AuthorityInfo {
  targets: string[];
  deference: number;
  pattern: AuthorityPattern;
}

/** Pair-wise result returned by contentshield.ScorePair(). Mirrors
 *  `contentshield.DiffResult` in go/pkg/contentshield/types.go. */
export interface DiffResult {
  prompt: ScoreResult;
  response: ScoreResult;
  differential: DifferentialInfo | null;
  authority: AuthorityInfo | null;
}

/** Maps a sycophancy tier number to the kebab-case class suffix the
 *  host renders. Identical vocabulary to composer-glyph's TIER_NAMES
 *  so designers reuse the same colour palette across the two
 *  surfaces — the `.composer-glyph--<tier>` and `.turn-score--<tier>`
 *  classes share their tone names.
 *
 *  Example (host template):
 *
 *  ```ts
 *  const tone = TIER_NAMES[result.response.sycophancy.tier] ?? "appropriate-empathy";
 *  return html`<span class="turn-score turn-score--${tone}">•</span>`;
 *  ```
 */
export const TIER_NAMES: readonly string[] = [
  "appropriate-empathy",
  "soft-agreement",
  "hollow-flattery",
  "submission",
];

/** Shape the controller needs off its host. Structural — same idiom
 *  as composer-glyph.controller.ts so the controller can be
 *  unit-tested against a plain stub host without mounting a Lit
 *  element. */
export type ChatTurnScoreHost = ReactiveControllerHost;

/** Cache-slot value. `"pending"` means a ScorePair call is in flight
 *  for this turn index; `null` means we tried (or skipped due to
 *  empty inputs) and have nothing to render; `DiffResult` means
 *  populated and ready to render. */
type Slot = DiffResult | "pending" | null;

export class ChatTurnScoreController implements ReactiveController {
  /** Cache of per-turn ScorePair results keyed by turn index. Public
   *  for test introspection; the host always reads via score(). */
  cache: Map<number, Slot> = new Map();

  /** Monotonic counter — each scheduled ScorePair dispatch captures
   *  the value at dispatch time. When the promise resolves we drop
   *  the result unless the captured value still matches `_seq`.
   *  Guards against late resolutions after reset() / hostDisconnected()
   *  (no leaked requestUpdate / no cache poison after teardown). */
  private _seq = 0;

  constructor(private readonly host: ChatTurnScoreHost) {
    host.addController(this);
  }

  hostConnected(): void { /* no-op — no listeners to wire */ }

  /** Invalidate any in-flight ScorePair + drop every cache entry.
   *  Cheap — no requestUpdate (the host is tearing down). */
  hostDisconnected(): void {
    this._seq++;
    this.cache.clear();
  }

  /** Synchronous reader with side-effect on cache miss. The render
   *  path calls this every paint; the controller handles the
   *  cache/dispatch/stale-guard logic so the host stays declarative.
   *
   *  Behaviour:
   *  - Empty prompt OR empty response → cache `null`, no ScorePair
   *    call (no point scoring an empty side).
   *  - Cache hit → return cached value immediately.
   *  - Cache miss → store `"pending"`, dispatch ScorePair via
   *    dynamic import (fire-and-forget; the reader returns synchronously),
   *    populate cache + call requestUpdate when the promise resolves.
   *
   *  Never throws — every failure mode (Wails not bound, binding
   *  rejected, Result.OK=false, malformed payload) collapses to a
   *  `null` cache entry so the host paints nothing. */
  score(turnIndex: number, prompt: string, response: string): Slot {
    if (prompt === "" || response === "") {
      // Cache the null so we don't re-evaluate the empty-side branch
      // on every render. The host's call site already skips when the
      // previous turn isn't a user message, but defence-in-depth.
      if (!this.cache.has(turnIndex)) {
        this.cache.set(turnIndex, null);
      }
      return this.cache.get(turnIndex) ?? null;
    }
    const existing = this.cache.get(turnIndex);
    if (existing !== undefined) {
      return existing;
    }
    this.cache.set(turnIndex, "pending");
    // Capture the sequence at dispatch time. A reset() /
    // hostDisconnected() between here and the resolution bumps
    // _seq and the resolver discards itself.
    const seq = this._seq;
    void this._dispatch(turnIndex, prompt, response, seq);
    return "pending";
  }

  /** Drop a single entry (e.g. the user regenerated this turn via
   *  /regen and the response text has changed underneath us). Cheap
   *  — only fires requestUpdate when the slot actually existed. */
  clear(turnIndex: number): void {
    if (this.cache.delete(turnIndex)) {
      // Invalidate any in-flight dispatch for this turn — the
      // resolver's seq check drops it.
      this._seq++;
      this.host.requestUpdate();
    }
  }

  /** Clear the entire cache (conversation switch, model unload, any
   *  context where every previously-scored pair is now meaningless).
   *  Invalidates in-flight dispatches via the seq bump. */
  reset(): void {
    this._seq++;
    if (this.cache.size > 0) {
      this.cache.clear();
      this.host.requestUpdate();
    }
  }

  /** Dispatch the ScorePair call + populate the cache on resolution.
   *  Stale-seq guard drops the result when reset / disconnect /
   *  clear bumped the counter while we were awaiting.
   *
   *  Example (host wiring — fire-and-forget from the synchronous reader):
   *
   *  ```ts
   *  void this._dispatch(turnIndex, prompt, response, this._seq);
   *  ```
   */
  private async _dispatch(turnIndex: number, prompt: string, response: string, seq: number): Promise<void> {
    try {
      const mod = await import("@desktop/contentshield/wailsservice");
      const r = (await mod.ScorePair(prompt, response)) as Result;
      if (seq !== this._seq) return; // stale — reset/disconnect/clear ran during await
      if (!r || !r.OK) {
        this.cache.set(turnIndex, null);
        this.host.requestUpdate();
        return;
      }
      const value = r.Value as DiffResult | null;
      if (!value || typeof value !== "object") {
        this.cache.set(turnIndex, null);
        this.host.requestUpdate();
        return;
      }
      this.cache.set(turnIndex, value);
      this.host.requestUpdate();
    } catch {
      // Wails binding not bound (vitest, design-preview, app outside
      // WebView) or the call rejected. Cache the null so we don't
      // re-attempt on every render pass — the indicator stays hidden
      // for this turn until clear() / reset() bumps the seq.
      if (seq !== this._seq) return;
      this.cache.set(turnIndex, null);
      this.host.requestUpdate();
    }
  }
}
