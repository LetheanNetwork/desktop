// SPDX-Licence-Identifier: EUPL-1.2

import { describe, it, expect, vi } from "vitest";
import { mountWindow, expectChromeTitle, isEmbedded } from "../../../test/window-fixture";

vi.mock("@desktop/tasks/service", () => ({
  List: vi.fn(),
}));

import { List as TasksList } from "@desktop/tasks/service";

import "./today";

describe("lthn-view-today — smoke", () => {
  it("mounts with the Today titlebar", async () => {
    const { host } = await mountWindow("lthn-view-today");
    expectChromeTitle(host, "Today");
    expect(host.querySelector("header")).not.toBeNull();
  });

  it("renders the three focus cards from the fixture", async () => {
    const { host } = await mountWindow("lthn-view-today");
    const cards = host.querySelectorAll(".lthn-today-focus-card");
    expect(cards.length).toBe(3);
    expect(host.textContent).toContain("Lethean v0.2 release prep");
    expect(host.textContent).toContain("Investor call · Calliope VC");
  });

  it("renders the agenda timeline rows", async () => {
    const { host } = await mountWindow("lthn-view-today");
    const rows = host.querySelectorAll(".lthn-today-agenda-row");
    expect(rows.length).toBeGreaterThanOrEqual(5);
    expect(host.textContent).toContain("Standup · core team");
  });

  it("renders the Vi daily-brief card", async () => {
    const { host } = await mountWindow("lthn-view-today");
    expect(host.querySelector(".lthn-today-vi-brief")).not.toBeNull();
    expect(host.textContent).toContain("Vi · daily brief");
  });

  it("renders the shipped-today list", async () => {
    const { host } = await mountWindow("lthn-view-today");
    const rows = host.querySelectorAll(".lthn-today-shipped-row");
    expect(rows.length).toBe(3);
    expect(host.textContent).toContain("MCP tools window");
  });

  it("renders the velocity bar at the right percentage", async () => {
    const { host } = await mountWindow("lthn-view-today");
    const bar = host.querySelector(".lthn-today-velocity-bar") as HTMLElement | null;
    expect(bar).not.toBeNull();
    // 14/18 = 77.8%, computed in render()
    expect(bar?.style.width).toMatch(/^7[78](\.\d)?%$/);
  });

  it("flags high-urgency focus cards via data attribute", async () => {
    const { host } = await mountWindow("lthn-view-today");
    const high = host.querySelectorAll('.lthn-today-focus-card[data-urgency="high"]');
    expect(high.length).toBe(2);
  });
});

describe("lthn-view-today — two-shell", () => {
  it("embedded mode collapses the chrome", async () => {
    const { host } = await mountWindow("lthn-view-today", { attrs: { embedded: "" } });
    expect(isEmbedded(host)).toBe(true);
    expect(host.querySelector("header")).toBeNull();
  });

  it("standalone mode renders the chrome titlebar", async () => {
    const { host } = await mountWindow("lthn-view-today");
    expect(isEmbedded(host)).toBe(false);
    expect(host.querySelector("header")).not.toBeNull();
  });
});

describe("lthn-view-today — backend wire", () => {
  type TodayEl = HTMLElement & {
    data: {
      focus: Array<{ t: string; urgency: string; due: string }>;
      agenda: unknown[];
      shipped: unknown[];
      velocity: unknown;
    };
    updateComplete: Promise<boolean>;
  };

  it("replaces fixture focus cards with high-priority Issues from tasks.List", async () => {
    (TasksList as unknown as { mockReset: () => void }).mockReset();
    (TasksList as unknown as { mockResolvedValue: (v: unknown) => void }).mockResolvedValue({
      Value: { issues: [
        { ID: "i1", Summary: "Ship Stage B serverkey package", Priority: "urgent" },
        { ID: "i2", Summary: "Review Cerberus DREAD findings",   Priority: "immediate" },
        { ID: "i3", Summary: "Wire Sales pipeline view",         Priority: "high" },
        { ID: "i4", Summary: "Update telemetry chart",           Priority: "normal" },
      ] },
    });
    const { el } = await mountWindow<TodayEl>("lthn-view-today");
    await new Promise((r) => setTimeout(r, 0));
    await el.updateComplete;
    expect(el.data.focus.length).toBe(3); // capped at 3
    expect(el.data.focus[0].t).toContain("Ship Stage B");
    expect(el.data.focus[0].urgency).toBe("high"); // urgent → high
    expect(el.data.focus[1].urgency).toBe("high"); // immediate → high
    expect(el.data.focus[2].urgency).toBe("med");  // high → med
    expect(el.data.focus.map(f => f.t)).not.toContain("Update telemetry chart"); // normal filtered out
  });

  it("keeps fixture when no high-priority Issues are returned", async () => {
    (TasksList as unknown as { mockReset: () => void }).mockReset();
    (TasksList as unknown as { mockResolvedValue: (v: unknown) => void }).mockResolvedValue({
      Value: { issues: [
        { ID: "i5", Summary: "Polish dropdown animation", Priority: "low" },
      ] },
    });
    const { el } = await mountWindow<TodayEl>("lthn-view-today");
    await new Promise((r) => setTimeout(r, 0));
    await el.updateComplete;
    expect(el.data.focus[0].t).toBe("Lethean v0.2 release prep"); // fixture preserved
  });

  it("keeps fixture when backend rejects", async () => {
    (TasksList as unknown as { mockReset: () => void }).mockReset();
    (TasksList as unknown as { mockRejectedValue: (v: unknown) => void }).mockRejectedValue(new Error("no binding"));
    const { el } = await mountWindow<TodayEl>("lthn-view-today");
    await new Promise((r) => setTimeout(r, 0));
    await el.updateComplete;
    expect(el.data.focus[0].t).toBe("Lethean v0.2 release prep"); // fixture preserved
  });
});
