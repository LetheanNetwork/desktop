// SPDX-Licence-Identifier: EUPL-1.2

import { describe, it, expect } from "vitest";
import { mountWindow, expectChromeTitle, isEmbedded } from "../../test/window-fixture";
import "./tools-window";

describe("lthn-tools-window — smoke", () => {
  it("mounts with the Tools · MCP titlebar", async () => {
    const { host } = await mountWindow("lthn-tools-window");
    expectChromeTitle(host, "Tools");
    expect(host.querySelector("header")).not.toBeNull();
  });
});

describe("lthn-tools-window — two-shell", () => {
  it("embedded mode collapses the chrome", async () => {
    const { host } = await mountWindow("lthn-tools-window", { attrs: { embedded: "" } });
    expect(isEmbedded(host)).toBe(true);
    expect(host.querySelector("header")).toBeNull();
  });
});
