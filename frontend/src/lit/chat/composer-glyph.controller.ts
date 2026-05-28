// SPDX-Licence-Identifier: EUPL-1.2

// Hephaestus 2026-05-20: composer-glyph is the second Reactive Controller
// extracted from chat-window.ts, mirroring the composer-history shape
// established by Athena 2026-05-16. Owns the debounced contentshield
// score for the live composer draft + nothing else. Zero render
// surface — the host pulls `controller.score` and decides where to paint
// the glyph in its composer template.
//
// Wire shape:
//
//   1. Host's @input handler on the composer textarea writes
//      `this.composerValue` and then calls `this._glyphController.notify()`.
//   2. notify() restarts a 250ms debounce. When the timer fires, the
//      controller dynamically imports the contentshield Wails binding
//      and calls Score(composerValue).
//   3. On a successful resolution the controller stores the typed
//      ScoreResult into `this.score` + calls `host.requestUpdate()` so
//      Lit re-renders. On rejection / Wails-not-bound / empty composer,
//      `this.score` stays null and the host renders no glyph.
//   4. `reset()` clears the score back to null (host calls on send +
//      on conversation switch).
//   5. `hostDisconnected()` cancels any pending debounce timer + marks
//      any in-flight Promise as stale so a late resolution doesn't
//      race a teardown.
//
// The dynamic import (`@desktop/contentshield/wailsservice`) is the same
// idiom training-window uses (Athena's wire pattern) — lazy + tree-shake
// friendly, and the test path can `vi.doMock` it without affecting
// non-test builds. When the binding isn't reachable (vitest happy-dom,
// design-preview harness, app booted outside the WebView) the import
// promise rejects and the catch branch keeps the controller in its
// null-score steady state. NEVER throws to the caller.
//
// UX rule (non-negotiable per the Mantis brief): the glyph is
// informational only. This controller does NOT expose any gating
// signal — the send button stays clickable regardless of tier. Surfaces
// that want to nudge the user style the glyph + tooltip; they MUST NOT
// disable submission.

import type { ReactiveController, ReactiveControllerHost } from "lit";
import type { Result } from "../../../bindings/dappco.re/go/models";

/** Per-tier match counts + spans returned by the sycophancy detector.
 *  Mirrors `contentshield.PhraseInfo` in go/pkg/contentshield/types.go. */
export interface PhraseInfo {
  spans?: ReadonlyArray<readonly [number, number]>;
  count_by_tier?: Record<string, number>;
}

/** Sycophancy classification carried inside [[ScoreResult.sycophancy]].
 *  Mirrors `contentshield.SycophancyInfo` in go/pkg/contentshield/types.go.
 *  `tier` is one of 0..3 — [[TIER_NAMES]] maps the numeric tier to its
 *  kebab-case render class. */
export interface SycophancyInfo {
  tier: number;
  label: string;
  composite: number;
  phrases?: PhraseInfo;
}

/** Span-level UI hint produced by Suggestions(). Carried in the
 *  ScoreResult.suggestions slot when the host service is configured
 *  with IncludeSuggestions. Mirrors `contentshield.Suggestion`. */
export interface Suggestion {
  type: string;
  span: readonly [number, number];
  severity: string;
  tier?: number;
  note: string;
}

/** Subset of contentshield.ScoreResult the controller surfaces back to
 *  the host. The wire binding actually returns a richer shape (Imprint,
 *  cross-text Differential / Authority on the pair variant) but the
 *  glyph only cares about Sycophancy + Suggestions count. Future
 *  consumers reading off the same controller will widen this interface
 *  rather than spinning up a second debouncer. */
export interface ScoreResult {
  sycophancy?: SycophancyInfo;
  suggestions?: Suggestion[];
}

/** Debounce window between the last notify() call and the actual
 *  contentshield.Score invocation. 250 ms is the canonical input-scoring
 *  latency target — short enough that the glyph feels responsive while
 *  the user is still typing, long enough that a normal multi-key burst
 *  coalesces to a single score call. */
export const GLYPH_DEBOUNCE_MS = 250;

/** Maps a sycophancy tier number to the kebab-case class suffix the
 *  host renders. Centralised here so the test + the host template
 *  share one source of truth and the controller stays the single
 *  owner of the tier vocabulary.
 *
 *  Example (host template):
 *
 *  ```ts
 *  const tone = TIER_NAMES[score.sycophancy.tier] ?? "appropriate-empathy";
 *  return html`<span class="composer-glyph composer-glyph--${tone}">•</span>`;
 *  ```
 */
export const TIER_NAMES: Record<number, string> = {
  0: "appropriate-empathy",
  1: "soft-agreement",
  2: "hollow-flattery",
  3: "submission",
};

/** Shape the controller needs off its host. Structural — same idiom as
 *  composer-history.controller.ts so the controller can be unit-tested
 *  against a plain stub host without mounting a Lit element. */
export interface ComposerGlyphHost extends ReactiveControllerHost {
  composerValue: string;
}

export class ComposerGlyphController implements ReactiveController {
  /** Latest score result. null when the composer is empty, when the
   *  Wails binding hasn't resolved yet, when scoring failed, or after
   *  reset() / hostDisconnected(). Public for both host-side render +
   *  test-introspection. */
  score: ScoreResult | null = null;

  /** Pending debounce timer handle. Cleared on each new notify() and on
   *  reset() / hostDisconnected(). */
  private _timer: ReturnType<typeof setTimeout> | null = null;

  /** Monotonic counter — each scheduled Score call captures the value
   *  at dispatch time. When the Promise resolves we drop the result
   *  unless the captured value still matches `_seq`. Guards against
   *  late resolutions after reset() / hostDisconnected() (no leaked
   *  requestUpdate after teardown). */
  private _seq = 0;

  constructor(private readonly host: ComposerGlyphHost) {
    host.addController(this);
  }

  hostConnected(): void { /* no-op — no listeners to wire */ }

  /** Cancel pending debounce + invalidate any in-flight Score result.
   *  Called automatically when the host element disconnects. */
  hostDisconnected(): void {
    if (this._timer !== null) {
      clearTimeout(this._timer);
      this._timer = null;
    }
    // Bump the sequence so any in-flight Score Promise's resolver finds
    // its captured seq stale + bails before touching `this.score`.
    this._seq++;
  }

  /** Called by the host's @input handler after writing composerValue.
   *  Restarts the debounce timer; the actual contentshield.Score call
   *  fires GLYPH_DEBOUNCE_MS after the last notify(). When the composer
   *  is empty the timer is cancelled + the score is cleared without
   *  scheduling a call. */
  notify(): void {
    if (this._timer !== null) {
      clearTimeout(this._timer);
      this._timer = null;
    }
    // Empty composer — clear the score immediately + skip the call.
    // Score("") would return a tier-0 result; skipping it keeps the
    // null-score state coherent with "nothing to surface".
    if (this.host.composerValue === "") {
      if (this.score !== null) {
        this.score = null;
        this.host.requestUpdate();
      }
      return;
    }
    this._timer = setTimeout(() => {
      this._timer = null;
      void this._run();
    }, GLYPH_DEBOUNCE_MS);
  }

  /** Clear the score back to null. Called from the host on send (the
   *  composer is now empty + the just-sent message lives in the
   *  transcript) and on conversation switch (the previous draft's
   *  score has no meaning against the new conversation). Cheap — only
   *  fires requestUpdate when the state actually changes. */
  reset(): void {
    if (this._timer !== null) {
      clearTimeout(this._timer);
      this._timer = null;
    }
    // Invalidate any in-flight Score so a late resolution doesn't
    // overwrite the freshly-cleared score.
    this._seq++;
    if (this.score !== null) {
      this.score = null;
      this.host.requestUpdate();
    }
  }

  /** Dispatch the contentshield.Score call. Captures the current
   *  composerValue + a sequence number so a slow Score promise that
   *  races a reset / hostDisconnected / next notify discards itself
   *  instead of clobbering the latest state.
   *
   *  Never throws — every failure mode (Wails not bound, binding
   *  rejected, Result.OK=false, malformed payload) collapses to
   *  "score stays whatever it was before". Matches the controller's
   *  null-on-failure contract. */
  private async _run(): Promise<void> {
    const seq = ++this._seq;
    const text = this.host.composerValue;
    if (text === "") {
      return;
    }
    try {
      const mod = await import("@desktop/contentshield/wailsservice");
      const r = (await mod.Score(text)) as Result;
      // Stale guard — a reset() / hostDisconnected() / fresher notify()
      // bumped _seq while we were awaiting. Drop this result.
      if (seq !== this._seq) return;
      if (!r || !r.OK) return;
      const value = r.Value as ScoreResult | null;
      if (!value || typeof value !== "object") return;
      this.score = value;
      this.host.requestUpdate();
    } catch {
      // Wails binding not bound (vitest, design-preview, app outside
      // WebView) or the call rejected. Keep the previous score —
      // clearing on every transient miss would flicker the glyph.
      // No requestUpdate; nothing changed.
    }
  }
}
