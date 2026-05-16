// SPDX-Licence-Identifier: EUPL-1.2

// Athena 2026-05-16: HelpOverlayController is the fifth (and final)
// Reactive Controller extracted from chat-window.ts per §3.5 of
// `plans/ops/lthn/desktop/chat-window-controllers-plan.md`. Owns the
// keyboard-shortcut cheat-sheet overlay mechanics — the open flag,
// open/close/toggle entry points, and the Escape-priority hand-off.
//
// The cheat-sheet CONTENT (the rendered sections + entries) STAYS on
// chat-window — that's a render concern, not mechanics. Same
// separation as lane 3's slash-menu (mechanics here, slash-action
// methods on host) and lane 4's context-menu (mechanics here, item
// action methods on host).
//
// Catalogue notes this is the smallest of the five lanes (~20 LOC of
// mechanics); extracted for chain symmetry — every overlay routes
// through a controller after this lands.
//
// Owned state:
//   - `open: boolean` — was `helpOpen` on the host. false = closed
//     (default, matches the previous host init).
//
// The host exposes a getter/setter shim for `helpOpen` that
// delegates to `_helpOverlayController.open`, so existing
// chat-window tests + render templates that read/write
// `this.helpOpen` keep their access path. Pattern proven by Find +
// SlashMenu + ContextMenu controllers (lanes 2 + 3 + 4).
//
// Escape priority chain (§4 option C). After this extraction the
// host's Escape ladder reads:
//
//   contextMenuController.tryHandleEscape()
//   → helpOverlayController.tryHandleEscape()
//   → findController.tryHandleEscape()
//   → slashMenuController.tryHandleEscape()
//
// Every overlay now routes through a controller.
//
// Note on outside-click: unlike ContextMenu + SlashMenu, the help
// overlay does NOT install a window-level click listener. Dismissal
// is handled inline by the render template's backdrop @click +
// close-button @click — both call back into chat-window's
// helpOpen-setter shim which routes through this controller's
// `close()`. The controller therefore has no `hostConnected` /
// `hostDisconnected` work to do; lifecycle is a no-op.
//
// Usage (inside LthnChatWindow-shaped host):
//
//   private _helpOverlayController = new HelpOverlayController(this);
//
//   get helpOpen() { return this._helpOverlayController.open; }
//   set helpOpen(v) {
//     this._helpOverlayController.open = v;
//     this.requestUpdate();
//   }

import type { ReactiveController, ReactiveControllerHost } from "lit";

/** Structural shape the controller needs off its host. Kept as the
 *  bare ReactiveControllerHost so the controller can be unit-tested
 *  against a plain object stub with zero Lit lifecycle. */
export type HelpOverlayHost = ReactiveControllerHost;

export class HelpOverlayController implements ReactiveController {
  /** Public for test-introspection + the host's getter shim. false =
   *  closed (default, matches the previous host init). */
  open: boolean = false;

  constructor(private readonly host: HelpOverlayHost) {
    host.addController(this);
  }

  /** Lit lifecycle — no-op. The overlay has no window-level listener;
   *  dismissal flows in through the render template's @click handlers
   *  back to `close()`. Stub here to satisfy ReactiveController and
   *  to leave a hook in case future mechanics need it (e.g. focus
   *  trap, restore-focus-on-close). */
  hostConnected(): void { /* no-op — see class comment */ }

  /** Lit lifecycle — no-op symmetric with hostConnected. */
  hostDisconnected(): void { /* no-op — see class comment */ }

  /** Open the overlay. Idempotent — second call when already open
   *  is a no-op. Was `this.helpOpen = true;` inline on the host
   *  (slash-help command + ? toggle). */
  show(): void {
    if (this.open) return;
    this.open = true;
    this.host.requestUpdate();
  }

  /** Force the overlay closed. Idempotent — returns true when a
   *  transition actually happened. Used by the host's shim setter
   *  when assigning false, by the backdrop @click + close-button
   *  @click in the render template, and by tryHandleEscape below. */
  close(): boolean {
    if (!this.open) return false;
    this.open = false;
    this.host.requestUpdate();
    return true;
  }

  /** Toggle the overlay open/closed. Was `this.helpOpen = !this.helpOpen`
   *  on the host's ? bare-key branch — a second ? while open closes,
   *  matching the user's mental model of "press the same key again to
   *  dismiss". */
  toggle(): void {
    this.open = !this.open;
    this.host.requestUpdate();
  }

  /** Escape-priority hand-off (§4 option C). Returns true when this
   *  controller consumed the Escape — the host stops the priority
   *  chain. Returns false when the overlay is closed; the host moves
   *  on to the next overlay (find → slash). */
  tryHandleEscape(): boolean {
    if (!this.open) return false;
    this.open = false;
    this.host.requestUpdate();
    return true;
  }
}
