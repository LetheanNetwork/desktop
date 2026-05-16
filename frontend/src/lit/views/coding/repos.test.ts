// SPDX-Licence-Identifier: EUPL-1.2

import { describe, it, expect } from "vitest";
import type { LitElement } from "lit";
import { mountWindow, expectChromeTitle, isEmbedded } from "../../../test/window-fixture";
import "./repos";

interface ReposTestElement extends LitElement {
  repos: Array<{
    name: string; lang: string; branch: string;
    commit: string; build: "passing" | "running" | "failing"; prs: number;
  }>;
  _summary(): { passing: number; failing: number };
}

describe("lthn-view-repos — smoke", () => {
  it("mounts with Repos titlebar", async () => {
    const { host } = await mountWindow("lthn-view-repos");
    expectChromeTitle(host, "Repos");
    expect(host.querySelector("header")).not.toBeNull();
  });

  it("embedded mode collapses the chrome", async () => {
    const { host } = await mountWindow("lthn-view-repos", { attrs: { embedded: "" } });
    expect(isEmbedded(host)).toBe(true);
    expect(host.querySelector("header")).toBeNull();
  });

  it("renders a row for each fixture repo", async () => {
    const { el, host } = await mountWindow<ReposTestElement>("lthn-view-repos");
    await el.updateComplete;
    const rows = host.querySelectorAll(".lthn-view-repos-row");
    expect(rows.length).toBe(el.repos.length);
  });
});

describe("lthn-view-repos — state derivations", () => {
  it("_summary counts by build state", async () => {
    const { el } = await mountWindow<ReposTestElement>("lthn-view-repos");
    el.repos = [
      { name: "a", lang: "Go", branch: "main", commit: "a1", build: "passing", prs: 0 },
      { name: "b", lang: "Go", branch: "main", commit: "b1", build: "passing", prs: 1 },
      { name: "c", lang: "TS", branch: "dev",  commit: "c1", build: "failing", prs: 2 },
    ];
    await el.updateComplete;
    const s = el._summary();
    expect(s.passing).toBe(2);
    expect(s.failing).toBe(1);
  });

  it("_summary counts running separately from passing/failing", async () => {
    const { el } = await mountWindow<ReposTestElement>("lthn-view-repos");
    el.repos = [
      { name: "a", lang: "Go", branch: "main", commit: "a1", build: "passing", prs: 0 },
      { name: "b", lang: "Go", branch: "main", commit: "b1", build: "running", prs: 0 },
      { name: "c", lang: "Go", branch: "main", commit: "c1", build: "failing", prs: 0 },
    ];
    await el.updateComplete;
    const s = el._summary();
    expect(s.passing).toBe(1);
    expect(s.failing).toBe(1);
  });

  it("subtitle reflects repo count", async () => {
    const { el, host } = await mountWindow<ReposTestElement>("lthn-view-repos");
    el.repos = [
      { name: "r1", lang: "Go", branch: "main", commit: "a", build: "passing", prs: 0 },
      { name: "r2", lang: "Go", branch: "main", commit: "b", build: "passing", prs: 0 },
    ];
    await el.updateComplete;
    const header = host.querySelector("header");
    expect(header?.textContent ?? "").toContain("2 watched");
  });
});

describe("lthn-view-repos — data-attributes", () => {
  it("each row has data-repo set to the repo name", async () => {
    const { el, host } = await mountWindow<ReposTestElement>("lthn-view-repos");
    el.repos = [
      { name: "lthn/desktop", lang: "Go", branch: "main", commit: "abc", build: "passing", prs: 0 },
    ];
    await el.updateComplete;
    const row = host.querySelector(".lthn-view-repos-row");
    expect(row?.getAttribute("data-repo")).toBe("lthn/desktop");
  });
});
