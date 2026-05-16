// SPDX-Licence-Identifier: EUPL-1.2

import { describe, it, expect } from "vitest";
import type { LitElement } from "lit";
import { mountWindow, expectChromeTitle, isEmbedded } from "../../../test/window-fixture";
import "./analytics";

type AnalyticsEl = LitElement & {
  window: "7d" | "30d" | "90d";
  updateComplete: Promise<boolean>;
};

describe("lthn-view-analytics — smoke", () => {
  it("mounts with the Analytics titlebar", async () => {
    const { host } = await mountWindow("lthn-view-analytics");
    expectChromeTitle(host, "Analytics");
  });

  it("subtitle defaults to web · 30d", async () => {
    const { host } = await mountWindow("lthn-view-analytics");
    const header = host.querySelector("header");
    expect(header?.textContent).toContain("web · 30d");
  });

  it("renders the sessions sparkline + top sources panel", async () => {
    const { host } = await mountWindow("lthn-view-analytics");
    expect(host.querySelector(".lthn-view-analytics-sources")).not.toBeNull();
    expect(host.querySelector("lthn-sparkline")).not.toBeNull();
    expect(host.textContent).toContain("Top sources");
  });

  it("renders six fixture source rows on default 30d window", async () => {
    const { host } = await mountWindow("lthn-view-analytics");
    expect(host.querySelectorAll(".lthn-view-analytics-source-row").length).toBe(6);
  });

  it("renders five fixture page rows on default 30d window", async () => {
    const { host } = await mountWindow("lthn-view-analytics");
    expect(host.querySelectorAll(".lthn-view-analytics-page-row").length).toBe(5);
  });

  it("window toolbar exposes 7d / 30d / 90d buttons", async () => {
    const { host } = await mountWindow("lthn-view-analytics");
    const btns = host.querySelectorAll(".lthn-view-analytics-win-btn");
    expect(btns.length).toBe(3);
    const labels = Array.from(btns).map(b => b.getAttribute("data-window"));
    expect(labels).toEqual(["7d", "30d", "90d"]);
  });

  it("switching to 7d shows the empty-state row and no source rows", async () => {
    const { el, host } = await mountWindow<AnalyticsEl>("lthn-view-analytics");
    el.window = "7d";
    await el.updateComplete;
    expect(host.querySelectorAll(".lthn-view-analytics-source-row").length).toBe(0);
    expect(host.querySelectorAll(".lthn-view-analytics-empty").length).toBeGreaterThan(0);
  });

  it("page table header lists Views, Bounce, Time", async () => {
    const { host } = await mountWindow("lthn-view-analytics");
    const pages = host.querySelector(".lthn-view-analytics-pages");
    expect(pages?.textContent).toContain("Views");
    expect(pages?.textContent).toContain("Bounce");
    expect(pages?.textContent).toContain("Time");
  });

  it("footer surfaces the self-hosted Plausible commitment", async () => {
    const { host } = await mountWindow("lthn-view-analytics");
    const footer = host.querySelector("footer");
    expect(footer?.textContent).toContain("self-hosted Plausible");
  });
});

describe("lthn-view-analytics — two-shell", () => {
  it("embedded mode collapses the chrome", async () => {
    const { host } = await mountWindow("lthn-view-analytics", { attrs: { embedded: "" } });
    expect(isEmbedded(host)).toBe(true);
    expect(host.querySelector("header")).toBeNull();
  });
});
