// SPDX-Licence-Identifier: EUPL-1.2
//
// Pure unit tests for FindController. No DOM mount, no Lit element
// registration — the controller is exercised against a plain stub
// host that satisfies the structural FindHost contract. End-to-end
// tests in chat-window.test.ts still drive the find bar through the
// live element and remain the integration check.

import { describe, it, expect } from "vitest";
import { FindController, type FindHost } from "./find.controller";
import type { ChatTurn } from "../types";

function makeHost(seed: Partial<FindHost> = {}): FindHost & {
  _updateCount: number;
  _scrollCalls: number;
  _activeEl: Element | null;
} {
  let updateCount = 0;
  let scrollCalls = 0;
  let activeEl: Element | null = null;
  const host: FindHost & {
    _updateCount: number;
    _scrollCalls: number;
    _activeEl: Element | null;
  } = {
    liveTurns: seed.liveTurns ?? null,
    addController() { /* no-op for tests */ },
    removeController() { /* no-op */ },
    requestUpdate() { updateCount++; },
    get updateComplete() { return Promise.resolve(true); },
    querySelector(sel: string): Element | null {
      if (sel !== ".lthn-chat-find-active") return null;
      return activeEl;
    },
    get _updateCount() { return updateCount; },
    get _scrollCalls() { return scrollCalls; },
    get _activeEl() { return activeEl; },
    // Test-only setter exposed for the scroll test.
    set _activeEl(el: Element | null) { activeEl = el; },
  } as FindHost & {
    _updateCount: number;
    _scrollCalls: number;
    _activeEl: Element | null;
  };
  // Wire a fake active element factory the scroll test can flip on.
  (host as unknown as { _installActive: (text: string) => void })._installActive = (text: string) => {
    activeEl = {
      scrollIntoView: () => { scrollCalls++; },
      textContent: text,
    } as unknown as Element;
  };
  return host;
}

function turns(msgs: string[]): ChatTurn[] {
  return msgs.map(text => ({ role: "you" as const, text }));
}

describe("FindController — toggle()", () => {
  it("toggle() from closed opens + sets cursor to 0", () => {
    const host = makeHost();
    const ctrl = new FindController(host);
    expect(ctrl.open).toBe(false);
    ctrl.toggle();
    expect(ctrl.open).toBe(true);
    expect(ctrl.cursor).toBe(0);
    expect(host._updateCount, "requestUpdate fired").toBe(1);
  });

  it("toggle() from open closes + clears query + resets cursor", () => {
    const host = makeHost({ liveTurns: turns(["alpha"]) });
    const ctrl = new FindController(host);
    ctrl.toggle();           // open
    ctrl.setQuery("alpha");
    ctrl.cursor = 3;         // pretend the user navigated
    ctrl.toggle();           // close
    expect(ctrl.open).toBe(false);
    expect(ctrl.query).toBe("");
    expect(ctrl.cursor).toBe(0);
  });

  it("toggle() roundtrip: closed → open → closed leaves state at defaults", () => {
    const host = makeHost();
    const ctrl = new FindController(host);
    ctrl.toggle();
    ctrl.toggle();
    expect(ctrl.open).toBe(false);
    expect(ctrl.query).toBe("");
    expect(ctrl.cursor).toBe(0);
  });
});

describe("FindController — step()", () => {
  it("step(+1) cycles forward with wrap-around", () => {
    const host = makeHost({ liveTurns: turns(["aaa"]) }); // 3 matches of "a"
    const ctrl = new FindController(host);
    ctrl.toggle();
    ctrl.setQuery("a");
    expect(ctrl.matchCount()).toBe(3);
    expect(ctrl.cursor).toBe(0);

    ctrl.step(1);
    expect(ctrl.cursor).toBe(1);
    ctrl.step(1);
    expect(ctrl.cursor).toBe(2);
    ctrl.step(1);
    expect(ctrl.cursor, "wrap forward to 0").toBe(0);
  });

  it("step(-1) cycles backward with wrap-around", () => {
    const host = makeHost({ liveTurns: turns(["aaa"]) });
    const ctrl = new FindController(host);
    ctrl.toggle();
    ctrl.setQuery("a");

    ctrl.step(-1);
    expect(ctrl.cursor, "wrap backward to last").toBe(2);
    ctrl.step(-1);
    expect(ctrl.cursor).toBe(1);
  });

  it("step() is a no-op when there are no matches", () => {
    const host = makeHost({ liveTurns: turns(["alpha"]) });
    const ctrl = new FindController(host);
    ctrl.toggle();
    ctrl.setQuery("ghost-needle");
    const updatesBefore = host._updateCount;
    ctrl.step(1);
    expect(ctrl.cursor).toBe(0);
    expect(host._updateCount, "no requestUpdate on no-op").toBe(updatesBefore);
  });
});

describe("FindController — matchCount()", () => {
  it("returns the total occurrences across every turn", () => {
    const host = makeHost({
      liveTurns: [
        { role: "you",   text: "regex one and regex two" },
        { role: "model", text: "regex three" },
        { role: "you",   text: "no needle here" },
        { role: "model", text: "regex four" },
      ],
    });
    const ctrl = new FindController(host);
    ctrl.toggle();
    ctrl.setQuery("regex");
    expect(ctrl.matchCount()).toBe(4);
  });

  it("returns 0 when find is closed (even with a non-empty query)", () => {
    const host = makeHost({ liveTurns: turns(["regex regex"]) });
    const ctrl = new FindController(host);
    // Closed by default.
    ctrl.query = "regex";
    expect(ctrl.matchCount()).toBe(0);
  });

  it("returns 0 for a blank/whitespace-only query", () => {
    const host = makeHost({ liveTurns: turns(["regex regex"]) });
    const ctrl = new FindController(host);
    ctrl.toggle();
    ctrl.setQuery("   ");
    expect(ctrl.matchCount()).toBe(0);
  });

  it("returns 0 when liveTurns is null", () => {
    const host = makeHost({ liveTurns: null });
    const ctrl = new FindController(host);
    ctrl.toggle();
    ctrl.setQuery("anything");
    expect(ctrl.matchCount()).toBe(0);
  });
});

describe("FindController — setQuery()", () => {
  it("setQuery() resets cursor to 0", () => {
    const host = makeHost({ liveTurns: turns(["aaa"]) });
    const ctrl = new FindController(host);
    ctrl.toggle();
    ctrl.setQuery("a");
    ctrl.step(1); ctrl.step(1);
    expect(ctrl.cursor).toBe(2);

    ctrl.setQuery("different");
    expect(ctrl.cursor, "new query → cursor 0").toBe(0);
  });

  it("setQuery() to the same value is a no-op (no extra update fires)", () => {
    const host = makeHost({ liveTurns: turns(["aaa"]) });
    const ctrl = new FindController(host);
    ctrl.toggle();
    ctrl.setQuery("a");
    const updatesBefore = host._updateCount;
    ctrl.setQuery("a");
    expect(host._updateCount, "identical setQuery is a no-op").toBe(updatesBefore);
  });
});

describe("FindController — tryHandleEscape()", () => {
  it("returns true + closes when open", () => {
    const host = makeHost({ liveTurns: turns(["aaa"]) });
    const ctrl = new FindController(host);
    ctrl.toggle();
    ctrl.setQuery("a");
    const consumed = ctrl.tryHandleEscape();
    expect(consumed, "open find consumes Escape").toBe(true);
    expect(ctrl.open).toBe(false);
    expect(ctrl.query, "Escape clears query").toBe("");
    expect(ctrl.cursor).toBe(0);
  });

  it("returns false when closed (lets the host dispatch onward)", () => {
    const host = makeHost();
    const ctrl = new FindController(host);
    const consumed = ctrl.tryHandleEscape();
    expect(consumed).toBe(false);
  });
});

describe("FindController — tryHandleAccel()", () => {
  it("tryHandleAccel('f') toggles the bar + returns true", () => {
    const host = makeHost();
    const ctrl = new FindController(host);
    const consumed = ctrl.tryHandleAccel("f");
    expect(consumed).toBe(true);
    expect(ctrl.open).toBe(true);
  });

  it("tryHandleAccel('f') a second time closes the bar", () => {
    const host = makeHost();
    const ctrl = new FindController(host);
    ctrl.tryHandleAccel("f"); // open
    ctrl.tryHandleAccel("f"); // close
    expect(ctrl.open).toBe(false);
  });

  it("tryHandleAccel('b') returns false (not this controller's key)", () => {
    const host = makeHost();
    const ctrl = new FindController(host);
    const consumed = ctrl.tryHandleAccel("b");
    expect(consumed).toBe(false);
    expect(ctrl.open).toBe(false);
  });

  it("tryHandleAccel('n') returns false (not this controller's key)", () => {
    const host = makeHost();
    const ctrl = new FindController(host);
    expect(ctrl.tryHandleAccel("n")).toBe(false);
    expect(ctrl.tryHandleAccel("k")).toBe(false);
  });
});

describe("FindController — scrollActiveIntoView()", () => {
  it("scrolls the active match into view when one exists", () => {
    const host = makeHost({ liveTurns: turns(["aaa"]) });
    (host as unknown as { _installActive: (t: string) => void })._installActive("a");
    const ctrl = new FindController(host);
    ctrl.toggle();
    ctrl.scrollActiveIntoView();
    expect(host._scrollCalls, "scrollIntoView fired exactly once").toBe(1);
  });

  it("is a no-op when find is closed", () => {
    const host = makeHost({ liveTurns: turns(["aaa"]) });
    (host as unknown as { _installActive: (t: string) => void })._installActive("a");
    const ctrl = new FindController(host);
    // Closed by default — do NOT open.
    ctrl.scrollActiveIntoView();
    expect(host._scrollCalls).toBe(0);
  });

  it("is a no-op when no active span is in the DOM", () => {
    const host = makeHost({ liveTurns: turns(["aaa"]) });
    // Don't install an active element — querySelector returns null.
    const ctrl = new FindController(host);
    ctrl.toggle();
    expect(() => ctrl.scrollActiveIntoView()).not.toThrow();
    expect(host._scrollCalls).toBe(0);
  });
});
