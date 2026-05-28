// SPDX-Licence-Identifier: EUPL-1.2
//
// Pure unit tests for ChatTurnScoreController. No DOM mount, no Lit
// element registration — the controller is exercised against a plain
// stub host that satisfies the ChatTurnScoreHost contract.
//
// The contentshield Wails binding (`@desktop/contentshield/wailsservice`)
// is mocked via `vi.doMock` so the dynamic import inside the controller
// resolves to the mock module. The mock returns the canonical Result
// envelope (`{ OK, Value }`) — the same shape the real Wails binding
// produces — so the controller's unwrap path is exercised end-to-end.
//
// Pre-warm pattern mirrors composer-glyph.controller.test.ts: reset the
// module cache, register the mock, await one `import(...)` of the
// mocked module to warm vitest's loader cache. No fake-timers are
// needed here — there's no debounce; the dynamic import + ScorePair
// resolution IS the latency. Microtask flushes drain the promise
// chain between dispatch and assertion. See [[feedback_vi_domock_fake_timers_prewarm]].

import { describe, it, expect, vi, afterEach } from "vitest";
import {
  ChatTurnScoreController,
  type ChatTurnScoreHost,
  type DiffResult,
  TIER_NAMES,
} from "./chat-turn-score.controller";

function makeHost(): ChatTurnScoreHost & { _updateCount: number } {
  let updateCount = 0;
  const host = {
    addController() { /* no-op for tests */ },
    removeController() { /* no-op */ },
    requestUpdate() { updateCount++; },
    get updateComplete() { return Promise.resolve(true); },
    get _updateCount() { return updateCount; },
  } as ChatTurnScoreHost & { _updateCount: number };
  return host;
}

/** Canonical DiffResult the mocked binding returns. Mirrors the JSON
 *  shape of `contentshield.DiffResult` — snake_case keys on
 *  Differential + Authority, per-side Sycophancy with the response
 *  side carrying tier-2 (hollow_flattery) for the assertion. */
const submissivePairResult: DiffResult = {
  prompt: {
    sycophancy: { tier: 0, label: "appropriate_empathy", composite: 5 },
  },
  response: {
    sycophancy: { tier: 2, label: "hollow_flattery", composite: 58 },
  },
  differential: {
    echo: 0.62,
    verb_shift: 0.18,
    tense_shift: 0.04,
    noun_echo: 0.71,
    question_flip: 0,
    domain_shift: 0.12,
  },
  authority: {
    targets: ["the spec"],
    deference: 0.78,
    pattern: "deference",
  },
};

/** Flush microtasks so the controller's `await import(...)` +
 *  `await mod.ScorePair(...)` chain drains before assertions read
 *  the cache. */
async function flushMicrotasks(): Promise<void> {
  for (let i = 0; i < 16; i++) await Promise.resolve();
}

/** Common setup — register a mock for the contentshield binding +
 *  pre-warm the module loader cache (real-time import resolves the
 *  doMock). */
async function setupMock(scorePair: ReturnType<typeof vi.fn>): Promise<void> {
  vi.resetModules();
  vi.doMock("@desktop/contentshield/wailsservice", () => ({
    Score: vi.fn(),
    ScorePair: scorePair,
    Suggestions: vi.fn(),
  }));
  await import("@desktop/contentshield/wailsservice"); // pre-warm
}

afterEach(() => {
  vi.doUnmock("@desktop/contentshield/wailsservice");
});

describe("ChatTurnScoreController — score_GoodPendingThenResolves", () => {
  it("first call returns 'pending'; resolution populates cache + fires requestUpdate", async () => {
    const scorePairMock = vi.fn().mockResolvedValue({ OK: true, Value: submissivePairResult });
    await setupMock(scorePairMock);

    const host = makeHost();
    const ctrl = new ChatTurnScoreController(host);

    const first = ctrl.score(2, "explain the spec", "you're absolutely right, the spec says…");
    expect(first, "synchronous reader returns 'pending' on cache miss").toBe("pending");

    // The dispatch goes through `await import(...)` (one microtask) then
    // `await mod.ScorePair(...)` — drain microtasks before asserting the
    // spy was hit + before reading the populated cache.
    await flushMicrotasks();
    expect(scorePairMock, "ScorePair dispatched exactly once").toHaveBeenCalledTimes(1);
    expect(scorePairMock).toHaveBeenCalledWith("explain the spec", "you're absolutely right, the spec says…");

    await flushMicrotasks();

    const second = ctrl.score(2, "explain the spec", "you're absolutely right, the spec says…");
    expect(second, "post-resolution read returns the populated DiffResult").toMatchObject({
      response: { sycophancy: { tier: 2 } },
      differential: { echo: 0.62 },
      authority: { pattern: "deference" },
    });
    expect(host._updateCount, "requestUpdate fired on resolution").toBe(1);
  });
});

describe("ChatTurnScoreController — score_GoodCached", () => {
  it("populated cache returns the entry without a fresh ScorePair call", async () => {
    const scorePairMock = vi.fn().mockResolvedValue({ OK: true, Value: submissivePairResult });
    await setupMock(scorePairMock);

    const host = makeHost();
    const ctrl = new ChatTurnScoreController(host);

    ctrl.score(2, "p", "r");
    await flushMicrotasks();
    await flushMicrotasks();
    expect(scorePairMock).toHaveBeenCalledTimes(1);

    // Subsequent reads hit the cache — no fresh dispatch, no second
    // requestUpdate.
    ctrl.score(2, "p", "r");
    ctrl.score(2, "p", "r");
    expect(scorePairMock, "no fresh dispatch on cache hit").toHaveBeenCalledTimes(1);
    expect(host._updateCount, "no extra requestUpdate on cache hit").toBe(1);
  });
});

describe("ChatTurnScoreController — score_BadReject", () => {
  it("rejected promise → cache entry is null, requestUpdate still fired", async () => {
    const scorePairMock = vi.fn().mockRejectedValue(new Error("binding not wired"));
    await setupMock(scorePairMock);

    const host = makeHost();
    const ctrl = new ChatTurnScoreController(host);

    const first = ctrl.score(3, "prompt", "response");
    expect(first).toBe("pending");

    // Drain microtasks twice — once for the dynamic-import resolution,
    // once for the rejected ScorePair to propagate into the catch
    // branch of _dispatch.
    await flushMicrotasks();
    await flushMicrotasks();

    expect(ctrl.cache.get(3), "rejected dispatch lands as null in cache").toBeNull();
    expect(host._updateCount, "requestUpdate fires on graceful-degrade path").toBe(1);

    // A second read does NOT re-attempt — the null is sticky until
    // clear() / reset() bumps the seq.
    const second = ctrl.score(3, "prompt", "response");
    expect(second).toBeNull();
    expect(scorePairMock).toHaveBeenCalledTimes(1);
  });

  it("Result.OK=false → cache entry is null, requestUpdate fired", async () => {
    const scorePairMock = vi.fn().mockResolvedValue({ OK: false, Value: "service down" });
    await setupMock(scorePairMock);

    const host = makeHost();
    const ctrl = new ChatTurnScoreController(host);
    ctrl.score(0, "p", "r");
    await flushMicrotasks();
    await flushMicrotasks();
    expect(ctrl.cache.get(0)).toBeNull();
    expect(host._updateCount).toBe(1);
  });
});

describe("ChatTurnScoreController — score_UglyEmpty", () => {
  it("empty prompt → cached null, no ScorePair call", async () => {
    const scorePairMock = vi.fn().mockResolvedValue({ OK: true, Value: submissivePairResult });
    await setupMock(scorePairMock);

    const host = makeHost();
    const ctrl = new ChatTurnScoreController(host);

    const result = ctrl.score(5, "", "non-empty response");
    expect(result, "empty prompt collapses to null synchronously").toBeNull();
    expect(scorePairMock, "no ScorePair call for empty prompt").not.toHaveBeenCalled();
    expect(ctrl.cache.get(5)).toBeNull();
  });

  it("empty response → cached null, no ScorePair call", async () => {
    const scorePairMock = vi.fn().mockResolvedValue({ OK: true, Value: submissivePairResult });
    await setupMock(scorePairMock);

    const host = makeHost();
    const ctrl = new ChatTurnScoreController(host);

    const result = ctrl.score(7, "non-empty prompt", "");
    expect(result, "empty response collapses to null synchronously").toBeNull();
    expect(scorePairMock, "no ScorePair call for empty response").not.toHaveBeenCalled();
    expect(ctrl.cache.get(7)).toBeNull();
  });
});

describe("ChatTurnScoreController — reset_Good", () => {
  it("reset() clears every cache entry + fires requestUpdate", async () => {
    const scorePairMock = vi.fn().mockResolvedValue({ OK: true, Value: submissivePairResult });
    await setupMock(scorePairMock);

    const host = makeHost();
    const ctrl = new ChatTurnScoreController(host);
    ctrl.score(2, "p", "r");
    ctrl.score(4, "p", "r");
    await flushMicrotasks();
    await flushMicrotasks();
    expect(ctrl.cache.size).toBe(2);

    const before = host._updateCount;
    ctrl.reset();
    expect(ctrl.cache.size, "cache empty after reset").toBe(0);
    expect(host._updateCount, "reset fires one requestUpdate").toBe(before + 1);
  });

  it("reset() on already-empty cache is a no-op (no spurious requestUpdate)", async () => {
    const scorePairMock = vi.fn();
    await setupMock(scorePairMock);

    const host = makeHost();
    const ctrl = new ChatTurnScoreController(host);
    const before = host._updateCount;
    ctrl.reset();
    expect(host._updateCount, "no spurious requestUpdate when nothing changed").toBe(before);
  });
});

describe("ChatTurnScoreController — clear_Good", () => {
  it("clear(turnIndex) drops one entry, leaves the rest intact", async () => {
    const scorePairMock = vi.fn().mockResolvedValue({ OK: true, Value: submissivePairResult });
    await setupMock(scorePairMock);

    const host = makeHost();
    const ctrl = new ChatTurnScoreController(host);
    ctrl.score(2, "p", "r");
    ctrl.score(4, "p", "r");
    await flushMicrotasks();
    await flushMicrotasks();
    expect(ctrl.cache.size).toBe(2);

    const before = host._updateCount;
    ctrl.clear(2);
    expect(ctrl.cache.has(2)).toBe(false);
    expect(ctrl.cache.has(4)).toBe(true);
    expect(host._updateCount, "clear fires one requestUpdate").toBe(before + 1);
  });

  it("clear on a missing key is a no-op (no requestUpdate)", async () => {
    const scorePairMock = vi.fn();
    await setupMock(scorePairMock);

    const host = makeHost();
    const ctrl = new ChatTurnScoreController(host);
    const before = host._updateCount;
    ctrl.clear(99);
    expect(host._updateCount).toBe(before);
  });
});

describe("ChatTurnScoreController — hostDisconnected_Good", () => {
  it("teardown drops in-flight ScorePair result + clears the cache", async () => {
    // Defer the ScorePair resolution so we can tear down between
    // dispatch and resolution, then drain microtasks and assert
    // nothing late-writes the cache.
    let resolvePair: (v: { OK: boolean; Value: DiffResult }) => void = () => {};
    const scorePairMock = vi.fn(() => new Promise<{ OK: boolean; Value: DiffResult }>((res) => {
      resolvePair = res;
    }));
    await setupMock(scorePairMock);

    const host = makeHost();
    const ctrl = new ChatTurnScoreController(host);
    ctrl.score(2, "p", "r");
    // Drain so the dynamic-import resolves + the ScorePair call
    // actually fires (the promise stays pending — `resolvePair` is
    // the held resolver).
    await flushMicrotasks();
    expect(scorePairMock).toHaveBeenCalledTimes(1);
    expect(ctrl.cache.get(2)).toBe("pending");

    ctrl.hostDisconnected();
    expect(ctrl.cache.size, "cache cleared on disconnect").toBe(0);

    const updateBefore = host._updateCount;
    resolvePair({ OK: true, Value: submissivePairResult });
    await flushMicrotasks();
    await flushMicrotasks();

    expect(ctrl.cache.size, "no late write after teardown").toBe(0);
    expect(host._updateCount, "no late requestUpdate after teardown").toBe(updateBefore);
  });
});

describe("ChatTurnScoreController — stale_GuardDropsLateResolution", () => {
  it("reset() between dispatch and resolution discards the late result", async () => {
    let resolvePair: (v: { OK: boolean; Value: DiffResult }) => void = () => {};
    const scorePairMock = vi.fn(() => new Promise<{ OK: boolean; Value: DiffResult }>((res) => {
      resolvePair = res;
    }));
    await setupMock(scorePairMock);

    const host = makeHost();
    const ctrl = new ChatTurnScoreController(host);
    ctrl.score(1, "p", "r");
    expect(ctrl.cache.get(1)).toBe("pending");

    // Let the dynamic-import resolve so ScorePair is actually
    // invoked (its promise stays pending — the resolver is held).
    await flushMicrotasks();

    // Reset BEFORE the dispatch resolves. The seq bump must make the
    // resolver bail without writing to the cache.
    ctrl.reset();
    expect(ctrl.cache.size).toBe(0);

    const updateBefore = host._updateCount;
    resolvePair({ OK: true, Value: submissivePairResult });
    await flushMicrotasks();
    await flushMicrotasks();

    expect(ctrl.cache.has(1), "late resolution discarded — cache stays empty for turnIndex 1").toBe(false);
    expect(ctrl.cache.size).toBe(0);
    expect(host._updateCount, "no extra requestUpdate from the dropped resolution").toBe(updateBefore);
  });
});

describe("ChatTurnScoreController — TIER_NAMES vocabulary", () => {
  it("covers all four sycophancy tiers with kebab-case names", () => {
    expect(TIER_NAMES[0]).toBe("appropriate-empathy");
    expect(TIER_NAMES[1]).toBe("soft-agreement");
    expect(TIER_NAMES[2]).toBe("hollow-flattery");
    expect(TIER_NAMES[3]).toBe("submission");
  });
});
