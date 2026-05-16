// SPDX-Licence-Identifier: EUPL-1.2

import { describe, it, expect } from "vitest";
import type { LitElement } from "lit";
import { mountWindow, expectChromeTitle, isEmbedded } from "../../../test/window-fixture";
import { visibleLanes } from "./roadmap";
import "./roadmap";

interface RoadmapEl extends LitElement {
  toggleLane: (id: string) => void;
  data: { hidden: ReadonlySet<string>; lanes: ReadonlyArray<{ id: string }> };
  updateComplete: Promise<boolean>;
}

describe("lthn-view-roadmap — smoke", () => {
  it("mounts with the Roadmap titlebar", async () => {
    const { host } = await mountWindow("lthn-view-roadmap");
    expectChromeTitle(host, "Roadmap");
    expect(host.querySelector("header")).not.toBeNull();
  });

  it("renders all four lanes from the fixture", async () => {
    const { host } = await mountWindow("lthn-view-roadmap");
    const lanes = host.querySelectorAll(".lthn-roadmap-lane");
    expect(lanes.length).toBe(4);
    expect(host.textContent).toContain("Tray · L1");
    expect(host.textContent).toContain("Commercial");
  });

  it("renders done items with the green-tick style", async () => {
    const { host } = await mountWindow("lthn-view-roadmap");
    const done = host.querySelectorAll('.lthn-roadmap-item[data-done="true"]');
    // v0.2 GA + go-mlx Metal optimisation + Host UK channel pilots = 3 done
    expect(done.length).toBe(3);
  });

  it("renders the lane filter pills, one per lane", async () => {
    const { host } = await mountWindow("lthn-view-roadmap");
    const toggles = host.querySelectorAll(".lthn-roadmap-lane-toggle");
    expect(toggles.length).toBe(4);
  });

  it("toggleLane hides the lane from the rendered list", async () => {
    const { el, host } = await mountWindow<RoadmapEl>("lthn-view-roadmap");
    el.toggleLane("L1");
    await el.updateComplete;
    const lanes = host.querySelectorAll(".lthn-roadmap-lane");
    expect(lanes.length).toBe(3);
    expect(host.textContent).not.toContain("v0.2 GA");
  });

  it("toggleLane is reversible", async () => {
    const { el, host } = await mountWindow<RoadmapEl>("lthn-view-roadmap");
    el.toggleLane("L1");
    await el.updateComplete;
    el.toggleLane("L1");
    await el.updateComplete;
    const lanes = host.querySelectorAll(".lthn-roadmap-lane");
    expect(lanes.length).toBe(4);
  });

  it("renders an empty-state hint when all lanes are hidden", async () => {
    const { el, host } = await mountWindow<RoadmapEl>("lthn-view-roadmap");
    for (const lane of el.data.lanes) el.toggleLane(lane.id);
    await el.updateComplete;
    expect(host.querySelector(".lthn-roadmap-empty")).not.toBeNull();
  });

  it("clicking a toggle pill flips lane visibility", async () => {
    const { el, host } = await mountWindow<RoadmapEl>("lthn-view-roadmap");
    const toggle = host.querySelector('.lthn-roadmap-lane-toggle[data-lane="GTM"]') as HTMLButtonElement;
    expect(toggle).not.toBeNull();
    toggle.click();
    await el.updateComplete;
    const lanes = host.querySelectorAll(".lthn-roadmap-lane");
    expect(lanes.length).toBe(3);
  });
});

describe("lthn-view-roadmap — two-shell", () => {
  it("embedded mode collapses the chrome", async () => {
    const { host } = await mountWindow("lthn-view-roadmap", { attrs: { embedded: "" } });
    expect(isEmbedded(host)).toBe(true);
    expect(host.querySelector("header")).toBeNull();
  });
});

describe("visibleLanes — pure helper", () => {
  const lanes = [
    { id: "A", label: "A", items: [] },
    { id: "B", label: "B", items: [] },
    { id: "C", label: "C", items: [] },
  ];

  it("returns all lanes when nothing is hidden", () => {
    expect(visibleLanes(lanes, new Set()).map(l => l.id)).toEqual(["A", "B", "C"]);
  });

  it("filters out hidden lane ids", () => {
    expect(visibleLanes(lanes, new Set(["B"])).map(l => l.id)).toEqual(["A", "C"]);
  });

  it("returns empty array when every lane is hidden", () => {
    expect(visibleLanes(lanes, new Set(["A", "B", "C"]))).toEqual([]);
  });
});
