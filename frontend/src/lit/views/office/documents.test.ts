// SPDX-Licence-Identifier: EUPL-1.2

import { describe, it, expect, vi } from "vitest";
import type { LitElement } from "lit";
import { mountWindow, expectChromeTitle, findCard, isEmbedded } from "../../../test/window-fixture";

// Mock the @desktop/office/documents/service module so the Wails binding
// import in documents.ts resolves cleanly in the test environment.
// The binding is gitignored; in tests we control List() via this mock.
vi.mock("@desktop/office/documents/service", () => ({
  List: vi.fn().mockResolvedValue({ Value: null }),
}));

import "./documents";

describe("lthn-view-documents — smoke", () => {
  it("mounts without crashing and produces the Documents card", async () => {
    const { host } = await mountWindow("lthn-view-documents");
    expect(findCard(host)).not.toBeNull();
    expectChromeTitle(host, "Documents");
  });

  it("renders the fixture document titles", async () => {
    const { host } = await mountWindow("lthn-view-documents");
    expect(host.textContent).toContain("v0.2 release notes");
    expect(host.textContent).toContain("Sovereign-compute manifesto");
    expect(host.textContent).toContain("Q2 board pack");
    expect(host.textContent).toContain("Runbook · DNS rotation");
  });

  it("renders six fixture rows by default (no filter applied)", async () => {
    const { host } = await mountWindow("lthn-view-documents");
    const rows = host.querySelectorAll(".lthn-view-documents-row");
    expect(rows.length).toBe(6);
  });

  it("shows the pkg/office/documents footer hint", async () => {
    const { host } = await mountWindow("lthn-view-documents");
    expect(host.textContent).toContain("local markdown");
    expect(host.textContent).toContain("pkg/office/documents");
  });
});

describe("lthn-view-documents — filter control", () => {
  type DocsEl = LitElement & {
    filter: "draft" | "ready" | "final" | "live" | null;
    docs: { title: string; state: string; author: string; edited: string; size: string }[];
    _setFilter: (state: "draft" | "ready" | "final" | "live") => void;
    updateComplete: Promise<boolean>;
  };

  it("setting filter='draft' narrows the visible rows to drafts only", async () => {
    const { el, host } = await mountWindow<DocsEl>("lthn-view-documents");
    el._setFilter("draft");
    await el.updateComplete;
    const rows = host.querySelectorAll(".lthn-view-documents-row");
    // Fixture has 2 drafts: "v0.2 release notes" and "SOW · Heritage Law LLP v2"
    expect(rows.length).toBe(2);
    expect(host.textContent).toContain("v0.2 release notes");
    expect(host.textContent).toContain("SOW · Heritage Law LLP v2");
  });

  it("clicking the same filter twice clears it", async () => {
    const { el, host } = await mountWindow<DocsEl>("lthn-view-documents");
    el._setFilter("final");
    await el.updateComplete;
    expect(el.filter).toBe("final");
    el._setFilter("final");
    await el.updateComplete;
    expect(el.filter).toBeNull();
    const rows = host.querySelectorAll(".lthn-view-documents-row");
    expect(rows.length).toBe(6);
  });

  it("ready filter returns the Sovereign-compute manifesto row", async () => {
    const { el, host } = await mountWindow<DocsEl>("lthn-view-documents");
    el._setFilter("ready");
    await el.updateComplete;
    const rows = host.querySelectorAll(".lthn-view-documents-row");
    expect(rows.length).toBe(1);
    expect(host.textContent).toContain("Sovereign-compute manifesto");
  });
});

describe("lthn-view-documents — embedded mode", () => {
  it("embedded attribute collapses the chrome frame", async () => {
    const { host } = await mountWindow("lthn-view-documents", { attrs: { embedded: "" } });
    expect(isEmbedded(host)).toBe(true);
    expect(host.querySelector("header")).toBeNull();
  });

  it("embedded mode still renders the document list body", async () => {
    const { host } = await mountWindow("lthn-view-documents", { attrs: { embedded: "" } });
    expect(host.querySelector(".lthn-view-documents")).not.toBeNull();
    const rows = host.querySelectorAll(".lthn-view-documents-row");
    expect(rows.length).toBe(6);
  });
});
