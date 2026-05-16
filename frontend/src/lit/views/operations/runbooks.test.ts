// SPDX-Licence-Identifier: EUPL-1.2

import { describe, it, expect, beforeEach } from "vitest";
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
  beforeEach(() => { document.body.innerHTML = ""; });

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
});
