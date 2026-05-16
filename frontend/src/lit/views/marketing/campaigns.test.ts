// SPDX-Licence-Identifier: EUPL-1.2

import { describe, it, expect, beforeEach, vi } from "vitest";
import { mountWindow, expectChromeTitle, isEmbedded } from "../../../test/window-fixture";
import "./campaigns";

// Mock the @desktop/marketing/campaigns/service module so we control List() output.
vi.mock("@desktop/marketing/campaigns/service", () => ({
  List: vi.fn(),
}));

import { List } from "@desktop/marketing/campaigns/service";

describe("lthn-view-campaigns — smoke", () => {
  it("mounts with the Campaigns titlebar", async () => {
    const { host } = await mountWindow("lthn-view-campaigns");
    expectChromeTitle(host, "Campaigns");
    expect(host.querySelector("header")).not.toBeNull();
  });

  it("renders the four summary cards", async () => {
    const { host } = await mountWindow("lthn-view-campaigns");
    expect(host.textContent).toContain("Live");
    expect(host.textContent).toContain("Scheduled");
    expect(host.textContent).toContain("Reach · 30d");
    expect(host.textContent).toContain("Spend · 30d");
  });

  it("renders every fixture campaign as a row", async () => {
    const { host } = await mountWindow("lthn-view-campaigns");
    const rows = host.querySelectorAll(".lthn-view-campaigns-row");
    expect(rows.length).toBe(6);
  });

  it("each row carries a state data-attribute for live/scheduled/draft/complete", async () => {
    const { host } = await mountWindow("lthn-view-campaigns");
    const states = Array.from(host.querySelectorAll(".lthn-view-campaigns-row"))
      .map(r => r.getAttribute("data-state"));
    expect(states).toContain("live");
    expect(states).toContain("scheduled");
    expect(states).toContain("draft");
    expect(states).toContain("complete");
  });

  it("renders the channel label under each campaign name", async () => {
    const { host } = await mountWindow("lthn-view-campaigns");
    expect(host.textContent).toContain("earned");
    expect(host.textContent).toContain("paid");
    expect(host.textContent).toContain("email");
  });

  it("footer carries the UTM-tracked tag", async () => {
    const { host } = await mountWindow("lthn-view-campaigns");
    const footer = host.querySelector("footer");
    expect(footer?.textContent).toContain("UTM-tracked");
  });
});

describe("lthn-view-campaigns — two-shell", () => {
  it("embedded mode collapses the chrome", async () => {
    const { host } = await mountWindow("lthn-view-campaigns", { attrs: { embedded: "" } });
    expect(isEmbedded(host)).toBe(true);
    expect(host.querySelector("header")).toBeNull();
  });

  it("embedded mode still renders the campaign list", async () => {
    const { host } = await mountWindow("lthn-view-campaigns", { attrs: { embedded: "" } });
    expect(host.querySelectorAll(".lthn-view-campaigns-row").length).toBe(6);
  });
});

describe("lthn-view-campaigns — backend wire", () => {
  beforeEach(() => {
    (List as unknown as { mockReset: () => void }).mockReset();
    document.body.innerHTML = "";
  });

  it("loads campaigns from backend when binding resolves", async () => {
    (List as unknown as { mockResolvedValue: (v: unknown) => void }).mockResolvedValue({
      Value: {
        campaigns: [
          { name: "Product Hunt launch", state: "live", reach: "10 K", convert: "4%", spend: "£0", channel: "earned" },
        ],
        liveCount: 1,
        scheduledCount: 0,
      },
    });

    const el = document.createElement("lthn-view-campaigns") as HTMLElement & {
      campaigns: unknown[];
      updateComplete: Promise<boolean>;
    };
    document.body.appendChild(el);
    await el.updateComplete;
    // Allow connectedCallback's _loadFromBackend to settle.
    await new Promise((r) => setTimeout(r, 0));
    await el.updateComplete;
    expect(el.campaigns.length).toBe(1);
    expect(el.textContent).toContain("Product Hunt launch");
  });

  it("keeps fixture data when binding rejects", async () => {
    (List as unknown as { mockRejectedValue: (v: unknown) => void })
      .mockRejectedValue(new Error("no binding"));

    const el = document.createElement("lthn-view-campaigns") as HTMLElement & {
      campaigns: unknown[];
      updateComplete: Promise<boolean>;
    };
    document.body.appendChild(el);
    await el.updateComplete;
    await new Promise((r) => setTimeout(r, 0));
    await el.updateComplete;
    // Fixture should still be in place (6 entries).
    expect(el.campaigns.length).toBe(6);
  });

  it("keeps fixture data when binding is absent", async () => {
    // List mock returns null-ish (simulates missing binding).
    (List as unknown as { mockResolvedValue: (v: unknown) => void }).mockResolvedValue(null);

    const el = document.createElement("lthn-view-campaigns") as HTMLElement & {
      campaigns: unknown[];
      updateComplete: Promise<boolean>;
    };
    document.body.appendChild(el);
    await el.updateComplete;
    await new Promise((r) => setTimeout(r, 0));
    await el.updateComplete;
    expect(el.campaigns.length).toBe(6);
  });
});

describe("lthn-view-campaigns — conflict-reload listener (Cascade W2)", () => {
  beforeEach(() => {
    (List as unknown as { mockReset: () => void }).mockReset();
    document.body.innerHTML = "";
  });

  it("CONFLICT_RELOAD_EVENT with matching service triggers _loadFromBackend", async () => {
    const calls: number[] = [];
    (List as unknown as { mockImplementation: (fn: () => Promise<unknown>) => void })
      .mockImplementation(() => {
        calls.push(1);
        return Promise.resolve({ Value: { campaigns: [
          { name: "Reloaded", state: "live", reach: "1", convert: "1%", spend: "£0", channel: "earned" },
        ], liveCount: 1, scheduledCount: 0 } });
      });

    const el = document.createElement("lthn-view-campaigns") as HTMLElement & {
      campaigns: unknown[];
      updateComplete: Promise<boolean>;
    };
    document.body.appendChild(el);
    await el.updateComplete;
    await new Promise(r => setTimeout(r, 0));
    await el.updateComplete;
    const baseline = calls.length;

    window.dispatchEvent(new CustomEvent("lthn:conflict:reload-requested", {
      detail: { service: "campaigns.update" },
    }));
    await new Promise(r => setTimeout(r, 0));
    await el.updateComplete;

    expect(calls.length).toBeGreaterThan(baseline);
  });

  it("CONFLICT_RELOAD_EVENT with non-matching service is ignored", async () => {
    const calls: number[] = [];
    (List as unknown as { mockImplementation: (fn: () => Promise<unknown>) => void })
      .mockImplementation(() => {
        calls.push(1);
        return Promise.resolve({ Value: { campaigns: [], liveCount: 0, scheduledCount: 0 } });
      });

    const el = document.createElement("lthn-view-campaigns") as HTMLElement & {
      campaigns: unknown[];
      updateComplete: Promise<boolean>;
    };
    document.body.appendChild(el);
    await el.updateComplete;
    await new Promise(r => setTimeout(r, 0));
    await el.updateComplete;
    const baseline = calls.length;

    window.dispatchEvent(new CustomEvent("lthn:conflict:reload-requested", {
      detail: { service: "audience.update" }, // not ours
    }));
    await new Promise(r => setTimeout(r, 0));
    await el.updateComplete;

    expect(calls.length).toBe(baseline);
  });
});
