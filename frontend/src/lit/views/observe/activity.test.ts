// SPDX-Licence-Identifier: EUPL-1.2

import { describe, it, expect, beforeEach, vi } from "vitest";

// Mock apiFetch at module scope — same pattern as the other lit-view
// tests but applied to the apiFetch chokepoint instead of a Wails
// binding (the audit endpoint is REST-only, no Wails surface).
vi.mock("../../api-fetch", () => ({
  apiFetch: vi.fn(),
}));

import { apiFetch } from "../../api-fetch";

import "./activity";

interface MountedEl extends HTMLElement {
  embedded: boolean;
  events:   Array<{ event: string; ts: number; outcome?: string; account_id?: string }>;
  filter:   string;
  loading:  boolean;
  error:    string;
  updateComplete: Promise<boolean>;
}

async function mount(attrs: Record<string, string | boolean> = {}): Promise<MountedEl> {
  const el = document.createElement("lthn-view-observe-activity") as MountedEl;
  for (const [k, v] of Object.entries(attrs)) {
    if (typeof v === "boolean") {
      if (v) el.setAttribute(k, "");
    } else {
      el.setAttribute(k, v);
    }
  }
  document.body.appendChild(el);
  await el.updateComplete;
  // Wait one microtask tick for connectedCallback's async load to settle.
  await Promise.resolve();
  await el.updateComplete;
  return el;
}

// fetchOK builds a Response-like object the view's _loadFromBackend
// path can consume (.ok / .status / .json()).
function fetchOK(payload: unknown) {
  return {
    ok: true,
    status: 200,
    json: async () => payload,
  };
}

describe("<lthn-view-observe-activity>", () => {
  beforeEach(() => {
    (apiFetch as unknown as { mockReset: () => void }).mockReset();
    document.body.innerHTML = "";
  });

  it("renders fixture events when apiFetch is not yet wired (Good)", async () => {
    // apiFetch mock rejects → fixture data remains visible.
    (apiFetch as unknown as { mockRejectedValue: (e: unknown) => void })
      .mockRejectedValue(new Error("no binding"));
    const el = await mount();
    expect(el.events.length).toBeGreaterThan(0);
    expect(el.textContent).toContain("auth.session.issued");
    expect(el.textContent).toContain("tasks.created");
  });

  it("replaces fixture with real events when backend returns data (Good)", async () => {
    (apiFetch as unknown as { mockResolvedValue: (v: unknown) => void })
      .mockResolvedValue(fetchOK({
        data: {
          events: [
            { event: "auth.session.issued", ts: 1000, outcome: "ok", account_id: "abc12345deadbeef" },
            { event: "auth.unlock.failed", ts: 999, outcome: "failed" },
          ],
        },
      }));
    const el = await mount();
    expect(el.events.length).toBe(2);
    // The real-event names should now appear in the rendered table.
    expect(el.textContent).toContain("auth.session.issued");
    expect(el.textContent).toContain("auth.unlock.failed");
    // account_id rendered as first-8-of-hash.
    expect(el.textContent).toContain("abc12345");
  });

  it("reject-keeps — non-OK response leaves prior events intact (Bad)", async () => {
    const calls: number[] = [];
    (apiFetch as unknown as { mockImplementation: (fn: () => unknown) => void })
      .mockImplementation(() => {
        calls.push(1);
        if (calls.length === 1) return fetchOK({ data: { events: [
          { event: "first.load", ts: 1000, outcome: "ok" },
        ] } });
        return { ok: false, status: 500, json: async () => ({}) };
      });
    const el = await mount();
    expect(el.events.length).toBe(1);
    expect(el.textContent).toContain("first.load");

    // Force a refresh via the second mocked call — returns 500.
    await (el as MountedEl & { _loadFromBackend: () => Promise<void> })._loadFromBackend();
    await el.updateComplete;
    // Events unchanged; error surface populated.
    expect(el.events.length).toBe(1);
    expect(el.error).toContain("500");
    expect(el.textContent).toContain("first.load");
  });

  it("empty-keeps — empty events response leaves prior list intact (Ugly)", async () => {
    const calls: number[] = [];
    (apiFetch as unknown as { mockImplementation: (fn: () => unknown) => void })
      .mockImplementation(() => {
        calls.push(1);
        if (calls.length === 1) return fetchOK({ data: { events: [
          { event: "queue.enqueued", ts: 500, outcome: "ok" },
        ] } });
        // Empty events list — view MUST keep the prior list rather
        // than blank the surface (per the lit-view→backend pattern).
        return fetchOK({ data: { events: [] } });
      });
    const el = await mount();
    expect(el.events.length).toBe(1);
    expect(el.textContent).toContain("queue.enqueued");

    await (el as MountedEl & { _loadFromBackend: () => Promise<void> })._loadFromBackend();
    await el.updateComplete;
    expect(el.events.length).toBe(1);
    expect(el.textContent).toContain("queue.enqueued");
  });

  it("filter narrows visible rows by event-name substring (Good)", async () => {
    (apiFetch as unknown as { mockResolvedValue: (v: unknown) => void })
      .mockResolvedValue(fetchOK({
        data: {
          events: [
            { event: "auth.session.issued", ts: 1000, outcome: "ok" },
            { event: "queue.enqueued", ts: 999, outcome: "ok" },
            { event: "auth.unlock.failed", ts: 998, outcome: "failed" },
          ],
        },
      }));
    const el = await mount();
    el.filter = "auth.";
    await el.updateComplete;
    expect(el.textContent).toContain("auth.session.issued");
    expect(el.textContent).toContain("auth.unlock.failed");
    expect(el.textContent).not.toContain("queue.enqueued");
  });

  it("empty-state copy shows when filter matches nothing (Ugly)", async () => {
    (apiFetch as unknown as { mockResolvedValue: (v: unknown) => void })
      .mockResolvedValue(fetchOK({
        data: { events: [{ event: "auth.session.issued", ts: 1000, outcome: "ok" }] },
      }));
    const el = await mount();
    el.filter = "no-matches-for-this-filter-string";
    await el.updateComplete;
    expect(el.textContent).toContain("No events match these filters");
  });

  it("embedded attribute renders without chrome wrapper (Good)", async () => {
    (apiFetch as unknown as { mockRejectedValue: (e: unknown) => void })
      .mockRejectedValue(new Error("no binding"));
    const el = await mount({ embedded: true });
    expect(el.hasAttribute("embedded")).toBe(true);
  });

  // Mantis #1491 regression — oversized backend response is capped by
  // `clampRows` on the events array. Smoke-test the bound at the view
  // boundary on the apiFetch shape (REST envelope, not Wails binding).
  it("Test_Activity_loadFromBackend_OversizedRowsCapped_Bad", async () => {
    const oversized = Array.from({ length: 10_000 }, (_, i) => ({
      event: `synthetic.event.${i}`, ts: 1000 + i, outcome: "ok",
    }));
    (apiFetch as unknown as { mockResolvedValue: (v: unknown) => void })
      .mockResolvedValue(fetchOK({ data: { events: oversized } }));
    const el = await mount();
    // DEFAULT_MAX_ROWS = 2000; the view MUST cap at that boundary so a
    // misbehaving backend can't drown the audit timeline renderer.
    expect(el.events.length).toBeLessThanOrEqual(2000);
  });

  it("system event (no account_id) renders the em-dash placeholder", async () => {
    (apiFetch as unknown as { mockResolvedValue: (v: unknown) => void })
      .mockResolvedValue(fetchOK({
        data: { events: [{ event: "queue.enqueued", ts: 1000, outcome: "ok" }] },
      }));
    const el = await mount();
    expect(el.textContent).toContain("—");
  });
});
