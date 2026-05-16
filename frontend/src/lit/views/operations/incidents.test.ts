// SPDX-Licence-Identifier: EUPL-1.2

import { describe, it, expect, beforeEach } from "vitest";
import "./incidents";

async function mount(attrs: Record<string, string | boolean> = {}) {
  const el = document.createElement("lthn-view-incidents") as HTMLElement & {
    embedded: boolean;
    incidents: unknown[];
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

describe("<lthn-view-incidents>", () => {
  beforeEach(() => { document.body.innerHTML = ""; });

  it("renders without crashing", async () => {
    const el = await mount();
    expect(el).toBeDefined();
  });

  it("renders all 5 fixture incidents", async () => {
    const el = await mount();
    expect(el.incidents.length).toBe(5);
    expect(el.textContent).toContain("hub.host.uk.com");
    expect(el.textContent).toContain("DNS cache poisoning");
  });

  it("shows the active count + resolved count", async () => {
    const el = await mount();
    // 1 investigating + 1 post-mortem = 2 active, 3 resolved
    expect(el.textContent).toMatch(/2 active/);
    expect(el.textContent).toMatch(/3 resolved/);
  });

  it("includes the Vi post-mortem callout", async () => {
    const el = await mount();
    expect(el.textContent).toContain("Vi");
    expect(el.textContent).toContain("post-mortem");
  });

  it("renders the severity badges", async () => {
    const el = await mount();
    expect(el.textContent).toContain("P1");
    expect(el.textContent).toContain("P2");
    expect(el.textContent).toContain("P3");
  });

  it("embedded mode reflects the attribute", async () => {
    const el = await mount({ embedded: true });
    expect(el.hasAttribute("embedded")).toBe(true);
  });
});
