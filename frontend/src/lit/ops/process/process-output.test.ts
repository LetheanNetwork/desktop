// SPDX-Licence-Identifier: EUPL-1.2
//
// Render-smoke test for the process-output light-DOM component. With no
// processId set it renders the "select a process" prompt; given a
// processId it paints the output panel header with the id + auto-scroll
// toggle.

import { describe, it, expect } from "vitest";
import { mountWindow } from "../../../test/window-fixture";

import "./process-output";

describe("process-output — render smoke", () => {
  it("renders the select-a-process prompt when no processId is set", async () => {
    const { host } = await mountWindow("process-output");
    expect(host.textContent).toContain("Select a process to view its output.");
  });

  it("renders the output panel header carrying the processId", async () => {
    const { host } = await mountWindow("process-output", { props: { processId: "proc-7" } });
    expect(host.textContent).toContain("Output: proc-7");
    expect(host.textContent).toContain("Auto-scroll");
  });
});
