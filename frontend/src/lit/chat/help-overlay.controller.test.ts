// SPDX-Licence-Identifier: EUPL-1.2
//
// Pure unit tests for HelpOverlayController. No DOM mount, no Lit
// element registration — the controller is exercised against a plain
// stub host that satisfies the structural HelpOverlayHost contract.
// End-to-end tests in chat-window.test.ts still drive the help
// overlay through the live element and remain the integration check.

import { describe, it, expect } from "vitest";
import { HelpOverlayController, type HelpOverlayHost } from "./help-overlay.controller";

interface StubHost extends HelpOverlayHost {
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

describe("HelpOverlayController — defaults", () => {
  it("starts closed (open === false)", () => {
    const host = makeHost();
    const ctrl = new HelpOverlayController(host);
    expect(ctrl.open).toBe(false);
  });
});

describe("HelpOverlayController — show()", () => {
  it("transitions closed → open + fires requestUpdate", () => {
    const host = makeHost();
    const ctrl = new HelpOverlayController(host);
    ctrl.show();
    expect(ctrl.open).toBe(true);
    expect(host._updateCount, "requestUpdate fired once").toBe(1);
  });

  it("show() when already open is a no-op (no requestUpdate)", () => {
    const host = makeHost();
    const ctrl = new HelpOverlayController(host);
    ctrl.show();
    const before = host._updateCount;
    ctrl.show();
    expect(ctrl.open).toBe(true);
    expect(host._updateCount, "no requestUpdate on no-op show").toBe(before);
  });
});

describe("HelpOverlayController — close()", () => {
  it("close() from open transitions + returns true", () => {
    const host = makeHost();
    const ctrl = new HelpOverlayController(host);
    ctrl.show();
    const transitioned = ctrl.close();
    expect(transitioned).toBe(true);
    expect(ctrl.open).toBe(false);
  });

  it("close() from closed is a no-op + returns false", () => {
    const host = makeHost();
    const ctrl = new HelpOverlayController(host);
    const before = host._updateCount;
    const transitioned = ctrl.close();
    expect(transitioned).toBe(false);
    expect(host._updateCount, "no requestUpdate on no-op close").toBe(before);
  });
});

describe("HelpOverlayController — toggle()", () => {
  it("toggle from closed opens the overlay", () => {
    const host = makeHost();
    const ctrl = new HelpOverlayController(host);
    ctrl.toggle();
    expect(ctrl.open).toBe(true);
    expect(host._updateCount).toBe(1);
  });

  it("toggle from open closes the overlay (mirrors ? second-press)", () => {
    const host = makeHost();
    const ctrl = new HelpOverlayController(host);
    ctrl.show();
    ctrl.toggle();
    expect(ctrl.open).toBe(false);
  });

  it("toggle then toggle returns to start state", () => {
    const host = makeHost();
    const ctrl = new HelpOverlayController(host);
    ctrl.toggle();
    ctrl.toggle();
    expect(ctrl.open).toBe(false);
  });
});

describe("HelpOverlayController — tryHandleEscape()", () => {
  it("returns true + closes when open", () => {
    const host = makeHost();
    const ctrl = new HelpOverlayController(host);
    ctrl.show();
    const consumed = ctrl.tryHandleEscape();
    expect(consumed, "open help overlay consumes Escape").toBe(true);
    expect(ctrl.open).toBe(false);
  });

  it("returns false when closed (lets the host dispatch onward)", () => {
    const host = makeHost();
    const ctrl = new HelpOverlayController(host);
    const consumed = ctrl.tryHandleEscape();
    expect(consumed).toBe(false);
  });
});

describe("HelpOverlayController — lifecycle", () => {
  it("hostConnected + hostDisconnected are no-ops (no window listeners)", () => {
    const host = makeHost();
    const ctrl = new HelpOverlayController(host);
    // Just call them — there's nothing to assert other than that they
    // don't throw + don't mutate state. The class-level comment is
    // the load-bearing reason; this test pins the behaviour.
    ctrl.hostConnected();
    ctrl.hostDisconnected();
    expect(ctrl.open).toBe(false);
    expect(host._updateCount).toBe(0);
  });
});
