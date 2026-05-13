// SPDX-Licence-Identifier: EUPL-1.2

import { describe, it, expect } from "vitest";
import { mountWindow, expectChromeTitle, isEmbedded } from "../../test/window-fixture";
import "./settings-window";

describe("lthn-settings-window — smoke", () => {
  it("mounts with the Settings titlebar", async () => {
    const { host } = await mountWindow("lthn-settings-window");
    expectChromeTitle(host, "Settings");
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
