// SPDX-Licence-Identifier: EUPL-1.2
//
// Pure unit tests for ContextMenuController. No DOM mount, no Lit
// element registration — the controller is exercised against a plain
// stub host that satisfies the structural ContextMenuHost contract.
// End-to-end tests in chat-window.test.ts still drive the context
// menu through the live element and remain the integration check.

import { describe, it, expect, beforeEach, afterEach } from "vitest";
import { ContextMenuController, type ContextMenuHost } from "./context-menu.controller";

interface StubHost extends ContextMenuHost {
  _updateCount: number;
}

function makeHost(): StubHost {
  let updateCount = 0;
  const host: StubHost = {
    addController() { /* no-op for tests */ },
    removeController() { /* no-op */ },
    requestUpdate() { updateCount++; },
    get updateComplete() { return Promise.resolve(true); },
    get _updateCount() { return updateCount; },
  };
  return host;
}

/** Build a Click event whose composedPath() returns the given chain.
 *  jsdom's MouseEvent.composedPath() walks the live DOM, so we fake
 *  it directly — matches the runtime shape the real handler reads
 *  (`ev.composedPath?.() || []`). */
function clickWithPath(path: Element[]): MouseEvent {
  const ev = new MouseEvent("click", { bubbles: true, composed: true });
  Object.defineProperty(ev, "composedPath", {
    value: () => path,
  });
  return ev;
}

function elementWithClass(cls: string): Element {
  const el = document.createElement("div");
  el.classList.add(cls);
  return el;
}

describe("ContextMenuController — defaults", () => {
  it("starts closed (for === null)", () => {
    const host = makeHost();
    const ctrl = new ContextMenuController(host);
    expect(ctrl.for).toBeNull();
  });
});

describe("ContextMenuController — openAt()", () => {
  it("opens with the supplied id + coords", () => {
    const host = makeHost();
    const ctrl = new ContextMenuController(host);
    ctrl.openAt("conv-7", 120, 80);
    expect(ctrl.for).not.toBeNull();
    expect(ctrl.for!.id).toBe("conv-7");
    expect(ctrl.for!.x).toBe(120);
    expect(ctrl.for!.y).toBe(80);
    expect(host._updateCount, "requestUpdate fired").toBe(1);
  });

  it("openAt twice swaps the target", () => {
    const host = makeHost();
    const ctrl = new ContextMenuController(host);
    ctrl.openAt("conv-a", 10, 10);
    ctrl.openAt("conv-b", 50, 60);
    expect(ctrl.for!.id).toBe("conv-b");
    expect(ctrl.for!.x).toBe(50);
    expect(ctrl.for!.y).toBe(60);
  });
});

describe("ContextMenuController — close()", () => {
  it("close() from open transitions + returns true", () => {
    const host = makeHost();
    const ctrl = new ContextMenuController(host);
    ctrl.openAt("conv-1", 0, 0);
    const transitioned = ctrl.close();
    expect(transitioned).toBe(true);
    expect(ctrl.for).toBeNull();
  });

  it("close() from closed is a no-op + returns false", () => {
    const host = makeHost();
    const ctrl = new ContextMenuController(host);
    const updatesBefore = host._updateCount;
    const transitioned = ctrl.close();
    expect(transitioned).toBe(false);
    expect(host._updateCount, "no requestUpdate on no-op close").toBe(updatesBefore);
  });
});

describe("ContextMenuController — runAction()", () => {
  it("runAction fires the callback AND closes the menu", () => {
    const host = makeHost();
    const ctrl = new ContextMenuController(host);
    ctrl.openAt("conv-1", 0, 0);
    let fired = false;
    ctrl.runAction(() => { fired = true; });
    expect(fired, "callback fired").toBe(true);
    expect(ctrl.for, "menu closed after action").toBeNull();
  });

  it("runAction tolerates a callback that returns a promise (fire-and-forget)", () => {
    const host = makeHost();
    const ctrl = new ContextMenuController(host);
    ctrl.openAt("conv-1", 0, 0);
    let fired = false;
    ctrl.runAction(() => Promise.resolve().then(() => { fired = true; }));
    // close happens synchronously even though the callback is async
    expect(ctrl.for).toBeNull();
    // callback resolves on the microtask queue — not asserted synchronously
    void fired;
  });
});

describe("ContextMenuController — tryHandleEscape()", () => {
  it("returns true + closes when open", () => {
    const host = makeHost();
    const ctrl = new ContextMenuController(host);
    ctrl.openAt("conv-1", 0, 0);
    const consumed = ctrl.tryHandleEscape();
    expect(consumed, "open context menu consumes Escape").toBe(true);
    expect(ctrl.for).toBeNull();
  });

  it("returns false when closed (lets the host dispatch onward)", () => {
    const host = makeHost();
    const ctrl = new ContextMenuController(host);
    const consumed = ctrl.tryHandleEscape();
    expect(consumed).toBe(false);
  });
});

describe("ContextMenuController — clampedPosition()", () => {
  it("returns null when the menu is closed", () => {
    const host = makeHost();
    const ctrl = new ContextMenuController(host);
    expect(ctrl.clampedPosition()).toBeNull();
  });

  it("returns the click coords when far from any viewport edge", () => {
    const host = makeHost();
    const ctrl = new ContextMenuController(host);
    // jsdom default viewport is 1024×768. (100,100) sits well inside.
    ctrl.openAt("c", 100, 100);
    const pos = ctrl.clampedPosition()!;
    expect(pos.x).toBe(100);
    expect(pos.y).toBe(100);
  });

  it("clamps X to maxX when click is near the right edge", () => {
    const host = makeHost();
    const ctrl = new ContextMenuController(host);
    const vw = window.innerWidth;
    // Click 4px from the right edge — menu width 180 + 8 pad means
    // maxX is vw - 188, and the click (vw - 4) exceeds it.
    ctrl.openAt("c", vw - 4, 100);
    const pos = ctrl.clampedPosition()!;
    expect(pos.x).toBe(vw - 180 - 8);
  });

  it("clamps Y to maxY when click is near the bottom edge", () => {
    const host = makeHost();
    const ctrl = new ContextMenuController(host);
    const vh = window.innerHeight;
    ctrl.openAt("c", 100, vh - 4);
    const pos = ctrl.clampedPosition()!;
    expect(pos.y).toBe(vh - 165 - 8);
  });

  it("floors X at VIEWPORT_PAD when click is near the left edge", () => {
    const host = makeHost();
    const ctrl = new ContextMenuController(host);
    ctrl.openAt("c", 0, 100);
    const pos = ctrl.clampedPosition()!;
    expect(pos.x).toBe(8);
  });

  it("floors Y at VIEWPORT_PAD when click is near the top edge", () => {
    const host = makeHost();
    const ctrl = new ContextMenuController(host);
    ctrl.openAt("c", 100, 0);
    const pos = ctrl.clampedPosition()!;
    expect(pos.y).toBe(8);
  });
});

describe("ContextMenuController — outside-click", () => {
  let host: StubHost;
  let ctrl: ContextMenuController;

  beforeEach(() => {
    host = makeHost();
    ctrl = new ContextMenuController(host);
    ctrl.hostConnected();
  });

  afterEach(() => {
    ctrl.hostDisconnected();
  });

  it("a click outside the menu closes it when open", () => {
    ctrl.openAt("conv-1", 0, 0);
    expect(ctrl.for).not.toBeNull();
    // Dispatch a real bubbling+composed click on document.body — the
    // controller's capture-phase window listener picks it up via the
    // capture chain. composedPath() returns body's natural path which
    // does NOT include the context-menu sentinel class, so the
    // handler closes. (lane 3 jsdom gotcha: dispatch on document.body,
    // not a detached node.)
    document.body.dispatchEvent(new MouseEvent("click", { bubbles: true, composed: true }));
    expect(ctrl.for, "outside click closes the menu").toBeNull();
  });

  it("a click inside .lthn-conversation-context-menu does NOT close", () => {
    ctrl.openAt("conv-1", 0, 0);
    const inside = elementWithClass("lthn-conversation-context-menu");
    document.body.appendChild(inside);
    inside.dispatchEvent(clickWithPath([inside]));
    expect(ctrl.for, "menu stays open on inside click").not.toBeNull();
    inside.remove();
  });

  it("a click is a no-op when the menu is already closed", () => {
    const updatesBefore = host._updateCount;
    const outside = document.createElement("div");
    document.body.appendChild(outside);
    outside.dispatchEvent(clickWithPath([outside]));
    expect(ctrl.for).toBeNull();
    expect(host._updateCount, "no requestUpdate when already closed").toBe(updatesBefore);
    outside.remove();
  });
});

describe("ContextMenuController — lifecycle", () => {
  it("hostDisconnected() removes the window listener (outside click no longer closes)", () => {
    const host = makeHost();
    const ctrl = new ContextMenuController(host);
    ctrl.hostConnected();
    ctrl.openAt("conv-1", 0, 0);
    ctrl.hostDisconnected();
    // After disconnect, a click that would otherwise close shouldn't fire the handler.
    const outside = document.createElement("div");
    window.dispatchEvent(clickWithPath([outside]));
    expect(ctrl.for, "listener stripped — menu state unchanged").not.toBeNull();
  });
});
