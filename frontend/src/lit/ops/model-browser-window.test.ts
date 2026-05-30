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

  // The HF search surface renders its facet controls without a live
  // network result: a Base-only toggle + a Sort select. (Live result rows
  // — and their per-row Download buttons, which call the Lemma.Download
  // verified-fetch binding — need a network fetch the test env lacks.)
  it("renders the HF search facets — base-only toggle + sort select", async () => {
    const { host } = await mountWindow("lthn-model-browser-window");
    expect(host.textContent).toContain("Base models only");
    expect(host.querySelector("lthn-toggle")).not.toBeNull();
    expect(host.querySelector("select")).not.toBeNull();
  });
});

describe("lthn-model-browser-window — two-shell", () => {
  it("embedded mode collapses the chrome", async () => {
    const { host } = await mountWindow("lthn-model-browser-window", { attrs: { embedded: "" } });
    expect(isEmbedded(host)).toBe(true);
    expect(host.querySelector("header")).toBeNull();
  });
});
