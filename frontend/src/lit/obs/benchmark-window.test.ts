// SPDX-Licence-Identifier: EUPL-1.2

import { describe, it, expect } from "vitest";
import { mountWindow, expectChromeTitle, isEmbedded } from "../../test/window-fixture";
import "./benchmark-window";

describe("lthn-benchmark-window — smoke", () => {
  it("mounts with the Benchmark titlebar", async () => {
    const { host } = await mountWindow("lthn-benchmark-window");
    expectChromeTitle(host, "Benchmark");
  });

  it("renders the results table with at least one historical run", async () => {
    const { host } = await mountWindow("lthn-benchmark-window");
    // The fixture seeds known model identifiers in the historical rows.
    expect(host.textContent).toContain("gemma-4-e2b");
  });
});

describe("lthn-benchmark-window — two-shell", () => {
  it("embedded mode collapses the chrome", async () => {
    const { host } = await mountWindow("lthn-benchmark-window", { attrs: { embedded: "" } });
    expect(isEmbedded(host)).toBe(true);
    expect(host.querySelector("header")).toBeNull();
  });
});
