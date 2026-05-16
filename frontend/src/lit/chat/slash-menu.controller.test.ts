// SPDX-Licence-Identifier: EUPL-1.2
//
// Pure unit tests for SlashMenuController. No DOM mount, no Lit element
// registration — the controller is exercised against a plain stub host
// that satisfies the structural SlashMenuHost contract. End-to-end
// tests in chat-window.test.ts still drive the slash menu through the
// live element and remain the integration check.

import { describe, it, expect, beforeEach, afterEach } from "vitest";
import { SlashMenuController, type SlashMenuHost } from "./slash-menu.controller";

interface StubHost extends SlashMenuHost {
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

describe("SlashMenuController — defaults", () => {
  it("starts closed", () => {
    const host = makeHost();
    const ctrl = new SlashMenuController(host);
    expect(ctrl.open).toBe(false);
  });
});

describe("SlashMenuController — toggle()", () => {
  it("toggle() from closed opens", () => {
    const host = makeHost();
    const ctrl = new SlashMenuController(host);
    ctrl.toggle();
    expect(ctrl.open).toBe(true);
    expect(host._updateCount, "requestUpdate fired").toBe(1);
  });

  it("toggle() from open closes", () => {
    const host = makeHost();
    const ctrl = new SlashMenuController(host);
    ctrl.toggle(); // open
    ctrl.toggle(); // close
    expect(ctrl.open).toBe(false);
    expect(host._updateCount, "requestUpdate fired twice").toBe(2);
  });

  it("toggle() roundtrip leaves state at defaults", () => {
    const host = makeHost();
    const ctrl = new SlashMenuController(host);
    ctrl.toggle();
    ctrl.toggle();
    expect(ctrl.open).toBe(false);
  });
});

describe("SlashMenuController — close()", () => {
  it("close() from open transitions + returns true", () => {
    const host = makeHost();
    const ctrl = new SlashMenuController(host);
    ctrl.toggle();
    const transitioned = ctrl.close();
    expect(transitioned).toBe(true);
    expect(ctrl.open).toBe(false);
  });

  it("close() from closed is a no-op + returns false", () => {
    const host = makeHost();
    const ctrl = new SlashMenuController(host);
    const updatesBefore = host._updateCount;
    const transitioned = ctrl.close();
    expect(transitioned).toBe(false);
    expect(host._updateCount, "no requestUpdate on no-op close").toBe(updatesBefore);
  });
});

describe("SlashMenuController — tryHandleEscape()", () => {
  it("returns true + closes when open", () => {
    const host = makeHost();
    const ctrl = new SlashMenuController(host);
    ctrl.toggle();
    const consumed = ctrl.tryHandleEscape();
    expect(consumed, "open slash menu consumes Escape").toBe(true);
    expect(ctrl.open).toBe(false);
  });

  it("returns false when closed (lets the host dispatch onward)", () => {
    const host = makeHost();
    const ctrl = new SlashMenuController(host);
    const consumed = ctrl.tryHandleEscape();
    expect(consumed).toBe(false);
  });
});

describe("SlashMenuController — outside-click", () => {
  let host: StubHost;
  let ctrl: SlashMenuController;

  beforeEach(() => {
    host = makeHost();
    ctrl = new SlashMenuController(host);
    ctrl.hostConnected();
  });

  afterEach(() => {
    ctrl.hostDisconnected();
  });

  it("a click outside the menu closes it when open", () => {
    ctrl.toggle();
    expect(ctrl.open).toBe(true);
    // Dispatch a real bubbling+composed click on document.body — the
    // controller's capture-phase window listener picks it up via the
    // capture chain. composedPath() returns body's natural path which
    // does not include any slash-menu sentinel class, so the handler
    // closes the menu.
    document.body.dispatchEvent(new MouseEvent("click", { bubbles: true, composed: true }));
    expect(ctrl.open, "outside click closes the menu").toBe(false);
  });

  it("a click inside .lthn-chat-slash-menu does NOT close", () => {
    ctrl.toggle();
    const inside = elementWithClass("lthn-chat-slash-menu");
    document.body.appendChild(inside);
    inside.dispatchEvent(clickWithPath([inside]));
    expect(ctrl.open, "menu stays open on inside click").toBe(true);
    inside.remove();
  });

  it("a click on .lthn-chat-slash-toggle does NOT close", () => {
    ctrl.toggle();
    const toggle = elementWithClass("lthn-chat-slash-toggle");
    document.body.appendChild(toggle);
    toggle.dispatchEvent(clickWithPath([toggle]));
    expect(ctrl.open).toBe(true);
    toggle.remove();
  });

  it("a click on .lthn-slash-menu-anchor does NOT close", () => {
    ctrl.toggle();
    const anchor = elementWithClass("lthn-slash-menu-anchor");
    document.body.appendChild(anchor);
    anchor.dispatchEvent(clickWithPath([anchor]));
    expect(ctrl.open).toBe(true);
    anchor.remove();
  });

  it("a click is a no-op when the menu is already closed", () => {
    // Menu closed (default).
    const updatesBefore = host._updateCount;
    const outside = document.createElement("div");
    document.body.appendChild(outside);
    outside.dispatchEvent(clickWithPath([outside]));
    expect(ctrl.open).toBe(false);
    expect(host._updateCount, "no requestUpdate when already closed").toBe(updatesBefore);
    outside.remove();
  });
});

describe("SlashMenuController — lifecycle", () => {
  it("hostDisconnected() removes the window listener (outside click no longer closes)", () => {
    const host = makeHost();
    const ctrl = new SlashMenuController(host);
    ctrl.hostConnected();
    ctrl.toggle(); // open
    ctrl.hostDisconnected();
    // After disconnect, a click that would otherwise close shouldn't fire the handler.
    const outside = document.createElement("div");
    window.dispatchEvent(clickWithPath([outside]));
    expect(ctrl.open, "listener stripped — menu state unchanged").toBe(true);
  });
});
