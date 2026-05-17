// SPDX-Licence-Identifier: EUPL-1.2
//
// Coverage for the mountWindow / mountWindowLit helper pair.
//
// mountWindow is intentionally permissive — `T extends HTMLElement` so
// inline-shape narrowing works without LitElement contamination. The
// strict sibling mountWindowLit reasserts the LitElement contract for
// callers that want compile-time access to the full Lit surface.
//
// Tests cover:
//   - mountWindow happy path on a LitElement (default T = LitElement)
//   - mountWindow happy path on a non-Lit custom element (microtask flush fallback)
//   - mountWindow with explicit narrow T (inline shape)
//   - mountWindowLit happy path on a LitElement
//   - mountWindowLit throws when the element does not expose updateComplete

import { describe, it, expect, beforeAll } from "vitest";
import { LitElement, html } from "lit";
import { customElement, property } from "lit/decorators.js";
import { mountWindow, mountWindowLit } from "./window-fixture";

@customElement("wfx-lit-fixture")
class WfxLitFixture extends LitElement {
  @property({ type: String }) label = "default";
  override render() {
    return html`<span class="lbl">${this.label}</span>`;
  }
}

class WfxPlainFixture extends HTMLElement {
  connectedCallback(): void {
    const span = document.createElement("span");
    span.className = "plain";
    span.textContent = "plain-element";
    this.appendChild(span);
  }
}

beforeAll(() => {
  // LitElement is registered via the decorator; plain element registered
  // imperatively so the test file owns both tag names without depending
  // on import-side-effects.
  if (!customElements.get("wfx-plain-fixture")) {
    customElements.define("wfx-plain-fixture", WfxPlainFixture);
  }
});

describe("mountWindow", () => {
  it("mounts a LitElement and awaits its first render (default T)", async () => {
    const { host, el } = await mountWindow("wfx-lit-fixture");
    expect(el).toBeInstanceOf(WfxLitFixture);
    expect(host.querySelector("wfx-lit-fixture")).toBe(el);
    // Default T is LitElement — shadowRoot is reachable via the el handle.
    expect((el as LitElement).shadowRoot?.querySelector(".lbl")?.textContent).toBe("default");
  });

  it("mounts a non-LitElement custom element via the microtask flush fallback", async () => {
    const { host, el } = await mountWindow("wfx-plain-fixture");
    expect(el).toBeInstanceOf(WfxPlainFixture);
    expect(host.querySelector(".plain")?.textContent).toBe("plain-element");
  });

  it("respects an explicit inline-shape T for caller-side narrowing", async () => {
    const { el } = await mountWindow<HTMLElement & { label: string }>(
      "wfx-lit-fixture",
      { props: { label: "narrowed" } },
    );
    // label is typed on el without an as cast — the inline shape carries.
    expect(el.label).toBe("narrowed");
  });
});

describe("mountWindowLit", () => {
  it("mounts a LitElement and surfaces the strict LitElement type", async () => {
    const { el } = await mountWindowLit<WfxLitFixture>("wfx-lit-fixture", {
      props: { label: "strict" },
    });
    // el is typed as WfxLitFixture — both LitElement members and the
    // subclass `label` property are reachable without casts.
    await el.updateComplete;
    expect(el.label).toBe("strict");
    expect(el.shadowRoot?.querySelector(".lbl")?.textContent).toBe("strict");
  });

  it("throws when the element does not expose LitElement.updateComplete", async () => {
    await expect(mountWindowLit("wfx-plain-fixture")).rejects.toThrow(
      /does not expose LitElement\.updateComplete/,
    );
  });
});
