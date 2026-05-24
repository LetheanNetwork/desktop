// SPDX-Licence-Identifier: EUPL-1.2

import { describe, it, expect, beforeEach } from "vitest";

import "./hero";
import { DEFAULT_LINE1, DEFAULT_LINE2, DEFAULT_SUBLINE } from "./hero";

interface MountedEl extends HTMLElement {
  line1:    string;
  line2:    string;
  subline:  string;
  accent:   boolean;
  embedded: boolean;
  updateComplete: Promise<boolean>;
}

async function mount(attrs: Record<string, string | boolean> = {}): Promise<MountedEl> {
  const el = document.createElement("lthn-marketing-hero") as MountedEl;
  for (const [k, v] of Object.entries(attrs)) {
    if (typeof v === "boolean") {
      if (v) el.setAttribute(k, "");
    } else {
      el.setAttribute(k, v);
    }
  }
  document.body.appendChild(el);
  await el.updateComplete;
  return el;
}

describe("<lthn-marketing-hero>", () => {
  beforeEach(() => {
    document.body.innerHTML = "";
  });

  it("renders RFC default copy when no attributes set (Good)", async () => {
    const el = await mount();
    expect(el.textContent).toContain(DEFAULT_LINE1);
    expect(el.textContent).toContain(DEFAULT_LINE2);
    expect(el.textContent).toContain(DEFAULT_SUBLINE);
  });

  it("accepts custom copy via attributes (Good)", async () => {
    const el = await mount({
      line1:   "Custom thesis line.",
      line2:   "Custom loop summary.",
      subline: "Custom amplifying sentence about the offering.",
    });
    expect(el.textContent).toContain("Custom thesis line.");
    expect(el.textContent).toContain("Custom loop summary.");
    expect(el.textContent).toContain("Custom amplifying sentence about the offering.");
    expect(el.textContent).not.toContain(DEFAULT_LINE1);
  });

  it("applies accent class to line 2 by default (Good)", async () => {
    const el = await mount();
    const line2 = el.querySelector(".hero-line-2");
    expect(line2).not.toBeNull();
    expect(line2?.classList.contains("accent")).toBe(true);
  });

  it("default copy contains no exclamation marks (Good)", async () => {
    // Per RFC §12 — the SaaS-register "!" is banned everywhere on the
    // marketing surface. Guard the defaults so they can't drift.
    const allDefaults = `${DEFAULT_LINE1} ${DEFAULT_LINE2} ${DEFAULT_SUBLINE}`;
    expect(allDefaults).not.toContain("!");
  });

  it("default copy contains no banned vocabulary (Bad)", async () => {
    // Per RFC §12 — these six words are banned everywhere on the lthn.ai
    // marketing surface. Guard the defaults so a future edit can't slip
    // one in.
    const banned = [
      "leverage", "utilise", "seamless", "robust",
      "game-changing", "best-in-class",
    ];
    const allDefaults = `${DEFAULT_LINE1} ${DEFAULT_LINE2} ${DEFAULT_SUBLINE}`.toLowerCase();
    for (const word of banned) {
      expect(allDefaults).not.toContain(word);
    }
  });

  it("embedded attribute skips chrome wrapper (Good)", async () => {
    const el = await mount({ embedded: true });
    expect(el.hasAttribute("embedded")).toBe(true);
    // When embedded, render() returns the inner stack only — no <section>
    // banner wrapper. The stack itself is always present.
    expect(el.querySelector(".hero-stack")).not.toBeNull();
    expect(el.querySelector(".marketing-hero")).toBeNull();
  });

  it("standalone (non-embedded) renders the marketing-hero section wrapper (Good)", async () => {
    const el = await mount();
    expect(el.querySelector(".marketing-hero")).not.toBeNull();
    expect(el.querySelector(".hero-stack")).not.toBeNull();
  });
});
