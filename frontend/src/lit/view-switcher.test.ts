// SPDX-Licence-Identifier: EUPL-1.2
//
// Tests for <lthn-view-switcher> per
// plans/code/lthn/desktop/views/RFC.plugin-views.md §4.1.
//
// Standard four-spec wire shape per design_lit_view_backend_wire_pattern
// (rows-replace / reject-keeps / empty-keeps / fixtures-still-pass) plus
// the dropdown interaction + emit contract.

import { describe, it, expect, beforeEach } from "vitest";
import { mountWindow } from "../test/window-fixture";
import {
  ViewSwitcherElement,
  VIEW_SWITCHER_SELECT_EVENT,
  type ViewSwitcherEntry,
} from "./view-switcher";
import "./view-switcher";

const FIXTURE_VIEWS: ViewSwitcherEntry[] = [
  { id: "admin", label: "Admin", icon: "fa-gauge" },
  { id: "coding", label: "Coding", icon: "fa-code" },
  { id: "opencode", label: "Opencode", icon: "fa-terminal" },
];

describe("lthn-view-switcher render", () => {
  it("renders nothing visible when views[] is empty (no layout shift)", async () => {
    const { el } = await mountWindow<ViewSwitcherElement>("lthn-view-switcher", {
      props: { views: [] },
    });
    await (el as unknown as { updateComplete: Promise<boolean> }).updateComplete;
    expect(el.querySelector("button.lthn-view-switcher-titlebar")).toBeNull();
  });

  it("renders the active view's label + icon", async () => {
    const { el } = await mountWindow<ViewSwitcherElement>("lthn-view-switcher", {
      props: { views: FIXTURE_VIEWS, activeView: "coding" },
    });
    await (el as unknown as { updateComplete: Promise<boolean> }).updateComplete;
    const btn = el.querySelector("button.lthn-view-switcher-titlebar");
    expect(btn).not.toBeNull();
    expect(btn?.textContent).toContain("Coding");
  });

  it("falls back to the first view when activeView is unknown", async () => {
    const { el } = await mountWindow<ViewSwitcherElement>("lthn-view-switcher", {
      props: { views: FIXTURE_VIEWS, activeView: "no-such-view" },
    });
    await (el as unknown as { updateComplete: Promise<boolean> }).updateComplete;
    const btn = el.querySelector("button.lthn-view-switcher-titlebar");
    expect(btn?.textContent).toContain("Admin"); // first entry
  });
});

describe("lthn-view-switcher dropdown interaction", () => {
  it("opens the menu on click + lists every entry", async () => {
    const { el } = await mountWindow<ViewSwitcherElement>("lthn-view-switcher", {
      props: { views: FIXTURE_VIEWS, activeView: "admin" },
    });
    await (el as unknown as { updateComplete: Promise<boolean> }).updateComplete;
    const trigger = el.querySelector("button.lthn-view-switcher-titlebar") as HTMLButtonElement;
    trigger.click();
    await (el as unknown as { updateComplete: Promise<boolean> }).updateComplete;
    // Menu rendered + 3 entry buttons inside (one per view).
    const menuButtons = el.querySelectorAll("div button");
    expect(menuButtons.length).toBe(FIXTURE_VIEWS.length);
  });

  it("emits lthn:view-switcher:select with the picked id on entry click", async () => {
    const { el } = await mountWindow<ViewSwitcherElement>("lthn-view-switcher", {
      props: { views: FIXTURE_VIEWS, activeView: "admin" },
    });
    await (el as unknown as { updateComplete: Promise<boolean> }).updateComplete;

    let picked: string | null = null;
    el.addEventListener(VIEW_SWITCHER_SELECT_EVENT, (e: Event) => {
      picked = (e as CustomEvent<{ viewId: string }>).detail.viewId;
    });

    const trigger = el.querySelector("button.lthn-view-switcher-titlebar") as HTMLButtonElement;
    trigger.click();
    await (el as unknown as { updateComplete: Promise<boolean> }).updateComplete;

    const codingEntry = Array.from(el.querySelectorAll("div button")).find(
      b => b.textContent?.includes("Coding"),
    ) as HTMLButtonElement;
    codingEntry.click();
    await (el as unknown as { updateComplete: Promise<boolean> }).updateComplete;

    expect(picked).toBe("coding");
  });

  it("closes the menu on outside click", async () => {
    const { el } = await mountWindow<ViewSwitcherElement>("lthn-view-switcher", {
      props: { views: FIXTURE_VIEWS, activeView: "admin" },
    });
    await (el as unknown as { updateComplete: Promise<boolean> }).updateComplete;
    const trigger = el.querySelector("button.lthn-view-switcher-titlebar") as HTMLButtonElement;
    trigger.click();
    await (el as unknown as { updateComplete: Promise<boolean> }).updateComplete;
    // Click outside.
    document.body.click();
    await (el as unknown as { updateComplete: Promise<boolean> }).updateComplete;
    expect(el.querySelectorAll("div button").length).toBe(0);
  });
});

// ─── Backend-wire pattern (4 specs, even though the element accepts
//     props directly; the contract still mirrors design_lit_view_backend_wire_pattern
//     so a future wire-in to VIEWS-via-Go service can drop in without
//     reshaping these tests).

describe("lthn-view-switcher backend wire (4-spec contract)", () => {
  it("props.views replaces fixture default", async () => {
    const custom: ViewSwitcherEntry[] = [{ id: "x", label: "X", icon: "fa-bolt" }];
    const { el } = await mountWindow<ViewSwitcherElement>("lthn-view-switcher", {
      props: { views: custom, activeView: "x" },
    });
    await (el as unknown as { updateComplete: Promise<boolean> }).updateComplete;
    expect(el.querySelector("button.lthn-view-switcher-titlebar")?.textContent).toContain("X");
  });

  it("absent views keeps the layout-safe placeholder (no chrome shift)", async () => {
    const { el } = await mountWindow<ViewSwitcherElement>("lthn-view-switcher");
    await (el as unknown as { updateComplete: Promise<boolean> }).updateComplete;
    expect(el.querySelector("button.lthn-view-switcher-titlebar")).toBeNull();
  });

  it("empty views[] keeps the placeholder", async () => {
    const { el } = await mountWindow<ViewSwitcherElement>("lthn-view-switcher", {
      props: { views: [] },
    });
    await (el as unknown as { updateComplete: Promise<boolean> }).updateComplete;
    expect(el.querySelector("button.lthn-view-switcher-titlebar")).toBeNull();
  });

  it("fixture-only render path stays clickable", async () => {
    const { el } = await mountWindow<ViewSwitcherElement>("lthn-view-switcher", {
      props: { views: FIXTURE_VIEWS, activeView: "admin" },
    });
    await (el as unknown as { updateComplete: Promise<boolean> }).updateComplete;
    const trigger = el.querySelector("button.lthn-view-switcher-titlebar") as HTMLButtonElement;
    expect(trigger).not.toBeNull();
    trigger.click();
    await (el as unknown as { updateComplete: Promise<boolean> }).updateComplete;
    expect(el.querySelectorAll("div button").length).toBe(FIXTURE_VIEWS.length);
  });
});
