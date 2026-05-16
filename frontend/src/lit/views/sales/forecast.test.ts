// SPDX-Licence-Identifier: EUPL-1.2

import { describe, it, expect } from "vitest";
import type { LitElement } from "lit";
import { mountWindow, expectChromeTitle, isEmbedded } from "../../../test/window-fixture";
import "./forecast";

interface ForecastRow {
  q: string; target: number; committed: number; best: number; low: number;
  status: "open" | "plan";
}
interface ForecastKpi { l: string; v: string; }
interface ForecastEl extends LitElement {
  rows: ForecastRow[];
  kpis: ForecastKpi[];
}

describe("lthn-view-forecast — smoke", () => {
  it("mounts with the Forecast titlebar", async () => {
    const { host } = await mountWindow("lthn-view-forecast");
    expectChromeTitle(host, "Forecast");
    expect(host.querySelector("header")).not.toBeNull();
  });

  it("renders four KPI cards", async () => {
    const { host } = await mountWindow("lthn-view-forecast");
    const kpis = host.querySelectorAll(".lthn-view-forecast-kpi");
    expect(kpis.length).toBe(4);
  });

  it("renders the canonical KPI labels", async () => {
    const { host } = await mountWindow("lthn-view-forecast");
    expect(host.textContent).toContain("Y1 target ARR");
    expect(host.textContent).toContain("Committed");
    expect(host.textContent).toContain("Best case");
    expect(host.textContent).toContain("Probability-weighted");
  });

  it("renders one bar row per fixture quarter", async () => {
    const { el, host } = await mountWindow<ForecastEl>("lthn-view-forecast");
    const rows = host.querySelectorAll(".lthn-view-forecast-row");
    expect(rows.length).toBe(el.rows.length);
  });

  it("each bar row carries data-quarter + data-status", async () => {
    const { host } = await mountWindow("lthn-view-forecast");
    const rows = Array.from(host.querySelectorAll(".lthn-view-forecast-row"));
    expect(rows.every(r => !!r.getAttribute("data-quarter"))).toBe(true);
    expect(rows.every(r => !!r.getAttribute("data-status"))).toBe(true);
  });

  it("renders the legend with all four glyphs", async () => {
    const { host } = await mountWindow("lthn-view-forecast");
    const legend = host.querySelector(".lthn-view-forecast-legend");
    expect(legend?.textContent ?? "").toContain("committed");
    expect(legend?.textContent ?? "").toContain("low case");
    expect(legend?.textContent ?? "").toContain("best case");
    expect(legend?.textContent ?? "").toContain("target");
  });

  it("subtitle reflects quarter count", async () => {
    const { host } = await mountWindow("lthn-view-forecast");
    const header = host.querySelector("header");
    expect(header?.textContent ?? "").toContain("4 quarters");
  });
});

describe("lthn-view-forecast — fixture content", () => {
  it("renders the Q2 2026 row", async () => {
    const { host } = await mountWindow("lthn-view-forecast");
    expect(host.textContent).toContain("Q2 · 2026");
  });

  it("renders the plan-status Q1 2027 row", async () => {
    const { host } = await mountWindow("lthn-view-forecast");
    const planRows = Array.from(host.querySelectorAll(".lthn-view-forecast-row"))
      .filter(r => r.getAttribute("data-status") === "plan");
    expect(planRows.length).toBe(1);
    expect(planRows[0].getAttribute("data-quarter")).toBe("Q1 · 2027");
  });

  it("each row paints committed + low + best + target glyphs", async () => {
    const { host } = await mountWindow("lthn-view-forecast");
    const row = host.querySelector(".lthn-view-forecast-row");
    expect(row?.querySelector(".lthn-view-forecast-bar-low")).not.toBeNull();
    expect(row?.querySelector(".lthn-view-forecast-bar-committed")).not.toBeNull();
    expect(row?.querySelector(".lthn-view-forecast-bar-best")).not.toBeNull();
    expect(row?.querySelector(".lthn-view-forecast-bar-target")).not.toBeNull();
  });
});

describe("lthn-view-forecast — two-shell", () => {
  it("embedded mode collapses the chrome", async () => {
    const { host } = await mountWindow("lthn-view-forecast", { attrs: { embedded: "" } });
    expect(isEmbedded(host)).toBe(true);
    expect(host.querySelector("header")).toBeNull();
  });

  it("embedded mode still renders all four quarter rows", async () => {
    const { host } = await mountWindow("lthn-view-forecast", { attrs: { embedded: "" } });
    expect(host.querySelectorAll(".lthn-view-forecast-row").length).toBe(4);
  });
});
