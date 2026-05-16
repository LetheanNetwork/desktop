// SPDX-Licence-Identifier: EUPL-1.2

import { describe, it, expect, beforeEach, vi } from "vitest";
import "./incidents";

// Mock the @desktop/incidents/service module so we control List() output.
vi.mock("@desktop/incidents/service", () => ({
  List: vi.fn(),
}));

import { List } from "@desktop/incidents/service";

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
  beforeEach(() => {
    (List as unknown as { mockReset: () => void }).mockReset();
    document.body.innerHTML = "";
  });

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

  it("loads incidents from backend when binding resolves", async () => {
    (List as unknown as { mockResolvedValue: (v: unknown) => void }).mockResolvedValue({
      Value: {
        incidents: [
          { id: "2026-05-INC-001", ts: "now", sev: "P2", state: "investigating",
            title: "hub · elevated latency", svc: "hub", who: "Mei", comments: 1 },
        ],
      },
    });
    const el = await mount();
    // Allow connectedCallback's _loadFromBackend to settle.
    await new Promise((r) => setTimeout(r, 0));
    await el.updateComplete;
    expect(el.incidents.length).toBe(1);
    expect(el.textContent).toContain("hub · elevated latency");
  });

  it("keeps fixture data when binding rejects", async () => {
    (List as unknown as { mockRejectedValue: (v: unknown) => void })
      .mockRejectedValue(new Error("no binding"));
    const el = await mount();
    await new Promise((r) => setTimeout(r, 0));
    await el.updateComplete;
    // Fixture should still be in place (5 entries).
    expect(el.incidents.length).toBe(5);
  });
});
