// SPDX-Licence-Identifier: EUPL-1.2
//
// Render-smoke test for lthn-view-agent-scan. See activity.test.ts for
// the pattern rationale.

import { describe, it, expect } from "vitest";
import { mountWindow, isEmbedded } from "../../../test/window-fixture";

import "./scan";

describe("lthn-view-agent-scan — render smoke", () => {
  it("mounts and renders the chrome titlebar without throwing", async () => {
    const { host } = await mountWindow("lthn-view-agent-scan");
    expect(host.querySelector("header")).not.toBeNull();
    expect(host.querySelector(".lthn-window")).not.toBeNull();
  });

  it("collapses to the embedded shell when the embedded attribute is set", async () => {
    const { host } = await mountWindow("lthn-view-agent-scan", { attrs: { embedded: "" } });
    expect(isEmbedded(host)).toBe(true);
    expect(host.querySelector("header")).toBeNull();
  });
});
