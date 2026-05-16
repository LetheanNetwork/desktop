// SPDX-Licence-Identifier: EUPL-1.2

import { describe, it, expect, vi, beforeEach } from "vitest";
import type { LitElement } from "lit";
import { mountWindow, expectChromeTitle, findCard, isEmbedded } from "../../../test/window-fixture";
import "./files";

vi.mock("@desktop/office/files/service", () => ({
  ListLocations: vi.fn(),
  ListRecent: vi.fn(),
  GetDiskUsage: vi.fn(),
}));

import { ListLocations as MockedListLocations, ListRecent as MockedListRecent, GetDiskUsage as MockedGetDiskUsage } from "@desktop/office/files/service";

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

describe("lthn-view-files — backend wire", () => {
  type FilesEl = LitElement & {
    locations: { name: string; count: number; size: string; brand?: boolean }[];
    recent: { name: string; path: string; when: string; size: string }[];
    disk: { free: string; total: string; used: number };
    _loadFromBackend: () => Promise<void>;
    updateComplete: Promise<boolean>;
  };

  beforeEach(() => {
    (MockedListLocations as unknown as { mockReset: () => void }).mockReset();
    (MockedListRecent as unknown as { mockReset: () => void }).mockReset();
    (MockedGetDiskUsage as unknown as { mockReset: () => void }).mockReset();
  });

  it("backend rows replace fixture when locations, recents, and disk are returned", async () => {
    (MockedListLocations as unknown as { mockResolvedValue: (v: unknown) => void })
      .mockResolvedValue({ Value: { locations: [{ name: "Backend Location", count: 7, size: "1.1 GB" }] } });
    (MockedListRecent as unknown as { mockResolvedValue: (v: unknown) => void })
      .mockResolvedValue({ Value: { files: [{ name: "backend-file.txt", path: "~/Backend/", when: "now", size: "1 KB" }] } });
    (MockedGetDiskUsage as unknown as { mockResolvedValue: (v: unknown) => void })
      .mockResolvedValue({ Value: { free: "500 GB", total: "2 TB", used: 25 } });

    const { el, host } = await mountWindow<FilesEl>("lthn-view-files");
    // Wait for connectedCallback's _loadFromBackend to finish.
    await new Promise(r => setTimeout(r, 0));
    await el.updateComplete;

    expect(host.textContent).toContain("Backend Location");
    expect(host.textContent).toContain("backend-file.txt");
    expect(host.textContent).toContain("500 GB");
  });

  it("backend rejection keeps fixture data visible", async () => {
    (MockedListLocations as unknown as { mockRejectedValue: (v: unknown) => void })
      .mockRejectedValue(new Error("unavailable"));
    (MockedListRecent as unknown as { mockRejectedValue: (v: unknown) => void })
      .mockRejectedValue(new Error("unavailable"));
    (MockedGetDiskUsage as unknown as { mockRejectedValue: (v: unknown) => void })
      .mockRejectedValue(new Error("unavailable"));

    const { el, host } = await mountWindow<FilesEl>("lthn-view-files");
    await el._loadFromBackend();
    await el.updateComplete;

    expect(host.textContent).toContain("Code");
    expect(host.textContent).toContain("sow-heritage-law-v2.md");
    expect(host.textContent).toContain("312 GB");
  });

  it("empty backend response keeps fixture data visible", async () => {
    (MockedListLocations as unknown as { mockResolvedValue: (v: unknown) => void })
      .mockResolvedValue({ Value: { locations: [] } });
    (MockedListRecent as unknown as { mockResolvedValue: (v: unknown) => void })
      .mockResolvedValue({ Value: { files: [] } });
    (MockedGetDiskUsage as unknown as { mockResolvedValue: (v: unknown) => void })
      .mockResolvedValue({ Value: { free: "", total: "", used: 0 } });

    const { el, host } = await mountWindow<FilesEl>("lthn-view-files");
    await el._loadFromBackend();
    await el.updateComplete;

    expect(host.textContent).toContain("Code");
    expect(host.textContent).toContain("sow-heritage-law-v2.md");
    expect(host.textContent).toContain("312 GB");
  });

  it("fixture-only tests remain green — smoke renders five locations", async () => {
    (MockedListLocations as unknown as { mockResolvedValue: (v: unknown) => void })
      .mockResolvedValue({ Value: { locations: [] } });
    (MockedListRecent as unknown as { mockResolvedValue: (v: unknown) => void })
      .mockResolvedValue({ Value: { files: [] } });
    (MockedGetDiskUsage as unknown as { mockResolvedValue: (v: unknown) => void })
      .mockResolvedValue({ Value: { free: "", total: "", used: 0 } });

    const { host } = await mountWindow("lthn-view-files");
    const locations = host.querySelectorAll(".lthn-view-files-location");
    expect(locations.length).toBe(5);
  });
});
