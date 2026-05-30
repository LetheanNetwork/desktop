// SPDX-Licence-Identifier: EUPL-1.2
//
// Render-smoke test for the process-daemons light-DOM component. It
// starts in a loading state (loading=true) and fires a daemon-list load
// on mount; the call is caught internally. First render shows the
// loading frame.

import { describe, it, expect } from "vitest";
import { mountWindow } from "../../../test/window-fixture";

import "./process-daemons";

describe("process-daemons — render smoke", () => {
  it("mounts and shows the loading frame on first render", async () => {
    const { host } = await mountWindow("process-daemons");
    expect(host.textContent).toContain("Loading daemons…");
  });
});
