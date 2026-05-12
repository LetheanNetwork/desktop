// SPDX-Licence-Identifier: EUPL-1.2
//
// Shared mount + assert helpers for <lthn-*-window> custom elements.
// Every window test file imports from here so the boilerplate stays
// out of the spec body and the per-window file can focus on what
// makes that window unique (its content sections + state shape).
//
// Pattern per spec:
//
//   import { describe, it, expect, beforeEach } from "vitest";
//   import { mountWindow } from "@/test/window-fixture";
//   import "../chat-window";   // side-effect: define the element
//
//   describe("lthn-chat-window", () => {
//     it("renders the chrome with the right title", async () => {
//       const { host } = await mountWindow("lthn-chat-window");
//       expect(host.textContent).toContain("lthn · chat");
//     });
//   });
//
// The fixture handles:
//   - creating a fresh host div + appending to document.body
//   - applying any prop / attribute overrides
//   - awaiting the element's first render (Lit's updateComplete)
//   - returning the host + the element handle for further assertions

import type { LitElement } from "lit";

export interface MountOptions {
  /** Initial attributes to set before connection — e.g. `{ embedded: "" }`. */
  attrs?: Record<string, string>;
  /** Property assignments applied after mount, before the render await. */
  props?: Record<string, unknown>;
}

export interface MountResult<T extends LitElement = LitElement> {
  /** The host div the element lives inside — query against this for text/DOM. */
  host: HTMLDivElement;
  /** The element instance — use for prop set + updateComplete cycles. */
  el: T;
}

/**
 * Mount a Lit custom element by tag name into a fresh host div, apply
 * the given attrs/props, and wait for the first render to complete.
 *
 * Returns `{ host, el }` so the caller can query the rendered DOM and
 * still mutate the element for state-change tests.
 */
export async function mountWindow<T extends LitElement = LitElement>(
  tag: string,
  opts: MountOptions = {},
): Promise<MountResult<T>> {
  const host = document.createElement("div");
  // Window-shaped elements expect a sized container so their 100% chrome
  // has somewhere to grow. The tokens.css global rule already handles
  // this in app builds; the test fixture sets it inline so a single test
  // file works even when tokens.css isn't loaded.
  host.style.width = "1200px";
  host.style.height = "800px";
  document.body.appendChild(host);

  const el = document.createElement(tag) as T;

  if (opts.attrs) {
    for (const [k, v] of Object.entries(opts.attrs)) {
      el.setAttribute(k, v);
    }
  }
  if (opts.props) {
    for (const [k, v] of Object.entries(opts.props)) {
      (el as unknown as Record<string, unknown>)[k] = v;
    }
  }

  host.appendChild(el);

  // Wait for the first render. Lit elements expose `updateComplete`
  // (a Promise that resolves when render() has run). Falls back to a
  // microtask flush if the element doesn't extend LitElement.
  if (typeof (el as unknown as { updateComplete?: Promise<boolean> }).updateComplete?.then === "function") {
    await (el as unknown as { updateComplete: Promise<boolean> }).updateComplete;
  } else {
    await Promise.resolve();
  }

  return { host, el };
}

/**
 * Assert the renderChrome titlebar contains the expected title + (optional)
 * subtitle. Convenience over manually querying `header` text.
 */
export function expectChromeTitle(host: HTMLElement, title: string, subtitle?: string): void {
  const header = host.querySelector("header");
  if (!header) {
    throw new Error(`mountWindow: no <header> found — chrome is missing or this is embedded mode`);
  }
  const text = header.textContent ?? "";
  if (!text.includes(title)) {
    throw new Error(`mountWindow: titlebar should contain '${title}', got: ${text}`);
  }
  if (subtitle && !text.includes(subtitle)) {
    throw new Error(`mountWindow: titlebar should contain subtitle '${subtitle}', got: ${text}`);
  }
}

/**
 * Returns the rendered `.lthn-window` outer card for the mounted
 * window, or null when the element is in embedded mode.
 */
export function findCard(host: HTMLElement): HTMLElement | null {
  return host.querySelector(".lthn-window") as HTMLElement | null;
}

/**
 * True when the window is rendering in embedded mode (no titlebar,
 * uses .lthn-window--embedded marker). Useful for asserting the two-
 * shell pattern: same component yields different chrome based on prop.
 */
export function isEmbedded(host: HTMLElement): boolean {
  return host.querySelector(".lthn-window--embedded") !== null;
}
