// SPDX-Licence-Identifier: EUPL-1.2

import { describe, it, expect } from "vitest";
import { mountWindow, expectChromeTitle, isEmbedded } from "../../../test/window-fixture";
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
