// SPDX-Licence-Identifier: EUPL-1.2

import { describe, it, expect } from "vitest";
import { mountWindow, expectChromeTitle, isEmbedded } from "../../test/window-fixture";
import "./fleet-window";

describe("lthn-fleet-window — smoke", () => {
  it("mounts with the Fleet titlebar", async () => {
    const { host } = await mountWindow("lthn-fleet-window");
    expectChromeTitle(host, "Fleet");
  });
});

describe("lthn-fleet-window — two-shell", () => {
  it("embedded mode collapses the chrome", async () => {
    const { host } = await mountWindow("lthn-fleet-window", { attrs: { embedded: "" } });
    expect(isEmbedded(host)).toBe(true);
    expect(host.querySelector("header")).toBeNull();
  });
});
