// SPDX-Licence-Identifier: EUPL-1.2
//
// Pure unit tests for ComposerGlyphController. No DOM mount, no Lit
// element registration — the controller is exercised against a plain
// stub host that satisfies the structural ComposerGlyphHost contract.
//
// The contentshield Wails binding (`@desktop/contentshield/wailsservice`)
// is mocked via `vi.doMock` so the dynamic import inside the controller
// resolves to the mock module. The mock returns the canonical Result
// envelope (`{ OK, Value }`) — the same shape the real Wails binding
// produces — so the controller's unwrap path is exercised end-to-end.
//
// Pre-warm pattern: every test resets the module cache, registers a
// mock with `vi.doMock`, then awaits one `import(...)` of the mocked
// module to warm vitest's loader cache BEFORE switching to fake timers.
// Without the pre-warm, fake timers + a setTimeout-driven dynamic
// import deadlock (vitest's async module resolver can't make progress
// while the test thread holds fake-timer scheduling). After pre-warm
// the controller's own `await import(...)` resolves synchronously off
// the cached module, so the debounce timer fires + microtasks drain
// cleanly under `vi.advanceTimersByTimeAsync`.

import { describe, it, expect, vi, afterEach } from "vitest";
import {
  ComposerGlyphController,
  type ComposerGlyphHost,
  type ScoreResult,
  GLYPH_DEBOUNCE_MS,
  TIER_NAMES,
} from "./composer-glyph.controller";

function makeHost(seed: Partial<ComposerGlyphHost> = {}): ComposerGlyphHost & {
  _updateCount: number;
} {
  let updateCount = 0;
  const host = {
    composerValue: seed.composerValue ?? "",
    addController() { /* no-op for tests */ },
    removeController() { /* no-op */ },
    requestUpdate() { updateCount++; },
    get updateComplete() { return Promise.resolve(true); },
    get _updateCount() { return updateCount; },
  } as ComposerGlyphHost & { _updateCount: number };
  return host;
}

/** Canonical tier-2 result the mocked binding returns. Mirrors the
 *  JSON shape of `contentshield.ScoreResult` — snake_case keys, the
 *  Sycophancy slot populated, no Imprint / Suggestions. */
const hollowFlatteryResult: ScoreResult = {
  sycophancy: {
    tier: 2,
    label: "hollow_flattery",
    composite: 47,
    phrases: { count_by_tier: { hollow_flattery: 2 } },
  },
};

/** Flush the microtask queue. Used after `advanceTimersByTimeAsync`
 *  to let the controller's `await import(...)` + `await mod.Score(...)`
 *  chain drain before the test assertions read `ctrl.score`. */
async function flushMicrotasks(): Promise<void> {
  for (let i = 0; i < 4; i++) await Promise.resolve();
}

/** Common setup — register a mock for the contentshield binding,
 *  pre-warm the module loader cache (real-time import resolves the
 *  doMock), then flip to fake timers for the test body. */
async function setupMock(score: ReturnType<typeof vi.fn>): Promise<void> {
  vi.resetModules();
  vi.doMock("@desktop/contentshield/wailsservice", () => ({ Score: score }));
  await import("@desktop/contentshield/wailsservice"); // pre-warm
  vi.useFakeTimers();
}

afterEach(() => {
  vi.useRealTimers();
  vi.doUnmock("@desktop/contentshield/wailsservice");
});

describe("ComposerGlyphController — notify_Good", () => {
  it("populates controller.score after debounce + Score resolves", async () => {
    const scoreMock = vi.fn().mockResolvedValue({ OK: true, Value: hollowFlatteryResult });
    await setupMock(scoreMock);

    const host = makeHost({ composerValue: "you're absolutely right" });
    const ctrl = new ComposerGlyphController(host);
    ctrl.notify();

    // Before the debounce elapses, Score hasn't been called + score is null.
    expect(ctrl.score).toBeNull();
    expect(scoreMock).not.toHaveBeenCalled();

    await vi.advanceTimersByTimeAsync(GLYPH_DEBOUNCE_MS);
    await flushMicrotasks();

    expect(scoreMock, "Score called exactly once after debounce").toHaveBeenCalledTimes(1);
    expect(scoreMock).toHaveBeenCalledWith("you're absolutely right");
    expect(ctrl.score, "score populated after debounce").not.toBeNull();
    expect(ctrl.score?.sycophancy?.tier).toBe(2);
    expect(ctrl.score?.sycophancy?.label).toBe("hollow_flattery");
    expect(host._updateCount, "requestUpdate fired").toBe(1);
  });
});

describe("ComposerGlyphController — notify_Bad_Reject", () => {
  it("keeps score null when Score rejects — graceful degrade, no throw", async () => {
    const scoreMock = vi.fn().mockRejectedValue(new Error("binding not wired"));
    await setupMock(scoreMock);

    const host = makeHost({ composerValue: "drafting…" });
    const ctrl = new ComposerGlyphController(host);
    ctrl.notify();
    await vi.advanceTimersByTimeAsync(GLYPH_DEBOUNCE_MS);
    await flushMicrotasks();

    expect(scoreMock).toHaveBeenCalledTimes(1);
    expect(ctrl.score, "score stays null on rejection").toBeNull();
    expect(host._updateCount, "no requestUpdate fired on failure path").toBe(0);
  });

  it("keeps score null when Result.OK is false", async () => {
    const scoreMock = vi.fn().mockResolvedValue({ OK: false, Value: "service down" });
    await setupMock(scoreMock);

    const host = makeHost({ composerValue: "drafting…" });
    const ctrl = new ComposerGlyphController(host);
    ctrl.notify();
    await vi.advanceTimersByTimeAsync(GLYPH_DEBOUNCE_MS);
    await flushMicrotasks();

    expect(scoreMock).toHaveBeenCalledTimes(1);
    expect(ctrl.score).toBeNull();
  });
});

describe("ComposerGlyphController — notify_UglyDebounce", () => {
  it("three rapid notify() calls coalesce to one Score invocation", async () => {
    const scoreMock = vi.fn().mockResolvedValue({ OK: true, Value: hollowFlatteryResult });
    await setupMock(scoreMock);

    const host = makeHost({ composerValue: "first" });
    const ctrl = new ComposerGlyphController(host);

    ctrl.notify();
    await vi.advanceTimersByTimeAsync(100); // mid-debounce
    host.composerValue = "first second";
    ctrl.notify();
    await vi.advanceTimersByTimeAsync(100); // still mid-debounce
    host.composerValue = "first second third";
    ctrl.notify();
    // Now let the last debounce elapse fully.
    await vi.advanceTimersByTimeAsync(GLYPH_DEBOUNCE_MS);
    await flushMicrotasks();

    expect(scoreMock, "only the last notify dispatches a Score call").toHaveBeenCalledTimes(1);
    expect(scoreMock).toHaveBeenCalledWith("first second third");
  });
});

describe("ComposerGlyphController — notify_UglyEmpty", () => {
  it("empty composerValue → Score is never called + score stays null", async () => {
    const scoreMock = vi.fn().mockResolvedValue({ OK: true, Value: hollowFlatteryResult });
    await setupMock(scoreMock);

    const host = makeHost({ composerValue: "" });
    const ctrl = new ComposerGlyphController(host);
    ctrl.notify();
    await vi.advanceTimersByTimeAsync(GLYPH_DEBOUNCE_MS * 2);
    await flushMicrotasks();

    expect(scoreMock, "no Score call for empty composer").not.toHaveBeenCalled();
    expect(ctrl.score, "score stays null").toBeNull();
  });

  it("transition non-empty → empty clears a previously-populated score", async () => {
    const scoreMock = vi.fn().mockResolvedValue({ OK: true, Value: hollowFlatteryResult });
    await setupMock(scoreMock);

    const host = makeHost({ composerValue: "you're absolutely right" });
    const ctrl = new ComposerGlyphController(host);

    ctrl.notify();
    await vi.advanceTimersByTimeAsync(GLYPH_DEBOUNCE_MS);
    await flushMicrotasks();
    expect(ctrl.score).not.toBeNull();

    // User deletes everything.
    host.composerValue = "";
    ctrl.notify();
    expect(ctrl.score, "empty composer clears the stale score").toBeNull();
  });
});

describe("ComposerGlyphController — reset_Good", () => {
  it("reset() clears a populated score back to null", async () => {
    const scoreMock = vi.fn().mockResolvedValue({ OK: true, Value: hollowFlatteryResult });
    await setupMock(scoreMock);

    const host = makeHost({ composerValue: "you're absolutely right" });
    const ctrl = new ComposerGlyphController(host);
    ctrl.notify();
    await vi.advanceTimersByTimeAsync(GLYPH_DEBOUNCE_MS);
    await flushMicrotasks();
    expect(ctrl.score).not.toBeNull();

    ctrl.reset();
    expect(ctrl.score, "score cleared on reset").toBeNull();
  });

  it("reset() on already-null score is a cheap no-op (no extra requestUpdate)", async () => {
    const scoreMock = vi.fn();
    await setupMock(scoreMock);

    const host = makeHost({ composerValue: "" });
    const ctrl = new ComposerGlyphController(host);
    const before = host._updateCount;
    ctrl.reset();
    expect(host._updateCount, "no spurious requestUpdate when nothing changed").toBe(before);
  });

  it("reset() cancels a pending debounce — no Score call lands after", async () => {
    const scoreMock = vi.fn().mockResolvedValue({ OK: true, Value: hollowFlatteryResult });
    await setupMock(scoreMock);

    const host = makeHost({ composerValue: "you're absolutely right" });
    const ctrl = new ComposerGlyphController(host);
    ctrl.notify();
    // Reset before the debounce fires.
    await vi.advanceTimersByTimeAsync(GLYPH_DEBOUNCE_MS / 2);
    ctrl.reset();
    // Let any zombie timer try to fire.
    await vi.advanceTimersByTimeAsync(GLYPH_DEBOUNCE_MS * 2);
    await flushMicrotasks();
    expect(scoreMock).not.toHaveBeenCalled();
    expect(ctrl.score).toBeNull();
  });
});

describe("ComposerGlyphController — hostDisconnected_Good", () => {
  it("cancels pending debounce + drops in-flight Score result", async () => {
    // Score resolution is artificially deferred so we can tear down
    // between the dispatch and the resolution, then advance the clock
    // and assert nothing late-writes onto ctrl.score.
    let resolveScore: (v: { OK: boolean; Value: ScoreResult }) => void = () => {};
    const scoreMock = vi.fn(() => new Promise<{ OK: boolean; Value: ScoreResult }>((res) => {
      resolveScore = res;
    }));
    await setupMock(scoreMock);

    const host = makeHost({ composerValue: "drafting" });
    const ctrl = new ComposerGlyphController(host);
    ctrl.notify();
    // Let the debounce timer fire — Score is now in flight.
    await vi.advanceTimersByTimeAsync(GLYPH_DEBOUNCE_MS);
    await flushMicrotasks();
    expect(scoreMock).toHaveBeenCalledTimes(1);

    // Tear down before the Score promise resolves.
    ctrl.hostDisconnected();

    // Now resolve the in-flight Score — the controller's stale-seq
    // guard MUST drop it without touching ctrl.score or calling
    // requestUpdate.
    const updateBefore = host._updateCount;
    resolveScore({ OK: true, Value: hollowFlatteryResult });
    await flushMicrotasks();

    expect(ctrl.score, "no late write to score after teardown").toBeNull();
    expect(host._updateCount, "no late requestUpdate after teardown").toBe(updateBefore);
  });

  it("cancels a pending debounce — no Score call after teardown", async () => {
    const scoreMock = vi.fn().mockResolvedValue({ OK: true, Value: hollowFlatteryResult });
    await setupMock(scoreMock);

    const host = makeHost({ composerValue: "drafting" });
    const ctrl = new ComposerGlyphController(host);
    ctrl.notify();
    ctrl.hostDisconnected();
    await vi.advanceTimersByTimeAsync(GLYPH_DEBOUNCE_MS * 2);
    await flushMicrotasks();
    expect(scoreMock).not.toHaveBeenCalled();
  });
});

describe("ComposerGlyphController — TIER_NAMES vocabulary", () => {
  it("covers all four sycophancy tiers with kebab-case names", () => {
    expect(TIER_NAMES[0]).toBe("appropriate-empathy");
    expect(TIER_NAMES[1]).toBe("soft-agreement");
    expect(TIER_NAMES[2]).toBe("hollow-flattery");
    expect(TIER_NAMES[3]).toBe("submission");
  });
});
