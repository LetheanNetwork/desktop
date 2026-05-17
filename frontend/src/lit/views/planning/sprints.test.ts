// SPDX-Licence-Identifier: EUPL-1.2

import { describe, it, expect, vi } from "vitest";
import type { LitElement } from "lit";
import { mountWindow, expectChromeTitle, isEmbedded } from "../../../test/window-fixture";
import { filterCards } from "./sprints";

vi.mock("@desktop/tasks/wailsservice", () => ({
  List: vi.fn(),
}));

import { List as TasksList } from "@desktop/tasks/wailsservice";

import "./sprints";

interface SprintsEl extends LitElement {
  setQuery: (q: string) => void;
  data: {
    columns: ReadonlyArray<{ id: string; cards: ReadonlyArray<{ id: string; title?: string }> }>;
  };
  _loadFromBackend: () => Promise<void>;
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

describe("lthn-view-sprints — backend wire (3-column partial)", () => {
  it("replaces todo/doing/done columns with Issues from tasks.List", async () => {
    (TasksList as unknown as { mockReset: () => void }).mockReset();
    (TasksList as unknown as { mockResolvedValue: (v: unknown) => void }).mockResolvedValue({
      Value: { issues: [
        { ID: "a1", Summary: "Wire pkg/sales contacts", State: "open" },
        { ID: "b2", Summary: "Stage E LetheanAccount unlock", State: "in_progress" },
        { ID: "c3", Summary: "Cerberus pass-9 audit",          State: "done" },
      ] },
    });
    const { el } = await mountWindow<SprintsEl>("lthn-view-sprints");
    await el._loadFromBackend();
    await el.updateComplete;
    const todo  = el.data.columns.find(c => c.id === "todo")!;
    const doing = el.data.columns.find(c => c.id === "doing")!;
    const done  = el.data.columns.find(c => c.id === "done")!;
    expect(todo.cards.length).toBe(1);
    expect(todo.cards[0].title).toBe("Wire pkg/sales contacts");
    expect(doing.cards.length).toBe(1);
    expect(doing.cards[0].title).toBe("Stage E LetheanAccount unlock");
    expect(done.cards.length).toBe(1);
    expect(done.cards[0].title).toBe("Cerberus pass-9 audit");
  });

  it("preserves the fixture 'review' column (no backend State maps to it)", async () => {
    (TasksList as unknown as { mockReset: () => void }).mockReset();
    (TasksList as unknown as { mockResolvedValue: (v: unknown) => void }).mockResolvedValue({
      Value: { issues: [
        { ID: "x1", Summary: "an open issue", State: "open" },
      ] },
    });
    const { el } = await mountWindow<SprintsEl>("lthn-view-sprints");
    await el._loadFromBackend();
    await el.updateComplete;
    const review = el.data.columns.find(c => c.id === "review")!;
    expect(review.cards.length).toBeGreaterThan(0); // fixture preserved
  });

  it("keeps fixture columns when backend rejects", async () => {
    (TasksList as unknown as { mockReset: () => void }).mockReset();
    (TasksList as unknown as { mockRejectedValue: (v: unknown) => void }).mockRejectedValue(new Error("no binding"));
    const { el } = await mountWindow<SprintsEl>("lthn-view-sprints");
    await el._loadFromBackend();
    await el.updateComplete;
    const todo = el.data.columns.find(c => c.id === "todo")!;
    expect(todo.cards.length).toBeGreaterThan(0); // fixture preserved
  });

  it("keeps fixture when backend returns empty", async () => {
    (TasksList as unknown as { mockReset: () => void }).mockReset();
    (TasksList as unknown as { mockResolvedValue: (v: unknown) => void }).mockResolvedValue({
      Value: { issues: [] },
    });
    const { el } = await mountWindow<SprintsEl>("lthn-view-sprints");
    await el._loadFromBackend();
    await el.updateComplete;
    const todo = el.data.columns.find(c => c.id === "todo")!;
    expect(todo.cards.length).toBeGreaterThan(0);
  });
});
