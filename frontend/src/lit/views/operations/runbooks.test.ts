// SPDX-Licence-Identifier: EUPL-1.2

import { describe, it, expect, beforeEach, vi } from "vitest";

// Module-level mock — overridden per test via the imported `List` ref.
vi.mock("@desktop/runbooks/service", () => ({
  List: vi.fn(),
}));

import { List as RunbooksList } from "@desktop/runbooks/service";

import "./runbooks";

async function mount(attrs: Record<string, string | boolean> = {}) {
  const el = document.createElement("lthn-view-runbooks") as HTMLElement & {
    embedded: boolean;
    filter: string;
    books: unknown[];
    updateComplete: Promise<boolean>;
  };
  for (const [k, v] of Object.entries(attrs)) {
    if (typeof v === "boolean") {
      if (v) el.setAttribute(k, "");
    } else {
      el.setAttribute(k, v);
    }
  }
  document.body.appendChild(el);
  await el.updateComplete;
  return el;
}

describe("<lthn-view-runbooks>", () => {
  beforeEach(() => {
    (RunbooksList as unknown as { mockReset: () => void }).mockReset();
    document.body.innerHTML = "";
  });

  it("renders all 7 fixture runbooks", async () => {
    const el = await mount();
    expect(el.books.length).toBe(7);
    expect(el.textContent).toContain("Rotate runtime API keys");
    expect(el.textContent).toContain("Drain a stuck Postfix queue");
  });

  it("shows fresh/aging/stale counts in the right rail", async () => {
    const el = await mount();
    // fixture: 4 fresh, 1 aging, 2 stale per the design.
    expect(el.textContent).toMatch(/\b4\b[\s\S]*fresh/i);
    expect(el.textContent).toMatch(/\b1\b[\s\S]*aging/i);
    expect(el.textContent).toMatch(/\b2\b[\s\S]*stale/i);
  });

  it("calls out stale runbooks for rehearsal", async () => {
    const el = await mount();
    expect(el.textContent).toContain("Stale");
    expect(el.textContent).toContain("R-05");
    expect(el.textContent).toContain("R-07");
  });

  it("search filter narrows the visible list", async () => {
    const el = await mount();
    el.filter = "runtime";
    await el.updateComplete;
    expect(el.textContent).toContain("Recover from corrupt model directory");
    expect(el.textContent).toContain("Trigger emergency model unload");
    expect(el.textContent).not.toContain("Drain a stuck Postfix queue");
  });

  it("empty filter shows everything", async () => {
    const el = await mount();
    el.filter = "";
    await el.updateComplete;
    expect(el.textContent).toContain("Rotate runtime API keys");
    expect(el.textContent).toContain("Drain a stuck Postfix queue");
  });

  it("embedded mode reflects the attribute", async () => {
    const el = await mount({ embedded: true });
    expect(el.hasAttribute("embedded")).toBe(true);
  });

  it("mentions ~/Lethean/runbooks/ as the canonical storage", async () => {
    const el = await mount();
    expect(el.textContent).toContain("~/Lethean/runbooks/");
  });

  it("replaces fixture with backend rows when List() returns books", async () => {
    (RunbooksList as unknown as { mockResolvedValue: (v: unknown) => void }).mockResolvedValue({
      Value: { books: [
        { id: "R-99", title: "Bootstrap a fresh M3 dev box", area: "client", last: "1 d", health: "fresh" },
        { id: "R-98", title: "Restore the rebuilt ~/Lethean/", area: "client", last: "3 d", health: "fresh" },
      ] },
    });
    const el = await mount();
    await new Promise((r) => setTimeout(r, 0));
    await el.updateComplete;
    expect(el.books.length).toBe(2);
    expect(el.textContent).toContain("Bootstrap a fresh M3 dev box");
    expect(el.textContent).not.toContain("Rotate runtime API keys"); // fixture pushed out
  });

  it("keeps fixture data when List() rejects", async () => {
    (RunbooksList as unknown as { mockRejectedValue: (v: unknown) => void }).mockRejectedValue(new Error("no binding"));
    const el = await mount();
    await new Promise((r) => setTimeout(r, 0));
    await el.updateComplete;
    expect(el.books.length).toBe(7); // FIXTURE_BOOKS retained
    expect(el.textContent).toContain("Rotate runtime API keys");
  });

  it("keeps fixture data when List() returns empty books", async () => {
    (RunbooksList as unknown as { mockResolvedValue: (v: unknown) => void }).mockResolvedValue({
      Value: { books: [] },
    });
    const el = await mount();
    await new Promise((r) => setTimeout(r, 0));
    await el.updateComplete;
    expect(el.books.length).toBe(7); // empty backend → fixture stays
  });
});

describe("<lthn-view-runbooks> — conflict-reload listener (Cascade W3)", () => {
  beforeEach(() => {
    (RunbooksList as unknown as { mockReset: () => void }).mockReset();
    document.body.innerHTML = "";
  });

  it("CONFLICT_RELOAD_EVENT with matching service triggers _loadFromBackend", async () => {
    const calls: number[] = [];
    (RunbooksList as unknown as { mockImplementation: (fn: () => Promise<unknown>) => void })
      .mockImplementation(() => {
        calls.push(1);
        return Promise.resolve({ Value: { books: [
          { id: "R-RELOAD", title: "Reloaded runbook", area: "client", last: "1 d", health: "fresh" },
        ] } });
      });

    const el = document.createElement("lthn-view-runbooks") as HTMLElement & {
      books: unknown[];
      updateComplete: Promise<boolean>;
    };
    document.body.appendChild(el);
    await el.updateComplete;
    await new Promise((r) => setTimeout(r, 0));
    await el.updateComplete;
    const baseline = calls.length;

    window.dispatchEvent(new CustomEvent("lthn:conflict:reload-requested", {
      detail: { service: "runbooks.update" },
    }));
    await new Promise((r) => setTimeout(r, 0));
    await el.updateComplete;

    expect(calls.length).toBeGreaterThan(baseline);
  });

  it("CONFLICT_RELOAD_EVENT with non-matching service is ignored", async () => {
    const calls: number[] = [];
    (RunbooksList as unknown as { mockImplementation: (fn: () => Promise<unknown>) => void })
      .mockImplementation(() => {
        calls.push(1);
        return Promise.resolve({ Value: { books: [] } });
      });

    const el = document.createElement("lthn-view-runbooks") as HTMLElement & {
      books: unknown[];
      updateComplete: Promise<boolean>;
    };
    document.body.appendChild(el);
    await el.updateComplete;
    await new Promise((r) => setTimeout(r, 0));
    await el.updateComplete;
    const baseline = calls.length;

    window.dispatchEvent(new CustomEvent("lthn:conflict:reload-requested", {
      detail: { service: "incidents.update" }, // not ours
    }));
    await new Promise((r) => setTimeout(r, 0));
    await el.updateComplete;

    expect(calls.length).toBe(baseline);
  });
});
