// SPDX-Licence-Identifier: EUPL-1.2

import { describe, it, expect } from "vitest";
import { mountWindow, expectChromeTitle, isEmbedded } from "../../test/window-fixture";
import "./model-browser-window";

describe("lthn-model-browser-window — smoke", () => {
  it("mounts with the Models titlebar", async () => {
    const { host } = await mountWindow("lthn-model-browser-window");
    expectChromeTitle(host, "Models");
    expect(host.querySelector("header")).not.toBeNull();
  });

  it("renders the Local rail header", async () => {
    const { host } = await mountWindow("lthn-model-browser-window");
    expect(host.textContent).toContain("Local");
  });

  it("renders the HuggingFace search results section", async () => {
    const { host } = await mountWindow("lthn-model-browser-window");
    expect(host.textContent).toContain("huggingface.co");
  });

  it("renders the right detail rail with selected model meta", async () => {
    const { host } = await mountWindow("lthn-model-browser-window");
    expect(host.textContent).toContain("Selected");
    // Default selected model carries a recognizable family + arch label.
    expect(host.textContent).toContain("gemma-4-e2b");
  });

  it("footer surfaces the model directory hint", async () => {
    const { host } = await mountWindow("lthn-model-browser-window");
    expect(host.textContent).toContain("~/.lthn/models/");
  });

  // Mantis #1682 Shape B — Hugging Face fixture rows expose a disabled
  // "Coming soon" button + a natural-language defer message instead of
  // a live Download button. Until pkg/downloader can verify the bytes
  // it receives match the file Hugging Face listed (the F-2 close on
  // #1676), no path from this surface invokes dl.Download.
  it("Hugging Face fixture rows render disabled Download buttons", async () => {
    const { host } = await mountWindow("lthn-model-browser-window");
    expect(host.textContent).toContain("Coming soon");
    // Defer message lives above the result list, in plain language —
    // no "digest" / "Wails" / "pkg/downloader" jargon per the
    // ui-text-offer-not-implementation discipline.
    expect(host.textContent).toContain("Browsing is a preview");
    // Every Coming-soon button is the disabled fixture-row trigger;
    // none of them dispatch the F-2 dl.Download vector.
    const buttons = host.querySelectorAll('lthn-btn[disabled]');
    const comingSoon = [...buttons].filter(b => (b.textContent || "").includes("Coming soon"));
    expect(comingSoon.length).toBeGreaterThan(0);
  });
});

describe("lthn-model-browser-window — two-shell", () => {
  it("embedded mode collapses the chrome", async () => {
    const { host } = await mountWindow("lthn-model-browser-window", { attrs: { embedded: "" } });
    expect(isEmbedded(host)).toBe(true);
    expect(host.querySelector("header")).toBeNull();
  });
});
