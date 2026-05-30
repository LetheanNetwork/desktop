// SPDX-Licence-Identifier: EUPL-1.2
//
// Render-smoke test for the process-panel light-DOM component. It hosts
// the daemon / process / output / runner tabs and a WebSocket event
// stream indicator. API + WS calls fired on mount are caught internally
// (no wsUrl by default → no socket), so first render is independent of
// the backend.

import { describe, it, expect } from "vitest";
import { mountWindow } from "../../../test/window-fixture";

import "./process-panel";

describe("process-panel — render smoke", () => {
  it("mounts and renders the panel root + tab strip without throwing", async () => {
    const { host } = await mountWindow("process-panel");
    expect(host.querySelector(".pp-root"), "panel root present").not.toBeNull();
    expect(host.querySelector(".pp-tabs"), "tab strip present").not.toBeNull();
  });

  it("shows the 'No event stream' label when no ws-url is set", async () => {
    const { host } = await mountWindow("process-panel");
    expect(host.textContent).toContain("No event stream");
  });
});
