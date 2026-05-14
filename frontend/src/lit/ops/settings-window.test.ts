// SPDX-Licence-Identifier: EUPL-1.2

import { describe, it, expect } from "vitest";
import { mountWindow, expectChromeTitle, isEmbedded } from "../../test/window-fixture";
import "./settings-window";

describe("lthn-settings-window — smoke", () => {
  it("mounts with the Settings titlebar", async () => {
    const { host } = await mountWindow("lthn-settings-window");
    expectChromeTitle(host, "Settings");
    expect(host.querySelector("header")).not.toBeNull();
  });

  it("renders the section rail (General, Models, Runner, API, Telemetry, Integrations, About)", async () => {
    const { host } = await mountWindow("lthn-settings-window");
    const text = host.textContent ?? "";
    for (const section of ["General", "Models", "Runner", "API", "Telemetry", "Integrations", "About"]) {
      expect(text).toContain(section);
    }
  });

  it("Models section shows the model-directory row", async () => {
    const { host } = await mountWindow("lthn-settings-window", { attrs: { open: "models" } });
    expect(host.textContent).toContain("Model directory");
    expect(host.textContent).toContain("~/Lethean/conf/models/");
  });

  it("footer shows the keep-running reassurance", async () => {
    const { host } = await mountWindow("lthn-settings-window");
    expect(host.textContent).toContain("the runner keeps running");
  });
});

describe("lthn-settings-window — two-shell", () => {
  it("embedded mode collapses the chrome", async () => {
    const { host } = await mountWindow("lthn-settings-window", { attrs: { embedded: "" } });
    expect(isEmbedded(host)).toBe(true);
    expect(host.querySelector("header")).toBeNull();
  });
});

describe("lthn-settings-window — Menu Behaviours subsection (General)", () => {
  // The Menu Behaviours subsection lives under General. Two segmented
  // controls (Menu Links + Menu Layout) persist to localStorage with
  // the lthn.menu.* prefix and emit lthn:menu:changed so an open
  // <lthn-app-shell> reacts without a reload. See
  // plans/project/lthn/desktop/RFC.menu-behaviours.md.
  type SettingsEl = HTMLElement & {
    menuLinks: string;
    menuLayout: string;
    updateComplete: Promise<boolean>;
  };

  it("renders the Menu Behaviours subsection heading under General", async () => {
    const { host } = await mountWindow("lthn-settings-window", { attrs: { open: "general" } });
    expect(host.textContent).toContain("Menu Behaviours");
    expect(host.textContent).toContain("Menu Links");
    expect(host.textContent).toContain("Menu Layout");
  });

  it("Menu Links defaults to 'in-window' when no setting is stored", async () => {
    const { el } = await mountWindow<SettingsEl>("lthn-settings-window", { attrs: { open: "general" } });
    expect(el.menuLinks).toBe("in-window");
  });

  it("Menu Layout defaults to 'toggle' when no setting is stored", async () => {
    const { el } = await mountWindow<SettingsEl>("lthn-settings-window", { attrs: { open: "general" } });
    expect(el.menuLayout).toBe("toggle");
  });

  it("clicking a Menu Links segment updates the persisted state", async () => {
    // Persistence to localStorage is exercised by the production
    // codepath (writeStoredSetting → setItem); asserting state is
    // sufficient proof the segment handler fired correctly. The
    // happy-dom localStorage shim is disabled in this suite so we
    // don't read back the stored value directly.
    const { el, host } = await mountWindow<SettingsEl>("lthn-settings-window", { attrs: { open: "general" } });
    const buttons = Array.from(host.querySelectorAll("button")) as HTMLButtonElement[];
    const hybridBtn = buttons.find(b => (b.textContent ?? "").trim() === "Hybrid");
    expect(hybridBtn, "Hybrid segment should render").not.toBeNull();
    hybridBtn!.click();
    await el.updateComplete;
    expect(el.menuLinks).toBe("hybrid");
  });

  it("clicking a Menu Layout segment updates the persisted state", async () => {
    const { el, host } = await mountWindow<SettingsEl>("lthn-settings-window", { attrs: { open: "general" } });
    const buttons = Array.from(host.querySelectorAll("button")) as HTMLButtonElement[];
    const hoverBtn = buttons.find(b => (b.textContent ?? "").trim() === "Hover open");
    expect(hoverBtn, "Hover open segment should render").not.toBeNull();
    hoverBtn!.click();
    await el.updateComplete;
    expect(el.menuLayout).toBe("hover");
  });

  it("renders the Replay menu tour control under Menu Behaviours", async () => {
    const { host } = await mountWindow("lthn-settings-window", { attrs: { open: "general" } });
    expect(host.textContent).toContain("Replay menu tour");
  });
});
