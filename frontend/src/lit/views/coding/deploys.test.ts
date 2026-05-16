// SPDX-Licence-Identifier: EUPL-1.2

import { describe, it, expect, vi, beforeEach } from "vitest";
import type { LitElement } from "lit";
import { mountWindow, expectChromeTitle, isEmbedded } from "../../../test/window-fixture";
import "./deploys";

vi.mock("@desktop/deploys/service", () => ({
  List: vi.fn(),
}));

import { List as MockedList } from "@desktop/deploys/service";

interface DeploysTestElement extends LitElement {
  envs: Array<{
    name: string; url: string; version: string;
    commit: string; age: string; health: "ok" | "degraded" | "down";
  }>;
  history: Array<{
    ts: string; env: string; by: string;
    commit: string; outcome: "success" | "rolled-back" | "failed"; dur: string;
  }>;
  loading: boolean;
  _loadFromBackend(): Promise<void>;
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

describe("lthn-view-deploys — backend wire", () => {
  beforeEach(() => {
    (MockedList as unknown as { mockReset: () => void }).mockReset();
  });

  it("backend rows replace fixture when envs and history are returned", async () => {
    (MockedList as unknown as { mockResolvedValue: (v: unknown) => void })
      .mockResolvedValue({
        Value: {
          envs: [{ name: "live-env", url: "live.lthn.ai", version: "v9", commit: "ffffff", age: "1m", health: "ok" }],
          history: [{ ts: "1m", env: "live-env", by: "backend-user", commit: "ffffff", outcome: "success", dur: "10s" }],
        },
      });

    const { el, host } = await mountWindow<DeploysTestElement>("lthn-view-deploys");
    await new Promise(r => setTimeout(r, 0));
    await el.updateComplete;

    expect(host.textContent).toContain("live-env");
    expect(host.textContent).toContain("backend-user");
    expect(host.textContent).toContain("live.lthn.ai");
  });

  it("backend rejection keeps fixture data visible", async () => {
    (MockedList as unknown as { mockRejectedValue: (v: unknown) => void })
      .mockRejectedValue(new Error("unavailable"));

    const { el, host } = await mountWindow<DeploysTestElement>("lthn-view-deploys");
    await el._loadFromBackend();
    await el.updateComplete;

    expect(host.textContent).toContain("production");
    expect(host.textContent).toContain("lthn.ai");
  });

  it("empty backend response keeps fixture data visible", async () => {
    (MockedList as unknown as { mockResolvedValue: (v: unknown) => void })
      .mockResolvedValue({ Value: { envs: [], history: [] } });

    const { el, host } = await mountWindow<DeploysTestElement>("lthn-view-deploys");
    await el._loadFromBackend();
    await el.updateComplete;

    expect(host.textContent).toContain("production");
    expect(host.textContent).toContain("4a82c1");
  });

  it("fixture-only tests remain green — smoke renders three env cards", async () => {
    (MockedList as unknown as { mockResolvedValue: (v: unknown) => void })
      .mockResolvedValue({ Value: { envs: [], history: [] } });

    const { el, host } = await mountWindow<DeploysTestElement>("lthn-view-deploys");
    await el.updateComplete;
    const cards = host.querySelectorAll(".lthn-view-deploys-env");
    expect(cards.length).toBe(3);
  });
});
