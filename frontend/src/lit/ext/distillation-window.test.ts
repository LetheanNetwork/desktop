// SPDX-Licence-Identifier: EUPL-1.2

import { describe, it, expect } from "vitest";
import { mountWindow, expectChromeTitle, isEmbedded } from "../../test/window-fixture";
import "./distillation-window";

describe("lthn-distillation-window — smoke", () => {
  it("mounts with the Fine-tune titlebar", async () => {
    const { host } = await mountWindow("lthn-distillation-window");
    expectChromeTitle(host, "Fine-tune");
    expect(host.querySelector("header")).not.toBeNull();
  });
});

describe("lthn-distillation-window — two-shell", () => {
  it("embedded mode collapses the chrome", async () => {
    const { host } = await mountWindow("lthn-distillation-window", { attrs: { embedded: "" } });
    expect(isEmbedded(host)).toBe(true);
    expect(host.querySelector("header")).toBeNull();
  });
});
