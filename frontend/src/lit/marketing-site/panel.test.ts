// SPDX-Licence-Identifier: EUPL-1.2

import { describe, it, expect, beforeEach } from "vitest";

import "./panel";

interface MountedEl extends HTMLElement {
  title:    string;
  subtitle: string;
  accent:   boolean;
  embedded: boolean;
  updateComplete: Promise<boolean>;
}

async function mount(
  attrs: Record<string, string | boolean> = {},
  bodyHTML: string = "",
): Promise<MountedEl> {
  const el = document.createElement("lthn-marketing-panel") as MountedEl;
  for (const [k, v] of Object.entries(attrs)) {
    if (typeof v === "boolean") {
      if (v) el.setAttribute(k, "");
    } else {
      el.setAttribute(k, v);
    }
  }
  if (bodyHTML) {
    el.innerHTML = bodyHTML;
  }
  document.body.appendChild(el);
  await el.updateComplete;
  return el;
}

describe("<lthn-marketing-panel>", () => {
  beforeEach(() => {
    document.body.innerHTML = "";
  });

  it("renders title and subtitle when provided (Good)", async () => {
    const el = await mount({
      title:    "Train",
      subtitle: "Three-tier scoring during training. Real numbers, not vibes.",
    });
    expect(el.textContent).toContain("Train");
    expect(el.textContent).toContain("Three-tier scoring during training. Real numbers, not vibes.");
    expect(el.querySelector(".panel-title")?.textContent).toBe("Train");
    expect(el.querySelector(".panel-subtitle")?.textContent).toContain("Three-tier");
  });

  it("default slot renders the body content (Good)", async () => {
    const el = await mount(
      { title: "Run" },
      `<div class="curl-example">curl localhost:8080</div>
       <div class="number-strip">portable kv cache</div>`,
    );
    // Slot content should land inside the body region.
    const body = el.querySelector(".panel-body");
    expect(body).not.toBeNull();
    expect(el.querySelector(".curl-example")?.textContent).toContain("curl localhost:8080");
    expect(el.querySelector(".number-strip")?.textContent).toContain("portable kv cache");
  });

  it("accent rail renders by default (Good)", async () => {
    const el = await mount({ title: "Score" });
    expect(el.querySelector(".accent-rail")).not.toBeNull();
  });

  it("accent rail hides when accent attr explicitly false (Bad)", async () => {
    // Construct the element without the accent attr first, then flip it.
    const el = await mount({ title: "Serve" });
    el.accent = false;
    await el.updateComplete;
    expect(el.querySelector(".accent-rail")).toBeNull();
  });

  it("missing title still renders the shell (Ugly)", async () => {
    // Body-only panel for prototyping. No title means no title element,
    // but the rail still renders (when accent on) so the chrome character
    // is visible. The body slot remains usable.
    const el = await mount({}, `<p class="body-only">just the body</p>`);
    expect(el.querySelector(".panel-title")).toBeNull();
    expect(el.querySelector(".panel-subtitle")).toBeNull();
    expect(el.querySelector(".accent-rail")).not.toBeNull();
    expect(el.querySelector(".body-only")?.textContent).toBe("just the body");
  });

  it("embedded attribute skips the outer chrome wrapper (Good)", async () => {
    const el = await mount({ title: "Embedded", embedded: true });
    expect(el.hasAttribute("embedded")).toBe(true);
    // When embedded, render returns the header + body directly,
    // no <section class="marketing-panel"> wrapper.
    expect(el.querySelector(".marketing-panel")).toBeNull();
    // But the panel-body region IS still rendered.
    expect(el.querySelector(".panel-body")).not.toBeNull();
  });

  it("standalone (non-embedded) renders the marketing-panel section wrapper (Good)", async () => {
    const el = await mount({ title: "Standalone" });
    expect(el.querySelector(".marketing-panel")).not.toBeNull();
    expect(el.querySelector(".panel-body")).not.toBeNull();
  });
});
