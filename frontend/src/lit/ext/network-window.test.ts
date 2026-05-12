// SPDX-Licence-Identifier: EUPL-1.2

import { describe, it, expect } from "vitest";
import { mountWindow, expectChromeTitle, isEmbedded } from "../../test/window-fixture";
import "./network-window";

describe("lthn-network-window — smoke", () => {
  it("mounts with the Network titlebar", async () => {
    const { host } = await mountWindow("lthn-network-window");
    expectChromeTitle(host, "Network");
  });
});

describe("lthn-network-window — two-shell", () => {
  it("embedded mode collapses the chrome", async () => {
    const { host } = await mountWindow("lthn-network-window", { attrs: { embedded: "" } });
    expect(isEmbedded(host)).toBe(true);
    expect(host.querySelector("header")).toBeNull();
  });
});
