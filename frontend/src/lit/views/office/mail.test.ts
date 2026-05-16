// SPDX-Licence-Identifier: EUPL-1.2

import { describe, it, expect } from "vitest";
import type { LitElement } from "lit";
import { mountWindow, expectChromeTitle, findCard, isEmbedded } from "../../../test/window-fixture";
import "./mail";

describe("lthn-view-mail — smoke", () => {
  it("mounts without crashing and produces the Mail card", async () => {
    const { host } = await mountWindow("lthn-view-mail");
    expect(findCard(host)).not.toBeNull();
    expectChromeTitle(host, "Mail");
  });

  it("renders all five fixture folders in the rail", async () => {
    const { host } = await mountWindow("lthn-view-mail");
    const folders = host.querySelectorAll(".lthn-view-mail-folder");
    expect(folders.length).toBe(5);
    expect(host.textContent).toContain("Inbox");
    expect(host.textContent).toContain("Sent");
    expect(host.textContent).toContain("Drafts");
    expect(host.textContent).toContain("Archive");
    expect(host.textContent).toContain("Trash");
  });

  it("renders all five fixture thread rows", async () => {
    const { host } = await mountWindow("lthn-view-mail");
    const threads = host.querySelectorAll(".lthn-view-mail-thread");
    expect(threads.length).toBe(5);
    expect(host.textContent).toContain("Ada Penley");
    expect(host.textContent).toContain("Calliope VC");
    expect(host.textContent).toContain("r/LocalLLaMA");
  });

  it("the subtitle reports the unread count from the fixtures", async () => {
    const { host } = await mountWindow("lthn-view-mail");
    // Two unread fixtures: Ada Penley + GitHub
    expectChromeTitle(host, "Mail", "2 unread");
  });

  it("includes the Vi triage callout pattern", async () => {
    const { host } = await mountWindow("lthn-view-mail");
    const vi = host.querySelector(".lthn-view-mail-vi");
    expect(vi, "Vi callout present").not.toBeNull();
    expect(vi?.textContent).toContain("Vi");
    expect(vi?.textContent).toContain("Suggested reply");
  });

  it("footer cites IMAP via host.uk.com and on-device Vi triage", async () => {
    const { host } = await mountWindow("lthn-view-mail");
    expect(host.textContent).toContain("IMAP via host.uk.com");
    expect(host.textContent).toContain("Vi triage on-device only");
    expect(host.textContent).toContain("pkg/mail pending");
  });
});

describe("lthn-view-mail — thread selection", () => {
  type MailEl = LitElement & {
    selectedIdx: number;
    updateComplete: Promise<boolean>;
  };

  it("selectedIdx=0 by default — first thread is the reading-pane subject", async () => {
    const { host } = await mountWindow("lthn-view-mail");
    const reading = host.querySelector(".lthn-view-mail-reading");
    expect(reading?.textContent).toContain("Re: SOW v2 — DPO feedback");
    expect(reading?.textContent).toContain("Ada Penley");
  });

  it("changing selectedIdx swaps which thread fills the reading pane", async () => {
    const { el, host } = await mountWindow<MailEl>("lthn-view-mail");
    el.selectedIdx = 2;
    await el.updateComplete;
    const reading = host.querySelector(".lthn-view-mail-reading");
    expect(reading?.textContent).toContain("Pitch follow-up");
    expect(reading?.textContent).toContain("Calliope VC");
  });

  it("clamps an out-of-range selectedIdx defensively (no throw)", async () => {
    const { el, host } = await mountWindow<MailEl>("lthn-view-mail");
    el.selectedIdx = 999;
    await el.updateComplete;
    // Clamps to last fixture: "r/LocalLLaMA"
    const reading = host.querySelector(".lthn-view-mail-reading");
    expect(reading?.textContent).toContain("r/LocalLLaMA");
  });
});

describe("lthn-view-mail — embedded mode", () => {
  it("embedded attribute collapses the chrome frame", async () => {
    const { host } = await mountWindow("lthn-view-mail", { attrs: { embedded: "" } });
    expect(isEmbedded(host)).toBe(true);
    expect(host.querySelector("header")).toBeNull();
  });

  it("embedded mode still renders the three-pane layout", async () => {
    const { host } = await mountWindow("lthn-view-mail", { attrs: { embedded: "" } });
    expect(host.querySelector(".lthn-view-mail-folders")).not.toBeNull();
    expect(host.querySelector(".lthn-view-mail-threads")).not.toBeNull();
    expect(host.querySelector(".lthn-view-mail-reading")).not.toBeNull();
  });
});
