// SPDX-Licence-Identifier: EUPL-1.2

import { describe, it, expect } from "vitest";
import type { LitElement } from "lit";
import { mountWindow, expectChromeTitle, isEmbedded } from "../../../test/window-fixture";
import "./content";

type ContentEl = LitElement & {
  filter: "" | "idea" | "draft" | "review" | "ready" | "live";
  updateComplete: Promise<boolean>;
};

describe("lthn-view-content — smoke", () => {
  it("mounts with the Content calendar titlebar", async () => {
    const { host } = await mountWindow("lthn-view-content");
    expectChromeTitle(host, "Content calendar");
  });

  it("renders all five pipeline columns", async () => {
    const { host } = await mountWindow("lthn-view-content");
    expect(host.textContent).toContain("Ideas");
    expect(host.textContent).toContain("Drafting");
    expect(host.textContent).toContain("Review");
    expect(host.textContent).toContain("Ready");
    expect(host.textContent).toContain("Live");
  });

  it("renders fixture cards", async () => {
    const { host } = await mountWindow("lthn-view-content");
    expect(host.textContent).toContain("v0.2 release notes");
    expect(host.textContent).toContain("Sovereign-compute manifesto");
    expect(host.textContent).toContain("Hiring · senior Go engineer");
  });

  it("shows due-today on the v0.2 release notes card", async () => {
    const { host } = await mountWindow("lthn-view-content");
    expect(host.textContent).toContain("due today");
  });

  it("subtitle counts in-flight posts excluding live", async () => {
    const { host } = await mountWindow("lthn-view-content");
    // 2 ideas + 2 drafting + 1 review + 1 ready = 6 in flight
    const header = host.querySelector("header");
    expect(header?.textContent).toContain("6 posts in flight");
  });

  it("clicking a column header sets the filter and narrows visible columns", async () => {
    const { el, host } = await mountWindow<ContentEl>("lthn-view-content");
    expect(host.querySelectorAll(".lthn-view-content-col-header").length).toBe(5);
    el.filter = "draft";
    await el.updateComplete;
    expect(host.querySelectorAll(".lthn-view-content-col-header").length).toBe(1);
    expect(host.textContent).toContain("Drafting");
    expect(host.textContent).not.toContain("Ideas");
  });
});

describe("lthn-view-content — two-shell", () => {
  it("embedded mode collapses the chrome", async () => {
    const { host } = await mountWindow("lthn-view-content", { attrs: { embedded: "" } });
    expect(isEmbedded(host)).toBe(true);
    expect(host.querySelector("header")).toBeNull();
  });
});
