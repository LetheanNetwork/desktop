// SPDX-Licence-Identifier: EUPL-1.2
//
// Render-smoke test for the process-runner light-DOM component. With no
// `result` it renders the empty-state hint; given a RunAllResult it
// paints the aggregate summary row.

import { describe, it, expect } from "vitest";
import { mountWindow } from "../../../test/window-fixture";

import "./process-runner";

describe("process-runner — render smoke", () => {
  it("renders the empty-state hint when no result is set", async () => {
    const { host } = await mountWindow("process-runner");
    expect(host.textContent).toContain("No pipeline results to display.");
  });

  it("renders the result summary when a RunAllResult is supplied", async () => {
    const { host } = await mountWindow("process-runner", {
      props: {
        result: {
          results: [{ name: "build", exitCode: 0, duration: 12, output: "", skipped: false, passed: true }],
          duration: 12,
          passed: 1,
          failed: 0,
          skipped: 0,
          success: true,
        },
      },
    });
    // The summary row renders the per-spec name from the results array.
    expect(host.textContent).toContain("build");
  });
});
