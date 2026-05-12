// SPDX-Licence-Identifier: EUPL-1.2
//
// Canonical Lit test for chrome.ts — the renderChrome() template helper
// and the primitive custom elements it composes. happy-dom + Lit's
// render() give us a real DOM tree we can query with the standard
// selectors. No browser needed.

import { describe, it, expect, beforeEach } from "vitest";
import { render } from "lit";
import { renderChrome } from "./chrome";

// Re-import the file for its side-effects so the custom elements
// (lthn-glyph, lthn-btn, lthn-status-dot, …) register on the
// happy-dom CustomElementRegistry. Idempotent — subsequent imports
// are no-ops.
import "./chrome";

let host: HTMLDivElement;

beforeEach(() => {
  host = document.createElement("div");
  document.body.appendChild(host);
});

describe("renderChrome — non-embedded branch", () => {
  it("paints the titlebar with title + subtitle when provided", () => {
    render(renderChrome({
      title: "lthn",
      subtitle: "local · ready",
      body: "body content",
    }), host);

    expect(host.textContent).toContain("lthn");
    expect(host.textContent).toContain("local · ready");
    expect(host.textContent).toContain("body content");
  });

  it("omits the subtitle dot-separator when subtitle is missing", () => {
    render(renderChrome({ title: "no-subtitle" }), host);
    // The `·` glyph only appears when subtitle renders; without
    // a subtitle the title row should not carry it.
    const titleStrip = host.querySelector("header");
    expect(titleStrip).not.toBeNull();
    expect(titleStrip?.textContent).not.toContain("·");
  });

  it("renders the traffic-lights primitive", () => {
    render(renderChrome({ title: "x" }), host);
    expect(host.querySelector("lthn-traffic-lights")).not.toBeNull();
  });

  it("renders the actions slot in the titlebar when provided", () => {
    // The actions slot was added so the tray popover can host a cog +
    // screen icon in the titlebar right. Verify the slotted content
    // ends up in the actions container with --wails-draggable: no-drag.
    render(renderChrome({
      title: "x",
      actions: document.createElement("button"),
    }), host);

    const header = host.querySelector("header");
    expect(header).not.toBeNull();
    const actionsContainer = Array.from(header!.querySelectorAll("div"))
      .find((el) => el.style.getPropertyValue("--wails-draggable") === "no-drag");
    expect(actionsContainer, "actions slot should opt out of drag").toBeDefined();
  });

  it("renders the toolbar strip when toolbar content is provided", () => {
    render(renderChrome({ title: "x", toolbar: "TOOLBAR-CONTENT" }), host);
    expect(host.textContent).toContain("TOOLBAR-CONTENT");
  });

  it("renders the footer strip when footer content is provided", () => {
    render(renderChrome({ title: "x", footer: "FOOTER-CONTENT" }), host);
    expect(host.textContent).toContain("FOOTER-CONTENT");
  });

  it("paints the outer lthn-window card with drag enabled", () => {
    render(renderChrome({ title: "x" }), host);
    const card = host.querySelector(".lthn-window");
    expect(card).not.toBeNull();
    // happy-dom doesn't expose custom properties through the
    // CSSStyleDeclaration API the same way real browsers do, so check
    // the raw style attribute text — the inline declaration is what
    // Wails3's drag handler actually reads via getComputedStyle.
    const styleAttr = (card as HTMLElement).getAttribute("style") || "";
    expect(styleAttr).toContain("--wails-draggable: drag");
  });
});

describe("renderChrome — embedded branch", () => {
  it("returns a flat body container when embedded=true", () => {
    render(renderChrome({
      title: "x",
      subtitle: "shell child",
      body: "EMBEDDED-BODY",
      embedded: true,
    }), host);

    // Embedded mode: no titlebar, no rounded card frame, no traffic
    // lights — the parent shell already paints those. The body
    // content should still be present.
    expect(host.textContent).toContain("EMBEDDED-BODY");
    expect(host.querySelector("lthn-traffic-lights")).toBeNull();
    expect(host.querySelector("header")).toBeNull();
  });

  it("still ships the toolbar in embedded mode (window-local concern)", () => {
    render(renderChrome({
      title: "x",
      embedded: true,
      toolbar: "EMBED-TOOLBAR",
      body: "body",
    }), host);
    expect(host.textContent).toContain("EMBED-TOOLBAR");
  });

  it("uses the embedded class marker on the outer container", () => {
    render(renderChrome({ title: "x", embedded: true }), host);
    expect(host.querySelector(".lthn-window--embedded")).not.toBeNull();
  });
});

describe("Lit primitives — custom elements register", () => {
  it("lthn-traffic-lights is defined", () => {
    expect(customElements.get("lthn-traffic-lights")).toBeDefined();
  });
  it("lthn-glyph is defined", () => {
    expect(customElements.get("lthn-glyph")).toBeDefined();
  });
  it("lthn-btn is defined", () => {
    expect(customElements.get("lthn-btn")).toBeDefined();
  });
  it("lthn-status-dot is defined", () => {
    expect(customElements.get("lthn-status-dot")).toBeDefined();
  });
  it("lthn-state-pill is defined", () => {
    expect(customElements.get("lthn-state-pill")).toBeDefined();
  });
  it("lthn-label is defined", () => {
    expect(customElements.get("lthn-label")).toBeDefined();
  });
  it("lthn-rail-row is defined", () => {
    expect(customElements.get("lthn-rail-row")).toBeDefined();
  });
  it("lthn-toggle is defined", () => {
    expect(customElements.get("lthn-toggle")).toBeDefined();
  });
  it("lthn-sparkline is defined", () => {
    expect(customElements.get("lthn-sparkline")).toBeDefined();
  });
});
