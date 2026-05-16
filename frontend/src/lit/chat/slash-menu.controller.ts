// SPDX-Licence-Identifier: EUPL-1.2

// Athena 2026-05-16: SlashMenuController is the third Reactive Controller
// extracted from chat-window.ts per §3.2 of
// `plans/ops/lthn/desktop/chat-window-controllers-plan.md`. Owns the
// composer slash-menu mechanics — open/closed flag, toggle, the
// window-level outside-click listener that auto-closes on stray clicks,
// and the Escape-priority hand-off. Render template + the slash-action
// methods (`_slashNew`, `_slashClear`, `_slashExport`, `_slashCopy`,
// `_slashHelp`, `_slashExportAll`) stay on chat-window — those mutate
// conversation state, not menu mechanics (§6 of the plan calls this
// boundary out explicitly).
//
// Owned state:
//   - `open: boolean` — was `slashMenuOpen` on the host.
//
// The host exposes a getter/setter shim for `slashMenuOpen` that
// delegates to `_slashMenuController.open`, so existing chat-window
// tests + render templates + the host's own slash-action methods
// (which set `this.slashMenuOpen = false` after firing) keep working
// unmodified. Pattern proven by FindController (lane 2).
//
// Escape priority chain (§4 option C — explicit declared order in
// the host dispatcher). The host calls `tryHandleEscape()` and the
// controller decides whether it owned the keystroke. After this
// extraction the host's Escape ladder reads:
//   contextMenu → helpOpen → findController.tryHandleEscape()
//                          → slashMenuController.tryHandleEscape()
//
// Usage (inside LthnChatWindow-shaped host):
//
//   private _slashMenuController = new SlashMenuController(this);
//
//   get slashMenuOpen() { return this._slashMenuController.open; }
//   set slashMenuOpen(v: boolean) {
//     if (this._slashMenuController.open === v) return;
//     this._slashMenuController.open = v;
//     this.requestUpdate();
//   }
//
//   _toggleSlashMenu() { this._slashMenuController.toggle(); }

import type { ReactiveController, ReactiveControllerHost } from "lit";

/** Structural shape the controller needs off its host. Kept as an
 *  interface (not an import of LthnChatWindow) so the controller can
 *  be unit-tested against a plain object stub with zero Lit lifecycle. */
export interface SlashMenuHost extends ReactiveControllerHost {
  /* No host-side reads required today — the menu's "should I close on
   * this click?" check inspects `ev.composedPath()` against well-known
   * class names, not host state. The interface is the empty
   * ReactiveControllerHost so future hooks (e.g. host-supplied
   * sentinel class names) can extend it without breaking tests. */
}

/** Class names of DOM nodes that are considered "inside" the slash
 *  menu for outside-click purposes. A click whose composedPath
 *  contains any of these is NOT outside — those paths handle their
 *  own state (toggle button calls `toggle()`, slash-cmd buttons call
 *  the action which closes the menu explicitly), so we don't double-
 *  close from the window listener. */
const SLASH_INSIDE_CLASSES = [
  "lthn-chat-slash-menu",
  "lthn-chat-slash-toggle",
  "lthn-slash-menu-anchor",
] as const;

export class SlashMenuController implements ReactiveController {
  /** Public for test-introspection. Defaults match the previous host
   *  default: menu closed. */
  open: boolean = false;

  constructor(private readonly host: SlashMenuHost) {
    host.addController(this);
  }

  /** Lit lifecycle — wire the window outside-click listener on mount.
   *  Was `window.addEventListener("click", this._onOutsideClickForSlash, true)`
   *  in chat-window's connectedCallback. Capture phase so the
   *  listener sees the click even if the deeper handler stops
   *  propagation. */
  hostConnected(): void {
    window.addEventListener("click", this._onOutsideClick, true);
  }

  /** Lit lifecycle — strip the listener on unmount so we don't leak
   *  across mount/unmount cycles. Was the matching removeEventListener
   *  in chat-window's disconnectedCallback. */
  hostDisconnected(): void {
    window.removeEventListener("click", this._onOutsideClick, true);
  }

  /** Flip the menu open ↔ closed. Was `_toggleSlashMenu()` on the
   *  host. Calling code goes through this so the requestUpdate stays
   *  atomic with the mutation. */
  toggle(): void {
    this.open = !this.open;
    this.host.requestUpdate();
  }

  /** Force the menu closed (idempotent). Used by slash-action methods
   *  on the host that need to close-then-fire without caring about
   *  the previous state. Returns true when a transition actually
   *  happened — handy for tests + the outside-click guard. */
  close(): boolean {
    if (!this.open) return false;
    this.open = false;
    this.host.requestUpdate();
    return true;
  }

  /** Escape-priority hand-off (§4 option C). Returns true when this
   *  controller consumed the Escape — the host stops the priority
   *  chain. Returns false when the menu is closed; the host moves on
   *  to the next overlay in the chain (today there is no overlay
   *  after slash, but the contract stays symmetric with FindController). */
  tryHandleEscape(): boolean {
    if (!this.open) return false;
    this.open = false;
    this.host.requestUpdate();
    return true;
  }

  /** Outside-click handler — installed on `window` in `hostConnected`
   *  (capture phase). When the menu is open, any click whose
   *  composedPath does NOT include the menu / toggle / anchor closes
   *  the menu. Clicks INSIDE those zones are handled by their own
   *  click handlers (toggle button, slash-cmd buttons) and either
   *  flip `open` themselves or fire a slash-action that closes via
   *  `host.slashMenuOpen = false`. */
  private _onOutsideClick = (ev: MouseEvent): void => {
    if (!this.open) return;
    const path = (ev.composedPath?.() || []) as Element[];
    for (const node of path) {
      if (!(node instanceof Element)) continue;
      for (const cls of SLASH_INSIDE_CLASSES) {
        if (node.classList?.contains(cls)) return;
      }
    }
    this.open = false;
    this.host.requestUpdate();
  };
}
