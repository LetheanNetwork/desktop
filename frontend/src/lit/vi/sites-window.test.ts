// SPDX-Licence-Identifier: EUPL-1.2

import { describe, it, expect } from "vitest";
import type { LitElement } from "lit";
import { mountWindow, expectChromeTitle, isEmbedded } from "../../test/window-fixture";
import "./sites-window";

interface SitesTestElement extends LitElement {
  sites: Array<{ domain: string; stack: string; status: "green" | "amber" | "red"; response: number; lastChecked: string; error?: string }>;
  scanned: number;
  filter: "all" | "ok" | "failing";
  loading: boolean;
  _counts(): { total: number; ok: number; failing: number };
  _visible(): Array<{ status: string }>;
  _formatChecked(iso: string): string;
}

describe("lthn-vi-sites-window — smoke", () => {
  it("mounts with the Sites titlebar", async () => {
    const { host } = await mountWindow("lthn-vi-sites-window");
    expectChromeTitle(host, "Sites");
    expect(host.querySelector("header")).not.toBeNull();
  });

  it("embedded mode collapses the chrome", async () => {
    const { host } = await mountWindow("lthn-vi-sites-window", { attrs: { embedded: "" } });
    expect(isEmbedded(host)).toBe(true);
    expect(host.querySelector("header")).toBeNull();
  });
});

describe("lthn-vi-sites-window — state derivations", () => {
  it("_counts splits green vs not-green", async () => {
    const { el } = await mountWindow<SitesTestElement>("lthn-vi-sites-window");
    el.sites = [
      { domain: "a.example", stack: "", status: "green",  response: 10, lastChecked: "" },
      { domain: "b.example", stack: "", status: "amber",  response: 99, lastChecked: "" },
      { domain: "c.example", stack: "", status: "red",    response: 0,  lastChecked: "" },
      { domain: "d.example", stack: "", status: "green",  response: 12, lastChecked: "" },
    ];
    await el.updateComplete;
    const c = el._counts();
    expect(c.total).toBe(4);
    expect(c.ok).toBe(2);
    expect(c.failing).toBe(2);
  });

  it("_visible filters by status — 'ok' shows only green; 'failing' shows amber+red", async () => {
    const { el } = await mountWindow<SitesTestElement>("lthn-vi-sites-window");
    el.sites = [
      { domain: "g.example", stack: "", status: "green", response: 1, lastChecked: "" },
      { domain: "a.example", stack: "", status: "amber", response: 1, lastChecked: "" },
      { domain: "r.example", stack: "", status: "red",   response: 0, lastChecked: "" },
    ];
    await el.updateComplete;

    el.filter = "ok";
    await el.updateComplete;
    const okOnly = el._visible();
    expect(okOnly).toHaveLength(1);
    expect(okOnly[0].status).toBe("green");

    el.filter = "failing";
    await el.updateComplete;
    const failOnly = el._visible();
    expect(failOnly).toHaveLength(2);
    expect(failOnly.every(s => s.status !== "green")).toBe(true);

    el.filter = "all";
    await el.updateComplete;
    expect(el._visible()).toHaveLength(3);
  });

  it("_formatChecked renders relative time + 'em-dash' for empty", async () => {
    const { el } = await mountWindow<SitesTestElement>("lthn-vi-sites-window");
    expect(el._formatChecked("")).toBe("—");
    expect(el._formatChecked("not a date")).toBe("not a date");

    const recent = new Date(Date.now() - 5000).toISOString();
    expect(el._formatChecked(recent)).toMatch(/^\d+s ago$/);

    const minutesAgo = new Date(Date.now() - 5 * 60_000).toISOString();
    expect(el._formatChecked(minutesAgo)).toMatch(/^\d+m ago$/);
  });
});

describe("lthn-vi-sites-window — render copy", () => {
  it("empty-state copy says 'set vi.sites in config' when scanned=0", async () => {
    const { el, host } = await mountWindow<SitesTestElement>("lthn-vi-sites-window");
    // connectedCallback fires an async _refresh() that leaves
    // loading=true while the (mocked-failing) Wails binding settles.
    // Force loading=false so the empty-state copy renders deterministically.
    el.sites = [];
    el.scanned = 0;
    el.loading = false;
    await el.updateComplete;
    expect(host.textContent ?? "").toContain("set vi.sites in config");
  });

  it("empty-state copy nudges 'give it a moment' when scanned>0 but no reports yet", async () => {
    const { el, host } = await mountWindow<SitesTestElement>("lthn-vi-sites-window");
    el.sites = [];
    el.scanned = 4;
    el.loading = false;
    await el.updateComplete;
    expect(host.textContent ?? "").toContain("give it a moment");
  });
});
