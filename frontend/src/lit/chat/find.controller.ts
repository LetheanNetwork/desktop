// SPDX-Licence-Identifier: EUPL-1.2

// Athena 2026-05-16: FindController is the second Reactive Controller
// extracted from chat-window.ts per §5 of
// `plans/ops/lthn/desktop/chat-window-controllers-plan.md`. Owns the
// ⌘F find-in-transcript overlay: open/closed flag, current query,
// cursor index, the keystroke hand-offs (Escape + ⌘F accelerator),
// the match-count derivation, and the "scroll the active match into
// view" DOM side-effect. Render template stays in chat-window.ts;
// the template just reads `this._findController.open` instead of
// `this.findOpen`. Pattern proven by ComposerHistoryController (lane 1).
//
// Owned state:
//   - `open: boolean`   — was `findOpen` on the host.
//   - `query: string`   — was `findQuery` on the host.
//   - `cursor: number`  — was `findCursor` on the host.
//
// Reads from host (not owned):
//   - `liveTurns: ChatTurn[] | null` — source for `matchCount()` +
//     the active-match offset used by `scrollActiveIntoView()`.
//
// Writes to self only; calls `host.requestUpdate()` after each
// mutation so Lit re-renders the templates that read controller state.
//
// Escape priority chain (§4 option C — explicit declared order in
// the host dispatcher). The host calls `tryHandleEscape()` and the
// controller decides whether it owned the keystroke. Pattern lets
// the priority order stay readable in one place (the host) without
// forcing controllers to know about each other.
//
// Usage (inside LthnChatWindow-shaped host):
//
//   private _findController = new FindController(this);
//
//   _onKeyDownForCmdK(ev: KeyboardEvent) {
//     if (ev.key === "Escape") {
//       if (this._findController.tryHandleEscape()) return;
//       // ... other overlays
//     }
//     const accel = ev.metaKey || ev.ctrlKey;
//     if (accel && this._findController.tryHandleAccel(ev.key.toLowerCase())) {
//       ev.preventDefault();
//       return;
//     }
//   }

import type { ReactiveController, ReactiveControllerHost } from "lit";
import type { ChatTurn } from "../types";
import { findMatchPositions } from "./chat-window.helpers";

/** Structural shape the controller needs off its host. Kept as an
 *  interface (not an import of LthnChatWindow) so the controller can
 *  be unit-tested against a plain object stub with zero Lit lifecycle. */
export interface FindHost extends ReactiveControllerHost {
  liveTurns: ChatTurn[] | null;
  /** Optional — the controller uses it for `scrollActiveIntoView()`
   *  but tests can skip by leaving it undefined; the method short-
   *  circuits when there's no querySelector or no active span. */
  querySelector?: (sel: string) => Element | null;
}

export class FindController implements ReactiveController {
  /** Public for test-introspection. Defaults match the previous host
   *  defaults exactly: closed, empty query, cursor at 0. */
  open: boolean = false;
  query: string = "";
  cursor: number = 0;

  constructor(private readonly host: FindHost) {
    host.addController(this);
  }

  hostConnected(): void { /* no-op — render-template-bound, no listeners to wire */ }
  hostDisconnected(): void { /* no-op */ }

  /** Open if closed, close if open. Resets query + cursor on every
   *  transition so reopening always starts fresh. Was `_toggleFind()`
   *  on the host. */
  toggle(): void {
    if (this.open) {
      this.open = false;
      this.query = "";
      this.cursor = 0;
    } else {
      this.open = true;
      this.cursor = 0;
    }
    this.host.requestUpdate();
  }

  /** Set the query string. Resets `cursor` to 0 so the user never
   *  lands at a stale position from a previous search. Mirrors the
   *  old `updated()` behaviour where `changed.has("findQuery")` reset
   *  the cursor. Calling code (the find-input @input handler) goes
   *  through this so the reset stays atomic with the mutation. */
  setQuery(q: string): void {
    if (this.query === q) return;
    this.query = q;
    this.cursor = 0;
    this.host.requestUpdate();
    // Active match changed — bring whatever is now-current into view.
    // queueMicrotask so the new spans have rendered first.
    queueMicrotask(() => this.scrollActiveIntoView());
  }

  /** Total match count across every turn's text for the active query.
   *  Drives the "X of N" counter in the find bar. Pure — derives from
   *  host.liveTurns; tests can assert without mounting the bar. Was
   *  `_findMatchCount()` on the host. */
  matchCount(): number {
    if (!this.open || !this.query.trim()) return 0;
    return findMatchPositions(this.host.liveTurns, this.query).length;
  }

  /** Step the cursor by delta (±1) modulo match count. Wraps in both
   *  directions; no-op when there are no matches. Was `_findStep()`
   *  on the host. */
  step(delta: number): void {
    const count = this.matchCount();
    if (count === 0) return;
    const next = ((this.cursor + delta) % count + count) % count;
    if (next === this.cursor) return;
    this.cursor = next;
    this.host.requestUpdate();
    queueMicrotask(() => this.scrollActiveIntoView());
  }

  /** Scroll the currently-active find match into view. Centred so
   *  the user can see surrounding context. No-op when find is closed
   *  or no active span is in the DOM (e.g. zero matches). Was
   *  `_scrollFindActiveIntoView()` on the host. */
  scrollActiveIntoView(): void {
    if (!this.open) return;
    if (typeof this.host.querySelector !== "function") return;
    const active = this.host.querySelector(".lthn-chat-find-active");
    if (!active || typeof (active as Element & {
      scrollIntoView?: (opts?: ScrollIntoViewOptions) => void;
    }).scrollIntoView !== "function") {
      return;
    }
    (active as HTMLElement).scrollIntoView({ behavior: "smooth", block: "center" });
  }

  /** Escape-priority hand-off (§4 option C). Returns true when this
   *  controller consumed the Escape — the host stops the priority
   *  chain. Returns false when find is closed; the host moves on to
   *  the next overlay. Closing also clears the query so reopening
   *  starts fresh. */
  tryHandleEscape(): boolean {
    if (!this.open) return false;
    this.open = false;
    this.query = "";
    this.cursor = 0;
    this.host.requestUpdate();
    return true;
  }

  /** Accelerator hand-off — host's keydown dispatcher passes the
   *  lowercased key + only when accel (metaKey || ctrlKey) is held.
   *  Returns true when this controller owns the key + handled it.
   *  Today only "f" is owned (⌘F / Ctrl+F toggles the find bar);
   *  any other key returns false so the host can continue dispatch. */
  tryHandleAccel(key: string): boolean {
    if (key !== "f") return false;
    this.toggle();
    return true;
  }
}
