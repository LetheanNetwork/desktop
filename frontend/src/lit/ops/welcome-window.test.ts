// SPDX-Licence-Identifier: EUPL-1.2

import { describe, it, expect } from "vitest";
import type { LitElement } from "lit";
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
    const { el, host } = await mountWindow<LitElement & { step: number; updateComplete: Promise<boolean> }>(
      "lthn-welcome-window",
      { props: { step: 2 } },
    );
    expect(host.textContent).toContain("Pick a model to start");
    el.step = 3;
    await el.updateComplete;
    expect(host.textContent).toContain("Want to wire it into your tools");
  });

  it("step 4 — renders the Menu Behaviours tour content", async () => {
    const { host } = await mountWindow<LitElement & { step: number }>(
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

describe("lthn-welcome-window — Choose folder wire", () => {
  type WelcomeEl = LitElement & {
    step: number;
    modelsDir: string;
    _chooseModelsFolder: () => Promise<void>;
    _adoptModelsFolder: (path: string) => Promise<void>;
  };

  it("renders the Choose folder button on step 1", async () => {
    const { host } = await mountWindow<WelcomeEl>("lthn-welcome-window", { props: { step: 1 } });
    const btn = host.querySelector(".lthn-welcome-choose-folder");
    expect(btn, "Choose folder button present on step 1").not.toBeNull();
    expect(btn?.textContent?.trim()).toContain("Choose folder");
  });

  it("_adoptModelsFolder('') is a silent no-op", async () => {
    const { el } = await mountWindow<WelcomeEl>("lthn-welcome-window");
    const initial = el.modelsDir;
    let threw: unknown = null;
    try { await el._adoptModelsFolder(""); }
    catch (e) { threw = e; }
    expect(threw, "empty path must not throw").toBeNull();
    expect(el.modelsDir, "modelsDir unchanged for empty path").toBe(initial);
  });

  it("renders the Reset to default button on step 1", async () => {
    const { host } = await mountWindow<WelcomeEl>("lthn-welcome-window", { props: { step: 1 } });
    const btn = host.querySelector(".lthn-welcome-reset-folder");
    expect(btn, "reset button present").not.toBeNull();
    expect(btn?.textContent?.trim()).toBe("Reset to default");
  });
});
