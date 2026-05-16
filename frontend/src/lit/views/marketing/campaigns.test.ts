// SPDX-Licence-Identifier: EUPL-1.2

import { describe, it, expect } from "vitest";
import { mountWindow, expectChromeTitle, isEmbedded } from "../../../test/window-fixture";
import "./campaigns";

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
