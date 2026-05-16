// SPDX-Licence-Identifier: EUPL-1.2

import { describe, it, expect } from "vitest";
import { mountWindow, expectChromeTitle, isEmbedded } from "../../../test/window-fixture";
import "./audience";

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
