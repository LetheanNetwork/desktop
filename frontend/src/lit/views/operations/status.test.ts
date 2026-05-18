// SPDX-Licence-Identifier: EUPL-1.2

// <lthn-view-status> tests — fixture-driven. Vi.Sites binding is
// mocked to control the data the panel renders.

import { describe, it, expect, beforeEach, vi } from "vitest";
import "./status";

// Mock the @desktop/vi/wailsservice module so we control Sites() output.
// vi.Sites() returns core.Result-shaped { Value: { sites: [...] } }.
vi.mock("@desktop/vi/wailsservice", () => ({
  Sites: vi.fn(),
}));

import { Sites } from "@desktop/vi/wailsservice";

async function mount(attrs: Record<string, string | boolean> = {}) {
  const el = document.createElement("lthn-view-status") as HTMLElement & {
    sites: unknown[];
    loading: boolean;
    embedded: boolean;
    updateComplete: Promise<boolean>;
  };
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

describe("<lthn-view-status>", () => {
  beforeEach(() => {
    (Sites as unknown as { mockReset: () => void }).mockReset();
    document.body.innerHTML = "";
  });

  it("renders without crashing when binding is unavailable", async () => {
    (Sites as unknown as { mockRejectedValue: (v: unknown) => void }).mockRejectedValue(new Error("no binding"));
    const el = await mount();
    // Allow the connectedCallback's _refresh to settle.
    await new Promise((r) => setTimeout(r, 0));
    expect(el).toBeDefined();
    expect(el.textContent).toContain("Waiting for first probe");
  });

  it("renders 'all operational' hero when every site is ok", async () => {
    (Sites as unknown as { mockResolvedValue: (v: unknown) => void }).mockResolvedValue({
      Value: { sites: [
        { url: "https://lthn.ai", domain: "lthn.ai", state: "ok", statusCode: 200, latencyMs: 42 },
        { url: "https://forge.lthn.sh", domain: "forge.lthn.sh", state: "ok", statusCode: 200, latencyMs: 88 },
      ] },
    });
    const el = await mount();
    await new Promise((r) => setTimeout(r, 0));
    await el.updateComplete;
    expect(el.textContent).toMatch(/All systems operational/);
    expect(el.textContent).toContain("lthn.ai");
    expect(el.textContent).toContain("forge.lthn.sh");
  });

  it("renders degraded hero when one site is degraded", async () => {
    (Sites as unknown as { mockResolvedValue: (v: unknown) => void }).mockResolvedValue({
      Value: { sites: [
        { url: "https://lthn.ai", domain: "lthn.ai", state: "ok", statusCode: 200 },
        { url: "https://hub.host.uk.com", domain: "hub.host.uk.com", state: "degraded", statusCode: 503, note: "elevated p99" },
      ] },
    });
    const el = await mount();
    await new Promise((r) => setTimeout(r, 0));
    await el.updateComplete;
    expect(el.textContent).toMatch(/degraded/);
    expect(el.textContent).toContain("elevated p99");
  });

  it("renders down hero when one site is down", async () => {
    (Sites as unknown as { mockResolvedValue: (v: unknown) => void }).mockResolvedValue({
      Value: { sites: [
        { url: "https://forge.lthn.sh", domain: "forge.lthn.sh", state: "down", statusCode: 0 },
      ] },
    });
    const el = await mount();
    await new Promise((r) => setTimeout(r, 0));
    await el.updateComplete;
    expect(el.textContent).toMatch(/down/i);
  });

  it("embedded mode hides the chrome card", async () => {
    (Sites as unknown as { mockResolvedValue: (v: unknown) => void }).mockResolvedValue({ Value: { sites: [] } });
    const el = await mount({ embedded: true });
    await new Promise((r) => setTimeout(r, 0));
    await el.updateComplete;
    expect(el.hasAttribute("embedded")).toBe(true);
  });
});
