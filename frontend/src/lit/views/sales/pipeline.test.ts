// SPDX-Licence-Identifier: EUPL-1.2

import { describe, it, expect } from "vitest";
import type { LitElement } from "lit";
import { mountWindow, expectChromeTitle, isEmbedded } from "../../../test/window-fixture";
import "./pipeline";

interface PipelineRow {
  c: string; v: string; t: string;
}
interface PipelineCol {
  id: "qual" | "engage" | "propose" | "close";
  label: string;
  value: string;
  deals: PipelineRow[];
}
interface PipelineEl extends LitElement {
  columns: PipelineCol[];
}

describe("lthn-view-pipeline — smoke", () => {
  it("mounts with the Pipeline titlebar", async () => {
    const { host } = await mountWindow("lthn-view-pipeline");
    expectChromeTitle(host, "Pipeline");
    expect(host.querySelector("header")).not.toBeNull();
  });

  it("renders four summary cards — one per stage", async () => {
    const { host } = await mountWindow("lthn-view-pipeline");
    const summary = host.querySelectorAll(".lthn-view-pipeline-summary-card");
    expect(summary.length).toBe(4);
  });

  it("renders four pipeline columns — one per stage", async () => {
    const { host } = await mountWindow("lthn-view-pipeline");
    const cols = host.querySelectorAll(".lthn-view-pipeline-column");
    expect(cols.length).toBe(4);
  });

  it("renders one deal card per fixture deal", async () => {
    const { el, host } = await mountWindow<PipelineEl>("lthn-view-pipeline");
    const expectedDeals = el.columns.reduce((n, c) => n + c.deals.length, 0);
    const cards = host.querySelectorAll(".lthn-view-pipeline-deal");
    expect(cards.length).toBe(expectedDeals);
  });

  it("each stage column carries a data-stage attribute", async () => {
    const { host } = await mountWindow("lthn-view-pipeline");
    const stages = Array.from(host.querySelectorAll(".lthn-view-pipeline-column"))
      .map(c => c.getAttribute("data-stage"));
    expect(stages).toContain("qual");
    expect(stages).toContain("engage");
    expect(stages).toContain("propose");
    expect(stages).toContain("close");
  });
});

describe("lthn-view-pipeline — fixture content", () => {
  it("renders the canonical Crown Estates deal", async () => {
    const { host } = await mountWindow("lthn-view-pipeline");
    expect(host.textContent).toContain("Crown Estates");
    expect(host.textContent).toContain("£82 K");
  });

  it("renders the Heritage Law qualifying deal", async () => {
    const { host } = await mountWindow("lthn-view-pipeline");
    expect(host.textContent).toContain("Heritage Law LLP");
  });

  it("subtitle reflects total deal count + £558 K total", async () => {
    const { host } = await mountWindow("lthn-view-pipeline");
    const header = host.querySelector("header");
    expect(header?.textContent ?? "").toContain("11 deals");
    expect(header?.textContent ?? "").toContain("£558 K");
  });

  it("subtitle updates when columns change", async () => {
    const { el, host } = await mountWindow<PipelineEl>("lthn-view-pipeline");
    el.columns = [
      { id: "qual", label: "Qualifying", value: "£10 K", deals: [
        { c: "A", v: "£10 K", t: "test" },
      ]},
    ];
    await el.updateComplete;
    const header = host.querySelector("header");
    expect(header?.textContent ?? "").toContain("1 deals");
  });
});

describe("lthn-view-pipeline — two-shell", () => {
  it("embedded mode collapses the chrome", async () => {
    const { host } = await mountWindow("lthn-view-pipeline", { attrs: { embedded: "" } });
    expect(isEmbedded(host)).toBe(true);
    expect(host.querySelector("header")).toBeNull();
  });

  it("embedded mode still renders all four columns", async () => {
    const { host } = await mountWindow("lthn-view-pipeline", { attrs: { embedded: "" } });
    expect(host.querySelectorAll(".lthn-view-pipeline-column").length).toBe(4);
  });
});

describe("lthn-view-pipeline — backend mock", () => {
  it("accepts backend data replacing fixture columns", async () => {
    // Simulate what the backend returns: two custom columns.
    const { el } = await mountWindow<PipelineEl>("lthn-view-pipeline");
    el.columns = [
      {
        id: "engage" as const, label: "Engaging", value: "£24 K",
        deals: [{ c: "Heritage Law LLP", v: "£24 K", t: "GDPR + privilege" }],
      },
      {
        id: "propose" as const, label: "Proposal", value: "£0",
        deals: [],
      },
    ];
    await el.updateComplete;
    expect(el.columns.length).toBe(2);
    expect(el.columns[0].deals.length).toBe(1);
    expect(el.columns[0].deals[0].c).toBe("Heritage Law LLP");
  });

  it("_loadFromBackend falls back to fixture when binding is absent", async () => {
    // In JSDOM the @desktop/sales/pipeline/service import fails → null.
    // The element should keep fixture data and loadState should be "idle".
    const { el } = await mountWindow<PipelineEl>("lthn-view-pipeline");
    // Reset to fixture to simulate a fresh element before connectedCallback.
    const FIXTURE_COUNT = el.columns.reduce((n, c) => n + c.deals.length, 0);
    await (el as PipelineEl & { _loadFromBackend: () => Promise<void> })._loadFromBackend();
    await el.updateComplete;
    // Fixture must still be intact (11 deals across 4 fixture columns).
    const stillFixture = el.columns.reduce((n, c) => n + c.deals.length, 0);
    expect(stillFixture).toBe(FIXTURE_COUNT);
  });
});
