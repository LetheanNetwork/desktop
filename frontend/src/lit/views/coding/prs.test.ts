// SPDX-Licence-Identifier: EUPL-1.2

import { describe, it, expect } from "vitest";
import type { LitElement } from "lit";
import { mountWindow, expectChromeTitle, isEmbedded } from "../../../test/window-fixture";
import "./prs";

interface PRsTestElement extends LitElement {
  items: Array<{
    kind: string; provider: string; repo: string; number: number;
    title: string; author: string; state: string; url: string;
    openedAt: string; updatedAt: string; checkedAt: string;
  }>;
  scanned: number;
  filter: "all" | "forge" | "github";
  loading: boolean;
  err: string;
  _counts(): { total: number; forge: number; github: number };
  _visible(): Array<{ provider: string }>;
}

const makeEntry = (provider: "forge" | "github", n: number) => ({
  kind: "pr", provider, repo: `org/repo-${n}`, number: n,
  title: `PR ${n}`, author: "me", state: "open", url: "",
  openedAt: "", updatedAt: "", checkedAt: "",
});

describe("lthn-view-prs — smoke", () => {
  it("mounts with Pull requests titlebar", async () => {
    const { host } = await mountWindow("lthn-view-prs");
    expectChromeTitle(host, "Pull requests");
    expect(host.querySelector("header")).not.toBeNull();
  });

  it("embedded mode collapses the chrome", async () => {
    const { host } = await mountWindow("lthn-view-prs", { attrs: { embedded: "" } });
    expect(isEmbedded(host)).toBe(true);
    expect(host.querySelector("header")).toBeNull();
  });
});

describe("lthn-view-prs — provider filter", () => {
  it("_counts splits by provider", async () => {
    const { el } = await mountWindow<PRsTestElement>("lthn-view-prs");
    el.items = [
      makeEntry("forge",  1),
      makeEntry("forge",  2),
      makeEntry("github", 3),
    ];
    await el.updateComplete;
    const c = el._counts();
    expect(c.total).toBe(3);
    expect(c.forge).toBe(2);
    expect(c.github).toBe(1);
  });

  it("_visible returns all when filter=all", async () => {
    const { el } = await mountWindow<PRsTestElement>("lthn-view-prs");
    el.items = [makeEntry("forge", 1), makeEntry("github", 2)];
    el.filter = "all";
    await el.updateComplete;
    expect(el._visible()).toHaveLength(2);
  });

  it("_visible filters to forge only", async () => {
    const { el } = await mountWindow<PRsTestElement>("lthn-view-prs");
    el.items = [makeEntry("forge", 1), makeEntry("github", 2), makeEntry("forge", 3)];
    el.filter = "forge";
    await el.updateComplete;
    const v = el._visible();
    expect(v).toHaveLength(2);
    expect(v.every(i => i.provider === "forge")).toBe(true);
  });

  it("_visible filters to github only", async () => {
    const { el } = await mountWindow<PRsTestElement>("lthn-view-prs");
    el.items = [makeEntry("forge", 1), makeEntry("github", 2)];
    el.filter = "github";
    await el.updateComplete;
    const v = el._visible();
    expect(v).toHaveLength(1);
    expect(v[0].provider).toBe("github");
  });
});

describe("lthn-view-prs — empty state copy", () => {
  it("shows 'set vi.repos in config' when scanned=0 and no items", async () => {
    const { el, host } = await mountWindow<PRsTestElement>("lthn-view-prs");
    el.items = [];
    el.scanned = 0;
    el.loading = false;
    await el.updateComplete;
    expect(host.textContent ?? "").toContain("set vi.repos in config");
  });

  it("shows 'no open PRs' when scanned>0 but no items", async () => {
    const { el, host } = await mountWindow<PRsTestElement>("lthn-view-prs");
    el.items = [];
    el.scanned = 3;
    el.loading = false;
    await el.updateComplete;
    expect(host.textContent ?? "").toContain("no open PRs");
  });
});

describe("lthn-view-prs — error state", () => {
  it("renders error message when err is set", async () => {
    const { el, host } = await mountWindow<PRsTestElement>("lthn-view-prs");
    el.err = "failed to reach vi service";
    await el.updateComplete;
    expect(host.textContent ?? "").toContain("failed to reach vi service");
  });
});

describe("lthn-view-prs — row data-attributes", () => {
  it("each row has data-pr set to the PR number", async () => {
    const { el, host } = await mountWindow<PRsTestElement>("lthn-view-prs");
    el.items = [makeEntry("forge", 42)];
    el.filter = "all";
    await el.updateComplete;
    const row = host.querySelector(".lthn-view-prs-row");
    expect(row?.getAttribute("data-pr")).toBe("42");
  });
});
