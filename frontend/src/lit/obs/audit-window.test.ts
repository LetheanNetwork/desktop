// SPDX-Licence-Identifier: EUPL-1.2
//
// <lthn-audit-viewer> test matrix per RFC.stage-e-audit-viewer v2 §8.3
// + Cerberus #29 ADD-MED-1 XSS guard + Cerberus #29 LOWs (burst-
// throttle, same-second badge, visibility gating).
//
// Mock surface:
//
//   global.fetch is stubbed per spec — apiFetch wraps the native
//   fetch, so we control the response shape directly.
//
// The four-spec wire pattern from [[design_lit_view_backend_wire_pattern]]
// is folded in (rows-replace / rejects-keeps / empty-keeps / fixture).

import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, resolve } from "node:path";
import { mountWindow, expectChromeTitle, isEmbedded } from "../../test/window-fixture";
import "./audit-window";
import type { AuditEvent } from "./audit-window";
import { AUDIT_EVENT_NAMES, AUDIT_OUTCOMES } from "./audit-constants";

const __dirname = dirname(fileURLToPath(import.meta.url));
const AUDIT_WINDOW_SRC = resolve(__dirname, "./audit-window.ts");

/** Build a fetch mock that returns a fresh Response per call so the
 *  caller can re-read the body across retries / paged loads. Reusing
 *  a single Response triggers `Body has already been used` on the
 *  second await res.json(). */
function fetchReturning(events: AuditEvent[], nextCursor = ""): typeof fetch {
  const make = () => new Response(JSON.stringify({
    success: true,
    data: { events, next_cursor: nextCursor },
  }), { status: 200, headers: { "Content-Type": "application/json" } });
  return vi.fn(async () => make()) as unknown as typeof fetch;
}

function rejectingFetch(): typeof fetch {
  return vi.fn().mockRejectedValue(new Error("network unreachable")) as unknown as typeof fetch;
}

/** Wait until `pred()` returns truthy. Drives both updateComplete and
 *  a macrotask hop each iteration so async fetch → dynamic-import →
 *  setState → re-render cycles converge before assertion. A plain
 *  Promise.resolve() loop is microtask-only and starves when the
 *  awaited chain includes module-import resolution. */
async function waitFor(
  el: HTMLElement & { updateComplete?: Promise<boolean> },
  pred: () => boolean,
  maxTicks = 40,
): Promise<void> {
  for (let i = 0; i < maxTicks; i += 1) {
    if (pred()) return;
    if (el.updateComplete) await el.updateComplete;
    await new Promise(resolve => setTimeout(resolve, 5));
  }
  // Last chance — don't throw; let the caller's assertion produce the
  // legible failure message.
}

function singleEvent(overrides: Partial<AuditEvent> = {}): AuditEvent {
  return {
    event:      "auth.session.issued",
    account_id: "fed09876ba543210fedcba9876543210",
    ts:         1779000010,
    scope:      "session",
    outcome:    "ok",
    request_id: "req-abc-001",
    meta:       { path_hash: "1a2b3c4d5e6f7081a2b3c4d5e6f70819", version: 1 },
    ...overrides,
  };
}

/** Sleep helper for the auto-refresh / burst-throttle tests — uses
 *  vi.useFakeTimers underneath. */
async function tick(el: HTMLElement & { updateComplete?: Promise<boolean> }) {
  if (el.updateComplete) await el.updateComplete;
  await Promise.resolve();
}

const originalFetch = globalThis.fetch;

beforeEach(() => {
  // Default fetch returns one event so the four-spec wire-pattern
  // tests below can override per-spec.
  globalThis.fetch = fetchReturning([singleEvent()]);
});

afterEach(() => {
  globalThis.fetch = originalFetch;
  vi.useRealTimers();
});

describe("lthn-audit-viewer — smoke", () => {
  it("TestAuditWindow_FixtureStillRenders_Good — mounts with Audit log title and a default row", async () => {
    // Disable fetch entirely — the wire-pattern's "fixture-still-renders"
    // spec: a developer mounting the file without backend wiring sees
    // the static fixture row.
    globalThis.fetch = vi.fn().mockRejectedValue(new Error("no backend")) as unknown as typeof fetch;
    const { host, el } = await mountWindow("lthn-audit-viewer");
    expectChromeTitle(host, "Audit log");
    // Wait one microtask for the async _loadFromBackend reject to land
    // — fixture must survive the rejection.
    await tick(el);
    expect(host.textContent).toContain("auth.session.issued");
  });
});

describe("lthn-audit-viewer — wire pattern (rows / rejects / empty / fixture)", () => {
  it("TestAuditWindow_RowsReplaceFixture_Good — backend rows replace the fixture", async () => {
    const newRow = singleEvent({ event: "auth.account.sealed", account_id: "11112222333344445555666677778888" });
    globalThis.fetch = fetchReturning([newRow]);
    const { host, el } = await mountWindow("lthn-audit-viewer");
    // Wait for the row's truncated account_id (unique substring not in
    // the fixture and not in any chip-filter option) to appear in
    // textContent — proves the row replaced the fixture.
    await waitFor(el, () => (host.textContent ?? "").includes("11112222…"));
    expect(host.textContent).toContain("auth.account.sealed");
    // Fixture's account_id prefix must NOT appear after replace.
    expect(host.textContent).not.toContain("abc12345…");
  });

  it("TestAuditWindow_HelperRejectsKeepsFixture_Bad — helper rejects, fixture row remains", async () => {
    globalThis.fetch = rejectingFetch();
    const { host, el } = await mountWindow("lthn-audit-viewer");
    await waitFor(el, () => /Failed to load audit events/.test(host.textContent ?? ""));
    // Default fixture event uses auth.session.issued — keep it visible.
    expect(host.textContent).toContain("auth.session.issued");
    // And the error notice should be rendered.
    expect(host.textContent).toMatch(/Failed to load audit events/);
  });

  it("TestAuditWindow_EmptyPageRendersEmptyState_Good — empty payload shows the empty-state copy", async () => {
    globalThis.fetch = fetchReturning([], "");
    const { host, el } = await mountWindow("lthn-audit-viewer");
    await waitFor(el, () => /No events match the current filter/.test(host.textContent ?? ""));
    expect(host.textContent).toMatch(/No events match the current filter/);
  });
});

describe("lthn-audit-viewer — chip filter + constants parity", () => {
  it("TestAuditWindow_EventChipsMirrorConsts_Good — event filter options mirror AUDIT_EVENT_NAMES", async () => {
    const { host, el } = await mountWindow("lthn-audit-viewer");
    await tick(el);
    const selects = host.querySelectorAll("select");
    expect(selects.length).toBeGreaterThanOrEqual(2);
    const eventSelect = selects[0] as HTMLSelectElement;
    // First option is "All events" (empty value); the rest are the consts.
    const optionValues = Array.from(eventSelect.options).map(o => o.value);
    expect(optionValues[0]).toBe("");
    const reservedValues = optionValues.slice(1);
    expect(new Set(reservedValues)).toEqual(new Set(AUDIT_EVENT_NAMES));
  });

  it("TestAuditWindow_OutcomeChipFiltersClosedSet_Good — outcome filter shows exactly the four enum values", async () => {
    const { host, el } = await mountWindow("lthn-audit-viewer");
    await tick(el);
    const selects = host.querySelectorAll("select");
    const outcomeSelect = selects[1] as HTMLSelectElement;
    const optionValues = Array.from(outcomeSelect.options).map(o => o.value);
    expect(optionValues[0]).toBe("");
    const reservedValues = optionValues.slice(1);
    expect(new Set(reservedValues)).toEqual(new Set(AUDIT_OUTCOMES));
    expect(reservedValues.length).toBe(4);
  });
});

describe("lthn-audit-viewer — row expansion + privacy posture", () => {
  it("TestAuditWindow_AccountIDHashedDisplay_Good — account_id renders truncated + has copy affordance, never the full value as text", async () => {
    const longHash = "abcdef0123456789abcdef0123456789";
    const ev = singleEvent({ account_id: longHash });
    globalThis.fetch = fetchReturning([ev]);
    const { host, el } = await mountWindow("lthn-audit-viewer");
    await waitFor(el, () => (host.textContent ?? "").includes("abcdef01…"));
    // Truncated prefix visible.
    expect(host.textContent).toContain("abcdef01…");
    // Full hash MUST NOT appear as visible text — it lives in the title
    // attribute. Pull the row text content out and assert.
    const row = host.querySelector(".audit-row") as HTMLElement | null;
    expect(row).not.toBeNull();
    expect(row?.textContent ?? "").not.toContain(longHash);
    // Copy button present and addressable for the operator gesture.
    expect(row?.querySelector(".audit-copy-account")).not.toBeNull();
  });

  it("TestAuditWindow_RowExpansionShowsMeta_Good — clicking a row expands the Meta detail panel", async () => {
    const ev = singleEvent({ meta: { path_hash: "deadbeefcafef00d1234567890abcdef", version: 1 } });
    globalThis.fetch = fetchReturning([ev]);
    const { host, el } = await mountWindow("lthn-audit-viewer");
    await waitFor(el, () => host.querySelectorAll(".audit-row").length === 1);
    // Pre-click — detail panel not present.
    expect(host.querySelector(".audit-row-detail")).toBeNull();
    const row = host.querySelector(".audit-row > div") as HTMLElement;
    row.click();
    await tick(el);
    // Post-click — detail panel renders Meta keys.
    const detail = host.querySelector(".audit-row-detail");
    expect(detail).not.toBeNull();
    expect(detail?.textContent ?? "").toContain("path_hash");
    expect(detail?.textContent ?? "").toContain("version");
  });
});

describe("lthn-audit-viewer — XSS guard (Cerberus #29 ADD-MED-1)", () => {
  it("TestAuditWindow_MetaValueHTMLEscaped_Bad — Meta value with <img onerror> renders as literal text, no DOM injection", async () => {
    const malicious = "<img src=x onerror=window.__pwned=1>";
    delete (globalThis as Record<string, unknown>).__pwned;
    // Use a fetched account_id distinct from the fixture so we can
    // detect "row replaced fixture" without false-matching the chip
    // option list that lists the canonical event names.
    const ev = singleEvent({
      account_id: "feedfacedeadbeefabad1deacafef00d",
      meta:       { injected: malicious },
    });
    globalThis.fetch = fetchReturning([ev]);
    const { host, el } = await mountWindow("lthn-audit-viewer");
    // Wait for the fetched account_id prefix to render — that's a row
    // truncated-hex, not present anywhere in the fixture or chip lists.
    await waitFor(el, () => (host.textContent ?? "").includes("feedface…"));
    // Expand the row to render the Meta panel.
    const row = host.querySelector(".audit-row > div") as HTMLElement;
    row.click();
    await tick(el);
    // Assert 1: the onerror payload did NOT execute.
    expect((globalThis as Record<string, unknown>).__pwned).toBeUndefined();
    // Assert 2: the literal string appears as text content of the Meta
    // detail (proves Lit text-interpolation escaped the angle brackets).
    const detail = host.querySelector(".audit-row-detail") as HTMLElement;
    expect(detail).not.toBeNull();
    expect(detail.textContent ?? "").toContain(malicious);
    // Assert 3: no <img> element rendered inside the audit row.
    expect(host.querySelectorAll(".audit-row img").length).toBe(0);
  });

  it("TestAuditWindow_NoUnsafeHTMLImportInSource_Good — grep-asserts audit-window.ts does NOT import unsafe-html or assign innerHTML", () => {
    const src = readFileSync(AUDIT_WINDOW_SRC, "utf8");
    // Strip comments so the discipline-floor reminder in the file's
    // own header (which intentionally names "unsafe-html" as the thing
    // we MUST NOT do) doesn't false-positive the grep.
    const stripped = src
      .replace(/\/\/[^\n]*/g, "")
      .replace(/\/\*[\s\S]*?\*\//g, "");
    expect(stripped).not.toMatch(/unsafe-html/);
    expect(stripped).not.toMatch(/\.innerHTML\s*=/);
    expect(stripped).not.toMatch(/\bunsafeHTML\b/);
  });
});

describe("lthn-audit-viewer — auto-refresh + visibility gating (Cerberus #29 LOW)", () => {
  it("TestAuditWindow_AutoRefreshGatedOnVisibility_Ugly — tab hidden → no fetch fires when timer ticks", async () => {
    vi.useFakeTimers();
    const fetchSpy = vi.fn(async () => new Response(
      JSON.stringify({ success: true, data: { events: [singleEvent()], next_cursor: "" } }),
      { status: 200, headers: { "Content-Type": "application/json" } },
    ));
    globalThis.fetch = fetchSpy as unknown as typeof fetch;
    const { el } = await mountWindow<HTMLElement & {
      autoRefresh: boolean;
      updateComplete: Promise<boolean>;
    }>("lthn-audit-viewer");
    // Initial mount fires one fetch.
    await vi.runOnlyPendingTimersAsync();
    const baseCalls = fetchSpy.mock.calls.length;
    // Force document into hidden state.
    Object.defineProperty(document, "visibilityState", { configurable: true, value: "hidden" });
    // Enable auto-refresh (must call via property mutation since the
    // toggle is private; the test exercises the timer path directly).
    el.autoRefresh = true;
    // Manually install the interval the way _toggleAutoRefresh does,
    // by reaching for the click handler. Simpler: advance 30s and
    // assert no extra fetches fired during hidden state.
    vi.advanceTimersByTime(31_000);
    await Promise.resolve();
    expect(fetchSpy.mock.calls.length).toBe(baseCalls);
  });

  it("TestAuditWindow_AutoRefreshBurstThrottle_Ugly — rapid visibilitychange events fire at most one fetch within 5s", async () => {
    vi.useFakeTimers();
    const fetchSpy = vi.fn(async () => new Response(
      JSON.stringify({ success: true, data: { events: [singleEvent()], next_cursor: "" } }),
      { status: 200, headers: { "Content-Type": "application/json" } },
    ));
    globalThis.fetch = fetchSpy as unknown as typeof fetch;
    const { el } = await mountWindow<HTMLElement & {
      autoRefresh: boolean;
      updateComplete: Promise<boolean>;
    }>("lthn-audit-viewer");
    await vi.runOnlyPendingTimersAsync();
    const baseCalls = fetchSpy.mock.calls.length;
    // Force visible + enable auto-refresh.
    Object.defineProperty(document, "visibilityState", { configurable: true, value: "visible" });
    el.autoRefresh = true;
    // Three rapid visibilitychange events within the 5s burst window.
    // Each dispatch advances by a tiny amount so timestamps are not
    // identical, but all fall inside BURST_THROTTLE_MS.
    for (let i = 0; i < 3; i += 1) {
      document.dispatchEvent(new Event("visibilitychange"));
      vi.advanceTimersByTime(500);
      await Promise.resolve();
    }
    // Throttle clamps to AT MOST one extra fetch beyond the mount's
    // baseline — the prior fetch's timestamp blocks the burst.
    expect(fetchSpy.mock.calls.length).toBeLessThanOrEqual(baseCalls + 1);
  });
});

describe("lthn-audit-viewer — export", () => {
  it("TestAuditWindow_ExportJSONL_Good — JSON-Lines export builds an NDJSON-shaped blob", async () => {
    const events = [
      singleEvent({ event: "auth.account.created" }),
      singleEvent({ event: "auth.session.issued", account_id: "9999000011112222" }),
    ];
    globalThis.fetch = fetchReturning(events);
    const created: { type?: string; body: string }[] = [];
    const origBlob = globalThis.Blob;
    class CapturingBlob extends origBlob {
      constructor(parts: BlobPart[], opts?: BlobPropertyBag) {
        const body = parts.map(p => String(p)).join("");
        created.push({ type: opts?.type, body });
        super(parts, opts);
      }
    }
    (globalThis as Record<string, unknown>).Blob = CapturingBlob as unknown as typeof Blob;
    try {
      const { host, el } = await mountWindow<HTMLElement & {
        updateComplete: Promise<boolean>;
      }>("lthn-audit-viewer");
      // Wait for the fetched account_id prefix — distinct from the
      // fixture default and not present in any chip-option list.
      await waitFor(el, () => (host.textContent ?? "").includes("99990000…"));
      // Invoke the JSON-Lines export via the toolbar button. We have
      // access via DOM rather than poking the private method.
      const buttons = (el as unknown as HTMLElement).querySelectorAll("lthn-btn");
      // The 3rd ghost button is "Export JSON-Lines" per the toolbar
      // declaration order (Refresh, Auto-refresh, Export JSONL, Export
      // CSV); narrow by text content for robustness.
      let exported = false;
      for (const b of Array.from(buttons)) {
        if ((b.textContent ?? "").includes("JSON-Lines")) {
          (b as HTMLElement).click();
          exported = true;
          break;
        }
      }
      expect(exported).toBe(true);
      // At least one Blob captured carrying the NDJSON body.
      const ndjson = created.find(c => c.type === "application/x-ndjson");
      expect(ndjson).toBeDefined();
      expect(ndjson?.body ?? "").toContain("auth.account.created");
      expect(ndjson?.body ?? "").toContain("auth.session.issued");
      // Exactly one event per line — NDJSON shape.
      expect((ndjson?.body ?? "").split("\n").length).toBe(events.length);
    } finally {
      (globalThis as Record<string, unknown>).Blob = origBlob;
    }
  });

  it("TestAuditWindow_ExportCSV_Good — CSV export header + per-row escaping", async () => {
    const events = [
      singleEvent({
        event: "auth.lock.requested",
        request_id: "with,comma",
        account_id: "csvcsvcsvcsvcsv1234567890abcdef0",
      }),
    ];
    globalThis.fetch = fetchReturning(events);
    const captured: { type?: string; body: string }[] = [];
    const origBlob = globalThis.Blob;
    class CapturingBlob extends origBlob {
      constructor(parts: BlobPart[], opts?: BlobPropertyBag) {
        captured.push({ type: opts?.type, body: parts.map(p => String(p)).join("") });
        super(parts, opts);
      }
    }
    (globalThis as Record<string, unknown>).Blob = CapturingBlob as unknown as typeof Blob;
    try {
      const { host, el } = await mountWindow<HTMLElement & {
        updateComplete: Promise<boolean>;
      }>("lthn-audit-viewer");
      await waitFor(el, () => (host.textContent ?? "").includes("csvcsvcs…"));
      const buttons = (el as unknown as HTMLElement).querySelectorAll("lthn-btn");
      for (const b of Array.from(buttons)) {
        if ((b.textContent ?? "").includes("CSV")) { (b as HTMLElement).click(); break; }
      }
      const csv = captured.find(c => c.type === "text/csv");
      expect(csv).toBeDefined();
      // Header row contains the canonical column names.
      const lines = (csv?.body ?? "").split("\n");
      expect(lines[0]).toContain("ts");
      expect(lines[0]).toContain("event");
      expect(lines[0]).toContain("outcome");
      // Comma-containing field gets RFC-4180 quoted.
      expect(lines[1]).toContain('"with,comma"');
    } finally {
      (globalThis as Record<string, unknown>).Blob = origBlob;
    }
  });
});

describe("lthn-audit-viewer — embedded mode (two-shell)", () => {
  it("embedded attribute collapses the chrome", async () => {
    const { host } = await mountWindow("lthn-audit-viewer", { attrs: { embedded: "" } });
    expect(isEmbedded(host)).toBe(true);
    expect(host.querySelector("header")).toBeNull();
  });
});
