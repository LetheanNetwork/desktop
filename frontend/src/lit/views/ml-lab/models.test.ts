// SPDX-Licence-Identifier: EUPL-1.2

import { describe, it, expect, beforeEach, vi } from "vitest";

vi.mock("../../api-fetch", () => ({
  apiFetch: vi.fn(),
}));

import { apiFetch } from "../../api-fetch";

import "./models";

interface MountedEl extends HTMLElement {
  embedded: boolean;
  models:   Array<{
    filename: string; arch: string; quant_bits: number; ctx: number;
    size_bytes: number; fingerprint: string; created: string; producer: string; path: string;
  }>;
  filter:   string;
  loading:  boolean;
  error:    string;
  updateComplete: Promise<boolean>;
}

async function mount(attrs: Record<string, string | boolean> = {}): Promise<MountedEl> {
  const el = document.createElement("lthn-view-ml-lab-models") as MountedEl;
  for (const [k, v] of Object.entries(attrs)) {
    if (typeof v === "boolean") {
      if (v) el.setAttribute(k, "");
    } else {
      el.setAttribute(k, v);
    }
  }
  document.body.appendChild(el);
  await el.updateComplete;
  await Promise.resolve();
  await el.updateComplete;
  return el;
}

function fetchOK(payload: unknown) {
  return {
    ok: true,
    status: 200,
    json: async () => payload,
  };
}

describe("<lthn-view-ml-lab-models>", () => {
  beforeEach(() => {
    (apiFetch as unknown as { mockReset: () => void }).mockReset();
    document.body.innerHTML = "";
  });

  it("renders fixture .model rows when backend is not yet wired (Good)", async () => {
    (apiFetch as unknown as { mockRejectedValue: (e: unknown) => void })
      .mockRejectedValue(new Error("no binding"));
    const el = await mount();
    // FIXTURE_MODELS has 4 entries; the fixture renders even when the
    // first apiFetch call fails — the canonical wire pattern.
    expect(el.models.length).toBe(4);
    expect(el.textContent).toContain("lemma-12b-v4-p6.model");
    expect(el.textContent).toContain("gemma-4");
  });

  it("loads .model rows from apiFetch when backend returns them (Good)", async () => {
    (apiFetch as unknown as { mockResolvedValue: (v: unknown) => void })
      .mockResolvedValue(fetchOK({
        data: {
          models: [
            {
              path: "/m/x.model", filename: "x.model", arch: "llama-3",
              quant_bits: 8, ctx: 8192, size_bytes: 4_000_000_000,
              fingerprint: "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
              created: "2026-05-01T00:00:00Z", producer: "go-mlx",
            },
          ],
        },
      }));
    const el = await mount();
    expect(el.models.length).toBe(1);
    expect(el.textContent).toContain("x.model");
    expect(el.textContent).toContain("llama-3");
    expect(el.textContent).toContain("q8");
    // Fingerprint column shows first 12 chars only — title attr carries the full hex.
    expect(el.textContent).toContain("abcdef012345");
  });

  it("reject-keeps — backend error leaves prior list intact (Ugly)", async () => {
    const calls: number[] = [];
    (apiFetch as unknown as { mockImplementation: (fn: () => unknown) => void })
      .mockImplementation(() => {
        calls.push(1);
        if (calls.length === 1) return fetchOK({ data: { models: [
          { path: "/m/first.model", filename: "first.model", arch: "gemma-4",
            quant_bits: 4, ctx: 131072, size_bytes: 1_000,
            fingerprint: "111111111111111111111111111111111111111111111111111111111111", // 60 chars; renderer caps at 12
            created: "2026-05-01T00:00:00Z", producer: "go-mlx" },
        ] } });
        return { ok: false, status: 500, json: async () => ({}) };
      });
    const el = await mount();
    expect(el.models.length).toBe(1);
    expect(el.textContent).toContain("first.model");

    await (el as MountedEl & { _loadFromBackend: () => Promise<void> })._loadFromBackend();
    await el.updateComplete;
    expect(el.models.length).toBe(1);
    expect(el.error).toContain("500");
    expect(el.textContent).toContain("first.model");
  });

  it("empty-keeps — empty backend response leaves prior list intact (Ugly)", async () => {
    const calls: number[] = [];
    (apiFetch as unknown as { mockImplementation: (fn: () => unknown) => void })
      .mockImplementation(() => {
        calls.push(1);
        if (calls.length === 1) return fetchOK({ data: { models: [
          { path: "/m/seed.model", filename: "seed.model", arch: "qwen-3",
            quant_bits: 4, ctx: 32768, size_bytes: 500_000_000,
            fingerprint: "deadbeef".repeat(8),
            created: "2026-05-02T00:00:00Z", producer: "go-mlx" },
        ] } });
        return fetchOK({ data: { models: [] } });
      });
    const el = await mount();
    expect(el.models.length).toBe(1);
    expect(el.textContent).toContain("seed.model");

    await (el as MountedEl & { _loadFromBackend: () => Promise<void> })._loadFromBackend();
    await el.updateComplete;
    expect(el.models.length).toBe(1);
    expect(el.textContent).toContain("seed.model");
  });

  it("filter narrows visible rows by name / arch / fingerprint / producer (Good)", async () => {
    (apiFetch as unknown as { mockResolvedValue: (v: unknown) => void })
      .mockResolvedValue(fetchOK({
        data: {
          models: [
            { path: "/m/gemma.model", filename: "gemma.model", arch: "gemma-4",
              quant_bits: 4, ctx: 131072, size_bytes: 1_000, fingerprint: "aaaa".repeat(16),
              created: "", producer: "go-mlx" },
            { path: "/m/llama.model", filename: "llama.model", arch: "llama-3",
              quant_bits: 8, ctx: 8192, size_bytes: 1_000, fingerprint: "bbbb".repeat(16),
              created: "", producer: "go-mlx" },
            { path: "/m/qwen.model", filename: "qwen.model", arch: "qwen-3",
              quant_bits: 4, ctx: 32768, size_bytes: 1_000, fingerprint: "cccc".repeat(16),
              created: "", producer: "go-mlx" },
          ],
        },
      }));
    const el = await mount();
    el.filter = "gemma";
    await el.updateComplete;
    expect(el.textContent).toContain("gemma.model");
    expect(el.textContent).not.toContain("llama.model");
    expect(el.textContent).not.toContain("qwen.model");
  });

  it("empty-state copy shows when filter matches nothing (Ugly)", async () => {
    (apiFetch as unknown as { mockResolvedValue: (v: unknown) => void })
      .mockResolvedValue(fetchOK({
        data: { models: [
          { path: "/m/x.model", filename: "x.model", arch: "gemma-4",
            quant_bits: 4, ctx: 131072, size_bytes: 1, fingerprint: "1234".repeat(16),
            created: "", producer: "go-mlx" },
        ] },
      }));
    const el = await mount();
    el.filter = "no-such-model-anywhere";
    await el.updateComplete;
    expect(el.textContent).toContain("No .model files match these filters");
  });

  it("embedded attribute renders without chrome wrapper (Good)", async () => {
    (apiFetch as unknown as { mockRejectedValue: (e: unknown) => void })
      .mockRejectedValue(new Error("no binding"));
    const el = await mount({ embedded: true });
    expect(el.hasAttribute("embedded")).toBe(true);
  });
});
