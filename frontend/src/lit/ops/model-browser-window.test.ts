// SPDX-Licence-Identifier: EUPL-1.2

import { describe, it, expect } from "vitest";
import { mountWindow, expectChromeTitle, isEmbedded } from "../../test/window-fixture";
import "./model-browser-window";

describe("lthn-model-browser-window — smoke", () => {
  it("mounts with the Models titlebar", async () => {
    const { host } = await mountWindow("lthn-model-browser-window");
    expectChromeTitle(host, "Models");
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
});

describe("lthn-model-browser-window — two-shell", () => {
  it("embedded mode collapses the chrome", async () => {
    const { host } = await mountWindow("lthn-model-browser-window", { attrs: { embedded: "" } });
    expect(isEmbedded(host)).toBe(true);
    expect(host.querySelector("header")).toBeNull();
  });
});
