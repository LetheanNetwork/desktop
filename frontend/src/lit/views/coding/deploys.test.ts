// SPDX-Licence-Identifier: EUPL-1.2

import { describe, it, expect } from "vitest";
import type { LitElement } from "lit";
import { mountWindow, expectChromeTitle, isEmbedded } from "../../../test/window-fixture";
import "./deploys";

interface DeploysTestElement extends LitElement {
  envs: Array<{
    name: string; url: string; version: string;
    commit: string; age: string; health: "ok" | "degraded" | "down";
  }>;
  history: Array<{
    ts: string; env: string; by: string;
    commit: string; outcome: "success" | "rolled-back" | "failed"; dur: string;
  }>;
  _healthSummary(): { ok: number; degraded: number };
}

describe("lthn-view-deploys — smoke", () => {
  it("mounts with Deploys titlebar", async () => {
    const { host } = await mountWindow("lthn-view-deploys");
    expectChromeTitle(host, "Deploys");
    expect(host.querySelector("header")).not.toBeNull();
  });

  it("embedded mode collapses the chrome", async () => {
    const { host } = await mountWindow("lthn-view-deploys", { attrs: { embedded: "" } });
    expect(isEmbedded(host)).toBe(true);
    expect(host.querySelector("header")).toBeNull();
  });

  it("renders one card per env", async () => {
    const { el, host } = await mountWindow<DeploysTestElement>("lthn-view-deploys");
    await el.updateComplete;
    const cards = host.querySelectorAll(".lthn-view-deploys-env");
    expect(cards.length).toBe(el.envs.length);
  });

  it("renders one row per history entry", async () => {
    const { el, host } = await mountWindow<DeploysTestElement>("lthn-view-deploys");
    await el.updateComplete;
    const rows = host.querySelectorAll(".lthn-view-deploys-row");
    expect(rows.length).toBe(el.history.length);
  });
});

describe("lthn-view-deploys — _healthSummary", () => {
  it("counts ok vs degraded envs", async () => {
    const { el } = await mountWindow<DeploysTestElement>("lthn-view-deploys");
    el.envs = [
      { name: "prod",    url: "lthn.ai",    version: "v1", commit: "a", age: "1d", health: "ok"       },
      { name: "staging", url: "s.lthn.ai",  version: "v2", commit: "b", age: "2h", health: "ok"       },
      { name: "preview", url: "p.lthn.ai",  version: "v3", commit: "c", age: "5m", health: "degraded" },
    ];
    await el.updateComplete;
    const s = el._healthSummary();
    expect(s.ok).toBe(2);
    expect(s.degraded).toBe(1);
  });

  it("all green → subtitle reads 'all green'", async () => {
    const { el, host } = await mountWindow<DeploysTestElement>("lthn-view-deploys");
    el.envs = [
      { name: "prod", url: "lthn.ai", version: "v1", commit: "a", age: "1d", health: "ok" },
    ];
    await el.updateComplete;
    const header = host.querySelector("header");
    expect(header?.textContent ?? "").toContain("all green");
  });
});

describe("lthn-view-deploys — data-attributes", () => {
  it("each env card has data-env set to the env name", async () => {
    const { el, host } = await mountWindow<DeploysTestElement>("lthn-view-deploys");
    el.envs = [
      { name: "production", url: "lthn.ai", version: "v1", commit: "a", age: "1d", health: "ok" },
    ];
    await el.updateComplete;
    const card = host.querySelector(".lthn-view-deploys-env");
    expect(card?.getAttribute("data-env")).toBe("production");
  });
});
