// SPDX-Licence-Identifier: EUPL-1.2
//
// Tests for <lthn-plugin-view> per plans/code/lthn/desktop/views/RFC.plugin-views.md.
//
// Coverage per RFC §12:
//   - TestPluginView_SandboxAttributes_Good (MED-5 / §3.2)
//   - TestPluginView_SandboxAttributesRejected_Bad (MED-5 — the
//     defaultPluginIframeSandbox const is the contract; render path
//     ALWAYS uses it verbatim)
//   - TestPluginView_MountTimeout_Bad (CRIT-3 / §6.3)
//   - Standard four-spec wire shape per
//     design_lit_view_backend_wire_pattern: rows-replace, reject-keeps,
//     empty-keeps, fixtures-still-pass.

import { describe, it, expect, vi, beforeEach } from "vitest";

vi.mock("@desktop/marketplace/service", () => ({
  GetViewDescriptor: vi.fn(),
}));

import { GetViewDescriptor as MockedGetViewDescriptor } from "@desktop/marketplace/service";

import { mountWindow } from "../test/window-fixture";
import {
  defaultPluginIframeSandbox,
  PLUGIN_VIEW_MOUNT_TIMEOUT_MS,
  PLUGIN_VIEW_MOUNT_TIMEOUT_EVENT,
  type PluginViewDescriptor,
} from "./lthn-plugin-view";
// Side-effect: define the element so document.createElement("lthn-plugin-view") resolves.
import "./lthn-plugin-view";

const FIXTURE_DESCRIPTOR: PluginViewDescriptor = {
  id: "opencode",
  label: "Opencode",
  icon: "fa-terminal",
  group: "plugin",
  kind: "iframe",
  source: "/opencode",
  pluginCode: "opencode",
  loopbackPort: 4096,
  loopbackOrigin: "http://127.0.0.1:4096",
  capabilities: ["session-token"],
};

beforeEach(() => {
  (MockedGetViewDescriptor as unknown as { mockReset: () => void }).mockReset();
});

// ─── Sandbox attribute floor (MED-5 / §3.2) ───────────────────────────

describe("TestPluginView_SandboxAttributes_Good", () => {
  it("renders iframe with exactly the defaultPluginIframeSandbox floor", async () => {
    const { el } = await mountWindow<HTMLElement & { descriptor: PluginViewDescriptor }>(
      "lthn-plugin-view",
      { props: { descriptor: FIXTURE_DESCRIPTOR } },
    );
    // Force a re-render with the descriptor in place.
    await (el as unknown as { updateComplete: Promise<boolean> }).updateComplete;
    const iframe = el.querySelector("iframe");
    expect(iframe).not.toBeNull();
    expect(iframe?.getAttribute("sandbox")).toBe(defaultPluginIframeSandbox);
    // Explicit floor — confirm the three attrs present + nothing else.
    expect(iframe?.getAttribute("sandbox")).toBe(
      "allow-scripts allow-forms allow-same-origin",
    );
  });
});

describe("TestPluginView_SandboxAttributesRejected_Bad", () => {
  it("ignores caller attempts to widen the sandbox via descriptor extension", async () => {
    // The element's render path hardcodes defaultPluginIframeSandbox;
    // a hypothetical "extra" field on the descriptor is ignored. Pin
    // the contract by adding a junk property and asserting the
    // sandbox attribute is still the floor verbatim.
    const widened = {
      ...FIXTURE_DESCRIPTOR,
      // Intentional shape extension to prove the render path ignores
      // any sandbox-widening attempt from caller-supplied descriptor.
      sandbox: "allow-scripts allow-forms allow-same-origin allow-popups allow-top-navigation",
    } as PluginViewDescriptor & { sandbox?: string };
    const { el } = await mountWindow<HTMLElement & { descriptor: PluginViewDescriptor }>(
      "lthn-plugin-view",
      { props: { descriptor: widened } },
    );
    await (el as unknown as { updateComplete: Promise<boolean> }).updateComplete;
    const iframe = el.querySelector("iframe");
    expect(iframe?.getAttribute("sandbox")).toBe(defaultPluginIframeSandbox);
    expect(iframe?.getAttribute("sandbox")).not.toContain("allow-popups");
    expect(iframe?.getAttribute("sandbox")).not.toContain("allow-top-navigation");
  });
});

// ─── Mount timeout (CRIT-3 / §6.3) ─────────────────────────────────────

describe("TestPluginView_MountTimeout_Bad", () => {
  it("dispatches lthn:plugin-view:mount-timeout when iframe load never fires", async () => {
    vi.useFakeTimers();
    const events: CustomEvent[] = [];
    const listener = (e: Event) => {
      events.push(e as CustomEvent);
    };
    window.addEventListener(PLUGIN_VIEW_MOUNT_TIMEOUT_EVENT, listener);

    const { el } = await mountWindow<HTMLElement & { descriptor: PluginViewDescriptor }>(
      "lthn-plugin-view",
      { props: { descriptor: FIXTURE_DESCRIPTOR } },
    );

    // Iframe is in the DOM but we never fire 'load'. Advance past
    // the mount-timeout window.
    vi.advanceTimersByTime(PLUGIN_VIEW_MOUNT_TIMEOUT_MS + 10);

    // Allow any microtask scheduling from the timeout callback.
    await Promise.resolve();
    await Promise.resolve();

    window.removeEventListener(PLUGIN_VIEW_MOUNT_TIMEOUT_EVENT, listener);
    vi.useRealTimers();

    expect(events.length).toBeGreaterThan(0);
    expect(events[0]?.detail).toEqual({
      viewId: "",
      pluginCode: "opencode",
    });
    // Element should also surface the timed-out state in its render.
    await (el as unknown as { updateComplete: Promise<boolean> }).updateComplete;
    expect(el.textContent).toContain("failed to mount");
  });

  it("does NOT dispatch timeout when iframe load fires before the cap", async () => {
    vi.useFakeTimers();
    const events: CustomEvent[] = [];
    const listener = (e: Event) => {
      events.push(e as CustomEvent);
    };
    window.addEventListener(PLUGIN_VIEW_MOUNT_TIMEOUT_EVENT, listener);

    const { el } = await mountWindow<HTMLElement & { descriptor: PluginViewDescriptor }>(
      "lthn-plugin-view",
      { props: { descriptor: FIXTURE_DESCRIPTOR } },
    );
    const iframe = el.querySelector("iframe");
    iframe?.dispatchEvent(new Event("load"));

    vi.advanceTimersByTime(PLUGIN_VIEW_MOUNT_TIMEOUT_MS + 10);
    await Promise.resolve();

    window.removeEventListener(PLUGIN_VIEW_MOUNT_TIMEOUT_EVENT, listener);
    vi.useRealTimers();

    expect(events.length).toBe(0);
  });
});

// ─── Backend wire shape (design_lit_view_backend_wire_pattern) ────────

describe("lthn-plugin-view backend wire (4-spec contract)", () => {
  it("backend rows replace fixture when present (descriptor pulled from Go)", async () => {
    (MockedGetViewDescriptor as unknown as { mockResolvedValue: (v: unknown) => void })
      .mockResolvedValue({ OK: true, Value: FIXTURE_DESCRIPTOR });

    const { el } = await mountWindow<HTMLElement & { descriptor: PluginViewDescriptor; viewId: string }>(
      "lthn-plugin-view",
      { attrs: { "view-id": "opencode" } },
    );
    // connectedCallback awaits a dynamic import (microtask) + a
    // Promise from the mocked GetViewDescriptor; vi.dynamicImportSettled
    // resolves once every pending import + microtask flush completes.
    await vi.dynamicImportSettled();
    for (let i = 0; i < 5; i++) await Promise.resolve();
    await (el as unknown as { updateComplete: Promise<boolean> }).updateComplete;

    expect(el.descriptor?.id).toBe("opencode");
    expect(el.querySelector("iframe")).not.toBeNull();
  });

  it("backend rejection keeps the loading placeholder", async () => {
    (MockedGetViewDescriptor as unknown as { mockResolvedValue: (v: unknown) => void })
      .mockResolvedValue({ OK: false });

    const { el } = await mountWindow<HTMLElement & { descriptor: PluginViewDescriptor | null }>(
      "lthn-plugin-view",
      { attrs: { "view-id": "missing" } },
    );
    await Promise.resolve();
    await (el as unknown as { updateComplete: Promise<boolean> }).updateComplete;

    // Descriptor stays null → render path shows the loading placeholder
    // until the timeout fallback fires (the shell handles fallback).
    expect(el.descriptor).toBeNull();
  });

  it("empty backend response keeps the descriptor null (dev/headless friendly)", async () => {
    (MockedGetViewDescriptor as unknown as { mockResolvedValue: (v: unknown) => void })
      .mockResolvedValue({ OK: true });

    const { el } = await mountWindow<HTMLElement & { descriptor: PluginViewDescriptor | null }>(
      "lthn-plugin-view",
      { attrs: { "view-id": "opencode" } },
    );
    await Promise.resolve();
    await (el as unknown as { updateComplete: Promise<boolean> }).updateComplete;

    expect(el.descriptor).toBeNull();
  });

  it("pre-set descriptor wins (no backend round-trip)", async () => {
    (MockedGetViewDescriptor as unknown as { mockResolvedValue: (v: unknown) => void })
      .mockResolvedValue({ OK: true, Value: { id: "other" } });

    const { el } = await mountWindow<HTMLElement & { descriptor: PluginViewDescriptor }>(
      "lthn-plugin-view",
      { props: { descriptor: FIXTURE_DESCRIPTOR } },
    );
    await Promise.resolve();
    await (el as unknown as { updateComplete: Promise<boolean> }).updateComplete;

    expect(el.descriptor?.id).toBe("opencode");
    expect(MockedGetViewDescriptor).not.toHaveBeenCalled();
  });
});

// ─── Lit-kind path ─────────────────────────────────────────────────────

describe("lthn-plugin-view lit-kind branch", () => {
  it("instantiates the registered custom-element tag when descriptor.kind === lit", async () => {
    // Register a fake first-party tag to test the lit branch.
    if (!customElements.get("lthn-test-panel")) {
      customElements.define(
        "lthn-test-panel",
        class extends HTMLElement {
          connectedCallback() {
            this.textContent = "test-panel-mounted";
          }
        },
      );
    }
    const litDesc: PluginViewDescriptor = {
      id: "lthn",
      label: "Lethean",
      icon: "fa-cube",
      group: "plugin",
      kind: "lit",
      source: "lthn-test-panel",
      pluginCode: "lthn",
    };
    const { el } = await mountWindow<HTMLElement & { descriptor: PluginViewDescriptor }>(
      "lthn-plugin-view",
      { props: { descriptor: litDesc } },
    );
    await (el as unknown as { updateComplete: Promise<boolean> }).updateComplete;
    expect(el.querySelector("lthn-test-panel")).not.toBeNull();
  });

  it("falls back when the lit tag is not registered", async () => {
    const litDesc: PluginViewDescriptor = {
      id: "lthn",
      label: "Lethean",
      icon: "fa-cube",
      group: "plugin",
      kind: "lit",
      source: "lthn-not-registered-xyz",
      pluginCode: "lthn",
    };
    const { el } = await mountWindow<HTMLElement & { descriptor: PluginViewDescriptor }>(
      "lthn-plugin-view",
      { props: { descriptor: litDesc } },
    );
    await (el as unknown as { updateComplete: Promise<boolean> }).updateComplete;
    // Render shows the not-registered fallback message OR will fall
    // back through the timeout — either is acceptable since the timer
    // fires below in fake-timer specs. Here we assert the synchronous
    // render message at minimum.
    expect(el.textContent).toMatch(/not registered|failed to mount/);
  });
});
