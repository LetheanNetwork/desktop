// SPDX-Licence-Identifier: EUPL-1.2
//
// Render-smoke test for the process-list light-DOM component. The list
// load fires synchronously from connectedCallback (loading=true) and
// awaits the ProcessApi call; that call routes through the mocked
// @wailsio runtime and is caught internally. The deterministic first
// render is therefore the loading frame.

import { describe, it, expect } from "vitest";
import { mountWindow } from "../../../test/window-fixture";

import "./process-list";

describe("process-list — render smoke", () => {
  it("mounts without throwing", async () => {
    const { el } = await mountWindow("process-list");
    expect(el).toBeInstanceOf(HTMLElement);
  });

  it("shows the loading frame on first render while the load is in flight", async () => {
    const { host } = await mountWindow("process-list");
    expect(host.textContent).toContain("Loading processes…");
  });
});
