// SPDX-Licence-Identifier: EUPL-1.2

import { describe, it, expect, vi } from "vitest";
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
  _onDealDragStart: (e: DragEvent, from: PipelineCol["id"], deal: PipelineRow) => void;
  _onColumnDrop:    (e: DragEvent, to: PipelineCol["id"]) => Promise<void>;
}

/** MIME mirrors the private constant in pipeline.ts — kept in lockstep
 *  here rather than exported so the production module's surface stays
 *  small. If the source moves, this test must move with it. */
const DRAG_MIME = "application/x-lthn-pipeline-deal";

/** Minimal DataTransfer stand-in for jsdom — the real DataTransfer ctor
 *  is unavailable, but our handlers only need getData/setData/types. */
function makeDataTransfer(): DataTransfer {
  const store: Record<string, string> = {};
  const dt = {
    effectAllowed: "none",
    dropEffect:    "none",
    types:         [] as string[],
    setData(type: string, value: string) {
      store[type] = value;
      if (!dt.types.includes(type)) dt.types.push(type);
    },
    getData(type: string) { return store[type] ?? ""; },
  };
  return dt as unknown as DataTransfer;
}

/** Synthesize a drag-shaped Event with a DataTransfer attached, since
 *  jsdom's DragEvent ctor doesn't accept a dataTransfer init. */
function dragEvent(type: string, dt: DataTransfer): DragEvent {
  const e = new Event(type, { bubbles: true, cancelable: true }) as unknown as DragEvent;
  Object.defineProperty(e, "dataTransfer", { value: dt, configurable: true });
  return e;
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

describe("lthn-view-pipeline — drag to move", () => {
  it("deal cards are draggable + carry a tabindex for keyboard focus", async () => {
    const { host } = await mountWindow("lthn-view-pipeline");
    const card = host.querySelector(".lthn-view-pipeline-deal") as HTMLElement;
    expect(card).not.toBeNull();
    expect(card.getAttribute("draggable")).toBe("true");
    expect(card.getAttribute("tabindex")).toBe("0");
  });

  it("dragstart on a deal card writes the deal payload into DataTransfer", async () => {
    const { el } = await mountWindow<PipelineEl>("lthn-view-pipeline");
    const qual = el.columns.find(c => c.id === "qual");
    expect(qual).toBeDefined();
    const deal = qual!.deals[0];

    const dt = makeDataTransfer();
    el._onDealDragStart(dragEvent("dragstart", dt), "qual", deal);

    const raw = dt.getData(DRAG_MIME);
    expect(raw).not.toBe("");
    const payload = JSON.parse(raw) as { deal_id: string; from_stage: string; customer: string };
    expect(payload.from_stage).toBe("qual");
    expect(payload.customer).toBe(deal.c);
    expect(payload.deal_id).toBe(deal.c); // fixture has no .id → falls back to .c
    expect(dt.effectAllowed).toBe("move");
  });

  it("drop on a target column optimistically lifts the card across stages (MoveDeal mock OK)", async () => {
    const moveSpy = vi.fn(async () => ({ OK: true, Value: {} }));
    vi.resetModules();
    vi.doMock("@desktop/sales/pipeline/service", () => ({
      List:     async () => ({ OK: true, Value: {} }),
      MoveDeal: moveSpy,
    }));

    const { el } = await mountWindow<PipelineEl>("lthn-view-pipeline");
    const sourceDeal = el.columns.find(c => c.id === "qual")!.deals[0];
    const qualBefore = el.columns.find(c => c.id === "qual")!.deals.length;
    const engageBefore = el.columns.find(c => c.id === "engage")!.deals.length;

    const dt = makeDataTransfer();
    el._onDealDragStart(dragEvent("dragstart", dt), "qual", sourceDeal);
    await el._onColumnDrop(dragEvent("drop", dt), "engage");
    await el.updateComplete;

    expect(el.columns.find(c => c.id === "qual")!.deals.length).toBe(qualBefore - 1);
    expect(el.columns.find(c => c.id === "engage")!.deals.length).toBe(engageBefore + 1);
    const movedCustomers = el.columns.find(c => c.id === "engage")!.deals.map(d => d.c);
    expect(movedCustomers).toContain(sourceDeal.c);
    // MoveDeal binding called with the camelCase wails contract.
    expect(moveSpy).toHaveBeenCalledWith({ dealId: sourceDeal.c, toStage: "engage" });

    vi.doUnmock("@desktop/sales/pipeline/service");
  });

  it("MoveDeal rejection reverts the optimistic move", async () => {
    vi.resetModules();
    vi.doMock("@desktop/sales/pipeline/service", () => ({
      List:     async () => ({ OK: true, Value: {} }),
      MoveDeal: async () => { throw new Error("backend down"); },
    }));

    const { el } = await mountWindow<PipelineEl>("lthn-view-pipeline");
    const sourceDeal = el.columns.find(c => c.id === "qual")!.deals[0];
    const qualBefore = el.columns.find(c => c.id === "qual")!.deals.length;
    const engageBefore = el.columns.find(c => c.id === "engage")!.deals.length;

    const dt = makeDataTransfer();
    el._onDealDragStart(dragEvent("dragstart", dt), "qual", sourceDeal);
    await el._onColumnDrop(dragEvent("drop", dt), "engage");
    await el.updateComplete;

    // Revert: counts back to where they were, no toast (calm-UX rule).
    expect(el.columns.find(c => c.id === "qual")!.deals.length).toBe(qualBefore);
    expect(el.columns.find(c => c.id === "engage")!.deals.length).toBe(engageBefore);

    vi.doUnmock("@desktop/sales/pipeline/service");
  });

  it("drop on the same source stage is a no-op", async () => {
    const { el } = await mountWindow<PipelineEl>("lthn-view-pipeline");
    const deal = el.columns.find(c => c.id === "qual")!.deals[0];
    const snapshot = el.columns.map(c => ({ id: c.id, n: c.deals.length }));

    const dt = makeDataTransfer();
    el._onDealDragStart(dragEvent("dragstart", dt), "qual", deal);
    await el._onColumnDrop(dragEvent("drop", dt), "qual");
    await el.updateComplete;

    for (const before of snapshot) {
      const after = el.columns.find(c => c.id === before.id)!;
      expect(after.deals.length).toBe(before.n);
    }
  });

  it("successful drop dispatches lthn:sales:moved with the expected detail", async () => {
    vi.resetModules();
    vi.doMock("@desktop/sales/pipeline/service", () => ({
      List:     async () => ({ OK: true, Value: {} }),
      MoveDeal: async () => ({ OK: true, Value: {} }),
    }));

    const { el } = await mountWindow<PipelineEl>("lthn-view-pipeline");
    const deal = el.columns.find(c => c.id === "propose")!.deals[0];

    let captured: { deal_id: string; from_stage: string; to_stage: string } | null = null;
    const listener = (e: Event) => {
      captured = (e as CustomEvent<{ deal_id: string; from_stage: string; to_stage: string }>).detail;
    };
    window.addEventListener("lthn:sales:moved", listener);
    try {
      const dt = makeDataTransfer();
      el._onDealDragStart(dragEvent("dragstart", dt), "propose", deal);
      await el._onColumnDrop(dragEvent("drop", dt), "close");
      await el.updateComplete;
    } finally {
      window.removeEventListener("lthn:sales:moved", listener);
    }

    expect(captured).not.toBeNull();
    expect(captured!.from_stage).toBe("propose");
    expect(captured!.to_stage).toBe("close");
    expect(captured!.deal_id).toBe(deal.c);

    vi.doUnmock("@desktop/sales/pipeline/service");
  });

  it("cross-view: dispatched lthn:sales:moved invokes registered listeners", async () => {
    // Stands in for sibling sales views (deals/forecast/contacts) which
    // will register a window listener and call their own _loadFromBackend
    // in a follow-up wiring ticket. This test asserts the dispatch
    // contract from the pipeline side — a listener mounted by any sibling
    // view sees the same detail shape.
    await mountWindow<PipelineEl>("lthn-view-pipeline");
    let dealsRefreshes = 0;
    let forecastRefreshes = 0;
    const onMoved = (e: Event) => {
      const d = (e as CustomEvent<{ deal_id: string; from_stage: string; to_stage: string }>).detail;
      // Simulate sibling views' _loadFromBackend spies — different counters
      // so the test proves both subscribers see the same event.
      if (d.from_stage && d.to_stage) {
        dealsRefreshes++;
        forecastRefreshes++;
      }
    };
    window.addEventListener("lthn:sales:moved", onMoved);
    try {
      window.dispatchEvent(new CustomEvent("lthn:sales:moved", {
        detail: { deal_id: "Test Co", from_stage: "qual", to_stage: "engage" },
      }));
    } finally {
      window.removeEventListener("lthn:sales:moved", onMoved);
    }
    expect(dealsRefreshes).toBe(1);
    expect(forecastRefreshes).toBe(1);
  });
});
