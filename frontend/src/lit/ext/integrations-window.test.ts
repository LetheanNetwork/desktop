// SPDX-Licence-Identifier: EUPL-1.2

import { describe, it, expect } from "vitest";
import { mountWindow, expectChromeTitle, isEmbedded } from "../../test/window-fixture";
import "./integrations-window";

describe("lthn-integrations-window — smoke", () => {
  it("mounts with the Integrations titlebar", async () => {
    const { host } = await mountWindow("lthn-integrations-window");
    expectChromeTitle(host, "Integrations");
    expect(host.querySelector("header")).not.toBeNull();
  });
});

describe("lthn-integrations-window — two-shell", () => {
  it("embedded mode collapses the chrome", async () => {
    const { host } = await mountWindow("lthn-integrations-window", { attrs: { embedded: "" } });
    expect(isEmbedded(host)).toBe(true);
    expect(host.querySelector("header")).toBeNull();
  });
});
