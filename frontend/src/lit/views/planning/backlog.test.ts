// SPDX-Licence-Identifier: EUPL-1.2

import { describe, it, expect } from "vitest";
import type { LitElement } from "lit";
import { mountWindow, expectChromeTitle, isEmbedded } from "../../../test/window-fixture";
import { projectBacklog } from "./backlog";
import "./backlog";

interface BacklogEl extends LitElement {
  setQuery: (q: string) => void;
  setSort:  (k: "score" | "impact" | "effort" | "conf") => void;
  data: { sortBy: string; sortDir: "asc" | "desc" };
  updateComplete: Promise<boolean>;
}

describe("lthn-view-backlog — smoke", () => {
  it("mounts with the Backlog titlebar", async () => {
    const { host } = await mountWindow("lthn-view-backlog");
    expectChromeTitle(host, "Backlog");
    expect(host.querySelector("header")).not.toBeNull();
  });

  it("renders all 8 fixture rows by default", async () => {
    const { host } = await mountWindow("lthn-view-backlog");
    const rows = host.querySelectorAll(".lthn-backlog-row");
    expect(rows.length).toBe(8);
  });

  it("renders the sort headers as clickable buttons", async () => {
    const { host } = await mountWindow("lthn-view-backlog");
    const headers = host.querySelectorAll(".lthn-backlog-sort");
    // Score / Impact / Effort / Conf
    expect(headers.length).toBe(4);
  });

  it("search input narrows the visible rows", async () => {
    const { el, host } = await mountWindow<BacklogEl>("lthn-view-backlog");
    el.setQuery("Telemetry");
    await el.updateComplete;
    const rows = host.querySelectorAll(".lthn-backlog-row");
    expect(rows.length).toBe(1);
    expect(host.textContent).toContain("Telemetry · per-model success rate");
  });

  it("empty state shows a helpful message", async () => {
    const { el, host } = await mountWindow<BacklogEl>("lthn-view-backlog");
    el.setQuery("zzz-nope-no-match");
    await el.updateComplete;
    expect(host.querySelector(".lthn-backlog-empty")).not.toBeNull();
    expect(host.textContent).toContain("No items match");
  });

  it("clicking a sort header flips direction on the active key", async () => {
    const { el } = await mountWindow<BacklogEl>("lthn-view-backlog");
    expect(el.data.sortBy).toBe("score");
    expect(el.data.sortDir).toBe("desc");
    el.setSort("score");
    expect(el.data.sortDir).toBe("asc");
  });

  it("clicking a different sort header switches key + resets to desc", async () => {
    const { el } = await mountWindow<BacklogEl>("lthn-view-backlog");
    el.setSort("impact");
    expect(el.data.sortBy).toBe("impact");
    expect(el.data.sortDir).toBe("desc");
  });
});

describe("lthn-view-backlog — two-shell", () => {
  it("embedded mode collapses the chrome", async () => {
    const { host } = await mountWindow("lthn-view-backlog", { attrs: { embedded: "" } });
    expect(isEmbedded(host)).toBe(true);
    expect(host.querySelector("header")).toBeNull();
  });
});

describe("projectBacklog — pure helper", () => {
  const items = [
    { id: "L-1", title: "alpha", score: 90, impact: 5, effort: 1, conf: 5, theme: "x" },
    { id: "L-2", title: "beta",  score: 70, impact: 3, effort: 3, conf: 4, theme: "x" },
    { id: "L-3", title: "gamma", score: 80, impact: 4, effort: 2, conf: 4, theme: "y" },
  ];

  it("returns descending by score by default", () => {
    const out = projectBacklog(items, "", "score", "desc");
    expect(out.map(i => i.id)).toEqual(["L-1", "L-3", "L-2"]);
  });

  it("returns ascending by score when sortDir = asc", () => {
    const out = projectBacklog(items, "", "score", "asc");
    expect(out.map(i => i.id)).toEqual(["L-2", "L-3", "L-1"]);
  });

  it("filters by title substring", () => {
    const out = projectBacklog(items, "beta", "score", "desc");
    expect(out.length).toBe(1);
    expect(out[0].id).toBe("L-2");
  });

  it("sorts by alternative keys (impact desc)", () => {
    const out = projectBacklog(items, "", "impact", "desc");
    expect(out.map(i => i.impact)).toEqual([5, 4, 3]);
  });

  it("does not mutate the input array", () => {
    const ids = items.map(i => i.id);
    projectBacklog(items, "", "score", "asc");
    expect(items.map(i => i.id)).toEqual(ids);
  });
});
