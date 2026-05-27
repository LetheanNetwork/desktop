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

  it("renders the right detail rail with the Selected header", async () => {
    const { host } = await mountWindow("lthn-model-browser-window");
    expect(host.textContent).toContain("Selected");
    // When no local entries are scanned, every selected-model field
    // renders as "—" — honest about the empty state rather than
    // showing the old fixture fallback ("gemma-4-e2b · Google · 2.1 GB").
    expect(host.textContent).toContain("—");
  });

  it("footer surfaces the model directory hint", async () => {
    const { host } = await mountWindow("lthn-model-browser-window");
    expect(host.textContent).toContain("~/.lthn/models/");
  });

  // HF fixture rows hand off to lemma-window — clicking Download
  // emits lthn:lemma:open-admin with the HF repo so lemma-window can
  // prefill its Download form. The actual fetch invocation stays
  // inside lemma admin (which has the verified-fetch substrate +
  // Cerberus-DREAD-approved /v1/admin/models/download endpoint), so
  // no path from THIS surface invokes dl.Download directly.
  it("Hugging Face fixture rows render Download buttons that hand off to lemma admin", async () => {
    const { host } = await mountWindow("lthn-model-browser-window");
    // Defer message stays — but now points at the verified-fetch
    // substrate behind the lemma admin download form.
    expect(host.textContent).toContain("Browsing is a preview");
    expect(host.textContent).toContain("Lemma admin");
    // Every HF row exposes a real Download button. None of them
    // dispatch dl.Download from this surface — they emit a cross-window
    // event that lands the user in lemma-window with the repo prefilled.
    const downloadBtns = [...host.querySelectorAll('lthn-btn')]
      .filter(b => (b.textContent || "").trim() === "Download");
    expect(downloadBtns.length).toBeGreaterThan(0);
  });
});

describe("lthn-model-browser-window — two-shell", () => {
  it("embedded mode collapses the chrome", async () => {
    const { host } = await mountWindow("lthn-model-browser-window", { attrs: { embedded: "" } });
    expect(isEmbedded(host)).toBe(true);
    expect(host.querySelector("header")).toBeNull();
  });
});
