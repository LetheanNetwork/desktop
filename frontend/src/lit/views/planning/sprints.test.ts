// SPDX-Licence-Identifier: EUPL-1.2

import { describe, it, expect } from "vitest";
import type { LitElement } from "lit";
import { mountWindow, expectChromeTitle, isEmbedded } from "../../../test/window-fixture";
import { filterCards } from "./sprints";
import "./sprints";

interface SprintsEl extends LitElement {
  setQuery: (q: string) => void;
  data: { columns: ReadonlyArray<{ id: string; cards: ReadonlyArray<{ id: string }> }> };
  updateComplete: Promise<boolean>;
}

describe("lthn-view-sprints — smoke", () => {
  it("mounts with the Sprint titlebar", async () => {
    const { host } = await mountWindow("lthn-view-sprints");
    expectChromeTitle(host, "Sprint 24");
    expect(host.querySelector("header")).not.toBeNull();
  });

  it("renders all four kanban columns", async () => {
    const { host } = await mountWindow("lthn-view-sprints");
    const cols = host.querySelectorAll(".lthn-sprints-column");
    expect(cols.length).toBe(4);
    const ids = Array.from(cols).map(c => c.getAttribute("data-col"));
    expect(ids).toEqual(["todo", "doing", "review", "done"]);
  });

  it("renders the fixture cards across the board", async () => {
    const { host } = await mountWindow("lthn-view-sprints");
    const cards = host.querySelectorAll(".lthn-sprints-card");
    // 4 + 3 + 2 + 5 = 14 from the fixture
    expect(cards.length).toBe(14);
    expect(host.textContent).toContain("L-184");
    expect(host.textContent).toContain("LoRA training UI");
  });

  it("filter input narrows the visible cards", async () => {
    const { el, host } = await mountWindow<SprintsEl>("lthn-view-sprints");
    el.setQuery("LoRA");
    await el.updateComplete;
    const cards = host.querySelectorAll(".lthn-sprints-card");
    expect(cards.length).toBe(1);
    expect(host.textContent).toContain("LoRA training UI");
  });

  it("filter is reset by an empty query", async () => {
    const { el, host } = await mountWindow<SprintsEl>("lthn-view-sprints");
    el.setQuery("LoRA");
    await el.updateComplete;
    el.setQuery("");
    await el.updateComplete;
    const cards = host.querySelectorAll(".lthn-sprints-card");
    expect(cards.length).toBe(14);
  });

  it("filter input is wired into setQuery via @input", async () => {
    const { el, host } = await mountWindow<SprintsEl>("lthn-view-sprints");
    const input = host.querySelector(".lthn-sprints-filter") as HTMLInputElement;
    expect(input).not.toBeNull();
    input.value = "MCP";
    input.dispatchEvent(new Event("input"));
    await el.updateComplete;
    const cards = host.querySelectorAll(".lthn-sprints-card");
    // Two MCP-related cards in the fixture: "Document MCP tool registry
    // format" (To do) and "MCP tools window v1" (Done).
    expect(cards.length).toBe(2);
  });
});

describe("lthn-view-sprints — two-shell", () => {
  it("embedded mode collapses the chrome", async () => {
    const { host } = await mountWindow("lthn-view-sprints", { attrs: { embedded: "" } });
    expect(isEmbedded(host)).toBe(true);
    expect(host.querySelector("header")).toBeNull();
  });
});

describe("filterCards — pure helper", () => {
  const cols = [
    { id: "todo" as const, label: "To do", count: 2, cards: [
      { id: "L-1", title: "alpha card", est: 1, tag: "feat" as const },
      { id: "L-2", title: "beta card",  est: 2, tag: "ops"  as const },
    ]},
    { id: "doing" as const, label: "In flight", count: 1, cards: [
      { id: "L-3", title: "gamma card", est: 3, tag: "ui" as const },
    ]},
    { id: "review" as const, label: "Review", count: 0, cards: [] },
    { id: "done"   as const, label: "Done",   count: 0, cards: [] },
  ];

  it("returns input columns when query is empty", () => {
    const out = filterCards(cols, "");
    expect(out.length).toBe(4);
    expect(out[0].cards.length).toBe(2);
  });

  it("filters by title substring", () => {
    const out = filterCards(cols, "alpha");
    expect(out[0].cards.length).toBe(1);
    expect(out[0].cards[0].id).toBe("L-1");
  });

  it("filters by id substring (case-insensitive)", () => {
    const out = filterCards(cols, "l-3");
    expect(out[1].cards.length).toBe(1);
    expect(out[1].cards[0].title).toBe("gamma card");
  });

  it("recomputes column count to match filtered cards", () => {
    const out = filterCards(cols, "alpha");
    expect(out[0].count).toBe(1);
    expect(out[1].count).toBe(0);
  });
});
