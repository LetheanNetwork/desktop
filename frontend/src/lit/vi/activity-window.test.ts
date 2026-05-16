// SPDX-Licence-Identifier: EUPL-1.2

import { describe, it, expect } from "vitest";
import type { LitElement } from "lit";
import { mountWindow, expectChromeTitle, isEmbedded } from "../../test/window-fixture";
import "./activity-window";

interface ActivityTestElement extends LitElement {
  items: Array<{
    kind: string;
    provider: string;
    repo: string;
    number: number;
    title: string;
    author: string;
    state: string;
    url: string;
    openedAt: string;
    updatedAt: string;
    checkedAt: string;
  }>;
  scanned: number;
  filter: "all" | "forge" | "github";
  loading: boolean;
  _counts(): { total: number; forge: number; github: number };
  _visible(): Array<{ provider: string }>;
  _formatRelative(iso: string): string;
  _providerLabel(p: string): string;
}

describe("lthn-vi-activity-window — smoke", () => {
  it("mounts with the Activity titlebar", async () => {
    const { host } = await mountWindow("lthn-vi-activity-window");
    expectChromeTitle(host, "Activity");
    expect(host.querySelector("header")).not.toBeNull();
  });

  it("embedded mode collapses the chrome", async () => {
    const { host } = await mountWindow("lthn-vi-activity-window", { attrs: { embedded: "" } });
    expect(isEmbedded(host)).toBe(true);
    expect(host.querySelector("header")).toBeNull();
  });
});

describe("lthn-vi-activity-window — state derivations", () => {
  it("_counts splits by provider", async () => {
    const { el } = await mountWindow<ActivityTestElement>("lthn-vi-activity-window");
    el.items = [
      { kind: "pr", provider: "forge",  repo: "lthn/desktop", number: 1, title: "a", author: "x", state: "open", url: "", openedAt: "", updatedAt: "", checkedAt: "" },
      { kind: "pr", provider: "github", repo: "z/q",          number: 2, title: "b", author: "x", state: "open", url: "", openedAt: "", updatedAt: "", checkedAt: "" },
      { kind: "pr", provider: "forge",  repo: "lthn/ai",      number: 3, title: "c", author: "x", state: "open", url: "", openedAt: "", updatedAt: "", checkedAt: "" },
      { kind: "pr", provider: "github", repo: "z/q",          number: 4, title: "d", author: "x", state: "open", url: "", openedAt: "", updatedAt: "", checkedAt: "" },
    ];
    await el.updateComplete;
    const c = el._counts();
    expect(c.total).toBe(4);
    expect(c.forge).toBe(2);
    expect(c.github).toBe(2);
  });

  it("_visible filters by provider", async () => {
    const { el } = await mountWindow<ActivityTestElement>("lthn-vi-activity-window");
    el.items = [
      { kind: "pr", provider: "forge",  repo: "r1", number: 1, title: "a", author: "x", state: "open", url: "", openedAt: "", updatedAt: "", checkedAt: "" },
      { kind: "pr", provider: "github", repo: "r2", number: 2, title: "b", author: "x", state: "open", url: "", openedAt: "", updatedAt: "", checkedAt: "" },
      { kind: "pr", provider: "forge",  repo: "r3", number: 3, title: "c", author: "x", state: "open", url: "", openedAt: "", updatedAt: "", checkedAt: "" },
    ];
    await el.updateComplete;

    el.filter = "forge";
    await el.updateComplete;
    const forgeOnly = el._visible();
    expect(forgeOnly).toHaveLength(2);
    expect(forgeOnly.every(i => i.provider === "forge")).toBe(true);

    el.filter = "github";
    await el.updateComplete;
    const ghOnly = el._visible();
    expect(ghOnly).toHaveLength(1);
    expect(ghOnly[0].provider).toBe("github");

    el.filter = "all";
    await el.updateComplete;
    expect(el._visible()).toHaveLength(3);
  });

  it("_formatRelative renders relative time + 'em-dash' for empty", async () => {
    const { el } = await mountWindow<ActivityTestElement>("lthn-vi-activity-window");
    expect(el._formatRelative("")).toBe("—");
    expect(el._formatRelative("not a date")).toBe("not a date");

    const recent = new Date(Date.now() - 5000).toISOString();
    expect(el._formatRelative(recent)).toMatch(/^\d+s ago$/);

    const minutesAgo = new Date(Date.now() - 5 * 60_000).toISOString();
    expect(el._formatRelative(minutesAgo)).toMatch(/^\d+m ago$/);

    const hoursAgo = new Date(Date.now() - 5 * 3600_000).toISOString();
    expect(el._formatRelative(hoursAgo)).toMatch(/^\d+h ago$/);

    const daysAgo = new Date(Date.now() - 5 * 86400_000).toISOString();
    expect(el._formatRelative(daysAgo)).toMatch(/^\d+d ago$/);
  });

  it("_providerLabel passes known providers through", async () => {
    const { el } = await mountWindow<ActivityTestElement>("lthn-vi-activity-window");
    expect(el._providerLabel("forge")).toBe("forge");
    expect(el._providerLabel("github")).toBe("github");
    expect(el._providerLabel("gitlab")).toBe("gitlab");
  });
});

describe("lthn-vi-activity-window — render copy", () => {
  it("empty-state copy says 'set vi.repos in config' when scanned=0", async () => {
    const { el, host } = await mountWindow<ActivityTestElement>("lthn-vi-activity-window");
    el.items = [];
    el.scanned = 0;
    el.loading = false;
    await el.updateComplete;
    expect(host.textContent ?? "").toContain("set vi.repos in config");
  });

  it("empty-state copy nudges 'or the fetcher hasn't run yet' when scanned>0 but no PRs yet", async () => {
    const { el, host } = await mountWindow<ActivityTestElement>("lthn-vi-activity-window");
    el.items = [];
    el.scanned = 2;
    el.loading = false;
    await el.updateComplete;
    expect(host.textContent ?? "").toContain("fetcher hasn't run yet");
  });
});
