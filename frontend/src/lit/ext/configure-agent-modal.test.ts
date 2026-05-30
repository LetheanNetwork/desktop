// SPDX-Licence-Identifier: EUPL-1.2
//
// Render-smoke test for lthn-configure-agent-modal. Unlike the window
// components this is a light-DOM modal that renders `nothing` while
// closed and a fixed-position backdrop dialog when `open` is set. Smoke
// coverage: closed mount renders no dialog (no throw); open mount paints
// the backdrop overlay.

import { describe, it, expect } from "vitest";
import { mountWindow } from "../../test/window-fixture";

import "./configure-agent-modal";

// The modal's outer backdrop carries z-index:9999 inline — a stable
// marker for "the dialog is painted" that doesn't depend on copy.
function backdrop(host: HTMLElement): HTMLElement | null {
  return Array.from(host.querySelectorAll<HTMLElement>("div"))
    .find((el) => (el.getAttribute("style") || "").includes("z-index:9999")) ?? null;
}

describe("lthn-configure-agent-modal — render smoke", () => {
  it("renders nothing while closed (default) without throwing", async () => {
    const { host } = await mountWindow("lthn-configure-agent-modal");
    expect(backdrop(host), "closed modal paints no backdrop").toBeNull();
  });

  it("paints the backdrop dialog when open", async () => {
    const { host } = await mountWindow("lthn-configure-agent-modal", { props: { open: true } });
    expect(backdrop(host), "open modal paints the fixed backdrop").not.toBeNull();
  });

  it("reactively shows then hides the dialog as `open` toggles", async () => {
    const { el, host } = await mountWindow<HTMLElement & { open: boolean; updateComplete: Promise<boolean> }>(
      "lthn-configure-agent-modal",
    );
    expect(backdrop(host)).toBeNull();
    el.open = true;
    await el.updateComplete;
    expect(backdrop(host)).not.toBeNull();
    el.open = false;
    await el.updateComplete;
    expect(backdrop(host)).toBeNull();
  });
});
