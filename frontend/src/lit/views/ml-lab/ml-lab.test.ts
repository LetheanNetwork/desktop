// SPDX-Licence-Identifier: EUPL-1.2

import { describe, it, expect, beforeEach, vi } from "vitest";

vi.mock("../../api-fetch", () => ({
  apiFetch: vi.fn(),
}));

import { apiFetch } from "../../api-fetch";
import "./ml-lab";

interface MountedEl extends HTMLElement {
  tab: string;
  selectedModel: string;
  selectedRun: string;
  models: Array<{ filename: string; arch: string; fingerprint: string }>;
  runs:   Array<{ id: string; status: string; base_model: string }>;
  updateComplete: Promise<boolean>;
}

async function mount(): Promise<MountedEl> {
  const el = document.createElement("lthn-view-ml-lab") as MountedEl;
  document.body.appendChild(el);
  await el.updateComplete;
  await Promise.resolve();
  await el.updateComplete;
  return el;
}

function fetchOK(payload: unknown) {
  return { ok: true, status: 200, json: async () => payload, text: async () => "" };
}

describe("<lthn-view-ml-lab>", () => {
  beforeEach(() => {
    (apiFetch as unknown as { mockReset: () => void }).mockReset();
    document.body.innerHTML = "";
  });

  it("renders the four tab pills + ASK box (Good)", async () => {
    (apiFetch as unknown as { mockResolvedValue: (v: unknown) => void })
      .mockResolvedValue(fetchOK({}));
    const el = await mount();
    expect(el.textContent).toContain("Influx");
    expect(el.textContent).toContain("DuckDB");
    expect(el.textContent).toContain("Models");
    expect(el.textContent).toContain("LoRA");
    expect(el.querySelector('input[placeholder^="Ask"]')).toBeTruthy();
  });

  it("defaults to the Models tab (Good)", async () => {
    (apiFetch as unknown as { mockResolvedValue: (v: unknown) => void })
      .mockResolvedValue(fetchOK({}));
    const el = await mount();
    expect(el.tab).toBe("models");
  });

  it("loads sidebar models + runs + saved-queries on connectedCallback (Good)", async () => {
    const fakeFetch = (apiFetch as unknown as { mockImplementation: (fn: any) => void });
    fakeFetch.mockImplementation((path: string) => {
      if (path.includes("/models")) {
        return Promise.resolve(fetchOK({ models: [{ filename: "x.model", arch: "g4", fingerprint: "abc" }] }));
      }
      if (path.includes("/runs")) {
        return Promise.resolve(fetchOK({ runs: [{ id: "r1", status: "active", base_model: "x.model" }] }));
      }
      if (path.includes("/saved-queries")) {
        return Promise.resolve(fetchOK({ queries: [] }));
      }
      return Promise.resolve(fetchOK({}));
    });
    const el = await mount();
    expect(el.models.length).toBe(1);
    expect(el.runs.length).toBe(1);
    expect(el.textContent).toContain("x.model");
    expect(el.textContent).toContain("r1");
  });

  it("offline fallback — backend errors leave empty navigators (Ugly)", async () => {
    (apiFetch as unknown as { mockRejectedValue: (e: unknown) => void })
      .mockRejectedValue(new Error("no binding"));
    const el = await mount();
    expect(el.models.length).toBe(0);
    expect(el.runs.length).toBe(0);
    expect(el.textContent).toContain("no models loaded");
    expect(el.textContent).toContain("no runs known");
  });
});
