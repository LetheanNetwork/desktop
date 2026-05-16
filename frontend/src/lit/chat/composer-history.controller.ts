// SPDX-Licence-Identifier: EUPL-1.2

// Athena 2026-05-16: composer-history is the first Reactive Controller
// extracted from chat-window.ts per the §5 first-lane decision in
// `plans/ops/lthn/desktop/chat-window-controllers-plan.md`. The cursor
// state + ArrowUp/ArrowDown handlers + reset hook are a controller-
// shaped concern: scoped per host instance, self-contained, zero
// render-template surface, zero window listeners. Extracting it
// proves the ReactiveController pattern fits before the larger
// Find / ContextMenu / SlashMenu lanes follow.
//
// Owned state:
//   - `cursor` — index into userMessageHistory(host.liveTurns) in
//     oldest-first order. -1 = not browsing history (the default,
//     resumed after `reset()` is called).
//
// Reads from host (not owned by the controller):
//   - `composerValue` — current composer text. Read to decide whether
//     ArrowUp enters history mode; written when a history entry loads.
//   - `liveTurns` — source of the history list via userMessageHistory.
//
// Writes back to host:
//   - `composerValue` — set to the loaded history text on ↑/↓ navigation.
//     `host.requestUpdate()` is called after each mutation so Lit
//     re-renders the textarea's bound `.value=${composerValue}`.
//
// Usage (inside an LthnChatWindow-shaped host):
//
//   private _historyController = new ComposerHistoryController(this);
//
//   _composerKeydown(e: KeyboardEvent) {
//     if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) { ...send }
//     if (e.key === "ArrowUp")   { this._historyController.up(e);   return; }
//     if (e.key === "ArrowDown") { this._historyController.down(e); return; }
//   }
//
//   // on send + on @input (any manual edit):
//   this._historyController.reset();
//
// `up()` / `down()` return `boolean` — true when the keystroke was
// consumed (preventDefault already called internally), false when the
// keystroke is a no-op and the textarea's default cursor behaviour
// should win. The host doesn't currently inspect the return value,
// but exposing it keeps the controller honest + lets future callers
// make their own decision.

import type { ReactiveController, ReactiveControllerHost } from "lit";
import type { ChatTurn } from "../types";
import { userMessageHistory } from "./chat-window.helpers";

/** Shape the controller needs off its host. Kept as a structural type
 *  rather than importing LthnChatWindow so the controller can be unit-
 *  tested against a plain object stub (no Lit element mount required). */
export interface ComposerHistoryHost extends ReactiveControllerHost {
  composerValue: string;
  liveTurns: ChatTurn[] | null;
}

export class ComposerHistoryController implements ReactiveController {
  /** Public for test-introspection; -1 = not browsing. */
  cursor: number = -1;

  constructor(private readonly host: ComposerHistoryHost) {
    host.addController(this);
  }

  hostConnected(): void { /* no-op — no listeners to wire */ }
  hostDisconnected(): void { /* no-op — no listeners to tear down */ }

  /** ArrowUp branch. Returns true when the keystroke was consumed
   *  (controller called preventDefault + mutated composerValue).
   *  Returns false when the textarea's default cursor-up should win
   *  (composer non-empty AND not already browsing history). */
  up(e: KeyboardEvent): boolean {
    if (this.host.composerValue !== "" && this.cursor === -1) return false;
    const hist = userMessageHistory(this.host.liveTurns);
    if (hist.length === 0) return false;
    e.preventDefault();
    if (this.cursor === -1) {
      this.cursor = hist.length - 1;
    } else if (this.cursor > 0) {
      this.cursor -= 1;
    } // else stay — already at the oldest
    this.host.composerValue = hist[this.cursor];
    this.host.requestUpdate();
    return true;
  }

  /** ArrowDown branch. Returns true when the keystroke was consumed;
   *  false when the cursor is -1 (textarea default cursor-down wins). */
  down(e: KeyboardEvent): boolean {
    if (this.cursor === -1) return false;
    const hist = userMessageHistory(this.host.liveTurns);
    e.preventDefault();
    if (this.cursor < hist.length - 1) {
      this.cursor += 1;
      this.host.composerValue = hist[this.cursor];
    } else {
      // Past the newest — exit history mode + clear so the user
      // can start typing fresh from a blank slate.
      this.cursor = -1;
      this.host.composerValue = "";
    }
    this.host.requestUpdate();
    return true;
  }

  /** Reset cursor to -1. Called from the host on send (next ↑ should
   *  start at the newest, which now includes the just-sent message)
   *  and on any manual composer edit (typing exits history mode). */
  reset(): void {
    this.cursor = -1;
  }
}
