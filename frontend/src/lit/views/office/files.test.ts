// SPDX-Licence-Identifier: EUPL-1.2

import { describe, it, expect } from "vitest";
import type { LitElement } from "lit";
import { mountWindow, expectChromeTitle, findCard, isEmbedded } from "../../../test/window-fixture";
import "./files";

describe("lthn-view-files — smoke", () => {
  it("mounts without crashing and produces the Files card", async () => {
    const { host } = await mountWindow("lthn-view-files");
    expect(findCard(host)).not.toBeNull();
    expectChromeTitle(host, "Files");
  });

  it("renders all five fixture locations in the left rail", async () => {
    const { host } = await mountWindow("lthn-view-files");
    const locations = host.querySelectorAll(".lthn-view-files-location");
    expect(locations.length).toBe(5);
    expect(host.textContent).toContain("Code");
    expect(host.textContent).toContain("Documents");
    expect(host.textContent).toContain("lthn / models");
    expect(host.textContent).toContain("Recordings");
    expect(host.textContent).toContain("Screenshots");
  });

  it("brand-flags the Lethean-managed models folder", async () => {
    const { host } = await mountWindow("lthn-view-files");
    const branded = host.querySelectorAll(".lthn-view-files-location[data-brand]");
    expect(branded.length).toBe(1);
    expect(branded[0].textContent).toContain("lthn / models");
  });

  it("renders all five fixture recent files", async () => {
    const { host } = await mountWindow("lthn-view-files");
    const rows = host.querySelectorAll(".lthn-view-files-row");
    expect(rows.length).toBe(5);
    expect(host.textContent).toContain("sow-heritage-law-v2.md");
    expect(host.textContent).toContain("gemma-4-e2b-q4_k_m.gguf");
    expect(host.textContent).toContain("lthn-icon-helmet@2x.png");
  });

  it("renders the disk-free meter at the bottom of the rail", async () => {
    const { host } = await mountWindow("lthn-view-files");
    const disk = host.querySelector(".lthn-view-files-disk");
    expect(disk, "disk meter present").not.toBeNull();
    expect(disk?.textContent).toContain("312 GB");
    expect(disk?.textContent).toContain("1 TB");
  });

  it("footer cites the local indexer and shortcut hints", async () => {
    const { host } = await mountWindow("lthn-view-files");
    expect(host.textContent).toContain("indexed by lthn");
    expect(host.textContent).toContain("⌘O to open");
    expect(host.textContent).toContain("pkg/files pending");
  });
});

describe("lthn-view-files — location selection", () => {
  type FilesEl = LitElement & {
    selectedLocation: string | null;
    _selectLocation: (name: string) => void;
    updateComplete: Promise<boolean>;
  };

  it("starts with no selected location", async () => {
    const { el, host } = await mountWindow<FilesEl>("lthn-view-files");
    expect(el.selectedLocation).toBeNull();
    expect(host.textContent).toContain("all locations");
  });

  it("selecting a location updates the filter banner", async () => {
    const { el, host } = await mountWindow<FilesEl>("lthn-view-files");
    el._selectLocation("Documents");
    await el.updateComplete;
    expect(el.selectedLocation).toBe("Documents");
    expect(host.textContent).toContain("filtered: Documents");
  });

  it("clicking the same location twice clears the filter", async () => {
    const { el, host } = await mountWindow<FilesEl>("lthn-view-files");
    el._selectLocation("Code");
    await el.updateComplete;
    expect(el.selectedLocation).toBe("Code");
    el._selectLocation("Code");
    await el.updateComplete;
    expect(el.selectedLocation).toBeNull();
    expect(host.textContent).toContain("all locations");
  });
});

describe("lthn-view-files — embedded mode", () => {
  it("embedded attribute collapses the chrome frame", async () => {
    const { host } = await mountWindow("lthn-view-files", { attrs: { embedded: "" } });
    expect(isEmbedded(host)).toBe(true);
    expect(host.querySelector("header")).toBeNull();
  });

  it("embedded mode still renders the two-pane layout", async () => {
    const { host } = await mountWindow("lthn-view-files", { attrs: { embedded: "" } });
    expect(host.querySelector(".lthn-view-files-locations")).not.toBeNull();
    expect(host.querySelector(".lthn-view-files-recent")).not.toBeNull();
  });
});
