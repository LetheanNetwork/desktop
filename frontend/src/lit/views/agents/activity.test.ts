// SPDX-Licence-Identifier: EUPL-1.2
//
// Render-smoke test for lthn-view-agent-activity. Light-DOM renderChrome
// view — same chrome / embedded pattern as the IDE windows. Service
// calls from connectedCallback route through the mocked @wailsio runtime
// and are caught internally.

import { describe, it, expect } from "vitest";
import { mountWindow, isEmbedded } from "../../../test/window-fixture";

import "./activity";

describe("lthn-view-agent-activity — render smoke", () => {
  it("mounts and renders the chrome titlebar without throwing", async () => {
    const { host } = await mountWindow("lthn-view-agent-activity");
    expect(host.querySelector("header")).not.toBeNull();
    expect(host.querySelector(".lthn-window")).not.toBeNull();
  });

  it("collapses to the embedded shell when the embedded attribute is set", async () => {
    const { host } = await mountWindow("lthn-view-agent-activity", { attrs: { embedded: "" } });
    expect(isEmbedded(host)).toBe(true);
    expect(host.querySelector("header")).toBeNull();
  });
});
