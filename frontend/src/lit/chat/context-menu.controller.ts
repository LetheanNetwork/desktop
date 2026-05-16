// SPDX-Licence-Identifier: EUPL-1.2

// Athena 2026-05-16: ContextMenuController is the fourth Reactive
// Controller extracted from chat-window.ts per §3.4 of
// `plans/ops/lthn/desktop/chat-window-controllers-plan.md`. Owns the
// conversation-rail right-click context-menu mechanics — the
// open-at-coords state, the open-on-right-click entry point, the
// window-level outside-click listener, the Escape-priority hand-off,
// and the viewport-clamp position math.
//
// The context-menu ACTIONS (rename / pin / duplicate / export /
// delete the targeted conversation) STAY on chat-window — they
// mutate session state, not menu mechanics. Same separation as
// lane 3's slash-menu vs slash-actions (§6 of the plan).
//
// Owned state:
//   - `for: { id: string; x: number; y: number } | null` — was
//     `contextMenuFor` on the host. Null = closed.
//
// The host exposes a getter/setter shim for `contextMenuFor` that
// delegates to `_contextMenuController.for`, so existing chat-window
// tests + render templates that read `this.contextMenuFor.{id,x,y}`
// keep their access path. Pattern proven by Find + SlashMenu
// controllers (lanes 2 + 3).
//
// Escape priority chain (§4 option C). After this extraction the
// host's Escape ladder reads:
//
//   contextMenuController.tryHandleEscape()
//   → helpOpen (inline)
//   → findController.tryHandleEscape()
//   → slashMenuController.tryHandleEscape()
//
// Context-menu first per the catalogue's reasoning: most-recently-
// summoned overlay = topmost in the user's mental stack.
//
// Usage (inside LthnChatWindow-shaped host):
//
//   private _contextMenuController = new ContextMenuController(this);
//
//   get contextMenuFor() { return this._contextMenuController.for; }
//   set contextMenuFor(v) {
//     this._contextMenuController.for = v;
//     this.requestUpdate();
//   }

import type { ReactiveController, ReactiveControllerHost } from "lit";

/** Structural shape the controller needs off its host. Kept as an
 *  interface (not an import of LthnChatWindow) so the controller can
 *  be unit-tested against a plain object stub with zero Lit lifecycle. */
export interface ContextMenuHost extends ReactiveControllerHost {
  /* No host-side reads required today — open-on-right-click receives
   *  its coords + conversation id from the caller (the rail's
   *  @contextmenu handler on the host). The interface stays the bare
   *  ReactiveControllerHost so future hooks can extend it without
   *  breaking tests. */
}

/** Snapshot of the menu's open state. Mirrors the previous host
 *  `contextMenuFor` shape so render templates that read
 *  `cm.{id,x,y}` keep working unchanged. */
export interface ContextMenuState {
  id: string;
  x: number;
  y: number;
}

/** Class name on the rendered context-menu root. A click whose
 *  composedPath contains an element with this class is INSIDE the
 *  menu — item handlers close the menu explicitly, so the
 *  window-level outside-click listener stays a no-op for those. */
const CONTEXT_MENU_CLASS = "lthn-conversation-context-menu";

/** Viewport-clamp constants. The rendered menu is 180px wide,
 *  ~165px tall, with an 8px viewport-edge pad so the cursor lands
 *  inside the menu (predictable arrow-keys + click-through). Lifted
 *  verbatim from the previous inline math in `_renderContextMenu`. */
const MENU_W = 180;
const MENU_H = 165;
const VIEWPORT_PAD = 8;

export class ContextMenuController implements ReactiveController {
  /** Public for test-introspection + the host's getter shim. null =
   *  closed (default, matches the previous host init). */
  for: ContextMenuState | null = null;

  constructor(private readonly host: ContextMenuHost) {
    host.addController(this);
  }

  /** Lit lifecycle — wire the window outside-click listener on
   *  mount. Was `window.addEventListener("click", this._onOutsideClickForContext, true)`
   *  in chat-window's connectedCallback. Capture phase so the
   *  listener sees the click even if a deeper handler stops
   *  propagation. */
  hostConnected(): void {
    window.addEventListener("click", this._onOutsideClick, true);
  }

  /** Lit lifecycle — strip the listener on unmount so we don't leak
   *  across mount/unmount cycles. */
  hostDisconnected(): void {
    window.removeEventListener("click", this._onOutsideClick, true);
  }

  /** Open the menu at viewport coords (clientX/Y from the
   *  contextmenu event) targeting `id`. Was the body of
   *  `_onContextMenuRailRow` on the host. The host's @contextmenu
   *  handler still owns the surrounding ceremony (preventDefault,
   *  stopPropagation, setting activeConversationId, closing the
   *  slash menu) — those are session-state mutations, not menu
   *  mechanics. */
  openAt(id: string, x: number, y: number): void {
    this.for = { id, x, y };
    this.host.requestUpdate();
  }

  /** Force the menu closed (idempotent). Returns true when a
   *  transition actually happened. Used by `runAction` below and
   *  by the host's shim setter when assigning null. */
  close(): boolean {
    if (this.for === null) return false;
    this.for = null;
    this.host.requestUpdate();
    return true;
  }

  /** Run a context-menu action then dismiss the menu. Centralises
   *  the close-after-fire pattern so item handlers stay one-liners.
   *  Was `_runContextMenuAction` on the host. The action callback
   *  itself still lives on the host (rename / pin / duplicate /
   *  export / delete) — those mutate conversation state, not menu
   *  mechanics. */
  runAction(fn: () => unknown): void {
    this.for = null;
    this.host.requestUpdate();
    void fn();
  }

  /** Escape-priority hand-off (§4 option C). Returns true when this
   *  controller consumed the Escape — the host stops the priority
   *  chain. Returns false when the menu is closed; the host moves
   *  on to the next overlay (today: help → find → slash). */
  tryHandleEscape(): boolean {
    if (this.for === null) return false;
    this.for = null;
    this.host.requestUpdate();
    return true;
  }

  /** Viewport-clamped position for the menu. Render template calls
   *  this; the controller is the single source of truth for the
   *  math. Returns null when the menu is closed so the render can
   *  early-return symmetrically. */
  clampedPosition(): { x: number; y: number } | null {
    const cm = this.for;
    if (!cm) return null;
    const vw = typeof window !== "undefined" ? window.innerWidth  : cm.x + MENU_W;
    const vh = typeof window !== "undefined" ? window.innerHeight : cm.y + MENU_H;
    const maxX = vw - MENU_W - VIEWPORT_PAD;
    const maxY = vh - MENU_H - VIEWPORT_PAD;
    const x = Math.max(VIEWPORT_PAD, Math.min(cm.x, maxX));
    const y = Math.max(VIEWPORT_PAD, Math.min(cm.y, maxY));
    return { x, y };
  }

  /** Outside-click handler — installed on `window` in
   *  `hostConnected` (capture phase). When the menu is open, any
   *  click whose composedPath does NOT include the menu root
   *  closes the menu. Clicks INSIDE the menu are handled by their
   *  own item handlers (which call `runAction` to close + fire). */
  private _onOutsideClick = (ev: MouseEvent): void => {
    if (this.for === null) return;
    const path = (ev.composedPath?.() || []) as Element[];
    for (const node of path) {
      if (!(node instanceof Element)) continue;
      if (node.classList?.contains(CONTEXT_MENU_CLASS)) return;
    }
    this.for = null;
    this.host.requestUpdate();
  };
}
