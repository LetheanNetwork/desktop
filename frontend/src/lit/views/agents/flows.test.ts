// SPDX-Licence-Identifier: EUPL-1.2
//
// Render-smoke test for lthn-view-agent-flows. See activity.test.ts for
// the pattern rationale.

import { describe, it, expect } from "vitest";
import { mountWindow, isEmbedded } from "../../../test/window-fixture";

import "./flows";

describe("lthn-view-agent-flows — render smoke", () => {
  it("mounts and renders the chrome titlebar without throwing", async () => {
    const { host } = await mountWindow("lthn-view-agent-flows");
    expect(host.querySelector("header")).not.toBeNull();
    expect(host.querySelector(".lthn-window")).not.toBeNull();
  });

  it("collapses to the embedded shell when the embedded attribute is set", async () => {
    const { host } = await mountWindow("lthn-view-agent-flows", { attrs: { embedded: "" } });
    expect(isEmbedded(host)).toBe(true);
    expect(host.querySelector("header")).toBeNull();
  });
});
