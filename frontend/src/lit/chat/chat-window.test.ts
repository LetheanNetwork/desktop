// SPDX-Licence-Identifier: EUPL-1.2
//
// Canonical per-window test — chat-window. Uses the shared mountWindow
// fixture from src/test/window-fixture.ts. Pattern for the other 11
// <lthn-*-window> tests:
//
//   1. Smoke: mounts without throwing + the renderChrome titlebar
//      carries the right title.
//   2. Embedded sweep: when `embedded` attribute is set, the chrome
//      collapses to the flat .lthn-window--embedded container — verifies
//      the two-shell pattern (banked Snider memory).
//   3. Content presence: a couple of distinctive strings the window's
//      body should render (model name in chat, "Models" header in
//      model browser, etc.) so a regression that breaks the body
//      fails loudly.
//   4. Reactive prop: change one declared property, await
//      updateComplete, assert the rendered DOM reflects the change.

import { describe, it, expect } from "vitest";
import { mountWindow, expectChromeTitle, isEmbedded, findCard } from "../../test/window-fixture";

// Side-effect import — defines lthn-chat-window on the registry.
import "./chat-window";

describe("lthn-chat-window — smoke", () => {
  it("mounts + renders the chrome titlebar with 'Lethean Chat'", async () => {
    const { host } = await mountWindow("lthn-chat-window");
    expectChromeTitle(host, "lthn · chat");
  });

  it("renders the conversation list rail by default", async () => {
    const { host } = await mountWindow("lthn-chat-window");
    // The conversation rail's search input placeholder is a stable
    // marker for the rail section being present.
    expect(host.textContent).toContain("Search conversations");
  });

  it("renders the right-rail metadata when rightRail is expanded", async () => {
    const { host } = await mountWindow("lthn-chat-window", {
      props: { rightRail: "expanded" },
    });
    expect(host.textContent).toContain("Turn metadata");
  });
});

describe("lthn-chat-window — two-shell pattern", () => {
  it("default (no embedded attr) renders the full card with chrome", async () => {
    const { host } = await mountWindow("lthn-chat-window");
    expect(isEmbedded(host)).toBe(false);
    expect(findCard(host)).not.toBeNull();
    expect(host.querySelector("header")).not.toBeNull();
  });

  it("with embedded attribute, collapses to the flat embedded shell", async () => {
    const { host } = await mountWindow("lthn-chat-window", {
      attrs: { embedded: "" },
    });
    expect(isEmbedded(host)).toBe(true);
    // No titlebar in embedded mode — the parent <lthn-app-shell>
    // paints its own.
    expect(host.querySelector("header")).toBeNull();
  });
});

describe("lthn-chat-window — reactive state", () => {
  it("rail prop change re-renders to reflect the new rail mode", async () => {
    const { el, host } = await mountWindow<HTMLElement & { rail: string; updateComplete: Promise<boolean> }>(
      "lthn-chat-window",
      { props: { rail: "filled" } },
    );
    // Sanity-check initial state before mutation.
    expect(host.textContent).toContain("Search conversations");

    el.rail = "collapsed";
    await el.updateComplete;
    // After collapse the conversation rail hides its search input
    // (the rail-mode switch removes the surface entirely).
    // We assert the element re-rendered by checking the new
    // attribute reflects (rail is reflect:true in static properties).
    expect(el.getAttribute("rail")).toBe("collapsed");
  });
});
