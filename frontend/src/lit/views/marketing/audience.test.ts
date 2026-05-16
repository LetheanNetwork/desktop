// SPDX-Licence-Identifier: EUPL-1.2

import { describe, it, expect, vi, beforeEach } from "vitest";
import type { LitElement } from "lit";
import { mountWindow, expectChromeTitle, isEmbedded } from "../../../test/window-fixture";
import "./audience";

vi.mock("@desktop/marketing/audience/service", () => ({
  List: vi.fn(),
}));

import { List as MockedList } from "@desktop/marketing/audience/service";

describe("lthn-view-audience — smoke", () => {
  it("mounts with the Audience titlebar", async () => {
    const { host } = await mountWindow("lthn-view-audience");
    expectChromeTitle(host, "Audience");
  });

  it("renders the hero subscribers card with the locale-formatted total", async () => {
    const { host } = await mountWindow("lthn-view-audience");
    const hero = host.querySelector(".lthn-view-audience-hero");
    expect(hero).not.toBeNull();
    // 8214 → "8,214" via Number.toLocaleString in en-* locales
    expect(hero?.textContent).toContain("8,214");
  });

  it("renders the 12-week sparkline element", async () => {
    const { host } = await mountWindow("lthn-view-audience");
    expect(host.querySelector("lthn-sparkline")).not.toBeNull();
  });

  it("renders five segment rows in the table", async () => {
    const { host } = await mountWindow("lthn-view-audience");
    expect(host.querySelectorAll(".lthn-view-audience-row").length).toBe(5);
  });

  it("renders the table header columns", async () => {
    const { host } = await mountWindow("lthn-view-audience");
    const table = host.querySelector(".lthn-view-audience-table");
    expect(table?.textContent).toContain("Segment");
    expect(table?.textContent).toContain("Size");
    expect(table?.textContent).toContain("Weekly");
    expect(table?.textContent).toContain("Source");
  });

  it("each row carries a data-segment attribute", async () => {
    const { host } = await mountWindow("lthn-view-audience");
    const segs = Array.from(host.querySelectorAll(".lthn-view-audience-row"))
      .map(r => r.getAttribute("data-segment"));
    expect(segs).toContain("All subscribers");
    expect(segs).toContain("Local-AI developers");
    expect(segs).toContain("Active runtime users (30d)");
  });

  it("footer surfaces the no-third-party-tracking commitment", async () => {
    const { host } = await mountWindow("lthn-view-audience");
    const footer = host.querySelector("footer");
    expect(footer?.textContent).toContain("no third-party tracking");
  });
});

describe("lthn-view-audience — two-shell", () => {
  it("embedded mode collapses the chrome", async () => {
    const { host } = await mountWindow("lthn-view-audience", { attrs: { embedded: "" } });
    expect(isEmbedded(host)).toBe(true);
    expect(host.querySelector("header")).toBeNull();
  });
});

describe("lthn-view-audience — backend wire", () => {
  type AudienceEl = LitElement & {
    segments: { name: string; n: number; growth: string; src: string }[];
    _loadFromBackend: () => Promise<void>;
    updateComplete: Promise<boolean>;
  };

  beforeEach(() => {
    (MockedList as unknown as { mockReset: () => void }).mockReset();
  });

  it("backend segments replace fixture when present", async () => {
    (MockedList as unknown as { mockResolvedValue: (v: unknown) => void })
      .mockResolvedValue({ Value: { segments: [{ name: "All subscribers", n: 9000, growth: "+200 / w", src: "all" }] } });

    const { el, host } = await mountWindow<AudienceEl>("lthn-view-audience");
    await new Promise(r => setTimeout(r, 0));
    await el.updateComplete;

    expect(host.textContent).toContain("9,000");
  });

  it("backend rejection keeps fixture data visible", async () => {
    (MockedList as unknown as { mockRejectedValue: (v: unknown) => void })
      .mockRejectedValue(new Error("unavailable"));

    const { el, host } = await mountWindow<AudienceEl>("lthn-view-audience");
    await el._loadFromBackend();
    await el.updateComplete;

    expect(host.textContent).toContain("8,214");
    expect(host.textContent).toContain("Local-AI developers");
  });

  it("empty backend response keeps fixture data visible", async () => {
    (MockedList as unknown as { mockResolvedValue: (v: unknown) => void })
      .mockResolvedValue({ Value: { segments: [] } });

    const { el, host } = await mountWindow<AudienceEl>("lthn-view-audience");
    await el._loadFromBackend();
    await el.updateComplete;

    expect(host.textContent).toContain("8,214");
  });
});

describe("lthn-view-audience — conflict-reload listener (Cascade W2)", () => {
  type AudienceEl = LitElement & {
    segments: { name: string; n: number; growth: string; src: string }[];
    updateComplete: Promise<boolean>;
  };

  beforeEach(() => {
    (MockedList as unknown as { mockReset: () => void }).mockReset();
  });

  it("CONFLICT_RELOAD_EVENT with matching service triggers _loadFromBackend", async () => {
    const calls: number[] = [];
    (MockedList as unknown as { mockImplementation: (fn: () => Promise<unknown>) => void })
      .mockImplementation(() => {
        calls.push(1);
        return Promise.resolve({ Value: { segments: [
          { name: "Reloaded", n: 1, growth: "+1 / w", src: "manual" },
        ] } });
      });

    const { el } = await mountWindow<AudienceEl>("lthn-view-audience");
    await new Promise(r => setTimeout(r, 0));
    await el.updateComplete;
    const baseline = calls.length;

    window.dispatchEvent(new CustomEvent("lthn:conflict:reload-requested", {
      detail: { service: "audience.update" },
    }));
    await new Promise(r => setTimeout(r, 0));
    await el.updateComplete;

    expect(calls.length).toBeGreaterThan(baseline);
  });

  it("CONFLICT_RELOAD_EVENT with non-matching service is ignored", async () => {
    const calls: number[] = [];
    (MockedList as unknown as { mockImplementation: (fn: () => Promise<unknown>) => void })
      .mockImplementation(() => {
        calls.push(1);
        return Promise.resolve({ Value: { segments: [] } });
      });

    const { el } = await mountWindow<AudienceEl>("lthn-view-audience");
    await new Promise(r => setTimeout(r, 0));
    await el.updateComplete;
    const baseline = calls.length;

    window.dispatchEvent(new CustomEvent("lthn:conflict:reload-requested", {
      detail: { service: "campaigns.update" }, // not ours
    }));
    await new Promise(r => setTimeout(r, 0));
    await el.updateComplete;

    expect(calls.length).toBe(baseline);
  });
});
