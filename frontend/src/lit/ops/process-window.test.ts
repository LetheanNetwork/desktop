// SPDX-Licence-Identifier: EUPL-1.2
//
// Render-smoke test for lthn-process-window — the renderChrome window
// hosting the process panel. Same pattern as the IDE windows.

import { describe, it, expect } from "vitest";
import { mountWindow, isEmbedded } from "../../test/window-fixture";

import "./process-window";

describe("lthn-process-window — render smoke", () => {
  it("mounts and renders the chrome titlebar hosting the process panel", async () => {
    const { host } = await mountWindow("lthn-process-window");
    expect(host.querySelector("header")).not.toBeNull();
    expect(host.querySelector(".lthn-window")).not.toBeNull();
    expect(host.querySelector("process-panel"), "embeds the process-panel").not.toBeNull();
  });

  it("collapses to the embedded shell when the embedded attribute is set", async () => {
    const { host } = await mountWindow("lthn-process-window", { attrs: { embedded: "" } });
    expect(isEmbedded(host)).toBe(true);
    expect(host.querySelector("header")).toBeNull();
  });
});
