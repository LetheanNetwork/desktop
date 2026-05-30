// SPDX-Licence-Identifier: EUPL-1.2
//
// Render-smoke test for lthn-provision-remote-modal. Light-DOM modal —
// renders `nothing` closed, a fixed backdrop dialog when `open`. See
// configure-agent-modal.test.ts for the backdrop-marker rationale.

import { describe, it, expect } from "vitest";
import { mountWindow } from "../../test/window-fixture";

import "./provision-remote-modal";

function backdrop(host: HTMLElement): HTMLElement | null {
  return Array.from(host.querySelectorAll<HTMLElement>("div"))
    .find((el) => (el.getAttribute("style") || "").includes("z-index:9999")) ?? null;
}

describe("lthn-provision-remote-modal — render smoke", () => {
  it("renders nothing while closed without throwing", async () => {
    const { host } = await mountWindow("lthn-provision-remote-modal");
    expect(backdrop(host)).toBeNull();
  });

  it("paints the backdrop dialog with the provision heading when open", async () => {
    const { host } = await mountWindow("lthn-provision-remote-modal", { props: { open: true } });
    expect(backdrop(host)).not.toBeNull();
    expect(host.textContent).toContain("Provision remote");
  });
});
