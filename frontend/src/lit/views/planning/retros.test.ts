// SPDX-Licence-Identifier: EUPL-1.2

import { describe, it, expect } from "vitest";
import type { LitElement } from "lit";
import { mountWindow, expectChromeTitle, isEmbedded } from "../../../test/window-fixture";
import { toneColours, totalItems } from "./retros";
import "./retros";

interface RetrosEl extends LitElement {
  addNote: (col: "good" | "bad" | "next", body: string, author?: string) => void;
  data: { columns: ReadonlyArray<{ id: string; items: ReadonlyArray<unknown> }> };
  updateComplete: Promise<boolean>;
}

describe("lthn-view-retros — smoke", () => {
  it("mounts with the Retro titlebar", async () => {
    const { host } = await mountWindow("lthn-view-retros");
    expectChromeTitle(host, "Retro");
    expect(host.querySelector("header")).not.toBeNull();
  });

  it("renders all three retro columns", async () => {
    const { host } = await mountWindow("lthn-view-retros");
    const cols = host.querySelectorAll(".lthn-retros-column");
    expect(cols.length).toBe(3);
    const ids = Array.from(cols).map(c => c.getAttribute("data-col"));
    expect(ids).toEqual(["good", "bad", "next"]);
  });

  it("renders the fixture notes across the columns", async () => {
    const { host } = await mountWindow("lthn-view-retros");
    const notes = host.querySelectorAll(".lthn-retros-note");
    // 3 + 2 + 3 = 8 from the fixture
    expect(notes.length).toBe(8);
    expect(host.textContent).toContain("Tray icon family landed on first review");
  });

  it("renders an Add button per column", async () => {
    const { host } = await mountWindow("lthn-view-retros");
    const adds = host.querySelectorAll(".lthn-retros-add");
    expect(adds.length).toBe(3);
  });

  it("addNote appends a note to the named column", async () => {
    const { el, host } = await mountWindow<RetrosEl>("lthn-view-retros");
    el.addNote("good", "shipped extra polish", "you");
    await el.updateComplete;
    const notes = host.querySelectorAll(".lthn-retros-note");
    expect(notes.length).toBe(9);
    expect(host.textContent).toContain("shipped extra polish");
  });

  it("addNote ignores empty body (defensive)", async () => {
    const { el, host } = await mountWindow<RetrosEl>("lthn-view-retros");
    el.addNote("good", "   ");
    await el.updateComplete;
    const notes = host.querySelectorAll(".lthn-retros-note");
    expect(notes.length).toBe(8);
  });

  it("clicking an Add button appends the default note", async () => {
    const { el, host } = await mountWindow<RetrosEl>("lthn-view-retros");
    const addGood = host.querySelector('.lthn-retros-add[data-col="good"]') as HTMLButtonElement;
    addGood.click();
    await el.updateComplete;
    const goodNotes = host.querySelectorAll('.lthn-retros-column[data-col="good"] .lthn-retros-note');
    expect(goodNotes.length).toBe(4);
  });
});

describe("lthn-view-retros — two-shell", () => {
  it("embedded mode collapses the chrome", async () => {
    const { host } = await mountWindow("lthn-view-retros", { attrs: { embedded: "" } });
    expect(isEmbedded(host)).toBe(true);
    expect(host.querySelector("header")).toBeNull();
  });
});

describe("retros pure helpers", () => {
  it("toneColours maps each tone to a [bg, border, fg] triple", () => {
    const ok    = toneColours("ok");
    const warn  = toneColours("warn");
    const brand = toneColours("brand");
    expect(ok.length).toBe(3);
    expect(warn.length).toBe(3);
    expect(brand.length).toBe(3);
    expect(ok[2]).toContain("success");
    expect(warn[2]).toContain("warning");
    expect(brand[2]).toContain("brand");
  });

  it("totalItems sums across columns", () => {
    const cols = [
      { id: "good" as const, label: "G", tone: "ok"    as const, icon: "x", items: [{ t: "a", by: "x" }, { t: "b", by: "y" }] },
      { id: "bad"  as const, label: "B", tone: "warn"  as const, icon: "x", items: [{ t: "c", by: "x" }] },
      { id: "next" as const, label: "N", tone: "brand" as const, icon: "x", items: [] },
    ];
    expect(totalItems(cols)).toBe(3);
  });
});
