// SPDX-Licence-Identifier: EUPL-1.2
//
// Render-smoke test for lthn-pair-machine-modal. Light-DOM modal —
// renders `nothing` closed, a fixed backdrop dialog when `open`. See
// configure-agent-modal.test.ts for the backdrop-marker rationale.

import { describe, it, expect } from "vitest";
import { mountWindow } from "../../test/window-fixture";

import "./pair-machine-modal";

function backdrop(host: HTMLElement): HTMLElement | null {
  return Array.from(host.querySelectorAll<HTMLElement>("div"))
    .find((el) => (el.getAttribute("style") || "").includes("z-index:9999")) ?? null;
}

describe("lthn-pair-machine-modal — render smoke", () => {
  it("renders nothing while closed without throwing", async () => {
    const { host } = await mountWindow("lthn-pair-machine-modal");
    expect(backdrop(host)).toBeNull();
  });

  it("paints the backdrop dialog with the pairing heading when open", async () => {
    const { host } = await mountWindow("lthn-pair-machine-modal", { props: { open: true } });
    expect(backdrop(host)).not.toBeNull();
    expect(host.textContent, "open modal renders the create-mode heading").toContain("Pair a machine");
  });

  it("shows the edit-mode heading when an editing target is supplied", async () => {
    const { host } = await mountWindow("lthn-pair-machine-modal", {
      props: { open: true, editing: { id: "m1", name: "bastion-1" } },
    });
    expect(host.textContent).toContain("Edit bastion-1");
  });
});
