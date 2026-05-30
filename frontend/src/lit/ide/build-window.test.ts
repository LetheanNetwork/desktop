// SPDX-Licence-Identifier: EUPL-1.2
//
// Render-smoke test for lthn-build-window. Mounts the element via the
// shared window fixture, awaits first render, and asserts it paints the
// renderChrome titlebar without throwing. Service calls fired from the
// async connectedCallback route through the mocked @wailsio/runtime and
// are caught internally (unwrap → fallback / demand → caught), so the
// first render is service-independent.

import { describe, it, expect } from "vitest";
import { mountWindow, isEmbedded } from "../../test/window-fixture";

import "./build-window";

describe("lthn-build-window — render smoke", () => {
  it("mounts and renders the chrome titlebar without throwing", async () => {
    const { host } = await mountWindow("lthn-build-window");
    expect(host.querySelector("header"), "non-embedded mount paints a titlebar").not.toBeNull();
    expect(host.querySelector(".lthn-window"), "outer window card present").not.toBeNull();
  });

  it("collapses to the embedded shell when the embedded attribute is set", async () => {
    const { host } = await mountWindow("lthn-build-window", { attrs: { embedded: "" } });
    expect(isEmbedded(host)).toBe(true);
    expect(host.querySelector("header"), "embedded mode drops the titlebar").toBeNull();
  });
});
