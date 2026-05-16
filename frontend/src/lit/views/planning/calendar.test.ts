// SPDX-Licence-Identifier: EUPL-1.2

import { describe, it, expect } from "vitest";
import { mountWindow, expectChromeTitle, isEmbedded } from "../../../test/window-fixture";
import { eventColours, countFocusBlocks } from "./calendar";
import "./calendar";

describe("lthn-view-calendar — smoke", () => {
  it("mounts with the Calendar titlebar", async () => {
    const { host } = await mountWindow("lthn-view-calendar");
    expectChromeTitle(host, "Calendar");
    expect(host.querySelector("header")).not.toBeNull();
  });

  it("renders the five day-headers", async () => {
    const { host } = await mountWindow("lthn-view-calendar");
    const headers = host.querySelectorAll(".lthn-calendar-day-header");
    expect(headers.length).toBe(5);
    expect(host.textContent).toContain("Mon");
    expect(host.textContent).toContain("Fri");
  });

  it("highlights the today column via data attribute", async () => {
    const { host } = await mountWindow("lthn-view-calendar");
    const today = host.querySelectorAll('.lthn-calendar-day-header[data-today="true"]');
    expect(today.length).toBe(1);
  });

  it("renders the fixture events on the grid", async () => {
    const { host } = await mountWindow("lthn-view-calendar");
    const events = host.querySelectorAll(".lthn-calendar-event");
    expect(events.length).toBe(14);
    expect(host.textContent).toContain("Calliope VC call");
    expect(host.textContent).toContain("Friday demo");
  });

  it("renders 9 hour rows (09:00 → 17:00)", async () => {
    const { host } = await mountWindow("lthn-view-calendar");
    expect(host.textContent).toContain("09:00");
    expect(host.textContent).toContain("17:00");
  });

  it("tags events with their tone via data attribute", async () => {
    const { host } = await mountWindow("lthn-view-calendar");
    const focus = host.querySelectorAll('.lthn-calendar-event[data-tone="focus"]');
    const ship  = host.querySelectorAll('.lthn-calendar-event[data-tone="ship"]');
    const brand = host.querySelectorAll('.lthn-calendar-event[data-tone="brand"]');
    expect(focus.length).toBe(3);   // 3 deep-work blocks
    expect(ship.length).toBe(2);    // v0.2 release + Friday demo
    expect(brand.length).toBe(3);   // VC call + Sprint planning + Retro
  });
});

describe("lthn-view-calendar — two-shell", () => {
  it("embedded mode collapses the chrome", async () => {
    const { host } = await mountWindow("lthn-view-calendar", { attrs: { embedded: "" } });
    expect(isEmbedded(host)).toBe(true);
    expect(host.querySelector("header")).toBeNull();
  });
});

describe("calendar pure helpers", () => {
  it("eventColours maps every tone to a [bg, border, fg] triple", () => {
    expect(eventColours("focus").length).toBe(3);
    expect(eventColours("brand").length).toBe(3);
    expect(eventColours("ship").length).toBe(3);
    expect(eventColours("mute").length).toBe(3);
  });

  it("eventColours brand uses the Lethean brand var", () => {
    const [, , fg] = eventColours("brand");
    expect(fg).toContain("brand");
  });

  it("countFocusBlocks returns the number of focus events", () => {
    const events = [
      { d: 0, h: 1, dur: 1, title: "a", tone: "focus" as const },
      { d: 0, h: 2, dur: 2, title: "b", tone: "focus" as const },
      { d: 1, h: 1, dur: 1, title: "c", tone: "ship"  as const },
      { d: 1, h: 3, dur: 1, title: "d", tone: "mute"  as const },
    ];
    expect(countFocusBlocks(events)).toBe(2);
  });

  it("countFocusBlocks returns 0 for an empty list", () => {
    expect(countFocusBlocks([])).toBe(0);
  });
});
