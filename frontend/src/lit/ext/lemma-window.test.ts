// SPDX-Licence-Identifier: EUPL-1.2
//
// Render-smoke test for lthn-lemma-window. Light-DOM renderChrome
// window — same pattern as the IDE windows. Service calls from
// connectedCallback route through the mocked @wailsio/runtime and are
// caught internally, so first render is service-independent.

import { describe, it, expect } from "vitest";
import { mountWindow, isEmbedded } from "../../test/window-fixture";

import "./lemma-window";

describe("lthn-lemma-window — render smoke", () => {
  it("mounts and renders the chrome titlebar without throwing", async () => {
    const { host } = await mountWindow("lthn-lemma-window");
    expect(host.querySelector("header")).not.toBeNull();
    expect(host.querySelector(".lthn-window")).not.toBeNull();
  });

  it("collapses to the embedded shell when the embedded attribute is set", async () => {
    const { host } = await mountWindow("lthn-lemma-window", { attrs: { embedded: "" } });
    expect(isEmbedded(host)).toBe(true);
    expect(host.querySelector("header")).toBeNull();
  });
});
