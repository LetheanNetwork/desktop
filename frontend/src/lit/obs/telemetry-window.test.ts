// SPDX-Licence-Identifier: EUPL-1.2

import { describe, it, expect } from "vitest";
import { mountWindow, expectChromeTitle, isEmbedded } from "../../test/window-fixture";
import "./telemetry-window";

describe("lthn-telemetry-window — smoke", () => {
  it("mounts with the Live telemetry titlebar", async () => {
    const { host } = await mountWindow("lthn-telemetry-window");
    expectChromeTitle(host, "Live telemetry");
    expect(host.querySelector("header")).not.toBeNull();
  });

  it("fullscreen prop reflects on the host element", async () => {
    const { el } = await mountWindow<HTMLElement & { fullscreen: boolean; updateComplete: Promise<boolean> }>(
      "lthn-telemetry-window",
      { props: { fullscreen: true } },
    );
    await el.updateComplete;
    expect(el.fullscreen).toBe(true);
  });
});

describe("lthn-telemetry-window — two-shell", () => {
  it("embedded mode collapses the chrome", async () => {
    const { host } = await mountWindow("lthn-telemetry-window", { attrs: { embedded: "" } });
    expect(isEmbedded(host)).toBe(true);
    expect(host.querySelector("header")).toBeNull();
  });
});
