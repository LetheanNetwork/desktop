// SPDX-Licence-Identifier: EUPL-1.2
//
// Pure unit tests for ComposerHistoryController. No DOM mount, no Lit
// element registration — the controller is exercised against a plain
// stub host that satisfies the structural ComposerHistoryHost contract.
// This is the payoff for ReactiveController extraction: behaviour is
// testable in isolation without spinning up the whole chat-window.
//
// The end-to-end tests in chat-window.test.ts still drive
// `_composerKeydown` through the live element and remain the
// integration check that the wiring works.

import { describe, it, expect } from "vitest";
import { ComposerHistoryController, type ComposerHistoryHost } from "./composer-history.controller";
import type { ChatTurn } from "../types";

function makeHost(seed: Partial<ComposerHistoryHost> = {}): ComposerHistoryHost & {
  _updateCount: number;
} {
  let updateCount = 0;
  const host = {
    composerValue: seed.composerValue ?? "",
    liveTurns: seed.liveTurns ?? null,
    addController() { /* no-op for tests */ },
    removeController() { /* no-op */ },
    requestUpdate() { updateCount++; },
    get updateComplete() { return Promise.resolve(true); },
    get _updateCount() { return updateCount; },
  } as ComposerHistoryHost & { _updateCount: number };
  return host;
}

function turns(msgs: string[]): ChatTurn[] {
  return msgs.map(text => ({ role: "you" as const, text }));
}

function keyEvent(key: "ArrowUp" | "ArrowDown"): KeyboardEvent {
  return new KeyboardEvent("keydown", { key, bubbles: true, cancelable: true });
}

describe("ComposerHistoryController — up() / cursor walk", () => {
  it("up() on empty composer loads the newest entry + sets cursor to last index", () => {
    const host = makeHost({ composerValue: "", liveTurns: turns(["alpha", "bravo", "charlie"]) });
    const ctrl = new ComposerHistoryController(host);

    const consumed = ctrl.up(keyEvent("ArrowUp"));
    expect(consumed, "↑ on empty composer is consumed").toBe(true);
    expect(host.composerValue).toBe("charlie");
    expect(ctrl.cursor).toBe(2);
    expect(host._updateCount, "requestUpdate fired").toBe(1);
  });

  it("up() walks back through history; stays at oldest at the end", () => {
    const host = makeHost({ composerValue: "", liveTurns: turns(["alpha", "bravo", "charlie"]) });
    const ctrl = new ComposerHistoryController(host);

    ctrl.up(keyEvent("ArrowUp"));
    expect(host.composerValue).toBe("charlie");
    ctrl.up(keyEvent("ArrowUp"));
    expect(host.composerValue).toBe("bravo");
    ctrl.up(keyEvent("ArrowUp"));
    expect(host.composerValue).toBe("alpha");
    // Further up stays put — no wrap.
    ctrl.up(keyEvent("ArrowUp"));
    expect(host.composerValue, "stays at the oldest entry").toBe("alpha");
    expect(ctrl.cursor).toBe(0);
  });

  it("up() on non-empty composer (not in history) is a no-op", () => {
    const host = makeHost({ composerValue: "drafting…", liveTurns: turns(["alpha"]) });
    const ctrl = new ComposerHistoryController(host);

    const consumed = ctrl.up(keyEvent("ArrowUp"));
    expect(consumed, "↑ on a draft is NOT consumed — textarea default wins").toBe(false);
    expect(host.composerValue, "draft preserved").toBe("drafting…");
    expect(ctrl.cursor).toBe(-1);
    expect(host._updateCount, "no update fired for no-op").toBe(0);
  });

  it("up() with no user-message history is a no-op", () => {
    const host = makeHost({ composerValue: "", liveTurns: [{ role: "assistant", text: "hi" }] });
    const ctrl = new ComposerHistoryController(host);

    const consumed = ctrl.up(keyEvent("ArrowUp"));
    expect(consumed).toBe(false);
    expect(host.composerValue).toBe("");
    expect(ctrl.cursor).toBe(-1);
  });

  it("up() with null liveTurns is a no-op", () => {
    const host = makeHost({ composerValue: "", liveTurns: null });
    const ctrl = new ComposerHistoryController(host);

    const consumed = ctrl.up(keyEvent("ArrowUp"));
    expect(consumed).toBe(false);
    expect(host.composerValue).toBe("");
  });
});

describe("ComposerHistoryController — down() / cursor walk", () => {
  it("down() with cursor === -1 is a no-op (textarea default wins)", () => {
    const host = makeHost({ composerValue: "", liveTurns: turns(["alpha"]) });
    const ctrl = new ComposerHistoryController(host);

    const consumed = ctrl.down(keyEvent("ArrowDown"));
    expect(consumed).toBe(false);
    expect(host.composerValue).toBe("");
    expect(ctrl.cursor).toBe(-1);
    expect(host._updateCount).toBe(0);
  });

  it("down() walks forward through history after entering via up()", () => {
    const host = makeHost({ composerValue: "", liveTurns: turns(["alpha", "bravo", "charlie"]) });
    const ctrl = new ComposerHistoryController(host);

    ctrl.up(keyEvent("ArrowUp"));   // charlie (cursor 2)
    ctrl.up(keyEvent("ArrowUp"));   // bravo   (cursor 1)
    ctrl.up(keyEvent("ArrowUp"));   // alpha   (cursor 0)
    expect(host.composerValue).toBe("alpha");

    ctrl.down(keyEvent("ArrowDown")); // bravo (cursor 1)
    expect(host.composerValue).toBe("bravo");
    expect(ctrl.cursor).toBe(1);
    ctrl.down(keyEvent("ArrowDown")); // charlie (cursor 2)
    expect(host.composerValue).toBe("charlie");
  });

  it("down() past the newest exits history mode + clears composer", () => {
    const host = makeHost({ composerValue: "", liveTurns: turns(["alpha", "bravo"]) });
    const ctrl = new ComposerHistoryController(host);

    ctrl.up(keyEvent("ArrowUp"));   // bravo (newest, cursor 1)
    ctrl.up(keyEvent("ArrowUp"));   // alpha (cursor 0)
    expect(host.composerValue).toBe("alpha");

    ctrl.down(keyEvent("ArrowDown")); // bravo (cursor 1, newest)
    expect(host.composerValue).toBe("bravo");

    const consumed = ctrl.down(keyEvent("ArrowDown")); // exit
    expect(consumed, "exit-history is still consumed (preventDefault called)").toBe(true);
    expect(host.composerValue, "composer cleared on exit").toBe("");
    expect(ctrl.cursor, "cursor back to -1 on exit").toBe(-1);
  });
});

describe("ComposerHistoryController — reset()", () => {
  it("reset() puts cursor back to -1", () => {
    const host = makeHost({ composerValue: "", liveTurns: turns(["alpha", "bravo"]) });
    const ctrl = new ComposerHistoryController(host);

    ctrl.up(keyEvent("ArrowUp"));
    expect(ctrl.cursor).toBe(1);
    ctrl.reset();
    expect(ctrl.cursor, "cursor back to default after reset").toBe(-1);
  });

  it("after reset(), up() re-enters at the newest entry (same as fresh start)", () => {
    const host = makeHost({ composerValue: "", liveTurns: turns(["alpha", "bravo", "charlie"]) });
    const ctrl = new ComposerHistoryController(host);

    ctrl.up(keyEvent("ArrowUp")); // charlie
    ctrl.up(keyEvent("ArrowUp")); // bravo
    ctrl.reset();
    host.composerValue = ""; // host clears composer on its own paths (e.g. send)

    ctrl.up(keyEvent("ArrowUp"));
    expect(host.composerValue, "post-reset ↑ starts at newest again").toBe("charlie");
    expect(ctrl.cursor).toBe(2);
  });
});

describe("ComposerHistoryController — preventDefault contract", () => {
  it("up() calls preventDefault when it consumes the keystroke", () => {
    const host = makeHost({ composerValue: "", liveTurns: turns(["alpha"]) });
    const ctrl = new ComposerHistoryController(host);
    const ev = keyEvent("ArrowUp");
    ctrl.up(ev);
    expect(ev.defaultPrevented).toBe(true);
  });

  it("up() does NOT call preventDefault when it's a no-op", () => {
    const host = makeHost({ composerValue: "drafting…", liveTurns: turns(["alpha"]) });
    const ctrl = new ComposerHistoryController(host);
    const ev = keyEvent("ArrowUp");
    ctrl.up(ev);
    expect(ev.defaultPrevented, "no-op leaves textarea default alone").toBe(false);
  });

  it("down() does NOT call preventDefault when cursor === -1", () => {
    const host = makeHost({ composerValue: "", liveTurns: turns(["alpha"]) });
    const ctrl = new ComposerHistoryController(host);
    const ev = keyEvent("ArrowDown");
    ctrl.down(ev);
    expect(ev.defaultPrevented).toBe(false);
  });
});
