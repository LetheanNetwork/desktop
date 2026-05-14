// SPDX-Licence-Identifier: EUPL-1.2

import { describe, it, expect } from "vitest";
import { mountWindow, expectChromeTitle, isEmbedded } from "../../test/window-fixture";
import "./welcome-window";

describe("lthn-welcome-window — smoke", () => {
  it("mounts with the Welcome titlebar", async () => {
    const { host } = await mountWindow("lthn-welcome-window");
    expectChromeTitle(host, "Welcome to lthn");
    expect(host.querySelector("header")).not.toBeNull();
  });

  it("renders the steps rail with all four wizard steps", async () => {
    const { host } = await mountWindow("lthn-welcome-window");
    expect(host.textContent).toContain("Model directory");
    expect(host.textContent).toContain("First model");
    expect(host.textContent).toContain("Connect");
    expect(host.textContent).toContain("Menu tour");
  });

  it("step 1 by default — shows the model directory question", async () => {
    const { host } = await mountWindow("lthn-welcome-window");
    expect(host.textContent).toContain("Where shall we keep your models");
  });

  it("step prop drives which body section renders", async () => {
    const { el, host } = await mountWindow<HTMLElement & { step: number; updateComplete: Promise<boolean> }>(
      "lthn-welcome-window",
      { props: { step: 2 } },
    );
    expect(host.textContent).toContain("Pick a model to start");
    el.step = 3;
    await el.updateComplete;
    expect(host.textContent).toContain("Want to wire it into your tools");
  });

  it("step 4 — renders the Menu Behaviours tour content", async () => {
    const { host } = await mountWindow<HTMLElement & { step: number }>(
      "lthn-welcome-window",
      { props: { step: 4 } },
    );
    expect(host.textContent).toContain("Two clicks, two outcomes");
    expect(host.textContent).toContain("click → switch here");
    expect(host.textContent).toContain("click → new window");
    expect(host.textContent).toContain("⌘ + click works anywhere on the row");
  });
});

describe("lthn-welcome-window — two-shell", () => {
  it("embedded mode collapses the chrome", async () => {
    const { host } = await mountWindow("lthn-welcome-window", { attrs: { embedded: "" } });
    expect(isEmbedded(host)).toBe(true);
    expect(host.querySelector("header")).toBeNull();
  });
});
